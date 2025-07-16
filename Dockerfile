FROM node:18-alpine

RUN apk add --no-cache \
    go \
    git \
    bash \
    curl \
    postgresql-client \
    python3 \
    make \
    g++

ENV GOPATH=/go
ENV PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /app

COPY package*.json ./
COPY go.mod go.sum ./

RUN npm install --only=production
RUN go mod download

COPY . .

# Build Go binary for production
RUN go build -o ezhealthkonnect main.go

# Create Docker-adapted version of your start.sh
RUN cat > docker-start.sh << 'SCRIPT_EOF'
#!/bin/bash
set -e

# Color codes (from your start.sh)
PURPLE='\033[0;35m'
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${PURPLE}🚀 Starting ezHealthKonnect Platform (Docker)...${NC}"
echo -e "${BLUE}═══════════════════════════════════════${NC}"

# Set variables (adapted from your start.sh)
FRONTEND_PORT=${PORT:-3000}
API_PORT=${API_PORT:-8080}

echo -e "${BLUE}📋 Configuration:${NC}"
echo -e "${BLUE}   Frontend: http://localhost:$FRONTEND_PORT${NC}"
echo -e "${BLUE}   Go API:   http://localhost:$API_PORT${NC}"

# Test database connection
echo -e "${BLUE}🔍 Testing database connection...${NC}"
if psql -h host.docker.internal -p 5432 -U ezhealth_user -d ezhealthkonnect -c "SELECT 1;" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Database connection successful${NC}"
else
    echo -e "${YELLOW}⚠️ Database connection failed, but continuing...${NC}"
fi

# Cleanup function (from your start.sh)
cleanup() {
    echo -e "\n${YELLOW}🧹 Shutting down services...${NC}"
    if [ ! -z "$API_PID" ]; then
        kill $API_PID 2>/dev/null
        echo -e "${BLUE}🔹 Go API server stopped${NC}"
    fi
    echo -e "${GREEN}👋 ezHealthKonnect shutdown complete${NC}"
    exit 0
}

trap cleanup SIGINT SIGTERM

# Start Go API (adapted from your start.sh logic)
echo -e "\n${PURPLE}🔧 Starting Go API Server...${NC}"
echo -e "${BLUE}   Port: $API_PORT${NC}"

# Use built binary instead of go run (production mode)
./ezhealthkonnect &
API_PID=$!
echo -e "${YELLOW}   PID: $API_PID${NC}"

# Wait for Go API
sleep 5

# Check if Go API started
if curl -s "http://localhost:$API_PORT/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Go API is ready!${NC}"
else
    echo -e "${YELLOW}⚠️ Go API may still be starting...${NC}"
fi

# Start Frontend (adapted from your start.sh logic)
echo -e "\n${PURPLE}🎨 Starting Frontend Server...${NC}"
echo -e "${BLUE}   Port: $FRONTEND_PORT${NC}"

# In production, use server.js directly instead of npm start
echo -e "${BLUE}   Using production server (server.js)${NC}"

# Start Node.js in foreground (keeps container alive)
echo -e "${GREEN}🎉 ezHealthKonnect Platform Started Successfully!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo -e "${GREEN}🌐 Frontend:  http://localhost:$FRONTEND_PORT${NC}"
echo -e "${GREEN}🔧 Go API:    http://localhost:$API_PORT${NC}"
echo -e "${GREEN}📊 Health:    http://localhost:$API_PORT/health${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"

# Start server.js in foreground
exec node server.js
SCRIPT_EOF

RUN chmod +x docker-start.sh

EXPOSE 3000 8080

CMD ["./docker-start.sh"]
