#!/bin/bash

# Gateway Integration Test Runner
# Sets up port-forward to OCP Redis and runs Gateway integration tests

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

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

# PID file for port-forward cleanup
PID_FILE="/tmp/gateway-redis-port-forward.pid"

# Cleanup function
cleanup() {
    if [ -f "$PID_FILE" ]; then
        log_info "Stopping Redis port-forward..."
        kill $(cat "$PID_FILE") 2>/dev/null || true
        rm -f "$PID_FILE"
        log_success "Port-forward stopped"
    fi
}

# Register cleanup on exit
trap cleanup EXIT

# Check if Redis is available in OCP cluster
check_redis_available() {
    log_info "Checking Redis in kubernaut-system namespace..."

    if ! kubectl get svc redis -n kubernaut-system &>/dev/null; then
        log_error "Redis service not found in kubernaut-system namespace"
        log_info "Expected: kubectl get svc redis -n kubernaut-system"
        return 1
    fi

    log_success "Redis service found in kubernaut-system"
    return 0
}

# Setup port-forward to Redis
setup_port_forward() {
    log_info "Setting up port-forward to Redis (localhost:6379 -> kubernaut-system/redis:6379)..."

    # Kill any existing port-forward on 6379
    lsof -ti:6379 | xargs kill -9 2>/dev/null || true

    # Start port-forward in background
    kubectl port-forward -n kubernaut-system svc/redis 6379:6379 &>/dev/null &
    echo $! > "$PID_FILE"

    # Wait for port-forward to be ready
    log_info "Waiting for port-forward to be ready..."
    for i in {1..10}; do
        if nc -z localhost 6379 2>/dev/null; then
            log_success "Port-forward ready!"
            return 0
        fi
        sleep 1
    done

    log_error "Port-forward failed to start"
    return 1
}

# Run integration tests
run_integration_tests() {
    log_info "Running Gateway integration tests..."
    echo ""

    cd "$(dirname "$0")/.."
    go test -v ./test/integration/gateway/... -timeout 2m

    local exit_code=$?
    echo ""

    if [ $exit_code -eq 0 ]; then
        log_success "Integration tests passed!"
    else
        log_error "Integration tests failed (exit code: $exit_code)"
    fi

    return $exit_code
}

# Main function
main() {
    log_info "Gateway Integration Test Runner"
    echo ""

    # Check if Redis is available
    if ! check_redis_available; then
        log_error "Cannot run integration tests without Redis"
        log_info "Deploy Redis: kubectl apply -f deploy/context-api/redis-deployment.yaml"
        exit 1
    fi

    # Setup port-forward
    if ! setup_port_forward; then
        log_error "Failed to setup port-forward"
        exit 1
    fi

    echo ""
    log_info "📋 Test Environment:"
    echo "  • Redis:     localhost:6379 → kubernaut-system/redis:6379"
    echo "  • Namespace: kubernaut-system"
    echo "  • Test DB:   1 (isolated from production)"
    echo ""

    # Run tests
    run_integration_tests
    exit_code=$?

    echo ""
    log_info "Cleaning up..."

    exit $exit_code
}

# Help function
show_help() {
    cat <<EOF
Gateway Integration Test Runner

Usage: $0 [OPTIONS]

Runs Gateway integration tests against Redis in OCP cluster (kubernaut-system namespace).

Prerequisites:
  - Redis deployed in kubernaut-system namespace
  - kubectl configured to access OCP cluster
  - Port 6379 available on localhost

Options:
  -h, --help    Show this help message

Examples:
  # Run integration tests
  $0

  # Skip integration tests (e.g., in CI without Redis)
  SKIP_REDIS_INTEGRATION=true go test ./test/integration/gateway/...

Manual port-forward (if script fails):
  kubectl port-forward -n kubernaut-system svc/redis 6379:6379

EOF
}

# Parse arguments
case "${1:-}" in
    -h|--help)
        show_help
        exit 0
        ;;
    *)
        main
        ;;
esac



