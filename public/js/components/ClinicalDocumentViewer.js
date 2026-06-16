/**
 * ClinicalDocumentViewer — shared component for rendering clinical document content.
 *
 * Handles two formats:
 *   - CDA/C-CDA ('ccda'): renders section tabs from parsed_content.sections.*.narrativeHTML
 *   - FHIR Bundle ('fhir'): groups resources by type, calls POST /api/fhir/narrative/:resourceType
 *
 * Usage:
 *   const viewer = new ClinicalDocumentViewer(containerEl, {
 *       interfaceId: '...', messageId: '...', format: 'ccda'
 *   });
 *   await viewer.mount();
 */
class ClinicalDocumentViewer {
    constructor(container, options) {
        options = options || {};
        this._container     = container;
        this._interfaceId   = options.interfaceId   || null;
        this._messageId     = options.messageId     || null;
        this._format        = options.format        || 'ccda';
        this._parsedContent = options.parsedContent || null;
        this._activeSection = null;

        this._tpl = {
            loading:     () => '<div style="display:flex;align-items:center;justify-content:center;height:100%;min-height:120px;color:#94a3b8;font-size:0.85rem;"><i class="fas fa-spinner fa-spin" style="margin-right:0.5rem;"></i> Loading clinical document...</div>',
            unavailable: () => '<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;min-height:120px;color:#94a3b8;text-align:center;padding:1.5rem;"><i class="fas fa-file-medical" style="font-size:2rem;margin-bottom:0.75rem;"></i><div style="font-weight:600;color:#64748b;margin-bottom:0.35rem;">Clinical document not available</div><div style="font-size:0.8rem;">Parsed content may still be processing or object storage may be unavailable.</div></div>',
            empty:       (msg) => '<div style="color:#94a3b8;font-size:0.82rem;padding:1rem 0;">' + this._esc(msg) + '</div>',
        };
    }

    // ── Public API ─────────────────────────────────────────────────────────────

    async mount() {
        this._container.innerHTML = this._tpl.loading();

        if (this._format === 'ccda') {
            await this._renderCDAWithXSLT();
            return;
        }

        // FHIR path: use parsedContent passed in directly, or fetch sections
        if (!this._parsedContent && this._interfaceId && this._messageId) {
            await this._fetchParsedContent();
        }
        if (!this._parsedContent) {
            this._container.innerHTML = this._tpl.unavailable();
            return;
        }
        await this._renderFHIR();
    }

    destroy() {
        this._container.innerHTML = '';
    }

    // ── XSLT-based CDA rendering ──────────────────────────────────────────────

    async _renderCDAWithXSLT() {
        try {
            // Fetch raw XML and XSLT stylesheet in parallel
            const [rawXml, xsltText] = await Promise.all([
                this._fetchRawXml(),
                this._loadXSLT(),
            ]);

            if (!rawXml || !xsltText) {
                this._container.innerHTML = this._tpl.unavailable();
                return;
            }

            const parser  = new DOMParser();
            const xmlDoc  = parser.parseFromString(rawXml,  'application/xml');
            const xsltDoc = parser.parseFromString(xsltText, 'application/xml');

            // Abort on parse errors
            if (xmlDoc.querySelector('parsererror') || xsltDoc.querySelector('parsererror')) {
                console.warn('[ClinicalDocumentViewer] XML/XSLT parse error');
                this._container.innerHTML = this._tpl.unavailable();
                return;
            }

            const proc = new XSLTProcessor();
            proc.importStylesheet(xsltDoc);
            const resultDoc = proc.transformToDocument(xmlDoc);

            if (!resultDoc || !resultDoc.documentElement) {
                this._container.innerHTML = this._tpl.unavailable();
                return;
            }

            // Render in a sandboxed iframe for clean CSS isolation
            this._container.innerHTML = '';
            const iframe = document.createElement('iframe');
            iframe.style.cssText = 'width:100%;height:100%;border:none;display:block;';
            iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin');
            this._container.style.overflow = 'hidden';
            this._container.appendChild(iframe);

            const iframeDoc = iframe.contentDocument || iframe.contentWindow.document;
            iframeDoc.open();
            iframeDoc.write('<!DOCTYPE html>');
            // XMLSerializer handles the transformed document tree
            const html = new XMLSerializer().serializeToString(resultDoc);
            iframeDoc.write(html.replace(/^<\?xml[^?]*\?>/, ''));
            iframeDoc.close();

        } catch (err) {
            console.warn('[ClinicalDocumentViewer] XSLT render failed:', err.message);
            this._container.innerHTML = this._tpl.unavailable();
        }
    }

    async _fetchRawXml() {
        if (!this._interfaceId || !this._messageId) return null;
        try {
            const url = '/api/messages/' + encodeURIComponent(this._messageId) +
                        '/raw?interfaceId=' + encodeURIComponent(this._interfaceId);
            const resp = await fetch(url);
            if (!resp.ok) return null;
            const data = await resp.json();
            return (data && data.success && data.content) ? data.content : null;
        } catch (_) { return null; }
    }

    async _loadXSLT() {
        if (ClinicalDocumentViewer._xsltCache) return ClinicalDocumentViewer._xsltCache;
        try {
            const resp = await fetch('/xslt/cda.xsl');
            if (!resp.ok) return null;
            const text = await resp.text();
            ClinicalDocumentViewer._xsltCache = text;
            return text;
        } catch (_) { return null; }
    }

    // ── Legacy section-based CDA rendering (kept as reference / FHIR viewer base) ──

    _renderCDA() {
        const sections = this._parsedContent.sections || {};
        const header   = this._parsedContent.header   || null;

        const sectionKeys = [];
        if (header) sectionKeys.push('header');
        for (var k in sections) {
            if (sections.hasOwnProperty(k) && k !== 'header') sectionKeys.push(k);
        }

        if (sectionKeys.length === 0) {
            this._container.innerHTML = this._tpl.empty('No sections found in document');
            return;
        }

        this._activeSection = sectionKeys[0];

        this._container.innerHTML =
            '<div class="cdv-root" style="display:flex;flex-direction:column;height:100%;background:#fff;border-radius:8px;overflow:hidden;border:1px solid #e2e8f0;">' +
            this._renderFormatBadge() +
            this._renderCDATabs(sectionKeys, sections, header) +
            '<div class="cdv-content" style="flex:1;overflow:auto;padding:1rem;">' +
            this._cdaSectionContent(this._activeSection, sections, header) +
            '</div></div>';

        this._attachTabListeners(sectionKeys, sections, header);
    }

    _renderFormatBadge() {
        var profile = String(this._parsedContent.profile || 'C-CDA').replace(/[<>]/g, '');
        return '<div style="padding:0.5rem 1rem;background:#f8fafc;border-bottom:1px solid #e2e8f0;display:flex;align-items:center;gap:0.5rem;flex-shrink:0;">' +
            '<span style="font-size:0.75rem;font-weight:700;color:#475569;text-transform:uppercase;">Clinical Document</span>' +
            '<span style="background:#dbeafe;color:#1e40af;padding:0.15rem 0.5rem;border-radius:4px;font-size:0.72rem;font-weight:600;">' + this._esc(profile) + '</span>' +
            '</div>';
    }

    _renderCDATabs(keys, sections, header) {
        var self = this;
        var tabs = keys.map(function(key) {
            var label = ClinicalDocumentViewer.SECTION_LABELS[key] || self._titleCase(key);
            var count = (key === 'header') ? '' : self._sectionCount(key, sections);
            var countBadge = (count !== '')
                ? '<span style="background:#e2e8f0;color:#475569;padding:0.05rem 0.4rem;border-radius:10px;font-size:0.68rem;margin-left:0.3rem;">' + count + '</span>'
                : '';
            var active = (key === self._activeSection);
            return '<button data-cdv-tab="' + self._esc(key) + '" style="' +
                'flex-shrink:0;padding:0.5rem 0.9rem;border:none;' +
                'background:' + (active ? '#fff' : 'transparent') + ';' +
                'border-bottom:2px solid ' + (active ? '#2563eb' : 'transparent') + ';' +
                'color:' + (active ? '#1e40af' : '#64748b') + ';' +
                'font-size:0.8rem;font-weight:' + (active ? '600' : '400') + ';cursor:pointer;white-space:nowrap;">' +
                self._esc(label) + countBadge + '</button>';
        }).join('');
        return '<div style="display:flex;overflow-x:auto;border-bottom:1px solid #e2e8f0;flex-shrink:0;background:#f8fafc;">' + tabs + '</div>';
    }

    _cdaSectionContent(key, sections, header) {
        if (key === 'header') return this._renderHeaderSection(header);
        var sec = sections[key];
        if (!sec) return this._tpl.empty('Section not found');
        if (sec.narrativeHTML) return '<div class="cdv-narrative">' + sec.narrativeHTML + '</div>';
        if (sec.entries && sec.entries.length > 0) return this._renderEntriesTable(sec.entries);
        return this._tpl.empty('No content in this section');
    }

    _renderHeaderSection(header) {
        if (!header) return this._tpl.empty('No header data');
        var p = header.patient || {};
        var addr = p.address || {};
        var rows = [
            ['Patient Name',  [p.firstName, p.middleName, p.lastName].filter(Boolean).join(' ')],
            ['Date of Birth', p.dateOfBirth || ''],
            ['Sex',           p.sexDisplay  || p.sex || ''],
            ['Race',          p.raceDisplay || p.race || ''],
            ['Ethnicity',     p.ethnicityDisplay || p.ethnicity || ''],
            ['Language',      p.preferredLanguage || ''],
            ['Address',       [addr.street, addr.city, addr.state, addr.postalCode].filter(Boolean).join(', ')],
            ['Phone',         p.phone || ''],
            ['Email',         p.email || ''],
            ['Document Author', (header.author  && header.author.name)    || ''],
            ['Custodian',       (header.custodian && header.custodian.name) || ''],
            ['Document Date', header.documentDate || ''],
        ].filter(function(r) { return r[1]; });

        var self = this;
        var trs = rows.map(function(row) {
            return '<tr><th style="width:38%;color:#64748b;font-weight:500;padding:0.35rem 0.5rem;font-size:0.83rem;">' + self._esc(row[0]) + '</th>' +
                   '<td style="padding:0.35rem 0.5rem;font-size:0.83rem;color:#1e293b;">' + self._esc(row[1]) + '</td></tr>';
        }).join('');
        return '<table style="width:100%;border-collapse:collapse;">' + trs + '</table>';
    }

    _renderEntriesTable(entries) {
        if (!entries || entries.length === 0) return '';
        var keys = Object.keys(entries[0]).filter(function(k) { return k.charAt(0) !== '_'; });
        var self = this;
        var headers = keys.map(function(k) {
            return '<th style="padding:0.35rem 0.5rem;font-size:0.75rem;font-weight:600;color:#475569;border-bottom:2px solid #e2e8f0;background:#f8fafc;">' + self._esc(self._titleCase(k)) + '</th>';
        }).join('');
        var rowsHtml = entries.map(function(entry) {
            return '<tr>' + keys.map(function(k) {
                return '<td style="padding:0.35rem 0.5rem;font-size:0.8rem;border-bottom:1px solid #f1f5f9;">' + self._esc(String(entry[k] != null ? entry[k] : '')) + '</td>';
            }).join('') + '</tr>';
        }).join('');
        return '<div style="overflow-x:auto;"><table style="width:100%;border-collapse:collapse;"><thead><tr>' + headers + '</tr></thead><tbody>' + rowsHtml + '</tbody></table></div>';
    }

    _attachTabListeners(keys, sections, header) {
        var self = this;
        this._container.querySelectorAll('[data-cdv-tab]').forEach(function(btn) {
            btn.addEventListener('click', function() {
                var key = btn.dataset.cdvTab;
                self._activeSection = key;
                self._container.querySelectorAll('[data-cdv-tab]').forEach(function(b) {
                    var a = b.dataset.cdvTab === key;
                    b.style.background   = a ? '#fff' : 'transparent';
                    b.style.borderBottom = '2px solid ' + (a ? '#2563eb' : 'transparent');
                    b.style.color        = a ? '#1e40af' : '#64748b';
                    b.style.fontWeight   = a ? '600' : '400';
                });
                var contentEl = self._container.querySelector('.cdv-content');
                if (contentEl) contentEl.innerHTML = self._cdaSectionContent(key, sections, header);
            });
        });
    }

    // ── FHIR rendering ─────────────────────────────────────────────────────────

    async _renderFHIR() {
        var content = this._parsedContent;
        var bundle = (content.resourceType === 'Bundle') ? content
                   : (content.bundle || content.fhirBundle || null);
        var entries = (bundle && bundle.entry) ? bundle.entry : (content.entry || []);
        var resources = entries.map(function(e) { return e.resource || e; })
                               .filter(function(r) { return r && r.resourceType; });

        if (resources.length === 0) {
            this._container.innerHTML = this._tpl.empty('No FHIR resources found');
            return;
        }

        var groups = {};
        resources.forEach(function(r) {
            var rt = r.resourceType;
            if (!groups[rt]) groups[rt] = [];
            groups[rt].push(r);
        });

        var narrativeMap = await this._fetchFHIRNarratives(groups);
        var resourceTypes = Object.keys(groups);
        this._activeSection = resourceTypes[0];

        this._container.innerHTML =
            '<div class="cdv-root" style="display:flex;flex-direction:column;height:100%;background:#fff;border-radius:8px;overflow:hidden;border:1px solid #e2e8f0;">' +
            this._renderFHIRFormatBadge(resources.length) +
            this._renderFHIRTabs(resourceTypes, groups) +
            '<div class="cdv-content" style="flex:1;overflow:auto;padding:1rem;">' +
            this._fhirSectionContent(this._activeSection, groups, narrativeMap) +
            '</div></div>';

        this._attachFHIRTabListeners(resourceTypes, groups, narrativeMap);
    }

    async _fetchFHIRNarratives(groups) {
        var narrativeMap = {};
        var fetchPromises = [];

        for (var rt in groups) {
            if (!groups.hasOwnProperty(rt)) continue;
            var resources = groups[rt];
            for (var i = 0; i < resources.length; i++) {
                var r = resources[i];
                if (r.text && r.text.div) continue;
                var key = rt + ':' + i;
                fetchPromises.push(
                    (function(k, resource, rtype) {
                        return fetch('/api/fhir/narrative/' + rtype, {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify(resource)
                        })
                        .then(function(resp) { return resp.ok ? resp.json() : null; })
                        .then(function(data) {
                            if (data && data.success && data.narrative) narrativeMap[k] = data.narrative;
                        })
                        .catch(function() {});
                    })(key, r, rt)
                );
            }
        }

        await Promise.allSettled(fetchPromises);
        return narrativeMap;
    }

    _renderFHIRFormatBadge(count) {
        return '<div style="padding:0.5rem 1rem;background:#f8fafc;border-bottom:1px solid #e2e8f0;display:flex;align-items:center;gap:0.5rem;flex-shrink:0;">' +
            '<span style="font-size:0.75rem;font-weight:700;color:#475569;text-transform:uppercase;">FHIR Resources</span>' +
            '<span style="background:#dcfce7;color:#166534;padding:0.15rem 0.5rem;border-radius:4px;font-size:0.72rem;font-weight:600;">R4 · ' + count + ' resources</span>' +
            '</div>';
    }

    _renderFHIRTabs(resourceTypes, groups) {
        var self = this;
        var tabs = resourceTypes.map(function(rt) {
            var label = ClinicalDocumentViewer.FHIR_RESOURCE_LABELS[rt] || rt;
            var count = groups[rt].length;
            var active = (rt === self._activeSection);
            return '<button data-cdv-tab="' + self._esc(rt) + '" style="' +
                'flex-shrink:0;padding:0.5rem 0.9rem;border:none;' +
                'background:' + (active ? '#fff' : 'transparent') + ';' +
                'border-bottom:2px solid ' + (active ? '#2563eb' : 'transparent') + ';' +
                'color:' + (active ? '#1e40af' : '#64748b') + ';' +
                'font-size:0.8rem;font-weight:' + (active ? '600' : '400') + ';cursor:pointer;white-space:nowrap;">' +
                self._esc(label) +
                '<span style="background:#e2e8f0;color:#475569;padding:0.05rem 0.4rem;border-radius:10px;font-size:0.68rem;margin-left:0.3rem;">' + count + '</span>' +
                '</button>';
        }).join('');
        return '<div style="display:flex;overflow-x:auto;border-bottom:1px solid #e2e8f0;flex-shrink:0;background:#f8fafc;">' + tabs + '</div>';
    }

    _fhirSectionContent(rt, groups, narrativeMap) {
        var self = this;
        var resources = groups[rt] || [];
        return resources.map(function(r, i) {
            var key = rt + ':' + i;
            var narrativeDiv = (r.text && r.text.div) ? r.text.div : (narrativeMap[key] || '');
            var instanceHeader = (resources.length > 1)
                ? '<div style="font-size:0.72rem;font-weight:600;color:#94a3b8;margin-bottom:0.5rem;">' + rt + ' ' + (i + 1) + ' of ' + resources.length + (r.id ? ' · ' + self._esc(r.id) : '') + '</div>'
                : '';
            var narrativeContent = narrativeDiv
                ? '<div class="cdv-narrative">' + narrativeDiv + '</div>'
                : '<div style="color:#94a3b8;font-size:0.82rem;padding:1rem 0;">No narrative available</div>';
            return '<div style="' + (i > 0 ? 'border-top:1px solid #e2e8f0;padding-top:1rem;margin-top:1rem;' : '') + '">' +
                instanceHeader + narrativeContent + '</div>';
        }).join('');
    }

    _attachFHIRTabListeners(resourceTypes, groups, narrativeMap) {
        var self = this;
        this._container.querySelectorAll('[data-cdv-tab]').forEach(function(btn) {
            btn.addEventListener('click', function() {
                var rt = btn.dataset.cdvTab;
                self._activeSection = rt;
                self._container.querySelectorAll('[data-cdv-tab]').forEach(function(b) {
                    var a = b.dataset.cdvTab === rt;
                    b.style.background   = a ? '#fff' : 'transparent';
                    b.style.borderBottom = '2px solid ' + (a ? '#2563eb' : 'transparent');
                    b.style.color        = a ? '#1e40af' : '#64748b';
                    b.style.fontWeight   = a ? '600' : '400';
                });
                var contentEl = self._container.querySelector('.cdv-content');
                if (contentEl) contentEl.innerHTML = self._fhirSectionContent(rt, groups, narrativeMap);
            });
        });
    }

    // ── Helpers ────────────────────────────────────────────────────────────────

    _sectionCount(key, sections) {
        var sec = sections[key];
        if (!sec) return '';
        if (typeof sec.entryCount === 'number') return sec.entryCount;
        if (Array.isArray(sec.entries)) return sec.entries.length;
        return '';
    }

    _titleCase(str) {
        return str.replace(/([A-Z])/g, ' $1').replace(/^./, function(c) { return c.toUpperCase(); }).trim();
    }

    _esc(s) {
        return String(s != null ? s : '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
    }
}

// Post-class static assignments (avoid static class field syntax per project standards)
ClinicalDocumentViewer.SECTION_LABELS = {
    allergiesAndIntolerances: 'Allergies',
    medications:              'Medications',
    problems:                 'Problems',
    vitalSigns:               'Vital Signs',
    results:                  'Lab Results',
    immunizations:            'Immunizations',
    procedures:               'Procedures',
    socialHistory:            'Social History',
    header:                   'Patient Info',
};

ClinicalDocumentViewer._xsltCache = null;

ClinicalDocumentViewer.FHIR_RESOURCE_LABELS = {
    Patient:             'Patient Demographics',
    AllergyIntolerance:  'Allergy and Intolerance',
    Condition:           'Problem',
    MedicationStatement: 'Medication',
    Observation:         'Clinical Observation',
    Immunization:        'Immunization',
    Procedure:           'Procedure',
    Bundle:              'Document',
};
