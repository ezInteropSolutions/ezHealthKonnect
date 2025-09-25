// internal/connectivity/mq_connector.go
// Message Queue connectivity handlers (RabbitMQ, Redis)

package connectivity

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/processing/pkg"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

// RabbitMQInputConnector handles RabbitMQ message consumption
type RabbitMQInputConnector struct {
	*BaseConnector
	connection    *amqp.Connection
	channel       *amqp.Channel
	queueName     string
	exchangeName  string
	routingKey    string
	consumerTag   string
	messageChan   chan<- *pkg.UniversalMessage
	deliveries    <-chan amqp.Delivery
	autoAck       bool
	prefetchCount int
}

// RabbitMQOutputConnector handles RabbitMQ message publishing
type RabbitMQOutputConnector struct {
	*BaseConnector
	connection   *amqp.Connection
	channel      *amqp.Channel
	exchangeName string
	routingKey   string
	persistent   bool
	priority     uint8
}

// RedisInputConnector handles Redis message consumption
type RedisInputConnector struct {
	*BaseConnector
	client       *redis.Client
	streamName   string
	consumerGroup string
	consumerName string
	messageChan  chan<- *pkg.UniversalMessage
	stopChan     chan bool
	batchSize    int64
	blockTime    time.Duration
}

// RedisOutputConnector handles Redis message publishing
type RedisOutputConnector struct {
	*BaseConnector
	client     *redis.Client
	streamName string
	listName   string
	maxLength  int64
	useStream  bool
}

// NewRabbitMQInputConnector creates a new RabbitMQ input connector
func NewRabbitMQInputConnector(config pkg.ConnectorConfig) (*RabbitMQInputConnector, error) {
	base := NewBaseConnector(config, "rabbitmq_input")

	queueName := "messages"
	if queue, exists := config.Settings["queue_name"]; exists {
		if q, ok := queue.(string); ok {
			queueName = q
		}
	}

	exchangeName := ""
	if exchange, exists := config.Settings["exchange_name"]; exists {
		if e, ok := exchange.(string); ok {
			exchangeName = e
		}
	}

	routingKey := ""
	if key, exists := config.Settings["routing_key"]; exists {
		if k, ok := key.(string); ok {
			routingKey = k
		}
	}

	autoAck := false
	if ack, exists := config.Settings["auto_ack"]; exists {
		if a, ok := ack.(bool); ok {
			autoAck = a
		}
	}

	prefetchCount := 10
	if prefetch, exists := config.Settings["prefetch_count"]; exists {
		if p, ok := prefetch.(float64); ok {
			prefetchCount = int(p)
		}
	}

	consumerTag := fmt.Sprintf("consumer_%s", uuid.New().String()[:8])

	connector := &RabbitMQInputConnector{
		BaseConnector: base,
		queueName:     queueName,
		exchangeName:  exchangeName,
		routingKey:    routingKey,
		consumerTag:   consumerTag,
		autoAck:       autoAck,
		prefetchCount: prefetchCount,
	}

	return connector, nil
}

// NewRabbitMQOutputConnector creates a new RabbitMQ output connector
func NewRabbitMQOutputConnector(config pkg.ConnectorConfig) (*RabbitMQOutputConnector, error) {
	base := NewBaseConnector(config, "rabbitmq_output")

	exchangeName := ""
	if exchange, exists := config.Settings["exchange_name"]; exists {
		if e, ok := exchange.(string); ok {
			exchangeName = e
		}
	}

	routingKey := "messages"
	if key, exists := config.Settings["routing_key"]; exists {
		if k, ok := key.(string); ok {
			routingKey = k
		}
	}

	persistent := true
	if pers, exists := config.Settings["persistent"]; exists {
		if p, ok := pers.(bool); ok {
			persistent = p
		}
	}

	priority := uint8(0)
	if prio, exists := config.Settings["priority"]; exists {
		if p, ok := prio.(float64); ok {
			priority = uint8(p)
		}
	}

	connector := &RabbitMQOutputConnector{
		BaseConnector: base,
		exchangeName:  exchangeName,
		routingKey:    routingKey,
		persistent:    persistent,
		priority:      priority,
	}

	return connector, nil
}

// NewRedisInputConnector creates a new Redis input connector
func NewRedisInputConnector(config pkg.ConnectorConfig) (*RedisInputConnector, error) {
	base := NewBaseConnector(config, "redis_input")

	streamName := "messages"
	if stream, exists := config.Settings["stream_name"]; exists {
		if s, ok := stream.(string); ok {
			streamName = s
		}
	}

	consumerGroup := "processors"
	if group, exists := config.Settings["consumer_group"]; exists {
		if g, ok := group.(string); ok {
			consumerGroup = g
		}
	}

	consumerName := fmt.Sprintf("consumer_%s", uuid.New().String()[:8])
	if name, exists := config.Settings["consumer_name"]; exists {
		if n, ok := name.(string); ok {
			consumerName = n
		}
	}

	batchSize := int64(10)
	if batch, exists := config.Settings["batch_size"]; exists {
		if b, ok := batch.(float64); ok {
			batchSize = int64(b)
		}
	}

	blockTime := 5 * time.Second
	if block, exists := config.Settings["block_time"]; exists {
		if blockStr, ok := block.(string); ok {
			if parsed, err := time.ParseDuration(blockStr); err == nil {
				blockTime = parsed
			}
		}
	}

	connector := &RedisInputConnector{
		BaseConnector: base,
		streamName:    streamName,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		stopChan:      make(chan bool),
		batchSize:     batchSize,
		blockTime:     blockTime,
	}

	return connector, nil
}

// NewRedisOutputConnector creates a new Redis output connector
func NewRedisOutputConnector(config pkg.ConnectorConfig) (*RedisOutputConnector, error) {
	base := NewBaseConnector(config, "redis_output")

	streamName := "messages"
	if stream, exists := config.Settings["stream_name"]; exists {
		if s, ok := stream.(string); ok {
			streamName = s
		}
	}

	listName := "messages"
	if list, exists := config.Settings["list_name"]; exists {
		if l, ok := list.(string); ok {
			listName = l
		}
	}

	maxLength := int64(10000)
	if maxLen, exists := config.Settings["max_length"]; exists {
		if ml, ok := maxLen.(float64); ok {
			maxLength = int64(ml)
		}
	}

	useStream := true
	if stream, exists := config.Settings["use_stream"]; exists {
		if s, ok := stream.(bool); ok {
			useStream = s
		}
	}

	connector := &RedisOutputConnector{
		BaseConnector: base,
		streamName:    streamName,
		listName:      listName,
		maxLength:     maxLength,
		useStream:     useStream,
	}

	return connector, nil
}

// RabbitMQ Input Connector Methods

// Connect establishes RabbitMQ connection
func (rc *RabbitMQInputConnector) Connect() error {
	connectionString := rc.GetConnectionString()
	if !strings.HasPrefix(connectionString, "amqp://") {
		connectionString = fmt.Sprintf("amqp://%s:%s@%s:%d/",
			rc.Config.Username, rc.Config.Password,
			rc.Config.Endpoint, rc.Config.Port)
	}

	conn, err := amqp.Dial(connectionString)
	if err != nil {
		rc.RecordError(err)
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to create channel: %w", err)
	}

	// Set QoS
	if err := channel.Qos(rc.prefetchCount, 0, false); err != nil {
		channel.Close()
		conn.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Declare queue
	_, err = channel.QueueDeclare(
		rc.queueName, // name
		true,         // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		channel.Close()
		conn.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to exchange if specified
	if rc.exchangeName != "" {
		err = channel.QueueBind(
			rc.queueName,    // queue name
			rc.routingKey,   // routing key
			rc.exchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			channel.Close()
			conn.Close()
			rc.RecordError(err)
			return fmt.Errorf("failed to bind queue: %w", err)
		}
	}

	rc.connection = conn
	rc.channel = channel
	rc.SetConnected(true)

	return nil
}

// Disconnect closes RabbitMQ connection
func (rc *RabbitMQInputConnector) Disconnect() error {
	if rc.channel != nil {
		rc.channel.Close()
		rc.channel = nil
	}
	if rc.connection != nil {
		rc.connection.Close()
		rc.connection = nil
	}
	rc.SetConnected(false)
	return nil
}

// TestConnection tests RabbitMQ connectivity
func (rc *RabbitMQInputConnector) TestConnection() error {
	if err := rc.Connect(); err != nil {
		return err
	}
	defer rc.Disconnect()
	return nil
}

// StartListening begins consuming RabbitMQ messages
func (rc *RabbitMQInputConnector) StartListening(messageChan chan<- *pkg.UniversalMessage) error {
	if err := rc.Start(rc.ctx); err != nil {
		return err
	}

	if !rc.IsConnected() {
		if err := rc.Connect(); err != nil {
			return err
		}
	}

	rc.messageChan = messageChan

	// Start consuming messages
	deliveries, err := rc.channel.Consume(
		rc.queueName,   // queue
		rc.consumerTag, // consumer
		rc.autoAck,     // auto-ack
		false,          // exclusive
		false,          // no-local
		false,          // no-wait
		nil,            // args
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	rc.deliveries = deliveries

	// Process messages in goroutine
	go rc.processMessages()

	return nil
}

// StopListening stops consuming RabbitMQ messages
func (rc *RabbitMQInputConnector) StopListening() error {
	if rc.channel != nil {
		rc.channel.Cancel(rc.consumerTag, false)
	}
	return rc.Stop()
}

// processMessages processes incoming RabbitMQ messages
func (rc *RabbitMQInputConnector) processMessages() {
	for {
		select {
		case <-rc.Context().Done():
			return
		case delivery, ok := <-rc.deliveries:
			if !ok {
				return
			}

			rc.handleDelivery(delivery)
		}
	}
}

// handleDelivery processes a single RabbitMQ delivery
func (rc *RabbitMQInputConnector) handleDelivery(delivery amqp.Delivery) {
	startTime := time.Now()

	// Create universal message
	message := pkg.NewUniversalMessage()
	message.Content = string(delivery.Body)
	message.ContentType = delivery.ContentType
	if message.ContentType == "" {
		message.ContentType = "TEXT"
	}
	message.SourceProtocol = string(rc.Protocol)
	message.Size = int64(len(delivery.Body))

	// Extract metadata from headers
	if delivery.Headers != nil {
		for key, value := range delivery.Headers {
			message.Metadata[key] = value
		}
	}

	// Add RabbitMQ-specific metadata
	message.Metadata["rabbitmq_delivery_tag"] = delivery.DeliveryTag
	message.Metadata["rabbitmq_exchange"] = delivery.Exchange
	message.Metadata["rabbitmq_routing_key"] = delivery.RoutingKey
	message.Metadata["rabbitmq_redelivered"] = delivery.Redelivered

	// Extract message ID from headers if present
	if msgID, exists := delivery.Headers["message_id"]; exists {
		if id, ok := msgID.(string); ok {
			message.ID = id
		}
	}

	if corrID := delivery.CorrelationId; corrID != "" {
		message.CorrelationID = corrID
	}

	// Send to processing channel
	select {
	case rc.messageChan <- message:
		// Acknowledge if not auto-ack
		if !rc.autoAck {
			delivery.Ack(false)
		}

		// Record metrics
		latency := time.Since(startTime).Milliseconds()
		rc.RecordMessage(message.Size, latency)

	case <-time.After(5 * time.Second):
		// Channel full - reject message
		if !rc.autoAck {
			delivery.Nack(false, true) // Requeue
		}
	}
}

// RabbitMQ Output Connector Methods

// Connect establishes RabbitMQ connection (output)
func (rc *RabbitMQOutputConnector) Connect() error {
	connectionString := rc.GetConnectionString()
	if !strings.HasPrefix(connectionString, "amqp://") {
		connectionString = fmt.Sprintf("amqp://%s:%s@%s:%d/",
			rc.Config.Username, rc.Config.Password,
			rc.Config.Endpoint, rc.Config.Port)
	}

	conn, err := amqp.Dial(connectionString)
	if err != nil {
		rc.RecordError(err)
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to create channel: %w", err)
	}

	// Declare exchange if specified
	if rc.exchangeName != "" {
		err = channel.ExchangeDeclare(
			rc.exchangeName, // name
			"direct",        // type
			true,            // durable
			false,           // auto-deleted
			false,           // internal
			false,           // no-wait
			nil,             // arguments
		)
		if err != nil {
			channel.Close()
			conn.Close()
			rc.RecordError(err)
			return fmt.Errorf("failed to declare exchange: %w", err)
		}
	}

	rc.connection = conn
	rc.channel = channel
	rc.SetConnected(true)

	return nil
}

// Disconnect closes RabbitMQ connection (output)
func (rc *RabbitMQOutputConnector) Disconnect() error {
	if rc.channel != nil {
		rc.channel.Close()
		rc.channel = nil
	}
	if rc.connection != nil {
		rc.connection.Close()
		rc.connection = nil
	}
	rc.SetConnected(false)
	return nil
}

// TestConnection tests RabbitMQ connectivity (output)
func (rc *RabbitMQOutputConnector) TestConnection() error {
	if err := rc.Connect(); err != nil {
		return err
	}
	defer rc.Disconnect()
	return nil
}

// SendMessage publishes a message to RabbitMQ
func (rc *RabbitMQOutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	if !rc.IsConnected() {
		if err := rc.Connect(); err != nil {
			return err
		}
	}

	startTime := time.Now()

	// Prepare message headers
	headers := make(amqp.Table)
	headers["message_id"] = message.ID
	headers["correlation_id"] = message.CorrelationID
	headers["source_interface"] = message.SourceInterface
	headers["timestamp"] = message.CreatedAt.Unix()

	// Add custom metadata
	for key, value := range message.Metadata {
		if key != "rabbitmq_delivery_tag" { // Skip internal metadata
			headers[key] = value
		}
	}

	// Prepare publishing
	publishing := amqp.Publishing{
		Headers:       headers,
		ContentType:   message.ContentType,
		Body:          []byte(message.Content),
		MessageId:     message.ID,
		CorrelationId: message.CorrelationID,
		Timestamp:     message.CreatedAt,
		Priority:      rc.priority,
	}

	if rc.persistent {
		publishing.DeliveryMode = amqp.Persistent
	}

	// Publish message
	err := rc.channel.PublishWithContext(
		ctx,
		rc.exchangeName, // exchange
		rc.routingKey,   // routing key
		false,           // mandatory
		false,           // immediate
		publishing,
	)

	if err != nil {
		rc.RecordError(err)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Update message status
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	rc.RecordMessage(int64(len(message.Content)), latency)

	return nil
}

// SendBatch publishes multiple messages to RabbitMQ
func (rc *RabbitMQOutputConnector) SendBatch(ctx context.Context, messages []*pkg.UniversalMessage) error {
	for _, message := range messages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := rc.SendMessage(ctx, message); err != nil {
				return err
			}
		}
	}
	return nil
}

// SupportsAcknowledgment returns true for RabbitMQ
func (rc *RabbitMQOutputConnector) SupportsAcknowledgment() bool {
	return true
}

// WaitForAcknowledgment waits for message acknowledgment
func (rc *RabbitMQOutputConnector) WaitForAcknowledgment(messageID string, timeout time.Duration) error {
	// RabbitMQ publishes are synchronous, so this is effectively a no-op
	return nil
}

// Redis Input Connector Methods

// Connect establishes Redis connection
func (rc *RedisInputConnector) Connect() error {
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", rc.Config.Endpoint, rc.Config.Port),
		Password: rc.Config.Password,
		DB:       0,
	}

	if db, exists := rc.Config.Settings["database"]; exists {
		if dbInt, ok := db.(float64); ok {
			options.DB = int(dbInt)
		}
	}

	client := redis.NewClient(options)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), rc.Config.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Create consumer group
	err := client.XGroupCreateMkStream(ctx, rc.streamName, rc.consumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		client.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	rc.client = client
	rc.SetConnected(true)

	return nil
}

// Disconnect closes Redis connection
func (rc *RedisInputConnector) Disconnect() error {
	if rc.client != nil {
		rc.client.Close()
		rc.client = nil
	}
	rc.SetConnected(false)
	return nil
}

// TestConnection tests Redis connectivity
func (rc *RedisInputConnector) TestConnection() error {
	if err := rc.Connect(); err != nil {
		return err
	}
	defer rc.Disconnect()
	return nil
}

// StartListening begins consuming Redis messages
func (rc *RedisInputConnector) StartListening(messageChan chan<- *pkg.UniversalMessage) error {
	if err := rc.Start(rc.ctx); err != nil {
		return err
	}

	if !rc.IsConnected() {
		if err := rc.Connect(); err != nil {
			return err
		}
	}

	rc.messageChan = messageChan

	// Start consuming messages
	go rc.consumeMessages()

	return nil
}

// StopListening stops consuming Redis messages
func (rc *RedisInputConnector) StopListening() error {
	select {
	case rc.stopChan <- true:
	default:
	}
	return rc.Stop()
}

// consumeMessages processes incoming Redis messages
func (rc *RedisInputConnector) consumeMessages() {
	for {
		select {
		case <-rc.Context().Done():
			return
		case <-rc.stopChan:
			return
		default:
			if err := rc.readMessages(); err != nil {
				rc.RecordError(err)
				time.Sleep(1 * time.Second) // Back off on error
			}
		}
	}
}

// readMessages reads messages from Redis stream
func (rc *RedisInputConnector) readMessages() error {
	ctx, cancel := context.WithTimeout(context.Background(), rc.blockTime+5*time.Second)
	defer cancel()

	streams, err := rc.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    rc.consumerGroup,
		Consumer: rc.consumerName,
		Streams:  []string{rc.streamName, ">"},
		Count:    rc.batchSize,
		Block:    rc.blockTime,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil // No messages
		}
		return err
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			rc.handleRedisMessage(message)
		}
	}

	return nil
}

// handleRedisMessage processes a single Redis message
func (rc *RedisInputConnector) handleRedisMessage(msg redis.XMessage) {
	startTime := time.Now()

	// Create universal message
	message := pkg.NewUniversalMessage()

	// Extract content and metadata from Redis message
	if content, exists := msg.Values["content"]; exists {
		if contentStr, ok := content.(string); ok {
			message.Content = contentStr
		}
	}

	if contentType, exists := msg.Values["content_type"]; exists {
		if ct, ok := contentType.(string); ok {
			message.ContentType = ct
		}
	}

	if message.ContentType == "" {
		message.ContentType = "TEXT"
	}

	message.SourceProtocol = string(rc.Protocol)
	message.Size = int64(len(message.Content))

	// Extract other fields
	if msgID, exists := msg.Values["message_id"]; exists {
		if id, ok := msgID.(string); ok {
			message.ID = id
		}
	}

	if corrID, exists := msg.Values["correlation_id"]; exists {
		if id, ok := corrID.(string); ok {
			message.CorrelationID = id
		}
	}

	// Add Redis-specific metadata
	message.Metadata["redis_message_id"] = msg.ID
	message.Metadata["redis_stream"] = rc.streamName

	// Add all other fields as metadata
	for key, value := range msg.Values {
		if key != "content" && key != "content_type" && key != "message_id" && key != "correlation_id" {
			message.Metadata[key] = value
		}
	}

	// Send to processing channel
	select {
	case rc.messageChan <- message:
		// Acknowledge message
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		rc.client.XAck(ctx, rc.streamName, rc.consumerGroup, msg.ID)
		cancel()

		// Record metrics
		latency := time.Since(startTime).Milliseconds()
		rc.RecordMessage(message.Size, latency)

	case <-time.After(5 * time.Second):
		// Channel full - message will be redelivered
	}
}

// Redis Output Connector Methods

// Connect establishes Redis connection (output)
func (rc *RedisOutputConnector) Connect() error {
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", rc.Config.Endpoint, rc.Config.Port),
		Password: rc.Config.Password,
		DB:       0,
	}

	if db, exists := rc.Config.Settings["database"]; exists {
		if dbInt, ok := db.(float64); ok {
			options.DB = int(dbInt)
		}
	}

	client := redis.NewClient(options)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), rc.Config.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		rc.RecordError(err)
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	rc.client = client
	rc.SetConnected(true)

	return nil
}

// Disconnect closes Redis connection (output)
func (rc *RedisOutputConnector) Disconnect() error {
	if rc.client != nil {
		rc.client.Close()
		rc.client = nil
	}
	rc.SetConnected(false)
	return nil
}

// TestConnection tests Redis connectivity (output)
func (rc *RedisOutputConnector) TestConnection() error {
	if err := rc.Connect(); err != nil {
		return err
	}
	defer rc.Disconnect()
	return nil
}

// SendMessage publishes a message to Redis
func (rc *RedisOutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	if !rc.IsConnected() {
		if err := rc.Connect(); err != nil {
			return err
		}
	}

	startTime := time.Now()

	if rc.useStream {
		// Send to Redis Stream
		values := map[string]interface{}{
			"message_id":     message.ID,
			"correlation_id": message.CorrelationID,
			"content":        message.Content,
			"content_type":   message.ContentType,
			"timestamp":      message.CreatedAt.Unix(),
		}

		// Add metadata
		for key, value := range message.Metadata {
			if !strings.HasPrefix(key, "redis_") { // Skip internal metadata
				values[key] = value
			}
		}

		_, err := rc.client.XAdd(ctx, &redis.XAddArgs{
			Stream: rc.streamName,
			MaxLen: rc.maxLength,
			Approx: true,
			Values: values,
		}).Result()

		if err != nil {
			rc.RecordError(err)
			return fmt.Errorf("failed to add to stream: %w", err)
		}
	} else {
		// Send to Redis List
		messageData, err := json.Marshal(map[string]interface{}{
			"message_id":     message.ID,
			"correlation_id": message.CorrelationID,
			"content":        message.Content,
			"content_type":   message.ContentType,
			"timestamp":      message.CreatedAt.Unix(),
			"metadata":       message.Metadata,
		})
		if err != nil {
			rc.RecordError(err)
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		_, err = rc.client.LPush(ctx, rc.listName, messageData).Result()
		if err != nil {
			rc.RecordError(err)
			return fmt.Errorf("failed to push to list: %w", err)
		}

		// Trim list to max length
		if rc.maxLength > 0 {
			rc.client.LTrim(ctx, rc.listName, 0, rc.maxLength-1)
		}
	}

	// Update message status
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	rc.RecordMessage(int64(len(message.Content)), latency)

	return nil
}

// SendBatch publishes multiple messages to Redis
func (rc *RedisOutputConnector) SendBatch(ctx context.Context, messages []*pkg.UniversalMessage) error {
	if rc.useStream {
		// Use pipeline for batch stream operations
		pipe := rc.client.Pipeline()

		for _, message := range messages {
			values := map[string]interface{}{
				"message_id":     message.ID,
				"correlation_id": message.CorrelationID,
				"content":        message.Content,
				"content_type":   message.ContentType,
				"timestamp":      message.CreatedAt.Unix(),
			}

			for key, value := range message.Metadata {
				if !strings.HasPrefix(key, "redis_") {
					values[key] = value
				}
			}

			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream: rc.streamName,
				MaxLen: rc.maxLength,
				Approx: true,
				Values: values,
			})
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to execute batch stream operations: %w", err)
		}
	} else {
		// Use pipeline for batch list operations
		pipe := rc.client.Pipeline()

		for _, message := range messages {
			messageData, err := json.Marshal(map[string]interface{}{
				"message_id":     message.ID,
				"correlation_id": message.CorrelationID,
				"content":        message.Content,
				"content_type":   message.ContentType,
				"timestamp":      message.CreatedAt.Unix(),
				"metadata":       message.Metadata,
			})
			if err != nil {
				return fmt.Errorf("failed to marshal message: %w", err)
			}

			pipe.LPush(ctx, rc.listName, messageData)
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to execute batch list operations: %w", err)
		}

		// Trim list
		if rc.maxLength > 0 {
			rc.client.LTrim(ctx, rc.listName, 0, rc.maxLength-1)
		}
	}

	// Update message statuses
	now := time.Now()
	for _, message := range messages {
		message.Status = pkg.StatusDelivered
		message.DeliveredAt = &now
	}

	return nil
}

// SupportsAcknowledgment returns true for Redis streams
func (rc *RedisOutputConnector) SupportsAcknowledgment() bool {
	return rc.useStream
}

// WaitForAcknowledgment is not applicable for Redis publishing
func (rc *RedisOutputConnector) WaitForAcknowledgment(messageID string, timeout time.Duration) error {
	return nil
}