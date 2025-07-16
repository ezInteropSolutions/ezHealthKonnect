#!/bin/bash
# build-docker.sh
# Automated Docker setup and testing for ezHealthKonnect

PURPLE='\033[0;35m'
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${PURPLE}🐳 ezHealthKonnect Docker Setup${NC}"
echo -e "${PURPLE}═══════════════════════════════${NC}"

# Function to check if Docker is installed
check_docker() {
    if command -v docker &> /dev/null && command -v docker-compose &> /dev/null; then
        echo -e "${GREEN}✅ Docker and Docker Compose are installed${NC}"
        return 0
    else
        echo -e "${RED}❌ Docker or Docker Compose not found${NC}"
        echo -e "${YELLOW}💡 Please install Docker Desktop from: https://www.docker.com/products/docker-desktop${NC}"
        return 1
    fi
}

# Function to create .dockerignore if it doesn't exist
create_dockerignore() {
    if [ ! -f ".dockerignore" ]; then
        echo -e "${YELLOW}📝 Creating .dockerignore...${NC}"
        cat > .dockerignore << 'EOF'
node_modules
.git
.env
*.log
logs/
uploads/
.automation/
dist/
.github/
README.md
CHANGELOG.md
docs/
.dockerignore
docker-entrypoint.sh
EOF
        echo -e "${GREEN}✅ .dockerignore created${NC}"
    else
        echo -e "${GREEN}✅ .dockerignore already exists${NC}"
    fi
}

# Function to create database directory
create_database_dir() {
    if [ ! -d "database" ]; then
        echo -e "${YELLOW}📁 Creating database directory...${NC}"
        mkdir -p database
        
        # Create a simple initialization script
        cat > database/001_initial_setup.sql << 'EOF'
-- Initial setup for ezHealthKonnect
-- This file runs automatically on first Docker startup

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create a simple health check table
CREATE TABLE IF NOT EXISTS system_health (
    id SERIAL PRIMARY KEY,
    component VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert initial health check
INSERT INTO system_health (component, status) 
VALUES ('database', 'healthy') 
ON CONFLICT DO NOTHING;

-- Log that setup completed
SELECT 'Database initialization completed' AS setup_status;
EOF
        echo -e "${GREEN}✅ Database directory created with initial setup${NC}"
    else
        echo -e "${GREEN}✅ Database directory already exists${NC}"
    fi
}

# Function to build and test Docker setup
build_and_test() {
    echo -e "${BLUE}🔨 Building Docker containers...${NC}"
    
    # Stop any existing containers
    docker-compose down 2>/dev/null || true
    
    # Build the containers
    if docker-compose build; then
        echo -e "${GREEN}✅ Docker build successful${NC}"
    else
        echo -e "${RED}❌ Docker build failed${NC}"
        return 1
    fi
    
    echo -e "${BLUE}🚀 Starting containers...${NC}"
    
    # Start in background
    if docker-compose up -d; then
        echo -e "${GREEN}✅ Containers started${NC}"
    else
        echo -e "${RED}❌ Failed to start containers${NC}"
        return 1
    fi
    
    # Wait for services to be ready
    echo -e "${YELLOW}⏳ Waiting for services to be ready...${NC}"
    sleep 10
    
    # Test the services
    test_services
}

# Function to test the running services
test_services() {
    echo -e "${BLUE}🧪 Testing services...${NC}"
    
    local tests_passed=0
    local total_tests=4
    
    # Test 1: Check if containers are running
    if docker-compose ps | grep -q "Up"; then
        echo -e "${GREEN}✅ Test 1/4: Containers are running${NC}"
        tests_passed=$((tests_passed + 1))
    else
        echo -e "${RED}❌ Test 1/4: Containers not running${NC}"
    fi
    
    # Test 2: Check Go API health
    if curl -s http://localhost:8080/health | grep -q "healthy"; then
        echo -e "${GREEN}✅ Test 2/4: Go API is responding${NC}"
        tests_passed=$((tests_passed + 1))
    else
        echo -e "${RED}❌ Test 2/4: Go API not responding${NC}"
    fi
    
    # Test 3: Check Node.js frontend
    if curl -s http://localhost:3000/api/status | grep -q "running"; then
        echo -e "${GREEN}✅ Test 3/4: Node.js frontend is responding${NC}"
        tests_passed=$((tests_passed + 1))
    else
        echo -e "${RED}❌ Test 3/4: Node.js frontend not responding${NC}"
    fi
    
    # Test 4: Check database connection
    if docker-compose exec -T postgres pg_isready -U ezhealth_user -d ezhealthkonnect; then
        echo -e "${GREEN}✅ Test 4/4: PostgreSQL is ready${NC}"
        tests_passed=$((tests_passed + 1))
    else
        echo -e "${RED}❌ Test 4/4: PostgreSQL not ready${NC}"
    fi
    
    echo -e "\n${BLUE}📊 Test Results: ${tests_passed}/${total_tests} passed${NC}"
    
    if [ $tests_passed -eq $total_tests ]; then
        echo -e "${GREEN}🎉 All tests passed! Docker setup is working correctly.${NC}"
        show_usage_info
        return 0
    else
        echo -e "${RED}❌ Some tests failed. Check the logs for details.${NC}"
        echo -e "${YELLOW}💡 Run 'docker-compose logs' to see what went wrong${NC}"
        return 1
    fi
}

# Function to show usage information
show_usage_info() {
    echo -e "\n${PURPLE}🎯 ezHealthKonnect is now running in Docker!${NC}"
    echo -e "${BLUE}════════════════════════════════════════════${NC}"
    echo -e "${GREEN}📱 Frontend: http://localhost:3000${NC}"
    echo -e "${GREEN}🔧 Go API:   http://localhost:8080${NC}"
    echo -e "${GREEN}🗄️ Database: localhost:5432${NC}"
    echo -e "\n${BLUE}🛠️ Useful Commands:${NC}"
    echo -e "${BLUE}   View logs:        docker-compose logs${NC}"
    echo -e "${BLUE}   Stop services:    docker-compose down${NC}"
    echo -e "${BLUE}   Restart:          docker-compose restart${NC}"
    echo -e "${BLUE}   Shell access:     docker-compose exec app bash${NC}"
    echo -e "${BLUE}   Fresh start:      docker-compose down -v && docker-compose up -d${NC}"
}

# Function to show help
show_help() {
    echo -e "${BLUE}Usage: $0 [OPTION]${NC}"
    echo -e "${BLUE}Options:${NC}"
    echo -e "${BLUE}  build     Build and start Docker containers${NC}"
    echo -e "${BLUE}  test      Test running containers${NC}"
    echo -e "${BLUE}  stop      Stop all containers${NC}"
    echo -e "${BLUE}  clean     Stop containers and remove volumes${NC}"
    echo -e "${BLUE}  logs      Show container logs${NC}"
    echo -e "${BLUE}  shell     Open shell in app container${NC}"
    echo -e "${BLUE}  help      Show this help message${NC}"
}

# Main execution
case "${1:-build}" in
    build)
        check_docker || exit 1
        create_dockerignore
        create_database_dir
        build_and_test
        ;;
    
    test)
        echo -e "${BLUE}🧪 Testing running containers...${NC}"
        test_services
        ;;
    
    stop)
        echo -e "${YELLOW}🛑 Stopping containers...${NC}"
        docker-compose down
        echo -e "${GREEN}✅ Containers stopped${NC}"
        ;;
    
    clean)
        echo -e "${YELLOW}🧹 Cleaning up containers and data...${NC}"
        docker-compose down -v
        docker system prune -f
        echo -e "${GREEN}✅ Cleanup complete${NC}"
        ;;
    
    logs)
        echo -e "${BLUE}📋 Showing container logs...${NC}"
        docker-compose logs --tail=50 -f
        ;;
    
    shell)
        echo -e "${BLUE}🐚 Opening shell in app container...${NC}"
        docker-compose exec app bash
        ;;
    
    help|--help|-h)
        show_help
        ;;
    
    *)
        echo -e "${RED}❌ Unknown option: $1${NC}"
        show_help
        exit 1
        ;;
esac