# OAuth 2.0 UI Debugging Guide

## Current Issue: Token URL Empty

### Error Message:
```
"error": "failed to get OAuth2 token: failed to request OAuth2 token:
HTTP request failed: Post \"\": unsupported protocol scheme \"\""
```

This means the `oauth2TokenUrl` field is **empty** when it reaches the backend.

---

## Quick Fix: Verify Fields Are Filled

Looking at your screenshot, the OAuth2 form is visible. Please:

### 1. Fill in ALL required fields:

```
Token URL: https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token
Client ID: pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua
Client Secret: r9uyn_4T2MZUOqaYBLnwGwy0C-ZXIno6H_4Y7zhOi6sO9VZgFbeFi7k7FR-pN9ct
Scope: (leave empty or add: read:users)
```

⚠️ **Don't just see the placeholder** - actually **type the values** into each field!

### 2. Click "Save" button at bottom of properties panel

### 3. Check Browser Console:

Open DevTools (F12) and look for:
```
[PropertiesPanel] ✅ Saved OAuth 2.0 config to step.config (flattened): {
  oauth2TokenUrl: "https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token",
  oauth2ClientId: "pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua",
  oauth2GrantType: "client_credentials"
}
```

### 4. Click "🧪 Test API Endpoint"

---

## What to Check in Browser Console

After clicking Save, type this in console:

```javascript
// Check if config was saved
console.log('Step config:', window.pipelineBuilder?.selectedStep?.config);
```

**Should show**:
```javascript
{
  authType: "oauth2",
  oauth2TokenUrl: "https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token",
  oauth2ClientId: "pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua",
  oauth2GrantType: "client_credentials"
}
```

**If oauth2TokenUrl is missing**, the form isn't saving correctly.

---

## Alternative Quick Test

Get a token manually and use Bearer auth:

```bash
# Get token from Auth0
curl -X POST https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials&client_id=pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua&client_secret=r9uyn_4T2MZUOqaYBLnwGwy0C-ZXIno6H_4Y7zhOi6sO9VZgFbeFi7k7FR-pN9ct&audience=https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/"
```

Then in UI:
```
Auth Type: Bearer Token
Bearer Token: <paste token from curl>
Endpoint: https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/users
```

This will prove the whole flow works while we debug OAuth2 form!
