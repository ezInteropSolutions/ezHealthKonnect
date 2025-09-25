// services/InterfaceEngineManager.js
// Class-based Multi-Interface Integration Engine Manager
// Handles lifecycle management of multiple interface listeners for scalability

const InterfaceListenerService = require('./InterfaceListenerService');

class InterfaceEngineManager {
    constructor() {
        this.listenerService = null;
        this.isRunning = false;
        this.startupTime = null;
        this.shutdownGracefully = false;

        // Configuration
        this.config = {
            autoRestart: true,
            healthCheckInterval: 30000, // 30 seconds
            startupTimeout: 15000, // 15 seconds
            shutdownTimeout: 10000, // 10 seconds
            maxRetries: 3
        };

        // Event tracking
        this.events = {
            startup: [],
            shutdown: [],
            interfaceAdded: [],
            interfaceRemoved: [],
            errors: []
        };
    }

    /**
     * Initialize and start the multi-interface engine
     */
    async initialize() {
        try {
            console.log('\n🏗️ ===============================================');
            console.log('🚀 Initializing Multi-Interface Integration Engine');
            console.log(`📊 Engine Configuration:`);
            console.log(`   Auto-restart: ${this.config.autoRestart}`);
            console.log(`   Health checks: ${this.config.healthCheckInterval}ms`);
            console.log(`   Startup timeout: ${this.config.startupTimeout}ms`);
            console.log('🏗️ ===============================================');

            // Initialize the listener service
            this.listenerService = new InterfaceListenerService();

            // Start all active interface listeners
            await this.startAllInterfaces();

            // Mark as running
            this.isRunning = true;
            this.startupTime = new Date();

            // Start health monitoring
            this.startHealthMonitoring();

            // Log startup event
            this.logEvent('startup', {
                timestamp: this.startupTime,
                interfaceCount: this.getActiveInterfaceCount(),
                status: 'success'
            });

            const interfaceStatus = this.getInterfaceStatus();

            console.log('\n📊 Engine Initialization Complete:');
            console.log(`   Status: Running`);
            console.log(`   Active Interfaces: ${interfaceStatus.length}`);
            console.log(`   Started At: ${this.startupTime.toISOString()}`);

            if (interfaceStatus.length > 0) {
                console.log('\n🔌 Active Interface Listeners:');
                interfaceStatus.forEach(status => {
                    console.log(`   ✅ ${status.name} (${status.type.toUpperCase()}) - Port ${status.port}`);
                });
            } else {
                console.log('   ⚠️ No active interfaces found in database');
                console.log('   💡 Create interfaces via the web UI to start processing messages');
            }

            console.log('\n🏗️ ===============================================');
            console.log('✅ Multi-Interface Integration Engine Ready!');
            console.log('🏗️ ===============================================\n');

            return true;

        } catch (error) {
            console.error('❌ Failed to initialize Interface Engine Manager:', error.message);
            this.logEvent('startup', {
                timestamp: new Date(),
                status: 'failed',
                error: error.message
            });

            // Cleanup on failure
            await this.cleanup();
            throw error;
        }
    }

    /**
     * Start all active interfaces
     */
    async startAllInterfaces() {
        if (!this.listenerService) {
            throw new Error('Listener service not initialized');
        }

        console.log('🚀 Starting all active interface listeners...');

        // Use a timeout to prevent hanging
        return new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error(`Interface startup timed out after ${this.config.startupTimeout}ms`));
            }, this.config.startupTimeout);

            this.listenerService.startAllListeners()
                .then(() => {
                    clearTimeout(timeout);
                    resolve();
                })
                .catch((error) => {
                    clearTimeout(timeout);
                    reject(error);
                });
        });
    }

    /**
     * Stop all interfaces gracefully
     */
    async shutdown() {
        try {
            console.log('\n🛑 Shutting down Multi-Interface Integration Engine...');
            this.shutdownGracefully = true;

            // Stop health monitoring
            this.stopHealthMonitoring();

            // Stop all listeners with timeout
            if (this.listenerService) {
                await Promise.race([
                    this.listenerService.stopAllListeners(),
                    new Promise((_, reject) =>
                        setTimeout(() => reject(new Error('Shutdown timeout')), this.config.shutdownTimeout)
                    )
                ]);
            }

            // Mark as stopped
            this.isRunning = false;
            const shutdownTime = new Date();

            // Log shutdown event
            this.logEvent('shutdown', {
                timestamp: shutdownTime,
                uptime: shutdownTime - this.startupTime,
                graceful: true,
                status: 'success'
            });

            console.log('✅ All interface listeners stopped gracefully');
            console.log(`⏱️ Total uptime: ${this.getUptime()}`);
            console.log('✅ Multi-Interface Integration Engine shutdown complete\n');

            return true;

        } catch (error) {
            console.error('❌ Error during engine shutdown:', error.message);
            this.logEvent('shutdown', {
                timestamp: new Date(),
                graceful: false,
                status: 'failed',
                error: error.message
            });

            // Force cleanup
            await this.cleanup();
            throw error;
        }
    }

    /**
     * Restart the entire engine
     */
    async restart() {
        console.log('🔄 Restarting Multi-Interface Integration Engine...');

        try {
            // Shutdown first
            if (this.isRunning) {
                await this.shutdown();
            }

            // Wait for ports to be released
            await new Promise(resolve => setTimeout(resolve, 1000));

            // Initialize again
            await this.initialize();

            console.log('✅ Multi-Interface Integration Engine restarted successfully');
            return true;

        } catch (error) {
            console.error('❌ Engine restart failed:', error.message);
            throw error;
        }
    }

    /**
     * Add a new interface dynamically
     */
    async addInterface(interfaceConfig) {
        try {
            console.log(`➕ Adding new interface: ${interfaceConfig.name}`);

            if (!this.listenerService) {
                throw new Error('Engine not initialized');
            }

            // Start the new interface listener
            await this.listenerService.startInterfaceListener(interfaceConfig);

            this.logEvent('interfaceAdded', {
                timestamp: new Date(),
                interfaceId: interfaceConfig.id,
                name: interfaceConfig.name,
                type: interfaceConfig.source_type
            });

            console.log(`✅ Interface added successfully: ${interfaceConfig.name}`);
            return true;

        } catch (error) {
            console.error(`❌ Failed to add interface ${interfaceConfig.name}:`, error.message);
            throw error;
        }
    }

    /**
     * Remove an interface dynamically
     */
    async removeInterface(interfaceId) {
        try {
            console.log(`➖ Removing interface: ${interfaceId}`);

            if (!this.listenerService || !this.listenerService.listeners.has(interfaceId)) {
                throw new Error('Interface not found or engine not initialized');
            }

            // Stop the specific interface listener
            const listener = this.listenerService.listeners.get(interfaceId);
            listener.server.close();
            this.listenerService.listeners.delete(interfaceId);

            this.logEvent('interfaceRemoved', {
                timestamp: new Date(),
                interfaceId,
                name: listener.config.name || 'Unknown'
            });

            console.log(`✅ Interface removed successfully: ${interfaceId}`);
            return true;

        } catch (error) {
            console.error(`❌ Failed to remove interface ${interfaceId}:`, error.message);
            throw error;
        }
    }

    /**
     * Get current interface status
     */
    getInterfaceStatus() {
        if (!this.listenerService) {
            return [];
        }
        return this.listenerService.getListenerStatus();
    }

    /**
     * Get active interface count
     */
    getActiveInterfaceCount() {
        return this.getInterfaceStatus().length;
    }

    /**
     * Get engine health status
     */
    getEngineHealth() {
        const status = this.getInterfaceStatus();
        const uptime = this.getUptime();

        return {
            isRunning: this.isRunning,
            startupTime: this.startupTime,
            uptime,
            activeInterfaces: status.length,
            interfaces: status,
            config: this.config,
            events: {
                startupEvents: this.events.startup.length,
                shutdownEvents: this.events.shutdown.length,
                interfaceEvents: this.events.interfaceAdded.length + this.events.interfaceRemoved.length,
                errorEvents: this.events.errors.length
            },
            lastHealthCheck: new Date().toISOString()
        };
    }

    /**
     * Get uptime in readable format
     */
    getUptime() {
        if (!this.startupTime) return '0ms';

        const uptime = Date.now() - this.startupTime.getTime();
        const seconds = Math.floor(uptime / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);

        if (hours > 0) {
            return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
        } else if (minutes > 0) {
            return `${minutes}m ${seconds % 60}s`;
        } else {
            return `${seconds}s`;
        }
    }

    /**
     * Start health monitoring
     */
    startHealthMonitoring() {
        if (this.healthCheckInterval) {
            clearInterval(this.healthCheckInterval);
        }

        this.healthCheckInterval = setInterval(() => {
            if (!this.shutdownGracefully) {
                this.performHealthCheck();
            }
        }, this.config.healthCheckInterval);

        console.log(`🩺 Health monitoring started (${this.config.healthCheckInterval}ms intervals)`);
    }

    /**
     * Stop health monitoring
     */
    stopHealthMonitoring() {
        if (this.healthCheckInterval) {
            clearInterval(this.healthCheckInterval);
            this.healthCheckInterval = null;
            console.log('🩺 Health monitoring stopped');
        }
    }

    /**
     * Perform health check
     */
    performHealthCheck() {
        try {
            const health = this.getEngineHealth();

            // Log health status periodically
            if (health.activeInterfaces > 0) {
                console.log(`💓 Engine Health: ${health.activeInterfaces} interfaces, uptime ${health.uptime}`);
            }

            // Auto-restart if no interfaces are running but should be
            if (this.config.autoRestart && health.activeInterfaces === 0 && this.isRunning) {
                console.log('⚠️ No active interfaces detected - checking if restart needed...');
                // Additional logic for auto-restart could go here
            }

        } catch (error) {
            console.error('❌ Health check failed:', error.message);
            this.logEvent('errors', {
                timestamp: new Date(),
                type: 'health_check_failed',
                error: error.message
            });
        }
    }

    /**
     * Log an event for debugging and monitoring
     */
    logEvent(eventType, eventData) {
        if (this.events[eventType]) {
            this.events[eventType].push(eventData);

            // Keep only last 100 events per type
            if (this.events[eventType].length > 100) {
                this.events[eventType] = this.events[eventType].slice(-100);
            }
        }
    }

    /**
     * Cleanup resources
     */
    async cleanup() {
        this.stopHealthMonitoring();
        this.isRunning = false;
        this.listenerService = null;
    }

    /**
     * Get recent events for debugging
     */
    getRecentEvents(eventType = null, limit = 10) {
        if (eventType && this.events[eventType]) {
            return this.events[eventType].slice(-limit);
        }

        // Return all recent events
        const allEvents = [];
        Object.keys(this.events).forEach(type => {
            this.events[type].forEach(event => {
                allEvents.push({ type, ...event });
            });
        });

        return allEvents
            .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp))
            .slice(0, limit);
    }
}

module.exports = InterfaceEngineManager;