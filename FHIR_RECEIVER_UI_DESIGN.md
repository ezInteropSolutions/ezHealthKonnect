# FHIR Receiver UI/UX Architecture Design

**Date**: October 26, 2025
**Architect**: Claude (AI Solution Architect)
**Scope**: FHIR Receiver Interface Design + Future FHIR Store Integration
**Status**: DESIGN PROPOSAL

---

## Executive Summary

Design a flexible, intuitive UI for FHIR receiver interfaces that:
1. Supports ALL FHIR resource types (150+ resources)
2. Handles bundle vs individual resource delivery
3. Provides clear configuration wizard
4. Scales to paid FHIR Store subscription (future)
5. Maintains consistency with existing interface wizard

---

## User Personas & Use Cases

### Persona 1: Integration Engineer (Primary)
**Goals**:
- Receive HL7 → transform to FHIR → send to downstream system
- Configure which resources to accept/reject
- Control bundle vs individual delivery
- Monitor delivery success/failure

**Use Cases**:
- UC1: Send Patient + Encounter as Bundle to Epic
- UC2: Send individual Patient resources to local FHIR server
- UC3: Filter only Patient/Observation to analytics platform
- UC4: Transform and store in ezHealthKonnect FHIR Store (future)

### Persona 2: Healthcare Administrator (Secondary)
**Goals**:
- Monitor FHIR data flow
- Audit FHIR resource delivery
- Troubleshoot failed deliveries
- Manage subscription (FHIR Store)

---

## Design Principles

### 1. Progressive Disclosure
- Simple defaults for 90% of users
- Advanced options hidden until needed
- Clear "why do I need this?" contextual help

### 2. Visual Hierarchy
- Primary actions prominent (Add Resource Type, Save)
- Secondary actions accessible (Advanced Settings)
- Tertiary actions hidden (Raw JSON Config)

### 3. Error Prevention
- Validation before save
- Clear warnings for breaking changes
- Test connection before activation

### 4. Consistency
- Match existing interface wizard patterns
- Reuse UI components (cards, forms, buttons)
- Same navigation flow

---

## UI Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                   Interface Creation Wizard                      │
└─────────────────────────────────────────────────────────────────┘
              │
              ├─> Step 1: Interface Type
              │   └─> Select: Inbound or Outbound
              │
              ├─> Step 2: Connectivity
              │   ├─> Inbound: TCP/MLLP, HTTP/REST, File, Database, Cloud
              │   └─> Outbound: FHIR Receiver (NEW), HTTP, Database, Cloud
              │
              ├─> Step 3: Message Format (if inbound)
              │   └─> HL7 v2.x, FHIR, JSON, XML, CSV
              │
              ├─> Step 4: Transformation (if HL7 → FHIR)
              │   └─> Existing mapping wizard
              │
              ├─> Step 5: FHIR Output Configuration (NEW)
              │   ├─> FHIR Server Type
              │   ├─> Resource Selection
              │   ├─> Bundle vs Individual
              │   └─> Advanced Settings
              │
              └─> Step 6: Review & Activate
```

---

## FHIR Output Configuration UI (Step 5 - NEW)

### Recommended Design: Category-Based Resource Selection

**Visual Layout**:
```
┌──────────────────────────────────────────────────────────────────┐
│  Step 5 of 6: FHIR Output Configuration                   [?] │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  1. FHIR DESTINATION                                             │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Destination Type: [▼ FHIR Server (HTTP/HTTPS)           ] │ │
│  │                                                             │ │
│  │ FHIR Version:     [▼ FHIR R4                            ] │ │
│  │ Base URL:         [https://fhir.example.com/fhir/r4     ] │ │
│  │                   🔗 Test Connection                        │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  2. DELIVERY MODE                                                │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  ● Bundle (recommended)        ○ Individual Resources      │ │
│  │  ℹ️  Bundle = single transaction, faster                   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  3. RESOURCE SELECTION                                           │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Quick Presets:                                              │ │
│  │ [📋 Essential] [🩺 Clinical] [📚 Comprehensive] [☑️ All]   │ │
│  │                                                              │ │
│  │ Selected: 8 resource types  [View All ▼]                    │ │
│  │                                                              │ │
│  │ ✓ Patient          ✓ Encounter       ✓ Observation         │ │
│  │ ✓ Condition        ✓ Procedure       ✓ MedicationRequest   │ │
│  │ ✓ AllergyIntolerance  ✓ MessageHeader                      │ │
│  │                                                              │ │
│  │ [+ Add Resources]  [⚙️ Configure All...]                    │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  [⚙️ Advanced Settings ▼]                                        │
│                                                                   │
│  [◀ Previous]                              [Next: Review ▶]     │
└──────────────────────────────────────────────────────────────────┘
```

**Key Components**:

1. **FHIR Destination Selector** - Dropdown with preset templates
2. **Delivery Mode Toggle** - Visual button group with explanation
3. **Resource Selection** - Category-based accordion with presets
4. **Advanced Settings** - Collapsed panel for auth, retry, timeout

---

## Detailed Component Designs

### 1. Resource Selection Modal (When "Configure All" Clicked)

```
┌──────────────────────────────────────────────────────────────────────┐
│  Select FHIR Resources to Send                            [X] Close │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  Search: [🔍 Type to filter resources...              ]              │
│                                                                       │
│  Categories:  [All (150)] [Selected (8)] [Core (10)] [Clinical (30)]│
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ 👤 Core Resources (8 / 10 selected)                  [- Collapse]││
│  ├─────────────────────────────────────────────────────────────────┤│
│  │  ✓ Patient           Demographics, identifiers                  ││
│  │  ✓ Practitioner      Doctors, nurses, providers                 ││
│  │  ✓ Organization      Hospitals, clinics                         ││
│  │  □ Location          Physical places                            ││
│  │  □ HealthcareService  Services offered                          ││
│  └─────────────────────────────────────────────────────────────────┘│
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ 🩺 Clinical Resources (5 / 30 selected)              [+ Expand] ││
│  └─────────────────────────────────────────────────────────────────┘│
│                                                                       │
│  Summary: 8 resources selected                                       │
│  [Cancel]                                      [Apply Selection]     │
└──────────────────────────────────────────────────────────────────────┘
```

**Resource Categories** (10 groups):
1. 👤 **Core** (10): Patient, Practitioner, Organization, Location
2. 🩺 **Clinical** (30): Observation, Condition, Procedure, DiagnosticReport
3. 💊 **Medications** (8): Medication, MedicationRequest, MedicationDispense
4. 🧬 **Diagnostic** (12): Specimen, ImagingStudy, BodyStructure
5. 💰 **Financial** (10): Claim, Coverage, ExplanationOfBenefit
6. 📋 **Administrative** (15): Appointment, Schedule, Slot
7. 🔄 **Workflow** (8): Task, ServiceRequest, Communication
8. 🔐 **Security** (5): Consent, Provenance, AuditEvent
9. 📊 **Reporting** (8): Measure, MeasureReport, Library
10. 🌐 **Foundation** (44): Bundle, MessageHeader, OperationOutcome

---

### 2. Bundle vs Individual Comparison

```
┌──────────────────────────────────────────────────────────┐
│  Delivery Mode                                           │
├──────────────────────────────────────────────────────────┤
│                                                           │
│  ┌──────────────────┐   ┌──────────────────┐            │
│  │  📦 BUNDLE       │   │  📄 INDIVIDUAL   │            │
│  │  ● Selected      │   │  ○ Not selected  │            │
│  │                  │   │                  │            │
│  │  Single API call │   │  Multiple calls  │            │
│  │  All-or-nothing  │   │  Partial success │            │
│  │  ✅ Recommended  │   │  ⚠️  Slower      │            │
│  └──────────────────┘   └──────────────────┘            │
│                                                           │
│  [Compare in detail...]                                  │
└──────────────────────────────────────────────────────────┘
```

**Comparison Table** (when "Compare in detail" clicked):

| Feature | Bundle | Individual |
|---------|--------|------------|
| Transaction Safety | ✅ All-or-nothing | ⚠️ Partial success possible |
| Performance | ✅ 1 HTTP request | ❌ N HTTP requests |
| Compatibility | ✅ Most FHIR servers | ✅ All FHIR servers |
| Error Handling | ❌ Full rollback on error | ✅ Partial delivery possible |
| Best For | Production systems | Legacy/limited servers |

---

### 3. FHIR Store Integration (Future - Premium)

```
┌──────────────────────────────────────────────────────────────┐
│  ⭐ Upgrade to ezHealthKonnect FHIR Store                   │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  Fully managed FHIR repository with built-in analytics       │
│                                                               │
│  ✅ Unlimited storage        ✅ Built-in analytics           │
│  ✅ High-speed querying      ✅ Automatic backups            │
│  ✅ HIPAA/GDPR compliant     ✅ No server management         │
│                                                               │
│  Pricing: $299/month + $0.10 per 1,000 resources             │
│                                                               │
│  [Start 14-Day Free Trial (No credit card required)]         │
│                                                               │
│  Learn more about FHIR Store ↗                               │
└──────────────────────────────────────────────────────────────┘
```

---

## Message Monitoring UI

### FHIR Message Detail View

```
┌────────────────────────────────────────────────────────────────┐
│  Message: MSG_DELIVERY_TEST_001          ✅ Delivered          │
├────────────────────────────────────────────────────────────────┤
│  [Summary] [FHIR Resources (3)] [Bundle] [Delivery Log] [Raw] │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  FHIR Resources Sent                                           │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ 👤 Patient: TestPatient, Delivery T                      │  │
│  │    ID: patient-1761476880948980359         ✅ Delivered  │  │
│  │    Gender: F  |  DOB: 1990-01-01  |  MRN: 67890         │  │
│  │    [View JSON ▼]                                         │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ 🏥 Encounter: Outpatient ER Visit                        │  │
│  │    ID: encounter-1761476880963313299       ✅ Delivered  │  │
│  │    Location: ER^101^1  |  Priority: URGENT              │  │
│  │    [View JSON ▼]                                         │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ 📨 MessageHeader: ADT^A01                                │  │
│  │    ID: MSG_DELIVERY_TEST_001                ✅ Delivered  │  │
│  │    Timestamp: 2025-10-26T11:08:00Z                       │  │
│  │    [View JSON ▼]                                         │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  Delivery Summary                                              │
│  Endpoint: http://localhost:8081/fhir/r4/                     │
│  Bundle Type: transaction                                      │
│  Response Time: 245ms                                          │
│  Status: HTTP 201 Created                                      │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
```

---

## Configuration JSON Structure

```json
{
  "destination_type": "fhir_http",
  "fhir_version": "R4",
  "base_url": "https://fhir.example.com/fhir/r4",

  "delivery_mode": "bundle",
  "bundle_type": "transaction",

  "resource_selection": {
    "mode": "whitelist",
    "resources": [
      "Patient",
      "Encounter",
      "Observation",
      "Condition",
      "Procedure",
      "MedicationRequest",
      "AllergyIntolerance",
      "MessageHeader"
    ]
  },

  "resource_filtering": {
    "exclude_empty": true,
    "validate_before_send": true
  },

  "authentication": {
    "type": "bearer_token",
    "token": "encrypted_token"
  },

  "retry_config": {
    "max_attempts": 3,
    "initial_delay_ms": 1000
  },

  "timeout_config": {
    "connect_timeout_ms": 30000,
    "read_timeout_ms": 60000
  }
}
```

---

## Implementation Roadmap

### Phase 1: Core FHIR Output (Week 1-2) - PRIORITY
✅ FHIR destination selector UI
✅ Resource selection (preset-based)
✅ Bundle vs Individual toggle
✅ Basic authentication (Bearer Token)
✅ Test connection button
✅ Configuration save/load

### Phase 2: Resource Management (Week 3)
⏳ Category-based resource selection
⏳ Search/filter resources
⏳ Resource preview cards
⏳ Validation before send

### Phase 3: Monitoring (Week 4)
⏳ FHIR message detail view
⏳ Resource-level delivery tracking
⏳ Bundle viewer with validation

### Phase 4: FHIR Store Integration (Month 2)
⏳ Subscription management UI
⏳ FHIR Store configuration
⏳ Analytics dashboard

---

## Key Recommendations

### 1. Start Simple, Scale Up
- **MVP**: Essential preset (8 resources) + Bundle mode + Basic auth
- **Phase 2**: Add full resource selection + Individual mode
- **Phase 3**: Add FHIR Store premium feature

### 2. Use Existing Patterns
- Reuse wizard navigation from current interface wizard
- Match card designs from existing UI
- Keep same color scheme and typography

### 3. Mobile-First Design
- Resource cards stack vertically on mobile
- Collapsible categories by default
- Bottom sheet for resource selection modal

### 4. Performance Optimizations
- Virtual scrolling for 150+ resources
- Lazy load resource metadata
- Debounced search (300ms)
- Progressive disclosure

### 5. Error Prevention
- Test connection before saving
- Validate FHIR endpoint URL format
- Check resource compatibility with FHIR version
- Warn if bundle not supported by server

---

## Mockup Tools & Next Steps

**Recommended Tools**:
1. Figma - High-fidelity mockups
2. InVision - Interactive prototype
3. UserTesting - User feedback

**Next Steps**:
1. Create high-fidelity mockups (Figma)
2. User testing with 5 integration engineers
3. Iterate based on feedback
4. Build Phase 1 MVP

---

## Conclusion

This UI design provides:

✅ **Simplicity** - Defaults work for 90% of users
✅ **Power** - Advanced users can customize everything
✅ **Scalability** - Easy to add new resources/destinations
✅ **Monetization** - Clear premium upgrade path
✅ **Consistency** - Matches existing wizard patterns
✅ **Flexibility** - Bundle vs Individual, all resource types

**Recommendation**: Start with Phase 1 (Core FHIR Output) using category-based resource selection with presets. This gives immediate value while laying foundation for premium features.

---

**Last Updated**: October 26, 2025
**Status**: DESIGN PROPOSAL - Ready for Implementation
**Next Step**: Create Figma mockups + Begin Phase 1 development
