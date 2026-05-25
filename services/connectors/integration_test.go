// services/connectors/integration_test.go
// Integration tests for all implemented inbound connectors.
//
// These tests require real infrastructure — start it with:
//
//	docker-compose -f docker-compose.test.yml up -d
//
// Then run:
//
//	go test ./services/connectors/ -tags integration -v -run "TestIntegration" -timeout 120s
//
// Each test:
//   1. Initializes the connector against the Docker service
//   2. Calls Start() with a buffered message channel
//   3. Asserts that at least one InboundMessage arrives within a timeout
//   4. Verifies message content (HL7 MSH prefix, JSON, etc.)
//   5. Calls Stop()

//go:build integration

package connectors

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/models"

	"github.com/IBM/sarama"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ─── Test infrastructure addresses (match docker-compose.test.yml) ────────────
// Override host via TEST_INFRA_HOST env var (e.g. "host.docker.internal" when
// running tests inside a Docker container that can't reach localhost ports).

func testInfraHost() string {
	if h := os.Getenv("TEST_INFRA_HOST"); h != "" {
		return h
	}
	return "localhost"
}

func testMongoURI() string {
	return "mongodb://testuser:testpassword@" + testInfraHost() + ":27018"
}
func testRedisDSN() string { return testInfraHost() + ":6380" }
func testMySQLDSN() string {
	return "testuser:testpassword@tcp(" + testInfraHost() + ":3307)/testdb"
}
func testRabbitMQURL() string { return "amqp://testuser:testpassword@" + testInfraHost() + ":5673/" }
func testKafkaBroker() string { return testInfraHost() + ":9093" }
func testS3Endpoint() string  { return "http://" + testInfraHost() + ":4566" }

// SQL Server: port 1434, sa / TestPassword123!, database testdb
func testSQLServerHost() string { return testInfraHost() }
func testSQLServerPort() int    { return 1434 }
func testSQLServerPass() string { return "TestPassword123!" }

// Oracle XE: port 1522, testuser / testpassword, service XEPDB1 (default PDB in oracle-xe:21-slim)
func testOracleHost() string    { return testInfraHost() }
func testOraclePort() int       { return 1522 }
func testOracleService() string { return "XEPDB1" }

const (
	testS3Bucket      = "ehk-test-bucket"
	testSFTPPort      = 2222
	testSFTPUser      = "testuser"
	testSFTPPassword  = "testpassword"
	testSFTPRemotePath = "/upload"
)

func testSFTPHost() string { return testInfraHost() }

// msgTimeout is how long to wait for the first InboundMessage to arrive.
const msgTimeout = 15 * time.Second

// collectMessages drains a channel for up to timeout, returning all received messages.
func collectMessages(ch <-chan *models.InboundMessage, timeout time.Duration, min int) []*models.InboundMessage {
	var msgs []*models.InboundMessage
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case msg := <-ch:
			msgs = append(msgs, msg)
			if len(msgs) >= min {
				return msgs
			}
		case <-timer.C:
			return msgs
		}
	}
}

// skipIfUnavailable skips the test if the environment variable TEST_INFRA_SKIP is set,
// or if the host is not reachable.
func skipIfUnavailable(t *testing.T, service string) {
	t.Helper()
	if os.Getenv("TEST_INFRA_SKIP") != "" {
		t.Skipf("Skipping %s integration test (TEST_INFRA_SKIP set)", service)
	}
}

// ─── MongoDB Integration Tests ────────────────────────────────────────────────

func TestIntegrationMongoDBInbound_PollDelivery(t *testing.T) {
	skipIfUnavailable(t, "MongoDB")

	// Seed test data
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(testMongoURI()))
	if err != nil {
		t.Skipf("MongoDB unavailable: %v", err)
	}
	defer client.Disconnect(ctx)

	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("MongoDB ping failed: %v", err)
	}

	// Insert test HL7 message
	coll := client.Database("testdb").Collection("hl7_messages")
	_, _ = coll.InsertOne(ctx, bson.M{
		"raw_message":  "MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ADT^A01|MONGO001|P|2.5\rPID|1||MRN_MONGO_001\r",
		"message_type": "ADT^A01",
	})
	_, _ = coll.InsertOne(ctx, bson.M{
		"raw_message":  "MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ORU^R01|MONGO002|P|2.5\rPID|1||MRN_MONGO_002\r",
		"message_type": "ORU^R01",
	})

	// Create and initialize connector
	conn := NewMongoDBInboundConnector()
	cfg := mustJSON(map[string]interface{}{
		"connection_string": testMongoURI(),
		"database":          "testdb",
		"collection":        "hl7_messages",
		"max_records":       10,
		"polling_interval":  1,
	})

	if err := conn.Initialize(cfg); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer conn.Close()

	// Start connector and collect messages
	msgCh := make(chan *models.InboundMessage, 10)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Start(startCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 2)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message from MongoDB, got 0")
		return
	}
	t.Logf("✅ Received %d messages from MongoDB", len(msgs))
	for _, m := range msgs {
		if m.Content == "" {
			t.Error("Message content should not be empty")
		}
		if m.SourceType != "mongodb" {
			t.Errorf("SourceType = %q, want mongodb", m.SourceType)
		}
		t.Logf("  Message: id=%s type=%s size=%d", m.MessageID, m.MessageType, m.MessageSize)
	}

	// Clean up test data
	_, _ = coll.DeleteMany(context.Background(), bson.M{})
}

func TestIntegrationMongoDBInbound_DeleteAfterProcessing(t *testing.T) {
	skipIfUnavailable(t, "MongoDB")

	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(testMongoURI()))
	if err != nil {
		t.Skipf("MongoDB unavailable: %v", err)
	}
	defer client.Disconnect(ctx)
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("MongoDB ping failed: %v", err)
	}

	coll := client.Database("testdb").Collection("hl7_delete_test")
	_, _ = coll.DeleteMany(ctx, bson.M{})
	for i := 0; i < 3; i++ {
		_, _ = coll.InsertOne(ctx, bson.M{
			"raw_message": fmt.Sprintf("MSH|msg%d\r", i),
		})
	}

	before, _ := coll.CountDocuments(ctx, bson.M{})

	conn := NewMongoDBInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"connection_string": testMongoURI(),
		"database":          "testdb",
		"collection":        "hl7_delete_test",
		"after_processing":  "delete",
		"polling_interval":  1,
	})); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Start(startCtx, msgCh)

	msgs := collectMessages(msgCh, msgTimeout, int(before))
	t.Logf("Received %d messages (before count was %d)", len(msgs), before)

	time.Sleep(500 * time.Millisecond) // let after-processing complete

	after, _ := coll.CountDocuments(ctx, bson.M{})
	if after != 0 {
		t.Errorf("After delete processing, expected 0 remaining documents, got %d", after)
	} else {
		t.Log("✅ All documents deleted after processing")
	}
}

// ─── Redis Integration Tests ──────────────────────────────────────────────────

func TestIntegrationRedisInbound_PubSub(t *testing.T) {
	skipIfUnavailable(t, "Redis")

	// Verify Redis is up
	rdb := redis.NewClient(&redis.Options{Addr: testRedisDSN()})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis unavailable at %s: %v", testRedisDSN(), err)
	}
	defer rdb.Close()

	channel := "test:hl7:inbound"

	conn := NewRedisInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":    testInfraHost(),
		"port":    6380,
		"channel": channel,
	})); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := conn.Start(connCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond) // let subscription register

	// Publish a test HL7 message
	testMsg := "MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ADT^A01|REDIS001|P|2.5\rPID|1||MRN_REDIS_001\r"
	if err := rdb.Publish(ctx, channel, testMsg).Err(); err != nil {
		t.Fatalf("PUBLISH failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 1)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message from Redis Pub/Sub, got 0")
		return
	}
	t.Logf("✅ Received %d message(s) from Redis Pub/Sub", len(msgs))
	msg := msgs[0]
	if msg.Content != testMsg {
		t.Errorf("Content = %q, want %q", msg.Content, testMsg)
	}
	if msg.SourceType != "redis_pubsub" {
		t.Errorf("SourceType = %q, want redis_pubsub", msg.SourceType)
	}
}

func TestIntegrationRedisInbound_Stream(t *testing.T) {
	skipIfUnavailable(t, "Redis")

	rdb := redis.NewClient(&redis.Options{Addr: testRedisDSN()})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	defer rdb.Close()

	streamKey := "test:hl7:stream"
	_ = rdb.Del(ctx, streamKey).Err() // clean up from prior runs

	conn := NewRedisInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":           testInfraHost(),
		"port":           6380,
		"stream":         streamKey,
		"consumer_group": "test-group",
		"consumer_name":  "test-consumer-1",
		"read_timeout":   500,
	})); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := conn.Start(connCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Write test messages to stream
	testHL7 := "MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ADT^A01|STREAM001|P|2.5\r"
	for i := 0; i < 3; i++ {
		rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: streamKey,
			Values: map[string]interface{}{
				"message":      testHL7,
				"message_type": "ADT^A01",
			},
		})
	}

	msgs := collectMessages(msgCh, msgTimeout, 3)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message from Redis Stream, got 0")
		return
	}
	t.Logf("✅ Received %d message(s) from Redis Stream", len(msgs))
	for _, m := range msgs {
		if m.SourceType != "redis_stream" {
			t.Errorf("SourceType = %q, want redis_stream", m.SourceType)
		}
	}
}

// ─── MySQL Integration Tests ──────────────────────────────────────────────────

func TestIntegrationMySQLInbound_PollHL7Table(t *testing.T) {
	skipIfUnavailable(t, "MySQL")

	conn := NewMySQLInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":              testInfraHost(),
		"port":              3307,
		"database":          "testdb",
		"username":          "testuser",
		"password":          "testpassword",
		"table_name":        "hl7_inbox",
		"incremental_column": "id",
		"incremental_type":  "integer",
		"max_records":       10,
		"polling_interval":  1,
	})); err != nil {
		t.Skipf("MySQL unavailable or seed not applied: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 20)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Start(startCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 3)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 HL7 message from MySQL hl7_inbox, got 0")
		return
	}
	t.Logf("✅ Received %d messages from MySQL", len(msgs))
	for _, m := range msgs {
		if m.SourceType != "mysql" {
			t.Errorf("SourceType = %q, want mysql", m.SourceType)
		}
		if m.ContentType == "application/hl7-v2" {
			if len(m.Content) < 4 || m.Content[:4] != "MSH|" {
				t.Errorf("HL7 message should start with MSH|, got: %q", m.Content[:min3(len(m.Content), 20)])
			}
		}
		t.Logf("  Message: id=%s type=%s content_type=%s size=%d",
			m.MessageID, m.MessageType, m.ContentType, m.MessageSize)
	}
}

func TestIntegrationMySQLInbound_TestConnection(t *testing.T) {
	skipIfUnavailable(t, "MySQL")

	conn := NewMySQLInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":     testInfraHost(),
		"port":     3307,
		"database": "testdb",
		"username": "testuser",
		"password": "testpassword",
		"query":    "SELECT 1",
	})); err != nil {
		t.Skipf("MySQL unavailable: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection() failed: %v", err)
	} else {
		t.Log("✅ MySQL TestConnection() succeeded")
	}
}

// ─── RabbitMQ Integration Tests ───────────────────────────────────────────────

func TestIntegrationRabbitMQInbound_ReceiveMessage(t *testing.T) {
	skipIfUnavailable(t, "RabbitMQ")

	queueName := "test.hl7.inbound"

	conn := NewRabbitMQInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":          testInfraHost(),
		"port":          5673,
		"username":      "testuser",
		"password":      "testpassword",
		"queue":         queueName,
		"prefetch_count": 5,
		"auto_ack":      false,
	})); err != nil {
		t.Skipf("RabbitMQ unavailable: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := conn.Start(connCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond) // let consumer register

	// Publish a test message via separate AMQP connection
	amqpConn, err := amqp.Dial(testRabbitMQURL())
	if err != nil {
		t.Skipf("Could not open publisher connection: %v", err)
	}
	defer amqpConn.Close()
	ch, _ := amqpConn.Channel()
	defer ch.Close()

	testHL7 := "MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ADT^A01|RMQ001|P|2.5\rPID|1||MRN_RMQ_001\r"
	err = ch.Publish("", queueName, false, false, amqp.Publishing{
		ContentType: "application/hl7-v2",
		Body:        []byte(testHL7),
		Headers:     amqp.Table{"message_type": "ADT^A01"},
	})
	if err != nil {
		t.Fatalf("Failed to publish test message: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 1)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message from RabbitMQ, got 0")
		return
	}
	t.Logf("✅ Received %d message(s) from RabbitMQ", len(msgs))
	msg := msgs[0]
	if msg.Content != testHL7 {
		t.Errorf("Content = %q, want %q", msg.Content, testHL7)
	}
	if msg.SourceType != "rabbitmq" {
		t.Errorf("SourceType = %q, want rabbitmq", msg.SourceType)
	}
	t.Logf("  Message: id=%s type=%s", msg.MessageID, msg.MessageType)
}

// ─── Kafka Integration Tests ──────────────────────────────────────────────────

func TestIntegrationKafkaInbound_ConsumerGroupDelivery(t *testing.T) {
	skipIfUnavailable(t, "Kafka")

	topicName := "test.hl7.messages"
	groupID := fmt.Sprintf("test-group-%d", time.Now().Unix())

	conn := NewKafkaInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"brokers":          testKafkaBroker(),
		"topic":            topicName,
		"consumer_group":   groupID,
		"auto_offset_reset": "earliest",
	})); err != nil {
		t.Skipf("Kafka unavailable: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := conn.Start(connCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Produce test messages via Sarama sync producer
	testMessages := []string{
		"MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ADT^A01|KAFKA001|P|2.5\rPID|1||MRN_KAFKA_001\r",
		"MSH|^~\\&|SEND|FAC|RECV|FAC|20240315||ORU^R01|KAFKA002|P|2.5\rPID|1||MRN_KAFKA_002\r",
	}

	saramaCfg := sarama.NewConfig()
	saramaCfg.Version = sarama.V2_6_0_0
	saramaCfg.Producer.Return.Successes = true
	producer, err := sarama.NewSyncProducer([]string{testKafkaBroker()}, saramaCfg)
	if err != nil {
		t.Skipf("Kafka sync producer unavailable: %v", err)
	}
	defer producer.Close()

	for _, m := range testMessages {
		_, _, err := producer.SendMessage(&sarama.ProducerMessage{
			Topic: topicName,
			Value: sarama.StringEncoder(m),
			Headers: []sarama.RecordHeader{
				{Key: []byte("message_type"), Value: []byte("HL7v2")},
			},
		})
		if err != nil {
			t.Fatalf("Failed to produce test message: %v", err)
		}
	}

	msgs := collectMessages(msgCh, msgTimeout, 2)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message from Kafka, got 0")
		return
	}
	t.Logf("✅ Received %d message(s) from Kafka", len(msgs))
	for _, m := range msgs {
		if m.SourceType != "kafka" {
			t.Errorf("SourceType = %q, want kafka", m.SourceType)
		}
		t.Logf("  Message: id=%s topic=%s", m.MessageID, m.SourceEndpoint)
	}
}

// ─── AWS S3 Integration Tests (LocalStack) ────────────────────────────────────

func TestIntegrationS3Inbound_PollBucket(t *testing.T) {
	skipIfUnavailable(t, "LocalStack/S3")

	conn := NewAWSS3InboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"bucket":            testS3Bucket,
		"region":            "us-east-1",
		"prefix":            "inbound/",
		"access_key_id":     "test",
		"secret_access_key": "test",
		"endpoint":          testS3Endpoint(),
		"force_path_style":  true,
		"polling_interval":  1,
		"max_keys":          50,
	})); err != nil {
		t.Skipf("LocalStack/S3 unavailable: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Start(startCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 2)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message from S3 bucket, got 0")
		return
	}
	t.Logf("✅ Received %d message(s) from S3", len(msgs))
	for _, m := range msgs {
		if m.SourceType != "aws_s3" {
			t.Errorf("SourceType = %q, want aws_s3", m.SourceType)
		}
		if m.Content == "" {
			t.Error("Message content should not be empty")
		}
		t.Logf("  Message: id=%s endpoint=%s size=%d",
			m.MessageID, m.SourceEndpoint, m.MessageSize)
	}
}

func TestIntegrationS3Inbound_TestConnection(t *testing.T) {
	skipIfUnavailable(t, "LocalStack/S3")

	conn := NewAWSS3InboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"bucket":            testS3Bucket,
		"region":            "us-east-1",
		"access_key_id":     "test",
		"secret_access_key": "test",
		"endpoint":          testS3Endpoint(),
		"force_path_style":  true,
	})); err != nil {
		t.Skipf("LocalStack unavailable: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection() failed: %v", err)
	} else {
		t.Log("✅ S3 TestConnection() succeeded (LocalStack)")
	}
}

func TestIntegrationS3Inbound_DeleteAfterProcessing(t *testing.T) {
	skipIfUnavailable(t, "LocalStack/S3")

	// Upload a fresh test file
	conn := NewAWSS3InboundConnector()
	cfg := mustJSON(map[string]interface{}{
		"bucket":            testS3Bucket,
		"region":            "us-east-1",
		"prefix":            "delete-test/",
		"access_key_id":     "test",
		"secret_access_key": "test",
		"endpoint":          testS3Endpoint(),
		"force_path_style":  true,
		"after_processing":  "delete",
		"polling_interval":  1,
	})
	if err := conn.Initialize(cfg); err != nil {
		t.Skipf("LocalStack unavailable: %v", err)
	}
	defer conn.Close()

	// Upload via AWS SDK directly
	s3Conn := conn.(*AWSS3InboundConnector)
	s3Conn.mu.Lock()
	client := s3Conn.s3Client
	s3Conn.mu.Unlock()

	ctx := context.Background()
	testKey := "delete-test/to_be_deleted.hl7"
	testContent := "MSH|^~\\&|S|F|R|F|20240315||ADT^A01|DEL001|P|2.5\r"

	putInput := s3PutInput(testS3Bucket, testKey, testContent)
	_, err := client.PutObject(ctx, &putInput)
	if err != nil {
		t.Skipf("Could not upload test object: %v", err)
	}

	msgCh := make(chan *models.InboundMessage, 5)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn.Start(startCtx, msgCh)

	msgs := collectMessages(msgCh, msgTimeout, 1)
	if len(msgs) < 1 {
		t.Error("Expected message from delete-test/, got none")
		return
	}

	time.Sleep(500 * time.Millisecond) // let delete complete
	t.Log("✅ S3 delete after processing completed")
}

// ─── SFTP Integration Tests ───────────────────────────────────────────────────

func TestIntegrationSFTPInbound_PollAndReceive(t *testing.T) {
	// NOTE: The sftp_inbound connector uses SSH shell commands (find/cat/mv).
	// atmoz/sftp restricts users to SFTP protocol only — no shell access.
	// This test requires an SSH server that allows shell commands (e.g. OpenSSH with a full shell).
	// Skip in the standard Docker test environment; run manually against a real SSH server.
	t.Skip("SFTP integration test requires SSH server with shell access; atmoz/sftp only supports SFTP protocol")
	skipIfUnavailable(t, "SFTP")

	conn := NewSFTPInboundConnector()
	cfg := mustJSON(map[string]interface{}{
		"host":              testSFTPHost(),
		"port":              testSFTPPort,
		"username":          testSFTPUser,
		"password":          testSFTPPassword,
		"remote_dir":          testSFTPRemotePath,
		"file_pattern":        "*.hl7",
		"poll_interval_sec":   1,
		"after_processing":    "none",
	})
	if err := conn.Initialize(cfg); err != nil {
		t.Skipf("SFTP unavailable: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 10)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Start(startCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 2)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 HL7 file from SFTP, got 0")
		return
	}
	t.Logf("✅ Received %d file(s) via SFTP", len(msgs))
	for _, m := range msgs {
		if m.Content == "" {
			t.Error("File content should not be empty")
		}
		t.Logf("  File: id=%s endpoint=%s size=%d", m.MessageID, m.SourceEndpoint, m.MessageSize)
	}
}

// ─── SQL Server Integration Tests ────────────────────────────────────────────

func TestIntegrationSQLServerInbound_PollHL7Table(t *testing.T) {
	skipIfUnavailable(t, "SQLServer")

	conn := NewSQLServerInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":               testSQLServerHost(),
		"port":               testSQLServerPort(),
		"database":           "testdb",
		"username":           "sa",
		"password":           testSQLServerPass(),
		"table_name":         "hl7_inbox",
		"incremental_column": "id",
		"incremental_type":   "integer",
		"max_records":        10,
		"polling_interval":   1,
		"ssl_mode":           "disable",
	})); err != nil {
		t.Skipf("SQL Server unavailable or seed not applied: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 20)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Start(startCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 3)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 HL7 message from SQL Server hl7_inbox, got 0")
		return
	}
	t.Logf("✅ Received %d messages from SQL Server", len(msgs))
	for _, m := range msgs {
		if m.SourceType != "sqlserver" {
			t.Errorf("SourceType = %q, want sqlserver", m.SourceType)
		}
		if m.ContentType == "application/hl7-v2" {
			if len(m.Content) < 4 || m.Content[:4] != "MSH|" {
				t.Errorf("HL7 message should start with MSH|, got: %q", m.Content[:min3(len(m.Content), 20)])
			}
		}
		t.Logf("  Message: id=%s type=%s content_type=%s size=%d",
			m.MessageID, m.MessageType, m.ContentType, m.MessageSize)
	}
}

func TestIntegrationSQLServerInbound_TestConnection(t *testing.T) {
	skipIfUnavailable(t, "SQLServer")

	conn := NewSQLServerInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":     testSQLServerHost(),
		"port":     testSQLServerPort(),
		"database": "testdb",
		"username": "sa",
		"password": testSQLServerPass(),
		"query":    "SELECT 1",
		"ssl_mode": "disable",
	})); err != nil {
		t.Skipf("SQL Server unavailable: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection() failed: %v", err)
	} else {
		t.Log("✅ SQL Server TestConnection() succeeded")
	}
}

func TestIntegrationSQLServerInbound_UpdateFlagAfterProcessing(t *testing.T) {
	skipIfUnavailable(t, "SQLServer")

	conn := NewSQLServerInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":               testSQLServerHost(),
		"port":               testSQLServerPort(),
		"database":           "testdb",
		"username":           "sa",
		"password":           testSQLServerPass(),
		"table_name":         "hl7_inbox",
		"after_processing":   "update_flag",
		"processed_flag_col": "processed",
		"processed_flag_val": "1",
		"polling_interval":   1,
		"ssl_mode":           "disable",
	})); err != nil {
		t.Skipf("SQL Server unavailable: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 20)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Start(startCtx, msgCh)

	msgs := collectMessages(msgCh, msgTimeout, 1)
	t.Logf("Received %d messages; after_processing=update_flag should set processed=1", len(msgs))
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message before flag update")
	} else {
		t.Log("✅ SQL Server update_flag after-processing exercised")
	}
}

// ─── Oracle Integration Tests ─────────────────────────────────────────────────

func TestIntegrationOracleInbound_PollHL7Table(t *testing.T) {
	skipIfUnavailable(t, "Oracle")

	conn := NewOracleInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":               testOracleHost(),
		"port":               testOraclePort(),
		"database":           testOracleService(),
		"username":           "testuser",
		"password":           "testpassword",
		"table_name":         "hl7_inbox",
		"incremental_column": "id",
		"incremental_type":   "integer",
		"max_records":        10,
		"polling_interval":   1,
	})); err != nil {
		t.Skipf("Oracle unavailable or seed not applied: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 20)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Start(startCtx, msgCh); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	msgs := collectMessages(msgCh, msgTimeout, 3)
	if len(msgs) < 1 {
		t.Error("Expected at least 1 HL7 message from Oracle hl7_inbox, got 0")
		return
	}
	t.Logf("✅ Received %d messages from Oracle", len(msgs))
	for _, m := range msgs {
		if m.SourceType != "oracle" {
			t.Errorf("SourceType = %q, want oracle", m.SourceType)
		}
		t.Logf("  Message: id=%s type=%s content_type=%s size=%d",
			m.MessageID, m.MessageType, m.ContentType, m.MessageSize)
	}
}

func TestIntegrationOracleInbound_TestConnection(t *testing.T) {
	skipIfUnavailable(t, "Oracle")

	conn := NewOracleInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":     testOracleHost(),
		"port":     testOraclePort(),
		"database": testOracleService(),
		"username": "testuser",
		"password": "testpassword",
		"query":    "SELECT 1 FROM DUAL",
	})); err != nil {
		t.Skipf("Oracle unavailable: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.TestConnection(ctx); err != nil {
		t.Errorf("TestConnection() failed: %v", err)
	} else {
		t.Log("✅ Oracle TestConnection() succeeded")
	}
}

func TestIntegrationOracleInbound_UpdateFlagAfterProcessing(t *testing.T) {
	skipIfUnavailable(t, "Oracle")

	conn := NewOracleInboundConnector()
	if err := conn.Initialize(mustJSON(map[string]interface{}{
		"host":               testOracleHost(),
		"port":               testOraclePort(),
		"database":           testOracleService(),
		"username":           "testuser",
		"password":           "testpassword",
		"table_name":         "hl7_inbox",
		"after_processing":   "update_flag",
		"processed_flag_col": "processed",
		"processed_flag_val": "1",
		"polling_interval":   1,
	})); err != nil {
		t.Skipf("Oracle unavailable: %v", err)
	}
	defer conn.Close()

	msgCh := make(chan *models.InboundMessage, 20)
	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Start(startCtx, msgCh)

	msgs := collectMessages(msgCh, msgTimeout, 1)
	t.Logf("Received %d messages; after_processing=update_flag should set PROCESSED=1", len(msgs))
	if len(msgs) < 1 {
		t.Error("Expected at least 1 message before flag update")
	} else {
		t.Log("✅ Oracle update_flag after-processing exercised")
	}
}

// ─── Cross-connector: TestConnection for all implemented connectors ────────────

func TestIntegrationAllConnectors_TestConnection(t *testing.T) {
	type connTest struct {
		name        string
		constructor func() InboundConnector
		config      map[string]interface{}
	}

	tests := []connTest{
		{
			"mongodb", NewMongoDBInboundConnector,
			map[string]interface{}{
				"connection_string": testMongoURI(),
				"database": "testdb", "collection": "healthcheck",
			},
		},
		{
			"redis", NewRedisInboundConnector,
			map[string]interface{}{
				"host": testInfraHost(), "port": 6380, "channel": "health",
			},
		},
		{
			"rabbitmq", NewRabbitMQInboundConnector,
			map[string]interface{}{
				"host": testInfraHost(), "port": 5673,
				"username": "testuser", "password": "testpassword", "queue": "health",
			},
		},
		{
			"aws_s3", NewAWSS3InboundConnector,
			map[string]interface{}{
				"bucket": testS3Bucket, "region": "us-east-1",
				"access_key_id": "test", "secret_access_key": "test",
				"endpoint": testS3Endpoint(), "force_path_style": true,
			},
		},
		{
			"sqlserver", NewSQLServerInboundConnector,
			map[string]interface{}{
				"host": testSQLServerHost(), "port": testSQLServerPort(),
				"database": "testdb", "username": "sa", "password": testSQLServerPass(),
				"query": "SELECT 1", "ssl_mode": "disable",
			},
		},
		{
			"oracle", NewOracleInboundConnector,
			map[string]interface{}{
				"host": testOracleHost(), "port": testOraclePort(),
				"database": testOracleService(), "username": "testuser", "password": "testpassword",
				"query": "SELECT 1 FROM DUAL",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			skipIfUnavailable(t, tc.name)
			conn := tc.constructor()
			if err := conn.Initialize(mustJSON(tc.config)); err != nil {
				t.Skipf("%s unavailable: %v", tc.name, err)
			}
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := conn.TestConnection(ctx); err != nil {
				t.Errorf("TestConnection() failed for %s: %v", tc.name, err)
			} else {
				t.Logf("✅ %s TestConnection() OK", tc.name)
			}
		})
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func min3(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// s3PutInput builds an S3 PutObjectInput — defined here to avoid import cycle
// with the aws_s3_inbound.go file (same package).
func s3PutInput(bucket, key, content string) s3.PutObjectInput {
	return s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	}
}

