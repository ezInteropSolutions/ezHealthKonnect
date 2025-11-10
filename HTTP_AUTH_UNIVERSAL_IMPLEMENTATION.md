# HTTP Authentication - Universal Implementation

## Overview
Implemented universal HTTP authentication configuration that works for **all HTTP-based connectivity**, not just FHIR endpoints. This follows the OOB (Out-of-Box) and MVC architectural patterns established in the project.

## Architecture Decision

### Original Design Issue
- Authentication was initially implemented as FHIR-specific (`fhirAuthType`)
- Only appeared in FHIR receiver configuration
- Could not be reused for other HTTP-based connections

### New Universal Design
- Authentication is now **connectivity-specific**, not source-type-specific
- Works for all HTTP connections:
  - FHIR receivers (sourceType='fhir', connectivity='http')
  - Generic HTTP REST endpoints (sourceType='hl7v2', connectivity='http')
  - Any future HTTP-based sources or targets

## Implementation Details

### 1. Universal HTTP Auth Config Method

**Location**: [WizardView.js:790-820](public/js/wizard/optimized/WizardView.js#L790-L820)

```javascript
getHttpAuthConfig(config = {}) {
    return `
        <div class="config-group http-auth-config">
            <h4>🔐 HTTP Authentication</h4>

            <select id="httpAuthType" class="form-control">
                <option value="none">No Authentication (Development)</option>
                <option value="api_key">API Key (Header)</option>
                <option value="basic">Basic Authentication</option>
                <option value="bearer">Bearer Token</option>
                <option value="oauth2">OAuth 2.0</option>
                <option value="mtls">Mutual TLS (Certificate)</option>
            </select>

            <div id="httpAuthDetails">
                ${this.getHttpAuthDetailsPanel(config.authType || 'none', config)}
            </div>
        </div>
    `;
}
```

### 2. Dynamic Auth Details Panel

**Location**: [WizardView.js:823-995](public/js/wizard/optimized/WizardView.js#L823-L995)

```javascript
getHttpAuthDetailsPanel(authType, config = {}) {
    switch (authType) {
        case 'none': // Warning message for public access
        case 'api_key': // Custom header name + API key value
        case 'basic': // Username/password + realm
        case 'bearer': // Token + JWT validation
        case 'oauth2': // Issuer URL, audience, scopes, SMART on FHIR
        case 'mtls': // Server cert, client CA, certificate paths
    }
}
```

### 3. Integration Points

**Standard HTTP Config**: [WizardView.js:556-558](public/js/wizard/optimized/WizardView.js#L556-L558)
```javascript
<!-- HTTP Authentication (Universal for all HTTP sources) -->
${this.getHttpAuthConfig(config)}
```

**FHIR Receiver Config**: [WizardView.js:761-762](public/js/wizard/optimized/WizardView.js#L761-L762)
```javascript
<!-- HTTP Authentication (Universal for all HTTP connections) -->
${this.getHttpAuthConfig(config)}
```

### 4. Event Listener for Dynamic Updates

**Location**: [WizardView.js:4656-4673](public/js/wizard/optimized/WizardView.js#L4656-L4673)

```javascript
// HTTP Authentication Type selector - update auth details panel dynamically
const httpAuthTypeSelect = container.querySelector('#httpAuthType');
if (httpAuthTypeSelect) {
    httpAuthTypeSelect.addEventListener('change', (e) => {
        const authType = e.target.value;
        console.log('🔐 HTTP auth type changed:', authType);

        const authDetailsPanel = container.querySelector('#httpAuthDetails');
        if (authDetailsPanel) {
            authDetailsPanel.innerHTML = this.getHttpAuthDetailsPanel(authType, {});
        }

        this.dispatchEvent(new CustomEvent('fieldChange', {
            detail: { field: 'httpAuthType', value: authType }
        }));
    });
}
```

## Authentication Types Supported

### 1. No Authentication (Development)
- **Use Case**: Development and testing environments
- **Warning**: Displays alert that endpoint is publicly accessible
- **Security**: None

### 2. API Key (Header)
- **Fields**:
  - Header Name (default: `X-API-Key`)
  - Expected API Key (password field)
  - Reject requests without valid key (checkbox)
- **Use Case**: Simple API protection
- **Security**: Low to Medium (depends on key complexity)

### 3. Basic Authentication
- **Fields**:
  - Username
  - Password
  - Realm (optional)
- **Warning**: Displays reminder to use HTTPS
- **Use Case**: Simple username/password protection
- **Security**: Medium (with HTTPS), Low (without HTTPS)

### 4. Bearer Token
- **Fields**:
  - Bearer Token (textarea for JWT or custom tokens)
  - Validate token signature (checkbox)
  - Token Secret / Public Key (for JWT validation)
- **Use Case**: OAuth tokens, custom JWT tokens
- **Security**: Medium to High (depends on token implementation)

### 5. OAuth 2.0
- **Fields**:
  - Token Issuer URL
  - Expected Audience (aud claim)
  - Required Scopes (comma-separated)
  - Enable SMART on FHIR compliance (checkbox)
- **Use Case**: Industry-standard authentication, SMART on FHIR for FHIR endpoints
- **Security**: High

### 6. Mutual TLS (mTLS)
- **Fields**:
  - Server Certificate Path
  - Server Private Key Path
  - Client CA Certificate Path
  - Require valid client certificate (checkbox)
- **Use Case**: Certificate-based authentication, highest security
- **Security**: Very High

## SMART on FHIR Support

When OAuth 2.0 is selected for a FHIR endpoint:
- SMART on FHIR compliance checkbox available
- Scopes field supports SMART scopes:
  - `patient/*.read` - Patient-level read access
  - `patient/*.write` - Patient-level write access
  - `user/*.read` - User-level read access
  - `user/*.write` - User-level write access
  - `system/*.read` - System-level read access
  - `system/*.write` - System-level write access

## User Experience

### Dynamic Form Behavior
1. User selects HTTP connectivity for source/target
2. Authentication section appears automatically
3. User selects authentication type from dropdown
4. Form dynamically updates to show relevant fields
5. Help icons (ⓘ) provide inline guidance
6. Form hints explain each field's purpose

### Form Validation
- Required fields marked with red asterisk
- Password fields use `type="password"` for security
- URL fields use `type="url"` for format validation
- Checkboxes for boolean options (clear yes/no choices)

## Backend Integration (Next Steps)

### Data Capture
The wizard will capture all auth configuration in the interface config:
```javascript
sourceConfig: {
    authType: 'oauth2',
    oauthIssuer: 'https://auth.example.com',
    oauthAudience: 'https://fhir.example.com',
    oauthScopes: 'patient/*.read, patient/*.write',
    smartEnabled: true
}
```

### Runtime Enforcement
Backend connectors will enforce authentication:
1. HTTP Inbound Connector checks incoming requests
2. HTTP Outbound Connector adds credentials to outgoing requests
3. Middleware validates tokens/certificates
4. Audit logging tracks authentication attempts

## Files Modified

1. **[public/js/wizard/optimized/WizardView.js](public/js/wizard/optimized/WizardView.js)**
   - Added `getHttpAuthConfig()` method (lines 790-820)
   - Added `getHttpAuthDetailsPanel()` method (lines 823-995)
   - Integrated auth config into HTTP source panel (line 557)
   - Integrated auth config into FHIR receiver panel (line 762)
   - Added event listener for auth type changes (lines 4656-4673)
   - Removed FHIR-specific authentication section (was lines 642-661)
   - Removed duplicate base URL field in FHIR config

## Testing Checklist

- [ ] Standard HTTP source shows authentication section
- [ ] FHIR receiver shows authentication section
- [ ] Selecting auth type updates form dynamically
- [ ] All 6 auth types display correct fields
- [ ] Help icons show correct information
- [ ] Form data saves to interface config
- [ ] Backend connectors enforce authentication rules

## Future Enhancements

1. **Certificate Upload**: Add file upload for mTLS certificates instead of path input
2. **Token Validation**: Real-time JWT token validation in UI
3. **Scope Builder**: Visual scope builder for SMART on FHIR
4. **Credential Vault**: Integration with secret management systems
5. **Multi-Factor Auth**: Support for MFA workflows
6. **IP Whitelisting**: Additional layer of security based on source IP

## Documentation References

- SMART on FHIR Scopes: http://hl7.org/fhir/smart-app-launch/scopes-and-launch-context.html
- OAuth 2.0 RFC: https://datatracker.ietf.org/doc/html/rfc6749
- mTLS Best Practices: https://www.cloudflare.com/learning/access-management/what-is-mutual-tls/
- HTTP Authentication RFC: https://datatracker.ietf.org/doc/html/rfc7235

---

**Status**: ✅ Complete
**Date**: October 27, 2025
**Follows**: OOB and MVC architectural patterns
