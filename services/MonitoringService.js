// services/MonitoringService.js
// Comprehensive monitoring and alerting service for processing engine

const EventEmitter = require('events');

class MonitoringService extends EventEmitter {
    constructor(options = {}) {
        super();

        this.config = {
            metricsInterval: options.metricsInterval || 60000, // 1 minute
            alertThresholds: {
                errorRate: options.errorRateThreshold || 0.1, // 10%
                avgProcessingTime: options.avgProcessingTimeThreshold || 30000, // 30 seconds
                queueDepth: options.queueDepthThreshold || 1000,
                memoryUsage: options.memoryThreshold || 0.8 // 80%
            },
            retentionPeriod: options.retentionPeriod || 7 * 24 * 60 * 60 * 1000, // 7 days
            ...options
        };

        // Metrics storage
        this.metrics = {
            processing: new Map(), // interfaceId -> metrics
            system: {
                startTime: Date.now(),
                totalProcessed: 0,
                totalErrors: 0,
                peakMemoryUsage: 0
            },
            alerts: []
        };

        this.isStarted = false;
        this.metricsTimer = null;

        // Initialize database connection when needed
    }

    /**
     * Initialize database connections for metrics storage
     */
    async initializeDatabase() {
        try {
            // Use existing database configuration
            const database = require('../config/database');
            this.pgPool = database.sequelize.connectionManager.pool;

            console.log('✅ Monitoring service using existing database connection');

        } catch (error) {
            console.error('❌ Failed to initialize monitoring database:', error);
            // Don't throw - monitoring should be optional
            this.pgPool = null;
        }
    }

    /**
     * Start monitoring service
     */
    async start() {
        if (this.isStarted) return;

        console.log('🔍 Starting Monitoring Service...');

        // Start metrics collection
        this.startMetricsCollection();

        // Start alert monitoring
        this.startAlertMonitoring();

        this.isStarted = true;
        this.emit('monitoring:started');

        console.log('✅ Monitoring Service started successfully');
    }

    /**
     * Record message processing metrics
     */
    recordProcessingMetrics(interfaceId, messageType, metrics) {
        try {
            const {
                messageId,
                processingTime,
                success,
                errorType = null,
                messageSize = 0,
                transformationTime = 0,
                outputTime = 0
            } = metrics;

            // Get or create interface metrics
            if (!this.metrics.processing.has(interfaceId)) {
                this.metrics.processing.set(interfaceId, {
                    interfaceId,
                    totalProcessed: 0,
                    totalErrors: 0,
                    totalProcessingTime: 0,
                    avgProcessingTime: 0,
                    messageTypes: new Map(),
                    recentMetrics: [], // Last 100 messages
                    lastProcessed: null,
                    errorsByType: new Map()
                });
            }

            const interfaceMetrics = this.metrics.processing.get(interfaceId);

            // Update counters
            interfaceMetrics.totalProcessed++;
            interfaceMetrics.totalProcessingTime += processingTime;
            interfaceMetrics.avgProcessingTime =
                interfaceMetrics.totalProcessingTime / interfaceMetrics.totalProcessed;
            interfaceMetrics.lastProcessed = new Date();

            if (!success) {
                interfaceMetrics.totalErrors++;
                if (errorType) {
                    const errorCount = interfaceMetrics.errorsByType.get(errorType) || 0;
                    interfaceMetrics.errorsByType.set(errorType, errorCount + 1);
                }
            }

            // Update message type metrics
            if (!interfaceMetrics.messageTypes.has(messageType)) {
                interfaceMetrics.messageTypes.set(messageType, {
                    count: 0,
                    avgTime: 0,
                    errors: 0
                });
            }

            const msgTypeMetrics = interfaceMetrics.messageTypes.get(messageType);
            msgTypeMetrics.count++;
            if (!success) msgTypeMetrics.errors++;

            // Store recent metrics (sliding window)
            interfaceMetrics.recentMetrics.push({
                messageId,
                messageType,
                processingTime,
                success,
                errorType,
                timestamp: new Date(),
                messageSize,
                transformationTime,
                outputTime
            });

            // Keep only last 100 metrics
            if (interfaceMetrics.recentMetrics.length > 100) {
                interfaceMetrics.recentMetrics.shift();
            }

            // Update system metrics
            this.metrics.system.totalProcessed++;
            if (!success) {
                this.metrics.system.totalErrors++;
            }

            // Emit metrics event
            this.emit('metrics:recorded', {
                interfaceId,
                messageType,
                success,
                processingTime
            });

            // Check for immediate alerts
            this.checkImmediateAlerts(interfaceId, interfaceMetrics);

        } catch (error) {
            console.error('❌ Error recording processing metrics:', error);
        }
    }

    /**
     * Record system performance metrics
     */
    recordSystemMetrics() {
        const memoryUsage = process.memoryUsage();
        const cpuUsage = process.cpuUsage();

        const systemMetrics = {
            timestamp: new Date(),
            memory: {
                rss: memoryUsage.rss,
                heapUsed: memoryUsage.heapUsed,
                heapTotal: memoryUsage.heapTotal,
                external: memoryUsage.external,
                usage: memoryUsage.heapUsed / memoryUsage.heapTotal
            },
            cpu: {
                user: cpuUsage.user,
                system: cpuUsage.system
            },
            uptime: Date.now() - this.metrics.system.startTime,
            activeInterfaces: this.metrics.processing.size
        };

        // Track peak memory usage
        if (systemMetrics.memory.usage > this.metrics.system.peakMemoryUsage) {
            this.metrics.system.peakMemoryUsage = systemMetrics.memory.usage;
        }

        // Store in database (async, non-blocking)
        this.storeSystemMetrics(systemMetrics).catch(error => {
            console.error('❌ Error storing system metrics:', error);
        });

        // Check system alerts
        this.checkSystemAlerts(systemMetrics);

        return systemMetrics;
    }

    /**
     * Check for immediate processing alerts
     */
    checkImmediateAlerts(interfaceId, interfaceMetrics) {
        const recentMetrics = interfaceMetrics.recentMetrics.slice(-10); // Last 10 messages

        if (recentMetrics.length < 10) return;

        // Calculate recent error rate
        const recentErrors = recentMetrics.filter(m => !m.success).length;
        const errorRate = recentErrors / recentMetrics.length;

        // Alert if error rate exceeds threshold
        if (errorRate > this.config.alertThresholds.errorRate) {
            this.triggerAlert('HIGH_ERROR_RATE', {
                interfaceId,
                errorRate: errorRate * 100,
                threshold: this.config.alertThresholds.errorRate * 100,
                recentErrors,
                totalMessages: recentMetrics.length
            });
        }

        // Check processing time alerts
        const avgRecentTime = recentMetrics.reduce((sum, m) => sum + m.processingTime, 0) / recentMetrics.length;

        if (avgRecentTime > this.config.alertThresholds.avgProcessingTime) {
            this.triggerAlert('SLOW_PROCESSING', {
                interfaceId,
                avgProcessingTime: avgRecentTime,
                threshold: this.config.alertThresholds.avgProcessingTime
            });
        }
    }

    /**
     * Check system-level alerts
     */
    checkSystemAlerts(systemMetrics) {
        // Memory usage alert
        if (systemMetrics.memory.usage > this.config.alertThresholds.memoryUsage) {
            this.triggerAlert('HIGH_MEMORY_USAGE', {
                currentUsage: (systemMetrics.memory.usage * 100).toFixed(1),
                threshold: (this.config.alertThresholds.memoryUsage * 100).toFixed(1),
                heapUsed: Math.round(systemMetrics.memory.heapUsed / 1024 / 1024),
                heapTotal: Math.round(systemMetrics.memory.heapTotal / 1024 / 1024)
            });
        }
    }

    /**
     * Trigger alert with deduplication
     */
    triggerAlert(alertType, alertData) {
        const alertKey = `${alertType}_${alertData.interfaceId || 'system'}`;
        const now = Date.now();

        // Check if same alert was triggered recently (deduplication)
        const recentAlert = this.metrics.alerts.find(
            alert => alert.key === alertKey &&
            (now - alert.timestamp) < 300000 // 5 minutes
        );

        if (recentAlert) {
            recentAlert.count = (recentAlert.count || 1) + 1;
            return;
        }

        // Create new alert
        const alert = {
            key: alertKey,
            type: alertType,
            data: alertData,
            timestamp: now,
            severity: this.getAlertSeverity(alertType),
            count: 1
        };

        this.metrics.alerts.push(alert);

        // Store in database
        this.storeAlert(alert).catch(error => {
            console.error('❌ Error storing alert:', error);
        });

        // Emit alert event
        this.emit('alert:triggered', alert);

        console.warn(`🚨 ALERT [${alert.severity}] ${alertType}:`, alertData);

        // Clean up old alerts
        this.cleanupOldAlerts();
    }

    /**
     * Get alert severity level
     */
    getAlertSeverity(alertType) {
        const severityMap = {
            'HIGH_ERROR_RATE': 'CRITICAL',
            'SLOW_PROCESSING': 'WARNING',
            'HIGH_MEMORY_USAGE': 'WARNING',
            'INTERFACE_DOWN': 'CRITICAL',
            'CONNECTION_FAILURE': 'ERROR'
        };

        return severityMap[alertType] || 'INFO';
    }

    /**
     * Start metrics collection timer
     */
    startMetricsCollection() {
        this.metricsTimer = setInterval(() => {
            try {
                this.recordSystemMetrics();
                this.persistProcessingMetrics();
            } catch (error) {
                console.error('❌ Error in metrics collection:', error);
            }
        }, this.config.metricsInterval);
    }

    /**
     * Start alert monitoring
     */
    startAlertMonitoring() {
        // Set up additional monitoring tasks
        setInterval(() => {
            this.checkInterfaceHealth();
        }, 60000); // Check every minute
    }

    /**
     * Check health of all active interfaces
     */
    checkInterfaceHealth() {
        const now = Date.now();
        const healthTimeout = 5 * 60 * 1000; // 5 minutes

        this.metrics.processing.forEach((interfaceMetrics, interfaceId) => {
            if (interfaceMetrics.lastProcessed) {
                const timeSinceLastMessage = now - interfaceMetrics.lastProcessed.getTime();

                if (timeSinceLastMessage > healthTimeout) {
                    this.triggerAlert('INTERFACE_INACTIVE', {
                        interfaceId,
                        timeSinceLastMessage: Math.round(timeSinceLastMessage / 1000 / 60),
                        threshold: Math.round(healthTimeout / 1000 / 60)
                    });
                }
            }
        });
    }

    /**
     * Persist processing metrics to database
     */
    async persistProcessingMetrics() {
        try {
            for (const [interfaceId, metrics] of this.metrics.processing.entries()) {
                // Update daily metrics
                await this.pgPool.query(
                    `INSERT INTO interface_processing_metrics
                    (interface_id, metric_date, messages_received, messages_processed, messages_failed,
                     avg_processing_time_ms, total_processing_time_ms, updated_at)
                    VALUES ($1, CURRENT_DATE, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
                    ON CONFLICT (interface_id, metric_date)
                    DO UPDATE SET
                        messages_received = interface_processing_metrics.messages_received + EXCLUDED.messages_received,
                        messages_processed = interface_processing_metrics.messages_processed + EXCLUDED.messages_processed,
                        messages_failed = interface_processing_metrics.messages_failed + EXCLUDED.messages_failed,
                        total_processing_time_ms = interface_processing_metrics.total_processing_time_ms + EXCLUDED.total_processing_time_ms,
                        avg_processing_time_ms = (interface_processing_metrics.total_processing_time_ms + EXCLUDED.total_processing_time_ms) /
                            GREATEST(interface_processing_metrics.messages_received + EXCLUDED.messages_received, 1),
                        updated_at = CURRENT_TIMESTAMP`,
                    [
                        interfaceId,
                        metrics.totalProcessed,
                        metrics.totalProcessed - metrics.totalErrors,
                        metrics.totalErrors,
                        metrics.avgProcessingTime,
                        metrics.totalProcessingTime
                    ]
                );
            }

        } catch (error) {
            console.error('❌ Error persisting metrics:', error);
        }
    }

    /**
     * Store system metrics in database
     */
    async storeSystemMetrics(systemMetrics) {
        try {
            await this.pgPool.query(
                `INSERT INTO system_metrics
                (timestamp, memory_usage_mb, heap_used_mb, heap_total_mb, cpu_user, cpu_system, uptime_ms, active_interfaces)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
                [
                    systemMetrics.timestamp,
                    Math.round(systemMetrics.memory.rss / 1024 / 1024),
                    Math.round(systemMetrics.memory.heapUsed / 1024 / 1024),
                    Math.round(systemMetrics.memory.heapTotal / 1024 / 1024),
                    systemMetrics.cpu.user,
                    systemMetrics.cpu.system,
                    systemMetrics.uptime,
                    systemMetrics.activeInterfaces
                ]
            );
        } catch (error) {
            // Create table if it doesn't exist
            if (error.code === '42P01') {
                await this.createSystemMetricsTable();
                // Retry insert
                await this.storeSystemMetrics(systemMetrics);
            }
        }
    }

    /**
     * Store alert in database
     */
    async storeAlert(alert) {
        try {
            await this.pgPool.query(
                `INSERT INTO processing_alerts
                (alert_type, alert_data, severity, interface_id, timestamp)
                VALUES ($1, $2, $3, $4, $5)`,
                [
                    alert.type,
                    JSON.stringify(alert.data),
                    alert.severity,
                    alert.data.interfaceId || null,
                    new Date(alert.timestamp)
                ]
            );
        } catch (error) {
            console.error('❌ Error storing alert:', error);
        }
    }

    /**
     * Create system metrics table if not exists
     */
    async createSystemMetricsTable() {
        const createTableSQL = `
            CREATE TABLE IF NOT EXISTS system_metrics (
                id SERIAL PRIMARY KEY,
                timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
                memory_usage_mb INTEGER,
                heap_used_mb INTEGER,
                heap_total_mb INTEGER,
                cpu_user BIGINT,
                cpu_system BIGINT,
                uptime_ms BIGINT,
                active_interfaces INTEGER,
                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
            );

            CREATE INDEX IF NOT EXISTS idx_system_metrics_timestamp ON system_metrics(timestamp DESC);

            CREATE TABLE IF NOT EXISTS processing_alerts (
                id SERIAL PRIMARY KEY,
                alert_type VARCHAR(100) NOT NULL,
                alert_data JSONB,
                severity VARCHAR(20) NOT NULL,
                interface_id UUID,
                timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
                acknowledged BOOLEAN DEFAULT FALSE,
                created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
            );

            CREATE INDEX IF NOT EXISTS idx_processing_alerts_timestamp ON processing_alerts(timestamp DESC);
            CREATE INDEX IF NOT EXISTS idx_processing_alerts_interface ON processing_alerts(interface_id, timestamp DESC);
        `;

        await this.pgPool.query(createTableSQL);
        console.log('✅ Created monitoring tables');
    }

    /**
     * Clean up old alerts
     */
    cleanupOldAlerts() {
        const cutoff = Date.now() - this.config.retentionPeriod;
        this.metrics.alerts = this.metrics.alerts.filter(alert => alert.timestamp > cutoff);
    }

    /**
     * Get comprehensive monitoring dashboard data
     */
    getDashboardData() {
        const systemMetrics = this.recordSystemMetrics();

        const interfaceStats = Array.from(this.metrics.processing.entries()).map(([interfaceId, metrics]) => ({
            interfaceId,
            totalProcessed: metrics.totalProcessed,
            totalErrors: metrics.totalErrors,
            errorRate: metrics.totalProcessed > 0 ? (metrics.totalErrors / metrics.totalProcessed) * 100 : 0,
            avgProcessingTime: metrics.avgProcessingTime,
            lastProcessed: metrics.lastProcessed,
            messageTypes: Array.from(metrics.messageTypes.entries()).map(([type, typeMetrics]) => ({
                type,
                ...typeMetrics
            })),
            recentErrors: Array.from(metrics.errorsByType.entries()).map(([type, count]) => ({
                type,
                count
            }))
        }));

        const recentAlerts = this.metrics.alerts
            .filter(alert => (Date.now() - alert.timestamp) < 24 * 60 * 60 * 1000) // Last 24 hours
            .sort((a, b) => b.timestamp - a.timestamp)
            .slice(0, 50);

        return {
            system: {
                ...systemMetrics,
                totalProcessed: this.metrics.system.totalProcessed,
                totalErrors: this.metrics.system.totalErrors,
                overallErrorRate: this.metrics.system.totalProcessed > 0 ?
                    (this.metrics.system.totalErrors / this.metrics.system.totalProcessed) * 100 : 0,
                peakMemoryUsage: this.metrics.system.peakMemoryUsage
            },
            interfaces: interfaceStats,
            alerts: recentAlerts
        };
    }

    /**
     * Stop monitoring service
     */
    async stop() {
        console.log('🛑 Stopping Monitoring Service...');

        if (this.metricsTimer) {
            clearInterval(this.metricsTimer);
            this.metricsTimer = null;
        }

        // Final metrics persistence
        await this.persistProcessingMetrics();

        this.isStarted = false;
        this.emit('monitoring:stopped');

        console.log('✅ Monitoring Service stopped');
    }
}

module.exports = MonitoringService;