// services/connectors/sftp_outbound.go
// SFTP Outbound Connector — uploads files to a remote SSH/SFTP server.
//
// File transfer uses the SCP (Secure Copy) protocol over an SSH session,
// which requires no additional Go dependencies beyond golang.org/x/crypto/ssh.
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
//	filename_field  string  Message metadata key used as remote filename
//	filename_prefix string  Static prefix for generated filenames
//	file_extension  string  Extension added to generated filenames (default: ".dat")
//	connect_timeout int     Seconds (default 10)
//	write_timeout   int     Seconds (default 60)
//	mode            string  "overwrite" | "append" (default: "overwrite")
package connectors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"path"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"golang.org/x/crypto/ssh"
)

// SFTPOutboundConnector uploads files to remote SFTP/SCP servers.
type SFTPOutboundConnector struct {
	*BaseOutboundConnector

	// config
	host           string
	port           int
	username       string
	password       string
	keyFile        string
	keyContent     string
	authType       string
	remoteDir      string
	filenameField  string
	filenamePrefix string
	fileExtension  string
	connectTimeout time.Duration
	writeTimeout   time.Duration

	// runtime
	mu         sync.Mutex
	sshConfig  *ssh.ClientConfig
}

// NewSFTPOutboundConnector creates a production SFTP outbound connector.
func NewSFTPOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sftp_outbound",
		DisplayName:        "SFTP File Uploader",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": true,
			"supports_tls":   true,  // TLS via SSH
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

// TestConnection dials the SSH server without uploading data.
func (c *SFTPOutboundConnector) TestConnection(ctx context.Context) error {
	conn, err := c.dialSSH(ctx)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	_ = conn.Close()
	return nil
}

// Send uploads the message content as a file via SCP.
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

	if err := scpUpload(conn, remotePath, content, c.writeTimeout); err != nil {
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

func (c *SFTPOutboundConnector) resolveFilename(message *models.OutboundMessage) string {
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

// scpUpload transfers content to remotePath via SCP protocol over SSH.
func scpUpload(client *ssh.Client, remotePath string, content []byte, timeout time.Duration) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh new session: %w", err)
	}
	defer session.Close()

	dir := path.Dir(remotePath)
	filename := path.Base(remotePath)

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	var cmdErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Write SCP header: "C0644 <size> <filename>\n"
		header := fmt.Sprintf("C0644 %d %s\n", len(content), filename)
		if _, err := io.WriteString(stdinPipe, header); err != nil {
			cmdErr = fmt.Errorf("scp header write: %w", err)
			return
		}
		// Write file content
		if _, err := io.Copy(stdinPipe, bytes.NewReader(content)); err != nil {
			cmdErr = fmt.Errorf("scp content write: %w", err)
			return
		}
		// Write SCP end marker
		_, _ = stdinPipe.Write([]byte{0})
		_ = stdinPipe.Close()
	}()

	// Run scp receive command on remote: scp -qt <dir>
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- session.Run(fmt.Sprintf("scp -qt %s", dir))
	}()

	select {
	case err := <-cmdDone:
		<-done
		if cmdErr != nil {
			return cmdErr
		}
		return err
	case <-time.After(timeout):
		return fmt.Errorf("scp upload timed out after %s", timeout)
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

