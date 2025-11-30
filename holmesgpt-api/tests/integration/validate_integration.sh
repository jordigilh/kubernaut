#!/bin/bash
# Quick validation script for Workflow Catalog Integration
#
# This script performs a quick smoke test to ensure everything is working
# before running the full integration test suite.
#
# Usage: ./validate_integration.sh

set -e

echo "========================================="
echo "Workflow Catalog Integration Validation"
echo "========================================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if services are running
echo "🔍 Checking if services are running..."
echo ""

# Check PostgreSQL
if curl -sf http://localhost:18090/health &> /dev/null; then
    echo -e "${GREEN}✅ Data Storage Service is running (port 18090)${NC}"
else
    echo -e "${RED}❌ Data Storage Service is NOT running${NC}"
    echo "   Run: ./setup_workflow_catalog_integration.sh"
    exit 1
fi

# Check Embedding Service
if curl -sf http://localhost:18000/health &> /dev/null; then
    echo -e "${GREEN}✅ Embedding Service is running (port 18000)${NC}"
else
    echo -e "${RED}❌ Embedding Service is NOT running${NC}"
    echo "   Run: ./setup_workflow_catalog_integration.sh"
    exit 1
fi

echo ""

# Test Data Storage Service API
echo "🔍 Testing Data Storage Service API..."
SEARCH_RESPONSE=$(curl -s -X POST http://localhost:18090/api/v1/workflows/search \
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

if [ $? -eq 0 ]; then
    RESULT_COUNT=$(echo "$SEARCH_RESPONSE" | grep -o '"total_results":[0-9]*' | grep -o '[0-9]*' || echo "0")
    if [ "$RESULT_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✅ Data Storage Service API working: $RESULT_COUNT workflows found${NC}"
    else
        echo -e "${YELLOW}⚠️  Warning: Data Storage Service API returned 0 results${NC}"
        echo "   This may be normal if embeddings haven't been generated yet"
    fi
else
    echo -e "${RED}❌ Data Storage Service API failed${NC}"
    exit 1
fi

echo ""

# Test Embedding Service API
echo "🔍 Testing Embedding Service API..."
EMBEDDING_RESPONSE=$(curl -s -X POST http://localhost:18000/embed \
    -H "Content-Type: application/json" \
    -d '{"text": "test embedding"}')

if [ $? -eq 0 ]; then
    EMBEDDING_DIM=$(echo "$EMBEDDING_RESPONSE" | grep -o '"embedding":\[[^]]*\]' | grep -o ',' | wc -l)
    if [ "$EMBEDDING_DIM" -gt 0 ]; then
        echo -e "${GREEN}✅ Embedding Service API working: ${EMBEDDING_DIM}-dimensional embeddings${NC}"
    else
        echo -e "${YELLOW}⚠️  Warning: Embedding Service returned unexpected format${NC}"
    fi
else
    echo -e "${RED}❌ Embedding Service API failed${NC}"
    exit 1
fi

echo ""

# Summary
echo "========================================="
echo -e "${GREEN}✅ All Services Validated${NC}"
echo "========================================="
echo ""
echo "Ready to run integration tests:"
echo "  cd ../.."
echo "  python3 -m pytest tests/integration/test_workflow_catalog_data_storage_integration.py -v"
echo ""

