package ai

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Key order matters: segment order must follow the JSON object order, not
// the files' unreliable "sequence" values (here deliberately scrambled).
const hl7FixtureADT = `{
  "messageType": "ADT_A01",
  "description": "Admit/visit notification. Sent when a patient is admitted.",
  "version": "2.5.1",
  "structure": {
    "MSH": {"description": "The MSH segment defines the intent of the message.", "usage": "R", "repeat": "1", "sequence": "9", "type": "segment",
      "fields": {"MSH.10": {"name": "Message Control ID", "dataType": "ST", "usage": "R", "repeat": "1"},
                 "MSH.9": {"name": "Message Type", "dataType": "MSG", "dataTypeName": "Message Type", "usage": "R", "repeat": "1",
                           "components": {"MSH.9.2": {"name": "Trigger Event", "dataType": "ID", "components": {"x": {"name": "deep"}}},
                                          "MSH.9.1": {"name": "Message Code", "dataType": "ID"}}}}},
    "PID": {"description": "Patient identification.", "usage": "R", "repeat": "1", "sequence": 1, "type": "segment",
      "fields": {"PID.3": {"name": "Patient Identifier List", "dataType": "CX", "usage": "R", "repeat": "*"},
                 "PID.8": {"name": "Administrative Sex", "dataType": "IS", "usage": "O", "repeat": "1", "tableId": 1, "tableName": "Administrative Sex"}}},
    "PROCEDURE": {"usage": "O", "repeat": "*", "sequence": null, "type": "group",
      "segments": {"PR1": {"description": "Procedures.", "usage": "R", "repeat": "1", "type": "segment",
                           "fields": {"PR1.1": {"name": "Set ID - PR1", "dataType": "SI", "usage": "R", "repeat": "1"}}}}}
  }
}`

const hl7FixtureA04 = `{"messageType": "ADT_A04", "structure": {
  "MSH": {"usage": "R", "repeat": "1", "type": "segment", "fields": {"MSH.9": {"name": "Message Type", "dataType": "MSG", "usage": "R", "repeat": "1"}}},
  "PID": {"usage": "R", "repeat": "1", "type": "segment", "fields": {"PID.3": {"name": "Patient Identifier List", "dataType": "CX", "usage": "R", "repeat": "*"}}}}}`

func docByRef(docs []SchemaDigestDoc, ref string) (SchemaDigestDoc, bool) {
	for _, d := range docs {
		if d.Ref == ref {
			return d, true
		}
	}
	return SchemaDigestDoc{}, false
}

func TestHL7Digester_MessageStructureKeepsOrderAndGroups(t *testing.T) {
	docs, err := newHL7v2Digester("2.5.1").DigestFile("ADT_A01", []byte(hl7FixtureADT))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := docByRef(docs, "message:ADT_A01")
	if !ok {
		t.Fatal("no message structure document")
	}
	iMSH, iPID, iGrp, iPR1 := strings.Index(msg.Text, "\nMSH ["), strings.Index(msg.Text, "\nPID ["), strings.Index(msg.Text, "PROCEDURE group [O, *]"), strings.Index(msg.Text, "  PR1 [R]")
	if iMSH < 0 || iPID < iMSH || iGrp < iPID || iPR1 < iGrp {
		t.Fatalf("segments not in JSON order with the group nested:\n%s", msg.Text)
	}
	if !strings.Contains(msg.Text, "HL7 v2.5.1 message structure ADT_A01 — Admit/visit notification.") {
		t.Errorf("header wrong:\n%s", msg.Text)
	}
}

func TestHL7Digester_SegmentFieldsSortedNumericallyWithComponents(t *testing.T) {
	docs, _ := newHL7v2Digester("2.5.1").DigestFile("ADT_A01", []byte(hl7FixtureADT))
	msh, ok := docByRef(docs, "segment:MSH")
	if !ok {
		t.Fatal("no MSH segment document")
	}
	if strings.Index(msh.Text, "MSH.9 ") > strings.Index(msh.Text, "MSH.10 ") {
		t.Errorf("MSH.9 must sort before MSH.10:\n%s", msh.Text)
	}
	if !strings.Contains(msh.Text, "components: MSH.9.1 Message Code (ID); MSH.9.2 Trigger Event (ID)") {
		t.Errorf("components missing or unordered:\n%s", msh.Text)
	}
	if strings.Contains(msh.Text, "deep") {
		t.Error("components must be rendered one level deep only")
	}
	pid, _ := docByRef(docs, "segment:PID")
	if !strings.Contains(pid.Text, "PID.3 Patient Identifier List (CX; R, *)") ||
		!strings.Contains(pid.Text, "PID.8 Administrative Sex (IS; O) table 1 Administrative Sex") {
		t.Errorf("PID fields rendered wrong:\n%s", pid.Text)
	}
	if _, ok := docByRef(docs, "segment:PR1"); !ok {
		t.Error("segments inside groups must be collected")
	}
}

func TestHL7Digester_SharedSegmentsEmittedOnce(t *testing.T) {
	d := newHL7v2Digester("2.5.1")
	first, _ := d.DigestFile("ADT_A01", []byte(hl7FixtureADT))
	second, _ := d.DigestFile("ADT_A04", []byte(hl7FixtureA04))
	if _, ok := docByRef(first, "segment:PID"); !ok {
		t.Fatal("PID expected in the first file's documents")
	}
	if _, ok := docByRef(second, "segment:PID"); ok {
		t.Error("PID was emitted twice")
	}
	if _, ok := docByRef(second, "message:ADT_A04"); !ok {
		t.Error("every message still gets its own structure document")
	}
}

const fhirFixturePatient = `{
  "resourceType": "Patient", "name": "Patient", "title": "", "description": "Demographics and other administrative information. More text.",
  "profile": "http://hl7.org/fhir/StructureDefinition/Patient",
  "elements": {
    "Patient.meta": {"path": "Patient.meta", "dataType": "Meta", "cardinality": "0..1"},
    "Patient.extension": {"path": "Patient.extension", "dataType": "Extension", "cardinality": "0..*"},
    "Patient.name": {"path": "Patient.name", "name": "A name associated with the patient", "dataType": "HumanName", "cardinality": "0..*", "mustSupport": true},
    "Patient.gender": {"path": "Patient.gender", "name": "male | female | other | unknown", "dataType": "code", "cardinality": "0..1", "valueSet": "http://hl7.org/fhir/ValueSet/administrative-gender", "bindingStrength": "required"},
    "Patient.communication.language": {"path": "Patient.communication.language", "name": "Language", "dataType": "CodeableConcept", "cardinality": "1..1"},
    "Patient.communication.extension": {"path": "Patient.communication.extension", "dataType": "Extension", "cardinality": "0..*"}
  }
}`

func TestFHIRDigester_ElementsWithoutBoilerplate(t *testing.T) {
	docs, err := newFHIRStructureDigester("FHIR R4 resource").DigestFile("Patient", []byte(fhirFixturePatient))
	if err != nil || len(docs) != 1 {
		t.Fatalf("docs=%d err=%v", len(docs), err)
	}
	text := docs[0].Text
	for _, want := range []string{
		"FHIR R4 resource Patient — Demographics and other administrative information.",
		"Patient.name (HumanName, 0..*, MS) — A name associated with the patient",
		"Patient.gender (code, 0..1) — male | female | other | unknown [binding required http://hl7.org/fhir/ValueSet/administrative-gender]",
		"Patient.communication.language (CodeableConcept, 1..1)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Patient.meta", "Patient.extension", "Patient.communication.extension"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("boilerplate %q should be omitted", unwanted)
		}
	}
}

func TestDigestDir_ReadsGzipAndPlainJSON(t *testing.T) {
	dir := t.TempDir()
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(hl7FixtureADT))
	zw.Close()
	if err := os.WriteFile(filepath.Join(dir, "ADT_A01.gz"), gz.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ADT_A04.json"), []byte(hl7FixtureA04), 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644)

	docs, files, errs := digestDir(dir, newHL7v2Digester("2.5.1"))
	if files != 3 || len(errs) != 1 {
		t.Fatalf("files=%d errs=%v, want 3 files and 1 error", files, errs)
	}
	for _, ref := range []string{"message:ADT_A01", "message:ADT_A04", "segment:PID"} {
		if _, ok := docByRef(docs, ref); !ok {
			t.Errorf("missing %s", ref)
		}
	}
}

// Real shipped schemas (needs schemas/ present — downloaded by CI, bind-
// mounted in the Docker test run; skipped otherwise). Guards the reason
// digests exist: a summary must stay small enough to embed in minutes, not
// months. FHIR sources are small and checked in full. The HL7 tree is ~130 MB
// compressed (~2 min to parse), too slow for CI's per-package timeout, so it
// is checked on a representative sample plus a size cap on every document —
// the cap is what bounds total growth (documents = distinct segments +
// message structures, both bounded by the standard itself).
func TestSchemaDigest_RealSchemasStayCompact(t *testing.T) {
	root := filepath.Join("..", "..", "schemas")
	const maxChunksPerDoc = 40 // ~50 KB; the real PID segment is ~5 chunks

	checkDocs := func(label string, docs []SchemaDigestDoc) {
		for _, d := range docs {
			if c := len(d.Text) / (chunkSize - chunkOverlap); c > maxChunksPerDoc {
				t.Errorf("%s %s digest document is too large (~%d chunks)", label, d.Ref, c)
			}
		}
	}

	for _, src := range schemaDigestSources {
		if src.SourceType == "hl7_v2" {
			continue // sampled below
		}
		dir := filepath.Join(root, src.Dir)
		if _, err := os.Stat(dir); err != nil {
			t.Logf("%s: %s not present — skipped", src.Key, dir)
			continue
		}
		docs, files, errs := digestDir(dir, src.New())
		if len(errs) > 0 {
			t.Errorf("%s: %d file errors, first: %s", src.Key, len(errs), errs[0])
		}
		if files > 0 && len(docs) == 0 {
			t.Errorf("%s produced no documents", src.Key)
		}
		total := 0
		for _, d := range docs {
			total += len(d.Text)
		}
		chunks := total / (chunkSize - chunkOverlap)
		t.Logf("%s: %d files → %d documents, ~%d chunks", src.Key, files, len(docs), chunks)
		if chunks > 3000 {
			t.Errorf("%s digest is too large to embed (~%d chunks)", src.Key, chunks)
		}
		checkDocs(src.Key, docs)
	}

	hl7Dir := filepath.Join(root, "hl7", "v2.5.1")
	if _, err := os.Stat(hl7Dir); err != nil {
		t.Logf("hl7_v2: %s not present — skipped", hl7Dir)
		return
	}
	d := newHL7v2Digester("2.5.1")
	var docs []SchemaDigestDoc
	for _, msg := range []string{"ADT_A01", "ORU_R01", "ORM_O01", "SIU_S12", "MDM_T02", "DFT_P03", "VXU_V04"} {
		content, err := readMaybeGzip(filepath.Join(hl7Dir, msg+".gz"))
		if err != nil {
			t.Errorf("hl7 sample %s: %v", msg, err)
			continue
		}
		out, err := d.DigestFile(msg, content)
		if err != nil {
			t.Errorf("hl7 sample %s: %v", msg, err)
			continue
		}
		docs = append(docs, out...)
	}
	checkDocs("hl7_v2", docs)
	if pid, ok := docByRef(docs, "segment:PID"); !ok || !strings.Contains(pid.Text, "PID.3 Patient Identifier List") {
		t.Errorf("real PID digest missing PID.3:\n%.600s", pid.Text)
	}
	if oru, ok := docByRef(docs, "message:ORU_R01"); !ok || !strings.Contains(oru.Text, "PATIENT RESULT group") {
		t.Errorf("real ORU_R01 structure missing its PATIENT RESULT group:\n%.600s", oru.Text)
	}
}
