#!/bin/bash

# Development Data Seeding Script
# This script adds sample data for development
# DO NOT run this in production!

set -e

DB_NAME="ezhealthkonnect"
DB_USER="ezhealth_user"
CONTAINER_NAME="ezhealthkonnect-postgres"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Seeding development data...${NC}"

# Check if running in development
if [ "$NODE_ENV" == "production" ]; then
    echo -e "${RED}ERROR: This script should not be run in production!${NC}"
    exit 1
fi

# Ensure container is running
if ! docker ps | grep -q $CONTAINER_NAME; then
    echo -e "${YELLOW}Starting database container...${NC}"
    docker-compose up -d postgres
    sleep 5
fi

# Create development seed data
docker-compose exec -T postgres psql -U $DB_USER $DB_NAME << 'EOF'

-- Development Users (DO NOT commit these passwords to production!)
INSERT INTO users (email, name, password, role, status) VALUES 
('admin@localhost', 'Local Admin', 
 '$2a$10$YQj3HqJ5qXnCL5nWPtMSLOvnOmJ5jX1K3.9sQXrV1mYqL5n8YQj3q', 
 'admin', 'active'),
('dev@localhost', 'Developer User', 
 '$2a$10$YQj3HqJ5qXnCL5nWPtMSLOvnOmJ5jX1K3.9sQXrV1mYqL5n8YQj3q', 
 'user', 'active'),
('test@localhost', 'Test User', 
 '$2a$10$YQj3HqJ5qXnCL5nWPtMSLOvnOmJ5jX1K3.9sQXrV1mYqL5n8YQj3q', 
 'user', 'active')
ON CONFLICT (email) DO NOTHING;

-- Sample Interfaces for testing
INSERT INTO interfaces (name, description, source_type, target_type, source_config, target_config, created_by) 
SELECT 
    'Test ADT Interface', 
    'Development test interface for ADT messages', 
    'hl7', 
    'fhir', 
    '{"host": "localhost", "port": 2575}',
    '{"endpoint": "http://localhost:8080/fhir"}',
    u.id
FROM users u WHERE u.email = 'dev@localhost'
ON CONFLICT (name) DO NOTHING;

INSERT INTO interfaces (name, description, source_type, target_type, source_config, target_config, created_by)
SELECT 
    'File Processing Test', 
    'Test file-based message processing', 
    'hl7', 
    'fhir', 
    '{"directory": "/data/input", "pattern": "*.hl7"}',
    '{"directory": "/data/output"}',
    u.id
FROM users u WHERE u.email = 'dev@localhost'
ON CONFLICT (name) DO NOTHING;

-- Sample audit logs for testing
INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details, ip_address)
SELECT 
    u.id,
    'interface_created',
    'interface',
    i.id,
    '{"interface_name": "Test ADT Interface"}',
    '127.0.0.1'::inet
FROM users u, interfaces i 
WHERE u.email = 'dev@localhost' 
AND i.name = 'Test ADT Interface';

EOF

echo -e "${GREEN}Development data seeded successfully!${NC}"
echo -e "${YELLOW}Login credentials for development:${NC}"
echo -e "${YELLOW}  admin@localhost / admin123${NC}"
echo -e "${YELLOW}  dev@localhost / admin123${NC}"
echo -e "${YELLOW}  test@localhost / admin123${NC}"