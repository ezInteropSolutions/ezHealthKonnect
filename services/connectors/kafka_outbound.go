// services/connectors/kafka_outbound.go
// Kafka Outbound Connector — synchronous producer that publishes messages to
// a Kafka topic.
//
// Features:
//   - SASL/PLAIN and SASL/SCRAM-SHA-256/512 authentication (same as kafka_inbound.go)
//   - TLS support for encrypted transport
//   - Configurable compression (none/gzip/snappy/lz4/zstd)
//   - Configurable required acks (none/leader/all)
//   - Optional message-key extraction from a JSON field, so related messages
//     (e.g. same patient MRN) land on the same partition for ordering
//
// Configuration:
//
//	brokers            string  Comma-separated broker list (e.g. broker1:9092,broker2:9092)
//	broker_list        []string Alternative to brokers
//	topic              string  Destination topic (required)
//	message_key_field  string  JSON field in the message content to use as the Kafka
//	                           message key (e.g. "patient_id") — omit for no explicit key
//	compression        string  "none" | "gzip" | "snappy" | "lz4" | "zstd" (default: none)
//	required_acks      string  "none" | "leader" | "all" (default: leader)
//	sasl_enabled       bool    Enable SASL authentication
//	sasl_mechanism     string  "PLAIN" | "SCRAM-SHA-256" | "SCRAM-SHA-512"
//	sasl_username      string
//	sasl_password      string
//	tls_enabled        bool    Enable TLS
//	tls_skip_verify    bool    Skip TLS cert verification (dev only)
package connectors

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/IBM/sarama"
	xdgscram "github.com/xdg-go/scram"
)

// KafkaOutboundConfig holds all Kafka producer configuration.
type KafkaOutboundConfig struct {
	Brokers         string   `json:"brokers"`
	BrokerList      []string `json:"broker_list"`
	Topic           string   `json:"topic"`
	MessageKeyField string   `json:"message_key_field"`
	Compression     string   `json:"compression"`
	RequiredAcks    string   `json:"required_acks"`
	SASLEnabled     bool     `json:"sasl_enabled"`
	SASLMechanism   string   `json:"sasl_mechanism"`
	SASLUsername    string   `json:"sasl_username"`
	SASLPassword    string   `json:"sasl_password"`
	TLSEnabled      bool     `json:"tls_enabled"`
	TLSSkipVerify   bool     `json:"tls_skip_verify"`
}

// KafkaOutboundConnector publishes messages to a Kafka topic.
type KafkaOutboundConnector struct {
	*BaseOutboundConnector
	config   KafkaOutboundConfig
	producer sarama.SyncProducer
	brokers  []string
	mu       sync.Mutex
}

// NewKafkaOutboundConnector creates a production Kafka outbound connector.
func NewKafkaOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "kafka_outbound",
		DisplayName:        "Kafka Producer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":       true,
			"supports_sasl":        true,
			"supports_compression": true,
			"supports_acks":        true,
		},
	}
	return &KafkaOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize configures and connects the synchronous producer.
func (k *KafkaOutboundConnector) Initialize(config []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if err := json.Unmarshal(config, &k.config); err != nil {
		return fmt.Errorf("failed to parse Kafka outbound config: %w", err)
	}

	if k.config.Topic == "" {
		return fmt.Errorf("topic is required")
	}
	k.brokers = k.resolveBrokers()
	if len(k.brokers) == 0 {
		return fmt.Errorf("brokers is required (e.g. 'localhost:9092')")
	}

	saramaCfg, err := k.buildSaramaConfig()
	if err != nil {
		return fmt.Errorf("invalid producer config: %w", err)
	}

	producer, err := sarama.NewSyncProducer(k.brokers, saramaCfg)
	if err != nil {
		return fmt.Errorf("failed to create Kafka producer at %v: %w", k.brokers, err)
	}

	k.producer = producer
	k.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ Kafka Outbound: Initialized (brokers=%v, topic=%s)", k.brokers, k.config.Topic)
	return nil
}

func (k *KafkaOutboundConnector) resolveBrokers() []string {
	if len(k.config.BrokerList) > 0 {
		return k.config.BrokerList
	}
	var result []string
	for _, b := range strings.Split(k.config.Brokers, ",") {
		if b = strings.TrimSpace(b); b != "" {
			result = append(result, b)
		}
	}
	return result
}

// buildSaramaConfig builds the producer config — SASL/TLS setup mirrors
// kafka_inbound.go's buildSaramaConfig exactly, since the same broker
// requires the same credentials regardless of which side is connecting.
func (k *KafkaOutboundConnector) buildSaramaConfig() (*sarama.Config, error) {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_6_0_0

	// SyncProducer requires both of these enabled.
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true

	switch strings.ToLower(k.config.RequiredAcks) {
	case "none":
		cfg.Producer.RequiredAcks = sarama.NoResponse
	case "all":
		cfg.Producer.RequiredAcks = sarama.WaitForAll
	case "leader", "":
		cfg.Producer.RequiredAcks = sarama.WaitForLocal
	default:
		return nil, fmt.Errorf("required_acks must be \"none\", \"leader\", or \"all\", got %q", k.config.RequiredAcks)
	}

	switch strings.ToLower(k.config.Compression) {
	case "", "none":
		cfg.Producer.Compression = sarama.CompressionNone
	case "gzip":
		cfg.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		cfg.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		cfg.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		cfg.Producer.Compression = sarama.CompressionZSTD
	default:
		return nil, fmt.Errorf("compression must be one of none/gzip/snappy/lz4/zstd, got %q", k.config.Compression)
	}

	if k.config.SASLEnabled {
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = k.config.SASLUsername
		cfg.Net.SASL.Password = k.config.SASLPassword
		switch strings.ToUpper(k.config.SASLMechanism) {
		case "SCRAM-SHA-256":
			cfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			cfg.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &kafkaSCRAMClient{HashGeneratorFcn: xdgscram.HashGeneratorFcn(sha256.New)}
			}
		case "SCRAM-SHA-512":
			cfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
			cfg.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &kafkaSCRAMClient{HashGeneratorFcn: xdgscram.HashGeneratorFcn(sha512.New)}
			}
		default:
			cfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}

	if k.config.TLSEnabled {
		cfg.Net.TLS.Enable = true
		if k.config.TLSSkipVerify {
			cfg.Net.TLS.Config = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		}
	}

	return cfg, nil
}

// messageKey extracts the Kafka partition key for a message, if
// message_key_field is configured. Returns "" (no explicit key — sarama
// falls back to round-robin/random partitioning) when unset, absent, or the
// content isn't valid JSON.
func (k *KafkaOutboundConnector) messageKey(message *models.OutboundMessage) string {
	if k.config.MessageKeyField == "" {
		return ""
	}
	var dataMap map[string]interface{}
	if err := json.Unmarshal([]byte(message.Content), &dataMap); err != nil {
		return ""
	}
	val, ok := dataMap[k.config.MessageKeyField]
	if !ok || val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	b, err := json.Marshal(val)
	if err != nil {
		return ""
	}
	return string(b)
}

func (k *KafkaOutboundConnector) buildProducerMessage(message *models.OutboundMessage) *sarama.ProducerMessage {
	pm := &sarama.ProducerMessage{
		Topic: k.config.Topic,
		Value: sarama.StringEncoder(message.Content),
	}
	if key := k.messageKey(message); key != "" {
		pm.Key = sarama.StringEncoder(key)
	}
	return pm
}

// Send publishes a single message to the configured topic.
func (k *KafkaOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	k.mu.Lock()
	producer := k.producer
	k.mu.Unlock()
	if producer == nil {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	pm := k.buildProducerMessage(message)

	partition, offset, err := producer.SendMessage(pm)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("produce failed: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	return &DeliveryResult{
		Success:        true,
		MessageID:      message.MessageID,
		Timestamp:      time.Now(),
		Acknowledgment: fmt.Sprintf("topic=%s partition=%d offset=%d", k.config.Topic, partition, offset),
		DurationMs:     int64(time.Since(startTime).Milliseconds()),
		Metadata: map[string]interface{}{
			"topic":     k.config.Topic,
			"partition": partition,
			"offset":    offset,
		},
	}, nil
}

// SendBatch publishes multiple messages using sarama's real batch-produce API
// (SendMessages) rather than looping Send — a genuine batch, not a fake one.
// Sarama's SendMessages either succeeds for all messages or returns a
// ProducerErrors describing exactly which ones failed, which is unwound below
// into a per-message DeliveryResult, same contract as every other connector's
// SendBatch.
func (k *KafkaOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	k.mu.Lock()
	producer := k.producer
	k.mu.Unlock()
	if producer == nil {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	pms := make([]*sarama.ProducerMessage, len(messages))
	for i, message := range messages {
		pms[i] = k.buildProducerMessage(message)
	}

	err := producer.SendMessages(pms)

	results := make([]*DeliveryResult, len(messages))
	failed := map[int]string{}
	if err != nil {
		if perrs, ok := err.(sarama.ProducerErrors); ok {
			for _, pe := range perrs {
				// sarama.ProducerMessage carries a Metadata field we don't set,
				// so match failures back to the original slice by pointer identity.
				for i, pm := range pms {
					if pm == pe.Msg {
						failed[i] = pe.Err.Error()
						break
					}
				}
			}
		} else {
			// Non-partial error — sarama couldn't attribute failures per-message.
			for i := range messages {
				failed[i] = err.Error()
			}
		}
	}

	successCount, failureCount := 0, 0
	for i, message := range messages {
		if errMsg, isFailed := failed[i]; isFailed {
			results[i] = &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: errMsg,
			}
			failureCount++
			continue
		}
		results[i] = &DeliveryResult{
			Success:        true,
			MessageID:      message.MessageID,
			Timestamp:      time.Now(),
			Acknowledgment: fmt.Sprintf("topic=%s partition=%d offset=%d", k.config.Topic, pms[i].Partition, pms[i].Offset),
			Metadata: map[string]interface{}{
				"topic":     k.config.Topic,
				"partition": pms[i].Partition,
				"offset":    pms[i].Offset,
			},
		}
		successCount++
	}

	log.Printf("✅ Kafka Outbound: Batch complete — success: %d, failed: %d, elapsed: %v",
		successCount, failureCount, time.Since(startTime))
	return results, nil
}

// SupportsBatch returns true.
func (k *KafkaOutboundConnector) SupportsBatch() bool { return true }

// TestConnection verifies broker connectivity by requesting topic metadata —
// it does not publish anything.
func (k *KafkaOutboundConnector) TestConnection(ctx context.Context) error {
	k.mu.Lock()
	brokers := k.brokers
	topic := k.config.Topic
	k.mu.Unlock()
	if len(brokers) == 0 {
		return fmt.Errorf("not initialized")
	}

	client, err := sarama.NewClient(brokers, sarama.NewConfig())
	if err != nil {
		return fmt.Errorf("failed to connect to brokers %v: %w", brokers, err)
	}
	defer client.Close()

	if _, err := client.Partitions(topic); err != nil {
		return fmt.Errorf("failed to fetch metadata for topic %q: %w", topic, err)
	}
	return nil
}

// Validate checks required fields.
func (k *KafkaOutboundConnector) Validate() error {
	if !k.BaseOutboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if k.config.Topic == "" {
		return fmt.Errorf("topic is required")
	}
	return nil
}

// Close shuts down the producer.
func (k *KafkaOutboundConnector) Close() error {
	log.Printf("📨 Kafka Outbound: Closing")
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.producer != nil {
		if err := k.producer.Close(); err != nil {
			return fmt.Errorf("failed to close Kafka producer: %w", err)
		}
		k.producer = nil
	}
	log.Printf("✅ Kafka Outbound: Closed")
	return nil
}

// GetStatus returns connector status with Kafka-specific metadata.
func (k *KafkaOutboundConnector) GetStatus() ConnectorStatus {
	status := k.BaseOutboundConnector.GetStatus()
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["topic"] = k.config.Topic
	status.Metadata["brokers"] = strings.Join(k.brokers, ",")
	return status
}
