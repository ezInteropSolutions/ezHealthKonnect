// services/connectors/mysql_outbound_test.go
// Coverage for buildWriteQuery -- the query-construction logic that doesn't
// need a live MySQL server, so it's tested directly without Initialize().
package connectors

import (
	"strings"
	"testing"
)

func newTestMySQLOutbound(cfg DatabaseOutboundConfig) *MySQLOutboundConnector {
	return &MySQLOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "mysql_outbound"}, true),
		config:                cfg,
	}
}

func TestBuildWriteQuery_PlainInsert_NoUpsertClause(t *testing.T) {
	c := newTestMySQLOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		WriteMode: "insert",
	})
	query, values := c.buildWriteQuery(map[string]interface{}{"mrn": "12345", "name": "Test"})

	if !strings.HasPrefix(query, "INSERT INTO `patients`") {
		t.Errorf("expected INSERT INTO with backtick-quoted table name, got: %s", query)
	}
	if strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("plain insert mode must not add an upsert clause, got: %s", query)
	}
	if !strings.Contains(query, "?") {
		t.Errorf("MySQL placeholders must be '?', got: %s", query)
	}
	if len(values) != 2 {
		t.Errorf("expected 2 bound values, got %d", len(values))
	}
}

func TestBuildWriteQuery_Upsert_UsesOnDuplicateKeyUpdate(t *testing.T) {
	c := newTestMySQLOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		WriteMode: "upsert",
		UniqueKey: "mrn",
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"mrn": "12345", "name": "Test"})

	if !strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("upsert mode with a unique_key must add ON DUPLICATE KEY UPDATE, got: %s", query)
	}
	if strings.Contains(query, "REPLACE INTO") {
		t.Error("must never use REPLACE INTO -- it deletes+reinserts, resetting auto-increment ids and firing ON DELETE CASCADE")
	}
	// The unique key column itself must not appear in the update clause (it's the match key, not something to overwrite).
	if strings.Contains(query, "`mrn` = VALUES(`mrn`)") {
		t.Errorf("unique_key column must be excluded from the UPDATE clause, got: %s", query)
	}
	if !strings.Contains(query, "`name` = VALUES(`name`)") {
		t.Errorf("non-key columns must appear in the UPDATE clause, got: %s", query)
	}
}

func TestBuildWriteQuery_UpsertWithoutUniqueKey_FallsBackToPlainInsert(t *testing.T) {
	c := newTestMySQLOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		WriteMode: "upsert",
		UniqueKey: "", // no unique key configured
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"mrn": "12345"})

	if strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("upsert mode without a unique_key must fall back to plain insert, got: %s", query)
	}
}

func TestMySQLOutboundValidate_RequiresUniqueKeyForUpsert(t *testing.T) {
	c := newTestMySQLOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		Database:  "test_db",
		WriteMode: "upsert",
	})
	c.BaseOutboundConnector.BaseConnector.initialized = true

	if err := c.Validate(); err == nil {
		t.Error("Validate must reject upsert mode with no unique_key configured")
	}
}

func TestMySQLOutboundValidate_PlainInsertNeedsNoUniqueKey(t *testing.T) {
	c := newTestMySQLOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		Database:  "test_db",
		WriteMode: "insert",
	})
	c.BaseOutboundConnector.BaseConnector.initialized = true

	if err := c.Validate(); err != nil {
		t.Errorf("plain insert mode should validate without a unique_key, got: %v", err)
	}
}

func TestMySQLOutboundBuildDSN_DefaultsHostAndPort(t *testing.T) {
	c := newTestMySQLOutbound(DatabaseOutboundConfig{
		Username: "root",
		Password: "secret",
		Database: "mydb",
	})
	dsn := c.buildDSN()
	if !strings.Contains(dsn, "@tcp(localhost:3306)/mydb") {
		t.Errorf("expected default host localhost and port 3306, got: %s", dsn)
	}
	if !strings.Contains(dsn, "tls=false") {
		t.Errorf("expected tls=false when ssl_mode is unset, got: %s", dsn)
	}
}
