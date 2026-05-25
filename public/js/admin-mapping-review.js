// public/js/admin-mapping-review.js
//
// Admin mapping review queue — loads flagged transformation records, shows
// HL7 and FHIR content side-by-side, and lets the integration team mark
// each record as reviewed with a resolution note.

(function () {
    'use strict';

    var API_BASE = '/api/fhir/quality';
    var MSG_API  = '/api/messages';          // existing Node endpoint for raw/transformed content

    var state = {
        records: [],
        offset: 0,
        limit: 30,
        total: 0,
        msgTypeFilter: '',
        activeId: null,
        activeRecord: null,
        knownMsgTypes: []
    };

    // ── Bootstrap ───────────────────────────────────────────────────────────

    document.addEventListener('DOMContentLoaded', function () {
        document.getElementById('msgTypeFilter').addEventListener('change', function () {
            state.msgTypeFilter = this.value;
            state.offset = 0;
            state.records = [];
            loadRecords(true);
        });
        document.getElementById('loadMoreBtn').addEventListener('click', function () {
            loadRecords(false);
        });
        loadStats();
        loadRecords(true);
    });

    // ── API helpers ──────────────────────────────────────────────────────────

    function apiFetch(path, opts) {
        return fetch(path, Object.assign({ credentials: 'include' }, opts || {}))
            .then(function (r) { return r.json(); });
    }

    // ── Stats (populate message type filter + unreviewed badge) ─────────────

    function loadStats() {
        apiFetch(API_BASE + '/stats').then(function (res) {
            if (!res.success) return;
            var sel = document.getElementById('msgTypeFilter');
            (res.stats || []).forEach(function (s) {
                if (state.knownMsgTypes.indexOf(s.message_type) === -1) {
                    state.knownMsgTypes.push(s.message_type);
                    var opt = document.createElement('option');
                    opt.value = s.message_type;
                    opt.textContent = s.message_type + ' (' + s.pending_review + ' pending)';
                    sel.appendChild(opt);
                }
            });
        }).catch(function () {});
    }

    // ── Record list ──────────────────────────────────────────────────────────

    function loadRecords(reset) {
        if (reset) {
            state.offset = 0;
            state.records = [];
            document.getElementById('recordList').innerHTML = '<div class="pane-loading">Loading…</div>';
        }

        var url = API_BASE + '/flagged?limit=' + state.limit + '&offset=' + state.offset;
        if (state.msgTypeFilter) url += '&message_type=' + encodeURIComponent(state.msgTypeFilter);

        apiFetch(url).then(function (res) {
            if (!res.success) { renderEmptyList('Failed to load records'); return; }

            // Update unreviewed badge
            var badge = document.getElementById('unreviewedBadge');
            if (res.total > 0) {
                badge.textContent = res.total;
                badge.style.display = '';
            } else {
                badge.style.display = 'none';
            }

            state.total = res.total;
            state.records = state.records.concat(res.records || []);
            state.offset += (res.records || []).length;

            renderList();

            var loadMoreBtn = document.getElementById('loadMoreBtn');
            loadMoreBtn.style.display = state.records.length < state.total ? '' : 'none';
        }).catch(function () { renderEmptyList('Network error'); });
    }

    function renderList() {
        var container = document.getElementById('recordList');
        if (state.records.length === 0) {
            container.innerHTML = '<div class="empty-list"><i class="fas fa-check-circle"></i>No unreviewed flagged records</div>';
            return;
        }
        var html = '';
        state.records.forEach(function (r) {
            var scoreClass = r.score >= 80 ? 'good' : (r.score >= 60 ? 'warn' : 'bad');
            var ts = r.created_at ? new Date(r.created_at).toLocaleString() : '';
            html += '<div class="record-item' + (r.id === state.activeId ? ' active' : '') + '" ' +
                    'data-id="' + esc(r.id) + '" onclick="window._mrSelectRecord(\'' + esc(r.id) + '\')">' +
                    '<div class="msg-type">' + esc(r.message_type || 'Unknown') + '</div>' +
                    '<div class="msg-id">' + esc(r.message_id || r.id) + '</div>' +
                    '<div class="record-meta">' +
                    '<span class="score-badge ' + scoreClass + '">' + (r.score || 0).toFixed(1) + '</span>' +
                    '<span class="flag-reason">' + esc(r.flag_reason || '') + '</span>' +
                    '<span class="record-ts">' + esc(ts) + '</span>' +
                    '</div>' +
                    '</div>';
        });
        container.innerHTML = html;
    }

    function renderEmptyList(msg) {
        document.getElementById('recordList').innerHTML =
            '<div class="empty-list"><i class="fas fa-exclamation-triangle"></i>' + esc(msg) + '</div>';
    }

    // ── Record detail ────────────────────────────────────────────────────────

    window._mrSelectRecord = function (id) {
        state.activeId = id;

        // Update active class in list
        document.querySelectorAll('.record-item').forEach(function (el) {
            el.classList.toggle('active', el.dataset.id === id);
        });

        // Show skeleton
        document.getElementById('emptyState').style.display = 'none';
        document.getElementById('actionBar').style.display = '';
        document.getElementById('detailChips').style.display = '';
        document.getElementById('contentSplit').style.display = '';
        document.getElementById('hl7Content').innerHTML = '<div class="pane-loading">Loading HL7…</div>';
        document.getElementById('fhirContent').innerHTML = '<div class="pane-loading">Loading FHIR…</div>';
        document.getElementById('detailChips').innerHTML = '';

        apiFetch(API_BASE + '/flagged/' + encodeURIComponent(id)).then(function (res) {
            if (!res.success || !res.record) return;
            var r = res.record;
            state.activeRecord = r;

            document.getElementById('detailMsgType').textContent = r.message_type || 'Unknown type';
            document.getElementById('detailMsgId').textContent = r.message_id || r.id;

            // OOB template link (open pipeline builder for the interface)
            if (r.interface_id) {
                document.getElementById('btnOOB').href =
                    'pipeline-builder.html?interfaceId=' + encodeURIComponent(r.interface_id) +
                    (r.message_type ? '&messageType=' + encodeURIComponent(r.message_type) : '');
            }

            // Chips
            var chips = '';
            var scoreClass = r.score >= 80 ? 'green' : (r.score >= 60 ? 'amber' : 'red');
            chips += '<span class="chip ' + scoreClass + '"><i class="fas fa-star"></i> Score: ' + (r.score || 0).toFixed(1) + '</span>';
            if (r.flag_reason) chips += '<span class="chip red"><i class="fas fa-flag"></i> ' + esc(r.flag_reason) + '</span>';
            if (r.missing_resources && r.missing_resources.length) {
                chips += '<span class="chip amber"><i class="fas fa-exclamation"></i> Missing: ' +
                         esc(r.missing_resources.join(', ')) + '</span>';
            }
            if (r.warnings && r.warnings.length) {
                chips += '<span class="chip amber"><i class="fas fa-exclamation-triangle"></i> ' +
                         r.warnings.length + ' warning' + (r.warnings.length !== 1 ? 's' : '') + '</span>';
            }
            if (r.errors && r.errors.length) {
                chips += '<span class="chip red"><i class="fas fa-times-circle"></i> ' +
                         r.errors.length + ' error' + (r.errors.length !== 1 ? 's' : '') + '</span>';
            }
            document.getElementById('detailChips').innerHTML = chips;

            loadMessageContent(r);
        }).catch(function () {
            document.getElementById('hl7Content').textContent = 'Failed to load record.';
        });
    };

    function loadMessageContent(r) {
        // Fetch raw HL7 and transformed FHIR via existing MessageContentController
        var messageId = r.message_id;
        var interfaceId = r.interface_id;

        if (!messageId || !interfaceId) {
            document.getElementById('hl7Content').textContent = 'No message ID available.';
            document.getElementById('fhirContent').textContent = 'No message ID available.';
            return;
        }

        // Raw HL7
        fetch('/api/messages/' + encodeURIComponent(messageId) + '/raw?interfaceId=' + encodeURIComponent(interfaceId), {
            credentials: 'include'
        }).then(function (resp) {
            return resp.ok ? resp.text() : Promise.reject('HTTP ' + resp.status);
        }).then(function (text) {
            // Format HL7 segments one per line
            var segments = text.replace(/\r\n/g, '\r').replace(/\n/g, '\r').split('\r').filter(Boolean);
            document.getElementById('hl7Content').textContent = segments.join('\n');
            document.getElementById('hl7Meta').textContent = segments.length + ' segments';
        }).catch(function (err) {
            document.getElementById('hl7Content').textContent = 'Could not load HL7 content: ' + err;
        });

        // Transformed FHIR
        fetch('/api/messages/' + encodeURIComponent(messageId) + '/transformed?interfaceId=' + encodeURIComponent(interfaceId), {
            credentials: 'include'
        }).then(function (resp) {
            return resp.ok ? resp.json() : Promise.reject('HTTP ' + resp.status);
        }).then(function (json) {
            var text = JSON.stringify(json, null, 2);
            document.getElementById('fhirContent').textContent = text;
            var resourceType = json.resourceType || 'Bundle';
            var entryCount = Array.isArray(json.entry) ? json.entry.length : 0;
            document.getElementById('fhirMeta').textContent =
                resourceType + (entryCount ? ' · ' + entryCount + ' entries' : '');
        }).catch(function (err) {
            document.getElementById('fhirContent').textContent = 'Could not load FHIR content: ' + err;
        });
    }

    // ── Review modal ─────────────────────────────────────────────────────────

    window.openReviewModal = function () {
        document.getElementById('resolutionText').value = '';
        document.getElementById('reviewModal').classList.add('open');
        setTimeout(function () { document.getElementById('resolutionText').focus(); }, 50);
    };

    window.closeReviewModal = function () {
        document.getElementById('reviewModal').classList.remove('open');
    };

    window.submitReview = function () {
        var resolution = document.getElementById('resolutionText').value.trim();
        if (!resolution) {
            document.getElementById('resolutionText').focus();
            return;
        }
        if (!state.activeId) return;

        var btn = document.querySelector('#reviewModal .btn-submit');
        btn.disabled = true;
        btn.textContent = 'Saving…';

        apiFetch(API_BASE + '/flagged/' + encodeURIComponent(state.activeId) + '/review', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ resolution: resolution })
        }).then(function (res) {
            if (!res.success) throw new Error(res.error || 'Save failed');

            closeReviewModal();

            // Remove from local list
            state.records = state.records.filter(function (r) { return r.id !== state.activeId; });
            state.total = Math.max(0, state.total - 1);
            state.activeId = null;
            state.activeRecord = null;

            // Reset detail panel
            document.getElementById('emptyState').style.display = '';
            document.getElementById('actionBar').style.display = 'none';
            document.getElementById('detailChips').style.display = 'none';
            document.getElementById('contentSplit').style.display = 'none';

            // Update badge
            var badge = document.getElementById('unreviewedBadge');
            if (state.total > 0) {
                badge.textContent = state.total;
            } else {
                badge.style.display = 'none';
            }

            renderList();
        }).catch(function (err) {
            alert('Failed to save review: ' + err.message);
        }).finally(function () {
            btn.disabled = false;
            btn.innerHTML = '<i class="fas fa-check"></i> Confirm';
        });
    };

    // Close modal on overlay click
    document.addEventListener('DOMContentLoaded', function () {
        document.getElementById('reviewModal').addEventListener('click', function (e) {
            if (e.target === this) closeReviewModal();
        });
    });

    // ── Helpers ───────────────────────────────────────────────────────────────

    function esc(s) {
        return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
                              .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

}());
