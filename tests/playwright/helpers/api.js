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

    // Coverage Audit config lives on the generic interface-update endpoint
    // (interfacesController.js's updateInterface) — a tri-state JSONB field,
    // see CLAUDE.md's Coverage Audit section. Passing a real object here
    // writes it verbatim; there is no server-side allowlist on its shape.
    async setCoverageAuditConfig(id, config) {
        const res = await this.request.put(`${this.base}/api/interfaces/${id}`, {
            data: { cda_coverage_audit_config: config },
        });
        return this._json(res, 'setCoverageAuditConfig');
    }

    // Reads the same endpoint the Journey tab's own Coverage Audit step
    // calls (MessageController.js's getCoverageAudit) — ownership-scoped by
    // the caller's session, returns { success, data: [...], count }.
    async getCoverageAudit(messageId) {
        const res = await this.request.get(`${this.base}/api/messages/${messageId}/coverage-audit`);
        const body = await this._json(res, 'getCoverageAudit');
        return body.data || [];
    }

    // ── Pipelines ───────────────────────────────────────────────────────────────
    // savePipeline expects steps wrapped as execution_groups (one flat group is enough
    // for a simple sequential pipeline) — a bare top-level "steps" array is silently ignored.
    async savePipeline({ interfaceId, messageType, steps, connections = [] }) {
        const res = await this.request.post(`${this.base}/api/pipelines`, {
            data: {
                interface_id: interfaceId,
                message_type: messageType,
                execution_groups: [{ steps }],
                connections,
            },
        });
        const body = await this._json(res, 'savePipeline');
        if (!body.success) throw new Error(`savePipeline: ${body.error}`);
        return body;
    }

    // Returns the merged interface_scaffold (incl. pipeline_config) a real
    // "Use Template" click would produce — does NOT create anything itself
    // (interfaceTemplateController.js's useTemplate is scaffold-only; the
    // real create happens via createInterface/savePipelineRaw below). Used
    // to copy an already-proven mapping-step chain verbatim instead of
    // hand-retyping it.
    async getTemplateScaffold(slugOrId) {
        const res = await this.request.post(`${this.base}/api/interface-templates/${slugOrId}/use`, { data: {} });
        const body = await this._json(res, 'getTemplateScaffold');
        if (!body.success) throw new Error(`getTemplateScaffold: ${body.error}`);
        return body.interface_scaffold;
    }

    // Like savePipeline, but takes a ready-made execution_groups array
    // as-is (preserving each template's own one-step-per-group shape)
    // instead of wrapping a flat `steps` array into a single group.
    async savePipelineRaw({ interfaceId, messageType, executionGroups, connections = [] }) {
        const res = await this.request.post(`${this.base}/api/pipelines`, {
            data: {
                interface_id: interfaceId,
                message_type: messageType,
                execution_groups: executionGroups,
                connections,
            },
        });
        const body = await this._json(res, 'savePipelineRaw');
        if (!body.success) throw new Error(`savePipelineRaw: ${body.error}`);
        return body;
    }

    async testPipeline(payload) {
        const res = await this.request.post(`${this.base}/api/fhir/pipeline/test`, { data: payload });
        return res.json();
    }

    // Activates via the Go engine directly (the same endpoint the UI's own
    // "Activate" button calls) — returns the full parsed response, including
    // a `warnings` array when one or more connector.inbound steps failed to
    // start (e.g. a required config field left empty): activation as a
    // whole still succeeds by design, but this is the only way a caller
    // learns a step was silently skipped (see processing/engine.go's
    // activationWarnings doc comment).
    async activateInterface(id) {
        const res = await this.request.post(`${this.base}/api/runtime/interfaces/${id}/activate`);
        return this._json(res, 'activateInterface');
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
