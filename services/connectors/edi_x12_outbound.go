// services/connectors/edi_x12_outbound.go
// EDI X12 Outbound Connector — uploads X12 files to a remote SFTP server
// (835 phase 1). Structurally mirrors sftp_outbound.go (real SFTP protocol,
// see that file's own header for why) but uses the config key names already
// live in the connectivity_types catalog for edi_x12_outbound
// (V69__EMR_Connectivity_Types.sql) — remote_path instead of remote_dir.
//
// Stays a transport-only "dumb byte shipper" — it uploads whatever content
// it's given (built by the edi.build pipeline step) to remote_path. All X12
// envelope/business logic (ISA/GS/GE/IEA construction, control numbers)
// lives in edi.build, not here, matching how every other connector in this
// codebase stays transport-only.
//
// Configuration (see connectivity_types.config_schema for edi_x12_outbound):
//
//	transport        string  Only "sftp" is implemented in phase 1; any
//	                         other value fails Validate() with a clear error
//	host             string  Remote hostname or IP
//	port             int     SSH port (default 22)
//	username         string  SSH username
//	password         string  SSH password
//	key_content      string  PEM private key (string form, for auth_type="key")
//	auth_type        string  "password" | "key" (default: "password")
//	remote_path      string  Remote directory path (default: "/outgoing")
//	filename_pattern string  Template for the remote filename (default: timestamp-based)
//	isa_sender_id    string  Accepted per the catalog schema — the actual ISA/GS
//	isa_receiver_id  string  sender/receiver identifiers are edi.build's own
//	                         config (isaSenderId/isaReceiverId), since they're
//	                         embedded IN the built document content, not
//	                         something this transport-only connector uses
//	timeout_seconds  int     Seconds (default 60)
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
	"time"

	"ezhealthkonnect/models"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// EDIX12OutboundConnector uploads X12 EDI files to remote SFTP servers.
type EDIX12OutboundConnector struct {
	*BaseOutboundConnector

	// config
	transport       string
	host            string
	port            int
	username        string
	password        string
	keyContent      string
	authType        string
	remotePath      string
	filenamePattern string
	connectTimeout  time.Duration
	writeTimeout    time.Duration

	// runtime
	sshConfig *ssh.ClientConfig
}

// NewEDIX12OutboundConnector creates a production EDI X12 outbound
// connector (SFTP transport only in phase 1).
func NewEDIX12OutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "edi_x12_outbound",
		DisplayName:        "EDI X12 Outbound",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": true,
			"supports_tls":   true, // TLS via SSH
			"supports_sftp":  true,
			// http/as2 are named future phases — not implemented, deliberately not claimed here.
		},
	}
	return &EDIX12OutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses configuration and builds the SSH client config.
func (c *EDIX12OutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
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
		c.remotePath = "/outgoing"
	}
	c.filenamePattern = cfg.GetString("filename_pattern")

	connectSec := cfg.GetInt("connect_timeout")
	if connectSec == 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	writeSec := cfg.GetInt("timeout_seconds")
	if writeSec == 0 {
		writeSec = 60
	}
	c.writeTimeout = time.Duration(writeSec) * time.Second

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
	return nil
}

// Validate checks configuration validity. transport values other than
// "sftp" are rejected explicitly — same rationale as the inbound
// connector's own Validate().
func (c *EDIX12OutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
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
	if c.authType == "password" && c.password == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("password required when auth_type is 'password'"), false)
	}
	if c.authType == "key" && c.keyContent == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("key_content required when auth_type is 'key'"), false)
	}
	return nil
}

// TestConnection dials SSH and opens a real SFTP session.
func (c *EDIX12OutboundConnector) TestConnection(ctx context.Context) error {
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

// Send uploads the message content (a built X12 interchange, from edi.build)
// as a file via real SFTP.
func (c *EDIX12OutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	if message.Content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	filename := c.resolveFilename(message)
	remotePath := path.Join(c.remotePath, filename)
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

	if err := ediSFTPUploadWithTimeout(sftpClient, c.remotePath, remotePath, content, c.writeTimeout); err != nil {
		c.RecordError(err)
		return failResult(message.MessageID, start, err), err
	}

	c.IncrementMessagesSent()
	log.Printf("[edi_x12_outbound] uploaded %d bytes → %s:%s", len(content), c.host, remotePath)

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

func (c *EDIX12OutboundConnector) dialSSH(ctx context.Context) (*ssh.Client, error) {
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

func (c *EDIX12OutboundConnector) buildAuthMethods() ([]ssh.AuthMethod, error) {
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

// resolveFilename builds the remote filename for a message. filename_pattern
// (same {message_id}/{interface_id}/{timestamp}/{date}/{time} placeholder
// convention as sftp_outbound.go's own resolvePatternFilename) takes
// priority when set; otherwise a timestamp+message-id default with a ".edi"
// extension.
func (c *EDIX12OutboundConnector) resolveFilename(message *models.OutboundMessage) string {
	if c.filenamePattern == "" {
		ts := time.Now().UTC().Format("20060102_150405")
		msgID := message.MessageID
		if len(msgID) > 8 {
			msgID = msgID[:8]
		}
		return fmt.Sprintf("edi_%s_%s.edi", ts, msgID)
	}

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
		name += ".edi"
	}
	return name
}

// ediSFTPUploadWithTimeout mirrors sftp_outbound.go's own
// sftpUploadWithTimeout exactly — a distinct name since both files live in
// the same connectors package.
func ediSFTPUploadWithTimeout(client *sftp.Client, remoteDir, remotePath string, content []byte, timeout time.Duration) error {
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
