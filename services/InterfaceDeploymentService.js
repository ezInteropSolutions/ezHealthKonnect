// services/InterfaceDeploymentService.js
// Handles interface deployment lifecycle: auto-start, manual-start, delayed-start

const axios = require('axios');

class InterfaceDeploymentService {
    constructor(database) {
        this.database = database;
        this.goBackendUrl = process.env.GO_BACKEND_URL || 'http://localhost:8080';
        this.deploymentTimers = new Map(); // Track delayed deployment timers
        this.isInitialized = false;
    }

    /**
     * Initialize deployment service and auto-start interfaces on application startup
     */
    async initializeOnStartup() {
        if (this.isInitialized) {
            console.log('⚠️  Deployment service already initialized');
            return;
        }

        console.log('\n🚀 ========================================');
        console.log('🚀 Interface Deployment Service Starting...');
        console.log('🚀 ========================================\n');

        try {
            // Wait for database connection
            if (!this.database.isConnected) {
                console.log('⏳ Waiting for database connection...');
                await this.database.connect();
            }

            // Wait a few seconds for Go backend to be ready
            console.log('⏳ Waiting for Go backend to be ready...');
            await this.waitForGoBackend(30); // 30 second timeout

            // Get all interfaces that should auto-start
            const interfaces = await this.database.sequelize.query(`
                SELECT
                    id,
                    name,
                    deployment_mode,
                    auto_start,
                    deployment_delay_seconds,
                    source_type,
                    target_type,
                    status
                FROM interfaces
                WHERE is_active = true
                  AND auto_start = true
                  AND deleted_at IS NULL
                ORDER BY deployment_delay_seconds ASC, created_at ASC
            `, {
                type: this.database.sequelize.QueryTypes.SELECT
            });

            console.log(`\n📊 Found ${interfaces.length} interface(s) configured for auto-start\n`);

            if (interfaces.length === 0) {
                console.log('ℹ️  No interfaces configured for auto-start');
                console.log('ℹ️  Interfaces can be started manually via UI or API\n');
                this.isInitialized = true;
                return;
            }

            // Process each interface based on deployment mode
            for (const intf of interfaces) {
                await this.processInterfaceDeployment(intf);
            }

            console.log('\n✅ ========================================');
            console.log('✅ Deployment Service Initialization Complete');
            console.log('✅ ========================================\n');

            this.isInitialized = true;

        } catch (error) {
            console.error('\n❌ Deployment Service Initialization Failed:', error.message);
            console.error('❌ Interfaces will need to be started manually\n');
            // Don't throw - allow app to continue running
        }
    }

    /**
     * Process interface deployment based on its configuration
     */
    async processInterfaceDeployment(intf) {
        const { id, name, deployment_mode, deployment_delay_seconds } = intf;

        console.log(`📋 Interface: ${name} (${id})`);
        console.log(`   Deployment Mode: ${deployment_mode}`);
        console.log(`   Delay: ${deployment_delay_seconds}s`);

        switch (deployment_mode) {
            case 'auto':
                // Deploy immediately
                console.log(`   🚀 Auto-deploying immediately...`);
                await this.deployInterface(intf);
                break;

            case 'delayed':
                // Deploy after delay
                const delayMs = (deployment_delay_seconds || 0) * 1000;
                console.log(`   ⏰ Scheduling deployment in ${deployment_delay_seconds}s...`);
                const timer = setTimeout(async () => {
                    console.log(`\n⏰ Delayed deployment triggered for: ${name}`);
                    await this.deployInterface(intf);
                    this.deploymentTimers.delete(id);
                }, delayMs);
                this.deploymentTimers.set(id, timer);
                console.log(`   ✅ Deployment scheduled\n`);
                break;

            case 'manual':
            default:
                // Skip - requires manual activation
                console.log(`   ⏭️  Manual deployment - skipping auto-start\n`);
                break;
        }
    }

    /**
     * Deploy (activate) an interface
     */
    async deployInterface(intf) {
        const { id, name, source_type, target_type } = intf;

        try {
            // Update deployment status to "deploying"
            await this.updateDeploymentStatus(id, 'deploying');

            // Call Go backend to activate interface
            console.log(`   🔗 Activating interface in Go backend...`);
            const response = await axios.post(
                `${this.goBackendUrl}/api/processing/interfaces/${id}/activate`,
                {},
                { timeout: 10000 }
            );

            if (response.data.success) {
                console.log(`   ✅ Interface activated successfully`);
                console.log(`   ✅ Source: ${source_type || 'N/A'}`);
                console.log(`   ✅ Target: ${target_type || 'N/A'}`);

                // Update deployment status to "deployed"
                await this.updateDeploymentStatus(id, 'deployed', true);

                console.log(`   ✅ Deployment complete\n`);
            } else {
                throw new Error(response.data.message || 'Activation failed');
            }

        } catch (error) {
            console.error(`   ❌ Deployment failed for ${name}:`, error.message);
            console.error(`   ❌ Interface will remain inactive\n`);

            // Update deployment status to "failed"
            await this.updateDeploymentStatus(id, 'failed');
        }
    }

    /**
     * Update interface deployment status in database
     */
    async updateDeploymentStatus(interfaceId, deploymentStatus, updateLastDeployed = false) {
        try {
            const query = updateLastDeployed
                ? `UPDATE interfaces
                   SET deployment_status = :deploymentStatus,
                       last_deployed_at = NOW(),
                       status = 'active'
                   WHERE id = :interfaceId`
                : `UPDATE interfaces
                   SET deployment_status = :deploymentStatus
                   WHERE id = :interfaceId`;

            await this.database.sequelize.query(query, {
                replacements: { interfaceId, deploymentStatus },
                type: this.database.sequelize.QueryTypes.UPDATE
            });
        } catch (error) {
            console.error(`⚠️  Failed to update deployment status:`, error.message);
        }
    }

    /**
     * Wait for Go backend to be ready
     */
    async waitForGoBackend(timeoutSeconds = 30) {
        const startTime = Date.now();
        const timeoutMs = timeoutSeconds * 1000;
        let attempts = 0;

        while (Date.now() - startTime < timeoutMs) {
            attempts++;
            try {
                const response = await axios.get(`${this.goBackendUrl}/health`, {
                    timeout: 2000
                });

                if (response.status === 200) {
                    console.log(`✅ Go backend is ready (attempt ${attempts})\n`);
                    return true;
                }
            } catch (error) {
                // Backend not ready yet, wait and retry
                await new Promise(resolve => setTimeout(resolve, 2000));
            }
        }

        throw new Error(`Go backend not ready after ${timeoutSeconds}s (${attempts} attempts)`);
    }

    /**
     * Manually deploy an interface (called from API/UI)
     */
    async manualDeploy(interfaceId) {
        console.log(`\n🔧 Manual deployment triggered for interface: ${interfaceId}`);

        try {
            // Get interface details
            const interfaces = await this.database.sequelize.query(`
                SELECT
                    id, name, deployment_mode, auto_start,
                    source_type, target_type, status
                FROM interfaces
                WHERE id = :interfaceId
                  AND is_active = true
                  AND deleted_at IS NULL
            `, {
                replacements: { interfaceId },
                type: this.database.sequelize.QueryTypes.SELECT
            });

            if (interfaces.length === 0) {
                throw new Error('Interface not found or inactive');
            }

            await this.deployInterface(interfaces[0]);
            return { success: true, message: 'Interface deployed successfully' };

        } catch (error) {
            console.error(`❌ Manual deployment failed:`, error.message);
            return { success: false, message: error.message };
        }
    }

    /**
     * Stop an interface (deactivate)
     */
    async stopInterface(interfaceId) {
        console.log(`\n🛑 Stopping interface: ${interfaceId}`);

        try {
            // Cancel any pending delayed deployment
            if (this.deploymentTimers.has(interfaceId)) {
                clearTimeout(this.deploymentTimers.get(interfaceId));
                this.deploymentTimers.delete(interfaceId);
                console.log(`⏰ Cancelled pending delayed deployment`);
            }

            // Call Go backend to deactivate interface
            const response = await axios.post(
                `${this.goBackendUrl}/api/processing/interfaces/${interfaceId}/deactivate`,
                {},
                { timeout: 10000 }
            );

            if (response.data.success) {
                console.log(`✅ Interface deactivated successfully`);

                // Update deployment status
                await this.updateDeploymentStatus(interfaceId, 'stopped');

                return { success: true, message: 'Interface stopped successfully' };
            } else {
                throw new Error(response.data.message || 'Deactivation failed');
            }

        } catch (error) {
            console.error(`❌ Stop failed:`, error.message);
            return { success: false, message: error.message };
        }
    }

    /**
     * Get deployment status for all interfaces
     */
    async getDeploymentStatus() {
        try {
            const interfaces = await this.database.sequelize.query(`
                SELECT
                    id,
                    name,
                    deployment_mode,
                    auto_start,
                    deployment_status,
                    last_deployed_at,
                    status
                FROM interfaces
                WHERE is_active = true
                  AND deleted_at IS NULL
                ORDER BY name
            `, {
                type: this.database.sequelize.QueryTypes.SELECT
            });

            return interfaces;
        } catch (error) {
            console.error('Failed to get deployment status:', error.message);
            return [];
        }
    }

    /**
     * Cleanup on shutdown
     */
    cleanup() {
        console.log('\n🧹 Cleaning up deployment timers...');
        for (const [interfaceId, timer] of this.deploymentTimers.entries()) {
            clearTimeout(timer);
            console.log(`   ⏰ Cancelled timer for interface: ${interfaceId}`);
        }
        this.deploymentTimers.clear();
        console.log('✅ Cleanup complete\n');
    }
}

module.exports = InterfaceDeploymentService;
