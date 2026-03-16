// services/connectors/sftp_inbound.go
// SFTP Inbound Connector — polls a remote directory for new files via SSH exec.
//
// The connector connects over SSH and uses shell commands (find, cat, mv/rm) to
// discover, download, and optionally archive/delete processed files.
// This approach requires no additional Go dependencies beyond golang.org/x/crypto/ssh.
//
// Configuration:
//
//	host              string   Remote hostname or IP
//	port              int      SSH port (default 22)
//	username          string   SSH username
//	password          string   SSH password
//	key_content       string   PEM private key (string form)
//	auth_type         string   "password" | "key" (default: "password")
//	remote_dir        string   Directory to poll (default: "/inbox")
//	file_pattern      string   Shell glob pattern for files (default: "*.hl7")
//	poll_interval_sec int      Polling interval in seconds (default: 30)
//	after_processing  string   "delete" | "archive" | "none" (default: "archive")
//	archive_dir       string   Directory to move processed files to (default: "/inbox/processed")
//	max_files_per_run int      Max files fetched per poll cycle (default: 100)
//	connect_timeout   int      Seconds (default 10)
//	read_timeout      int      Seconds per file download (default 60)
package connectors

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"path"
	"strings"
	"time"

	"ezhealthkonnect/models"

	"golang.org/x/crypto/ssh"
)

// SFTPInboundConnector polls a remote SFTP directory for new files.
type SFTPInboundConnector struct {
	*BaseInboundConnector

	// config
	host            string
	port            int
	username        string
	password        string
	keyContent      string
	authType        string
	remoteDir       string
	filePattern     string
	pollInterval    time.Duration
	afterProcessing string // "delete" | "archive" | "none"
	archiveDir      string
	maxFilesPerRun  int
	connectTimeout  time.Duration
	readTimeout     time.Duration

	// runtime
	sshConfig *ssh.ClientConfig
}

// NewSFTPInboundConnector creates a production SFTP inbound connector.
func NewSFTPInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sftp_inbound",
		DisplayName:        "SFTP File Poller",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_tls":              true, // TLS via SSH
			"supports_auth":             true,
			"supports_after_processing": true,
		},
	}
	return &SFTPInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses configuration and builds the SSH client config.
func (c *SFTPInboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

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
	c.remoteDir = cfg.GetString("remote_dir")
	if c.remoteDir == "" {
		c.remoteDir = "/inbox"
	}
	c.filePattern = cfg.GetString("file_pattern")
	if c.filePattern == "" {
		c.filePattern = "*.hl7"
	}

	pollSec := cfg.GetInt("poll_interval_sec")
	if pollSec == 0 {
		pollSec = 30
	}
	c.pollInterval = time.Duration(pollSec) * time.Second

	c.afterProcessing = cfg.GetString("after_processing")
	if c.afterProcessing == "" {
		c.afterProcessing = "archive"
	}
	c.archiveDir = cfg.GetString("archive_dir")
	if c.archiveDir == "" {
		c.archiveDir = path.Join(c.remoteDir, "processed")
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

	// Build SSH auth
	authMethods, err := c.buildSFTPInboundAuthMethods()
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize", err, false)
	}
	c.sshConfig = &ssh.ClientConfig{
		User:            c.username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         c.connectTimeout,
	}

	c.SetMetadata("host", c.host)
	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("remote_dir", c.remoteDir)
	c.SetMetadata("poll_interval", c.pollInterval.String())
	return nil
}

// Validate checks configuration validity.
func (c *SFTPInboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
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

// TestConnection verifies SSH connectivity.
func (c *SFTPInboundConnector) TestConnection(ctx context.Context) error {
	conn, err := c.dialSSHInbound(ctx)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	defer conn.Close()

	// Verify the remote directory exists
	out, err := sshExec(conn, fmt.Sprintf("test -d %s && echo ok", c.remoteDir), c.connectTimeout)
	if err != nil || strings.TrimSpace(out) != "ok" {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection",
			fmt.Errorf("remote_dir '%s' not accessible: %v", c.remoteDir, err), false)
	}
	return nil
}

// Start begins polling the remote directory on an interval.
func (c *SFTPInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	c.SetState(StateRunning)
	log.Printf("[sftp_inbound] started polling %s:%s every %s", c.host, c.remoteDir, c.pollInterval)

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
					log.Printf("[sftp_inbound] poll error: %v", err)
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

// pollOnce connects, lists files, downloads and processes each one.
func (c *SFTPInboundConnector) pollOnce(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	conn, err := c.dialSSHInbound(ctx)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer conn.Close()

	// List matching files
	listCmd := fmt.Sprintf("find %s -maxdepth 1 -type f -name '%s' | head -%d",
		c.remoteDir, c.filePattern, c.maxFilesPerRun)
	listOut, err := sshExec(conn, listCmd, c.connectTimeout)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}

	files := filterNonEmpty(strings.Split(strings.TrimSpace(listOut), "\n"))
	if len(files) == 0 {
		return nil
	}

	log.Printf("[sftp_inbound] found %d file(s) to process", len(files))
	c.IncrementMessagesReceived()

	for _, filePath := range files {
		filePath = strings.TrimSpace(filePath)
		if filePath == "" {
			continue
		}

		content, err := c.downloadFile(conn, filePath)
		if err != nil {
			log.Printf("[sftp_inbound] download error for %s: %v", filePath, err)
			continue
		}

		msg := &models.InboundMessage{
			MessageID:      generateSFTPMessageID(filePath),
			Content:        content,
			SourceType:     "sftp",
			SourceEndpoint: fmt.Sprintf("%s:%d", c.host, c.port),
			ReceivedAt:     time.Now(),
			SourceMetadata: map[string]string{
				"sftp_host":     c.host,
				"sftp_path":     filePath,
				"sftp_filename": path.Base(filePath),
			},
		}

		select {
		case messageChan <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Post-processing
		if err := c.postProcess(conn, filePath); err != nil {
			log.Printf("[sftp_inbound] post-process error for %s: %v", filePath, err)
		}
	}

	return nil
}

// downloadFile fetches remote file content via SSH cat.
func (c *SFTPInboundConnector) downloadFile(conn *ssh.Client, remotePath string) (string, error) {
	out, err := sshExec(conn, fmt.Sprintf("cat %s", remotePath), c.readTimeout)
	if err != nil {
		return "", fmt.Errorf("cat %s: %w", remotePath, err)
	}
	return out, nil
}

// postProcess moves or deletes the remote file after successful delivery.
func (c *SFTPInboundConnector) postProcess(conn *ssh.Client, filePath string) error {
	switch c.afterProcessing {
	case "delete":
		_, err := sshExec(conn, fmt.Sprintf("rm -f %s", filePath), c.connectTimeout)
		return err
	case "archive":
		// Ensure archive dir exists then move
		mkdirCmd := fmt.Sprintf("mkdir -p %s && mv %s %s/",
			c.archiveDir, filePath, c.archiveDir)
		_, err := sshExec(conn, mkdirCmd, c.connectTimeout)
		return err
	default: // "none"
		return nil
	}
}

func (c *SFTPInboundConnector) dialSSHInbound(ctx context.Context) (*ssh.Client, error) {
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

func (c *SFTPInboundConnector) buildSFTPInboundAuthMethods() ([]ssh.AuthMethod, error) {
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

// sshExec runs a command on the SSH connection and returns stdout.
func sshExec(client *ssh.Client, cmd string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout

	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	select {
	case err := <-done:
		return stdout.String(), err
	case <-time.After(timeout):
		return "", fmt.Errorf("ssh exec timed out after %s: %s", timeout, cmd)
	}
}

// generateSFTPMessageID produces a reproducible message ID from the file path.
func generateSFTPMessageID(filePath string) string {
	ts := time.Now().UTC().Format("20060102150405")
	base := path.Base(filePath)
	return fmt.Sprintf("sftp_%s_%s", ts, base)
}

// filterNonEmpty filters empty strings from a slice.
func filterNonEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
