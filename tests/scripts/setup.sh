#!/bin/bash

# =============================================================================
# ezHealthKonnect Complete Setup Script
# Sets up Go API Server, Node.js dependencies, and project structure
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
        set -a && source .env && set +a
        echo -e "${GREEN}✅ Environment variables loaded${NC}"
    else
        echo -e "${YELLOW}⚠️  No .env file found, using defaults${NC}"
    fi
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# =============================================================================
# MAIN SETUP SEQUENCE
# =============================================================================

echo -e "${PURPLE}🔧 ezHealthKonnect Complete Setup${NC}"
echo -e "${PURPLE}═══════════════════════════════════${NC}"

# Load environment variables
load_env_vars

# Set default values (only for variables not in .env)
API_PORT=${API_PORT:-8080}
GO_MAIN_PATH=${GO_MAIN_PATH:-main.go}
FRONTEND_PORT=${PORT:-3000}  # Use PORT instead of FRONTEND_PORT
HL7_PARSER_DIR=${HL7_PARSER_DIR:-hl7}

echo -e "\n${BLUE}📋 Setup Configuration:${NC}"
echo -e "${BLUE}   Go API Port: $API_PORT${NC}"
echo -e "${BLUE}   Frontend Port: $FRONTEND_PORT${NC}"
echo -e "${BLUE}   Go Main File: $GO_MAIN_PATH${NC}"
echo -e "${BLUE}   HL7 Parser Dir: $HL7_PARSER_DIR${NC}"

# =============================================================================
# PREREQUISITES CHECK
# =============================================================================

echo -e "\n${PURPLE}🔍 Checking Prerequisites...${NC}"

# Check Go installation
if command_exists go; then
    GO_VERSION=$(go version | cut -d' ' -f3)
    echo -e "${GREEN}✅ Go installed: $GO_VERSION${NC}"
else
    echo -e "${RED}❌ Go not found. Please install Go 1.21+ from https://golang.org/dl/${NC}"
    exit 1
fi

# Check Node.js installation
if command_exists node; then
    NODE_VERSION=$(node --version)
    echo -e "${GREEN}✅ Node.js installed: $NODE_VERSION${NC}"
else
    echo -e "${RED}❌ Node.js not found. Please install Node.js 18+ from https://nodejs.org/${NC}"
    exit 1
fi

# Check npm installation
if command_exists npm; then
    NPM_VERSION=$(npm --version)
    echo -e "${GREEN}✅ npm installed: v$NPM_VERSION${NC}"
else
    echo -e "${RED}❌ npm not found. Please install npm${NC}"
    exit 1
fi

# =============================================================================
# GO MODULE SETUP
# =============================================================================

echo -e "\n${PURPLE}⚡ Setting up Go API Server...${NC}"

# Check if Go module already exists
if [ -f "go.mod" ]; then
    echo -e "${GREEN}✅ Found existing go.mod - preserving your configuration${NC}"
    
    # Get the existing module name
    EXISTING_MODULE=$(grep "^module " go.mod | cut -d' ' -f2)
    echo -e "${BLUE}   Module: $EXISTING_MODULE${NC}"
    
    # Check if required dependencies are present
    echo -e "${YELLOW}📦 Checking for required Go dependencies...${NC}"
    
    NEEDS_GIN=false
    NEEDS_CORS=false
    
    if ! grep -q "github.com/gin-gonic/gin" go.mod; then
        NEEDS_GIN=true
        echo -e "${YELLOW}   Adding: github.com/gin-gonic/gin${NC}"
    fi
    
    if ! grep -q "github.com/gin-contrib/cors" go.mod; then
        NEEDS_CORS=true
        echo -e "${YELLOW}   Adding: github.com/gin-contrib/cors${NC}"
    fi
    
    # Add missing dependencies without overwriting
    if [ "$NEEDS_GIN" = true ] || [ "$NEEDS_CORS" = true ]; then
        echo -e "${YELLOW}📦 Adding missing Go dependencies...${NC}"
        
        if [ "$NEEDS_GIN" = true ]; then
            go get github.com/gin-gonic/gin@latest
        fi
        
        if [ "$NEEDS_CORS" = true ]; then
            go get github.com/gin-contrib/cors@latest
        fi
        
        go mod tidy
    else
        echo -e "${GREEN}✅ All required Go dependencies already present${NC}"
        # Still run go mod tidy to clean up
        go mod tidy
    fi
    
else
    # No go.mod exists, create new one
    echo -e "${YELLOW}📦 Initializing new Go module...${NC}"
    
    # Use module name from environment (required)
    if [ -z "$GO_MODULE_NAME" ]; then
        echo -e "${RED}❌ GO_MODULE_NAME not set in .env file${NC}"
        echo -e "${YELLOW}💡 Add GO_MODULE_NAME=github.com/yourusername/ezhealthkonnect to .env${NC}"
        exit 1
    fi
    
    go mod init "$GO_MODULE_NAME"
    
    echo -e "${YELLOW}📦 Installing required Go dependencies...${NC}"
    go get github.com/gin-gonic/gin@latest
    go get github.com/gin-contrib/cors@latest
    go mod tidy
fi

# Create basic main.go ONLY if it doesn't exist
if [ ! -f "$GO_MAIN_PATH" ]; then
    echo -e "${YELLOW}📝 Creating basic $GO_MAIN_PATH...${NC}"
    echo -e "${BLUE}   This is a placeholder - replace with your actual parser integration${NC}"
    
    # Get the actual module name from go.mod
    ACTUAL_MODULE=$(grep "^module " go.mod | cut -d' ' -f2)
    
    cat > "$GO_MAIN_PATH" << EOF
package main

import (
	"log"
	"net/http"
	"time"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	
	// TODO: Uncomment and update the import path for your HL7 parser
	// "${ACTUAL_MODULE}/hl7"
)

func main() {
	// Get port from environment or default to 8080
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	r := gin.Default()

	// Configure CORS - Use PORT instead of FRONTEND_PORT
	frontendPort := os.Getenv("PORT")
	if frontendPort == "" {
		frontendPort = "3000"
	}
	
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{
		"http://localhost:3000", 
		"http://127.0.0.1:3000",
		"http://localhost:" + frontendPort,
	}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"}
	config.AllowCredentials = true
	r.Use(cors.New(config))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"service":   "ezHealthKonnect-go-api",
			"version":   "1.0.0",
			"port":      port,
			"module":    "${ACTUAL_MODULE}",
			"frontend":  "http://localhost:" + frontendPort,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// HL7 Dictionary endpoints (integrated into API)
	r.GET("/api/hl7/dictionary/segments", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "HL7 segments dictionary - TODO: implement",
			"data":    []string{"MSH", "PID", "PV1", "OBX", "AL1"},
		})
	})

	r.GET("/api/hl7/dictionary/fields/:segment", func(c *gin.Context) {
		segment := c.Param("segment")
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"segment": segment,
			"message": "HL7 fields for " + segment + " - TODO: implement",
		})
	})

	// HL7 Processing endpoints
	r.POST("/api/hl7/parse", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "HL7 parsing endpoint - TODO: integrate your enhanced parser from ${HL7_PARSER_DIR}/",
			"note":    "Update this endpoint to use: hl7.ParseHL7MessageEnhanced()",
		})
	})

	// Interface management endpoints
	r.POST("/api/interfaces", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Interface creation endpoint - TODO: implement interface logic",
		})
	})

	r.GET("/api/interfaces", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "List interfaces - TODO: implement",
			"data":    []interface{}{},
		})
	})

	log.Printf("🚀 ezHealthKonnect Go API Server starting on port %s", port)
	log.Printf("📦 Module: %s", "${ACTUAL_MODULE}")
	log.Printf("🌐 Frontend: http://localhost:%s", frontendPort)
	log.Printf("🔧 TODO: Integrate your HL7 parsers from ${HL7_PARSER_DIR}/ directory")
	r.Run(":" + port)
}
EOF

    echo -e "${GREEN}✅ Created placeholder main.go with HL7 dictionary endpoints${NC}"
    echo -e "${YELLOW}⚠️  IMPORTANT: You need to integrate your actual HL7 parser!${NC}"
    echo -e "${BLUE}   1. Uncomment the hl7 import in main.go${NC}"
    echo -e "${BLUE}   2. Replace placeholder endpoints with your actual parser calls${NC}"
    echo -e "${BLUE}   3. Update the import path to match your module name${NC}"

else
    echo -e "${GREEN}✅ Found existing $GO_MAIN_PATH - keeping your implementation${NC}"
fi

# =============================================================================
# NODE.JS DEPENDENCIES SETUP
# =============================================================================

echo -e "\n${PURPLE}📦 Setting up Node.js Dependencies...${NC}"

# Check if package.json exists
if [ -f "package.json" ]; then
    echo -e "${GREEN}✅ Found existing package.json${NC}"
    
    # Show current package.json info
    if command_exists jq; then
        PACKAGE_NAME=$(jq -r '.name // "unknown"' package.json)
        PACKAGE_VERSION=$(jq -r '.version // "unknown"' package.json)
        echo -e "${BLUE}   Package: $PACKAGE_NAME v$PACKAGE_VERSION${NC}"
    fi
    
else
    echo -e "${YELLOW}📝 Creating package.json...${NC}"
    cat > package.json << EOF
{
  "name": "ezhealthkonnect",
  "version": "1.0.0",
  "description": "AI-Powered Healthcare Integration Platform",
  "main": "server.js",
  "scripts": {
    "start": "node server.js",
    "dev": "nodemon server.js",
    "test": "echo \\"Error: no test specified\\" && exit 1"
  },
  "dependencies": {
    "express": "^4.18.2",
    "bull": "^4.12.2",
    "redis": "^4.6.5",
    "ioredis": "^5.3.2",
    "cors": "^2.8.5",
    "dotenv": "^16.0.3",
    "helmet": "^6.1.5",
    "morgan": "^1.10.0",
    "bcryptjs": "^2.4.3",
    "jsonwebtoken": "^9.0.2",
    "multer": "^1.4.5",
    "winston": "^3.8.2",
    "compression": "^1.7.4",
    "rate-limiter-flexible": "^2.4.1"
  },
  "devDependencies": {
    "nodemon": "^2.0.22"
  },
  "author": "ezHealthKonnect Team",
  "license": "MIT"
}
EOF
    echo -e "${GREEN}✅ Created package.json with ezHealthKonnect dependencies${NC}"
fi

# Install Node.js dependencies
echo -e "${YELLOW}📦 Installing Node.js dependencies...${NC}"

if [ ! -d "node_modules" ]; then
    echo -e "${BLUE}   Installing all dependencies...${NC}"
    if command_exists yarn; then
        echo -e "${BLUE}   Using yarn...${NC}"
        yarn install
    else
        echo -e "${BLUE}   Using npm...${NC}"
        npm install
    fi
else
    echo -e "${BLUE}   node_modules exists, checking for missing packages...${NC}"
    
    # List of critical packages for ezHealthKonnect
    CRITICAL_PACKAGES=("bull" "redis" "express" "cors" "dotenv")
    MISSING_PACKAGES=()
    
    for package in "${CRITICAL_PACKAGES[@]}"; do
        if [ ! -d "node_modules/$package" ]; then
            MISSING_PACKAGES+=("$package")
        fi
    done
    
    if [ ${#MISSING_PACKAGES[@]} -gt 0 ]; then
        echo -e "${YELLOW}   Missing critical packages: ${MISSING_PACKAGES[*]}${NC}"
        echo -e "${YELLOW}   Installing missing packages...${NC}"
        
        if command_exists yarn; then
            yarn add "${MISSING_PACKAGES[@]}"
        else
            npm install "${MISSING_PACKAGES[@]}"
        fi
    else
        echo -e "${GREEN}   ✅ All critical packages present${NC}"
    fi
fi

# =============================================================================
# PROJECT STRUCTURE SETUP
# =============================================================================

echo -e "\n${PURPLE}📁 Setting up Project Structure...${NC}"

# Create essential directories
DIRECTORIES=("logs" "uploads" "public" "routes" "middleware" "models" "config")

for dir in "${DIRECTORIES[@]}"; do
    if [ ! -d "$dir" ]; then
        mkdir -p "$dir"
        echo -e "${BLUE}   Created: $dir/${NC}"
    else
        echo -e "${GREEN}   ✅ Exists: $dir/${NC}"
    fi
done

# Create basic frontend if public directory is empty
if [ ! -f "public/index.html" ]; then
    echo -e "${YELLOW}📝 Creating basic frontend placeholder...${NC}"
    cat > public/index.html << EOF
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>ezHealthKonnect</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #2c3e50; }
        .status { padding: 10px; border-radius: 5px; margin: 10px 0; }
        .success { background: #d4edda; color: #155724; border: 1px solid #c3e6cb; }
        .api-link { color: #007bff; text-decoration: none; }
        .api-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 ezHealthKonnect Platform</h1>
        <div class="status success">
            ✅ Frontend server is running successfully!
        </div>
        <p>Welcome to the ezHealthKonnect AI-Powered Healthcare Integration Platform.</p>
        <h3>🔗 Quick Links:</h3>
        <ul>
            <li><a href="http://localhost:${API_PORT}/health" class="api-link" target="_blank">Go API Health Check</a></li>
            <li><a href="http://localhost:${API_PORT}/api/hl7/dictionary/segments" class="api-link" target="_blank">HL7 Dictionary</a></li>
        </ul>
        <h3>📋 Next Steps:</h3>
        <ol>
            <li>Replace this placeholder with your React application</li>
            <li>Integrate your HL7 parser into the Go API</li>
            <li>Build the Visual Logic Builder interface</li>
            <li>Add AI-powered workflow generation</li>
        </ol>
        <p><strong>Status:</strong> <span id="api-status">Checking API...</span></p>
    </div>
    
    <script>
        // Check API status
        fetch('http://localhost:${API_PORT}/health')
            .then(response => response.json())
            .then(data => {
                document.getElementById('api-status').innerHTML = 
                    '<span style="color: green;">✅ Go API is running (' + data.service + ')</span>';
            })
            .catch(error => {
                document.getElementById('api-status').innerHTML = 
                    '<span style="color: red;">❌ Go API not responding</span>';
            });
    </script>
</body>
</html>
EOF
    echo -e "${GREEN}   ✅ Created basic frontend placeholder${NC}"
fi

# =============================================================================
# FINAL STATUS AND SUMMARY
# =============================================================================

echo -e "\n${PURPLE}📋 Setup Summary${NC}"
echo -e "${PURPLE}═══════════════════${NC}"

# Show Go module status
if [ -f "go.mod" ]; then
    GO_MODULE=$(grep "^module " go.mod | cut -d' ' -f2)
    GO_VERSION_FILE=$(grep "^go " go.mod | cut -d' ' -f2 2>/dev/null || echo 'not specified')
    GO_DEPS=$(grep -c "^	" go.mod 2>/dev/null || echo 0)
    
    echo -e "${GREEN}✅ Go Module: $GO_MODULE${NC}"
    echo -e "${GREEN}✅ Go Version: $GO_VERSION_FILE${NC}"
    echo -e "${GREEN}✅ Go Dependencies: $GO_DEPS packages${NC}"
else
    echo -e "${RED}❌ Go module not initialized${NC}"
fi

# Show Node.js status
if [ -f "package.json" ] && [ -d "node_modules" ]; then
    NODE_DEPS=$(find node_modules -maxdepth 1 -type d | wc -l)
    echo -e "${GREEN}✅ Node.js Dependencies: $((NODE_DEPS - 1)) packages${NC}"
    
    # Check critical packages
    CRITICAL_OK=true
    for package in "bull" "redis" "express"; do
        if [ ! -d "node_modules/$package" ]; then
            echo -e "${RED}❌ Missing critical package: $package${NC}"
            CRITICAL_OK=false
        fi
    done
    
    if [ "$CRITICAL_OK" = true ]; then
        echo -e "${GREEN}✅ All critical Node.js packages installed${NC}"
    fi
else
    echo -e "${RED}❌ Node.js dependencies not installed${NC}"
fi

# Show project structure
echo -e "${GREEN}✅ Project Structure: Created essential directories${NC}"
echo -e "${GREEN}✅ Frontend Placeholder: Basic HTML page created${NC}"

echo -e "\n${GREEN}🎉 ezHealthKonnect Setup Complete!${NC}"
echo -e "${GREEN}═══════════════════════════════════${NC}"

echo -e "\n${BLUE}💡 Next Steps:${NC}"
echo -e "${BLUE}   1. Run: ./start.sh (to start both frontend and API)${NC}"
echo -e "${BLUE}   2. Or manually:${NC}"
echo -e "${BLUE}      • Go API: go run ${GO_MAIN_PATH}${NC}"
echo -e "${BLUE}      • Node.js: npm start${NC}"
echo -e "${BLUE}   3. Open: http://localhost:${FRONTEND_PORT}${NC}"
echo -e "${BLUE}   4. Test API: curl http://localhost:${API_PORT}/health${NC}"
echo -e "${BLUE}   5. Integrate your HL7 parser code${NC}"

echo -e "\n${YELLOW}🔧 Development Workflow:${NC}"
echo -e "${YELLOW}   • Run ./setup.sh when setting up or adding dependencies${NC}"
echo -e "${YELLOW}   • Run ./start.sh for daily development${NC}"
echo -e "${YELLOW}   • Modify .env for configuration changes${NC}"

echo -e "\n${PURPLE}Ready to build the Mirth Connect killer! 🚀${NC}"