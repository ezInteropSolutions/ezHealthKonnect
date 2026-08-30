// services/connectors/kafka_outbound_test.go
// Coverage for messageKey extraction and Sarama config mapping -- pure logic
// that doesn't need a live broker, so it's tested directly without Initialize().
package connectors

import (
	"testing"

	"ezhealthkonnect/models"

	"github.com/IBM/sarama"
)

func newTestKafkaOutbound(cfg KafkaOutboundConfig) *KafkaOutboundConnector {
	return &KafkaOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "kafka_outbound"}, true),
		config:                cfg,
	}
}

func TestMessageKey_NoFieldConfigured_ReturnsEmpty(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{})
	msg := &models.OutboundMessage{Content: `{"patient_id":"MRN001"}`}
	if key := c.messageKey(msg); key != "" {
		t.Errorf("expected no key when message_key_field is unset, got %q", key)
	}
}

func TestMessageKey_ExtractsConfiguredStringField(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{MessageKeyField: "patient_id"})
	msg := &models.OutboundMessage{Content: `{"patient_id":"MRN001","name":"Test"}`}
	if key := c.messageKey(msg); key != "MRN001" {
		t.Errorf("expected key 'MRN001', got %q", key)
	}
}

func TestMessageKey_FieldAbsent_ReturnsEmpty(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{MessageKeyField: "patient_id"})
	msg := &models.OutboundMessage{Content: `{"other_field":"x"}`}
	if key := c.messageKey(msg); key != "" {
		t.Errorf("expected empty key when the configured field is absent, got %q", key)
	}
}

func TestMessageKey_InvalidJSON_ReturnsEmptyNotError(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{MessageKeyField: "patient_id"})
	msg := &models.OutboundMessage{Content: `not valid json`}
	if key := c.messageKey(msg); key != "" {
		t.Errorf("expected empty key for non-JSON content (not a crash), got %q", key)
	}
}

func TestMessageKey_NonStringField_MarshalsToString(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{MessageKeyField: "visit_number"})
	msg := &models.OutboundMessage{Content: `{"visit_number":12345}`}
	if key := c.messageKey(msg); key != "12345" {
		t.Errorf("expected numeric field to render as '12345', got %q", key)
	}
}

func TestBuildProducerMessage_UsesConfiguredTopicAndKey(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{Topic: "adt-events", MessageKeyField: "patient_id"})
	msg := &models.OutboundMessage{Content: `{"patient_id":"MRN001"}`}
	pm := c.buildProducerMessage(msg)

	if pm.Topic != "adt-events" {
		t.Errorf("expected topic 'adt-events', got %q", pm.Topic)
	}
	if pm.Key == nil {
		t.Fatal("expected a partition key to be set")
	}
	keyBytes, _ := pm.Key.Encode()
	if string(keyBytes) != "MRN001" {
		t.Errorf("expected key 'MRN001', got %q", string(keyBytes))
	}
}

func TestBuildSaramaConfig_RequiredAcks(t *testing.T) {
	cases := map[string]sarama.RequiredAcks{
		"":       sarama.WaitForLocal,
		"leader": sarama.WaitForLocal,
		"all":    sarama.WaitForAll,
		"none":   sarama.NoResponse,
	}
	for input, want := range cases {
		c := newTestKafkaOutbound(KafkaOutboundConfig{RequiredAcks: input})
		cfg, err := c.buildSaramaConfig()
		if err != nil {
			t.Fatalf("required_acks=%q: unexpected error: %v", input, err)
		}
		if cfg.Producer.RequiredAcks != want {
			t.Errorf("required_acks=%q: expected %v, got %v", input, want, cfg.Producer.RequiredAcks)
		}
	}
}

func TestBuildSaramaConfig_InvalidRequiredAcks_Errors(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{RequiredAcks: "bogus"})
	if _, err := c.buildSaramaConfig(); err == nil {
		t.Error("expected an error for an invalid required_acks value")
	}
}

func TestBuildSaramaConfig_Compression(t *testing.T) {
	cases := map[string]sarama.CompressionCodec{
		"":       sarama.CompressionNone,
		"none":   sarama.CompressionNone,
		"gzip":   sarama.CompressionGZIP,
		"snappy": sarama.CompressionSnappy,
		"lz4":    sarama.CompressionLZ4,
		"zstd":   sarama.CompressionZSTD,
	}
	for input, want := range cases {
		c := newTestKafkaOutbound(KafkaOutboundConfig{Compression: input})
		cfg, err := c.buildSaramaConfig()
		if err != nil {
			t.Fatalf("compression=%q: unexpected error: %v", input, err)
		}
		if cfg.Producer.Compression != want {
			t.Errorf("compression=%q: expected %v, got %v", input, want, cfg.Producer.Compression)
		}
	}
}

func TestBuildSaramaConfig_InvalidCompression_Errors(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{Compression: "bogus"})
	if _, err := c.buildSaramaConfig(); err == nil {
		t.Error("expected an error for an invalid compression value")
	}
}

func TestBuildSaramaConfig_ProducerReturnFlagsRequiredForSyncProducer(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{})
	cfg, err := c.buildSaramaConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Producer.Return.Successes || !cfg.Producer.Return.Errors {
		t.Error("sarama.NewSyncProducer requires both Return.Successes and Return.Errors enabled, or it panics at construction")
	}
}

func TestKafkaOutboundValidate_RequiresTopic(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err == nil {
		t.Error("Validate must reject a missing topic")
	}

	c2 := newTestKafkaOutbound(KafkaOutboundConfig{Topic: "adt-events"})
	c2.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c2.Validate(); err != nil {
		t.Errorf("Validate should pass with topic set, got: %v", err)
	}
}

func TestResolveBrokers_PrefersBrokerListOverCommaSeparated(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{
		Brokers:    "ignored:9092",
		BrokerList: []string{"a:9092", "b:9092"},
	})
	brokers := c.resolveBrokers()
	if len(brokers) != 2 || brokers[0] != "a:9092" || brokers[1] != "b:9092" {
		t.Errorf("expected broker_list to take precedence, got %v", brokers)
	}
}

func TestResolveBrokers_ParsesCommaSeparatedAndTrimsWhitespace(t *testing.T) {
	c := newTestKafkaOutbound(KafkaOutboundConfig{Brokers: "a:9092, b:9092 ,c:9092"})
	brokers := c.resolveBrokers()
	if len(brokers) != 3 || brokers[1] != "b:9092" {
		t.Errorf("expected 3 trimmed brokers, got %v", brokers)
	}
}
