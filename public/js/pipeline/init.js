/**
 * Pipeline Builder Initialization
 * Starts the application when DOM is ready
 */

(function() {
    'use strict';

    let pipelineBuilder;

    /**
     * Initialize application
     */
    async function init() {
        console.log('Initializing Pipeline Builder...');

        try {
            // Check authentication using OOB AuthService
            const isAuthenticated = await window.AuthService.initialize();

            if (!isAuthenticated) {
                console.warn('🚫 Authentication failed, initialization aborted');
                return;
            }

            // Create pipeline builder instance
            pipelineBuilder = new PipelineBuilder();

            // Make available globally for debugging
            window.pipelineBuilder = pipelineBuilder;

            console.log('Pipeline Builder ready');

        } catch (error) {
            console.error('Failed to initialize Pipeline Builder:', error);
            showFatalError(error);
        }
    }

    /**
     * Show fatal error
     */
    function showFatalError(error) {
        const body = document.body;
        if (!body) return;

        const errorDiv = document.createElement('div');
        errorDiv.style.cssText = `
            position: fixed;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: white;
            padding: 2rem;
            border-radius: 0.5rem;
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
            max-width: 500px;
            text-align: center;
            z-index: 10000;
        `;

        errorDiv.innerHTML = `
            <i class="fas fa-exclamation-triangle" style="font-size: 3rem; color: #ef4444; margin-bottom: 1rem;"></i>
            <h2 style="margin-bottom: 1rem; color: #0f172a;">Failed to Load Pipeline Builder</h2>
            <p style="color: #64748b; margin-bottom: 1rem;">${error.message}</p>
            <button onclick="location.reload()" class="btn btn-primary">
                <i class="fas fa-redo"></i> Reload Page
            </button>
        `;

        body.appendChild(errorDiv);
    }

    /**
     * Check dependencies
     */
    function checkDependencies() {
        const required = [
            'VisualPipeline',
            // VisualLayer removed - layer system eliminated
            'VisualExecutionGroup',
            'VisualStep',
            'StepTemplate',
            'PipelineAPIService',
            'DragDropManager',
            'CanvasRenderer',
            'StepNodeManager',
            'ToolboxManager',
            'PropertiesPanel',
            'LayerContainer',
            'PipelineBuilder'
        ];

        const missing = required.filter(dep => typeof window[dep] === 'undefined');

        if (missing.length > 0) {
            throw new Error(`Missing dependencies: ${missing.join(', ')}`);
        }

        console.log('All dependencies loaded');
    }

    /**
     * Wait for DOM ready
     */
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            checkDependencies();
            init();
        });
    } else {
        checkDependencies();
        init();
    }

})();
