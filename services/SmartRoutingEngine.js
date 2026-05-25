// services/SmartRoutingEngine.js
// Intelligent Routing Decision Engine for Multi-Endpoint Message Routing

const HL7MessageAnalyzer = require('./HL7MessageAnalyzer');

class SmartRoutingEngine {
    constructor() {
        this.messageAnalyzer = new HL7MessageAnalyzer();
        this.routingStrategies = new Map();
        this.endpointHealthCache = new Map();
        this.routingHistory = [];

        // Initialize routing strategies
        this.initializeRoutingStrategies();
    }

    /**
     * Initialize supported routing strategies
     */
    initializeRoutingStrategies() {
        this.routingStrategies.set('single_endpoint', this.singleEndpointRouting.bind(this));
        this.routingStrategies.set('content_based', this.contentBasedRouting.bind(this));
        this.routingStrategies.set('load_balanced', this.loadBalancedRouting.bind(this));
        this.routingStrategies.set('broadcast', this.broadcastRouting.bind(this));
        this.routingStrategies.set('failover', this.failoverRouting.bind(this));
        this.routingStrategies.set('smart_multi_cast', this.smartMultiCastRouting.bind(this));
    }

    /**
     * Main routing decision method
     */
    async routeMessage(hl7Message, interfaceConfig, messageId = null) {
        console.log(`🧠 Starting smart routing for message: ${messageId || 'unknown'}`);

        try {
            // Step 1: Analyze the HL7 message
            const messageAnalysis = this.messageAnalyzer.analyzeMessage(hl7Message, messageId);
            console.log(`📊 Message analysis completed:`, {
                type: `${messageAnalysis.messageType}^${messageAnalysis.triggerEvent}`,
                facility: messageAnalysis.facility,
                urgency: messageAnalysis.urgency,
                patientClass: messageAnalysis.patientClass
            });

            // Step 2: Extract routing configuration
            const routingConfig = this.extractRoutingConfig(interfaceConfig);
            console.log(`⚙️ Routing strategy: ${routingConfig.strategy}`);

            // Step 3: Apply routing strategy
            const routingStrategy = this.routingStrategies.get(routingConfig.strategy);
            if (!routingStrategy) {
                throw new Error(`Unknown routing strategy: ${routingConfig.strategy}`);
            }

            const routingDecision = await routingStrategy(messageAnalysis, routingConfig);

            // Step 4: Filter healthy endpoints
            const healthyEndpoints = await this.filterHealthyEndpoints(routingDecision.selectedEndpoints);

            // Step 5: Record routing decision
            const finalDecision = {
                messageId: messageAnalysis.messageId,
                messageAnalysis,
                routingConfig,
                selectedEndpoints: healthyEndpoints,
                transformationRequired: routingDecision.transformationRequired,
                transformationConfig: routingDecision.transformationConfig,
                routingReason: routingDecision.routingReason,
                fallbackEndpoints: routingDecision.fallbackEndpoints || [],
                deliveryOptions: routingDecision.deliveryOptions || {},
                timestamp: new Date().toISOString()
            };

            this.recordRoutingHistory(finalDecision);

            console.log(`✅ Routing decision completed: ${healthyEndpoints.length} endpoints selected`);
            return { success: true, routingDecision: finalDecision };

        } catch (error) {
            console.error(`❌ Routing error:`, error.message);
            return {
                success: false,
                error: error.message,
                fallbackDecision: await this.generateFallbackRouting(interfaceConfig)
            };
        }
    }

    /**
     * Extract routing configuration from interface config
     */
    extractRoutingConfig(interfaceConfig) {
        const targetConfig = interfaceConfig.targetConfig || interfaceConfig.target_config || {};
        const parsedConfig = typeof targetConfig === 'string' ? JSON.parse(targetConfig) : targetConfig;

        return {
            strategy: parsedConfig.routing_strategy || 'single_endpoint',
            endpoints: parsedConfig.endpoints || this.createDefaultEndpoints(parsedConfig),
            routingRules: parsedConfig.routing_rules || [],
            loadBalancing: parsedConfig.load_balancing || 'round_robin',
            circuitBreaker: parsedConfig.circuit_breaker || { failure_threshold: 5 },
            retryPolicy: parsedConfig.retry_policy || { max_attempts: 3 }
        };
    }

    /**
     * Create default endpoints from legacy configuration
     */
    createDefaultEndpoints(config) {
        if (!config.host && !config.url) {
            return [];
        }

        return [{
            id: 'default_endpoint',
            name: 'Default Endpoint',
            type: 'fhir_r4',
            url: config.url || `${config.protocol || 'http'}://${config.host}:${config.port}/fhir`,
            resource_endpoint: config.path || 'Patient',
            priority: 1,
            weight: 100,
            enabled: true
        }];
    }

    /**
     * Single endpoint routing (simplest strategy)
     */
    async singleEndpointRouting(messageAnalysis, routingConfig) {
        const endpoints = routingConfig.endpoints.filter(ep => ep.enabled);

        if (endpoints.length === 0) {
            throw new Error('No enabled endpoints configured');
        }

        // Select highest priority endpoint
        const selectedEndpoint = endpoints.sort((a, b) => (a.priority || 1) - (b.priority || 1))[0];

        return {
            selectedEndpoints: [selectedEndpoint],
            transformationRequired: this.requiresTransformation(messageAnalysis, selectedEndpoint),
            transformationConfig: this.getTransformationConfig(messageAnalysis, selectedEndpoint),
            routingReason: 'Single endpoint strategy - selected highest priority endpoint'
        };
    }

    /**
     * Content-based routing using HL7 message content
     */
    async contentBasedRouting(messageAnalysis, routingConfig) {
        const applicableRules = this.findApplicableRules(messageAnalysis, routingConfig.routingRules);

        if (applicableRules.length === 0) {
            // Fall back to single endpoint
            return await this.singleEndpointRouting(messageAnalysis, routingConfig);
        }

        // Sort rules by priority
        const sortedRules = applicableRules.sort((a, b) => (a.priority || 999) - (b.priority || 999));
        const selectedRule = sortedRules[0];

        // Get endpoints for the selected rule
        const ruleEndpoints = routingConfig.endpoints.filter(ep =>
            selectedRule.endpoints.includes(ep.id) && ep.enabled
        );

        if (ruleEndpoints.length === 0) {
            throw new Error(`No valid endpoints found for rule: ${selectedRule.name}`);
        }

        return {
            selectedEndpoints: ruleEndpoints,
            transformationRequired: selectedRule.transformation || this.requiresTransformation(messageAnalysis, ruleEndpoints[0]),
            transformationConfig: selectedRule.transformation_config || this.getTransformationConfig(messageAnalysis, ruleEndpoints[0]),
            routingReason: `Content-based routing - matched rule: ${selectedRule.name} (${selectedRule.condition.field} ${selectedRule.condition.operator} ${selectedRule.condition.value})`
        };
    }

    /**
     * Load balanced routing across multiple endpoints
     */
    async loadBalancedRouting(messageAnalysis, routingConfig) {
        const enabledEndpoints = routingConfig.endpoints.filter(ep => ep.enabled);

        if (enabledEndpoints.length === 0) {
            throw new Error('No enabled endpoints for load balancing');
        }

        let selectedEndpoint;

        switch (routingConfig.loadBalancing) {
            case 'round_robin':
                selectedEndpoint = this.selectRoundRobin(enabledEndpoints, messageAnalysis.messageId);
                break;
            case 'weighted_round_robin':
                selectedEndpoint = this.selectWeightedRoundRobin(enabledEndpoints);
                break;
            case 'least_connections':
                selectedEndpoint = await this.selectLeastConnections(enabledEndpoints);
                break;
            default:
                selectedEndpoint = enabledEndpoints[0];
        }

        return {
            selectedEndpoints: [selectedEndpoint],
            transformationRequired: this.requiresTransformation(messageAnalysis, selectedEndpoint),
            transformationConfig: this.getTransformationConfig(messageAnalysis, selectedEndpoint),
            routingReason: `Load balanced routing using ${routingConfig.loadBalancing} strategy`
        };
    }

    /**
     * Broadcast to all endpoints
     */
    async broadcastRouting(messageAnalysis, routingConfig) {
        const enabledEndpoints = routingConfig.endpoints.filter(ep => ep.enabled);

        if (enabledEndpoints.length === 0) {
            throw new Error('No enabled endpoints for broadcast');
        }

        return {
            selectedEndpoints: enabledEndpoints,
            transformationRequired: enabledEndpoints.some(ep => this.requiresTransformation(messageAnalysis, ep)),
            transformationConfig: this.getTransformationConfig(messageAnalysis, enabledEndpoints[0]),
            routingReason: `Broadcast routing - sending to all ${enabledEndpoints.length} enabled endpoints`,
            deliveryOptions: {
                parallel: true,
                timeout: 30000
            }
        };
    }

    /**
     * Failover routing with primary and backup endpoints
     */
    async failoverRouting(messageAnalysis, routingConfig) {
        const endpoints = routingConfig.endpoints
            .filter(ep => ep.enabled)
            .sort((a, b) => (a.priority || 999) - (b.priority || 999));

        if (endpoints.length === 0) {
            throw new Error('No enabled endpoints for failover');
        }

        const primaryEndpoint = endpoints[0];
        const fallbackEndpoints = endpoints.slice(1);

        return {
            selectedEndpoints: [primaryEndpoint],
            fallbackEndpoints: fallbackEndpoints,
            transformationRequired: this.requiresTransformation(messageAnalysis, primaryEndpoint),
            transformationConfig: this.getTransformationConfig(messageAnalysis, primaryEndpoint),
            routingReason: `Failover routing - primary endpoint with ${fallbackEndpoints.length} fallback options`
        };
    }

    /**
     * Smart multi-cast routing combining content analysis and load balancing
     */
    async smartMultiCastRouting(messageAnalysis, routingConfig) {
        // First try content-based routing
        try {
            const contentResult = await this.contentBasedRouting(messageAnalysis, routingConfig);
            if (contentResult.selectedEndpoints.length > 0) {
                contentResult.routingReason = 'Smart multi-cast - content-based selection succeeded';
                return contentResult;
            }
        } catch (error) {
            console.log('Content-based routing failed, trying load balanced approach');
        }

        // Fall back to load balanced routing
        const loadBalanceResult = await this.loadBalancedRouting(messageAnalysis, routingConfig);
        loadBalanceResult.routingReason = 'Smart multi-cast - fell back to load balanced routing';
        return loadBalanceResult;
    }

    /**
     * Find routing rules that match the message analysis
     */
    findApplicableRules(messageAnalysis, routingRules) {
        return routingRules.filter(rule => {
            try {
                return this.evaluateRuleCondition(rule.condition, messageAnalysis);
            } catch (error) {
                console.warn(`Error evaluating rule ${rule.name}:`, error.message);
                return false;
            }
        });
    }

    /**
     * Evaluate a routing rule condition against message analysis
     */
    evaluateRuleCondition(condition, messageAnalysis) {
        const fieldValue = this.getFieldValue(condition.field, messageAnalysis);

        switch (condition.operator) {
            case 'equals':
                return fieldValue === condition.value;
            case 'not_equals':
                return fieldValue !== condition.value;
            case 'in':
                return condition.values && condition.values.includes(fieldValue);
            case 'not_in':
                return condition.values && !condition.values.includes(fieldValue);
            case 'starts_with':
                return fieldValue && fieldValue.startsWith(condition.value);
            case 'ends_with':
                return fieldValue && fieldValue.endsWith(condition.value);
            case 'contains':
                return fieldValue && fieldValue.includes(condition.value);
            case 'regex':
                return fieldValue && new RegExp(condition.value).test(fieldValue);
            case 'greater_than':
                return parseFloat(fieldValue) > parseFloat(condition.value);
            case 'less_than':
                return parseFloat(fieldValue) < parseFloat(condition.value);
            default:
                console.warn(`Unknown condition operator: ${condition.operator}`);
                return false;
        }
    }

    /**
     * Get field value from message analysis using dot notation
     */
    getFieldValue(fieldPath, messageAnalysis) {
        const parts = fieldPath.split('.');
        let value = messageAnalysis;

        for (const part of parts) {
            if (value && typeof value === 'object') {
                value = value[part];
            } else {
                return null;
            }
        }

        return value;
    }

    /**
     * Round robin endpoint selection
     */
    selectRoundRobin(endpoints, messageId) {
        const hash = this.hashString(messageId || Date.now().toString());
        const index = hash % endpoints.length;
        return endpoints[index];
    }

    /**
     * Weighted round robin endpoint selection
     */
    selectWeightedRoundRobin(endpoints) {
        const totalWeight = endpoints.reduce((sum, ep) => sum + (ep.weight || 1), 0);
        const random = Math.random() * totalWeight;

        let currentWeight = 0;
        for (const endpoint of endpoints) {
            currentWeight += (endpoint.weight || 1);
            if (random <= currentWeight) {
                return endpoint;
            }
        }

        return endpoints[0]; // Fallback
    }

    /**
     * Select endpoint with least connections (mock implementation)
     */
    async selectLeastConnections(endpoints) {
        // In a real implementation, this would check actual connection counts
        // For now, we'll randomly select one
        return endpoints[Math.floor(Math.random() * endpoints.length)];
    }

    /**
     * Filter out unhealthy endpoints
     */
    async filterHealthyEndpoints(endpoints) {
        // For now, return all endpoints (health checking would be implemented separately)
        return endpoints.filter(ep => ep.enabled !== false);
    }

    /**
     * Determine if transformation is required
     */
    requiresTransformation(messageAnalysis, endpoint) {
        if (endpoint.type && endpoint.type.includes('fhir')) {
            return messageAnalysis.messageType !== 'FHIR';
        }
        return false;
    }

    /**
     * Get transformation configuration
     */
    getTransformationConfig(messageAnalysis, endpoint) {
        const messageTypeEvent = `${messageAnalysis.messageType}^${messageAnalysis.triggerEvent}`;

        return {
            sourceFormat: 'HL7',
            targetFormat: 'FHIR',
            messageType: messageTypeEvent,
            targetResourceType: this.determineTargetResourceType(messageAnalysis, endpoint),
            templateName: `${messageTypeEvent}_to_FHIR_${this.determineTargetResourceType(messageAnalysis, endpoint)}`
        };
    }

    /**
     * Determine target FHIR resource type
     */
    determineTargetResourceType(messageAnalysis, endpoint) {
        if (endpoint.resource_endpoint) {
            return endpoint.resource_endpoint;
        }

        // Intelligent mapping based on message type
        switch (messageAnalysis.messageType) {
            case 'ADT':
                return 'Patient';
            case 'ORU':
                return 'DiagnosticReport';
            case 'ORM':
                return 'ServiceRequest';
            default:
                return 'Patient';
        }
    }

    /**
     * Generate fallback routing for error cases
     */
    async generateFallbackRouting(interfaceConfig) {
        return {
            selectedEndpoints: [],
            transformationRequired: false,
            routingReason: 'Fallback routing due to routing error',
            error: true
        };
    }

    /**
     * Record routing decision in history
     */
    recordRoutingHistory(decision) {
        this.routingHistory.push(decision);

        // Keep only last 1000 decisions
        if (this.routingHistory.length > 1000) {
            this.routingHistory = this.routingHistory.slice(-1000);
        }
    }

    /**
     * Simple string hash function
     */
    hashString(str) {
        let hash = 0;
        for (let i = 0; i < str.length; i++) {
            const char = str.charCodeAt(i);
            hash = ((hash << 5) - hash) + char;
            hash = hash & hash; // Convert to 32-bit integer
        }
        return Math.abs(hash);
    }

    /**
     * Get routing statistics
     */
    getRoutingStats() {
        const total = this.routingHistory.length;
        const strategies = {};
        const endpoints = {};

        this.routingHistory.forEach(decision => {
            const strategy = decision.routingConfig.strategy;
            strategies[strategy] = (strategies[strategy] || 0) + 1;

            decision.selectedEndpoints.forEach(ep => {
                endpoints[ep.id] = (endpoints[ep.id] || 0) + 1;
            });
        });

        return {
            totalDecisions: total,
            strategiesUsed: strategies,
            endpointUsage: endpoints,
            cacheStats: this.messageAnalyzer.getCacheStats()
        };
    }
}

module.exports = SmartRoutingEngine;