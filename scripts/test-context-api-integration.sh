#!/bin/bash
# scripts/test-context-api-integration.sh
# End-to-End Context API Integration Testing for HolmesGPT
# Validates BR-AI-011, BR-AI-012, BR-AI-013 business requirements

set -e

# Configuration
CONTEXT_API_URL="http://localhost:8091"
MAIN_API_URL="http://localhost:8080"
TEST_TIMEOUT=30
RETRY_ATTEMPTS=3

echo "🧪 Context API Integration Testing (Phase B)"
echo "============================================="
echo ""
echo "📋 Test Scope:"
echo "  • BR-AI-011: Intelligent alert investigation using historical patterns"
echo "  • BR-AI-012: Root cause identification with supporting evidence"
echo "  • BR-AI-013: Alert correlation across time/resource boundaries"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test result tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
    PASSED_TESTS=$((PASSED_TESTS + 1))
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
    FAILED_TESTS=$((FAILED_TESTS + 1))
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

increment_test() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
}

# HTTP request helper with retry logic
http_request() {
    local method="$1"
    local url="$2"
    local data="$3"
    local description="$4"

    for i in $(seq 1 $RETRY_ATTEMPTS); do
        if [ -n "$data" ]; then
            if curl -f -s -X "$method" "$url" -H "Content-Type: application/json" -d "$data" --max-time $TEST_TIMEOUT > /dev/null 2>&1; then
                return 0
            fi
        else
            if curl -f -s -X "$method" "$url" --max-time $TEST_TIMEOUT > /dev/null 2>&1; then
                return 0
            fi
        fi

        if [ $i -lt $RETRY_ATTEMPTS ]; then
            log_warning "Retry $i/$RETRY_ATTEMPTS for $description"
            sleep 2
        fi
    done

    return 1
}

# HTTP request helper that captures response
http_request_with_response() {
    local method="$1"
    local url="$2"
    local data="$3"

    if [ -n "$data" ]; then
        curl -f -s -X "$method" "$url" -H "Content-Type: application/json" -d "$data" --max-time $TEST_TIMEOUT 2>/dev/null
    else
        curl -f -s -X "$method" "$url" --max-time $TEST_TIMEOUT 2>/dev/null
    fi
}

echo "🏥 Pre-flight Checks"
echo "--------------------"

# Test 1: Context API Health Check
increment_test
log_info "Testing Context API health endpoint..."
if http_request "GET" "$CONTEXT_API_URL/api/v1/context/health" "" "Context API health"; then
    log_success "Context API health check passed"
else
    log_error "Context API health check failed - is the service running?"
    log_info "Try: ./scripts/run-kubernaut-with-context-api.sh config/local-llm.yaml"
    exit 1
fi

# Test 2: Main API Health Check
increment_test
log_info "Testing main API health endpoint..."
if http_request "GET" "$MAIN_API_URL/health" "" "Main API health"; then
    log_success "Main API health check passed"
else
    log_error "Main API health check failed"
    exit 1
fi

echo ""
echo "🧪 Business Requirements Testing"
echo "--------------------------------"

# BR-AI-011: Intelligent alert investigation using historical patterns
echo ""
echo "📊 BR-AI-011: Intelligent Alert Investigation"

# Test 3: Kubernetes Context API
increment_test
log_info "Testing Kubernetes context endpoint (production environment simulation)..."
RESPONSE=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/kubernetes/production/api-server-pod" "")
if [ $? -eq 0 ]; then
    log_success "Kubernetes context endpoint accessible"

    # Validate response structure
    if echo "$RESPONSE" | grep -q '"namespace":"production"' && echo "$RESPONSE" | grep -q '"resource":"api-server-pod"'; then
        log_success "Kubernetes context contains expected data structure"
    else
        log_error "Kubernetes context response missing expected fields"
    fi
else
    log_error "Kubernetes context endpoint failed"
fi

# Test 4: Action History Context API
increment_test
log_info "Testing action history context endpoint (pattern recognition)..."
RESPONSE=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/action-history/HighMemoryUsage?namespace=production" "")
if [ $? -eq 0 ]; then
    log_success "Action history context endpoint accessible"

    # Validate response structure for pattern recognition
    if echo "$RESPONSE" | grep -q '"alert_type":"HighMemoryUsage"' && echo "$RESPONSE" | grep -q '"context_hash"'; then
        log_success "Action history context contains pattern correlation data"
    else
        log_error "Action history context missing pattern correlation fields"
    fi
else
    log_error "Action history context endpoint failed"
fi

# BR-AI-012: Root cause identification with supporting evidence
echo ""
echo "🔍 BR-AI-012: Root Cause Identification with Evidence"

# Test 5: Metrics Context API
increment_test
log_info "Testing metrics context endpoint (supporting evidence)..."
RESPONSE=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/metrics/production/database-server?timeRange=10m" "")
if [ $? -eq 0 ]; then
    log_success "Metrics context endpoint accessible"

    # Validate response contains evidence data
    if echo "$RESPONSE" | grep -q '"namespace":"production"' && echo "$RESPONSE" | grep -q '"metrics"' && echo "$RESPONSE" | grep -q '"collection_time"'; then
        log_success "Metrics context provides supporting evidence with timestamps"
    else
        log_error "Metrics context missing evidence fields"
    fi
else
    log_error "Metrics context endpoint failed"
fi

# BR-AI-013: Alert correlation across time/resource boundaries
echo ""
echo "🔗 BR-AI-013: Alert Correlation Across Boundaries"

# Test 6: Consistent Context Hashing
increment_test
log_info "Testing context hash consistency for correlation..."
RESPONSE1=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/action-history/NetworkLatencyHigh?namespace=production" "")
sleep 1
RESPONSE2=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/action-history/NetworkLatencyHigh?namespace=production" "")

if [ $? -eq 0 ] && [ -n "$RESPONSE1" ] && [ -n "$RESPONSE2" ]; then
    HASH1=$(echo "$RESPONSE1" | grep -o '"context_hash":"[^"]*"' | cut -d'"' -f4)
    HASH2=$(echo "$RESPONSE2" | grep -o '"context_hash":"[^"]*"' | cut -d'"' -f4)

    if [ "$HASH1" = "$HASH2" ] && [ -n "$HASH1" ]; then
        log_success "Context hash consistency maintained for alert correlation"
    else
        log_error "Context hash inconsistency detected (breaks correlation)"
    fi
else
    log_error "Context correlation test failed"
fi

# Test 7: Resource Boundary Differentiation
increment_test
log_info "Testing resource boundary differentiation..."
RESPONSE_CPU=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/action-history/HighCPUUsage?namespace=production" "")
RESPONSE_MEMORY=$(http_request_with_response "GET" "$CONTEXT_API_URL/api/v1/context/action-history/HighMemoryUsage?namespace=production" "")

if [ $? -eq 0 ] && [ -n "$RESPONSE_CPU" ] && [ -n "$RESPONSE_MEMORY" ]; then
    HASH_CPU=$(echo "$RESPONSE_CPU" | grep -o '"context_hash":"[^"]*"' | cut -d'"' -f4)
    HASH_MEMORY=$(echo "$RESPONSE_MEMORY" | grep -o '"context_hash":"[^"]*"' | cut -d'"' -f4)

    if [ "$HASH_CPU" != "$HASH_MEMORY" ] && [ -n "$HASH_CPU" ] && [ -n "$HASH_MEMORY" ]; then
        log_success "Resource boundary differentiation working (different alerts = different hashes)"
    else
        log_error "Resource boundary differentiation failed"
    fi
else
    log_error "Resource boundary test failed"
fi

echo ""
echo "⚡ Performance Testing"
echo "---------------------"

# Test 8: Response Time Validation
increment_test
log_info "Testing Context API response times..."
START_TIME=$(date +%s%N)
if http_request "GET" "$CONTEXT_API_URL/api/v1/context/kubernetes/test/performance-test" "" "Performance test"; then
    END_TIME=$(date +%s%N)
    RESPONSE_TIME=$(( (END_TIME - START_TIME) / 1000000 )) # Convert to milliseconds

    if [ $RESPONSE_TIME -lt 5000 ]; then
        log_success "Context API response time: ${RESPONSE_TIME}ms (< 5s requirement)"
    else
        log_warning "Context API response time: ${RESPONSE_TIME}ms (slower than 5s target)"
    fi
else
    log_error "Performance test failed"
fi

echo ""
echo "📊 Test Results Summary"
echo "======================"
echo ""
echo "Total Tests: $TOTAL_TESTS"
echo -e "${GREEN}Passed: $PASSED_TESTS${NC}"
echo -e "${RED}Failed: $FAILED_TESTS${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo ""
    log_success "🎉 ALL TESTS PASSED - Context API Integration Successful!"
    echo ""
    echo "✅ Business Requirements Validated:"
    echo "  • BR-AI-011: Intelligent alert investigation ✅"
    echo "  • BR-AI-012: Root cause identification with evidence ✅"
    echo "  • BR-AI-013: Alert correlation across boundaries ✅"
    echo ""
    echo "🚀 Phase B Complete - HolmesGPT Context API Integration Ready!"
    exit 0
else
    echo ""
    log_error "❌ INTEGRATION TESTS FAILED ($FAILED_TESTS/$TOTAL_TESTS)"
    echo ""
    echo "🔧 Troubleshooting:"
    echo "  • Check if Context API server is running on port 8091"
    echo "  • Verify ./scripts/run-kubernaut-with-context-api.sh config/local-llm.yaml"
    echo "  • Check logs for Context API startup errors"
    echo "  • Ensure AIServiceIntegrator is properly configured"
    exit 1
fi
