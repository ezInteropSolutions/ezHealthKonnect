// services/parsers/cda/cda_parser_service_test.go
// Phase 3: verifies the typed CDADocument tree is exposed under
// ParsedJSON["document"] and survives a real JSON marshal/unmarshal round
// trip — i.e. it's reachable as plain JSON by any consumer, not just the
// in-process Go object handed to cda.to_fhir.

package cdaparser

import (
	"encoding/json"
	"testing"
)

const negatedAllergyCCD = `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <realmCode code="US"/>
  <typeId root="2.16.840.1.113883.1.3" extension="POCD_HD000040"/>
  <templateId root="2.16.840.1.113883.10.20.22.1.1"/>
  <id root="2.16.840.1.113883.19.5.99999.1" extension="DOC001"/>
  <code code="34133-9" codeSystem="2.16.840.1.113883.6.1" displayName="Summarization of Episode Note"/>
  <title>Continuity of Care Document</title>
  <effectiveTime value="20240101120000"/>
  <recordTarget>
    <patientRole>
      <id root="2.16.840.1.113883.19.5.99999.2" extension="MRN-003"/>
      <patient>
        <name use="L"><given>Pat</given><family>Test</family></name>
        <administrativeGenderCode code="F" codeSystem="2.16.840.1.113883.5.1"/>
        <birthTime value="19900101"/>
      </patient>
    </patientRole>
  </recordTarget>
  <component>
    <structuredBody>
      <component>
        <section>
          <templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>
          <code code="48765-2" codeSystem="2.16.840.1.113883.6.1" displayName="Allergies and Adverse Reactions"/>
          <title>Allergies</title>
          <text><list><item>No known active allergies</item></list></text>
          <entry typeCode="DRIV">
            <act classCode="ACT" moodCode="EVN">
              <id root="36e3e930-7b14-11db-9fe1-0800200c9a66"/>
              <code code="CONC" codeSystem="2.16.840.1.113883.5.6"/>
              <statusCode code="active"/>
              <entryRelationship typeCode="SUBJ">
                <observation classCode="OBS" moodCode="EVN" negationInd="true">
                  <id root="4adc1020-7b14-11db-9fe1-0800200c9a66"/>
                  <code code="ASSERTION" codeSystem="2.16.840.1.113883.5.4"/>
                  <statusCode code="completed"/>
                  <value xsi:type="CD" code="419199007" displayName="Allergy to substance (disorder)"
                         codeSystem="2.16.840.1.113883.6.96" codeSystemName="SNOMED CT"/>
                </observation>
              </entryRelationship>
            </act>
          </entry>
        </section>
      </component>
    </structuredBody>
  </component>
</ClinicalDocument>`

func TestParse_DocumentKeyPresent(t *testing.T) {
	svc, err := NewFromSchemaDir("../../../cda/schemas")
	if err != nil {
		t.Fatalf("NewFromSchemaDir: %v", err)
	}

	result := svc.Parse(negatedAllergyCCD)
	if !result.Success {
		t.Fatalf("Parse failed: %s", result.Error)
	}

	if _, ok := result.ParsedJSON["document"]; !ok {
		t.Fatal(`expected ParsedJSON["document"] to be present`)
	}
}

func TestParse_DocumentKeyRoundTripsAsJSON(t *testing.T) {
	svc, err := NewFromSchemaDir("../../../cda/schemas")
	if err != nil {
		t.Fatalf("NewFromSchemaDir: %v", err)
	}

	result := svc.Parse(negatedAllergyCCD)
	if !result.Success {
		t.Fatalf("Parse failed: %s", result.Error)
	}

	// Marshal the whole ParsedJSON the same way storage/HTTP responses would,
	// then unmarshal into a generic map — proving "document" is reachable as
	// plain JSON, not just a live Go object only cda.to_fhir can see.
	raw, err := json.Marshal(result.ParsedJSON)
	if err != nil {
		t.Fatalf("marshal ParsedJSON: %v", err)
	}

	var roundTripped map[string]interface{}
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal ParsedJSON: %v", err)
	}

	doc, ok := roundTripped["document"].(map[string]interface{})
	if !ok {
		t.Fatal(`expected "document" to unmarshal as a JSON object`)
	}
	sections, ok := doc["sectionsByKey"].(map[string]interface{})
	if !ok {
		t.Fatal(`expected document.sectionsByKey to be a JSON object`)
	}
	allergies, ok := sections["allergiesAndIntolerances"].(map[string]interface{})
	if !ok {
		t.Fatal("expected document.sectionsByKey.allergiesAndIntolerances to be present")
	}
	entries, ok := allergies["entries"].([]interface{})
	if !ok || len(entries) == 0 {
		t.Fatal("expected at least one entry under the allergies section")
	}
	act, ok := entries[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first allergy entry to be a JSON object")
	}
	rels, ok := act["entryRelationships"].([]interface{})
	if !ok || len(rels) == 0 {
		t.Fatal("expected entryRelationships on the allergy concern act")
	}
	rel, ok := rels[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected entryRelationship to be a JSON object")
	}
	if rel["typeCode"] != "SUBJ" {
		t.Errorf("entryRelationship.typeCode = %v, want SUBJ", rel["typeCode"])
	}
	nestedObs, ok := rel["entry"].(map[string]interface{})
	if !ok {
		t.Fatal("expected entryRelationship.entry to be a JSON object")
	}
	// This is exactly the field that's invisible in ParsedJSON["sections"]
	// (the curated whitelist) but must be reachable here.
	if got, _ := nestedObs["negationInd"].(bool); !got {
		t.Errorf("nestedObs[negationInd] = %v, want true — this is the field the curated "+
			"sections.* whitelist can never carry", nestedObs["negationInd"])
	}
	if nestedObs["moodCode"] != "EVN" {
		t.Errorf("nestedObs[moodCode] = %v, want EVN", nestedObs["moodCode"])
	}
}
