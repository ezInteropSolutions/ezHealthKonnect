/**
 * Step 4 Integration - FHIR Resource Mapping API Client
 * File: public/js/modules/step4-integration.js
 * 
 * IMPORTANT: Save this file as public/js/modules/step4-integration.js
 */

console.log('🔄 Loading Step 4 FHIR Resource Mapping integration...');

class Step4API {
    constructor(baseURL = '/api/fhir/transform') {
        this.baseURL = baseURL;
        console.log('🚀 Step4API initialized with baseURL:', baseURL);
    }

    async request(method, endpoint, data = null) {
        const url = this.baseURL + endpoint;
        
        const options = {
            method: method.toUpperCase(),
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json',
            }
        };

        if (data && ['POST', 'PUT', 'PATCH'].includes(method.toUpperCase())) {
            options.body = JSON.stringify(data);
        }

        try {
            console.log(`📡 API Request: ${method} ${url}`);
            const response = await fetch(url, options);
            
            if (!response.ok) {
                const errorData = await response.json().catch(() => ({}));
                throw new Error(`API Error ${response.status}: ${errorData.error || response.statusText}`);
            }

            const result = await response.json();
            console.log('✅ API Response:', result);
            return result;
        } catch (error) {
            console.error('❌ Step 4 API Error:', error);
            throw error;
        }
    }

    // Test the API connection
    async testConnection() {
        try {
            return await this.request('GET', '/test');
        } catch (error) {
            console.warn('⚠️ Test endpoint not available, trying status endpoint');
            return await this.request('GET', '/status');
        }
    }

    // Get HL7 structure
    async getHL7Structure(messageType) {
        return await this.request('GET', `/structure/hl7/${encodeURIComponent(messageType)}`);
    }

    // Get FHIR structure  
    async getFHIRStructure(profile = 'base') {
        return await this.request('GET', `/structure/fhir/${profile}`);
    }

    // Get mapping suggestions
    async getMappingSuggestions(hl7Message, messageType, profile = 'base') {
        const data = { hl7Message, messageType, profile };
        return await this.request('POST', '/mapping/suggestions', data);
    }

    // Create configuration
    async createConfiguration(configData) {
        return await this.request('POST', '/mapping/configurations', configData);
    }

    // Save mappings
    async saveMappings(configId, mappings) {
        const data = { configId: parseInt(configId), mappings: mappings };
        return await this.request('POST', `/mapping/configurations/${configId}/mappings`, data);
    }

    // Preview transformation
    async previewTransformation(hl7Message, mappings, options = {}) {
        const data = { hl7Message, mappings, options };
        return await this.request('POST', '/mapping/preview', data);
    }

    // Get configurations
    async listConfigurations(filters = {}) {
        const queryParams = new URLSearchParams(filters).toString();
        const endpoint = '/mapping/configurations' + (queryParams ? `?${queryParams}` : '');
        return await this.request('GET', endpoint);
    }
}

// Global API client
window.step4API = new Step4API();

// Test function for debugging
window.testStep4API = async function() {
    try {
        console.log('🧪 Testing Step 4 API connection...');
        const result = await window.step4API.testConnection();
        console.log('✅ Step 4 API test successful:', result);
        return result;
    } catch (error) {
        console.error('❌ Step 4 API test failed:', error);
        return { error: error.message };
    }
};

// Test HL7 structure lookup
window.testHL7Structure = async function(messageType = 'ADT^A01') {
    try {
        console.log(`🧪 Testing HL7 structure lookup for: ${messageType}`);
        const result = await window.step4API.getHL7Structure(messageType);
        console.log('✅ HL7 structure test successful:', result);
        return result;
    } catch (error) {
        console.error('❌ HL7 structure test failed:', error);
        return { error: error.message };
    }
};

// Auto-test on load
document.addEventListener('DOMContentLoaded', function() {
    console.log('🎯 Step 4 FHIR Resource Mapping integration ready');
    
    // Auto-test the API connection
    setTimeout(() => {
        window.testStep4API().then(result => {
            if (result.success) {
                console.log('✅ Step 4 backend integration confirmed');
            } else {
                console.warn('⚠️ Step 4 backend integration issues detected');
            }
        });
    }, 1000);
});

console.log('✅ Step 4 API integration loaded successfully');