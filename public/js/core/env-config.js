// js/core/env-config.js - Environment Configuration Handler
// Handles API URLs and environment variables properly

(function() {
    'use strict';
    
    /**
     * Environment Configuration Manager
     * Handles API URLs, ports, and environment variables
     */
    class EnvironmentConfig {
        constructor() {
            this.config = {
                api: {
                    baseUrl: null,
                    port: null,
                    timeout: 30000
                },
                frontend: {
                    port: null,
                    host: null
                },
                features: {
                    debug: false,
                    autoDetectPorts: true
                }
            };
            
            this.init();
        }
        
        init() {
            this.loadEnvironmentVariables();
            this.detectConfiguration();
            this.validateConfiguration();
            
            // Make config globally available
            window.ENV_CONFIG = this.config;
            
            console.log('🔧 Environment configuration loaded:', this.config);
        }
        
        /**
         * Load environment variables from multiple sources
         */
        loadEnvironmentVariables() {
            // 1. Try window.ENV (if exposed from server)
            if (typeof window !== 'undefined' && window.ENV) {
                this.mergeConfig(window.ENV);
            }
            
            // 2. Try process.env (if available in build)
            if (typeof process !== 'undefined' && process.env) {
                this.mergeFromProcessEnv(process.env);
            }
            
            // 3. Try meta tags (if injected in HTML)
            this.loadFromMetaTags();
            
            // 4. Try data attributes (if set on body/html)
            this.loadFromDataAttributes();
        }
        
        /**
         * Merge configuration from window.ENV
         */
        mergeConfig(envObj) {
            const apiVars = [
                'API_BASE_URL', 'API_URL', 'BACKEND_URL',
                'REACT_APP_API_URL', 'VUE_APP_API_URL', 'VITE_API_URL'
            ];
            
            const portVars = [
                'API_PORT', 'BACKEND_PORT', 'SERVER_PORT'
            ];
            
            // Check for API URL
            for (const varName of apiVars) {
                if (envObj[varName]) {
                    this.config.api.baseUrl = envObj[varName];
                    console.log(`🔧 Using API URL from ${varName}: ${envObj[varName]}`);
                    break;
                }
            }
            
            // Check for API port
            for (const varName of portVars) {
                if (envObj[varName]) {
                    this.config.api.port = envObj[varName];
                    console.log(`🔧 Using API port from ${varName}: ${envObj[varName]}`);
                    break;
                }
            }
            
            // Debug mode
            if (envObj.NODE_ENV === 'development' || envObj.DEBUG === 'true') {
                this.config.features.debug = true;
            }
        }
        
        /**
         * Load from process.env (build-time variables)
         */
        mergeFromProcessEnv(processEnv) {
            const mapping = {
                'REACT_APP_API_URL': 'api.baseUrl',
                'VUE_APP_API_URL': 'api.baseUrl', 
                'VITE_API_URL': 'api.baseUrl',
                'API_BASE_URL': 'api.baseUrl',
                'API_PORT': 'api.port',
                'PORT': 'frontend.port',
                'NODE_ENV': 'environment'
            };
            
            Object.entries(mapping).forEach(([envVar, configPath]) => {
                if (processEnv[envVar]) {
                    this.setNestedConfig(configPath, processEnv[envVar]);
                    console.log(`🔧 Using ${envVar} from process.env: ${processEnv[envVar]}`);
                }
            });
        }
        
        /**
         * Load configuration from HTML meta tags
         * Usage: <meta name="api-base-url" content="http://localhost:8080">
         */
        loadFromMetaTags() {
            const metaMapping = {
                'api-base-url': 'api.baseUrl',
                'api-port': 'api.port',
                'frontend-port': 'frontend.port',
                'debug-mode': 'features.debug'
            };
            
            Object.entries(metaMapping).forEach(([metaName, configPath]) => {
                const metaTag = document.querySelector(`meta[name="${metaName}"]`);
                if (metaTag && metaTag.content) {
                    this.setNestedConfig(configPath, metaTag.content);
                    console.log(`🔧 Using ${metaName} from meta tag: ${metaTag.content}`);
                }
            });
        }
        
        /**
         * Load configuration from data attributes
         * Usage: <body data-api-url="http://localhost:8080">
         */
        loadFromDataAttributes() {
            const dataMapping = {
                'apiUrl': 'api.baseUrl',
                'apiPort': 'api.port',
                'frontendPort': 'frontend.port',
                'debugMode': 'features.debug'
            };
            
            const bodyEl = document.body || document.documentElement;
            
            Object.entries(dataMapping).forEach(([dataAttr, configPath]) => {
                const value = bodyEl.dataset[dataAttr];
                if (value) {
                    this.setNestedConfig(configPath, value);
                    console.log(`🔧 Using data-${dataAttr}: ${value}`);
                }
            });
        }
        
        /**
         * Auto-detect configuration from current page
         */
        detectConfiguration() {
            // Get current page info
            const currentHost = window.location.hostname;
            const currentPort = window.location.port;
            const currentProtocol = window.location.protocol;
            
            this.config.frontend.host = currentHost;
            this.config.frontend.port = currentPort || (currentProtocol === 'https:' ? '443' : '80');
            
            // Auto-detect API URL if not set
            if (!this.config.api.baseUrl && this.config.features.autoDetectPorts) {
                const apiPort = this.config.api.port || this.detectApiPort();
                this.config.api.baseUrl = `${currentProtocol}//${currentHost}:${apiPort}`;
                console.log(`🔧 Auto-detected API URL: ${this.config.api.baseUrl}`);
            }
        }
        
        /**
         * Detect likely API port based on common patterns
         */
        detectApiPort() {
            const currentPort = parseInt(this.config.frontend.port);
            
            // Common development patterns
            const portMappings = {
                3000: 8080, // React dev -> Go API
                3001: 8081, // Secondary frontend -> API
                5173: 8080, // Vite -> Go API 
                4200: 8080, // Angular -> Go API
                8000: 8080, // Django frontend -> Go API
            };
            
            if (portMappings[currentPort]) {
                console.log(`🔧 Detected API port based on frontend port ${currentPort}: ${portMappings[currentPort]}`);
                return portMappings[currentPort];
            }
            
            // Default fallback
            return '8080';
        }
        
        /**
         * Validate configuration and warn about issues
         */
        validateConfiguration() {
            // Check for required configuration
            if (!this.config.api.baseUrl) {
                console.warn('⚠️ No API base URL configured. Using auto-detected value.');
            }
            
            // Validate URL format
            if (this.config.api.baseUrl) {
                try {
                    new URL(this.config.api.baseUrl);
                } catch (error) {
                    console.error('❌ Invalid API base URL format:', this.config.api.baseUrl);
                    this.config.api.baseUrl = this.getDefaultApiUrl();
                }
            }
            
            // Check for localhost in production
            if (this.config.api.baseUrl && this.config.api.baseUrl.includes('localhost')) {
                if (window.location.hostname !== 'localhost') {
                    console.warn('⚠️ Using localhost API URL on non-localhost domain. This may cause issues.');
                }
            }
        }
        
        /**
         * Get default API URL as fallback
         */
        getDefaultApiUrl() {
            const protocol = window.location.protocol;
            const hostname = window.location.hostname;
            const defaultPort = '8080';
            
            return `${protocol}//${hostname}:${defaultPort}`;
        }
        
        /**
         * Helper to set nested configuration
         */
        setNestedConfig(path, value) {
            const keys = path.split('.');
            let current = this.config;
            
            for (let i = 0; i < keys.length - 1; i++) {
                if (!current[keys[i]]) {
                    current[keys[i]] = {};
                }
                current = current[keys[i]];
            }
            
            current[keys[keys.length - 1]] = value;
        }
        
        /**
         * Public API methods
         */
        getApiBaseUrl() {
            return this.config.api.baseUrl;
        }
        
        getApiUrl(endpoint = '') {
            const baseUrl = this.getApiBaseUrl();
            if (!baseUrl) {
                console.error('❌ No API base URL configured');
                return '';
            }
            
            // Remove leading slash from endpoint to avoid double slashes
            const cleanEndpoint = endpoint.startsWith('/') ? endpoint.slice(1) : endpoint;
            
            // Ensure base URL doesn't end with slash
            const cleanBaseUrl = baseUrl.endsWith('/') ? baseUrl.slice(0, -1) : baseUrl;
            
            return cleanEndpoint ? `${cleanBaseUrl}/${cleanEndpoint}` : cleanBaseUrl;
        }
        
        isDebugMode() {
            return this.config.features.debug;
        }
        
        getFrontendInfo() {
            return this.config.frontend;
        }
        
        /**
         * Create fetch wrapper with automatic URL resolution
         */
        createApiClient() {
            const self = this;
            
            return {
                async get(endpoint, options = {}) {
                    const url = self.getApiUrl(endpoint);
                    const config = {
                        method: 'GET',
                        headers: {
                            'Content-Type': 'application/json',
                            ...options.headers
                        },
                        ...options
                    };
                    
                    if (self.isDebugMode()) {
                        console.log(`🌐 API GET: ${url}`);
                    }
                    
                    return fetch(url, config);
                },
                
                async post(endpoint, data, options = {}) {
                    const url = self.getApiUrl(endpoint);
                    const config = {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            ...options.headers
                        },
                        body: data ? JSON.stringify(data) : undefined,
                        ...options
                    };
                    
                    if (self.isDebugMode()) {
                        console.log(`🌐 API POST: ${url}`, data);
                    }
                    
                    return fetch(url, config);
                },
                
                async put(endpoint, data, options = {}) {
                    const url = self.getApiUrl(endpoint);
                    const config = {
                        method: 'PUT',
                        headers: {
                            'Content-Type': 'application/json',
                            ...options.headers
                        },
                        body: data ? JSON.stringify(data) : undefined,
                        ...options
                    };
                    
                    if (self.isDebugMode()) {
                        console.log(`🌐 API PUT: ${url}`, data);
                    }
                    
                    return fetch(url, config);
                },
                
                async delete(endpoint, options = {}) {
                    const url = self.getApiUrl(endpoint);
                    const config = {
                        method: 'DELETE',
                        headers: {
                            'Content-Type': 'application/json',
                            ...options.headers
                        },
                        ...options
                    };
                    
                    if (self.isDebugMode()) {
                        console.log(`🌐 API DELETE: ${url}`);
                    }
                    
                    return fetch(url, config);
                }
            };
        }
    }
    
    // Initialize environment configuration
    const envConfig = new EnvironmentConfig();
    
    // Make globally available
    window.ENV_CONFIG = envConfig.config;
    window.getApiBaseUrl = () => envConfig.getApiBaseUrl();
    window.getApiUrl = (endpoint) => envConfig.getApiUrl(endpoint);
    window.createApiClient = () => envConfig.createApiClient();
    
    console.log('✅ Environment configuration manager loaded');
    
})();