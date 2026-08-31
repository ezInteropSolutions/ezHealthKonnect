// services/mirth_migration_service_test.go
// Unit tests for the Mirth Connect migration service.
//
// Unit tests: pure Go, no database required — test parsing, mapping, and analysis.
// Integration tests: require DATABASE_URL env var with V86 migration applied.
//
// Run unit tests:
//
//	go test ./services/ -v -run TestMirthMigration
//
// Run integration tests:
//
//	DATABASE_URL="postgres://user:pass@localhost:5432/ezhkonnect?sslmode=disable" \
//	  go test ./services/ -v -run TestMirthMigrationIntegration

package services

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"ezhealthkonnect/models"

	_ "github.com/lib/pq"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test fixtures
// ─────────────────────────────────────────────────────────────────────────────

const validMirthXML = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>550e8400-e29b-41d4-a716-446655440000</id>
  <name>ADT Test Channel</name>
  <description>Unit test channel</description>
  <enabled>true</enabled>
  <version>3.12.0</version>
  <sourceConnector>
    <name>sourceConnector</name>
    <transportName>TCP Listener</transportName>
    <properties class="com.mirth.connect.connectors.tcp.TcpReceiverProperties">
      <listenerConnectorProperties>
        <host>0.0.0.0</host>
        <port>6661</port>
      </listenerConnectorProperties>
      <mllpMode>true</mllpMode>
      <maxConnections>10</maxConnections>
    </properties>
    <transformer>
      <steps>
        <step>
          <sequenceNumber>0</sequenceNumber>
          <name>Extract Patient ID</name>
          <type>JavaScript</type>
          <data class="map">
            <entry>
              <string>Script</string>
              <string>var pid = msg['PID']['PID.3']['PID.3.1'].toString();</string>
            </entry>
          </data>
        </step>
        <step>
          <sequenceNumber>1</sequenceNumber>
          <name>Map to FHIR</name>
          <type>Mapper</type>
          <data class="map">
            <entry>
              <string>Mappings</string>
              <list>
                <com.mirth.connect.plugins.mapper.MapperStep>
                  <variable>patientId</variable>
                  <mapping>msg['PID']['PID.3']['PID.3.1'].toString()</mapping>
                  <defaultValue></defaultValue>
                  <scope>LOCAL</scope>
                </com.mirth.connect.plugins.mapper.MapperStep>
              </list>
            </entry>
          </data>
        </step>
      </steps>
    </transformer>
    <filter>
      <rules>
        <rule>
          <sequenceNumber>0</sequenceNumber>
          <name>Accept ADT messages</name>
          <type>JavaScript</type>
          <data class="map">
            <entry>
              <string>Script</string>
              <string>msg['MSH']['MSH.9']['MSH.9.1'].toString() == 'ADT'</string>
            </entry>
          </data>
        </rule>
      </rules>
    </filter>
  </sourceConnector>
  <destinationConnectors>
    <connector>
      <name>Send to FHIR API</name>
      <transportName>HTTP Sender</transportName>
      <properties class="com.mirth.connect.connectors.http.HttpDispatcherProperties">
        <host>http://fhir.hospital.org/api/adt</host>
        <method>POST</method>
      </properties>
      <transformer><steps/></transformer>
      <filter><rules/></filter>
    </connector>
  </destinationConnectors>
</channel>`

const xmlNoName = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>abc123</id>
  <enabled>true</enabled>
  <sourceConnector>
    <transportName>TCP Listener</transportName>
    <properties class="com.mirth.connect.connectors.tcp.TcpReceiverProperties">
      <listenerConnectorProperties><host>0.0.0.0</host><port>6661</port></listenerConnectorProperties>
    </properties>
    <transformer><steps/></transformer>
    <filter><rules/></filter>
  </sourceConnector>
  <destinationConnectors/>
</channel>`

const xmlHTTPSender = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>http-channel-001</id>
  <name>HTTP Outbound Channel</name>
  <enabled>true</enabled>
  <version>3.10.0</version>
  <sourceConnector>
    <transportName>HTTP Listener</transportName>
    <properties class="com.mirth.connect.connectors.http.HttpReceiverProperties">
      <listenerConnectorProperties>
        <host>0.0.0.0</host>
        <port>8080</port>
        <contextPath>/hl7</contextPath>
      </listenerConnectorProperties>
    </properties>
    <transformer><steps/></transformer>
    <filter><rules/></filter>
  </sourceConnector>
  <destinationConnectors>
    <connector>
      <name>HTTP Dest</name>
      <transportName>HTTP Sender</transportName>
      <properties class="com.mirth.connect.connectors.http.HttpDispatcherProperties">
        <host>https://api.example.com/fhir</host>
        <method>POST</method>
      </properties>
      <transformer><steps/></transformer>
      <filter><rules/></filter>
    </connector>
  </destinationConnectors>
</channel>`

const xmlInternalTransport = `<?xml version="1.0" encoding="UTF-8"?>
<channel>
  <id>internal-001</id>
  <name>Internal Channel</name>
  <enabled>true</enabled>
  <version>3.8.0</version>
  <sourceConnector>
    <transportName>Channel Reader</transportName>
    <properties class="com.mirth.connect.connectors.vm.VmReceiverProperties"/>
    <transformer><steps/></transformer>
    <filter><rules/></filter>
  </sourceConnector>
  <destinationConnectors/>
</channel>`

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newTestSvc() *MirthMigrationService {
	return &MirthMigrationService{db: nil}
}

func testDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping integration test: DATABASE_URL not set")
		return nil, func() {}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping integration test: cannot reach DB: %v", err)
	}
	return db, func() { db.Close() }
}

// testUserID returns the real UUID of the admin user V2__default_config.sql
// always seeds, since interfaces.user_id is a NOT NULL FK to users.id — a
// literal placeholder string like "test-user" can't satisfy that constraint.
func testUserID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM users WHERE email = 'admin@ezhealthkonnect.com'`).Scan(&id); err != nil {
		t.Skipf("skipping integration test: seeded admin user not found: %v", err)
	}
	return id
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] XML parsing
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigration_ParseValidXML(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview() returned unexpected error: %v", err)
	}
	if preview.ChannelName != "ADT Test Channel" {
		t.Errorf("ChannelName = %q; want %q", preview.ChannelName, "ADT Test Channel")
	}
	if preview.MirthVersion != "3.12.0" {
		t.Errorf("MirthVersion = %q; want %q", preview.MirthVersion, "3.12.0")
	}
	if preview.ChannelID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("ChannelID = %q; want correct UUID", preview.ChannelID)
	}
	if !preview.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestMirthMigration_ParseInvalidXML(t *testing.T) {
	svc := newTestSvc()
	_, err := svc.Preview([]byte("<not valid xml"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestMirthMigration_ParseEmptyBody(t *testing.T) {
	svc := newTestSvc()
	_, err := svc.Preview([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
}

func TestMirthMigration_ParseChannelWithNoName(t *testing.T) {
	svc := newTestSvc()
	_, err := svc.Preview([]byte(xmlNoName))
	if err == nil {
		t.Fatal("expected error for channel with no name, got nil")
	}
	if !strings.Contains(err.Error(), "no name") {
		t.Errorf("expected 'no name' in error, got: %v", err)
	}
}

func TestMirthMigration_SizeLimitEnforced(t *testing.T) {
	svc := newTestSvc()
	oversized := make([]byte, MaxMirthXMLBytes+1)
	_, err := svc.Preview(oversized)
	if err == nil {
		t.Fatal("expected error for oversized input, got nil")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("expected 'maximum' in error, got: %v", err)
	}
}

func TestMirthMigration_SizeLimitAtBoundary(t *testing.T) {
	svc := newTestSvc()
	// Exactly at the limit should fail on invalid XML, not size limit
	atLimit := make([]byte, MaxMirthXMLBytes)
	_, err := svc.Preview(atLimit)
	if err != nil && strings.Contains(err.Error(), "maximum") {
		t.Errorf("should NOT hit size limit at exactly %d bytes", MaxMirthXMLBytes)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Source connector mapping
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigration_SourceConnector_TCPListener(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	src := preview.Source
	if src.EzHKConnectorType != "tcp_mllp_inbound" {
		t.Errorf("Source connector = %q; want tcp_mllp_inbound", src.EzHKConnectorType)
	}
	if src.Status != "auto" {
		t.Errorf("Source status = %q; want auto", src.Status)
	}
	if src.Config["port"] != 6661 {
		t.Errorf("port = %v; want 6661", src.Config["port"])
	}
	if src.Config["host"] != "0.0.0.0" {
		t.Errorf("host = %v; want 0.0.0.0", src.Config["host"])
	}
}

func TestMirthMigration_SourceConnector_HTTPListener(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(xmlHTTPSender))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	src := preview.Source
	if src.EzHKConnectorType != "http_rest_inbound" {
		t.Errorf("Source = %q; want http_rest_inbound", src.EzHKConnectorType)
	}
	if src.Config["port"] != 8080 {
		t.Errorf("port = %v; want 8080", src.Config["port"])
	}
	if src.Config["path"] != "/hl7" {
		t.Errorf("path = %v; want /hl7", src.Config["path"])
	}
}

func TestMirthMigration_SourceConnector_InternalTransport_IsUnsupported(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(xmlInternalTransport))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.Source.Status != "unsupported" {
		t.Errorf("Internal transport should be unsupported; got %q", preview.Source.Status)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Destination connector mapping
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigration_DestConnector_HTTPSender(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if len(preview.Destinations) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(preview.Destinations))
	}
	dest := preview.Destinations[0]
	if dest.EzHKConnectorType != "http_outbound" {
		t.Errorf("Dest connector = %q; want http_outbound", dest.EzHKConnectorType)
	}
	if dest.Config["endpoint"] != "http://fhir.hospital.org/api/adt" {
		t.Errorf("endpoint = %v; want http://fhir.hospital.org/api/adt", dest.Config["endpoint"])
	}
	if dest.Config["method"] != "POST" {
		t.Errorf("method = %v; want POST", dest.Config["method"])
	}
}

func TestMirthMigration_NoDestinations(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(xmlInternalTransport))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if len(preview.Destinations) != 0 {
		t.Errorf("expected 0 destinations, got %d", len(preview.Destinations))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Step analysis
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigration_Steps_CountAndOrder(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	// 1 filter rule + 2 transformer steps = 3 total
	if len(preview.Steps) != 3 {
		t.Errorf("expected 3 steps (1 filter + 2 transformer), got %d", len(preview.Steps))
	}
	// Filter rule first
	if !preview.Steps[0].IsFilterRule {
		t.Error("first step should be a filter rule")
	}
	// Transformer steps follow
	if preview.Steps[1].IsFilterRule || preview.Steps[2].IsFilterRule {
		t.Error("steps 1 and 2 should not be filter rules")
	}
}

func TestMirthMigration_Steps_JavaScriptTypeMapping(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	jsStep := preview.Steps[1] // first transformer step
	if jsStep.MirthType != "JavaScript" {
		t.Errorf("MirthType = %q; want JavaScript", jsStep.MirthType)
	}
	if jsStep.SuggestedType != "enrichment.script" {
		t.Errorf("SuggestedType = %q; want enrichment.script", jsStep.SuggestedType)
	}
	if !strings.Contains(jsStep.Script, "PID.3") {
		t.Errorf("expected script to contain 'PID.3', got: %s", jsStep.Script)
	}
}

func TestMirthMigration_Steps_MapperTypeMapping(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	mapperStep := preview.Steps[2] // second transformer step (Mapper)
	if mapperStep.MirthType != "Mapper" {
		t.Errorf("MirthType = %q; want Mapper", mapperStep.MirthType)
	}
	if mapperStep.SuggestedType != "field_mapping" {
		t.Errorf("SuggestedType = %q; want field_mapping", mapperStep.SuggestedType)
	}
}

func TestMirthMigration_FilterRule_TrivialMSHCheck(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	filterStep := preview.Steps[0]
	if filterStep.SuggestedType != "control.if_then_else" {
		t.Errorf("trivial MSH filter should suggest control.if_then_else, got %q", filterStep.SuggestedType)
	}
}

func TestMirthMigration_ScriptContainsMirthHeader(t *testing.T) {
	script := addMirthHeader("var x = 1;", "Test Step", "JavaScript")
	if !strings.Contains(script, "Migrated from Mirth Connect") {
		t.Error("script header should contain 'Migrated from Mirth Connect'")
	}
	if !strings.Contains(script, "var x = 1;") {
		t.Error("original script should be preserved in output")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Summary
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigration_Summary_TotalsCorrect(t *testing.T) {
	svc := newTestSvc()
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	s := preview.Summary
	if s.TotalSteps != 3 {
		t.Errorf("TotalSteps = %d; want 3", s.TotalSteps)
	}
	// Source (auto) + dest (auto) = 2 auto-mapped connectors
	// Steps are all review (no auto-converted steps in current logic)
	if s.AutoMapped < 2 {
		t.Errorf("AutoMapped = %d; expected at least 2 (connectors)", s.AutoMapped)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Message type detection
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigration_DetectMessageType_ADT(t *testing.T) {
	ch, _ := parseMirthXML([]byte(validMirthXML))
	mt := detectMessageType(ch)
	if mt != "ADT" {
		t.Errorf("detectMessageType = %q; want ADT", mt)
	}
}

func TestMirthMigration_DetectMessageType_NoFilter(t *testing.T) {
	ch, _ := parseMirthXML([]byte(xmlHTTPSender))
	mt := detectMessageType(ch)
	if mt != "*" {
		t.Errorf("detectMessageType = %q; want * (no filter)", mt)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Port clamping
// ─────────────────────────────────────────────────────────────────────────────

func TestClampPort(t *testing.T) {
	cases := []struct{ p, def, want int }{
		{6661, 6661, 6661},   // valid
		{0, 2575, 2575},      // zero → default
		{-1, 2575, 2575},     // negative → default
		{65535, 443, 65535},  // max valid
		{65536, 443, 443},    // overflow → default
		{80, 8080, 80},       // normal HTTP
	}
	for _, c := range cases {
		got := clampPort(c.p, c.def)
		if got != c.want {
			t.Errorf("clampPort(%d, %d) = %d; want %d", c.p, c.def, got, c.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] xmlVal helpers
// ─────────────────────────────────────────────────────────────────────────────

func TestXmlVal_ExtractsSimpleTag(t *testing.T) {
	inner := []byte("<host>0.0.0.0</host><port>6661</port>")
	if got := xmlVal(inner, "host"); got != "0.0.0.0" {
		t.Errorf("xmlVal host = %q; want 0.0.0.0", got)
	}
	if got := xmlValInt(inner, "port", 0); got != 6661 {
		t.Errorf("xmlValInt port = %d; want 6661", got)
	}
}

func TestXmlVal_MissingTag_ReturnsDefault(t *testing.T) {
	inner := []byte("<host>localhost</host>")
	if got := xmlValDefault(inner, "port", "2575"); got != "2575" {
		t.Errorf("xmlValDefault(missing) = %q; want 2575", got)
	}
	if got := xmlValInt(inner, "port", 99); got != 99 {
		t.Errorf("xmlValInt(missing) = %d; want 99", got)
	}
}

func TestXmlVal_MultilineContent(t *testing.T) {
	inner := []byte("<script>var x = 1;\nvar y = 2;</script>")
	val := xmlVal(inner, "script")
	if !strings.Contains(val, "var x") {
		t.Errorf("multiline xmlVal failed: %q", val)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [UNIT] Script extraction
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractScript_MirthMapFormat(t *testing.T) {
	inner := []byte(`
		<data class="map">
		  <entry>
		    <string>Script</string>
		    <string>var pid = msg['PID']['PID.3'];</string>
		  </entry>
		</data>`)
	got := extractScript(inner)
	if got != "var pid = msg['PID']['PID.3'];" {
		t.Errorf("extractScript = %q; want the JS content", got)
	}
}

func TestExtractScript_EmptyStep_ReturnsEmpty(t *testing.T) {
	inner := []byte("<data class=\"map\"><entry></entry></data>")
	got := extractScript(inner)
	if got != "" {
		t.Errorf("extractScript(empty) = %q; want empty string", got)
	}
}

func TestExtractScript_MultilineScript(t *testing.T) {
	inner := []byte(`<data class="map"><entry>
		<string>Script</string>
		<string>var a = 1;
var b = 2;
return a + b;</string>
	</entry></data>`)
	got := extractScript(inner)
	if !strings.Contains(got, "var a = 1;") || !strings.Contains(got, "return a + b;") {
		t.Errorf("multiline script extraction failed: %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// [INTEGRATION] Full import flow
// ─────────────────────────────────────────────────────────────────────────────

func TestMirthMigrationIntegration_Import(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	svc := NewMirthMigrationService(db)

	// Preview first
	preview, err := svc.Preview([]byte(validMirthXML))
	if err != nil {
		t.Fatalf("Preview error: %v", err)
	}
	if preview.ChannelName != "ADT Test Channel" {
		t.Fatalf("unexpected channel name: %q", preview.ChannelName)
	}

	// Import — UserID not provided by request body (extracted from header in real requests)
	req := &models.MigrationImportRequest{
		ChannelXML:    validMirthXML,
		InterfaceName: "Integration Test ADT Channel",
		Description:   "Created by integration test — safe to delete",
		UserID:        testUserID(t, db),
	}
	result, err := svc.Import(context.Background(), req)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}

	if result.InterfaceID == "" {
		t.Error("InterfaceID should not be empty")
	}
	if result.PipelineID == "" {
		t.Error("PipelineID should not be empty")
	}
	if result.StepsCreated != 3 {
		t.Errorf("StepsCreated = %d; want 3", result.StepsCreated)
	}

	// Verify interface exists in DB
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM interfaces WHERE id = $1`, result.InterfaceID).Scan(&count); err != nil || count != 1 {
		t.Errorf("interface not found in DB: count=%d err=%v", count, err)
	}

	// Verify pipeline exists in DB
	if err = db.QueryRow(`SELECT COUNT(*) FROM transformation_pipelines WHERE id = $1`, result.PipelineID).Scan(&count); err != nil || count != 1 {
		t.Errorf("pipeline not found in DB: count=%d err=%v", count, err)
	}

	// Verify steps exist in DB
	if err = db.QueryRow(`SELECT COUNT(*) FROM transformation_steps WHERE pipeline_id = $1`, result.PipelineID).Scan(&count); err != nil || count != 3 {
		t.Errorf("expected 3 steps in DB, got %d (err: %v)", count, err)
	}

	// Verify audit history
	if err = db.QueryRow(`SELECT COUNT(*) FROM mirth_migration_history WHERE interface_id = $1`, result.InterfaceID).Scan(&count); err != nil || count != 1 {
		t.Errorf("expected 1 audit history row, got %d (err: %v)", count, err)
	}

	// Cleanup test data
	db.Exec(`DELETE FROM transformation_steps WHERE pipeline_id = $1`, result.PipelineID)
	db.Exec(`DELETE FROM transformation_pipelines WHERE id = $1`, result.PipelineID)
	db.Exec(`DELETE FROM mirth_migration_history WHERE interface_id = $1`, result.InterfaceID)
	db.Exec(`DELETE FROM interfaces WHERE id = $1`, result.InterfaceID)
}

func TestMirthMigrationIntegration_SkipSteps(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	svc := NewMirthMigrationService(db)
	req := &models.MigrationImportRequest{
		ChannelXML:      validMirthXML,
		InterfaceName:   "Skip Test Channel",
		Description:     "Integration test — safe to delete",
		UserID:          testUserID(t, db),
		SkipStepIndices: []int{0, 2}, // skip filter rule (0) and mapper step (2)
	}

	result, err := svc.Import(context.Background(), req)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if result.StepsCreated != 1 {
		t.Errorf("StepsCreated = %d; want 1 (only JS step)", result.StepsCreated)
	}
	if result.StepsSkipped != 2 {
		t.Errorf("StepsSkipped = %d; want 2", result.StepsSkipped)
	}

	// Cleanup
	db.Exec(`DELETE FROM transformation_steps WHERE pipeline_id = $1`, result.PipelineID)
	db.Exec(`DELETE FROM transformation_pipelines WHERE id = $1`, result.PipelineID)
	db.Exec(`DELETE FROM mirth_migration_history WHERE interface_id = $1`, result.InterfaceID)
	db.Exec(`DELETE FROM interfaces WHERE id = $1`, result.InterfaceID)
}

func TestMirthMigrationIntegration_HistoryQuery(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	svc := NewMirthMigrationService(db)
	entries, err := svc.History(context.Background(), 10)
	if err != nil {
		t.Fatalf("History error: %v", err)
	}
	if entries == nil {
		t.Error("History() should return empty slice, not nil")
	}
}
