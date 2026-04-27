#!/bin/bash

echo "🚀 Ubuntu Auto-Update Container Test Script"
echo "==========================================="

# Check if containers are running
echo "📦 Checking container status..."
docker-compose ps

echo -e "\n🔍 Testing services..."

# Test frontend
echo "• Frontend (http://localhost:3000):"
FRONTEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000)
if [ "$FRONTEND_STATUS" = "200" ]; then
    echo "  ✅ Frontend is accessible"
else
    echo "  ❌ Frontend not accessible (HTTP $FRONTEND_STATUS)"
fi

# Test backend API
echo "• Backend API (http://localhost:8082):"
BACKEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8082/api/v1/hosts)
if [ "$BACKEND_STATUS" = "401" ] || [ "$BACKEND_STATUS" = "200" ]; then
    echo "  ✅ Backend API is responding"
else
    echo "  ❌ Backend API not responding (HTTP $BACKEND_STATUS)"
fi

# Test database connection
echo "• PostgreSQL Database:"
DB_STATUS=$(docker-compose exec -T postgres pg_isready -U user -d uau_db 2>/dev/null)
if echo "$DB_STATUS" | grep -q "accepting connections"; then
    echo "  ✅ Database is accessible"
else
    echo "  ❌ Database not accessible"
fi

# Test Redis
echo "• Redis Cache:"
REDIS_STATUS=$(docker-compose exec -T redis redis-cli ping 2>/dev/null)
if [ "$REDIS_STATUS" = "PONG" ]; then
    echo "  ✅ Redis is accessible"
else
    echo "  ❌ Redis not accessible"
fi

echo -e "\n📊 Container Resource Usage:"
docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" $(docker-compose ps -q)

echo -e "\n🌐 Application URLs:"
echo "  • Frontend: http://localhost:3000"
echo "  • Backend API: http://localhost:8082"
echo "  • PostgreSQL: localhost:5432"
echo "  • Redis: localhost:6379"

echo -e "\n📝 Container Logs (last 5 lines each):"
echo "Frontend:"
docker-compose logs --tail=5 frontend 2>/dev/null | sed 's/^/  /'

echo "Backend:"
docker-compose logs --tail=5 backend 2>/dev/null | sed 's/^/  /'

echo -e "\n🎯 Next Steps:"
echo "  1. Open http://localhost:3000 in your browser"
echo "  2. Use admin/password to login (configured in docker-compose.yml)"
echo "  3. Explore the advanced dashboard with:"
echo "     • Real-time system monitoring"
echo "     • Host management"
echo "     • Update scheduling"
echo "     • Service management"
echo "  4. To stop: docker-compose down"
echo "  5. To view logs: docker-compose logs -f [service]"

echo -e "\n✨ Advanced Features Available:"
echo "  • Modern React dashboard with dark/light themes"
echo "  • Advanced UI components (modals, alerts, cards)"
echo "  • Real-time WebSocket updates"
echo "  • Responsive design for desktop and mobile"
echo "  • Enterprise-grade monitoring and analytics"