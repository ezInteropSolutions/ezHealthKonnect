'use strict';

// ── State ─────────────────────────────────────────────────────────────────────
var _alerts       = [];
var _rules        = [];
var _channels     = [];
var _silences     = [];
var _history      = [];
var _interfaces   = [];
var _severityFilter = '';
var _assignRuleId   = '';
var _editChannelId  = '';
var _pollTimer      = null;
var _tabLoaded      = {};

// ── Tab routing ────────────────────────────────────────────────────────────────
function activateTab(name) {
    document.querySelectorAll('.leftnav-tab').forEach(function(el) {
        el.classList.toggle('active', el.dataset.tab === name);
    });
    document.querySelectorAll('.tab-section').forEach(function(el) {
        el.classList.toggle('active', el.id === 'tab-' + name);
    });
    location.hash = name;

    if (!_tabLoaded[name]) {
        _tabLoaded[name] = true;
        if      (name === 'active-alerts') loadActiveAlerts();
        else if (name === 'rules')         loadRules();
        else if (name === 'channels')      loadChannels();
        else if (name === 'silences')      loadSilences();
        else if (name === 'history')       loadHistory();
    }
}

// ── API helpers ────────────────────────────────────────────────────────────────
function api(method, path, body) {
    var opts = { method: method, credentials: 'include', headers: {} };
    if (body) {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
    }
    return fetch('/api/alerts' + path, opts).then(function(r) { return r.json(); });
}

function showToast(msg, ok) {
    var t = document.createElement('div');
    t.style.cssText = 'position:fixed;bottom:20px;right:20px;z-index:9999;padding:10px 18px;border-radius:8px;font-size:13px;font-weight:600;color:#fff;background:' + (ok ? '#16a34a' : '#dc2626');
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(function() { t.remove(); }, 3500);
}

// ── Active Alerts ──────────────────────────────────────────────────────────────
function loadActiveAlerts() {
    return api('GET', '/fired').then(function(res) {
        _alerts = res.data || [];
        renderAlerts();
        updateSeverityChips();
        updateSidebarBadge(_alerts.length);
        document.getElementById('lastUpdated').textContent = 'Updated ' + new Date().toLocaleTimeString();
    }).catch(function() {});
}

function filterBySeverity(sev) {
    _severityFilter = sev;
    document.querySelectorAll('.sev-chip').forEach(function(el) {
        el.classList.toggle('active', el.getAttribute('onclick').indexOf("'" + sev + "'") !== -1);
    });
    renderAlerts();
}

function renderAlerts() {
    var ifaceFilter = (document.getElementById('filterInterface') || {}).value || '';
    var typeFilter  = (document.getElementById('filterType') || {}).value || '';

    var filtered = _alerts.filter(function(a) {
        if (_severityFilter && a.severity !== _severityFilter) return false;
        if (ifaceFilter && a.interface_id !== ifaceFilter) return false;
        if (typeFilter && a.alert_type !== typeFilter) return false;
        return true;
    });

    var el = document.getElementById('alertList');
    if (!el) return;

    if (filtered.length === 0) {
        el.innerHTML = '<div class="empty-state"><i class="fas fa-pencilcil" style="color:#22c55e"></i><h3>No active alerts</h3><p>All systems are operating within configured thresholds.</p></div>';
        return;
    }

    el.innerHTML = filtered.map(function(a) {
        var elapsed = formatElapsed(a.elapsed_seconds);
        return '<div class="alert-card ' + a.severity + '">' +
            '<div class="alert-sev-dot ' + a.severity + '"></div>' +
            '<div class="alert-body">' +
                '<div class="alert-title">' + esc(a.alert_type.replace(/_/g,' ')) + '</div>' +
                '<div class="alert-meta">' +
                    '<span><i class="fas fa-pencilcil"></i>' + esc(a.interface_name) + '</span>' +
                    '<span><i class="fas fa-pencilcil"></i>' + elapsed + '</span>' +
                    '<span style="color:var(--muted);">' + esc(a.message) + '</span>' +
                '</div>' +
            '</div>' +
            '<div class="alert-actions">' +
                '<button class="btn-sm ack" onclick="acknowledgeAlert(\'' + a.id + '\')">Ack</button>' +
                '<button class="btn-sm resolve" onclick="resolveAlert(\'' + a.id + '\')">Resolve</button>' +
            '</div>' +
        '</div>';
    }).join('');
}

function updateSeverityChips() {
    var c = 0, w = 0, i = 0;
    _alerts.forEach(function(a) {
        if (a.severity === 'critical') c++;
        else if (a.severity === 'warning') w++;
        else i++;
    });
    setText('chipAll', _alerts.length);
    setText('chipCritical', c);
    setText('chipWarning', w);
    setText('chipInfo', i);

    var badge = document.getElementById('activeCountBadge');
    if (badge) {
        var total = c + w;
        badge.textContent = total > 99 ? '99+' : total;
        badge.style.display = total > 0 ? '' : 'none';
        badge.className = 'leftnav-badge' + (c > 0 ? '' : ' warn');
    }
}

function acknowledgeAlert(id) {
    api('POST', '/fired/' + id + '/acknowledge', { acknowledged_by: getCurrentUser() })
        .then(function(r) {
            if (r.success) {
                _alerts = _alerts.filter(function(a) { return a.id !== id; });
                renderAlerts(); updateSeverityChips(); updateSidebarBadge(_alerts.length);
            } else { showToast('Failed: ' + r.error, false); }
        });
}

function resolveAlert(id) {
    api('POST', '/fired/' + id + '/resolve', {})
        .then(function(r) {
            if (r.success) {
                _alerts = _alerts.filter(function(a) { return a.id !== id; });
                renderAlerts(); updateSeverityChips(); updateSidebarBadge(_alerts.length);
            } else { showToast('Failed: ' + r.error, false); }
        });
}

function acknowledgeAll() {
    if (!confirm('Acknowledge all active alerts?')) return;
    api('POST', '/fired/acknowledge-all', { acknowledged_by: getCurrentUser() })
        .then(function(r) {
            if (r.success) {
                showToast('Acknowledged ' + (r.acknowledged || 0) + ' alerts', true);
                _alerts = [];
                renderAlerts(); updateSeverityChips(); updateSidebarBadge(0);
            } else { showToast('Failed: ' + r.error, false); }
        });
}

// ── Rules ─────────────────────────────────────────────────────────────────────
function loadRules() {
    return api('GET', '/rules').then(function(res) {
        _rules = res.data || [];
        renderRules();
    });
}

function renderRules() {
    var globalRules    = _rules.filter(function(r) { return !r.interface_id; });
    var interfaceRules = _rules.filter(function(r) { return  r.interface_id; });

    document.getElementById('globalRulesList').innerHTML    = renderRuleCards(globalRules);
    document.getElementById('interfaceRulesList').innerHTML = interfaceRules.length
        ? renderRuleCards(interfaceRules)
        : '<div style="font-size:13px;color:var(--muted);padding:8px 0">No per-interface rules defined. Create a rule and select a specific interface.</div>';
}

function renderRuleCards(rules) {
    if (!rules.length) return '';
    return rules.map(function(r) {
        var thresholds = [];
        if (r.threshold_warning  != null) thresholds.push('<span class="threshold-warn">⚠ ' + r.threshold_warning + '</span>');
        if (r.threshold_critical != null) thresholds.push('<span class="threshold-crit">🔴 ' + r.threshold_critical + '</span>');

        return '<div class="rule-card">' +
            '<span class="rule-type-badge">' + esc(r.rule_type.replace(/_/g,' ')) + '</span>' +
            '<div class="rule-info">' +
                '<div class="rule-name">' + esc(r.name) + (r.is_system ? ' <span style="font-size:10px;color:var(--muted)">(system)</span>' : '') + '</div>' +
                (r.interface_name ? '<div class="rule-desc">Interface: ' + esc(r.interface_name) + '</div>' : '') +
                '<div class="rule-thresholds">' + thresholds.join(' ') +
                    '<span style="color:var(--muted)">window: ' + r.evaluation_window_minutes + 'min</span>' +
                    '<span style="color:var(--muted)">cooldown: ' + r.cooldown_minutes + 'min</span>' +
                '</div>' +
            '</div>' +
            '<div class="rule-actions">' +
                '<span class="channel-count-chip" onclick="openAssignChannels(\'' + r.id + '\',\'' + esc(r.name) + '\')">' +
                    '<i class="fas fa-pencilcil"></i> ' + (r.channel_count || 0) + '</span>' +
                '<label class="toggle-switch" title="' + (r.is_enabled ? 'Disable' : 'Enable') + '">' +
                    '<input type="checkbox" ' + (r.is_enabled ? 'checked' : '') + ' onchange="toggleRule(\'' + r.id + '\',this.checked)">' +
                    '<span class="toggle-slider"></span>' +
                '</label>' +
                (!r.is_system ? '<button class="btn-sm" onclick="openRuleModal(\'' + r.id + '\')">Edit</button>' : '') +
                (!r.is_system ? '<button class="btn-sm" onclick="deleteRule(\'' + r.id + '\')" style="color:#dc2626">Del</button>' : '') +
            '</div>' +
        '</div>';
    }).join('');
}

function openRuleModal(id) {
    document.getElementById('ruleId').value = '';
    document.getElementById('ruleName').value = '';
    document.getElementById('ruleDescription').value = '';
    document.getElementById('ruleType').value = 'ERROR_RATE';
    document.getElementById('ruleCondition').value = 'gt';
    document.getElementById('ruleThresholdWarn').value = '';
    document.getElementById('ruleThresholdCrit').value = '';
    document.getElementById('ruleWindowMins').value = '60';
    document.getElementById('ruleCooldownMins').value = '60';
    document.getElementById('ruleInterfaceId').value = '';
    document.getElementById('ruleModalTitle').textContent = id ? 'Edit Alert Rule' : 'New Alert Rule';

    if (id) {
        var r = _rules.find(function(x) { return x.id === id; });
        if (r) {
            document.getElementById('ruleId').value             = r.id;
            document.getElementById('ruleName').value           = r.name;
            document.getElementById('ruleDescription').value    = r.description || '';
            document.getElementById('ruleType').value           = r.rule_type;
            document.getElementById('ruleCondition').value      = r.condition;
            document.getElementById('ruleThresholdWarn').value  = r.threshold_warning  != null ? r.threshold_warning  : '';
            document.getElementById('ruleThresholdCrit').value  = r.threshold_critical != null ? r.threshold_critical : '';
            document.getElementById('ruleWindowMins').value     = r.evaluation_window_minutes;
            document.getElementById('ruleCooldownMins').value   = r.cooldown_minutes;
            document.getElementById('ruleInterfaceId').value    = r.interface_id || '';
        }
    }
    openModal('ruleModal');
    populateInterfaceSelects();
}

function saveRule() {
    var id = document.getElementById('ruleId').value;
    var warn = document.getElementById('ruleThresholdWarn').value;
    var crit = document.getElementById('ruleThresholdCrit').value;
    var ifaceId = document.getElementById('ruleInterfaceId').value;
    var body = {
        name:                     document.getElementById('ruleName').value.trim(),
        description:              document.getElementById('ruleDescription').value.trim(),
        rule_type:                document.getElementById('ruleType').value,
        condition:                document.getElementById('ruleCondition').value,
        threshold_warning:        warn !== '' ? parseFloat(warn) : null,
        threshold_critical:       crit !== '' ? parseFloat(crit) : null,
        evaluation_window_minutes:parseInt(document.getElementById('ruleWindowMins').value) || 60,
        cooldown_minutes:         parseInt(document.getElementById('ruleCooldownMins').value) || 60,
        interface_id:             ifaceId || null,
        is_enabled:               true,
    };
    if (!body.name) { showToast('Name is required', false); return; }
    var req = id
        ? api('PUT',  '/rules/' + id, body)
        : api('POST', '/rules',       body);
    req.then(function(r) {
        if (r.success) {
            showToast(id ? 'Rule updated' : 'Rule created', true);
            closeModal('ruleModal');
            _tabLoaded['rules'] = false;
            loadRules();
        } else { showToast(r.error || 'Error', false); }
    });
}

function toggleRule(id, enabled) {
    var r = _rules.find(function(x) { return x.id === id; });
    if (!r) return;
    api('PUT', '/rules/' + id, Object.assign({}, r, { is_enabled: enabled }))
        .then(function(res) {
            if (!res.success) { showToast(res.error, false); loadRules(); }
        });
}

function deleteRule(id) {
    if (!confirm('Delete this alert rule?')) return;
    api('DELETE', '/rules/' + id).then(function(r) {
        if (r.success) { showToast('Rule deleted', true); loadRules(); }
        else { showToast(r.error, false); }
    });
}

// ── Channel assignment ─────────────────────────────────────────────────────────
function openAssignChannels(ruleId, ruleName) {
    _assignRuleId = ruleId;
    document.getElementById('assignChannelsTitle').textContent = 'Channels for: ' + ruleName;
    var el = document.getElementById('assignChannelsList');
    el.innerHTML = '<div class="loading-row"><i class="fas fa-pencilcil"></i></div>';
    openModal('assignChannelsModal');

    Promise.all([
        api('GET', '/channels'),
        api('GET', '/rules/' + ruleId + '/channels')
    ]).then(function(results) {
        var allChannels     = results[0].data || [];
        var assignedChannels = results[1].data || [];
        var assignedIds = assignedChannels.map(function(a) { return a.channel_id; });

        if (!allChannels.length) {
            el.innerHTML = '<p style="font-size:13px;color:var(--muted)">No channels configured. Create a channel first.</p>';
            return;
        }
        el.innerHTML = allChannels.map(function(ch) {
            var checked = assignedIds.indexOf(ch.id) !== -1;
            var iconClass = ch.channel_type === 'email' ? 'fa-envelope' : ch.channel_type === 'slack' ? 'fa-slack' : 'fa-globe';
            return '<label style="display:flex;align-items:center;gap:10px;padding:10px 0;border-bottom:1px solid #f3f4f6;cursor:pointer">' +
                '<input type="checkbox" data-channel-id="' + ch.id + '" ' + (checked ? 'checked' : '') + '>' +
                '<i class="fab ' + iconClass + '" style="width:18px;color:var(--muted)"></i>' +
                '<span style="font-size:13px;font-weight:600">' + esc(ch.name) + '</span>' +
                '<span style="font-size:11px;color:var(--muted);margin-left:auto">' + ch.channel_type + '</span>' +
            '</label>';
        }).join('');
    });
}

function saveChannelAssignments() {
    var checked = document.querySelectorAll('#assignChannelsList input[type=checkbox]:checked');
    var assignments = Array.from(checked).map(function(cb) {
        return { rule_id: _assignRuleId, channel_id: cb.dataset.channelId, severity_filter: [] };
    });
    api('PUT', '/rules/' + _assignRuleId + '/channels', { channels: assignments })
        .then(function(r) {
            if (r.success) {
                showToast('Channels saved', true);
                closeModal('assignChannelsModal');
                loadRules();
            } else { showToast(r.error, false); }
        });
}

// ── Channels ──────────────────────────────────────────────────────────────────
function loadChannels() {
    return api('GET', '/channels').then(function(res) {
        _channels = res.data || [];
        renderChannels();
    });
}

function renderChannels() {
    var el = document.getElementById('channelsList');
    if (!el) return;
    if (!_channels.length) {
        el.innerHTML = '<div class="empty-state"><i class="fas fa-pencilcil"></i><h3>No channels configured</h3><p>Add an email, webhook, or Slack channel to start receiving notifications.</p></div>';
        return;
    }
    var iconMap = { email: 'fa-envelope', webhook: 'fa-globe', slack: 'fa-slack' };
    el.innerHTML = _channels.map(function(ch) {
        var icon = iconMap[ch.channel_type] || 'fa-bell';
        var testClass = ch.test_status === 'success' ? 'success' : ch.test_status === 'failure' ? 'failure' : 'untested';
        var testLabel = ch.test_status === 'success' ? '✓ Tested' : ch.test_status === 'failure' ? '✗ Failed' : 'Untested';
        var fabOrFas = ch.channel_type === 'slack' ? 'fab' : 'fas';
        return '<div class="channel-card">' +
            '<div class="channel-header">' +
                '<div class="channel-icon ' + ch.channel_type + '"><i class="' + fabOrFas + ' ' + icon + '"></i></div>' +
                '<div>' +
                    '<div class="channel-name">' + esc(ch.name) + '</div>' +
                    '<div class="channel-type-label">' + ch.channel_type.toUpperCase() + '</div>' +
                '</div>' +
                '<span class="test-badge ' + testClass + '">' + testLabel + '</span>' +
            '</div>' +
            '<div class="channel-footer">' +
                '<button class="btn-sm" onclick="testChannel(\'' + ch.id + '\')"><i class="fas fa-pencilcil"></i> Test</button>' +
                '<button class="btn-sm" onclick="openChannelModal(\'' + ch.id + '\')">Edit</button>' +
                '<button class="btn-sm" onclick="deleteChannel(\'' + ch.id + '\')" style="color:#dc2626">Delete</button>' +
            '</div>' +
        '</div>';
    }).join('');
}

function openChannelModal(id) {
    _editChannelId = id || '';
    document.getElementById('channelId').value = id || '';
    document.getElementById('channelName').value = '';
    document.getElementById('channelType').value = 'email';
    document.getElementById('channelModalTitle').textContent = id ? 'Edit Channel' : 'New Notification Channel';
    if (id) {
        var ch = _channels.find(function(x) { return x.id === id; });
        if (ch) {
            document.getElementById('channelName').value = ch.name;
            document.getElementById('channelType').value = ch.channel_type;
        }
    }
    renderChannelFields(id);
    openModal('channelModal');
}

function renderChannelFields(id) {
    var type = document.getElementById('channelType').value;
    var ch = id && _channels.find(function(x) { return x.id === id; });
    var cfg = {};
    if (ch && ch.config) {
        try { cfg = typeof ch.config === 'string' ? JSON.parse(ch.config) : ch.config; } catch(e) {}
    }
    var html = '';
    if (type === 'email') {
        html = field('SMTP Host', 'chSmtpHost', cfg.smtp_host || '', 'smtp.example.com') +
               field('SMTP Port', 'chSmtpPort', cfg.smtp_port || '587', '587', 'number') +
               field('SMTP User', 'chSmtpUser', cfg.smtp_user || '', 'user@example.com') +
               field('SMTP Password', 'chSmtpPass', '', '••••••••', 'password') +
               field('From Address', 'chFrom', cfg.from || '', 'alerts@example.com') +
               field('To (comma-separated)', 'chTo', (cfg.to || []).join(', '), 'ops@example.com, oncall@example.com');
    } else if (type === 'webhook') {
        html = field('Webhook URL *', 'chUrl', cfg.url || '', 'https://example.com/webhook') +
               field('Method', 'chMethod', cfg.method || 'POST', 'POST') +
               field('Authorization Header', 'chAuthHeader', (cfg.headers || {}).Authorization || '', 'Bearer token...');
    } else if (type === 'slack') {
        html = field('Slack Webhook URL *', 'chSlackUrl', cfg.webhook_url || '', 'https://hooks.slack.com/services/...') +
               field('Channel', 'chSlackChannel', cfg.channel || '#alerts', '#alerts') +
               field('Bot Username', 'chSlackUser', cfg.username || 'ezHealthKonnect', 'ezHealthKonnect');
    }
    document.getElementById('channelDynamicFields').innerHTML = html;
}

function field(label, id, val, placeholder, type) {
    return '<div class="form-group"><label>' + label + '</label>' +
        '<input type="' + (type || 'text') + '" id="' + id + '" value="' + esc(val) + '" placeholder="' + esc(placeholder) + '"></div>';
}

function collectChannelConfig() {
    var type = document.getElementById('channelType').value;
    var g = function(id) { return (document.getElementById(id) || {}).value || ''; };
    if (type === 'email') {
        var toList = g('chTo').split(',').map(function(s) { return s.trim(); }).filter(Boolean);
        return { smtp_host: g('chSmtpHost'), smtp_port: g('chSmtpPort'), smtp_user: g('chSmtpUser'),
                 smtp_password_enc: g('chSmtpPass'), from: g('chFrom'), to: toList };
    }
    if (type === 'webhook') {
        var headers = {};
        var auth = g('chAuthHeader');
        if (auth) headers.Authorization = auth;
        return { url: g('chUrl'), method: g('chMethod') || 'POST', headers: headers };
    }
    if (type === 'slack') {
        return { webhook_url: g('chSlackUrl'), channel: g('chSlackChannel'), username: g('chSlackUser') };
    }
    return {};
}

function saveChannel() {
    var id = document.getElementById('channelId').value;
    var body = {
        name:         document.getElementById('channelName').value.trim(),
        channel_type: document.getElementById('channelType').value,
        config:       collectChannelConfig(),
        is_enabled:   true,
    };
    if (!body.name) { showToast('Name is required', false); return; }
    var req = id
        ? api('PUT',  '/channels/' + id, body)
        : api('POST', '/channels',        body);
    req.then(function(r) {
        if (r.success) {
            showToast(id ? 'Channel updated' : 'Channel created', true);
            closeModal('channelModal');
            loadChannels();
        } else { showToast(r.error || 'Error', false); }
    });
}

function testChannel(id) {
    showToast('Sending test…', true);
    api('POST', '/channels/' + id + '/test').then(function(r) {
        if (r.success) { showToast('Test sent successfully', true); loadChannels(); }
        else { showToast('Test failed: ' + r.error, false); loadChannels(); }
    });
}

function deleteChannel(id) {
    if (!confirm('Delete this channel?')) return;
    api('DELETE', '/channels/' + id).then(function(r) {
        if (r.success) { showToast('Channel deleted', true); loadChannels(); }
        else { showToast(r.error, false); }
    });
}

// ── Silences ──────────────────────────────────────────────────────────────────
function loadSilences() {
    return api('GET', '/silences').then(function(res) {
        _silences = res.data || [];
        renderSilences();
    });
}

function renderSilences() {
    var el = document.getElementById('silencesList');
    if (!el) return;
    if (!_silences.length) {
        el.innerHTML = '<div class="empty-state"><i class="fas fa-pencilcil"></i><h3>No silences configured</h3><p>Create a silence to suppress alerts during maintenance windows.</p></div>';
        return;
    }
    el.innerHTML = _silences.map(function(s) {
        var isActive = s.is_active;
        return '<div class="silence-card">' +
            '<div class="' + (isActive ? 'silence-active-dot' : 'silence-inactive-dot') + '" title="' + (isActive ? 'Active' : 'Inactive') + '"></div>' +
            '<div class="silence-info">' +
                '<div class="silence-name">' + esc(s.name) + (isActive ? ' <span style="font-size:10px;background:#dcfce7;color:#166534;padding:1px 6px;border-radius:8px">ACTIVE</span>' : '') + '</div>' +
                '<div class="silence-meta">' +
                    (s.interface_name ? 'Interface: ' + esc(s.interface_name) + ' · ' : 'All interfaces · ') +
                    (s.alert_rule_name ? 'Rule: ' + esc(s.alert_rule_name) + ' · ' : 'All rules · ') +
                    formatDT(s.starts_at) + ' → ' + formatDT(s.ends_at) +
                    (s.reason ? ' · ' + esc(s.reason) : '') +
                '</div>' +
            '</div>' +
            '<button class="btn-sm" onclick="deleteSilence(\'' + s.id + '\')" style="color:#dc2626">Remove</button>' +
        '</div>';
    }).join('');
}

function openSilenceModal() {
    var now = new Date();
    var end = new Date(now.getTime() + 2 * 60 * 60 * 1000);
    document.getElementById('silenceName').value = '';
    document.getElementById('silenceReason').value = '';
    document.getElementById('silenceStartsAt').value = toLocalInput(now);
    document.getElementById('silenceEndsAt').value   = toLocalInput(end);
    document.getElementById('silenceInterfaceId').value = '';
    document.getElementById('silenceRuleId').value = '';
    populateInterfaceSelects();
    populateRuleSelects();
    openModal('silenceModal');
}

function saveSilence() {
    var body = {
        name:         document.getElementById('silenceName').value.trim(),
        reason:       document.getElementById('silenceReason').value.trim(),
        starts_at:    document.getElementById('silenceStartsAt').value,
        ends_at:      document.getElementById('silenceEndsAt').value,
        interface_id: document.getElementById('silenceInterfaceId').value || null,
        alert_rule_id:document.getElementById('silenceRuleId').value || null,
    };
    if (!body.name || !body.starts_at || !body.ends_at) { showToast('Name, start and end are required', false); return; }
    api('POST', '/silences', body).then(function(r) {
        if (r.success) {
            showToast('Silence created', true);
            closeModal('silenceModal');
            loadSilences();
        } else { showToast(r.error || 'Error', false); }
    });
}

function deleteSilence(id) {
    if (!confirm('Remove this silence?')) return;
    api('DELETE', '/silences/' + id).then(function(r) {
        if (r.success) { showToast('Silence removed', true); loadSilences(); }
        else { showToast(r.error, false); }
    });
}

// ── History ────────────────────────────────────────────────────────────────────
function loadHistory() {
    return fetch('/api/alerts/history', { credentials: 'include' })
        .then(function(r) { return r.json(); })
        .then(function(res) {
            _history = res.data || [];
            renderHistory();
        });
}

function renderHistory() {
    var sevF  = (document.getElementById('histSeverityFilter') || {}).value || '';
    var typeF = (document.getElementById('histTypeFilter') || {}).value || '';
    var rows = _history.filter(function(a) {
        if (sevF  && a.severity   !== sevF)  return false;
        if (typeF && a.alert_type !== typeF) return false;
        return true;
    });
    var body = document.getElementById('historyBody');
    if (!body) return;
    if (!rows.length) {
        body.innerHTML = '<tr><td colspan="8" class="empty-state">No alert history found.</td></tr>';
        return;
    }
    body.innerHTML = rows.map(function(a) {
        var status = a.auto_resolved ? '<span style="color:#166534">Resolved</span>'
                   : a.acknowledged  ? '<span style="color:var(--navy)">Acknowledged</span>'
                   : '<span style="color:#dc2626">Active</span>';
        var resolvedAt = a.resolved_at ? formatDT(a.resolved_at) : '—';
        var duration   = '—';
        if (a.resolved_at && a.created_at) {
            var secs = Math.round((new Date(a.resolved_at) - new Date(a.created_at)) / 1000);
            duration = secs < 60 ? secs + 's'
                     : secs < 3600 ? Math.round(secs / 60) + 'm'
                     : (secs / 3600).toFixed(1) + 'h';
        }
        return '<tr>' +
            '<td><span class="sev-pill ' + a.severity + '">' + a.severity.toUpperCase() + '</span></td>' +
            '<td>' + esc(a.alert_type.replace(/_/g,' ')) + '</td>' +
            '<td>' + esc(a.interface_name || 'System') + '</td>' +
            '<td style="max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="' + esc(a.message) + '">' + esc(a.message) + '</td>' +
            '<td style="white-space:nowrap">' + formatDT(a.created_at) + '</td>' +
            '<td style="white-space:nowrap">' + resolvedAt + '</td>' +
            '<td style="white-space:nowrap">' + duration + '</td>' +
            '<td>' + status + '</td>' +
        '</tr>';
    }).join('');
}

function exportHistoryCSV() {
    var sevF  = (document.getElementById('histSeverityFilter') || {}).value || '';
    var typeF = (document.getElementById('histTypeFilter') || {}).value || '';
    var rows = _history.filter(function(a) {
        if (sevF  && a.severity   !== sevF)  return false;
        if (typeF && a.alert_type !== typeF) return false;
        return true;
    });
    if (!rows.length) { showToast('No rows to export', false); return; }
    var header = ['Severity','Type','Interface','Message','Fired At','Resolved At','Duration','Status'];
    var lines  = [header.join(',')];
    rows.forEach(function(a) {
        var resolvedAt = a.resolved_at || '';
        var duration   = '';
        if (a.resolved_at && a.created_at) {
            var secs = Math.round((new Date(a.resolved_at) - new Date(a.created_at)) / 1000);
            duration = secs < 60 ? secs + 's' : secs < 3600 ? Math.round(secs/60) + 'm' : (secs/3600).toFixed(1) + 'h';
        }
        var status = a.auto_resolved ? 'Resolved' : a.acknowledged ? 'Acknowledged' : 'Active';
        lines.push([
            a.severity, a.alert_type, a.interface_name || 'System',
            '"' + (a.message || '').replace(/"/g,'""') + '"',
            a.created_at, resolvedAt, duration, status
        ].join(','));
    });
    var blob = new Blob([lines.join('\n')], { type: 'text/csv' });
    var url  = URL.createObjectURL(blob);
    var a    = document.createElement('a');
    a.href   = url;
    a.download = 'alert-history-' + new Date().toISOString().slice(0,10) + '.csv';
    a.click();
    URL.revokeObjectURL(url);
}

// ── Sidebar badge ──────────────────────────────────────────────────────────────
function updateSidebarBadge(count) {
    var badge = document.getElementById('alertsNavBadge');
    if (!badge) return;
    badge.textContent = count > 99 ? '99+' : count;
    badge.style.display = count > 0 ? '' : 'none';
}

// ── Modal helpers ──────────────────────────────────────────────────────────────
function openModal(id)  { document.getElementById(id).classList.add('open'); }
function closeModal(id) { document.getElementById(id).classList.remove('open'); }

// ── Interface / Rule selects ───────────────────────────────────────────────────
function populateInterfaceSelects() {
    fetch('/api/interfaces', { credentials: 'include' })
        .then(function(r) { return r.json(); })
        .then(function(res) {
            var interfaces = res.data || res.interfaces || [];
            var opts = '<option value="">— Global rule (all interfaces) —</option>' +
                interfaces.map(function(i) {
                    return '<option value="' + i.id + '">' + esc(i.name) + '</option>';
                }).join('');
            ['ruleInterfaceId', 'silenceInterfaceId'].forEach(function(id) {
                var el = document.getElementById(id);
                if (el) {
                    var current = el.value;
                    el.innerHTML = opts;
                    el.value = current;
                }
            });
        }).catch(function() {});
}

function populateRuleSelects() {
    var opts = '<option value="">All rules</option>' +
        _rules.map(function(r) {
            return '<option value="' + r.id + '">' + esc(r.name) + '</option>';
        }).join('');
    var el = document.getElementById('silenceRuleId');
    if (el) el.innerHTML = opts;
}

// ── Utilities ─────────────────────────────────────────────────────────────────
function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function setText(id, val) {
    var el = document.getElementById(id);
    if (el) el.textContent = val;
}

function formatElapsed(seconds) {
    if (!seconds) return 'just now';
    if (seconds < 60) return seconds + 's ago';
    if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
    if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
    return Math.floor(seconds / 86400) + 'd ago';
}

function formatDT(iso) {
    if (!iso) return '—';
    try {
        return new Date(iso).toLocaleString(undefined, { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' });
    } catch(e) { return iso; }
}

function toLocalInput(date) {
    var off = date.getTimezoneOffset() * 60000;
    return new Date(date - off).toISOString().slice(0, 16);
}

function getCurrentUser() {
    var el = document.querySelector('[data-current-user]');
    return el ? el.dataset.currentUser : 'system';
}

// ── Init ──────────────────────────────────────────────────────────────────────
window.addEventListener('load', function() {
    var hash = location.hash.replace('#', '') || 'active-alerts';
    activateTab(hash);

    // Live polling: refresh active alerts every 30s
    _pollTimer = setInterval(function() {
        if (!document.hidden) loadActiveAlerts();
    }, 30000);
});

window.addEventListener('beforeunload', function() {
    if (_pollTimer) clearInterval(_pollTimer);
});
