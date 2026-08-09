// services/connectors/sink_outbound.go
// Sink Outbound Connector — terminal "store only" target for interfaces that
// should not forward transformed messages to any external system.
//
// Every message already reaches Postgres/MongoDB via the standard pipeline
// persistence path regardless of which (if any) outbound connector runs —
// this connector does no external I/O. It exists so an interface can
// explicitly declare "no forwarding" intent instead of simply omitting an
// outbound connector step, and to carry sink-specific settings (ack
// generation, retention intent) alongside that declaration.
//
// Configuration:
//
//	enable_logging     bool  Log each accepted message (default: true)
//	enable_validation  bool  Reject messages with empty content (default: true)
//	retention_days     int   Informational retention intent, recorded on the
//	                         delivery result metadata; not enforced by this
//	                         connector — pair with a retention job if the
//	                         messages table itself needs purging (default: 30)
//	generate_ack       bool  Include an acknowledgment string on the delivery
//	                         result (default: true)
package connectors

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"ezhealthkonnect/models"
)

// SinkOutboundConnector accepts messages as successfully delivered without
// sending them anywhere — a no-op terminal connector for store-only interfaces.
type SinkOutboundConnector struct {
	*BaseOutboundConnector

	enableLogging    bool
	enableValidation bool
	retentionDays    int
	generateAck      bool

	mu sync.RWMutex
}

// NewSinkOutboundConnector creates a new sink (store-only) outbound connector.
func NewSinkOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sink_outbound",
		DisplayName:        "Sink (Store Only)",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch": true,
		},
	}
	return &SinkOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses sink-specific configuration.
func (s *SinkOutboundConnector) Initialize(config []byte) error {
	if err := s.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := s.GetConfig()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.enableLogging = cfg.GetBoolDefault("enable_logging", true)
	s.enableValidation = cfg.GetBoolDefault("enable_validation", true)
	s.generateAck = cfg.GetBoolDefault("generate_ack", true)

	s.retentionDays = cfg.GetInt("retention_days")
	if s.retentionDays == 0 {
		s.retentionDays = 30
	}

	s.SetMetadata("retention_days", fmt.Sprintf("%d", s.retentionDays))
	return nil
}

// Validate confirms the connector has been initialized. Sink has no external
// configuration to validate — there is nothing to connect to.
func (s *SinkOutboundConnector) Validate() error {
	return s.BaseOutboundConnector.Validate()
}

// TestConnection always succeeds — sink has no external dependency to test.
func (s *SinkOutboundConnector) TestConnection(ctx context.Context) error {
	return nil
}

// Send accepts the message as delivered without forwarding it anywhere.
func (s *SinkOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := s.GetMetadata().TypeName

	s.mu.RLock()
	enableLogging := s.enableLogging
	enableValidation := s.enableValidation
	generateAck := s.generateAck
	retentionDays := s.retentionDays
	s.mu.RUnlock()

	if enableValidation && message.Content == "" {
		err := fmt.Errorf("message content is empty")
		s.RecordError(err)
		return nil, NewConnectorError(typeName, "send", err, false)
	}

	if enableLogging {
		log.Printf("💾 Sink: accepted message %s (interface=%s, %d bytes) — no forwarding configured",
			message.MessageID, message.InterfaceID, len(message.Content))
	}

	s.IncrementMessagesSent()

	result := &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Metadata: map[string]interface{}{
			"mode":           "sink",
			"retention_days": retentionDays,
		},
	}
	if generateAck {
		result.Acknowledgment = "Message stored (sink — no forwarding configured)"
	}
	return result, nil
}
