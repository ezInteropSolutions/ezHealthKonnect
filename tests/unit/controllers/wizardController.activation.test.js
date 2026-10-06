// tests/unit/controllers/wizardController.activation.test.js
//
// handleOptimizedWizard used to gate auto-activation with:
//   if (wizardData.auto_start || wizardData.deployment_mode === 'auto' || interfaceData.status === 'active')
// `interfaceData.status` defaults to 'active' whenever a caller doesn't pass
// a different one (the wizard's own normalization step), so that third
// clause silently auto-activated EVERY interface regardless of
// deployment_mode — including one explicitly created with
// deployment_mode: 'manual'. This was root-caused during a full-stack mock
// device scenario (a Mindray host-query interface kept getting activated
// immediately on creation, well before the test's own explicit activation
// call, confirmed via Postgres's own timestamped statement log) and fixed by
// extracting the decision into shouldAutoActivateOnWizardComplete(), which
// deliberately consults ONLY auto_start/deployment_mode — matching
// InterfaceDeploymentService.js's own correct, deployment_mode-only gating
// for the identical decision.
//
// These tests are the checked-in, repeatable version of that fix, so a
// future edit can't silently reintroduce the status-based auto-activation
// bug without a test failing.
const { shouldAutoActivateOnWizardComplete } = require('../../../controllers/wizardController');

describe('shouldAutoActivateOnWizardComplete', () => {
    it('does NOT auto-activate when deployment_mode is "manual", even with the default status of "active"', () => {
        expect(shouldAutoActivateOnWizardComplete({
            deployment_mode: 'manual',
            status: 'active', // the default every wizard payload gets unless overridden
        })).toBe(false);
    });

    it('does NOT auto-activate when deployment_mode is "delayed"', () => {
        expect(shouldAutoActivateOnWizardComplete({
            deployment_mode: 'delayed',
            status: 'active',
        })).toBe(false);
    });

    it('does NOT auto-activate when no deployment_mode/auto_start is set at all (status still defaults to active)', () => {
        expect(shouldAutoActivateOnWizardComplete({
            status: 'active',
        })).toBe(false);
    });

    it('DOES auto-activate when deployment_mode is "auto"', () => {
        expect(shouldAutoActivateOnWizardComplete({
            deployment_mode: 'auto',
            status: 'active',
        })).toBe(true);
    });

    it('DOES auto-activate when auto_start is true, regardless of deployment_mode', () => {
        expect(shouldAutoActivateOnWizardComplete({
            auto_start: true,
            deployment_mode: 'manual',
        })).toBe(true);
    });

    it('does NOT auto-activate when auto_start is explicitly false and deployment_mode is unset', () => {
        expect(shouldAutoActivateOnWizardComplete({
            auto_start: false,
        })).toBe(false);
    });

    it('is safe against a missing/undefined wizardData argument', () => {
        expect(shouldAutoActivateOnWizardComplete(undefined)).toBe(false);
        expect(shouldAutoActivateOnWizardComplete(null)).toBe(false);
        expect(shouldAutoActivateOnWizardComplete({})).toBe(false);
    });

    it('always returns a real boolean, never a truthy/falsy non-boolean', () => {
        // auto_start is often passed through from a form as the string "true"/"" —
        // guard against the !! coercion ever regressing to returning the raw value.
        expect(shouldAutoActivateOnWizardComplete({ auto_start: 'true' })).toBe(true);
        expect(shouldAutoActivateOnWizardComplete({ auto_start: '' })).toBe(false);
    });
});
