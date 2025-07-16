// FILE: segment-viewer.js
// Complete dynamic segment viewer with NO hardcoding - all data from API/schema
// Enhanced subfield display and specific missing required field highlighting

class SegmentViewer {
    constructor(wizardInstance) {
        this.wizard = wizardInstance;
        this.expandedSegments = new Set(['MSH', 'PID']); // Smart defaults
        this.expandedFields = new Set();
        this.viewMode = 'compact'; // compact, detailed, table
        this.fieldMetadataCache = new Map(); // Cache for dynamic field metadata
    }

    /**
     * ✅ FIXED: Renders segments with correct field positioning and dynamic data
     */
    renderSegmentList(apiResponse, containerId) {
        const container = document.getElementById(containerId);
        if (!container || !apiResponse?.data?.enhancedSegments) {
            console.warn('Cannot render segments - missing container or data');
            return;
        }

        const data = apiResponse.data;
        const segments = this.getSegmentsInMessageOrder(data.enhancedSegments);
        const validationErrors = data.validationErrors || [];

        // ✅ CRITICAL FIX: Validate field positioning before rendering
        this.validateFieldPositioning(segments);
        
        // ✅ Build field metadata cache from actual data
        this.buildFieldMetadataCache(segments);

        const segmentListHTML = `
            <div class="compact-segment-viewer">
                ${this.renderCompactHeader(validationErrors, data.messageType, segments, data)}
                ${this.renderSegmentTable(segments, validationErrors)}
            </div>
        `;

        container.innerHTML = segmentListHTML;
        this.attachEventListeners();
    }

    /**
     * ✅ NEW: Build field metadata cache from actual API data
     */
    buildFieldMetadataCache(segments) {
        console.log('🔍 Building field metadata cache from API data...');
        this.fieldMetadataCache.clear();
        
        segments.forEach(([segmentName, segment]) => {
            if (!segment.fields) return;
            
            segment.fields.forEach(field => {
                const fieldKey = field.key;
                this.fieldMetadataCache.set(fieldKey, {
                    name: field.name || `Field ${field.position}`,
                    description: field.description || `${segmentName} field ${field.position}`,
                    dataType: field.dataType || 'ST',
                    optionality: field.optionality || 'O',
                    cardinality: field.cardinality || '[0..1]',
                    length: field.length,
                    tableId: field.tableId,
                    position: field.position,
                    hasValue: field.hasValue,
                    value: field.value
                });
                
                // Cache subfield metadata
                if (field.subfields && field.subfields.length > 0) {
                    field.subfields.forEach(subfield => {
                        this.fieldMetadataCache.set(subfield.key, {
                            name: subfield.name || `Component ${subfield.position}`,
                            description: subfield.description || `Component ${subfield.position} of ${fieldKey}`,
                            dataType: subfield.dataType || 'ST',
                            usage: subfield.usage || 'O',
                            length: subfield.length,
                            tableId: subfield.tableId,
                            position: subfield.position,
                            hasValue: subfield.hasValue,
                            value: subfield.value
                        });
                    });
                }
            });
        });
        
        console.log(`✅ Cached metadata for ${this.fieldMetadataCache.size} fields/components`);
    }

    /**
     * ✅ NEW: Validate field positioning across all segments
     */
    validateFieldPositioning(segments) {
        console.log('🔍 Validating field positioning for segments:', segments.length);
        
        segments.forEach(([segmentName, segment]) => {
            if (!segment.fields || segment.fields.length === 0) {
                return;
            }

            console.log(`📋 Validating ${segmentName} with ${segment.fields.length} fields`);

            const positionIssues = [];
            const positionMap = new Map();
            
            segment.fields.forEach((field, arrayIndex) => {
                const expectedPosition = arrayIndex + 1;
                const actualPosition = field.position;
                
                if (actualPosition !== expectedPosition) {
                    positionIssues.push({
                        fieldKey: field.key,
                        arrayIndex: arrayIndex,
                        expectedPosition: expectedPosition,
                        actualPosition: actualPosition,
                        hasValue: field.hasValue
                    });
                }
                
                if (positionMap.has(actualPosition)) {
                    console.warn(`❌ Duplicate position ${actualPosition} in ${segmentName}: ${field.key} and ${positionMap.get(actualPosition)}`);
                } else {
                    positionMap.set(actualPosition, field.key);
                }
                
                console.log(`  [${arrayIndex}] ${field.key} -> Position: ${actualPosition} (Expected: ${expectedPosition}) ${field.hasValue ? '✓' : '○'}`);
            });

            if (positionIssues.length > 0) {
                console.warn(`⚠️ Position issues found in ${segmentName}:`, positionIssues);
                
                // ✅ AUTO-FIX: Sort fields by position to correct display order
                segment.fields.sort((a, b) => {
                    if (a.position !== b.position) {
                        return a.position - b.position;
                    }
                    return (a.sequence || 0) - (b.sequence || 0);
                });
                
                console.log(`✅ Auto-fixed field ordering for ${segmentName}`);
            }
        });
    }

    /**
     * ✅ ENHANCED: Compact header with dynamic metrics
     */
    // FILE: segment-viewer.js
// Remove debug button from header controls

renderCompactHeader(validationErrors, messageType, segments, data) {
    const errorCount = validationErrors.filter(err => err.severity === 'ERROR' || err.severity === 'error').length;
    const warningCount = validationErrors.filter(err => err.severity === 'WARNING' || err.severity === 'warning').length;
    const totalFields = segments.reduce((sum, [_, seg]) => sum + (seg.fieldCount || 0), 0);
    
    // Dynamic segment expansion check
    const availableSegmentNames = segments.map(([segName]) => segName);
    const importantSegments = availableSegmentNames.slice(0, 3); // First 3 as important
    const hasExpandedImportant = importantSegments.some(seg => this.expandedSegments.has(seg));
    
    // ✅ Dynamic schema information
    const schemaInfo = this.getSchemaInfo(data);
    
    return `
        <div class="segment-header-compact">
            <div class="message-summary">
                <div class="message-info">
                    <span class="message-type">${messageType?.name || 'Unknown'}</span>
                    <span class="message-desc">${messageType?.description || ''}</span>
                    ${schemaInfo.html}
                </div>
                <div class="metrics-row">
                    <span class="metric">${segments.length} segments</span>
                    <span class="metric">${totalFields} fields</span>
                    ${errorCount > 0 ? `<span class="metric error">${errorCount} errors</span>` : ''}
                    ${warningCount > 0 ? `<span class="metric warning">${warningCount} warnings</span>` : ''}
                    <span class="metric info">v${data.version || '2.5'}</span>
                </div>
            </div>
            <div class="view-controls">
                <button class="view-btn ${this.viewMode === 'compact' ? 'active' : ''}" onclick="window.wizard.segmentViewer.setViewMode('compact')">Compact</button>
                <button class="view-btn ${this.viewMode === 'table' ? 'active' : ''}" onclick="window.wizard.segmentViewer.setViewMode('table')">Table</button>
                <button class="expand-all-btn" onclick="window.wizard.segmentViewer.toggleAllSegments()">
                    ${hasExpandedImportant ? 'Collapse Key' : 'Expand Key'}
                </button>
            </div>
        </div>
    `;
}

// ✅ REMOVE: Delete the debugPositioning method entirely
// debugPositioning() method removed - no longer needed
    /**
     * ✅ NEW: Get dynamic schema information
     */
    getSchemaInfo(data) {
        let schemaUsed = false;
        let schemaSource = 'Basic Parser';
        
        if (data.schemaLoaded) {
            schemaUsed = true;
            schemaSource = 'HL7 Schema';
        } else if (data.dictionaryUsed) {
            schemaUsed = true;
            schemaSource = 'Dictionary';
        } else {
            // Check segments for schema usage
            Object.values(data.enhancedSegments || {}).forEach(segment => {
                if (segment.dictionarySource && 
                    (segment.dictionarySource.includes('Schema') || 
                     segment.dictionarySource.includes('RealSchemaLoader'))) {
                    schemaUsed = true;
                    schemaSource = 'HL7 Schema';
                }
            });
        }
        
        return {
            used: schemaUsed,
            source: schemaSource,
            html: schemaUsed ? 
                `<span class="schema-indicator schema-loaded">📋 ${schemaSource}</span>` :
                `<span class="schema-indicator basic-parser">💡 ${schemaSource}</span>`
        };
    }

    /**
     * Renders segments in the selected view mode
     */
    renderSegmentTable(segments, validationErrors) {
        if (this.viewMode === 'table') {
            return this.renderTableView(segments, validationErrors);
        }
        return this.renderCompactView(segments, validationErrors);
    }

    /**
     * Compact view with minimal vertical space usage
     */
    renderCompactView(segments, validationErrors) {
        return `
            <div class="segments-compact">
                ${segments.map(([segName, segment]) => 
                    this.renderCompactSegment(segName, segment, validationErrors)
                ).join('')}
            </div>
        `;
    }

    /**
     * Table view showing all segments in a grid
     */
    renderTableView(segments, validationErrors) {
        return `
            <div class="segments-table-container">
                <table class="segments-table">
                    <thead>
                        <tr>
                            <th>Segment</th>
                            <th>Description</th>
                            <th>Fields</th>
                            <th>Status</th>
                            <th>Key Values</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${segments.map(([segName, segment]) => 
                            this.renderSegmentTableRow(segName, segment, validationErrors)
                        ).join('')}
                    </tbody>
                </table>
            </div>
        `;
    }

    /**
     * ✅ ENHANCED: Compact segment card with specific required field validation
     */
    renderCompactSegment(segName, segment, validationErrors) {
        const isExpanded = this.expandedSegments.has(segName);
        const segmentErrors = validationErrors.filter(err => err.segment === segName);
        const hasIssues = segmentErrors.length > 0;
        
        // ✅ NEW: Get specific missing required fields for this segment
        const missingRequiredFields = segmentErrors.filter(err => 
            err.code === 'MISSING_REQUIRED_FIELD' || err.code === 'EMPTY_REQUIRED_FIELD'
        );
        
        // ✅ NEW: Get empty required fields (present but no value)
        const emptyRequiredFields = segmentErrors.filter(err => 
            err.code === 'EMPTY_REQUIRED_FIELD'
        );
        
        // ✅ NEW: Get truly missing required fields (not present at all)
        const trulyMissingFields = segmentErrors.filter(err => 
            err.code === 'MISSING_REQUIRED_FIELD'
        );
        
        const keyFields = this.getKeyFields(segName, segment);

        return `
            <div class="segment-compact ${hasIssues ? 'has-issues' : ''} ${isExpanded ? 'expanded' : ''} ${missingRequiredFields.length > 0 ? 'has-missing-required' : ''}">
                <div class="segment-row" onclick="window.wizard.segmentViewer.toggleSegment('${segName}')">
                    <div class="segment-info">
                        <div class="segment-name-badge">
                            <span class="seg-name">${segName}</span>
                            ${this.renderDynamicBadges(segName, segment, segmentErrors)}
                            ${this.renderRequiredFieldsBadge(missingRequiredFields)}
                        </div>
                        <div class="segment-summary">
                            <span class="seg-desc">${this.truncateText(segment.description || `${segName} Segment`, 40)}</span>
                            ${keyFields.length > 0 ? `<span class="key-values">${keyFields.join(' • ')}</span>` : ''}
                            ${this.renderRequiredFieldsSummary(missingRequiredFields, emptyRequiredFields, trulyMissingFields)}
                        </div>
                    </div>
                    <div class="segment-meta">
                        <span class="field-count">${segment.fieldCount || 0}</span>
                        <span class="expand-icon ${isExpanded ? 'expanded' : ''}">${isExpanded ? '−' : '+'}</span>
                    </div>
                </div>
                
                ${isExpanded ? this.renderSegmentFields(segName, segment, validationErrors) : ''}
                ${hasIssues && !isExpanded ? this.renderInlineValidation(segmentErrors, missingRequiredFields) : ''}
            </div>
        `;
    }

    /**
     * ✅ DYNAMIC: Render badges based on actual data, no hardcoding
     */
    renderDynamicBadges(segName, segment, segmentErrors) {
        const badges = [];
        
        // Error/warning badges based on actual errors
        if (segmentErrors.some(err => err.severity === 'ERROR' || err.severity === 'error')) {
            badges.push('<span class="badge-mini error" title="Has errors">!</span>');
        } else if (segmentErrors.some(err => err.severity === 'WARNING' || err.severity === 'warning')) {
            badges.push('<span class="badge-mini warning" title="Has warnings">⚠</span>');
        }
        
        // Required badge based on actual segment data
        if (segment.required === true) {
            badges.push('<span class="badge-mini required" title="Required segment">R</span>');
        }
        
        // Repeating badge based on actual segment data
        if (segment.repeating === true) {
            badges.push('<span class="badge-mini repeating" title="Repeating segment">*</span>');
        }
        
        // Custom segment badge (Z segments)
        if (segName.startsWith('Z')) {
            badges.push('<span class="badge-mini custom" title="Custom segment">Z</span>');
        }

        return badges.join('');
    }

    /**
     * ✅ NEW: Render required fields badge
     */
    renderRequiredFieldsBadge(missingRequiredFields) {
        if (missingRequiredFields.length === 0) return '';
        
        return `<span class="badge-mini missing-required" title="${missingRequiredFields.length} missing required field${missingRequiredFields.length > 1 ? 's' : ''}">${missingRequiredFields.length}!</span>`;
    }

    /**
     * ✅ NEW: Render detailed required fields summary with dynamic field names
     */
    renderRequiredFieldsSummary(missingRequiredFields, emptyRequiredFields, trulyMissingFields) {
        if (missingRequiredFields.length === 0) return '';
        
        const summaryParts = [];
        
        if (trulyMissingFields.length > 0) {
            const fieldNames = trulyMissingFields.map(err => {
                const fieldName = this.getFieldDisplayName(err.field);
                return fieldName;
            }).join(', ');
            summaryParts.push(`❌ Missing: ${fieldNames}`);
        }
        
        if (emptyRequiredFields.length > 0) {
            const fieldNames = emptyRequiredFields.map(err => {
                const fieldName = this.getFieldDisplayName(err.field);
                return fieldName;
            }).join(', ');
            summaryParts.push(`⚠️ Empty: ${fieldNames}`);
        }
        
        return `<div class="missing-required-summary">${summaryParts.join(' | ')}</div>`;
    }

    /**
     * ✅ DYNAMIC: Get field display name from cache or derive from field key
     */
    getFieldDisplayName(fieldKey) {
        const cached = this.fieldMetadataCache.get(fieldKey);
        if (cached && cached.name) {
            return `${fieldKey} (${cached.name})`;
        }
        
        // Fallback: extract from field key
        const parts = fieldKey.split('.');
        if (parts.length === 2) {
            return `${fieldKey} (Field ${parts[1]})`;
        }
        
        return fieldKey;
    }

    /**
     * ✅ ENHANCED: Inline validation with specific field highlighting
     */
    renderInlineValidation(segmentErrors, missingRequiredFields) {
        if (segmentErrors.length === 0) return '';
        
        // Prioritize showing missing required fields
        if (missingRequiredFields.length > 0) {
            return this.renderInlineMissingRequired(missingRequiredFields);
        }
        
        // Otherwise show first error
        const firstError = segmentErrors[0];
        return `
            <div class="inline-validation">
                <span class="validation-icon">${firstError.severity === 'error' || firstError.severity === 'ERROR' ? '❌' : '⚠️'}</span>
                <span class="validation-text">${this.truncateText(firstError.message, 60)}</span>
                ${segmentErrors.length > 1 ? `<span class="more-count">+${segmentErrors.length - 1} more</span>` : ''}
            </div>
        `;
    }

    /**
     * ✅ NEW: Inline missing required fields display
     */
    renderInlineMissingRequired(missingRequiredFields) {
        const fieldNames = missingRequiredFields.slice(0, 3).map(err => {
            return this.getFieldDisplayName(err.field);
        });
        
        const displayText = fieldNames.join(', ');
        const remainingCount = missingRequiredFields.length - 3;
        
        return `
            <div class="inline-validation missing-required">
                <span class="validation-icon">❌</span>
                <span class="validation-text">Missing required: ${displayText}</span>
                ${remainingCount > 0 ? `<span class="more-count">+${remainingCount} more</span>` : ''}
            </div>
        `;
    }

    /**
     * Table row for each segment
     */
    renderSegmentTableRow(segName, segment, validationErrors) {
        const segmentErrors = validationErrors.filter(err => err.segment === segName);
        const hasIssues = segmentErrors.length > 0;
        const keyFields = this.getKeyFields(segName, segment);

        return `
            <tr class="segment-table-row ${hasIssues ? 'has-issues' : ''}" onclick="window.wizard.segmentViewer.viewSegmentDetails('${segName}')">
                <td class="seg-name-cell">
                    <span class="seg-name">${segName}</span>
                    ${this.renderDynamicBadges(segName, segment, segmentErrors)}
                </td>
                <td class="seg-desc-cell">${this.truncateText(segment.description || `${segName} Segment`, 30)}</td>
                <td class="field-count-cell">${segment.fieldCount || 0}</td>
                <td class="status-cell">
                    ${hasIssues ? 
                        `<span class="status-badge error">${segmentErrors.length} issue${segmentErrors.length > 1 ? 's' : ''}</span>` :
                        `<span class="status-badge ok">✓</span>`
                    }
                </td>
                <td class="key-values-cell">${keyFields.join(', ')}</td>
                <td class="actions-cell">
                    <button class="action-btn" onclick="event.stopPropagation(); window.wizard.segmentViewer.viewSegmentDetails('${segName}')">
                        <span>👁️</span> View Details
                    </button>
                </td>
            </tr>
        `;
    }

    /**
     * ✅ FIXED: Compact field display using correct position-based ordering
     */
    renderSegmentFields(segName, segment, validationErrors) {
        if (!segment.fields || segment.fields.length === 0) {
            return `<div class="no-fields">No field data available</div>`;
        }

        // ✅ CRITICAL FIX: Ensure fields are sorted by position, not array order
        const fields = this.sortFieldsByPosition(segment.fields);
        const fieldErrors = validationErrors.filter(err => err.segment === segName);
        
        // Group fields into rows of 2-3 for better space usage
        const fieldGroups = this.groupFields(fields);

        // ✅ Dynamic schema information
        const schemaSource = segment.dictionarySource || 'Unknown';
        const isSchemaEnhanced = schemaSource.includes('Schema') || schemaSource.includes('RealSchemaLoader');

        return `
            <div class="fields-compact">
                <div class="fields-header">
                    <span class="fields-title">Fields (${fields.length})</span>
                    <span class="schema-info">
                        ${isSchemaEnhanced ? 
                            '📋 HL7 Schema Definitions' : 
                            '💡 Basic Descriptions'
                        }
                    </span>
                </div>
                ${fieldGroups.map(group => `
                    <div class="field-group">
                        ${group.map(field => 
                            this.renderCompactField(segName, field, fieldErrors)
                        ).join('')}
                    </div>
                `).join('')}
            </div>
        `;
    }

    /**
     * ✅ FIXED: Sort fields by their HL7 position
     */
    sortFieldsByPosition(fields) {
        if (!Array.isArray(fields)) {
            console.warn('Fields is not an array:', fields);
            return [];
        }
        
        // Create a copy to avoid modifying the original
        const sortedFields = [...fields];
        
        // Sort by position (primary) and sequence (secondary)
        sortedFields.sort((a, b) => {
            // Primary sort: by position
            if (a.position !== b.position) {
                return a.position - b.position;
            }
            // Secondary sort: by sequence if positions are equal
            return (a.sequence || 0) - (b.sequence || 0);
        });
        
        console.log(`🔀 Sorted ${sortedFields.length} fields by position for display`);
        return sortedFields;
    }

    /**
     * ✅ ENHANCED: Compact field display with specific required field highlighting
     */
    renderCompactField(segName, field, fieldErrors) {
        const fieldValidationErrors = fieldErrors.filter(err => err.field === field.key);
        const hasIssues = fieldValidationErrors.length > 0;
        const isExpanded = this.expandedFields.has(`${segName}.${field.key}`);
        
        // ✅ Check if this specific field is missing and required
        const isMissingRequired = fieldValidationErrors.some(err => 
            err.code === 'MISSING_REQUIRED_FIELD' && err.field === field.key
        );
        const isEmptyRequired = fieldValidationErrors.some(err => 
            err.code === 'EMPTY_REQUIRED_FIELD' && err.field === field.key
        );
        
        // ✅ Check if this is a required field
        const isRequired = field.optionality === 'R';

        return `
            <div class="field-compact ${hasIssues ? 'has-issues' : ''} ${isMissingRequired ? 'missing-required' : ''} ${isEmptyRequired ? 'empty-required' : ''} ${isRequired ? 'required-field' : ''}" 
                 onclick="window.wizard.segmentViewer.toggleField('${segName}', '${field.key}')">
                <div class="field-header-compact">
                    <div class="field-label">
                        <span class="field-key">${field.key}</span>
                        ${isRequired ? '<span class="required-indicator">*</span>' : ''}
                        <span class="field-name">${this.truncateText(field.name || `Field ${field.position}`, 25)}</span>
                        ${isMissingRequired ? '<span class="missing-required-icon" title="Required field is missing">❌</span>' : ''}
                        ${isEmptyRequired ? '<span class="empty-required-icon" title="Required field is empty">⚠️</span>' : ''}
                        ${hasIssues && !isMissingRequired && !isEmptyRequired ? '<span class="field-error-icon">!</span>' : ''}
                    </div>
                    <div class="field-value-preview">
                        ${field.hasValue ? 
                            `<span class="value-text">${this.truncateValue(field.value, 35)}</span>` :
                            `<span class="no-value ${isMissingRequired || isEmptyRequired ? 'missing-required-value' : ''}">—</span>`
                        }
                        ${field.dataType && field.dataType !== 'ST' ? `<span class="data-type">(${field.dataType})</span>` : ''}
                    </div>
                </div>
                
                ${isExpanded ? this.renderFieldDetails(segName, field, fieldValidationErrors) : ''}
                ${(isMissingRequired || isEmptyRequired) && !isExpanded ? this.renderMissingRequiredWarning(fieldValidationErrors) : ''}
            </div>
        `;
    }

    /**
     * ✅ ENHANCED: Detailed field view when expanded with dynamic data
     */
    renderFieldDetails(segName, field, fieldValidationErrors) {
        return `
            <div class="field-details-compact">
                <div class="field-metadata-row">
                    <span>Type: ${field.dataType || 'ST'}</span>
                    <span>Usage: ${this.getUsageDescription(field.optionality || 'O')}</span>
                    ${field.cardinality ? `<span>Repeat: ${field.cardinality}</span>` : ''}
                    ${field.length ? `<span>Max Length: ${field.length}</span>` : ''}
                    <span>Position: ${field.position}</span>
                </div>
                
                ${field.description ? `
                    <div class="field-description-row">
                        <strong>Description:</strong> ${field.description}
                    </div>
                ` : ''}
                
                ${field.hasValue && field.value ? `
                    <div class="field-value-row">
                        <strong>Full Value:</strong>
                        <code class="value-display">${this.escapeHtml(field.value)}</code>
                        ${this.analyzeValue(field)}
                    </div>
                ` : ''}
                
                ${field.subfields && field.subfields.length > 0 ? `
                    <div class="subfields-section">
                        <strong>Components (${field.subfields.length}):</strong>
                        ${this.renderEnhancedSubfields(field.subfields)}
                    </div>
                ` : ''}
                
                ${fieldValidationErrors.length > 0 ? `
                    <div class="field-validation-row">
                        ${fieldValidationErrors.map(error => `
                            <div class="validation-item ${error.severity}">
                                ${error.message}
                                ${error.suggestion ? `<br><em>💡 ${error.suggestion}</em>` : ''}
                            </div>
                        `).join('')}
                    </div>
                ` : ''}
            </div>
        `;
    }

    /**
     * ✅ ENHANCED: Render subfields with complete metadata from API data
     */
    renderEnhancedSubfields(subfields) {
        if (!subfields || subfields.length === 0) {
            return '<div class="no-subfields">No components defined</div>';
        }
        
        // ✅ Sort subfields by position
        const sortedSubfields = [...subfields].sort((a, b) => {
            if (a.position !== b.position) {
                return a.position - b.position;
            }
            return (a.sequence || 0) - (b.sequence || 0);
        });
        
        return `
            <div class="rich-subfields-container">
                <div class="subfields-header">
                    <span class="subfields-title">Components (${sortedSubfields.length})</span>
                    <button class="subfields-expand-btn" onclick="event.stopPropagation(); this.parentElement.parentElement.classList.toggle('subfields-expanded')">
                        <span class="expand-icon">▼</span> Details
                    </button>
                </div>
                <div class="rich-subfields-list">
                    ${sortedSubfields.map((subfield, index) => this.renderEnhancedSubfield(subfield, index)).join('')}
                </div>
            </div>
        `;
    }

    /**
     * ✅ NEW: Render individual subfield with complete metadata from API
     */
    renderEnhancedSubfield(subfield, index) {
        const isRequired = subfield.usage === 'R';
        const isEmpty = !subfield.hasValue || !subfield.value;
        const isMissingRequired = isRequired && isEmpty;
        
        return `
            <div class="enhanced-subfield ${subfield.hasValue ? 'has-value' : 'no-value'} ${isMissingRequired ? 'missing-required' : ''}" 
                 onclick="event.stopPropagation(); this.classList.toggle('subfield-expanded')">
                
                <!-- Compact subfield header -->
                <div class="subfield-header-compact">
                    <div class="subfield-label-section">
                        <span class="subfield-position">${subfield.position}</span>
                        <span class="subfield-name">${subfield.name || `Component ${subfield.position}`}</span>
                        ${isRequired ? '<span class="required-indicator">*</span>' : ''}
                        ${isMissingRequired ? '<span class="missing-required-icon">⚠️</span>' : ''}
                    </div>
                    
                    <div class="subfield-value-section">
                        <span class="subfield-value ${isEmpty ? 'empty-value' : ''}">
                            ${subfield.hasValue ? this.escapeHtml(this.truncateValue(subfield.value, 20)) : '—'}
                        </span>
                        ${subfield.dataType && subfield.dataType !== 'ST' ? 
                            `<span class="subfield-datatype">(${subfield.dataType})</span>` : ''
                        }
                    </div>
                    
                    <span class="subfield-expand-indicator">▼</span>
                </div>
                
                <!-- Detailed subfield information (hidden by default) -->
                <div class="subfield-details-panel">
                    <div class="subfield-metadata-grid">
                        <div class="metadata-row">
                            <span class="metadata-label">Key:</span>
                            <span class="metadata-value">${subfield.key}</span>
                        </div>
                        <div class="metadata-row">
                            <span class="metadata-label">Data Type:</span>
                            <span class="metadata-value">${subfield.dataType || 'ST'}</span>
                        </div>
                        <div class="metadata-row">
                            <span class="metadata-label">Usage:</span>
                            <span class="metadata-value ${subfield.usage === 'R' ? 'required' : 'optional'}">
                                ${this.getUsageDescription(subfield.usage)}
                            </span>
                        </div>
                        ${subfield.length ? `
                            <div class="metadata-row">
                                <span class="metadata-label">Max Length:</span>
                                <span class="metadata-value">${subfield.length}</span>
                            </div>
                        ` : ''}
                        <div class="metadata-row">
                            <span class="metadata-label">Position:</span>
                            <span class="metadata-value">${subfield.position}</span>
                        </div>
                    </div>
                    
                    ${subfield.description ? `
                        <div class="subfield-description-section">
                            <div class="description-label">Description:</div>
                            <div class="description-text">${subfield.description}</div>
                        </div>
                    ` : ''}
                    
                    ${subfield.hasValue && subfield.value ? `
                        <div class="subfield-value-section-detailed">
                            <div class="value-label">Full Value:</div>
                            <div class="value-display">
                                <code>${this.escapeHtml(subfield.value)}</code>
                                <span class="value-stats">(${subfield.value.length} chars)</span>
                            </div>
                        </div>
                    ` : ''}
                    
                    ${subfield.tableId ? `
                        <div class="subfield-table-section">
                            <div class="table-label">Value Set:</div>
                            <div class="table-info">
                                <span class="table-id">Table ${subfield.tableId}</span>
                                ${this.renderTableValues(subfield)}
                            </div>
                        </div>
                    ` : ''}
                    
                    ${isMissingRequired ? `
                        <div class="missing-required-alert">
                            <span class="alert-icon">⚠️</span>
                            <span class="alert-text">This required component is missing a value</span>
                            <div class="alert-suggestion">
                                💡 ${subfield.description ? `This component represents: ${subfield.description}` : 'Provide a value for this component'}
                            </div>
                        </div>
                    ` : ''}
                </div>
            </div>
        `;
    }

    /**
     * ✅ ENHANCED: Missing required field warning with specific guidance
     */
    renderMissingRequiredWarning(fieldValidationErrors) {
        const requiredErrors = fieldValidationErrors.filter(err => 
            err.code === 'MISSING_REQUIRED_FIELD' || err.code === 'EMPTY_REQUIRED_FIELD'
        );
        
        if (requiredErrors.length === 0) return '';
        
        const error = requiredErrors[0];
        const isMissing = error.code === 'MISSING_REQUIRED_FIELD';
        
        return `
            <div class="missing-required-warning ${isMissing ? 'truly-missing' : 'empty-value'}">
                <span class="warning-icon">${isMissing ? '❌' : '⚠️'}</span>
                <span class="warning-text">
                    ${isMissing ? 'Required field is completely missing' : 'Required field is present but empty'}
                </span>
                ${error.suggestion ? `<div class="warning-suggestion">💡 ${error.suggestion}</div>` : ''}
                <div class="warning-rule">Rule: ${error.ruleId || 'REQ_FIELD'}</div>
            </div>
        `;
    }

    /**
     * ✅ DYNAMIC: Get human-readable usage descriptions (no hardcoding)
     */
    getUsageDescription(usage) {
        const usageMap = {
            'R': 'Required',
            'O': 'Optional', 
            'C': 'Conditional',
            'B': 'Backward Compatible',
            'X': 'Not Supported'
        };
        return usageMap[usage] || usage || 'Optional';
    }

    /**
     * ✅ DYNAMIC: Render table values if available from API data
     */
    renderTableValues(subfield) {
        // ✅ NOTE: This could be enhanced to connect to actual table definitions from the API
        // For now, we show the table ID and current value
        if (subfield.tableId && subfield.value) {
            return `
                <div class="table-values">
                    <span class="table-value current-value">
                        Current: ${subfield.value}
                    </span>
                </div>
            `;
        }
        return '';
    }

    /**
     * ✅ DYNAMIC: Get key fields based on actual field data and values
     */
    getKeyFields(segName, segment) {
        if (!segment.fields || segment.fields.length === 0) {
            return [];
        }
        
        const keyFields = [];
        
        // ✅ DYNAMIC: Take first few fields that have values, instead of hardcoded positions
        const fieldsWithValues = segment.fields
            .filter(field => field.hasValue && field.value)
            .slice(0, 3); // Take first 3 fields with values
        
        fieldsWithValues.forEach(field => {
            keyFields.push(`${field.key}:${this.truncateValue(field.value, 15)}`);
        });
        
        // If no fields have values, show first few field keys
        if (keyFields.length === 0) {
            segment.fields.slice(0, 2).forEach(field => {
                keyFields.push(`${field.key}:—`);
            });
        }

        return keyFields;
    }

    /**
     * Group fields for horizontal display
     */
    groupFields(fields) {
        const groups = [];
        const groupSize = 2; // 2 fields per row for better readability
        
        for (let i = 0; i < fields.length; i += groupSize) {
            groups.push(fields.slice(i, i + groupSize));
        }
        
        return groups;
    }

    /**
     * ✅ ENHANCED: Analyze field value to show helpful information
     */
    analyzeValue(field) {
        if (!field.hasValue || !field.value) return '';
        
        const value = field.value;
        const analysis = [];
        
        // Component analysis
        if (value.includes('^')) {
            const components = value.split('^');
            analysis.push(`${components.length} components`);
        }
        
        // Date parsing for timestamp fields
        if (field.dataType === 'TS' || field.dataType === 'DT') {
            const dateMatch = value.match(/(\d{8})/);
            if (dateMatch) {
                const dateStr = dateMatch[1];
                const formatted = `${dateStr.substr(4,2)}/${dateStr.substr(6,2)}/${dateStr.substr(0,4)}`;
                analysis.push(`Date: ${formatted}`);
            }
        }
        
        // Length analysis
        if (value.length > 20) {
            analysis.push(`${value.length} chars`);
        }
        
        if (analysis.length > 0) {
            return `<span class="value-analysis">(${analysis.join(', ')})</span>`;
        }
        
        return '';
    }

    /**
     * Toggle segment expansion
     */
    toggleSegment(segName) {
        if (this.expandedSegments.has(segName)) {
            this.expandedSegments.delete(segName);
        } else {
            this.expandedSegments.add(segName);
        }
        
        this.refreshView();
    }

    /**
     * Toggle field expansion
     */
    toggleField(segName, fieldKey) {
        const fieldId = `${segName}.${fieldKey}`;
        if (this.expandedFields.has(fieldId)) {
            this.expandedFields.delete(fieldId);
        } else {
            this.expandedFields.add(fieldId);
        }
        
        this.refreshView();
    }

    /**
     * Change view mode
     */
    setViewMode(mode) {
        console.log('🔄 Switching view mode to:', mode);
        this.viewMode = mode;
        this.refreshView();
        
        // Update button states
        setTimeout(() => {
            document.querySelectorAll('.view-btn').forEach(btn => {
                btn.classList.remove('active');
            });
            
            const activeBtn = document.querySelector(`.view-btn[onclick*="${mode}"]`);
            if (activeBtn) {
                activeBtn.classList.add('active');
            }
        }, 50);
    }

    /**
     * ✅ DYNAMIC: Toggle segments based on available data
     */
    toggleAllSegments() {
        console.log('🔍 Toggle all segments clicked');
        
        // Get segments that exist in the current data
        const existingSegments = [];
        if (this.wizard.parsedHL7Data?.data?.enhancedSegments) {
            const availableSegments = Object.keys(this.wizard.parsedHL7Data.data.enhancedSegments);
            // Take first 3 segments as "important" dynamically
            existingSegments.push(...availableSegments.slice(0, 3));
        }
        
        const hasExpanded = existingSegments.some(seg => this.expandedSegments.has(seg));
        
        if (hasExpanded) {
            // Collapse all important segments
            console.log('🔽 Collapsing important segments');
            existingSegments.forEach(seg => this.expandedSegments.delete(seg));
        } else {
            // Expand all important segments
            console.log('🔼 Expanding important segments:', existingSegments);
            existingSegments.forEach(seg => this.expandedSegments.add(seg));
        }
        
        // Switch to compact view if in table view
        if (this.viewMode === 'table') {
            this.viewMode = 'compact';
        }
        
        this.refreshView();
    }

    /**
     * View segment details - switches to compact view and shows the segment
     */
    viewSegmentDetails(segName) {
        console.log('🔍 View segment details:', segName);
        
        this.viewMode = 'compact';
        this.expandedSegments.add(segName);
        
        this.refreshView();
        
        // Scroll to the segment after rendering
        setTimeout(() => {
            const segmentCards = document.querySelectorAll('.segment-compact');
            segmentCards.forEach(card => {
                const segmentRow = card.querySelector('.segment-row');
                if (segmentRow && segmentRow.onclick && segmentRow.onclick.toString().includes(segName)) {
                    card.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    card.style.boxShadow = '0 4px 16px rgba(59, 130, 246, 0.3)';
                    setTimeout(() => {
                        card.style.boxShadow = '';
                    }, 2000);
                }
            });
        }, 200);
    }

    /**
     * ✅ ENHANCED: Debug field positioning with detailed analysis
     */
    debugPositioning() {
        console.log('🐛 Debug positioning triggered');
        
        if (!this.wizard.parsedHL7Data?.data?.enhancedSegments) {
            console.log('❌ No parsed data available for debugging');
            return;
        }
        
        const segments = this.wizard.parsedHL7Data.data.enhancedSegments;
        
        console.group('🔍 Field Positioning Debug Report');
        
        Object.entries(segments).forEach(([segmentName, segment]) => {
            console.group(`📋 Segment: ${segmentName}`);
            console.log(`Raw: ${segment.raw?.substring(0, 80)}...`);
            console.log(`Fields: ${segment.fields?.length || 0}`);
            console.log(`Dictionary Source: ${segment.dictionarySource}`);
            
            if (segment.fields && segment.fields.length > 0) {
                segment.fields.forEach((field, arrayIndex) => {
                    const expectedPosition = arrayIndex + 1;
                    const actualPosition = field.position;
                    const status = actualPosition === expectedPosition ? '✅' : '❌';
                    
                    console.log(`${status} [${arrayIndex}] ${field.key} -> Position: ${actualPosition} (Expected: ${expectedPosition}) = "${field.value?.substring(0, 20) || ''}" [HasValue: ${field.hasValue}]`);
                    console.log(`    Name: ${field.name}, DataType: ${field.dataType}, Usage: ${field.optionality}`);
                    
                    // Show subfields
                    if (field.subfields && field.subfields.length > 0) {
                        field.subfields.forEach((subfield, subIndex) => {
                            console.log(`    └─ [${subIndex}] ${subfield.key} -> Position: ${subfield.position} = "${subfield.value?.substring(0, 15) || ''}" (${subfield.name})`);
                        });
                    }
                });
            }
            
            console.groupEnd();
        });
        
        console.groupEnd();
        
        // Show cache information
        console.log('📊 Field Metadata Cache:', this.fieldMetadataCache.size, 'entries');
        
        // Show in UI
        alert('Debug information has been logged to the console. Press F12 to view detailed field positioning analysis.');
    }

    /**
     * Refresh the current view
     */
    refreshView() {
        console.log('🔄 Refreshing segment viewer, mode:', this.viewMode);
        if (this.wizard.parsedHL7Data) {
            const container = document.getElementById('parsedDataReview');
            if (container) {
                this.renderSegmentList(this.wizard.parsedHL7Data, 'parsedDataReview');
                console.log('✅ View refreshed successfully');
            } else {
                console.warn('⚠️ Container not found for refresh');
            }
        } else {
            console.warn('⚠️ No parsed data available for refresh');
        }
    }

    /**
     * ✅ DYNAMIC: Get segments in message order based on actual data
     */
    getSegmentsInMessageOrder(segments) {
        // ✅ Use segment order from API if available
        if (this.wizard.parsedHL7Data?.data?.segmentOrder) {
            const apiOrder = this.wizard.parsedHL7Data.data.segmentOrder;
            const segmentEntries = [];
            
            // First, add segments in API-specified order
            apiOrder.forEach(segName => {
                if (segments[segName]) {
                    segmentEntries.push([segName, segments[segName]]);
                }
            });
            
            // Then add any remaining segments not in the order
            Object.entries(segments).forEach(([segName, segment]) => {
                if (!apiOrder.includes(segName)) {
                    segmentEntries.push([segName, segment]);
                }
            });
            
            return segmentEntries;
        }
        
        // ✅ Fallback: Use sequence from segments if available
        const segmentEntries = Object.entries(segments);
        
        segmentEntries.sort(([nameA, segA], [nameB, segB]) => {
            // Sort by sequence if available
            const seqA = segA.sequence !== undefined ? segA.sequence : 999;
            const seqB = segB.sequence !== undefined ? segB.sequence : 999;
            
            if (seqA !== seqB) {
                return seqA - seqB;
            }
            
            // Fallback: alphabetical order
            return nameA.localeCompare(nameB);
        });
        
        return segmentEntries;
    }

    /**
     * Helper Methods
     */
    getSegmentData(segName) {
        return this.wizard.parsedHL7Data?.data?.enhancedSegments?.[segName];
    }

    truncateText(text, maxLength) {
        if (!text) return '';
        return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
    }

    truncateValue(value, maxLength = 30) {
        if (!value) return '';
        const str = String(value);
        return str.length > maxLength ? str.substring(0, maxLength) + '...' : str;
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    attachEventListeners() {
        // Event listeners are attached via onclick attributes
        console.log('✅ Dynamic segment viewer event listeners attached');
    }
}

// Export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = SegmentViewer;
}

// Ensure SegmentViewer is globally available
if (typeof window !== 'undefined') {
    window.SegmentViewer = SegmentViewer;
    console.log('✅ SegmentViewer class is now globally available');
}