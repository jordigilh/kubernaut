#!/bin/bash
# Gateway Integration Test Runner
# Automatically sets up Redis port-forward and runs integration tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REDIS_POD_NAME="redis-75cfb58d99-s8vwp"
REDIS_NAMESPACE="kubernaut-system"
REDIS_LOCAL_PORT=6379
TEST_TIMEOUT=600

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}🚀 Gateway Integration Test Runner${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Function to cleanup on exit
cleanup() {
    echo ""
    echo -e "${YELLOW}🧹 Cleaning up...${NC}"

    # Kill Redis port-forward if we started it
    if [ ! -z "$REDIS_PF_PID" ]; then
        echo "  Stopping Redis port-forward (PID: $REDIS_PF_PID)"
        kill $REDIS_PF_PID 2>/dev/null || true
    fi

    # Kill any orphaned port-forwards
    pkill -f "kubectl port-forward.*redis" 2>/dev/null || true

    echo -e "${GREEN}✅ Cleanup complete${NC}"
}

# Register cleanup on exit
trap cleanup EXIT INT TERM

# Step 1: Check if Redis pod exists
echo -e "${BLUE}📋 Step 1: Checking Redis pod...${NC}"
if ! kubectl get pod -n $REDIS_NAMESPACE $REDIS_POD_NAME &>/dev/null; then
    echo -e "${RED}❌ Redis pod not found: $REDIS_POD_NAME${NC}"
    echo "   Available Redis pods:"
    kubectl get pods -n $REDIS_NAMESPACE | grep redis || echo "   No Redis pods found"
    echo ""
    echo "   Please update REDIS_POD_NAME in this script with the correct pod name."
    exit 1
fi
echo -e "${GREEN}✅ Redis pod found: $REDIS_POD_NAME${NC}"
echo ""

# Step 2: Kill any existing port-forwards
echo -e "${BLUE}📋 Step 2: Cleaning up existing port-forwards...${NC}"
pkill -f "kubectl port-forward.*redis" 2>/dev/null || true
sleep 1
echo -e "${GREEN}✅ Existing port-forwards cleaned up${NC}"
echo ""

# Step 3: Start Redis port-forward
echo -e "${BLUE}📋 Step 3: Starting Redis port-forward...${NC}"
kubectl port-forward -n $REDIS_NAMESPACE $REDIS_POD_NAME $REDIS_LOCAL_PORT:6379 &>/dev/null &
REDIS_PF_PID=$!
echo "   Port-forward started (PID: $REDIS_PF_PID)"
echo "   Waiting for port-forward to be ready..."
sleep 3
echo -e "${GREEN}✅ Redis port-forward ready${NC}"
echo ""

# Step 4: Verify Redis connectivity
echo -e "${BLUE}📋 Step 4: Verifying Redis connectivity...${NC}"
if command -v redis-cli &>/dev/null; then
    if redis-cli -h localhost -p $REDIS_LOCAL_PORT ping &>/dev/null; then
        echo -e "${GREEN}✅ Redis is accessible (PONG received)${NC}"
    else
        echo -e "${YELLOW}⚠️  Redis ping failed, but continuing anyway${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  redis-cli not installed, skipping ping test${NC}"
fi
echo ""

# Step 5: Run integration tests
echo -e "${BLUE}📋 Step 5: Running integration tests...${NC}"
echo "   Timeout: ${TEST_TIMEOUT}s"
echo "   Test package: ./test/integration/gateway"
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Change to project root
cd "$(dirname "$0")/../../.."

# Run tests with timeout
if timeout $TEST_TIMEOUT go test -v ./test/integration/gateway -run "TestGatewayIntegration" 2>&1 | tee /tmp/gateway-integration-tests.log; then
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}✅ Integration tests PASSED${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 0
else
    TEST_EXIT_CODE=$?
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    if [ $TEST_EXIT_CODE -eq 124 ]; then
        echo -e "${RED}❌ Integration tests TIMED OUT after ${TEST_TIMEOUT}s${NC}"
    else
        echo -e "${RED}❌ Integration tests FAILED (exit code: $TEST_EXIT_CODE)${NC}"
    fi
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "${YELLOW}📄 Full test log saved to: /tmp/gateway-integration-tests.log${NC}"
    echo ""
    echo -e "${YELLOW}💡 Troubleshooting tips:${NC}"
    echo "   1. Check Redis connectivity: redis-cli -h localhost -p $REDIS_LOCAL_PORT ping"
    echo "   2. Check Redis pod logs: kubectl logs -n $REDIS_NAMESPACE $REDIS_POD_NAME"
    echo "   3. Check Gateway logs: kubectl logs -n kubernaut-system deployment/gateway"
    echo "   4. Review test log: tail -100 /tmp/gateway-integration-tests.log"
    exit $TEST_EXIT_CODE
fi


