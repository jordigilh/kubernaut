#!/bin/bash

# Intelligent Workflow Builder Integration Test Runner
# Follows the established testing framework patterns

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_DIR="${PROJECT_ROOT}/test/integration"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default configuration
LLM_PROVIDER="${LLM_PROVIDER:-ollama}"
LLM_MODEL="${LLM_MODEL:-granite3.1-dense:8b}"
LLM_ENDPOINT="${LLM_ENDPOINT:-http://localhost:11434}"
LOG_LEVEL="${LOG_LEVEL:-info}"
TEST_TIMEOUT="${TEST_TIMEOUT:-5m}"

# Test control flags
SKIP_SLM_TESTS="${SKIP_SLM_TESTS:-false}"
SKIP_PERFORMANCE_TESTS="${SKIP_PERFORMANCE_TESTS:-false}"
SKIP_SLOW_TESTS="${SKIP_SLOW_TESTS:-false}"

# Output control
VERBOSE="${VERBOSE:-false}"
REPORT_FILE="${REPORT_FILE:-intelligent_workflow_builder_integration_report.txt}"

# Function to print colored output
print_status() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

print_header() {
    echo
    print_status $BLUE "=================================================="
    print_status $BLUE "$1"
    print_status $BLUE "=================================================="
}

print_success() {
    print_status $GREEN "✅ $1"
}

print_warning() {
    print_status $YELLOW "⚠️  $1"
}

print_error() {
    print_status $RED "❌ $1"
}

print_info() {
    print_status $BLUE "ℹ️  $1"
}

# Help function
show_help() {
    cat << EOF
Intelligent Workflow Builder Integration Test Runner

Usage: $0 [OPTIONS] [TEST_CATEGORY]

TEST_CATEGORIES:
    all                     Run all integration tests (default)
    slm                     Run only SLM integration tests
    vector                  Run only vector database integration tests
    e2e                     Run only end-to-end lifecycle tests
    performance             Run only performance and load tests

OPTIONS:
    -h, --help              Show this help message
    -v, --verbose           Enable verbose output
    -q, --quick             Skip slow and performance tests
    --skip-slm              Skip SLM integration tests
    --skip-performance      Skip performance tests
    --no-setup              Skip environment setup validation
    --report-file FILE      Output report to specified file
    --timeout DURATION      Test timeout (default: 5m)

ENVIRONMENT VARIABLES:
    LLM_PROVIDER           SLM provider (ollama, ramalama) [default: ollama]
    LLM_MODEL              LLM model name [default: granite3.1-dense:8b]
    LLM_ENDPOINT           LLM service endpoint [default: http://localhost:11434]
    LOG_LEVEL              Log level (debug, info, warn) [default: info]
    SKIP_SLM_TESTS         Skip SLM tests [default: false]
    SKIP_PERFORMANCE_TESTS Skip performance tests [default: false]
    SKIP_SLOW_TESTS        Skip slow tests [default: false]

EXAMPLES:
    # Run all tests with default configuration
    $0

    # Run only SLM integration tests with debug logging
    LOG_LEVEL=debug $0 slm

    # Quick test run (skip slow tests)
    $0 --quick

    # Run tests with custom LLM model
    LLM_MODEL=llama2:7b $0

    # Run tests with custom endpoint and verbose output
    LLM_ENDPOINT=http://custom-llm:8080 $0 --verbose

    # Run performance tests only
    $0 performance

    # Generate detailed report
    $0 --report-file detailed_report.txt --verbose

EOF
}

# Parse command line arguments
parse_arguments() {
    TEST_CATEGORY="all"
    SKIP_SETUP="false"

    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -v|--verbose)
                VERBOSE="true"
                shift
                ;;
            -q|--quick)
                SKIP_SLOW_TESTS="true"
                SKIP_PERFORMANCE_TESTS="true"
                shift
                ;;
            --skip-slm)
                SKIP_SLM_TESTS="true"
                shift
                ;;
            --skip-performance)
                SKIP_PERFORMANCE_TESTS="true"
                shift
                ;;
            --no-setup)
                SKIP_SETUP="true"
                shift
                ;;
            --report-file)
                REPORT_FILE="$2"
                shift 2
                ;;
            --timeout)
                TEST_TIMEOUT="$2"
                shift 2
                ;;
            all|slm|vector|e2e|performance)
                TEST_CATEGORY="$1"
                shift
                ;;
            *)
                print_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Validate environment
validate_environment() {
    print_header "Validating Test Environment"

    # Check if we're in the right directory
    if [[ ! -f "${PROJECT_ROOT}/go.mod" ]]; then
        print_error "Not in project root directory"
        exit 1
    fi

    # Check for required tools
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed or not in PATH"
        exit 1
    fi

    if ! command -v ginkgo &> /dev/null; then
        print_warning "Ginkgo not found, installing..."
        go install github.com/onsi/ginkgo/v2/ginkgo@latest
        if ! command -v ginkgo &> /dev/null; then
            print_error "Failed to install Ginkgo"
            exit 1
        fi
    fi

    print_success "Go and Ginkgo are available"

    # Check test files exist
    if [[ ! -f "${TEST_DIR}/intelligent_workflow_builder_integration_test.go" ]]; then
        print_error "Integration test files not found in ${TEST_DIR}"
        exit 1
    fi

    print_success "Integration test files found"
}

# Validate SLM connectivity
validate_slm_connectivity() {
    if [[ "${SKIP_SLM_TESTS}" == "true" ]]; then
        print_info "Skipping SLM connectivity validation (SKIP_SLM_TESTS=true)"
        return 0
    fi

    print_header "Validating SLM Connectivity"

    print_info "Checking SLM endpoint: ${LLM_ENDPOINT}"

    # Check if endpoint is reachable
    if command -v curl &> /dev/null; then
        if curl -f -s "${LLM_ENDPOINT}/api/health" &> /dev/null; then
            print_success "SLM endpoint is reachable"
        else
            print_warning "SLM endpoint not reachable, tests may fail"
            print_info "To skip SLM tests, set SKIP_SLM_TESTS=true"
        fi
    else
        print_warning "curl not available, cannot validate SLM connectivity"
    fi

    # For Ollama, check if model is available
    if [[ "${LLM_PROVIDER}" == "ollama" ]] && command -v ollama &> /dev/null; then
        print_info "Checking if model ${LLM_MODEL} is available..."
        if ollama list | grep -q "${LLM_MODEL}"; then
            print_success "Model ${LLM_MODEL} is available"
        else
            print_warning "Model ${LLM_MODEL} not found"
            print_info "To pull the model, run: ollama pull ${LLM_MODEL}"
            print_info "Or set SKIP_SLM_TESTS=true to skip SLM tests"
        fi
    fi
}

# Set up test environment
setup_test_environment() {
    if [[ "${SKIP_SETUP}" == "true" ]]; then
        print_info "Skipping environment setup (--no-setup specified)"
        return 0
    fi

    validate_environment
    validate_slm_connectivity
}

# Build focus string for Ginkgo based on test category
build_ginkgo_focus() {
    case "${TEST_CATEGORY}" in
        all)
            echo ""
            ;;
        slm)
            echo "--focus=\"Real SLM Client Integration\""
            ;;
        vector)
            echo "--focus=\"Vector Database Integration\""
            ;;
        e2e)
            echo "--focus=\"End-to-End Workflow Lifecycle\""
            ;;
        performance)
            echo "--focus=\"Performance and Load Testing\""
            ;;
        *)
            print_error "Unknown test category: ${TEST_CATEGORY}"
            exit 1
            ;;
    esac
}

# Run the integration tests
run_tests() {
    print_header "Running Intelligent Workflow Builder Integration Tests"

    # Change to project root
    cd "${PROJECT_ROOT}"

    # Set environment variables
    export LLM_PROVIDER
    export LLM_MODEL
    export LLM_ENDPOINT
    export LOG_LEVEL
    export SKIP_SLM_TESTS
    export SKIP_PERFORMANCE_TESTS
    export SKIP_SLOW_TESTS

    # Build Ginkgo command
    local ginkgo_cmd="ginkgo -tags=integration"

    # Add focus if specified
    local focus_arg
    focus_arg=$(build_ginkgo_focus)
    if [[ -n "${focus_arg}" ]]; then
        ginkgo_cmd="${ginkgo_cmd} ${focus_arg}"
    fi

    # Add verbose flag if requested
    if [[ "${VERBOSE}" == "true" ]]; then
        ginkgo_cmd="${ginkgo_cmd} -v"
        export GINKGO_REPORTER="verbose"
    fi

    # Add timeout
    ginkgo_cmd="${ginkgo_cmd} --timeout=${TEST_TIMEOUT}"

    # Add test directory
    ginkgo_cmd="${ginkgo_cmd} ${TEST_DIR}"

    print_info "Test Category: ${TEST_CATEGORY}"
    print_info "LLM Provider: ${LLM_PROVIDER}"
    print_info "LLM Model: ${LLM_MODEL}"
    print_info "LLM Endpoint: ${LLM_ENDPOINT}"
    print_info "Skip SLM Tests: ${SKIP_SLM_TESTS}"
    print_info "Skip Performance Tests: ${SKIP_PERFORMANCE_TESTS}"
    print_info "Skip Slow Tests: ${SKIP_SLOW_TESTS}"
    print_info "Timeout: ${TEST_TIMEOUT}"

    if [[ "${VERBOSE}" == "true" ]]; then
        print_info "Command: ${ginkgo_cmd}"
    fi

    echo
    print_info "Starting test execution..."

    # Run the tests and capture output
    local start_time
    start_time=$(date +%s)

    if [[ -n "${REPORT_FILE}" ]]; then
        # Run tests and save output to report file
        if eval "${ginkgo_cmd}" 2>&1 | tee "${REPORT_FILE}"; then
            local test_result=0
        else
            local test_result=$?
        fi
    else
        # Run tests without saving output
        if eval "${ginkgo_cmd}"; then
            local test_result=0
        else
            local test_result=$?
        fi
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    echo
    print_header "Test Execution Summary"

    if [[ ${test_result} -eq 0 ]]; then
        print_success "All tests passed!"
        print_success "Test execution completed in ${duration} seconds"
    else
        print_error "Some tests failed!"
        print_error "Test execution completed in ${duration} seconds with exit code ${test_result}"
    fi

    if [[ -n "${REPORT_FILE}" ]]; then
        print_info "Detailed report saved to: ${REPORT_FILE}"
    fi

    return ${test_result}
}

# Main execution
main() {
    print_header "Intelligent Workflow Builder Integration Test Runner"

    parse_arguments "$@"
    setup_test_environment
    run_tests

    local exit_code=$?

    if [[ ${exit_code} -eq 0 ]]; then
        print_success "Integration tests completed successfully!"
    else
        print_error "Integration tests failed!"
    fi

    exit ${exit_code}
}

# Execute main function with all arguments
main "$@"
