# FHIR Receiver Architecture

**Date**: October 26, 2025
**Status**: 🏗️ DESIGN PHASE
**Context**: User requirement - "we do have to use the same tables as we will for other source, capture raw messages in mongo and meta in postgres"

---

## Core Principle: Unified Message Storage

**SAME ARCHITECTURE FOR ALL SOURCES** (HL7, FHIR, Database, File, etc.):
- ✅ PostgreSQL: Metadata in interface-specific tables (`messages_intf_<id>`)
- ✅ MongoDB: Raw content in interface-specific collections (`raw_messages_intf_<id>`)
- ✅ Transformation pipeline applies to all
- ✅ Output delivery works identically

---

## FHIR Receiver Flow

```
Incoming FHIR Resource (HTTP POST)
    ↓
1. HTTP Endpoint: POST /fhir/receiver/<interface-id>
    ↓
2. Authentication Check (Bearer Token, Basic Auth, OAuth)
    ↓
3. Store Raw in MongoDB
   - Collection: raw_messages_intf_<interface-id>
   - Document: { message_id, raw_content: {...FHIR JSON...}, received_at, source: "fhir_http" }
    ↓
4. Store Metadata in PostgreSQL
   - Table: messages_intf_<interface-id>
   - Columns: message_id, status, received_at, source_type="fhir_http", message_type, etc.
    ↓
5. Trigger Transformation Pipeline (if configured)
    ↓
6. Deliver to Destination (FHIR server, database, etc.)
```

---

## Step 1: Interface Configuration (FHIR Receiver)

### User selects:
- **Source Type**: FHIR Receiver (HTTP Endpoint)
- **Target Type**: FHIR Server / Database / File / etc.

### Configuration Needed:

#### A. Receiving Endpoint Configuration
```json
{
  "source_connectivity": {
    "type": "fhir_http_receiver",
    "endpoint_path": "/fhir/receiver/fafc66da-995a-46e4-b00d-330a9d62a0e0",
    "http_method": "POST",
    "accepted_resource_types": ["Patient", "Observation", "Encounter", ...],
    "authentication": {
      "enabled": true,
      "type": "bearer_token", // or "basic_auth", "oauth2", "none"
      "token": "encrypted_token_here",
      "username": null,
      "password": null,
      "oauth_config": null
    },
    "validation": {
      "validate_fhir_schema": true,
      "fhir_version": "R4",
      "reject_invalid": false // Store invalid but flag them
    },
    "rate_limiting": {
      "enabled": true,
      "max_requests_per_minute": 100
    }
  }
}
```

#### B. Output Configuration (Step 5 - already implemented)
```json
{
  "target_connectivity": {
    "type": "fhir_http",
    "base_url": "https://destination-fhir.example.com/fhir/r4",
    "delivery_mode": "bundle",
    "resource_selection": { ... },
    "authentication": { ... }
  }
}
```

---

## UI Design: Step 1 - FHIR Receiver Configuration

### When Source Type = "FHIR Receiver", show:

```html
<!-- FHIR Receiver Configuration Section -->
<div id="fhirReceiverConfig" style="display: none;">
    <h4>🌐 FHIR Receiver Endpoint</h4>

    <!-- Generated Endpoint URL (Read-only) -->
    <div class="config-section">
        <label>Your FHIR Endpoint URL</label>
        <div class="endpoint-display">
            <input type="text" readonly
                   value="https://your-domain.com/fhir/receiver/{interface-id}"
                   id="fhirReceiverEndpoint">
            <button onclick="copyToClipboard('fhirReceiverEndpoint')">📋 Copy</button>
        </div>
        <small>This endpoint will be generated after interface creation</small>
    </div>

    <!-- Accepted HTTP Methods -->
    <div class="config-section">
        <label>HTTP Methods</label>
        <div class="checkbox-group">
            <label><input type="checkbox" checked disabled> POST</label>
            <label><input type="checkbox"> PUT</label>
            <label><input type="checkbox"> PATCH</label>
        </div>
    </div>

    <!-- Authentication Configuration -->
    <div class="config-section">
        <label>Authentication</label>
        <select id="fhirReceiverAuthType">
            <option value="none">None (Not Recommended)</option>
            <option value="bearer_token" selected>Bearer Token</option>
            <option value="basic_auth">Basic Authentication</option>
            <option value="oauth2">OAuth 2.0</option>
            <option value="api_key">API Key (Header)</option>
        </select>
    </div>

    <!-- Bearer Token (Auto-generated or Custom) -->
    <div id="fhirReceiverBearerTokenSection">
        <label>Bearer Token</label>
        <div class="token-input-group">
            <input type="text" id="fhirReceiverBearerToken"
                   placeholder="Auto-generated on save" readonly>
            <button onclick="generateBearerToken()">🔄 Generate</button>
            <button onclick="toggleTokenEdit()">✏️ Custom</button>
        </div>
        <small>⚠️ Save this token securely - you'll need it to send FHIR resources to this endpoint</small>
    </div>

    <!-- OAuth 2.0 Config (Conditional) -->
    <div id="fhirReceiverOAuthSection" style="display: none;">
        <label>OAuth 2.0 Provider</label>
        <select>
            <option>Epic</option>
            <option>Cerner</option>
            <option>Azure AD</option>
            <option>Custom</option>
        </select>
        <!-- More OAuth fields... -->
    </div>

    <!-- Resource Type Filter -->
    <div class="config-section">
        <label>Accepted FHIR Resource Types</label>
        <select id="fhirReceiverResourceFilter" multiple>
            <option value="all" selected>All Resource Types</option>
            <option value="Patient">Patient</option>
            <option value="Observation">Observation</option>
            <option value="Encounter">Encounter</option>
            <!-- ... more resources ... -->
        </select>
        <small>Select specific resources or accept all</small>
    </div>

    <!-- FHIR Validation -->
    <div class="config-section">
        <label>
            <input type="checkbox" id="validateFhirSchema" checked>
            Validate FHIR Schema
        </label>
        <label>
            <input type="checkbox" id="rejectInvalidFhir">
            Reject Invalid FHIR (or store with error flag)
        </label>
    </div>

    <!-- Rate Limiting -->
    <div class="config-section">
        <label>
            <input type="checkbox" id="enableRateLimit" checked>
            Enable Rate Limiting
        </label>
        <div id="rateLimitConfig">
            <label>Max Requests per Minute</label>
            <input type="number" value="100" min="1" max="10000">
        </div>
    </div>
</div>
```

---

## Backend: HTTP Endpoint Handler

### New Route: POST /fhir/receiver/:interfaceId

```javascript
// routes/fhirReceiverRoutes.js
const express = require('express');
const router = express.Router();
const FhirReceiverController = require('../controllers/FhirReceiverController');

// FHIR Receiver endpoint
router.post('/receiver/:interfaceId', FhirReceiverController.receiveResource);
router.put('/receiver/:interfaceId/:resourceType/:resourceId', FhirReceiverController.updateResource);

module.exports = router;
```

### Controller: FhirReceiverController.js

```javascript
// controllers/FhirReceiverController.js
const InterfaceService = require('../services/interfaceService');
const MongoDBService = require('../services/MongoDBConnectionService');
const { v4: uuidv4 } = require('uuid');

class FhirReceiverController {
    async receiveResource(req, res) {
        const { interfaceId } = req.params;
        const fhirResource = req.body;

        try {
            // 1. Load interface configuration
            const iface = await InterfaceService.getInterfaceById(interfaceId);
            if (!iface || iface.source_type !== 'fhir_receiver') {
                return res.status(404).json({
                    error: 'FHIR receiver interface not found'
                });
            }

            // 2. Check if interface is active
            if (iface.status !== 'active') {
                return res.status(503).json({
                    error: 'Interface is not active'
                });
            }

            // 3. Authenticate request
            const authResult = await this.authenticateRequest(req, iface.source_connectivity);
            if (!authResult.success) {
                return res.status(401).json({
                    error: 'Authentication failed',
                    details: authResult.error
                });
            }

            // 4. Validate FHIR resource (if enabled)
            if (iface.source_connectivity.validation?.validate_fhir_schema) {
                const validationResult = await this.validateFhirResource(fhirResource);
                if (!validationResult.valid && iface.source_connectivity.validation.reject_invalid) {
                    return res.status(400).json({
                        error: 'Invalid FHIR resource',
                        details: validationResult.errors
                    });
                }
            }

            // 5. Generate message ID
            const messageId = `fhir_${Date.now()}${Math.floor(Math.random() * 1000000)}`;

            // 6. Store raw FHIR in MongoDB
            const mongoService = await MongoDBService.getInstance();
            await mongoService.storeRawMessage(interfaceId, {
                message_id: messageId,
                raw_content: fhirResource,
                received_at: new Date(),
                source_type: 'fhir_http',
                source_endpoint: req.path,
                source_ip: req.ip
            });

            // 7. Store metadata in PostgreSQL
            const db = await getDatabase();
            const tableName = `messages_intf_${interfaceId.replace(/-/g, '_')}`;

            await db.query(`
                INSERT INTO ${tableName} (
                    message_id, interface_id, status, received_at,
                    source_type, source_endpoint, source_ip,
                    message_type, message_size, raw_message
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
            `, [
                messageId,
                interfaceId,
                'received',
                new Date(),
                'fhir_http',
                req.path,
                req.ip,
                fhirResource.resourceType || 'Unknown',
                JSON.stringify(fhirResource).length,
                JSON.stringify(fhirResource) // Fallback if MongoDB fails
            ]);

            // 8. Trigger async processing (transformation pipeline)
            // TODO: Call Go backend processing engine
            // await triggerProcessingPipeline(interfaceId, messageId);

            // 9. Return FHIR OperationOutcome success
            res.status(201).json({
                resourceType: 'OperationOutcome',
                issue: [{
                    severity: 'information',
                    code: 'informational',
                    diagnostics: `Resource received successfully. Message ID: ${messageId}`
                }],
                messageId: messageId
            });

        } catch (error) {
            console.error('FHIR receiver error:', error);
            res.status(500).json({
                resourceType: 'OperationOutcome',
                issue: [{
                    severity: 'error',
                    code: 'exception',
                    diagnostics: error.message
                }]
            });
        }
    }

    async authenticateRequest(req, authConfig) {
        if (!authConfig.authentication?.enabled) {
            return { success: true };
        }

        const authType = authConfig.authentication.type;

        switch (authType) {
            case 'bearer_token':
                const authHeader = req.headers.authorization;
                if (!authHeader || !authHeader.startsWith('Bearer ')) {
                    return { success: false, error: 'Missing Bearer token' };
                }
                const token = authHeader.substring(7);
                if (token !== authConfig.authentication.token) {
                    return { success: false, error: 'Invalid Bearer token' };
                }
                return { success: true };

            case 'basic_auth':
                // Implement Basic Auth
                break;

            case 'oauth2':
                // Implement OAuth 2.0 validation
                break;

            case 'api_key':
                // Implement API Key validation
                break;

            default:
                return { success: false, error: 'Unknown auth type' };
        }
    }

    async validateFhirResource(resource) {
        // TODO: Use FHIR validation library
        // For now, basic validation
        if (!resource.resourceType) {
            return {
                valid: false,
                errors: ['Missing resourceType']
            };
        }
        return { valid: true };
    }
}

module.exports = new FhirReceiverController();
```

---

## Database Migration: V30

```sql
-- V30__Add_FHIR_Receiver_Support.sql

-- Add source_connectivity and target_connectivity columns to interfaces table
ALTER TABLE interfaces
ADD COLUMN IF NOT EXISTS source_connectivity JSONB DEFAULT '{}'::jsonb,
ADD COLUMN IF NOT EXISTS target_connectivity JSONB DEFAULT '{}'::jsonb;

-- Migrate existing connectivity data to source_connectivity
-- (For backward compatibility with existing interfaces)
UPDATE interfaces
SET source_connectivity = jsonb_build_object(
    'type', COALESCE(source_type, 'unknown'),
    'config', COALESCE(connectivity_config, '{}'::jsonb)
)
WHERE source_connectivity = '{}'::jsonb;

-- Create indexes for JSONB queries
CREATE INDEX IF NOT EXISTS idx_interfaces_source_connectivity
  ON interfaces USING GIN (source_connectivity);

CREATE INDEX IF NOT EXISTS idx_interfaces_target_connectivity
  ON interfaces USING GIN (target_connectivity);

-- Add comments
COMMENT ON COLUMN interfaces.source_connectivity IS
  'Source/Input connectivity configuration: type, authentication, validation, etc.';

COMMENT ON COLUMN interfaces.target_connectivity IS
  'Target/Output connectivity configuration: destination, delivery mode, authentication, etc.';
```

---

## Unified Interface Schema

```json
{
  "interface": {
    "id": "fafc66da-995a-46e4-b00d-330a9d62a0e0",
    "name": "Epic FHIR Receiver → Internal DB",
    "source_type": "fhir_receiver",
    "target_type": "database",

    "source_connectivity": {
      "type": "fhir_http_receiver",
      "endpoint_path": "/fhir/receiver/fafc66da-995a-46e4-b00d-330a9d62a0e0",
      "http_method": "POST",
      "accepted_resource_types": ["Patient", "Observation", "Encounter"],
      "authentication": {
        "enabled": true,
        "type": "bearer_token",
        "token": "encrypted_bearer_token_here"
      },
      "validation": {
        "validate_fhir_schema": true,
        "fhir_version": "R4",
        "reject_invalid": false
      },
      "rate_limiting": {
        "enabled": true,
        "max_requests_per_minute": 100
      }
    },

    "target_connectivity": {
      "type": "postgresql_outbound",
      "host": "internal-db.example.com",
      "port": 5432,
      "database": "clinical_data",
      "table": "fhir_resources"
    },

    "transformation_pipeline": {
      "enabled": true,
      "steps": [
        {
          "sequence": 10,
          "step_type": "validation",
          "config": { ... }
        }
      ]
    }
  }
}
```

---

## Key Architectural Decisions

### 1. Same Storage Layer ✅
- PostgreSQL: `messages_intf_<id>` table (metadata)
- MongoDB: `raw_messages_intf_<id>` collection (raw FHIR JSON)
- **NO DIFFERENCE** between HL7, FHIR, or other sources at storage layer

### 2. Authentication at Source ✅
- Bearer Token (auto-generated or custom)
- Basic Auth (username/password)
- OAuth 2.0 (Epic, Cerner, Azure AD)
- API Key (header-based)
- None (for testing only)

### 3. Endpoint Pattern ✅
- `/fhir/receiver/<interface-id>` - Receive FHIR resource
- `/fhir/receiver/<interface-id>/<resourceType>/<resourceId>` - Update specific resource

### 4. FHIR-Specific Features ✅
- Resource type filtering (accept only Patient, Observation, etc.)
- FHIR schema validation (optional)
- Rate limiting per interface
- FHIR OperationOutcome responses

### 5. Processing Pipeline ✅
- Same transformation pipeline for all sources
- Step 1: Receive → Store
- Step 2: Transform (if configured)
- Step 3: Deliver to destination

---

## Next Implementation Steps

1. ✅ Create V30 database migration
2. ✅ Add FHIR Receiver UI to Step 1 (conditional display)
3. ✅ Create FhirReceiverController.js
4. ✅ Create /fhir/receiver/:interfaceId route
5. ✅ Add bearer token generation utility
6. ✅ Update wizardController.js to save source_connectivity
7. ✅ Test FHIR receiver with Postman/curl
8. ✅ Wire to Go processing engine

---

## Example: Testing FHIR Receiver

```bash
# After creating interface with FHIR Receiver source
curl -X POST https://your-domain.com/fhir/receiver/fafc66da-995a-46e4-b00d-330a9d62a0e0 \
  -H "Authorization: Bearer your-generated-token" \
  -H "Content-Type: application/fhir+json" \
  -d '{
    "resourceType": "Patient",
    "id": "example",
    "name": [{
      "family": "Doe",
      "given": ["John"]
    }]
  }'

# Response:
{
  "resourceType": "OperationOutcome",
  "issue": [{
    "severity": "information",
    "code": "informational",
    "diagnostics": "Resource received successfully. Message ID: fhir_1730000000123456"
  }],
  "messageId": "fhir_1730000000123456"
}
```

---

**Last Updated**: October 26, 2025
**Status**: 🏗️ Architecture Design Complete → Ready for Implementation
