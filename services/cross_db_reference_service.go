package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CrossDBReferenceService manages referential integrity between PostgreSQL and MongoDB
type CrossDBReferenceService struct {
	db            *sql.DB
	mongoClient   *mongo.Client
	mongoDatabase string
}

// ReferenceStatus represents the consistency status of a message across databases
type ReferenceStatus struct {
	MessageID        string
	InterfaceID      string
	PostgreSQLExists bool
	MongoDBExists    bool
	IsConsistent     bool
	IsBroken         bool
	PostgreSQLData   map[string]interface{}
	MongoDBData      map[string]interface{}
	CheckedAt        time.Time
}

// OrphanedRecordsReport contains information about orphaned records
type OrphanedRecordsReport struct {
	InterfaceID       string
	PostgreSQLOrphans []string // message_ids only in PostgreSQL
	MongoDBOrphans    []string // message_ids only in MongoDB
	TotalOrphans      int
	ScannedAt         time.Time
}

// IntegrityReport provides overall integrity status
type IntegrityReport struct {
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
	Interfaces map[string]*InterfaceIntegrityStatus
}

// InterfaceIntegrityStatus tracks integrity for one interface
type InterfaceIntegrityStatus struct {
	InterfaceID      string
	PostgreSQLCount  int64
	MongoDBCount     int64
	OrphanedRecords  int
	UnsyncedRecords  int
	IntegrityScore   float64
}

// NewCrossDBReferenceService creates a new cross-database reference service
func NewCrossDBReferenceService(db *sql.DB, mongoClient *mongo.Client, mongoDatabase string) *CrossDBReferenceService {
	return &CrossDBReferenceService{
		db:            db,
		mongoClient:   mongoClient,
		mongoDatabase: mongoDatabase,
	}
}

// VerifyReference checks if a message exists in both databases
func (crs *CrossDBReferenceService) VerifyReference(ctx context.Context, interfaceID string, messageID string) (*ReferenceStatus, error) {
	status := &ReferenceStatus{
		MessageID:   messageID,
		InterfaceID: interfaceID,
		CheckedAt:   time.Now(),
	}

	// Check PostgreSQL
	pgExists, pgData, err := crs.checkPostgreSQL(interfaceID, messageID)
	if err != nil {
		return nil, fmt.Errorf("PostgreSQL check failed: %w", err)
	}
	status.PostgreSQLExists = pgExists
	status.PostgreSQLData = pgData

	// Check MongoDB
	mongoExists, mongoData, err := crs.checkMongoDB(ctx, interfaceID, messageID)
	if err != nil {
		return nil, fmt.Errorf("MongoDB check failed: %w", err)
	}
	status.MongoDBExists = mongoExists
	status.MongoDBData = mongoData

	// Verify consistency
	status.IsConsistent = pgExists && mongoExists
	status.IsBroken = (pgExists && !mongoExists) || (!pgExists && mongoExists)

	return status, nil
}

// checkPostgreSQL verifies if message exists in PostgreSQL
func (crs *CrossDBReferenceService) checkPostgreSQL(interfaceID string, messageID string) (bool, map[string]interface{}, error) {
	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

	query := fmt.Sprintf(`
		SELECT message_id, status, mongo_document_id, mongo_synced, received_at
		FROM %s
		WHERE message_id = $1
	`, tableName)

	var (
		msgID          string
		status         string
		mongoDocID     sql.NullString
		mongoSynced    sql.NullBool
		receivedAt     time.Time
	)

	err := crs.db.QueryRow(query, messageID).Scan(&msgID, &status, &mongoDocID, &mongoSynced, &receivedAt)
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}

	data := map[string]interface{}{
		"message_id":        msgID,
		"status":            status,
		"mongo_document_id": mongoDocID.String,
		"mongo_synced":      mongoSynced.Bool,
		"received_at":       receivedAt,
	}

	return true, data, nil
}

// checkMongoDB verifies if message exists in MongoDB
func (crs *CrossDBReferenceService) checkMongoDB(ctx context.Context, interfaceID string, messageID string) (bool, map[string]interface{}, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := crs.mongoClient.Database(crs.mongoDatabase).Collection(collectionName)

	filter := bson.M{"message_id": messageID}
	var result bson.M

	err := collection.FindOne(ctx, filter).Decode(&result)
	if err == mongo.ErrNoDocuments {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}

	data := map[string]interface{}{
		"message_id":   result["message_id"],
		"interface_id": result["interface_id"],
		"received_at":  result["received_at"],
		"message_size": result["message_size"],
	}

	return true, data, nil
}

// FindOrphanedRecords finds records that exist in one DB but not the other
func (crs *CrossDBReferenceService) FindOrphanedRecords(ctx context.Context, interfaceID string, limit int) (*OrphanedRecordsReport, error) {
	report := &OrphanedRecordsReport{
		InterfaceID:       interfaceID,
		ScannedAt:         time.Now(),
		PostgreSQLOrphans: make([]string, 0),
		MongoDBOrphans:    make([]string, 0),
	}

	// Find PostgreSQL orphans (records without MongoDB counterpart)
	pgOrphans, err := crs.findPostgreSQLOrphans(ctx, interfaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find PostgreSQL orphans: %w", err)
	}
	report.PostgreSQLOrphans = pgOrphans

	// Find MongoDB orphans (records without PostgreSQL counterpart)
	mongoOrphans, err := crs.findMongoDBOrphans(ctx, interfaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find MongoDB orphans: %w", err)
	}
	report.MongoDBOrphans = mongoOrphans

	report.TotalOrphans = len(pgOrphans) + len(mongoOrphans)

	return report, nil
}

// findPostgreSQLOrphans finds PostgreSQL records not in MongoDB
func (crs *CrossDBReferenceService) findPostgreSQLOrphans(ctx context.Context, interfaceID string, limit int) ([]string, error) {
	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

	query := fmt.Sprintf(`
		SELECT message_id
		FROM %s
		WHERE mongo_synced = false OR mongo_synced IS NULL
		ORDER BY received_at DESC
		LIMIT $1
	`, tableName)

	rows, err := crs.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orphans := make([]string, 0)
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			continue
		}

		// Verify it's actually missing from MongoDB
		exists, _, _ := crs.checkMongoDB(ctx, interfaceID, messageID)
		if !exists {
			orphans = append(orphans, messageID)
		}
	}

	return orphans, nil
}

// findMongoDBOrphans finds MongoDB records not in PostgreSQL
func (crs *CrossDBReferenceService) findMongoDBOrphans(ctx context.Context, interfaceID string, limit int) ([]string, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := crs.mongoClient.Database(crs.mongoDatabase).Collection(collectionName)

	// Find documents where pg_synced is false
	filter := bson.M{
		"$or": []bson.M{
			{"sync_status.pg_synced": false},
			{"sync_status.pg_synced": bson.M{"$exists": false}},
		},
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	orphans := make([]string, 0)
	count := 0

	for cursor.Next(ctx) && count < limit {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		messageID, ok := doc["message_id"].(string)
		if !ok {
			continue
		}

		// Verify it's actually missing from PostgreSQL
		exists, _, _ := crs.checkPostgreSQL(interfaceID, messageID)
		if !exists {
			orphans = append(orphans, messageID)
			count++
		}
	}

	return orphans, nil
}

// SyncReferences ensures both databases are aware of each other
func (crs *CrossDBReferenceService) SyncReferences(ctx context.Context, interfaceID string, messageID string) error {
	// Update PostgreSQL with MongoDB sync status
	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
	pgQuery := fmt.Sprintf(`
		UPDATE %s
		SET mongo_synced = true,
			mongo_synced_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE message_id = $1
	`, tableName)

	_, err := crs.db.Exec(pgQuery, messageID)
	if err != nil {
		return fmt.Errorf("failed to sync PostgreSQL: %w", err)
	}

	// Update MongoDB with PostgreSQL sync status
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := crs.mongoClient.Database(crs.mongoDatabase).Collection(collectionName)

	filter := bson.M{"message_id": messageID}
	update := bson.M{
		"$set": bson.M{
			"pg_metadata.row_exists":     true,
			"pg_metadata.last_verified":  time.Now(),
			"sync_status.pg_synced":      true,
			"sync_status.last_pg_sync":   time.Now(),
		},
	}

	_, err = collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to sync MongoDB: %w", err)
	}

	fmt.Printf("✅ Cross-database references synced for message: %s\n", messageID)
	return nil
}

// RepairBrokenReference repairs inconsistent cross-database references
func (crs *CrossDBReferenceService) RepairBrokenReference(ctx context.Context, interfaceID string, messageID string) error {
	status, err := crs.VerifyReference(ctx, interfaceID, messageID)
	if err != nil {
		return err
	}

	if status.IsConsistent {
		return nil // No repair needed
	}

	// PostgreSQL exists but MongoDB missing
	if status.PostgreSQLExists && !status.MongoDBExists {
		fmt.Printf("🔧 Repairing: PostgreSQL orphan %s (creating MongoDB record)\n", messageID)
		return crs.createMissingMongoDBRecord(ctx, interfaceID, messageID, status.PostgreSQLData)
	}

	// MongoDB exists but PostgreSQL missing
	if status.MongoDBExists && !status.PostgreSQLExists {
		fmt.Printf("🔧 Repairing: MongoDB orphan %s (creating PostgreSQL record)\n", messageID)
		return crs.createMissingPostgreSQLRecord(interfaceID, messageID, status.MongoDBData)
	}

	return fmt.Errorf("both references missing - cannot repair")
}

// createMissingMongoDBRecord creates a MongoDB record from PostgreSQL data
func (crs *CrossDBReferenceService) createMissingMongoDBRecord(ctx context.Context, interfaceID string, messageID string, pgData map[string]interface{}) error {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := crs.mongoClient.Database(crs.mongoDatabase).Collection(collectionName)

	doc := bson.M{
		"message_id":   messageID,
		"interface_id": interfaceID,
		"received_at":  pgData["received_at"],
		"raw_content":  "[Content lost - repaired from metadata]",
		"pg_metadata": bson.M{
			"table_name":    fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_")),
			"row_exists":    true,
			"last_verified": time.Now(),
		},
		"sync_status": bson.M{
			"pg_synced":     true,
			"last_pg_sync":  time.Now(),
			"repair_source": "postgresql",
			"repaired_at":   time.Now(),
		},
		"created_at": time.Now(),
	}

	_, err := collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB record: %w", err)
	}

	// Update PostgreSQL sync status
	return crs.SyncReferences(ctx, interfaceID, messageID)
}

// createMissingPostgreSQLRecord creates a PostgreSQL record from MongoDB data
func (crs *CrossDBReferenceService) createMissingPostgreSQLRecord(interfaceID string, messageID string, mongoData map[string]interface{}) error {
	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, message_id, interface_id, status, received_at,
			mongo_document_id, mongo_synced, mongo_synced_at,
			created_at, updated_at
		) VALUES (
			gen_random_uuid(), $1, $2, 'received', $3,
			$1, true, CURRENT_TIMESTAMP,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`, tableName)

	receivedAt, ok := mongoData["received_at"].(time.Time)
	if !ok {
		receivedAt = time.Now()
	}

	_, err := crs.db.Exec(query, messageID, interfaceID, receivedAt)
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL record: %w", err)
	}

	return nil
}

// GetIntegrityStatistics returns overall integrity statistics
func (crs *CrossDBReferenceService) GetIntegrityStatistics(ctx context.Context, interfaceID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// PostgreSQL count
	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
	var pgCount int64
	pgQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	_ = crs.db.QueryRow(pgQuery).Scan(&pgCount)
	stats["postgresql_count"] = pgCount

	// MongoDB count
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := crs.mongoClient.Database(crs.mongoDatabase).Collection(collectionName)
	mongoCount, _ := collection.CountDocuments(ctx, bson.M{})
	stats["mongodb_count"] = mongoCount

	// Unsynced count
	var unsyncedCount int64
	unsyncedQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE mongo_synced = false OR mongo_synced IS NULL", tableName)
	_ = crs.db.QueryRow(unsyncedQuery).Scan(&unsyncedCount)
	stats["unsynced_count"] = unsyncedCount

	// Calculate integrity score
	if pgCount > 0 {
		stats["integrity_score"] = float64(pgCount-unsyncedCount) / float64(pgCount) * 100
	} else {
		stats["integrity_score"] = 100.0
	}

	stats["interface_id"] = interfaceID
	stats["checked_at"] = time.Now()

	return stats, nil
}
