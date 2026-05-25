class Step4ConfigManager {
    constructor(handler) {
        this.handler = handler;
        this.savedConfigurations = new Map();
        this.currentConfig = null;
    }

    openSaveModal() {
        const modal = document.getElementById('configSaveOverlay');
        if (!modal) return;

        // Pre-populate with current data
        this.populateSaveModal();
        modal.classList.add('show');
    }

    closeSaveModal() {
        const modal = document.getElementById('configSaveOverlay');
        if (modal) {
            modal.classList.remove('show');
            this.clearSaveModal();
        }
    }

    populateSaveModal() {
        const data = this.handler.transformationData;
        if (!data) return;

        const nameInput = document.getElementById('configName');
        const descInput = document.getElementById('configDescription');
        
        if (nameInput) {
            nameInput.value = `${data.messageType || 'Unknown'} Standard Mapping`;
        }
        
        if (descInput) {
            const resourceCount = data.fhirResources?.length || 0;
            const fieldCount = data.mappingStats?.totalFieldsMapped || 0;
            descInput.value = `Configuration for ${data.messageType} with ${resourceCount} resources and ${fieldCount} field mappings`;
        }
    }

    clearSaveModal() {
        const nameInput = document.getElementById('configName');
        const descInput = document.getElementById('configDescription');
        
        if (nameInput) nameInput.value = '';
        if (descInput) descInput.value = '';
    }

    async saveConfiguration() {
        const configName = document.getElementById('configName')?.value?.trim();
        const configDescription = document.getElementById('configDescription')?.value?.trim();

        if (!configName) {
            this.handler.showNotification('Please enter a configuration name', 'warning');
            return;
        }

        try {
            const config = this.createConfiguration(configName, configDescription);
            
            // Save to backend
            const saved = await this.saveToBackend(config);
            
            if (saved) {
                // Store locally
                this.savedConfigurations.set(config.id, config);
                this.currentConfig = config;
                
                this.handler.showNotification('✅ Configuration saved successfully!', 'success');
                this.closeSaveModal();
            }
        } catch (error) {
            console.error('❌ Failed to save configuration:', error);
            this.handler.showNotification('Failed to save configuration', 'error');
        }
    }

    createConfiguration(name, description) {
        const data = this.handler.transformationData;
        const mappings = this.handler.mapping?.editableMappings || new Map();
        
        return {
            id: `config_${Date.now()}`,
            name: name,
            description: description,
            messageType: data?.messageType || 'Unknown',
            version: '1.0.0',
            createdAt: new Date().toISOString(),
            createdBy: this.getCurrentUser(),
            fhirVersion: 'R4',
            resources: data?.fhirResources?.map(r => ({
                type: r.resourceType,
                count: 1,
                mappings: this.extractResourceMappings(r)
            })) || [],
            customMappings: Array.from(mappings.entries()),
            statistics: {
                totalResources: data?.fhirResources?.length || 0,
                totalMappings: data?.mappingStats?.totalFieldsMapped || 0,
                validationScore: this.calculateValidationScore()
            },
            metadata: {
                hl7Version: '2.x',
                environment: 'production',
                tags: ['auto-generated', data?.messageType]
            }
        };
    }

    extractResourceMappings(resource) {
        if (!this.handler.mapping) return [];
        return this.handler.mapping.extractDetailedMappings(resource);
    }

    calculateValidationScore() {
        const errors = this.handler.validation?.getFilteredErrors() || [];
        const total = this.handler.transformationData?.mappingStats?.totalFieldsMapped || 0;
        
        if (total === 0) return 100;
        return Math.max(0, Math.round(((total - errors.length) / total) * 100));
    }

    async saveToBackend(config) {
        try {
            const response = await fetch(`${this.handler.apiBaseUrl}/api/configurations`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${this.getAuthToken()}`
                },
                body: JSON.stringify(config)
            });

            return response.ok;
        } catch (error) {
            console.error('Backend save failed:', error);
            // Fallback to local storage
            this.saveToLocalStorage(config);
            return true;
        }
    }

    saveToLocalStorage(config) {
        const configs = JSON.parse(localStorage.getItem('fhir_configurations') || '[]');
        configs.push(config);
        localStorage.setItem('fhir_configurations', JSON.stringify(configs));
    }

    async loadConfiguration(configId) {
        try {
            // Try backend first
            const response = await fetch(`${this.handler.apiBaseUrl}/api/configurations/${configId}`);
            if (response.ok) {
                const config = await response.json();
                return this.applyConfiguration(config);
            }
        } catch (error) {
            console.error('Failed to load from backend:', error);
        }

        // Fallback to local storage
        const configs = JSON.parse(localStorage.getItem('fhir_configurations') || '[]');
        const config = configs.find(c => c.id === configId);
        
        if (config) {
            return this.applyConfiguration(config);
        }
        
        this.handler.showNotification('Configuration not found', 'error');
        return false;
    }

    applyConfiguration(config) {
        console.log('📦 Applying configuration:', config);
        
        // Apply custom mappings
        if (config.customMappings && this.handler.mapping) {
            config.customMappings.forEach(([key, value]) => {
                this.handler.mapping.editableMappings.set(key, value);
            });
        }
        
        this.currentConfig = config;
        this.handler.showNotification(`Loaded configuration: ${config.name}`, 'success');
        return true;
    }

    getCurrentUser() {
        return window.currentUser?.username || 'system';
    }

    getAuthToken() {
        return localStorage.getItem('auth_token') || '';
    }

    reset() {
        this.currentConfig = null;
    }
}
