#!/bin/bash

# =====================================
# Flexible HL7→FHIR Mapping Setup Script
# =====================================

set -e  # Exit on any error

echo "🚀 Setting up HL7→FHIR Transformation Mapping..."
echo "=================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️ $1${NC}"
}

# Configuration with environment variable defaults
CONTAINER_NAME="${POSTGRES_CONTAINER:-ezhealthkonnect-postgres}"
DB_NAME="${POSTGRES_DB:-ezhealthkonnect}"
DB_USER="${POSTGRES_USER:-postgres}"
DB_PASSWORD="${POSTGRES_PASSWORD:-}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"

echo "🔧 Configuration:"
echo "   Container: ${CONTAINER_NAME}"
echo "   Database:  ${DB_NAME}"
echo "   User:      ${DB_USER}"
echo "   Host:      ${DB_HOST}"
echo "   Port:      ${DB_PORT}"
if [ -n "$DB_PASSWORD" ]; then
    echo "   Password:  [SET]"
else
    echo "   Password:  [NOT SET]"
fi

# Check if Docker container is running
echo ""
echo "🔍 Checking Docker PostgreSQL container..."
if ! docker ps --format "{{.Names}}" | grep -q "^${CONTAINER_NAME}$"; then
    print_error "PostgreSQL container '${CONTAINER_NAME}' is not running!"
    print_info "Available containers:"
    docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep postgres || echo "No PostgreSQL containers found"
    echo ""
    print_info "Start it with: docker start ${CONTAINER_NAME}"
    exit 1
fi
print_status "PostgreSQL container is running"

# Function to detect database credentials
detect_credentials() {
    print_info "Detecting database credentials..."
    
    # Check container environment variables
    echo "🔍 Container environment variables:"
    docker exec ${CONTAINER_NAME} env | grep -E "(POSTGRES|DB)" || echo "No PostgreSQL env vars found"
    
    echo ""
    print_info "Trying to detect credentials..."
    
    # Try different common username/password combinations
    local users=("postgres" "root" "admin" "$DB_USER")
    local passwords=("" "postgres" "password" "admin" "$DB_PASSWORD")
    
    for user in "${users[@]}"; do
        for pass in "${passwords[@]}"; do
            print_info "Trying user: $user, password: ${pass:-[empty]}"
            
            if [ -z "$pass" ]; then
                # Try without password
                if docker exec ${CONTAINER_NAME} psql -U "$user" -d postgres -c "SELECT 1;" >/dev/null 2>&1; then
                    print_status "Found working credentials: user=$user, password=[empty]"
                    DB_USER="$user"
                    DB_PASSWORD=""
                    return 0
                fi
            else
                # Try with password
                if docker exec ${CONTAINER_NAME} env PGPASSWORD="$pass" psql -U "$user" -d postgres -c "SELECT 1;" >/dev/null 2>&1; then
                    print_status "Found working credentials: user=$user, password=$pass"
                    DB_USER="$user"
                    DB_PASSWORD="$pass"
                    return 0
                fi
            fi
        done
    done
    
    return 1
}

# Function to execute SQL with detected credentials
execute_sql() {
    if [ -z "$DB_PASSWORD" ]; then
        docker exec -i ${CONTAINER_NAME} psql -U ${DB_USER} -d postgres -c "$1"
    else
        docker exec -i ${CONTAINER_NAME} env PGPASSWORD="${DB_PASSWORD}" psql -U ${DB_USER} -d postgres -c "$1"
    fi
}

execute_sql_db() {
    if [ -z "$DB_PASSWORD" ]; then
        docker exec -i ${CONTAINER_NAME} psql -U ${DB_USER} -d ${DB_NAME} -c "$1"
    else
        docker exec -i ${CONTAINER_NAME} env PGPASSWORD="${DB_PASSWORD}" psql -U ${DB_USER} -d ${DB_NAME} -c "$1"
    fi
}

execute_sql_script_db() {
    if [ -z "$DB_PASSWORD" ]; then
        docker exec -i ${CONTAINER_NAME} psql -U ${DB_USER} -d ${DB_NAME}
    else
        docker exec -i ${CONTAINER_NAME} env PGPASSWORD="${DB_PASSWORD}" psql -U ${DB_USER} -d ${DB_NAME}
    fi
}

# Try to connect with provided credentials, detect if needed
echo ""
echo "🔐 Testing database connection..."
if ! execute_sql "SELECT 1;" >/dev/null 2>&1; then
    print_warning "Cannot connect with provided credentials"
    
    if detect_credentials; then
        print_status "Auto-detected working credentials"
    else
        print_error "Could not detect working credentials"
        echo ""
        print_info "Please set the correct environment variables:"
        echo "export POSTGRES_USER='your_username'"
        echo "export POSTGRES_PASSWORD='your_password'"
        echo "export POSTGRES_DB='your_database'"
        echo ""
        print_info "Or check your container logs:"
        echo "docker logs ${CONTAINER_NAME}"
        exit 1
    fi
else
    print_status "Connected successfully with provided credentials"
fi

# Check if database exists, create if not
echo ""
echo "🗄️ Setting up database..."
if execute_sql "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q "1"; then
    print_status "Database '${DB_NAME}' already exists"
else
    print_info "Creating database '${DB_NAME}'..."
    execute_sql "CREATE DATABASE ${DB_NAME};"
    print_status "Database '${DB_NAME}' created"
fi

# Check if table exists, create if not
echo ""
echo "📋 Setting up hl7_fhir_mappings table..."
if execute_sql_db "\dt hl7_fhir_mappings" 2>/dev/null | grep -q "hl7_fhir_mappings"; then
    print_warning "Table 'hl7_fhir_mappings' already exists"
    read -p "🤔 Do you want to recreate it? (y/N): " recreate
    if [[ $recreate =~ ^[Yy]$ ]]; then
        execute_sql_db "DROP TABLE IF EXISTS hl7_fhir_mappings CASCADE;"
        print_info "Dropped existing table"
    else
        print_info "Keeping existing table"
        SKIP_TABLE_CREATION=true
    fi
fi

if [[ "$SKIP_TABLE_CREATION" != "true" ]]; then
    print_info "Creating hl7_fhir_mappings table..."
    
    # Create table with script
    execute_sql_script_db << 'EOF'
CREATE TABLE hl7_fhir_mappings (
    id SERIAL PRIMARY KEY,
    hl7_version VARCHAR(10),
    hl7_message_type VARCHAR(20),
    hl7_segment VARCHAR(10),
    hl7_field VARCHAR(20),
    hl7_component VARCHAR(10),
    fhir_resource VARCHAR(50),
    fhir_profile VARCHAR(100),
    fhir_path VARCHAR(200),
    transformation_rule JSONB,
    condition_expression TEXT,
    is_required BOOLEAN DEFAULT false,
    priority INTEGER DEFAULT 100,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for performance
CREATE INDEX idx_hl7_mappings_lookup 
ON hl7_fhir_mappings(hl7_message_type, hl7_segment, hl7_field);

CREATE INDEX idx_hl7_mappings_rule_gin 
ON hl7_fhir_mappings USING gin(transformation_rule);

CREATE INDEX idx_hl7_mappings_priority 
ON hl7_fhir_mappings(hl7_message_type, priority);
EOF

    print_status "Table created successfully"
fi

# Insert comprehensive sample data
echo ""
echo "📝 Inserting comprehensive HL7→FHIR mapping rules..."

execute_sql_script_db << 'EOF'
-- Clear existing data (if recreating)
DELETE FROM hl7_fhir_mappings;

-- =====================================
-- ADT^A01 → Patient Resource Mappings
-- =====================================

INSERT INTO hl7_fhir_mappings (hl7_version, hl7_message_type, hl7_segment, hl7_field, fhir_resource, fhir_profile, fhir_path, transformation_rule, is_required, priority) VALUES 
('2.5.1', 'ADT^A01', 'PID', 'PID.3', 'Patient', 'us-core', 'Patient.identifier', '{"type": "hl7_identifier", "parameters": {"use": "usual"}}', true, 10),
('2.5.1', 'ADT^A01', 'PID', 'PID.5', 'Patient', 'us-core', 'Patient.name', '{"type": "hl7_name", "parameters": {"use": "official"}}', true, 20),
('2.5.1', 'ADT^A01', 'PID', 'PID.7', 'Patient', 'us-core', 'Patient.birthDate', '{"type": "hl7_date", "format": "YYYYMMDD"}', false, 30),
('2.5.1', 'ADT^A01', 'PID', 'PID.8', 'Patient', 'us-core', 'Patient.gender', '{"type": "code_map", "map": {"M": "male", "F": "female", "O": "other", "U": "unknown"}, "default": "unknown"}', false, 40),
('2.5.1', 'ADT^A01', 'PID', 'PID.11', 'Patient', 'us-core', 'Patient.address', '{"type": "hl7_address", "parameters": {"use": "home"}}', false, 50),
('2.5.1', 'ADT^A01', 'PID', 'PID.13', 'Patient', 'us-core', 'Patient.telecom', '{"type": "hl7_telecom", "parameters": {"system": "phone", "use": "home"}}', false, 60);

-- =====================================
-- ADT^A01 → Encounter Resource Mappings
-- =====================================

INSERT INTO hl7_fhir_mappings (hl7_version, hl7_message_type, hl7_segment, hl7_field, fhir_resource, fhir_profile, fhir_path, transformation_rule, condition_expression, is_required, priority) VALUES 
('2.5.1', 'ADT^A01', 'PV1', 'PV1.2', 'Encounter', 'base', 'Encounter.class', '{"type": "code_map", "map": {"I": "IMP", "O": "AMB", "E": "EMER"}, "default": "AMB"}', 'PV1 segment exists', true, 100),
('2.5.1', 'ADT^A01', 'PV1', 'PV1.44', 'Encounter', 'base', 'Encounter.period.start', '{"type": "hl7_datetime", "format": "YYYYMMDDHHMMSS"}', 'PV1 segment exists', false, 110),
('2.5.1', 'ADT^A01', 'PV1', 'static', 'Encounter', 'base', 'Encounter.status', '{"type": "direct", "value": "finished"}', 'PV1 segment exists', true, 120);

-- =====================================
-- ORU^R01 → DiagnosticReport Mappings
-- =====================================

INSERT INTO hl7_fhir_mappings (hl7_version, hl7_message_type, hl7_segment, hl7_field, fhir_resource, fhir_profile, fhir_path, transformation_rule, is_required, priority) VALUES 
('2.5.1', 'ORU^R01', 'OBR', 'OBR.4', 'DiagnosticReport', 'base', 'DiagnosticReport.code', '{"type": "hl7_coded_element", "system_mapping": {"LN": "http://loinc.org"}}', true, 200),
('2.5.1', 'ORU^R01', 'OBR', 'OBR.7', 'DiagnosticReport', 'base', 'DiagnosticReport.effectiveDateTime', '{"type": "hl7_datetime", "format": "YYYYMMDDHHMMSS"}', false, 210),
('2.5.1', 'ORU^R01', 'OBR', 'OBR.25', 'DiagnosticReport', 'base', 'DiagnosticReport.status', '{"type": "code_map", "map": {"F": "final", "P": "preliminary", "C": "corrected"}, "default": "final"}', true, 220);

-- =====================================
-- ORU^R01 → Observation Mappings
-- =====================================

INSERT INTO hl7_fhir_mappings (hl7_version, hl7_message_type, hl7_segment, hl7_field, fhir_resource, fhir_profile, fhir_path, transformation_rule, is_required, priority) VALUES 
('2.5.1', 'ORU^R01', 'OBX', 'OBX.3', 'Observation', 'base', 'Observation.code', '{"type": "hl7_coded_element", "system_mapping": {"LN": "http://loinc.org"}}', true, 300),
('2.5.1', 'ORU^R01', 'OBX', 'OBX.5', 'Observation', 'base', 'Observation.value[x]', '{"type": "hl7_observation_value", "data_type_field": "OBX.2"}', false, 310),
('2.5.1', 'ORU^R01', 'OBX', 'OBX.11', 'Observation', 'base', 'Observation.status', '{"type": "code_map", "map": {"F": "final", "P": "preliminary"}, "default": "final"}', true, 320),
('2.5.1', 'ORU^R01', 'OBX', 'OBX.14', 'Observation', 'base', 'Observation.effectiveDateTime', '{"type": "hl7_datetime", "format": "YYYYMMDDHHMMSS"}', false, 330);
EOF

print_status "Sample mapping rules inserted successfully"

# Verify the setup
echo ""
echo "🔍 Verifying setup..."

# Count total rules
RULE_COUNT=$(execute_sql_db "SELECT COUNT(*) FROM hl7_fhir_mappings;" | grep -o '[0-9]\+' | head -1)
print_status "Total transformation rules: ${RULE_COUNT}"

# Show rules by message type
echo ""
print_info "Rules by message type:"
execute_sql_db "SELECT hl7_message_type, COUNT(*) FROM hl7_fhir_mappings GROUP BY hl7_message_type ORDER BY hl7_message_type;" | grep -E "(ADT|ORU)" | while read line; do
    echo "  $line"
done

# Generate DATABASE_URL
if [ -z "$DB_PASSWORD" ]; then
    DATABASE_URL="postgres://${DB_USER}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
else
    DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
fi

echo ""
print_status "Setup complete! 🎉"
echo ""
echo "=================================================="
echo "🔗 Connection Information:"
echo "=================================================="
echo "Container: ${CONTAINER_NAME}"
echo "Database:  ${DB_NAME}"
echo "User:      ${DB_USER}"
echo "Password:  ${DB_PASSWORD:-[empty]}"
echo "Host:      ${DB_HOST}"
echo "Port:      ${DB_PORT}"
echo ""
echo "📝 Set this environment variable:"
echo ""
echo -e "${GREEN}export DATABASE_URL=\"${DATABASE_URL}\"${NC}"
echo ""
echo "🧪 Test commands:"
echo ""
echo "# Set environment and start server:"
echo "export DATABASE_URL=\"${DATABASE_URL}\""
echo "go run main.go"
echo ""
echo "# Test transformation endpoints:"
echo "curl \"http://localhost:8080/api/fhir/transform/status\""
echo "curl \"http://localhost:8080/api/fhir/transform/rules?messageType=ADT^A01\""
echo ""

# Create environment file
cat > .env.fhir << EOF
# HL7→FHIR Transformation Database Configuration
DATABASE_URL="${DATABASE_URL}"
POSTGRES_CONTAINER="${CONTAINER_NAME}"
POSTGRES_USER="${DB_USER}"
POSTGRES_PASSWORD="${DB_PASSWORD}"
POSTGRES_DB="${DB_NAME}"
DB_HOST="${DB_HOST}"
DB_PORT="${DB_PORT}"
EOF

print_info "Created .env.fhir file with database configuration"
print_info "Source it with: source .env.fhir"

print_status "HL7→FHIR Mapping setup completed successfully! 🚀"