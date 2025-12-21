#!/bin/bash

# =============================================================================
# ezHealthKonnect Complete Startup Script
# Starts Frontend (React) + Go API Server
# =============================================================================

# Define color codes for output
PURPLE='\033[0;35m'
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to safely load environment variables
load_env_vars() {
    if [ -f .env ]; then
        echo -e "${BLUE}📋 Loading environment variables from .env...${NC}"
        # Use safer environment loading
        set -a && source .env && set +a
        echo -e "${GREEN}✅ Environment variables loaded${NC}"
    else
        echo -e "${YELLOW}⚠️  No .env file found, using defaults${NC}"
    fi
}

# Function to check if a port is available
check_port() {
    local port=$1
    local service_name=$2
    
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${RED}❌ Port $port is already in use (needed for $service_name)${NC}"
        echo -e "${YELLOW}💡 Kill the process or change the port in .env${NC}"
        return 1
    else
        echo -e "${GREEN}✅ Port $port is available for $service_name${NC}"
        return 0
    fi
}

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local service_name=$2
    local timeout=${3:-30}
    
    echo -e "${YELLOW}⏳ Waiting for $service_name to be ready...${NC}"
    
    for i in $(seq 1 $timeout); do
        if curl -s "$url" > /dev/null 2>&1; then
            echo -e "${GREEN}✅ $service_name is ready!${NC}"
            return 0
        fi
        echo -n "."
        sleep 1
    done
    
    echo -e "\n${RED}❌ $service_name failed to start within $timeout seconds${NC}"
    return 1
}

# Function to cleanup processes on exit
cleanup() {
    echo -e "\n${YELLOW}🧹 Shutting down services...${NC}"
    
    # Kill background processes
    if [ ! -z "$FRONTEND_PID" ]; then
        kill $FRONTEND_PID 2>/dev/null
        echo -e "${BLUE}🔹 Frontend server stopped${NC}"
    fi
    
    if [ ! -z "$API_PID" ]; then
        kill $API_PID 2>/dev/null
        echo -e "${BLUE}🔹 Go API server stopped${NC}"
    fi
    
    echo -e "${GREEN}👋 ezHealthKonnect shutdown complete${NC}"
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM

# =============================================================================
# MAIN STARTUP SEQUENCE
# =============================================================================

echo -e "${PURPLE}🚀 Starting ezHealthKonnect Platform...${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

# Load environment variables
load_env_vars

# Set default values (only for variables not in .env)
FRONTEND_PORT=${PORT:-3000}
API_PORT=${API_PORT:-8080}
FRONTEND_DIR=${FRONTEND_DIR:-public}
GO_MAIN_PATH=${GO_MAIN_PATH:-main.go}
AUTO_OPEN_BROWSER=${AUTO_OPEN_BROWSER:-true}
SERVICE_START_DELAY=${SERVICE_START_DELAY:-3}

echo -e "${BLUE}📋 Configuration:${NC}"
echo -e "${BLUE}   Frontend: http://localhost:$FRONTEND_PORT${NC}"
echo -e "${BLUE}   Go API:   http://localhost:$API_PORT${NC}"
echo -e "${BLUE}   Frontend Dir: $FRONTEND_DIR${NC}"
echo -e "${BLUE}   Go Main: $GO_MAIN_PATH${NC}"

# Check if required files exist
if [ ! -f "$GO_MAIN_PATH" ]; then
    echo -e "${RED}❌ Go main file not found: $GO_MAIN_PATH${NC}"
    echo -e "${YELLOW}💡 Run the setup script first or check GO_MAIN_PATH in .env${NC}"
    exit 1
fi

if [ ! -d "$FRONTEND_DIR" ]; then
    echo -e "${YELLOW}⚠️  Frontend directory not found: $FRONTEND_DIR${NC}"
    echo -e "${YELLOW}💡 Creating basic frontend directory...${NC}"
    mkdir -p "$FRONTEND_DIR"
    echo "<h1>ezHealthKonnect Frontend</h1><p>Replace this with your React app</p>" > "$FRONTEND_DIR/index.html"
fi

# Check port availability
echo -e "\n${BLUE}🔍 Checking port availability...${NC}"
check_port $FRONTEND_PORT "Frontend" || exit 1
check_port $API_PORT "Go API" || exit 1

# =============================================================================
# START GO API SERVER
# =============================================================================

echo -e "\n${PURPLE}🔧 Starting Go API Server...${NC}"
echo -e "${BLUE}   Port: $API_PORT${NC}"
echo -e "${BLUE}   File: $GO_MAIN_PATH${NC}"

# Set environment variables for Go app
export API_PORT
export PORT=$FRONTEND_PORT

# Start Go API server in background
go run "$GO_MAIN_PATH" &
API_PID=$!

echo -e "${YELLOW}   PID: $API_PID${NC}"

# Wait for Go API to be ready
if ! wait_for_service "http://localhost:$API_PORT/health" "Go API Server" 30; then
    echo -e "${RED}❌ Failed to start Go API Server${NC}"
    cleanup
fi

# =============================================================================
# START FRONTEND SERVER
# =============================================================================

echo -e "\n${PURPLE}🎨 Starting Frontend Server...${NC}"
echo -e "${BLUE}   Port: $FRONTEND_PORT${NC}"
echo -e "${BLUE}   Directory: $FRONTEND_DIR${NC}"

# Check if we have a package.json (React app)
if [ -f "package.json" ]; then
    echo -e "${BLUE}   Detected React app - using npm/yarn${NC}"
    
    # Install dependencies if node_modules doesn't exist
    if [ ! -d "node_modules" ]; then
        echo -e "${YELLOW}📦 Installing dependencies...${NC}"
        if command -v yarn >/dev/null 2>&1; then
            yarn install
        else
            npm install
        fi
    fi
    
    # Start React development server
    if command -v yarn >/dev/null 2>&1; then
        BROWSER=none PORT=$FRONTEND_PORT yarn start &
    else
        BROWSER=none PORT=$FRONTEND_PORT npm start &
    fi
    FRONTEND_PID=$!
    
elif command -v live-server >/dev/null 2>&1; then
    echo -e "${BLUE}   Using live-server for static files${NC}"
    cd "$FRONTEND_DIR"
    live-server --port=$FRONTEND_PORT --host=localhost --cors --quiet &
    FRONTEND_PID=$!
    cd ..
    
elif command -v python3 >/dev/null 2>&1; then
    echo -e "${BLUE}   Using Python HTTP server${NC}"
    cd "$FRONTEND_DIR"
    python3 -m http.server $FRONTEND_PORT &
    FRONTEND_PID=$!
    cd ..
    
elif command -v python >/dev/null 2>&1; then
    echo -e "${BLUE}   Using Python 2 HTTP server${NC}"
    cd "$FRONTEND_DIR"
    python -m SimpleHTTPServer $FRONTEND_PORT &
    FRONTEND_PID=$!
    cd ..
    
else
    echo -e "${RED}❌ No suitable web server found${NC}"
    echo -e "${YELLOW}💡 Install live-server: npm install -g live-server${NC}"
    cleanup
fi

echo -e "${YELLOW}   PID: $FRONTEND_PID${NC}"

# Wait a moment for frontend to start
sleep $SERVICE_START_DELAY

# Wait for frontend to be ready
if ! wait_for_service "http://localhost:$FRONTEND_PORT" "Frontend Server" 15; then
    echo -e "${YELLOW}⚠️  Frontend may still be starting up...${NC}"
fi

# =============================================================================
# STARTUP COMPLETE
# =============================================================================

echo -e "\n${GREEN}🎉 ezHealthKonnect Platform Started Successfully!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo -e "${GREEN}🌐 Frontend:  http://localhost:$FRONTEND_PORT${NC}"
echo -e "${GREEN}🔧 Go API:    http://localhost:$API_PORT${NC}"
echo -e "${GREEN}📊 Health:    http://localhost:$API_PORT/health${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"

# Open browser if requested
if [ "$AUTO_OPEN_BROWSER" = "true" ]; then
    echo -e "${BLUE}🌐 Opening browser...${NC}"
    sleep 2
    
    # Try different browser opening commands
    if command -v xdg-open >/dev/null 2>&1; then
        xdg-open "http://localhost:$FRONTEND_PORT"
    elif command -v open >/dev/null 2>&1; then
        open "http://localhost:$FRONTEND_PORT"
    elif command -v start >/dev/null 2>&1; then
        start "http://localhost:$FRONTEND_PORT"
    else
        echo -e "${YELLOW}💡 Manually open: http://localhost:$FRONTEND_PORT${NC}"
    fi
fi

echo -e "\n${BLUE}💡 Development Tips:${NC}"
echo -e "${BLUE}   • Press Ctrl+C to stop all services${NC}"
echo -e "${BLUE}   • Frontend changes auto-reload${NC}"
echo -e "${BLUE}   • Restart Go API after code changes${NC}"
echo -e "${BLUE}   • Check logs above for any errors${NC}"

echo -e "\n${YELLOW}⏳ Services running... Press Ctrl+C to stop${NC}"

# Keep script running and wait for user to stop
wait