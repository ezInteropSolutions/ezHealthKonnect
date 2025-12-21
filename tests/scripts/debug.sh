#!/bin/bash

echo "🔍 Verifying HL7 Schema Fix Applied"
echo "=================================="

echo "🔍 Step 1: Check if the fix was built and deployed"
echo "Looking for the debug output in logs..."

echo ""
echo "Recent container logs (looking for our debug message):"
docker logs ezhealthkonnect-app --tail 20 | grep -E "(HL7 Schema Path|Initializing HL7|WARNING.*HL7)" || echo "Debug message not found in recent logs"

echo ""
echo "🔍 Step 2: Check if container was rebuilt after the change"
echo "Container build time:"
docker inspect ezhealthkonnect-app | grep '"Created"' || echo "Could not get container creation time"

echo ""
echo "🔍 Step 3: Test current status"
echo "Health endpoint HL7 schema status:"
curl -s http://localhost:8080/health | grep -A 10 -B 5 -E "(hl7|schema|Schema)" 2>/dev/null || echo "Could not get schema status from health endpoint"

echo ""
echo "🔍 Step 4: Manual verification - check actual file structure"
echo "Files the HL7 loader should now see in /app/schemas/hl7:"
docker exec ezhealthkonnect-app find /app/schemas/hl7 -name "*.gz" | head -10 2>/dev/null || echo "Could not access /app/schemas/hl7 or no .gz files found"

echo ""
echo "🔧 TROUBLESHOOTING:"
echo "=================="

# Check if we see our debug message
DEBUG_FOUND=$(docker logs ezhealthkonnect-app --tail 50 | grep "HL7 Schema Path:" || echo "NOT_FOUND")

if [ "$DEBUG_FOUND" = "NOT_FOUND" ]; then
    echo "❌ ISSUE: Debug message not found in logs"
    echo ""
    echo "POSSIBLE CAUSES:"
    echo "1. Container wasn't rebuilt after adding the fix"
    echo "2. Build failed due to syntax error"
    echo "3. Fix was added in wrong location"
    echo ""
    echo "SOLUTIONS:"
    echo "1. Rebuild container: docker-compose down && docker-compose up --build -d"
    echo "2. Check for build errors during rebuild"
    echo "3. Verify the fix is in the right place in main.go"
    
else
    echo "✅ DEBUG MESSAGE FOUND!"
    echo "Debug output: $DEBUG_FOUND"
    
    # Extract the actual path from debug message
    ACTUAL_PATH=$(echo "$DEBUG_FOUND" | sed 's/.*HL7 Schema Path: //')
    echo "Detected HL7 schema path: $ACTUAL_PATH"
    
    # Check if files exist at that path
    echo ""
    echo "Checking if .gz files exist at the detected path:"
    docker exec ezhealthkonnect-app ls -la "$ACTUAL_PATH"/*.gz 2>/dev/null | head -5 || echo "No .gz files found at $ACTUAL_PATH"
    
    # Check if warning still appears
    WARNING_STILL_EXISTS=$(docker logs ezhealthkonnect-app --tail 20 | grep "WARNING.*No HL7 schema files" || echo "NO_WARNING")
    
    if [ "$WARNING_STILL_EXISTS" != "NO_WARNING" ]; then
        echo ""
        echo "❌ WARNING STILL EXISTS despite fix"
        echo "This means the HL7 loader isn't finding .gz files at the path"
        echo ""
        echo "NEXT STEPS:"
        echo "1. Verify .gz files exist: docker exec ezhealthkonnect-app find /app/schemas/hl7 -name '*.gz'"
        echo "2. Check HL7 loader implementation - might need additional fixes"
        echo "3. Consider the symlink workaround temporarily"
    else
        echo ""
        echo "✅ NO WARNING FOUND - FIX SUCCESSFUL!"
    fi
fi

echo ""
echo "🎯 IMMEDIATE ACTIONS:"
echo "===================="

echo ""
echo "1. If debug message not found, rebuild:"
echo "   docker-compose down"
echo "   docker-compose up --build -d"
echo ""

echo "2. If debug message found but warning persists:"
echo "   # Check if .gz files exist in the HL7 directory"
echo "   docker exec ezhealthkonnect-app find /app/schemas/hl7 -name '*.gz' | head -5"
echo ""

echo "3. Quick test of HL7 functionality:"
echo "   curl -s http://localhost:8080/api/schema/status"
echo ""

echo "4. Remember: HL7 schema warnings don't affect FHIR transformations!"
echo "   FHIR transforms use database rules, not HL7 schema files"

echo ""
echo "🔄 Let's rebuild to ensure the fix takes effect:"
echo "Run: docker-compose down && docker-compose up --build -d"