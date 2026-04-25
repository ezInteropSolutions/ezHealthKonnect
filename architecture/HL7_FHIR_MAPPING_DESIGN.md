# HL7 → FHIR Mapping Design

## Philosophy

The mapping engine is **schema-driven on both sides** — the HL7 schema (data
dictionary: segments, fields, data types, cardinality, descriptions) drives the
source, and the FHIR R4 schema (resource definitions, element types, bindings,
cardinality) drives the target.  The same engine code handles any HL7 version
and any FHIR version because all rules are stored in database templates, not in
Go code.  Adding support for a new message type means inserting a template row;
it never requires a code deployment.

---

## Two-Phase Model

### Phase 1 — Authoring (description-based)

OOB templates are authored by matching **HL7 field descriptions** to **FHIR
element descriptions**.  The author reads the HL7 data dictionary (field name,
data type, table binding, cardinality) and the FHIR specification (element
definition, type, binding strength, cardinality) and decides the correct
mapping.  This semantic, human-readable process is what makes templates correct
across HL7 versions: `PID.5` is "Patient Name (XPN)" in v2.3 and v2.5 alike —
the description is stable even if position assignments shift in edge cases.

Sources used to author OOB templates:
- HL7 v2.x data dictionary (loaded into the HL7 schema service)
- FHIR R4 resource definitions (loaded into the FHIR schema loader)
- Sample HL7 messages: https://docs.webchartnow.com/features/system-administration/interfaces/sample-hl7-messages/
  (and other sample sources for completeness and edge-case coverage)

### Phase 2 — Runtime (position-based)

The authored template is stored in `hl7_fhir_templates.template_config` as
explicit, pre-resolved paths.  At transform time the engine reads the template
JSON and applies the mappings directly — no schema look-up, no description
parsing.  This makes runtime fast and deterministic.

---

## Template Structure

Templates are stored in `hl7_fhir_templates.template_config` (JSONB).

### Current format (v1.0)

```json
{
  "version": "1.0",
  "fhirVersion": "R4",
  "resources": {
    "Patient": {
      "mappings": [
        {
          "hl7Path":     "PID.3.1",
          "fhirPath":    "Patient.identifier[0].value",
          "hl7DataType": "ST",
          "fhirDataType":"string",
          "transform":   "string_direct",
          "required":    true,
          "confidence":  1.0
        },
        {
          "hl7Path":     "PID.8",
          "fhirPath":    "Patient.gender",
          "hl7DataType": "IS",
          "fhirDataType":"code",
          "transform":   "hl7_table_0001_gender",
          "required":    false,
          "confidence":  1.0,
          "valueMap": { "M":"male","F":"female","O":"other","U":"unknown" }
        }
      ]
    },
    "Encounter": {
      "mappings": [ ... ]
    }
  }
}
```

### Target format (v1.1) — adds context declaration

The `context` block and per-resource `contextLinks` are the only additions.
All existing v1.0 fields are preserved unchanged.  The engine reads
`contextLinks` when present and falls back to existing behaviour when absent,
so the migration is non-breaking.

```json
{
  "version": "1.1",
  "fhirVersion": "R4",
  "context": {
    "patient":   "PID",
    "encounter": "PV1",
    "order":     "ORC"
  },
  "resources": {
    "Patient": {
      "segment": "PID",
      "mappings": [ ... ]
    },
    "Encounter": {
      "segment": "PV1",
      "contextLinks": {
        "subject": "patient"
      },
      "mappings": [ ... ]
    },
    "AllergyIntolerance": {
      "segment": "AL1",
      "contextLinks": {
        "patient":    "patient",
        "encounter":  "encounter"
      },
      "mappings": [ ... ]
    },
    "Observation": {
      "segment": "OBX",
      "repeating": true,
      "contextLinks": {
        "subject":   "patient",
        "encounter": "encounter"
      },
      "mappings": [ ... ]
    }
  }
}
```

**`context` block** — declares which HL7 segment is the authoritative source
for each named context role (`patient`, `encounter`, `order`, etc.).  The
engine uses this to know which already-built FHIR resource to reference.

**`contextLinks`** — per resource, maps FHIR reference elements to a named
context role.  The engine resolves the role to the actual resource ID that was
built earlier in the same message, then sets the reference automatically.
No Go code change is needed when a new message type introduces a new context
relationship — it is declared entirely in the template.

**`repeating: true`** — tells the engine this resource is built once per
occurrence of the segment (OBX, AL1, DG1, PR1, etc.).

---

## Mapping Rule Fields

| Field | Purpose |
|---|---|
| `hl7Path` | Source path: `SEGMENT.field.component` (e.g. `PID.5.1`) |
| `fhirPath` | Target path in FHIR resource (e.g. `Patient.name[0].family`) |
| `hl7DataType` | HL7 primitive or composite type (ST, XPN, TS, CE, CWE, …) |
| `fhirDataType` | FHIR type (string, code, dateTime, HumanName, …) |
| `transform` | Named transformation function (see below) |
| `required` | Whether the FHIR element is mandatory |
| `confidence` | Authoring confidence score 0–1 (informational; used by ezCompanion) |
| `valueMap` | Code translation table for coded fields (HL7 table → FHIR codes) |

---

## Transformations

Transformations convert an HL7 raw value to the correct FHIR representation.
They are referenced by name in the `transform` field; the engine dispatches to
the registered function.

| Transform name | HL7 type | FHIR type | Description |
|---|---|---|---|
| `string_direct` | any | string/id/uri | Pass-through with whitespace trim |
| `hl7_timestamp_to_fhir_date` | TS | date | `YYYYMMDD` → `YYYY-MM-DD` |
| `hl7_timestamp_to_fhir_datetime` | TS | dateTime | `YYYYMMDDHHmmss±ZZZZ` → ISO 8601 |
| `hl7_timestamp_to_fhir_instant` | TS | instant | Same as datetime, with timezone |
| `hl7_table_0001_gender` | IS | code | HL7 Table 0001 → FHIR administrative-gender |
| `hl7_table_0004_patient_class` | IS | Coding | HL7 Table 0004 → FHIR v3 ActCode |
| `name_component` | XPN/FN | string | Extract single component from composite name |
| `address_component` | XAD/SAD | string | Extract component from composite address |
| `telecom_value` | XTN | string | Extract phone/email value |
| `telecom_system_mapping` | ID | code | Equipment type → FHIR ContactPointSystem |
| `telecom_use_mapping` | ID | code | Use code → FHIR ContactPointUse |
| `address_use_mapping` | ID | code | Address type → FHIR AddressUse |
| `name_use_mapping` | ID | code | Name type → FHIR NameUse |
| `assigning_authority_to_uri` | HD | uri | Build `urn:oid:` or system URI from HD |
| `identifier_type_mapping` | ID | code | HL7 identifier type → FHIR IdentifierType |
| `marital_status_mapping` | CE | code | HL7 Table 0002 → FHIR marital-status |
| `discharge_disposition_mapping` | IS | code | UB-04 codes → FHIR discharge-disposition |
| `allergy_category_mapping` | IS | code | HL7 Table 0127 → FHIR AllergyIntoleranceCategory |
| `allergy_severity_mapping` | IS | code | HL7 Table 0128 → FHIR AllergyIntoleranceSeverity |
| `diagnosis_type_mapping` | IS | code | HL7 Table 0052 → FHIR condition-category |
| `coding_system_mapping` | ID | uri | OID/name → standard FHIR system URI |
| `boolean_yn_mapping` | ID | boolean | Y/N → true/false |
| `admission_type_mapping` | IS | code | HL7 Table 0007 → FHIR admission type |

New transforms are registered by name in the transform engine; templates
reference them by name, so adding a new transform never requires changing
existing templates.

---

## Bindings (Value Set Mappings)

Coded fields carry a `valueMap` inline in the mapping rule.  For larger or
shared code tables, the `value_set_mappings` database table holds reusable
HL7 table → FHIR ValueSet translations that the engine can reference by name.

The `transform` field acts as the binding declaration: a transform named
`hl7_table_0001_gender` implies the binding to
`http://hl7.org/fhir/ValueSet/administrative-gender`.  The FHIR schema loader
validates the output code against the bound ValueSet when validation is enabled.

---

## Validations

Validation is layered:

1. **Required field check** — enforced by `"required": true` in the mapping
   rule; the engine logs a warning or error if the source field is absent.

2. **Cardinality check** — the FHIR schema loader knows the cardinality of each
   element (`0..1`, `1..*`, etc.) and validates the number of values produced.

3. **Data type check** — the engine applies the named transform and verifies the
   output is type-compatible with the declared `fhirDataType`.

4. **Binding validation** — for coded elements, the output code is checked
   against the `valueMap` or the bound FHIR ValueSet.

5. **FHIR profile conformance** — optional post-transform step; the produced
   FHIR resource is validated against the HL7 FHIR R4 profile using the
   FHIR schema loaded at startup.

---

## Parsing — Template Selection

When a message arrives the parser selects the schema in this order:

1. **message_type + event** exact match (e.g. `ADT^A01`)
2. **message_type only** fallback (e.g. `ADT`) — used when the event code is
   absent or unrecognised
3. **version** — `hl7_version` column in `hl7_fhir_templates`; defaults to
   `2.5`, matches are attempted from most to least specific version

`ParseWithRealSchema()` produces the **enhanced HL7 JSON**: each field carries
its description, data type, subfields, and dictionary source.  This enriched
representation is what the mapping engine reads — it never re-parses raw HL7
at map time.

---

## OOB Templates

Out-of-box templates are seeded by Flyway migrations and cover the most common
HL7 v2 message types.  They are marked `is_system = true` and cannot be deleted
through the UI.

### Currently seeded (V62 migration — 57 templates)

| Family | Events |
|---|---|
| ADT | A01–A08, A11–A17, A20–A24, A28–A29, A31, A34–A36, A39–A40 |
| BAR | P01, P02 |
| DFT | P03 |
| MDM | T01–T03, T05, T07, T11 |
| OMG | O19 |
| OML | O21 |
| ORM | O01 |
| ORU | R01, R30 |
| PPR | PC1–PC3 |
| RAS | O17 |
| RDE | O11 |
| REF | I12, I14 |
| SIU | S12–S15, S17 |
| VXQ | V01 |
| VXU | V04 |

### Enrichment plan

All 57 templates will be enriched against real-world sample messages to ensure
edge cases (optional segments, repeating groups, missing fields) are handled
correctly.  The primary sample source is:

> https://docs.webchartnow.com/features/system-administration/interfaces/sample-hl7-messages/

Additional sources are used to expand coverage to less-common segments (NK1,
GT1, IN1/IN2, ROL, TXA, etc.) and to validate `valueMap` completeness.

All enriched templates will be migrated to **v1.1 format** with `context` and
`contextLinks` at the same time.

---

## Wizard Integration

The wizard is the user-facing tool for viewing and customising templates.

**Default behaviour:** When a user configures a new interface, the wizard
pre-populates the mapping from the OOB template for the selected message type.
The user sees every field mapping with its source (`hl7Path`), destination
(`fhirPath`), transform, and confidence score.

**Customisation:** Users can:
- Add new mapping rules
- Modify an existing rule's `fhirPath` or `transform`
- Override a `valueMap` entry
- Toggle `required` on/off
- Add or remove resources from the output bundle

**Persistence:** Custom changes are stored in `interface_message_mappings`
(`custom_mapping_config` JSONB) linked to the specific interface and message
type.  The OOB template is never modified.  At transform time the engine merges
the OOB template with any interface-level overrides.

> **Status:** The wizard UI and mapping display are built.  The save-to-DB path
> (`interface_message_mappings` INSERT/UPDATE) is currently a stub with a TODO
> and must be implemented.

---

## ezCompanion Integration

ezCompanion assists the user within the wizard:

- **Explain** — given an `hl7Path`, describes the HL7 field (name, data type,
  purpose, HL7 table if coded) in plain language
- **Explain** — given a `fhirPath`, describes the FHIR element (definition,
  type, cardinality, binding) in plain language
- **Suggest** — when a user adds a new HL7 field, suggests the most likely FHIR
  target based on description similarity and the `confidence` scores in nearby
  mappings
- **Validate** — reviews a custom mapping for correctness (type compatibility,
  cardinality, binding) and explains any issues
- **Transform advice** — recommends the correct `transform` function for a given
  HL7/FHIR type pair

ezCompanion does not modify templates directly; it presents suggestions which
the user accepts or rejects through the wizard UI.

---

## Implementation Status

| Component | Status |
|---|---|
| HL7 parse with schema (`ParseWithRealSchema`) | ✅ Complete |
| Template load from DB (`loadFromV9OOBTemplates`) | ✅ Complete |
| Field mapping application (v1.0 format) | ✅ Complete |
| Data type enrichment (`enrichMappingsWithDataTypes`) | ✅ Complete |
| Transform functions (all named transforms above) | ✅ Complete |
| Value set / valueMap application | ✅ Complete |
| OOB templates seeded (57 message types) | ✅ Complete |
| Context wiring (hardcoded in Go for existing types) | ✅ Works, not template-driven |
| `contextLinks` in template + engine code path | ⬜ Planned |
| Template upgrade to v1.1 (57 templates) | ⬜ Planned |
| Template enrichment from sample messages | ⬜ Planned |
| Wizard save to DB (`interface_message_mappings`) | ⬜ Stub — must be implemented |
| Custom mapping merge at transform time | ⬜ Planned |
| ezCompanion mapping assistance | ⬜ Planned |
