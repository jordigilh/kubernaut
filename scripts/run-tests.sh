#!/bin/bash

# Integration Test Runner for Development Environment
# Assumes environment has been bootstrapped with bootstrap-dev-environment.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Load environment configuration
load_environment() {
    local env_file="${PROJECT_ROOT}/.env.development"

    if [ -f "$env_file" ]; then
        log_info "Loading development environment configuration..."
        source "$env_file"
        log_success "Environment configuration loaded"
    else
        log_error "Development environment not found: $env_file"
        echo ""
        echo "Please run the bootstrap script first:"
        echo "  ./scripts/bootstrap-dev-environment.sh"
        exit 1
    fi
}

# Verify environment is ready
verify_environment() {
    log_info "Verifying environment is ready for testing..."

    local verification_failed=false

    # Check database connections using container clients
    if ! podman exec kubernaut-integration-postgres psql -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        log_error "Main database not accessible"
        verification_failed=true
    fi

    if ! podman exec kubernaut-integration-vectordb psql -U "$VECTOR_DB_USER" -d "$VECTOR_DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        log_error "Vector database not accessible"
        verification_failed=true
    fi

    # Check Redis using container client
    if ! podman exec kubernaut-integration-redis redis-cli ping >/dev/null 2>&1; then
        log_error "Redis not accessible"
        verification_failed=true
    fi

    # Check Kubernetes
    if ! kubectl get nodes >/dev/null 2>&1; then
        log_error "Kubernetes cluster not accessible"
        verification_failed=true
    fi

    # Check LLM
    if ! curl -s --connect-timeout 5 "$LLM_ENDPOINT/v1/models" >/dev/null 2>&1; then
        log_error "LLM service not accessible at $LLM_ENDPOINT"
        verification_failed=true
    fi

    if [ "$verification_failed" = true ]; then
        log_error "Environment verification failed"
        echo ""
        echo "Please ensure the development environment is running:"
        echo "  ./scripts/bootstrap-dev-environment.sh"
        exit 1
    fi

    log_success "Environment verification passed"
}

# Run integration tests
run_integration_tests() {
    local test_category="${1:-all}"
    local test_args="${2:-}"

    cd "$PROJECT_ROOT"

    case "$test_category" in
        all)
            log_info "Running all integration tests..."
            go test ./test/integration/... -tags=integration -v $test_args
            ;;
        ai)
            log_info "Running AI integration tests..."
            go test ./test/integration/ai/... -tags=integration -v $test_args
            ;;
        infrastructure)
            log_info "Running infrastructure integration tests..."
            go test ./test/integration/infrastructure_integration/... -tags=integration -v $test_args
            ;;
        performance)
            log_info "Running performance integration tests..."
            go test ./test/integration/performance_scale/... -tags=integration -v $test_args
            ;;
        production)
            log_info "Running production readiness tests..."
            go test ./test/integration/production_readiness/... -tags=integration -v $test_args
            ;;
        e2e)
            log_info "Running end-to-end tests..."
            go test ./test/integration/end_to_end/... -tags=integration -v $test_args
            ;;
        validation)
            log_info "Running validation quality tests..."
            go test ./test/integration/validation_quality/... -tags=integration -v $test_args
            ;;
        vector)
            log_info "Running vector database tests..."
            go test ./test/integration/infrastructure_integration/vector_database_test.go -tags=integration -v $test_args
            go test ./test/integration/infrastructure_integration/performance_and_resilience_test.go -tags=integration -v $test_args
            ;;
        cache)
            log_info "Running cache tests..."
            go test ./test/integration/infrastructure_integration/redis_cache_*_test.go -tags=integration -v $test_args
            go test ./test/integration/infrastructure_integration/cache_hit_ratio_validation_test.go -tags=integration -v $test_args
            ;;
        monitoring)
            log_info "Running monitoring integration tests..."
            go test ./test/integration/infrastructure_integration/monitoring_integration_test.go -tags=integration -v $test_args
            ;;
        security)
            log_info "Running security validation tests..."
            go test ./test/integration/infrastructure_integration/security_validation_test.go -tags=integration -v $test_args
            ;;
        quick)
            log_info "Running quick integration tests (excluding slow tests)..."
            export SKIP_SLOW_TESTS=true
            go test ./test/integration/ai/ai_integration_validation_test.go -tags=integration -v $test_args
            go test ./test/integration/infrastructure_integration/simple_debug_test.go -tags=integration -v $test_args
            ;;
        *)
            log_error "Unknown test category: $test_category"
            show_usage
            exit 1
            ;;
    esac
}

# Show test results summary
show_test_results() {
    local exit_code=$1

    echo ""
    if [ $exit_code -eq 0 ]; then
        log_success "🎉 All integration tests passed!"
    else
        log_error "❌ Some integration tests failed"
    fi

    echo ""
    echo "📊 Test Environment Status:"
    echo "  • Main Database:    $(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT count(*) || ' tables' FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null || echo 'Disconnected')"
    echo "  • Vector Database:  $(PGPASSWORD="$VECTOR_DB_PASSWORD" psql -h "$VECTOR_DB_HOST" -p "$VECTOR_DB_PORT" -U "$VECTOR_DB_USER" -d "$VECTOR_DB_NAME" -t -c "SELECT count(*) || ' patterns' FROM action_patterns;" 2>/dev/null || echo 'Disconnected')"
    echo "  • Kubernetes:       $(kubectl get nodes --no-headers 2>/dev/null | wc -l | xargs) nodes"
    echo "  • LLM Service:      $(curl -s "$LLM_ENDPOINT/v1/models" | jq -r '.data | length' 2>/dev/null || echo '0') models"
    echo ""
}

# Show usage information
show_usage() {
    echo "Integration Test Runner for Kubernaut Development Environment"
    echo ""
    echo "Usage: $0 [category] [options]"
    echo ""
    echo "Categories:"
    echo "  all               - Run all integration tests (default)"
    echo "  ai                - AI integration tests only"
    echo "  infrastructure    - Infrastructure integration tests"
    echo "  performance       - Performance and scale tests"
    echo "  production        - Production readiness tests"
    echo "  e2e               - End-to-end integration tests"
    echo "  validation        - Validation quality tests"
    echo "  vector            - Vector database tests"
    echo "  cache             - Redis cache tests"
    echo "  monitoring        - Monitoring integration tests"
    echo "  security          - Security validation tests"
    echo "  quick             - Quick tests (excludes slow tests)"
    echo ""
    echo "Options:"
    echo "  --verbose         - Verbose test output"
    echo "  --failfast        - Stop on first test failure"
    echo "  --count N         - Run tests N times"
    echo "  --run PATTERN     - Run only tests matching pattern"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Run all tests"
    echo "  $0 ai                                 # Run AI tests only"
    echo "  $0 infrastructure --verbose          # Verbose infrastructure tests"
    echo "  $0 quick                             # Quick test run"
    echo "  $0 all --run TestSpecificFunction    # Run specific test"
    echo ""
    echo "Prerequisites:"
    echo "  Run bootstrap script first: ./scripts/bootstrap-dev-environment.sh"
    echo ""
}

# Main execution
main() {
    local start_time=$(date +%s)

    # Parse command line arguments
    local category="all"
    local test_args=""

    while [ $# -gt 0 ]; do
        case $1 in
            --help|-h)
                show_usage
                exit 0
                ;;
            --verbose)
                test_args="$test_args -v"
                shift
                ;;
            --failfast)
                test_args="$test_args -failfast"
                shift
                ;;
            --count)
                test_args="$test_args -count $2"
                shift 2
                ;;
            --run)
                test_args="$test_args -run $2"
                shift 2
                ;;
            all|ai|infrastructure|performance|production|e2e|validation|vector|cache|monitoring|security|quick)
                category="$1"
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    echo "🧪 Kubernaut Integration Test Runner"
    echo "===================================="
    echo ""
    echo "Test Category: $category"
    echo "Test Arguments: ${test_args:-none}"
    echo ""

    # Load environment and verify
    load_environment
    verify_environment

    # Run tests
    local test_exit_code=0
    run_integration_tests "$category" "$test_args" || test_exit_code=$?

    # Show results
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    echo ""
    log_info "Test run completed in ${duration} seconds"

    show_test_results $test_exit_code

    exit $test_exit_code
}

# Execute main function with all arguments
main "$@"
