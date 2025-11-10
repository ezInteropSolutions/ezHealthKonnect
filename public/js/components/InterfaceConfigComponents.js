/**
 * InterfaceConfigComponents.js
 * Shared Interface Configuration Components Library
 *
 * Single source of truth for source/target configuration forms
 * Used by both Wizard and Edit Modal to ensure consistency
 *
 * Design Principles:
 * 1. All forms generated from shared methods
 * 2. ID prefix strategy to avoid conflicts (wizard_, edit, etc.)
 * 3. Consistent behavior across all usages
 * 4. Easy to maintain and extend
 */

class InterfaceConfigComponents {

    /**
     * Get source type selector dropdown
     * @param {string} selectedValue - Currently selected value
     * @param {object} config - Configuration options (idPrefix, showHint, etc.)
     */
    static getSourceTypeSelector(selectedValue = '', config = {}) {
        const idPrefix = config.idPrefix || '';
        return `
            <div class="form-group">
                <label for="${idPrefix}sourceType" class="form-label required">
                    Source Format
                </label>
                <select id="${idPrefix}sourceType"
                        class="form-control source-type-selector"
                        name="sourceType"
                        required>
                    <option value="">Select source format...</option>
                    <option value="hl7v2" ${selectedValue === 'hl7v2' ? 'selected' : ''}>HL7 v2.x (ADT, ORU, etc.)</option>
                    <option value="fhir" ${selectedValue === 'fhir' ? 'selected' : ''}>FHIR (R4, STU3)</option>
                    <option value="cda" ${selectedValue === 'cda' ? 'selected' : ''}>CDA/C-CDA Documents</option>
                    <option value="x12" ${selectedValue === 'x12' ? 'selected' : ''}>X12 (Claims)</option>
                    <option value="json" ${selectedValue === 'json' ? 'selected' : ''}>JSON</option>
                    <option value="xml" ${selectedValue === 'xml' ? 'selected' : ''}>XML</option>
                    <option value="csv" ${selectedValue === 'csv' ? 'selected' : ''}>CSV</option>
                </select>
                ${config.showHint !== false ? '<div class="form-hint">💡 OOB: HL7 v2.x is most common in healthcare</div>' : ''}
            </div>
        `;
    }

    /**
     * Get source connectivity selector dropdown
     * @param {string} selectedValue - Currently selected value
     * @param {object} config - Configuration options
     */
    static getSourceConnectivitySelector(selectedValue = '', config = {}) {
        const idPrefix = config.idPrefix || '';
        return `
            <div class="form-group">
                <label for="${idPrefix}sourceConnectivity" class="form-label required">
                    Connectivity
                </label>
                <select id="${idPrefix}sourceConnectivity"
                        class="form-control source-connectivity-selector"
                        name="sourceConnectivity"
                        required>
                    <option value="">Select connectivity...</option>
                    <option value="tcp" ${selectedValue === 'tcp' ? 'selected' : ''}>TCP/MLLP (HL7 Standard)</option>
                    <option value="http" ${selectedValue === 'http' ? 'selected' : ''}>HTTP/REST (FHIR Standard)</option>
                    <option value="file" ${selectedValue === 'file' ? 'selected' : ''}>File Listener</option>
                    <option value="database" ${selectedValue === 'database' ? 'selected' : ''}>Database Query</option>
                    <option value="sftp" ${selectedValue === 'sftp' ? 'selected' : ''}>SFTP</option>
                    <option value="rabbitmq" ${selectedValue === 'rabbitmq' ? 'selected' : ''}>RabbitMQ</option>
                    <option value="kafka" ${selectedValue === 'kafka' ? 'selected' : ''}>Apache Kafka</option>
                </select>
                ${config.showHint !== false ? '<div class="form-hint">💡 OOB: TCP/MLLP for HL7, HTTP for FHIR</div>' : ''}
            </div>
        `;
    }

    /**
     * Get source configuration panel (dynamic based on connectivity)
     * @param {string} connectivity - Connectivity type (tcp, http, etc.)
     * @param {string} sourceType - Source format type (hl7v2, fhir, etc.)
     * @param {object} config - Configuration values
     * @param {object} formConfig - Form configuration (idPrefix, etc.)
     */
    static getSourceConfigPanel(connectivity, sourceType, config = {}, formConfig = {}) {
        const idPrefix = formConfig.idPrefix || '';

        switch (connectivity) {
            case 'tcp':
                return this.getTcpMllpConfig('source', config, idPrefix);

            case 'http':
                const isFhirReceiver = (sourceType === 'fhir');
                if (isFhirReceiver) {
                    return this.getFhirReceiverConfig(config, idPrefix);
                } else {
                    // Universal HTTP Receiver (for HL7, JSON, XML, custom payloads)
                    return this.getHttpReceiverConfig(config, idPrefix);
                }

            case 'file':
                return this.getFileConfig('source', config, idPrefix);

            case 'database':
                return this.getDatabaseConfig('source', config, idPrefix);

            case 'sftp':
                return this.getSftpConfig('source', config, idPrefix);

            case 'rabbitmq':
                return this.getRabbitMQConfig('source', config, idPrefix);

            case 'kafka':
                return this.getKafkaConfig('source', config, idPrefix);

            default:
                return '<div class="config-placeholder">Select connectivity type to configure</div>';
        }
    }

    /**
     * TCP/MLLP Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getTcpMllpConfig(direction = 'source', config = {}, idPrefix = '') {
        const isSource = direction === 'source';
        const prefix = idPrefix + direction;
        const label = direction.charAt(0).toUpperCase() + direction.slice(1);

        return `
            <div class="config-group">
                <h4>TCP/MLLP Configuration</h4>
                ${isSource ? `
                    <div class="form-row">
                        <div class="form-group">
                            <label for="${prefix}Host" class="form-label required">Host</label>
                            <input type="text" id="${prefix}Host" class="form-control"
                                   value="${config.host || 'localhost'}"
                                   placeholder="localhost"
                                   name="${direction}Host">
                            <div class="form-hint">💡 OOB: localhost for development</div>
                        </div>
                        <div class="form-group">
                            <label for="${prefix}Port" class="form-label required">Port</label>
                            <input type="number" id="${prefix}Port" class="form-control"
                                   value="${config.port || 2575}"
                                   min="1" max="65535"
                                   placeholder="2575"
                                   name="${direction}Port">
                            <div class="form-hint">💡 OOB: 2575 is standard HL7 port</div>
                        </div>
                    </div>
                ` : `
                    <div class="form-group">
                        <label for="${prefix}Endpoint" class="form-label required">Remote Endpoint</label>
                        <input type="text" id="${prefix}Endpoint" class="form-control"
                               value="${config.endpoint || 'localhost:2575'}"
                               placeholder="hostname:port"
                               name="${direction}Endpoint">
                        <div class="form-hint">Format: hostname:port or IP:port</div>
                    </div>
                `}
                <div class="form-row">
                    <div class="form-group">
                        <label for="${prefix}Timeout" class="form-label">Timeout (ms)</label>
                        <input type="number" id="${prefix}Timeout" class="form-control"
                               value="${config.timeout || 30000}"
                               min="1000" max="300000"
                               name="${direction}Timeout">
                        <div class="form-hint">Connection timeout in milliseconds</div>
                    </div>
                    <div class="form-group">
                        <label for="${prefix}Encoding" class="form-label">Encoding</label>
                        <select id="${prefix}Encoding" class="form-control" name="${direction}Encoding">
                            <option value="utf8" ${!config.encoding || config.encoding === 'utf8' ? 'selected' : ''}>UTF-8</option>
                            <option value="ascii" ${config.encoding === 'ascii' ? 'selected' : ''}>ASCII</option>
                            <option value="latin1" ${config.encoding === 'latin1' ? 'selected' : ''}>Latin-1</option>
                        </select>
                    </div>
                </div>
                ${isSource ? `
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" id="${prefix}TlsEnabled" name="${direction}TlsEnabled"
                                   ${config.tlsEnabled ? 'checked' : ''}>
                            <span>Enable TLS/SSL Encryption</span>
                        </label>
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * HTTP/REST Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getHttpConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;
        const isSource = direction === 'source';

        return `
            <div class="config-group">
                <h4>HTTP/REST Configuration</h4>
                <div class="form-group">
                    <label for="${prefix}Endpoint" class="form-label required">Endpoint URL</label>
                    <input type="url" id="${prefix}Endpoint" class="form-control"
                           value="${config.endpoint || (isSource ? 'http://localhost:3000/api/hl7/receive' : 'http://localhost:8080/fhir')}"
                           placeholder="${isSource ? 'http://localhost:3000/api/hl7/receive' : 'http://localhost:8080/fhir'}"
                           name="${direction}Endpoint">
                    <div class="form-hint">${isSource ? 'URL where messages will be received' : 'Target FHIR server URL'}</div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${prefix}Method" class="form-label">HTTP Method</label>
                        <select id="${prefix}Method" class="form-control" name="${direction}Method">
                            <option value="POST" ${!config.method || config.method === 'POST' ? 'selected' : ''}>POST</option>
                            <option value="PUT" ${config.method === 'PUT' ? 'selected' : ''}>PUT</option>
                            ${isSource ? '<option value="GET" ' + (config.method === 'GET' ? 'selected' : '') + '>GET</option>' : ''}
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="${prefix}ContentType" class="form-label">Content Type</label>
                        <select id="${prefix}ContentType" class="form-control" name="${direction}ContentType">
                            <option value="text/plain" ${config.contentType === 'text/plain' ? 'selected' : ''}>text/plain</option>
                            <option value="application/hl7-v2" ${config.contentType === 'application/hl7-v2' ? 'selected' : ''}>application/hl7-v2</option>
                            <option value="application/json" ${!config.contentType || config.contentType === 'application/json' ? 'selected' : ''}>application/json</option>
                            <option value="application/fhir+json" ${config.contentType === 'application/fhir+json' ? 'selected' : ''}>application/fhir+json</option>
                            <option value="application/xml" ${config.contentType === 'application/xml' ? 'selected' : ''}>application/xml</option>
                        </select>
                    </div>
                </div>

                <!-- HTTP Authentication (Universal) -->
                ${this.getHttpAuthConfig(config, prefix)}
            </div>
        `;
    }

    /**
     * HTTP Authentication Configuration (Universal for all HTTP connections)
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getHttpAuthConfig(config = {}, idPrefix = '') {
        console.log('🔐 getHttpAuthConfig called with:', { config, idPrefix });
        return `
            <div class="config-group http-auth-config">
                <h4>🔐 HTTP Authentication</h4>
                <div class="form-hint" style="background: #e7f3ff; padding: 8px; margin-bottom: 10px; border-left: 3px solid #0066cc;">
                    <strong>Secure your endpoint:</strong> Choose authentication based on your security requirements and client capabilities.
                </div>

                <div class="form-group">
                    <label for="${idPrefix}HttpAuthType" class="form-label required">
                        Authentication Type
                        <a href="#" class="help-link" onclick="event.preventDefault(); alert('Secure your HTTP endpoint:\\n\\n• No Auth - Development only\\n• API Key - Simple token in header\\n• Basic Auth - Username/password (use HTTPS!)\\n• Bearer Token - OAuth or JWT tokens\\n• OAuth 2.0 - Industry standard (SMART on FHIR)\\n• mTLS - Certificate-based, highest security\\n\\nProduction: OAuth 2.0 or mTLS');" title="Click for help">ⓘ</a>
                    </label>
                    <select id="${idPrefix}HttpAuthType" class="form-control http-auth-type-selector" name="authType">
                        <option value="none" ${!config.authType || config.authType === 'none' ? 'selected' : ''}>No Authentication (Development)</option>
                        <option value="api_key" ${config.authType === 'api_key' ? 'selected' : ''}>API Key (Header)</option>
                        <option value="basic" ${config.authType === 'basic' ? 'selected' : ''}>Basic Authentication</option>
                        <option value="bearer" ${config.authType === 'bearer' ? 'selected' : ''}>Bearer Token</option>
                        <option value="oauth2" ${config.authType === 'oauth2' ? 'selected' : ''}>OAuth 2.0</option>
                        <option value="mtls" ${config.authType === 'mtls' ? 'selected' : ''}>Mutual TLS (Certificate)</option>
                    </select>
                    <div class="form-hint">Choose based on security requirements and client capabilities</div>
                </div>

                <!-- Auth Details Panel (Dynamic based on selection) -->
                <div id="${idPrefix}HttpAuthDetails" class="auth-details-panel">
                    ${this.getHttpAuthDetailsPanel(config.authType || 'none', config, idPrefix)}
                </div>
            </div>
        `;
    }

    /**
     * HTTP Authentication Details Panel (Dynamic based on auth type)
     * @param {string} authType - Authentication type
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getHttpAuthDetailsPanel(authType, config = {}, idPrefix = '') {
        switch (authType) {
            case 'none':
                return `
                    <div class="auth-info">
                        <div class="alert alert-warning">
                            ⚠️ <strong>No Authentication</strong> - Endpoint is publicly accessible. Only use in development environments.
                        </div>
                    </div>
                `;

            case 'api_key':
                return `
                    <div class="auth-fields">
                        <div class="form-group">
                            <label for="${idPrefix}AuthApiKeyHeader" class="form-label">Header Name</label>
                            <input type="text" id="${idPrefix}AuthApiKeyHeader" class="form-control"
                                   value="${config.apiKeyHeader || 'X-API-Key'}"
                                   placeholder="X-API-Key"
                                   name="apiKeyHeader">
                            <div class="form-hint">Header name for API key (e.g., X-API-Key, Authorization)</div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthApiKeyValue" class="form-label">Expected API Key</label>
                            <input type="password" id="${idPrefix}AuthApiKeyValue" class="form-control"
                                   value="${config.apiKeyValue || ''}"
                                   placeholder="Enter API key"
                                   name="apiKeyValue">
                            <div class="form-hint">The API key value to validate against</div>
                        </div>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" id="${idPrefix}AuthApiKeyRequired" name="apiKeyRequired"
                                       ${config.apiKeyRequired !== false ? 'checked' : ''}>
                                <span>Reject requests without valid API key</span>
                            </label>
                        </div>
                    </div>
                `;

            case 'basic':
                return `
                    <div class="auth-fields">
                        <div class="alert alert-info">
                            ℹ️ Basic Authentication encodes credentials in Base64. <strong>Always use HTTPS</strong> to protect credentials in transit.
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="${idPrefix}AuthUsername" class="form-label">Username</label>
                                <input type="text" id="${idPrefix}AuthUsername" class="form-control"
                                       value="${config.username || ''}"
                                       placeholder="username"
                                       name="username">
                            </div>
                            <div class="form-group">
                                <label for="${idPrefix}AuthPassword" class="form-label">Password</label>
                                <input type="password" id="${idPrefix}AuthPassword" class="form-control"
                                       value="${config.password || ''}"
                                       placeholder="password"
                                       name="password">
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthRealm" class="form-label">Realm (Optional)</label>
                            <input type="text" id="${idPrefix}AuthRealm" class="form-control"
                                   value="${config.realm || ''}"
                                   placeholder="Restricted Area"
                                   name="realm">
                            <div class="form-hint">Displayed in browser authentication prompt</div>
                        </div>
                    </div>
                `;

            case 'bearer':
                return `
                    <div class="auth-fields">
                        <div class="form-group">
                            <label for="${idPrefix}AuthBearerToken" class="form-label">Bearer Token</label>
                            <textarea id="${idPrefix}AuthBearerToken" class="form-control" rows="3"
                                      placeholder="Enter bearer token (e.g., JWT)"
                                      name="bearerToken">${config.bearerToken || ''}</textarea>
                            <div class="form-hint">Token sent in Authorization: Bearer {token} header</div>
                        </div>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" id="${idPrefix}AuthValidateToken" name="validateToken"
                                       ${config.validateToken !== false ? 'checked' : ''}>
                                <span>Validate token signature (JWT only)</span>
                            </label>
                        </div>
                        <div id="${idPrefix}TokenValidationFields" style="display: ${config.validateToken !== false ? 'block' : 'none'};">
                            <div class="form-group">
                                <label for="${idPrefix}AuthTokenSecret" class="form-label">Token Secret / Public Key</label>
                                <textarea id="${idPrefix}AuthTokenSecret" class="form-control" rows="4"
                                          placeholder="Enter HMAC secret or RSA public key"
                                          name="tokenSecret">${config.tokenSecret || ''}</textarea>
                                <div class="form-hint">Secret for HMAC or public key for RSA signature validation</div>
                            </div>
                        </div>
                    </div>
                `;

            case 'oauth2':
                return `
                    <div class="auth-fields">
                        <div class="alert alert-info">
                            ℹ️ OAuth 2.0 configuration. For FHIR endpoints, this supports SMART on FHIR workflows.
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthOAuthIssuer" class="form-label">Token Issuer URL</label>
                            <input type="url" id="${idPrefix}AuthOAuthIssuer" class="form-control"
                                   value="${config.oauthIssuer || ''}"
                                   placeholder="https://auth.example.com"
                                   name="oauthIssuer">
                            <div class="form-hint">OAuth 2.0 authorization server URL</div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthOAuthAudience" class="form-label">Expected Audience</label>
                            <input type="text" id="${idPrefix}AuthOAuthAudience" class="form-control"
                                   value="${config.oauthAudience || ''}"
                                   placeholder="https://fhir.example.com"
                                   name="oauthAudience">
                            <div class="form-hint">Expected 'aud' claim in JWT token</div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthOAuthScopes" class="form-label">Required Scopes (comma-separated)</label>
                            <input type="text" id="${idPrefix}AuthOAuthScopes" class="form-control"
                                   value="${config.oauthScopes || ''}"
                                   placeholder="patient/*.read, user/*.write"
                                   name="oauthScopes">
                            <div class="form-hint">SMART on FHIR scopes (e.g., patient/*.read, user/*.write)</div>
                        </div>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" id="${idPrefix}AuthOAuthSmartEnabled" name="smartEnabled"
                                       ${config.smartEnabled ? 'checked' : ''}>
                                <span>Enable SMART on FHIR compliance</span>
                            </label>
                        </div>
                    </div>
                `;

            case 'mtls':
                return `
                    <div class="auth-fields">
                        <div class="alert alert-info">
                            ℹ️ Mutual TLS (mTLS) requires both server and client certificates. Highest security level.
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthMtlsServerCert" class="form-label">Server Certificate Path</label>
                            <input type="text" id="${idPrefix}AuthMtlsServerCert" class="form-control"
                                   value="${config.serverCertPath || ''}"
                                   placeholder="/path/to/server-cert.pem"
                                   name="serverCertPath">
                            <div class="form-hint">Path to server TLS certificate</div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthMtlsServerKey" class="form-label">Server Private Key Path</label>
                            <input type="text" id="${idPrefix}AuthMtlsServerKey" class="form-control"
                                   value="${config.serverKeyPath || ''}"
                                   placeholder="/path/to/server-key.pem"
                                   name="serverKeyPath">
                            <div class="form-hint">Path to server private key</div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}AuthMtlsCaCert" class="form-label">Client CA Certificate Path</label>
                            <input type="text" id="${idPrefix}AuthMtlsCaCert" class="form-control"
                                   value="${config.caCertPath || ''}"
                                   placeholder="/path/to/client-ca.pem"
                                   name="caCertPath">
                            <div class="form-hint">CA certificate to verify client certificates</div>
                        </div>
                        <div class="form-group">
                            <label class="checkbox-label">
                                <input type="checkbox" id="${idPrefix}AuthMtlsRequireClientCert" name="requireClientCert"
                                       ${config.requireClientCert !== false ? 'checked' : ''}>
                                <span>Require valid client certificate</span>
                            </label>
                        </div>
                    </div>
                `;

            default:
                return '';
        }
    }

    /**
     * HTTP Receiver Configuration (Universal HTTP listener for any payload)
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getHttpReceiverConfig(config = {}, idPrefix = '') {
        // Get reserved ports list
        const reservedPorts = [3000, 8080, 8081, 8090, 8091, 8092, 8093, 8094, 8095, 8096, 8097, 5432, 27017];
        const reservedPortsStr = reservedPorts.join(', ');

        return `
            <div class="config-group http-receiver-config">
                <h4>🌐 HTTP/REST Receiver Configuration</h4>
                <div class="form-hint" style="background: #fff3cd; padding: 10px; border-left: 4px solid #ffc107; margin-bottom: 15px;">
                    <strong>ℹ️ Universal HTTP Receiver</strong><br>
                    Accept any HTTP payload: HL7 messages, JSON, XML, custom formats, webhooks, etc.
                </div>

                <!-- Listener Configuration -->
                <div class="config-group" style="background: #f8f9fa; padding: 15px; border-radius: 6px; margin-bottom: 20px;">
                    <h5 style="margin-top: 0;">🎧 Listener Configuration</h5>
                    <div class="form-row">
                        <div class="form-group" style="flex: 2;">
                            <label for="${idPrefix}sourcePort" class="form-label required">
                                Listen Port
                                <a href="#" class="help-link" onclick="event.preventDefault(); alert('Port where ezHealthKonnect will LISTEN for incoming HTTP requests.\\n\\nReserved Ports (cannot use):\\n${reservedPortsStr}\\n\\nRecommended:\\n• 8082-8089 (HTTP receivers)\\n• 9000-9999 (additional services)\\n\\nExternal systems will POST/GET to:\\nhttp://your-server:[THIS PORT][CONTEXT PATH]');" title="Click for help">ⓘ</a>
                            </label>
                            <input type="number" id="${idPrefix}sourcePort" class="form-control"
                                   value="${config.port || '8082'}"
                                   placeholder="8082"
                                   min="1024"
                                   max="65535"
                                   required
                                   name="port">
                            <div class="form-hint">
                                ⚠️ <strong>Reserved</strong>: ${reservedPortsStr}<br>
                                ✅ <strong>Recommended</strong>: 8082-8089, 9000-9999
                            </div>
                        </div>
                        <div class="form-group" style="flex: 1;">
                            <label for="${idPrefix}sourceHost" class="form-label">Listen Host</label>
                            <input type="text" id="${idPrefix}sourceHost" class="form-control"
                                   value="${config.host || '0.0.0.0'}"
                                   placeholder="0.0.0.0"
                                   name="host">
                            <div class="form-hint">0.0.0.0 = all interfaces</div>
                        </div>
                    </div>
                    <div class="alert alert-info" style="margin-top: 12px; padding: 10px; background: #d1ecf1; border: 1px solid #bee5eb; border-radius: 4px; font-size: 13px;">
                        <strong>📡 External systems will send requests to:</strong><br>
                        <code style="background: #fff; padding: 2px 6px; border-radius: 3px;">http://your-server:<span id="${idPrefix}HttpPortPreview">${config.port || '8082'}</span><span id="${idPrefix}HttpContextPreview">${config.contextPath || '/api/receive'}</span></code>
                    </div>
                </div>

                <!-- Context Path Configuration -->
                <div class="form-group">
                    <label for="${idPrefix}httpContextPath" class="form-label required">
                        Context Path
                        <a href="#" class="help-link" onclick="event.preventDefault(); alert('URL path where messages will be received.\\n\\nExamples:\\n• /api/receive - Simple path\\n• /hl7/inbound - HL7-specific\\n• /webhook/{id} - With parameters\\n• /integration/epic - Partner-specific\\n\\nExternal systems POST to: http://your-server:port[THIS PATH]');" title="Click for help">ⓘ</a>
                    </label>
                    <input type="text" id="${idPrefix}httpContextPath" class="form-control"
                           value="${config.contextPath || '/api/receive'}"
                           placeholder="/api/receive"
                           required
                           name="contextPath">
                    <div class="form-hint">💡 URL path for incoming requests (e.g., /api/receive, /hl7/inbound, /webhook)</div>
                </div>

                <!-- HTTP Methods -->
                <div class="form-group">
                    <label class="form-label">
                        Allowed HTTP Methods
                        <a href="#" class="help-link" onclick="event.preventDefault(); alert('Which HTTP methods to accept:\\n\\n• POST - Most common for sending data\\n• PUT - Update/replace operations\\n• GET - Query/retrieve operations\\n• PATCH - Partial updates\\n\\nRecommendation: Start with POST only');" title="Click for help">ⓘ</a>
                    </label>
                    <div class="checkbox-group">
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}HttpMethodPost" name="httpMethodPost"
                                   ${!config.allowedMethods || config.allowedMethods.includes('POST') ? 'checked' : ''}>
                            <span>POST (Most Common)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}HttpMethodPut" name="httpMethodPut"
                                   ${config.allowedMethods?.includes('PUT') ? 'checked' : ''}>
                            <span>PUT</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}HttpMethodGet" name="httpMethodGet"
                                   ${config.allowedMethods?.includes('GET') ? 'checked' : ''}>
                            <span>GET</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}HttpMethodPatch" name="httpMethodPatch"
                                   ${config.allowedMethods?.includes('PATCH') ? 'checked' : ''}>
                            <span>PATCH</span>
                        </label>
                    </div>
                </div>

                <!-- Content Type -->
                <div class="form-group">
                    <label for="${idPrefix}httpContentType" class="form-label">Expected Content Type</label>
                    <select id="${idPrefix}httpContentType" class="form-control" name="contentType">
                        <option value="any" ${!config.contentType || config.contentType === 'any' ? 'selected' : ''}>Any (Accept All)</option>
                        <option value="application/json" ${config.contentType === 'application/json' ? 'selected' : ''}>application/json</option>
                        <option value="application/xml" ${config.contentType === 'application/xml' ? 'selected' : ''}>application/xml</option>
                        <option value="text/plain" ${config.contentType === 'text/plain' ? 'selected' : ''}>text/plain (HL7 v2)</option>
                        <option value="application/hl7-v2" ${config.contentType === 'application/hl7-v2' ? 'selected' : ''}>application/hl7-v2</option>
                        <option value="application/fhir+json" ${config.contentType === 'application/fhir+json' ? 'selected' : ''}>application/fhir+json</option>
                        <option value="application/fhir+xml" ${config.contentType === 'application/fhir+xml' ? 'selected' : ''}>application/fhir+xml</option>
                    </select>
                    <div class="form-hint">Content-Type validation (choose "Any" to accept all formats)</div>
                </div>

                <!-- HTTP Authentication (Universal for all HTTP connections) -->
                ${this.getHttpAuthConfig(config, idPrefix)}

                <!-- Response Configuration -->
                <div class="form-group">
                    <label class="form-label">Response Configuration</label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}HttpReturnCorrelationId" name="returnCorrelationId"
                               ${config.returnCorrelationId !== false ? 'checked' : ''}>
                        <span>Return correlation ID in response headers (X-Correlation-ID)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}HttpReturnMessageId" name="returnMessageId"
                               ${config.returnMessageId !== false ? 'checked' : ''}>
                        <span>Return message ID in response (useful for tracking)</span>
                    </label>
                </div>

                <!-- Advanced Settings -->
                <div class="form-group">
                    <label class="form-label">Advanced Settings</label>
                    <div class="form-row">
                        <div class="form-group">
                            <label for="${idPrefix}HttpMaxPayloadSize" class="form-label">Max Payload Size (MB)</label>
                            <input type="number" id="${idPrefix}HttpMaxPayloadSize" class="form-control"
                                   value="${config.maxPayloadSize || '10'}"
                                   placeholder="10"
                                   min="1"
                                   max="100"
                                   name="maxPayloadSize">
                            <div class="form-hint">Maximum request body size</div>
                        </div>
                        <div class="form-group">
                            <label for="${idPrefix}HttpTimeout" class="form-label">Timeout (seconds)</label>
                            <input type="number" id="${idPrefix}HttpTimeout" class="form-control"
                                   value="${config.timeout || '30'}"
                                   placeholder="30"
                                   min="5"
                                   max="300"
                                   name="timeout">
                            <div class="form-hint">Request timeout</div>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * FHIR Receiver Configuration (HTTP source with sourceType='fhir')
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getFhirReceiverConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group fhir-receiver-config">
                <h4>🏥 FHIR Receiver Configuration</h4>

                <!-- Listener Configuration -->
                <div class="config-group" style="background: #f8f9fa; padding: 15px; border-radius: 6px; margin-bottom: 20px;">
                    <h5 style="margin-top: 0;">🎧 Listener Configuration</h5>
                    <div class="form-row">
                        <div class="form-group" style="flex: 2;">
                            <label for="${idPrefix}sourcePort" class="form-label required">
                                Listen Port
                                <a href="#" class="help-link" onclick="event.preventDefault(); alert('This is the port where ezHealthKonnect will LISTEN for incoming FHIR requests from external systems.\\n\\nRecommended Ports:\\n• 8082-8089 (FHIR receivers)\\n• 9000-9999 (additional services)\\n\\nPort will be validated for availability.');" title="Click for help">ⓘ</a>
                            </label>
                            <input type="number" id="${idPrefix}sourcePort" class="form-control fhir-port-input"
                                   value="${config.port || '8082'}"
                                   placeholder="8082"
                                   min="1024"
                                   max="65535"
                                   required
                                   name="port"
                                   data-id-prefix="${idPrefix}">
                            <div id="${idPrefix}portValidation" class="form-hint">
                                ✅ <strong>Recommended</strong>: 8082-8089, 9000-9999
                            </div>
                        </div>
                        <div class="form-group" style="flex: 1;">
                            <label for="${idPrefix}sourceHost" class="form-label">Listen Host</label>
                            <input type="text" id="${idPrefix}sourceHost" class="form-control"
                                   value="${config.host || '0.0.0.0'}"
                                   placeholder="0.0.0.0"
                                   name="host">
                            <div class="form-hint">0.0.0.0 = all interfaces</div>
                        </div>
                    </div>
                    <div class="alert alert-info" style="margin-top: 12px; padding: 10px; background: #d1ecf1; border: 1px solid #bee5eb; border-radius: 4px; font-size: 13px;">
                        <strong>📡 External systems will send FHIR resources to:</strong><br>
                        <code style="background: #fff; padding: 2px 6px; border-radius: 3px;">http://your-server:<span id="${idPrefix}PortPreview">${config.port || '8082'}</span><span id="${idPrefix}BasePathPreview">${config.basePath || '/fhir'}</span>/Patient</code>
                    </div>
                </div>

                <!-- Base URL Path -->
                <div class="form-group">
                    <label for="${idPrefix}fhirBasePath" class="form-label required">Base URL Path</label>
                    <input type="text" id="${idPrefix}fhirBasePath" class="form-control fhir-basepath-input"
                           value="${config.basePath || '/fhir'}"
                           placeholder="/fhir"
                           name="basePath"
                           data-id-prefix="${idPrefix}">
                    <div class="form-hint">💡 URL path prefix for FHIR endpoints (e.g., /fhir/r4, /api/fhir)</div>
                </div>

                <!-- FHIR Version -->
                <div class="form-group">
                    <label for="${idPrefix}FhirVersion" class="form-label required">FHIR Version</label>
                    <select id="${idPrefix}FhirVersion" class="form-control" name="fhirVersion">
                        <option value="R4" ${!config.fhirVersion || config.fhirVersion === 'R4' ? 'selected' : ''}>FHIR R4 (Recommended)</option>
                        <option value="STU3" ${config.fhirVersion === 'STU3' ? 'selected' : ''}>FHIR STU3</option>
                        <option value="DSTU2" ${config.fhirVersion === 'DSTU2' ? 'selected' : ''}>FHIR DSTU2 (Legacy)</option>
                    </select>
                    <div class="form-hint">Select the FHIR version your sender systems will use</div>
                </div>

                <!-- Supported Operations -->
                <div class="form-group">
                    <label class="form-label">
                        Supported REST Operations
                        <a href="#" class="help-link" onclick="event.preventDefault(); alert('Choose which FHIR REST API operations to enable:\\n\\n• CREATE (POST) - Most common\\n• READ (GET) - Retrieve by ID\\n• UPDATE (PUT) - Replace entire resource\\n• PATCH - Partial updates\\n• DELETE - Remove resources\\n• SEARCH - Query with parameters\\n• BATCH/TRANSACTION - Multiple operations\\n\\nRecommendation: Start with CREATE only');" title="Click for help">ⓘ</a>
                    </label>
                    <div class="checkbox-group">
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpCreate" name="opCreate"
                                   ${!config.operations || config.operations.includes('CREATE') ? 'checked' : ''}>
                            <span>CREATE (POST)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpRead" name="opRead"
                                   ${config.operations?.includes('READ') ? 'checked' : ''}>
                            <span>READ (GET)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpUpdate" name="opUpdate"
                                   ${config.operations?.includes('UPDATE') ? 'checked' : ''}>
                            <span>UPDATE (PUT)</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpPatch" name="opPatch"
                                   ${config.operations?.includes('PATCH') ? 'checked' : ''}>
                            <span>PATCH</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpDelete" name="opDelete"
                                   ${config.operations?.includes('DELETE') ? 'checked' : ''}>
                            <span>DELETE</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpSearch" name="opSearch"
                                   ${config.operations?.includes('SEARCH') ? 'checked' : ''}>
                            <span>SEARCH</span>
                        </label>
                        <label class="checkbox-label">
                            <input type="checkbox" id="${idPrefix}OpBatch" name="opBatch"
                                   ${config.operations?.includes('BATCH') ? 'checked' : ''}>
                            <span>BATCH/TRANSACTION</span>
                        </label>
                    </div>
                </div>

                <!-- HTTP Authentication (Universal for all HTTP connections) -->
                ${this.getHttpAuthConfig(config, idPrefix)}

                <!-- Resource Filtering -->
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}EnableResourceFilter" name="enableResourceFilter"
                               ${config.enableResourceFilter ? 'checked' : ''}>
                        <span>Restrict Accepted Resource Types</span>
                    </label>
                    <div id="${idPrefix}ResourceFilterPanel" style="display: ${config.enableResourceFilter ? 'block' : 'none'}; margin-top: 12px;">
                        <div class="checkbox-group" style="max-height: 200px; overflow-y: auto; border: 1px solid #ddd; padding: 12px; border-radius: 4px;">
                            ${this.getFhirResourceCheckboxes(config.acceptedResources || [], idPrefix)}
                        </div>
                        <label class="checkbox-label" style="margin-top: 8px;">
                            <input type="checkbox" id="${idPrefix}RejectUnknownResources" name="rejectUnknownResources"
                                   ${config.rejectUnknownResources ? 'checked' : ''}>
                            <span>Reject resources not in the list above</span>
                        </label>
                    </div>
                </div>

                <!-- Validation Settings -->
                <div class="form-group">
                    <label class="form-label">Validation Settings</label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ValidateStructure" name="validateStructure"
                               ${config.validateStructure !== false ? 'checked' : ''}>
                        <span>Validate Resource Structure (required fields, data types)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ValidateProfiles" name="validateProfiles"
                               ${config.validateProfiles ? 'checked' : ''}>
                        <span>Validate Against FHIR Profiles (US Core, etc.)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ValidateTerminology" name="validateTerminology"
                               ${config.validateTerminology ? 'checked' : ''}>
                        <span>Validate Terminology (code systems, value sets)</span>
                    </label>
                </div>

                <!-- Post-Reception Actions -->
                <div class="form-group">
                    <h4>🔄 Post-Reception Actions</h4>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ActionStoreOnly" name="actionStoreOnly" checked disabled>
                        <span>Store in Database (Always enabled)</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ActionTransform" name="actionTransform"
                               ${config.actionTransform ? 'checked' : ''}>
                        <span>Apply Transformation Pipeline</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ActionForward" name="actionForward"
                               ${config.actionForward ? 'checked' : ''}>
                        <span>Forward to Destination</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ActionWorkflow" name="actionWorkflow"
                               ${config.actionWorkflow ? 'checked' : ''}>
                        <span>Trigger Workflow</span>
                    </label>
                    <label class="checkbox-label">
                        <input type="checkbox" id="${idPrefix}ActionAudit" name="actionAudit"
                               ${config.actionAudit ? 'checked' : ''}>
                        <span>Generate FHIR AuditEvent</span>
                    </label>
                </div>
            </div>
        `;
    }

    /**
     * Get FHIR resource type checkboxes
     * @param {array} selectedResources - Currently selected resource types
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getFhirResourceCheckboxes(selectedResources = [], idPrefix = '') {
        const commonResources = [
            'Patient', 'Practitioner', 'Organization', 'Location',
            'Encounter', 'Observation', 'Condition', 'Procedure',
            'MedicationRequest', 'MedicationAdministration', 'Immunization',
            'AllergyIntolerance', 'DiagnosticReport', 'DocumentReference',
            'Appointment', 'ServiceRequest', 'CarePlan', 'Goal',
            'Coverage', 'Claim', 'ExplanationOfBenefit'
        ];

        return commonResources.map(resource => `
            <label class="checkbox-label">
                <input type="checkbox" class="fhir-resource-checkbox" value="${resource}"
                       name="resource_${resource}"
                       id="${idPrefix}Resource${resource}"
                       ${selectedResources.includes(resource) ? 'checked' : ''}>
                <span>${resource}</span>
            </label>
        `).join('');
    }

    /**
     * File Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getFileConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;
        const isSource = direction === 'source';

        return `
            <div class="config-group">
                <h4>File ${isSource ? 'Listener' : 'Writer'} Configuration</h4>
                <div class="form-group">
                    <label for="${prefix}FilePath" class="form-label required">${isSource ? 'Watch Directory' : 'Output Directory'}</label>
                    <input type="text" id="${prefix}FilePath" class="form-control"
                           value="${config.filePath || '/data/hl7/incoming'}"
                           placeholder="/data/hl7/incoming"
                           name="${direction}FilePath">
                    <div class="form-hint">${isSource ? 'Directory to monitor for new files' : 'Directory where files will be written'}</div>
                </div>
                ${isSource ? `
                    <div class="form-group">
                        <label for="${prefix}FilePattern" class="form-label">File Pattern</label>
                        <input type="text" id="${prefix}FilePattern" class="form-control"
                               value="${config.filePattern || '*.hl7'}"
                               placeholder="*.hl7"
                               name="${direction}FilePattern">
                        <div class="form-hint">Glob pattern to match files (e.g., *.hl7, ADT_*.txt)</div>
                    </div>
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" id="${prefix}DeleteAfterProcess" name="${direction}DeleteAfterProcess"
                                   ${config.deleteAfterProcess ? 'checked' : ''}>
                            <span>Delete files after successful processing</span>
                        </label>
                    </div>
                ` : `
                    <div class="form-group">
                        <label for="${prefix}FileNaming" class="form-label">File Naming Pattern</label>
                        <input type="text" id="${prefix}FileNaming" class="form-control"
                               value="${config.fileNaming || '{timestamp}_{messageId}.hl7'}"
                               placeholder="{timestamp}_{messageId}.hl7"
                               name="${direction}FileNaming">
                        <div class="form-hint">Variables: {timestamp}, {messageId}, {interfaceId}</div>
                    </div>
                `}
            </div>
        `;
    }

    /**
     * Database Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getDatabaseConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;
        return `
            <div class="config-group">
                <h4>Database Configuration</h4>
                <div class="alert alert-info">
                    ℹ️ Database connectivity configuration coming soon. For now, use file or HTTP connectivity.
                </div>
            </div>
        `;
    }

    /**
     * SFTP Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getSftpConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;
        return `
            <div class="config-group">
                <h4>SFTP Configuration</h4>
                <div class="alert alert-info">
                    ℹ️ SFTP connectivity configuration coming soon.
                </div>
            </div>
        `;
    }

    /**
     * RabbitMQ Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getRabbitMQConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;
        return `
            <div class="config-group">
                <h4>RabbitMQ Configuration</h4>
                <div class="alert alert-info">
                    ℹ️ RabbitMQ connectivity configuration coming soon.
                </div>
            </div>
        `;
    }

    /**
     * Kafka Configuration (Source or Target)
     * @param {string} direction - 'source' or 'target'
     * @param {object} config - Configuration values
     * @param {string} idPrefix - ID prefix for form fields
     */
    static getKafkaConfig(direction = 'source', config = {}, idPrefix = '') {
        const prefix = idPrefix + direction;
        return `
            <div class="config-group">
                <h4>Apache Kafka Configuration</h4>
                <div class="alert alert-info">
                    ℹ️ Kafka connectivity configuration coming soon.
                </div>
            </div>
        `;
    }

    /**
     * Attach event listeners for dynamic form behavior
     * Call this after rendering forms
     * @param {HTMLElement} containerElement - Container element with the form
     * @param {string} idPrefix - ID prefix used in the form
     * @param {object} callbacks - Optional callbacks for events
     */
    static attachEventListeners(containerElement, idPrefix = '', callbacks = {}) {
        // HTTP Auth type change listener
        this.attachAuthTypeListeners(containerElement, idPrefix);

        // FHIR port validation and preview update
        const portInput = containerElement.querySelector(`#${idPrefix}sourcePort`);
        if (portInput && portInput.classList.contains('fhir-port-input')) {
            portInput.addEventListener('input', (e) => {
                const port = e.target.value;
                const portPreview = containerElement.querySelector(`#${idPrefix}PortPreview`);
                const portValidation = containerElement.querySelector(`#${idPrefix}portValidation`);

                if (portPreview) {
                    portPreview.textContent = port;
                }

                // Validate port availability
                if (portValidation) {
                    this.validatePort(port, portValidation);
                }
            });
        }

        // FHIR base path preview update
        const basePathInput = containerElement.querySelector(`#${idPrefix}fhirBasePath`);
        if (basePathInput && basePathInput.classList.contains('fhir-basepath-input')) {
            basePathInput.addEventListener('input', (e) => {
                const basePath = e.target.value;
                const basePathPreview = containerElement.querySelector(`#${idPrefix}BasePathPreview`);

                if (basePathPreview) {
                    basePathPreview.textContent = basePath;
                }
            });
        }

        // Bearer token validation checkbox
        const validateTokenCheckbox = containerElement.querySelector(`#${idPrefix}AuthValidateToken`);
        if (validateTokenCheckbox) {
            validateTokenCheckbox.addEventListener('change', (e) => {
                const validationFields = containerElement.querySelector(`#${idPrefix}TokenValidationFields`);
                if (validationFields) {
                    validationFields.style.display = e.target.checked ? 'block' : 'none';
                }
            });
        }

        // Resource filter toggle
        const enableResourceFilter = containerElement.querySelector(`#${idPrefix}EnableResourceFilter`);
        if (enableResourceFilter) {
            enableResourceFilter.addEventListener('change', (e) => {
                const filterPanel = containerElement.querySelector(`#${idPrefix}ResourceFilterPanel`);
                if (filterPanel) {
                    filterPanel.style.display = e.target.checked ? 'block' : 'none';
                }
            });
        }

        // Source connectivity change (if callback provided)
        if (callbacks.onConnectivityChange) {
            const connectivitySelect = containerElement.querySelector(`#${idPrefix}sourceConnectivity`);
            if (connectivitySelect) {
                connectivitySelect.addEventListener('change', (e) => {
                    callbacks.onConnectivityChange(e.target.value);
                });
            }
        }

        // Source type change (if callback provided)
        if (callbacks.onSourceTypeChange) {
            const sourceTypeSelect = containerElement.querySelector(`#${idPrefix}sourceType`);
            if (sourceTypeSelect) {
                sourceTypeSelect.addEventListener('change', (e) => {
                    callbacks.onSourceTypeChange(e.target.value);
                });
            }
        }
    }

    /**
     * Validate port availability
     * @param {string} port - Port number to validate
     * @param {HTMLElement} validationElement - Element to display validation message
     */
    static validatePort(port, validationElement) {
        const portNum = parseInt(port);
        const reservedPorts = [3000, 8080, 8081, 8090, 8091, 8092, 8093, 8094, 8095, 8096, 8097, 5432, 27017];

        if (!port || isNaN(portNum)) {
            validationElement.innerHTML = '✅ <strong>Recommended</strong>: 8082-8089, 9000-9999';
            validationElement.style.color = '';
            return;
        }

        if (reservedPorts.includes(portNum)) {
            validationElement.innerHTML = `⚠️ <strong>Port ${portNum} is reserved</strong> for system services. Please choose another port.`;
            validationElement.style.color = '#856404';
            validationElement.style.background = '#fff3cd';
            validationElement.style.padding = '8px';
            validationElement.style.borderRadius = '4px';
        } else if (portNum >= 8082 && portNum <= 8089) {
            validationElement.innerHTML = `✅ <strong>Port ${portNum} is recommended</strong> for FHIR receivers`;
            validationElement.style.color = '#155724';
            validationElement.style.background = '#d4edda';
            validationElement.style.padding = '8px';
            validationElement.style.borderRadius = '4px';
        } else if (portNum >= 9000 && portNum <= 9999) {
            validationElement.innerHTML = `✅ <strong>Port ${portNum} is available</strong> - good choice`;
            validationElement.style.color = '#155724';
            validationElement.style.background = '#d4edda';
            validationElement.style.padding = '8px';
            validationElement.style.borderRadius = '4px';
        } else {
            validationElement.innerHTML = `ℹ️ Port ${portNum} - will be validated on save`;
            validationElement.style.color = '#004085';
            validationElement.style.background = '#cce5ff';
            validationElement.style.padding = '8px';
            validationElement.style.borderRadius = '4px';
        }
    }

    /**
     * Attach HTTP authentication type change listener
     * @param {HTMLElement} containerElement - Container element
     * @param {string} idPrefix - ID prefix
     */
    static attachAuthTypeListeners(containerElement, idPrefix = '') {
        const authTypeSelect = containerElement.querySelector(`#${idPrefix}HttpAuthType`);
        if (authTypeSelect) {
            authTypeSelect.addEventListener('change', (e) => {
                const authType = e.target.value;
                console.log(`🔐 HTTP auth type changed to: ${authType} (prefix: ${idPrefix})`);

                const authDetailsPanel = containerElement.querySelector(`#${idPrefix}HttpAuthDetails`);
                if (authDetailsPanel) {
                    authDetailsPanel.innerHTML = this.getHttpAuthDetailsPanel(authType, {}, idPrefix);

                    // Re-attach nested listeners (like bearer token validation)
                    this.attachEventListeners(containerElement, idPrefix);
                }
            });
            console.log(`✅ HTTP auth type listener attached (prefix: ${idPrefix})`);
        }
    }

    // ==================== TARGET CONFIGURATION METHODS ====================

    /**
     * Get target connectivity selector dropdown
     * @param {string} selectedValue - Currently selected value
     * @param {object} config - Configuration options (idPrefix, showHint, etc.)
     */
    static getTargetConnectivitySelector(selectedValue = '', config = {}) {
        const idPrefix = config.idPrefix || '';
        return `
            <div class="form-group">
                <label for="${idPrefix}targetConnectivity" class="form-label required">
                    Target Connectivity
                </label>
                <select id="${idPrefix}targetConnectivity"
                        class="form-control target-connectivity-selector"
                        name="targetConnectivity"
                        required>
                    <option value="">Select connectivity...</option>
                    <option value="sink" ${selectedValue === 'sink' ? 'selected' : ''}>🗄️ Sink (Store Only - No Routing)</option>
                    <option value="http" ${selectedValue === 'http' ? 'selected' : ''}>HTTP/REST (Standard)</option>
                    <option value="tcp" ${selectedValue === 'tcp' ? 'selected' : ''}>TCP/MLLP</option>
                    <option value="file" ${selectedValue === 'file' ? 'selected' : ''}>File Output</option>
                    <option value="database" ${selectedValue === 'database' ? 'selected' : ''}>Database</option>
                </select>
                ${config.showHint !== false ? '<div class="form-hint">💡 Sink stores messages without routing to external systems</div>' : ''}
            </div>
        `;
    }

    /**
     * Get target configuration panel based on connectivity type
     * @param {string} connectivity - Connectivity type (http, tcp, file, database)
     * @param {string} targetType - Target type (fhir, hl7, etc.)
     * @param {object} config - Configuration object with values
     * @param {object} options - Options (idPrefix, etc.)
     */
    static getTargetConfigPanel(connectivity, targetType, config = {}, options = {}) {
        const idPrefix = options.idPrefix || '';

        switch (connectivity) {
            case 'sink':
                return this.getSinkTargetConfig(config, idPrefix);
            case 'http':
                return this.getHttpTargetConfig(config, idPrefix);
            case 'tcp':
                return this.getTcpTargetConfig(config, idPrefix);
            case 'file':
                return this.getFileTargetConfig(config, idPrefix);
            case 'database':
                return this.getDatabaseTargetConfig(config, idPrefix);
            default:
                return '<div class="config-placeholder">📋 Select target connectivity type to configure</div>';
        }
    }

    /**
     * Get Sink target configuration (store only, no routing)
     */
    static getSinkTargetConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group">
                <h4>🗄️ Sink Configuration</h4>
                <div class="alert alert-info" style="background: #e7f3ff; padding: 15px; border-left: 4px solid #2196F3; margin-bottom: 20px;">
                    <strong>ℹ️ Sink Mode - Store Only</strong><br>
                    Messages are received and stored in the database but NOT routed to any external system.
                    <br><br>
                    <strong>Use Cases:</strong>
                    <ul style="margin: 10px 0 0 20px; padding: 0;">
                        <li>Testing and debugging message reception</li>
                        <li>Message archiving and compliance</li>
                        <li>Quality assurance and validation</li>
                        <li>Development and staging environments</li>
                    </ul>
                </div>

                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}sinkEnableLogging" class="form-check-input"
                               ${config.enableLogging !== false ? 'checked' : ''}>
                        Enable detailed logging for stored messages
                    </label>
                    <div class="form-hint">💡 Logs all message details including headers and metadata</div>
                </div>

                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}sinkEnableValidation" class="form-check-input"
                               ${config.enableValidation === true ? 'checked' : ''}>
                        Validate message format before storing
                    </label>
                    <div class="form-hint">⚠️ Validates HL7/FHIR structure (may impact performance)</div>
                </div>

                <div class="form-group">
                    <label for="${idPrefix}sinkRetentionDays" class="form-label">Message Retention Period</label>
                    <select id="${idPrefix}sinkRetentionDays" class="form-control">
                        <option value="7" ${config.retentionDays === '7' || config.retentionDays === 7 ? 'selected' : ''}>7 days</option>
                        <option value="30" ${config.retentionDays === '30' || config.retentionDays === 30 || !config.retentionDays ? 'selected' : ''}>30 days (Default)</option>
                        <option value="90" ${config.retentionDays === '90' || config.retentionDays === 90 ? 'selected' : ''}>90 days</option>
                        <option value="365" ${config.retentionDays === '365' || config.retentionDays === 365 ? 'selected' : ''}>1 year</option>
                        <option value="-1" ${config.retentionDays === '-1' || config.retentionDays === -1 ? 'selected' : ''}>Indefinite (Manual cleanup only)</option>
                    </select>
                    <div class="form-hint">💡 Old messages are automatically purged after this period</div>
                </div>

                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}sinkGenerateAck" class="form-check-input"
                               ${config.generateAck !== false ? 'checked' : ''}>
                        Generate ACK/NACK responses (for HL7)
                    </label>
                    <div class="form-hint">✅ Sends acknowledgment back to source system</div>
                </div>

                <div class="alert alert-success" style="background: #d4edda; padding: 12px; border-left: 4px solid #28a745; margin-top: 20px;">
                    <strong>✅ Sink is Ready</strong><br>
                    No destination configuration required. Messages will be stored in your interface-specific table.
                </div>
            </div>
        `;
    }

    /**
     * Get HTTP/REST target configuration (FHIR server)
     */
    static getHttpTargetConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group">
                <h4>🌐 FHIR Server Configuration</h4>
                <div class="form-group">
                    <label for="${idPrefix}targetEndpoint" class="form-label required">FHIR Base URL</label>
                    <input type="url" id="${idPrefix}targetEndpoint" class="form-control"
                           value="${config.endpoint || 'http://localhost:8080/fhir'}"
                           placeholder="http://localhost:8080/fhir"
                           required>
                    <div class="form-hint">💡 OOB: Local HAPI FHIR server</div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetVersion" class="form-label">FHIR Version</label>
                        <select id="${idPrefix}targetVersion" class="form-control">
                            <option value="R4" ${config.version === 'R4' || !config.version ? 'selected' : ''}>R4 (Recommended)</option>
                            <option value="STU3" ${config.version === 'STU3' ? 'selected' : ''}>STU3</option>
                            <option value="DSTU2" ${config.version === 'DSTU2' ? 'selected' : ''}>DSTU2</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetFormat" class="form-label">Format</label>
                        <select id="${idPrefix}targetFormat" class="form-control">
                            <option value="json" ${config.format === 'json' || !config.format ? 'selected' : ''}>JSON (Recommended)</option>
                            <option value="xml" ${config.format === 'xml' ? 'selected' : ''}>XML</option>
                        </select>
                    </div>
                </div>
            </div>

            <!-- HTTP Authentication -->
            ${this.getHttpAuthConfig(config, idPrefix + 'target')}
        `;
    }

    /**
     * Get TCP/MLLP target configuration
     */
    static getTcpTargetConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group">
                <h4>🔌 TCP/MLLP Output Configuration</h4>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetHost" class="form-label required">Host</label>
                        <input type="text" id="${idPrefix}targetHost" class="form-control"
                               value="${config.host || 'localhost'}"
                               placeholder="localhost"
                               required>
                        <div class="form-hint">Target server hostname or IP</div>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetPort" class="form-label required">Port</label>
                        <input type="number" id="${idPrefix}targetPort" class="form-control"
                               value="${config.port || '2576'}"
                               placeholder="2576"
                               min="1"
                               max="65535"
                               required>
                        <div class="form-hint">TCP port for MLLP</div>
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetTimeout" class="form-label">Connection Timeout (ms)</label>
                        <input type="number" id="${idPrefix}targetTimeout" class="form-control"
                               value="${config.timeout || '30000'}"
                               placeholder="30000"
                               min="1000">
                        <div class="form-hint">Connection timeout in milliseconds</div>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetEncoding" class="form-label">Message Encoding</label>
                        <select id="${idPrefix}targetEncoding" class="form-control">
                            <option value="utf8" ${config.encoding === 'utf8' || !config.encoding ? 'selected' : ''}>UTF-8 (Recommended)</option>
                            <option value="ascii" ${config.encoding === 'ascii' ? 'selected' : ''}>ASCII</option>
                            <option value="iso-8859-1" ${config.encoding === 'iso-8859-1' ? 'selected' : ''}>ISO-8859-1</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}targetMllpEnabled" class="form-check-input"
                               ${config.mllpEnabled !== false ? 'checked' : ''}>
                        Use MLLP Protocol (Recommended)
                    </label>
                    <div class="form-hint">Wraps messages with MLLP start/end bytes</div>
                </div>
            </div>
        `;
    }

    /**
     * Get File Output target configuration
     */
    static getFileTargetConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group">
                <h4>📁 File Output Configuration</h4>
                <div class="form-group">
                    <label for="${idPrefix}targetFilePath" class="form-label required">Output Directory</label>
                    <input type="text" id="${idPrefix}targetFilePath" class="form-control"
                           value="${config.filePath || '/app/output'}"
                           placeholder="/app/output"
                           required>
                    <div class="form-hint">💡 Directory where files will be written</div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetFilePattern" class="form-label">File Name Pattern</label>
                        <input type="text" id="${idPrefix}targetFilePattern" class="form-control"
                               value="${config.filePattern || '{messageType}_{timestamp}_{messageId}.{ext}'}"
                               placeholder="{messageType}_{timestamp}_{messageId}.{ext}">
                        <div class="form-hint">Supports: {messageType}, {timestamp}, {messageId}, {interfaceId}, {ext}</div>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetFileFormat" class="form-label">File Format</label>
                        <select id="${idPrefix}targetFileFormat" class="form-control">
                            <option value="json" ${config.fileFormat === 'json' || !config.fileFormat ? 'selected' : ''}>JSON</option>
                            <option value="xml" ${config.fileFormat === 'xml' ? 'selected' : ''}>XML</option>
                            <option value="hl7" ${config.fileFormat === 'hl7' ? 'selected' : ''}>HL7 v2</option>
                            <option value="txt" ${config.fileFormat === 'txt' ? 'selected' : ''}>Text</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}targetCreateDirectory" class="form-check-input"
                               ${config.createDirectory !== false ? 'checked' : ''}>
                        Auto-create directory if it doesn't exist
                    </label>
                </div>
                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}targetOverwriteExisting" class="form-check-input"
                               ${config.overwriteExisting === true ? 'checked' : ''}>
                        Overwrite existing files
                    </label>
                    <div class="form-hint">⚠️ If unchecked, will append timestamp to avoid conflicts</div>
                </div>
            </div>
        `;
    }

    /**
     * Get Database target configuration
     */
    static getDatabaseTargetConfig(config = {}, idPrefix = '') {
        return `
            <div class="config-group">
                <h4>🗄️ Database Output Configuration</h4>
                <div class="form-group">
                    <label for="${idPrefix}targetDbType" class="form-label required">Database Type</label>
                    <select id="${idPrefix}targetDbType" class="form-control" required>
                        <option value="">Select database type...</option>
                        <option value="postgresql" ${config.dbType === 'postgresql' ? 'selected' : ''}>PostgreSQL</option>
                        <option value="mysql" ${config.dbType === 'mysql' ? 'selected' : ''}>MySQL</option>
                        <option value="sqlserver" ${config.dbType === 'sqlserver' ? 'selected' : ''}>SQL Server</option>
                        <option value="mongodb" ${config.dbType === 'mongodb' ? 'selected' : ''}>MongoDB</option>
                        <option value="oracle" ${config.dbType === 'oracle' ? 'selected' : ''}>Oracle</option>
                    </select>
                    <div class="form-hint">💡 Select your target database system</div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetDbHost" class="form-label required">Host</label>
                        <input type="text" id="${idPrefix}targetDbHost" class="form-control"
                               value="${config.dbHost || 'localhost'}"
                               placeholder="localhost"
                               required>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetDbPort" class="form-label required">Port</label>
                        <input type="number" id="${idPrefix}targetDbPort" class="form-control"
                               value="${config.dbPort || '5432'}"
                               placeholder="5432"
                               required>
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetDbName" class="form-label required">Database Name</label>
                        <input type="text" id="${idPrefix}targetDbName" class="form-control"
                               value="${config.dbName || ''}"
                               placeholder="database_name"
                               required>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetDbTable" class="form-label required">Table Name</label>
                        <input type="text" id="${idPrefix}targetDbTable" class="form-control"
                               value="${config.dbTable || ''}"
                               placeholder="table_name"
                               required>
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="${idPrefix}targetDbUsername" class="form-label required">Username</label>
                        <input type="text" id="${idPrefix}targetDbUsername" class="form-control"
                               value="${config.dbUsername || ''}"
                               placeholder="db_user"
                               required>
                    </div>
                    <div class="form-group">
                        <label for="${idPrefix}targetDbPassword" class="form-label required">Password</label>
                        <input type="password" id="${idPrefix}targetDbPassword" class="form-control"
                               value="${config.dbPassword || ''}"
                               placeholder="••••••••"
                               required>
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-check-label">
                        <input type="checkbox" id="${idPrefix}targetDbSSL" class="form-check-input"
                               ${config.dbSSL === true ? 'checked' : ''}>
                        Use SSL/TLS connection
                    </label>
                    <div class="form-hint">🔒 Recommended for production databases</div>
                </div>
            </div>
        `;
    }
}

// Make it globally available
window.InterfaceConfigComponents = InterfaceConfigComponents;

console.log('✅ InterfaceConfigComponents loaded successfully');
