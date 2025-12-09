# Visual Architecture Guide
## Universal Transformation Pipeline - Format Agnostic Design

---

## 🎨 High-Level Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         INPUT FORMATS                                    │
├─────────────────────────────────────────────────────────────────────────┤
│  HL7 v2.x  │  FHIR R4  │  CCD/C-CDA  │  X12 EDI  │  DICOM  │  Custom   │
└──────┬──────┴─────┬─────┴──────┬──────┴─────┬─────┴────┬────┴─────┬─────┘
       │            │            │            │          │          │
       ▼            ▼            ▼            ▼          ▼          ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      FORMAT ADAPTERS (Input)                             │
│                     Plugin Architecture                                  │
├─────────────────────────────────────────────────────────────────────────┤
│  • HL7Adapter       • FHIRAdapter      • CCDAdapter                     │
│  • X12Adapter       • DICOMAdapter     • CustomAdapter                  │
│                                                                          │
│  Each adapter implements: IFormatAdapter interface                      │
│  - Parse(rawMessage) → UniversalEnvelope                               │
│  - ExtractMetadata() → EnvelopeMetadata                                │
│  - Validate() → bool                                                    │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      UNIVERSAL MESSAGE ENVELOPE                          │
│                     (Format-Independent Container)                       │
├─────────────────────────────────────────────────────────────────────────┤
│  {                                                                       │
│    "envelope": {                                                         │
│      "messageId": "uuid",                                               │
│      "sourceFormat": "hl7v2" | "fhir" | "ccd" | "x12",                 │
│      "messageType": "ADT^A01" | "Patient" | "CCD" | "837P",            │
│      "schema": { "validated": true, "version": "..." }                  │
│    },                                                                    │
│    "content": { /* Parsed structured data */ },                         │
│    "processing": { /* Pipeline state */ }                               │
│  }                                                                       │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│               TRANSFORMATION PIPELINE (Format-Agnostic)                  │
│                         Step-by-Step Processing                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌───────────────────────────────────────────────────────────┐          │
│  │  PRE-PROCESSING LAYER                                     │          │
│  ├───────────────────────────────────────────────────────────┤          │
│  │  Step 10:  Format-Aware Validation                        │          │
│  │            ├─ If HL7: HL7ValidationExecutor               │          │
│  │            ├─ If FHIR: FHIRValidationExecutor             │          │
│  │            └─ If CCD: CCDValidationExecutor               │          │
│  │                                                            │          │
│  │  Step 20:  Universal Enrichment                           │          │
│  │            └─ Works with any format (*)                   │          │
│  │                                                            │          │
│  │  Step 30:  Data Quality Check                             │          │
│  │            └─ Format-agnostic rules                       │          │
│  └───────────────────────────┬────────────────────────────────┘          │
│                              ▼                                           │
│  ┌───────────────────────────────────────────────────────────┐          │
│  │  CORE TRANSFORMATION LAYER                                │          │
│  ├───────────────────────────────────────────────────────────┤          │
│  │  Step 100: Format Conversion (if needed)                  │          │
│  │            ├─ HL7 → FHIR  (HL7ToFHIRTransformer)          │          │
│  │            ├─ FHIR → HL7  (FHIRToHL7Transformer)          │          │
│  │            ├─ CCD → FHIR  (CCDToFHIRTransformer)          │          │
│  │            ├─ X12 → HL7   (X12ToHL7Transformer)           │          │
│  │            └─ Updates envelope.sourceFormat               │          │
│  │                                                            │          │
│  │  Step 110: Business Logic                                 │          │
│  │            └─ Custom transformations                      │          │
│  └───────────────────────────┬────────────────────────────────┘          │
│                              ▼                                           │
│  ┌───────────────────────────────────────────────────────────┐          │
│  │  POST-PROCESSING LAYER                                    │          │
│  ├───────────────────────────────────────────────────────────┤          │
│  │  Step 200: Output Format Validation                       │          │
│  │            └─ Validate against target schema              │          │
│  │                                                            │          │
│  │  Step 210: Anonymization (if required)                    │          │
│  │            └─ PHI removal/masking                         │          │
│  │                                                            │          │
│  │  Step 220: Routing Logic                                  │          │
│  │            └─ Determine destination                       │          │
│  └───────────────────────────────────────────────────────────┘          │
│                                                                          │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    FORMAT ADAPTERS (Output)                              │
│                    Serialization Layer                                   │
├─────────────────────────────────────────────────────────────────────────┤
│  • HL7Serializer    • FHIRSerializer   • CCDSerializer                  │
│  • X12Serializer    • JSONSerializer   • PDFSerializer                  │
│                                                                          │
│  Each serializer implements: IFormatSerializer interface                │
│  - Serialize(envelope) → formattedOutput                                │
│  - Validate(output) → bool                                              │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        OUTPUT FORMATS                                    │
├─────────────────────────────────────────────────────────────────────────┤
│  HL7 v2.x  │  FHIR R4  │  CCD/C-CDA  │  X12 EDI  │  JSON  │  PDF       │
└────────────┴───────────┴─────────────┴───────────┴────────┴────────────┘
```

---

## 🔄 Execution Flow Example: HL7 → FHIR

```
1. INPUT
   ┌─────────────────┐
   │  HL7 Message    │
   │  MSH|^~\&|...   │
   └────────┬────────┘
            │
            ▼
2. PARSE (HL7Adapter)
   ┌──────────────────────────┐
   │  Universal Envelope      │
   │  {                       │
   │    "envelope": {         │
   │      "sourceFormat":     │
   │        "hl7v2",          │
   │      "messageType":      │
   │        "ADT^A01"         │
   │    },                    │
   │    "content": {          │
   │      "enhancedSegments": │
   │        { MSH, PID... }   │
   │    }                     │
   │  }                       │
   └────────┬─────────────────┘
            │
            ▼
3. PIPELINE EXECUTION
   ┌──────────────────────────────────┐
   │  Step 10: HL7 Validation         │
   │  ├─ sourceFormats: ["hl7v2"]     │
   │  ├─ Checks: MSH.9, PID.3         │
   │  └─ Result: ✓ Valid              │
   └────────┬─────────────────────────┘
            │
            ▼
   ┌──────────────────────────────────┐
   │  Step 20: Universal Enrichment   │
   │  ├─ sourceFormats: ["*"]         │
   │  ├─ Looks up patient in EMPI     │
   │  └─ Adds: MRN, demographics      │
   └────────┬─────────────────────────┘
            │
            ▼
   ┌──────────────────────────────────┐
   │  Step 100: HL7 → FHIR Transform  │
   │  ├─ sourceFormats: ["hl7v2"]     │
   │  ├─ targetFormat: "fhir"         │
   │  ├─ Maps: PID → Patient          │
   │  │         PV1 → Encounter        │
   │  └─ Updates envelope:            │
   │      sourceFormat = "fhir"       │
   └────────┬─────────────────────────┘
            │
            ▼
   ┌──────────────────────────────────┐
   │  Step 200: FHIR Validation       │
   │  ├─ sourceFormats: ["fhir"]      │
   │  ├─ Validates: Bundle structure  │
   │  └─ Result: ✓ Valid              │
   └────────┬─────────────────────────┘
            │
            ▼
4. SERIALIZE (FHIRSerializer)
   ┌──────────────────────────┐
   │  FHIR Bundle (JSON)      │
   │  {                       │
   │    "resourceType":       │
   │      "Bundle",           │
   │    "type": "message",    │
   │    "entry": [            │
   │      { Patient },        │
   │      { Encounter }       │
   │    ]                     │
   │  }                       │
   └────────┬─────────────────┘
            │
            ▼
5. OUTPUT
   ┌─────────────────┐
   │  FHIR Message   │
   │  (Ready to send)│
   └─────────────────┘
```

---

## 🧩 Executor Selection Matrix

```
┌────────────────────────────────────────────────────────────────────────┐
│              EXECUTOR COMPATIBILITY MATRIX                              │
├────────────────┬───────────────────────────────────────────────────────┤
│   EXECUTOR     │  HL7  │  FHIR │  CCD  │  X12  │  Custom │  Notes     │
├────────────────┼───────┼───────┼───────┼───────┼─────────┼────────────┤
│ HL7 Validation │   ✓   │   ✗   │   ✗   │   ✗   │    ✗    │ HL7 only   │
│ FHIR Validation│   ✗   │   ✓   │   ✗   │   ✗   │    ✗    │ FHIR only  │
│ CCD Validation │   ✗   │   ✗   │   ✓   │   ✗   │    ✗    │ CCD only   │
│ X12 Validation │   ✗   │   ✗   │   ✗   │   ✓   │    ✗    │ X12 only   │
├────────────────┼───────┼───────┼───────┼───────┼─────────┼────────────┤
│ HL7 → FHIR     │   ✓   │   ✗   │   ✗   │   ✗   │    ✗    │ Transform  │
│ FHIR → HL7     │   ✗   │   ✓   │   ✗   │   ✗   │    ✗    │ Transform  │
│ CCD → FHIR     │   ✗   │   ✗   │   ✓   │   ✗   │    ✗    │ Transform  │
│ X12 → HL7      │   ✗   │   ✗   │   ✗   │   ✓   │    ✗    │ Transform  │
├────────────────┼───────┼───────┼───────┼───────┼─────────┼────────────┤
│ Enrichment     │   ✓   │   ✓   │   ✓   │   ✓   │    ✓    │ Universal  │
│ Routing        │   ✓   │   ✓   │   ✓   │   ✓   │    ✓    │ Universal  │
│ Logging        │   ✓   │   ✓   │   ✓   │   ✓   │    ✓    │ Universal  │
│ Anonymization  │   ✓   │   ✓   │   ✓   │   ✓   │    ✓    │ Universal  │
└────────────────┴───────┴───────┴───────┴───────┴─────────┴────────────┘

Legend:
  ✓ = Supported
  ✗ = Not supported
  Universal = Works with all formats
```

---

## 📊 Pipeline Types & Use Cases

### **Type 1: Format-Specific Pipeline**
```
Pipeline: "HL7 ADT Processing"
Source Formats: ["hl7v2"]
Target Format: "hl7v2" (passthrough)

┌─────────────────┐
│  HL7 Message    │
└────────┬────────┘
         │
    ┌────▼─────┐
    │ Validate │  HL7 Validation Executor
    └────┬─────┘
         │
    ┌────▼─────┐
    │ Enrich   │  Universal Enrichment
    └────┬─────┘
         │
    ┌────▼─────┐
    │ Route    │  Universal Routing
    └────┬─────┘
         │
┌────────▼────────┐
│  HL7 Message    │
└─────────────────┘

Use Case: HL7 message validation and routing
```

### **Type 2: Transformation Pipeline**
```
Pipeline: "HL7 to FHIR Conversion"
Source Formats: ["hl7v2"]
Target Format: "fhir"

┌─────────────────┐
│  HL7 Message    │
└────────┬────────┘
         │
    ┌────▼─────┐
    │ Validate │  HL7 Validation Executor
    └────┬─────┘
         │
    ┌────▼─────┐
    │Transform │  HL7ToFHIRTransformer
    └────┬─────┘  (Changes sourceFormat to "fhir")
         │
    ┌────▼─────┐
    │ Validate │  FHIR Validation Executor
    └────┬─────┘
         │
┌────────▼────────┐
│  FHIR Bundle    │
└─────────────────┘

Use Case: Convert HL7 ADT to FHIR Patient resource
```

### **Type 3: Multi-Format Pipeline**
```
Pipeline: "Universal Healthcare Processor"
Source Formats: ["*"]  (accepts any format)
Target Format: "fhir" (normalizes to FHIR)

┌─────────┐  ┌─────────┐  ┌─────────┐
│   HL7   │  │  FHIR   │  │   CCD   │
└────┬────┘  └────┬────┘  └────┬────┘
     │            │            │
     └────────────┼────────────┘
                  │
            ┌─────▼─────┐
            │ Normalize │  Format-specific transformers
            └─────┬─────┘  (All → FHIR)
                  │
            ┌─────▼─────┐
            │ Enrich    │  Universal Enrichment
            └─────┬─────┘
                  │
            ┌─────▼─────┐
            │  Store    │  FHIR Repository
            └─────┬─────┘
                  │
            ┌─────▼─────┐
            │FHIR Bundle│
            └───────────┘

Use Case: Accept any format, normalize to FHIR, store in FHIR server
```

### **Type 4: Bi-Directional Pipeline**
```
Pipeline: "HL7 ↔ FHIR Sync"
Source Formats: ["hl7v2", "fhir"]
Target Format: "dynamic" (depends on source)

         ┌─────────┐
         │ Message │
         └────┬────┘
              │
         ┌────▼────────────────┐
         │ Format Detection    │
         └────┬────────────────┘
              │
         ┌────▼────────────────┐
    ┌────┤ If HL7 → FHIR      │
    │    └────┬────────────────┘
    │         │
    │    ┌────▼────────────────┐
    └───►│ If FHIR → HL7      │
         └────┬────────────────┘
              │
         ┌────▼────┐
         │ Output  │
         └─────────┘

Use Case: Two-way sync between HL7 system and FHIR server
```

---

## 🎯 Smart Toolbox Filtering

```
Current Pipeline Format: HL7v2
Current Step: 3 (after HL7 validation)

┌────────────────────────────────────────────────────────────┐
│                   AVAILABLE STEPS                          │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  ✓ HL7-SPECIFIC STEPS                                     │
│    • HL7 Segment Validation      [sourceFormats: hl7v2]   │
│    • HL7 Field Mapping           [sourceFormats: hl7v2]   │
│    • HL7 Acknowledgment          [sourceFormats: hl7v2]   │
│                                                            │
│  ✓ TRANSFORMATIONS                                        │
│    • HL7 → FHIR Transform        [hl7v2 → fhir]          │
│    • HL7 → CCD Transform         [hl7v2 → ccd]           │
│                                                            │
│  ✓ UNIVERSAL STEPS                                        │
│    • Patient Enrichment          [sourceFormats: *]       │
│    • Data Quality Check          [sourceFormats: *]       │
│    • Conditional Routing         [sourceFormats: *]       │
│                                                            │
│  ✗ INCOMPATIBLE STEPS (Hidden)                            │
│    • FHIR Bundle Validation      [sourceFormats: fhir]    │
│    • CCD Section Extraction      [sourceFormats: ccd]     │
│    • X12 Claims Processing       [sourceFormats: x12]     │
│                                                            │
└────────────────────────────────────────────────────────────┘

After adding "HL7 → FHIR Transform" step:
Pipeline format changes to: FHIR

┌────────────────────────────────────────────────────────────┐
│                   AVAILABLE STEPS (Updated)                │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  ✓ FHIR-SPECIFIC STEPS                                    │
│    • FHIR Bundle Validation      [sourceFormats: fhir]    │
│    • FHIR Profile Check          [sourceFormats: fhir]    │
│    • FHIR Resource Extraction    [sourceFormats: fhir]    │
│                                                            │
│  ✓ TRANSFORMATIONS                                        │
│    • FHIR → HL7 Transform        [fhir → hl7v2]          │
│    • FHIR → CCD Transform        [fhir → ccd]            │
│                                                            │
│  ✓ UNIVERSAL STEPS                                        │
│    • Patient Enrichment          [sourceFormats: *]       │
│    • Data Quality Check          [sourceFormats: *]       │
│                                                            │
│  ✗ INCOMPATIBLE STEPS (Hidden)                            │
│    • HL7 Segment Validation      [sourceFormats: hl7v2]   │
│    • CCD Section Extraction      [sourceFormats: ccd]     │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

---

## 🔐 Configuration Intelligence

### **Smart Step Configuration**

```javascript
// User adds "Field Validation" step
// System detects current format and shows appropriate config

// If current format = HL7:
{
    "stepType": "validation",
    "executorId": "hl7_field_validation",  // Auto-selected
    "config": {
        "rules": [
            { "field": "MSH.9", "type": "required" },  // HL7 field syntax
            { "field": "PID.3", "type": "required" }
        ]
    }
}

// If current format = FHIR:
{
    "stepType": "validation",
    "executorId": "fhir_resource_validation",  // Auto-selected
    "config": {
        "rules": [
            { "resource": "Patient", "field": "identifier", "type": "required" },  // FHIR syntax
            { "resource": "Patient", "field": "name", "type": "required" }
        ]
    }
}

// If current format = CCD:
{
    "stepType": "validation",
    "executorId": "ccd_section_validation",  // Auto-selected
    "config": {
        "rules": [
            { "section": "allergies", "type": "required" },  // CCD syntax
            { "section": "medications", "type": "required" }
        ]
    }
}
```

---

## ✨ Key Advantages Visualized

```
┌────────────────────────────────────────────────────────────────┐
│                   WITHOUT THIS ARCHITECTURE                     │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  HL7 Pipeline ────────► HL7 Validator ─► HL7 Transform        │
│                                                                 │
│  FHIR Pipeline ───────► FHIR Validator ─► FHIR Transform      │
│                                                                 │
│  CCD Pipeline ────────► CCD Validator ──► CCD Transform        │
│                                                                 │
│  Problem: 3 separate pipelines, 9 separate components          │
│  Code Duplication: ~300%                                       │
│  Maintenance: Update 3 places for each change                  │
│                                                                 │
└────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────┐
│                    WITH THIS ARCHITECTURE                       │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│              Universal Pipeline (One)                           │
│                        │                                        │
│         ┌──────────────┼──────────────┐                        │
│         ▼              ▼              ▼                        │
│    HL7 Executor   FHIR Executor   CCD Executor                │
│                                                                 │
│  + Shared Universal Executors (Enrichment, Routing, etc.)      │
│                                                                 │
│  Benefit: 1 pipeline, pluggable executors                      │
│  Code Reuse: ~80%                                              │
│  Maintenance: Update 1 place, works for all formats            │
│  Extensibility: Add new format = Add 1 adapter + executors     │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
```

---

## 🎓 Summary

This architecture enables:

✅ **One Pipeline, Any Format** - HL7, FHIR, CCD, X12, custom
✅ **Smart Filtering** - UI shows only compatible steps
✅ **Zero Coupling** - Format changes don't break pipeline
✅ **Infinite Reusability** - Executors shared across formats
✅ **Future-Proof** - Add new formats without core changes
✅ **Clean Code** - MVC, OOP, Plugin Architecture
✅ **Maintainable** - Single source of truth
✅ **Scalable** - Add complexity without adding code

**Result**: Enterprise-grade, production-ready transformation engine.
