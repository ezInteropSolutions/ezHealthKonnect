/**
 * alert-docs.js
 * In-app documentation data for the Alert Management system.
 * Consumed by the help panel rendered in alerts.html.
 */

/* eslint-disable no-unused-vars */
window.ALERT_DOCS = {

    // ── Top-level sections ──────────────────────────────────────────────────────
    sections: [

        // ── Overview ────────────────────────────────────────────────────────────
        {
            id: 'overview',
            title: 'Overview',
            subsections: [
                {
                    id: 'overview-what',
                    title: 'What is Alert Management?',
                    body: `
                        <p>The Alert Management system monitors your integration interfaces in
                        real-time and notifies your team when something goes wrong — before
                        downstream systems are affected.</p>
                        <p>It replaces hardcoded thresholds with a fully configurable, database-driven
                        rule engine. Every rule is evaluated on a rolling window of live metrics and can
                        deliver notifications to email, webhook, or Slack channels.</p>
                        <h4>Core Concepts</h4>
                        <table class="doc-table">
                            <thead><tr><th>Concept</th><th>Description</th></tr></thead>
                            <tbody>
                                <tr><td><strong>Alert Rule</strong></td><td>A condition that, when met, fires a monitoring alert. Example: <em>error rate > 10% for 60 minutes</em>.</td></tr>
                                <tr><td><strong>Notification Channel</strong></td><td>A delivery destination: email inbox, HTTP webhook, or Slack channel.</td></tr>
                                <tr><td><strong>Fired Alert</strong></td><td>A record in the <code>monitoring_alerts</code> table created when a rule threshold is exceeded.</td></tr>
                                <tr><td><strong>Silence</strong></td><td>A maintenance window that suppresses alert firing during a scheduled time range.</td></tr>
                                <tr><td><strong>OOB Rule</strong></td><td>A built-in system rule seeded at startup. Can be disabled but never deleted.</td></tr>
                            </tbody>
                        </table>
                    `
                },
                {
                    id: 'overview-flow',
                    title: 'How it Works',
                    body: `
                        <h4>Evaluation Flow</h4>
                        <div class="doc-flow">
                            <div class="flow-step"><span class="flow-num">1</span><span>Analytics engine runs every 5 minutes per interface</span></div>
                            <div class="flow-step"><span class="flow-num">2</span><span>Loads all enabled rules (global + per-interface)</span></div>
                            <div class="flow-step"><span class="flow-num">3</span><span>Computes live metric for each rule type over the evaluation window</span></div>
                            <div class="flow-step"><span class="flow-num">4</span><span>Checks for active silence — suppresses if match found</span></div>
                            <div class="flow-step"><span class="flow-num">5</span><span>Checks cooldown dedup — suppresses if same alert fired recently</span></div>
                            <div class="flow-step"><span class="flow-num">6</span><span>Inserts row in <code>monitoring_alerts</code> and dispatches notifications</span></div>
                            <div class="flow-step"><span class="flow-num">7</span><span>Auto-resolves existing open alert when metric returns to normal</span></div>
                        </div>
                        <h4>Rule Precedence</h4>
                        <p>When a per-interface rule and a global rule share the same <code>rule_type</code>, the
                        <strong>per-interface rule takes precedence</strong> for that interface. This lets you tighten
                        thresholds on critical interfaces while keeping relaxed global defaults.</p>
                    `
                }
            ]
        },

        // ── Alert Rules ─────────────────────────────────────────────────────────
        {
            id: 'rules',
            title: 'Alert Rules',
            subsections: [
                {
                    id: 'rules-types',
                    title: 'Rule Types',
                    body: `
                        <p>Each rule targets one metric type. The system computes the metric value over the
                        configured <strong>Evaluation Window</strong> and compares it against your thresholds.</p>
                        <table class="doc-table">
                            <thead><tr><th>Rule Type</th><th>Metric Computed</th><th>Unit</th><th>Typical Thresholds</th></tr></thead>
                            <tbody>
                                <tr>
                                    <td><code>ERROR_RATE</code></td>
                                    <td>% of messages with status <code>error</code> or <code>failed</code></td>
                                    <td>percent</td>
                                    <td>Warning: 5 &nbsp; Critical: 10</td>
                                </tr>
                                <tr>
                                    <td><code>PROCESSING_TIME</code></td>
                                    <td>Average <code>processing_time_ms</code> across all messages</td>
                                    <td>milliseconds</td>
                                    <td>Warning: 2000 &nbsp; Critical: 5000</td>
                                </tr>
                                <tr>
                                    <td><code>DLQ_DEPTH</code></td>
                                    <td>Count of unprocessed rows in <code>dead_letter_queue</code></td>
                                    <td>count</td>
                                    <td>Warning: 10 &nbsp; Critical: 50</td>
                                </tr>
                                <tr>
                                    <td><code>INTERFACE_INACTIVE</code></td>
                                    <td>Minutes since the last message was received</td>
                                    <td>minutes</td>
                                    <td>Warning: 60 &nbsp; Critical: 120</td>
                                </tr>
                                <tr>
                                    <td><code>CONNECTOR_FAILURE</code></td>
                                    <td>Count of <code>failed</code> rows in <code>connectivity_execution_log</code></td>
                                    <td>count</td>
                                    <td>Warning: 3 &nbsp; Critical: 10</td>
                                </tr>
                                <tr>
                                    <td><code>THROUGHPUT_LOW</code></td>
                                    <td>Messages per minute over the evaluation window</td>
                                    <td>msg/min</td>
                                    <td>Warning: 10 &nbsp; Critical: 2</td>
                                </tr>
                                <tr>
                                    <td><code>VALIDATION_FAILURE</code></td>
                                    <td>Count of validation failures in the processing queue</td>
                                    <td>count</td>
                                    <td>Warning: 5 &nbsp; Critical: 20</td>
                                </tr>
                            </tbody>
                        </table>
                        <div class="doc-tip">
                            <strong>Tip:</strong> For <code>THROUGHPUT_LOW</code> the condition should be <code>lt</code> (less than)
                            because low throughput is the problem. For all other types, use <code>gt</code> (greater than).
                        </div>
                    `
                },
                {
                    id: 'rules-scope',
                    title: 'Global vs Per-Interface',
                    body: `
                        <h4>Global Rules</h4>
                        <p>A rule created without an <strong>Interface</strong> selection applies to <em>every</em> interface
                        in the system. The OOB (out-of-box) system rules are all global rules.</p>
                        <h4>Per-Interface Rules</h4>
                        <p>Assign a rule to a specific interface to override the global threshold for that interface only.
                        This is useful when one interface handles higher volumes or has stricter SLA requirements.</p>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — tighter threshold for a high-priority inbound feed</div>
<pre>{
  "name": "CRM Inbound — Strict Error Rate",
  "rule_type": "ERROR_RATE",
  "interface_id": "762aebb9-0408-4a42-82c5-202f13f28315",
  "condition": "gt",
  "threshold_warning": 2,
  "threshold_critical": 5,
  "evaluation_window_minutes": 30,
  "cooldown_minutes": 15,
  "is_enabled": true
}</pre>
                        </div>
                        <p>In this example the <em>global</em> OOB rule fires at 5% / 10%, but this specific interface
                        fires at 2% / 5% — a tighter SLA without touching the global rule.</p>
                    `
                },
                {
                    id: 'rules-thresholds',
                    title: 'Thresholds & Conditions',
                    body: `
                        <p>Each rule can have an optional <strong>Warning</strong> threshold and an optional
                        <strong>Critical</strong> threshold. Omit one to disable that severity level.</p>
                        <table class="doc-table">
                            <thead><tr><th>Condition</th><th>Symbol</th><th>Fires when metric is…</th></tr></thead>
                            <tbody>
                                <tr><td><code>gt</code></td><td>&gt;</td><td>strictly greater than the threshold</td></tr>
                                <tr><td><code>gte</code></td><td>≥</td><td>greater than or equal to the threshold</td></tr>
                                <tr><td><code>lt</code></td><td>&lt;</td><td>strictly less than the threshold (use for THROUGHPUT_LOW)</td></tr>
                                <tr><td><code>lte</code></td><td>≤</td><td>less than or equal to the threshold</td></tr>
                            </tbody>
                        </table>
                        <h4>Severity Resolution</h4>
                        <p>When a metric breaches <em>both</em> warning and critical, the rule fires as <strong>critical</strong>.
                        If only warning is breached, severity is <strong>warning</strong>. A rule with only one threshold
                        configured always fires at that severity.</p>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — warning-only rule (no critical)</div>
<pre>{
  "name": "DLQ Early Warning",
  "rule_type": "DLQ_DEPTH",
  "condition": "gt",
  "threshold_warning": 5,
  "threshold_critical": null,
  "evaluation_window_minutes": 60
}</pre>
                        </div>
                    `
                },
                {
                    id: 'rules-windows',
                    title: 'Evaluation Window & Cooldown',
                    body: `
                        <h4>Evaluation Window</h4>
                        <p>The rolling time window (in minutes) over which the metric is computed.
                        Shorter windows are more sensitive to spikes. Longer windows smooth out transient blips.</p>
                        <table class="doc-table">
                            <thead><tr><th>Use Case</th><th>Recommended Window</th></tr></thead>
                            <tbody>
                                <tr><td>Spike detection (real-time ops)</td><td>5 – 15 minutes</td></tr>
                                <tr><td>Normal operational alerting</td><td>30 – 60 minutes</td></tr>
                                <tr><td>SLA / sustained degradation</td><td>120 – 240 minutes</td></tr>
                                <tr><td>Overnight inactivity</td><td>480 minutes (8 hours)</td></tr>
                            </tbody>
                        </table>
                        <h4>Cooldown</h4>
                        <p>After an alert fires, the cooldown period (in minutes) prevents the same rule from
                        firing again — even if the metric stays above the threshold. This avoids notification
                        storms during sustained incidents.</p>
                        <div class="doc-tip">
                            <strong>Best practice:</strong> Set the cooldown to at least 2× the evaluation window
                            so alerts don't fire again before the window has fully refreshed.
                        </div>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — 30-min window with 60-min cooldown</div>
<pre>{
  "name": "Slow Processing Alert",
  "rule_type": "PROCESSING_TIME",
  "condition": "gt",
  "threshold_warning": 2000,
  "threshold_critical": 5000,
  "evaluation_window_minutes": 30,
  "cooldown_minutes": 60
}</pre>
                        </div>
                    `
                },
                {
                    id: 'rules-oob',
                    title: 'Out-of-Box (OOB) System Rules',
                    body: `
                        <p>Six system rules are seeded at startup. They implement the thresholds that were
                        previously hardcoded in the analytics engine.</p>
                        <table class="doc-table">
                            <thead><tr><th>Name</th><th>Type</th><th>Warning</th><th>Critical</th><th>Window</th></tr></thead>
                            <tbody>
                                <tr><td>Error Rate — Warning</td><td>ERROR_RATE</td><td>5%</td><td>10%</td><td>60 min</td></tr>
                                <tr><td>Slow Processing</td><td>PROCESSING_TIME</td><td>2000 ms</td><td>5000 ms</td><td>60 min</td></tr>
                                <tr><td>DLQ Depth</td><td>DLQ_DEPTH</td><td>10</td><td>50</td><td>60 min</td></tr>
                                <tr><td>Interface Inactive</td><td>INTERFACE_INACTIVE</td><td>60 min</td><td>120 min</td><td>240 min</td></tr>
                                <tr><td>Connector Failures</td><td>CONNECTOR_FAILURE</td><td>3</td><td>10</td><td>60 min</td></tr>
                                <tr><td>Validation Failures</td><td>VALIDATION_FAILURE</td><td>5</td><td>20</td><td>60 min</td></tr>
                            </tbody>
                        </table>
                        <div class="doc-warning">
                            <strong>OOB rules cannot be deleted</strong> — they carry the <code>is_system = true</code> flag.
                            You can <strong>disable</strong> them using the toggle on the Rules tab, or create a
                            per-interface rule to override the threshold for specific interfaces.
                        </div>
                    `
                }
            ]
        },

        // ── Notification Channels ────────────────────────────────────────────────
        {
            id: 'channels',
            title: 'Notification Channels',
            subsections: [
                {
                    id: 'channels-overview',
                    title: 'Overview',
                    body: `
                        <p>Channels are delivery destinations for alert notifications. Once created, assign
                        channels to rules on the <strong>Rules</strong> tab using the channel count chip on each rule card.</p>
                        <p>A channel can be assigned to multiple rules. A rule can have multiple channels —
                        all assigned channels receive the notification concurrently.</p>
                        <h4>Channel Types</h4>
                        <table class="doc-table">
                            <thead><tr><th>Type</th><th>Use Case</th><th>Delivery Method</th></tr></thead>
                            <tbody>
                                <tr><td>Email (SMTP)</td><td>Operations team inboxes, on-call distribution lists</td><td>SMTP STARTTLS, HTML + plain text</td></tr>
                                <tr><td>Webhook (HTTP)</td><td>PagerDuty, OpsGenie, custom incident systems</td><td>HTTP POST with JSON payload</td></tr>
                                <tr><td>Slack</td><td>Engineering Slack channels, #ops-alerts</td><td>Slack Incoming Webhook (Block Kit)</td></tr>
                            </tbody>
                        </table>
                    `
                },
                {
                    id: 'channels-email',
                    title: 'Email (SMTP)',
                    body: `
                        <p>Delivers HTML + plain-text email via SMTP with STARTTLS. Supports multiple recipients.</p>
                        <h4>Required Configuration Fields</h4>
                        <table class="doc-table">
                            <thead><tr><th>Field</th><th>Description</th><th>Example</th></tr></thead>
                            <tbody>
                                <tr><td><code>to</code></td><td>Recipient email(s), comma-separated</td><td><code>ops@company.com, oncall@company.com</code></td></tr>
                                <tr><td><code>smtp_host</code></td><td>SMTP server hostname</td><td><code>smtp.office365.com</code></td></tr>
                                <tr><td><code>smtp_port</code></td><td>SMTP port (usually 587 for STARTTLS)</td><td><code>587</code></td></tr>
                                <tr><td><code>smtp_user</code></td><td>SMTP authentication username</td><td><code>alerts@company.com</code></td></tr>
                                <tr><td><code>smtp_password</code></td><td>SMTP password (stored encrypted)</td><td>—</td></tr>
                                <tr><td><code>from</code></td><td>Sender display address</td><td><code>ezHealthKonnect Alerts &lt;alerts@company.com&gt;</code></td></tr>
                            </tbody>
                        </table>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — Office 365 SMTP channel</div>
<pre>{
  "name": "Ops Team Email",
  "channel_type": "email",
  "config": {
    "to": "ops-team@company.com, oncall@company.com",
    "smtp_host": "smtp.office365.com",
    "smtp_port": 587,
    "smtp_user": "alerts@company.com",
    "smtp_password": "••••••••",
    "from": "ezHealthKonnect &lt;alerts@company.com&gt;"
  }
}</pre>
                        </div>
                        <div class="doc-tip">
                            <strong>Test before assigning:</strong> Use the <em>Test</em> button on the channel card to
                            send a sample notification and verify SMTP connectivity. The card will show a
                            green <strong>Tested</strong> or red <strong>Failed</strong> badge.
                        </div>
                    `
                },
                {
                    id: 'channels-webhook',
                    title: 'Webhook (HTTP)',
                    body: `
                        <p>Sends a JSON POST to any HTTP endpoint. Ideal for PagerDuty, OpsGenie, ServiceNow, or
                        custom incident management APIs.</p>
                        <h4>Configuration Fields</h4>
                        <table class="doc-table">
                            <thead><tr><th>Field</th><th>Description</th><th>Default</th></tr></thead>
                            <tbody>
                                <tr><td><code>url</code></td><td>Target URL (must include protocol)</td><td>—</td></tr>
                                <tr><td><code>method</code></td><td>HTTP method</td><td>POST</td></tr>
                                <tr><td><code>headers</code></td><td>Custom request headers (key-value pairs)</td><td>{}</td></tr>
                                <tr><td><code>timeout_seconds</code></td><td>Request timeout in seconds</td><td>10</td></tr>
                            </tbody>
                        </table>
                        <h4>Payload Structure</h4>
                        <p>The webhook receives a JSON body with the following fields:</p>
                        <div class="doc-example">
                            <div class="doc-example-label">Webhook payload example</div>
<pre>{
  "alert_id": "a1b2c3d4-...",
  "alert_type": "ERROR_RATE",
  "severity": "critical",
  "interface_id": "762aebb9-...",
  "interface_name": "CRM Order Inbound",
  "message": "Error rate 12.4% exceeds critical threshold of 10%",
  "value": 12.4,
  "threshold": 10,
  "fired_at": "2026-03-21T14:00:00Z",
  "source": "ezHealthKonnect"
}</pre>
                        </div>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — PagerDuty Events API v2</div>
<pre>{
  "name": "PagerDuty Critical",
  "channel_type": "webhook",
  "config": {
    "url": "https://events.pagerduty.com/v2/enqueue",
    "method": "POST",
    "headers": {
      "Content-Type": "application/json",
      "Authorization": "Token token=YOUR_KEY"
    },
    "timeout_seconds": 15
  }
}</pre>
                        </div>
                    `
                },
                {
                    id: 'channels-slack',
                    title: 'Slack',
                    body: `
                        <p>Delivers alerts to a Slack channel using an <strong>Incoming Webhook URL</strong>
                        and Slack Block Kit formatting for rich, readable messages.</p>
                        <h4>Setup Steps</h4>
                        <div class="doc-flow">
                            <div class="flow-step"><span class="flow-num">1</span><span>Go to <em>api.slack.com/apps</em> and create a new Slack App</span></div>
                            <div class="flow-step"><span class="flow-num">2</span><span>Enable <strong>Incoming Webhooks</strong> under Features</span></div>
                            <div class="flow-step"><span class="flow-num">3</span><span>Click <strong>Add New Webhook to Workspace</strong> and choose the target channel</span></div>
                            <div class="flow-step"><span class="flow-num">4</span><span>Copy the Webhook URL and paste it into the channel config below</span></div>
                        </div>
                        <h4>Configuration Fields</h4>
                        <table class="doc-table">
                            <thead><tr><th>Field</th><th>Description</th><th>Example</th></tr></thead>
                            <tbody>
                                <tr><td><code>webhook_url</code></td><td>Slack Incoming Webhook URL</td><td><code>https://hooks.slack.com/services/T.../B.../...</code></td></tr>
                                <tr><td><code>channel</code></td><td>Override target channel (optional)</td><td><code>#ops-alerts</code></td></tr>
                                <tr><td><code>username</code></td><td>Bot display name (optional)</td><td><code>ezHealthKonnect</code></td></tr>
                                <tr><td><code>icon_emoji</code></td><td>Bot emoji icon (optional)</td><td><code>:bell:</code></td></tr>
                            </tbody>
                        </table>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — Slack channel config</div>
<pre>{
  "name": "Slack #ops-alerts",
  "channel_type": "slack",
  "config": {
    "webhook_url": "https://hooks.slack.com/services/T00000/B00000/xxxxxxxx",
    "channel": "#ops-alerts",
    "username": "ezHealthKonnect",
    "icon_emoji": ":bell:"
  }
}</pre>
                        </div>
                    `
                }
            ]
        },

        // ── Silences ────────────────────────────────────────────────────────────
        {
            id: 'silences',
            title: 'Silences',
            subsections: [
                {
                    id: 'silences-overview',
                    title: 'What are Silences?',
                    body: `
                        <p>A silence is a <strong>maintenance window</strong> that suppresses alert firing for a
                        defined time range. During an active silence, the evaluation engine still runs and
                        computes metrics, but no alert rows are inserted and no notifications are sent.</p>
                        <h4>Scope Options</h4>
                        <table class="doc-table">
                            <thead><tr><th>Scope</th><th>Configuration</th><th>Effect</th></tr></thead>
                            <tbody>
                                <tr><td>Global</td><td>Leave Interface and Rule blank</td><td>Suppresses ALL alerts across ALL interfaces and rules</td></tr>
                                <tr><td>Per-Interface</td><td>Set Interface, leave Rule blank</td><td>Suppresses all alerts for a specific interface</td></tr>
                                <tr><td>Per-Rule</td><td>Set Rule, leave Interface blank</td><td>Suppresses one specific rule type across all interfaces</td></tr>
                                <tr><td>Targeted</td><td>Set both Interface and Rule</td><td>Suppresses one rule on one specific interface only</td></tr>
                            </tbody>
                        </table>
                    `
                },
                {
                    id: 'silences-examples',
                    title: 'Silence Examples',
                    body: `
                        <div class="doc-example">
                            <div class="doc-example-label">Example — Weekend maintenance window (global)</div>
<pre>{
  "name": "Weekend Database Maintenance",
  "reason": "Planned DB upgrade — all interfaces offline 02:00–05:00 UTC",
  "interface_id": null,
  "alert_rule_id": null,
  "starts_at": "2026-03-22T02:00:00Z",
  "ends_at":   "2026-03-22T05:00:00Z"
}</pre>
                        </div>
                        <div class="doc-example">
                            <div class="doc-example-label">Example — Suppress inactivity alerts for one interface during vendor maintenance</div>
<pre>{
  "name": "Vendor System Maintenance",
  "reason": "Vendor scheduled maintenance, feed will be paused",
  "interface_id": "762aebb9-0408-4a42-82c5-202f13f28315",
  "alert_rule_id": null,
  "starts_at": "2026-03-25T01:00:00Z",
  "ends_at":   "2026-03-25T06:00:00Z"
}</pre>
                        </div>
                        <div class="doc-tip">
                            <strong>Active silences</strong> are shown with a green dot in the Silences tab.
                            Upcoming silences show with a grey dot.
                            Expired silences are automatically excluded from the active-only filter.
                        </div>
                    `
                }
            ]
        },

        // ── Fired Alerts ────────────────────────────────────────────────────────
        {
            id: 'fired',
            title: 'Managing Fired Alerts',
            subsections: [
                {
                    id: 'fired-lifecycle',
                    title: 'Alert Lifecycle',
                    body: `
                        <p>A fired alert moves through three states:</p>
                        <div class="doc-flow">
                            <div class="flow-step"><span class="flow-num state-open">●</span><span><strong>Open</strong> — threshold exceeded, not yet acknowledged</span></div>
                            <div class="flow-step"><span class="flow-num state-ack">✓</span><span><strong>Acknowledged</strong> — team member has seen it (<code>acknowledged = true</code>)</span></div>
                            <div class="flow-step"><span class="flow-num state-res">✓</span><span><strong>Resolved</strong> — metric returned to normal (auto-resolve) or manually resolved</span></div>
                        </div>
                        <p>The Active Alerts tab shows only open + acknowledged (unresolved) alerts.
                        Resolved alerts appear in the History tab.</p>
                    `
                },
                {
                    id: 'fired-actions',
                    title: 'Acknowledge & Resolve',
                    body: `
                        <h4>Acknowledge</h4>
                        <p>Acknowledging an alert marks it as seen. It remains visible in the active tab until
                        resolved. Use this to signal "someone is investigating".</p>
                        <div class="doc-example">
                            <div class="doc-example-label">API — Acknowledge a single alert</div>
<pre>POST /api/alerts/fired/{alert_id}/acknowledge
Content-Type: application/json

{ "acknowledged_by": "dr.smith@company.com" }</pre>
                        </div>
                        <div class="doc-example">
                            <div class="doc-example-label">API — Bulk acknowledge all active alerts</div>
<pre>POST /api/alerts/fired/acknowledge-all
Content-Type: application/json

{
  "interface_id": "762aebb9-...",
  "acknowledged_by": "ops-team"
}</pre>
                        </div>
                        <h4>Resolve</h4>
                        <p>The evaluation engine <strong>auto-resolves</strong> an open alert when the metric drops
                        back below the threshold. You can also manually resolve an alert that won't clear on its own.</p>
                        <div class="doc-example">
                            <div class="doc-example-label">API — Manual resolve</div>
<pre>POST /api/alerts/fired/{alert_id}/resolve</pre>
                        </div>
                    `
                }
            ]
        },

        // ── API Reference ────────────────────────────────────────────────────────
        {
            id: 'api',
            title: 'API Reference',
            subsections: [
                {
                    id: 'api-rules',
                    title: 'Rules API',
                    body: `
                        <table class="doc-table">
                            <thead><tr><th>Method</th><th>Endpoint</th><th>Description</th></tr></thead>
                            <tbody>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/rules</code></td><td>List all rules. Filter by <code>?interfaceId=</code></td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/rules</code></td><td>Create a new rule</td></tr>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/rules/:id</code></td><td>Get one rule by ID</td></tr>
                                <tr><td><span class="http-put">PUT</span></td><td><code>/api/alerts/rules/:id</code></td><td>Update a rule (returns 404 if missing, 403 if system rule locked)</td></tr>
                                <tr><td><span class="http-del">DELETE</span></td><td><code>/api/alerts/rules/:id</code></td><td>Delete rule (403 for OOB system rules)</td></tr>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/rules/:id/channels</code></td><td>Get assigned channels</td></tr>
                                <tr><td><span class="http-put">PUT</span></td><td><code>/api/alerts/rules/:id/channels</code></td><td>Replace channel assignments</td></tr>
                            </tbody>
                        </table>
                        <div class="doc-example">
                            <div class="doc-example-label">Create rule — minimal payload</div>
<pre>POST /api/alerts/rules
{
  "name": "My Error Rate Rule",
  "rule_type": "ERROR_RATE",
  "condition": "gt",
  "threshold_warning": 5,
  "threshold_critical": 10
}</pre>
                        </div>
                    `
                },
                {
                    id: 'api-channels',
                    title: 'Channels API',
                    body: `
                        <table class="doc-table">
                            <thead><tr><th>Method</th><th>Endpoint</th><th>Description</th></tr></thead>
                            <tbody>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/channels</code></td><td>List all channels</td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/channels</code></td><td>Create channel (<code>name</code>, <code>channel_type</code>, <code>config</code> required)</td></tr>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/channels/:id</code></td><td>Get one channel</td></tr>
                                <tr><td><span class="http-put">PUT</span></td><td><code>/api/alerts/channels/:id</code></td><td>Update channel config</td></tr>
                                <tr><td><span class="http-del">DELETE</span></td><td><code>/api/alerts/channels/:id</code></td><td>Delete channel (cascade-removes assignments)</td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/channels/:id/test</code></td><td>Send test notification, updates <code>test_status</code></td></tr>
                            </tbody>
                        </table>
                        <div class="doc-tip">
                            Valid <code>channel_type</code> values: <code>email</code>, <code>webhook</code>, <code>slack</code>.
                            Any other value returns <code>400 Bad Request</code>.
                        </div>
                    `
                },
                {
                    id: 'api-silences',
                    title: 'Silences & Fired Alerts API',
                    body: `
                        <table class="doc-table">
                            <thead><tr><th>Method</th><th>Endpoint</th><th>Description</th></tr></thead>
                            <tbody>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/silences</code></td><td>List silences. Add <code>?activeOnly=true</code> for current only</td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/silences</code></td><td>Create silence (<code>name</code>, <code>starts_at</code>, <code>ends_at</code> required)</td></tr>
                                <tr><td><span class="http-del">DELETE</span></td><td><code>/api/alerts/silences/:id</code></td><td>Delete silence (404 if not found)</td></tr>
                                <tr><td><span class="http-get">GET</span></td><td><code>/api/alerts/fired</code></td><td>Active alerts. Filter: <code>?interfaceId=</code>, <code>?countOnly=true</code></td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/fired/:id/acknowledge</code></td><td>Acknowledge one alert</td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/fired/:id/resolve</code></td><td>Manually resolve one alert</td></tr>
                                <tr><td><span class="http-post">POST</span></td><td><code>/api/alerts/fired/acknowledge-all</code></td><td>Bulk acknowledge (optional <code>interface_id</code> filter)</td></tr>
                            </tbody>
                        </table>
                    `
                }
            ]
        },

        // ── Examples & Recipes ──────────────────────────────────────────────────
        {
            id: 'examples',
            title: 'Examples & Recipes',
            subsections: [
                {
                    id: 'examples-inbound-feed',
                    title: 'High-Priority Inbound Feed Monitoring',
                    body: `
                        <p>A comprehensive setup for monitoring a critical inbound integration feed with
                        email + Slack notifications and a weekend maintenance silence.</p>
                        <h4>Step 1 — Create per-interface error rate rule</h4>
                        <div class="doc-example">
<pre>POST /api/alerts/rules
{
  "name": "CRM Inbound — Error Rate",
  "rule_type": "ERROR_RATE",
  "interface_id": "762aebb9-0408-4a42-82c5-202f13f28315",
  "condition": "gt",
  "threshold_warning": 3,
  "threshold_critical": 8,
  "evaluation_window_minutes": 30,
  "cooldown_minutes": 60
}</pre>
                        </div>
                        <h4>Step 2 — Create inactivity rule</h4>
                        <div class="doc-example">
<pre>POST /api/alerts/rules
{
  "name": "CRM Inbound — Inactivity",
  "rule_type": "INTERFACE_INACTIVE",
  "interface_id": "762aebb9-0408-4a42-82c5-202f13f28315",
  "condition": "gt",
  "threshold_warning": 30,
  "threshold_critical": 60,
  "evaluation_window_minutes": 120,
  "cooldown_minutes": 120
}</pre>
                        </div>
                        <h4>Step 3 — Create Slack channel and assign</h4>
                        <div class="doc-example">
<pre>POST /api/alerts/channels
{
  "name": "Ops Monitoring Slack",
  "channel_type": "slack",
  "config": {
    "webhook_url": "https://hooks.slack.com/services/...",
    "channel": "#ops-monitoring"
  }
}

// Assign channel to both rules above
PUT /api/alerts/rules/{rule_id}/channels
{ "channels": [{ "channel_id": "{channel_id}", "severity_filter": [] }] }</pre>
                        </div>
                        <h4>Step 4 — Create weekend silence</h4>
                        <div class="doc-example">
<pre>POST /api/alerts/silences
{
  "name": "Vendor Weekend Maintenance",
  "reason": "Planned vendor upgrade window",
  "interface_id": "762aebb9-0408-4a42-82c5-202f13f28315",
  "starts_at": "2026-03-28T22:00:00Z",
  "ends_at":   "2026-03-29T06:00:00Z"
}</pre>
                        </div>
                    `
                },
                {
                    id: 'examples-pagerduty',
                    title: 'PagerDuty Integration',
                    body: `
                        <p>Route critical alerts only to PagerDuty while sending all severities to Slack.</p>
                        <div class="doc-example">
                            <div class="doc-example-label">PagerDuty webhook channel</div>
<pre>POST /api/alerts/channels
{
  "name": "PagerDuty Critical",
  "channel_type": "webhook",
  "config": {
    "url": "https://events.pagerduty.com/v2/enqueue",
    "method": "POST",
    "headers": {
      "Content-Type": "application/json"
    },
    "timeout_seconds": 15
  }
}</pre>
                        </div>
                        <div class="doc-example">
                            <div class="doc-example-label">Assign with severity_filter — critical only</div>
<pre>PUT /api/alerts/rules/{rule_id}/channels
{
  "channels": [
    {
      "channel_id": "{pagerduty_channel_id}",
      "severity_filter": ["critical"]
    },
    {
      "channel_id": "{slack_channel_id}",
      "severity_filter": []
    }
  ]
}</pre>
                        </div>
                        <p>Setting <code>severity_filter: ["critical"]</code> means the PagerDuty channel only
                        receives notifications when the severity is critical. The empty array on Slack means
                        it receives all severities.</p>
                    `
                },
                {
                    id: 'examples-throughput',
                    title: 'Low Throughput Detection',
                    body: `
                        <p>Detect when message volume drops — useful for interfaces where silence means
                        an upstream system has stopped sending data.</p>
                        <div class="doc-tip">
                            <strong>Important:</strong> Use condition <code>lt</code> (less than) for THROUGHPUT_LOW
                            rules, not <code>gt</code>. You want to fire when throughput <em>drops below</em> the threshold.
                        </div>
                        <div class="doc-example">
<pre>POST /api/alerts/rules
{
  "name": "Inbound Feed — Low Throughput",
  "rule_type": "THROUGHPUT_LOW",
  "condition": "lt",
  "threshold_warning": 10,
  "threshold_critical": 2,
  "evaluation_window_minutes": 60,
  "cooldown_minutes": 120,
  "description": "Messages should arrive at > 10/min during business hours"
}</pre>
                        </div>
                        <div class="doc-warning">
                            Combine with a <strong>silence</strong> covering overnight hours to avoid
                            false-positive low-throughput alerts when the upstream system is legitimately quiet.
                        </div>
                    `
                }
            ]
        },

        // ── FAQ ─────────────────────────────────────────────────────────────────
        {
            id: 'faq',
            title: 'FAQ & Troubleshooting',
            subsections: [
                {
                    id: 'faq-questions',
                    title: 'Frequently Asked Questions',
                    body: `
                        <div class="faq-item">
                            <div class="faq-q">Why is my rule not firing even though the metric is above the threshold?</div>
                            <div class="faq-a">
                                Check three things: (1) Is the rule <strong>enabled</strong>? The toggle on the rule card must be on.
                                (2) Is there an <strong>active silence</strong> that covers this interface or rule?
                                (3) Is the alert still within the <strong>cooldown window</strong> from a previous fire?
                                Check the History tab to see recent fired alerts for this rule.
                            </div>
                        </div>
                        <div class="faq-item">
                            <div class="faq-q">Can I delete an OOB system rule?</div>
                            <div class="faq-a">
                                No. OOB system rules (marked with <code>is_system = true</code>) return a
                                <code>403 Forbidden</code> on DELETE attempts. You can <strong>disable</strong> them
                                using the toggle, or create a per-interface rule to override the threshold.
                            </div>
                        </div>
                        <div class="faq-item">
                            <div class="faq-q">How do I test a notification channel without triggering a real alert?</div>
                            <div class="faq-a">
                                Use the <strong>Test</strong> button on the channel card in the Channels tab.
                                It sends a sample payload to the configured destination and updates the
                                <code>test_status</code> field to <code>success</code> or <code>failure</code>.
                            </div>
                        </div>
                        <div class="faq-item">
                            <div class="faq-q">What happens if my webhook endpoint is unreachable?</div>
                            <div class="faq-a">
                                The notification service retries <strong>3 times</strong> with a 2-second backoff.
                                If all attempts fail, the error is logged to the alert's <code>context_data</code>
                                field. The alert is still created in the database — only the notification delivery fails.
                            </div>
                        </div>
                        <div class="faq-item">
                            <div class="faq-q">My THROUGHPUT_LOW rule fires during overnight hours when nothing is sending.</div>
                            <div class="faq-a">
                                Create a <strong>recurring silence</strong> that covers the quiet hours. For example,
                                set <code>starts_at</code> to <code>22:00 UTC</code> and <code>ends_at</code> to
                                <code>06:00 UTC</code> the next day. Recreate it each week or use the API to create
                                it programmatically as part of your deployment pipeline.
                            </div>
                        </div>
                        <div class="faq-item">
                            <div class="faq-q">Can I have multiple channels for the same rule?</div>
                            <div class="faq-a">
                                Yes. A rule can have any number of channels assigned. All assigned channels
                                receive the notification concurrently. Use <code>severity_filter</code> to
                                route different severities to different destinations (e.g. warning → Slack,
                                critical → Slack + PagerDuty).
                            </div>
                        </div>
                    `
                }
            ]
        }
    ]
};
