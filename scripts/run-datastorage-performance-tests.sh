#!/bin/bash
# Performance validation script for Data Storage Service
# BR-STORAGE-027: Validate p95 <250ms, p99 <500ms, large datasets <1s

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "======================================"
echo "Data Storage Service Performance Tests"
echo "BR-STORAGE-027 Validation"
echo "======================================"
echo ""

# Check if PostgreSQL is running
echo "✓ Checking PostgreSQL..."
if ! podman ps | grep -q datastorage-postgres; then
    echo "❌ PostgreSQL not running"
    echo "   Start with: podman run -d --name datastorage-postgres -p 5432:5432 \\"
    echo "              -e POSTGRESQL_USER=db_user -e POSTGRESQL_PASSWORD=test \\"
    echo "              -e POSTGRESQL_DATABASE=action_history \\"
    echo "              registry.redhat.io/rhel9/postgresql-16:latest"
    exit 1
fi
echo "✅ PostgreSQL is running"
echo ""

# Start Data Storage Service in background
echo "✓ Starting Data Storage Service..."
cd "$PROJECT_ROOT/cmd/datastorage"

# Kill any existing instance
pkill -f "datastorage/main" 2>/dev/null || true
sleep 1

# Start service
go run main.go \
    -addr=:8080 \
    -db-host=localhost \
    -db-port=5432 \
    -db-name=action_history \
    -db-user=db_user \
    -db-password=test \
    > /tmp/datastorage-perf.log 2>&1 &

SERVICE_PID=$!
echo "✅ Service started (PID: $SERVICE_PID)"
echo ""

# Wait for service to be ready
echo "✓ Waiting for service to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8080/health >/dev/null 2>&1; then
        echo "✅ Service is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ Service failed to start"
        kill $SERVICE_PID 2>/dev/null || true
        cat /tmp/datastorage-perf.log
        exit 1
    fi
    sleep 1
done
echo ""

# Run performance tests
echo "======================================"
echo "Running Performance Benchmarks"
echo "======================================"
echo ""

cd "$PROJECT_ROOT"

# Run comprehensive performance report
echo "Test 1: Comprehensive Performance Report"
go test -v -run TestPerformanceReport ./test/performance/datastorage/ -timeout 5m

echo ""
echo "Test 2: Latency Benchmarks"
go test -bench=BenchmarkListIncidentsLatency -benchmem -benchtime=1x ./test/performance/datastorage/

echo ""
echo "Test 3: Large Result Set Benchmarks"
go test -bench=BenchmarkLargeResultSet -benchmem -benchtime=1x ./test/performance/datastorage/

echo ""
echo "Test 4: Concurrent Load Benchmarks"
go test -bench=BenchmarkConcurrentRequests -benchmem -benchtime=1x ./test/performance/datastorage/

# Cleanup
echo ""
echo "======================================"
echo "Cleanup"
echo "======================================"
kill $SERVICE_PID 2>/dev/null || true
echo "✅ Service stopped"
echo ""

echo "======================================"
echo "Performance Tests Complete"
echo "======================================"
echo ""
echo "📊 Results Summary:"
echo "   - See output above for detailed metrics"
echo "   - Service logs: /tmp/datastorage-perf.log"
echo ""

