#!/bin/bash

# Database development tools for ezHealthKonnect

set -e

CONTAINER_NAME="ezhealthkonnect-postgres"
DB_NAME="ezhealthkonnect"
DB_USER="ezhealth_user"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

show_help() {
    echo "Database Tools for ezHealthKonnect"
    echo "Usage: ./scripts/db-tools.sh [command]"
    echo ""
    echo "Commands:"
    echo "  migrate     - Run new migrations"
    echo "  rollback    - Rollback last migration (if supported)"
    echo "  status      - Show migration status"
    echo "  backup      - Create database backup"
    echo "  restore     - Restore from backup"
    echo "  reset       - Reset database (DANGER: loses all data)"
    echo "  connect     - Connect to database via psql"
    echo "  logs        - Show database logs"
    echo "  new-migration <name> - Create new migration file"
    echo ""
}

ensure_container_running() {
    if ! docker ps | grep -q $CONTAINER_NAME; then
        echo -e "${YELLOW}Starting database container...${NC}"
        docker-compose up -d postgres
        sleep 5
    fi
}

run_migrations() {
    echo -e "${BLUE}Running database migrations...${NC}"
    docker-compose up flyway
    echo -e "${GREEN}Migrations completed${NC}"
}

show_migration_status() {
    ensure_container_running
    echo -e "${BLUE}Migration status:${NC}"
    docker-compose exec postgres psql -U $DB_USER -d $DB_NAME -c "
        SELECT filename, executed_at 
        FROM flyway_schema_history 
        ORDER BY executed_at DESC 
        LIMIT 10;
    " 2>/dev/null || echo "No migration history found"
}

backup_database() {
    ensure_container_running
    
    mkdir -p database/backups
    BACKUP_FILE="database/backups/backup_$(date +%Y%m%d_%H%M%S).sql"
    
    echo -e "${BLUE}Creating backup: $BACKUP_FILE${NC}"
    docker-compose exec -T postgres pg_dump -U $DB_USER $DB_NAME > $BACKUP_FILE
    echo -e "${GREEN}Backup created: $BACKUP_FILE${NC}"
}

restore_database() {
    if [ -z "$1" ]; then
        echo -e "${RED}Usage: ./db-tools.sh restore <backup-file.sql>${NC}"
        echo -e "${YELLOW}Available backups:${NC}"
        ls -la database/backups/*.sql 2>/dev/null || echo "No backups found"
        exit 1
    fi
    
    if [ ! -f "$1" ]; then
        echo -e "${RED}Backup file not found: $1${NC}"
        exit 1
    fi
    
    ensure_container_running
    
    echo -e "${YELLOW}WARNING: This will replace all data in the database!${NC}"
    echo -e "${YELLOW}Press Enter to continue, or Ctrl+C to cancel...${NC}"
    read
    
    echo -e "${BLUE}Restoring from: $1${NC}"
    docker-compose exec -T postgres psql -U $DB_USER $DB_NAME < $1
    echo -e "${GREEN}Database restored from: $1${NC}"
}

reset_database() {
    echo -e "${RED}WARNING: This will DELETE ALL DATA in the database!${NC}"
    echo -e "${YELLOW}Type 'RESET' to confirm: ${NC}"
    read confirmation
    
    if [ "$confirmation" != "RESET" ]; then
        echo -e "${GREEN}Operation cancelled${NC}"
        exit 0
    fi
    
    # Create backup before reset
    backup_database
    
    echo -e "${BLUE}Resetting database...${NC}"
    docker-compose down
    docker volume rm ezhealthkonnect_postgres_data 2>/dev/null || true
    docker-compose up -d postgres
    sleep 10
    run_migrations
    echo -e "${GREEN}Database reset complete${NC}"
}

connect_to_db() {
    ensure_container_running
    echo -e "${BLUE}Connecting to database...${NC}"
    docker-compose exec postgres psql -U $DB_USER $DB_NAME
}

show_logs() {
    echo -e "${BLUE}Database logs:${NC}"
    docker-compose logs postgres --tail=50
}

create_migration() {
    if [ -z "$1" ]; then
        echo -e "${RED}Usage: ./db-tools.sh new-migration <migration_name>${NC}"
        exit 1
    fi
    
    mkdir -p database/migrations
    
    # Find next version number
    LAST_VERSION=$(ls database/migrations/V*.sql 2>/dev/null | sed 's/.*V\([0-9]*\)__.*/\1/' | sort -n | tail -1)
    if [ -z "$LAST_VERSION" ]; then
        NEXT_VERSION=1
    else
        NEXT_VERSION=$((LAST_VERSION + 1))
    fi
    
    MIGRATION_FILE="database/migrations/V${NEXT_VERSION}__$1.sql"
    
    cat > "$MIGRATION_FILE" << EOF
-- V${NEXT_VERSION}__$1.sql
-- Description: $1

-- Add your SQL here
-- Example:
-- ALTER TABLE users ADD COLUMN new_field VARCHAR(255);

-- Remember:
-- - Use transactions for complex changes
-- - Add appropriate indexes
-- - Consider data migration if needed
EOF
    
    echo -e "${GREEN}Created migration: $MIGRATION_FILE${NC}"
    echo -e "${YELLOW}Edit the file and run: ./db-tools.sh migrate${NC}"
}

# Main command handling
case "${1:-help}" in
    migrate)
        run_migrations
        ;;
    status)
        show_migration_status
        ;;
    backup)
        backup_database
        ;;
    restore)
        restore_database "$2"
        ;;
    reset)
        reset_database
        ;;
    connect)
        connect_to_db
        ;;
    logs)
        show_logs
        ;;
    new-migration)
        create_migration "$2"
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        show_help
        exit 1
        ;;
esac