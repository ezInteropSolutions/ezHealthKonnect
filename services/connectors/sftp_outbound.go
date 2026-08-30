// services/connectors/sftp_outbound.go
// SFTP Outbound Connector — uploads files to a remote SFTP server.
//
// Uses the real SFTP protocol via github.com/pkg/sftp (built on top of the
// same golang.org/x/crypto/ssh connection already established) — NOT shell
// commands over SSH exec. An earlier version of this connector hand-rolled
// the legacy SCP protocol by writing "C0644 <size> <name>\n" headers and
// running `scp -qt <dir>` as a literal SSH exec command; that requires the
// remote server to allow arbitrary shell command execution for the SFTP
// user. Confirmed via a live test against a standard, properly-locked-down
// SFTP server (the common atmoz/sftp Docker test image, configured the way
// real production SFTP endpoints — banks, healthcare partners, AWS Transfer
// Family, etc. — normally are) that this fails outright: the server rejects
// the exec request with "This service allows sftp connections only." The
// real SFTP protocol (what this file now uses) is exactly what such servers
// exist to serve, and works correctly against them.
//
// Authentication:
//
//	password   — username + password
//	key        — PEM private key (key_file path or key_content string)
//
// Configuration:
//
//	host            string  Remote hostname or IP
//	port            int     SSH port (default 22)
//	username        string  SSH username
//	password        string  SSH password (for password auth)
//	key_file        string  Path to PEM private key (for key auth)
//	key_content     string  PEM private key as string (for key auth)
//	auth_type       string  "password" | "key" (default: "password")
//	remote_dir      string  Remote directory path (default: "/upload")
//	filename_pattern string Template for the remote filename, e.g.
//	                        "{interface_id}/{message_id}_{timestamp}.hl7" —
//	                        same placeholder convention and sanitizer as
//	                        aws_s3_outbound.go/azure_blob_outbound.go's
//	                        key_pattern. Takes priority over filename_field/
//	                        filename_prefix/file_extension when set. Any
//	                        subdirectories in the pattern are created
//	                        automatically via the SFTP client's MkdirAll.
//	filename_field  string  Message metadata key used as remote filename
//	                        (legacy — used only when filename_pattern is unset)
//	filename_prefix string  Static prefix for generated filenames (legacy)
//	file_extension  string  Extension added to generated filenames (legacy, default: ".dat")
//	connect_timeout int     Seconds (default 10)
//	write_timeout   int     Seconds (default 60)
package connectors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"path"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPOutboundConnector uploads files to remote SFTP servers.
type SFTPOutboundConnector struct {
	*BaseOutboundConnector

	// config
	host            string
	port            int
	username        string
	password        string
	keyFile         string
	keyContent      string
	authType        string
	remoteDir       string
	filenamePattern string
	filenameField   string
	filenamePrefix  string
	fileExtension   string
	connectTimeout  time.Duration
	writeTimeout    time.Duration

	// runtime
	mu        sync.Mutex
	sshConfig *ssh.ClientConfig
}

// NewSFTPOutboundConnector creates a production SFTP outbound connector.
func NewSFTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sftp_outbound",
		DisplayName:        "SFTP File Uploader",
		Version:            "2.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": true,
			"supports_tls":   true, // TLS via SSH
			"supports_auth":  true,
		},
	}
	return &SFTPOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses configuration and builds the SSH client config.
func (c *SFTPOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
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
	c.keyFile = cfg.GetString("key_file")
	c.keyContent = cfg.GetString("key_content")
	c.authType = cfg.GetString("auth_type")
	if c.authType == "" {
		c.authType = "password"
	}
	c.remoteDir = cfg.GetString("remote_dir")
	if c.remoteDir == "" {
		c.remoteDir = "/upload"
	}
	c.filenamePattern = cfg.GetString("filename_pattern")
	c.filenameField = cfg.GetString("filename_field")
	c.filenamePrefix = cfg.GetString("filename_prefix")
	if c.filenamePrefix == "" {
		c.filenamePrefix = "msg_"
	}
	c.fileExtension = cfg.GetString("file_extension")
	if c.fileExtension == "" {
		c.fileExtension = ".dat"
	}

	connectSec := cfg.GetInt("connect_timeout")
	if connectSec == 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	writeSec := cfg.GetInt("write_timeout")
	if writeSec == 0 {
		writeSec = 60
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

	// Build SSH auth
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

	c.SetMetadata("host", c.host)
	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	c.SetMetadata("remote_dir", c.remoteDir)
	return nil
}

// Validate checks configuration validity.
func (c *SFTPOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
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
	if c.authType == "password" && c.password == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("password required when auth_type is 'password'"), false)
	}
	if c.authType == "key" && c.keyFile == "" && c.keyContent == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("key_file or key_content required when auth_type is 'key'"), false)
	}
	return nil
}

// TestConnection dials SSH and opens a real SFTP session (not just a bare SSH
// connection) — this is what actually proves the server accepts SFTP, since
// an SSH TCP/handshake can succeed even against a server that later rejects
// the actual file-transfer subsystem for other reasons.
func (c *SFTPOutboundConnector) TestConnection(ctx context.Context) error {
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
	return nil
}

// Send uploads the message content as a file via real SFTP.
func (c *SFTPOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	if message.Content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	filename := c.resolveFilename(message)
	remotePath := path.Join(c.remoteDir, filename)
	content := []byte(message.Content)

	conn, err := c.dialSSH(ctx)
	if err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}
	defer conn.Close()

	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		wrapped := fmt.Errorf("open SFTP session: %w", err)
		c.RecordError(wrapped)
		return failResult(message.MessageID, start, wrapped), wrapped
	}
	defer sftpClient.Close()

	if err := sftpUploadWithTimeout(sftpClient, c.remoteDir, remotePath, content, c.writeTimeout); err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}

	c.IncrementMessagesSent()
	log.Printf("[sftp_outbound] uploaded %d bytes → %s:%s", len(content), c.host, remotePath)

	return &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Metadata:   map[string]interface{}{"remote_path": remotePath, "bytes": len(content)},
	}, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func (c *SFTPOutboundConnector) dialSSH(ctx context.Context) (*ssh.Client, error) {
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

func (c *SFTPOutboundConnector) buildAuthMethods() ([]ssh.AuthMethod, error) {
	switch c.authType {
	case "key":
		var pemBytes []byte
		if c.keyContent != "" {
			pemBytes = []byte(c.keyContent)
		} else {
			return nil, fmt.Errorf("key_file reading requires filesystem access — set key_content instead")
		}
		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	default: // password
		return []ssh.AuthMethod{ssh.Password(c.password)}, nil
	}
}

// resolveFilename builds the remote filename for a message. filename_pattern
// (a real template — {message_id}/{interface_id}/{timestamp}/{date}/{time},
// same convention and sanitizer as aws_s3_outbound.go/azure_blob_outbound.go's
// key_pattern) takes priority when set; the legacy filename_field/prefix/
// extension behavior is preserved as a fallback for configs that don't set it.
func (c *SFTPOutboundConnector) resolveFilename(message *models.OutboundMessage) string {
	if c.filenamePattern != "" {
		return c.resolvePatternFilename(message)
	}

	if c.filenameField != "" && message.Metadata != nil {
		if name, ok := message.Metadata[c.filenameField]; ok && name != "" {
			return name
		}
	}
	ts := time.Now().UTC().Format("20060102_150405")
	msgID := message.MessageID
	if len(msgID) > 8 {
		msgID = msgID[:8]
	}
	return fmt.Sprintf("%s%s_%s%s", c.filenamePrefix, ts, msgID, c.fileExtension)
}

// resolvePatternFilename renders filename_pattern's placeholders for one
// message, appending an extension inferred from content type when the
// pattern doesn't already specify one — identical behavior to
// aws_s3_outbound.go's buildKey/azure_blob_outbound.go's buildBlobName.
func (c *SFTPOutboundConnector) resolvePatternFilename(message *models.OutboundMessage) string {
	name := c.filenamePattern
	now := time.Now()

	replacements := map[string]string{
		"{timestamp}":    now.Format("20060102_150405"),
		"{date}":         now.Format("20060102"),
		"{time}":         now.Format("150405"),
		"{message_id}":   sanitizeObjectKeySegment(message.MessageID),
		"{interface_id}": sanitizeObjectKeySegment(message.InterfaceID),
	}
	for placeholder, value := range replacements {
		name = strings.ReplaceAll(name, placeholder, value)
	}

	if path.Ext(name) == "" {
		name += extensionForContentType(contentTypeOrDefault("", message.ContentType))
	}
	return name
}

// sftpUploadWithTimeout creates any missing subdirectories under remoteDir
// (filename_pattern can include them, e.g. "{interface_id}/{message_id}.hl7")
// and writes content to remotePath via the real SFTP protocol, bounded by
// timeout. The blocking SFTP calls run in a goroutine so a hung connection
// can still be reported as a timeout rather than blocking Send() forever.
func sftpUploadWithTimeout(client *sftp.Client, remoteDir, remotePath string, content []byte, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		if targetDir := path.Dir(remotePath); targetDir != "." && targetDir != remoteDir {
			if err := client.MkdirAll(targetDir); err != nil {
				done <- fmt.Errorf("mkdir -p %s: %w", targetDir, err)
				return
			}
		}

		f, err := client.Create(remotePath)
		if err != nil {
			done <- fmt.Errorf("create %s: %w", remotePath, err)
			return
		}
		defer f.Close()

		if _, err := io.Copy(f, bytes.NewReader(content)); err != nil {
			done <- fmt.Errorf("write %s: %w", remotePath, err)
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("SFTP upload timed out after %s", timeout)
	}
}

// failResult creates a failed DeliveryResult.
func failResult(messageID string, start time.Time, err error) *DeliveryResult {
	return &DeliveryResult{
		Success:      false,
		MessageID:    messageID,
		Timestamp:    time.Now(),
		ErrorMessage: err.Error(),
		DurationMs:   time.Since(start).Milliseconds(),
	}
}
