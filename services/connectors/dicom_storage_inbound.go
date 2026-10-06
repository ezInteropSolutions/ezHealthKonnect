// services/connectors/dicom_storage_inbound.go
// DICOM Storage SCP Inbound Connector — a thin adapter over the dicom/
// package, which owns all real protocol logic (handshake, DIMSE dispatch,
// AE allowlisting, Part 10 serialization, metadata extraction). This file's
// own job is the same as every other connector's: parse config, start/stop
// the real server, and translate a received instance into an
// InboundMessage.
//
// Configuration:
//
//	ae_title                      string    This SCP's own AE title (required)
//	port                          int       Listener port (required)
//	bind_address                  string    (default: "0.0.0.0")
//	max_associations              int       (default: 0, unlimited — library-native bound)
//	allowed_calling_ae_titles     []string  (default: empty, accept any)
//	additional_abstract_syntaxes  []string  (default: empty)
//	max_instance_size_mb          int       (default: 100 MB; 0 means "use the default", not
//	                                         unlimited — a real CR/DX image is commonly 8-25MB,
//	                                         so 100MB is generous headroom while still bounding a
//	                                         misbehaving/oversized sender; hard-capped at 1024MB
//	                                         regardless of a larger configured value)
//	tls_enabled                   bool      (default: false)
//	tls_cert_file                 string    Required if tls_enabled
//	tls_key_file                  string    Required if tls_enabled
//	tls_ca_file                   string    Optional — enables real mutual TLS (client certs
//	                                         signed by this CA are required); only meaningful
//	                                         when tls_enabled is true
package connectors

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"ezhealthkonnect/dicom"
	"ezhealthkonnect/models"

	"github.com/amrshadid/go-dicom/network"
	"github.com/google/uuid"
)

// generateDICOMMessageID returns a unique message ID. Uses a real UUID —
// not a shared counter — for the same reason generateASTMMessageID
// (astm_framing.go) does: multiple associations are handled concurrently by
// the library's own per-association goroutines, so any shared mutable
// counter here would be exactly the kind of unguarded data race already
// found and fixed in this codebase's ASTM connectors this session.
func generateDICOMMessageID() string {
	return "dicom_" + uuid.New().String()
}

// DICOMStorageInboundConnector receives DICOM images pushed from a modality
// (e.g. a CR/DX reader) acting as a Storage SCU.
type DICOMStorageInboundConnector struct {
	*BaseInboundConnector

	aeTitle                    string
	port                       int
	bindAddress                string
	maxAssociations            int
	allowedCallingAETitles     []string
	additionalAbstractSyntaxes []string
	maxInstanceSizeBytes       int64
	tlsEnabled                 bool
	tlsCertFile                string
	tlsKeyFile                 string
	tlsCAFile                  string

	receiver *dicom.Receiver
}

const (
	defaultMaxDICOMInstanceSizeMB = 100  // applied when max_instance_size_mb is 0/unset
	hardMaxDICOMInstanceSizeMB    = 1024 // a configured value above this is clamped, not rejected
)

// NewDICOMStorageInboundConnector creates a production DICOM Storage SCP
// inbound connector.
func NewDICOMStorageInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "dicom_storage_inbound",
		DisplayName:        "DICOM Storage SCP",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron": false,
		},
	}
	return &DICOMStorageInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses configuration.
func (c *DICOMStorageInboundConnector) Initialize(config []byte) error {
	if err := c.BaseInboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.aeTitle = cfg.GetString("ae_title")
	c.port = cfg.GetInt("port")

	c.bindAddress = cfg.GetString("bind_address")
	if c.bindAddress == "" {
		c.bindAddress = "0.0.0.0"
	}

	c.maxAssociations = cfg.GetInt("max_associations")
	c.allowedCallingAETitles = cfg.GetStringSlice("allowed_calling_ae_titles")
	c.additionalAbstractSyntaxes = cfg.GetStringSlice("additional_abstract_syntaxes")

	maxInstanceMB := cfg.GetInt("max_instance_size_mb")
	if maxInstanceMB <= 0 {
		maxInstanceMB = defaultMaxDICOMInstanceSizeMB
	} else if maxInstanceMB > hardMaxDICOMInstanceSizeMB {
		maxInstanceMB = hardMaxDICOMInstanceSizeMB
	}
	c.maxInstanceSizeBytes = int64(maxInstanceMB) * 1024 * 1024

	c.tlsEnabled = cfg.GetBool("tls_enabled")
	c.tlsCertFile = cfg.GetString("tls_cert_file")
	c.tlsKeyFile = cfg.GetString("tls_key_file")
	c.tlsCAFile = cfg.GetString("tls_ca_file")
	if c.tlsEnabled && (c.tlsCertFile == "" || c.tlsKeyFile == "") {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize",
			fmt.Errorf("tls_enabled but tls_cert_file and/or tls_key_file not specified"), false)
	}

	c.SetMetadata("ae_title", c.aeTitle)
	c.SetMetadata("port", fmt.Sprintf("%d", c.port))
	return nil
}

// Validate checks configuration validity.
func (c *DICOMStorageInboundConnector) Validate() error {
	if err := c.BaseInboundConnector.Validate(); err != nil {
		return err
	}
	if c.aeTitle == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("ae_title is required"), false)
	}
	if c.port == 0 {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("port is required"), false)
	}
	return nil
}

// TestConnection opens and immediately closes a listener on the configured
// port — proves the port was free to bind a moment ago, nothing about
// whether a real modality has ever reached it (the same honest caveat every
// other listener-style connector's own TestConnection carries in this
// codebase).
func (c *DICOMStorageInboundConnector) TestConnection(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.port))
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return listener.Close()
}

// Start builds and starts the real DICOM Storage SCP. Returns synchronously
// with a real error if the port could not be bound — dicom.Receiver.Start
// wraps network.StartServer/StartServerTLS, both non-blocking.
func (c *DICOMStorageInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if c.IsRunning() {
		return ErrConnectorAlreadyRunning
	}
	if err := c.Validate(); err != nil {
		return err
	}

	c.receiver = dicom.NewReceiver(dicom.ReceiverConfig{
		AETitle:                    c.aeTitle,
		Port:                       c.port,
		BindAddress:                c.bindAddress,
		MaxAssociations:            c.maxAssociations,
		AllowedCallingAETitles:     c.allowedCallingAETitles,
		AdditionalAbstractSyntaxes: c.additionalAbstractSyntaxes,
		MaxInstanceSizeBytes:       c.maxInstanceSizeBytes,
		TLSEnabled:                 c.tlsEnabled,
		TLSCertFile:                c.tlsCertFile,
		TLSKeyFile:                 c.tlsKeyFile,
		TLSCAFile:                  c.tlsCAFile,
		OnInstanceStored:           c.onInstanceStored(messageChan),
		OnRejected:                 c.onRejected,
	})

	if err := c.receiver.Start(ctx); err != nil {
		c.RecordError(err)
		return NewConnectorError(c.GetMetadata().TypeName, "start", err, true)
	}

	c.SetState(StateRunning)
	c.SetConnected(true)
	log.Printf("🩻 DICOM Storage SCP: Listening on %s (AE title: %s)", c.receiver.Addr(), c.aeTitle)
	return nil
}

// onInstanceStored builds the callback dicom.Receiver calls for each
// successfully received, allowed C-STORE.
func (c *DICOMStorageInboundConnector) onInstanceStored(messageChan chan<- *models.InboundMessage) func(context.Context, *dicom.StoredInstance) uint16 {
	return func(_ context.Context, inst *dicom.StoredInstance) uint16 {
		msg := &models.InboundMessage{
			MessageID:      generateDICOMMessageID(),
			Content:        string(inst.Part10),
			ContentType:    "application/dicom",
			SourceType:     "dicom_storage_inbound",
			SourceEndpoint: fmt.Sprintf("%d", c.port),
			ReceivedAt:     time.Now(),
			// storeMessage's own INSERT binds msg.MessageSize directly into
			// the message_size column — it is never auto-computed from
			// Content, so a connector that omits this ships a real,
			// confirmed-live 0 every time (caught during this feature's own
			// full-stack verification). Set explicitly here, unlike the
			// pre-existing astm_tcp_inbound.go, which has the same gap but
			// is out of scope for this change.
			MessageSize: len(inst.Part10),
			SourceMetadata: map[string]string{
				"calling_ae":       inst.CallingAE,
				"sop_instance_uid": inst.SOPInstanceUID,
				"sop_class_uid":    inst.SOPClassUID,
				"modality":         inst.Metadata.Modality,
				"patient_id":       inst.Metadata.PatientID,
			},
		}

		select {
		case messageChan <- msg:
			c.IncrementMessagesReceived()
			return network.StatusSuccess
		default:
			log.Printf("⚠️ DICOM Storage SCP: message queue full, refusing instance %s", inst.SOPInstanceUID)
			return network.StatusOutOfResources
		}
	}
}

// onRejected logs every refused C-ECHO/C-STORE (AE not on the allowlist, no
// concurrency slot, an invalid peer-supplied UID, or a serialization
// failure). Before this existed, a rejected association left zero trace
// anywhere in this application — the peer got the correct DIMSE refusal
// status, but a support engineer troubleshooting "the device can't connect"
// had nothing to look at in logs or the UI (found during a full QA pass,
// October 2026).
func (c *DICOMStorageInboundConnector) onRejected(_ context.Context, callingAE, reason string) {
	if callingAE == "" {
		callingAE = "(unknown)"
	}
	log.Printf("⚠️ DICOM Storage SCP: rejected association from AE %q: %s", callingAE, reason)
}

// Stop stops the real server.
func (c *DICOMStorageInboundConnector) Stop() error {
	if c.receiver != nil {
		c.receiver.Stop()
	}
	c.SetState(StateStopped)
	c.SetConnected(false)
	return c.BaseInboundConnector.Stop()
}

// Close releases all resources.
func (c *DICOMStorageInboundConnector) Close() error {
	c.Stop()
	return c.BaseInboundConnector.Close()
}
