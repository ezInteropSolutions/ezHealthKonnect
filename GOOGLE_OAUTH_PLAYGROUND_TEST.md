# ✅ Testing with Google OAuth 2.0 Playground

## Using Google OAuth Playground to Test OAuth2 Integration

Google provides a free OAuth 2.0 Playground at https://developers.google.com/oauthplayground/ which is perfect for testing our OAuth2 integration!

---

## Method 1: Get Access Token from Playground (Quickest)

### Step 1: Get an Access Token

1. **Go to**: https://developers.google.com/oauthplayground/

2. **Select API Scope**:
   - In the left panel, expand "Google OAuth2 API v2"
   - Check: `https://www.googleapis.com/auth/userinfo.email`
   - Click "Authorize APIs"

3. **Sign in** with your Google account

4. **Exchange authorization code for tokens**:
   - Click "Exchange authorization code for tokens"
   - You'll get an **Access Token** and **Refresh Token**

5. **Copy the Access Token** (it will look like: `ya29.a0AfB_byD...`)

### Step 2: Test in Pipeline Builder UI

1. **Open**: http://localhost:3000/pipeline-builder.html

2. **Create API Enrichment step**

3. **Configure as Bearer Token** (simpler for testing):
   ```
   Auth Type: Bearer Token
   Bearer Token: <paste the access token from playground>
   Endpoint: https://www.googleapis.com/oauth2/v1/userinfo
   Method: GET
   ```

4. **Click "🧪 Test API Endpoint"**

5. **You'll see your Google profile info!** ✅
   ```json
   {
     "id": "123456789",
     "email": "your-email@gmail.com",
     "verified_email": true,
     "picture": "https://..."
   }
   ```

---

## Method 2: Full OAuth2 Flow (More Advanced)

To test the **full OAuth2 client credentials flow**, you need to create your own Google Cloud project:

### Step 1: Create Google Cloud Project

1. Go to: https://console.cloud.google.com/
2. Create a new project (or use existing)
3. Enable "Google+ API" or "People API"

### Step 2: Create OAuth 2.0 Credentials

1. Go to **APIs & Services > Credentials**
2. Click **"Create Credentials" > "OAuth client ID"**
3. Application type: **Web application**
4. Add authorized redirect URI: `https://developers.google.com/oauthplayground`
5. Click **Create**
6. **Copy Client ID and Client Secret**

### Step 3: Configure OAuth Playground

1. Go to: https://developers.google.com/oauthplayground/
2. Click **Settings (gear icon)** in top right
3. Check **"Use your own OAuth credentials"**
4. Paste your **Client ID** and **Client Secret**
5. Close settings

### Step 4: Get Tokens

1. Select scope: `https://www.googleapis.com/auth/userinfo.email`
2. Click "Authorize APIs"
3. Sign in with Google
4. Click "Exchange authorization code for tokens"
5. **Copy Access Token and Refresh Token**

### Step 5: Test Full OAuth2 in Pipeline Builder

1. **Open**: http://localhost:3000/pipeline-builder.html

2. **Create API Enrichment step**

3. **Configure OAuth 2.0**:
   ```
   Auth Type: OAuth 2.0
   Grant Type: Refresh Token
   Token URL: https://oauth2.googleapis.com/token
   Client ID: <your-client-id>
   Client Secret: <your-client-secret>
   Refresh Token: <paste from playground>
   Scope: https://www.googleapis.com/auth/userinfo.email
   ```

4. **Click "Test Connection"** in OAuth2ConfigBuilder
   - You'll see the token exchange happen!
   - New access token obtained ✅

5. **Configure endpoint**:
   ```
   Endpoint: https://www.googleapis.com/oauth2/v1/userinfo
   Method: GET
   ```

6. **Click "🧪 Test API Endpoint"**
   - Full OAuth2 flow executes
   - Token automatically obtained and cached
   - API called successfully ✅

---

## Method 3: Client Credentials (Service Account)

For true **client_credentials** grant type (machine-to-machine):

### Step 1: Create Service Account

1. Go to: https://console.cloud.google.com/
2. **IAM & Admin > Service Accounts**
3. Click **"Create Service Account"**
4. Give it a name and create
5. Click on the service account
6. Go to **"Keys"** tab
7. Click **"Add Key" > "Create new key" > JSON**
8. Download the JSON file

### Step 2: Use Service Account JWT

Service accounts use JWT assertions instead of client credentials. This requires a different flow.

**For simplicity, use Method 1 or Method 2 above!**

---

## Expected Results

### Success Response:
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
    }
  },
  "response": {
    "status_code": 200,
    "body_parsed": {
      "id": "123456789",
      "email": "your-email@gmail.com",
      "verified_email": true,
      "picture": "https://lh3.googleusercontent.com/..."
    },
    "field_structure": [
      {"path": "$.id", "type": "string", "sample": "123456789"},
      {"path": "$.email", "type": "string", "sample": "your-email@gmail.com"},
      {"path": "$.verified_email", "type": "boolean", "sample": "true"}
    ]
  }
}
```

### In UI:
- ✅ See clickable fields from Google profile
- ✅ Add fields to response mapping with one click
- ✅ Token caching works (2nd call instant)

### In Docker Logs:
```bash
docker-compose logs -f app | grep OAuth

# First call:
🔐 Requesting new OAuth2 token from https://oauth2.googleapis.com/token
✅ OAuth2 token obtained successfully

# Second call:
♻️  Using cached OAuth2 token (expires: 2025-12-20 18:40:00)
```

---

## Recommended: Start with Method 1

**Method 1 is the easiest** - just get a token from the playground and paste it as a Bearer token. This proves the entire system works end-to-end!

Once that works, you can try Method 2 for full OAuth2 flow testing.

---

**Sources**:
- [Google OAuth 2.0 Playground](https://developers.google.com/oauthplayground/)
- [Google Cloud Console](https://console.cloud.google.com/)
- [OAuth 2.0 for Web Server Applications](https://developers.google.com/identity/protocols/oauth2/web-server)
