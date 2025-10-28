#!/bin/bash
set -euo pipefail

echo "🚀 Starting local Redis for integration tests..."

# Check if Redis is already running with correct configuration
if podman ps | grep -q redis-gateway-test; then
    echo "📋 Redis container exists, checking configuration..."

    # Check current maxmemory setting
    CURRENT_MAXMEM=$(podman exec redis-gateway-test redis-cli CONFIG GET maxmemory | tail -1)
    EXPECTED_MAXMEM="2147483648"  # 2GB in bytes

    if [ "$CURRENT_MAXMEM" = "$EXPECTED_MAXMEM" ]; then
        echo "✅ Redis already running with correct configuration (2GB)"
        podman exec redis-gateway-test redis-cli PING
        exit 0
    else
        echo "⚠️  Redis running with incorrect maxmemory: $CURRENT_MAXMEM bytes (expected: $EXPECTED_MAXMEM)"
        echo "🔄 Recreating Redis with correct configuration..."
        podman stop redis-gateway-test 2>/dev/null || true
        podman rm -f redis-gateway-test 2>/dev/null || true
    fi
fi

# Remove old container if exists
podman rm -f redis-gateway-test 2>/dev/null || true

# Create network
podman network create kubernaut-test 2>/dev/null || true

# Start Redis
podman run -d \
  --name redis-gateway-test \
  --network kubernaut-test \
  -p 6379:6379 \
  redis:7-alpine \
  redis-server \
    --maxmemory 2gb \
    --maxmemory-policy allkeys-lru \
    --save "" \
    --appendonly no

# Wait for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1


echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
    if podman exec redis-gateway-test redis-cli PING 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready (localhost:6379)"
        exit 0
    fi
    sleep 1
done

echo "❌ Redis failed to start"
exit 1

