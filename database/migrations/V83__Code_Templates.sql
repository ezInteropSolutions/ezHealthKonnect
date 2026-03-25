-- V83: Code Templates
-- Named JavaScript function libraries that are automatically injected into
-- every script step's goja VM at execution time (inspired by Mirth Code Templates).
-- Scope: 'global' = injected into all interfaces; 'interface' = only when linked.

CREATE TABLE IF NOT EXISTS code_templates (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 VARCHAR(255) NOT NULL UNIQUE,
    description          TEXT,
    category             VARCHAR(100) NOT NULL DEFAULT 'general',
    -- Allowed values: 'general' | 'hl7' | 'fhir' | 'date_time' | 'validation' | 'string'
    code                 TEXT         NOT NULL,
    function_signatures  JSONB        NOT NULL DEFAULT '[]',
    -- Array of human-readable signatures, e.g. ["formatHL7Date(hl7Date)", "ageFromHL7DOB(hl7Date)"]
    -- Used by the UI to show available functions without parsing JS
    scope                VARCHAR(20)  NOT NULL DEFAULT 'global',
    -- 'global'    = auto-inject into every script step across all interfaces
    -- 'interface' = only injected when linked via code_template_interface_links
    is_system            BOOLEAN      NOT NULL DEFAULT false,
    -- System templates are read-only (non-deletable, code not editable by users)
    is_active            BOOLEAN      NOT NULL DEFAULT true,
    sort_order           INTEGER      NOT NULL DEFAULT 100,
    -- Lower sort_order = earlier in preamble (dependencies should come first)
    usage_count          INTEGER      NOT NULL DEFAULT 0,
    created_by_user_id   UUID         REFERENCES users(id) ON DELETE SET NULL,
    created_at           TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at           TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Interface-scoped template links
-- A row here means: inject this template when executing scripts for this interface
CREATE TABLE IF NOT EXISTS code_template_interface_links (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id  UUID NOT NULL REFERENCES code_templates(id) ON DELETE CASCADE,
    interface_id UUID NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(template_id, interface_id)
);

CREATE INDEX IF NOT EXISTS idx_code_templates_scope    ON code_templates(scope, is_active);
CREATE INDEX IF NOT EXISTS idx_code_templates_category ON code_templates(category);
CREATE INDEX IF NOT EXISTS idx_code_templates_system   ON code_templates(is_system);
CREATE INDEX IF NOT EXISTS idx_ct_iface_links_iface    ON code_template_interface_links(interface_id);
CREATE INDEX IF NOT EXISTS idx_ct_iface_links_template ON code_template_interface_links(template_id);

-- Auto-update updated_at (reuses existing trigger function from V1 migration)
CREATE TRIGGER trg_code_templates_updated_at
    BEFORE UPDATE ON code_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================
-- OOB SYSTEM TEMPLATES (6 built-in libraries)
-- These are seeded here AND in Go's SeedSystemTemplates() which
-- uses ON CONFLICT DO NOTHING, so both paths are idempotent.
-- ============================================================

INSERT INTO code_templates (name, description, category, scope, is_system, is_active, sort_order, code, function_signatures)
VALUES

-- 1. HL7 Date Utilities
('hl7_date_utils',
 'Date and time helpers for HL7 date strings (YYYYMMDD / YYYYMMDDHHmmss)',
 'date_time', 'global', true, true, 10,
 $CODE$
// Converts an HL7 date (YYYYMMDD or YYYYMMDDHHmmss) to ISO 8601 string
function formatHL7Date(hl7Date) {
    if (!hl7Date || String(hl7Date).length < 8) return null;
    var s = String(hl7Date);
    var y = s.slice(0,4), m = s.slice(4,6), d = s.slice(6,8);
    if (s.length >= 14) {
        return y+'-'+m+'-'+d+'T'+s.slice(8,10)+':'+s.slice(10,12)+':'+s.slice(12,14);
    }
    return y+'-'+m+'-'+d;
}

// Returns age in full years from an HL7 DOB string
function ageFromHL7DOB(hl7Date) {
    var iso = formatHL7Date(hl7Date);
    if (!iso) return null;
    var birth = new Date(iso), now = new Date();
    var age = now.getFullYear() - birth.getFullYear();
    var bm = birth.getMonth(), bd = birth.getDate();
    if (now.getMonth() < bm || (now.getMonth() === bm && now.getDate() < bd)) age--;
    return age;
}

// Returns current timestamp as HL7 format (YYYYMMDDHHmmss)
function nowAsHL7DateTime() {
    var now = new Date();
    var pad = function(n) { return String(n).padStart(2,'0'); };
    return String(now.getFullYear()) + pad(now.getMonth()+1) + pad(now.getDate()) +
           pad(now.getHours()) + pad(now.getMinutes()) + pad(now.getSeconds());
}
$CODE$,
 '["formatHL7Date(hl7Date)", "ageFromHL7DOB(hl7Date)", "nowAsHL7DateTime()"]'::jsonb),

-- 2. HL7 Field Helpers
('hl7_field_helpers',
 'Convenience functions for reading common HL7 patient demographics',
 'hl7', 'global', true, true, 20,
 $CODE$
// Safely reads an HL7 field value using short notation (e.g. "PID.5.1")
function hl7Get(path) {
    return getHL7Field(path) || null;
}

// Returns patient full name as "LAST, FIRST MI"
function getPatientName() {
    var last  = hl7Get('PID.5.1') || '';
    var first = hl7Get('PID.5.2') || '';
    var mi    = hl7Get('PID.5.3') || '';
    var parts = [];
    if (last)  parts.push(last + (first || mi ? ',' : ''));
    if (first) parts.push(first);
    if (mi)    parts.push(mi);
    return parts.join(' ').trim();
}

// Returns the patient MRN (PID.3.1 — first identifier in the CX list)
function getPatientMRN() {
    return hl7Get('PID.3.1') || hl7Get('PID.3') || null;
}

// Returns patient date of birth as ISO date string
function getPatientDOB() {
    return formatHL7Date(hl7Get('PID.7'));
}

// Returns patient gender code (M/F/U/O)
function getPatientGender() {
    return hl7Get('PID.8') || 'U';
}
$CODE$,
 '["hl7Get(path)", "getPatientName()", "getPatientMRN()", "getPatientDOB()", "getPatientGender()"]'::jsonb),

-- 3. FHIR Resource Builders
('fhir_resource_builders',
 'Helpers to build common FHIR R4 resource fragments from HL7 data',
 'fhir', 'global', true, true, 30,
 $CODE$
// Builds a FHIR R4 HumanName object from HL7 PID.5 components
function buildFHIRHumanName() {
    var given = [hl7Get('PID.5.2'), hl7Get('PID.5.3')].filter(Boolean);
    return {
        use: 'official',
        family: hl7Get('PID.5.1') || undefined,
        given: given.length > 0 ? given : undefined
    };
}

// Builds a FHIR Identifier object
function buildFHIRIdentifier(value, system, use) {
    return { use: use || 'usual', system: system || '', value: String(value || '') };
}

// Builds a FHIR CodeableConcept from a code + display + optional system URI
function buildFHIRCodeableConcept(code, display, system) {
    return {
        coding: [{ system: system || '', code: String(code || ''), display: display || '' }],
        text: display || String(code || '')
    };
}

// Wraps FHIR resources into a Bundle of type 'collection'
function buildFHIRBundle(resources) {
    return {
        resourceType: 'Bundle',
        type: 'collection',
        entry: (resources || []).map(function(r) { return { resource: r }; })
    };
}
$CODE$,
 '["buildFHIRHumanName()", "buildFHIRIdentifier(value, system, use)", "buildFHIRCodeableConcept(code, display, system)", "buildFHIRBundle(resources)"]'::jsonb),

-- 4. String Utilities
('string_utils',
 'General-purpose string manipulation helpers',
 'string', 'global', true, true, 40,
 $CODE$
function padLeft(str, len, ch)  { return String(str || '').padStart(len, ch || ' '); }
function padRight(str, len, ch) { return String(str || '').padEnd(len, ch || ' '); }
function capitalize(str)  { var s = String(str||''); return s.charAt(0).toUpperCase()+s.slice(1).toLowerCase(); }
function trim(str)        { return String(str || '').trim(); }
function isBlank(str)     { return !str || String(str).trim() === ''; }
function truncate(str, n) { var s = String(str||''); return s.length > n ? s.slice(0,n)+'...' : s; }

// Safely coerce a value to string; returns empty string for null/undefined
function safeStr(val) { return val == null ? '' : String(val); }
$CODE$,
 '["padLeft(str, len, ch)", "padRight(str, len, ch)", "capitalize(str)", "trim(str)", "isBlank(str)", "truncate(str, n)", "safeStr(val)"]'::jsonb),

-- 5. Validation Utilities
('validation_utils',
 'Field-level validation helpers for healthcare identifiers and HL7 values',
 'validation', 'global', true, true, 50,
 $CODE$
function isValidMRN(mrn) { return /^[A-Z0-9\-]{3,20}$/i.test(String(mrn || '')); }
function isValidNPI(npi) { return /^\d{10}$/.test(String(npi || '')); }
function isValidHL7Date(d) { return /^\d{8}(\d{6})?$/.test(String(d || '')); }
function isValidSSN(ssn) { return /^\d{3}-?\d{2}-?\d{4}$/.test(String(ssn || '')); }

// Throws an error if the value is blank, with a descriptive field name
function requireField(value, fieldName) {
    if (isBlank(value)) throw new Error('Required field missing: ' + fieldName);
    return value;
}

// Returns true if value is one of the allowed HL7 gender codes
function isValidGender(code) {
    return ['M', 'F', 'O', 'U', 'A', 'N'].indexOf(String(code||'').toUpperCase()) !== -1;
}
$CODE$,
 '["isValidMRN(mrn)", "isValidNPI(npi)", "isValidHL7Date(d)", "isValidSSN(ssn)", "requireField(value, fieldName)", "isValidGender(code)"]'::jsonb),

-- 6. Message Context Helpers
('message_context_helpers',
 'Quick access to common MSH header fields and message-level metadata',
 'general', 'global', true, true, 60,
 $CODE$
// Returns the HL7 message type as "TYPE^EVENT" (e.g. "ADT^A01")
function getMessageType() {
    var type  = hl7Get('MSH.9.1') || '';
    var event = hl7Get('MSH.9.2') || '';
    return event ? type + '^' + event : type;
}

// Returns the sending application name (MSH.3)
function getSendingApp() { return hl7Get('MSH.3') || ''; }

// Returns the sending facility (MSH.4)
function getSendingFacility() { return hl7Get('MSH.4') || ''; }

// Returns the receiving application (MSH.5)
function getReceivingApp() { return hl7Get('MSH.5') || ''; }

// Returns the message control ID (MSH.10) for correlation
function getMessageControlID() { return hl7Get('MSH.10') || ''; }
$CODE$,
 '["getMessageType()", "getSendingApp()", "getSendingFacility()", "getReceivingApp()", "getMessageControlID()"]'::jsonb)

ON CONFLICT (name) DO NOTHING;
