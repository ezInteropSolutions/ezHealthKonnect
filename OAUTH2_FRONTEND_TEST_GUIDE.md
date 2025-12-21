# OAuth 2.0 Frontend Testing Guide

## ⚠️ Important: How to Test OAuth2 in the UI

### CORS Issue Explained

The OAuth2ConfigBuilder has a "Test Connection" button that makes OAuth requests **directly from the browser**. This causes CORS errors because:
- OAuth servers don't allow browser requests (CORS blocked)
- OAuth tokens should be obtained server-side for security

### ✅ Correct Way to Test: Use "🧪 Test API Endpoint"

The **"🧪 Test API Endpoint"** button sends the request through the **backend**, which:
- ✅ Makes OAuth token request from server (no CORS issues)
- ✅ Caches tokens properly
- ✅ Is the actual production flow

---

## Step-by-Step Testing Instructions

### Step 1: Open Pipeline Builder
```
http://localhost:3000/pipeline-builder.html
```

### Step 2: Create API Enrichment Step
1. Click "New Pipeline"
2. Drag "API Enrichment" from toolbox to canvas
3. Click on the step to open properties panel

### Step 3: Configure OAuth 2.0

**Use Google OAuth Playground Credentials**:

1. **Get Access Token from Google** (2 minutes):
   - Go to: https://developers.google.com/oauthplayground/
   - Select: `https://www.googleapis.com/auth/userinfo.email`
   - Click "Authorize APIs" → Sign in
   - Click "Exchange authorization code for tokens"
   - **Copy the Access Token**

2. **Configure in UI** (easiest method):
   ```
   Auth Type: Bearer Token (simpler for testing)
   Bearer Token: <paste token from playground>
   Endpoint: https://www.googleapis.com/oauth2/v1/userinfo
   Method: GET
   ```

### Step 4: Test the API
1. **Click "🧪 Test API Endpoint"** (NOT "Test Connection")
2. ✅ **Success!** You'll see your Google profile:
   ```json
   {
     "id": "123456789",
     "email": "your-email@gmail.com",
     "verified_email": true
   }
   ```

---

## Testing Full OAuth2 Flow (Advanced)

If you want to test the **full OAuth2 client credentials flow**:

### Option 1: Use Auth0 (Free)

1. **Create Auth0 Account**: https://auth0.com/signup (free tier)

2. **Create Machine-to-Machine Application**:
   - Go to Applications → Create Application
   - Choose "Machine to Machine Applications"
   - Authorize it for "Auth0 Management API"
   - Copy **Client ID** and **Client Secret**

3. **Configure in UI**:
   ```
   Auth Type: OAuth 2.0
   Grant Type: Client Credentials
   Token URL: https://YOUR-TENANT.auth0.com/oauth/token
   Client ID: <from Auth0 dashboard>
   Client Secret: <from Auth0 dashboard>
   Scope: read:users (or whatever your API allows)
   Endpoint: https://YOUR-TENANT.auth0.com/api/v2/users
   Method: GET
   ```

4. **Skip "Test Connection"** (it will fail due to CORS)

5. **Click "🧪 Test API Endpoint"** ✅
   - Backend makes OAuth request (no CORS!)
   - Token obtained and cached
   - API called successfully

### Option 2: Use Google Cloud Project

1. **Create Project**: https://console.cloud.google.com/
2. **Enable API**: Google+ API or People API
3. **Create OAuth Credentials**:
   - APIs & Services → Credentials
   - Create OAuth client ID
   - Application type: Web application
   - Get Client ID and Secret

4. **Get Refresh Token**:
   - Use OAuth Playground with your credentials
   - Get refresh token

5. **Configure in UI**:
   ```
   Auth Type: OAuth 2.0
   Grant Type: Refresh Token
   Token URL: https://oauth2.googleapis.com/token
   Client ID: <your-client-id>
   Client Secret: <your-client-secret>
   Refresh Token: <from playground>
   Endpoint: https://www.googleapis.com/oauth2/v1/userinfo
   Method: GET
   ```

6. **Click "🧪 Test API Endpoint"** ✅

---

## What You'll See

### Browser Console (Success):
```javascript
[APIEndpointTester] Testing API endpoint...
[APIEndpointTester] ✅ Test successful
Response: {
  "id": "123456789",
  "email": "user@example.com"
}
```

### Docker Logs (OAuth Flow):
```bash
docker-compose logs -f app | grep -i oauth

# First call:
🔐 Requesting new OAuth2 token from https://oauth2.googleapis.com/token
✅ OAuth2 token obtained successfully (expires: 2025-12-20 18:45:00)
🔐 Added OAuth2 token authentication

# Second call (cached):
♻️  Using cached OAuth2 token (expires: 2025-12-20 18:45:00)
🔐 Added OAuth2 token authentication
```

---

## Common Errors and Solutions

### Error: CORS policy blocked
**Cause**: Using "Test Connection" button in OAuth2ConfigBuilder
**Solution**: Use "🧪 Test API Endpoint" instead (goes through backend)

### Error: DNS resolution failed (demo.identityserver.io)
**Cause**: That server doesn't exist
**Solution**: Use real OAuth provider (Google, Auth0, etc.)

### Error: Invalid credentials
**Cause**: Using test/fake credentials
**Solution**: Get real credentials from OAuth provider

### Error: Token expired
**Cause**: Token lifetime exceeded
**Solution**: Get new token from OAuth Playground or let system refresh automatically

---

## Recommended Test Flow

### Quick Win (5 minutes):
1. ✅ Get token from Google OAuth Playground
2. ✅ Use as **Bearer Token** (not OAuth2 flow)
3. ✅ Click "🧪 Test API Endpoint"
4. ✅ See instant success!

### Full OAuth2 Test (15 minutes):
1. ✅ Create free Auth0 account
2. ✅ Create Machine-to-Machine app
3. ✅ Configure OAuth2 in UI
4. ✅ Click "🧪 Test API Endpoint"
5. ✅ Watch token caching in Docker logs

---

## Why "Test Connection" Fails

The OAuth2ConfigBuilder "Test Connection" button:
- ❌ Makes request **from browser** (CORS blocked)
- ❌ Only for debugging OAuth providers that allow CORS
- ⚠️ Should be removed or proxied through backend

The "🧪 Test API Endpoint" button:
- ✅ Makes request **from backend** (no CORS issues)
- ✅ Uses actual OAuth2Service with caching
- ✅ This is the real production flow
- ✅ **This is what you should use!**

---

## Summary

**Don't use**: "Test Connection" in OAuth2ConfigBuilder (CORS issues)
**Do use**: "🧪 Test API Endpoint" (proper backend flow)

**Easiest test**:
1. Get token from Google OAuth Playground
2. Use Bearer Token auth (not OAuth2)
3. Test endpoint immediately ✅

**Full OAuth2 test**:
1. Get Auth0 free account
2. Configure OAuth2 with real credentials
3. Use "🧪 Test API Endpoint" ✅
4. Watch automatic token caching work!

---

The OAuth2 integration is **fully working** - just make sure to use the backend endpoint tester, not the browser-based test connection button!
