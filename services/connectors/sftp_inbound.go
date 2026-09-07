// services/connectors/sftp_inbound.go
// SFTP Inbound Connector — polls a remote directory for new files.
//
// Uses the real SFTP protocol via github.com/pkg/sftp (built on top of the
// same golang.org/x/crypto/ssh connection already established) — NOT shell
// commands over SSH exec. An earlier version of this connector ran shell
// commands (find/cat/rm/mkdir+mv) over an SSH session; that requires the
// remote server to allow arbitrary shell command execution for the SFTP
// user. Confirmed via a live test against a standard, properly-locked-down
// SFTP server (see sftp_outbound.go's file header for the exact error and
// what it means) that this fails outright against real SFTP-only servers.
// The real SFTP protocol (what this file now uses) is exactly what such
// servers exist to serve.
//
// The actual SSH/SFTP mechanics (dial, auth, directory listing, download,
// archive/delete, poll loop) live in sftp_poller.go, shared with
// edi_x12_inbound.go — this connector's own fields still hold its own
// config values directly (unchanged, so existing field-level tests keep
// working); toSFTPPollerConfig() below is the one place they're assembled
// into the shared config shape at the point pollSFTPOnce/testSFTPDirectory
// actually need it.
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
//	file_pattern      string   Glob pattern for files (default: "*.hl7")
//	poll_interval_sec int      Polling interval in seconds (default: 30)
//	after_processing  string   "delete" | "archive" | "none" (default: "archive")
//	archive_dir       string   Directory to move processed files to (default: "/inbox/processed")
//	max_files_per_run int      Max files fetched per poll cycle (default: 100)
//	connect_timeout   int      Seconds (default 10)
//	read_timeout      int      Seconds per file download (default 60)
package connectors

import (
	"context"
	"fmt"
	"log"
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
		Version:            "2.0.0",
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

// toSFTPPollerConfig assembles this connector's own fields into the shape
// sftp_poller.go's shared functions take — the one seam between this
// connector's own config surface and edi_x12_inbound.go's, which otherwise
// differ in field names/defaults (remote_dir vs remote_path,
// poll_interval_sec vs polling_interval_seconds).
func (c *SFTPInboundConnector) toSFTPPollerConfig() sftpPollerConfig {
	return sftpPollerConfig{
		Host:            c.host,
		Port:            c.port,
		SSHConfig:       c.sshConfig,
		RemoteDir:       c.remoteDir,
		FilePattern:     c.filePattern,
		MaxFilesPerRun:  c.maxFilesPerRun,
		ConnectTimeout:  c.connectTimeout,
		ReadTimeout:     c.readTimeout,
		AfterProcessing: c.afterProcessing,
		ArchiveDir:      c.archiveDir,
		SourceType:      "sftp",
		MessageType:     "",
		LogPrefix:       "[sftp_inbound]",
	}
}

// TestConnection verifies SFTP connectivity and that remote_dir is accessible.
func (c *SFTPInboundConnector) TestConnection(ctx context.Context) error {
	if retryable, err := testSFTPDirectory(ctx, c.toSFTPPollerConfig()); err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, retryable)
	}
	return nil
}

// Start begins polling the remote directory on an interval.
func (c *SFTPInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	c.SetState(StateRunning)
	log.Printf("[sftp_inbound] started polling %s:%s every %s", c.host, c.remoteDir, c.pollInterval)

	runSFTPPollLoop(ctx, c.GetStopChannel(), c.pollInterval, "[sftp_inbound]",
		func(ctx context.Context) error {
			return pollSFTPOnce(ctx, c.toSFTPPollerConfig(), messageChan, c.IncrementMessagesReceived)
		},
		c.RecordError,
	)

	return nil
}

// filterNonEmpty filters empty/whitespace-only strings from a slice. No
// longer used by this file directly (file listing now comes from
// sftp.Client.ReadDir, not shell output that needed splitting/trimming), but
// kept — it's covered by its own tests in connector_test.go as a standalone
// utility.
func filterNonEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
