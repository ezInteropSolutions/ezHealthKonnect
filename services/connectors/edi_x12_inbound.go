// services/connectors/edi_x12_inbound.go
// EDI X12 Inbound Connector — polls a remote SFTP directory for X12 files
// (835 phase 1). Structurally mirrors sftp_inbound.go (real SFTP protocol
// via github.com/pkg/sftp, not shell-over-SSH — see that file's own header
// for why) but uses the config key names already live in the
// connectivity_types catalog for edi_x12_inbound (V69__EMR_Connectivity_Types.sql),
// not SFTP-generic names — remote_path/polling_interval_seconds instead of
// remote_dir/poll_interval_sec.
//
// No pre-splitting or transaction-set filtering happens here: a file may
// contain multiple ST...SE transaction sets, and processing/batch_splitter.go's
// splitEDITransactions already runs centrally on every connector's payload in
// processing/engine_message_processor.go — duplicating that here would be
// redundant. transaction_types is accepted in config (matching the catalog
// schema) but not yet enforced at the connector level in phase 1 — with only
// "835" supported end-to-end, filtering has no real effect yet; a genuine
// multi-transaction-type phase 2 should implement it then, not now.
//
// Configuration (see connectivity_types.config_schema for edi_x12_inbound):
//
//	transport                 string  Only "sftp" is implemented in phase 1;
//	                                  any other value fails Validate() with a
//	                                  clear error rather than silently no-op'ing
//	host                       string  Remote hostname or IP
//	port                       int     SSH port (default 22)
//	username                   string  SSH username
//	password                   string  SSH password
//	key_content                string  PEM private key (string form, for auth_type="key")
//	auth_type                  string  "password" | "key" (default: "password")
//	remote_path                string  Directory to poll (default: "/incoming")
//	file_pattern               string  Glob pattern for files (default: "*.edi")
//	transaction_types          []string  Accepted but not yet enforced — see file header
//	polling_interval_seconds   int     Polling interval in seconds (default: 300)
//	after_processing           string  "delete" | "archive" | "none" (default: "archive")
//	archive_dir                string  Directory to move processed files to (default: "<remote_path>/processed")
//	max_files_per_run          int     Max files fetched per poll cycle (default: 100)
//	connect_timeout            int     Seconds (default 10)
//	read_timeout                int    Seconds per file download (default 60)
package connectors

import (
	"context"
	"fmt"
	"log"
	"net"
	"path"
	"sort"
	"time"

	"ezhealthkonnect/models"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// EDIX12InboundConnector polls a remote SFTP directory for X12 EDI files.
type EDIX12InboundConnector struct {
	*BaseInboundConnector

	// config
	transport       string
	host            string
	port            int
	username        string
	password        string
	keyContent      string
	authType        string
	remotePath      string
	filePattern     string
	pollInterval    time.Duration
	afterProcessing string
	archiveDir      string
	maxFilesPerRun  int
	connectTimeout  time.Duration
	readTimeout     time.Duration

	// runtime
	sshConfig *ssh.ClientConfig
}

// NewEDIX12InboundConnector creates a production EDI X12 inbound connector
// (SFTP transport only in phase 1).
func NewEDIX12InboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "edi_x12_inbound",
		DisplayName:        "EDI X12 Inbound",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron": true,
			"supports_tls":  true, // TLS via SSH
			"supports_auth": true,
			"supports_sftp": true,
			// http/as2/999_ack are named future phases (see CLAUDE.md's EDI
			// section) — not implemented, deliberately not claimed here.
		},
	}
	return &EDIX12InboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses configuration and builds the SSH client config.
func (c *EDIX12InboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.transport = cfg.GetString("transport")
	if c.transport == "" {
		c.transport = "sftp"
	}
	c.host = cfg.GetString("host")
	c.port = cfg.GetInt("port")
	if c.port == 0 {
		c.port = 22
	}
	c.username = cfg.GetString("username")
	c.password = cfg.GetString("password")
	c.keyContent = cfg.GetString("key_content")
	c.authType = cfg.GetString("auth_type")
	if c.authType == "" {
		c.authType = "password"
	}
	c.remotePath = cfg.GetString("remote_path")
	if c.remotePath == "" {
		c.remotePath = "/incoming"
	}
	c.filePattern = cfg.GetString("file_pattern")
	if c.filePattern == "" {
		c.filePattern = "*.edi"
	}

	pollSec := cfg.GetInt("polling_interval_seconds")
	if pollSec == 0 {
		pollSec = 300
	}
	c.pollInterval = time.Duration(pollSec) * time.Second

	c.afterProcessing = cfg.GetString("after_processing")
	if c.afterProcessing == "" {
		c.afterProcessing = "archive"
	}
	c.archiveDir = cfg.GetString("archive_dir")
	if c.archiveDir == "" {
		c.archiveDir = path.Join(c.remotePath, "processed")
	}

	c.maxFilesPerRun = cfg.GetInt("max_files_per_run")
	if c.maxFilesPerRun == 0 {
		c.maxFilesPerRun = 100
	}

	connectSec := cfg.GetInt("connect_timeout")
	if connectSec == 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	readSec := cfg.GetInt("read_timeout")
	if readSec == 0 {
		readSec = 60
	}
	c.readTimeout = time.Duration(readSec) * time.Second

	if c.transport == "sftp" {
		authMethods, err := c.buildAuthMethods()
		if err != nil {
			return NewConnectorError(c.GetMetadata().TypeName, "initialize", err, false)
		}
		c.sshConfig = &ssh.ClientConfig{
			User:            c.username,
			Auth:            authMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
			Timeout:         c.connectTimeout,
		}
	}

	c.SetMetadata("transport", c.transport)
	c.SetMetadata("host", c.host)
	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("remote_path", c.remotePath)
	c.SetMetadata("poll_interval", c.pollInterval.String())
	return nil
}

// Validate checks configuration validity. transport values other than
// "sftp" are rejected explicitly here — the catalog's own config_schema
// still lists http/as2 as selectable (forward-compat for later phases), so
// this is where phase 1's actual capability boundary is enforced, with a
// clear message, rather than silently no-op'ing on Start().
func (c *EDIX12InboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	if c.transport != "sftp" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("transport %q is not implemented in phase 1 — only \"sftp\" is supported", c.transport), false)
	}
	if c.host == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("host is required"), false)
	}
	if c.username == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("username is required"), false)
	}
	switch c.afterProcessing {
	case "delete", "archive", "none":
	default:
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("after_processing must be 'delete', 'archive', or 'none'"), false)
	}
	return nil
}

// TestConnection verifies SFTP connectivity and that remote_path is accessible.
func (c *EDIX12InboundConnector) TestConnection(ctx context.Context) error {
	conn, err := c.dialSSH(ctx)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection",
			fmt.Errorf("SFTP subsystem unavailable: %w", err), true)
	}
	defer sftpClient.Close()

	if _, err := sftpClient.Stat(c.remotePath); err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection",
			fmt.Errorf("remote_path %q not accessible: %w", c.remotePath, err), false)
	}
	return nil
}

// Start begins polling the remote directory on an interval.
func (c *EDIX12InboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	c.SetState(StateRunning)
	log.Printf("[edi_x12_inbound] started polling %s:%s every %s", c.host, c.remotePath, c.pollInterval)

	go func() {
		ticker := time.NewTicker(c.pollInterval)
		defer ticker.Stop()

		stopCh := c.GetStopChannel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				if err := c.pollOnce(ctx, messageChan); err != nil {
					log.Printf("[edi_x12_inbound] poll error: %v", err)
					c.RecordError(err)
				}
			}
		}
	}()

	return nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func (c *EDIX12InboundConnector) pollOnce(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	conn, err := c.dialSSH(ctx)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("open SFTP session: %w", err)
	}
	defer sftpClient.Close()

	entries, err := sftpClient.ReadDir(c.remotePath)
	if err != nil {
		return fmt.Errorf("list %s: %w", c.remotePath, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matched, _ := path.Match(c.filePattern, entry.Name())
		if matched {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names) // deterministic processing order
	if len(names) > c.maxFilesPerRun {
		names = names[:c.maxFilesPerRun]
	}

	log.Printf("[edi_x12_inbound] found %d file(s) to process", len(names))
	c.IncrementMessagesReceived()

	for _, name := range names {
		filePath := path.Join(c.remotePath, name)

		content, err := sftpDownloadWithTimeout(sftpClient, filePath, c.readTimeout)
		if err != nil {
			log.Printf("[edi_x12_inbound] download error for %s: %v", filePath, err)
			continue
		}

		msg := &models.InboundMessage{
			MessageID:      generateSFTPMessageID(filePath),
			Content:        content,
			SourceType:     "edi_x12_sftp",
			SourceEndpoint: fmt.Sprintf("%s:%d", c.host, c.port),
			MessageType:    "EDI",
			ReceivedAt:     time.Now(),
			SourceMetadata: map[string]string{
				"sftp_host":     c.host,
				"sftp_path":     filePath,
				"sftp_filename": name,
			},
		}

		select {
		case messageChan <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}

		if err := c.postProcess(sftpClient, filePath, name); err != nil {
			log.Printf("[edi_x12_inbound] post-process error for %s: %v", filePath, err)
		}
	}

	return nil
}

func (c *EDIX12InboundConnector) postProcess(client *sftp.Client, filePath, filename string) error {
	switch c.afterProcessing {
	case "delete":
		return client.Remove(filePath)
	case "archive":
		if err := client.MkdirAll(c.archiveDir); err != nil {
			return fmt.Errorf("mkdir %s: %w", c.archiveDir, err)
		}
		destPath := path.Join(c.archiveDir, filename)
		if err := client.PosixRename(filePath, destPath); err != nil {
			return fmt.Errorf("move %s -> %s: %w", filePath, destPath, err)
		}
		return nil
	default: // "none"
		return nil
	}
}

func (c *EDIX12InboundConnector) dialSSH(ctx context.Context) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	d := &net.Dialer{Timeout: c.connectTimeout}
	netConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh tcp dial: %w", err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, c.sshConfig)
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

func (c *EDIX12InboundConnector) buildAuthMethods() ([]ssh.AuthMethod, error) {
	switch c.authType {
	case "key":
		if c.keyContent == "" {
			return nil, fmt.Errorf("key_content is required when auth_type is 'key'")
		}
		signer, err := ssh.ParsePrivateKey([]byte(c.keyContent))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default:
		return []ssh.AuthMethod{ssh.Password(c.password)}, nil
	}
}
