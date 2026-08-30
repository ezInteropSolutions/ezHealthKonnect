// services/connectors/snowflake_outbound_test.go
// Coverage for buildDSN/buildWriteQuery/quoteIdentSnowflake/Validate -- pure
// logic that doesn't need a live Snowflake account, so it's tested directly
// without Initialize(). There is no live-account equivalent to the
// MySQL/S3/Kafka live tests this session -- see the file header comment on
// snowflake_outbound.go for why: this connector's actual connectivity to a
// real Snowflake account remains unverified.
package connectors

import (
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func newTestSnowflakeOutbound(cfg DatabaseOutboundConfig) *SnowflakeOutboundConnector {
	return &SnowflakeOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "snowflake_outbound"}, true),
		config:                cfg,
	}
}

func TestSnowflakeBuildDSN_Basic(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
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

func TestSnowflakeBuildDSN_WithRoleAndWarehouse(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
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

func TestSnowflakeBuildDSN_EscapesSpecialCharsInPassword(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
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

func TestSnowflakeBuildDSN_OmitsSchemaWhenDatabaseEmpty(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
		Account:  "xy12345",
		Username: "svc_user",
		Password: "secret",
		Schema:   "PUBLIC", // schema without database must not be emitted
	})
	dsn := c.buildDSN()
	if strings.Contains(dsn, "PUBLIC") {
		t.Errorf("expected schema to be omitted when database is empty, got: %s", dsn)
	}
}

func TestSnowflakeQuoteIdent_EscapesEmbeddedDoubleQuote(t *testing.T) {
	got := quoteIdentSnowflake(`weird"name`)
	want := `"weird""name"`
	if got != want {
		t.Errorf("quoteIdentSnowflake(%q) = %q, want %q", `weird"name`, got, want)
	}
}

func TestSnowflakeQuoteIdent_UsesDoubleQuotesNotBackticks(t *testing.T) {
	got := quoteIdentSnowflake("patients")
	if strings.Contains(got, "`") {
		t.Errorf("Snowflake identifiers must use double quotes, not backticks, got: %s", got)
	}
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("expected double-quote wrapped identifier, got: %s", got)
	}
}

func TestSnowflakeBuildWriteQuery_PlainInsert(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
		Database:  "ANALYTICS",
		Schema:    "PUBLIC",
		TableName: "patients",
		WriteMode: "insert",
	})
	query, values := c.buildWriteQuery(map[string]interface{}{"mrn": "12345", "name": "Test"})

	if !strings.HasPrefix(query, `INSERT INTO "ANALYTICS"."PUBLIC"."patients"`) {
		t.Errorf("expected qualified INSERT with double-quoted 3-level namespace, got: %s", query)
	}
	if strings.Contains(query, "MERGE") {
		t.Errorf("plain insert mode must not produce a MERGE statement, got: %s", query)
	}
	if !strings.Contains(query, "?") {
		t.Errorf("expected '?' positional placeholders (confirmed supported by the official driver), got: %s", query)
	}
	if len(values) != 2 {
		t.Errorf("expected 2 bound values, got %d", len(values))
	}
}

func TestSnowflakeBuildWriteQuery_Upsert_UsesMergeInto(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		WriteMode: "upsert",
		UniqueKey: "mrn",
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"mrn": "12345", "name": "Test"})

	if !strings.Contains(query, "MERGE INTO") {
		t.Errorf("upsert mode with a unique_key must produce a MERGE INTO statement, got: %s", query)
	}
	if !strings.Contains(query, "WHEN MATCHED THEN UPDATE") || !strings.Contains(query, "WHEN NOT MATCHED THEN INSERT") {
		t.Errorf("expected both MATCHED/NOT MATCHED clauses, got: %s", query)
	}
	if strings.Contains(query, `target."mrn" = source."mrn"`) && strings.Contains(query, `UPDATE SET target."mrn"`) {
		t.Errorf("unique_key column must be excluded from the UPDATE SET clause, got: %s", query)
	}
	if !strings.Contains(query, `target."name" = source."name"`) {
		t.Errorf("non-key columns must appear in the UPDATE SET clause, got: %s", query)
	}
}

func TestSnowflakeBuildWriteQuery_MultiColumnUniqueKey(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
		TableName: "claims",
		WriteMode: "upsert",
		UniqueKey: "claim_id, line_num",
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"claim_id": "C1", "line_num": 1, "amount": 100})

	if !strings.Contains(query, `target."claim_id" = source."claim_id"`) || !strings.Contains(query, `target."line_num" = source."line_num"`) {
		t.Errorf("expected both comma-separated unique_key columns in the ON clause, got: %s", query)
	}
	if !strings.Contains(query, " AND ") {
		t.Errorf("expected multiple ON conditions joined with AND, got: %s", query)
	}
}

func TestSnowflakeBuildWriteQuery_UpsertWithoutUniqueKey_FallsBackToInsert(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		WriteMode: "upsert",
		UniqueKey: "",
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"mrn": "12345"})
	if strings.Contains(query, "MERGE") {
		t.Errorf("upsert mode without a unique_key must fall back to plain insert, got: %s", query)
	}
}

func TestSnowflakeQualifiedTable_OmitsEmptyParts(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{TableName: "patients"})
	if got := c.qualifiedTable(); got != `"patients"` {
		t.Errorf("expected bare table name when database/schema are unset, got: %s", got)
	}

	c2 := newTestSnowflakeOutbound(DatabaseOutboundConfig{Database: "ANALYTICS", TableName: "patients"})
	if got := c2.qualifiedTable(); got != `"ANALYTICS"."patients"` {
		t.Errorf("expected database.table when only database is set, got: %s", got)
	}
}

func TestSnowflakeInitialize_RejectsKeyPairAuth_ViaAuthType(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{})
	cfg := []byte(`{"account":"xy12345","username":"u","password":"p","table_name":"x","auth_type":"key_pair"}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject key_pair auth_type")
	}
	if !strings.Contains(err.Error(), "key-pair authentication is not yet implemented") {
		t.Errorf("expected a clear key-pair-not-implemented error, got: %v", err)
	}
}

func TestSnowflakeInitialize_RejectsKeyPairAuth_ViaPrivateKeyField(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{})
	cfg := []byte(`{"account":"xy12345","username":"u","password":"p","table_name":"x","private_key":"-----BEGIN PRIVATE KEY-----..."}`)
	err := c.Initialize(cfg)
	if err == nil {
		t.Fatal("expected Initialize to reject a non-empty private_key even without auth_type set")
	}
	if !strings.Contains(err.Error(), "key-pair authentication is not yet implemented") {
		t.Errorf("expected a clear key-pair-not-implemented error, got: %v", err)
	}
}

func TestSnowflakeValidate_RequiresConnectionFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  DatabaseOutboundConfig
	}{
		{"missing account", DatabaseOutboundConfig{Username: "u", Password: "p", TableName: "x"}},
		{"missing username", DatabaseOutboundConfig{Account: "a", Password: "p", TableName: "x"}},
		{"missing password", DatabaseOutboundConfig{Account: "a", Username: "u", TableName: "x"}},
		{"missing table_name", DatabaseOutboundConfig{Account: "a", Username: "u", Password: "p"}},
		{"upsert without unique_key", DatabaseOutboundConfig{Account: "a", Username: "u", Password: "p", TableName: "x", WriteMode: "upsert"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestSnowflakeOutbound(tc.cfg)
			c.BaseOutboundConnector.BaseConnector.initialized = true
			if err := c.Validate(); err == nil {
				t.Errorf("expected Validate to reject config: %+v", tc.cfg)
			}
		})
	}
}

func TestSnowflakeValidate_PassesWithAllRequiredFields(t *testing.T) {
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{
		Account: "a", Username: "u", Password: "p", TableName: "x", WriteMode: "insert",
	})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestSnowflakeSendBatch_AggregatesPerMessageResults(t *testing.T) {
	// SendBatch requires an initialized db to actually Send(); this just
	// confirms the uninitialized-connector guard fires before any query runs.
	c := newTestSnowflakeOutbound(DatabaseOutboundConfig{})
	_, err := c.SendBatch(nil, []*models.OutboundMessage{{MessageID: "m1"}})
	if err == nil {
		t.Error("expected an error when SendBatch is called on an uninitialized connector")
	}
}
