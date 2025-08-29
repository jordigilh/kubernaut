#!/bin/bash

# Database Integration Test Runner
# Sets up PostgreSQL database and runs integration tests with database functionality

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
POSTGRES_CONTAINER="prometheus-alerts-slm-postgres"
DB_NAME="action_history"
DB_USER="slm_user"
DB_PASSWORD="slm_password_dev"
DB_PORT="5432"

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

# Function to check if container exists
container_exists() {
    podman ps -a --format "{{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"
}

# Function to check if container is running
container_running() {
    podman ps --format "{{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"
}

# Function to wait for PostgreSQL to be ready
wait_for_postgres() {
    local max_attempts=30
    local attempt=1
    
    log_info "Waiting for PostgreSQL to be ready..."
    
    while [ $attempt -le $max_attempts ]; do
        if podman exec "$POSTGRES_CONTAINER" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
            log_success "PostgreSQL is ready!"
            return 0
        fi
        
        if [ $attempt -eq $max_attempts ]; then
            log_error "PostgreSQL failed to start within expected time"
            return 1
        fi
        
        log_info "Attempt $attempt/$max_attempts - waiting 2 seconds..."
        sleep 2
        ((attempt++))
    done
}

# Function to setup PostgreSQL database
setup_database() {
    log_info "Setting up PostgreSQL database for integration tests..."
    
    # Deploy PostgreSQL using our existing script
    if [ -f "$SCRIPT_DIR/deploy-postgres.sh" ]; then
        log_info "Running PostgreSQL deployment script..."
        bash "$SCRIPT_DIR/deploy-postgres.sh"
    else
        log_error "PostgreSQL deployment script not found at $SCRIPT_DIR/deploy-postgres.sh"
        return 1
    fi
    
    # Wait for PostgreSQL to be ready
    if ! wait_for_postgres; then
        return 1
    fi
    
    # Run database migrations
    log_info "Running database migrations..."
    cd "$PROJECT_ROOT"
    
    # Set environment variables for migration tool (if we had one)
    export DATABASE_URL="postgres://$DB_USER:$DB_PASSWORD@localhost:$DB_PORT/$DB_NAME?sslmode=disable"
    
    # For now, we'll let the integration tests handle migrations
    log_info "Database setup completed. Migrations will be handled by integration tests."
    
    return 0
}

# Function to cleanup database
cleanup_database() {
    log_info "Cleaning up database..."
    
    if container_running; then
        log_info "Stopping PostgreSQL container..."
        podman stop "$POSTGRES_CONTAINER" || true
    fi
    
    if container_exists; then
        log_info "Removing PostgreSQL container..."
        podman rm "$POSTGRES_CONTAINER" || true
    fi
    
    log_success "Database cleanup completed"
}

# Function to check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if podman is installed
    if ! command -v podman &> /dev/null; then
        log_error "Podman is not installed or not in PATH"
        return 1
    fi
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed or not in PATH"
        return 1
    fi
    
    # Check if we're in the right directory
    if [ ! -f "$PROJECT_ROOT/go.mod" ]; then
        log_error "go.mod not found. Please run this script from the project root."
        return 1
    fi
    
    log_success "Prerequisites check passed"
    return 0
}

# Function to run integration tests
run_integration_tests() {
    log_info "Running database integration tests..."
    
    cd "$PROJECT_ROOT"
    
    # Set environment variables for tests
    export DB_HOST="localhost"
    export DB_PORT="$DB_PORT"
    export DB_NAME="$DB_NAME"
    export DB_USER="$DB_USER"
    export DB_PASSWORD="$DB_PASSWORD"
    export DB_SSL_MODE="disable"
    
    # Set test configuration
    export OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://localhost:11434}"
    export OLLAMA_MODEL="${OLLAMA_MODEL:-granite3.1-dense:8b}"
    export TEST_TIMEOUT="${TEST_TIMEOUT:-120s}"
    export LOG_LEVEL="${LOG_LEVEL:-debug}"
    export SKIP_SLOW_TESTS="${SKIP_SLOW_TESTS:-false}"
    
    # Ensure Kubebuilder assets are available
    if [ -z "${KUBEBUILDER_ASSETS:-}" ]; then
        if [ -d "bin/k8s/1.33.0-darwin-arm64" ]; then
            export KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.33.0-darwin-arm64"
        else
            log_warning "KUBEBUILDER_ASSETS not set. Some tests may fail."
        fi
    fi
    
    # Run the tests
    local test_command="go test -v -tags=integration ./test/integration/... -timeout=30m"
    
    # Add specific test patterns if requested
    if [ "${RUN_SPECIFIC_TESTS:-}" != "" ]; then
        test_command="$test_command -run=$RUN_SPECIFIC_TESTS"
    fi
    
    log_info "Executing: $test_command"
    
    if eval "$test_command"; then
        log_success "Integration tests completed successfully!"
        return 0
    else
        log_error "Integration tests failed!"
        return 1
    fi
}

# Function to run specific database test contexts
run_database_tests() {
    log_info "Running specific database integration test contexts..."
    
    local contexts=(
        "TestDatabaseIntegration"
        "TestOscillationDetectionIntegration" 
        "TestMCPDatabaseIntegration"
        "TestWorkflowDatabaseIntegration"
    )
    
    local failed_contexts=()
    
    for context in "${contexts[@]}"; do
        log_info "Running context: $context"
        
        if RUN_SPECIFIC_TESTS="$context" run_integration_tests; then
            log_success "Context $context passed"
        else
            log_error "Context $context failed"
            failed_contexts+=("$context")
        fi
    done
    
    if [ ${#failed_contexts[@]} -eq 0 ]; then
        log_success "All database integration test contexts passed!"
        return 0
    else
        log_error "Failed contexts: ${failed_contexts[*]}"
        return 1
    fi
}

# Function to show usage
show_usage() {
    cat << EOF
Database Integration Test Runner

Usage: $0 [OPTIONS] [COMMAND]

Commands:
    setup       Setup PostgreSQL database only
    test        Run integration tests only (requires existing database)
    full        Setup database and run tests (default)
    contexts    Run specific database test contexts
    cleanup     Cleanup database containers
    help        Show this help message

Options:
    --skip-setup        Skip database setup (assume database is running)
    --skip-cleanup      Skip cleanup after tests
    --specific=PATTERN  Run only tests matching PATTERN
    --ollama-endpoint   Ollama endpoint (default: http://localhost:11434)
    --ollama-model      Ollama model (default: granite3.1-dense:8b)
    --skip-slow         Skip slow tests

Environment Variables:
    OLLAMA_ENDPOINT     Ollama server endpoint
    OLLAMA_MODEL        Model to use for tests
    SKIP_SLOW_TESTS     Skip slow tests (true/false)
    TEST_TIMEOUT        Test timeout duration
    LOG_LEVEL           Logging level (debug, info, warn, error)

Examples:
    $0                                  # Setup database and run all tests
    $0 contexts                         # Run only database-specific test contexts
    $0 test --skip-setup               # Run tests with existing database
    $0 --specific="TestDatabaseConnectivity"  # Run specific test
    $0 full --skip-slow                # Run all tests but skip slow ones

EOF
}

# Main execution function
main() {
    local command="full"
    local skip_setup=false
    local skip_cleanup=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            setup|test|full|contexts|cleanup|help)
                command="$1"
                shift
                ;;
            --skip-setup)
                skip_setup=true
                shift
                ;;
            --skip-cleanup)
                skip_cleanup=true
                shift
                ;;
            --specific=*)
                export RUN_SPECIFIC_TESTS="${1#*=}"
                shift
                ;;
            --ollama-endpoint=*)
                export OLLAMA_ENDPOINT="${1#*=}"
                shift
                ;;
            --ollama-model=*)
                export OLLAMA_MODEL="${1#*=}"
                shift
                ;;
            --skip-slow)
                export SKIP_SLOW_TESTS="true"
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    # Handle help command
    if [ "$command" = "help" ]; then
        show_usage
        exit 0
    fi
    
    # Check prerequisites
    if ! check_prerequisites; then
        exit 1
    fi
    
    # Setup cleanup trap
    if [ "$skip_cleanup" = false ] && [ "$command" != "cleanup" ]; then
        trap cleanup_database EXIT
    fi
    
    # Execute requested command
    case $command in
        setup)
            setup_database
            ;;
        test)
            if [ "$skip_setup" = false ]; then
                log_warning "Running tests without explicit --skip-setup. Database should already be running."
            fi
            run_integration_tests
            ;;
        full)
            if [ "$skip_setup" = false ]; then
                setup_database || exit 1
            fi
            run_integration_tests
            ;;
        contexts)
            if [ "$skip_setup" = false ]; then
                setup_database || exit 1
            fi
            run_database_tests
            ;;
        cleanup)
            cleanup_database
            ;;
        *)
            log_error "Unknown command: $command"
            show_usage
            exit 1
            ;;
    esac
}

# Script entry point
log_info "Starting Database Integration Test Runner..."
log_info "Project root: $PROJECT_ROOT"

main "$@"