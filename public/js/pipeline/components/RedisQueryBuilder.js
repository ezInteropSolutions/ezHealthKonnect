/**
 * RedisQueryBuilder Component
 * NO-CODE visual builder for Redis queries
 * Version: 1.4 - Fixed backend compatibility (redisKey + redisCommand separate fields)
 */

class RedisQueryBuilder {
    constructor(container, config = {}) {
        this.container = container;
        this.config = config;

        // Default values
        this.command = 'GET';
        this.keyPrefix = 'patient';
        this.separator = ':';
        this.hlField = 'PID.3.1';
        this.subPrefix = '';
        this.hashField = '';
        this.keyPattern = '';

        // Load existing config if provided
        // Priority: redisCommand/redisKey (backend format) > redisQuery (legacy format)
        if (config.redisCommand && config.redisKey) {
            // Load from backend format (separate fields)
            this.command = config.redisCommand.toUpperCase();
            this.hashField = config.redisHashField || '';
            this.parseExistingKey(config.redisKey);
        } else if (config.redisQuery) {
            // Load from legacy format (full command string)
            this.parseExistingQuery(config.redisQuery);
        }

        this.render();
    }

    /**
     * Parse existing Redis key into builder fields
     */
    parseExistingKey(key) {
        // Check if it contains template {{ }}
        const templateMatch = key.match(/\{\{\s*([^}]+)\s*\}\}/);
        if (templateMatch) {
            this.hlField = templateMatch[1].trim();

            // Extract prefix before template
            const beforeTemplate = key.split('{{')[0];
            const prefixParts = beforeTemplate.split(':').filter(p => p);

            if (prefixParts.length > 0) {
                this.keyPrefix = prefixParts[0];
            }
            if (prefixParts.length > 1) {
                this.subPrefix = prefixParts[1];
            }
        } else {
            // Static key pattern (like patient:*)
            this.keyPattern = key;
        }
    }

    /**
     * Parse existing Redis query into builder fields
     */
    parseExistingQuery(query) {
        // Extract command (first word)
        const parts = query.trim().split(/\s+/);
        this.command = parts[0].toUpperCase();

        // Extract key pattern
        if (parts.length > 1) {
            const keyPart = parts[1];

            // Check if it contains template {{ }}
            const templateMatch = keyPart.match(/\{\{\s*([^}]+)\s*\}\}/);
            if (templateMatch) {
                this.hlField = templateMatch[1].trim();

                // Extract prefix before template
                const beforeTemplate = keyPart.split('{{')[0];
                const prefixParts = beforeTemplate.split(':').filter(p => p);

                if (prefixParts.length > 0) {
                    this.keyPrefix = prefixParts[0];
                }
                if (prefixParts.length > 1) {
                    this.subPrefix = prefixParts[1];
                }
            } else {
                // Static key pattern (like KEYS patient:*)
                this.keyPattern = keyPart;
            }
        }

        // Extract hash field for HGET
        if (this.command === 'HGET' && parts.length > 2) {
            this.hashField = parts[2];
        }
    }

    /**
     * Render the visual builder UI
     */
    render() {
        this.container.innerHTML = `
            <div class="redis-query-builder">
                <div class="builder-header">
                    <h4>
                        <i class="fas fa-database"></i>
                        Redis Query Builder (NO CODE)
                    </h4>
                    <small>
                        Build Redis queries visually - no syntax knowledge needed!
                    </small>
                </div>

                <div class="builder-body" style="margin-top: 16px;">
                    <!-- Step 1: Command Selection -->
                    <div class="builder-section">
                        <label class="builder-label">
                            <span class="step-number">1</span>
                            What do you want to do?
                            <span class="required-star">*</span>
                        </label>
                        <select id="redisCommand" class="form-control">
                            <option value="GET" ${this.command === 'GET' ? 'selected' : ''}>GET - Retrieve a value (like getting patient JSON data)</option>
                            <option value="HGETALL" ${this.command === 'HGETALL' ? 'selected' : ''}>HGETALL - Get all fields from a record</option>
                            <option value="HGET" ${this.command === 'HGET' ? 'selected' : ''}>HGET - Get one specific field from a record</option>
                            <option value="KEYS" ${this.command === 'KEYS' ? 'selected' : ''}>KEYS - Search for keys by pattern</option>
                            <option value="SMEMBERS" ${this.command === 'SMEMBERS' ? 'selected' : ''}>SMEMBERS - Get all items in a set/list</option>
                        </select>
                        <small class="form-text text-muted" id="commandHelp">
                            ${this.getCommandHelp(this.command)}
                        </small>
                    </div>

                    <!-- Step 2: Key Pattern Builder (for GET, HGETALL, HGET, SMEMBERS) -->
                    <div class="builder-section" id="keyPatternSection" style="display: ${this.command !== 'KEYS' ? 'block' : 'none'};">
                        <label class="builder-label">
                            <span class="step-number">2</span>
                            How is your data organized in Redis?
                            <span class="required-star">*</span>
                        </label>

                        <div style="background: #f0f9ff; border: 1px solid #bae6fd; border-radius: 6px; padding: 12px; margin-bottom: 16px;">
                            <strong style="color: #0c4a6e; display: block; margin-bottom: 8px;">📘 Understanding Redis Keys:</strong>
                            <p style="margin: 0; font-size: 13px; color: #0c4a6e; line-height: 1.5;">
                                Redis stores data using <strong>keys</strong> (like file paths). Keys are typically structured like:
                                <code style="background: white; padding: 2px 6px; border-radius: 3px;">patient:P123456</code> or
                                <code style="background: white; padding: 2px 6px; border-radius: 3px;">patient:details:P123456</code>
                            </p>
                        </div>

                        <div class="key-pattern-builder">
                            <div class="form-row" style="display: flex; gap: 8px; align-items: flex-end;">
                                <div style="flex: 1;">
                                    <label style="font-size: 13px; color: #1f2937; margin-bottom: 4px; font-weight: 600;">
                                        Main Category
                                        <i class="fas fa-question-circle" style="color: #3b82f6; cursor: help; font-size: 12px;"
                                           title="What type of data? E.g., 'patient', 'user', 'order', 'session'"></i>
                                    </label>
                                    <input type="text"
                                           id="keyPrefix"
                                           class="form-control"
                                           value="${this.escapeHtml(this.keyPrefix)}"
                                           placeholder="patient">
                                    <small class="text-muted">Examples: patient, user, order, session</small>
                                </div>

                                <div style="width: 80px;">
                                    <label style="font-size: 13px; color: #1f2937; margin-bottom: 4px; font-weight: 600;">
                                        Separator
                                    </label>
                                    <input type="text"
                                           id="separator"
                                           class="form-control text-center"
                                           value="${this.escapeHtml(this.separator)}"
                                           placeholder=":"
                                           title="Character to separate key parts (usually ':')">
                                    <small class="text-muted" style="text-align: center;">Usually :</small>
                                </div>

                                <div style="flex: 1;">
                                    <label style="font-size: 13px; color: #1f2937; margin-bottom: 4px; font-weight: 600;">
                                        Sub-Category (optional)
                                        <i class="fas fa-question-circle" style="color: #3b82f6; cursor: help; font-size: 12px;"
                                           title="Optional second level. E.g., 'details', 'cache', 'summary'. Leave blank if not needed."></i>
                                    </label>
                                    <input type="text"
                                           id="subPrefix"
                                           class="form-control"
                                           value="${this.escapeHtml(this.subPrefix)}"
                                           placeholder="(optional)">
                                    <small class="text-muted">Examples: details, cache, summary</small>
                                </div>
                            </div>

                            <div style="margin-top: 16px; padding-top: 16px; border-top: 1px dashed #e5e7eb;">
                                <label style="font-size: 13px; color: #1f2937; margin-bottom: 4px; font-weight: 600;">
                                    Which field contains the unique ID?
                                    <i class="fas fa-question-circle" style="color: #3b82f6; cursor: help; font-size: 12px;"
                                       title="This field's value will be used to build the complete key. For example, if PID.3.1 contains 'P123456', the final key will be 'patient:P123456'"></i>
                                </label>
                                <div style="display: flex; gap: 8px;">
                                    <input type="text"
                                           id="hlField"
                                           class="form-control"
                                           value="${this.escapeHtml(this.hlField)}"
                                           placeholder="Type field path or click Browse">
                                    <button type="button"
                                            class="btn btn-outline-primary"
                                            id="selectFieldBtn"
                                            style="white-space: nowrap;">
                                        <i class="fas fa-search"></i> Browse
                                    </button>
                                </div>
                                <small class="text-muted">
                                    <strong>💡 Tip:</strong> Click "Browse" to see available fields from your message or previous steps
                                </small>
                            </div>
                        </div>
                    </div>

                    <!-- Step 2 Alternative: Key Pattern (for KEYS command) -->
                    <div class="builder-section" id="keyPatternStaticSection" style="display: ${this.command === 'KEYS' ? 'block' : 'none'};">
                        <label class="builder-label">
                            <span class="step-number">2</span>
                            Key Pattern (with wildcards)
                            <span class="required-star">*</span>
                        </label>
                        <input type="text"
                               id="keyPattern"
                               class="form-control"
                               value="${this.escapeHtml(this.keyPattern || 'patient:*')}"
                               placeholder="patient:*">
                        <small class="form-text text-muted">
                            Use * for wildcards. Example: patient:* matches all patient keys
                        </small>
                    </div>

                    <!-- Step 3: Hash Field (for HGET only) -->
                    <div class="builder-section" id="hashFieldSection" style="display: ${this.command === 'HGET' ? 'block' : 'none'};">
                        <label class="builder-label">
                            <span class="step-number">3</span>
                            Hash Field to Retrieve
                            <span class="required-star">*</span>
                        </label>
                        <input type="text"
                               id="hashField"
                               class="form-control"
                               value="${this.escapeHtml(this.hashField)}"
                               placeholder="insurance_id">
                        <small class="form-text text-muted">
                            Specific field name to retrieve from the hash
                        </small>
                    </div>

                    <!-- Live Preview -->
                    <div class="builder-section" style="margin-top: 20px;">
                        <label class="builder-label">
                            <i class="fas fa-eye"></i> Generated Command
                        </label>
                        <div class="command-preview" id="commandPreview">
                            ${this.generateCommand()}
                        </div>
                        <small class="form-text text-muted">
                            This is the Redis command that will be executed
                        </small>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
    }

    /**
     * Get help text for each command
     */
    getCommandHelp(command) {
        const helpTexts = {
            'GET': 'Retrieves the entire value stored at a key. Best for JSON strings.',
            'HGETALL': 'Retrieves all field-value pairs from a hash. Returns multiple fields.',
            'HGET': 'Retrieves a specific field from a hash. Returns single field value.',
            'KEYS': 'Lists all keys matching a pattern. Use * as wildcard.',
            'SMEMBERS': 'Retrieves all members of a set. Returns array of values.'
        };
        return helpTexts[command] || '';
    }

    /**
     * Generate Redis command from builder values
     */
    generateCommand() {
        let command = this.command;

        if (this.command === 'KEYS') {
            command = `KEYS ${this.keyPattern || 'patient:*'}`;
        } else {
            // Build dynamic key
            let key = this.keyPrefix;

            if (this.subPrefix) {
                key += this.separator + this.subPrefix;
            }

            key += this.separator + '{{ ' + this.hlField + ' }}';

            if (this.command === 'HGET') {
                command = `HGET ${key} ${this.hashField}`;
            } else {
                command = `${this.command} ${key}`;
            }
        }

        return command;
    }

    /**
     * Attach event listeners
     */
    attachEventListeners() {
        // Command selection
        const commandSelect = this.container.querySelector('#redisCommand');
        commandSelect?.addEventListener('change', (e) => {
            this.command = e.target.value;
            this.updateUI();
        });

        // Key pattern fields
        const keyPrefix = this.container.querySelector('#keyPrefix');
        keyPrefix?.addEventListener('input', (e) => {
            this.keyPrefix = e.target.value;
            this.updatePreview();
        });

        const separator = this.container.querySelector('#separator');
        separator?.addEventListener('input', (e) => {
            this.separator = e.target.value;
            this.updatePreview();
        });

        const subPrefix = this.container.querySelector('#subPrefix');
        subPrefix?.addEventListener('input', (e) => {
            this.subPrefix = e.target.value;
            this.updatePreview();
        });

        // Field path input - allow manual editing
        const hlField = this.container.querySelector('#hlField');
        hlField?.addEventListener('input', (e) => {
            this.hlField = e.target.value;
            this.updatePreview();
        });

        const keyPattern = this.container.querySelector('#keyPattern');
        keyPattern?.addEventListener('input', (e) => {
            this.keyPattern = e.target.value;
            this.updatePreview();
        });

        const hashField = this.container.querySelector('#hashField');
        hashField?.addEventListener('input', (e) => {
            this.hashField = e.target.value;
            this.updatePreview();
        });

        // Field selector button
        const selectFieldBtn = this.container.querySelector('#selectFieldBtn');
        selectFieldBtn?.addEventListener('click', () => {
            this.openFieldSelector();
        });
    }

    /**
     * Update UI based on selected command
     */
    updateUI() {
        // Update help text
        const commandHelp = this.container.querySelector('#commandHelp');
        if (commandHelp) {
            commandHelp.textContent = this.getCommandHelp(this.command);
        }

        // Show/hide sections based on command
        const keyPatternSection = this.container.querySelector('#keyPatternSection');
        const keyPatternStaticSection = this.container.querySelector('#keyPatternStaticSection');
        const hashFieldSection = this.container.querySelector('#hashFieldSection');

        if (this.command === 'KEYS') {
            keyPatternSection.style.display = 'none';
            keyPatternStaticSection.style.display = 'block';
            hashFieldSection.style.display = 'none';
        } else {
            keyPatternSection.style.display = 'block';
            keyPatternStaticSection.style.display = 'none';
            hashFieldSection.style.display = this.command === 'HGET' ? 'block' : 'none';
        }

        this.updatePreview();
    }

    /**
     * Update command preview
     */
    updatePreview() {
        const preview = this.container.querySelector('#commandPreview');
        if (preview) {
            preview.textContent = this.generateCommand();
        }
    }

    /**
     * Open field selector modal
     */
    openFieldSelector() {
        // Create modal overlay
        const modal = document.createElement('div');
        modal.className = 'field-selector-modal';
        modal.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0,0,0,0.5);
            display: flex;
            align-items: center;
            justify-content: center;
            z-index: 10000;
        `;

        const modalContent = document.createElement('div');
        modalContent.style.cssText = `
            background: white;
            padding: 24px;
            border-radius: 8px;
            max-width: 700px;
            width: 90%;
            max-height: 80vh;
            overflow-y: auto;
        `;

        modalContent.innerHTML = `
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                <div>
                    <h4 style="margin: 0;">Browse Available Fields</h4>
                    <small style="color: #6b7280;">Select a field path or type your own in the input</small>
                </div>
                <button class="btn btn-sm btn-secondary close-modal">&times;</button>
            </div>

            <!-- Tab Navigation -->
            <div style="border-bottom: 2px solid #e5e7eb; margin-bottom: 16px;">
                <div style="display: flex; gap: 8px;">
                    <button class="field-tab active" data-tab="hl7" style="padding: 10px 16px; border: none; background: none; border-bottom: 3px solid #667eea; font-weight: 600; color: #667eea; cursor: pointer;">
                        HL7 Message Fields
                    </button>
                    <button class="field-tab" data-tab="stepoutput" style="padding: 10px 16px; border: none; background: none; border-bottom: 3px solid transparent; font-weight: 600; color: #6b7280; cursor: pointer;">
                        Step Outputs
                    </button>
                    <button class="field-tab" data-tab="custom" style="padding: 10px 16px; border: none; background: none; border-bottom: 3px solid transparent; font-weight: 600; color: #6b7280; cursor: pointer;">
                        Custom Paths
                    </button>
                </div>
            </div>

            <!-- Tab Content -->
            <div id="hl7TabContent" class="tab-content">
                <div id="fieldSearchContainer"></div>
            </div>

            <div id="stepoutputTabContent" class="tab-content" style="display: none;">
                <div style="padding: 20px; background: #f9fafb; border-radius: 6px; border: 1px solid #e5e7eb;">
                    <h5 style="margin: 0 0 12px 0; color: #1f2937;">Use data from previous steps:</h5>
                    <div style="display: flex; flex-direction: column; gap: 8px;">
                        <button class="example-field-btn" data-value="stepOutput.db_lookup.enriched_data[0].customer_id">
                            <code>stepOutput.db_lookup.enriched_data[0].customer_id</code>
                            <small>Database enrichment result</small>
                        </button>
                        <button class="example-field-btn" data-value="stepOutput.api_call.response.user_id">
                            <code>stepOutput.api_call.response.user_id</code>
                            <small>API call response field</small>
                        </button>
                        <button class="example-field-btn" data-value="stepOutput.validation.patient_verified">
                            <code>stepOutput.validation.patient_verified</code>
                            <small>Validation step result</small>
                        </button>
                    </div>
                    <p style="margin-top: 16px; padding: 12px; background: #fef3c7; border-left: 4px solid #f59e0b; border-radius: 4px; font-size: 13px; color: #92400e;">
                        <strong>💡 Tip:</strong> Step outputs let you chain data between pipeline steps. Use the alias you gave to previous steps.
                    </p>
                </div>
            </div>

            <div id="customTabContent" class="tab-content" style="display: none;">
                <div style="padding: 20px; background: #f9fafb; border-radius: 6px; border: 1px solid #e5e7eb;">
                    <div style="background: #fef3c7; border-left: 4px solid #f59e0b; padding: 12px; margin-bottom: 16px; border-radius: 4px;">
                        <strong style="color: #92400e;">💡 About Custom Paths:</strong>
                        <p style="margin: 4px 0 0 0; font-size: 13px; color: #92400e;">
                            These are example patterns for accessing data in JSON messages. You can <strong>type any custom path directly in the input field</strong> - this tab just shows common patterns to help you get started.
                        </p>
                    </div>

                    <h5 style="margin: 0 0 12px 0; color: #1f2937;">Common field path patterns:</h5>
                    <div style="display: flex; flex-direction: column; gap: 8px;">
                        <button class="example-field-btn" data-value="message.customer.id">
                            <code>message.customer.id</code>
                            <small>JSON message field</small>
                        </button>
                        <button class="example-field-btn" data-value="message.order.number">
                            <code>message.order.number</code>
                            <small>Order number from JSON</small>
                        </button>
                        <button class="example-field-btn" data-value="enriched.database.account_number">
                            <code>enriched.database.account_number</code>
                            <small>Enriched field from transformation</small>
                        </button>
                        <button class="example-field-btn" data-value="metadata.source_system">
                            <code>metadata.source_system</code>
                            <small>Message metadata</small>
                        </button>
                    </div>
                    <p style="margin-top: 12px; font-size: 13px; color: #6b7280; font-style: italic;">
                        Remember: You can always type your own custom path in the input field above!
                    </p>
                </div>
            </div>
        `;

        modal.appendChild(modalContent);
        document.body.appendChild(modal);

        // Tab switching logic
        const tabs = modalContent.querySelectorAll('.field-tab');
        const tabContents = {
            'hl7': modalContent.querySelector('#hl7TabContent'),
            'stepoutput': modalContent.querySelector('#stepoutputTabContent'),
            'custom': modalContent.querySelector('#customTabContent')
        };

        tabs.forEach(tab => {
            tab.addEventListener('click', () => {
                // Update active tab
                tabs.forEach(t => {
                    t.style.borderBottomColor = 'transparent';
                    t.style.color = '#6b7280';
                    t.classList.remove('active');
                });
                tab.style.borderBottomColor = '#667eea';
                tab.style.color = '#667eea';
                tab.classList.add('active');

                // Show corresponding content
                Object.values(tabContents).forEach(content => content.style.display = 'none');
                tabContents[tab.dataset.tab].style.display = 'block';
            });
        });

        // Initialize FieldPathSearchComponent for HL7 tab
        const searchContainer = modalContent.querySelector('#fieldSearchContainer');

        // Check if FieldPathSearchComponent is available
        if (typeof FieldPathSearchComponent !== 'undefined') {
            const fieldSearch = new FieldPathSearchComponent(searchContainer);

            fieldSearch.onFieldSelect = (fieldPath) => {
                this.hlField = fieldPath;
                const hlFieldInput = this.container.querySelector('#hlField');
                if (hlFieldInput) {
                    hlFieldInput.value = fieldPath;
                }
                this.updatePreview();
                modal.remove();
            };

            // Add helpful message if no fields are loaded
            setTimeout(() => {
                const fieldSearchResults = searchContainer.querySelector('.field-search-results');
                if (fieldSearchResults && fieldSearchResults.children.length === 0) {
                    fieldSearchResults.innerHTML = `
                        <div style="padding: 20px; text-align: center; background: #fef3c7; border-radius: 6px; border: 1px solid #fbbf24;">
                            <p style="margin: 0; font-size: 14px; color: #92400e;">
                                <strong>ℹ️ No HL7 message loaded</strong><br>
                                <small style="font-size: 13px;">
                                    HL7 fields will appear here when you test your pipeline with an actual message.<br>
                                    For now, you can type a field path manually (like <code>PID.3.1</code>) or use the Step Outputs tab.
                                </small>
                            </p>
                        </div>
                    `;
                }
            }, 100);
        } else {
            searchContainer.innerHTML = `
                <div style="padding: 20px; text-align: center; background: #fee2e2; border-radius: 6px; border: 1px solid #fca5a5;">
                    <p style="margin: 0; font-size: 14px; color: #991b1b;">
                        <strong>⚠️ Component not available</strong><br>
                        <small>Please type the field path manually in the input field.</small>
                    </p>
                </div>
            `;
        }

        // Example field button handlers
        const exampleButtons = modalContent.querySelectorAll('.example-field-btn');
        exampleButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const value = btn.dataset.value;
                this.hlField = value;
                const hlFieldInput = this.container.querySelector('#hlField');
                if (hlFieldInput) {
                    hlFieldInput.value = value;
                }
                this.updatePreview();
                modal.remove();
            });
        });

        // Close modal handlers
        modal.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.remove();
            }
        });

        modalContent.querySelector('.close-modal')?.addEventListener('click', () => {
            modal.remove();
        });
    }

    /**
     * Get current configuration
     */
    getConfig() {
        // Build the key pattern
        let key = '';
        if (this.command === 'KEYS') {
            key = this.keyPattern || 'patient:*';
        } else {
            key = this.keyPrefix;
            if (this.subPrefix) {
                key += this.separator + this.subPrefix;
            }
            key += this.separator + '{{ ' + this.hlField + ' }}';
        }

        // Return separate fields to match backend model
        return {
            redisKey: key,
            redisCommand: this.command,
            redisHashField: this.hashField || '',
            redisQuery: this.generateCommand()  // Keep for backward compatibility
        };
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Export for use in other modules
if (typeof window !== 'undefined') {
    window.RedisQueryBuilder = RedisQueryBuilder;
}
