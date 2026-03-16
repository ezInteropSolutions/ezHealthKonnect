// services/connectors/redis_inbound.go
// Redis Inbound Connector — receives messages from Redis Pub/Sub channels or Redis Streams.
//
// Two modes (auto-detected from config):
//   - Pub/Sub:  Subscribe to one or more channels. Messages arrive in real-time as PUBLISH
//               commands hit the channel. Use when integrating with Redis-based event buses.
//   - Streams:  XREADGROUP consumer group on a Redis Stream. Provides at-least-once delivery
//               with acknowledgement. Preferred for production healthcare message ingestion.
//
// Configuration:
//
//	host              string  Redis host (default: localhost)
//	port              int     Redis port (default: 6379)
//	password          string  Redis AUTH password
//	db_number         int     Redis logical database (0-15, default: 0)
//	channel           string  Pub/Sub channel name (triggers Pub/Sub mode)
//	channels          []string Multiple Pub/Sub channels
//	stream            string  Stream key name (triggers Stream mode)
//	consumer_group    string  Consumer group name (default: ehk-consumer-group)
//	consumer_name     string  Unique consumer name (default: ehk-consumer-<timestamp>)
//	read_timeout      int     Milliseconds to block on XREAD (default: 2000)
//	max_pending_ack   int     Max messages to fetch per XREAD call (default: 10)
//	use_tls           bool    Enable TLS
//	tls_skip_verify   bool    Skip TLS cert verification (dev only)

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"github.com/redis/go-redis/v9"
)

// RedisInboundConfig holds all Redis inbound connector configuration.
type RedisInboundConfig struct {
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Password       string   `json:"password"`
	DBNumber       int      `json:"db_number"`
	Channel        string   `json:"channel"`          // Pub/Sub single channel
	Channels       []string `json:"channels"`         // Pub/Sub multiple channels
	Stream         string   `json:"stream"`           // Stream key
	ConsumerGroup  string   `json:"consumer_group"`
	ConsumerName   string   `json:"consumer_name"`
	ReadTimeoutMs  int      `json:"read_timeout"`     // ms to block on XREAD
	MaxPendingAck  int      `json:"max_pending_ack"`  // messages per XREAD
	UseTLS         bool     `json:"use_tls"`
	TLSSkipVerify  bool     `json:"tls_skip_verify"`
}

// RedisInboundConnector receives messages from Redis Pub/Sub or Streams.
type RedisInboundConnector struct {
	*BaseInboundConnector
	config      RedisInboundConfig
	client      *redis.Client
	mu          sync.Mutex
}

// NewRedisInboundConnector creates a production Redis inbound connector.
func NewRedisInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "redis_inbound",
		DisplayName:        "Redis Consumer",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":    false,
			"supports_tls":     true,
			"supports_pubsub":  true,
			"supports_streams": true,
		},
	}
	return &RedisInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize configures the connector and tests connectivity.
func (c *RedisInboundConnector) Initialize(config []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := json.Unmarshal(config, &c.config); err != nil {
		return fmt.Errorf("failed to parse Redis inbound config: %w", err)
	}

	// Apply defaults
	if c.config.Host == "" {
		c.config.Host = "localhost"
	}
	if c.config.Port == 0 {
		c.config.Port = 6379
	}
	if c.config.ConsumerGroup == "" {
		c.config.ConsumerGroup = "ehk-consumer-group"
	}
	if c.config.ConsumerName == "" {
		c.config.ConsumerName = fmt.Sprintf("ehk-consumer-%d", time.Now().UnixNano())
	}
	if c.config.ReadTimeoutMs == 0 {
		c.config.ReadTimeoutMs = 2000
	}
	if c.config.MaxPendingAck == 0 {
		c.config.MaxPendingAck = 10
	}

	// Build redis options
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		Password: c.config.Password,
		DB:       c.config.DBNumber,
	}

	client := redis.NewClient(opts)

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return fmt.Errorf("failed to ping Redis at %s: %w", opts.Addr, err)
	}

	c.client = client
	c.BaseInboundConnector.BaseConnector.initialized = true
	c.BaseInboundConnector.SetState(StateReady)

	mode := c.resolveMode()
	log.Printf("✅ Redis Inbound: Initialized (%s:%d, mode=%s)", c.config.Host, c.config.Port, mode)
	return nil
}

// resolveMode returns "pubsub" or "stream" based on config.
func (c *RedisInboundConnector) resolveMode() string {
	if c.config.Stream != "" {
		return "stream"
	}
	return "pubsub"
}

// TestConnection pings the Redis server.
func (c *RedisInboundConnector) TestConnection(ctx context.Context) error {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return fmt.Errorf("not initialized")
	}
	return client.Ping(ctx).Err()
}

// Validate checks required fields.
func (c *RedisInboundConnector) Validate() error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if c.config.Channel == "" && len(c.config.Channels) == 0 && c.config.Stream == "" {
		return fmt.Errorf("one of channel, channels, or stream must be specified")
	}
	return nil
}

// SupportsCron returns false — Redis push connectors run continuously.
func (c *RedisInboundConnector) SupportsCron() bool { return false }

// Start begins the receive loop in the appropriate mode.
func (c *RedisInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	c.BaseInboundConnector.SetState(StateRunning)

	if c.resolveMode() == "stream" {
		go c.streamLoop(ctx, messageChan)
	} else {
		go c.pubSubLoop(ctx, messageChan)
	}
	return nil
}

// pubSubLoop subscribes to one or more channels and forwards messages.
func (c *RedisInboundConnector) pubSubLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	// Collect all channel names
	channels := c.config.Channels
	if c.config.Channel != "" {
		channels = append(channels, c.config.Channel)
	}
	if len(channels) == 0 {
		log.Printf("⚠️  Redis Inbound: No channels configured for pub/sub mode")
		return
	}

	pubsub := client.Subscribe(ctx, channels...)
	defer pubsub.Close()

	log.Printf("📡 Redis Inbound: Subscribed to channels: %v", channels)

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Printf("🔴 Redis Inbound: Context cancelled")
			return
		case <-c.BaseInboundConnector.stopCh:
			log.Printf("🔴 Redis Inbound: Stop signal received")
			return
		case msg, ok := <-ch:
			if !ok {
				log.Printf("⚠️  Redis Inbound: Pub/Sub channel closed")
				return
			}
			messageChan <- c.pubSubMsgToInbound(msg)
		}
	}
}

// streamLoop uses XREADGROUP to consume from a Redis Stream with consumer group semantics.
func (c *RedisInboundConnector) streamLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	stream := c.config.Stream
	group := c.config.ConsumerGroup
	consumer := c.config.ConsumerName
	blockDuration := time.Duration(c.config.ReadTimeoutMs) * time.Millisecond

	// Ensure consumer group exists (create if absent, starting from $ = new messages only)
	createCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := client.XGroupCreateMkStream(createCtx, stream, group, "$").Err()
	cancel()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		// BUSYGROUP means already exists — that's fine
		log.Printf("⚠️  Redis Inbound: XGroupCreate warning: %v", err)
	}

	log.Printf("📡 Redis Inbound: Stream consumer group active (stream=%s, group=%s, consumer=%s)",
		stream, group, consumer)

	for {
		select {
		case <-ctx.Done():
			log.Printf("🔴 Redis Inbound: Stream loop context cancelled")
			return
		case <-c.BaseInboundConnector.stopCh:
			log.Printf("🔴 Redis Inbound: Stream loop stop signal")
			return
		default:
		}

		result, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    int64(c.config.MaxPendingAck),
			Block:    blockDuration,
		}).Result()

		if err != nil {
			if err == redis.Nil || strings.Contains(err.Error(), "redis: nil") {
				// Timeout — no messages, loop back
				continue
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("❌ Redis Inbound: XREADGROUP error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, xStream := range result {
			for _, msg := range xStream.Messages {
				inbound := c.streamMsgToInbound(msg, stream)
				messageChan <- inbound
				// Acknowledge message
				ackCtx, ackCancel := context.WithTimeout(ctx, 5*time.Second)
				if err := client.XAck(ackCtx, stream, group, msg.ID).Err(); err != nil {
					log.Printf("⚠️  Redis Inbound: XAck failed for %s: %v", msg.ID, err)
				}
				ackCancel()
			}
		}
	}
}

// pubSubMsgToInbound converts a Redis Pub/Sub message to an InboundMessage.
func (c *RedisInboundConnector) pubSubMsgToInbound(msg *redis.Message) *models.InboundMessage {
	content := msg.Payload
	contentType := detectContentType(content)
	return &models.InboundMessage{
		MessageID:      fmt.Sprintf("redis_ps_%d", time.Now().UnixNano()),
		Content:        content,
		ContentType:    contentType,
		SourceType:     "redis_pubsub",
		SourceEndpoint: msg.Channel,
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"Redis-Channel": msg.Channel,
			"Redis-Pattern": msg.Pattern,
		},
	}
}

// streamMsgToInbound converts a Redis Stream message to an InboundMessage.
func (c *RedisInboundConnector) streamMsgToInbound(msg redis.XMessage, streamKey string) *models.InboundMessage {
	// Try common field names for message content
	contentFields := []string{"message", "content", "payload", "data", "body", "raw"}
	content := ""
	for _, f := range contentFields {
		if v, ok := msg.Values[f]; ok && v != nil {
			content = fmt.Sprintf("%v", v)
			break
		}
	}
	// Fall back to JSON of all fields
	if content == "" {
		b, _ := json.Marshal(msg.Values)
		content = string(b)
	}

	contentType := detectContentType(content)
	msgType := ""
	if v, ok := msg.Values["message_type"]; ok {
		msgType = fmt.Sprintf("%v", v)
	}

	return &models.InboundMessage{
		MessageID:      fmt.Sprintf("redis_stream_%s", msg.ID),
		Content:        content,
		ContentType:    contentType,
		SourceType:     "redis_stream",
		SourceEndpoint: streamKey,
		MessageType:    msgType,
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"Redis-Stream":         streamKey,
			"Redis-Message-ID":     msg.ID,
			"Redis-Consumer-Group": c.config.ConsumerGroup,
		},
	}
}

// detectContentType inspects the first bytes of content to determine MIME type.
func detectContentType(content string) string {
	if len(content) >= 4 && content[:4] == "MSH|" {
		return "application/hl7-v2"
	}
	trimmed := strings.TrimSpace(content)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return "application/json"
	}
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return "application/xml"
	}
	return "text/plain"
}

// Stop closes the Redis client and halts the receive loop.
func (c *RedisInboundConnector) Stop() error {
	log.Printf("🔴 Redis Inbound: Stopping")
	if err := c.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			return fmt.Errorf("failed to close Redis client: %w", err)
		}
		c.client = nil
	}
	log.Printf("✅ Redis Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (c *RedisInboundConnector) Close() error { return c.Stop() }

// GetStatus returns connector status with Redis-specific metadata.
func (c *RedisInboundConnector) GetStatus() ConnectorStatus {
	status := c.BaseInboundConnector.GetStatus()
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := client.Ping(ctx).Err(); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	return status
}
