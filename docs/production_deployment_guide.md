# Interface-Centric Configuration Engine - Production Deployment Guide

## Overview

This guide provides step-by-step instructions for deploying the Interface-Centric Configuration Engine to production environments. The system provides a robust, scalable configuration management platform for healthcare interface processing.

## Architecture Overview

The Interface-Centric Configuration Engine consists of:

- **MongoDB Configuration Manager**: Stores and manages interface configurations
- **Interface Engine**: Processes messages through configurable pipelines
- **Migration Service**: Migrates existing PostgreSQL data to MongoDB
- **Health Monitor**: Monitors system health and handles graceful degradation
- **REST API**: Provides configuration management endpoints

## Prerequisites

### System Requirements

#### Hardware Requirements
- **CPU**: Minimum 4 cores, recommended 8+ cores
- **Memory**: Minimum 8GB RAM, recommended 16+ GB
- **Storage**: Minimum 100GB SSD, recommended 500+ GB
- **Network**: Gigabit ethernet, low latency to database servers

#### Software Requirements
- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+, RHEL 8+)
- **Go**: Version 1.19 or higher
- **Node.js**: Version 16 or higher (for existing frontend compatibility)
- **MongoDB**: Version 5.0 or higher
- **PostgreSQL**: Version 12 or higher (for existing data and interface tables)

#### Network Requirements
- **MongoDB**: Port 27017 (configurable)
- **PostgreSQL**: Port 5432 (configurable)
- **API Server**: Port 8080 (configurable)
- **MLLP Listeners**: Ports 2575-2580 (configurable per interface)

### Database Setup

#### MongoDB Configuration

1. **Install MongoDB**
```bash
# Ubuntu/Debian
wget -qO - https://www.mongodb.org/static/pgp/server-5.0.asc | sudo apt-key add -
echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/ubuntu focal/mongodb-org/5.0 multiverse" | sudo tee /etc/apt/sources.list.d/mongodb-org-5.0.list
sudo apt-get update
sudo apt-get install -y mongodb-org

# CentOS/RHEL
sudo yum install -y mongodb-org
```

2. **Configure MongoDB**
```yaml
# /etc/mongod.conf
storage:
  dbPath: /var/lib/mongo
  journal:
    enabled: true

systemLog:
  destination: file
  logAppend: true
  path: /var/log/mongodb/mongod.log

net:
  port: 27017
  bindIp: 0.0.0.0  # Change to specific IPs in production

processManagement:
  fork: true
  pidFilePath: /var/run/mongodb/mongod.pid

security:
  authorization: enabled

replication:
  replSetName: "ezhealthkonnect"
```

3. **Initialize Replica Set**
```bash
# Start MongoDB
sudo systemctl start mongod

# Connect and initialize
mongo
> rs.initiate()
> use admin
> db.createUser({
    user: "ezhealthkonnect",
    pwd: "secure_password_here",
    roles: [
      { role: "readWrite", db: "ezhealthkonnect" },
      { role: "dbAdmin", db: "ezhealthkonnect" }
    ]
  })
```

#### PostgreSQL Configuration

1. **Verify Existing Installation**
```bash
# Check PostgreSQL status
sudo systemctl status postgresql

# Check existing interfaces table
psql -U postgres -d ezhealthkonnect -c "\dt interfaces"
```

2. **Create Configuration Engine User**
```sql
-- Create dedicated user for configuration engine
CREATE USER config_engine WITH PASSWORD 'secure_password_here';
GRANT SELECT, INSERT, UPDATE ON interfaces TO config_engine;
GRANT SELECT, INSERT, UPDATE ON wizard_mappings TO config_engine;
GRANT ALL PRIVILEGES ON SCHEMA public TO config_engine;
```

### Security Configuration

#### SSL/TLS Setup

1. **Generate Certificates**
```bash
# Generate CA key and certificate
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 365 -key ca-key.pem -out ca-cert.pem

# Generate server key and certificate
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server-csr.pem
openssl x509 -req -days 365 -in server-csr.pem -CA ca-cert.pem -CAkey ca-key.pem -out server-cert.pem -CAcreateserial

# Set proper permissions
chmod 400 ca-key.pem server-key.pem
chmod 444 ca-cert.pem server-cert.pem
```

2. **MongoDB SSL Configuration**
```yaml
# Add to /etc/mongod.conf
net:
  ssl:
    mode: requireSSL
    PEMKeyFile: /etc/ssl/mongodb/server.pem
    CAFile: /etc/ssl/mongodb/ca.pem
```

3. **PostgreSQL SSL Configuration**
```bash
# Add to postgresql.conf
ssl = on
ssl_cert_file = '/etc/ssl/postgres/server.crt'
ssl_key_file = '/etc/ssl/postgres/server.key'
ssl_ca_file = '/etc/ssl/postgres/ca.crt'
```

## Installation Process

### 1. Application Deployment

#### Download and Extract
```bash
# Create application directory
sudo mkdir -p /opt/ezhealthkonnect
sudo chown -R ezhealthkonnect:ezhealthkonnect /opt/ezhealthkonnect

# Download release
cd /opt/ezhealthkonnect
wget https://github.com/your-org/ezhealthkonnect/releases/download/v1.0.0/ezhealthkonnect-v1.0.0.tar.gz
tar -xzf ezhealthkonnect-v1.0.0.tar.gz
```

#### Build from Source (Alternative)
```bash
# Clone repository
git clone https://github.com/your-org/ezhealthkonnect.git
cd ezhealthkonnect

# Build Go application
go mod download
go build -o bin/ezhealthkonnect main.go

# Build Node.js components (if needed)
npm install
npm run build
```

### 2. Configuration Setup

#### Environment Configuration
```bash
# Create environment file
sudo nano /opt/ezhealthkonnect/.env
```

```env
# Database Configuration
MONGODB_URI=mongodb://ezhealthkonnect:secure_password@localhost:27017/ezhealthkonnect?authSource=admin&ssl=true
MONGODB_DATABASE=ezhealthkonnect
DATABASE_URL=postgres://config_engine:secure_password@localhost:5432/ezhealthkonnect?sslmode=require

# Application Configuration
PORT=8080
API_PORT=8080
VERBOSE_LOGGING=false
ENVIRONMENT=production

# Security Configuration
SESSION_SECRET=very_secure_session_secret_here
JWT_SECRET=very_secure_jwt_secret_here

# SSL Configuration
TLS_CERT_FILE=/etc/ssl/ezhealthkonnect/server.crt
TLS_KEY_FILE=/etc/ssl/ezhealthkonnect/server.key
TLS_CA_FILE=/etc/ssl/ezhealthkonnect/ca.crt

# Health Check Configuration
HEALTH_CHECK_INTERVAL=30s
STARTUP_TIMEOUT=60s

# Performance Configuration
MAX_CONCURRENT_MESSAGES=100
MESSAGE_PROCESSING_TIMEOUT=30s
CONFIGURATION_CACHE_SIZE=1000
```

#### System Service Configuration
```bash
# Create systemd service file
sudo nano /etc/systemd/system/ezhealthkonnect.service
```

```ini
[Unit]
Description=ezHealthKonnect Interface Configuration Engine
After=network.target mongodb.service postgresql.service
Requires=mongodb.service postgresql.service

[Service]
Type=simple
User=ezhealthkonnect
Group=ezhealthkonnect
WorkingDirectory=/opt/ezhealthkonnect
ExecStart=/opt/ezhealthkonnect/bin/ezhealthkonnect
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=ezhealthkonnect

# Environment
EnvironmentFile=/opt/ezhealthkonnect/.env

# Security
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/ezhealthkonnect/logs /var/log/ezhealthkonnect

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

### 3. Service Initialization

#### Create Service User
```bash
# Create dedicated user
sudo useradd -r -s /bin/false ezhealthkonnect
sudo usermod -a -G mongodb,postgres ezhealthkonnect

# Set ownership
sudo chown -R ezhealthkonnect:ezhealthkonnect /opt/ezhealthkonnect
sudo mkdir -p /var/log/ezhealthkonnect
sudo chown -R ezhealthkonnect:ezhealthkonnect /var/log/ezhealthkonnect
```

#### Database Initialization
```bash
# Run as application user
sudo -u ezhealthkonnect /opt/ezhealthkonnect/bin/ezhealthkonnect --init-db
```

#### Service Management
```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service
sudo systemctl enable ezhealthkonnect

# Start service
sudo systemctl start ezhealthkonnect

# Check status
sudo systemctl status ezhealthkonnect
```

## Migration Process

### Pre-Migration Checklist

1. **Backup Existing Data**
```bash
# Backup PostgreSQL
pg_dump -U postgres ezhealthkonnect > ezhealthkonnect_backup_$(date +%Y%m%d).sql

# Backup configuration files
tar -czf config_backup_$(date +%Y%m%d).tar.gz /opt/ezhealthkonnect/config/
```

2. **Verify System Health**
```bash
# Check service status
curl http://localhost:8080/health

# Check database connectivity
curl http://localhost:8080/api/config/health
```

### Migration Execution

1. **Validate Migration**
```bash
# Test migration (dry run)
curl -X POST "http://localhost:8080/api/config/migrate/validate"
```

2. **Execute Migration**
```bash
# Start migration
curl -X POST "http://localhost:8080/api/config/migrate"

# Monitor progress
watch curl -s http://localhost:8080/api/config/migrate/status
```

3. **Verify Migration Results**
```bash
# Check migrated configurations
curl http://localhost:8080/api/config/interfaces

# Validate specific configuration
curl http://localhost:8080/api/config/interfaces/{interface-id}
```

## Monitoring and Alerting

### Health Monitoring Setup

1. **Configure Health Checks**
```bash
# Add to monitoring system (Prometheus example)
# /etc/prometheus/prometheus.yml
scrape_configs:
  - job_name: 'ezhealthkonnect'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/api/system/metrics'
    scrape_interval: 30s
```

2. **Set Up Alerts**
```yaml
# alerts.yml
groups:
  - name: ezhealthkonnect
    rules:
      - alert: HighErrorRate
        expr: ezhealthkonnect_error_rate > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"

      - alert: ServiceDown
        expr: up{job="ezhealthkonnect"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "ezHealthKonnect service is down"
```

### Log Management

1. **Configure Centralized Logging**
```bash
# Configure rsyslog
echo "local0.*    /var/log/ezhealthkonnect/application.log" >> /etc/rsyslog.d/50-ezhealthkonnect.conf
sudo systemctl restart rsyslog
```

2. **Log Rotation**
```bash
# /etc/logrotate.d/ezhealthkonnect
/var/log/ezhealthkonnect/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 644 ezhealthkonnect ezhealthkonnect
    postrotate
        /bin/kill -HUP $(cat /var/run/ezhealthkonnect/ezhealthkonnect.pid 2> /dev/null) 2> /dev/null || true
    endscript
}
```

## Performance Optimization

### Database Tuning

#### MongoDB Optimization
```javascript
// Run in MongoDB shell
use ezhealthkonnect

// Create performance indexes
db.interface_configs.createIndex({"interface_id": 1}, {unique: true})
db.interface_configs.createIndex({"status": 1, "updated_at": -1})
db.interface_configs.createIndex({"config_hash": 1})

// Configure read preferences for performance
db.runCommand({
  collMod: "interface_configs",
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["interface_id", "name", "pipeline"],
      properties: {
        interface_id: {bsonType: "string"},
        name: {bsonType: "string"},
        status: {enum: ["draft", "active", "paused", "stopped"]}
      }
    }
  }
})
```

#### PostgreSQL Optimization
```sql
-- Optimize interface tables
CREATE INDEX CONCURRENTLY idx_interfaces_status_updated ON interfaces(status, updated_at);
CREATE INDEX CONCURRENTLY idx_wizard_mappings_interface ON wizard_mappings(interface_id);

-- Update statistics
ANALYZE interfaces;
ANALYZE wizard_mappings;

-- Configure autovacuum
ALTER TABLE interfaces SET (autovacuum_vacuum_scale_factor = 0.1);
ALTER TABLE wizard_mappings SET (autovacuum_vacuum_scale_factor = 0.1);
```

### Application Tuning

1. **Memory Configuration**
```env
# Add to .env file
GO_MAX_PROCS=8
GO_GC_PERCENT=100
CONFIG_CACHE_SIZE=2000
MESSAGE_BUFFER_SIZE=10000
```

2. **Connection Pooling**
```env
# Database connection limits
MONGODB_MAX_POOL_SIZE=100
MONGODB_MIN_POOL_SIZE=5
POSTGRES_MAX_CONNECTIONS=50
POSTGRES_MAX_IDLE_CONNECTIONS=10
```

## Security Hardening

### Network Security

1. **Firewall Configuration**
```bash
# Configure iptables
sudo iptables -A INPUT -p tcp --dport 8080 -s trusted_network/24 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 27017 -s localhost -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 5432 -s localhost -j ACCEPT

# Save rules
sudo iptables-save > /etc/iptables/rules.v4
```

2. **Service Account Security**
```bash
# Limit service account privileges
sudo passwd -l ezhealthkonnect  # Lock password
sudo usermod -s /usr/sbin/nologin ezhealthkonnect  # Disable shell

# Set up sudo rules for limited admin access
echo "ezhealthkonnect ALL=(root) NOPASSWD: /bin/systemctl restart ezhealthkonnect" >> /etc/sudoers.d/ezhealthkonnect
```

### Application Security

1. **Enable Security Features**
```env
# Add to .env
ENABLE_RATE_LIMITING=true
RATE_LIMIT_REQUESTS=1000
RATE_LIMIT_WINDOW=60s

ENABLE_AUDIT_LOGGING=true
AUDIT_LOG_FILE=/var/log/ezhealthkonnect/audit.log

ENABLE_TLS=true
TLS_MIN_VERSION=1.2
```

2. **Database Security**
```bash
# MongoDB user permissions
mongo --ssl --host localhost:27017 -u admin -p
> use ezhealthkonnect
> db.createUser({
    user: "app_readonly",
    pwd: "readonly_password",
    roles: [{ role: "read", db: "ezhealthkonnect" }]
  })
```

## Backup and Recovery

### Automated Backup Setup

1. **MongoDB Backup**
```bash
#!/bin/bash
# /opt/ezhealthkonnect/scripts/backup_mongodb.sh

BACKUP_DIR="/backup/mongodb/$(date +%Y-%m-%d)"
mkdir -p $BACKUP_DIR

mongodump --ssl --host localhost:27017 \
  --username ezhealthkonnect \
  --password $MONGODB_PASSWORD \
  --db ezhealthkonnect \
  --out $BACKUP_DIR

# Compress backup
tar -czf $BACKUP_DIR.tar.gz -C /backup/mongodb $(date +%Y-%m-%d)
rm -rf $BACKUP_DIR

# Cleanup old backups (keep 30 days)
find /backup/mongodb -name "*.tar.gz" -mtime +30 -delete
```

2. **PostgreSQL Backup**
```bash
#!/bin/bash
# /opt/ezhealthkonnect/scripts/backup_postgresql.sh

BACKUP_DIR="/backup/postgresql"
mkdir -p $BACKUP_DIR

pg_dump -h localhost -U config_engine ezhealthkonnect | \
  gzip > $BACKUP_DIR/ezhealthkonnect_$(date +%Y%m%d_%H%M%S).sql.gz

# Cleanup old backups (keep 30 days)
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete
```

3. **Automated Scheduling**
```bash
# Add to crontab
sudo crontab -e -u ezhealthkonnect

# Backup MongoDB daily at 2 AM
0 2 * * * /opt/ezhealthkonnect/scripts/backup_mongodb.sh

# Backup PostgreSQL every 6 hours
0 */6 * * * /opt/ezhealthkonnect/scripts/backup_postgresql.sh
```

### Recovery Procedures

1. **MongoDB Recovery**
```bash
# Stop service
sudo systemctl stop ezhealthkonnect

# Restore from backup
tar -xzf /backup/mongodb/2024-01-15.tar.gz -C /tmp/
mongorestore --ssl --host localhost:27017 \
  --username ezhealthkonnect \
  --password $MONGODB_PASSWORD \
  --db ezhealthkonnect \
  /tmp/2024-01-15/ezhealthkonnect

# Start service
sudo systemctl start ezhealthkonnect
```

2. **PostgreSQL Recovery**
```bash
# Restore specific tables
gunzip -c /backup/postgresql/ezhealthkonnect_20240115_020000.sql.gz | \
  psql -h localhost -U config_engine ezhealthkonnect
```

## Troubleshooting

### Common Issues

1. **Service Won't Start**
```bash
# Check service status
sudo systemctl status ezhealthkonnect

# Check logs
journalctl -u ezhealthkonnect -f

# Check configuration
/opt/ezhealthkonnect/bin/ezhealthkonnect --validate-config
```

2. **Database Connection Issues**
```bash
# Test MongoDB connection
mongo --ssl --host localhost:27017 -u ezhealthkonnect -p

# Test PostgreSQL connection
psql -h localhost -U config_engine ezhealthkonnect
```

3. **Performance Issues**
```bash
# Check resource usage
htop
iostat -x 1

# Check database performance
curl http://localhost:8080/api/system/metrics
```

### Log Analysis

1. **Application Logs**
```bash
# Real-time log monitoring
tail -f /var/log/ezhealthkonnect/application.log

# Search for errors
grep "ERROR" /var/log/ezhealthkonnect/application.log | tail -20

# Check health status
grep "health" /var/log/ezhealthkonnect/application.log | tail -10
```

2. **Database Logs**
```bash
# MongoDB logs
tail -f /var/log/mongodb/mongod.log

# PostgreSQL logs
tail -f /var/log/postgresql/postgresql-*.log
```

## Maintenance Procedures

### Regular Maintenance

1. **Weekly Tasks**
```bash
#!/bin/bash
# /opt/ezhealthkonnect/scripts/weekly_maintenance.sh

# Update system packages
sudo apt update && sudo apt upgrade -y

# Cleanup old logs
find /var/log/ezhealthkonnect -name "*.log" -mtime +7 -delete

# Database maintenance
mongo --ssl --eval "db.runCommand({compact: 'interface_configs'})"
psql -U config_engine -d ezhealthkonnect -c "VACUUM ANALYZE;"

# Check disk space
df -h | grep -E "/(|var|opt)"
```

2. **Monthly Tasks**
```bash
#!/bin/bash
# /opt/ezhealthkonnect/scripts/monthly_maintenance.sh

# Full system backup
rsync -av /opt/ezhealthkonnect/ /backup/application/$(date +%Y%m)/

# Security updates
sudo apt update && sudo apt upgrade -y

# Certificate renewal (if using Let's Encrypt)
sudo certbot renew

# Performance analysis
curl -s http://localhost:8080/api/system/metrics > /tmp/metrics_$(date +%Y%m%d).json
```

### Scaling Procedures

1. **Horizontal Scaling**
```bash
# Add new application server
# 1. Provision new server
# 2. Install application
# 3. Configure with same .env settings
# 4. Point to shared MongoDB cluster
# 5. Add to load balancer
```

2. **Database Scaling**
```bash
# MongoDB replica set scaling
mongo --ssl
> rs.add("new-mongodb-server:27017")
> rs.status()
```

## Support and Maintenance

### Support Contacts
- **Technical Support**: support@ezhealthkonnect.com
- **Emergency Escalation**: +1-555-HEALTH (24/7)
- **Documentation**: https://docs.ezhealthkonnect.com

### Maintenance Windows
- **Regular Maintenance**: Sunday 2:00 AM - 4:00 AM EST
- **Emergency Maintenance**: As needed with 2-hour notice
- **Major Updates**: Quarterly with 1-week notice

---

## Deployment Checklist

- [ ] Hardware requirements met
- [ ] MongoDB installed and configured
- [ ] PostgreSQL verified and accessible
- [ ] SSL certificates generated and installed
- [ ] Application deployed and configured
- [ ] Service user created with proper permissions
- [ ] Systemd service configured and enabled
- [ ] Migration completed successfully
- [ ] Health checks passing
- [ ] Monitoring and alerting configured
- [ ] Backup procedures tested
- [ ] Security hardening implemented
- [ ] Performance optimization applied
- [ ] Documentation updated
- [ ] Team trained on new system

**Deployment Sign-off**:
- Technical Lead: _________________ Date: _________
- Operations Manager: _____________ Date: _________
- Security Officer: ______________ Date: _________