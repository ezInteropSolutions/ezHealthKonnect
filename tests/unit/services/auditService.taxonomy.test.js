// tests/unit/services/auditService.taxonomy.test.js
//
// services/auditService.js's resolveEvent()/lookupTaxonomy()/atnaOutcomeIndicator()
// are the Node mirror of services/audit/taxonomy.go's and audit_logger.go's own
// resolveEvent logic — one shared taxonomy (services/audit/audit_events.json),
// read by both languages, so an action fired from Node gets the identical
// ATNA/RFC 3881 shape and HIPAA-compliance defaults as one fired from Go. The
// Go side has its own audit_logger_test.go covering the same logic; this file
// is the Node-side equivalent, added retroactively so a future edit to either
// resolveEvent() or audit_events.json can't silently drift the two languages
// apart again without a test failing.
const {
    resolveEvent,
    lookupTaxonomy,
    atnaOutcomeIndicator,
    TAXONOMY,
} = require('../../../services/auditService');

describe('shared taxonomy loading', () => {
    it('loads a non-empty taxonomy from services/audit/audit_events.json', () => {
        expect(Object.keys(TAXONOMY).length).toBeGreaterThan(0);
    });

    it('has an entry for a known, real action (LOGIN_SUCCESS)', () => {
        expect(TAXONOMY.LOGIN_SUCCESS).toBeDefined();
        expect(TAXONOMY.LOGIN_SUCCESS.atnaEventId).toBe('110114');
    });
});

describe('atnaOutcomeIndicator', () => {
    it.each([
        ['success', '0'],
        ['failure', '4'],
        ['error', '8'],
        ['something-unrecognized', '4'], // matches Go's default case
        [undefined, '4'],
    ])('maps result %s to ATNA outcome indicator %s', (result, expected) => {
        expect(atnaOutcomeIndicator(result)).toBe(expected);
    });
});

describe('lookupTaxonomy', () => {
    it('returns the real entry for a known action', () => {
        const entry = lookupTaxonomy('LOGIN_SUCCESS');
        expect(entry.defaultRiskLevel).toBe('low');
        expect(entry.defaultResult).toBe('success');
        expect(entry.atnaEventId).toBe('110114');
    });

    it('degrades gracefully for an unregistered action — never throws, never blocks', () => {
        const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});
        const entry = lookupTaxonomy('DEFINITELY_NOT_A_REAL_ACTION');
        expect(entry).toEqual({ defaultRiskLevel: 'low', defaultResult: 'success' });
        expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining('DEFINITELY_NOT_A_REAL_ACTION'));
        warnSpy.mockRestore();
    });
});

describe('resolveEvent', () => {
    it('applies taxonomy defaults when the caller supplies no overrides', () => {
        const resolved = resolveEvent({ action: 'LOGIN_SUCCESS' });
        expect(resolved.result).toBe('success');
        expect(resolved.riskLevel).toBe('low');
        expect(resolved.complianceFlags).toEqual({ hipaa: true });
    });

    it('folds ATNA identity fields into metadata.atna', () => {
        const resolved = resolveEvent({ action: 'LOGIN_SUCCESS' });
        expect(resolved.metadata.atna).toEqual({
            event_id: '110114',
            event_id_display: 'User Authentication',
            event_action_code: 'E',
            event_outcome_indicator: '0', // result defaulted to 'success'
        });
    });

    it('an explicit caller-supplied result/riskLevel/complianceFlags wins over the taxonomy default', () => {
        const resolved = resolveEvent({
            action: 'LOGIN_SUCCESS', // taxonomy default is success/low
            result: 'failure',
            riskLevel: 'high',
            complianceFlags: { custom: true },
        });
        expect(resolved.result).toBe('failure');
        expect(resolved.riskLevel).toBe('high');
        expect(resolved.complianceFlags).toEqual({ custom: true });
        // ATNA outcome indicator must reflect the OVERRIDDEN result, not the taxonomy default.
        expect(resolved.metadata.atna.event_outcome_indicator).toBe('4');
    });

    it('preserves caller-supplied metadata alongside the folded-in ATNA fields', () => {
        const resolved = resolveEvent({
            action: 'LOGIN_SUCCESS',
            metadata: { userEmail: 'someone@example.com' },
        });
        expect(resolved.metadata.userEmail).toBe('someone@example.com');
        expect(resolved.metadata.atna).toBeDefined();
    });

    it('degrades to generic low/success defaults for an unregistered action, with no metadata.atna at all', () => {
        const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});
        const resolved = resolveEvent({ action: 'DEFINITELY_NOT_A_REAL_ACTION' });
        expect(resolved.result).toBe('success');
        expect(resolved.riskLevel).toBe('low');
        expect(resolved.complianceFlags).toEqual({});
        expect(resolved.metadata.atna).toBeUndefined();
        warnSpy.mockRestore();
    });

    it('splits high-risk credential changes from routine updates via distinct real taxonomy entries', () => {
        // Regression guard for the exact bug this session's Phase 3/4 work fixed:
        // a credential change must resolve to a materially different risk tier
        // than a routine interface update.
        const routine = resolveEvent({ action: 'INTERFACE_UPDATED' });
        const credential = resolveEvent({ action: 'INTERFACE_CREDENTIALS_UPDATED' });
        expect(credential.riskLevel).toBe('high');
        expect(credential.complianceFlags.credential_change).toBe(true);
        expect(routine.riskLevel).not.toBe('high');
    });
});
