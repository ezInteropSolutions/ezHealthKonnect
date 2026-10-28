'use strict';

const { AccessPolicyService } = require('../../../services/AccessPolicyService');

// A settings reader that returns whatever "system_settings" currently holds for the key.
const settingsReturning = (stored) => ({ getSetting: jest.fn().mockResolvedValue(stored) });

describe('AccessPolicyService', () => {
    test('administrators are never restricted, whatever is stored', async () => {
        const svc = new AccessPolicyService(settingsReturning({ roles: { admin: { interfaceScope: 'own', messageContent: 'metadata' } } }));
        for (const role of ['admin', 'super_admin', 'superadmin', 'ADMIN']) {
            expect(await svc.policyFor(role)).toEqual({ interfaceScope: 'all', messageContent: 'full' });
        }
    });

    test('shipped defaults when nothing is stored', async () => {
        const svc = new AccessPolicyService(settingsReturning({}));
        expect(await svc.policyFor('operator')).toEqual({ interfaceScope: 'all', messageContent: 'full' });
        expect(await svc.policyFor('viewer')).toEqual({ interfaceScope: 'all', messageContent: 'metadata' });
        expect(await svc.policyFor('user')).toEqual({ interfaceScope: 'own', messageContent: 'full' });
    });

    test('a stored value overrides the default for that role only', async () => {
        const svc = new AccessPolicyService(settingsReturning({ roles: { viewer: { interfaceScope: 'all', messageContent: 'full' }, user: { interfaceScope: 'all', messageContent: 'metadata' } } }));
        expect(await svc.policyFor('viewer')).toEqual({ interfaceScope: 'all', messageContent: 'full' });
        expect(await svc.policyFor('user')).toEqual({ interfaceScope: 'all', messageContent: 'metadata' });
        expect(await svc.policyFor('operator')).toEqual({ interfaceScope: 'all', messageContent: 'full' });
    });

    test('a half-specified or invalid stored value falls back per field, not wholesale', async () => {
        const svc = new AccessPolicyService(settingsReturning({ roles: { user: { interfaceScope: 'own', messageContent: 'everything' } } }));
        expect(await svc.policyFor('user')).toEqual({ interfaceScope: 'own', messageContent: 'full' });
    });

    test.each(['operator', 'viewer'])('%s always sees every interface, even if "own" is stored', async (role) => {
        const svc = new AccessPolicyService(settingsReturning({ roles: { [role]: { interfaceScope: 'own', messageContent: 'metadata' } } }));
        const p = await svc.policyFor(role);
        expect(p.interfaceScope).toBe('all');
        expect(p.messageContent).toBe('metadata');   // content stays a real choice
        expect(await svc.canSeeAllInterfaces({ role })).toBe(true);
    });

    test('an unknown or missing role gets the most restrictive policy', async () => {
        const svc = new AccessPolicyService(settingsReturning({}));
        const strictest = { interfaceScope: 'own', messageContent: 'metadata' };
        expect(await svc.policyFor('contractor')).toEqual(strictest);
        expect(await svc.policyFor(undefined)).toEqual(strictest);
        expect(await svc.policyFor('')).toEqual(strictest);
    });

    test('a null or malformed stored setting is treated as nothing stored', async () => {
        for (const stored of [null, undefined, 'x', { roles: null }, { roles: 'x' }]) {
            const svc = new AccessPolicyService(settingsReturning(stored));
            expect(await svc.policyFor('viewer')).toEqual({ interfaceScope: 'all', messageContent: 'metadata' });
        }
    });

    test('helpers answer from the policy', async () => {
        const svc = new AccessPolicyService(settingsReturning({}));
        expect(await svc.canSeeAllInterfaces({ role: 'viewer' })).toBe(true);
        expect(await svc.canSeeAllInterfaces({ role: 'user' })).toBe(false);
        expect(await svc.canViewMessageContent({ role: 'viewer' })).toBe(false);
        expect(await svc.canViewMessageContent({ role: 'operator' })).toBe(true);
        expect(await svc.canViewMessageContent(undefined)).toBe(false);
    });

    test('reads the setting with a short cache lifetime', async () => {
        const settings = settingsReturning({});
        await new AccessPolicyService(settings).policyFor('viewer');
        const [key, , ttl] = settings.getSetting.mock.calls[0];
        expect(key).toBe('access_policy');
        expect(ttl).toBeLessThanOrEqual(30 * 1000);
    });

    test('effectiveRoles lists every configurable role', async () => {
        const roles = await new AccessPolicyService(settingsReturning({})).effectiveRoles();
        expect(Object.keys(roles).sort()).toEqual(['operator', 'user', 'viewer']);
    });
});
