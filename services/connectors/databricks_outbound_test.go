// services/connectors/databricks_outbound_test.go
// Coverage for buildDSN/buildWriteQuery/Validate -- pure logic that doesn't
// need a live Databricks workspace, so it's tested directly without
// Initialize(). There is no live-workspace equivalent to the MySQL/S3/Kafka
// live tests this session -- see the file header comment on
// databricks_outbound.go for why: this connector's actual connectivity to a
// real Databricks SQL Warehouse remains unverified.
package connectors

import (
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func newTestDatabricksOutbound(cfg DatabaseOutboundConfig) *DatabricksOutboundConnector {
	return &DatabricksOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(ConnectorMetadata{TypeName: "databricks_outbound"}, true),
		config:                cfg,
	}
}

func TestDatabricksBuildDSN_DefaultPort(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
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

func TestDatabricksBuildDSN_CustomPortAndCatalogSchema(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
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

func TestDatabricksBuildWriteQuery_PlainInsert(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
		Database:  "main",
		Schema:    "clinical",
		TableName: "patients",
		WriteMode: "insert",
	})
	query, values := c.buildWriteQuery(map[string]interface{}{"mrn": "12345", "name": "Test"})

	if !strings.HasPrefix(query, "INSERT INTO `main`.`clinical`.`patients`") {
		t.Errorf("expected qualified INSERT with backtick-quoted 3-level namespace, got: %s", query)
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

func TestDatabricksBuildWriteQuery_Upsert_UsesMergeInto(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
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
	// The key column must drive the ON clause, not appear in the UPDATE SET clause.
	if strings.Contains(query, "target.`mrn` = source.`mrn`") && strings.Contains(query, "UPDATE SET target.`mrn`") {
		t.Errorf("unique_key column must be excluded from the UPDATE SET clause, got: %s", query)
	}
	if !strings.Contains(query, "target.`name` = source.`name`") {
		t.Errorf("non-key columns must appear in the UPDATE SET clause, got: %s", query)
	}
}

func TestDatabricksBuildWriteQuery_MultiColumnUniqueKey(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
		TableName: "claims",
		WriteMode: "upsert",
		UniqueKey: "claim_id, line_num",
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"claim_id": "C1", "line_num": 1, "amount": 100})

	if !strings.Contains(query, "target.`claim_id` = source.`claim_id`") || !strings.Contains(query, "target.`line_num` = source.`line_num`") {
		t.Errorf("expected both comma-separated unique_key columns in the ON clause, got: %s", query)
	}
	if !strings.Contains(query, " AND ") {
		t.Errorf("expected multiple ON conditions joined with AND, got: %s", query)
	}
}

func TestDatabricksBuildWriteQuery_UpsertWithoutUniqueKey_FallsBackToInsert(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
		TableName: "patients",
		WriteMode: "upsert",
		UniqueKey: "",
	})
	query, _ := c.buildWriteQuery(map[string]interface{}{"mrn": "12345"})
	if strings.Contains(query, "MERGE") {
		t.Errorf("upsert mode without a unique_key must fall back to plain insert, got: %s", query)
	}
}

func TestDatabricksQualifiedTable_OmitsEmptyParts(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{TableName: "patients"})
	if got := c.qualifiedTable(); got != "`patients`" {
		t.Errorf("expected bare table name when catalog/schema are unset, got: %s", got)
	}

	c2 := newTestDatabricksOutbound(DatabaseOutboundConfig{Database: "main", TableName: "patients"})
	if got := c2.qualifiedTable(); got != "`main`.`patients`" {
		t.Errorf("expected catalog.table when only catalog is set, got: %s", got)
	}
}

func TestDatabricksValidate_RequiresConnectionFields(t *testing.T) {
	cases := []struct {
		name string
		cfg  DatabaseOutboundConfig
	}{
		{"missing host", DatabaseOutboundConfig{Token: "t", HTTPPath: "/p", TableName: "x"}},
		{"missing token", DatabaseOutboundConfig{Host: "h", HTTPPath: "/p", TableName: "x"}},
		{"missing http_path", DatabaseOutboundConfig{Host: "h", Token: "t", TableName: "x"}},
		{"missing table_name", DatabaseOutboundConfig{Host: "h", Token: "t", HTTPPath: "/p"}},
		{"upsert without unique_key", DatabaseOutboundConfig{Host: "h", Token: "t", HTTPPath: "/p", TableName: "x", WriteMode: "upsert"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestDatabricksOutbound(tc.cfg)
			c.BaseOutboundConnector.BaseConnector.initialized = true
			if err := c.Validate(); err == nil {
				t.Errorf("expected Validate to reject config: %+v", tc.cfg)
			}
		})
	}
}

func TestDatabricksValidate_PassesWithAllRequiredFields(t *testing.T) {
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{
		Host: "h", Token: "t", HTTPPath: "/p", TableName: "x", WriteMode: "insert",
	})
	c.BaseOutboundConnector.BaseConnector.initialized = true
	if err := c.Validate(); err != nil {
		t.Errorf("expected Validate to pass, got: %v", err)
	}
}

func TestDatabricksSendBatch_AggregatesPerMessageResults(t *testing.T) {
	// SendBatch requires an initialized db to actually Send(); this just
	// confirms the uninitialized-connector guard fires before any query runs.
	c := newTestDatabricksOutbound(DatabaseOutboundConfig{})
	_, err := c.SendBatch(nil, []*models.OutboundMessage{{MessageID: "m1"}})
	if err == nil {
		t.Error("expected an error when SendBatch is called on an uninitialized connector")
	}
}
