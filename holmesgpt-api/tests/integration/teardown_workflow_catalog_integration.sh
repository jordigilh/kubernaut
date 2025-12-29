#!/bin/bash
# Teardown script for Workflow Catalog Integration Tests
#
# This script stops and removes all Docker containers and volumes
# used by the integration tests.
#
# Usage: ./teardown_workflow_catalog_integration.sh

set -e

echo "========================================="
echo "Workflow Catalog Integration Test Teardown"
echo "========================================="
echo ""

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.workflow-catalog.yml"
PROJECT_NAME="kubernaut-hapi-workflow-catalog-integration"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if Docker/Podman is available
if command -v docker &> /dev/null; then
    COMPOSE_CMD="docker-compose"
elif command -v podman &> /dev/null; then
    COMPOSE_CMD="podman-compose"
else
    echo -e "${RED}❌ Error: Neither Docker nor Podman is installed${NC}"
    exit 1
fi

echo "🧹 Stopping and removing containers..."
$COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" down -v

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Containers and volumes removed${NC}"
else
    echo -e "${RED}❌ Failed to remove containers${NC}"
    exit 1
fi

echo ""
echo "========================================="
echo "✅ Teardown Complete"
echo "========================================="
echo ""

