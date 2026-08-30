// services/connectors/databricks_inbound_test.go
// Coverage for buildDSN/buildQuery/qualifiedTable/Validate -- pure logic that
// doesn't need a live Databricks workspace, so it's tested directly without
// Initialize(). Same caveat as databricks_outbound_test.go: there is no
// live-workspace equivalent to the MySQL/S3/Kafka/Azurite live tests this
// session -- this connector's actual connectivity to a real Databricks SQL
// Warehouse remains unverified. See the file header on databricks_inbound.go.
package connectors

import (
	"strings"
	"testing"
)

func newTestDatabricksInbound(cfg DatabaseInboundConfig) *DatabricksInboundConnector {
	return &DatabricksInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(ConnectorMetadata{TypeName: "databricks_inbound"}),
		config:               cfg,
	}
}

func TestDatabricksInboundBuildDSN_DefaultPort(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		Host:     "dbc-a1b2345c-d6e7.cloud.databricks.com",
		Token:    "dapiSECRET",
		HTTPPath: "/sql/1.0/warehouses/abc123",
	})
	dsn := c.buildDSN()
	want := "token:dapiSECRET@dbc-a1b2345c-d6e7.cloud.databricks.com:443/sql/1.0/warehouses/abc123"
	if dsn != want {
		t.Errorf("buildDSN() = %q, want %q", dsn, want)
	}
}

func TestDatabricksInboundBuildDSN_CustomPortAndCatalogSchema(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		Host:     "example.databricks.com",
		Port:     8443,
		Token:    "dapiXYZ",
		HTTPPath: "/sql/1.0/warehouses/xyz",
		Database: "main",
		Schema:   "clinical",
	})
	dsn := c.buildDSN()
	if !strings.HasPrefix(dsn, "token:dapiXYZ@example.databricks.com:8443/sql/1.0/warehouses/xyz?") {
		t.Errorf("unexpected DSN prefix: %s", dsn)
	}
	if !strings.Contains(dsn, "catalog=main") || !strings.Contains(dsn, "schema=clinical") {
		t.Errorf("expected catalog/schema query params, got: %s", dsn)
	}
}

func TestDatabricksInboundQualifiedTable_OmitsEmptyParts(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{TableName: "patients"})
	if got := c.qualifiedTable(); got != "`patients`" {
		t.Errorf("expected bare table name when catalog/schema are unset, got: %s", got)
	}

	c2 := newTestDatabricksInbound(DatabaseInboundConfig{Database: "main", Schema: "clinical", TableName: "patients"})
	if got := c2.qualifiedTable(); got != "`main`.`clinical`.`patients`" {
		t.Errorf("expected catalog.schema.table when both set, got: %s", got)
	}
}

func TestDatabricksInboundBuildQuery_UsesCustomQueryVerbatim(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{Query: "SELECT * FROM custom_view"})
	if got := c.buildQuery(nil); got != "SELECT * FROM custom_view" {
		t.Errorf("expected custom query to override table_name logic, got: %s", got)
	}
}

func TestDatabricksInboundBuildQuery_PlainSelect(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{Database: "main", TableName: "messages", MaxRecords: 50})
	got := c.buildQuery(nil)
	if !strings.HasPrefix(got, "SELECT * FROM `main`.`messages`") {
		t.Errorf("expected qualified SELECT, got: %s", got)
	}
	if !strings.Contains(got, "LIMIT 50") {
		t.Errorf("expected LIMIT clause from MaxRecords, got: %s", got)
	}
	if strings.Contains(got, "WHERE") {
		t.Errorf("no incremental_column set — expected no WHERE clause, got: %s", got)
	}
}

func TestDatabricksInboundBuildQuery_IncrementalInteger(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		TableName:         "events",
		IncrementalColumn: "id",
		IncrementalType:   "integer",
	})
	got := c.buildQuery(int64(42))
	if !strings.Contains(got, "WHERE `id` > 42") {
		t.Errorf("expected integer incremental WHERE clause, got: %s", got)
	}
	if !strings.Contains(got, "ORDER BY `id`") {
		t.Errorf("expected default ORDER BY incremental_column, got: %s", got)
	}
}

func TestDatabricksInboundBuildQuery_IncrementalTimestamp(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		TableName:         "events",
		IncrementalColumn: "updated_at",
		IncrementalType:   "timestamp",
	})
	got := c.buildQuery("2026-01-01 00:00:00")
	if !strings.Contains(got, "WHERE `updated_at` > '2026-01-01 00:00:00'") {
		t.Errorf("expected quoted timestamp incremental WHERE clause, got: %s", got)
	}
}

func TestDatabricksInboundBuildQuery_CustomOrderByOverridesDefault(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		TableName:         "events",
		IncrementalColumn: "id",
		OrderBy:           "created_at DESC",
	})
	got := c.buildQuery(nil)
	if !strings.Contains(got, "ORDER BY created_at DESC") {
		t.Errorf("expected explicit order_by to be used, got: %s", got)
	}
	if strings.Contains(got, "ORDER BY `id`") {
		t.Errorf("explicit order_by must override the incremental_column default, got: %s", got)
	}
}

func TestDatabricksInboundValidate_RequiresConnectionFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  DatabaseInboundConfig
	}{
		{"missing host", DatabaseInboundConfig{Token: "t", HTTPPath: "/p", TableName: "x"}},
		{"missing token", DatabaseInboundConfig{Host: "h", HTTPPath: "/p", TableName: "x"}},
		{"missing http_path", DatabaseInboundConfig{Host: "h", Token: "t", TableName: "x"}},
		{"missing table_name and query", DatabaseInboundConfig{Host: "h", Token: "t", HTTPPath: "/p"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestDatabricksInbound(tc.cfg)
			c.BaseInboundConnector.BaseConnector.initialized = true
			if err := c.Validate(); err == nil {
				t.Errorf("expected Validate to reject config: %+v", tc.cfg)
			}
		})
	}
}

func TestDatabricksInboundValidate_PassesWithAllRequiredFields(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		Host: "h", Token: "t", HTTPPath: "/p", TableName: "x",
	})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestDatabricksInboundValidate_PassesWithQueryInsteadOfTableName(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{
		Host: "h", Token: "t", HTTPPath: "/p", Query: "SELECT * FROM v",
	})
	c.BaseInboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass with query set instead of table_name, got: %v", err)
	}
}

func TestDatabricksInboundSupportsCron(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{})
	if !c.SupportsCron() {
		t.Error("expected SupportsCron to return true")
	}
}

func TestDatabricksInboundValidate_BeforeInit(t *testing.T) {
	c := newTestDatabricksInbound(DatabaseInboundConfig{Host: "h", Token: "t", HTTPPath: "/p", TableName: "x"})
	if err := c.Validate(); err == nil {
		t.Error("expected Validate to fail before initialization")
	}
}
