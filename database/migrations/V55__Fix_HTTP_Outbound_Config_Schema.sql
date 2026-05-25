-- V55: Fix http_outbound connector config_schema
-- Adds missing credential fields (username, password, bearer_token, api_key)
-- Fixes parameter_groups with conditional auth field grouping
-- Aligns field names with Go connector implementation

UPDATE connectivity_types
SET config_schema = '{
  "type": "object",
  "required": ["url"],
  "properties": {
    "url":               {"type": "string",  "title": "Destination URL",        "format": "uri"},
    "method":            {"type": "string",  "title": "HTTP Method",            "enum": ["POST", "PUT", "PATCH"], "default": "POST"},
    "content_type":      {"type": "string",  "title": "Content-Type",           "default": "application/json"},
    "timeout_seconds":   {"type": "integer", "title": "Timeout (seconds)",      "default": 30},
    "retry_attempts":    {"type": "integer", "title": "Retry Attempts",         "default": 3},
    "retry_delay_seconds": {"type": "integer", "title": "Retry Delay (seconds)", "default": 1},
    "authentication_type": {"type": "string", "title": "Authentication",
      "enum": ["none", "basic_auth", "bearer_token", "api_key"], "default": "none"},
    "username":          {"type": "string",  "title": "Username"},
    "password":          {"type": "string",  "title": "Password",       "format": "password"},
    "bearer_token":      {"type": "string",  "title": "Bearer Token",   "format": "password"},
    "api_key":           {"type": "string",  "title": "API Key",        "format": "password"},
    "api_key_header":    {"type": "string",  "title": "API Key Header", "default": "X-API-Key"}
  },
  "parameter_groups": {
    "basic":    ["url", "method", "content_type"],
    "advanced": ["timeout_seconds", "retry_attempts", "retry_delay_seconds"],
    "security": ["authentication_type", "username", "password", "bearer_token", "api_key", "api_key_header"]
  }
}'::jsonb
WHERE type_name = 'http_outbound';
