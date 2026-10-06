/**
 * ASTMStepBuilder — step config builders for ASTM E1394-97 pipeline steps.
 *
 * Registers three step types with StepBuilderRegistry:
 *   - astm.parse     Parse raw ASTM content (H/P/O/R/C/Q/L records) into JSON
 *   - astm.validate  Check a parsed ASTM message against the schema (component
 *                    data types, required fields)
 *   - astm.build     Build a complete ASTM record stream from canonical JSON
 *
 * Builder contract (StepBuilderRegistry), matching EDIStepBuilder.js exactly:
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 *
 * All three executors (astm_parse_executor.go/astm_validate_executor.go/
 * astm_build_executor.go) take only sourceField/outputField — no step has any
 * further config, unlike edi.build's own ISA/GS sender-id fields — so all
 * three builders below share the identical shape, differing only in label
 * text and default field names.
 */

function astmEsc(s) {
    return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function astmRenderSourceOutput(idPrefix, sourceField, outputField, sourceDefault, outputDefault, sourceHint, outputHint) {
    return `
        <div class="config-group" style="margin-bottom:1.1rem;">
            <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
            <input id="${idPrefix}SourceField" type="text" class="form-control form-control-sm"
                value="${astmEsc(sourceField)}" placeholder="${astmEsc(sourceDefault)}"
                style="font-family:monospace;font-size:0.82rem;">
            <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">${astmEsc(sourceHint)}</div>
        </div>
        <div class="config-group">
            <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
            <input id="${idPrefix}OutputField" type="text" class="form-control form-control-sm"
                value="${astmEsc(outputField)}" placeholder="${astmEsc(outputDefault)}"
                style="font-family:monospace;font-size:0.82rem;">
            <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">${astmEsc(outputHint)}</div>
        </div>`;
}

function astmCollectSourceOutput(idPrefix, step, sourceDefault, outputDefault) {
    const form = document.querySelector('.properties-form') || document;
    step.config = step.config || {};

    const sourceEl = form.querySelector(`#${idPrefix}SourceField`);
    const outputEl = form.querySelector(`#${idPrefix}OutputField`);

    if (sourceEl) step.config.sourceField = sourceEl.value.trim() || sourceDefault;
    if (outputEl) step.config.outputField = outputEl.value.trim() || outputDefault;
}

// ── AstmParseStepBuilder ─────────────────────────────────────────────────────
// Config: { sourceField: "raw", outputField: "parsedASTM" }

class AstmParseStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        return `
        <div class="astm-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Parse step.</strong> Converts raw ASTM E1394-97 content (H/P/O/R/C/Q/L records)
                into structured JSON. Note: content received by a <code>serial_inbound</code> or
                <code>astm_tcp_inbound</code> connector is already parsed automatically before any
                pipeline step runs — this step is for re-parsing ASTM content that shows up mid-pipeline
                from elsewhere.
            </div>
            ${astmRenderSourceOutput('astmParse', cfg.sourceField || 'raw', cfg.outputField || 'parsedASTM',
                'raw', 'parsedASTM',
                'Pipeline field containing the raw ASTM record stream.',
                'Pipeline field where the parsed JSON will be written.')}
        </div>`;
    }

    collectConfig(step) { astmCollectSourceOutput('astmParse', step, 'raw', 'parsedASTM'); }
    destroy() { this._ac.abort(); }
}

// ── AstmValidateStepBuilder ──────────────────────────────────────────────────
// Config: { sourceField: "parsedASTM", outputField: "astmValidation" }

class AstmValidateStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        return `
        <div class="astm-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Validate step.</strong> Checks a parsed ASTM message's component fields against
                the schema's own data types (numeric/date fields, etc.) and reports missing required
                fields. Never blocks the pipeline on a missing field — only malformed values are
                reported as errors.
            </div>
            ${astmRenderSourceOutput('astmValidate', cfg.sourceField || 'parsedASTM', cfg.outputField || 'astmValidation',
                'parsedASTM', 'astmValidation',
                'Pipeline field containing astm.parse\'s own output (or raw content, parsed again).',
                'Pipeline field where the validation result will be written.')}
        </div>`;
    }

    collectConfig(step) { astmCollectSourceOutput('astmValidate', step, 'parsedASTM', 'astmValidation'); }
    destroy() { this._ac.abort(); }
}

// ── AstmBuildStepBuilder ─────────────────────────────────────────────────────
// Config: { sourceField: "parsedASTM", outputField: "astmMessage" }

class AstmBuildStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        return `
        <div class="astm-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Build step.</strong> Builds a complete ASTM record stream (H/P/O/R/C/Q/L) from
                canonical JSON — accepts <code>astm.parse</code>'s own output shape (a round trip) or
                any data shaped the same way.
            </div>
            ${astmRenderSourceOutput('astmBuild', cfg.sourceField || 'parsedASTM', cfg.outputField || 'astmMessage',
                'parsedASTM', 'astmMessage',
                'Pipeline field containing the canonical header/patientBlocks/queryBlocks/trailer JSON.',
                'Pipeline field where the built ASTM record stream (a string) will be written.')}
        </div>`;
    }

    collectConfig(step) { astmCollectSourceOutput('astmBuild', step, 'parsedASTM', 'astmMessage'); }
    destroy() { this._ac.abort(); }
}

// ── registration ─────────────────────────────────────────────────────────

StepBuilderRegistry.register('astm.parse', AstmParseStepBuilder);
StepBuilderRegistry.register('astm.validate', AstmValidateStepBuilder);
StepBuilderRegistry.register('astm.build', AstmBuildStepBuilder);
