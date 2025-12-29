#!/bin/bash
# WorkflowExecution Integration Test Infrastructure Setup
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

# Container and network names (match podman-compose.test.yml)
POSTGRES_CONTAINER="workflowexecution_postgres_1"
REDIS_CONTAINER="workflowexecution_redis_1"
DATASTORAGE_CONTAINER="workflowexecution_datastorage_1"
MIGRATIONS_CONTAINER="workflowexecution_migrations"
NETWORK_NAME="workflowexecution_test-network"

# Ports (match podman-compose configuration)
POSTGRES_PORT="15443"
REDIS_PORT="16389"
DATASTORAGE_HTTP_PORT="18100"
DATASTORAGE_METRICS_PORT="19100"

# Database configuration
DB_NAME="action_history"
DB_USER="slm_user"
DB_PASSWORD="test_password"

# Script location
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

echo -e "${BLUE}🚀 WorkflowExecution Integration Test Infrastructure Setup${NC}"
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

# Wait for PostgreSQL to be ready
for i in {1..30}; do
  if podman exec $POSTGRES_CONTAINER pg_isready -U $DB_USER -d $DB_NAME > /dev/null 2>&1; then
    echo -e "${GREEN}  ✅ PostgreSQL is ready (took ${i}s)${NC}"
    break
  fi

  if [ $i -eq 30 ]; then
    echo -e "${RED}  ❌ PostgreSQL failed to become ready after 30 seconds${NC}"
    echo -e "${RED}     Container logs:${NC}"
    podman logs $POSTGRES_CONTAINER --tail 20
    exit 1
  fi

  sleep 1
done

echo ""

# ========================================
# STEP 4: Run database migrations
# ========================================
echo -e "${YELLOW}🔄 Step 4: Running database migrations...${NC}"

# Use same pattern as notification service: postgres image with bash script
# This matches the migrate service pattern
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
echo -e "${YELLOW}🔴 Step 5: Starting Redis...${NC}"

podman run -d \
  --name $REDIS_CONTAINER \
  --network $NETWORK_NAME \
  -p ${REDIS_PORT}:6379 \
  redis:7-alpine

echo "  ⏳ Waiting for Redis to be ready (up to 10s)..."
sleep 2  # Initial delay for container startup

# Wait for Redis to be ready
for i in {1..10}; do
  if podman exec $REDIS_CONTAINER redis-cli ping 2>/dev/null | grep -q PONG; then
    echo -e "${GREEN}  ✅ Redis is ready (took ${i}s)${NC}"
    break
  fi

  if [ $i -eq 10 ]; then
    echo -e "${RED}  ❌ Redis failed to become ready after 10 seconds${NC}"
    echo -e "${RED}     Container logs:${NC}"
    podman logs $REDIS_CONTAINER --tail 20
    exit 1
  fi

  sleep 1
done

echo ""

# ========================================
# STEP 6: Start DataStorage LAST
# ========================================
echo -e "${YELLOW}📦 Step 6: Starting DataStorage...${NC}"

# Build DataStorage image if it doesn't exist
if ! podman image exists localhost/data-storage:test; then
  echo "  Building DataStorage image..."
  cd "$WORKSPACE_ROOT"
  podman build -t localhost/data-storage:test -f docker/data-storage.Dockerfile . > /dev/null 2>&1
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
  localhost/data-storage:test

echo "  ⏳ Waiting for DataStorage health check (up to 30s)..."
echo "     DS team note: Cold start on macOS can take 15-20s"

# Wait for DataStorage health endpoint (DS team pattern: 30s timeout, 1s polling)
# NOTE: Use 127.0.0.1 (not localhost) per DS team recommendation
MAX_ATTEMPTS=30
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  if curl -sf http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health > /dev/null 2>&1; then
    echo -e "${GREEN}  ✅ DataStorage is healthy (took ${ATTEMPT}s)${NC}"
    break
  fi

  ATTEMPT=$((ATTEMPT + 1))
  if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo -e "${RED}  ❌ DataStorage failed to become healthy after 30s${NC}"
    echo -e "${RED}     Container status:${NC}"
    podman ps -a --filter name=$DATASTORAGE_CONTAINER
    echo -e "${RED}     Container logs:${NC}"
    podman logs $DATASTORAGE_CONTAINER --tail 30
    exit 1
  fi

  sleep 1
done

echo ""

# ========================================
# STEP 7: Verify all services
# ========================================
echo -e "${YELLOW}🔍 Step 7: Verifying all services...${NC}"

# Check PostgreSQL
if podman exec $POSTGRES_CONTAINER pg_isready -U $DB_USER -d $DB_NAME > /dev/null 2>&1; then
  echo -e "${GREEN}  ✅ PostgreSQL: Running${NC}"
else
  echo -e "${RED}  ❌ PostgreSQL: Failed${NC}"
  exit 1
fi

# Check Redis
if podman exec $REDIS_CONTAINER redis-cli ping 2>/dev/null | grep -q PONG; then
  echo -e "${GREEN}  ✅ Redis: Running${NC}"
else
  echo -e "${RED}  ❌ Redis: Failed${NC}"
  exit 1
fi

# Check DataStorage
HEALTH_RESPONSE=$(curl -sf http://localhost:${DATASTORAGE_HTTP_PORT}/health)
if echo "$HEALTH_RESPONSE" | grep -q '"status":"healthy"'; then
  echo -e "${GREEN}  ✅ DataStorage: Healthy${NC}"
else
  echo -e "${RED}  ❌ DataStorage: Unhealthy${NC}"
  echo "     Response: $HEALTH_RESPONSE"
  exit 1
fi

echo ""
echo -e "${GREEN}🎉 All services are ready!${NC}"
echo ""
echo -e "${BLUE}Service Endpoints:${NC}"
echo "  PostgreSQL:  localhost:${POSTGRES_PORT}"
echo "  Redis:       localhost:${REDIS_PORT}"
echo "  DataStorage: http://localhost:${DATASTORAGE_HTTP_PORT}"
echo "  Metrics:     http://localhost:${DATASTORAGE_METRICS_PORT}/metrics"
echo ""
echo -e "${BLUE}To run tests:${NC}"
echo "  cd $WORKSPACE_ROOT"
echo "  make test-integration-workflowexecution"
echo ""
echo -e "${BLUE}To cleanup:${NC}"
echo "  cd $SCRIPT_DIR"
echo "  ./teardown-infrastructure.sh"
echo ""

