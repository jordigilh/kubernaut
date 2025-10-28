#!/bin/bash
set -euo pipefail

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🧪 Gateway Integration Tests (Local Redis + Remote K8s)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "✅ Redis: localhost:6379 (Podman container)"
echo "✅ K8s API: helios08 OCP cluster (real auth/authz)"
echo ""

# Start Redis
./test/integration/gateway/start-redis.sh

# Cleanup function
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."
    ./test/integration/gateway/stop-redis.sh
}
trap cleanup EXIT

# Run tests
echo ""
echo "🚀 Running integration tests..."
go test -v ./test/integration/gateway -run "TestGatewayIntegration" -timeout 30m 2>&1 | tee /tmp/local-redis-tests.log

echo ""
echo "✅ Tests complete"
echo "📄 Full log: /tmp/local-redis-tests.log"


