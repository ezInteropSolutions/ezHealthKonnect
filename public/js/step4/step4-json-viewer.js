// step4-json-viewer.js - Enhanced JSON Viewer with collapsible tree

class Step4JsonViewer {
    constructor(handler) {
        this.handler = handler;
        this.currentTab = 'bundle';
        this.currentJson = null;
    }

    open() {
        const modal = document.getElementById('jsonViewerOverlay');
        if (modal) {
            modal.classList.add('show');
            this.displayJson('bundle');
        }
    }

    close() {
        const modal = document.getElementById('jsonViewerOverlay');
        if (modal) {
            modal.classList.remove('show');
        }
    }

    displayJson(tabType) {
        this.currentTab = tabType;
        this.displayJsonTab(tabType);
    }

    displayJsonTab(tabType) {
        const jsonOutput = document.getElementById('jsonOutput');
        const tabs = document.querySelectorAll('.json-tab');
        
        // Update active tab
        tabs.forEach(tab => {
            tab.classList.toggle('active', tab.dataset.tab === tabType);
        });

        this.currentTab = tabType;
        let content = null;

        switch (tabType) {
            case 'bundle':
                content = this.createBundle();
                break;
            
            case 'resources':
                content = this.getIndividualResources();
                break;
            
            case 'validation':
                content = this.getValidationReport();
                break;
        }

        if (jsonOutput && content) {
            this.currentJson = content;
            jsonOutput.innerHTML = this.renderCollapsibleJson(content);
            this.attachJsonEventListeners();
        }
    }

    createBundle() {
        const resources = this.handler.transformationData?.fhirResources;
        if (!resources || !Array.isArray(resources)) {
            return {
                resourceType: "Bundle",
                type: "transaction",
                entry: []
            };
        }
        
        return {
            resourceType: "Bundle",
            id: `bundle-${Date.now()}`,
            type: "transaction",
            timestamp: new Date().toISOString(),
            entry: resources.map(resource => ({
                fullUrl: `urn:uuid:${this.generateUUID()}`,
                resource: resource,
                request: {
                    method: "POST",
                    url: resource.resourceType
                }
            }))
        };
    }

    getIndividualResources() {
        return this.handler.transformationData?.fhirResources || [];
    }

    getValidationReport() {
        const data = this.handler.transformationData;
        const totalMappings = data?.mappingStats?.totalFieldsMapped || 0;
        const errors = this.getFilteredErrors();
        const warnings = this.getFilteredWarnings();
        const successfulMappings = Math.max(0, totalMappings - errors.length);
        const score = totalMappings > 0 ? 
            Math.round((successfulMappings / totalMappings) * 100) : 100;

        return {
            timestamp: new Date().toISOString(),
            messageType: data?.messageType || 'Unknown',
            summary: {
                status: data?.success !== false ? 'SUCCESS' : 'HAS_ISSUES',
                validationScore: `${score}%`,
                statistics: {
                    totalMappings: totalMappings,
                    successfulMappings: successfulMappings,
                    warnings: warnings.length,
                    errors: errors.length
                }
            },
            resources: {
                total: data?.fhirResources?.length || 0,
                types: this.getResourceTypes()
            },
            issues: {
                errors: errors.map(e => ({
                    severity: 'error',
                    message: e
                })),
                warnings: warnings.map(w => ({
                    severity: 'warning',
                    message: w
                }))
            },
            performance: {
                processingTime: data?.performance?.totalTime || 'N/A',
                timestamp: data?.performance?.timestamp || new Date().toISOString()
            }
        };
    }

    renderCollapsibleJson(obj, indent = 0) {
        if (obj === null) return '<span class="json-null">null</span>';
        if (obj === undefined) return '<span class="json-null">undefined</span>';
        
        const type = typeof obj;
        
        if (type === 'string') {
            return `<span class="json-string">"${this.escapeHtml(obj)}"</span>`;
        }
        
        if (type === 'number') {
            return `<span class="json-number">${obj}</span>`;
        }
        
        if (type === 'boolean') {
            return `<span class="json-boolean">${obj}</span>`;
        }
        
        if (Array.isArray(obj)) {
            if (obj.length === 0) return '[]';
            
            const id = `json-array-${Math.random().toString(36).substr(2, 9)}`;
            let html = `<span class="json-key json-expanded" data-target="${id}">[</span>`;
            html += `<div class="json-container" id="${id}">`;
            
            obj.forEach((item, index) => {
                const isLast = index === obj.length - 1;
                html += `<div style="margin: 2px 0;">`;
                
                // Add index for arrays of objects
                if (typeof item === 'object' && item !== null) {
                    html += `<span style="color: #6b7280; margin-right: 8px;">[${index}]</span>`;
                }
                
                html += this.renderCollapsibleJson(item, indent + 1);
                if (!isLast) html += ',';
                html += '</div>';
            });
            
            html += '</div>]';
            return html;
        }
        
        if (type === 'object') {
            const keys = Object.keys(obj);
            if (keys.length === 0) return '{}';
            
            const id = `json-object-${Math.random().toString(36).substr(2, 9)}`;
            const isExpanded = indent < 2; // Auto-expand first 2 levels
            
            let html = `<span class="json-key ${isExpanded ? 'json-expanded' : 'json-collapsed'}" data-target="${id}">{</span>`;
            html += `<div class="json-container ${isExpanded ? '' : 'json-hidden'}" id="${id}">`;
            
            keys.forEach((key, index) => {
                const isLast = index === keys.length - 1;
                html += `<div style="margin: 2px 0;">`;
                html += `<span class="json-key">"${key}"</span>: `;
                html += this.renderCollapsibleJson(obj[key], indent + 1);
                if (!isLast) html += ',';
                html += '</div>';
            });
            
            html += '</div>}';
            return html;
        }
        
        return String(obj);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    attachJsonEventListeners() {
        document.querySelectorAll('.json-key[data-target]').forEach(key => {
            key.addEventListener('click', (e) => {
                e.stopPropagation();
                const targetId = key.dataset.target;
                const target = document.getElementById(targetId);
                
                if (target) {
                    if (target.classList.contains('json-hidden')) {
                        target.classList.remove('json-hidden');
                        key.classList.remove('json-collapsed');
                        key.classList.add('json-expanded');
                    } else {
                        target.classList.add('json-hidden');
                        key.classList.remove('json-expanded');
                        key.classList.add('json-collapsed');
                    }
                }
            });
        });
    }

    async copyJson() {
        try {
            const jsonText = JSON.stringify(this.currentJson, null, 2);
            await navigator.clipboard.writeText(jsonText);
            this.handler.showNotification('JSON copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy JSON:', error);
            this.handler.showNotification('Failed to copy JSON', 'error');
        }
    }

    downloadJson() {
        try {
            const jsonText = JSON.stringify(this.currentJson, null, 2);
            const blob = new Blob([jsonText], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            
            const filename = `fhir-${this.currentTab}-${Date.now()}.json`;
            a.href = url;
            a.download = filename;
            a.click();
            
            URL.revokeObjectURL(url);
            this.handler.showNotification(`Downloaded ${filename}`, 'success');
        } catch (error) {
            console.error('Failed to download JSON:', error);
            this.handler.showNotification('Failed to download JSON', 'error');
        }
    }

    switchTab(tab) {
        this.displayJsonTab(tab);
    }

    // Helper methods
    getFilteredErrors() {
        if (!this.handler.transformationData?.errors) return [];
        
        const createdResources = this.handler.transformationData.fhirResources?.map(r => r.resourceType) || [];
        return this.handler.transformationData.errors.filter(error => 
            createdResources.some(type => error.includes(type))
        );
    }

    getFilteredWarnings() {
        if (!this.handler.transformationData?.warnings) return [];
        
        const createdResources = this.handler.transformationData.fhirResources?.map(r => r.resourceType) || [];
        return this.handler.transformationData.warnings.filter(warning => 
            createdResources.some(type => warning.includes(type))
        );
    }

    getResourceTypes() {
        const resources = this.handler.transformationData?.fhirResources || [];
        const types = {};
        resources.forEach(r => {
            types[r.resourceType] = (types[r.resourceType] || 0) + 1;
        });
        return types;
    }

    generateUUID() {
        return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
            const r = Math.random() * 16 | 0;
            const v = c === 'x' ? r : (r & 0x3 | 0x8);
            return v.toString(16);
        });
    }
}

// Export for use
window.Step4JsonViewer = Step4JsonViewer;