class Step4Validation {
    constructor(handler) {
        this.handler = handler;
    }

    validate() {
        if (!this.handler.transformationData) {
            this.handler.showNotification('FHIR transformation is required', 'error');
            return false;
        }

        const errors = this.getFilteredErrors();
        if (errors.length > 0) {
            this.handler.showNotification(`Please resolve ${errors.length} validation errors`, 'warning');
            this.handler.openValidationSidebar();
            return false;
        }

        return true;
    }

    getFilteredErrors() {
        const data = this.handler.transformationData;
        if (!data?.errors) return [];
        
        const createdResources = data.fhirResources?.map(r => r.resourceType) || [];
        return data.errors.filter(error => 
            createdResources.some(type => error.includes(type))
        );
    }

    getFilteredWarnings() {
        const data = this.handler.transformationData;
        if (!data?.warnings) return [];
        
        const createdResources = data.fhirResources?.map(r => r.resourceType) || [];
        return data.warnings.filter(warning => 
            createdResources.some(type => warning.includes(type))
        );
    }

    getValidationResults() {
        const errors = this.getFilteredErrors();
        const warnings = this.getFilteredWarnings();
        
        let html = '';
        
        if (errors.length === 0 && warnings.length === 0) {
            html = '<div class="validation-success">✅ All validations passed!</div>';
        } else {
            errors.forEach(error => {
                html += `<div class="validation-error">❌ ${error}</div>`;
            });
            warnings.forEach(warning => {
                html += `<div class="validation-warning">⚠️ ${warning}</div>`;
            });
        }
        
        return { html, errors, warnings };
    }
}
