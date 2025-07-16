// migrations/mongodb/init_collections.js
// MongoDB initialization for FHIR schemas and documents

// Switch to FHIR database
db = db.getSiblingDB('fhir_store');

// Create FHIR user
db.createUser({
  user: "fhir_user",
  pwd: "fhir_password", 
  roles: [
    { role: "readWrite", db: "fhir_store" }
  ]
});

// ===================================
// FHIR Schema Collections
// ===================================

// Full FHIR StructureDefinitions
db.createCollection("fhir_schemas", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "resourceType", "version", "profile"],
      properties: {
        _id: { bsonType: "string" },
        resourceType: { bsonType: "string" },
        version: { bsonType: "string" },
        profile: { bsonType: "string" },
        fullDefinition: { bsonType: "object" },
        compiledSchema: { bsonType: "object" },
        metadata: {
          bsonType: "object",
          properties: {
            lastUpdated: { bsonType: "date" },
            size: { bsonType: "int" },
            usage: { enum: ["frequent", "occasional", "rare"] },
            loadTime: { bsonType: "double" }
          }
        }
      }
    }
  }
});

// Create indexes for FHIR schemas
db.fhir_schemas.createIndex(
  { "resourceType": 1, "version": 1, "profile": 1 }, 
  { unique: true, name: "idx_schema_lookup" }
);

db.fhir_schemas.createIndex(
  { "resourceType": 1 }, 
  { name: "idx_resource_type" }
);

db.fhir_schemas.createIndex(
  { "profile": 1 }, 
  { name: "idx_profile" }
);

db.fhir_schemas.createIndex(
  { "metadata.usage": 1, "metadata.lastUpdated": -1 }, 
  { name: "idx_usage_tracking" }
);

// Text search on schema definitions
db.fhir_schemas.createIndex(
  { 
    "fullDefinition.title": "text", 
    "fullDefinition.description": "text",
    "resourceType": "text"
  },
  { name: "idx_schema_text_search" }
);

// ===================================
// FHIR Value Sets and Code Systems
// ===================================

// Value sets with full expansion
db.createCollection("fhir_valuesets", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "url", "version"],
      properties: {
        _id: { bsonType: "string" },
        url: { bsonType: "string" },
        version: { bsonType: "string" },
        name: { bsonType: "string" },
        title: { bsonType: "string" },
        status: { enum: ["active", "draft", "retired"] },
        expansion: {
          bsonType: "object",
          properties: {
            contains: { bsonType: "array" }
          }
        }
      }
    }
  }
});

db.fhir_valuesets.createIndex(
  { "url": 1, "version": 1 }, 
  { unique: true, name: "idx_valueset_url_version" }
);

db.fhir_valuesets.createIndex(
  { "name": 1 }, 
  { name: "idx_valueset_name" }
);

db.fhir_valuesets.createIndex(
  { "status": 1 }, 
  { name: "idx_valueset_status" }
);

// Code systems
db.createCollection("fhir_codesystems", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "url"],
      properties: {
        _id: { bsonType: "string" },
        url: { bsonType: "string" },
        version: { bsonType: "string" },
        name: { bsonType: "string" },
        status: { enum: ["active", "draft", "retired"] },
        concept: { bsonType: "array" }
      }
    }
  }
});

db.fhir_codesystems.createIndex(
  { "url": 1 }, 
  { unique: true, name: "idx_codesystem_url" }
);

db.fhir_codesystems.createIndex(
  { "concept.code": 1 }, 
  { name: "idx_concept_codes" }
);

// ===================================
// HL7→FHIR Mapping Rules
// ===================================

// Transformation mapping rules
db.createCollection("hl7_fhir_mappings", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "hl7MessageType", "targetFhirResource"],
      properties: {
        _id: { bsonType: "string" },
        hl7MessageType: { bsonType: "string" },
        hl7Version: { bsonType: "string" },
        targetFhirResource: { bsonType: "string" },
        targetProfile: { bsonType: "string" },
        mappingRules: { bsonType: "object" },
        conditions: { bsonType: "array" },
        priority: { bsonType: "int" },
        isActive: { bsonType: "bool" }
      }
    }
  }
});

db.hl7_fhir_mappings.createIndex(
  { "hl7MessageType": 1, "targetFhirResource": 1 }, 
  { name: "idx_mapping_lookup" }
);

db.hl7_fhir_mappings.createIndex(
  { "targetProfile": 1 }, 
  { name: "idx_target_profile" }
);

db.hl7_fhir_mappings.createIndex(
  { "priority": 1, "isActive": 1 }, 
  { name: "idx_mapping_priority" }
);

// ===================================
// FHIR Validation Rules
// ===================================

// Custom validation rules and profiles
db.createCollection("fhir_validation_rules", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["_id", "resourceType", "ruleType"],
      properties: {
        _id: { bsonType: "string" },
        resourceType: { bsonType: "string" },
        profile: { bsonType: "string" },
        ruleType: { enum: ["constraint", "invariant", "profile-specific"] },
        expression: { bsonType: "string" },
        severity: { enum: ["error", "warning", "information"] },
        description: { bsonType: "string" }
      }
    }
  }
});

db.fhir_validation_rules.createIndex(
  { "resourceType": 1, "profile": 1 }, 
  { name: "idx_validation_resource_profile" }
);

db.fhir_validation_rules.createIndex(
  { "ruleType": 1, "severity": 1 }, 
  { name: "idx_validation_type_severity" }
);

// ===================================
// Sample Data Insertion
// ===================================

// Insert sample FHIR schema document
db.fhir_schemas.insertOne({
  _id: "Patient_R4_base",
  resourceType: "Patient",
  version: "R4",
  profile: "base",
  fullDefinition: {
    resourceType: "StructureDefinition",
    id: "Patient",
    url: "http://hl7.org/fhir/StructureDefinition/Patient",
    name: "Patient",
    title: "Patient",
    status: "active",
    kind: "resource",
    abstract: false,
    type: "Patient",
    baseDefinition: "http://hl7.org/fhir/StructureDefinition/DomainResource"
  },
  compiledSchema: {
    requiredFields: ["id"],
    optionalFields: ["name", "gender", "birthDate"],
    fieldTypes: {
      "id": "id",
      "name": "HumanName[]",
      "gender": "code",
      "birthDate": "date"
    }
  },
  metadata: {
    lastUpdated: new Date(),
    size: 1024,
    usage: "frequent",
    loadTime: 0.5
  }
});

// Insert sample HL7→FHIR mapping
db.hl7_fhir_mappings.insertOne({
  _id: "ADT_A01_to_Patient",
  hl7MessageType: "ADT^A01",
  hl7Version: "2.5.1",
  targetFhirResource: "Patient",
  targetProfile: "us-core-patient",
  mappingRules: {
    "PID.3": {
      target: "Patient.identifier",
      transformation: "hl7_identifier_to_fhir"
    },
    "PID.5": {
      target: "Patient.name",
      transformation: "hl7_name_to_fhir"
    },
    "PID.7": {
      target: "Patient.birthDate",
      transformation: "hl7_date_to_fhir"
    },
    "PID.8": {
      target: "Patient.gender",
      transformation: "hl7_gender_to_fhir",
      valueMap: {
        "M": "male",
        "F": "female",
        "O": "other",
        "U": "unknown"
      }
    }
  },
  conditions: [
    "PID segment exists",
    "PID.3 (identifier) is not empty"
  ],
  priority: 100,
  isActive: true
});

// Create compound text index for advanced search
db.fhir_schemas.createIndex({
  "resourceType": "text",
  "fullDefinition.title": "text",
  "fullDefinition.description": "text"
}, {
  name: "text_search_compound",
  weights: {
    "resourceType": 10,
    "fullDefinition.title": 5,
    "fullDefinition.description": 1
  }
});

print("MongoDB FHIR collections initialized successfully!");
print("Collections created: fhir_schemas, fhir_valuesets, fhir_codesystems, hl7_fhir_mappings, fhir_validation_rules");
print("Indexes created for optimal query performance");
print("Sample data inserted for testing");