'use strict';
/**
 * API helper — thin wrapper around Playwright's APIRequestContext.
 * All calls go to the Node.js frontend (port 3000), which proxies to Go where needed.
 *
 * Usage:
 *   const api = new ApiHelper(request, 'http://localhost:3000');
 *   const { id } = await api.createInterface({ name: '...', ... });
 *   await api.deleteInterface(id);
 */

class ApiHelper {
    constructor(request, baseURL = 'http://localhost:3000') {
        this.request = request;
        this.base = baseURL;
    }

    async _json(response, label) {
        if (!response.ok()) {
            const text = await response.text();
            throw new Error(`${label} failed (${response.status()}): ${text.slice(0, 300)}`);
        }
        return response.json();
    }

    // ── Auth ────────────────────────────────────────────────────────────────────
    async login(email = 'admin@ezhealthkonnect.com', password = 'admin123') {
        const res = await this.request.post(`${this.base}/api/auth/login`, {
            data: { email, password },
        });
        return this._json(res, 'login');
    }

    // ── Interfaces ──────────────────────────────────────────────────────────────
    async listInterfaces() {
        const res = await this.request.get(`${this.base}/api/interfaces`);
        const body = await this._json(res, 'listInterfaces');
        return body.data || body.interfaces || body || [];
    }

    async createInterface(payload) {
        const res = await this.request.post(`${this.base}/api/wizard/complete`, {
            data: { wizardData: payload },
        });
        const body = await this._json(res, 'createInterface');
        if (!body.success) throw new Error(`createInterface: ${body.error}`);
        return body;
    }

    async getInterface(id) {
        const res = await this.request.get(`${this.base}/api/interfaces/${id}`);
        return this._json(res, 'getInterface');
    }

    async deactivateInterface(id) {
        // wizard route deactivates via Go engine (releases the port listener)
        const res = await this.request.post(
            `${this.base}/api/wizard/interfaces/${id}/deactivate`,
            { data: { reason: 'mvp-smoke-teardown' } }
        );
        return res.ok();
    }

    async deleteInterface(id) {
        // Deactivate first so Go releases the port before Node removes the DB row
        await this.deactivateInterface(id).catch(() => {});
        await new Promise(r => setTimeout(r, 500)); // brief settle
        const res = await this.request.delete(`${this.base}/api/interfaces/${id}`);
        return res.ok();
    }

    // ── Messages ────────────────────────────────────────────────────────────────
    async getMessages(interfaceId, limit = 20) {
        const res = await this.request.get(
            `${this.base}/api/messages/interface/${interfaceId}?limit=${limit}`
        );
        const body = await this._json(res, 'getMessages');
        // API returns { success, data: { messages: [...], pagination: {...} } }
        const inner = body.data || body;
        return Array.isArray(inner) ? inner : inner.messages || inner.data || [];
    }

    // ── Health ──────────────────────────────────────────────────────────────────
    async goHealth() {
        const res = await this.request.get(`${this.base.replace('3000', '8080')}/health`);
        return this._json(res, 'goHealth');
    }

    async nodeHealth() {
        const res = await this.request.get(`${this.base}/api/health`);
        return res.ok();
    }

    // ── Connectivity types ──────────────────────────────────────────────────────
    async getConnectivityTypes(direction = 'inbound') {
        const res = await this.request.get(
            `${this.base}/api/connectivity/types/category/${direction}`
        );
        const body = await this._json(res, 'getConnectivityTypes');
        return body.data || body || [];
    }
}

module.exports = { ApiHelper };
