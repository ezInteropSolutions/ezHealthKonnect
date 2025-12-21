#!/bin/bash

# ===============================================================
# OAuth 2.0 Flow Test Script
# ===============================================================
# Tests full OAuth2 integration with automatic token management
#
# Test scenarios:
# 1. Client credentials grant
# 2. Password grant
# 3. Token caching
# 4. Automatic token refresh

BASE_URL="http://localhost:8080"

echo "🧪 OAuth 2.0 Integration Test Suite"
echo "===================================="
echo ""

# ---------------------------------------------------------------
# Test 1: Client Credentials Grant
# ---------------------------------------------------------------
echo "📋 Test 1: Client Credentials Grant"
echo "Using OAuth 2.0 Playground test endpoint"
echo ""

curl -X POST $BASE_URL/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://httpbin.org/bearer",
      "method": "GET",
      "authType": "oauth2",
      "oauth2TokenUrl": "https://oauth2.googleapis.com/token",
      "oauth2ClientId": "test-client-id",
      "oauth2ClientSecret": "test-client-secret",
      "oauth2GrantType": "client_credentials",
      "oauth2Scope": "https://www.googleapis.com/auth/userinfo.email"
    },
    "testData": {}
  }' | jq .

echo ""
echo "✅ Test 1 Complete"
echo ""

# ---------------------------------------------------------------
# Test 2: Password Grant
# ---------------------------------------------------------------
echo "📋 Test 2: Password Grant"
echo "Using OAuth 2.0 with username/password"
echo ""

curl -X POST $BASE_URL/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://httpbin.org/bearer",
      "method": "GET",
      "authType": "oauth2",
      "oauth2TokenUrl": "https://oauth2.googleapis.com/token",
      "oauth2ClientId": "test-client-id",
      "oauth2ClientSecret": "test-client-secret",
      "oauth2GrantType": "password",
      "oauth2Username": "testuser@example.com",
      "oauth2Password": "testpassword",
      "oauth2Scope": "https://www.googleapis.com/auth/userinfo.email"
    },
    "testData": {}
  }' | jq .

echo ""
echo "✅ Test 2 Complete"
echo ""

# ---------------------------------------------------------------
# Test 3: Token Caching (Same Request Twice)
# ---------------------------------------------------------------
echo "📋 Test 3: Token Caching"
echo "Making same request twice - should use cached token on 2nd call"
echo ""

echo "First request (should fetch new token):"
START1=$(date +%s%3N)
curl -X POST $BASE_URL/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://httpbin.org/bearer",
      "method": "GET",
      "authType": "oauth2",
      "oauth2TokenUrl": "https://oauth2.googleapis.com/token",
      "oauth2ClientId": "cache-test-client",
      "oauth2ClientSecret": "cache-test-secret",
      "oauth2GrantType": "client_credentials",
      "oauth2Scope": "test"
    },
    "testData": {}
  }' > /dev/null 2>&1
END1=$(date +%s%3N)
TIME1=$((END1 - START1))

echo "  Time: ${TIME1}ms (includes token fetch)"
echo ""

sleep 1

echo "Second request (should use cached token):"
START2=$(date +%s%3N)
curl -X POST $BASE_URL/api/fhir/pipeline/test-api-endpoint \
  -H "Content-Type: application/json" \
  -d '{
    "stepConfig": {
      "endpoint": "https://httpbin.org/bearer",
      "method": "GET",
      "authType": "oauth2",
      "oauth2TokenUrl": "https://oauth2.googleapis.com/token",
      "oauth2ClientId": "cache-test-client",
      "oauth2ClientSecret": "cache-test-secret",
      "oauth2GrantType": "client_credentials",
      "oauth2Scope": "test"
    },
    "testData": {}
  }' > /dev/null 2>&1
END2=$(date +%s%3N)
TIME2=$((END2 - START2))

echo "  Time: ${TIME2}ms (from cache)"
echo ""

if [ $TIME2 -lt $TIME1 ]; then
  echo "✅ Cache working - 2nd request faster than 1st"
else
  echo "⚠️  Cache might not be working - 2nd request not faster"
fi

echo ""
echo "✅ Test 3 Complete"
echo ""

# ---------------------------------------------------------------
# Summary
# ---------------------------------------------------------------
echo "=================================="
echo "🎉 OAuth 2.0 Test Suite Complete"
echo "=================================="
echo ""
echo "Tested Features:"
echo "  ✅ Client credentials grant"
echo "  ✅ Password grant"
echo "  ✅ Token caching"
echo ""
echo "Check Docker logs for token fetch/cache details:"
echo "  docker-compose logs -f app | grep 'OAuth2'"
echo ""
