/**
 * FileParserBuilder — extracted from PropertiesPanel.createFileParserUI().
 * Owns the HTML rendering, event wiring, template cache, and AC ref for File Parser steps.
 *
 * Registered with StepBuilderRegistry as 'file_parser'.
 */
class FileParserBuilder {
    constructor(panel) {
        this._panel = panel;
        this._fpSourceFieldAC = null;
        this._fpTemplateCache = null;
    }

    // ── Public builder contract ──────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        // Capture render-time flags needed inside _attachEvents
        this._isFieldAsPath = (step.config.sourceType || 'field') === 'field_as_path';
        this._currentTemplate = step.config.template || '';
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(), 0);
        return html;
    }

    collectConfig(step) {
        const form = document.querySelector('.properties-form') || document;
        step.config = step.config || {};

        const fpSourceTypeRadio = form.querySelector('input[name="fpSourceType"]:checked');
        const sourceType = fpSourceTypeRadio ? fpSourceTypeRadio.value : 'field';
        step.config.sourceType = sourceType;
        if (sourceType === 'local_path') {
            const fpFilePath = form.querySelector('#fpFilePath');
            if (fpFilePath) step.config.filePath = fpFilePath.value.trim();
            const fpBatchMode = form.querySelector('#fpBatchMode');
            step.config.batchMode = fpBatchMode ? fpBatchMode.checked : false;
            const fpFilePattern = form.querySelector('#fpFilePattern');
            if (fpFilePattern) step.config.filePattern = fpFilePattern.value.trim();
        } else {
            const fpSourceField = form.querySelector('#fpSourceField');
            if (fpSourceField) step.config.sourceField = fpSourceField.value;
        }

        const fpAutoDetect = form.querySelector('#fpAutoDetect');
        step.config.autoDetect = fpAutoDetect ? fpAutoDetect.checked : false;

        const fpFileFormat = form.querySelector('#fpFileFormat');
        if (fpFileFormat) step.config.fileFormat = fpFileFormat.value;
        else if (step.config.autoDetect) step.config.fileFormat = 'auto';

        const fpDelimiter = form.querySelector('#fpDelimiter');
        if (fpDelimiter) step.config.delimiter = fpDelimiter.value;

        const fpHasHeader = form.querySelector('#fpHasHeader');
        if (fpHasHeader) step.config.hasHeader = fpHasHeader.checked;

        const fpTrimFields = form.querySelector('#fpTrimFields');
        if (fpTrimFields) step.config.trimFields = fpTrimFields.checked;

        const fpSkipRows = form.querySelector('#fpSkipRows');
        if (fpSkipRows) step.config.skipRows = parseInt(fpSkipRows.value) || 0;

        const fpMaxRecords = form.querySelector('#fpMaxRecords');
        if (fpMaxRecords) step.config.maxRecords = parseInt(fpMaxRecords.value) || 0;

        const fpMaxFileSizeMB = form.querySelector('#fpMaxFileSizeMB');
        if (fpMaxFileSizeMB) step.config.maxFileSizeMB = parseInt(fpMaxFileSizeMB.value) || 0;

        const fpSheetName = form.querySelector('#fpSheetName');
        if (fpSheetName) step.config.sheetName = fpSheetName.value;

        const fpSheetIndex = form.querySelector('#fpSheetIndex');
        if (fpSheetIndex) step.config.sheetIndex = parseInt(fpSheetIndex.value) || 0;

        const fpContentEncoding = form.querySelector('#fpContentEncoding');
        if (fpContentEncoding) step.config.contentEncoding = fpContentEncoding.value;

        const fpTemplate = form.querySelector('#fpTemplate');
        if (fpTemplate) step.config.template = fpTemplate.value;

        const columnsBody = form.querySelector('#fpColumnsBody');
        if (columnsBody) {
            const columns = [];
            columnsBody.querySelectorAll('tr').forEach(row => {
                const name = row.querySelector('.fp-col-name')?.value || '';
                const start = parseInt(row.querySelector('.fp-col-start')?.value) || 1;
                const length = parseInt(row.querySelector('.fp-col-length')?.value) || 1;
                if (name) columns.push({ name, start, length });
            });
            step.config.columns = columns;
        }

        console.log('[FileParserBuilder] ✅ Saved File Parser config:', step.config);
    }

    destroy() {
        if (this._fpSourceFieldAC) {
            try { this._fpSourceFieldAC.destroy(); } catch (_) {}
            this._fpSourceFieldAC = null;
        }
    }

    // ── Private: HTML construction ───────────────────────────────────────────

    _buildHTML(step) {
        const sourceType    = step.config.sourceType || 'field';
        const sourceField   = step.config.sourceField || '';
        const filePath      = step.config.filePath || '';
        const batchMode     = step.config.batchMode === true;
        const filePattern   = step.config.filePattern || '';
        const isLocalPath   = sourceType === 'local_path';
        const isFieldAsPath = sourceType === 'field_as_path';

        const autoDetect    = step.config.autoDetect === true;
        const fileFormat    = step.config.fileFormat || (autoDetect ? 'auto' : 'csv');
        const delimiter     = step.config.delimiter || ',';
        const hasHeader     = step.config.hasHeader !== false;
        const trimFields    = step.config.trimFields !== false;
        const skipRows      = step.config.skipRows || 0;
        const maxRecords    = step.config.maxRecords || 0;
        const maxFileSizeMB = step.config.maxFileSizeMB || 0;
        const columns       = step.config.columns || [];
        const sheetName     = step.config.sheetName || '';
        const sheetIndex    = step.config.sheetIndex || 0;
        const template      = step.config.template || '';
        const contentEncoding = step.config.contentEncoding || '';

        const isAuto      = fileFormat === 'auto' || autoDetect;
        const isFixedWidth = fileFormat === 'fixed_width';
        const isExcel     = fileFormat === 'xlsx' || fileFormat === 'xls';
        const isAvro      = fileFormat === 'avro';
        const isParquet   = fileFormat === 'parquet';
        const isBinary    = isExcel || isAvro || isParquet;
        const isDelimited = !isFixedWidth && !isBinary && !isAuto;

        const templateOptions = template
            ? `<option value="${template}" selected>${template}</option>`
            : '';

        let columnRowsHtml = columns.map((col, idx) =>
            `<tr data-col-idx="${idx}">
                <td><input type="text" class="form-control form-control-sm fp-col-name" value="${col.name || ''}" placeholder="Column name"></td>
                <td><input type="number" class="form-control form-control-sm fp-col-start" value="${col.start || 1}" min="1" style="width:70px"></td>
                <td><input type="number" class="form-control form-control-sm fp-col-length" value="${col.length || 1}" min="1" style="width:70px"></td>
                <td><button type="button" class="btn btn-sm btn-outline-danger fp-remove-col-btn" title="Remove column">&times;</button></td>
            </tr>`
        ).join('');

        const section = document.createElement('div');
        section.className = 'form-section';
        section.innerHTML = `
            <h4><i class="fas fa-file-csv" style="color: var(--primary-color, #007bff); margin-right: 6px;"></i>File Parser Configuration</h4>

            <div class="form-group">
                <label style="font-weight: 600;">Source Type <span style="color: var(--danger-color, #dc3545);">*</span></label>
                <div class="btn-group btn-group-sm w-100" role="group" style="display:flex;">
                    <input type="radio" class="btn-check" name="fpSourceType" id="fpSourceTypeField"
                        value="field" autocomplete="off" ${sourceType === 'field' ? 'checked' : ''}>
                    <label class="btn btn-outline-secondary flex-fill" for="fpSourceTypeField"
                        style="font-size:0.8rem; padding:4px 8px;">
                        <i class="fas fa-exchange-alt" style="margin-right:4px;"></i>Field Content
                    </label>
                    <input type="radio" class="btn-check" name="fpSourceType" id="fpSourceTypeFieldAsPath"
                        value="field_as_path" autocomplete="off" ${isFieldAsPath ? 'checked' : ''}>
                    <label class="btn btn-outline-secondary flex-fill" for="fpSourceTypeFieldAsPath"
                        style="font-size:0.8rem; padding:4px 8px;">
                        🔗 Field URI
                    </label>
                    <input type="radio" class="btn-check" name="fpSourceType" id="fpSourceTypeLocal"
                        value="local_path" autocomplete="off" ${isLocalPath ? 'checked' : ''}>
                    <label class="btn btn-outline-secondary flex-fill" for="fpSourceTypeLocal"
                        style="font-size:0.8rem; padding:4px 8px;">
                        📂 Local Path
                    </label>
                </div>
            </div>

            <div id="fpFieldSourceGroup" style="display: ${isLocalPath ? 'none' : 'block'};">
                <div class="form-group">
                    <label>Source Field <span style="color: var(--danger-color, #dc3545);">*</span></label>
                    <input type="text" id="fpSourceField" class="form-control" value="${sourceField}"
                        placeholder="${isFieldAsPath ? 'steps.s3_connector.file_uri' : 'enriched.connector_result.content'}">
                    <small id="fpSourceFieldHelp" class="form-text text-muted">${
                        isFieldAsPath
                            ? 'Field containing a file URI — resolved at runtime. Supported: <code>s3://bucket/key</code>, <code>https://host/file</code>, <code>/data/file.csv</code>.'
                            : 'Field containing raw file content (bytes/string from inbound connector)'
                    }</small>
                </div>
            </div>

            <div id="fpLocalSourceGroup" style="display: ${isLocalPath ? 'block' : 'none'};">
                <div class="form-group">
                    <label>File Path or Glob Pattern <span style="color: var(--danger-color, #dc3545);">*</span></label>
                    <div class="input-group">
                        <input type="text" id="fpFilePath" class="form-control" value="${filePath}"
                            placeholder="/data/claims/cclf1.dat  or  /data/claims/*.csv">
                        <div class="input-group-append">
                            <button type="button" id="fpBrowseBtn" class="btn btn-outline-secondary"
                                title="Browse server file system" style="white-space:nowrap;">
                                <i class="fas fa-folder-open"></i> Browse
                            </button>
                        </div>
                    </div>
                    <small class="form-text text-muted">Absolute path on the server. Glob patterns enable batch mode.</small>
                </div>
                <div class="form-group" style="display: flex; align-items: center; gap: 10px;">
                    <label class="d-flex align-items-center mb-0" style="font-weight: 600; cursor: pointer;">
                        <input type="checkbox" id="fpBatchMode" ${batchMode ? 'checked' : ''} style="margin-right: 6px;">
                        Batch Mode
                        <span style="margin-left: 6px; font-size: 0.75rem; background: #17a2b8; color: #fff; padding: 1px 6px; border-radius: 10px; font-weight: 400;">Folder</span>
                    </label>
                </div>
                <small class="form-text text-muted" style="margin-top: -8px; margin-bottom: 8px; display: block;">
                    Process all files matching the path/glob. Records are merged with a <code>_source_file</code> column added.
                </small>
                <div id="fpFilePatternGroup" style="display: ${batchMode ? 'block' : 'none'};">
                    <div class="form-group">
                        <label>File Pattern <small class="text-muted">(optional)</small></label>
                        <input type="text" id="fpFilePattern" class="form-control" value="${filePattern}"
                            placeholder="*.csv  or  cclf_*.dat">
                    </div>
                </div>
            </div>

            <div class="form-group">
                <label class="d-flex align-items-center" style="font-weight: 600;">
                    <input type="checkbox" id="fpAutoDetect" ${autoDetect ? 'checked' : ''} style="margin-right: 6px;">
                    Auto-Detect Format
                    <span style="margin-left: 6px; font-size: 0.75rem; background: var(--primary-color, #007bff); color: #fff; padding: 1px 6px; border-radius: 10px; font-weight: 400;">Smart</span>
                </label>
                <small class="form-text text-muted">Automatically detect CSV/TSV/fixed-width/XLSX/XLS from content</small>
            </div>

            <div id="fpManualFormatGroup" style="display: ${autoDetect ? 'none' : 'block'};">
                <div class="form-group">
                    <label>File Format <span style="color: var(--danger-color, #dc3545);">*</span></label>
                    <select id="fpFileFormat" class="form-control">
                        <option value="csv"         ${fileFormat === 'csv'         ? 'selected' : ''}>CSV (Comma Separated)</option>
                        <option value="tsv"         ${fileFormat === 'tsv'         ? 'selected' : ''}>TSV (Tab Separated)</option>
                        <option value="fixed_width" ${fileFormat === 'fixed_width' ? 'selected' : ''}>Fixed Width / Positional (CCLF, NACHA, X12)</option>
                        <option value="xlsx"        ${fileFormat === 'xlsx'        ? 'selected' : ''}>Excel (.xlsx)</option>
                        <option value="xls"         ${fileFormat === 'xls'         ? 'selected' : ''}>Excel Legacy (.xls)</option>
                        <option value="avro"        ${fileFormat === 'avro'        ? 'selected' : ''}>Apache Avro (.avro)</option>
                        <option value="parquet"     ${fileFormat === 'parquet'     ? 'selected' : ''}>Apache Parquet (.parquet)</option>
                    </select>
                </div>
            </div>

            <div id="fpDelimiterGroup" class="form-group" style="display: ${isDelimited ? 'block' : 'none'};">
                <label>Delimiter</label>
                <input type="text" id="fpDelimiter" class="form-control" value="${delimiter}" placeholder="," maxlength="2" style="width: 80px;">
            </div>

            <div class="form-group" id="fpHasHeaderGroup" style="display: ${isExcel || isFixedWidth || isDelimited ? 'block' : 'none'};">
                <label class="d-flex align-items-center">
                    <input type="checkbox" id="fpHasHeader" ${hasHeader ? 'checked' : ''} style="margin-right: 6px;">
                    First row contains column names
                </label>
            </div>

            <div id="fpTemplateSection" style="display: ${isFixedWidth ? 'block' : 'none'};">
                <div class="form-group" style="margin-bottom: 0;">
                    <label>OOB Template</label>
                    <select id="fpTemplate" class="form-control">
                        <option value="">— Manual column definitions —</option>
                        ${templateOptions}
                    </select>
                    <small class="form-text text-muted">Pre-built column layouts for CCLF, NACHA, and remittance formats</small>
                </div>
                <div id="fpTemplatePreview" style="display: none; margin-top: 8px; margin-bottom: 12px; border: 1px solid #e5e7eb; border-radius: 6px; overflow: hidden;">
                    <div id="fpTemplateConfidenceBar" style="padding: 8px 12px; display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap;">
                        <span id="fpTemplateConfidenceBadge"></span>
                        <span id="fpTemplateConfidenceNote" style="font-size: 0.79rem; line-height: 1.4;"></span>
                    </div>
                    <div style="overflow-x: auto; max-height: 210px; overflow-y: auto;">
                        <table style="width: 100%; border-collapse: collapse; font-size: 0.8rem;">
                            <thead>
                                <tr style="background: #f3f4f6; position: sticky; top: 0; z-index: 1;">
                                    <th style="padding: 5px 8px; text-align: left; border-bottom: 1px solid #e5e7eb; color: #6b7280; font-weight: 600;">#</th>
                                    <th style="padding: 5px 8px; text-align: left; border-bottom: 1px solid #e5e7eb; color: #6b7280; font-weight: 600;">Column Name</th>
                                    <th style="padding: 5px 8px; text-align: right; border-bottom: 1px solid #e5e7eb; color: #6b7280; font-weight: 600;">Start</th>
                                    <th style="padding: 5px 8px; text-align: right; border-bottom: 1px solid #e5e7eb; color: #6b7280; font-weight: 600;">Length</th>
                                    <th style="padding: 5px 8px; text-align: right; border-bottom: 1px solid #e5e7eb; color: #6b7280; font-weight: 600;">End</th>
                                </tr>
                            </thead>
                            <tbody id="fpTemplateColumnsBody"></tbody>
                        </table>
                    </div>
                </div>
            </div>

            <div id="fpColumnsSection" style="display: ${isFixedWidth ? 'block' : 'none'};">
                <div class="form-group">
                    <label>Column Definitions <span style="color: var(--danger-color, #dc3545);">*</span></label>
                    <small class="form-text text-muted mb-2">Select an OOB template above to pre-fill columns, then edit freely.</small>
                    <table class="table table-sm table-bordered" style="font-size: 0.85rem;">
                        <thead><tr><th>Name</th><th>Start</th><th>Length</th><th style="width: 40px;"></th></tr></thead>
                        <tbody id="fpColumnsBody">${columnRowsHtml}</tbody>
                    </table>
                    <button type="button" id="fpAddColumnBtn" class="btn btn-sm btn-outline-primary">
                        <i class="fas fa-plus"></i> Add Column
                    </button>
                </div>
            </div>

            <div id="fpExcelSection" style="display: ${isExcel ? 'block' : 'none'};">
                <div class="row">
                    <div class="col-6">
                        <div class="form-group">
                            <label>Sheet Name</label>
                            <input type="text" id="fpSheetName" class="form-control" value="${sheetName}" placeholder="(default: first sheet)">
                        </div>
                    </div>
                    <div class="col-6">
                        <div class="form-group">
                            <label>Sheet Index</label>
                            <input type="number" id="fpSheetIndex" class="form-control" value="${sheetIndex}" min="0">
                            <small class="form-text text-muted">0-based (0 = first sheet)</small>
                        </div>
                    </div>
                </div>
                <div class="form-group">
                    <label>Content Encoding</label>
                    <select id="fpContentEncoding" class="form-control">
                        <option value=""       ${contentEncoding === ''       ? 'selected' : ''}>None (raw binary)</option>
                        <option value="base64" ${contentEncoding === 'base64' ? 'selected' : ''}>Base64 (JSON-serialized binary)</option>
                    </select>
                </div>
            </div>

            <div class="form-group">
                <label class="d-flex align-items-center">
                    <input type="checkbox" id="fpTrimFields" ${trimFields ? 'checked' : ''} style="margin-right: 6px;">
                    Trim leading/trailing whitespace from values
                </label>
            </div>

            <div class="row">
                <div class="col-6">
                    <div class="form-group">
                        <label>Skip Rows</label>
                        <input type="number" id="fpSkipRows" class="form-control" value="${skipRows}" min="0">
                    </div>
                </div>
                <div class="col-6">
                    <div class="form-group">
                        <label>Max Records</label>
                        <input type="number" id="fpMaxRecords" class="form-control" value="${maxRecords}" min="0">
                        <small class="form-text text-muted">0 = unlimited</small>
                    </div>
                </div>
            </div>

            <div class="row">
                <div class="col-6">
                    <div class="form-group">
                        <label>Max File Size (MB)</label>
                        <input type="number" id="fpMaxFileSizeMB" class="form-control" value="${maxFileSizeMB}" min="0" max="500">
                        <small class="form-text text-muted">0 = default (100 MB), cap 500 MB</small>
                    </div>
                </div>
                <div class="col-6" style="display:flex; align-items:flex-end; padding-bottom:4px;">
                    <div id="fpLargeFileWarning" style="display:${(maxRecords === 0 && (maxFileSizeMB === 0 || maxFileSizeMB > 50)) ? 'block' : 'none'}; background:#fff3cd; border:1px solid #ffc107; border-radius:4px; padding:7px 10px; font-size:0.8rem; color:#856404; width:100%;">
                        <i class="fas fa-exclamation-triangle"></i> Large files may use significant memory.
                    </div>
                </div>
            </div>

            <div class="form-group" style="border-top: 1px solid var(--border-color, #dee2e6); padding-top: 12px; margin-top: 4px;">
                <label>Preview Parser</label>
                <div id="fpLocalPreviewNote" style="display: ${isLocalPath ? 'block' : 'none'}; background:#e8f4fd; border:1px solid #b8daff; border-radius:4px; padding:8px 10px; margin-bottom:8px; font-size:0.82rem; color:#004085;">
                    <i class="fas fa-info-circle"></i> Preview will read directly from the file at the path above.
                </div>
                <div id="fpPasteSection" style="display: ${isLocalPath ? 'none' : 'block'};">
                    <textarea id="fpPreviewContent" class="form-control" rows="4"
                        placeholder="Paste sample file content here to preview…"
                        style="font-family: monospace; font-size: 0.8rem;"></textarea>
                </div>
                <div class="d-flex align-items-center mt-1" style="gap: 8px;">
                    <button type="button" id="fpRunPreviewBtn" class="btn btn-sm btn-outline-primary">
                        <i class="fas fa-play"></i> Run Preview
                    </button>
                    <span id="fpPreviewStatus" style="font-size: 0.8rem; color: var(--text-muted, #6c757d);"></span>
                </div>
                <div id="fpPreviewResult" style="display:none; margin-top: 8px; max-height: 200px; overflow: auto;">
                    <table class="table table-sm table-bordered table-striped" style="font-size: 0.78rem; margin-bottom: 0;">
                        <thead id="fpPreviewHead"></thead>
                        <tbody id="fpPreviewBody"></tbody>
                    </table>
                </div>
            </div>
        `;

        return section.outerHTML;
    }

    // ── Private: event wiring (runs inside setTimeout after DOM insertion) ───

    _attachEvents() {
        const isFieldAsPath   = this._isFieldAsPath;
        const currentTemplate = this._currentTemplate;

        // Source type radio
        const fieldSourceGroup = document.getElementById('fpFieldSourceGroup');
        const localSourceGroup = document.getElementById('fpLocalSourceGroup');
        document.querySelectorAll('input[name="fpSourceType"]').forEach(radio => {
            radio.addEventListener('change', () => {
                const isLocal = radio.value === 'local_path';
                const isURI   = radio.value === 'field_as_path';
                if (fieldSourceGroup) fieldSourceGroup.style.display = isLocal ? 'none' : 'block';
                if (localSourceGroup) localSourceGroup.style.display = isLocal ? 'block' : 'none';
                const helpEl    = document.getElementById('fpSourceFieldHelp');
                const srcFieldEl = document.getElementById('fpSourceField');
                if (helpEl) {
                    helpEl.innerHTML = isURI
                        ? 'Field containing a file URI — resolved at runtime. Supported: <code>s3://bucket/key</code>, <code>https://host/file</code>, <code>/data/file.csv</code>.'
                        : 'Field containing raw file content (bytes/string from inbound connector)';
                }
                if (srcFieldEl && !srcFieldEl.value) {
                    srcFieldEl.placeholder = isURI ? 'steps.s3_connector.file_uri' : 'enriched.connector_result.content';
                }
                const pasteSection    = document.getElementById('fpPasteSection');
                const localPreviewNote = document.getElementById('fpLocalPreviewNote');
                if (pasteSection)     pasteSection.style.display     = isLocal ? 'none' : 'block';
                if (localPreviewNote) localPreviewNote.style.display  = isLocal ? 'block' : 'none';
            });
        });

        // IntelliSense on source field
        const fpSourceFieldEl = document.getElementById('fpSourceField');
        if (fpSourceFieldEl && typeof FieldPathSearchComponent !== 'undefined') {
            if (this._fpSourceFieldAC) { try { this._fpSourceFieldAC.destroy(); } catch (_) {} }
            if (typeof this._panel.loadStepVariables === 'function') this._panel.loadStepVariables();
            this._fpSourceFieldAC = new FieldPathSearchComponent(fpSourceFieldEl, {
                includeHL7Fields: false,
                allowCustom: true,
                showCategories: true,
                placeholder: isFieldAsPath ? 'e.g. enriched.sftp.file_uri' : 'e.g. enriched.connector_result.content',
                getStepVariables: () => this._panel.getStepVariablesForSearch ? this._panel.getStepVariablesForSearch() : [],
                onSelect: (path) => {
                    fpSourceFieldEl.value = path;
                    fpSourceFieldEl.dispatchEvent(new Event('input', { bubbles: true }));
                }
            });
        }

        // Batch mode
        const batchModeChk   = document.getElementById('fpBatchMode');
        const filePatternGroup = document.getElementById('fpFilePatternGroup');
        if (batchModeChk && filePatternGroup) {
            batchModeChk.addEventListener('change', () => {
                filePatternGroup.style.display = batchModeChk.checked ? 'block' : 'none';
            });
        }

        // Format visibility
        const autoDetectChk  = document.getElementById('fpAutoDetect');
        const manualFormatGroup = document.getElementById('fpManualFormatGroup');
        const formatSelect   = document.getElementById('fpFileFormat');

        const refreshSections = (fmt, isAuto) => {
            const fixed     = fmt === 'fixed_width';
            const excel     = fmt === 'xlsx' || fmt === 'xls';
            const binary    = excel || fmt === 'avro' || fmt === 'parquet';
            const delimited = !fixed && !binary && !isAuto;
            const get = id => document.getElementById(id);
            if (get('fpDelimiterGroup'))  get('fpDelimiterGroup').style.display  = delimited ? 'block' : 'none';
            if (get('fpColumnsSection'))  get('fpColumnsSection').style.display  = fixed     ? 'block' : 'none';
            if (get('fpTemplateSection')) get('fpTemplateSection').style.display = fixed     ? 'block' : 'none';
            if (get('fpExcelSection'))    get('fpExcelSection').style.display    = excel     ? 'block' : 'none';
            if (get('fpHasHeaderGroup'))  get('fpHasHeaderGroup').style.display  = (fixed || excel || delimited) ? 'block' : 'none';
        };

        if (autoDetectChk) {
            autoDetectChk.addEventListener('change', () => {
                const on = autoDetectChk.checked;
                if (manualFormatGroup) manualFormatGroup.style.display = on ? 'none' : 'block';
                refreshSections(formatSelect ? formatSelect.value : 'csv', on);
            });
        }
        if (formatSelect) {
            formatSelect.addEventListener('change', () => {
                const fmt = formatSelect.value;
                refreshSections(fmt, autoDetectChk ? autoDetectChk.checked : false);
                const delimInput = document.getElementById('fpDelimiter');
                if (delimInput) {
                    if (fmt === 'tsv') delimInput.value = '\t';
                    else if (fmt === 'csv') delimInput.value = ',';
                }
            });
        }

        // Large-file warning
        const updateLargeFileWarning = () => {
            const warn = document.getElementById('fpLargeFileWarning');
            if (!warn) return;
            const mr = parseInt(document.getElementById('fpMaxRecords')?.value) || 0;
            const ms = parseInt(document.getElementById('fpMaxFileSizeMB')?.value) || 0;
            warn.style.display = (mr === 0 && (ms === 0 || ms > 50)) ? 'block' : 'none';
        };
        document.getElementById('fpMaxRecords')?.addEventListener('input', updateLargeFileWarning);
        document.getElementById('fpMaxFileSizeMB')?.addEventListener('input', updateLargeFileWarning);

        // Add/remove column rows
        const addBtn = document.getElementById('fpAddColumnBtn');
        if (addBtn) {
            addBtn.addEventListener('click', () => {
                const tbody = document.getElementById('fpColumnsBody');
                if (!tbody) return;
                const idx = tbody.rows.length;
                const tr = document.createElement('tr');
                tr.setAttribute('data-col-idx', idx);
                tr.innerHTML = `
                    <td><input type="text" class="form-control form-control-sm fp-col-name" placeholder="Column name"></td>
                    <td><input type="number" class="form-control form-control-sm fp-col-start" value="1" min="1" style="width:70px"></td>
                    <td><input type="number" class="form-control form-control-sm fp-col-length" value="1" min="1" style="width:70px"></td>
                    <td><button type="button" class="btn btn-sm btn-outline-danger fp-remove-col-btn" title="Remove column">&times;</button></td>
                `;
                tbody.appendChild(tr);
                tr.querySelector('.fp-remove-col-btn').addEventListener('click', () => tr.remove());
            });
        }
        document.querySelectorAll('.fp-remove-col-btn').forEach(btn => {
            btn.addEventListener('click', () => btn.closest('tr').remove());
        });

        // OOB template picker
        const showTemplatePreview = (tpl) => {
            const preview = document.getElementById('fpTemplatePreview');
            if (!preview || !tpl) { if (preview) preview.style.display = 'none'; return; }
            const confStyles = {
                high:   { bg: '#ecfdf5', border: '#10b981', badgeBg: '#059669', noteColor: '#065f46' },
                medium: { bg: '#fffbeb', border: '#f59e0b', badgeBg: '#d97706', noteColor: '#78350f' },
                low:    { bg: '#fef2f2', border: '#ef4444', badgeBg: '#dc2626', noteColor: '#991b1b' },
            };
            const s = confStyles[tpl.confidence] || confStyles.medium;
            const bar = document.getElementById('fpTemplateConfidenceBar');
            if (bar) { bar.style.background = s.bg; bar.style.borderBottom = `1px solid ${s.border}`; }
            const badge = document.getElementById('fpTemplateConfidenceBadge');
            if (badge) {
                const label = (tpl.confidence || 'unknown');
                badge.textContent = label.charAt(0).toUpperCase() + label.slice(1) + ' Confidence';
                badge.style.cssText = `background:${s.badgeBg}; color:#fff; padding:2px 9px; border-radius:10px; font-size:0.73rem; font-weight:700; white-space:nowrap;`;
            }
            const noteEl = document.getElementById('fpTemplateConfidenceNote');
            if (noteEl) { noteEl.textContent = tpl.confidenceNote || ''; noteEl.style.color = s.noteColor; }
            const tbody = document.getElementById('fpTemplateColumnsBody');
            if (tbody && tpl.columns) {
                const maxEnd = tpl.columns.length ? Math.max(...tpl.columns.map(c => (c.start || 1) + (c.length || 1) - 1)) : 0;
                tbody.innerHTML = tpl.columns.map((col, i) => `
                    <tr style="background:${i % 2 === 0 ? '#fff' : '#f9fafb'};">
                        <td style="padding:4px 8px; border-bottom:1px solid #f3f4f6; color:#9ca3af; font-size:0.75rem;">${i + 1}</td>
                        <td style="padding:4px 8px; border-bottom:1px solid #f3f4f6; font-family:monospace; color:#059669; font-weight:600; font-size:0.8rem;">${col.name}</td>
                        <td style="padding:4px 8px; border-bottom:1px solid #f3f4f6; text-align:right; color:#4b5563; font-size:0.8rem;">${col.start}</td>
                        <td style="padding:4px 8px; border-bottom:1px solid #f3f4f6; text-align:right; color:#4b5563; font-size:0.8rem;">${col.length}</td>
                        <td style="padding:4px 8px; border-bottom:1px solid #f3f4f6; text-align:right; color:#6b7280; font-size:0.8rem;">${(col.start || 1) + (col.length || 1) - 1}</td>
                    </tr>
                `).join('') + `
                    <tr style="background:#f3f4f6;">
                        <td colspan="2" style="padding:4px 8px; font-size:0.75rem; color:#4b5563; font-weight:600;">${tpl.columns.length} columns</td>
                        <td colspan="3" style="padding:4px 8px; text-align:right; font-size:0.75rem; color:#4b5563; font-weight:600;">Record width: ${maxEnd} chars</td>
                    </tr>
                `;
            }
            preview.style.display = 'block';
        };

        const populateColumnsFromTemplate = (tpl) => {
            const tbody = document.getElementById('fpColumnsBody');
            if (!tbody || !tpl.columns || !tpl.columns.length) return;
            tbody.innerHTML = tpl.columns.map((col, idx) => `
                <tr data-col-idx="${idx}">
                    <td><input type="text" class="form-control form-control-sm fp-col-name" value="${col.name || ''}" placeholder="Column name"></td>
                    <td><input type="number" class="form-control form-control-sm fp-col-start" value="${col.start || 1}" min="1" style="width:70px"></td>
                    <td><input type="number" class="form-control form-control-sm fp-col-length" value="${col.length || 1}" min="1" style="width:70px"></td>
                    <td><button type="button" class="btn btn-sm btn-outline-danger fp-remove-col-btn" title="Remove column">&times;</button></td>
                </tr>
            `).join('');
            tbody.querySelectorAll('.fp-remove-col-btn').forEach(btn => {
                btn.addEventListener('click', () => btn.closest('tr').remove());
            });
        };

        const fpTemplateSelect = document.getElementById('fpTemplate');
        if (fpTemplateSelect) {
            fpTemplateSelect.addEventListener('change', () => {
                const key = fpTemplateSelect.value;
                if (key && this._fpTemplateCache && this._fpTemplateCache[key]) {
                    showTemplatePreview(this._fpTemplateCache[key]);
                    populateColumnsFromTemplate(this._fpTemplateCache[key]);
                } else {
                    const preview = document.getElementById('fpTemplatePreview');
                    if (preview) preview.style.display = 'none';
                }
            });

            (async () => {
                try {
                    const resp = await fetch('/api/file-parser/templates');
                    const data = await resp.json();
                    if (!data.success) return;
                    this._fpTemplateCache = {};
                    (data.templates || []).forEach(t => { this._fpTemplateCache[t.key] = t; });
                    fpTemplateSelect.innerHTML = '<option value="">— Manual column definitions —</option>';
                    const byCategory = data.by_category || {};
                    Object.keys(byCategory).sort().forEach(cat => {
                        const grp = document.createElement('optgroup');
                        grp.label = cat;
                        [...byCategory[cat]].sort((a, b) => a.name.localeCompare(b.name)).forEach(t => {
                            const opt = document.createElement('option');
                            opt.value = t.key;
                            opt.textContent = t.name;
                            if (t.key === currentTemplate) opt.selected = true;
                            grp.appendChild(opt);
                        });
                        fpTemplateSelect.appendChild(grp);
                    });
                    if (currentTemplate && this._fpTemplateCache[currentTemplate]) {
                        showTemplatePreview(this._fpTemplateCache[currentTemplate]);
                        const tbody = document.getElementById('fpColumnsBody');
                        if (tbody && tbody.rows.length === 0) populateColumnsFromTemplate(this._fpTemplateCache[currentTemplate]);
                    }
                } catch (e) {
                    console.warn('[FileParserBuilder] Failed to load templates:', e);
                }
            })();
        }

        // File browser modal
        const fpBrowseBtn = document.getElementById('fpBrowseBtn');
        if (fpBrowseBtn) {
            if (!document.getElementById('fpFileBrowserModal')) {
                document.body.insertAdjacentHTML('beforeend', `
                    <div id="fpFileBrowserModal" style="display:none; position:fixed; top:0; left:0; width:100%; height:100%; z-index:10050; background:rgba(0,0,0,0.5); align-items:center; justify-content:center;">
                        <div style="background:#fff; width:680px; max-width:96vw; max-height:78vh; border-radius:8px; display:flex; flex-direction:column; box-shadow:0 4px 24px rgba(0,0,0,0.35);">
                            <div style="padding:11px 16px; border-bottom:1px solid #dee2e6; display:flex; align-items:center; justify-content:space-between; flex-shrink:0;">
                                <div style="display:flex; align-items:center; gap:8px;">
                                    <i class="fas fa-folder-open" style="color:#ffc107;"></i>
                                    <strong style="font-size:0.93rem;">Browse Server Files</strong>
                                    <span id="fpBrowserOsBadge" style="font-size:0.72rem; background:#6c757d; color:#fff; padding:1px 7px; border-radius:10px;"></span>
                                </div>
                                <button id="fpBrowserClose" type="button" style="background:none; border:none; font-size:1.4rem; line-height:1; cursor:pointer; color:#6c757d;">&times;</button>
                            </div>
                            <div style="padding:5px 14px; border-bottom:1px solid #dee2e6; font-size:0.78rem; color:#6c757d; word-break:break-all; flex-shrink:0; background:#f8f9fa; font-family:monospace;" id="fpBrowserPath"></div>
                            <div style="display:flex; flex:1; overflow:hidden;">
                                <div id="fpBrowserSidebar" style="width:140px; flex-shrink:0; border-right:1px solid #dee2e6; overflow-y:auto; background:#f8f9fa; padding:8px 0;">
                                    <div style="padding:4px 12px; font-size:0.7rem; text-transform:uppercase; letter-spacing:0.05em; color:#adb5bd; font-weight:600;">Quick Access</div>
                                </div>
                                <div id="fpBrowserList" style="flex:1; overflow-y:auto; font-size:0.875rem;"></div>
                            </div>
                            <div style="padding:9px 14px; border-top:1px solid #dee2e6; display:flex; align-items:center; justify-content:space-between; flex-shrink:0;">
                                <span id="fpBrowserSelLabel" style="font-size:0.8rem; color:#6c757d; font-family:monospace; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:65%;"></span>
                                <div style="display:flex; gap:8px; flex-shrink:0;">
                                    <button id="fpBrowserCancel" type="button" class="btn btn-sm btn-secondary">Cancel</button>
                                    <button id="fpBrowserSelect" type="button" class="btn btn-sm btn-primary" disabled>Select</button>
                                </div>
                            </div>
                        </div>
                    </div>
                `);
            }

            const modal     = document.getElementById('fpFileBrowserModal');
            const pathEl    = document.getElementById('fpBrowserPath');
            const listEl    = document.getElementById('fpBrowserList');
            const sidebarEl = document.getElementById('fpBrowserSidebar');
            const selectBtn = document.getElementById('fpBrowserSelect');
            const selLabel  = document.getElementById('fpBrowserSelLabel');
            const osBadge   = document.getElementById('fpBrowserOsBadge');
            let _selected = null, _serverOS = null, _shortcuts = [];

            const fmtBytes = b => b < 1024 ? b + ' B' : b < 1048576 ? (b/1024).toFixed(1)+' KB' : (b/1048576).toFixed(1)+' MB';

            const setSelected = path => {
                listEl.querySelectorAll('.fp-br-sel').forEach(e => e.style.outline = '');
                _selected = path; selectBtn.disabled = false; selLabel.textContent = path;
            };

            const renderSidebar = shortcuts => {
                const rows = shortcuts.map(s =>
                    `<div class="fp-br-shortcut" data-path="${s.path}" style="padding:6px 12px; cursor:pointer; font-size:0.8rem; display:flex; align-items:center; gap:6px; border-radius:4px; margin:1px 4px;">
                        <span>${s.icon || '📁'}</span><span style="overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${s.name}</span>
                    </div>`
                ).join('');
                const header = sidebarEl.querySelector('div');
                sidebarEl.innerHTML = '';
                if (header) sidebarEl.appendChild(header);
                sidebarEl.insertAdjacentHTML('beforeend', rows);
                sidebarEl.querySelectorAll('.fp-br-shortcut').forEach(el => {
                    el.addEventListener('mouseenter', () => el.style.background = '#e9ecef');
                    el.addEventListener('mouseleave', () => el.style.background = '');
                    el.addEventListener('click', () => loadDir(el.dataset.path));
                });
            };

            const loadDir = async path => {
                listEl.innerHTML = '<div style="padding:14px 16px; color:#6c757d;">Loading…</div>';
                selectBtn.disabled = true; selLabel.textContent = ''; _selected = null;
                try {
                    const resp = await fetch('/api/file-parser/browse?path=' + encodeURIComponent(path));
                    const rawText = await resp.text();
                    let data;
                    try { data = JSON.parse(rawText); } catch (_e) {
                        listEl.innerHTML = `<div style="padding:14px 16px; color:#dc3545;"><strong>Cannot reach browse endpoint</strong><br><span style="font-size:0.82rem;">HTTP ${resp.status} — Go backend may need to be restarted.</span></div>`;
                        return;
                    }
                    if (!data.success) { listEl.innerHTML = `<div style="padding:14px 16px; color:#dc3545;">${data.error}</div>`; return; }
                    if (data.serverOS && data.serverOS !== _serverOS) {
                        _serverOS = data.serverOS;
                        const osLabels = { linux: '🐧 Linux', windows: '🪟 Windows', darwin: '🍎 macOS' };
                        osBadge.textContent = osLabels[_serverOS] || _serverOS;
                    }
                    if (data.shortcuts && data.shortcuts.length) { _shortcuts = data.shortcuts; renderSidebar(_shortcuts); }
                    pathEl.textContent = data.path;
                    const rows = [];
                    if (data.parent !== undefined && data.parent !== null && data.parent !== '') {
                        const upLabel = (_serverOS === 'windows' && data.parent === '/') ? 'Drives' : '..';
                        rows.push(`<div class="fp-br-entry fp-br-nav" data-path="${data.parent}" style="padding:7px 14px; cursor:pointer; display:flex; align-items:center; gap:9px; border-bottom:1px solid #f0f0f0;"><span style="width:18px; text-align:center; font-size:0.9rem;">⬆</span><span style="color:#6c757d; font-size:0.82rem;">${upLabel}</span></div>`);
                    }
                    if (data.path && data.path !== '/') {
                        rows.push(`<div class="fp-br-entry fp-br-sel" data-path="${data.path}" style="padding:7px 14px; cursor:pointer; display:flex; align-items:center; gap:9px; background:#fffbea; border-bottom:1px solid #ffe082;"><span style="width:18px; text-align:center;">📂</span><span style="color:#856404; font-size:0.82rem; flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">Use this folder</span></div>`);
                    }
                    (data.entries || []).filter(e => e.isDir).forEach(e => {
                        rows.push(`<div class="fp-br-entry fp-br-nav" data-path="${e.path}" style="padding:7px 14px; cursor:pointer; display:flex; align-items:center; gap:9px; border-bottom:1px solid #f8f9fa;"><span style="width:18px; text-align:center;">${e.icon || '📁'}</span><span style="flex:1;">${e.name}</span><span style="font-size:0.72rem; color:#adb5bd;">folder</span></div>`);
                    });
                    (data.entries || []).filter(e => !e.isDir).forEach(e => {
                        const isLarge = e.size > 50 * 1024 * 1024;
                        rows.push(`<div class="fp-br-entry fp-br-sel" data-path="${e.path}" style="padding:7px 14px; cursor:pointer; display:flex; align-items:center; gap:9px; border-bottom:1px solid #f8f9fa;${isLarge ? ' background:#fffbea;' : ''}"><span style="width:18px; text-align:center;">📄</span><span style="flex:1;">${e.name}</span><span style="font-size:0.72rem; color:${isLarge ? '#856404' : '#adb5bd'}; white-space:nowrap;">${fmtBytes(e.size)}${isLarge ? ' ⚠' : ''}</span></div>`);
                    });
                    if (!rows.length) rows.push('<div style="padding:14px 16px; color:#adb5bd; font-style:italic;">Empty directory</div>');
                    listEl.innerHTML = rows.join('');
                    listEl.querySelectorAll('.fp-br-nav').forEach(el => {
                        el.addEventListener('mouseenter', () => el.style.background = '#f0f4f8');
                        el.addEventListener('mouseleave', () => el.style.background = '');
                        el.addEventListener('click', () => loadDir(el.dataset.path));
                    });
                    listEl.querySelectorAll('.fp-br-sel').forEach(el => {
                        el.addEventListener('mouseenter', () => { if (_selected !== el.dataset.path) el.style.background = '#e8f4fd'; });
                        el.addEventListener('mouseleave', () => { if (_selected !== el.dataset.path) el.style.background = el.style.background.includes('fffbea') ? '#fffbea' : ''; });
                        el.addEventListener('click', () => setSelected(el.dataset.path));
                    });
                } catch (err) {
                    listEl.innerHTML = `<div style="padding:14px 16px; color:#dc3545;">Error: ${err.message}</div>`;
                }
            };

            const applyFormatFromPath = path => {
                if (!path) return;
                const fmtSel = document.getElementById('fpFileFormat');
                if (!fmtSel) return;
                const ext = path.split('.').pop().toLowerCase();
                const extMap = { xlsx: 'xlsx', xls: 'xls', tsv: 'tsv', tab: 'tsv', csv: 'csv', avro: 'avro', parquet: 'parquet' };
                const detected = extMap[ext];
                if (detected) { fmtSel.value = detected; fmtSel.dispatchEvent(new Event('change')); }
            };

            document.getElementById('fpBrowserClose').onclick  = () => { modal.style.display = 'none'; };
            document.getElementById('fpBrowserCancel').onclick = () => { modal.style.display = 'none'; };
            modal.onclick = e => { if (e.target === modal) modal.style.display = 'none'; };
            document.getElementById('fpBrowserSelect').onclick = () => {
                if (_selected) { const inp = document.getElementById('fpFilePath'); if (inp) { inp.value = _selected; applyFormatFromPath(_selected); } }
                modal.style.display = 'none';
            };
            const fpFilePathEl = document.getElementById('fpFilePath');
            if (fpFilePathEl) fpFilePathEl.addEventListener('blur', () => applyFormatFromPath(fpFilePathEl.value.trim()));

            fpBrowseBtn.addEventListener('click', () => {
                _selected = null; selectBtn.disabled = true; selLabel.textContent = '';
                const currentVal = document.getElementById('fpFilePath')?.value?.trim() || '';
                modal.style.display = 'flex';
                loadDir(currentVal);
            });
        }

        // Preview
        const previewBtn = document.getElementById('fpRunPreviewBtn');
        if (previewBtn) {
            previewBtn.addEventListener('click', async () => {
                const sourceType  = document.querySelector('input[name="fpSourceType"]:checked')?.value || 'field';
                const isLocalPath = sourceType === 'local_path';
                const filePath    = document.getElementById('fpFilePath')?.value?.trim() || '';
                const content     = document.getElementById('fpPreviewContent')?.value || '';
                const statusEl    = document.getElementById('fpPreviewStatus');
                if (isLocalPath && !filePath) { if (statusEl) statusEl.textContent = 'Enter or browse to a file path first.'; return; }
                if (!isLocalPath && !content.trim()) { if (statusEl) statusEl.textContent = 'Paste some content first.'; return; }
                const autoDetectChk = document.getElementById('fpAutoDetect');
                const fmt = document.getElementById('fpFileFormat')?.value || 'csv';
                const config = {
                    fileFormat: autoDetectChk?.checked ? 'auto' : fmt, autoDetect: autoDetectChk?.checked || false,
                    hasHeader: document.getElementById('fpHasHeader')?.checked !== false,
                    delimiter: document.getElementById('fpDelimiter')?.value || ',',
                    trimFields: document.getElementById('fpTrimFields')?.checked !== false,
                    skipRows: parseInt(document.getElementById('fpSkipRows')?.value) || 0,
                    maxRecords: 5,
                    template: document.getElementById('fpTemplate')?.value || '',
                    sheetName: document.getElementById('fpSheetName')?.value || '',
                    sheetIndex: parseInt(document.getElementById('fpSheetIndex')?.value) || 0,
                    contentEncoding: document.getElementById('fpContentEncoding')?.value || '',
                };
                if (fmt === 'fixed_width') {
                    const cols = [];
                    document.querySelectorAll('#fpColumnsBody tr').forEach(row => {
                        const name = row.querySelector('.fp-col-name')?.value || '';
                        const start = parseInt(row.querySelector('.fp-col-start')?.value) || 1;
                        const length = parseInt(row.querySelector('.fp-col-length')?.value) || 1;
                        if (name) cols.push({ name, start, length });
                    });
                    config.columns = cols;
                }
                const resultEl = document.getElementById('fpPreviewResult');
                if (statusEl) statusEl.textContent = 'Parsing…';
                if (resultEl) resultEl.style.display = 'none';
                const previewBody = isLocalPath ? { filePath, config } : { content, config };
                try {
                    const resp = await fetch('/api/file-parser/preview', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(previewBody) });
                    const json = await resp.json();
                    if (!json.success) { if (statusEl) statusEl.textContent = `Error: ${json.error}`; return; }
                    const { columns, records, record_count, format, detected_format } = json.preview;
                    if (statusEl) statusEl.textContent = `${record_count} record(s) — format: ${detected_format || format || '?'}`;
                    if (resultEl && columns && records) {
                        const head = document.getElementById('fpPreviewHead');
                        const body = document.getElementById('fpPreviewBody');
                        if (head) head.innerHTML = `<tr>${columns.map(c => `<th>${c}</th>`).join('')}</tr>`;
                        if (body) body.innerHTML = records.map(r => `<tr>${columns.map(c => `<td>${r[c] ?? ''}</td>`).join('')}</tr>`).join('');
                        resultEl.style.display = 'block';
                    }
                } catch (e) {
                    if (statusEl) statusEl.textContent = `Request failed: ${e.message}`;
                }
            });
        }
    }
}

StepBuilderRegistry.register('file_parser', FileParserBuilder);
