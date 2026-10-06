/**
 * DICOMStepBuilder — step config builder for the DICOM pipeline step.
 *
 * Registers one step type with StepBuilderRegistry:
 *   - dicom.parse  Parse a raw DICOM Part 10 file into metadata JSON
 *                  (patient/study/series/instance identifiers; never pixel data)
 *
 * Builder contract (StepBuilderRegistry), matching EDIStepBuilder.js exactly:
 *   render(step)         → string  HTML for the properties panel form tab
 *   collectConfig(step)  → void    reads DOM, writes into step.config
 *   destroy()            → void    tears down event listeners / AC refs
 */

function dicomEsc(s) {
    return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// ── DicomParseStepBuilder ────────────────────────────────────────────────────
// Config: { sourceField, outputField }

class DicomParseStepBuilder {
    constructor(panel) {
        this._panel = panel;
        this._ac = new AbortController();
    }

    render(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;

        const sourceField = dicomEsc(cfg.sourceField || 'raw');
        const outputField = dicomEsc(cfg.outputField || 'parsedDICOM');

        return `
        <div class="dicom-step-config">
            <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:6px;padding:0.65rem 0.85rem;margin-bottom:1rem;font-size:0.8rem;color:#1e40af;">
                <strong>Parse step.</strong> Extracts metadata (patient ID, patient name, study/series/
                instance UIDs, modality, accession number, manufacturer) from a DICOM Part 10 file into
                structured JSON. Pixel data is never read or inlined — the full image stays intact,
                unmodified, inside the file itself. Note: content received by a
                <code>dicom_storage_inbound</code> connector is already parsed automatically before any
                pipeline step runs — this step is for re-parsing DICOM content that shows up mid-pipeline
                from elsewhere (a DB lookup, a mid-pipeline connector pull, etc.).
            </div>
            <div class="config-group" style="margin-bottom:1.1rem;">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Source Field</label>
                <input id="dicomParseSourceField" type="text" class="form-control form-control-sm"
                    value="${sourceField}" placeholder="raw"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field containing the raw DICOM Part 10 content.</div>
            </div>
            <div class="config-group">
                <label style="font-size:0.75rem;font-weight:600;text-transform:uppercase;color:#64748b;display:block;margin-bottom:0.4rem;">Output Field</label>
                <input id="dicomParseOutputField" type="text" class="form-control form-control-sm"
                    value="${outputField}" placeholder="parsedDICOM"
                    style="font-family:monospace;font-size:0.82rem;">
                <div style="font-size:0.72rem;color:#94a3b8;margin-top:0.25rem;">Pipeline field where the parsed metadata JSON will be written.</div>
            </div>
        </div>`;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const sourceEl = form.querySelector('#dicomParseSourceField');
        const outputEl = form.querySelector('#dicomParseOutputField');

        if (sourceEl) step.config.sourceField = sourceEl.value.trim() || 'raw';
        if (outputEl) step.config.outputField = outputEl.value.trim() || 'parsedDICOM';
    }

    destroy() {
        this._ac.abort();
    }
}

// ── registration ─────────────────────────────────────────────────────────

StepBuilderRegistry.register('dicom.parse', DicomParseStepBuilder);
