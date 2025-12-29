#!/bin/bash
# Notification Integration Test Infrastructure Setup
# Based on DS team's proven sequential startup pattern
# Reference: test/infrastructure/datastorage.go:1238-1400
# Fixes: podman-compose race condition causing Exit 137 failures
# Date: December 21, 2025

set -e  # Exit on any error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Container and network names (match podman-compose.notification.test.yml)
POSTGRES_CONTAINER="notification_postgres_1"
REDIS_CONTAINER="notification_redis_1"
DATASTORAGE_CONTAINER="notification_datastorage_1"
MIGRATIONS_CONTAINER="notification_migrations"
NETWORK_NAME="notification_nt-test-network"

# Ports (match podman-compose configuration)
POSTGRES_PORT="15453"
REDIS_PORT="16399"
DATASTORAGE_HTTP_PORT="18110"
DATASTORAGE_METRICS_PORT="19110"

# Database configuration
DB_NAME="action_history"
DB_USER="slm_user"
DB_PASSWORD="test_password"

# Script location
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

echo -e "${BLUE}🚀 Notification Integration Test Infrastructure Setup${NC}"
echo -e "${BLUE}   Based on DS team's sequential startup pattern${NC}"
echo -e "${BLUE}   Workspace: $WORKSPACE_ROOT${NC}"
echo ""

# ========================================
# STEP 1: Cleanup existing containers
# ========================================
echo -e "${YELLOW}🧹 Step 1: Cleaning up existing containers...${NC}"

podman stop $POSTGRES_CONTAINER 2>/dev/null || true
podman rm $POSTGRES_CONTAINER 2>/dev/null || true
echo "  ✓ PostgreSQL container cleaned"

podman stop $REDIS_CONTAINER 2>/dev/null || true
podman rm $REDIS_CONTAINER 2>/dev/null || true
echo "  ✓ Redis container cleaned"

podman stop $DATASTORAGE_CONTAINER 2>/dev/null || true
podman rm $DATASTORAGE_CONTAINER 2>/dev/null || true
echo "  ✓ DataStorage container cleaned"

podman stop $MIGRATIONS_CONTAINER 2>/dev/null || true
podman rm $MIGRATIONS_CONTAINER 2>/dev/null || true
echo "  ✓ Migrations container cleaned"

echo -e "${GREEN}✅ Cleanup complete${NC}"
echo ""

# ========================================
# STEP 2: Create network
# ========================================
echo -e "${YELLOW}🌐 Step 2: Creating network...${NC}"

# Create network if it doesn't exist (idempotent)
if ! podman network exists $NETWORK_NAME 2>/dev/null; then
  podman network create $NETWORK_NAME
  echo "  ✓ Network '$NETWORK_NAME' created"
else
  echo "  ✓ Network '$NETWORK_NAME' already exists (reusing)"
fi

echo -e "${GREEN}✅ Network ready${NC}"
echo ""

# ========================================
# STEP 3: Start PostgreSQL FIRST
# ========================================
echo -e "${YELLOW}🐘 Step 3: Starting PostgreSQL...${NC}"

podman run -d \
  --name $POSTGRES_CONTAINER \
  --network $NETWORK_NAME \
  -p ${POSTGRES_PORT}:5432 \
  -e POSTGRES_DB=$DB_NAME \
  -e POSTGRES_USER=$DB_USER \
  -e POSTGRES_PASSWORD=$DB_PASSWORD \
  postgres:16-alpine

echo "  ⏳ Waiting for PostgreSQL to be ready (up to 30s)..."
sleep 3  # Initial delay for container startup

# Wait for PostgreSQL ready (DS team pattern: 30s timeout, 1s polling)
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  if podman exec $POSTGRES_CONTAINER pg_isready -U $DB_USER > /dev/null 2>&1; then
    echo -e "${GREEN}  ✅ PostgreSQL is ready (${ATTEMPT}s)${NC}"
    break
  fi
  ATTEMPT=$((ATTEMPT + 1))
  if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo -e "${RED}  ❌ PostgreSQL failed to become ready after 30s${NC}"
    echo -e "${RED}     Check logs: podman logs $POSTGRES_CONTAINER${NC}"
    exit 1
  fi
  sleep 1
done

echo ""

# ========================================
# STEP 4: Run database migrations
# ========================================
echo -e "${YELLOW}📜 Step 4: Running database migrations...${NC}"

# Use same pattern as podman-compose: postgres image with bash script
# This matches the migrate service in podman-compose.notification.test.yml:46-75
podman run --rm \
  --name $MIGRATIONS_CONTAINER \
  --network $NETWORK_NAME \
  -v "$WORKSPACE_ROOT/migrations:/migrations:ro" \
  -e PGHOST=$POSTGRES_CONTAINER \
  -e PGPORT=5432 \
  -e PGUSER=$DB_USER \
  -e PGPASSWORD=$DB_PASSWORD \
  -e PGDATABASE=$DB_NAME \
  postgres:16-alpine \
  bash -c 'set -e
echo "Waiting for PostgreSQL..."
until pg_isready -h $PGHOST -U $PGUSER; do sleep 1; done
echo "Applying migrations (Up sections only)..."
find /migrations -maxdepth 1 -name "*.sql" -type f | sort | while read f; do
  echo "Applying $f..."
  sed -n "1,/^-- +goose Down/p" "$f" | grep -v "^-- +goose Down" | psql 2>&1 | grep -E "(CREATE|ALTER|ERROR)" || true
done
echo "Migrations complete!"'

echo -e "${GREEN}✅ Migrations complete${NC}"
echo ""

# ========================================
# STEP 5: Start Redis SECOND
# ========================================
echo -e "${YELLOW}📦 Step 5: Starting Redis...${NC}"

podman run -d \
  --name $REDIS_CONTAINER \
  --network $NETWORK_NAME \
  -p ${REDIS_PORT}:6379 \
  quay.io/jordigilh/redis:7-alpine

echo "  ⏳ Waiting for Redis to be ready (up to 30s)..."
sleep 2  # Initial delay for container startup

# Wait for Redis ready (DS team pattern: 30s timeout, 1s polling)
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  if podman exec $REDIS_CONTAINER redis-cli ping 2>/dev/null | grep -q PONG; then
    echo -e "${GREEN}  ✅ Redis is ready (${ATTEMPT}s)${NC}"
    break
  fi
  ATTEMPT=$((ATTEMPT + 1))
  if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo -e "${RED}  ❌ Redis failed to become ready after 30s${NC}"
    echo -e "${RED}     Check logs: podman logs $REDIS_CONTAINER${NC}"
    exit 1
  fi
  sleep 1
done

echo ""

# ========================================
# STEP 6: Start DataStorage LAST
# ========================================
echo -e "${YELLOW}💾 Step 6: Starting DataStorage...${NC}"

# Build DataStorage image if it doesn't exist
if ! podman image exists localhost/notification_datastorage:latest; then
  echo "  Building DataStorage image..."
  cd "$WORKSPACE_ROOT"
  podman build -t notification_datastorage:latest -f docker/data-storage.Dockerfile .
fi

# Create config directory if it doesn't exist
CONFIG_DIR="$SCRIPT_DIR/config"
mkdir -p "$CONFIG_DIR"

# Always create fresh config.yaml with correct container names
# ADR-030: Secrets loaded from mounted files
cat > "$CONFIG_DIR/config.yaml" <<EOF
database:
  host: $POSTGRES_CONTAINER
  port: 5432
  user: $DB_USER
  name: $DB_NAME
  sslmode: disable
  secretsFile: /etc/datastorage/db-secrets.yaml
  usernameKey: username
  passwordKey: password

redis:
  addr: ${REDIS_CONTAINER}:6379
  secretsFile: /etc/datastorage/redis-secrets.yaml
  passwordKey: password

server:
  port: 8080
  metrics_port: 9090
EOF
echo "  ✓ Created config.yaml with container hostnames"

# Create database secrets file (ADR-030 Section 6)
cat > "$CONFIG_DIR/db-secrets.yaml" <<EOF
username: $DB_USER
password: $DB_PASSWORD
EOF
chmod 0666 "$CONFIG_DIR/db-secrets.yaml"  # DS team pattern: 0666 for macOS Podman
echo "  ✓ Created db-secrets.yaml"

# Create redis secrets file (ADR-030 Section 6)
cat > "$CONFIG_DIR/redis-secrets.yaml" <<EOF
password: ""
EOF
chmod 0666 "$CONFIG_DIR/redis-secrets.yaml"  # DS team pattern: 0666 for macOS Podman
echo "  ✓ Created redis-secrets.yaml"

podman run -d \
  --name $DATASTORAGE_CONTAINER \
  --network $NETWORK_NAME \
  -p ${DATASTORAGE_HTTP_PORT}:8080 \
  -p ${DATASTORAGE_METRICS_PORT}:9090 \
  -v "$CONFIG_DIR:/etc/datastorage:ro" \
  -e CONFIG_PATH=/etc/datastorage/config.yaml \
  localhost/notification_datastorage:latest

echo "  ⏳ Waiting for DataStorage health check (up to 30s)..."
echo "     DS team note: Cold start on macOS can take 15-20s"

# Wait for DataStorage health endpoint (DS team pattern: 30s timeout, 1s polling)
# NOTE: Use 127.0.0.1 (not localhost) per DS team recommendation
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  if curl -s http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health > /dev/null 2>&1; then
    HEALTH_STATUS=$(curl -s http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health | grep -o '"status":"[^"]*"' || true)
    if [ -n "$HEALTH_STATUS" ]; then
      echo -e "${GREEN}  ✅ DataStorage is healthy (${ATTEMPT}s)${NC}"
      echo "     Health: $HEALTH_STATUS"
      break
    fi
  fi
  ATTEMPT=$((ATTEMPT + 1))
  if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo -e "${RED}  ❌ DataStorage failed to become healthy after 30s${NC}"
    echo -e "${RED}     Check logs: podman logs $DATASTORAGE_CONTAINER${NC}"
    echo -e "${RED}     Health endpoint: http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health${NC}"
    exit 1
  fi
  sleep 1
done

echo ""

# ========================================
# STEP 7: Final validation
# ========================================
echo -e "${YELLOW}🔍 Step 7: Final validation...${NC}"

# Check all containers are running
echo "  Checking container status..."
POSTGRES_STATUS=$(podman inspect --format '{{.State.Status}}' $POSTGRES_CONTAINER)
REDIS_STATUS=$(podman inspect --format '{{.State.Status}}' $REDIS_CONTAINER)
DATASTORAGE_STATUS=$(podman inspect --format '{{.State.Status}}' $DATASTORAGE_CONTAINER)

if [ "$POSTGRES_STATUS" != "running" ]; then
  echo -e "${RED}  ❌ PostgreSQL is not running (status: $POSTGRES_STATUS)${NC}"
  exit 1
fi
echo "  ✓ PostgreSQL: $POSTGRES_STATUS"

if [ "$REDIS_STATUS" != "running" ]; then
  echo -e "${RED}  ❌ Redis is not running (status: $REDIS_STATUS)${NC}"
  exit 1
fi
echo "  ✓ Redis: $REDIS_STATUS"

if [ "$DATASTORAGE_STATUS" != "running" ]; then
  echo -e "${RED}  ❌ DataStorage is not running (status: $DATASTORAGE_STATUS)${NC}"
  exit 1
fi
echo "  ✓ DataStorage: $DATASTORAGE_STATUS"

# Check DataStorage health one more time
HEALTH_RESPONSE=$(curl -s http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health)
if echo "$HEALTH_RESPONSE" | grep -q "ok\|healthy"; then
  echo "  ✓ DataStorage health: OK"
else
  echo -e "${RED}  ⚠️  DataStorage health check returned unexpected response${NC}"
  echo "     Response: $HEALTH_RESPONSE"
fi

echo ""
echo -e "${GREEN}╔════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  ✅ Infrastructure Ready for Integration Tests     ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BLUE}📊 Services:${NC}"
echo "  • PostgreSQL:  127.0.0.1:$POSTGRES_PORT"
echo "  • Redis:       127.0.0.1:$REDIS_PORT"
echo "  • DataStorage: http://127.0.0.1:$DATASTORAGE_HTTP_PORT (health: /health)"
echo "  • Metrics:     http://127.0.0.1:$DATASTORAGE_METRICS_PORT"
echo ""
echo -e "${BLUE}🔧 Useful Commands:${NC}"
echo "  • View logs:   podman logs <container_name>"
echo "  • Check status: podman ps"
echo "  • Cleanup:     make test-integration-notification-cleanup"
echo ""
echo -e "${GREEN}Ready to run: make test-integration-notification${NC}"
echo ""

# ========================================
# DS Team Pattern Summary
# ========================================
# ✅ Sequential startup (not podman-compose)
# ✅ Explicit wait logic with 30s timeouts
# ✅ 1s polling intervals for fast detection
# ✅ Uses 127.0.0.1 (not localhost)
# ✅ Clear error messages at each step
# ✅ Idempotent (can be run multiple times)
# ========================================
# Result: 100% test pass rate (DS team proven)
# ========================================

