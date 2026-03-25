# User Management — Quality Assurance Use Cases
**System:** ezHealthKonnect Enterprise User Management
**Version:** 1.0 | **Date:** 2026-03-22
**Coverage:** Functional · Business · User · Technical · Security Workflows

---

## Index

| Category | ID Range | Count |
|---|---|---|
| Authentication & Account Lockout | UC-AUTH-001–005 | 5 |
| User Onboarding & Invitation | UC-ONB-001–005 | 5 |
| User Profile Management | UC-PROF-001–005 | 5 |
| Role-Based Access Control | UC-RBAC-001–005 | 5 |
| Bulk Operations | UC-BULK-001–004 | 4 |
| HIPAA / GDPR Compliance | UC-COMP-001–005 | 5 |
| Audit Log & Activity Monitoring | UC-AUDIT-001–005 | 5 |
| Search, Filter & Sort | UC-SRCH-001–004 | 4 |
| User Offboarding | UC-OFF-001–003 | 3 |
| Technical / API Workflows | UC-API-001–005 | 5 |
| **Total** | | **46** |

---

## Legend

| Field | Description |
|---|---|
| **Priority** | P1 = Critical · P2 = High · P3 = Medium · P4 = Low |
| **Actor** | Who initiates the use case |
| **Preconditions** | State required before execution |
| **Main Flow** | Happy-path steps |
| **Alt Flow** | Divergence from main flow |
| **Exception Flow** | Error / edge-case behaviour |
| **Verification** | How to assert expected result |

---

## UC-AUTH — Authentication & Account Lockout

---

### UC-AUTH-001 · Successful Admin Login
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
All users must authenticate before accessing patient integration data. Healthcare regulations (HIPAA §164.312) require audit trails for all access events. Every successful login must be recorded.

**Preconditions:**
- User account exists with status = `active`
- Password is correct
- Account is not locked (`locked_until` is null or past)

**Main Flow:**
1. User navigates to `http://localhost:3000/`
2. Enters email `admin@ezhealthkonnect.com` and password `admin123`
3. Clicks **Login**
4. System verifies credentials via `POST /api/auth/login`
5. Session is created; JWT token issued (24h expiry)
6. User is redirected to `dashboard.html`
7. Audit event `LOGIN_SUCCESS` (risk: low) written to `audit_logs`

**Expected Result:** Dashboard loads, sidebar shows Admin section, `audit_logs` has one new `LOGIN_SUCCESS` entry with IP address and user-agent.

**Verification:**
```sql
SELECT action, result, risk_level, ip_address, created_at
FROM audit_logs
WHERE action = 'LOGIN_SUCCESS'
ORDER BY created_at DESC LIMIT 1;
-- Expected: result='success', risk_level='low'
```

**Security Notes:** Password is never logged. Session cookie is HttpOnly, SameSite=lax.

---

### UC-AUTH-002 · Failed Login — Invalid Password
**Priority:** P1 | **Actor:** Any User

**Business Context:**
Brute-force protection is mandatory for systems handling PHI. Each failed attempt must be recorded and eventually trigger account lockout.

**Preconditions:**
- Valid user account exists (status = `active`)
- `login_attempts` < 5

**Main Flow:**
1. User submits correct email + wrong password
2. `POST /api/auth/login` returns HTTP 401
3. Response body: `{ "message": "Invalid email or password" }`
4. `login_attempts` counter incremented by 1 in DB
5. Audit event `LOGIN_FAILED` (reason: "Invalid password", risk: high) written

**Alt Flow — 4th failure:**
- `login_attempts` = 4; system warns but does not lock yet

**Exception Flow — 5th failure:**
- `locked_until` = `NOW() + 15 minutes` set in DB
- Next login attempt returns HTTP 423

**Expected Result:** 401 response; no session created; `login_attempts` incremented; audit logged.

**Verification:**
```sql
SELECT login_attempts, locked_until FROM users WHERE email = 'testuser@example.com';
-- After 5 failures: login_attempts=5, locked_until IS NOT NULL
```

---

### UC-AUTH-003 · Account Lockout After 5 Failures
**Priority:** P1 | **Actor:** Automated Attack / Brute-Force Scenario

**Business Context:**
HIPAA requires controls to prevent repeated login attempts. Lockout after 5 failures with a 15-minute window is the business rule.

**Preconditions:**
- User account exists, `login_attempts` = 5, `locked_until` > NOW()

**Main Flow:**
1. Attacker (or legitimate user who forgot password) submits credentials
2. System checks `locked_until > NOW()` before password verification
3. Returns HTTP 423 with body: `{ "message": "Account is temporarily locked...", "lockedUntil": "<ISO timestamp>" }`
4. `LOGIN_FAILED` audit event with reason "Account locked" (risk: high)

**Alt Flow — After 15 minutes:**
- `locked_until` has passed; account accepts credentials normally
- On success: `login_attempts` reset to 0, `locked_until` cleared

**Expected Result:** HTTP 423; no password check performed; lockout timestamp returned.

**Security Notes:** Response message does not reveal password correctness, preventing oracle attacks.

---

### UC-AUTH-004 · Admin Manually Unlocks Locked Account
**Priority:** P1 | **Actor:** System Administrator | **Triggered By:** UC-AUTH-003

**Business Context:**
A legitimate employee is locked out. The help desk escalates to the system admin, who unlocks the account via the Security tab in the user drawer — without needing to reset the password.

**Preconditions:**
- Admin is authenticated
- Target user is locked (`locked_until` > NOW(), `login_attempts` ≥ 5)
- Admin opens User Management → clicks locked user row → Security tab

**Main Flow:**
1. Admin navigates to User Management
2. Observes "Locked" count > 0 in the stats bar
3. Searches for user by name/email
4. Clicks user row → drawer opens
5. Selects **Security** tab
6. Observes: `Login Attempts: 5`, `Locked Until: <timestamp>`
7. Clicks **Unlock** button
8. `POST /api/users/:id/unlock` called
9. `locked_until` = null, `login_attempts` = 0 in DB
10. Drawer refreshes: `Login Attempts: 0`, `Locked Until: Not locked`
11. Stats bar "Locked" counter decrements

**Expected Result:** Account accessible again; audit event `USER_UNLOCKED` recorded.

**Verification:**
```sql
SELECT login_attempts, locked_until FROM users WHERE id = '<userId>';
-- Expected: login_attempts=0, locked_until IS NULL
```

---

### UC-AUTH-005 · Session Expiry and Re-Authentication
**Priority:** P2 | **Actor:** Any Authenticated User

**Business Context:**
Sessions expire after 24 hours (cookie `maxAge`). Users must re-authenticate rather than having indefinite access — an important HIPAA access control requirement.

**Preconditions:**
- User has an active session cookie

**Main Flow:**
1. User session expires (24h or cookie cleared)
2. User navigates to `user-management.html`
3. `GET /api/auth/session` returns HTTP 401
4. JS redirects to `window.location.href = '/'`
5. User is presented with login form

**Alt Flow — Intercepted API call:**
- Mid-session API call returns 401
- `showAlert('Session expired')` displayed
- User manually redirects

**Expected Result:** Seamless redirect to login; no PHI accessible without re-auth.

---

## UC-ONB — User Onboarding & Invitation

---

### UC-ONB-001 · Admin Creates User Directly
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
A new interface analyst joins the team. The admin creates their account immediately with a temporary password, assigning the `operator` role so they can configure interfaces but not manage other users.

**Preconditions:**
- Admin authenticated
- Email not already in the `users` table

**Main Flow:**
1. Admin clicks **Create User** in the Users tab toolbar
2. Modal opens with fields: First Name, Last Name, Email, Password, Role
3. Admin fills: `Jane`, `Smith`, `jane.smith@hospital.org`, `Temp@2026!`, `operator`
4. Password strength meter shows "Strong"
5. Admin clicks **Create User**
6. `POST /api/users` body: `{ name: "Jane Smith", email: "...", password: "...", role: "operator" }`
7. New user appears in table with status `active`
8. Stats bar **Total** increments by 1

**Alt Flow — Duplicate Email:**
- System returns HTTP 400: `"User with this email already exists"`
- Modal stays open; error message displayed in alert

**Expected Result:** New user row in table; `USER_CREATED` audit event; email is lowercased in DB.

**Verification:**
```sql
SELECT id, email, role, status, email_verified FROM users
WHERE email = 'jane.smith@hospital.org';
-- Expected: role='operator', status='active', email_verified=true
```

---

### UC-ONB-002 · Admin Invites User via Email Link
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
The hospital's IT security policy prohibits admins from setting initial passwords for users. Instead, an invitation link is generated that allows the user to set their own password — eliminating the need for password sharing over insecure channels.

**Preconditions:**
- Admin authenticated
- Target email not already active in the system

**Main Flow:**
1. Admin clicks **Invite User** button
2. Invite modal opens: email field + role selector
3. Admin enters `dr.chen@hospital.org`, role = `viewer`
4. Clicks **Generate Invite**
5. `POST /api/users/invite` called
6. System creates `pending` user record with:
   - `email_verification_token` = 64-char hex token
   - `invitation_expires_at` = NOW() + 48h
   - `status` = `pending`
7. Invite link displayed: `http://localhost:3000/invite.html?token=<hex>`
8. Admin copies link (clipboard button)
9. Admin shares link via corporate email/Slack

**Alt Flow — Re-invite pending user:**
- Same email invited again → existing pending record updated with new token and expiry

**Expected Result:** Pending user appears in table; invite link generated; `USER_INVITED` audit event logged.

**Verification:**
```sql
SELECT status, email_verification_token, invitation_expires_at
FROM users WHERE email = 'dr.chen@hospital.org';
-- Expected: status='pending', token IS NOT NULL, expires_at ~= NOW()+48h
```

---

### UC-ONB-003 · Invited User Accepts Invitation
**Priority:** P1 | **Actor:** Invited User (no prior authentication)

**Business Context:**
Dr. Chen receives the invite link, opens it on their hospital laptop, sets a password, and gains immediate access. This is the self-service onboarding flow used by most healthcare organisations to reduce IT overhead.

**Preconditions:**
- Valid invitation token exists (not expired, user status = `pending`)
- Token in URL query param: `/invite.html?token=<hex>`

**Main Flow:**
1. User opens invite URL in browser
2. `invite.html` loads, extracts token from URL
3. Form displayed: First Name, Last Name, Password, Confirm Password
4. User fills in details; password strength meter updates live
5. User clicks **Activate Account**
6. `POST /api/users/accept-invite` body: `{ token, firstName, lastName, password }`
7. System validates token, hashes password, updates user:
   - `status` = `active`
   - `email_verified` = true
   - `password_hash` = bcrypt(password, 12)
   - `email_verification_token` = null
   - `invitation_expires_at` = null
8. Success screen shown with **Go to Login** button

**Exception Flow — Password mismatch:**
- Client validates before submission; shows "Passwords do not match"

**Exception Flow — Expired token:**
- Server returns HTTP 400: "Invalid or expired invitation token"
- Expired view shown on invite page

**Expected Result:** User account active; can log in; pending→active status change in table.

---

### UC-ONB-004 · Invitation Token Expires Before Acceptance
**Priority:** P2 | **Actor:** Invited User

**Business Context:**
Invitation links expire after 48 hours for security. If an employee doesn't act in time, the admin must generate a new invite.

**Preconditions:**
- `invitation_expires_at` < NOW() (token expired)
- User status is still `pending`

**Main Flow:**
1. User opens expired invite URL
2. `invite.js` extracts token from URL
3. User submits form
4. `POST /api/users/accept-invite` returns HTTP 400: "Invalid or expired invitation token"
5. Error displayed on invite page

**Expected Result:** HTTP 400; user remains `pending`; admin must re-invite.

---

### UC-ONB-005 · Admin Forces Password Reset on First Login
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
When creating a user directly (with a temp password), the admin flags the account to require a password reset. This ensures the temporary password is never reused.

**Preconditions:**
- User account created directly (UC-ONB-001)
- Admin opens that user's Security tab

**Main Flow:**
1. Admin opens user drawer → Security tab
2. Clicks **Force Reset** button
3. `POST /api/users/:id/force-reset` called
4. `force_password_reset` = true in DB
5. Security info row updates: "Force Reset: Yes"
6. Audit event `USER_FORCE_RESET` logged

**Expected Result:** `force_password_reset = true`; audit trail updated. (UI enforcement of the flag on next login is a planned Phase 2 feature.)

---

## UC-PROF — User Profile Management

---

### UC-PROF-001 · Admin Views Full User Profile
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
The compliance officer needs to verify that all user accounts have complete profile data (department, job title, organization) before the annual HIPAA audit.

**Preconditions:**
- Admin authenticated
- Target user exists with profile data populated

**Main Flow:**
1. Admin navigates to User Management
2. Clicks a user's name in the table
3. Right-side drawer slides in
4. **Profile** tab active by default
5. All fields displayed: First Name, Last Name, Email, Phone, Organization, Department, Job Title, Timezone, Locale
6. `GET /api/users/:id` called (excludes password_hash, tokens)

**Expected Result:** All profile fields visible; sensitive fields (password, tokens) never exposed in response.

**Verification:**
```javascript
// API response must NOT contain these keys:
const forbidden = ['password_hash', 'email_verification_token', 'password_reset_token'];
forbidden.forEach(key => expect(response.body).not.toHaveProperty(key));
```

---

### UC-PROF-002 · Admin Updates User Profile
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
An employee changes department after an internal transfer. The admin updates their profile to reflect the new department and job title, ensuring correct audit attribution.

**Preconditions:**
- Admin authenticated; user drawer open on Profile tab

**Main Flow:**
1. Admin changes `Department` from "Radiology" to "Cardiology"
2. Admin changes `Job Title` from "Technician" to "Lead Technician"
3. Clicks **Save Profile**
4. `PUT /api/users/:id/profile` called with updated fields
5. Alert: "Profile saved"
6. Drawer refreshes from API
7. Table row updates (Department column shows "Cardiology")
8. `USER_PROFILE_UPDATED` audit event with `updatedFields: ['department', 'job_title']`

**Expected Result:** DB updated; audit event includes exactly which fields changed; table reflects new department.

---

### UC-PROF-003 · Admin Changes User Role
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
A senior nurse is promoted to interface administrator. Their role changes from `user` to `operator`, granting them access to pipeline builder and interface management.

**Preconditions:**
- Admin authenticated; target user is `user` role

**Main Flow:**
1. Admin opens user drawer → **Security** tab
2. Changes role dropdown from `user` to `operator`
3. Clicks **Save Security**
4. `PUT /api/users/:id/profile` body: `{ role: "operator" }`
5. Drawer header badge updates to "operator"
6. Table row role badge updates

**Alt Flow — Attempting invalid role:**
- API returns HTTP 400: "Invalid role"

**Security Notes:** Only admins can change roles. Role changes logged at medium risk.

---

### UC-PROF-004 · Admin Suspends User Account
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
An employee is under HR investigation. IT security requires immediate access suspension (not deletion, as the account may be reinstated). Status changes to `suspended` — distinct from `inactive` which is voluntary deactivation.

**Preconditions:**
- Admin authenticated; target user is `active`

**Main Flow:**
1. Admin opens user drawer → Security tab
2. Changes Status dropdown to `suspended`
3. Clicks **Save Security**
4. `PUT /api/users/:id/profile` body: `{ status: "suspended" }`
5. Status badge in drawer changes to red "suspended"
6. Table row status badge updates
7. Next login attempt returns HTTP 401: "Account is inactive. Please contact administrator."

**Expected Result:** User cannot log in; account data preserved; audit trail complete.

---

### UC-PROF-005 · Admin Reactivates Suspended Account
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
HR investigation concluded; employee reinstated. Admin reactivates the account — no re-onboarding needed.

**Preconditions:**
- Account status = `suspended`

**Main Flow:**
1. Admin opens user drawer → Security tab
2. Changes Status to `active`
3. Clicks **Save Security**
4. User can log in again immediately

**Expected Result:** Status = `active`; `USER_PROFILE_UPDATED` audit event; user can authenticate.

---

## UC-RBAC — Role-Based Access Control

---

### UC-RBAC-001 · Operator Role — Interface Access Granted
**Priority:** P1 | **Actor:** Operator User

**Business Context:**
Operators manage HL7 interfaces and pipelines but must not have access to user management or system settings. The role matrix enforces this boundary.

**Preconditions:**
- User has role = `operator`, status = `active`

**Main Flow:**
1. Operator logs in
2. Dashboard shows Interfaces, Messages, Pipeline Builder links
3. Navigates to `interfaces.html` — full access granted
4. Attempts to navigate to `user-management.html`
5. `GET /api/auth/session` returns `role: "operator"`
6. Page JS detects non-admin role; shows access denied message

**Expected Result:** Interface access works; user-management access blocked; no API routes accessible.

---

### UC-RBAC-002 · Viewer Role — Read-Only Enforcement
**Priority:** P1 | **Actor:** Viewer User

**Business Context:**
Clinical auditors are given `viewer` role — they can view monitoring dashboards and reports but cannot modify any configuration.

**Preconditions:**
- User has role = `viewer`

**Main Flow:**
1. Viewer logs in; dashboard renders with read-only sections visible
2. Attempts `POST /api/users` (create user) with session cookie
3. `requireAdmin` middleware returns HTTP 403: "Admin access required"
4. Attempts `PUT /api/users/:id/profile`
5. Returns HTTP 403

**Expected Result:** All write endpoints return 403; read-only dashboard access works.

---

### UC-RBAC-003 · Non-Admin Attempts User Management Access
**Priority:** P1 | **Actor:** Regular `user` role

**Business Context:**
Enforcing the principle of least privilege. A basic `user` account should have no access to PHI management functions including user administration.

**Preconditions:**
- Session exists with role = `user`

**Main Flow:**
1. User navigates to `user-management.html`
2. `checkAuth()` in JS calls `GET /api/auth/session`
3. Role is `user` (not `admin`)
4. Page displays: "Admin access required" message
5. No users table rendered; no API calls to `/api/users` made

**Expected Result:** Zero API calls to user management endpoints; access denied message rendered.

---

### UC-RBAC-004 · requireRole() Middleware Enforcement
**Priority:** P1 | **Actor:** System (automated)

**Business Context:**
Server-side enforcement is the authoritative security layer — the frontend access denial is UX only. This use case verifies the backend middleware rejects non-admins.

**Preconditions:**
- Valid session with role = `operator`

**Main Flow:**
1. Craft request: `GET /api/users` with session cookie (role: operator)
2. `requireAdmin` middleware checks `req.session.user.role !== 'admin'`
3. Returns HTTP 403: `{ "message": "Admin access required" }`

**Verification:**
```bash
curl -X GET http://localhost:3000/api/users \
  -H "Cookie: ezhealth.sid=<operator_session>" \
  # Expected: HTTP 403
```

---

### UC-RBAC-005 · Roles & Permissions Matrix — Visual Accuracy
**Priority:** P2 | **Actor:** System Administrator (audit review)

**Business Context:**
The Roles & Permissions tab must accurately reflect the server-side enforcement. Discrepancies between the UI matrix and actual behaviour create compliance documentation gaps.

**Main Flow:**
1. Admin opens Roles & Permissions tab
2. Reviews matrix: Admin=Full, Operator=Full (interfaces), Viewer=View only, User=None for restricted features
3. Validates each row against actual API behaviour
4. Each "Full" cell corresponds to an accessible endpoint
5. Each "—" cell corresponds to a 403 response

**Expected Result:** Matrix rows match actual middleware enforcement for all 12 feature categories.

---

## UC-BULK — Bulk Operations

---

### UC-BULK-001 · Bulk Deactivate Multiple Users
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
A hospital department is restructured; 8 contractors lose system access on the same day. The admin selects all 8 and deactivates in one operation rather than 8 separate edits — critical during fast-paced HR events.

**Preconditions:**
- Multiple users with status = `active` exist in the table
- Admin authenticated

**Main Flow:**
1. Admin searches/filters to find the target users
2. Checks checkboxes for each target user (or Select All)
3. Bulk action toolbar appears: "8 selected"
4. Clicks **Deactivate** button
5. Browser confirm: "deactivate 8 selected user(s)?"
6. `POST /api/users/bulk` body: `{ action: "deactivate", userIds: ["id1","id2",...] }`
7. All 8 users updated to `status = inactive` in one DB operation
8. Table refreshes; status badges update
9. Stats bar Active count decrements by 8

**Expected Result:** All 8 users deactivated; single `USERS_BULK_ACTION` audit event with count=8.

**Verification:**
```sql
SELECT status FROM users WHERE id = ANY(ARRAY['id1','id2',...]);
-- Expected: all rows status='inactive'
```

---

### UC-BULK-002 · Bulk Delete with Self-Protection
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
Prevents the admin from accidentally deleting their own account during a bulk delete, which would leave the system with no admin.

**Preconditions:**
- Admin selects multiple users including their own account

**Main Flow:**
1. Admin selects 5 users including themselves
2. Clicks **Delete** → confirm dialog
3. `POST /api/users/bulk` body includes admin's own ID
4. Server filters: `const safeIds = userIds.filter(id => id !== req.session.user.id)`
5. Only 4 accounts deleted; admin's own ID silently excluded
6. Response: "Bulk delete completed for 4 user(s)"

**Expected Result:** Own account never deleted; 4 others removed; no error thrown.

---

### UC-BULK-003 · Bulk Activate Pending Users
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
After a batch import of user accounts (e.g., from an LDAP sync), all accounts are in `pending` state. Admin approves the batch at once.

**Preconditions:**
- Multiple users with status = `pending`

**Main Flow:**
1. Admin filters by Status = `pending`
2. Clicks Select All
3. Clicks **Activate** → confirm
4. `POST /api/users/bulk` body: `{ action: "activate", userIds: [...] }`
5. All pending users set to `active`

**Expected Result:** All selected accounts active; stats counters updated.

---

### UC-BULK-004 · Bulk Action Cancelled by User
**Priority:** P3 | **Actor:** System Administrator

**Business Context:**
Admin accidentally selects wrong users. They can cancel the confirm dialog or use the Cancel button in the bulk toolbar to clear selection without any changes.

**Main Flow:**
1. Admin selects 3 users → bulk toolbar appears
2. Clicks **Delete** → browser confirm dialog appears
3. Admin clicks **Cancel** in the browser dialog
4. No API call made; selection preserved
5. Admin clicks **Cancel** in the toolbar → selection cleared; toolbar hidden

**Expected Result:** Zero database changes; no audit events; UI returns to normal state.

---

## UC-COMP — HIPAA / GDPR Compliance

---

### UC-COMP-001 · Admin Records Data Consent
**Priority:** P1 | **Actor:** System Administrator / Compliance Officer

**Business Context:**
Under GDPR Article 7 and HIPAA Privacy Rule, user data processing requires documented consent. The compliance officer records the consent date when a signed consent form is received.

**Preconditions:**
- User has `data_consent_given = false`
- Admin opens user drawer → Compliance tab

**Main Flow:**
1. Admin opens Compliance tab
2. Toggles **Data Consent Given** to ON
3. Sets **Consent Date** to today's date (2026-03-22)
4. Clicks **Save Compliance**
5. `PUT /api/users/:id/compliance` body: `{ data_consent_given: true, data_consent_date: "2026-03-22" }`
6. `USER_COMPLIANCE_UPDATED` audit event logged with `complianceFlags: { gdpr: true, hipaa: true }`

**Expected Result:** DB updated; audit event with compliance flags; changes reflected in UI on drawer refresh.

---

### UC-COMP-002 · Admin Sets Data Retention Date
**Priority:** P1 | **Actor:** Compliance Officer

**Business Context:**
GDPR Article 5 requires data is kept "no longer than necessary." The organisation's retention policy is 7 years post-employment. Admin sets the retention date at offboarding.

**Preconditions:**
- User being offboarded
- Compliance tab open

**Main Flow:**
1. Admin sets **Data Retention Until** = `2033-03-22` (7 years)
2. Clicks **Save Compliance**
3. `data_retention_until` stored in DB
4. Future automated job (planned) will flag accounts past this date

**Expected Result:** `data_retention_until` = `2033-03-22` in DB.

---

### UC-COMP-003 · Admin Flags GDPR Right-to-be-Forgotten Request
**Priority:** P1 | **Actor:** System Administrator (acting on user's written request)

**Business Context:**
A former user invokes GDPR Article 17 (right to erasure). The admin records this in the system, triggering the compliance workflow. Actual deletion must be approved by the DPO first — the flag is an interim step.

**Preconditions:**
- Written GDPR deletion request received from user
- Admin authenticated; user drawer open on Compliance tab

**Main Flow:**
1. Admin clicks **Request GDPR Deletion** button
2. Browser confirm: "Flag this user for GDPR deletion? This will be logged and is irreversible."
3. Admin confirms
4. `POST /api/users/:id/gdpr-request` called
5. `gdpr_delete_requested = true`, `gdpr_delete_requested_at = NOW()` in DB
6. Compliance tab shows: "GDPR Delete Requested: Requested" (red badge)
7. "Request GDPR Deletion" button is disabled (cannot flag twice)
8. `GDPR_DELETE_REQUESTED` audit event with `riskLevel: high`, `complianceFlags: { gdpr: true }`

**Alt Flow — Duplicate request:**
- Button is disabled after first flagging; cannot be clicked again

**Expected Result:** Flag recorded; button disabled; high-risk audit event with GDPR compliance flag.

**Verification:**
```sql
SELECT gdpr_delete_requested, gdpr_delete_requested_at
FROM users WHERE id = '<userId>';
-- Expected: gdpr_delete_requested=true, gdpr_delete_requested_at IS NOT NULL
```

---

### UC-COMP-004 · Admin Marks User Data as Anonymized
**Priority:** P2 | **Actor:** Compliance Officer

**Business Context:**
After completing the GDPR deletion process (name, contact info replaced with anonymized values), the compliance flag is set to confirm the process is complete.

**Main Flow:**
1. Admin has anonymized user's PII fields manually
2. Opens Compliance tab → toggles **Data Anonymized** ON
3. Saves compliance
4. `data_anonymized = true` in DB
5. Audit event logged

**Expected Result:** `data_anonymized = true`; compliance record complete for audit purposes.

---

### UC-COMP-005 · Compliance Report — Audit Export
**Priority:** P1 | **Actor:** Compliance Officer / External Auditor

**Business Context:**
Annual HIPAA audit requires a full access log for the past 12 months. The compliance officer exports audit logs to CSV and submits to the auditor.

**Main Flow:**
1. Admin navigates to **Audit Logs** tab
2. Sets date range: from `2025-03-22`, to `2026-03-22`
3. Clicks **Apply**
4. Audit log table populates with 12 months of events
5. Clicks **Export CSV**
6. Browser downloads `audit-logs.csv`
7. CSV contains columns: id, user_id, action, entity_type, entity_id, ip_address, result, risk_level, created_at

**Verification:**
```
CSV header row: id,user_id,action,entity_type,entity_id,ip_address,result,risk_level,created_at
Each row: non-empty id, valid ISO timestamp in created_at
```

---

## UC-AUDIT — Audit Log & Activity Monitoring

---

### UC-AUDIT-001 · View User Activity Timeline
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
An anomaly is detected in interface processing. The admin investigates whether a specific user made any configuration changes in the past 24 hours by reviewing their activity timeline.

**Preconditions:**
- Target user has at least one audit_log entry

**Main Flow:**
1. Admin opens user drawer → **Activity** tab
2. `GET /api/users/:id/activity` called
3. Timeline renders showing last 50 audit events for this user
4. Each entry shows: action, timestamp, IP address, risk badge
5. Events color-coded: green (low), yellow (medium), orange (high), red (critical)

**Expected Result:** Timeline renders chronologically; correct risk-level colour coding; IP addresses visible.

---

### UC-AUDIT-002 · Filter Audit Logs by Risk Level
**Priority:** P1 | **Actor:** Security Administrator

**Business Context:**
After a suspected security incident, the security team filters audit logs to show only `high` and `critical` events in the past 7 days, identifying the attack vector.

**Main Flow:**
1. Admin opens **Audit Logs** tab
2. Sets Risk Level filter = `high`
3. Sets From date = `2026-03-15`
4. Clicks **Apply**
5. `GET /api/audit-logs?riskLevel=high&from=2026-03-15` called
6. Only high-risk events shown (LOGIN_FAILED, USER_DELETED, GDPR_DELETE_REQUESTED, etc.)
7. Page count and total count updated

**Expected Result:** Only high-risk events returned; pagination accurate; Export CSV link includes same filters.

---

### UC-AUDIT-003 · Search Audit Logs by Action
**Priority:** P2 | **Actor:** Compliance Officer

**Business Context:**
The compliance officer needs to review all `USER_CREATED` events for the past quarter to verify onboarding processes were followed.

**Main Flow:**
1. Admin types `USER_CREATED` in the **Action** filter field
2. Clicks **Apply**
3. `GET /api/audit-logs?action=USER_CREATED` called (uses ILIKE for partial match)
4. All user creation events displayed

**Expected Result:** Only `USER_CREATED` events returned; partial match works (`USER_CREATE` also returns `USER_CREATE_FAILED`).

---

### UC-AUDIT-004 · Detect Brute-Force Attack Pattern
**Priority:** P1 | **Actor:** Security Administrator

**Business Context:**
Multiple `LOGIN_FAILED` events from the same IP in a short window indicate a brute-force attack. The security admin uses audit logs + activity timeline to identify and respond.

**Main Flow:**
1. Admin filters audit logs: action = `LOGIN_FAILED`, risk = `high`, from = `2026-03-22`
2. Identifies multiple entries with same `ip_address` in quick succession
3. Opens affected user's drawer → Activity tab
4. Confirms pattern: 5 `LOGIN_FAILED` entries + 1 account lock event
5. Verifies `locked_until` is still active
6. Optionally: changes user password via Security tab

**Expected Result:** Audit trail clearly shows attack pattern; timestamp, IP, and attempt count all visible.

---

### UC-AUDIT-005 · Audit Trail Preserved After User Deletion
**Priority:** P1 | **Actor:** Compliance Officer

**Business Context:**
HIPAA requires that audit logs be retained even after a user account is deleted. The audit trail must not be cascade-deleted — it uses `SET NULL` on `user_id` to preserve the record.

**Preconditions:**
- User has audit log entries
- Admin deletes user via Delete button

**Main Flow:**
1. Admin deletes user account
2. `DELETE /api/users/:id` called; user row removed from `users` table
3. Admin queries audit logs for the deleted user's previous ID

**Verification:**
```sql
SELECT COUNT(*) FROM audit_logs WHERE user_id IS NULL AND entity_id = '<deletedUserId>';
-- Expected: COUNT > 0 (audit records preserved with user_id nulled out)
-- Or: COUNT(*) WHERE metadata->>'deletedUser' = 'deleted@email.com' > 0
```

**Expected Result:** Audit logs preserved; `user_id` = null (FK on DELETE SET NULL); no orphan cleanup removes records.

---

## UC-SRCH — Search, Filter & Sort

---

### UC-SRCH-001 · Live Search by Name and Email
**Priority:** P1 | **Actor:** System Administrator

**Business Context:**
With 200+ users in the system, the admin needs to quickly locate a specific user without scrolling through paginated tables.

**Main Flow:**
1. Admin types "chen" in the search box
2. Table filters live (no submit button required) — debounced on `input` event
3. Shows only users where name or email contains "chen" (case-insensitive)
4. Pagination updates to show filtered count
5. Admin clears search — full table returns

**Expected Result:** Instant filter; case-insensitive match across first_name, last_name, email fields.

---

### UC-SRCH-002 · Filter by Role and Status Combined
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
The admin wants to see all `operator` accounts that are currently `inactive` — to clean up unused operator accounts.

**Main Flow:**
1. Admin sets Role filter = `operator`
2. Sets Status filter = `inactive`
3. Table shows only users matching both criteria simultaneously
4. Export or bulk-delete those accounts

**Expected Result:** AND logic applied; only rows matching both role AND status returned.

---

### UC-SRCH-003 · Sort by Last Login (Oldest First)
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
Users who haven't logged in for over 6 months are candidates for account deactivation (HIPAA access review). Sorting by last login ascending surfaces dormant accounts.

**Main Flow:**
1. Admin clicks **Last Login** column header
2. Table sorts ascending (oldest first) — arrow indicator updates
3. Accounts with no login show "—" at the top
4. Clicking again sorts descending (most recent first)

**Expected Result:** Correct ascending/descending toggle; null values sort to top on ascending; arrow icon reflects direction.

---

### UC-SRCH-004 · Pagination Through Large User Set
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
A large healthcare network has 500+ users. Pagination ensures the table remains performant and navigable.

**Main Flow:**
1. 500 users in system; page size = 25
2. Table shows users 1–25; pagination: "1–25 of 500 users" | "1 / 20"
3. Admin changes page size to 50; table shows 1–50; pagination updates
4. Clicks next page → rows 51–100
5. Clicks previous page → returns to 1–50
6. Pages are calculated client-side from cached `state.filtered` array

**Expected Result:** Pagination math correct; page controls enabled/disabled appropriately; count displays accurate.

---

## UC-OFF — User Offboarding

---

### UC-OFF-001 · Standard Employee Offboarding
**Priority:** P1 | **Actor:** System Administrator (HR-triggered)

**Business Context:**
An employee resigns. HR notifies IT. Standard process: deactivate account → set data retention date → document in audit trail. Account is NOT deleted immediately (may be needed for interface attribution for up to 90 days).

**Workflow Steps:**
1. **Security tab:** Change status → `inactive`; save
2. **Security tab:** Click **Force Reset** (invalidates any shared credentials)
3. **Compliance tab:** Set `Data Retention Until` = departure date + 7 years
4. **Compliance tab:** Record `Data Consent Date` if needed
5. **Save** compliance changes
6. Review Activity tab — confirm no suspicious last-minute access

**Expected Result:** Account inaccessible; data retained for compliance period; full audit trail of admin actions.

---

### UC-OFF-002 · Immediate Account Deletion (GDPR Request)
**Priority:** P1 | **Actor:** System Administrator + Data Protection Officer

**Business Context:**
A contractor who never signed a BAA invokes GDPR right-to-erasure. After DPO approval, the admin permanently deletes the account.

**Main Flow:**
1. Admin opens user → Compliance tab → flags `gdpr_delete_requested`
2. DPO reviews and approves (out-of-system)
3. Admin clicks **Delete** (trash icon in table or bulk delete)
4. Confirm: "Delete user 'name'? This cannot be undone."
5. `DELETE /api/users/:id` called
6. User row removed from `users` table
7. Audit logs preserved (user_id SET NULL)

**Expected Result:** User deleted; audit trail preserved; `USER_DELETED` event at high risk level.

---

### UC-OFF-003 · Offboarding Bulk — Department Closure
**Priority:** P2 | **Actor:** System Administrator

**Business Context:**
A satellite clinic closes. 15 users need simultaneous deactivation, then a staggered deletion 90 days later.

**Main Flow:**
1. Filter by Department = "Satellite Clinic North"
2. Select all 15 users
3. Bulk action → **Deactivate** (immediate)
4. 90 days later: filter inactive + department, select all, bulk **Delete**

**Expected Result:** Two-phase offboarding; audit events at both stages; data preserved for 90 days.

---

## UC-API — Technical / API Workflows

---

### UC-API-001 · Stats Endpoint Returns Correct Counts
**Priority:** P1 | **Actor:** System (automated health check)

**Business Context:**
The stats bar is the admin's first view of system user health. Incorrect counts undermine trust in the system.

**Verification:**
```bash
curl -s http://localhost:3000/api/users/stats \
  -H "Cookie: ezhealth.sid=<admin_session>"
# Expected response:
{
  "total": <N>,
  "active": <A>,
  "inactive": <I>,
  "suspended": <S>,
  "pending": <P>,
  "admins": <AD>,
  "locked": <L>
}
# Assert: total = active + inactive + suspended + pending
```

---

### UC-API-002 · Invite Token — 48-Hour Expiry Enforcement
**Priority:** P1 | **Actor:** System (automated)

**Business Context:**
Invite links must expire. The server enforces this at the database level, not just in application logic.

**Verification:**
```sql
-- Simulate expired token
UPDATE users SET invitation_expires_at = NOW() - INTERVAL '1 hour'
WHERE email = 'test@example.com' AND status = 'pending';

-- Attempt acceptance
POST /api/users/accept-invite { token: "<expired_token>", ... }
-- Expected: HTTP 400 "Invalid or expired invitation token"
```

---

### UC-API-003 · Audit Log CSV Export Format Validation
**Priority:** P2 | **Actor:** Compliance Officer

**Business Context:**
The exported CSV must be parseable by Excel and comply with the audit format required by the compliance team.

**Verification:**
```bash
curl -o audit.csv \
  "http://localhost:3000/api/audit-logs?export=csv&from=2026-01-01&to=2026-03-22&limit=500" \
  -H "Cookie: ezhealth.sid=<admin_session>"

# Validate:
# 1. First row = header: id,user_id,action,entity_type,entity_id,ip_address,result,risk_level,created_at
# 2. Timestamps in ISO 8601 format
# 3. No SQL injection in content (field values escaped)
# 4. Content-Type: text/csv
# 5. Content-Disposition: attachment; filename="audit-logs.csv"
```

---

### UC-API-004 · Bulk Operation Atomicity
**Priority:** P2 | **Actor:** System (automated)

**Business Context:**
If the bulk operation fails partway through, the system should not leave half the users in one state and half in another — partial updates are worse than no update.

**Note:** Current implementation uses Sequelize `Model.update()` which is a single SQL statement — atomicity guaranteed at DB level.

**Verification:**
```bash
POST /api/users/bulk
{ "action": "deactivate", "userIds": ["valid-uuid-1", "invalid-uuid-x", "valid-uuid-2"] }
# Current behaviour: invalid UUID causes query to silently skip or error
# Expected: all valid IDs processed; response indicates count
```

---

### UC-API-005 · Password Hash Never Exposed in API Response
**Priority:** P1 | **Actor:** Security Auditor

**Business Context:**
Any API that returns user data must explicitly exclude `password_hash`. A single leaked hash could enable offline cracking.

**Verification:**
```javascript
// GET /api/users (list)
const users = await GET('/api/users');
users.forEach(u => {
    expect(u).not.toHaveProperty('password_hash');
    expect(u).not.toHaveProperty('email_verification_token');
    expect(u).not.toHaveProperty('password_reset_token');
});

// GET /api/users/:id (detail)
const user = await GET('/api/users/valid-id');
expect(user).not.toHaveProperty('password_hash');
```

---

## Test Data Reference

### Standard Test Users

| Email | Password | Role | Status | Purpose |
|---|---|---|---|---|
| admin@ezhealthkonnect.com | admin123 | admin | active | Default admin (OOB) |
| operator@test.com | Test@2026! | operator | active | Operator role testing |
| viewer@test.com | Test@2026! | viewer | active | Viewer role testing |
| inactive@test.com | Test@2026! | user | inactive | Inactive account testing |
| pending@test.com | — | user | pending | Invite flow testing |
| locked@test.com | — | user | active | Lockout testing (set login_attempts=5) |

### SQL Setup Scripts

```sql
-- Create test users for QA
INSERT INTO users (id, email, password_hash, first_name, last_name, role, status, email_verified)
VALUES
  (gen_random_uuid(), 'operator@test.com',  '<bcrypt>', 'Test', 'Operator', 'operator', 'active', true),
  (gen_random_uuid(), 'viewer@test.com',    '<bcrypt>', 'Test', 'Viewer',   'viewer',   'active', true),
  (gen_random_uuid(), 'inactive@test.com',  '<bcrypt>', 'Test', 'Inactive', 'user',     'inactive', true),
  (gen_random_uuid(), 'suspended@test.com', '<bcrypt>', 'Test', 'Suspended','user',     'suspended', true);

-- Simulate locked account
UPDATE users SET login_attempts = 5, locked_until = NOW() + INTERVAL '10 minutes'
WHERE email = 'locked@test.com';
```

---

## Risk Matrix

| Risk | Use Cases | Severity | Mitigation |
|---|---|---|---|
| Password brute-force | UC-AUTH-002/003 | Critical | 5-attempt lockout, 15-min cooldown |
| Privilege escalation | UC-RBAC-003/004 | High | Server-side requireAdmin middleware |
| GDPR non-compliance | UC-COMP-003/004 | High | Flagging + audit trail |
| Credential exposure | UC-API-005 | High | Explicit field exclusion in queries |
| Insider threat | UC-AUDIT-004 | High | Full audit trail, IP logging |
| Session hijacking | UC-AUTH-005 | Medium | HttpOnly cookies, 24h expiry |
| Data retention breach | UC-COMP-002 | Medium | Retention date tracking |
| Bulk misuse | UC-BULK-002 | Medium | Self-protection filter |
