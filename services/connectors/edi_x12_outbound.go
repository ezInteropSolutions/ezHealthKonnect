// services/connectors/edi_x12_outbound.go
// EDI X12 Outbound Connector — uploads X12 files to a remote SFTP server
// (835 phase 1). Uses the config key names already live in the
// connectivity_types catalog for edi_x12_outbound
// (V69__EMR_Connectivity_Types.sql) — remote_path instead of remote_dir.
//
// The actual SSH/SFTP mechanics (dial, connection test, upload-with-timeout,
// filename-pattern rendering) live in sftp_uploader.go (dial/auth further
// shared from sftp_poller.go), shared with sftp_outbound.go — a real,
// confirmed duplication (this connector's own prior
// ediSFTPUploadWithTimeout said outright in its doc comment that it
// "mirrors sftp_outbound.go's own sftpUploadWithTimeout exactly") was
// extracted into that shared file during this round.
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
	"context"
	"fmt"
	"log"
	"path"
	"time"

	"ezhealthkonnect/models"

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
	if err := testSFTPConnection(ctx, c.host, c.port, c.sshConfig, c.connectTimeout); err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
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

	if err := sftpSendFile(ctx, c.host, c.port, c.sshConfig, c.connectTimeout, c.remotePath, remotePath, content, c.writeTimeout); err != nil {
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

// resolveFilename builds the remote filename for a message. filename_pattern
// (same {message_id}/{interface_id}/{timestamp}/{date}/{time} placeholder
// convention as sftp_outbound.go) takes priority when set; otherwise a
// timestamp+message-id default with a fixed ".edi" extension — deliberately
// NOT content-type-inferred like sftp_outbound.go's own default, since EDI
// output is always .edi regardless of whatever ContentType metadata a
// message happens to carry.
func (c *EDIX12OutboundConnector) resolveFilename(message *models.OutboundMessage) string {
	if c.filenamePattern == "" {
		ts := time.Now().UTC().Format("20060102_150405")
		msgID := message.MessageID
		if len(msgID) > 8 {
			msgID = msgID[:8]
		}
		return fmt.Sprintf("edi_%s_%s.edi", ts, msgID)
	}
	return renderFilenamePattern(c.filenamePattern, message, ".edi")
}
