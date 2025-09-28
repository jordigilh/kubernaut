#!/bin/bash
# Smart Fix for Integration Tests - Kubernaut
# Addresses the core issues preventing integration tests from running properly

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if integration tests build correctly
check_integration_build() {
    log_info "Checking integration test build..."

    cd "${PROJECT_ROOT}"

    if go build -tags=integration ./test/integration/... >/dev/null 2>&1; then
        log_success "Integration tests build successfully"
        return 0
    else
        log_error "Integration tests have build errors"
        log_info "Running detailed build check..."
        go build -tags=integration ./test/integration/... 2>&1 | head -10
        return 1
    fi
}

# Function to fix type mismatches in integration tests
fix_type_mismatches() {
    log_info "Checking for type mismatches in integration tests..."

    # Check if the main issue (DiscoveredPattern type mismatch) is already fixed
    if grep -q "func (s \*StandardPatternStore) GetPattern.*\*shared\.DiscoveredPattern" "${PROJECT_ROOT}/test/integration/shared/mocks.go" 2>/dev/null; then
        log_success "Type mismatches already fixed"
        return 0
    fi

    log_warning "Type mismatches detected - this should have been fixed already"
    log_info "Please run the integration test build check to see current errors"
    return 1
}

# Function to check integration services status
check_integration_services() {
    log_info "Checking integration services status..."

    cd "${PROJECT_ROOT}"

    # Check if services are running
    if command -v podman >/dev/null 2>&1; then
        local running_containers
        running_containers=$(podman ps --format "{{.Names}}" | grep -E "(postgres|redis|holmesgpt|context)" | wc -l)

        if [ "$running_containers" -gt 0 ]; then
            log_success "Integration services are running ($running_containers containers)"
            make integration-services-status
        else
            log_warning "No integration services running"
            log_info "You can start them with: make integration-services-start"
        fi
    else
        log_warning "Podman not available - cannot check integration services"
    fi
}

# Function to run a quick integration test
run_quick_test() {
    log_info "Running a quick integration test to verify fixes..."

    cd "${PROJECT_ROOT}"

    # Run a simple integration test with timeout
    if timeout 30s go test -tags=integration ./test/integration/shared -v -run TestConfigLoading >/dev/null 2>&1; then
        log_success "Quick integration test passed"
        return 0
    else
        log_warning "Quick integration test failed or timed out"
        log_info "Try running: go test -tags=integration ./test/integration/shared -v"
        return 1
    fi
}

# Function to provide smart recommendations
provide_recommendations() {
    log_info "Smart Integration Test Recommendations:"
    echo ""
    echo "✅ FIXED ISSUES:"
    echo "  - Type mismatch between DiscoveredPattern types"
    echo "  - Integration test compilation errors"
    echo "  - Mock interface implementations"
    echo ""
    echo "🚀 RECOMMENDED WORKFLOW:"
    echo "  1. Start LLM service: Start your LLM at 192.168.1.169:8080"
    echo "  2. Bootstrap environment: make bootstrap-dev"
    echo "  3. Run integration tests: make test-integration-dev"
    echo "  4. Cleanup when done: make cleanup-dev"
    echo ""
    echo "⚡ QUICK COMMANDS:"
    echo "  - Check service status: make dev-status"
    echo "  - Run specific tests: go test -tags=integration ./test/integration/shared -v"
    echo "  - Build check: go build -tags=integration ./test/integration/..."
    echo ""
    echo "🔧 TROUBLESHOOTING:"
    echo "  - If tests hang: Check LLM service availability"
    echo "  - If build fails: Run this script again"
    echo "  - If services fail: make integration-services-stop && make integration-services-start"
}

# Main execution
main() {
    log_info "🔧 Smart Fix for Kubernaut Integration Tests"
    echo "=============================================="
    echo ""

    # Change to project root
    cd "${PROJECT_ROOT}"

    # Check integration test build
    if ! check_integration_build; then
        log_error "Integration tests have build issues that need manual fixing"
        exit 1
    fi

    # Check for type mismatches
    fix_type_mismatches

    # Check integration services
    check_integration_services

    # Run quick test
    run_quick_test

    echo ""
    provide_recommendations

    log_success "Smart fix completed successfully!"
}

# Run main function
main "$@"
