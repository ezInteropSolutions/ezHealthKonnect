'use strict';
/**
 * AccessPolicyService — what each role may SEE, as an admin-editable setting instead of code.
 *
 * Two independent choices per role (route guards still decide who may CHANGE things; this decides visibility):
 *   interfaceScope  'all' | 'own'          every interface, or only the ones the user created
 *   messageContent  'full' | 'metadata'    patient content, or only details (time, type, status, errors)
 *
 * Stored in system_settings under "access_policy" as { roles: { operator: {...}, viewer: {...}, user: {...} } },
 * edited on Settings -> Access policy (PUT /api/system/settings/access-policy).
 * Administrators always see everything and cannot be restricted, so a bad setting can never lock them out.
 */

const settingsService = require('./settingsService');

const SETTING_KEY = 'access_policy';
const INTERFACE_SCOPES = ['all', 'own'];
const MESSAGE_CONTENT_LEVELS = ['full', 'metadata'];

// Roles that always see every interface, whatever is stored: operators and viewers are the support and monitoring
// staff, who cannot do their job (or help anyone) if part of the system is hidden from them. Only what they may read
// INSIDE a message (messageContent) is a policy choice for them. Scoping to "own" interfaces is for the plain user role.
const ALL_INTERFACES_ROLES = Object.freeze(['operator', 'viewer']);

// Roles that are never restricted.
const UNRESTRICTED_ROLES = ['admin', 'super_admin', 'superadmin'];

// Shipped defaults: an operator runs the integrations and sees all of them; a viewer sees that everything is
// healthy but not patient content ("minimum necessary"); a plain user sees only their own.
const DEFAULT_ROLE_POLICIES = Object.freeze({
    operator: Object.freeze({ interfaceScope: 'all', messageContent: 'full' }),
    viewer:   Object.freeze({ interfaceScope: 'all', messageContent: 'metadata' }),
    user:     Object.freeze({ interfaceScope: 'own', messageContent: 'full' }),
});

// A role nobody has described gets the most restrictive policy.
const FALLBACK_POLICY = Object.freeze({ interfaceScope: 'own', messageContent: 'metadata' });
const UNRESTRICTED_POLICY = Object.freeze({ interfaceScope: 'all', messageContent: 'full' });

// How long a read policy is trusted. Short, because the admin API that changes it runs in another process.
const POLICY_TTL_MS = 10 * 1000;

class AccessPolicyService {
    /** @param {{ getSetting: Function }} [settings] injectable for tests */
    constructor(settings = settingsService) {
        this.settings = settings;
    }

    /** @returns {Promise<{interfaceScope: string, messageContent: string}>} */
    async policyFor(role) {
        const key = String(role || '').toLowerCase();
        if (UNRESTRICTED_ROLES.includes(key)) return UNRESTRICTED_POLICY;

        const base = DEFAULT_ROLE_POLICIES[key] || FALLBACK_POLICY;
        const stored = await this.settings.getSetting(SETTING_KEY, {}, POLICY_TTL_MS);
        const mine = stored && stored.roles && stored.roles[key];
        const scope = ALL_INTERFACES_ROLES.includes(key) ? 'all'
            : INTERFACE_SCOPES.includes(mine && mine.interfaceScope) ? mine.interfaceScope : base.interfaceScope;
        return {
            interfaceScope: scope,
            messageContent: MESSAGE_CONTENT_LEVELS.includes(mine && mine.messageContent) ? mine.messageContent : base.messageContent,
        };
    }

    /** True when this user may see interfaces they did not create. */
    async canSeeAllInterfaces(user) {
        return (await this.policyFor(user && user.role)).interfaceScope === 'all';
    }

    /** True when this user may read message content (not only details). */
    async canViewMessageContent(user) {
        return (await this.policyFor(user && user.role)).messageContent === 'full';
    }

    /** The shape the Settings page edits: every configurable role with its effective policy. */
    async effectiveRoles() {
        const out = {};
        for (const role of Object.keys(DEFAULT_ROLE_POLICIES)) out[role] = await this.policyFor(role);
        return out;
    }
}

module.exports = new AccessPolicyService();
module.exports.AccessPolicyService = AccessPolicyService;
module.exports.DEFAULT_ROLE_POLICIES = DEFAULT_ROLE_POLICIES;
module.exports.ALL_INTERFACES_ROLES = ALL_INTERFACES_ROLES;
module.exports.INTERFACE_SCOPES = INTERFACE_SCOPES;
module.exports.MESSAGE_CONTENT_LEVELS = MESSAGE_CONTENT_LEVELS;
module.exports.SETTING_KEY = SETTING_KEY;
