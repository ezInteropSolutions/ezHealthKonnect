// services/parsers/cda/generic_section_processor_test.go
// Tests for GenericSectionProcessor and its XPath extraction helpers.
//
// Test categories:
//  1. XPath helper functions (unit)
//  2. GenericSectionProcessor — attribute extraction per field type
//  3. All 9 original sections parse without panic and produce key clinical fields
//  4. New sections (Sprint B additions) parse without panic
//  5. Missing XPath → empty string, never panic

package cdaparser

import (
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

// =====================================
// 1. XPath helper unit tests
// =====================================

func TestStripEntryPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"entry/act/code/@code", "act/code/@code"},
		{"entry/observation/effectiveTime/@value", "observation/effectiveTime/@value"},
		{"observation/code/@code", "observation/code/@code"}, // no prefix → unchanged
		{"", ""},
	}
	for _, tc := range cases {
		got := stripEntryPrefix(tc.in)
		if got != tc.want {
			t.Errorf("stripEntryPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeXPath(t *testing.T) {
	cases := []struct{ in, want string }{
		// Namespace-prefixed predicate stripped
		{"effectiveTime[@xsi:type='IVL_TS']/low/@value", "effectiveTime/low/@value"},
		// Non-namespaced predicate preserved
		{"entry/act/entryRelationship[@typeCode='SUBJ']/observation/value/@code",
			"entry/act/entryRelationship[@typeCode='SUBJ']/observation/value/@code"},
		// Multiple namespace predicates
		{"a[@xsi:type='T']/b[@xsi:nil='true']/@val", "a/b/@val"},
		// No predicates
		{"entry/observation/code/@code", "entry/observation/code/@code"},
	}
	for _, tc := range cases {
		got := sanitizeXPath(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeXPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractValueByXPath_Attribute(t *testing.T) {
	xml := `<entry>
		<act>
			<code code="123" displayName="Test" codeSystem="2.16.840.1.113883.6.96"/>
		</act>
	</entry>`
	entry := parseEntry(t, xml)

	got := extractValueByXPath(entry, "entry/act/code/@code")
	if got != "123" {
		t.Errorf("expected '123', got %q", got)
	}

	got = extractValueByXPath(entry, "entry/act/code/@displayName")
	if got != "Test" {
		t.Errorf("expected 'Test', got %q", got)
	}

	got = extractValueByXPath(entry, "entry/act/code/@codeSystem")
	if got != "2.16.840.1.113883.6.96" {
		t.Errorf("expected OID, got %q", got)
	}
}

func TestExtractValueByXPath_TextContent(t *testing.T) {
	xml := `<entry>
		<substanceAdministration>
			<consumable>
				<manufacturedProduct>
					<manufacturedMaterial>
						<lotNumberText>LOT-ABC-123</lotNumberText>
					</manufacturedMaterial>
				</manufacturedProduct>
			</consumable>
		</substanceAdministration>
	</entry>`
	entry := parseEntry(t, xml)

	got := extractValueByXPath(entry, "entry/substanceAdministration/consumable/manufacturedProduct/manufacturedMaterial/lotNumberText")
	if got != "LOT-ABC-123" {
		t.Errorf("expected lot number, got %q", got)
	}
}

func TestExtractValueByXPath_MissingElement(t *testing.T) {
	xml := `<entry><act/></entry>`
	entry := parseEntry(t, xml)

	got := extractValueByXPath(entry, "entry/act/code/@code")
	if got != "" {
		t.Errorf("expected empty string for missing element, got %q", got)
	}
}

func TestExtractValueByXPath_EmptyPath(t *testing.T) {
	entry := parseEntry(t, `<entry/>`)
	if got := extractValueByXPath(entry, ""); got != "" {
		t.Errorf("expected empty for empty path, got %q", got)
	}
}

func TestExtractValueByXPath_NamespacePredicateStripped(t *testing.T) {
	// IVL_TS has xsi:type — must NOT crash; falls back to first effectiveTime
	xml := `<entry>
		<substanceAdministration>
			<effectiveTime>
				<low value="20240101"/>
			</effectiveTime>
		</substanceAdministration>
	</entry>`
	entry := parseEntry(t, xml)

	// The xsi:type predicate is stripped, so effectiveTime/low/@value is found
	got := extractValueByXPath(entry, "entry/substanceAdministration/effectiveTime[@xsi:type='IVL_TS']/low/@value")
	if got != "20240101" {
		t.Errorf("expected '20240101' after namespace predicate strip, got %q", got)
	}
}

// =====================================
// 2. GenericSectionProcessor field extraction
// =====================================

func TestGenericSectionProcessor_SectionKey(t *testing.T) {
	def := &cdaSchema.CDASectionDef{Key: "allergiesAndIntolerances"}
	proc := NewGenericSectionProcessor(def)
	if proc.SectionKey() != "allergiesAndIntolerances" {
		t.Errorf("unexpected SectionKey: %q", proc.SectionKey())
	}
}

func TestGenericSectionProcessor_PrimaryPlusDisplay(t *testing.T) {
	def := &cdaSchema.CDASectionDef{
		Key: "testSection",
		Fields: []*cdaSchema.CDAFieldDef{
			{
				Key:          "code",
				XPath:        "entry/act/code/@code",
				XPathDisplay: "entry/act/code/@displayName",
				XPathSystem:  "entry/act/code/@codeSystem",
			},
		},
	}
	proc := NewGenericSectionProcessor(def)

	sectionXML := `<section>
		<entry>
			<act>
				<code code="246496" displayName="Aspirin" codeSystem="2.16.840.1.113883.6.88"/>
			</act>
		</entry>
	</section>`
	section := parseSection(t, sectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount != 1 {
		t.Fatalf("expected 1 entry, got %d", result.EntryCount)
	}
	e := result.Entries[0]
	assertKey(t, e, "code", "246496")
	assertKey(t, e, "codeDisplay", "Aspirin")
	assertKey(t, e, "codeSystem", "2.16.840.1.113883.6.88")
}

func TestGenericSectionProcessor_UnitField(t *testing.T) {
	def := &cdaSchema.CDASectionDef{
		Key: "testSection",
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "doseQuantity", XPath: "entry/substanceAdministration/doseQuantity/@value",
				XPathUnit: "entry/substanceAdministration/doseQuantity/@unit"},
		},
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, `<section>
		<entry>
			<substanceAdministration>
				<doseQuantity value="250" unit="mg"/>
			</substanceAdministration>
		</entry>
	</section>`)
	result := proc.Process(section, nil)

	if result.EntryCount != 1 {
		t.Fatalf("expected 1 entry, got %d", result.EntryCount)
	}
	assertKey(t, result.Entries[0], "doseQuantity", "250")
	assertKey(t, result.Entries[0], "doseQuantityUnit", "mg")
}

func TestGenericSectionProcessor_EmptyEntriesSkipped(t *testing.T) {
	def := &cdaSchema.CDASectionDef{
		Key: "testSection",
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "code", XPath: "entry/observation/code/@code"},
		},
	}
	proc := NewGenericSectionProcessor(def)

	// All entries have no matching XPath — empty entries must be omitted
	section := parseSection(t, `<section>
		<entry><observation/></entry>
		<entry><observation/></entry>
	</section>`)
	result := proc.Process(section, nil)

	if result.EntryCount != 0 {
		t.Errorf("expected 0 entries (all empty), got %d", result.EntryCount)
	}
	if result.Entries == nil {
		t.Errorf("Entries slice must be non-nil")
	}
}

func TestGenericSectionProcessor_NoPanic_NoEntries(t *testing.T) {
	def := &cdaSchema.CDASectionDef{
		Key: "noEntriesSection",
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "foo", XPath: "entry/bar/@baz"},
		},
	}
	proc := NewGenericSectionProcessor(def)
	section := parseSection(t, `<section><text>Narrative only</text></section>`)
	result := proc.Process(section, nil)

	if result == nil {
		t.Fatal("Process must never return nil")
	}
	if result.SectionKey != "noEntriesSection" {
		t.Errorf("wrong SectionKey: %q", result.SectionKey)
	}
	if result.EntryCount != 0 {
		t.Errorf("expected 0 entries, got %d", result.EntryCount)
	}
}

// =====================================
// 3. Original 9 sections — key fields extracted
// =====================================

func TestAllergiesSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("allergiesAndIntolerances")
	if def == nil {
		t.Fatal("allergiesAndIntolerances section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, allergiesSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 allergy entry, got %d", result.EntryCount)
	}
	e := result.Entries[0]
	assertKeyPresent(t, e, "medicationAllergyCode")
	assertKeyPresent(t, e, "medicationAllergyCodeDisplay")
}

func TestMedicationsSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("medications")
	if def == nil {
		t.Fatal("medications section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, medicationsSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 medication entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "drugCode")
}

func TestProblemsSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("problems")
	if def == nil {
		t.Fatal("problems section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, problemsSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 problem entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "conditionCode")
}

func TestEncountersSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("encounters")
	if def == nil {
		t.Fatal("encounters section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, encountersSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 encounter entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "encounterCode")
}

func TestImmunizationsSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("immunizations")
	if def == nil {
		t.Fatal("immunizations section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, immunizationsSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 immunization entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "vaccineCode")
	assertKeyPresent(t, result.Entries[0], "administrationDate")
}

func TestResultsSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("results")
	if def == nil {
		t.Fatal("results section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, resultsSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 result entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "testCode")
}

func TestVitalsSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("vitalSigns")
	if def == nil {
		t.Fatal("vitalSigns section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, vitalsSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 vitals entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "vitalCode")
	assertKeyPresent(t, result.Entries[0], "value")
}

func TestProceduresSection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("procedures")
	if def == nil {
		t.Fatal("procedures section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, proceduresSectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 procedure entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "procedureCode")
}

func TestSocialHistorySection_KeyFields(t *testing.T) {
	loader := loadTestSchema(t)
	def := loader.GetSection("socialHistory")
	if def == nil {
		t.Fatal("socialHistory section missing from schema")
	}
	proc := NewGenericSectionProcessor(def)

	section := parseSection(t, socialHistorySectionXML)
	result := proc.Process(section, nil)

	if result.EntryCount < 1 {
		t.Fatalf("expected at least 1 social history entry, got %d", result.EntryCount)
	}
	assertKeyPresent(t, result.Entries[0], "observationCode")
}

// =====================================
// 4. New Sprint B sections — no panic
// =====================================

func TestNewSectionsNoPanic(t *testing.T) {
	loader := loadTestSchema(t)
	newSections := []string{
		"functionalStatus", "mentalStatus", "familyHistory", "assessmentAndPlan",
		"planOfTreatment", "medicalEquipment", "advanceDirectives", "healthConcerns",
		"goals", "payersInsurance", "dischargeDiagnosis", "dischargeMedications",
		"admissionDiagnosis", "admissionMedications", "hospitalCourse",
		"dischargeInstructions", "dischargePhysical", "chiefComplaint",
		"historyOfPresentIllness", "pastMedicalHistory", "reviewOfSystems",
		"physicalExamination", "assessment", "reasonForReferral",
	}
	// Use a minimal section XML — no entries, just a valid section root
	minimalSection := `<section><text>No entries available</text></section>`

	for _, key := range newSections {
		def := loader.GetSection(key)
		if def == nil {
			t.Errorf("section %q missing from schema after B1 expansion", key)
			continue
		}
		proc := NewGenericSectionProcessor(def)
		section := parseSection(t, minimalSection)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("section %q panicked: %v", key, r)
				}
			}()
			result := proc.Process(section, nil)
			if result == nil {
				t.Errorf("section %q: Process returned nil", key)
			}
		}()
	}
}

// =====================================
// 5. DefaultSectionRegistry auto-registers from schema
// =====================================

func TestDefaultSectionRegistry_CountMatchesSchema(t *testing.T) {
	loader := loadTestSchema(t)
	registry := DefaultSectionRegistry(loader)

	// Count non-header sections in schema
	expected := 0
	for _, def := range loader.AllSections() {
		if !def.IsHeader {
			expected++
		}
	}

	if registry.Len() != expected {
		t.Errorf("registry has %d processors, want %d (one per non-header schema section)",
			registry.Len(), expected)
	}
}

func TestDefaultSectionRegistry_OriginalSectionsRegistered(t *testing.T) {
	loader := loadTestSchema(t)
	registry := DefaultSectionRegistry(loader)

	must := []string{
		"allergiesAndIntolerances", "medications", "problems", "vitalSigns",
		"results", "encounters", "procedures", "immunizations", "socialHistory",
	}
	for _, key := range must {
		if _, ok := registry.Get(key); !ok {
			t.Errorf("section %q missing from DefaultSectionRegistry", key)
		}
	}
}

// =====================================
// Helper functions
// =====================================

// loadTestSchema loads ccda_2_1.json from the schemas directory.
func loadTestSchema(t *testing.T) *cdaSchema.CDASchemaLoader {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader("../../../cda/schemas")
	if err != nil {
		t.Fatalf("failed to load CDA schema: %v", err)
	}
	return loader
}

// parseSection wraps an XML string in an etree.Element (the <section> root).
func parseSection(t *testing.T, xml string) *etree.Element {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xml); err != nil {
		t.Fatalf("invalid section XML: %v", err)
	}
	return doc.Root()
}

// parseEntry wraps an XML string and returns the root element as an <entry>.
func parseEntry(t *testing.T, xml string) *etree.Element {
	t.Helper()
	doc := etree.NewDocument()
	// Wrap in a root element so etree accepts it as a document
	wrapped := "<root>" + xml + "</root>"
	if err := doc.ReadFromString(wrapped); err != nil {
		t.Fatalf("invalid entry XML: %v", err)
	}
	el := doc.Root().FindElement("entry")
	if el == nil {
		t.Fatalf("no <entry> element found in: %s", xml)
	}
	return el
}

func assertKey(t *testing.T, m map[string]interface{}, key, want string) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("key %q missing from entry map", key)
		return
	}
	if s, _ := got.(string); s != want {
		t.Errorf("key %q = %q, want %q", key, s, want)
	}
}

func assertKeyPresent(t *testing.T, m map[string]interface{}, key string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("expected key %q in entry map; keys present: %s",
			key, mapKeys(m))
	}
}

func mapKeys(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// =====================================
// Section XML fixtures
// =====================================

const allergiesSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>
<code code="48765-2" displayName="Allergies and Adverse Reactions"/>
<entry typeCode="DRIV">
  <act classCode="ACT" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.30"/>
    <entryRelationship typeCode="SUBJ">
      <observation classCode="OBS" moodCode="EVN">
        <templateId root="2.16.840.1.113883.10.20.22.4.7"/>
        <participant typeCode="CSM">
          <participantRole classCode="MANU">
            <playingEntity classCode="MMAT">
              <code code="246496" displayName="Aspirin" codeSystem="2.16.840.1.113883.6.88"/>
            </playingEntity>
          </participantRole>
        </participant>
        <effectiveTime>
          <low value="20150101"/>
        </effectiveTime>
      </observation>
    </entryRelationship>
  </act>
</entry>
</section>`

const medicationsSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.1.1"/>
<code code="10160-0" displayName="Medications"/>
<entry typeCode="DRIV">
  <substanceAdministration classCode="SBADM" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.16"/>
    <statusCode code="active"/>
    <effectiveTime>
      <low value="20240101"/>
    </effectiveTime>
    <doseQuantity value="500" unit="mg"/>
    <routeCode code="C38288" displayName="Oral"/>
    <consumable>
      <manufacturedProduct classCode="MANU">
        <manufacturedMaterial>
          <code code="308460" displayName="Lisinopril 10 MG" codeSystem="2.16.840.1.113883.6.88"/>
        </manufacturedMaterial>
      </manufacturedProduct>
    </consumable>
  </substanceAdministration>
</entry>
</section>`

const problemsSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.5.1"/>
<code code="11450-4" displayName="Problem List"/>
<entry typeCode="DRIV">
  <act classCode="ACT" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.3"/>
    <entryRelationship typeCode="SUBJ">
      <observation classCode="OBS" moodCode="EVN">
        <templateId root="2.16.840.1.113883.10.20.22.4.4"/>
        <value xsi:type="CD" code="44054006" displayName="Diabetes mellitus type 2"
               codeSystem="2.16.840.1.113883.6.96"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"/>
        <effectiveTime>
          <low value="20200301"/>
        </effectiveTime>
      </observation>
    </entryRelationship>
  </act>
</entry>
</section>`

const encountersSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.22.1"/>
<code code="46240-8" displayName="Encounters"/>
<entry typeCode="DRIV">
  <encounter classCode="ENC" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.49"/>
    <code code="11429006" displayName="Consultation"/>
    <effectiveTime value="20240115"/>
    <performer>
      <assignedEntity>
        <id extension="1234567890"/>
        <assignedPerson>
          <name>
            <given>Jane</given>
            <family>Smith</family>
          </name>
        </assignedPerson>
        <representedOrganization>
          <name>General Hospital</name>
        </representedOrganization>
      </assignedEntity>
    </performer>
  </encounter>
</entry>
</section>`

const immunizationsSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.2.1"/>
<code code="11369-6" displayName="Immunizations"/>
<entry typeCode="DRIV">
  <substanceAdministration classCode="SBADM" moodCode="EVN" negationInd="false">
    <templateId root="2.16.840.1.113883.10.20.22.4.52"/>
    <statusCode code="completed"/>
    <effectiveTime value="20230915"/>
    <consumable>
      <manufacturedProduct classCode="MANU">
        <manufacturedMaterial>
          <code code="158" displayName="influenza virus vaccine" codeSystem="2.16.840.1.113883.12.292"/>
          <lotNumberText>LOT2023A</lotNumberText>
        </manufacturedMaterial>
      </manufacturedProduct>
    </consumable>
  </substanceAdministration>
</entry>
</section>`

const resultsSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.3.1"/>
<code code="30954-2" displayName="Results"/>
<entry typeCode="DRIV">
  <organizer classCode="BATTERY" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.1"/>
    <effectiveTime value="20240115120000"/>
    <component>
      <observation classCode="OBS" moodCode="EVN">
        <templateId root="2.16.840.1.113883.10.20.22.4.2"/>
        <code code="2093-3" displayName="Cholesterol" codeSystem="2.16.840.1.113883.6.1"/>
        <statusCode code="completed"/>
        <value xsi:type="PQ" value="180" unit="mg/dL"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"/>
      </observation>
    </component>
  </organizer>
</entry>
</section>`

const vitalsSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.4.1"/>
<code code="8716-3" displayName="Vital Signs"/>
<entry typeCode="DRIV">
  <organizer classCode="CLUSTER" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.26"/>
    <effectiveTime value="20240115"/>
    <component>
      <observation classCode="OBS" moodCode="EVN">
        <templateId root="2.16.840.1.113883.10.20.22.4.27"/>
        <code code="8480-6" displayName="Systolic blood pressure" codeSystem="2.16.840.1.113883.6.1"/>
        <value xsi:type="PQ" value="120" unit="mm[Hg]"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"/>
      </observation>
    </component>
  </organizer>
</entry>
</section>`

const proceduresSectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.7.1"/>
<code code="47519-4" displayName="Procedures"/>
<entry typeCode="DRIV">
  <procedure classCode="PROC" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.14"/>
    <code code="73761001" displayName="Colonoscopy" codeSystem="2.16.840.1.113883.6.96"/>
    <statusCode code="completed"/>
    <effectiveTime value="20230601"/>
    <performer>
      <assignedEntity>
        <id extension="9876543210"/>
        <assignedPerson>
          <name>
            <given>Robert</given>
            <family>Jones</family>
          </name>
        </assignedPerson>
      </assignedEntity>
    </performer>
  </procedure>
</entry>
</section>`

const socialHistorySectionXML = `<section>
<templateId root="2.16.840.1.113883.10.20.22.2.17"/>
<code code="29762-2" displayName="Social History"/>
<entry typeCode="DRIV">
  <observation classCode="OBS" moodCode="EVN">
    <templateId root="2.16.840.1.113883.10.20.22.4.38"/>
    <code code="72166-2" displayName="Tobacco smoking status"/>
    <effectiveTime value="20240101"/>
    <value xsi:type="CD" code="266919005" displayName="Never smoked tobacco"
           codeSystem="2.16.840.1.113883.6.96"
           xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"/>
  </observation>
</entry>
</section>`
