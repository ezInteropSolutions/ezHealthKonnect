// services/RoutingService.js
// Enterprise Routing Service - Intelligent destination determination and load balancing
// Handles routing rules, failover, load balancing, and destination health monitoring
// ENHANCED with Smart Routing Engine

const SmartRoutingEngine = require('./SmartRoutingEngine');

class RoutingService {
    constructor() {
        this.routingRules = new Map();
        this.destinationHealth = new Map();
        this.loadBalancers = new Map();

        // Initialize Smart Routing Engine
        this.smartRoutingEngine = new SmartRoutingEngine();

        // Performance metrics
        this.metrics = {
            routingDecisions: 0,
            routingFailures: 0,
            destinationFailovers: 0,
            avgRoutingTime: 0,
            smartRoutingDecisions: 0,
            legacyRoutingDecisions: 0,
            startTime: new Date()
        };

        // Health check configuration
        this.healthCheckInterval = 30000; // 30 seconds
        this.healthCheckTimeout = 5000; // 5 seconds

        this.initializeBuiltInRoutes();
        this.startHealthMonitoring();

        console.log('✅ Enhanced Routing Service initialized with Smart Routing Engine');
    }

    /**
     * Initialize built-in routing patterns
     */
    initializeBuiltInRoutes() {
        // Route HL7 ADT messages to FHIR Patient endpoints
        this.addRoutingRule('HL7_ADT_to_FHIR', {
            sourcePattern: {
                messageType: 'ADT^A01',
                sourceType: 'tcp_mllp'
            },
            routingLogic: 'transformation_required',
            transformationConfig: {
                targetType: 'FHIR',
                targetResourceType: 'Patient'
            },
            destinationSelectionStrategy: 'primary_with_failover',
            loadBalancing: {
                strategy: 'round_robin',
                healthCheckRequired: true
            }
        });

        // Route FHIR resources to external systems
        this.addRoutingRule('FHIR_to_External', {
            sourcePattern: {
                messageType: 'FHIR',
                sourceType: 'http_endpoint'
            },
            routingLogic: 'direct_forward',
            destinationSelectionStrategy: 'load_balanced',
            loadBalancing: {
                strategy: 'least_connections',
                healthCheckRequired: true
            }
        });

        console.log(`🔀 Initialized ${this.routingRules.size} built-in routing rules`);
    }

    /**
     * Determine routing destination for message
     */
    async routeMessage(message, interfaceConfig) {
        const startTime = Date.now();

        try {
            console.log(`🔀 Determining route for message ${message.message_id}...`);

            // Check if interface has smart routing configuration
            const targetConfig = interfaceConfig.targetConfig || interfaceConfig.target_config || {};
            const parsedConfig = typeof targetConfig === 'string' ? JSON.parse(targetConfig) : targetConfig;

            const hasSmartRouting = parsedConfig.routing_strategy &&
                                  parsedConfig.routing_strategy !== 'single_endpoint' &&
                                  parsedConfig.endpoints &&
                                  parsedConfig.endpoints.length > 0;

            if (hasSmartRouting) {
                console.log(`🧠 Using Smart Routing Engine for message ${message.message_id}`);

                // Use Smart Routing Engine for multi-endpoint routing
                const smartResult = await this.smartRoutingEngine.routeMessage(
                    message.raw_message,
                    interfaceConfig,
                    message.message_id
                );

                if (smartResult.success) {
                    this.metrics.smartRoutingDecisions++;
                    this.updateMetrics(true, Date.now() - startTime);

                    console.log(`✅ Smart routing completed for ${message.message_id}: ${smartResult.routingDecision.selectedEndpoints.length} endpoints selected`);

                    return {
                        success: true,
                        routingDecision: smartResult.routingDecision
                    };
                } else {
                    console.log(`⚠️ Smart routing failed, falling back to legacy routing`);
                    // Fall through to legacy routing
                }
            }

            // Legacy routing for single endpoint or fallback
            console.log(`🔀 Using legacy routing for message ${message.message_id}`);
            this.metrics.legacyRoutingDecisions++;

            // Extract routing criteria from message
            const routingCriteria = this.extractRoutingCriteria(message, interfaceConfig);

            // Find matching routing rule
            const routingRule = this.findMatchingRoutingRule(routingCriteria);

            if (!routingRule) {
                throw new Error(`No routing rule found for message type: ${routingCriteria.messageType}`);
            }

            // Determine destination(s) based on interface target configuration
            const destinations = await this.selectDestinations(interfaceConfig, routingRule);

            // Apply load balancing if multiple destinations
            const selectedDestination = await this.applyLoadBalancing(destinations, routingRule);

            // Create routing decision (legacy format)
            const routingDecision = {
                routingRuleId: routingRule.id,
                selectedDestination,
                selectedEndpoints: [selectedDestination], // For compatibility with smart routing
                alternativeDestinations: destinations.filter(d => d !== selectedDestination),
                routingStrategy: routingRule.destinationSelectionStrategy,
                transformationRequired: routingRule.routingLogic === 'transformation_required',
                transformationConfig: routingRule.transformationConfig,
                routingTime: Date.now() - startTime,
                routingType: 'legacy'
            };

            this.updateMetrics(true, Date.now() - startTime);

            console.log(`✅ Legacy route determined for ${message.message_id}: ${selectedDestination.type}://${selectedDestination.host}:${selectedDestination.port}`);

            return {
                success: true,
                routingDecision
            };

        } catch (error) {
            this.updateMetrics(false, Date.now() - startTime);
            console.error(`❌ Routing failed for message ${message.message_id}:`, error.message);

            return {
                success: false,
                error: error.message,
                routingTime: Date.now() - startTime
            };
        }
    }

    /**
     * Extract routing criteria from message and interface config
     */
    extractRoutingCriteria(message, interfaceConfig) {
        return {
            messageType: message.message_type || 'UNKNOWN',
            sourceType: message.source_type || 'unknown',
            interfaceId: message.interface_id,
            interfaceName: interfaceConfig.name,
            priority: message.priority || 5,
            messageSize: message.message_size || 0,
            sourceEndpoint: message.source_endpoint,
            correlationId: message.correlation_id
        };
    }

    /**
     * Find matching routing rule based on criteria
     */
    findMatchingRoutingRule(criteria) {
        for (const [ruleId, rule] of this.routingRules) {
            if (this.matchesPattern(criteria, rule.sourcePattern)) {
                return { ...rule, id: ruleId };
            }
        }

        // Return default rule if no specific match
        return {
            id: 'default',
            routingLogic: 'direct_forward',
            destinationSelectionStrategy: 'primary_only',
            loadBalancing: { strategy: 'none' }
        };
    }

    /**
     * Check if criteria matches routing pattern
     */
    matchesPattern(criteria, pattern) {
        for (const [key, value] of Object.entries(pattern)) {
            if (criteria[key] !== value) {
                // Support for wildcard matching
                if (typeof value === 'string' && value.includes('*')) {
                    const regex = new RegExp(value.replace(/\*/g, '.*'));
                    if (!regex.test(criteria[key])) {
                        return false;
                    }
                } else {
                    return false;
                }
            }
        }
        return true;
    }

    /**
     * Select destination(s) based on interface configuration
     */
    async selectDestinations(interfaceConfig, routingRule) {
        const destinations = [];

        // Parse target configuration
        const targetConfig = typeof interfaceConfig.target_config === 'string' ?
            JSON.parse(interfaceConfig.target_config) : interfaceConfig.target_config;

        if (!targetConfig) {
            throw new Error('No target configuration found in interface');
        }

        // Primary destination
        const primaryDestination = {
            id: targetConfig.targetInterfaceId || 'primary',
            type: this.determineDestinationType(targetConfig),
            host: targetConfig.host || 'localhost',
            port: targetConfig.port || 8080,
            path: targetConfig.path || '/',
            method: targetConfig.method || 'POST',
            contentType: targetConfig.contentType || 'application/json',
            headers: targetConfig.headers || {},
            isPrimary: true,
            priority: 1
        };

        destinations.push(primaryDestination);

        // Add failover destinations if configured
        if (targetConfig.failover && Array.isArray(targetConfig.failover)) {
            for (let i = 0; i < targetConfig.failover.length; i++) {
                const failoverConfig = targetConfig.failover[i];
                destinations.push({
                    id: `failover_${i}`,
                    type: this.determineDestinationType(failoverConfig),
                    host: failoverConfig.host,
                    port: failoverConfig.port,
                    path: failoverConfig.path || '/',
                    method: failoverConfig.method || 'POST',
                    contentType: failoverConfig.contentType || 'application/json',
                    headers: failoverConfig.headers || {},
                    isPrimary: false,
                    priority: i + 2
                });
            }
        }

        return destinations;
    }

    /**
     * Determine destination type from configuration
     */
    determineDestinationType(config) {
        if (config.protocol) return config.protocol;
        if (config.path && config.path.startsWith('/')) return 'http';
        if (config.host && config.port) return 'tcp';
        return 'http'; // default
    }

    /**
     * Apply load balancing strategy to select destination
     */
    async applyLoadBalancing(destinations, routingRule) {
        if (destinations.length === 1) {
            return destinations[0];
        }

        const strategy = routingRule.loadBalancing?.strategy || 'primary_only';

        switch (strategy) {
            case 'primary_only':
                return destinations.find(d => d.isPrimary) || destinations[0];

            case 'round_robin':
                return this.roundRobinSelection(destinations, routingRule.id);

            case 'least_connections':
                return this.leastConnectionsSelection(destinations);

            case 'health_based':
                return this.healthBasedSelection(destinations);

            case 'random':
                return destinations[Math.floor(Math.random() * destinations.length)];

            default:
                console.warn(`⚠️ Unknown load balancing strategy: ${strategy}`);
                return destinations[0];
        }
    }

    /**
     * Round robin destination selection
     */
    roundRobinSelection(destinations, ruleId) {
        if (!this.loadBalancers.has(ruleId)) {
            this.loadBalancers.set(ruleId, { currentIndex: 0, connectionCounts: new Map() });
        }

        const balancer = this.loadBalancers.get(ruleId);
        const healthyDestinations = destinations.filter(d => this.isDestinationHealthy(d));

        if (healthyDestinations.length === 0) {
            console.warn(`⚠️ No healthy destinations available, using primary`);
            return destinations.find(d => d.isPrimary) || destinations[0];
        }

        const selected = healthyDestinations[balancer.currentIndex % healthyDestinations.length];
        balancer.currentIndex++;

        return selected;
    }

    /**
     * Least connections destination selection
     */
    leastConnectionsSelection(destinations) {
        const healthyDestinations = destinations.filter(d => this.isDestinationHealthy(d));

        if (healthyDestinations.length === 0) {
            return destinations.find(d => d.isPrimary) || destinations[0];
        }

        // Find destination with least connections (simplified implementation)
        return healthyDestinations.reduce((least, current) => {
            const leastConnections = this.getConnectionCount(least);
            const currentConnections = this.getConnectionCount(current);
            return currentConnections < leastConnections ? current : least;
        });
    }

    /**
     * Health-based destination selection
     */
    healthBasedSelection(destinations) {
        const healthyDestinations = destinations.filter(d => this.isDestinationHealthy(d));

        if (healthyDestinations.length === 0) {
            console.warn(`⚠️ No healthy destinations available for health-based selection`);
            return destinations.find(d => d.isPrimary) || destinations[0];
        }

        // Select based on priority and health score
        return healthyDestinations.sort((a, b) => {
            const aHealth = this.getHealthScore(a);
            const bHealth = this.getHealthScore(b);
            return bHealth - aHealth; // Higher health score first
        })[0];
    }

    /**
     * Check if destination is healthy
     */
    isDestinationHealthy(destination) {
        const healthInfo = this.destinationHealth.get(destination.id);
        if (!healthInfo) return true; // Assume healthy if no health info

        return healthInfo.status === 'healthy' &&
               (Date.now() - healthInfo.lastCheck) < (this.healthCheckInterval * 2);
    }

    /**
     * Get connection count for destination
     */
    getConnectionCount(destination) {
        // Simplified implementation - would track actual connections in production
        return Math.floor(Math.random() * 10);
    }

    /**
     * Get health score for destination
     */
    getHealthScore(destination) {
        const healthInfo = this.destinationHealth.get(destination.id);
        if (!healthInfo) return 100; // Default score

        return healthInfo.score || 100;
    }

    /**
     * Start health monitoring for destinations
     */
    startHealthMonitoring() {
        console.log(`🔍 Starting destination health monitoring (interval: ${this.healthCheckInterval}ms)...`);

        setInterval(async () => {
            await this.performHealthChecks();
        }, this.healthCheckInterval);
    }

    /**
     * Perform health checks on all known destinations
     */
    async performHealthChecks() {
        // Get all unique destinations from active interfaces
        const database = require('../config/database');

        try {
            const interfaces = await database.sequelize.query(`
                SELECT id, name, target_config, status
                FROM interfaces
                WHERE status IN ('active', 'configured')
            `, {
                type: database.sequelize.QueryTypes.SELECT
            });

            for (const interfaceConfig of interfaces) {
                const destinations = await this.selectDestinations(interfaceConfig, { loadBalancing: {} });

                for (const destination of destinations) {
                    await this.checkDestinationHealth(destination);
                }
            }
        } catch (error) {
            console.error('❌ Error during health checks:', error.message);
        }
    }

    /**
     * Check health of specific destination
     */
    async checkDestinationHealth(destination) {
        const startTime = Date.now();

        try {
            let isHealthy = false;
            let responseTime = 0;

            if (destination.type === 'http') {
                const result = await this.httpHealthCheck(destination);
                isHealthy = result.healthy;
                responseTime = result.responseTime;
            } else if (destination.type === 'tcp') {
                const result = await this.tcpHealthCheck(destination);
                isHealthy = result.healthy;
                responseTime = result.responseTime;
            }

            // Update health status
            this.destinationHealth.set(destination.id, {
                status: isHealthy ? 'healthy' : 'unhealthy',
                responseTime,
                lastCheck: Date.now(),
                score: isHealthy ? Math.max(100 - responseTime, 50) : 0
            });

        } catch (error) {
            this.destinationHealth.set(destination.id, {
                status: 'unhealthy',
                responseTime: this.healthCheckTimeout,
                lastCheck: Date.now(),
                score: 0,
                error: error.message
            });
        }
    }

    /**
     * Perform HTTP health check
     */
    async httpHealthCheck(destination) {
        return new Promise((resolve) => {
            const http = require('http');
            const startTime = Date.now();

            const options = {
                hostname: destination.host,
                port: destination.port,
                path: '/health', // Health endpoint
                method: 'GET',
                timeout: this.healthCheckTimeout
            };

            const req = http.request(options, (res) => {
                const responseTime = Date.now() - startTime;
                resolve({
                    healthy: res.statusCode >= 200 && res.statusCode < 300,
                    responseTime
                });
            });

            req.on('error', () => {
                resolve({
                    healthy: false,
                    responseTime: Date.now() - startTime
                });
            });

            req.on('timeout', () => {
                req.destroy();
                resolve({
                    healthy: false,
                    responseTime: this.healthCheckTimeout
                });
            });

            req.end();
        });
    }

    /**
     * Perform TCP health check
     */
    async tcpHealthCheck(destination) {
        return new Promise((resolve) => {
            const net = require('net');
            const startTime = Date.now();

            const socket = new net.Socket();

            socket.setTimeout(this.healthCheckTimeout);

            socket.connect(destination.port, destination.host, () => {
                const responseTime = Date.now() - startTime;
                socket.destroy();
                resolve({
                    healthy: true,
                    responseTime
                });
            });

            socket.on('error', () => {
                resolve({
                    healthy: false,
                    responseTime: Date.now() - startTime
                });
            });

            socket.on('timeout', () => {
                socket.destroy();
                resolve({
                    healthy: false,
                    responseTime: this.healthCheckTimeout
                });
            });
        });
    }

    /**
     * Add new routing rule
     */
    addRoutingRule(ruleId, rule) {
        this.routingRules.set(ruleId, rule);
        console.log(`📝 Added routing rule: ${ruleId}`);
    }

    /**
     * Update performance metrics
     */
    updateMetrics(success, routingTime) {
        if (success) {
            this.metrics.routingDecisions++;

            // Update average routing time
            const totalDecisions = this.metrics.routingDecisions;
            this.metrics.avgRoutingTime =
                ((this.metrics.avgRoutingTime * (totalDecisions - 1)) + routingTime) / totalDecisions;
        } else {
            this.metrics.routingFailures++;
        }
    }

    /**
     * Get routing service metrics
     */
    getMetrics() {
        const uptime = Math.floor((new Date() - this.metrics.startTime) / 1000);
        const totalDecisions = this.metrics.routingDecisions + this.metrics.routingFailures;

        const healthyDestinations = Array.from(this.destinationHealth.values())
            .filter(health => health.status === 'healthy').length;
        const totalDestinations = this.destinationHealth.size;

        return {
            uptime: `${Math.floor(uptime / 60)}m ${uptime % 60}s`,
            routingDecisions: {
                successful: this.metrics.routingDecisions,
                failed: this.metrics.routingFailures,
                total: totalDecisions,
                successRate: totalDecisions > 0 ?
                    ((this.metrics.routingDecisions / totalDecisions) * 100).toFixed(2) + '%' : '0%'
            },
            performance: {
                avgRoutingTime: Math.round(this.metrics.avgRoutingTime),
                destinationFailovers: this.metrics.destinationFailovers
            },
            destinations: {
                healthy: healthyDestinations,
                total: totalDestinations,
                healthyPercentage: totalDestinations > 0 ?
                    ((healthyDestinations / totalDestinations) * 100).toFixed(1) + '%' : '0%'
            },
            routingRules: this.routingRules.size
        };
    }

    /**
     * Get destination health status
     */
    getDestinationHealth() {
        return Array.from(this.destinationHealth.entries()).map(([id, health]) => ({
            id,
            status: health.status,
            responseTime: health.responseTime,
            lastCheck: new Date(health.lastCheck).toISOString(),
            score: health.score,
            error: health.error
        }));
    }
}

module.exports = RoutingService;