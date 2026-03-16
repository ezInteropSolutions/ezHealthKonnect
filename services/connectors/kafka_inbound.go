// services/connectors/kafka_inbound.go
// Kafka Inbound Connector — consumer group subscriber that reads messages from one or more
// Kafka topics and delivers them into the pipeline as InboundMessages.
//
// Features:
//   - Consumer group with automatic partition rebalancing
//   - SASL/PLAIN and SASL/SCRAM-SHA-256/512 authentication
//   - TLS support for encrypted transport
//   - Configurable offset reset (earliest / latest)
//   - At-least-once delivery with per-message offset commit
//
// Configuration:
//
//	brokers           string  Comma-separated broker list (e.g. broker1:9092,broker2:9092)
//	topic             string  Single topic to subscribe to
//	topics            []string Multiple topics
//	consumer_group    string  Consumer group ID (required)
//	auto_offset_reset string  "earliest" | "latest" (default: latest)
//	session_timeout   int     Seconds (default: 30)
//	heartbeat_interval int    Seconds (default: 3)
//	max_bytes         int     Max bytes per fetch (default: 1048576 = 1 MB)
//	sasl_enabled      bool    Enable SASL authentication
//	sasl_mechanism    string  "PLAIN" | "SCRAM-SHA-256" | "SCRAM-SHA-512"
//	sasl_username     string
//	sasl_password     string
//	tls_enabled       bool    Enable TLS
//	tls_skip_verify   bool    Skip TLS cert verification (dev only)

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

// Kafka SCRAM hash generator function types
var (
	kafkaSHA256 xdgscram.HashGeneratorFcn = sha256.New
	kafkaSHA512 xdgscram.HashGeneratorFcn = sha512.New
)

// KafkaInboundConfig holds all Kafka consumer configuration.
type KafkaInboundConfig struct {
	Brokers           string   `json:"brokers"`
	BrokerList        []string `json:"broker_list"`
	Topic             string   `json:"topic"`
	Topics            []string `json:"topics"`
	ConsumerGroup     string   `json:"consumer_group"`
	AutoOffsetReset   string   `json:"auto_offset_reset"` // "earliest" | "latest"
	SessionTimeoutSec int      `json:"session_timeout"`
	HeartbeatSec      int      `json:"heartbeat_interval"`
	MaxBytes          int      `json:"max_bytes"`
	SASLEnabled       bool     `json:"sasl_enabled"`
	SASLMechanism     string   `json:"sasl_mechanism"`
	SASLUsername      string   `json:"sasl_username"`
	SASLPassword      string   `json:"sasl_password"`
	TLSEnabled        bool     `json:"tls_enabled"`
	TLSSkipVerify     bool     `json:"tls_skip_verify"`
}

// KafkaInboundConnector subscribes to Kafka topics in a consumer group.
type KafkaInboundConnector struct {
	*BaseInboundConnector
	config        KafkaInboundConfig
	consumerGroup sarama.ConsumerGroup
	topics        []string
	mu            sync.Mutex
}

// NewKafkaInboundConnector creates a production Kafka inbound connector.
func NewKafkaInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "kafka_inbound",
		DisplayName:        "Kafka Consumer",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":           false,
			"supports_sasl":           true,
			"supports_consumer_group": true,
			"supports_offset_mgmt":    true,
		},
	}
	return &KafkaInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize configures and connects the consumer group.
func (k *KafkaInboundConnector) Initialize(config []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if err := json.Unmarshal(config, &k.config); err != nil {
		return fmt.Errorf("failed to parse Kafka inbound config: %w", err)
	}

	if k.config.ConsumerGroup == "" {
		return fmt.Errorf("consumer_group is required")
	}
	brokers := k.resolveBrokers()
	if len(brokers) == 0 {
		return fmt.Errorf("brokers is required (e.g. 'localhost:9092')")
	}
	k.topics = k.resolveTopics()
	if len(k.topics) == 0 {
		return fmt.Errorf("at least one topic must be configured")
	}

	// Apply defaults
	if k.config.AutoOffsetReset == "" {
		k.config.AutoOffsetReset = "latest"
	}
	if k.config.SessionTimeoutSec == 0 {
		k.config.SessionTimeoutSec = 30
	}
	if k.config.HeartbeatSec == 0 {
		k.config.HeartbeatSec = 3
	}
	if k.config.MaxBytes == 0 {
		k.config.MaxBytes = 1024 * 1024
	}

	saramaCfg := k.buildSaramaConfig()
	group, err := sarama.NewConsumerGroup(brokers, k.config.ConsumerGroup, saramaCfg)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer group at %v: %w", brokers, err)
	}

	k.consumerGroup = group
	k.BaseInboundConnector.BaseConnector.initialized = true
	k.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ Kafka Inbound: Initialized (brokers=%v, topics=%v, group=%s)",
		brokers, k.topics, k.config.ConsumerGroup)
	return nil
}

func (k *KafkaInboundConnector) resolveBrokers() []string {
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

func (k *KafkaInboundConnector) resolveTopics() []string {
	if len(k.config.Topics) > 0 {
		return k.config.Topics
	}
	if k.config.Topic != "" {
		return []string{k.config.Topic}
	}
	return nil
}

func (k *KafkaInboundConnector) buildSaramaConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_6_0_0
	cfg.Consumer.Group.Session.Timeout = time.Duration(k.config.SessionTimeoutSec) * time.Second
	cfg.Consumer.Group.Heartbeat.Interval = time.Duration(k.config.HeartbeatSec) * time.Second
	cfg.Consumer.Fetch.Max = int32(k.config.MaxBytes)
	cfg.Consumer.Offsets.AutoCommit.Enable = true
	cfg.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second

	if k.config.AutoOffsetReset == "earliest" {
		cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	} else {
		cfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	if k.config.SASLEnabled {
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = k.config.SASLUsername
		cfg.Net.SASL.Password = k.config.SASLPassword
		switch strings.ToUpper(k.config.SASLMechanism) {
		case "SCRAM-SHA-256":
			cfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			cfg.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &kafkaSCRAMClient{HashGeneratorFcn: kafkaSHA256}
			}
		case "SCRAM-SHA-512":
			cfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
			cfg.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &kafkaSCRAMClient{HashGeneratorFcn: kafkaSHA512}
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
	return cfg
}

// TestConnection verifies the consumer group was created successfully.
func (k *KafkaInboundConnector) TestConnection(ctx context.Context) error {
	k.mu.Lock()
	cg := k.consumerGroup
	k.mu.Unlock()
	if cg == nil {
		return fmt.Errorf("not initialized")
	}
	return nil
}

// Validate checks required fields.
func (k *KafkaInboundConnector) Validate() error {
	if !k.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if len(k.topics) == 0 {
		return fmt.Errorf("at least one topic must be configured")
	}
	return nil
}

// SupportsCron returns false — Kafka is event-driven.
func (k *KafkaInboundConnector) SupportsCron() bool { return false }

// Start launches the consumer group loop goroutine.
func (k *KafkaInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !k.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	k.BaseInboundConnector.SetState(StateRunning)
	go k.consumeLoop(ctx, messageChan)
	return nil
}

func (k *KafkaInboundConnector) consumeLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	handler := &kafkaConsumerGroupHandler{
		messageChan: messageChan,
		config:      k.config,
	}
	for {
		select {
		case <-ctx.Done():
			log.Printf("📨 Kafka Inbound: Context cancelled")
			return
		case <-k.BaseInboundConnector.stopCh:
			log.Printf("📨 Kafka Inbound: Stop signal received")
			return
		default:
		}
		k.mu.Lock()
		cg := k.consumerGroup
		k.mu.Unlock()
		if err := cg.Consume(ctx, k.topics, handler); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("❌ Kafka Inbound: Consumer group error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// Stop closes the consumer group.
func (k *KafkaInboundConnector) Stop() error {
	log.Printf("📨 Kafka Inbound: Stopping")
	if err := k.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.consumerGroup != nil {
		if err := k.consumerGroup.Close(); err != nil {
			return fmt.Errorf("failed to close Kafka consumer group: %w", err)
		}
		k.consumerGroup = nil
	}
	log.Printf("✅ Kafka Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (k *KafkaInboundConnector) Close() error { return k.Stop() }

// -----------------------------------------------------------------------------
// kafkaConsumerGroupHandler — implements sarama.ConsumerGroupHandler
// -----------------------------------------------------------------------------

type kafkaConsumerGroupHandler struct {
	messageChan chan<- *models.InboundMessage
	config      KafkaInboundConfig
}

func (h *kafkaConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *kafkaConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (h *kafkaConsumerGroupHandler) ConsumeClaim(
	session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim,
) error {
	for msg := range claim.Messages() {
		h.messageChan <- kafkaMsgToInbound(msg, h.config)
		session.MarkMessage(msg, "")
	}
	return nil
}

func kafkaMsgToInbound(msg *sarama.ConsumerMessage, cfg KafkaInboundConfig) *models.InboundMessage {
	content := string(msg.Value)
	ct := detectContentType(content)
	msgType := ""
	for _, hdr := range msg.Headers {
		key := string(hdr.Key)
		if key == "message_type" || key == "content-type" {
			msgType = string(hdr.Value)
			break
		}
	}
	headers := map[string]string{
		"Kafka-Topic":     msg.Topic,
		"Kafka-Partition": fmt.Sprintf("%d", msg.Partition),
		"Kafka-Offset":    fmt.Sprintf("%d", msg.Offset),
		"Kafka-Group":     cfg.ConsumerGroup,
	}
	if msg.Key != nil {
		headers["Kafka-Key"] = string(msg.Key)
	}
	return &models.InboundMessage{
		MessageID:      fmt.Sprintf("kafka_%s_%d_%d", msg.Topic, msg.Partition, msg.Offset),
		Content:        content,
		ContentType:    ct,
		SourceType:     "kafka",
		SourceEndpoint: msg.Topic,
		MessageType:    msgType,
		MessageSize:    len(content),
		ReceivedAt:     msg.Timestamp,
		Headers:        headers,
	}
}

// -----------------------------------------------------------------------------
// kafkaSCRAMClient — SCRAM authentication client for SASL/SCRAM-SHA-256/512
// -----------------------------------------------------------------------------

type kafkaSCRAMClient struct {
	*xdgscram.Client
	*xdgscram.ClientConversation
	xdgscram.HashGeneratorFcn
}

func (x *kafkaSCRAMClient) Begin(userName, password, authzID string) error {
	var err error
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

func (x *kafkaSCRAMClient) Step(challenge string) (string, error) {
	return x.ClientConversation.Step(challenge)
}

func (x *kafkaSCRAMClient) Done() bool {
	return x.ClientConversation.Done()
}
