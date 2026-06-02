/**
 * HL7TypeCatalog
 *
 * Authoritative static catalog of HL7 v2 composite type component definitions.
 * Provides isComposite(), getComponents(), and getWholeObjectHint() with no
 * runtime dependency — all component knowledge is spec-defined and baked in.
 *
 * The type registry from GET /api/hl7/type-registry is loaded lazily via
 * load() and used only to enrich descriptions; it does not affect the core
 * isComposite() / getComponents() results.
 *
 * Usage:
 *   if (HL7TypeCatalog.isComposite('XPN')) {
 *       const comps = HL7TypeCatalog.getComponents('XPN');
 *       const hint  = HL7TypeCatalog.getWholeObjectHint('XPN');
 *   }
 *   // Optionally pre-warm the API cache:
 *   HL7TypeCatalog.load().then(() => { ... });
 */
class HL7TypeCatalog {

    /**
     * Returns true when the HL7 data type is composite (has named sub-components).
     * Determined entirely from the static COMPONENT_DEFS table — no API call.
     * @param {string} hl7DataType  e.g. "XPN", "CE", "ST"
     * @returns {boolean}
     */
    static isComposite(hl7DataType) {
        if (!hl7DataType) return false;
        return Object.prototype.hasOwnProperty.call(
            HL7TypeCatalog.COMPONENT_DEFS,
            hl7DataType.toUpperCase()
        );
    }

    /**
     * Returns the array of component definitions for a composite type.
     * Returns an empty array for unknown / primitive types.
     * @param {string} hl7DataType
     * @returns {Array<{position:number, name:string, fhirHint:string, transform:string, note?:string}>}
     */
    static getComponents(hl7DataType) {
        if (!hl7DataType) return [];
        return HL7TypeCatalog.COMPONENT_DEFS[hl7DataType.toUpperCase()] || [];
    }

    /**
     * Returns the whole-object mapping hint for a composite type, or null if
     * no meaningful whole-object mapping exists for the type.
     * @param {string} hl7DataType
     * @returns {{ fhirHint:string, transform:string, label:string } | null}
     */
    static getWholeObjectHint(hl7DataType) {
        if (!hl7DataType) return null;
        return HL7TypeCatalog.WHOLE_OBJECT_HINTS[hl7DataType.toUpperCase()] || null;
    }

    /**
     * Loads the Go-side type registry from GET /api/hl7/type-registry and caches
     * it. Safe to call multiple times — de-duplicates concurrent requests.
     * @returns {Promise<Object>}  key → { fhirType, transformKey, componentSeparator, notes }
     */
    static load() {
        if (HL7TypeCatalog._typeRegistry) {
            return Promise.resolve(HL7TypeCatalog._typeRegistry);
        }
        if (HL7TypeCatalog._loadPromise) return HL7TypeCatalog._loadPromise;

        HL7TypeCatalog._loadPromise = fetch('/api/hl7/type-registry', { credentials: 'include' })
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (data.success && data.data) {
                    HL7TypeCatalog._typeRegistry = data.data;
                }
                return HL7TypeCatalog._typeRegistry || {};
            })
            .catch(function() {
                HL7TypeCatalog._loadPromise = null;
                return {};
            });

        return HL7TypeCatalog._loadPromise;
    }

    /**
     * Returns a single entry from the API-loaded type registry, or null if the
     * registry has not been loaded yet. Call load() first to guarantee results.
     * @param {string} hl7DataType
     * @returns {{ fhirType:string, transformKey:string, componentSeparator:boolean, notes:string } | null}
     */
    static getTypeEntry(hl7DataType) {
        if (!hl7DataType || !HL7TypeCatalog._typeRegistry) return null;
        return HL7TypeCatalog._typeRegistry[hl7DataType.toUpperCase()] || null;
    }
}

// ── Post-class initialization (ES5 compatible — no static class fields) ──────

HL7TypeCatalog._typeRegistry = null;
HL7TypeCatalog._loadPromise  = null;

/**
 * Whole-object mapping hints.
 * Used when the user maps the entire composite field (not a specific component).
 * fhirHint is relative to the FHIR resource — callers prepend the resource type.
 * '(target path)' means the caller must supply the FHIR element path manually
 * because it varies per context (e.g. CE is used for dozens of FHIR elements).
 */
HL7TypeCatalog.WHOLE_OBJECT_HINTS = {
    XPN: { fhirHint: 'name[0]',       transform: 'xpn_to_humanname',     label: 'HumanName object'       },
    XAD: { fhirHint: 'address[0]',    transform: 'xad_to_address',        label: 'Address object'         },
    XTN: { fhirHint: 'telecom[0]',    transform: 'xtn_to_contactpoint',   label: 'ContactPoint object'    },
    CE:  { fhirHint: '(target path)', transform: 'ce_to_codeableconcept', label: 'CodeableConcept object' },
    CWE: { fhirHint: '(target path)', transform: 'ce_to_codeableconcept', label: 'CodeableConcept object' },
    CNE: { fhirHint: '(target path)', transform: 'ce_to_codeableconcept', label: 'CodeableConcept object' },
    CX:  { fhirHint: 'identifier[0]', transform: 'cx_to_identifier',      label: 'Identifier object'      },
    EI:  { fhirHint: 'identifier[0]', transform: 'cx_to_identifier',      label: 'Identifier object'      },
};

/**
 * Component definitions for each supported composite type.
 *
 * position  HL7 component number (1-based), used to build the sub-path
 *           e.g. position 1 of PID.5 → PID.5.1
 * name      Human-readable component label
 * fhirHint  Relative FHIR element path (no resource prefix)
 * transform Transform key the runtime engine should apply to this component
 * note      Optional clarification shown as a tooltip in the picker UI
 */
HL7TypeCatalog.COMPONENT_DEFS = {

    // ── XPN — Extended Person Name → HumanName ─────────────────────────────
    XPN: [
        { position: 1, name: 'Family name',
          fhirHint: 'name[0].family',    transform: 'name_component' },
        { position: 2, name: 'Given name (first)',
          fhirHint: 'name[0].given[0]',  transform: 'name_component' },
        { position: 3, name: 'Middle name / initial',
          fhirHint: 'name[0].given[1]',  transform: 'name_component' },
        { position: 4, name: 'Suffix (Jr., Sr.)',
          fhirHint: 'name[0].suffix[0]', transform: 'name_component' },
        { position: 5, name: 'Prefix (Dr., Mr.)',
          fhirHint: 'name[0].prefix[0]', transform: 'name_component' },
        { position: 7, name: 'Name type code',
          fhirHint: 'name[0].use',        transform: 'name_use_mapping',
          note: 'L=official, A=alias, N=nickname, M=maiden' },
    ],

    // ── XAD — Extended Address → Address ───────────────────────────────────
    XAD: [
        { position: 1, name: 'Street address',
          fhirHint: 'address[0].line[0]',     transform: 'address_component' },
        { position: 2, name: 'Other designation',
          fhirHint: 'address[0].line[1]',     transform: 'address_component' },
        { position: 3, name: 'City',
          fhirHint: 'address[0].city',        transform: 'string_direct' },
        { position: 4, name: 'State or province',
          fhirHint: 'address[0].state',       transform: 'string_direct' },
        { position: 5, name: 'Zip / postal code',
          fhirHint: 'address[0].postalCode',  transform: 'string_direct' },
        { position: 6, name: 'Country',
          fhirHint: 'address[0].country',     transform: 'string_direct' },
        { position: 7, name: 'Address type',
          fhirHint: 'address[0].type',        transform: 'address_use_mapping',
          note: 'H=home, B=work, C=current, P=permanent' },
        { position: 9, name: 'County / parish',
          fhirHint: 'address[0].district',    transform: 'string_direct' },
    ],

    // ── XTN — Extended Telecommunication → ContactPoint ────────────────────
    XTN: [
        { position: 1, name: 'Phone number (v2.6 and earlier)',
          fhirHint: 'telecom[0].value',  transform: 'string_direct',
          note: 'Deprecated in HL7 v2.7; use component 7 for v2.7+ messages' },
        { position: 2, name: 'Use code',
          fhirHint: 'telecom[0].use',    transform: 'telecom_use_mapping',
          note: 'PRN=home, WPN=work, NET=email/url' },
        { position: 3, name: 'Equipment type',
          fhirHint: 'telecom[0].system', transform: 'telecom_system_mapping',
          note: 'PH=phone, CP=mobile, Internet=email/url' },
        { position: 4, name: 'Email address',
          fhirHint: 'telecom[0].value',  transform: 'string_direct' },
        { position: 7, name: 'Local number (v2.7+)',
          fhirHint: 'telecom[0].value',  transform: 'telecom_value' },
    ],

    // ── CE — Coded Element → CodeableConcept ───────────────────────────────
    CE: [
        { position: 1, name: 'Code identifier',
          fhirHint: 'coding[0].code',    transform: 'string_direct' },
        { position: 2, name: 'Display text',
          fhirHint: 'coding[0].display', transform: 'string_direct' },
        { position: 3, name: 'Coding system',
          fhirHint: 'coding[0].system',  transform: 'coding_system_mapping',
          note: 'Converts HL7 coding system names to FHIR URIs (e.g. LN → http://loinc.org)' },
        { position: 5, name: 'Alternate display text',
          fhirHint: 'text',              transform: 'string_direct' },
    ],

    // ── CWE — Coded With Exceptions (v2.6+ replacement for CE) ────────────
    CWE: [
        { position: 1, name: 'Code identifier',
          fhirHint: 'coding[0].code',    transform: 'string_direct' },
        { position: 2, name: 'Display text',
          fhirHint: 'coding[0].display', transform: 'string_direct' },
        { position: 3, name: 'Coding system',
          fhirHint: 'coding[0].system',  transform: 'coding_system_mapping' },
        { position: 4, name: 'Alternate code',
          fhirHint: 'coding[1].code',    transform: 'string_direct' },
        { position: 5, name: 'Alternate display text',
          fhirHint: 'coding[1].display', transform: 'string_direct' },
        { position: 9, name: 'Original text',
          fhirHint: 'text',              transform: 'string_direct' },
    ],

    // ── CNE — Coded No Exceptions (required binding, same structure as CWE) ─
    CNE: [
        { position: 1, name: 'Code identifier',
          fhirHint: 'coding[0].code',    transform: 'string_direct' },
        { position: 2, name: 'Display text',
          fhirHint: 'coding[0].display', transform: 'string_direct' },
        { position: 3, name: 'Coding system',
          fhirHint: 'coding[0].system',  transform: 'coding_system_mapping' },
        { position: 9, name: 'Original text',
          fhirHint: 'text',              transform: 'string_direct' },
    ],

    // ── CX — Extended Composite ID → Identifier ────────────────────────────
    CX: [
        { position: 1, name: 'ID value',
          fhirHint: 'identifier[0].value',                  transform: 'string_direct' },
        { position: 4, name: 'Assigning authority',
          fhirHint: 'identifier[0].system',                 transform: 'assigning_authority_to_uri',
          note: 'Converts namespace/OID to a FHIR system URI (e.g. urn:oid:2.16...)' },
        { position: 5, name: 'Identifier type code',
          fhirHint: 'identifier[0].type.coding[0].code',   transform: 'identifier_type_mapping',
          note: 'MR=Medical Record, PI=Patient Internal ID, SS=Social Security' },
    ],

    // ── EI — Entity Identifier (same shape as CX) ──────────────────────────
    EI: [
        { position: 1, name: 'Entity ID',
          fhirHint: 'identifier[0].value',  transform: 'string_direct' },
        { position: 2, name: 'Namespace ID',
          fhirHint: 'identifier[0].system', transform: 'string_direct' },
        { position: 3, name: 'Universal ID (OID)',
          fhirHint: 'identifier[0].system', transform: 'assigning_authority_to_uri' },
    ],

    // ── HD — Hierarchic Designator ──────────────────────────────────────────
    HD: [
        { position: 1, name: 'Namespace ID',
          fhirHint: 'identifier[0].assigner.display', transform: 'string_direct' },
        { position: 2, name: 'Universal ID (OID / URI)',
          fhirHint: 'identifier[0].system',            transform: 'assigning_authority_to_uri' },
        { position: 3, name: 'Universal ID type',
          fhirHint: 'identifier[0].type.code',         transform: 'string_direct' },
    ],
};
