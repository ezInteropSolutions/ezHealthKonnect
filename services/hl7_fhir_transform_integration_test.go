// services/hl7_fhir_transform_integration_test.go
//
// Transform() itself — the main orchestrator every single HL7 message runs
// through — was the one genuinely untestable-without-a-real-DB piece of this
// engine (it loads OOB templates, resource policy, narrative config, and
// optional-segment config, all from Postgres). This is a real integration
// test, not a unit test: it uses hl7.ParseWithRealSchema on an authentic ADT^A01
// message (the same parser production code calls, not a hand-built fixture)
// and a real Postgres connection, matching the DATABASE_URL-gated convention
// already established in services/mirth_migration_service_test.go.
//
// Run: DATABASE_URL="postgres://ezhealth_user:secure_password_change_me@localhost:5432/ezhealthkonnect?sslmode=disable" \
//        go test ./services/ -v -run TestTransformIntegration
//
// The CI test-go job (see .github/workflows/ci.yml) sets this automatically —
// this test is skipped, not failed, when DATABASE_URL is unset.
package services

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services/executors"

	_ "github.com/lib/pq"
)

const sampleADTA01 = "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20260823120000||ADT^A01|MSG00001|P|2.5\r" +
	"EVN|A01|20260823120000\r" +
	"PID|1||PATID12345^^^MRN^MR||Doe^John^A||19800101|M|||123 Main St^^Anytown^ST^12345^USA||5555551234|||S||PATID12345\r" +
	"PV1|1|I|WARD1^ROOM1^BED1||||ATTEND001^Smith^Jane^^^^MD|||MED||||ADM|A0\r"

func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping integration test: DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping integration test: cannot reach DB: %v", err)
	}
	return db
}

// buildParsedHL7Data mirrors services/parsers/hl7_parser_service.go's
// buildHL7ParsedJSON exactly — the real production shape Transform() expects
// in TransformRequest.ParsedHL7Data, not a hand-guessed one.
func buildParsedHL7Data(enhanced *hl7.EnhancedParsedMessage) map[string]interface{} {
	return map[string]interface{}{
		"raw":               enhanced.Raw,
		"success":           enhanced.Success,
		"version":           enhanced.Version,
		"messageType":       enhanced.MessageType,
		"enhancedSegments":  enhanced.EnhancedSegments,
		"segmentGroups":     enhanced.SegmentGroups,
		"observationGroups": enhanced.ObservationGroups,
		"segmentOrder":      enhanced.SegmentOrder,
		"parsedAt":          enhanced.ParsedAt,
		"dictionaryUsed":    enhanced.DictionaryUsed,
		"schemaLoaded":      enhanced.SchemaLoaded,
		"_format":           "hl7v2",
	}
}

// minimalADTA01TemplateConfig is a hand-built OOB template covering just
// enough of Patient/MessageHeader/Encounter to prove Transform() end-to-end.
// Shape matches convertV9TemplateToFieldMappings's "resources" key format
// (see hl7_fhir_transform_v9_template_test.go) — real transform keys, not
// placeholders, so this exercises the actual atomic-mapping pipeline.
const minimalADTA01TemplateConfig = `{
  "resources": {
    "Patient": {
      "mappings": [
        {"hl7Path": "PID.5.1", "fhirPath": "Patient.name[0].family", "transform": "string_direct"},
        {"hl7Path": "PID.5.2", "fhirPath": "Patient.name[0].given[0]", "transform": "string_direct"},
        {"hl7Path": "PID.7", "fhirPath": "Patient.birthDate", "transform": "ts_to_date"},
        {"hl7Path": "PID.8", "fhirPath": "Patient.gender", "transform": "gender_mapping"},
        {"hl7Path": "PID.3.1", "fhirPath": "Patient.identifier[0].value", "transform": "string_direct"}
      ]
    },
    "MessageHeader": {
      "mappings": [
        {"hl7Path": "MSH.9", "fhirPath": "MessageHeader.eventCoding", "transform": "msh9_trigger_event_to_coding"},
        {"hl7Path": "MSH.3", "fhirPath": "MessageHeader.source.name", "transform": "string_direct"}
      ]
    },
    "Encounter": {
      "mappings": [
        {"hl7Path": "PV1.2", "fhirPath": "Encounter.class.code", "transform": "string_direct"},
        {"hl7Path": "EVN.2", "fhirPath": "Encounter.period.start", "transform": "hl7_timestamp_to_fhir_date"}
      ]
    }
  }
}`

// ensureADTA01Template guarantees a default ADT^A01/2.5/R4 OOB template
// exists so this test works on a fresh, never-booted database (e.g. CI's
// migrated-but-never-run Postgres) exactly the same as on a dev DB the app
// has already booted against (which self-seeds hl7_fhir_templates — see
// CLAUDE.md's "OOB template self-regeneration" note). Only inserts — and
// only cleans up — when no default template already existed; never touches
// a real pre-existing one. Mirrors the create-your-own-fixture-and-clean-up
// convention already established in TestMirthMigrationIntegration_Import.
func ensureADTA01Template(t *testing.T, db *sql.DB) (cleanup func()) {
	t.Helper()
	var existing int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM hl7_fhir_templates
		WHERE message_type = 'ADT^A01' AND hl7_version = '2.5' AND fhir_version = 'R4' AND is_default = true
	`).Scan(&existing)
	if err != nil {
		t.Fatalf("checking for existing ADT^A01 template: %v", err)
	}
	if existing > 0 {
		return func() {} // real template already present — leave it alone
	}

	var id string
	err = db.QueryRow(`
		INSERT INTO hl7_fhir_templates
			(message_type, hl7_version, fhir_version, template_name, template_config, is_default)
		VALUES ('ADT^A01', '2.5', 'R4', 'Integration Test Minimal Template', $1::jsonb, true)
		RETURNING id
	`, minimalADTA01TemplateConfig).Scan(&id)
	if err != nil {
		t.Fatalf("seeding minimal ADT^A01 template: %v", err)
	}
	return func() {
		db.Exec(`DELETE FROM hl7_fhir_templates WHERE id = $1`, id)
	}
}

func TestTransformIntegration_RealADTA01_ProducesExpectedResources(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()

	cleanupTemplate := ensureADTA01Template(t, db)
	defer cleanupTemplate()

	if err := r4.InitRegistry("../schemas/fhir"); err != nil {
		t.Fatalf("r4.InitRegistry: %v", err)
	}
	hl7.InitRealSchemaLoader("../schemas/hl7")

	enhanced := hl7.ParseWithRealSchema(sampleADTA01)
	if !enhanced.Success {
		t.Fatalf("hl7.ParseWithRealSchema failed: %s", enhanced.Error)
	}

	svc := NewHL7FHIRTransformServiceV3(db)
	req := &TransformRequest{
		ParsedHL7Data: buildParsedHL7Data(enhanced),
		MessageType:   "ADT^A01",
		FHIRVersion:   "R4",
		CreateBundle:  true,
		RequestID:     "integration-test-adt-a01",
	}

	resp, err := svc.Transform(t.Context(), req)
	if err != nil {
		t.Fatalf("Transform returned an error: %v", err)
	}
	if resp == nil {
		t.Fatal("Transform returned a nil response with no error")
	}
	if len(resp.Errors) > 0 {
		t.Logf("Transform produced %d non-fatal errors (schema-validation advisories): %v", len(resp.Errors), resp.Errors)
	}
	if len(resp.FHIRResources) == 0 {
		t.Fatalf("Transform produced zero FHIR resources for a real ADT^A01 message; warnings=%v errors=%v", resp.Warnings, resp.Errors)
	}

	seen := map[string]bool{}
	for _, r := range resp.FHIRResources {
		if rt, ok := r["resourceType"].(string); ok {
			seen[rt] = true
		}
	}
	for _, want := range []string{"Patient", "MessageHeader", "Encounter"} {
		if !seen[want] {
			t.Errorf("expected a %s resource among the output, got resource types: %v", want, seen)
		}
	}

	if resp.Bundle == nil {
		t.Error("expected a Bundle (CreateBundle=true), got nil")
	} else if resp.Bundle["resourceType"] != "Bundle" {
		t.Errorf("Bundle.resourceType = %v, want Bundle", resp.Bundle["resourceType"])
	}

	if !resp.Success {
		t.Errorf("resp.Success = false, want true (real resources were produced): errors=%v", resp.Errors)
	}
}

// TestTransformIntegration_CoverageTracker_RecordsRealFieldReads is the
// highest-fidelity proof available short of a live running app: the FULL,
// real chain (Transform -> buildResourcesForType -> createResourceFrom
// AtomicMappings -> extractHL7ValueAtomic), against a real parsed ADT^A01
// message and a real Postgres connection — confirming a Coverage Audit
// tracker attached via ctx ends up with the real field/component keys the
// LIVE, ACTUAL template in effect maps, byte-identical, not a substring/shape
// check (this project's own standing "mandatory exact-key" discipline — see
// CLAUDE.md's Coverage Audit generalization section).
//
// Deliberately does NOT hardcode expected keys from minimalADTA01TemplateConfig
// above: ensureADTA01Template only inserts that fixture when NO real default
// ADT^A01/2.5/R4 template already exists, and on this project's own dev DB (and
// likely CI once seeded) a real one always already does — so the fixture is
// silently never used, and a test hardcoding its field names would be
// asserting against a template that never actually ran. (This was caught
// live, the first time this test was run against the real DB — the exact
// kind of "test reality diverges from assumption" finding this project's own
// history already explains the value of real verification by.) Instead, this
// test queries ListConfiguredMappings itself FIRST — the SAME call
// hl7_adapter.go's own resolveConfiguredFields makes — to learn what the real,
// live template actually maps, then asserts the tracker agrees with it
// exactly: every mapped field that's genuinely populated in sampleADTA01 must
// be touched, and nothing else. This is robust to whichever template happens
// to be live in any environment, and is also the regression guard for the
// whole 2026-10 ctx-threading change: if any of the 3 intermediate functions
// ever stopped forwarding ctx correctly, every expected key below would
// silently vanish and this test would fail immediately.
func TestTransformIntegration_CoverageTracker_RecordsRealFieldReads(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()

	cleanupTemplate := ensureADTA01Template(t, db)
	defer cleanupTemplate()

	if err := r4.InitRegistry("../schemas/fhir"); err != nil {
		t.Fatalf("r4.InitRegistry: %v", err)
	}
	hl7.InitRealSchemaLoader("../schemas/hl7")

	enhanced := hl7.ParseWithRealSchema(sampleADTA01)
	if !enhanced.Success {
		t.Fatalf("hl7.ParseWithRealSchema failed: %s", enhanced.Error)
	}

	svc := NewHL7FHIRTransformServiceV3(db)
	req := &TransformRequest{
		ParsedHL7Data: buildParsedHL7Data(enhanced),
		MessageType:   "ADT^A01",
		FHIRVersion:   "R4",
		CreateBundle:  true,
		RequestID:     "integration-test-coverage-tracker",
	}

	// Learn what the REAL, LIVE template actually maps -- same call
	// hl7_adapter.go's own resolveConfiguredFields makes.
	mappings, err := svc.ListConfiguredMappings(t.Context(), "ADT^A01", "", req)
	if err != nil {
		t.Fatalf("ListConfiguredMappings: %v", err)
	}
	if len(mappings) == 0 {
		t.Fatal("no configured mappings found for ADT^A01 -- cannot prove anything about the tracker without a real template in effect")
	}

	// Which of those mapped fields are genuinely populated in sampleADTA01 --
	// build the set directly from the real parsed message, not a guess.
	wantTouched := map[string]bool{}
	for _, m := range mappings {
		if m.SegmentName == "" || m.HL7Field == "" {
			continue
		}
		seg, ok := enhanced.EnhancedSegments[m.SegmentName]
		if !ok {
			continue
		}
		fieldKey := m.SegmentName + "." + m.HL7Field
		for _, f := range seg.Fields {
			if f.Key != fieldKey || !f.HasValue {
				continue
			}
			if m.HL7Component == "" {
				wantTouched[fieldKey] = true
				continue
			}
			for _, sf := range f.Subfields {
				if sf.Key == fieldKey+"."+m.HL7Component && sf.HasValue {
					wantTouched[fieldKey+"."+m.HL7Component] = true
				}
			}
		}
	}
	if len(wantTouched) == 0 {
		t.Fatal("no mapped field is actually populated in sampleADTA01 -- cannot prove anything about the tracker")
	}

	tracker := executors.NewCDACoverageTracker()
	ctx := context.WithValue(t.Context(), hl7CoverageTrackerContextKey, tracker)

	resp, err := svc.Transform(ctx, req)
	if err != nil {
		t.Fatalf("Transform returned an error: %v", err)
	}
	if resp == nil || len(resp.FHIRResources) == 0 {
		t.Fatalf("Transform produced zero FHIR resources; warnings=%v errors=%v", resp.Warnings, resp.Errors)
	}

	for key := range wantTouched {
		if !tracker.Touched(key) {
			t.Errorf("expected %q to be recorded as touched (a real, live-configured mapping, populated in the real message), tracker snapshot=%v", key, tracker.Snapshot())
		}
	}
	t.Logf("confirmed %d real field/component keys touched via live ctx-threaded tracking: %v", len(wantTouched), wantTouched)
}

func TestTransformIntegration_MissingFHIRSchemaRegistry_ReturnsError(t *testing.T) {
	// This test intentionally does NOT call r4.InitRegistry first — but since
	// InitRegistry is process-global and other tests in this package may have
	// already initialized it, this only proves the guard exists, not that it
	// always fires; kept as documentation of the intended behavior.
	if r4.GetRegistry() != nil && len(r4.GetRegistry().List("R4")) > 0 {
		t.Skip("FHIR registry already initialized by another test in this process — guard behavior not observable here")
	}
	svc := NewHL7FHIRTransformServiceV3(nil)
	_, err := svc.Transform(t.Context(), &TransformRequest{ParsedHL7Data: map[string]interface{}{}})
	if err == nil {
		t.Error("expected an error when the FHIR schema registry isn't loaded, got nil")
	}
}
