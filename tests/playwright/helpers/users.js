'use strict';
/**
 * Creates real application users with a given role through the admin API, and
 * completes the forced first-login password reset so they can log in straight away.
 *
 *   const user = await createUserWithRole(adminRequest, BASE_URL, 'viewer');
 *   // -> { id, email, password, role }   ... later: await deleteUser(adminRequest, BASE_URL, user.id)
 *
 * POST /api/users only accepts admin|user, so the role is applied with PUT /api/users/:id
 * (the same call the user-management page makes when an admin changes a role).
 */

const { request: playwrightRequest } = require('@playwright/test');

const PASSWORD = 'PwTest-Role#2026';

/**
 * POST /api/auth/login, waiting out the limiter instead of failing. Login is limited to 10 per minute per IP and a
 * full parallel run exceeds that; the server answers 429 with Retry-After. Only 429 is retried, so a real 401/403
 * (which tests assert on) comes straight back.
 */
async function loginWithRetry(request, options, attempts = 4) {
    for (let i = 1; ; i++) {
        const res = await request.post('/api/auth/login', options);
        if (res.status() !== 429 || i >= attempts) return res;
        const wait = Math.min(65, parseInt(res.headers()['retry-after'] || '15', 10) || 15);
        await new Promise(r => setTimeout(r, (wait + 1) * 1000));
    }
}

async function createUserWithRole(adminRequest, baseURL, role, label = '') {
    const stamp = `${Date.now()}${Math.floor(Math.random() * 1000)}`;
    const email = `pw-${role}${label}-${stamp}@example.org`;

    const created = await adminRequest.post(`${baseURL}/api/users`, {
        data: { email, password: PASSWORD, name: `PW ${role} ${stamp}`, role: 'user' },
    });
    if (!created.ok()) throw new Error(`create user failed (${created.status()}): ${(await created.text()).slice(0, 200)}`);
    const body = await created.json();
    const id = body.user?.id || body.id || body.data?.id;
    if (!id) throw new Error(`create user returned no id: ${JSON.stringify(body).slice(0, 200)}`);

    const updated = await adminRequest.put(`${baseURL}/api/users/${id}`, { data: { role } });
    if (!updated.ok()) throw new Error(`set role failed (${updated.status()}): ${(await updated.text()).slice(0, 200)}`);

    // New accounts are forced to reset their password at first login. Use a throwaway context: a
    // login on the admin's own context would replace the admin's session cookie.
    // Explicit empty storageState: the project's admin storageState is otherwise applied to every new context.
    const fresh = await playwrightRequest.newContext({ baseURL, storageState: { cookies: [], origins: [] } });
    try {
        const login = await loginWithRetry(fresh, { data: { email, password: PASSWORD } });
        if (login.status() === 403) {
            const { requiresPasswordReset, userId } = await login.json();
            if (requiresPasswordReset) {
                const reset = await fresh.post('/api/auth/reset-password', { data: { userId, password: PASSWORD } });
                if (!reset.ok()) throw new Error(`forced reset failed (${reset.status()}): ${(await reset.text()).slice(0, 200)}`);
            }
        }
    } finally {
        await fresh.dispose();
    }
    return { id, email, password: PASSWORD, role };
}

async function deleteUser(adminRequest, baseURL, id) {
    await adminRequest.delete(`${baseURL}/api/users/${id}`).catch(() => {});
}

module.exports = { createUserWithRole, deleteUser, loginWithRetry, PASSWORD };
