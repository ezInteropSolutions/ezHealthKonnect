// services/connectors/snowflake_inbound_test.go
// Coverage for buildDSN/buildQuery/qualifiedTable/Validate -- pure logic that
// doesn't need a live Snowflake account, so it's tested directly without
// Initialize() reaching a real connection (the key-pair-rejection tests do
// call the real Initialize(), since that check runs before any DB open).
// Same caveat as snowflake_outbound_test.go: there is no live-account
// equivalent to the MySQL/S3/Kafka/Azurite live tests this session -- this
// connector's actual connectivity to a real Snowflake account remains
// unverified. See the file header on snowflake_inbound.go.
package connectors

import (
	"strings"
	"testing"
)

func newTestSnowflakeInbound(cfg DatabaseInboundConfig) *SnowflakeInboundConnector {
	return &SnowflakeInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(ConnectorMetadata{TypeName: "snowflake_inbound"}),
		config:               cfg,
	}
}

func TestSnowflakeInboundBuildDSN_Basic(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		Account:  "xy12345.us-east-1",
		Username: "svc_user",
		Password: "secret",
		Database: "ANALYTICS",
		Schema:   "PUBLIC",
	})
	dsn := c.buildDSN()
	want := "svc_user:secret@xy12345.us-east-1/ANALYTICS/PUBLIC"
	if dsn != want {
		t.Errorf("buildDSN() = %q, want %q", dsn, want)
	}
}

func TestSnowflakeInboundBuildDSN_WithRoleAndWarehouse(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		Account:   "xy12345",
		Username:  "svc_user",
		Password:  "secret",
		Database:  "ANALYTICS",
		Schema:    "PUBLIC",
		Warehouse: "COMPUTE_WH",
		Role:      "SYSADMIN",
	})
	dsn := c.buildDSN()
	if !strings.HasPrefix(dsn, "svc_user:secret@xy12345/ANALYTICS/PUBLIC?") {
		t.Errorf("unexpected DSN prefix: %s", dsn)
	}
	if !strings.Contains(dsn, "warehouse=COMPUTE_WH") || !strings.Contains(dsn, "role=SYSADMIN") {
		t.Errorf("expected warehouse/role query params, got: %s", dsn)
	}
}

func TestSnowflakeInboundBuildDSN_EscapesSpecialCharsInPassword(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		Account:  "xy12345",
		Username: "svc_user",
		Password: "p@ss/word&1",
		Database: "DB",
	})
	dsn := c.buildDSN()
	if strings.Contains(dsn, "p@ss/word&1") {
		t.Errorf("expected password special characters to be URL-escaped, got raw password in DSN: %s", dsn)
	}
}

func TestSnowflakeInboundQualifiedTable_OmitsEmptyParts(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{TableName: "patients"})
	if got := c.qualifiedTable(); got != `"patients"` {
		t.Errorf("expected bare table name when database/schema are unset, got: %s", got)
	}

	c2 := newTestSnowflakeInbound(DatabaseInboundConfig{Database: "ANALYTICS", Schema: "PUBLIC", TableName: "patients"})
	if got := c2.qualifiedTable(); got != `"ANALYTICS"."PUBLIC"."patients"` {
		t.Errorf("expected database.schema.table when both set, got: %s", got)
	}
}

func TestSnowflakeInboundBuildQuery_UsesCustomQueryVerbatim(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{Query: "SELECT * FROM custom_view"})
	if got := c.buildQuery(nil); got != "SELECT * FROM custom_view" {
		t.Errorf("expected custom query to override table_name logic, got: %s", got)
	}
}

func TestSnowflakeInboundBuildQuery_PlainSelect(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{Database: "ANALYTICS", TableName: "messages", MaxRecords: 50})
	got := c.buildQuery(nil)
	if !strings.HasPrefix(got, `SELECT * FROM "ANALYTICS"."messages"`) {
		t.Errorf("expected qualified SELECT with double-quoted identifiers, got: %s", got)
	}
	if !strings.Contains(got, "LIMIT 50") {
		t.Errorf("expected LIMIT clause from MaxRecords, got: %s", got)
	}
	if strings.Contains(got, "WHERE") {
		t.Errorf("no incremental_column set — expected no WHERE clause, got: %s", got)
	}
}

func TestSnowflakeInboundBuildQuery_IncrementalInteger(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		TableName:         "events",
		IncrementalColumn: "id",
		IncrementalType:   "integer",
	})
	got := c.buildQuery(int64(42))
	if !strings.Contains(got, `WHERE "id" > 42`) {
		t.Errorf("expected integer incremental WHERE clause, got: %s", got)
	}
	if !strings.Contains(got, `ORDER BY "id"`) {
		t.Errorf("expected default ORDER BY incremental_column, got: %s", got)
	}
}

func TestSnowflakeInboundBuildQuery_IncrementalTimestamp(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		TableName:         "events",
		IncrementalColumn: "updated_at",
		IncrementalType:   "timestamp",
	})
	got := c.buildQuery("2026-01-01 00:00:00")
	if !strings.Contains(got, `WHERE "updated_at" > '2026-01-01 00:00:00'`) {
		t.Errorf("expected quoted timestamp incremental WHERE clause, got: %s", got)
	}
}

func TestSnowflakeInboundBuildQuery_CustomOrderByOverridesDefault(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		TableName:         "events",
		IncrementalColumn: "id",
		OrderBy:           "created_at DESC",
	})
	got := c.buildQuery(nil)
	if !strings.Contains(got, "ORDER BY created_at DESC") {
		t.Errorf("expected explicit order_by to be used, got: %s", got)
	}
	if strings.Contains(got, `ORDER BY "id"`) {
		t.Errorf("explicit order_by must override the incremental_column default, got: %s", got)
	}
}

func TestSnowflakeInboundInitialize_RejectsKeyPairAuth_ViaAuthType(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{})
	cfg := []byte(`{"account":"xy12345","username":"u","password":"p","auth_type":"key_pair"}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject key_pair auth_type")
	}
	if !strings.Contains(err.Error(), "key-pair authentication is not yet implemented") {
		t.Errorf("expected a clear key-pair-not-implemented error, got: %v", err)
	}
}

func TestSnowflakeInboundInitialize_RejectsKeyPairAuth_ViaPrivateKeyField(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{})
	cfg := []byte(`{"account":"xy12345","username":"u","password":"p","private_key":"-----BEGIN PRIVATE KEY-----..."}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject a non-empty private_key even without auth_type set")
	}
	if !strings.Contains(err.Error(), "key-pair authentication is not yet implemented") {
		t.Errorf("expected a clear key-pair-not-implemented error, got: %v", err)
	}
}

func TestSnowflakeInboundValidate_RequiresConnectionFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  DatabaseInboundConfig
	}{
		{"missing account", DatabaseInboundConfig{Username: "u", Password: "p", TableName: "x"}},
		{"missing username", DatabaseInboundConfig{Account: "a", Password: "p", TableName: "x"}},
		{"missing password", DatabaseInboundConfig{Account: "a", Username: "u", TableName: "x"}},
		{"missing table_name and query", DatabaseInboundConfig{Account: "a", Username: "u", Password: "p"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestSnowflakeInbound(tc.cfg)
			c.BaseInboundConnector.BaseConnector.initialized = true
			if err := c.Validate(); err == nil {
				t.Errorf("expected Validate to reject config: %+v", tc.cfg)
			}
		})
	}
}

func TestSnowflakeInboundValidate_PassesWithAllRequiredFields(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		Account: "a", Username: "u", Password: "p", TableName: "x",
	})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestSnowflakeInboundValidate_PassesWithQueryInsteadOfTableName(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{
		Account: "a", Username: "u", Password: "p", Query: "SELECT * FROM v",
	})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass with query set instead of table_name, got: %v", err)
	}
}

func TestSnowflakeInboundSupportsCron(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{})
	if !c.SupportsCron() {
		t.Error("expected SupportsCron to return true")
	}
}

func TestSnowflakeInboundValidate_BeforeInit(t *testing.T) {
	c := newTestSnowflakeInbound(DatabaseInboundConfig{Account: "a", Username: "u", Password: "p", TableName: "x"})
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail before initialization")
	}
}
