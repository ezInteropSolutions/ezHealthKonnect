// services/connectors/edi_x12_inbound.go
// EDI X12 Inbound Connector — polls a remote SFTP directory for X12 files
// (835 phase 1).
//
// The actual SSH/SFTP mechanics (dial, auth, directory listing, download,
// archive/delete, poll loop) live in sftp_poller.go, shared with
// sftp_inbound.go — a real, confirmed duplication between the two files
// (this connector was originally built by "structurally mirroring"
// sftp_inbound.go rather than sharing it — every piece of the actual SSH/
// SFTP handling was copy-pasted with renamed fields) was extracted into
// that file during this round. This connector's own fields still hold its
// own config values directly, using the key names already live in the
// connectivity_types catalog for edi_x12_inbound
// (V69__EMR_Connectivity_Types.sql) — remote_path/polling_interval_seconds
// instead of SFTP-generic remote_dir/poll_interval_sec — plus the
// phase-1-capability-boundary transport check no other SFTP connector
// needs; toSFTPPollerConfig() below is the one place they're assembled
// into the shared config shape.
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
	"path"
	"time"

	"ezhealthkonnect/models"

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
		authMethods, err := buildSFTPAuthMethods(c.authType, c.password, c.keyContent)
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

// toSFTPPollerConfig assembles this connector's own fields into the shape
// sftp_poller.go's shared functions take — see sftp_inbound.go's own
// toSFTPPollerConfig for why this seam exists instead of a shared struct
// with a shared set of field names.
func (c *EDIX12InboundConnector) toSFTPPollerConfig() sftpPollerConfig {
	return sftpPollerConfig{
		Host:            c.host,
		Port:            c.port,
		SSHConfig:       c.sshConfig,
		RemoteDir:       c.remotePath,
		FilePattern:     c.filePattern,
		MaxFilesPerRun:  c.maxFilesPerRun,
		ConnectTimeout:  c.connectTimeout,
		ReadTimeout:     c.readTimeout,
		AfterProcessing: c.afterProcessing,
		ArchiveDir:      c.archiveDir,
		SourceType:      "edi_x12_sftp",
		MessageType:     "EDI",
		LogPrefix:       "[edi_x12_inbound]",
	}
}

// TestConnection verifies SFTP connectivity and that remote_path is accessible.
func (c *EDIX12InboundConnector) TestConnection(ctx context.Context) error {
	if retryable, err := testSFTPDirectory(ctx, c.toSFTPPollerConfig()); err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, retryable)
	}
	return nil
}

// Start begins polling the remote directory on an interval.
func (c *EDIX12InboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	c.SetState(StateRunning)
	log.Printf("[edi_x12_inbound] started polling %s:%s every %s", c.host, c.remotePath, c.pollInterval)

	runSFTPPollLoop(ctx, c.GetStopChannel(), c.pollInterval, "[edi_x12_inbound]",
		func(ctx context.Context) error {
			return pollSFTPOnce(ctx, c.toSFTPPollerConfig(), messageChan, c.IncrementMessagesReceived)
		},
		c.RecordError,
	)

	return nil
}
