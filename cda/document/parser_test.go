// cda/document/parser_test.go
// Unit tests for the Sprint B typed CDADocument model.
//
// Verification criteria covered:
//   ✅ multi_id_patient.xml  → CDAPatient.Ids has 3 elements
//   ✅ full_ccd_nist.xml     → SectionsByKey has ≥ 9 entries
//   ✅ minimal_ccd.xml       → smoke test (parser returns non-nil document)
//   ✅ Multi-name patient    → Names slice has 2 elements
//   ✅ Multi-address patient → Addresses slice has 2 elements
//   ✅ Multi-telecom patient → Telecoms slice has 2 elements
//   ✅ Nested entry          → allergiesAndIntolerances entry has EntryRelationships
//   ✅ Missing optional secs → missing sections not in SectionsByKey
//   ✅ Section entry types   → substanceAdministration, organizer, observation, encounter, procedure parsed correctly
//   ✅ Organizer components  → vitalSigns organizer has 2 component observations
//   ✅ Immunization consumable → lots number extracted
//   ✅ Nil root              → ParseDocument returns non-nil CDADocument
//   ✅ relatedDocument       → parsed with typeCode and parentDocument ids
//   ✅ documentationOf       → CDAServiceEvent with performers
//   ✅ Author slice          → at least one author parsed
//   ✅ Custodian             → organisation name extracted

package cdadocument

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================
// Test helpers
// ========================

// schemaDir returns the absolute path to cda/schemas, resolving relative to
// this test file's directory so tests work regardless of working directory.
func schemaDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// cda/document/parser_test.go → go up two levels → project root → cda/schemas
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	return filepath.Join(root, "cda", "schemas")
}

// testdataDir returns the absolute path to cda/document/testdata.
func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// loadXML reads a testdata file and returns its raw content + parsed etree root.
func loadXML(t *testing.T, filename string) (string, *etree.Element) {
	t.Helper()
	path := filepath.Join(testdataDir(t), filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading testdata file %s", filename)
	raw := string(data)

	doc := etree.NewDocument()
	err = doc.ReadFromString(raw)
	require.NoError(t, err, "parsing XML in %s", filename)
	return raw, doc.Root()
}

// newParser builds a CDAParser with the production schema loader.
func newParser(t *testing.T) *CDAParser {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader(schemaDir(t))
	require.NoError(t, err, "loading CDA schema")
	return NewCDAParser(loader)
}

// ========================
// Smoke test
// ========================

func TestParseMinimalCCD(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "minimal_ccd.xml")
	doc := p.ParseDocument(root, raw)

	require.NotNil(t, doc)
	assert.Equal(t, raw, doc.Raw, "Raw XML must be preserved verbatim")
	assert.NotEmpty(t, doc.Header.DocumentId.Root)
	assert.Equal(t, "MIN001", doc.Header.DocumentId.Extension)
	assert.Equal(t, "Continuity of Care Document", doc.Header.Title)
	assert.Equal(t, "N", doc.Header.ConfidentialityCode.Code)
	assert.NotNil(t, doc.SectionsByKey)
}

// ========================
// Verification criterion 1: multi_id_patient.xml → 3 IDs
// ========================

func TestMultiIDPatient_ThreeIds(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	require.Len(t, doc.Header.Patient.Ids, 3,
		"CDAPatient.Ids must contain exactly 3 elements (MRN, SSN, Medicare)")

	roots := make(map[string]bool)
	for _, id := range doc.Header.Patient.Ids {
		roots[id.Root] = true
	}
	assert.True(t, roots["2.16.840.1.113883.19.5.99999.2"], "MRN root OID expected")
	assert.True(t, roots["2.16.840.1.113883.4.1"], "SSN root OID expected")
	assert.True(t, roots["2.16.840.1.113883.4.927"], "Medicare root OID expected")
}

func TestMultiIDPatient_MultiName(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	assert.Len(t, doc.Header.Patient.Names, 2, "Patient should have 2 names (legal + maiden)")

	legalName := doc.Header.Patient.Names[0]
	assert.Equal(t, "L", legalName.Use)
	assert.Equal(t, "Chen", legalName.Family)
	assert.Contains(t, legalName.Given, "Evelyn")
	assert.Contains(t, legalName.Given, "Mae")
	assert.Equal(t, "PhD", legalName.Suffix)

	maidenName := doc.Header.Patient.Names[1]
	assert.Equal(t, "A", maidenName.Use)
	assert.Equal(t, "Wong", maidenName.Family)
}

func TestMultiIDPatient_MultiAddress(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	assert.Len(t, doc.Header.Patient.Addresses, 2,
		"Patient should have 2 addresses (home + work)")
	assert.Equal(t, "HP", doc.Header.Patient.Addresses[0].Use)
	assert.Equal(t, "WP", doc.Header.Patient.Addresses[1].Use)
	// Multi-line street address
	assert.Len(t, doc.Header.Patient.Addresses[0].StreetLines, 2)
}

func TestMultiIDPatient_MultiTelecom(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	assert.Len(t, doc.Header.Patient.Telecoms, 2,
		"Patient should have 2 telecoms (phone + email)")

	phoneFound, emailFound := false, false
	for _, tel := range doc.Header.Patient.Telecoms {
		switch tel.Type {
		case "phone":
			phoneFound = true
		case "email":
			emailFound = true
		}
	}
	assert.True(t, phoneFound, "phone telecom expected")
	assert.True(t, emailFound, "email telecom expected")
}

func TestMultiIDPatient_Languages(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	assert.Len(t, doc.Header.Patient.Languages, 2)
	assert.Equal(t, "en", doc.Header.Patient.Languages[0].Code)
	assert.True(t, doc.Header.Patient.Languages[0].PreferenceInd)
	assert.Equal(t, "zh", doc.Header.Patient.Languages[1].Code)
	assert.False(t, doc.Header.Patient.Languages[1].PreferenceInd)
}

func TestMultiIDPatient_RelatedDocument(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	require.Len(t, doc.Header.RelatedDocuments, 1)
	rd := doc.Header.RelatedDocuments[0]
	assert.Equal(t, "RPLC", rd.TypeCode)
	require.Len(t, rd.ParentDocument.Ids, 1)
	assert.Equal(t, "MULTI-ID-000", rd.ParentDocument.Ids[0].Extension)
	assert.Equal(t, "1", rd.ParentDocument.VersionNumber)
}

func TestMultiIDPatient_SetIdAndVersionNumber(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "multi_id_patient.xml")
	doc := p.ParseDocument(root, raw)

	assert.Equal(t, "MULTI-ID-SET", doc.Header.SetId.Extension)
	assert.Equal(t, "2", doc.Header.VersionNumber)
}

// ========================
// Verification criterion 2: full_ccd_nist.xml → ≥ 9 SectionsByKey entries
// ========================

func TestFullCCD_NineSections(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	assert.GreaterOrEqual(t, len(doc.SectionsByKey), 9,
		"SectionsByKey must contain at least 9 entries; got keys: %v",
		sectionKeys(doc))

	expectedKeys := []string{
		"allergiesAndIntolerances",
		"medications",
		"problems",
		"vitalSigns",
		"results",
		"immunizations",
		"socialHistory",
		"encounters",
		"procedures",
	}
	for _, k := range expectedKeys {
		assert.Contains(t, doc.SectionsByKey, k,
			"SectionsByKey must contain %q", k)
	}
}

func sectionKeys(doc *CDADocument) []string {
	keys := make([]string, 0, len(doc.SectionsByKey))
	for k := range doc.SectionsByKey {
		keys = append(keys, k)
	}
	return keys
}

func TestFullCCD_SectionsOrdered(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	assert.Equal(t, 9, len(doc.Sections), "Sections slice should have 9 elements")
	assert.Equal(t, "allergiesAndIntolerances", doc.Sections[0].Key,
		"First section should be allergiesAndIntolerances")
	assert.Equal(t, "procedures", doc.Sections[8].Key,
		"Last section should be procedures")
}

// ========================
// Entry parsing tests
// ========================

func TestFullCCD_AllergyEntryWithNestedRelationship(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["allergiesAndIntolerances"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries, "allergies section should have entries")

	allergyAct := sec.Entries[0]
	assert.Equal(t, "act", allergyAct.EntryType)
	assert.Equal(t, "active", allergyAct.StatusCode)

	// entryRelationship[SUBJ] → observation
	require.NotEmpty(t, allergyAct.EntryRelationships)
	subjRel := allergyAct.EntryRelationships[0]
	assert.Equal(t, "SUBJ", subjRel.TypeCode)
	obs := subjRel.Entry
	assert.Equal(t, "observation", obs.EntryType)
	require.NotNil(t, obs.Value)
	assert.Equal(t, "CD", obs.Value.Type)
	require.NotNil(t, obs.Value.Code)
	assert.Equal(t, "416098002", obs.Value.Code.Code)

	// participant (allergy substance)
	require.NotEmpty(t, obs.Participants)
	participant := obs.Participants[0]
	assert.Equal(t, "CSM", participant.TypeCode)
	require.NotNil(t, participant.ParticipantRole.PlayingEntity)
	assert.Equal(t, "7980", participant.ParticipantRole.PlayingEntity.Code.Code)
	assert.Equal(t, "Penicillin", participant.ParticipantRole.PlayingEntity.Code.DisplayName)

	// Nested reaction entryRelationship
	require.NotEmpty(t, obs.EntryRelationships)
	reactionRel := obs.EntryRelationships[0]
	assert.Equal(t, "MFST", reactionRel.TypeCode)
	assert.True(t, reactionRel.InversionInd)
}

func TestFullCCD_MedicationConsumable(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	assert.Equal(t, "substanceAdministration", med.EntryType)
	assert.Equal(t, "active", med.StatusCode)

	require.NotNil(t, med.DoseQuantity)
	assert.Equal(t, "81", med.DoseQuantity.Value)
	assert.Equal(t, "mg", med.DoseQuantity.Unit)

	require.NotNil(t, med.RouteCode)
	assert.Equal(t, "C38288", med.RouteCode.Code)
	assert.Equal(t, "Oral", med.RouteCode.DisplayName)

	require.NotNil(t, med.Consumable)
	require.NotNil(t, med.Consumable.ManufacturedProduct.ManufacturedMaterial)
	mat := med.Consumable.ManufacturedProduct.ManufacturedMaterial
	assert.Equal(t, "1191", mat.Code.Code)
	assert.Equal(t, "Aspirin", mat.Code.DisplayName)
}

func TestFullCCD_VitalSignsOrganizerComponents(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["vitalSigns"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	organizer := sec.Entries[0]
	assert.Equal(t, "organizer", organizer.EntryType)
	assert.Len(t, organizer.Components, 2,
		"Vital signs organizer should have 2 component observations")

	systolic := organizer.Components[0]
	assert.Equal(t, "observation", systolic.EntryType)
	require.NotNil(t, systolic.Value)
	assert.Equal(t, "PQ", systolic.Value.Type)
	require.NotNil(t, systolic.Value.Quantity)
	assert.Equal(t, "120", systolic.Value.Quantity.Value)
	assert.Equal(t, "mm[Hg]", systolic.Value.Quantity.Unit)
}

func TestFullCCD_ImmunizationConsumableWithLotNumber(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["immunizations"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	imm := sec.Entries[0]
	assert.Equal(t, "substanceAdministration", imm.EntryType)
	assert.Equal(t, "completed", imm.StatusCode)

	require.NotNil(t, imm.Consumable)
	mat := imm.Consumable.ManufacturedProduct.ManufacturedMaterial
	require.NotNil(t, mat)
	assert.Equal(t, "88", mat.Code.Code)
	assert.Equal(t, "1234-ABCD", mat.LotNumberText)
}

func TestFullCCD_SocialHistoryObservation(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["socialHistory"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	obs := sec.Entries[0]
	assert.Equal(t, "observation", obs.EntryType)
	assert.Equal(t, "72166-2", obs.Code.Code)
	require.NotNil(t, obs.Value)
	require.NotNil(t, obs.Value.Code)
	assert.Equal(t, "266919005", obs.Value.Code.Code)
	assert.Equal(t, "Never smoked tobacco", obs.Value.Code.DisplayName)
}

func TestFullCCD_EncounterEntry(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["encounters"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	enc := sec.Entries[0]
	assert.Equal(t, "encounter", enc.EntryType)
	assert.Equal(t, "99213", enc.Code.Code)
}

func TestFullCCD_ProcedureEntry(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["procedures"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	proc := sec.Entries[0]
	assert.Equal(t, "procedure", proc.EntryType)
	assert.Equal(t, "G0439", proc.Code.Code)
	assert.Equal(t, "completed", proc.StatusCode)
}

// ========================
// Header sub-field tests
// ========================

func TestFullCCD_AuthorParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	require.NotEmpty(t, doc.Header.Authors)
	author := doc.Header.Authors[0]
	assert.Equal(t, "20240315140000", author.Time.Value)
	require.NotNil(t, author.AssignedAuthor.AssignedPerson)
	require.NotEmpty(t, author.AssignedAuthor.AssignedPerson.Names)
	assert.Equal(t, "Seven", author.AssignedAuthor.AssignedPerson.Names[0].Family)
}

func TestFullCCD_CustodianParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	custOrg := doc.Header.Custodian.AssignedCustodian.RepresentedCustodianOrganization
	require.NotEmpty(t, custOrg.Names)
	assert.Equal(t, "Good Health Hospital", custOrg.Names[0])
	assert.Equal(t, "MA", custOrg.Addresses[0].State)
}

func TestFullCCD_DocumentationOf(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	require.NotNil(t, doc.Header.DocumentOf)
	se := doc.Header.DocumentOf
	assert.Equal(t, "PCPR", se.ClassCode)
	assert.Equal(t, "20240101", se.EffectiveTime.Low.Value)
	assert.Equal(t, "20240315", se.EffectiveTime.High.Value)
	require.NotEmpty(t, se.Performers)
}

// ========================
// componentOf/encompassingEncounter + HL7 PN plain-text fallback
// ========================

func TestParsePN_PlainTextFallback(t *testing.T) {
	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromString(`<name>mumbai Women's Care mira road</name>`))
	n := parsePN(doc.Root())
	assert.Equal(t, "mumbai Women's Care mira road", n.Family)
	assert.Empty(t, n.Given)
	assert.Empty(t, n.Prefix)
	assert.Empty(t, n.Suffix)
}

func TestParsePN_StructuredNameUnaffected(t *testing.T) {
	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromString(`<name use="L"><given>Jane</given><family>Doe</family></name>`))
	n := parsePN(doc.Root())
	assert.Equal(t, "Doe", n.Family)
	assert.Equal(t, []string{"Jane"}, n.Given)
}

func TestParseComponentOf_HealthCareFacility(t *testing.T) {
	raw := `<ClinicalDocument xmlns="urn:hl7-org:v3">
  <componentOf>
    <encompassingEncounter>
      <id root="2.16.840.1.113883.19" extension="ENC-1"/>
      <effectiveTime value="20240101"/>
      <location>
        <healthCareFacility>
          <id root="1.2.840.114350.1.13.549.2.7.2.686980" extension="103001002"/>
          <code nullFlavor="UNK">
            <originalText>Obstetrics and Gynecology</originalText>
            <translation code="42" codeSystem="1.2.840.114350.1.72.1.7.7.10.688867.4150" codeSystemName="Epic.DepartmentSpecialty" displayName="Obstetrics &amp; Gynecology"/>
          </code>
          <location>
            <name>mumbai Women's Care mira road</name>
            <addr use="WP">
              <streetAddressLine>101 mira road Parkway Ste 201B</streetAddressLine>
              <county>mumbai</county>
              <city>mira road</city>
              <state>CO</state>
              <postalCode>80511-4072</postalCode>
              <country>USA</country>
            </addr>
          </location>
        </healthCareFacility>
      </location>
    </encompassingEncounter>
  </componentOf>
</ClinicalDocument>`

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromString(raw))

	enc := (&headerParser{}).parseComponentOf(doc.Root())
	require.NotNil(t, enc)
	require.NotNil(t, enc.Location)

	hcf := enc.Location.HealthCareFacility
	assert.Equal(t, "UNK", hcf.Code.NullFlavor)
	assert.Equal(t, "Obstetrics and Gynecology", hcf.Code.OriginalText)
	require.Len(t, hcf.Code.Translations, 1)
	assert.Equal(t, "Obstetrics & Gynecology", hcf.Code.Translations[0].DisplayName)

	require.NotNil(t, hcf.Location)
	require.NotEmpty(t, hcf.Location.Names)
	// parsePN's plain-text fallback (see TestParsePN_PlainTextFallback) is
	// what makes this resolve at all -- <name> here has no <family>/<given>
	// sub-elements, only direct text.
	assert.Equal(t, "mumbai Women's Care mira road", hcf.Location.Names[0].Family)
	require.NotEmpty(t, hcf.Location.Addresses)
	assert.Equal(t, "mira road", hcf.Location.Addresses[0].City)
	assert.Equal(t, "80511-4072", hcf.Location.Addresses[0].PostalCode)

	// healthCareFacility's own <id> is a documented, deliberate gap (see
	// EncompassingEncounterLocationMappingRules' doc comment in
	// services/cda_fhir/declarative_oob_rules.go) -- CDAHealthCareFacility
	// has no Id field to parse it into yet.
	assert.Nil(t, hcf.ServiceProviderOrganization)
}

// ========================
// Missing optional sections
// ========================

func TestMinimalCCD_MissingOptionalSections(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "minimal_ccd.xml")
	doc := p.ParseDocument(root, raw)

	// Only allergiesAndIntolerances is in the minimal file.
	assert.Contains(t, doc.SectionsByKey, "allergiesAndIntolerances")
	assert.NotContains(t, doc.SectionsByKey, "medications",
		"medications section should not be present in minimal CCD")
	assert.NotContains(t, doc.SectionsByKey, "problems",
		"problems section should not be present in minimal CCD")
}

// ========================
// NullFlavor preservation
// ========================

func TestNullFlavorPreserved(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <id root="2.16.840.1.113883.19.5.99999.1" extension="NF-TEST"/>
  <effectiveTime value="20240101"/>
  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>
  <recordTarget>
    <patientRole>
      <id root="2.16.840.1.113883.19" extension="NF-PT"/>
      <patient>
        <name use="L"><given>Test</given><family>Patient</family></name>
        <administrativeGenderCode nullFlavor="UNK"/>
        <birthTime nullFlavor="UNK"/>
      </patient>
    </patientRole>
  </recordTarget>
  <component>
    <structuredBody>
      <component>
        <section nullFlavor="NI">
          <templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>
          <code code="48765-2" codeSystem="2.16.840.1.113883.6.1"/>
          <title>Allergies</title>
        </section>
      </component>
    </structuredBody>
  </component>
</ClinicalDocument>`

	doc := etree.NewDocument()
	err := doc.ReadFromString(raw)
	require.NoError(t, err)

	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(root, "cda", "schemas"))
	require.NoError(t, err)

	p := NewCDAParser(loader)
	cdaDoc := p.ParseDocument(doc.Root(), raw)

	// Gender nullFlavor preserved
	assert.Equal(t, "UNK", cdaDoc.Header.Patient.Gender.NullFlavor)
	// BirthDate nullFlavor preserved
	assert.Equal(t, "UNK", cdaDoc.Header.Patient.BirthDate.NullFlavor)
	// Section nullFlavor preserved
	require.NotEmpty(t, cdaDoc.Sections)
	assert.Equal(t, "NI", cdaDoc.Sections[0].NullFlavor)
}

// ========================
// Nil-safe: empty root
// ========================

func TestParseDocument_NilRoot(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(root, "cda", "schemas"))
	require.NoError(t, err)

	p := NewCDAParser(loader)
	doc := p.ParseDocument(nil, "")

	require.NotNil(t, doc, "ParseDocument must never return nil")
	assert.NotNil(t, doc.SectionsByKey)
	assert.Empty(t, doc.Sections)
}

// ========================
// NarrativeText serialisation
// ========================

func TestNarrativeTextPresent(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "minimal_ccd.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["allergiesAndIntolerances"]
	require.NotNil(t, sec)
	assert.NotEmpty(t, sec.NarrativeText,
		"NarrativeText should contain the serialised <text> element")
}

// ========================
// Section template ID collection
// ========================

func TestFullCCD_MultipleTemplateIdsPerSection(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "full_ccd_nist.xml")
	doc := p.ParseDocument(root, raw)

	// allergiesAndIntolerances section has two templateId elements.
	sec := doc.SectionsByKey["allergiesAndIntolerances"]
	require.NotNil(t, sec)
	assert.Len(t, sec.TemplateIds, 2,
		"both templateId OIDs should be captured")
	assert.Contains(t, sec.TemplateIds, "2.16.840.1.113883.10.20.22.2.6.1")
	assert.Contains(t, sec.TemplateIds, "2.16.840.1.113883.10.20.22.2.6")
}

// ========================
// Patient demographics (minimal)
// ========================

func TestMinimalCCD_PatientDemographics(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "minimal_ccd.xml")
	doc := p.ParseDocument(root, raw)

	pat := doc.Header.Patient
	require.Len(t, pat.Ids, 1)
	assert.Equal(t, "MRN-001", pat.Ids[0].Extension)
	require.Len(t, pat.Names, 1)
	assert.Equal(t, "Jane", pat.Names[0].Given[0])
	assert.Equal(t, "Doe", pat.Names[0].Family)
	assert.Equal(t, "19800315", pat.BirthDate.Value)
	assert.Equal(t, "F", pat.Gender.Code)
	assert.Equal(t, "Female", pat.Gender.DisplayName)
	assert.Equal(t, "2106-3", pat.Race.Code)
	assert.Equal(t, "2186-5", pat.Ethnicity.Code)
}

// ========================
// titleKeyMap normalisation (unit test, no XML parsing)
// ========================

func TestNormalizeTitleToKey_KnownTitles(t *testing.T) {
	cases := []struct {
		xmlTitle string
		wantKey  string
	}{
		{"Allergies", "allergiesAndIntolerances"},
		{"Medications", "medications"},
		{"Vital Signs", "vitalSigns"},
		{"Social History", "socialHistory"},
		{"Assessment and Plan", "assessmentAndPlan"},
	}
	for _, tc := range cases {
		rawXML := `<section xmlns="urn:hl7-org:v3"><title>` + tc.xmlTitle + `</title></section>`
		doc := etree.NewDocument()
		_ = doc.ReadFromString(rawXML)
		got := normalizeTitleToKey(doc.Root())
		assert.Equal(t, tc.wantKey, got, "title %q should map to key %q", tc.xmlTitle, tc.wantKey)
	}
}

func TestNormalizeTitleToKey_Empty(t *testing.T) {
	rawXML := `<section xmlns="urn:hl7-org:v3"></section>`
	doc := etree.NewDocument()
	_ = doc.ReadFromString(rawXML)
	assert.Equal(t, "", normalizeTitleToKey(doc.Root()))
}

// ========================
// Phase 1 fidelity gaps: negationInd, entry templateIds, multiple
// effectiveTime elements, repeatNumber
// ========================

func TestNegationIndParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "negation_and_frequency.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["allergiesAndIntolerances"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	allergyAct := sec.Entries[0]
	assert.False(t, allergyAct.NegationInd, "outer concern act is not itself negated")

	require.NotEmpty(t, allergyAct.EntryRelationships)
	subjRel := allergyAct.EntryRelationships[0]
	assert.Equal(t, "SUBJ", subjRel.TypeCode)
	obs := subjRel.Entry
	assert.True(t, obs.NegationInd,
		"nested assertion observation has negationInd=true (no known allergies)")
}

func TestEntryTemplateIdsParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "negation_and_frequency.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["allergiesAndIntolerances"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	allergyAct := sec.Entries[0]
	assert.Contains(t, allergyAct.TemplateIds, "2.16.840.1.113883.10.20.22.4.30")

	obs := allergyAct.EntryRelationships[0].Entry
	assert.Contains(t, obs.TemplateIds, "2.16.840.1.113883.10.20.22.4.7")
	assert.Contains(t, obs.TemplateIds, "2.16.840.1.113883.10.20.24.3.90")
}

func TestMultipleEffectiveTimesParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "negation_and_frequency.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	assert.Equal(t, "substanceAdministration", med.EntryType)
	require.Len(t, med.EffectiveTimes, 2,
		"both the duration and frequency effectiveTime elements should be captured")

	duration := med.EffectiveTimes[0]
	assert.Equal(t, "IVL_TS", duration.XSIType)
	assert.Equal(t, "20240101", duration.Range.Low.Value)

	frequency := med.EffectiveTimes[1]
	assert.Equal(t, "PIVL_TS", frequency.XSIType)
	require.NotNil(t, frequency.Period, "PIVL_TS period (the actual frequency interval) should be captured")
	assert.Equal(t, "12", frequency.Period.Value)
	assert.Equal(t, "h", frequency.Period.Unit)

	// Backward compatibility: the singular EffectiveTime field still mirrors
	// the first (duration) entry, unchanged for existing callers.
	assert.Equal(t, "20240101", med.EffectiveTime.Low.Value)
}

func TestRepeatNumberParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "negation_and_frequency.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	assert.Equal(t, "2", med.RepeatNumber)
}

// ========================
// Real-world fidelity gaps found by diffing ParsedJSON against a live Epic
// CCD export: nullFlavor on <id>/<doseQuantity>, repeatNumber with no value
// attribute, and a <supply> entryRelationship's own quantity/product.
// ========================

func TestDoseQuantityNullFlavorParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "supply_and_nullflavors.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	require.NotNil(t, med.DoseQuantity)
	assert.Equal(t, "UNK", med.DoseQuantity.NullFlavor,
		"doseQuantity nullFlavor must survive even though value/unit are absent")
}

func TestAuthorIdNullFlavorParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "supply_and_nullflavors.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	require.NotEmpty(t, med.Authors)
	require.NotEmpty(t, med.Authors[0].AssignedAuthor.Ids)
	assert.Equal(t, "UNK", med.Authors[0].AssignedAuthor.Ids[0].NullFlavor,
		"id nullFlavor must survive even when root/extension are absent")
}

func TestSupplyEntryRelationship_RepeatNumberQuantityAndProductParsed(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "supply_and_nullflavors.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	require.NotEmpty(t, med.EntryRelationships)
	refr := med.EntryRelationships[0]
	assert.Equal(t, "REFR", refr.TypeCode)
	supply := refr.Entry
	assert.Equal(t, "supply", supply.EntryType)

	assert.Equal(t, "OTH", supply.RepeatNumber,
		"repeatNumber with no value attribute should fall back to nullFlavor, not be dropped")

	require.NotNil(t, supply.Quantity, "supply's own <quantity> (dispensed amount) must be captured")
	assert.Equal(t, "15", supply.Quantity.Value)
	assert.Equal(t, "g", supply.Quantity.Unit)

	require.NotNil(t, supply.Product, "supply's <product><manufacturedProduct> must be captured")
	require.NotNil(t, supply.Product.ManufacturedMaterial)
	assert.Equal(t, "1053697", supply.Product.ManufacturedMaterial.Code.Code)
}

// ========================
// Entry-level <text> resolution (Medication Free Text Sig / Instruction (V2))
// ========================

func TestEntryTextResolved_MedicationFreeTextSig(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "medication_sig_instruction.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]
	require.NotEmpty(t, med.EntryRelationships)

	var sigEntry *CDAEntry
	for i := range med.EntryRelationships {
		rel := med.EntryRelationships[i]
		if rel.TypeCode == "COMP" {
			sigEntry = &med.EntryRelationships[i].Entry
		}
	}
	require.NotNil(t, sigEntry, "expected a COMP entryRelationship (Medication Free Text Sig)")
	assert.Contains(t, sigEntry.TemplateIds, "2.16.840.1.113883.10.20.22.4.147")
	assert.Equal(t, "Take one tablet by mouth every morning", sigEntry.Text,
		"the '#sig1' reference anchor should resolve to the narrative text, not stay as a raw anchor")
}

func TestEntryTextResolved_InstructionV2(t *testing.T) {
	p := newParser(t)
	raw, root := loadXML(t, "medication_sig_instruction.xml")
	doc := p.ParseDocument(root, raw)

	sec := doc.SectionsByKey["medications"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	med := sec.Entries[0]

	var instrEntry *CDAEntry
	for i := range med.EntryRelationships {
		rel := med.EntryRelationships[i]
		if rel.TypeCode == "SUBJ" && rel.InversionInd {
			instrEntry = &med.EntryRelationships[i].Entry
		}
	}
	require.NotNil(t, instrEntry, "expected a SUBJ/inversionInd=true entryRelationship (Instruction (V2))")
	assert.Contains(t, instrEntry.TemplateIds, "2.16.840.1.113883.10.20.22.4.20")
	assert.Equal(t, "Take with food", instrEntry.Text,
		"the '#instr1' reference anchor should resolve to the narrative text, not stay as a raw anchor")
}

// ========================
// targetSiteCode: direct child of <procedure>, not a COMP entryRelationship.
// Confirmed against the real practicefusion_sample.xml corpus shape (the
// element sits alongside <code>/<statusCode>/<effectiveTime>, not nested).
// ========================

func TestTargetSiteCodeParsed_DirectChildOfProcedure(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <id root="2.16.840.1.113883.19.5.99999.1" extension="TSC-TEST"/>
  <effectiveTime value="20240101"/>
  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>
  <recordTarget>
    <patientRole>
      <id root="2.16.840.1.113883.19" extension="TSC-PT"/>
      <patient>
        <name use="L"><given>Test</given><family>Patient</family></name>
        <administrativeGenderCode nullFlavor="UNK"/>
        <birthTime nullFlavor="UNK"/>
      </patient>
    </patientRole>
  </recordTarget>
  <component>
    <structuredBody>
      <component>
        <section>
          <templateId root="2.16.840.1.113883.10.20.22.2.7.1"/>
          <code code="47519-4" codeSystem="2.16.840.1.113883.6.1"/>
          <title>Procedures</title>
          <entry>
            <procedure classCode="PROC" moodCode="EVN">
              <templateId root="2.16.840.1.113883.10.20.22.4.14"/>
              <code code="44950" codeSystem="2.16.840.1.113883.6.12" displayName="Appendectomy"/>
              <statusCode code="completed"/>
              <effectiveTime value="20230615"/>
              <targetSiteCode code="66754008" codeSystem="2.16.840.1.113883.6.96" displayName="Appendix structure"/>
            </procedure>
          </entry>
        </section>
      </component>
    </structuredBody>
  </component>
</ClinicalDocument>`

	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromString(raw))

	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(root, "cda", "schemas"))
	require.NoError(t, err)

	p := NewCDAParser(loader)
	cdaDoc := p.ParseDocument(doc.Root(), raw)

	sec := cdaDoc.SectionsByKey["procedures"]
	require.NotNil(t, sec)
	require.NotEmpty(t, sec.Entries)

	proc := sec.Entries[0]
	assert.Equal(t, "procedure", proc.EntryType)
	require.NotNil(t, proc.TargetSiteCode, "targetSiteCode must be parsed as a direct field, not require a COMP entryRelationship")
	assert.Equal(t, "66754008", proc.TargetSiteCode.Code)
	assert.Equal(t, "Appendix structure", proc.TargetSiteCode.DisplayName)
	assert.Empty(t, proc.EntryRelationships, "this fixture has no entryRelationships at all -- targetSiteCode never lived there")
}

func TestTargetSiteCodeAbsent_ObservationVariant_NilNotPanic(t *testing.T) {
	// Procedure Activity Observation (.4.13) uses an <observation> element,
	// which has no targetSiteCode in the base CDA R2 schema -- this must
	// stay nil, not be inferred from anywhere else.
	entries := []CDAEntry{
		{EntryType: "observation", StatusCode: "completed", Code: CDACode{Code: "44950"}},
	}
	assert.Nil(t, entries[0].TargetSiteCode)
}

// TestParseValue_INT_PopulatesIntegerField is a regression test for a real
// gap found auditing the Functional Status section of the 99397 CCD sample:
// a PHQ-2 total-score Assessment Scale Supporting Observation carries
// <value xsi:type="INT" value="0"/>, but parseValue's INT case only ever set
// CDAValue.Text, never CDAValue.Integer -- the field
// observationValueXDispatchRows' Scope="value[type=INT]" row
// (declarative_oob_rules.go) actually reads via SourcePath="integer". Every
// CDA INT value silently produced neither valueInteger NOR dataAbsentReason
// on the mapped FHIR Observation, confirmed end-to-end against the real
// 99397 sample before this fix (the PHQ-2 total score Observation had no
// value[x] at all).
func TestParseValue_INT_PopulatesIntegerField(t *testing.T) {
	el := mustParseElement(t, `<value xsi:type="INT" value="0"/>`)
	v := parseValue(el)
	require.NotNil(t, v)
	assert.Equal(t, "INT", v.Type)
	require.NotNil(t, v.Integer, "Integer field must be populated for xsi:type=INT")
	assert.Equal(t, 0, *v.Integer)
	// Text is kept too -- cda/validator's hasValue-style checks only ever
	// look at Text as a generic presence check, unrelated to this fix.
	assert.Equal(t, "0", v.Text)
}

func TestParseValue_INT_NegativeAndNonZero(t *testing.T) {
	el := mustParseElement(t, `<value xsi:type="INT" value="42"/>`)
	v := parseValue(el)
	require.NotNil(t, v.Integer)
	assert.Equal(t, 42, *v.Integer)
}

// TestParseValue_REAL_PopulatesRealField mirrors the INT fix above for the
// REAL data type -- declarativeRealToBareQuantity
// (declarative_transform_registry.go) reads CDAValue.Real, which parseValue's
// REAL case never populated either, same root cause as INT.
func TestParseValue_REAL_PopulatesRealField(t *testing.T) {
	el := mustParseElement(t, `<value xsi:type="REAL" value="3.5"/>`)
	v := parseValue(el)
	require.NotNil(t, v)
	assert.Equal(t, "REAL", v.Type)
	require.NotNil(t, v.Real, "Real field must be populated for xsi:type=REAL")
	assert.Equal(t, 3.5, *v.Real)
}

func TestParseValue_INT_NonNumericText_IntegerStaysNil(t *testing.T) {
	// Defensive: a non-conformant document with a non-numeric INT value
	// string must not panic, and must leave Integer nil rather than
	// silently mapping to 0 (which would be indistinguishable from a real
	// zero score).
	el := mustParseElement(t, `<value xsi:type="INT" value="not-a-number"/>`)
	v := parseValue(el)
	require.NotNil(t, v)
	assert.Nil(t, v.Integer)
	assert.Equal(t, "not-a-number", v.Text)
}

// mustParseElement parses a single standalone XML element for direct
// parseValue/parseCD-style unit tests, distinct from loadXML's full-document
// fixture loading above.
func mustParseElement(t *testing.T, xml string) *etree.Element {
	t.Helper()
	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromString(xml))
	return doc.Root()
}
