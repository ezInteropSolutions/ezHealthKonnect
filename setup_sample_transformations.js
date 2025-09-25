// setup_sample_transformations.js
// Script to create sample transformation pipelines in MongoDB for testing

// Connect to MongoDB (run with: mongo < setup_sample_transformations.js)
use ezhealthkonnect;

print("🔧 Setting up sample transformation pipelines...");

// Sample transformation pipeline for HL7 ADT^A01 to FHIR Patient
db.transformation_pipelines.insertOne({
  interface_id: "146941d7-dc19-4ee2-964a-7fe6c1cb429f", // Test Interface1 ID from your system
  name: "HL7 ADT to FHIR Patient Transformation",
  description: "Converts HL7 ADT^A01 messages to FHIR Patient resources",
  source_format: "hl7",
  target_format: "fhir",
  steps: [
    {
      id: "step_1",
      name: "Parse HL7 Message",
      step_type: "hl7_parse",
      order: 1,
      config: {
        extract_segments: ["MSH", "EVN", "PID", "PV1"]
      },
      conditions: [],
      on_error: {
        action: "fail",
        max_retries: 2
      },
      is_active: true,
      description: "Parse incoming HL7 message into structured format"
    },
    {
      id: "step_2",
      name: "Map Patient Demographics",
      step_type: "field_mapping",
      order: 2,
      config: {
        source_path: "parsed_hl7.PID",
        target_path: "patient.demographics",
        mappings: [
          {
            source_path: "parsed_hl7.PID.5", // Patient Name
            target_path: "patient.name.family",
            data_type: "string"
          },
          {
            source_path: "parsed_hl7.PID.5", // Patient Name
            target_path: "patient.name.given",
            data_type: "string"
          },
          {
            source_path: "parsed_hl7.PID.7", // Date of Birth
            target_path: "patient.birthDate",
            data_type: "datetime",
            format: "YYYYMMDD"
          },
          {
            source_path: "parsed_hl7.PID.8", // Gender
            target_path: "patient.gender",
            data_type: "string"
          }
        ]
      },
      conditions: [],
      on_error: {
        action: "skip",
        default_value: null
      },
      is_active: true,
      description: "Extract patient demographics from PID segment"
    },
    {
      id: "step_3",
      name: "Build FHIR Patient Resource",
      step_type: "fhir_build",
      order: 3,
      config: {
        resource_type: "Patient",
        template: "fhir_patient_r4",
        include_metadata: true
      },
      conditions: [],
      on_error: {
        action: "fail"
      },
      is_active: true,
      description: "Construct FHIR Patient resource from mapped data"
    },
    {
      id: "step_4",
      name: "Add Message Tracking",
      step_type: "field_mapping",
      order: 4,
      config: {
        source_path: "message_id",
        target_path: "meta.source_message_id",
        data_type: "string",
        required: true
      },
      conditions: [],
      on_error: {
        action: "skip"
      },
      is_active: true,
      description: "Add source message tracking for audit trail"
    }
  ],
  is_active: true,
  created_at: new Date(),
  updated_at: new Date(),
  created_by: "system",
  version: 1
});

print("✅ Created HL7 ADT to FHIR Patient transformation pipeline");

// Sample transformation template for reuse
db.transformation_templates.insertOne({
  name: "Standard HL7 ADT to FHIR Patient",
  description: "Standard transformation template for HL7 admission messages to FHIR Patient resources",
  source_format: "hl7",
  target_format: "fhir",
  message_type: "ADT^A01",
  steps: [
    {
      id: "template_step_1",
      name: "Parse HL7 Message",
      step_type: "hl7_parse",
      order: 1,
      config: {
        extract_segments: ["MSH", "EVN", "PID", "PV1", "NK1"]
      },
      conditions: [],
      on_error: {
        action: "fail"
      },
      is_active: true
    },
    {
      id: "template_step_2",
      name: "Map Core Demographics",
      step_type: "field_mapping",
      order: 2,
      config: {
        mappings: [
          {
            source_path: "parsed_hl7.PID.3.1", // Patient ID
            target_path: "patient.identifier.value",
            data_type: "string",
            required: true
          },
          {
            source_path: "parsed_hl7.PID.5.1", // Family Name
            target_path: "patient.name.family",
            data_type: "string",
            required: true
          },
          {
            source_path: "parsed_hl7.PID.5.2", // Given Name
            target_path: "patient.name.given",
            data_type: "string"
          }
        ]
      },
      conditions: [],
      on_error: {
        action: "default",
        default_value: "Unknown"
      },
      is_active: true
    }
  ],
  is_public: true,
  category: "healthcare",
  created_at: new Date(),
  updated_at: new Date(),
  usage_count: 0,
  version: "1.0"
});

print("✅ Created standard HL7 ADT to FHIR transformation template");

// Sample value map for gender codes
db.value_maps.insertOne({
  name: "HL7 Gender to FHIR Gender Mapping",
  description: "Maps HL7 gender codes to FHIR gender values",
  category: "gender",
  mappings: {
    "M": "male",
    "F": "female",
    "O": "other",
    "U": "unknown",
    "": "unknown"
  },
  is_active: true,
  created_at: new Date(),
  updated_at: new Date()
});

print("✅ Created HL7 to FHIR gender value mapping");

// Sample value map for message types
db.value_maps.insertOne({
  name: "HL7 Message Type Classifications",
  description: "Classification of HL7 message types for routing",
  category: "message_types",
  mappings: {
    "ADT^A01": { type: "admission", priority: "high", resource_type: "Patient" },
    "ADT^A03": { type: "discharge", priority: "high", resource_type: "Encounter" },
    "ORU^R01": { type: "lab_result", priority: "medium", resource_type: "Observation" },
    "ORM^O01": { type: "order", priority: "medium", resource_type: "ServiceRequest" },
    "SIU^S12": { type: "appointment", priority: "low", resource_type: "Appointment" }
  },
  is_active: true,
  created_at: new Date(),
  updated_at: new Date()
});

print("✅ Created HL7 message type classification mapping");

print("🎉 Sample transformation setup completed!");
print("");
print("📋 Summary:");
print("   • 1 transformation pipeline created for interface: 146941d7-dc19-4ee2-964a-7fe6c1cb429f");
print("   • 1 reusable transformation template created");
print("   • 2 value maps created for lookups");
print("");
print("🔧 To test the transformation:");
print("   1. Send an HL7 ADT^A01 message to port 6661");
print("   2. Check Docker logs for transformation processing");
print("   3. Verify FHIR output in target system");