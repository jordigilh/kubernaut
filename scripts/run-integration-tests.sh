#!/bin/bash

# Integration Test Runner Script
# This script manages the complete integration test lifecycle including containerized services

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"
INTEGRATION_BOOTSTRAP="${PROJECT_ROOT}/test/integration/scripts/bootstrap-integration-tests.sh"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Function to check if integration services are running
check_integration_services() {
    local bootstrap_script="$INTEGRATION_BOOTSTRAP"

    if [ ! -f "$bootstrap_script" ]; then
        log_error "Integration bootstrap script not found: $bootstrap_script"
        return 1
    fi

    # Check if services are running by querying status
    if ! "$bootstrap_script" status >/dev/null 2>&1; then
        return 1
    fi

    return 0
}

# Function to start integration services
start_services() {
    log_info "Starting integration test services..."

    if ! "$INTEGRATION_BOOTSTRAP" start; then
        log_error "Failed to start integration test services"
        return 1
    fi

    log_success "Integration test services started successfully"
    return 0
}

# Function to stop integration services
stop_services() {
    log_info "Stopping integration test services..."

    if ! "$INTEGRATION_BOOTSTRAP" stop; then
        log_error "Failed to stop integration test services"
        return 1
    fi

    log_success "Integration test services stopped successfully"
    return 0
}

# Function to run integration tests
run_tests() {
    local test_path="${1:-./test/integration/...}"
    local extra_args="${2:-}"

    cd "$PROJECT_ROOT"

    log_info "Running integration tests: $test_path"

    # Set environment variables for containerized databases
    export USE_CONTAINER_DB=true
    export DB_HOST=localhost
    export DB_PORT=5433
    export DB_NAME=action_history
    export DB_USER=slm_user
    export DB_PASSWORD=slm_password_dev
    export DB_SSL_MODE=disable

    export VECTOR_DB_HOST=localhost
    export VECTOR_DB_PORT=5434
    export VECTOR_DB_NAME=vector_store
    export VECTOR_DB_USER=vector_user
    export VECTOR_DB_PASSWORD=vector_password_dev

    # Run the tests
    if go test "$test_path" -tags=integration -v $extra_args; then
        log_success "Integration tests completed successfully"
        return 0
    else
        log_error "Integration tests failed"
        return 1
    fi
}

# Function to run tests with automatic service management
run_tests_with_services() {
    local test_path="${1:-./test/integration/...}"
    local extra_args="${2:-}"
    local cleanup_after="${3:-true}"

    log_info "🚀 Starting complete integration test run..."

    # Start services if not already running
    if ! check_integration_services; then
        log_info "Integration services not running, starting them..."
        if ! start_services; then
            log_error "Failed to start integration services"
            exit 1
        fi
        local started_services=true
    else
        log_info "Integration services already running"
        local started_services=false
    fi

    # Wait a moment for services to be fully ready
    sleep 3

    # Run the tests
    local test_result=0
    run_tests "$test_path" "$extra_args" || test_result=$?

    # Cleanup if we started the services and cleanup is requested
    if [ "$started_services" = true ] && [ "$cleanup_after" = true ]; then
        log_info "Cleaning up integration services..."
        stop_services
    fi

    if [ $test_result -eq 0 ]; then
        log_success "🎉 Integration test run completed successfully!"
    else
        log_error "❌ Integration test run failed"
    fi

    exit $test_result
}

# Function to show usage
show_usage() {
    echo "Integration Test Runner"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  start-services              - Start integration test services (PostgreSQL, Vector DB, Redis)"
    echo "  stop-services               - Stop integration test services"
    echo "  status                      - Show status of integration test services"
    echo "  test [path] [args]          - Run integration tests (requires services to be running)"
    echo "  test-with-services [path]   - Run integration tests with automatic service management"
    echo "  test-infrastructure         - Run infrastructure integration tests only"
    echo "  test-performance            - Run performance integration tests only"
    echo "  test-vector                 - Run vector database integration tests only"
    echo "  test-all                    - Run all integration tests with automatic service management"
    echo ""
    echo "Examples:"
    echo "  $0 start-services                           # Start all integration services"
    echo "  $0 test ./test/integration/infrastructure   # Run infrastructure tests (services must be running)"
    echo "  $0 test-with-services                       # Run all tests with automatic service management"
    echo "  $0 test-infrastructure                      # Run infrastructure tests only"
    echo "  $0 test-vector                             # Run vector database tests only"
    echo ""
    echo "Environment Variables:"
    echo "  USE_CONTAINER_DB=true                       # Use containerized databases (default)"
    echo "  SKIP_DB_TESTS=false                        # Don't skip database tests (default)"
    echo "  SKIP_SLM_TESTS=true                        # Skip SLM tests (optional)"
    echo "  LOG_LEVEL=debug                            # Set log level"
    echo ""
}

# Main script logic
case "${1:-help}" in
    start-services)
        start_services
        ;;
    stop-services)
        stop_services
        ;;
    status)
        "$INTEGRATION_BOOTSTRAP" status
        ;;
    test)
        run_tests "${2:-./test/integration/...}" "${3:-}"
        ;;
    test-with-services)
        run_tests_with_services "${2:-./test/integration/...}" "${3:-}"
        ;;
    test-infrastructure)
        run_tests_with_services "./test/integration/infrastructure" "" false
        ;;
    test-performance)
        run_tests_with_services "./test/integration/performance" "" false
        ;;
    test-vector)
        run_tests_with_services "./test/integration/vector" "" false
        ;;
    test-all)
        run_tests_with_services "./test/integration/..." "" true
        ;;
    logs)
        "$INTEGRATION_BOOTSTRAP" logs "${2:-}"
        ;;
    help|--help|-h)
        show_usage
        ;;
    *)
        log_error "Unknown command: $1"
        echo ""
        show_usage
        exit 1
        ;;
esac
