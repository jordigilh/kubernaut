#!/bin/bash
# Setup script for Workflow Catalog Integration Tests
#
# Business Requirement: BR-STORAGE-013 - Semantic Search for Remediation Workflows
# Design Decision: DD-TEST-001 - Port Allocation Strategy
#
# DD-TEST-001 Port Allocation for HolmesGPT-API (as dependency consumer):
#   - PostgreSQL: 15435 (not 15433 - that's Data Storage's own tests)
#   - Redis: 16381 (not 16379 - that's Data Storage's own tests)
#   - Embedding Service: 18001 (not 18000 - that's Data Storage's own tests)
#   - Data Storage: 18094 (not 18090 - that's Data Storage's own tests)
#
# This script:
# 1. Starts Docker containers (PostgreSQL, Redis, Embedding Service, Data Storage Service)
# 2. Waits for services to be healthy
# 3. Bootstraps test data in database
# 4. Validates services are ready
#
# Usage: ./setup_workflow_catalog_integration.sh

set -e

echo "========================================="
echo "Workflow Catalog Integration Test Setup"
echo "========================================="
echo ""

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.workflow-catalog.yml"
PROJECT_NAME="kubernaut-hapi-workflow-catalog-integration"

# DD-TEST-001: HolmesGPT-API ports (different from Data Storage's own tests)
POSTGRES_PORT=15435
REDIS_PORT=16381
EMBEDDING_SERVICE_PORT=18001
DATA_STORAGE_PORT=18094

# Container names (prefixed with 'hapi' to avoid conflicts)
POSTGRES_CONTAINER="kubernaut-hapi-postgres-integration"
REDIS_CONTAINER="kubernaut-hapi-redis-integration"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if Docker/Podman is available
if command -v docker &> /dev/null; then
    DOCKER_CMD="docker"
    COMPOSE_CMD="docker-compose"
elif command -v podman &> /dev/null; then
    DOCKER_CMD="podman"
    COMPOSE_CMD="podman-compose"
else
    echo -e "${RED}❌ Error: Neither Docker nor Podman is installed${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Using: $DOCKER_CMD${NC}"
echo ""

# Stop any existing containers
echo "🧹 Cleaning up existing containers..."
$COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" down -v 2>/dev/null || true
echo ""

# Start services
echo "🚀 Starting services..."
$COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" up -d

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Failed to start services${NC}"
    exit 1
fi
echo ""

# Wait for PostgreSQL
echo "⏳ Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
    if $DOCKER_CMD exec $POSTGRES_CONTAINER pg_isready -U kubernaut -d kubernaut_test &> /dev/null; then
        echo -e "${GREEN}✅ PostgreSQL is ready${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}❌ PostgreSQL failed to start after 30 seconds${NC}"
        $COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs postgres-integration
        exit 1
    fi
    sleep 1
done
echo ""

# Wait for Redis
echo "⏳ Waiting for Redis to be ready..."
for i in {1..30}; do
    if $DOCKER_CMD exec $REDIS_CONTAINER redis-cli ping &> /dev/null; then
        echo -e "${GREEN}✅ Redis is ready${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}❌ Redis failed to start after 30 seconds${NC}"
        exit 1
    fi
    sleep 1
done
echo ""

# Wait for Embedding Service
echo "⏳ Waiting for Embedding Service to be ready..."
for i in {1..60}; do
    if curl -sf http://localhost:$EMBEDDING_SERVICE_PORT/health &> /dev/null; then
        echo -e "${GREEN}✅ Embedding Service is ready${NC}"
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}❌ Embedding Service failed to start after 60 seconds${NC}"
        $COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs embedding-service
        exit 1
    fi
    sleep 1
done
echo ""

# Wait for Data Storage Service
echo "⏳ Waiting for Data Storage Service to be ready..."
for i in {1..60}; do
    if curl -sf http://localhost:$DATA_STORAGE_PORT/health &> /dev/null; then
        echo -e "${GREEN}✅ Data Storage Service is ready${NC}"
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}❌ Data Storage Service failed to start after 60 seconds${NC}"
        $COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs data-storage-service
        exit 1
    fi
    sleep 1
done
echo ""

# Bootstrap workflows via REST API (DD-STORAGE-011)
# This ensures embeddings are auto-generated instead of SQL-based seeding
echo "🔧 Bootstrapping test workflows via API..."
"$SCRIPT_DIR/bootstrap-workflows.sh"
if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Failed to bootstrap workflows${NC}"
    exit 1
fi
echo ""

# Verify test data with embeddings
echo "🔍 Verifying test data in database..."
# DD-NAMING-001: Table is "remediation_workflow_catalog" (not "workflow_catalog")
WORKFLOW_COUNT=$($DOCKER_CMD exec $POSTGRES_CONTAINER psql -U kubernaut -d kubernaut_test -t -c "SELECT COUNT(*) FROM remediation_workflow_catalog WHERE embedding IS NOT NULL;" | tr -d ' ')

if [ "$WORKFLOW_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✅ Test data verified: $WORKFLOW_COUNT workflows with embeddings${NC}"
else
    echo -e "${YELLOW}⚠️  Warning: No workflows with embeddings found${NC}"
fi
echo ""

# Test Data Storage Service API
echo "🔍 Testing Data Storage Service API..."
SEARCH_RESPONSE=$(curl -s -X POST http://localhost:$DATA_STORAGE_PORT/api/v1/workflows/search \
    -H "Content-Type: application/json" \
    -d '{
        "query": "OOMKilled critical",
        "filters": {
            "signal_type": "OOMKilled",
            "severity": "critical"
        },
        "top_k": 5,
        "min_similarity": 0.0
    }')

RESULT_COUNT=$(echo "$SEARCH_RESPONSE" | grep -o '"total_results":[0-9]*' | grep -o '[0-9]*' || echo "0")

if [ "$RESULT_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✅ Data Storage Service API working: $RESULT_COUNT workflows found${NC}"
else
    echo -e "${YELLOW}⚠️  Warning: Data Storage Service API returned 0 results${NC}"
    echo "Response: $SEARCH_RESPONSE"
fi
echo ""

# Summary
echo "========================================="
echo "✅ Integration Test Environment Ready"
echo "========================================="
echo ""
echo "Services (DD-TEST-001 HolmesGPT-API ports):"
echo "  - PostgreSQL:          localhost:$POSTGRES_PORT"
echo "  - Redis:               localhost:$REDIS_PORT"
echo "  - Embedding Service:   http://localhost:$EMBEDDING_SERVICE_PORT"
echo "  - Data Storage Service: http://localhost:$DATA_STORAGE_PORT"
echo ""
echo "Test data: $WORKFLOW_COUNT workflows"
echo ""
echo "Run tests with:"
echo "  cd $SCRIPT_DIR/../.."
echo "  python3 -m pytest tests/integration/test_workflow_catalog_data_storage_integration.py -v"
echo ""
echo "View logs with:"
echo "  $COMPOSE_CMD -f $COMPOSE_FILE -p $PROJECT_NAME logs -f"
echo ""
echo "Teardown with:"
echo "  ./tests/integration/teardown_workflow_catalog_integration.sh"
echo ""
