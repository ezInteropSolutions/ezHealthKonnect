/**
 * ezCompanion — local, on-premise, no PHI leaves the network.
 * Provides a floating chat panel on any page with auto-injected UI context.
 *
 * Usage: include this script on any page, optionally set window.AI_CONTEXT:
 *   window.AI_CONTEXT = {
 *     interface_id: '...',
 *     pipeline_id:  '...',
 *     step_id:      '...',
 *     message_type: 'ADT^A01',
 *     page:         'pipeline-builder'
 *   };
 */

(function () {
    'use strict';

    // ── Constants ──────────────────────────────────────────────────────────────
    const API_BASE    = '/api/ai';
    const WIDGET_ID   = 'ai-assistant-widget';

    // ── User identity + persistent session ────────────────────────────────────
    // Session IDs are stored in localStorage (not sessionStorage) so they survive
    // tab closes and browser restarts — users return to their conversation.
    // Key is scoped per user so different logins don't share history.
    let _userId   = '';
    let _userName = '';

    function sessionKey() {
        return _userId ? `ezc_sid_${_userId}` : 'ezc_sid_anon';
    }

    function getSessionId() {
        let id = localStorage.getItem(sessionKey());
        if (!id) {
            id = 'sid-' + Date.now() + '-' + Math.random().toString(36).slice(2, 9);
            localStorage.setItem(sessionKey(), id);
        }
        return id;
    }

    function newConversation() {
        const id = 'sid-' + Date.now() + '-' + Math.random().toString(36).slice(2, 9);
        localStorage.setItem(sessionKey(), id);
        _historyLoaded = false;
        const msgs = document.getElementById('ai-messages');
        if (msgs) {
            msgs.innerHTML = '<div class="ai-msg system">Started a new conversation. How can I help?</div>';
        }
    }

    // Fetch the logged-in user from the existing /api/auth/session endpoint.
    // Caches result in module scope — called once on widget init.
    async function resolveCurrentUser() {
        // Prefer explicitly injected user (set by page before script runs)
        if (window.__currentUser && window.__currentUser.id) {
            _userId   = window.__currentUser.id;
            _userName = window.__currentUser.name || '';
            return;
        }
        try {
            const resp = await fetch('/api/auth/session', { credentials: 'include' });
            if (!resp.ok) return;
            const data = await resp.json();
            if (data.authenticated && data.user) {
                _userId   = data.user.id   || '';
                _userName = data.user.name || '';
                window.__currentUser = data.user;
            }
        } catch { /* silent — widget still works without user ID */ }
    }

    // ── History loading ────────────────────────────────────────────────────────
    let _historyLoaded = false;

    async function loadAndRenderHistory() {
        if (_historyLoaded) return;
        _historyLoaded = true;

        const sid = getSessionId();
        try {
            const resp = await fetch(`${API_BASE}/conversations?session_id=${encodeURIComponent(sid)}&limit=60`,
                { credentials: 'include' });
            if (!resp.ok) return;
            const data = await resp.json();
            const msgs = data.data;
            if (!Array.isArray(msgs) || msgs.length === 0) return;

            const container = document.getElementById('ai-messages');
            if (!container) return;

            // Insert history above the existing welcome message
            const welcome = container.firstChild;
            const divider = document.createElement('div');
            divider.className = 'ai-msg system';
            divider.style.cssText = 'font-size:11px;color:#94a3b8;text-align:center;padding:6px 0';
            divider.textContent = `── previous conversation (${msgs.length} messages) ──`;
            container.insertBefore(divider, welcome);

            for (const msg of msgs) {
                const el = document.createElement('div');
                el.className = `ai-msg ${msg.role === 'user' ? 'user' : 'assistant'}`;
                el.innerHTML = renderMarkdownInline(msg.content);
                container.insertBefore(el, welcome);
            }
            container.scrollTop = container.scrollHeight;
        } catch { /* silent */ }
    }

    // ── Auto-detect page context ───────────────────────────────────────────────
    function detectContext() {
        // Prefer explicitly set window variable
        if (window.AI_CONTEXT) return window.AI_CONTEXT;

        const params = new URLSearchParams(window.location.search);
        const path   = window.location.pathname;

        const ctx = {};

        // Pull common query params
        ctx.interface_id  = params.get('interfaceId') || params.get('interface_id') || '';
        ctx.pipeline_id   = params.get('pipelineId')  || params.get('pipeline_id')  || '';
        ctx.message_type  = params.get('messageType') || params.get('message_type') || '';

        // Detect page from pathname
        if (path.includes('pipeline-builder'))  ctx.page = 'pipeline-builder';
        else if (path.includes('monitoring'))   ctx.page = 'monitoring';
        else if (path.includes('wizard'))       ctx.page = 'wizard';
        else if (path.includes('interfaces'))   ctx.page = 'interfaces';
        else if (path.includes('alerts'))       ctx.page = 'alerts';
        else if (path.includes('analytics'))    ctx.page = 'analytics';
        else if (path.includes('hl7-reader'))   ctx.page = 'hl7-reader';
        else if (path.includes('fhir-valid'))   ctx.page = 'fhir-validator';
        else if (path.includes('review-queue')) ctx.page = 'review-queue';
        else if (path.includes('settings'))     ctx.page = 'settings';
        else if (path.includes('dashboard'))    ctx.page = 'dashboard';

        return ctx;
    }

    // ── API helpers ────────────────────────────────────────────────────────────
    async function apiPost(path, body) {
        const resp = await fetch(`${API_BASE}${path}`, {
            method:      'POST',
            headers:     { 'Content-Type': 'application/json' },
            credentials: 'include',
            body:        JSON.stringify(body)
        });
        const text = await resp.text();
        let json;
        try { json = JSON.parse(text); } catch {
            // Non-JSON response — Go backend likely unreachable or not started
            const preview = text.slice(0, 120).replace(/\s+/g, ' ').trim();
            throw new Error(
                resp.status === 404
                    ? 'AI service endpoint not found — check that the Go backend has started.'
                    : `AI service returned an unexpected response (HTTP ${resp.status}). ` +
                      'Make sure the Go backend is running.\n\nDetails: ' + preview
            );
        }
        return json;
    }

    // ── API calls ──────────────────────────────────────────────────────────────
    // askAIStream streams tokens directly into a DOM element as they arrive.
    async function askAIStream(question, targetEl) {
        const ctx = detectContext();
        const body = {
            question,
            session_id: getSessionId(),
            user_id:    _userId,
            context:    ctx,
            history:    []
        };
        return streamIntoEl('/ask/stream', body, targetEl);
    }

    async function detectFormat(message) {
        return apiPost('/detect-format', { message });
    }

    async function suggestMappings(message, targetFormat) {
        const ctx = detectContext();
        return apiPost('/suggest-mappings', { message, target_format: targetFormat, context: ctx });
    }

    async function explainError(error, messageContext) {
        const ctx = detectContext();
        return apiPost('/explain-error', { error, message_context: messageContext || '', context: ctx });
    }

    async function sendFeedback(type, payload) {
        try {
            await apiPost(`/feedback/${type}`, payload);
        } catch { /* fire-and-forget */ }
    }

    // ── SSE stream helper — shared by all streaming endpoints ──────────────────
    // Calls onToken(token) for each token, returns the full accumulated text.
    // onSpecial(tag, payload) handles special SSE events like [TRACE]{...} or [DONE].
    async function readSSEStream(resp, onToken, onSpecial) {
        const reader  = resp.body.getReader();
        const decoder = new TextDecoder();
        let   buffer  = '';
        let   output  = '';

        while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop();
            for (const line of lines) {
                if (!line.startsWith('data: ')) continue;
                const payload = line.slice(6);
                if (payload === '[DONE]') {
                    if (onSpecial) onSpecial('[DONE]', null);
                    return output;
                }
                if (payload.startsWith('[ERROR]')) {
                    throw new Error(payload.slice(7).trim());
                }
                // Special tagged events e.g. [TRACE]{...json...}
                const specialMatch = payload.match(/^\[([A-Z]+)\](.*)$/);
                if (specialMatch) {
                    let parsed = null;
                    try { parsed = JSON.parse(specialMatch[2]); } catch { parsed = specialMatch[2]; }
                    if (onSpecial) onSpecial('[' + specialMatch[1] + ']', parsed);
                    continue;
                }
                // Regular token
                let token;
                try { token = JSON.parse(payload); } catch { token = payload; }
                output += token;
                if (onToken) onToken(token, output);
            }
        }
        return output;
    }

    // ── Streaming into a target DOM element ────────────────────────────────────
    async function streamIntoEl(endpoint, body, targetEl) {
        const resp = await fetch(`${API_BASE}${endpoint}`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            credentials: 'include', body: JSON.stringify(body)
        });
        if (!resp.ok) throw new Error(`AI service error (HTTP ${resp.status})`);

        return readSSEStream(resp, (_tok, full) => {
            targetEl.innerHTML = renderMarkdownInline(full);
            const msgs = document.getElementById('ai-messages');
            if (msgs) msgs.scrollTop = msgs.scrollHeight;
        });
    }

    // ── generateScript: returns the full script as a string ───────────────────
    async function generateScriptStream(opts, onToken) {
        const ctx  = detectContext();
        const body = {
            description:    opts.description,
            step_id:        opts.stepId        || ctx.step_id        || '',
            pipeline_id:    opts.pipelineId    || ctx.pipeline_id    || '',
            interface_id:   opts.interfaceId   || ctx.interface_id   || '',
            message_type:   opts.messageType   || ctx.message_type   || '',
            step_type:      opts.stepType      || '',
            current_script: opts.currentScript || ''
        };
        const resp = await fetch(`${API_BASE}/generate-script/stream`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            credentials: 'include', body: JSON.stringify(body)
        });
        if (!resp.ok) throw new Error(`Script generation failed (HTTP ${resp.status})`);
        return readSSEStream(resp, onToken);
    }

    // ── traceMessage: fetch trace JSON + LLM analysis ─────────────────────────
    async function traceMessageStream(messageId, interfaceId, onTrace, onToken) {
        const body = { message_id: messageId, interface_id: interfaceId || '' };
        const resp = await fetch(`${API_BASE}/trace-message/stream`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            credentials: 'include', body: JSON.stringify(body)
        });
        if (!resp.ok) throw new Error(`Trace failed (HTTP ${resp.status})`);
        return readSSEStream(resp, onToken, (tag, payload) => {
            if (tag === '[TRACE]' && onTrace) onTrace(payload);
        });
    }

    // ── explainStep: stream explanation for a step ────────────────────────────
    async function explainStepStream(stepId, onToken) {
        const resp = await fetch(`${API_BASE}/explain-step/stream`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            credentials: 'include', body: JSON.stringify({ step_id: stepId })
        });
        if (!resp.ok) throw new Error(`Explain failed (HTTP ${resp.status})`);
        return readSSEStream(resp, onToken);
    }

    // ── Widget HTML + CSS ──────────────────────────────────────────────────────
    function injectStyles() {
        if (document.getElementById('ai-assistant-styles')) return;
        const style = document.createElement('style');
        style.id = 'ai-assistant-styles';
        style.textContent = `
/* ── Toggle button — fixed bottom-right ──────────────────────────── */
#ai-toggle-btn {
    position: fixed;
    bottom: 28px;
    right: 28px;
    z-index: 10001;
    width: 52px;
    height: 52px;
    border-radius: 50%;
    background: linear-gradient(135deg, #f472b6 0%, #1e3a8a 100%);
    border: none;
    cursor: pointer;
    box-shadow: 0 4px 16px rgba(30,58,138,0.35);
    display: flex;
    align-items: center;
    justify-content: center;
    color: #fff;
    font-size: 22px;
    transition: transform 0.2s, box-shadow 0.2s;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}
#ai-toggle-btn:hover { transform: scale(1.1); box-shadow: 0 6px 22px rgba(30,58,138,0.5); }

/* ── Backdrop ─────────────────────────────────────────────────────── */
#ai-backdrop {
    position: fixed;
    inset: 0;
    z-index: 10002;
    background: rgba(30,58,138,0.18);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.3s ease;
}
#ai-backdrop.visible { opacity: 1; pointer-events: auto; }

/* ── Drawer panel ─────────────────────────────────────────────────── */
#ai-panel {
    position: fixed;
    top: 0;
    right: 0;
    z-index: 10003;
    width: 420px;
    max-width: 100vw;
    height: 100vh;
    background: #ffffff;
    border-left: 1px solid #e2e8f0;
    display: flex;
    flex-direction: column;
    box-shadow: -4px 0 24px rgba(30,58,138,0.12);
    transform: translateX(100%);
    transition: transform 0.3s cubic-bezier(0.4,0,0.2,1);
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    font-size: 14px;
    color: #1a202c;
}
#ai-panel.open { transform: translateX(0); }

/* ── Header ───────────────────────────────────────────────────────── */
#ai-panel-header {
    padding: 16px 18px;
    background: linear-gradient(135deg, #f472b6 0%, #1e3a8a 100%);
    display: flex;
    align-items: center;
    justify-content: space-between;
    color: #fff;
    flex-shrink: 0;
}
#ai-panel-header .ai-title { font-weight: 700; font-size: 15px; letter-spacing: 0.01em; }
#ai-panel-header .ai-subtitle { font-size: 11px; opacity: 0.88; margin-top: 2px; }
#ai-close-btn {
    background: rgba(255,255,255,0.15);
    border: none;
    color: #fff;
    cursor: pointer;
    font-size: 16px;
    padding: 4px 8px;
    border-radius: 6px;
    opacity: 0.9;
    line-height: 1;
    transition: background 0.15s;
}
#ai-close-btn:hover { background: rgba(255,255,255,0.28); opacity: 1; }

/* ── Context bar ──────────────────────────────────────────────────── */
#ai-context-bar {
    padding: 7px 16px;
    background: #f8fafc;
    font-size: 11px;
    color: #94a3b8;
    border-bottom: 1px solid #e2e8f0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex-shrink: 0;
}

/* ── Messages ─────────────────────────────────────────────────────── */
#ai-messages {
    flex: 1;
    overflow-y: auto;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    background: #f8fafc;
}
.ai-msg {
    max-width: 88%;
    padding: 11px 15px;
    border-radius: 14px;
    line-height: 1.55;
    word-break: break-word;
    font-size: 13.5px;
}
.ai-msg.user {
    align-self: flex-end;
    background: #1e3a8a;
    color: #fff;
    border-bottom-right-radius: 4px;
}
.ai-msg.assistant {
    align-self: flex-start;
    background: #ffffff;
    color: #1a202c;
    border: 1px solid #e2e8f0;
    border-bottom-left-radius: 4px;
    box-shadow: 0 1px 4px rgba(30,58,138,0.06);
}
.ai-msg.system {
    align-self: center;
    background: transparent;
    color: #94a3b8;
    font-size: 12px;
    font-style: italic;
    max-width: 100%;
    text-align: center;
}
.ai-msg pre { white-space: pre-wrap; margin: 8px 0 0; font-size: 12px; background: #f1f5f9; padding: 10px; border-radius: 6px; color: #1e3a8a; }
.ai-msg code { background: #f1f5f9; padding: 1px 5px; border-radius: 4px; font-size: 12px; color: #1e3a8a; }
.ai-sources { font-size: 11px; color: #94a3b8; margin-top: 8px; border-top: 1px solid #f1f5f9; padding-top: 6px; }

/* ── Typing indicator ─────────────────────────────────────────────── */
.ai-typing { display: flex; gap: 5px; align-items: center; padding: 11px 15px; background: #ffffff; border: 1px solid #e2e8f0; border-radius: 14px; border-bottom-left-radius: 4px; align-self: flex-start; }
.ai-typing span { width: 7px; height: 7px; background: #f472b6; border-radius: 50%; animation: ai-bounce 1s infinite; }
.ai-typing span:nth-child(2) { animation-delay: 0.15s; }
.ai-typing span:nth-child(3) { animation-delay: 0.3s; }
@keyframes ai-bounce { 0%,60%,100%{transform:translateY(0)} 30%{transform:translateY(-6px)} }

/* ── Quick actions ────────────────────────────────────────────────── */
#ai-quick-actions {
    padding: 10px 14px;
    display: flex;
    gap: 7px;
    flex-wrap: wrap;
    border-top: 1px solid #e2e8f0;
    background: #ffffff;
    flex-shrink: 0;
}
.ai-quick-btn {
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    color: #64748b;
    padding: 5px 12px;
    border-radius: 20px;
    cursor: pointer;
    font-size: 11.5px;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
}
.ai-quick-btn:hover { background: #fef7f7; border-color: #f472b6; color: #1e3a8a; }

/* ── Input row ────────────────────────────────────────────────────── */
#ai-input-row {
    display: flex;
    gap: 8px;
    padding: 12px 14px;
    border-top: 1px solid #e2e8f0;
    background: #ffffff;
    flex-shrink: 0;
}
#ai-input {
    flex: 1;
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    color: #1a202c;
    padding: 9px 13px;
    border-radius: 10px;
    resize: none;
    font-size: 13px;
    font-family: inherit;
    outline: none;
    height: 40px;
    max-height: 120px;
    line-height: 1.4;
    transition: border-color 0.15s;
}
#ai-input::placeholder { color: #94a3b8; }
#ai-input:focus { border-color: #1e3a8a; background: #fff; }
#ai-send-btn {
    background: linear-gradient(135deg, #f472b6 0%, #1e3a8a 100%);
    border: none;
    color: #fff;
    padding: 9px 16px;
    border-radius: 10px;
    cursor: pointer;
    font-size: 15px;
    transition: opacity 0.15s;
    flex-shrink: 0;
}
#ai-send-btn:hover { opacity: 0.88; }
#ai-send-btn:disabled { opacity: 0.35; cursor: default; }
`;
        document.head.appendChild(style);
    }

    function buildWidget() {
        const el = document.createElement('div');
        el.id = WIDGET_ID;
        el.innerHTML = `
<button id="ai-toggle-btn" title="ezCompanion">✏️</button>
<div id="ai-backdrop"></div>
<div id="ai-panel">
    <div id="ai-panel-header">
        <div style="flex:1;min-width:0">
            <div class="ai-title">✏️ ezCompanion</div>
            <div class="ai-subtitle" id="ai-user-subtitle">Co-developer · Integration Expert · App Guide</div>
        </div>
        <div style="display:flex;gap:6px;align-items:center;flex-shrink:0">
            <button id="ai-new-btn" title="New conversation"
                style="background:rgba(255,255,255,0.15);border:none;color:#fff;cursor:pointer;font-size:12px;padding:4px 8px;border-radius:6px;opacity:0.9;line-height:1;white-space:nowrap">
                + New
            </button>
            <button id="ai-close-btn">✕</button>
        </div>
    </div>
    <div id="ai-context-bar">Loading context…</div>
    <div id="ai-messages">
        <div class="ai-msg system">Hi! I'm ezCompanion — your co-developer and healthcare integration expert.<br><br>I can help you <strong>build &amp; configure</strong> interfaces and pipelines, <strong>write pipeline scripts</strong>, answer questions about <strong>HL7, FHIR, X12, CCD</strong>, debug integration errors, or walk you through any <strong>ezHealthKonnect</strong> feature. Just ask!</div>
    </div>
    <div id="ai-quick-actions">
        <button class="ai-quick-btn" data-q="How do I create a new interface in ezHealthKonnect?">Create interface</button>
        <button class="ai-quick-btn" data-q="Write a JavaScript pipeline script step that extracts the patient name from PID.5 and adds it to the output">Write a script step</button>
        <button class="ai-quick-btn" data-q="How do I map an HL7 ADT^A01 message to a FHIR Patient resource?">ADT→FHIR mapping</button>
        <button class="ai-quick-btn" data-q="Explain how the transformation pipeline works in ezHealthKonnect">How pipelines work</button>
    </div>
    <div id="ai-input-row">
        <textarea id="ai-input" placeholder="Ask anything — build, configure, HL7, FHIR, X12…" rows="1"></textarea>
        <button id="ai-send-btn">➤</button>
    </div>
</div>`;
        document.body.appendChild(el);
    }

    // ── Message rendering ──────────────────────────────────────────────────────
    function renderMessage(role, text, sources) {
        const msgs = document.getElementById('ai-messages');
        const div = document.createElement('div');
        div.className = `ai-msg ${role}`;

        // Basic markdown: **bold**, `code`, ```blocks```
        let html = escapeHtml(text)
            .replace(/```([\s\S]*?)```/g, '<pre>$1</pre>')
            .replace(/`([^`]+)`/g, '<code>$1</code>')
            .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
            .replace(/\n/g, '<br>');
        div.innerHTML = html;

        if (sources && sources.length > 0) {
            const src = document.createElement('div');
            src.className = 'ai-sources';
            src.textContent = '📚 ' + sources.slice(0, 3).join(' · ');
            div.appendChild(src);
        }

        msgs.appendChild(div);
        msgs.scrollTop = msgs.scrollHeight;
        return div;
    }

    function showTyping() {
        const msgs = document.getElementById('ai-messages');
        const div = document.createElement('div');
        div.className = 'ai-msg assistant ai-typing';
        div.id = 'ai-typing-indicator';
        div.innerHTML = '<span></span><span></span><span></span>';
        msgs.appendChild(div);
        msgs.scrollTop = msgs.scrollHeight;
    }

    function hideTyping() {
        const el = document.getElementById('ai-typing-indicator');
        if (el) el.remove();
    }

    function updateContextBar() {
        const bar = document.getElementById('ai-context-bar');
        if (!bar) return;
        const ctx = detectContext();
        const parts = [];
        if (ctx.page)         parts.push('📄 ' + ctx.page);
        if (ctx.interface_id) parts.push('🔌 ' + ctx.interface_id.slice(0, 8) + '…');
        if (ctx.message_type) parts.push('💬 ' + ctx.message_type);
        bar.textContent = parts.length ? parts.join('  ·  ') : 'No context detected';
    }

    function escapeHtml(s) {
        return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
    }

    function renderMarkdownInline(text) {
        return escapeHtml(text)
            .replace(/```([\s\S]*?)```/g, '<pre>$1</pre>')
            .replace(/`([^`]+)`/g, '<code>$1</code>')
            .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
            .replace(/\n/g, '<br>');
    }

    // ── Feedback widget ────────────────────────────────────────────────────────
    // Appended after every assistant response. Sends thumbs up/down + optional comment.
    // Positive responses are also ingested into the local KB to improve future answers.

    function appendFeedbackWidget(msgEl, opts) {
        // opts: { endpoint, promptPreview, responsePreview, sessionId, interfaceId, pipelineId, stepId }
        const widget = document.createElement('div');
        widget.className = 'ai-feedback-widget';
        widget.style.cssText = 'display:flex;align-items:center;gap:8px;margin-top:8px;padding-top:6px;border-top:1px solid rgba(0,0,0,0.07);font-size:12px;color:#6b7280';
        widget.innerHTML = `
            <span style="opacity:0.7">Helpful?</span>
            <button class="ai-fb-btn" data-v="positive" title="Yes, helpful" style="background:none;border:none;cursor:pointer;font-size:15px;padding:2px 4px;border-radius:4px;opacity:0.6">👍</button>
            <button class="ai-fb-btn" data-v="negative" title="Not helpful" style="background:none;border:none;cursor:pointer;font-size:15px;padding:2px 4px;border-radius:4px;opacity:0.6">👎</button>
            <span class="ai-fb-status" style="font-size:11px;color:#059669"></span>
        `;

        // Comment box (hidden by default, shown after thumbs down)
        const commentBox = document.createElement('div');
        commentBox.style.cssText = 'display:none;margin-top:6px;';
        commentBox.innerHTML = `
            <textarea placeholder="What was wrong? (optional — helps improve AI)" rows="2"
              style="width:100%;font-size:12px;border:1px solid #d1d5db;border-radius:6px;padding:6px 8px;resize:none;box-sizing:border-box;color:#374151"></textarea>
            <button style="margin-top:4px;font-size:11px;padding:3px 10px;border:1px solid #d1d5db;border-radius:4px;background:#f9fafb;cursor:pointer">Send</button>
        `;
        msgEl.appendChild(widget);
        msgEl.appendChild(commentBox);

        const statusEl = widget.querySelector('.ai-fb-status');

        async function submitFeedback(sentiment, comment) {
            const payload = {
                endpoint:         opts.endpoint || 'ask',
                sentiment,
                prompt_preview:   (opts.promptPreview   || '').slice(0, 200),
                response_preview: (opts.responsePreview || '').slice(0, 200),
                comment:          comment || '',
                session_id:       opts.sessionId   || '',
                interface_id:     opts.interfaceId  || '',
                pipeline_id:      opts.pipelineId   || '',
                step_id:          opts.stepId       || '',
            };
            try {
                await fetch(`${API_BASE}/feedback/response`, {
                    method: 'POST', credentials: 'include',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload),
                });
                statusEl.textContent = sentiment === 'positive' ? '✅ Thanks!' : '✅ Noted';
                commentBox.style.display = 'none';
                // Dim the buttons so they can't be clicked again
                widget.querySelectorAll('.ai-fb-btn').forEach(b => {
                    b.disabled = true;
                    b.style.opacity = b.dataset.v === sentiment ? '1' : '0.25';
                });
            } catch { statusEl.textContent = '(feedback failed)'; }
        }

        widget.querySelectorAll('.ai-fb-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const sentiment = btn.dataset.v;
                if (sentiment === 'negative') {
                    // Show comment box before submitting
                    commentBox.style.display = 'block';
                    const ta = commentBox.querySelector('textarea');
                    const sendBtn = commentBox.querySelector('button');
                    sendBtn.onclick = () => submitFeedback('negative', ta.value.trim());
                    ta.focus();
                } else {
                    submitFeedback('positive', '');
                }
            });
        });
    }

    // ── Event wiring ───────────────────────────────────────────────────────────
    let isThinking = false;

    async function checkStatus() {
        try {
            const resp = await fetch(`${API_BASE}/status`, { credentials: 'include' });
            const text = await resp.text();
            let json;
            try { json = JSON.parse(text); } catch { return; }
            if (json && json.data && !json.data.ollama_reachable) {
                renderMessage('system',
                    '⚠️ Ollama is not reachable. Start Ollama and run your model to enable AI responses.');
            }
        } catch { /* silent — if Go is down, the first real message will surface the error */ }
    }

    function openDrawer() {
        const panel = document.getElementById('ai-panel');
        const wasAlreadyOpen = panel.classList.contains('open');
        panel.classList.add('open');
        document.getElementById('ai-backdrop').classList.add('visible');
        updateContextBar();
        document.getElementById('ai-input').focus();
        if (!wasAlreadyOpen) {
            checkStatus();
            loadAndRenderHistory();
        }
    }

    function closeDrawer() {
        document.getElementById('ai-panel').classList.remove('open');
        document.getElementById('ai-backdrop').classList.remove('visible');
    }

    function wireEvents() {
        const toggleBtn = document.getElementById('ai-toggle-btn');
        const closeBtn  = document.getElementById('ai-close-btn');
        const newBtn    = document.getElementById('ai-new-btn');
        const backdrop  = document.getElementById('ai-backdrop');
        const input     = document.getElementById('ai-input');
        const sendBtn   = document.getElementById('ai-send-btn');

        toggleBtn.addEventListener('click', openDrawer);
        closeBtn.addEventListener('click', closeDrawer);
        backdrop.addEventListener('click', closeDrawer);
        if (newBtn) newBtn.addEventListener('click', newConversation);

        // Close on Escape
        document.addEventListener('keydown', e => { if (e.key === 'Escape') closeDrawer(); });

        // Quick action chips
        document.querySelectorAll('.ai-quick-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                input.value = btn.dataset.q;
                sendMessage();
            });
        });

        // Send on Enter (Shift+Enter = newline)
        input.addEventListener('keydown', e => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendMessage();
            }
        });
        // Auto-resize textarea
        input.addEventListener('input', () => {
            input.style.height = 'auto';
            input.style.height = Math.min(input.scrollHeight, 100) + 'px';
        });

        sendBtn.addEventListener('click', sendMessage);
    }

    async function sendMessage() {
        if (isThinking) return;
        const input   = document.getElementById('ai-input');
        const sendBtn = document.getElementById('ai-send-btn');
        const question = input.value.trim();
        if (!question) return;

        input.value = '';
        input.style.height = 'auto';
        renderMessage('user', question);

        isThinking = true;
        sendBtn.disabled = true;
        showTyping();

        try {
            hideTyping();
            // Create the assistant message bubble now so tokens stream into it
            const msgs = document.getElementById('ai-messages');
            const bubble = document.createElement('div');
            bubble.className = 'ai-msg assistant';
            bubble.textContent = '…';
            msgs.appendChild(bubble);
            msgs.scrollTop = msgs.scrollHeight;

            const fullResponse = await askAIStream(question, bubble);
            const ctx = detectContext();
            appendFeedbackWidget(bubble, {
                endpoint:        'ask',
                promptPreview:   question,
                responsePreview: typeof fullResponse === 'string' ? fullResponse : bubble.textContent,
                interfaceId:     ctx.interface_id || '',
                pipelineId:      ctx.pipeline_id  || '',
            });
        } catch (err) {
            hideTyping();
            renderMessage('assistant', '⚠️ ' + err.message);
        } finally {
            isThinking = false;
            sendBtn.disabled = false;
            input.focus();
        }
    }

    // ── Public API ─────────────────────────────────────────────────────────────
    window.AIAssistant = {
        /**
         * Programmatically open the drawer and send a question.
         *   AIAssistant.ask('Why does this 835 keep failing?');
         */
        ask(question) {
            openDrawer();
            const input = document.getElementById('ai-input');
            if (input) { input.value = question; sendMessage(); }
        },

        /**
         * Open the drawer pre-seeded with context and an optional question.
         * Perfect for "Ask AI about this step" buttons.
         *   AIAssistant.openWithContext({ step_id: '...', pipeline_id: '...' }, 'Explain this step');
         */
        openWithContext(ctx, initialQuestion) {
            window.AI_CONTEXT = { ...detectContext(), ...ctx };
            updateContextBar();
            openDrawer();
            if (initialQuestion) {
                const input = document.getElementById('ai-input');
                if (input) { input.value = initialQuestion; sendMessage(); }
            }
        },

        /**
         * Generate a JavaScript transform(input) script and stream tokens to onToken.
         * Returns a promise resolving to the full generated script string.
         *
         * opts: { description, stepId?, pipelineId?, interfaceId?, messageType?,
         *         stepType?, currentScript? }
         * onToken: (token, fullSoFar) => void
         */
        generateScript: generateScriptStream,

        /**
         * Stream a plain-English explanation of a pipeline step.
         * Returns a promise resolving to the full explanation string.
         */
        explainStep: explainStepStream,

        /**
         * Fetch the step execution trace for a message + stream LLM failure analysis.
         * onTrace: (traceObject) => void  — called once with the raw trace
         * onToken: (token, fullSoFar) => void  — called per LLM token
         */
        traceMessage: traceMessageStream,

        /**
         * Detect format of a raw message.
         * Returns a promise resolving to { format, version, transaction_type, confidence, description }
         */
        detectFormat,

        /**
         * Suggest mappings from a sample message to a target format.
         * Returns a promise resolving to { suggestions: [...], count, sources }
         */
        suggestMappings,

        /**
         * Explain a processing error in plain English.
         * Returns a promise resolving to { summary, root_cause, suggestions[], severity }
         */
        explainError,

        /**
         * Record that a mapping suggestion was accepted or rejected.
         */
        feedbackMapping(interfaceId, sourceField, targetField, accepted, reasoning) {
            return sendFeedback('mapping', { interface_id: interfaceId, source_field: sourceField,
                target_field: targetField, accepted, reasoning: reasoning || '' });
        },

        /**
         * Record that an error was resolved with a known solution.
         */
        feedbackErrorResolved(interfaceId, errorMessage, solution) {
            return sendFeedback('error-resolved', { interface_id: interfaceId,
                error_message: errorMessage, solution });
        },

        /**
         * Update the page context. Call when user navigates to a different step/interface.
         */
        setContext(ctx) {
            window.AI_CONTEXT = { ...detectContext(), ...ctx };
            updateContextBar();
        }
    };

    // ── Init ───────────────────────────────────────────────────────────────────
    async function init() {
        if (document.getElementById(WIDGET_ID)) return; // already mounted
        injectStyles();
        buildWidget();
        wireEvents();
        updateContextBar();

        // Resolve user identity so session keys are user-scoped
        await resolveCurrentUser();
        if (_userName) {
            const sub = document.getElementById('ai-user-subtitle');
            if (sub) sub.textContent = `Signed in as ${_userName}`;
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
