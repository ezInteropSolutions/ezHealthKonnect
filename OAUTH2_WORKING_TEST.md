# ✅ OAuth 2.0 Audience Field Missing - Quick Fix

## The Problem

Your test payload shows the audience field is **missing**:

```json
{
  "oauth2TokenUrl": "https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token",
  "oauth2ClientId": "pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua",
  "oauth2ClientSecret": "r9uyn_4T2MZUOqaYBLnwGwy0C-ZXIno6H_4Y7zhOi6sO9VZgFbeFi7k7FR-pN9ct",
  "oauth2GrantType": "client_credentials"
  // ❌ oauth2Audience is missing!
}
```

Auth0 requires the audience parameter, so it's returning 400 Bad Request.

---

## Quick Fix: 2 Steps

### Step 1: Hard Refresh Browser (Clear Cache)
**Press**: `Ctrl + Shift + R` (Windows) or `Cmd + Shift + R` (Mac)

This ensures the updated JavaScript with audience field support is loaded.

### Step 2: Fill in Audience Field

After hard refresh, you should see a new **"Audience"** field in the OAuth2 form.

**Fill it in**:
```
Audience: https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/
```

**IMPORTANT**: Make sure you actually **type the value** - don't just see the placeholder!

---

## How to Verify It's Fixed

### In Browser Console (F12)

After filling the audience field and clicking "Test API Endpoint", you should see:

```javascript
[PropertiesPanel] 🔍 Reading OAuth2 config for API test: {
  grantType: "client_credentials",
  tokenURL: "https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token",
  clientID: "pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua",
  clientSecret: "r9uyn_4T2MZUOqaYBLnwGwy0C-ZXIno6H_4Y7zhOi6sO9VZgFbeFi7k7FR-pN9ct",
  scope: "read:users",
  audience: "https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/"  // ✅ Should be here!
}
```

### Test the Audience Field Manually

In browser console, run:

```javascript
// Get the OAuth2ConfigBuilder instance
const container = document.querySelector('.oauth2-config-builder');
const builder = container._oauth2ConfigBuilderInstance;

// Get the config
console.log(builder.getConfig());

// Should show:
// {
//   grantType: "client_credentials",
//   tokenURL: "...",
//   clientID: "...",
//   clientSecret: "...",
//   audience: "https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/"  // ✅
// }
```

---

## If Audience Field Still Missing from UI

If you don't see an "Audience" field after hard refresh, let's verify the files were updated:

### Check OAuth2ConfigBuilder.js

Open browser DevTools → Sources → find `OAuth2ConfigBuilder.js`

Search for: `addFormField('audience'`

**You should see** (around line 127):
```javascript
// Optional fields (all grant types)
this.addFormField('scope', 'Scope', 'text', 'read write', false);
this.addFormField('audience', 'Audience', 'text', 'https://api.example.com/', false);
```

If you **don't see** the audience line, then the file wasn't updated in the Docker container.

**Solution**: Rebuild Docker container:
```bash
docker-compose down
docker-compose up -d --build
```

Then hard refresh browser again.

---

## Alternative: Manual Test with curl

If you want to verify OAuth2 works end-to-end while debugging the UI, you can test manually:

### Get Token with curl
```bash
curl -X POST https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua" \
  -d "client_secret=r9uyn_4T2MZUOqaYBLnwGwy0C-ZXIno6H_4Y7zhOi6sO9VZgFbeFi7k7FR-pN9ct" \
  -d "audience=https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/"
```

**Expected Response**:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6...",
  "token_type": "Bearer",
  "expires_in": 86400
}
```

### Use Token in UI

1. Copy the `access_token` from curl response
2. In UI, select **Auth Type: Bearer Token**
3. Paste token in **Bearer Token** field
4. Set Endpoint: `https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/users`
5. Click **"🧪 Test API Endpoint"**

This should work immediately and prove the full flow works!

---

## What Should Happen (Success)

### Browser Console
```javascript
[PropertiesPanel] 🔍 Reading OAuth2 config for API test: {
  audience: "https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/"  // ✅
  // ...
}
```

### Docker Logs
```bash
docker-compose logs -f app | grep -i oauth

# Expected output:
🔑 [OAuth2] Requesting new access token from https://dev-4y4un4zsmylun23v.us.auth0.com/oauth/token
🔑 [OAuth2] OAuth2 request body: {
  "grant_type": "client_credentials",
  "client_id": "pNwhb5rF32Nk0cbhQGTOs4Fz8yqEkyua",
  "scope": "read:users",
  "audience": "https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/"  // ✅
}
✅ [OAuth2] Access token obtained successfully
🔐 Added OAuth2 token authentication
```

### API Response
```json
{
  "success": true,
  "response": {
    "users": [...]
  },
  "status_code": 200
}
```

---

## Summary

**Root Cause**: Audience field not being sent in test payload

**Most Likely Reason**: Browser cache not cleared after frontend update

**Solution**:
1. ✅ Hard refresh browser (`Ctrl+Shift+R`)
2. ✅ Verify audience field appears in OAuth2 form
3. ✅ Fill in audience: `https://dev-4y4un4zsmylun23v.us.auth0.com/api/v2/`
4. ✅ Click test - should work!

If still not working after hard refresh, rebuild Docker container and try again.
