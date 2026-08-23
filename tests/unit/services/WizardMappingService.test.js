// tests/unit/services/WizardMappingService.test.js
//
// WizardMappingService's constructor accepts an injectable pgPool
// (constructor(pgPool = null)), sidestepping its global-fallback lookup
// entirely when one is provided — one of only two classes in the whole
// Node.js layer already written for constructor injection. This file covers
// its pure data-shaping methods, none of which touch the pool at all.
const WizardMappingService = require('../../../services/WizardMappingService');

function makeService() {
    // A truthy fake pool short-circuits getExistingDatabaseConnection()'s
    // reach into other global service singletons entirely.
    return new WizardMappingService({ query: jest.fn() });
}

describe('extractAtomicMappings', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    const validMapping = { hl7Field: 'PID.5.1', fhirPath: 'Patient.name.family', resourceType: 'Patient' };

    it('finds mappings directly on atomicMappings', () => {
        expect(svc.extractAtomicMappings({ atomicMappings: [validMapping] })).toEqual([validMapping]);
    });

    it('falls back through mappingData.atomicMappings when direct is absent', () => {
        expect(svc.extractAtomicMappings({ mappingData: { atomicMappings: [validMapping] } })).toEqual([validMapping]);
    });

    it('falls back through mappingConfiguration.atomicMappings', () => {
        expect(svc.extractAtomicMappings({ mappingConfiguration: { atomicMappings: [validMapping] } })).toEqual([validMapping]);
    });

    it('falls back through fhirTransformResult.mappings', () => {
        expect(svc.extractAtomicMappings({ fhirTransformResult: { mappings: [validMapping] } })).toEqual([validMapping]);
    });

    it('falls back through step4Data.atomicMappings', () => {
        expect(svc.extractAtomicMappings({ step4Data: { atomicMappings: [validMapping] } })).toEqual([validMapping]);
    });

    it('filters out mappings missing hl7Field/fhirPath/resourceType', () => {
        const incomplete = { hl7Field: 'PID.5.1' }; // missing fhirPath and resourceType
        const result = svc.extractAtomicMappings({ atomicMappings: [validMapping, incomplete] });
        expect(result).toEqual([validMapping]);
    });

    it('returns an empty array when no location has any mappings', () => {
        expect(svc.extractAtomicMappings({})).toEqual([]);
    });
});

describe('extractFhirResources', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('reads from fhirTransformResult.fhirResources first', () => {
        expect(svc.extractFhirResources({ fhirTransformResult: { fhirResources: [{ resourceType: 'Patient' }] } }))
            .toEqual([{ resourceType: 'Patient' }]);
    });

    it('falls back to transformationData.fhirResources', () => {
        expect(svc.extractFhirResources({ transformationData: { fhirResources: [{ resourceType: 'Encounter' }] } }))
            .toEqual([{ resourceType: 'Encounter' }]);
    });

    it('defaults to an empty array', () => {
        expect(svc.extractFhirResources({})).toEqual([]);
    });
});

describe('extractMessageTypes', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('uses detectedMessageType as the primary type', () => {
        expect(svc.extractMessageTypes({ detectedMessageType: 'ADT^A01' })).toEqual(['ADT^A01']);
    });

    it('defaults to ADT^A01 when nothing is found anywhere', () => {
        expect(svc.extractMessageTypes({})).toEqual(['ADT^A01']);
    });

    it('adds distinct message types found on atomic mappings, without duplicating the primary', () => {
        const wizardData = {
            detectedMessageType: 'ADT^A01',
            atomicMappings: [
                { hl7Field: 'PID.5', fhirPath: 'Patient.name', resourceType: 'Patient', messageType: 'ADT^A01' },
                { hl7Field: 'OBX.5', fhirPath: 'Observation.value', resourceType: 'Observation', messageType: 'ORU^R01' },
            ],
        };
        expect(svc.extractMessageTypes(wizardData)).toEqual(['ADT^A01', 'ORU^R01']);
    });
});

describe('parseHL7FieldPath', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('parses a 3-part path into segment/field/component', () => {
        expect(svc.parseHL7FieldPath('PID.5.1')).toEqual({ segment: 'PID', field: '5', component: '1' });
    });

    it('parses a 2-part path with component defaulting to null', () => {
        expect(svc.parseHL7FieldPath('PID.7')).toEqual({ segment: 'PID', field: '7', component: null });
    });
});

describe('convertAtomicMappingToFieldMapping', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('builds the hl7_fhir_mappings row shape from an atomic mapping', () => {
        const atomicMapping = { hl7Field: 'PID.5.1', fhirPath: 'Patient.name.family', resourceType: 'Patient', value: 'Doe', transformationType: 'direct' };
        const result = svc.convertAtomicMappingToFieldMapping(atomicMapping, { detectedMessageType: 'ADT^A01' });

        expect(result).toMatchObject({
            hl7_message_type: 'ADT^A01',
            hl7_segment: 'PID',
            hl7_field: '5',
            hl7_component: '1',
            fhir_resource: 'Patient',
            fhir_profile: 'http://hl7.org/fhir/StructureDefinition/Patient',
            fhir_path: 'Patient.name.family',
        });
        expect(result.transformation_rule.sourceValue).toBe('Doe');
    });

    it('defaults messageType to "Unknown" when the wizard data has none', () => {
        const atomicMapping = { hl7Field: 'PID.5', fhirPath: 'Patient.name', resourceType: 'Patient' };
        const result = svc.convertAtomicMappingToFieldMapping(atomicMapping, {});
        expect(result.hl7_message_type).toBe('Unknown');
    });
});

describe('getFhirProfile', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('returns the known profile URL for a standard resource type', () => {
        expect(svc.getFhirProfile('Patient')).toBe('http://hl7.org/fhir/StructureDefinition/Patient');
    });

    it('falls back to a generated URL for an unrecognized resource type', () => {
        expect(svc.getFhirProfile('AllergyIntolerance')).toBe('http://hl7.org/fhir/StructureDefinition/AllergyIntolerance');
    });
});

describe('extractValueSetMappings', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('translates a known HL7 value (e.g. gender M/F) into a FHIR value-set mapping row', () => {
        const wizardData = { atomicMappings: [{ hl7Field: 'PID.8', fhirPath: 'Patient.gender', resourceType: 'Patient', value: 'M' }] };
        const result = svc.extractValueSetMappings(wizardData);
        expect(result).toEqual([
            expect.objectContaining({ hl7_value: 'M', fhir_code: 'male', fhir_system: 'http://hl7.org/fhir/administrative-gender' }),
        ]);
    });

    it('skips mappings whose value has no known translation', () => {
        const wizardData = { atomicMappings: [{ hl7Field: 'PID.5', fhirPath: 'Patient.name', resourceType: 'Patient', value: 'Doe' }] };
        expect(svc.extractValueSetMappings(wizardData)).toEqual([]);
    });
});

describe('countTotalFields', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('sums field counts across all segments', () => {
        const segments = { PID: { fields: [1, 2, 3] }, PV1: { fields: [1, 2] } };
        expect(svc.countTotalFields(segments)).toBe(5);
    });

    it('treats a segment with no fields array as contributing zero', () => {
        expect(svc.countTotalFields({ MSH: {} })).toBe(0);
    });

    it('returns 0 for an empty segments object', () => {
        expect(svc.countTotalFields({})).toBe(0);
    });
});

describe('groupMappingsByResource', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('groups mappings under their resourceType, keeping only hl7Field/fhirPath/value', () => {
        const mappings = [
            { hl7Field: 'PID.5', fhirPath: 'Patient.name', resourceType: 'Patient', value: 'Doe', extra: 'dropped' },
            { hl7Field: 'PID.7', fhirPath: 'Patient.birthDate', resourceType: 'Patient', value: '19800101' },
            { hl7Field: 'OBX.5', fhirPath: 'Observation.value', resourceType: 'Observation', value: '5.2' },
        ];
        const grouped = svc.groupMappingsByResource(mappings);
        expect(Object.keys(grouped)).toEqual(['Patient', 'Observation']);
        expect(grouped.Patient).toHaveLength(2);
        expect(grouped.Patient[0]).toEqual({ hl7Field: 'PID.5', fhirPath: 'Patient.name', value: 'Doe' });
    });
});

describe('compileTransformationMapping (full integration of the pure methods)', () => {
    let svc;
    beforeEach(() => { svc = makeService(); });

    it('assembles the complete compiled-mapping structure from wizard data', () => {
        const wizardData = {
            detectedMessageType: 'ADT^A01',
            enhancedSegments: { PID: { fields: [1, 2, 3] } },
            atomicMappings: [
                { hl7Field: 'PID.5', fhirPath: 'Patient.name', resourceType: 'Patient', value: 'Doe' },
            ],
        };
        const result = svc.compileTransformationMapping(wizardData);

        expect(result.messageType).toBe('ADT^A01');
        expect(result.sourceData).toEqual({ segmentCount: 1, segmentTypes: ['PID'], totalFields: 3 });
        expect(result.mappingData.totalMappings).toBe(1);
        expect(result.mappingData.resourceTypes).toEqual(['Patient']);
        // compiledAt/generatedAt are real timestamps — just prove they parse, not an exact value.
        expect(new Date(result.compiledAt).toString()).not.toBe('Invalid Date');
    });
});
