/**
 * Pipeline API Service
 * Handles all communication with backend API
 */

class PipelineAPIService {
    constructor() {
        this.baseURL = '/api';
        this.timeout = 30000;
    }

    /**
     * Generic HTTP request handler
     */
    async request(method, endpoint, data = null, options = {}) {
        const url = `${this.baseURL}${endpoint}`;
        const config = {
            method: method.toUpperCase(),
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            credentials: 'include', // Important: Include cookies for session
            ...options
        };

        if (data && (method === 'POST' || method === 'PUT')) {
            config.body = JSON.stringify(data);
        }

        try {
            const response = await fetch(url, config);
            const contentType = response.headers.get('content-type');

            // Check for authentication errors
            if (response.status === 401 || response.status === 403) {
                console.warn('🚫 Session expired or unauthorized access');
                this.handleAuthenticationError();
                throw new Error('Session expired. Redirecting to login...');
            }

            let responseData;
            if (contentType && contentType.includes('application/json')) {
                responseData = await response.json();
            } else {
                responseData = await response.text();
            }

            if (!response.ok) {
                throw new Error(responseData.error || responseData || `HTTP ${response.status}`);
            }

            return responseData;
        } catch (error) {
            console.error(`API Error [${method} ${endpoint}]:`, error);
            throw error;
        }
    }

    /**
     * Handle authentication errors
     */
    handleAuthenticationError() {
        // Store current URL to redirect back after login
        const currentPath = window.location.pathname + window.location.search;
        sessionStorage.setItem('redirectAfterLogin', currentPath);

        // Redirect to login page
        setTimeout(() => {
            window.location.href = '/login.html';
        }, 500);
    }

    /**
     * Check if user session is valid
     */
    async checkSession() {
        try {
            const response = await fetch('/api/auth/session', {
                method: 'GET',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            if (response.status === 401 || response.status === 403) {
                return false;
            }

            if (!response.ok) {
                return false;
            }

            const data = await response.json();
            return data.authenticated === true;
        } catch (error) {
            console.error('Session check failed:', error);
            return false;
        }
    }

    /**
     * Save visual pipeline
     */
    async savePipeline(visualPipeline) {
        return await this.request('POST', '/pipelines', visualPipeline.toJSON());
    }

    /**
     * Load pipeline by ID
     */
    async loadPipeline(pipelineId) {
        const response = await this.request('GET', `/pipelines/${pipelineId}`);
        return VisualPipeline.fromJSON(response.data || response);
    }

    /**
     * Load pipeline by interface and message type
     */
    async loadPipelineByInterface(interfaceId, messageType) {
        const response = await this.request('GET', `/pipelines/interface/${interfaceId}/${messageType}`);
        console.log('📥 Pipeline API response:', response);

        if (response.data || response.pipeline) {
            const pipelineData = response.data || response.pipeline;
            console.log('📋 Pipeline data to load:', pipelineData);
            return VisualPipeline.fromJSON(pipelineData);
        }
        return null;
    }

    /**
     * Delete pipeline
     */
    async deletePipeline(pipelineId) {
        return await this.request('DELETE', `/pipelines/${pipelineId}`);
    }

    /**
     * Clone pipeline
     */
    async clonePipeline(pipelineId, newName) {
        return await this.request('POST', `/pipelines/${pipelineId}/clone`, { new_name: newName });
    }

    /**
     * Test pipeline with sample message
     */
    async testPipeline(visualPipeline, sampleMessage) {
        const pipelineData = visualPipeline.toJSON();
        return await this.request('POST', '/pipelines/test', {
            pipeline_id: pipelineData.id,
            pipeline: pipelineData,
            sample_message: sampleMessage
        });
    }

    /**
     * Execute pipeline
     */
    async executePipeline(pipelineId, messageId, inputData) {
        return await this.request('POST', '/pipelines/execute', {
            pipeline_id: pipelineId,
            message_id: messageId,
            input_data: inputData
        });
    }

    /**
     * Get pipeline statistics
     */
    async getPipelineStats(pipelineId) {
        return await this.request('GET', `/pipelines/${pipelineId}/stats`);
    }

    /**
     * List all templates
     */
    async listTemplates() {
        const response = await this.request('GET', '/templates');
        const templates = response.data || response.templates || [];
        return templates.map(t => StepTemplate.fromJSON(t));
    }

    /**
     * Get specific template
     */
    async getTemplate(templateId) {
        const response = await this.request('GET', `/templates/${templateId}`);
        return StepTemplate.fromJSON(response.data || response);
    }

    /**
     * Create custom template
     */
    async createTemplate(template) {
        return await this.request('POST', '/templates', template);
    }
    /**
     * Create template from step
     */
    async createTemplateFromStep(stepId, templateName, description, isPublic = true) {
        return await this.request('POST', '/templates/from-step', {
            step_id: stepId,
            template_name: templateName,
            description: description,
            is_public: isPublic
        });
    }

    /**
     * Apply template to pipeline
     */
    async applyTemplate(templateId, pipelineId, sequence, stepName = null) {
        return await this.request('POST', `/templates/${templateId}/apply`, {
            pipeline_id: pipelineId,
            sequence: sequence,
            step_name: stepName
        });
    }


    /**
     * Update template
     */
    async updateTemplate(templateId, template) {
        return await this.request('PUT', `/templates/${templateId}`, template);
    }

    /**
     * Delete template
     */
    async deleteTemplate(templateId) {
        return await this.request('DELETE', `/templates/${templateId}`);
    }

    /**
     * Validate pipeline configuration
     */
    async validatePipeline(visualPipeline) {
        return await this.request('POST', '/pipelines/validate', visualPipeline.toJSON());
    }

    /**
     * Get execution history
     */
    async getExecutionHistory(pipelineId, limit = 50) {
        return await this.request('GET', `/pipelines/${pipelineId}/executions?limit=${limit}`);
    }

    /**
     * Get step execution details
     */
    async getStepExecution(executionId, stepId) {
        return await this.request('GET', `/executions/${executionId}/steps/${stepId}`);
    }

    /**
     * Test individual step
     */
    async testStep(step, inputData) {
        return await this.request('POST', '/processing/execute/validation', {
            step: step.toJSON(),
            input_data: inputData
        });
    }

    /**
     * Get available executors
     */
    async getExecutors() {
        return await this.request('GET', '/executors');
    }

    /**
     * Validate step configuration
     */
    async validateStep(step) {
        return await this.request('POST', '/steps/validate', step.toJSON());
    }
}

// Export singleton instance
if (typeof window !== 'undefined') {
    window.PipelineAPIService = PipelineAPIService;
    window.pipelineAPI = new PipelineAPIService();
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = PipelineAPIService;
}
