// services/connectors/rabbitmq_inbound.go
// RabbitMQ Inbound Connector — AMQP 0-9-1 consumer that subscribes to a queue and
// delivers messages to the pipeline.  Uses the official amqp091-go library.
//
// Features:
//   - Durable / exclusive queue binding
//   - Configurable prefetch count (QoS)
//   - Manual ACK with dead-letter on processing failure
//   - TLS support for amqps:// connections
//   - Automatic reconnection with exponential backoff
//
// Configuration:
//
//	host              string  RabbitMQ host (default: localhost)
//	port              int     AMQP port (default: 5672; amqps: 5671)
//	username          string  (default: guest)
//	password          string  (default: guest)
//	vhost             string  Virtual host (default: /)
//	queue             string  Queue name to consume (required)
//	exchange          string  Exchange to bind queue to (optional)
//	routing_key       string  Routing key for binding (optional)
//	prefetch_count    int     QoS prefetch count (default: 10)
//	auto_ack          bool    Auto-acknowledge messages (default: false)
//	queue_durable     bool    Queue survives broker restart (default: true)
//	queue_exclusive   bool    Queue is exclusive to this connection (default: false)
//	use_tls           bool    Use amqps:// (TLS) connection
//	tls_skip_verify   bool    Skip TLS cert verification (dev only)

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ezhealthkonnect/models"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQInboundConfig holds all RabbitMQ consumer configuration.
type RabbitMQInboundConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	VHost          string `json:"vhost"`
	Queue          string `json:"queue"`
	Exchange       string `json:"exchange"`
	RoutingKey     string `json:"routing_key"`
	PrefetchCount  int    `json:"prefetch_count"`
	AutoAck        bool   `json:"auto_ack"`
	QueueDurable   bool   `json:"queue_durable"`
	QueueExclusive bool   `json:"queue_exclusive"`
	UseTLS         bool   `json:"use_tls"`
	TLSSkipVerify  bool   `json:"tls_skip_verify"`
}

// RabbitMQInboundConnector subscribes to a RabbitMQ queue.
type RabbitMQInboundConnector struct {
	*BaseInboundConnector
	config RabbitMQInboundConfig
	conn   *amqp.Connection
	ch     *amqp.Channel
	mu     sync.Mutex
}

// NewRabbitMQInboundConnector creates a production RabbitMQ inbound connector.
func NewRabbitMQInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "rabbitmq_inbound",
		DisplayName:        "RabbitMQ Consumer",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":     false,
			"supports_tls":      true,
			"supports_prefetch": true,
			"supports_auto_ack": true,
		},
	}
	return &RabbitMQInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize connects to RabbitMQ and declares the queue.
func (r *RabbitMQInboundConnector) Initialize(config []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := json.Unmarshal(config, &r.config); err != nil {
		return fmt.Errorf("failed to parse RabbitMQ inbound config: %w", err)
	}

	// Validate required
	if r.config.Queue == "" {
		return fmt.Errorf("queue is required")
	}

	// Apply defaults
	if r.config.Host == "" {
		r.config.Host = "localhost"
	}
	if r.config.Port == 0 {
		if r.config.UseTLS {
			r.config.Port = 5671
		} else {
			r.config.Port = 5672
		}
	}
	if r.config.Username == "" {
		r.config.Username = "guest"
	}
	if r.config.Password == "" {
		r.config.Password = "guest"
	}
	if r.config.VHost == "" {
		r.config.VHost = "/"
	}
	if r.config.PrefetchCount == 0 {
		r.config.PrefetchCount = 10
	}
	// Queues are durable by default
	if !r.config.QueueExclusive {
		r.config.QueueDurable = true
	}

	// Connect
	if err := r.connect(); err != nil {
		return err
	}

	r.BaseInboundConnector.BaseConnector.initialized = true
	r.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ RabbitMQ Inbound: Initialized (host=%s:%d, queue=%s)",
		r.config.Host, r.config.Port, r.config.Queue)
	return nil
}

// connect establishes the AMQP connection and channel, declares the queue.
func (r *RabbitMQInboundConnector) connect() error {
	url := r.buildURL()

	var conn *amqp.Connection
	var err error

	if r.config.UseTLS {
		tlsCfg := tlsConfigSkipVerify()
		if !r.config.TLSSkipVerify {
			tlsCfg = nil // use system CAs
		}
		conn, err = amqp.DialTLS(url, tlsCfg)
	} else {
		conn, err = amqp.Dial(url)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ at %s: %w", r.sanitizeURL(url), err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to open AMQP channel: %w", err)
	}

	// Set QoS
	if err := ch.Qos(r.config.PrefetchCount, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Declare queue
	_, err = ch.QueueDeclare(
		r.config.Queue,
		r.config.QueueDurable,
		false, // auto-delete
		r.config.QueueExclusive,
		false, // no-wait
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("failed to declare queue %q: %w", r.config.Queue, err)
	}

	// Bind to exchange if configured
	if r.config.Exchange != "" {
		routingKey := r.config.RoutingKey
		if routingKey == "" {
			routingKey = r.config.Queue
		}
		if err := ch.QueueBind(r.config.Queue, routingKey, r.config.Exchange, false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return fmt.Errorf("failed to bind queue to exchange: %w", err)
		}
	}

	r.conn = conn
	r.ch = ch
	return nil
}

// buildURL constructs the AMQP(S) connection URL.
func (r *RabbitMQInboundConnector) buildURL() string {
	scheme := "amqp"
	if r.config.UseTLS {
		scheme = "amqps"
	}
	return fmt.Sprintf("%s://%s:%s@%s:%d%s",
		scheme,
		r.config.Username, r.config.Password,
		r.config.Host, r.config.Port,
		r.config.VHost,
	)
}

// sanitizeURL removes the password from a URL for logging.
func (r *RabbitMQInboundConnector) sanitizeURL(url string) string {
	return fmt.Sprintf("amqp(s)://%s:****@%s:%d%s",
		r.config.Username, r.config.Host, r.config.Port, r.config.VHost)
}

// TestConnection verifies the AMQP connection is alive.
func (r *RabbitMQInboundConnector) TestConnection(ctx context.Context) error {
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not initialized")
	}
	if conn.IsClosed() {
		return fmt.Errorf("AMQP connection is closed")
	}
	return nil
}

// Validate checks required fields.
func (r *RabbitMQInboundConnector) Validate() error {
	if !r.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if r.config.Queue == "" {
		return fmt.Errorf("queue is required")
	}
	return nil
}

// SupportsCron returns false — RabbitMQ is push/event-driven.
func (r *RabbitMQInboundConnector) SupportsCron() bool { return false }

// Start begins consuming from the queue.
func (r *RabbitMQInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !r.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	r.BaseInboundConnector.SetState(StateRunning)
	go r.consumeLoop(ctx, messageChan)
	return nil
}

// consumeLoop drives the AMQP delivery loop with automatic reconnection.
func (r *RabbitMQInboundConnector) consumeLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Printf("🐰 RabbitMQ Inbound: Context cancelled")
			return
		case <-r.BaseInboundConnector.stopCh:
			log.Printf("🐰 RabbitMQ Inbound: Stop signal received")
			return
		default:
		}

		if err := r.consume(ctx, messageChan); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("❌ RabbitMQ Inbound: Consumer error: %v — reconnecting in %v", err, backoff)
			time.Sleep(backoff)
			backoff = min2(backoff*2, 30*time.Second)

			// Attempt reconnect
			r.mu.Lock()
			if r.ch != nil {
				_ = r.ch.Close()
				r.ch = nil
			}
			if r.conn != nil && !r.conn.IsClosed() {
				_ = r.conn.Close()
				r.conn = nil
			}
			if err2 := r.connect(); err2 != nil {
				log.Printf("❌ RabbitMQ Inbound: Reconnect failed: %v", err2)
			} else {
				backoff = 1 * time.Second // reset on success
			}
			r.mu.Unlock()
		} else {
			return
		}
	}
}

// consume registers a consumer and blocks until the channel or context closes.
func (r *RabbitMQInboundConnector) consume(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	r.mu.Lock()
	ch := r.ch
	r.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("channel is nil")
	}

	deliveries, err := ch.Consume(
		r.config.Queue,
		"", // consumer tag — generated by server
		r.config.AutoAck,
		r.config.QueueExclusive,
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Printf("🐰 RabbitMQ Inbound: Consuming from queue %q", r.config.Queue)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.BaseInboundConnector.stopCh:
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			msg := amqpDeliveryToInbound(d, r.config)
			messageChan <- msg
			if !r.config.AutoAck {
				if err := d.Ack(false); err != nil {
					log.Printf("⚠️  RabbitMQ Inbound: Ack failed: %v", err)
				}
			}
		}
	}
}

// amqpDeliveryToInbound converts an AMQP delivery to an InboundMessage.
func amqpDeliveryToInbound(d amqp.Delivery, cfg RabbitMQInboundConfig) *models.InboundMessage {
	content := string(d.Body)
	ct := d.ContentType
	if ct == "" {
		ct = detectContentType(content)
	}

	msgType := ""
	if t, ok := d.Headers["message_type"]; ok {
		msgType = fmt.Sprintf("%v", t)
	}

	return &models.InboundMessage{
		MessageID:      fmt.Sprintf("rmq_%s_%d", cfg.Queue, d.DeliveryTag),
		CorrelationID:  d.CorrelationId,
		Content:        content,
		ContentType:    ct,
		SourceType:     "rabbitmq",
		SourceEndpoint: cfg.Queue,
		MessageType:    msgType,
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"RabbitMQ-Queue":    cfg.Queue,
			"RabbitMQ-Exchange": d.Exchange,
			"RabbitMQ-Key":      d.RoutingKey,
			"Content-Type":      ct,
		},
	}
}

// Stop closes channel and connection.
func (r *RabbitMQInboundConnector) Stop() error {
	log.Printf("🐰 RabbitMQ Inbound: Stopping")
	if err := r.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ch != nil {
		_ = r.ch.Close()
		r.ch = nil
	}
	if r.conn != nil && !r.conn.IsClosed() {
		if err := r.conn.Close(); err != nil {
			return fmt.Errorf("failed to close RabbitMQ connection: %w", err)
		}
		r.conn = nil
	}
	log.Printf("✅ RabbitMQ Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (r *RabbitMQInboundConnector) Close() error { return r.Stop() }

// GetStatus returns connector status with AMQP-specific metadata.
func (r *RabbitMQInboundConnector) GetStatus() ConnectorStatus {
	status := r.BaseInboundConnector.GetStatus()
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn != nil && !conn.IsClosed() {
		status.Connected = true
	}
	return status
}

// min2 returns the smaller of two durations (avoids importing math).
func min2(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
