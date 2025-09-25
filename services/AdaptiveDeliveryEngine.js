// services/AdaptiveDeliveryEngine.js
// Adaptive Delivery Engine - Handles flexible data delivery strategies
// Determines HOW to send data to each endpoint (individual resources vs bundles)

const { v4: uuidv4 } = require('uuid');

class AdaptiveDeliveryEngine {
    constructor() {
        this.deliveryStrategies = new Map();
        this.bundleGenerators = new Map();
        this.resourceSequencers = new Map();

        this.initializeDeliveryStrategies();
        this.initializeBundleGenerators();
    }

    /**
     * Initialize delivery strategy handlers
     */
    initializeDeliveryStrategies() {
        this.deliveryStrategies.set('individual_resources', this.deliverIndividualResources.bind(this));
        this.deliveryStrategies.set('bundle', this.deliverAsBundle.bind(this));
        this.deliveryStrategies.set('both', this.deliverBoth.bind(this));
        this.deliveryStrategies.set('adaptive', this.deliverAdaptive.bind(this));
    }

    /**
     * Initialize bundle generation strategies
     */
    initializeBundleGenerators() {
        this.bundleGenerators.set('transaction', this.createTransactionBundle.bind(this));
        this.bundleGenerators.set('collection', this.createCollectionBundle.bind(this));
        this.bundleGenerators.set('message', this.createMessageBundle.bind(this));
        this.bundleGenerators.set('document', this.createDocumentBundle.bind(this));
    }

    /**
     * Main delivery orchestration method
     */
    async deliverToEndpoints(fhirResources, routingDecision, messageMetadata) {
        console.log(`🚀 Starting adaptive delivery for ${routingDecision.selectedEndpoints.length} endpoints`);

        const deliveryResults = [];

        for (const endpoint of routingDecision.selectedEndpoints) {
            try {
                console.log(`📤 Processing delivery to endpoint: ${endpoint.id} (${endpoint.delivery_mode})`);

                const deliveryPlan = this.createDeliveryPlan(endpoint, fhirResources, routingDecision, messageMetadata);
                const result = await this.executeDeliveryPlan(deliveryPlan);

                deliveryResults.push({
                    endpointId: endpoint.id,
                    success: true,
                    result: result,
                    deliveryPlan: deliveryPlan
                });

                console.log(`✅ Delivery completed to ${endpoint.id}: ${result.deliveries.length} deliveries`);

            } catch (error) {
                console.error(`❌ Delivery failed to ${endpoint.id}:`, error.message);

                deliveryResults.push({
                    endpointId: endpoint.id,
                    success: false,
                    error: error.message
                });
            }
        }

        return {
            success: deliveryResults.every(r => r.success),
            results: deliveryResults,
            summary: this.generateDeliverySummary(deliveryResults)
        };
    }

    /**
     * Create delivery plan for specific endpoint
     */
    createDeliveryPlan(endpoint, fhirResources, routingDecision, messageMetadata) {
        // Determine delivery mode (endpoint config or routing rule override)
        const routingOverride = this.getRoutingOverride(endpoint.id, routingDecision);
        const deliveryMode = routingOverride?.mode || endpoint.delivery_mode || 'individual_resources';

        // Get resource preferences
        const resourcePreferences = endpoint.resource_preferences || {};

        // Filter resources based on endpoint preferences
        const relevantResources = this.filterRelevantResources(fhirResources, resourcePreferences);

        const plan = {
            endpointId: endpoint.id,
            baseUrl: endpoint.url,
            deliveryMode: deliveryMode,
            resources: relevantResources,
            resourcePreferences: resourcePreferences,
            routingOverride: routingOverride,
            messageMetadata: messageMetadata,
            deliveries: []
        };

        // Generate specific delivery instructions
        this.planDeliveries(plan);

        return plan;
    }

    /**
     * Plan specific deliveries based on delivery mode
     */
    planDeliveries(plan) {
        const strategy = this.deliveryStrategies.get(plan.deliveryMode);

        if (!strategy) {
            throw new Error(`Unknown delivery mode: ${plan.deliveryMode}`);
        }

        strategy(plan);
    }

    /**
     * Delivery Strategy: Individual Resources
     */
    deliverIndividualResources(plan) {
        const sequence = plan.routingOverride?.sequence || 'parallel';

        Object.entries(plan.resources).forEach(([resourceType, resources]) => {
            const preference = plan.resourcePreferences[resourceType];

            if (!preference || preference.required === false) {
                return; // Skip non-required resources
            }

            resources.forEach((resource, index) => {
                plan.deliveries.push({
                    type: 'individual_resource',
                    method: 'POST',
                    url: `${plan.baseUrl}${preference.endpoint}`,
                    data: resource,
                    headers: {
                        'Content-Type': 'application/fhir+json',
                        'X-Message-ID': plan.messageMetadata.messageId,
                        'X-Resource-Type': resourceType,
                        'X-Resource-Index': index
                    },
                    sequence: sequence,
                    priority: preference.priority || 1
                });
            });
        });

        // Sort deliveries by sequence and priority
        this.sortDeliveries(plan.deliveries, sequence);
    }

    /**
     * Delivery Strategy: Bundle
     */
    deliverAsBundle(plan) {
        const bundleType = plan.routingOverride?.bundle_type || 'transaction';
        const includeProvenance = plan.routingOverride?.include_provenance || false;

        const bundleGenerator = this.bundleGenerators.get(bundleType);
        if (!bundleGenerator) {
            throw new Error(`Unknown bundle type: ${bundleType}`);
        }

        const bundle = bundleGenerator(plan.resources, plan.messageMetadata, {
            includeProvenance: includeProvenance
        });

        const bundlePreference = plan.resourcePreferences['Bundle'];
        if (!bundlePreference) {
            throw new Error('Bundle delivery mode selected but no Bundle endpoint configured');
        }

        plan.deliveries.push({
            type: 'bundle',
            method: 'POST',
            url: `${plan.baseUrl}${bundlePreference.endpoint}`,
            data: bundle,
            headers: {
                'Content-Type': 'application/fhir+json',
                'X-Message-ID': plan.messageMetadata.messageId,
                'X-Bundle-Type': bundleType,
                'X-Resource-Count': this.countResourcesInBundle(bundle)
            },
            priority: 1
        });
    }

    /**
     * Delivery Strategy: Both (Individual + Bundle)
     */
    deliverBoth(plan) {
        // Create individual resource deliveries
        this.deliverIndividualResources(plan);

        // Create bundle delivery
        const bundlePlan = { ...plan, deliveries: [] };
        this.deliverAsBundle(bundlePlan);

        // Merge deliveries
        plan.deliveries = plan.deliveries.concat(bundlePlan.deliveries);
    }

    /**
     * Delivery Strategy: Adaptive (Smart Decision)
     */
    deliverAdaptive(plan) {
        const resourceCount = Object.values(plan.resources).flat().length;
        const hasComplexRelationships = this.hasComplexRelationships(plan.resources);

        // Decision logic for adaptive delivery
        if (resourceCount > 5 || hasComplexRelationships) {
            console.log(`🧠 Adaptive delivery: Using bundle mode (${resourceCount} resources, complex: ${hasComplexRelationships})`);
            this.deliverAsBundle(plan);
        } else {
            console.log(`🧠 Adaptive delivery: Using individual resources mode (${resourceCount} resources)`);
            this.deliverIndividualResources(plan);
        }
    }

    /**
     * Execute delivery plan
     */
    async executeDeliveryPlan(plan) {
        const results = [];

        // Group deliveries by sequence requirements
        const sequencedGroups = this.groupDeliveriesBySequence(plan.deliveries);

        for (const group of sequencedGroups) {
            if (group.sequence === 'parallel') {
                // Execute parallel deliveries
                const promises = group.deliveries.map(delivery => this.executeDelivery(delivery));
                const groupResults = await Promise.allSettled(promises);
                results.push(...groupResults);
            } else {
                // Execute sequential deliveries
                for (const delivery of group.deliveries) {
                    const result = await this.executeDelivery(delivery);
                    results.push(result);
                }
            }
        }

        return {
            endpointId: plan.endpointId,
            deliveries: results,
            totalDeliveries: results.length,
            successfulDeliveries: results.filter(r => r.status === 'fulfilled').length
        };
    }

    /**
     * Execute individual delivery
     */
    async executeDelivery(delivery) {
        // This would integrate with the actual HTTP delivery service
        console.log(`📡 Executing ${delivery.type} delivery to ${delivery.url}`);

        // Mock implementation - replace with actual HTTP client
        return {
            type: delivery.type,
            url: delivery.url,
            status: 'fulfilled',
            timestamp: new Date().toISOString(),
            responseStatus: 201,
            resourceId: delivery.data.id || 'generated-id'
        };
    }

    /**
     * Create Transaction Bundle
     */
    createTransactionBundle(resources, messageMetadata, options = {}) {
        const bundle = {
            resourceType: 'Bundle',
            id: uuidv4(),
            type: 'transaction',
            timestamp: new Date().toISOString(),
            entry: []
        };

        Object.entries(resources).forEach(([resourceType, resourceList]) => {
            resourceList.forEach(resource => {
                bundle.entry.push({
                    resource: resource,
                    request: {
                        method: 'POST',
                        url: resourceType
                    }
                });
            });
        });

        if (options.includeProvenance) {
            bundle.entry.push(this.createProvenanceResource(messageMetadata));
        }

        return bundle;
    }

    /**
     * Create Collection Bundle
     */
    createCollectionBundle(resources, messageMetadata) {
        const bundle = {
            resourceType: 'Bundle',
            id: uuidv4(),
            type: 'collection',
            timestamp: new Date().toISOString(),
            entry: []
        };

        Object.values(resources).flat().forEach(resource => {
            bundle.entry.push({
                resource: resource
            });
        });

        return bundle;
    }

    /**
     * Helper methods
     */
    filterRelevantResources(fhirResources, resourcePreferences) {
        const filtered = {};

        Object.keys(resourcePreferences).forEach(resourceType => {
            if (fhirResources[resourceType]) {
                filtered[resourceType] = fhirResources[resourceType];
            }
        });

        return filtered;
    }

    getRoutingOverride(endpointId, routingDecision) {
        for (const rule of routingDecision.appliedRules || []) {
            if (rule.delivery_override && rule.delivery_override[endpointId]) {
                return rule.delivery_override[endpointId];
            }
        }
        return null;
    }

    hasComplexRelationships(resources) {
        // Check if resources have complex interdependencies
        return Object.keys(resources).length > 2;
    }

    sortDeliveries(deliveries, sequence) {
        if (sequence === 'patient_first') {
            deliveries.sort((a, b) => {
                if (a.headers['X-Resource-Type'] === 'Patient') return -1;
                if (b.headers['X-Resource-Type'] === 'Patient') return 1;
                return a.priority - b.priority;
            });
        } else {
            deliveries.sort((a, b) => a.priority - b.priority);
        }
    }

    groupDeliveriesBySequence(deliveries) {
        const groups = [];
        let currentGroup = { sequence: 'parallel', deliveries: [] };

        deliveries.forEach(delivery => {
            if (delivery.sequence !== currentGroup.sequence) {
                if (currentGroup.deliveries.length > 0) {
                    groups.push(currentGroup);
                }
                currentGroup = { sequence: delivery.sequence, deliveries: [delivery] };
            } else {
                currentGroup.deliveries.push(delivery);
            }
        });

        if (currentGroup.deliveries.length > 0) {
            groups.push(currentGroup);
        }

        return groups;
    }

    countResourcesInBundle(bundle) {
        return bundle.entry ? bundle.entry.length : 0;
    }

    createProvenanceResource(messageMetadata) {
        return {
            resource: {
                resourceType: 'Provenance',
                id: uuidv4(),
                target: [{ reference: `Message/${messageMetadata.messageId}` }],
                recorded: new Date().toISOString(),
                activity: {
                    coding: [{
                        system: 'http://terminology.hl7.org/CodeSystem/v3-DataOperation',
                        code: 'CREATE'
                    }]
                },
                agent: [{
                    type: {
                        coding: [{
                            system: 'http://terminology.hl7.org/CodeSystem/provenance-participant-type',
                            code: 'performer'
                        }]
                    },
                    who: {
                        display: 'ezHealthKonnect Integration Engine'
                    }
                }]
            },
            request: {
                method: 'POST',
                url: 'Provenance'
            }
        };
    }

    generateDeliverySummary(deliveryResults) {
        const successful = deliveryResults.filter(r => r.success).length;
        const total = deliveryResults.length;

        return {
            totalEndpoints: total,
            successfulEndpoints: successful,
            failedEndpoints: total - successful,
            successRate: total > 0 ? (successful / total * 100).toFixed(1) + '%' : '0%'
        };
    }
}

module.exports = AdaptiveDeliveryEngine;