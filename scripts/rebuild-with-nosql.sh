#!/bin/bash

# Rebuild Docker Containers with NoSQL Support
# This script rebuilds the app container with MongoDB and Redis drivers

echo "🔄 Rebuilding ezHealthKonnect with MongoDB & Redis support..."
echo ""

# Stop existing containers
echo "1️⃣ Stopping existing containers..."
docker-compose stop app
echo "✅ App container stopped"
echo ""

# Rebuild app container (downloads new Go dependencies)
echo "2️⃣ Rebuilding app container..."
docker-compose build --no-cache app
echo "✅ App container rebuilt"
echo ""

# Start all services
echo "3️⃣ Starting all services..."
docker-compose up -d
echo "✅ All services started"
echo ""

# Wait for services to be healthy
echo "4️⃣ Waiting for services to be healthy..."
sleep 10

# Check service status
echo "5️⃣ Checking service status..."
docker-compose ps
echo ""

# Seed NoSQL test data
echo "6️⃣ Seeding MongoDB and Redis with test data..."
docker-compose exec -T app node /app/scripts/seed_nosql_test_data.js
echo "✅ Test data seeded"
echo ""

echo "🎉 Rebuild complete!"
echo ""
echo "📝 Next steps:"
echo "   1. Open http://localhost:3000"
echo "   2. Navigate to Pipeline Builder"
echo "   3. Add Database Enrichment step"
echo "   4. Try MongoDB or Redis as database type"
echo "   5. Test with Query Tester"
echo ""
echo "📖 Documentation:"
echo "   - DATABASE_ENRICHMENT_MONGODB_REDIS.md"
echo "   - DATABASE_ENRICHMENT_MULTI_DB_PLAN.md"
echo ""
