/**
 * FhirExtensionCatalog
 *
 * Static catalog of common FHIR R4 extensions, organised by category.
 * Each entry carries enough metadata to build the extension path, suggest
 * the correct value[x] type, and pre-select the right transform.
 *
 * HL7 field hints: when the user picks a source field whose segment.field
 * matches an entry in HL7_HINTS, that extension surfaces at the top of the
 * catalog as "Suggested".
 *
 * No API dependency — the catalog is bundled with the app.  Users can add
 * custom extensions via the ExtensionBuilder UI; those are stored in
 * localStorage under the key defined by CUSTOM_STORAGE_KEY.
 */
class FhirExtensionCatalog {

    /**
     * Returns all extensions in the catalog, grouped by category.
     * Custom extensions (saved by the user) are merged into their own group.
     * @returns {Array<{ category:string, extensions:Array<ExtensionEntry> }>}
     */
    static getGroups() {
        var groups = JSON.parse(JSON.stringify(FhirExtensionCatalog.GROUPS));
        var custom = FhirExtensionCatalog.getCustomExtensions();
        if (custom.length > 0) {
            groups.unshift({ category: 'My Extensions', icon: '⚙️', extensions: custom });
        }
        return groups;
    }

    /**
     * Returns all extensions as a flat array (excluding custom).
     * @returns {Array<ExtensionEntry>}
     */
    static getAll() {
        var all = [];
        FhirExtensionCatalog.GROUPS.forEach(function(g) {
            all = all.concat(g.extensions);
        });
        return all;
    }

    /**
     * Returns extensions suggested for the given HL7 field path.
     * @param {string} hl7Field  e.g. "PD1.21", "PID.10"
     * @returns {Array<ExtensionEntry>}
     */
    static getSuggestions(hl7Field) {
        if (!hl7Field) return [];
        var key = hl7Field.toUpperCase().trim();

        // Exact match first, then segment-only fallback
        var ids = FhirExtensionCatalog.HL7_HINTS[key] || [];
        if (ids.length === 0) {
            var seg = key.split('.')[0];
            ids = FhirExtensionCatalog.HL7_HINTS[seg] || [];
        }
        if (ids.length === 0) return [];

        var allExt = FhirExtensionCatalog.getAll().concat(FhirExtensionCatalog.getCustomExtensions());
        return ids.map(function(id) {
            return allExt.find(function(e) { return e.id === id; });
        }).filter(Boolean);
    }

    /**
     * Searches the catalog by name, description, or URL (case-insensitive).
     * @param {string} query
     * @returns {Array<ExtensionEntry>}
     */
    static search(query) {
        if (!query || !query.trim()) return FhirExtensionCatalog.getAll();
        var q = query.toLowerCase();
        return FhirExtensionCatalog.getAll()
            .concat(FhirExtensionCatalog.getCustomExtensions())
            .filter(function(e) {
                return e.name.toLowerCase().includes(q)
                    || (e.description || '').toLowerCase().includes(q)
                    || e.url.toLowerCase().includes(q);
            });
    }

    /**
     * Returns a single extension entry by ID, or null.
     * @param {string} id
     * @returns {ExtensionEntry|null}
     */
    static getById(id) {
        return FhirExtensionCatalog.getAll()
            .concat(FhirExtensionCatalog.getCustomExtensions())
            .find(function(e) { return e.id === id; }) || null;
    }

    // ── Custom extension persistence (localStorage) ─────────────────────

    static getCustomExtensions() {
        try {
            var raw = localStorage.getItem(FhirExtensionCatalog.CUSTOM_STORAGE_KEY);
            return raw ? JSON.parse(raw) : [];
        } catch (e) {
            return [];
        }
    }

    static saveCustomExtension(entry) {
        var existing = FhirExtensionCatalog.getCustomExtensions();
        var idx = existing.findIndex(function(e) { return e.id === entry.id || e.url === entry.url; });
        if (idx >= 0) {
            existing[idx] = entry;
        } else {
            existing.push(entry);
        }
        localStorage.setItem(FhirExtensionCatalog.CUSTOM_STORAGE_KEY, JSON.stringify(existing));
    }

    static deleteCustomExtension(id) {
        var existing = FhirExtensionCatalog.getCustomExtensions().filter(function(e) { return e.id !== id; });
        localStorage.setItem(FhirExtensionCatalog.CUSTOM_STORAGE_KEY, JSON.stringify(existing));
    }

    /**
     * Generates a FHIR path string from an extension entry and a value type.
     * @param {string} resourceType  e.g. "Patient"
     * @param {string} url           extension URL
     * @param {string} valueType     e.g. "valueBoolean"
     * @returns {string}
     */
    static buildPath(resourceType, url, valueType) {
        var base = resourceType ? resourceType + '.extension' : 'extension';
        return base + '[url=' + url + '].' + (valueType || 'valueString');
    }

    /**
     * Returns all value[x] options for the value type picker.
     * @returns {Array<{value:string, label:string, description:string}>}
     */
    static getValueTypes() {
        return FhirExtensionCatalog.VALUE_TYPES;
    }

    /**
     * Suggests the best value[x] type given an HL7 data type.
     * @param {string} hl7DataType  e.g. "IS", "NM", "TS"
     * @returns {string}  e.g. "valueBoolean"
     */
    static suggestValueType(hl7DataType) {
        return FhirExtensionCatalog.HL7_TYPE_TO_VALUE_TYPE[hl7DataType.toUpperCase()] || 'valueString';
    }

    /**
     * Suggests the best transform key given an HL7 data type and value type.
     * @param {string} hl7DataType
     * @param {string} valueType
     * @returns {string}
     */
    static suggestTransform(hl7DataType, valueType) {
        var key = hl7DataType.toUpperCase() + ':' + valueType;
        return FhirExtensionCatalog.TRANSFORM_HINTS[key]
            || FhirExtensionCatalog.TRANSFORM_HINTS[hl7DataType.toUpperCase()]
            || '';
    }
}

// ── Post-class initialization ─────────────────────────────────────────────

FhirExtensionCatalog.CUSTOM_STORAGE_KEY = 'ehk_custom_extensions';

/**
 * Value [x] type options shown in the ExtensionBuilder dropdown.
 * @type {Array<{value:string, label:string, description:string}>}
 */
FhirExtensionCatalog.VALUE_TYPES = [
    { value: 'valueBoolean',          label: 'Boolean',          description: 'true / false — use for Y/N indicator fields' },
    { value: 'valueString',           label: 'String',           description: 'Plain text — use for free-text fields' },
    { value: 'valueCode',             label: 'Code',             description: 'Coded value from a value set' },
    { value: 'valueInteger',          label: 'Integer',          description: 'Whole number' },
    { value: 'valueDecimal',          label: 'Decimal',          description: 'Numeric with decimal precision' },
    { value: 'valueDate',             label: 'Date',             description: 'Date only (YYYY-MM-DD)' },
    { value: 'valueDateTime',         label: 'DateTime',         description: 'Date and time (ISO 8601)' },
    { value: 'valueUri',              label: 'URI',              description: 'Uniform Resource Identifier' },
    { value: 'valueCoding',           label: 'Coding',           description: 'code + system + display — use for CE/CWE fields' },
    { value: 'valueCodeableConcept',  label: 'CodeableConcept',  description: 'coding array + text — use for coded concepts' },
    { value: 'valueQuantity',         label: 'Quantity',         description: 'Numeric value with units' },
    { value: 'valueReference',        label: 'Reference',        description: 'Reference to another FHIR resource' },
];

/**
 * HL7 data type → suggested value[x] type mapping.
 * Used to pre-select the value type in the popup.
 */
FhirExtensionCatalog.HL7_TYPE_TO_VALUE_TYPE = {
    // Y/N flag types → Boolean
    'ID': 'valueBoolean',
    'IS': 'valueBoolean',
    // String types
    'ST': 'valueString',
    'FT': 'valueString',
    'TX': 'valueString',
    // Numeric
    'NM': 'valueDecimal',
    'SI': 'valueInteger',
    // Dates
    'DT': 'valueDate',
    'TS': 'valueDateTime',
    'DTM': 'valueDateTime',
    // Coded types
    'CE': 'valueCodeableConcept',
    'CWE': 'valueCodeableConcept',
    'CNE': 'valueCodeableConcept',
    'CX': 'valueString',
    // URI / reference
    'RP': 'valueUri',
};

/**
 * HL7DataType[:valueType] → transform key suggestions.
 */
FhirExtensionCatalog.TRANSFORM_HINTS = {
    'ID:valueBoolean':          'hl7_active_flag',
    'IS:valueBoolean':          'hl7_active_flag',
    'ID':                       'hl7_active_flag',
    'IS':                       'hl7_active_flag',
    'CE:valueCodeableConcept':  'ce_to_codeableconcept',
    'CWE:valueCodeableConcept': 'ce_to_codeableconcept',
    'CNE:valueCodeableConcept': 'ce_to_codeableconcept',
    'TS:valueDateTime':         'ts_to_datetime',
    'DTM:valueDateTime':        'ts_to_datetime',
    'DT:valueDate':             'ts_to_date',
};

/**
 * HL7 field path → extension ID suggestions.
 * Keys are uppercase segment.field paths (e.g. "PD1.21") or segment-only
 * fallbacks (e.g. "ZPD").
 */
FhirExtensionCatalog.HL7_HINTS = {
    'PD1.21':  ['organ-donor'],
    'PID.10':  ['us-core-race'],
    'PID.22':  ['us-core-ethnicity'],
    'PD1.17':  ['us-core-birth-sex'],
    'PID.8':   ['us-core-birth-sex'],
    'NK1.2':   ['mothers-maiden-name'],
    'PID.23':  ['birth-place'],
    'PID.26':  ['citizenship'],
    'PID.27':  ['nationality'],
    'PD1.3':   ['preferred-language'],
    'PID.15':  ['preferred-language'],
    'PD1.4':   ['patient-deceased'],
    'PID.29':  ['patient-deceased'],
    // Z-segments — always suggest defining a custom extension
    'ZPD':     [],
    'ZIN':     [],
    'ZEN':     [],
};

/**
 * Extension catalog grouped by category.
 * @type {Array<{ category:string, icon:string, extensions:Array<ExtensionEntry> }>}
 *
 * Each ExtensionEntry:
 *   id           {string}   Stable identifier used in HL7_HINTS
 *   name         {string}   Human-readable name shown in the picker
 *   url          {string}   Full extension URL written to FHIR
 *   valueType    {string}   Default value[x] type
 *   transform    {string}   Default transform key (may be overridden)
 *   description  {string}   One-line description shown as tooltip / sub-label
 *   resources    {string[]} FHIR resources this extension applies to
 *   system       {string}   For coded extensions — default system URI (optional)
 */
FhirExtensionCatalog.GROUPS = [
    {
        category: 'US Core Patient',
        icon: '🇺🇸',
        extensions: [
            {
                id: 'us-core-race',
                name: 'Race',
                url: 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-race',
                valueType: 'valueCodeableConcept',
                transform: 'ce_to_codeableconcept',
                description: 'US Core race category (OMB categories)',
                resources: ['Patient'],
                system: 'urn:oid:2.16.840.1.113883.6.238',
            },
            {
                id: 'us-core-ethnicity',
                name: 'Ethnicity',
                url: 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-ethnicity',
                valueType: 'valueCodeableConcept',
                transform: 'ce_to_codeableconcept',
                description: 'US Core ethnicity category (OMB categories)',
                resources: ['Patient'],
                system: 'urn:oid:2.16.840.1.113883.6.238',
            },
            {
                id: 'us-core-birth-sex',
                name: 'Birth Sex',
                url: 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-birthsex',
                valueType: 'valueCode',
                transform: '',
                description: 'Sex recorded at birth (M / F / OTH / UNK)',
                resources: ['Patient'],
                system: 'http://terminology.hl7.org/CodeSystem/v3-AdministrativeGender',
            },
            {
                id: 'us-core-genderIdentity',
                name: 'Gender Identity',
                url: 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-genderIdentity',
                valueType: 'valueCodeableConcept',
                transform: 'ce_to_codeableconcept',
                description: 'Patient\'s personal sense of their own gender',
                resources: ['Patient'],
            },
            {
                id: 'us-core-tribal-affiliation',
                name: 'Tribal Affiliation',
                url: 'http://hl7.org/fhir/us/core/StructureDefinition/us-core-tribal-affiliation',
                valueType: 'valueCodeableConcept',
                transform: 'ce_to_codeableconcept',
                description: 'Tribe or band with which the patient is affiliated',
                resources: ['Patient'],
            },
        ],
    },
    {
        category: 'Organ & Tissue',
        icon: '🏥',
        extensions: [
            {
                id: 'organ-donor',
                name: 'Organ Donor Status',
                url: 'http://hl7.org/fhir/StructureDefinition/patient-organ-donor',
                valueType: 'valueBoolean',
                transform: 'hl7_active_flag',
                description: 'Whether the patient has consented to organ donation (Y → true, N → false)',
                resources: ['Patient'],
            },
        ],
    },
    {
        category: 'Demographics',
        icon: '👤',
        extensions: [
            {
                id: 'mothers-maiden-name',
                name: "Mother's Maiden Name",
                url: 'http://hl7.org/fhir/StructureDefinition/patient-mothersMaidenName',
                valueType: 'valueString',
                transform: '',
                description: "Patient's mother's maiden name",
                resources: ['Patient'],
            },
            {
                id: 'birth-place',
                name: 'Birth Place',
                url: 'http://hl7.org/fhir/StructureDefinition/patient-birthPlace',
                valueType: 'valueString',
                transform: '',
                description: 'Place of birth (city, state, country)',
                resources: ['Patient'],
            },
            {
                id: 'citizenship',
                name: 'Citizenship',
                url: 'http://hl7.org/fhir/StructureDefinition/patient-citizenship',
                valueType: 'valueCode',
                transform: '',
                description: 'Country of citizenship (ISO 3166)',
                resources: ['Patient'],
                system: 'urn:iso:std:iso:3166',
            },
            {
                id: 'nationality',
                name: 'Nationality',
                url: 'http://hl7.org/fhir/StructureDefinition/patient-nationality',
                valueType: 'valueCode',
                transform: '',
                description: 'Country of national identity',
                resources: ['Patient'],
                system: 'urn:iso:std:iso:3166',
            },
            {
                id: 'preferred-language',
                name: 'Preferred Language',
                url: 'http://hl7.org/fhir/StructureDefinition/patient-preferredLanguage',
                valueType: 'valueCode',
                transform: '',
                description: 'Patient\'s preferred language for communication',
                resources: ['Patient'],
                system: 'urn:ietf:bcp:47',
            },
            {
                id: 'patient-deceased',
                name: 'Deceased Indicator',
                url: 'http://hl7.org/fhir/StructureDefinition/patient-deceasedDateTime',
                valueType: 'valueBoolean',
                transform: 'hl7_active_flag',
                description: 'Whether the patient is deceased (Y → true)',
                resources: ['Patient'],
            },
        ],
    },
    {
        category: 'Clinical',
        icon: '🩺',
        extensions: [
            {
                id: 'bodySite',
                name: 'Body Site',
                url: 'http://hl7.org/fhir/StructureDefinition/bodySite',
                valueType: 'valueCodeableConcept',
                transform: 'ce_to_codeableconcept',
                description: 'Body site to which the element applies',
                resources: ['Observation', 'Procedure', 'Condition'],
            },
            {
                id: 'observation-focus',
                name: 'Observation Focus',
                url: 'http://hl7.org/fhir/StructureDefinition/observation-focus',
                valueType: 'valueReference',
                transform: '',
                description: 'Identifies what the observation is about, distinct from the subject',
                resources: ['Observation'],
            },
            {
                id: 'condition-assertedDate',
                name: 'Condition Asserted Date',
                url: 'http://hl7.org/fhir/StructureDefinition/condition-assertedDate',
                valueType: 'valueDateTime',
                transform: 'ts_to_datetime',
                description: 'Date the condition was first asserted or acknowledged',
                resources: ['Condition'],
            },
        ],
    },
    {
        category: 'Coverage & Insurance',
        icon: '📋',
        extensions: [
            {
                id: 'coverage-selfpay',
                name: 'Self-Pay Indicator',
                url: 'http://hl7.org/fhir/StructureDefinition/coverage-selfpay',
                valueType: 'valueBoolean',
                transform: 'hl7_active_flag',
                description: 'Whether the patient is self-paying (no insurer)',
                resources: ['Coverage'],
            },
        ],
    },
];
