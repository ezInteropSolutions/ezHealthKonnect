/**
 * user-management-help.js
 * In-app contextual help panel for the User Management page.
 *
 * Pattern: slide-in right drawer (480 px), tabbed content, collapsible sections.
 * Wired to a single "?" button in the toolbar.
 */

(function () {
    'use strict';

    // ── Content ────────────────────────────────────────────────────────────────
    const HELP_TABS = [
        { id: 'quick-start', icon: 'fa-rocket',          label: 'Quick Start' },
        { id: 'workflows',   icon: 'fa-diagram-project',  label: 'Workflows' },
        { id: 'security',    icon: 'fa-shield-halved',    label: 'Security' },
        { id: 'compliance',  icon: 'fa-scale-balanced',   label: 'Compliance' },
        { id: 'audit',       icon: 'fa-clock-rotate-left', label: 'Audit Logs' },
        { id: 'roles',       icon: 'fa-lock',             label: 'Roles' },
        { id: 'faq',         icon: 'fa-circle-question',  label: 'FAQ' },
        { id: 'glossary',    icon: 'fa-book-open',        label: 'Glossary' },
    ];

    const HELP_CONTENT = {

        'quick-start': `
<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-circle-play"></i> Get Started in 3 Steps
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <ol class="hp-steps">
      <li>
        <strong>Create or Invite a user</strong><br>
        Use <em>Create User</em> to add a user with a password immediately, or
        <em>Invite User</em> to send a self-service activation link valid for 48 hours.
      </li>
      <li>
        <strong>Assign the correct role</strong><br>
        Choose from <span class="hp-badge hp-badge--admin">Admin</span>,
        <span class="hp-badge hp-badge--operator">Operator</span>,
        <span class="hp-badge hp-badge--viewer">Viewer</span>, or
        <span class="hp-badge hp-badge--user">User</span>.
        See the <strong>Roles &amp; Permissions</strong> tab for a full capability matrix.
      </li>
      <li>
        <strong>Monitor &amp; audit</strong><br>
        Open any user's row to see their <em>Activity</em> timeline, manage
        <em>Security</em> settings, and review <em>Compliance</em> flags —
        all from the slide-in drawer.
      </li>
    </ol>
  </div>
</div>

<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-table-columns"></i> Page Layout
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead><tr><th>Element</th><th>What it does</th><th>Help tab</th></tr></thead>
      <tbody>
        <tr><td>Stats bar (top right)</td><td>Live counts: Total · Active · Inactive · Locked · Admins</td><td>—</td></tr>
        <tr><td><strong>Users</strong> tab</td><td>Search, filter, sort, paginate and manage all users</td><td>Workflows</td></tr>
        <tr><td><strong>Audit Logs</strong> tab</td><td>Filterable, exportable record of every user action. Click <strong>Apply</strong> to load entries.</td><td>Audit Logs</td></tr>
        <tr><td><strong>Roles &amp; Permissions</strong> tab</td><td>Read-only capability matrix for each role</td><td>Roles</td></tr>
        <tr><td>User drawer → Profile</td><td>Edit name, email, phone, department, title, timezone</td><td>Workflows</td></tr>
        <tr><td><strong>Role column dropdown</strong></td><td>Click the role badge in any row to instantly change the user's role</td><td>Roles</td></tr>
        <tr><td>User drawer → Security</td><td>Change role/status, set password, unlock, force reset</td><td>Security</td></tr>
        <tr><td>User drawer → Activity</td><td>Last 50 audit events for this specific user</td><td>Audit Logs</td></tr>
        <tr><td>User drawer → Compliance</td><td>GDPR consent, retention date, anonymization, erasure request</td><td>Compliance</td></tr>
      </tbody>
    </table>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-keyboard"></i> Keyboard Shortcuts
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead><tr><th>Key</th><th>Action</th></tr></thead>
      <tbody>
        <tr><td><kbd>Esc</kbd></td><td>Close the user drawer or any open modal</td></tr>
        <tr><td><kbd>?</kbd></td><td>Toggle this help panel</td></tr>
        <tr><td><kbd>Ctrl + F</kbd></td><td>Focus the search box</td></tr>
      </tbody>
    </table>
  </div>
</div>`,

        'workflows': `
<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-user-plus"></i> Inviting a New User
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <ol class="hp-steps">
      <li>Click <strong>Invite User</strong> in the toolbar.</li>
      <li>Enter the user's email address and select their role.</li>
      <li>Click <strong>Generate Invite</strong>. A 48-hour link is created.</li>
      <li>Copy the link and share it with the user (email, Slack, etc.).</li>
      <li>The user visits the link, sets their name and password, and activates their account.</li>
      <li>The user's status changes from <em>Pending</em> to <em>Active</em> automatically.</li>
    </ol>
    <div class="hp-callout hp-callout--info">
      <i class="fas fa-circle-info"></i>
      Invite links expire after <strong>48 hours</strong>. If it expires, send a new invitation
      from the same menu — the pending user record is reused.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-user-pen"></i> Editing a User Profile
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <ol class="hp-steps">
      <li>Click anywhere on a user's row to open the <strong>User Drawer</strong>.</li>
      <li>The <strong>Profile</strong> tab is selected by default. Edit name, email, phone, department, job title, timezone, or locale.</li>
      <li>Click <strong>Save Profile</strong>. Changes take effect immediately.</li>
    </ol>
    <div class="hp-callout hp-callout--warn">
      <i class="fas fa-triangle-exclamation"></i>
      Changing a user's email address updates their login credential. Notify the user.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-users-gear"></i> Bulk Operations
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <ol class="hp-steps">
      <li>Select one or more users using the checkboxes in the left column.</li>
      <li>The <strong>Bulk Action</strong> toolbar appears at the top of the table.</li>
      <li>Choose <strong>Activate</strong>, <strong>Deactivate</strong>, or <strong>Delete</strong>.</li>
      <li>Confirm the action in the dialog. A success/error banner appears when complete.</li>
    </ol>
    <div class="hp-callout hp-callout--info">
      <i class="fas fa-circle-info"></i>
      You cannot bulk-delete your own account. Self-protection is enforced automatically.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-magnifying-glass"></i> Searching &amp; Filtering
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead><tr><th>Control</th><th>Filters by</th></tr></thead>
      <tbody>
        <tr><td>Search box</td><td>First name, last name, or email (live, as you type)</td></tr>
        <tr><td>Role dropdown</td><td>Admin · Operator · Viewer · User</td></tr>
        <tr><td>Status dropdown</td><td>Active · Inactive · Suspended · Pending</td></tr>
        <tr><td>Column headers</td><td>Click to sort ascending; click again to sort descending</td></tr>
      </tbody>
    </table>
  </div>
</div>`,

        'security': `
<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-lock"></i> Account Lockout
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>An account is automatically locked after <strong>5 consecutive failed login attempts</strong>.
    The lock lasts <strong>15 minutes</strong> and protects against brute-force attacks.</p>
    <p style="margin-top:8px"><strong>To unlock manually:</strong></p>
    <ol class="hp-steps">
      <li>Open the user's drawer.</li>
      <li>Click the <strong>Security</strong> tab.</li>
      <li>The <em>Locked Until</em> field shows the expiry time.</li>
      <li>Click <strong>Unlock</strong> to clear the lockout immediately.</li>
    </ol>
    <div class="hp-callout hp-callout--warn">
      <i class="fas fa-triangle-exclamation"></i>
      Only unlock accounts for users you have verified through an out-of-band channel (phone, badge, HR ticket).
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-key"></i> Force Password Reset
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>Use <strong>Force Reset</strong> in the Security tab to require the user to change their
    password on their next login. This is appropriate when:</p>
    <ul class="hp-list">
      <li>You suspect a compromised credential.</li>
      <li>A user shares a screen and their password may have been observed.</li>
      <li>Regular password-rotation policy requires it.</li>
    </ul>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-user-slash"></i> Suspending vs Deactivating
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead><tr><th>Status</th><th>Can login?</th><th>Data retained?</th><th>Use case</th></tr></thead>
      <tbody>
        <tr><td><span class="hp-badge hp-badge--active">Active</span></td><td>Yes</td><td>Yes</td><td>Normal operation</td></tr>
        <tr><td><span class="hp-badge hp-badge--inactive">Inactive</span></td><td>No</td><td>Yes</td><td>Leave of absence, temporary hold</td></tr>
        <tr><td><span class="hp-badge hp-badge--suspended">Suspended</span></td><td>No</td><td>Yes</td><td>Security incident, policy violation</td></tr>
        <tr><td><span class="hp-badge hp-badge--pending">Pending</span></td><td>No</td><td>Yes</td><td>Invite sent, not yet accepted</td></tr>
      </tbody>
    </table>
    <div class="hp-callout hp-callout--warn">
      <i class="fas fa-triangle-exclamation"></i>
      <strong>Delete</strong> is permanent and removes the user record. The audit trail is preserved
      (entries become anonymous). Prefer <em>Inactive</em> or <em>Suspended</em> for offboarding.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-shield-halved"></i> Role Security Model
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>Roles are enforced <strong>server-side</strong> on every API request — the UI reflects
    them but cannot override them. The principle of least privilege applies:</p>
    <ul class="hp-list">
      <li><strong>Admin</strong> — Full platform access. Grant sparingly.</li>
      <li><strong>Operator</strong> — Can manage interfaces and pipelines; no user management.</li>
      <li><strong>Viewer</strong> — Read-only across most features. Safe for auditors, observers.</li>
      <li><strong>User</strong> — HL7 Reader only. No operational access.</li>
    </ul>
  </div>
</div>`,

        'compliance': `
<div class="hp-callout hp-callout--info" style="margin:0 0 16px 0">
  <i class="fas fa-circle-info"></i>
  ezHealthKonnect stores and processes Protected Health Information (PHI). The compliance
  features below help satisfy HIPAA Security Rule and GDPR Article 17 requirements.
</div>

<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-check-circle"></i> Data Consent
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>The <strong>Data Consent Given</strong> toggle records whether the user has consented to
    data processing under your organization's privacy policy.</p>
    <ul class="hp-list">
      <li>Set the <strong>Consent Date</strong> to the date the consent was obtained.</li>
      <li>This date is immutable once recorded — it is the legal timestamp of consent.</li>
      <li>Required for GDPR Article 6 lawful basis documentation.</li>
    </ul>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-calendar-check"></i> Data Retention
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>Set <strong>Data Retention Until</strong> to define when this user's data should be
    reviewed for deletion or anonymization.</p>
    <ul class="hp-list">
      <li>Typical HIPAA retention: <strong>6 years</strong> from the date of creation.</li>
      <li>Typical GDPR retention: defined by your Data Protection Officer.</li>
      <li>This field is <strong>advisory</strong> — use the Audit Logs CSV export filtered by date to identify users whose retention window has passed. Automated purge is a future platform feature.</li>
    </ul>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-user-slash"></i> Data Anonymization
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>The <strong>Data Anonymized</strong> toggle marks that this user's PII has been
    anonymized in the system. Once set:</p>
    <ul class="hp-list">
      <li>The flag is visible to all admins reviewing the user record.</li>
      <li>Downstream pipeline steps configured for anonymization will use this flag.</li>
      <li>The audit trail entry for the action is preserved for compliance proof.</li>
    </ul>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-user-xmark"></i> GDPR Right to Erasure
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>When a user exercises their <strong>right to erasure</strong> (GDPR Article 17),
    use <em>Request GDPR Deletion</em> in the Compliance tab:</p>
    <ol class="hp-steps">
      <li>Click <strong>Request GDPR Deletion</strong> in the drawer's Compliance tab.</li>
      <li>The request timestamp is recorded and shown in the <em>GDPR Request Date</em> field.</li>
      <li>The action appears in the Audit Logs tab (action: <code>GDPR_DELETION_REQUESTED</code>, risk: Critical) — use this as your DPO notification trigger.</li>
      <li>After verification, set <em>Data Anonymized</em> to ON and/or delete the account.</li>
    </ol>
    <div class="hp-callout hp-callout--warn">
      <i class="fas fa-triangle-exclamation"></i>
      GDPR erasure requests must be fulfilled within <strong>30 days</strong> under Article 12(3).
      Track open requests in the Audit Logs tab filtered by action <code>GDPR_REQUEST</code>.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-file-shield"></i> HIPAA Audit Requirements
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>The <strong>Audit Logs</strong> tab provides the evidence required by HIPAA 45 CFR §164.312(b)
    for information system activity review:</p>
    <ul class="hp-list">
      <li>Every login, profile change, role assignment, and deletion is logged.</li>
      <li>Risk levels (Low · Medium · High · Critical) flag sensitive actions.</li>
      <li>Logs are immutable — they cannot be edited or deleted, even by admins.</li>
      <li>Export CSV for your compliance officer or external audit.</li>
    </ul>
  </div>
</div>`,

        'audit': `
<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-table-list"></i> What is the Audit Logs Tab?
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>The <strong>Audit Logs</strong> tab is a real-time, immutable record of every action
    performed in the system. It supports HIPAA 45 CFR §164.312(b) information system activity
    review requirements.</p>
    <p style="margin-top:8px">Each entry records:</p>
    <ul class="hp-list">
      <li><strong>Timestamp</strong> — exact date and time of the action</li>
      <li><strong>User</strong> — who performed the action (email)</li>
      <li><strong>Action</strong> — what was done (e.g. USER_CREATED, LOGIN_FAILED)</li>
      <li><strong>Entity</strong> — what was affected (e.g. User, Interface)</li>
      <li><strong>IP Address</strong> — originating network address</li>
      <li><strong>Result</strong> — success or failure</li>
      <li><strong>Risk Level</strong> — Low / Medium / High / Critical</li>
    </ul>
  </div>
</div>

<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-filter"></i> Filtering Audit Logs
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead><tr><th>Filter</th><th>What it does</th></tr></thead>
      <tbody>
        <tr><td>Action keyword</td><td>Free-text search on the action field (e.g. type "LOGIN" to see all login events)</td></tr>
        <tr><td>Risk Level</td><td>Filter to Low · Medium · High · Critical entries only</td></tr>
        <tr><td>From / To dates</td><td>Narrow results to a date range (inclusive)</td></tr>
      </tbody>
    </table>
    <p style="margin-top:10px">Click <strong>Apply</strong> after setting filters. Pagination
    controls appear below the table when results exceed one page.</p>
    <div class="hp-callout hp-callout--info">
      <i class="fas fa-circle-info"></i>
      Audit logs cannot be edited or deleted — even by Admins. This is by design to
      satisfy HIPAA and SOC 2 immutability requirements.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-download"></i> Exporting Audit Logs (CSV)
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <ol class="hp-steps">
      <li>Apply any filters you need (date range, risk level, action keyword).</li>
      <li>Click <strong>Export CSV</strong> in the top-right of the Audit Logs toolbar.</li>
      <li>The browser downloads a <code>.csv</code> file with all matching records.</li>
    </ol>
    <p style="margin-top:8px">Use the CSV export for:</p>
    <ul class="hp-list">
      <li>Quarterly HIPAA audit reviews</li>
      <li>Sharing with an external auditor or compliance officer</li>
      <li>Building compliance reports in Excel or Power BI</li>
    </ul>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-user-clock"></i> Per-User Activity (User Drawer)
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>For a single user's history, use the <strong>Activity</strong> tab inside the user
    drawer (click any row → Activity tab) rather than the global Audit Logs tab.
    It shows the last 50 events for that specific user in a timeline view.</p>
    <div class="hp-callout hp-callout--info">
      <i class="fas fa-circle-info"></i>
      If a user is deleted, their audit log entries are preserved but anonymized
      (the user link is removed). The audit trail is never deleted.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-circle-exclamation"></i> Risk Level Reference
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead><tr><th>Level</th><th>Typical Actions</th></tr></thead>
      <tbody>
        <tr><td><span class="hp-badge hp-badge--low">Low</span></td><td>Page views, user list queries, read-only access</td></tr>
        <tr><td><span class="hp-badge hp-badge--medium">Medium</span></td><td>Profile updates, role changes, interface saves</td></tr>
        <tr><td><span class="hp-badge hp-badge--high">High</span></td><td>User creation/deletion, bulk operations, password resets</td></tr>
        <tr><td><span class="hp-badge hp-badge--critical">Critical</span></td><td>Login failures, account lockouts, GDPR requests, admin changes</td></tr>
      </tbody>
    </table>
  </div>
</div>`,

        'roles': `
<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-lock"></i> How Roles Work
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>The <strong>Roles &amp; Permissions</strong> tab shows a read-only capability matrix.
    Roles are enforced <strong>server-side on every API request</strong> — the UI reflects
    them but cannot override the server check. Changing a user's role in the Security drawer
    takes effect immediately on their next request.</p>
    <div class="hp-callout hp-callout--info">
      <i class="fas fa-circle-info"></i>
      There is no way to grant a partial role (e.g. "Viewer who can also edit one interface").
      Role assignments are all-or-nothing per role definition.
    </div>
  </div>
</div>

<div class="hp-section hp-section--open">
  <button class="hp-section-toggle">
    <i class="fas fa-table"></i> Role Capability Summary
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <table class="hp-table">
      <thead>
        <tr>
          <th>Role</th>
          <th>Best for</th>
          <th>Key restrictions</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td><span class="hp-badge hp-badge--admin">Admin</span></td>
          <td>Platform owners, IT managers</td>
          <td>None — full access. Grant sparingly.</td>
        </tr>
        <tr>
          <td><span class="hp-badge hp-badge--operator">Operator</span></td>
          <td>Integration engineers, HL7 developers</td>
          <td>No user management, no system settings, audit logs read-only</td>
        </tr>
        <tr>
          <td><span class="hp-badge hp-badge--viewer">Viewer</span></td>
          <td>Auditors, clinical observers, external reviewers</td>
          <td>Read-only everywhere; cannot run pipelines or send messages</td>
        </tr>
        <tr>
          <td><span class="hp-badge hp-badge--user">User</span></td>
          <td>Basic access (HL7 Reader only)</td>
          <td>Cannot access interfaces, pipelines, reports, or admin features</td>
        </tr>
      </tbody>
    </table>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-user-pen"></i> Changing a User's Role
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p><strong>Quick way — inline dropdown (recommended):</strong></p>
    <ol class="hp-steps">
      <li>Find the user in the table.</li>
      <li>Click the <em>role badge</em> in the <strong>Role</strong> column — it is a dropdown.</li>
      <li>Select a new role. The change saves instantly with no extra click.</li>
    </ol>
    <p style="margin-top:10px"><strong>Full way — Security tab:</strong></p>
    <ol class="hp-steps">
      <li>Click the user's name to open the <strong>User Drawer</strong>.</li>
      <li>Click the <strong>Security</strong> tab.</li>
      <li>Select the new role from the <em>Role</em> dropdown and click <strong>Save Security</strong>.</li>
    </ol>
    <div class="hp-callout hp-callout--warn">
      <i class="fas fa-triangle-exclamation"></i>
      Promoting a user to <strong>Admin</strong> gives them full platform access including
      the ability to manage other admins. This action is logged as a Critical risk event.
    </div>
  </div>
</div>

<div class="hp-section">
  <button class="hp-section-toggle">
    <i class="fas fa-shield-check"></i> Principle of Least Privilege
    <i class="fas fa-chevron-down hp-chevron"></i>
  </button>
  <div class="hp-section-body">
    <p>Assign the <em>minimum role</em> required for a user to do their job:</p>
    <ul class="hp-list">
      <li>Clinical staff who only need HL7 message reading → <strong>User</strong></li>
      <li>External auditors reviewing interface logs → <strong>Viewer</strong></li>
      <li>HL7/FHIR developers building pipelines → <strong>Operator</strong></li>
      <li>IT platform administrators → <strong>Admin</strong></li>
    </ul>
    <p style="margin-top:8px">Start with a lower role and promote if needed — it's easier to
    grant access than to explain a security incident.</p>
  </div>
</div>`,

        'faq': `
<div class="hp-faq">

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> Why can't I delete my own account?</button>
    <div class="hp-faq-a">
      Self-deletion is blocked to prevent accidental loss of the last admin account.
      Ask another admin to delete your account, or deactivate it first.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> The invite link says "expired" — what do I do?</button>
    <div class="hp-faq-a">
      Invite links are valid for 48 hours. Click <strong>Invite User</strong> again and enter the
      same email — a new link is generated and the pending record is reused.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> How do I change a user's password?</button>
    <div class="hp-faq-a">
      Open the user drawer → Security tab → enter a new password in the <em>New Password</em>
      field → Save. Leave it blank to keep the existing password unchanged.
      Alternatively, use <strong>Force Reset</strong> to prompt the user to choose their own.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> What's the difference between Create User and Invite User?</button>
    <div class="hp-faq-a">
      <strong>Create User</strong>: You set the password. The account is active immediately.
      Use this for service accounts or when onboarding on behalf of a colleague.<br><br>
      <strong>Invite User</strong>: The user sets their own password via a secure link.
      Use this for self-service onboarding — best practice for compliance.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> Why do I see "—" in the Last Login column?</button>
    <div class="hp-faq-a">
      The user has never logged in. This is normal for newly created or invited accounts
      that haven't been activated yet.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> The Activity tab shows no events for a user — is that normal?</button>
    <div class="hp-faq-a">
      Yes. New users with no recorded actions will show an empty activity list.
      Events are created on login, profile changes, and any audited action.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> Can I export the full user list?</button>
    <div class="hp-faq-a">
      Not directly from the Users tab (coming in a future release).
      Use the <strong>Audit Logs</strong> tab → <strong>Export CSV</strong> to export audit activity.
      For a full user data export, contact your database administrator.
    </div>
  </div>

  <div class="hp-faq-item">
    <button class="hp-faq-q"><i class="fas fa-circle-question"></i> How do I find locked accounts quickly?</button>
    <div class="hp-faq-a">
      Use the <strong>Status</strong> filter — there is no dedicated "Locked" status filter since
      lockout is a transient security state, not a permanent status.
      The <strong>Locked</strong> counter in the stats bar shows the current count.
      Search by email if you need to find a specific locked user.
    </div>
  </div>

</div>`,

        'glossary': `
<div class="hp-glossary">

  <dl class="hp-dl">
    <dt>Account Lockout</dt>
    <dd>Automatic temporary block applied after 5 consecutive failed login attempts.
        Lasts 15 minutes or until manually cleared by an Admin.</dd>

    <dt>Audit Log</dt>
    <dd>Immutable record of every user action in the system, including timestamp,
        IP address, action type, affected entity, result, and risk level.</dd>

    <dt>Data Anonymization</dt>
    <dd>Process of removing or obfuscating personally identifiable information (PII)
        so the data can no longer be linked to a specific individual.</dd>

    <dt>Data Consent</dt>
    <dd>Recorded confirmation that a user has agreed to how their personal data
        will be processed, as required by GDPR Article 6.</dd>

    <dt>Data Retention Policy</dt>
    <dd>The date after which a user's data should be reviewed for deletion or
        anonymization in compliance with HIPAA or GDPR requirements.</dd>

    <dt>Force Password Reset</dt>
    <dd>A flag that requires a user to change their password on next login.
        Applied by an Admin when credential security is in question.</dd>

    <dt>GDPR</dt>
    <dd>General Data Protection Regulation — EU law governing the collection and
        processing of personal data for individuals in the European Union.</dd>

    <dt>GDPR Right to Erasure</dt>
    <dd>An individual's right under GDPR Article 17 to request deletion of their
        personal data. Must be fulfilled within 30 days.</dd>

    <dt>HIPAA</dt>
    <dd>Health Insurance Portability and Accountability Act — US law that sets
        standards for protecting sensitive patient health information (PHI).</dd>

    <dt>Invitation Token</dt>
    <dd>A cryptographically random, single-use token embedded in an invite link.
        Valid for 48 hours; invalidated upon account activation.</dd>

    <dt>Operator</dt>
    <dd>A role with full access to interface and pipeline management but no ability
        to manage users, view audit logs in full, or change system settings.</dd>

    <dt>PHI</dt>
    <dd>Protected Health Information — any individually identifiable health information
        covered under HIPAA.</dd>

    <dt>Risk Level</dt>
    <dd>A classification applied to audit log entries:
        <span class="hp-badge hp-badge--low">Low</span> routine actions,
        <span class="hp-badge hp-badge--medium">Medium</span> notable changes,
        <span class="hp-badge hp-badge--high">High</span> sensitive operations,
        <span class="hp-badge hp-badge--critical">Critical</span> security events.</dd>

    <dt>Role</dt>
    <dd>A named permission set (Admin, Operator, Viewer, User) assigned to a user
        and enforced server-side on every API request.</dd>

    <dt>Session</dt>
    <dd>An authenticated browser context. Sessions expire on logout or after
        inactivity (configurable in system settings).</dd>

    <dt>Viewer</dt>
    <dd>A read-only role suitable for auditors and observers who need visibility
        but must not make changes to interfaces, pipelines, or users.</dd>
  </dl>

</div>`
    };

    // ── Render ─────────────────────────────────────────────────────────────────
    function buildPanel() {
        const overlay = document.createElement('div');
        overlay.id = 'um-help-overlay';
        overlay.className = 'um-help-overlay';
        overlay.setAttribute('aria-hidden', 'true');

        const panel = document.createElement('aside');
        panel.id = 'um-help-panel';
        panel.className = 'um-help-panel';
        panel.setAttribute('role', 'complementary');
        panel.setAttribute('aria-label', 'User Management Help');

        // Header
        const header = document.createElement('div');
        header.className = 'um-help-header';
        header.innerHTML = `
            <div class="um-help-title">
                <i class="fas fa-circle-question"></i>
                <span>User Management Help</span>
            </div>
            <button class="um-help-close" id="um-help-close" title="Close help (Esc)">
                <i class="fas fa-xmark"></i>
            </button>`;

        // Tab bar
        const tabBar = document.createElement('div');
        tabBar.className = 'um-help-tabs';
        tabBar.innerHTML = HELP_TABS.map((t, i) => `
            <button class="um-help-tab${i === 0 ? ' active' : ''}" data-htab="${t.id}">
                <i class="fas ${t.icon}"></i>
                <span>${t.label}</span>
            </button>`).join('');

        // Body
        const body = document.createElement('div');
        body.className = 'um-help-body';

        HELP_TABS.forEach((t, i) => {
            const pane = document.createElement('div');
            pane.id = `htab-${t.id}`;
            pane.className = `um-help-pane${i === 0 ? ' active' : ''}`;
            pane.innerHTML = HELP_CONTENT[t.id] || '<p>Coming soon.</p>';
            body.appendChild(pane);
        });

        panel.appendChild(header);
        panel.appendChild(tabBar);
        panel.appendChild(body);

        document.body.appendChild(overlay);
        document.body.appendChild(panel);
    }

    // ── Open / Close ───────────────────────────────────────────────────────────
    function open(tabId) {
        const panel   = document.getElementById('um-help-panel');
        const overlay = document.getElementById('um-help-overlay');
        if (!panel) return;
        panel.classList.add('um-help-panel--open');
        overlay.style.display = 'block';
        overlay.setAttribute('aria-hidden', 'false');
        if (tabId) switchTab(tabId);
        panel.querySelector('#um-help-close').focus();
    }

    function close() {
        const panel   = document.getElementById('um-help-panel');
        const overlay = document.getElementById('um-help-overlay');
        if (!panel) return;
        panel.classList.remove('um-help-panel--open');
        overlay.style.display = 'none';
        overlay.setAttribute('aria-hidden', 'true');
        const trigger = document.getElementById('btn-help');
        if (trigger) trigger.focus();
    }

    function isOpen() {
        return document.getElementById('um-help-panel')?.classList.contains('um-help-panel--open');
    }

    function switchTab(tabId) {
        document.querySelectorAll('.um-help-tab').forEach(t => t.classList.toggle('active', t.dataset.htab === tabId));
        document.querySelectorAll('.um-help-pane').forEach(p => p.classList.toggle('active', p.id === `htab-${tabId}`));
    }

    // ── Collapsible sections ───────────────────────────────────────────────────
    function attachSectionToggles(root) {
        root.querySelectorAll('.hp-section-toggle').forEach(btn => {
            btn.addEventListener('click', () => {
                const section = btn.closest('.hp-section');
                section.classList.toggle('hp-section--open');
            });
        });
    }

    function attachFaqToggles(root) {
        root.querySelectorAll('.hp-faq-q').forEach(btn => {
            btn.addEventListener('click', () => {
                const item = btn.closest('.hp-faq-item');
                item.classList.toggle('hp-faq-item--open');
            });
        });
    }

    // ── Wire events ────────────────────────────────────────────────────────────
    function wireEvents() {
        const panel   = document.getElementById('um-help-panel');
        const overlay = document.getElementById('um-help-overlay');

        // Close button
        document.getElementById('um-help-close').addEventListener('click', close);

        // Overlay click to close
        overlay.addEventListener('click', close);

        // Tab switching
        panel.querySelector('.um-help-tabs').addEventListener('click', e => {
            const btn = e.target.closest('[data-htab]');
            if (btn) switchTab(btn.dataset.htab);
        });

        // Collapsible sections & FAQ (delegated once after render)
        attachSectionToggles(panel);
        attachFaqToggles(panel);

        // Help trigger button
        const triggerBtn = document.getElementById('btn-help');
        if (triggerBtn) {
            triggerBtn.addEventListener('click', () => isOpen() ? close() : open());
        }

        // Keyboard: Esc closes, ? toggles, Ctrl+F focuses search
        document.addEventListener('keydown', e => {
            if (e.key === 'Escape' && isOpen()) { e.stopPropagation(); close(); return; }

            if (e.key === '?' && !e.ctrlKey && !e.metaKey) {
                const active = document.activeElement;
                const isInput = active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA' || active.tagName === 'SELECT' || active.isContentEditable);
                if (!isInput) { e.preventDefault(); isOpen() ? close() : open(); }
                return;
            }

            // Ctrl+F — focus the user search box (only when search box is on screen)
            if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
                const search = document.getElementById('um-search');
                if (search && search.offsetParent !== null) {
                    e.preventDefault();
                    search.focus();
                    search.select();
                }
            }
        });
    }

    // ── Public API (for contextual help links) ─────────────────────────────────
    window.umHelp = { open, close, switchTab };

    // ── Init ───────────────────────────────────────────────────────────────────
    function init() {
        buildPanel();
        wireEvents();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
