# Smart Multi-Endpoint Routing - Test Example

## Test Scenario: Emergency ADT Message Routing

### Sample HL7 ADT^A01 Message (Emergency Patient)
```
MSH|^~\&|HIS|MAIN_HOSPITAL|FHIR|EZHEALTHKONNECT|20241225100000||ADT^A01|MSG123456|P|2.4
EVN|A01|20241225100000|||^ADMIN^REGISTRATION
PID|1||987654321^^^MR||SMITH^JOHN^MICHAEL||19850315|M|||123 MAIN ST^APT 5B^CITY^STATE^12345||(555)123-4567||(555)987-6543||S||987654321|||||||||||20241225100000
PV1|1|E|ED^101^1^MAIN_HOSPITAL||||ER123^JONES^SARAH^M^MD||ER|||||||987654321|V|||||||||||||||||||||||20241225100000
```

### Interface Configuration with Smart Routing

```javascript
{
  "name": "Emergency ADT Interface",
  "targetConfig": {
    "routing_strategy": "content_based",
    "endpoints": [
      {
        "id": "emergency_fhir",
        "name": "Emergency FHIR Server",
        "type": "fhir_r4",
        "url": "https://emergency.hospital.org/fhir",
        "resource_endpoint": "Patient",
        "priority": 1,
        "weight": 100,
        "enabled": true
      },
      {
        "id": "main_fhir",
        "name": "Main Hospital FHIR",
        "type": "fhir_r4",
        "url": "https://fhir.hospital.org/R4",
        "resource_endpoint": "Patient",
        "priority": 2,
        "weight": 50,
        "enabled": true
      },
      {
        "id": "analytics_warehouse",
        "name": "Real-time Analytics",
        "type": "database",
        "connection": "analytics-db",
        "priority": 3,
        "async": true,
        "enabled": true
      }
    ],
    "routing_rules": [
      {
        "name": "Emergency_Patients",
        "condition": {
          "field": "patientClass",
          "operator": "equals",
          "value": "E"
        },
        "endpoints": ["emergency_fhir", "analytics_warehouse"],
        "priority": 1,
        "transformation": "adt_emergency_template"
      },
      {
        "name": "Main_Hospital_Patients",
        "condition": {
          "field": "facility",
          "operator": "contains",
          "value": "MAIN_HOSPITAL"
        },
        "endpoints": ["main_fhir", "analytics_warehouse"],
        "priority": 2
      }
    ]
  }
}
```

## Expected Routing Flow

### Step 1: HL7 Message Analysis
```javascript
{
  messageType: "ADT",
  triggerEvent: "A01",
  facility: "MAIN_HOSPITAL",
  patientId: "987654321",
  patientClass: "E",      // Emergency patient!
  department: "ED",
  urgency: "HIGH",        // Automatically detected from patient class
  specialty: "EMERGENCY",
  routingHints: {
    preferredEndpoints: ["emergency_fhir", "emergency_dashboard"],
    routingReasons: ["Emergency patient detected"],
    transformationHints: ["include_emergency_flags"]
  }
}
```

### Step 2: Routing Rule Evaluation
1. **Emergency_Patients Rule**:
   - Condition: `patientClass == "E"` ✅ MATCH
   - Priority: 1 (highest)
   - Selected endpoints: ["emergency_fhir", "analytics_warehouse"]

2. **Main_Hospital_Patients Rule**:
   - Condition: `facility.contains("MAIN_HOSPITAL")` ✅ MATCH
   - Priority: 2 (lower)
   - Not selected due to higher priority rule match

### Step 3: Final Routing Decision
```javascript
{
  messageId: "MSG123456",
  selectedEndpoints: [
    {
      id: "emergency_fhir",
      name: "Emergency FHIR Server",
      url: "https://emergency.hospital.org/fhir",
      resource_endpoint: "Patient"
    },
    {
      id: "analytics_warehouse",
      name: "Real-time Analytics",
      type: "database"
    }
  ],
  transformationRequired: true,
  transformationConfig: {
    sourceFormat: "HL7",
    targetFormat: "FHIR",
    messageType: "ADT^A01",
    targetResourceType: "Patient",
    templateName: "ADT^A01_to_FHIR_Patient"
  },
  routingReason: "Content-based routing - matched rule: Emergency_Patients (patientClass equals E)"
}
```

## Delivery Flow

### Parallel Delivery to Multiple Endpoints

1. **Emergency FHIR Server** (Priority 1):
   - Transform HL7 → FHIR Patient resource
   - POST to `https://emergency.hospital.org/fhir/Patient`
   - Include emergency flags in FHIR resource
   - Timeout: 5 seconds (high priority)

2. **Analytics Warehouse** (Priority 3):
   - Transform HL7 → analytics schema
   - INSERT into real-time analytics database
   - Async delivery (doesn't block primary)
   - Timeout: 30 seconds

## Smart Routing Benefits Demonstrated

1. **Content Awareness**: Automatically detected emergency patient and routed to emergency systems
2. **Multi-Endpoint**: Simultaneously sends to emergency FHIR and analytics
3. **Priority Handling**: Emergency endpoint gets priority treatment
4. **Transformation Intelligence**: Uses emergency-specific transformation template
5. **Failover Ready**: If emergency FHIR is down, could fall back to main FHIR
6. **Performance**: Analytics delivery is async and doesn't block critical path

## Alternative Scenarios

### Scenario 2: Regular Inpatient
```
PV1|1|I|MED^201^A^MAIN_HOSPITAL  // Inpatient (I), Medical unit
```
**Result**: Routes to main_fhir + analytics_warehouse

### Scenario 3: Outpatient Lab
```
MSH|^~\&|LAB|CLINIC_WEST
PV1|1|O|LAB^101^1^CLINIC_WEST    // Outpatient (O), Clinic
```
**Result**: Routes to clinic_fhir (if configured) or main_fhir fallback

### Scenario 4: Load Balanced Scenario
```json
"routing_strategy": "load_balanced",
"load_balancing": "weighted_round_robin"
```
**Result**: Distributes messages across endpoints based on weights

This demonstrates how the smart routing system provides intelligent, content-aware message routing that can handle complex healthcare integration scenarios automatically.