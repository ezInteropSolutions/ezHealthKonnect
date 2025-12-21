# OAuth 2.0 Testing Commands

## Test OAuth2 Integration

Since we need a real OAuth2 server to test against, here are several options:

---

## Option 1: Test with Google OAuth 2.0 (Recommended)

**Prerequisites**: Get credentials from [Google Cloud Console](https://console.cloud.google.com/)

```bash
curl -X POST http://localhost:8080/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://www.googleapis.com/oauth2/v1/userinfo",
      "method": "GET",
      "authType": "oauth2",
      "oauth2TokenUrl": "https://oauth2.googleapis.com/token",
      "oauth2ClientId": "YOUR-CLIENT-ID.apps.googleusercontent.com",
      "oauth2ClientSecret": "YOUR-CLIENT-SECRET",
      "oauth2GrantType": "client_credentials",
      "oauth2Scope": "https://www.googleapis.com/auth/userinfo.email"
    },
    "testData": {}
  }'
```

---

## Option 2: Test in the UI (Easiest)

**This is the recommended approach** - use the Pipeline Builder UI:

### Steps:

1. **Open Pipeline Builder**:
   ```
   http://localhost:3000/pipeline-builder.html
   ```

2. **Create API Enrichment Step**:
   - Click "New Pipeline"
   - Drag "API Enrichment" step to canvas
   - Click on step to open properties

3. **Configure OAuth 2.0**:
   - Auth Type: Select "OAuth 2.0"
   - Grant Type: "Client Credentials"
   - Token URL: `https://oauth2.googleapis.com/token`
   - Client ID: `your-client-id`
   - Client Secret: `your-client-secret`
   - Scope: `https://www.googleapis.com/auth/userinfo.email`

4. **Test Connection**:
   - Click "Test Connection" button in OAuth2ConfigBuilder
   - You'll see if token is obtained successfully

5. **Test API Endpoint**:
   - Configure Endpoint URL: `https://www.googleapis.com/oauth2/v1/userinfo`
   - Click "🧪 Test API Endpoint"
   - See the OAuth2 flow in action!

---

## Option 3: Use Auth0 Test Tenant (Free)

1. **Create free Auth0 account**: https://auth0.com/signup

2. **Get test credentials** from Auth0 Dashboard

3. **Test with Auth0**:
```bash
curl -X POST http://localhost:8080/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://YOUR-DOMAIN.auth0.com/userinfo",
      "method": "GET",
      "authType": "oauth2",
      "oauth2TokenUrl": "https://YOUR-DOMAIN.auth0.com/oauth/token",
      "oauth2ClientId": "YOUR-CLIENT-ID",
      "oauth2ClientSecret": "YOUR-CLIENT-SECRET",
      "oauth2GrantType": "client_credentials",
      "oauth2Scope": "openid profile"
    },
    "testData": {}
  }'
```

---

## Option 4: Mock OAuth2 Server (For Quick Testing)

I can create a simple mock OAuth2 server for testing. Want me to do that?

---

## What You'll See When It Works

### Expected Response (Success):
```json
{
  "success": true,
  "message": "API call successful - inspect response to configure field mapping",
  "request": {
    "method": "GET",
    "url": "https://www.googleapis.com/oauth2/v1/userinfo",
    "headers": {
      "Authorization": "Bearer ya29.a0AfB_byD8X...",
      "Content-Type": "application/json"
    },
    "sent_at": "2025-12-20T17:35:00Z",
    "timeout_ms": 5000
  },
  "response": {
    "status_code": 200,
    "status_text": "OK",
    "duration_ms": 245,
    "headers": {...},
    "body_parsed": {
      "id": "123456789",
      "email": "user@example.com",
      "verified_email": true
    },
    "enriched_fields": 3,
    "field_structure": [
      {
        "path": "$.id",
        "key": "id",
        "type": "string",
        "sample": "123456789"
      },
      {
        "path": "$.email",
        "key": "email",
        "type": "string",
        "sample": "user@example.com"
      }
    ]
  }
}
```

### Docker Logs (Token Caching):
```bash
# First call
docker-compose logs -f app | grep OAuth2
# Output: 🔐 Requesting new OAuth2 token from https://oauth2.googleapis.com/token
# Output: ✅ OAuth2 token obtained successfully (expires: 2025-12-20 18:35:00)
# Output: 🔐 Added OAuth2 token authentication (expires: 2025-12-20 18:35:00)

# Second call (within token lifetime)
# Output: ♻️  Using cached OAuth2 token (expires: 2025-12-20 18:35:00)
# Output: 🔐 Added OAuth2 token authentication (expires: 2025-12-20 18:35:00)
```

---

## Recommended: UI Testing

**The easiest way to test OAuth2 is through the UI**:

1. Go to: http://localhost:3000/pipeline-builder.html
2. Create API Enrichment step
3. Select Auth Type: "OAuth 2.0"
4. Fill in OAuth2 configuration
5. Click "Test Connection" (tests OAuth flow)
6. Click "🧪 Test API Endpoint" (tests full integration)

This gives you:
- ✅ Visual feedback
- ✅ Token display
- ✅ Field picker with clickable fields
- ✅ Real-time testing

---

Would you like me to:
1. Create a simple mock OAuth2 server for testing?
2. Set up a test with a free OAuth provider?
3. Show you how to get Google OAuth credentials?
