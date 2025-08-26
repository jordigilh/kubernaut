#!/bin/bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
OLLAMA_ENDPOINT="${OLLAMA_ENDPOINT:-http://localhost:11434}"
OLLAMA_MODEL="${OLLAMA_MODEL:-granite3.1-dense:8b}"
MIN_MEMORY_GB=8
MIN_DISK_GB=20
TIMEOUT_SECONDS=300

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

info() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] INFO: $1${NC}"
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check system resources on different platforms
check_system_resources() {
    log "Checking system resources..."
    
    # Check available memory
    if command_exists free; then
        # Linux
        AVAILABLE_MEMORY=$(free -g | awk '/^Mem:/{print $7}')
        TOTAL_MEMORY=$(free -g | awk '/^Mem:/{print $2}')
    elif command_exists vm_stat; then
        # macOS
        TOTAL_MEMORY=$(( $(sysctl -n hw.memsize) / 1024 / 1024 / 1024 ))
        FREE_PAGES=$(vm_stat | grep "Pages free" | awk '{print $3}' | sed 's/\.//')
        PAGE_SIZE=$(vm_stat | grep "page size" | awk '{print $8}')
        AVAILABLE_MEMORY=$(( FREE_PAGES * PAGE_SIZE / 1024 / 1024 / 1024 ))
    else
        warn "Cannot determine memory usage on this system"
        AVAILABLE_MEMORY=16  # Assume sufficient
        TOTAL_MEMORY=16
    fi
    
    if [ "$AVAILABLE_MEMORY" -lt $MIN_MEMORY_GB ]; then
        error "Insufficient memory: ${AVAILABLE_MEMORY}GB available, ${MIN_MEMORY_GB}GB+ required"
        return 1
    fi
    log "Memory: ${AVAILABLE_MEMORY}GB available / ${TOTAL_MEMORY}GB total ✅"
    
    # Check available disk space
    if command_exists df; then
        if df -BG . >/dev/null 2>&1; then
            # Linux/GNU df
            AVAILABLE_DISK=$(df -BG . | tail -1 | awk '{print $4}' | sed 's/G//')
        else
            # macOS df
            AVAILABLE_DISK=$(df -g . | tail -1 | awk '{print $4}')
        fi
        
        if [ "$AVAILABLE_DISK" -lt $MIN_DISK_GB ]; then
            error "Insufficient disk space: ${AVAILABLE_DISK}GB available, ${MIN_DISK_GB}GB+ required"
            return 1
        fi
        log "Disk space: ${AVAILABLE_DISK}GB available ✅"
    else
        warn "Cannot check disk space on this system"
    fi
    
    # Check CPU cores
    if command_exists nproc; then
        CPU_CORES=$(nproc)
    elif command_exists sysctl; then
        CPU_CORES=$(sysctl -n hw.ncpu)
    else
        CPU_CORES="unknown"
    fi
    log "CPU cores: ${CPU_CORES} ✅"
}

# Function to check required tools
check_required_tools() {
    log "Checking required tools..."
    
    local missing_tools=()
    
    # Essential tools
    for tool in curl jq; do
        if ! command_exists "$tool"; then
            missing_tools+=("$tool")
        fi
    done
    
    # Go (for running tests)
    if ! command_exists go; then
        missing_tools+=("go")
    else
        GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        log "Go version: ${GO_VERSION} ✅"
    fi
    
    # Docker/Podman (optional but recommended)
    if command_exists docker; then
        log "Docker: available ✅"
    elif command_exists podman; then
        log "Podman: available ✅"
    else
        warn "Neither Docker nor Podman found - containerized testing not available"
    fi
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        error "Missing required tools: ${missing_tools[*]}"
        info "Please install the missing tools and try again"
        return 1
    fi
    
    log "All required tools are available ✅"
}

# Function to check Ollama connectivity
check_ollama_connectivity() {
    log "Checking Ollama connectivity at ${OLLAMA_ENDPOINT}..."
    
    local retry_count=0
    local max_retries=5
    
    while [ $retry_count -lt $max_retries ]; do
        if curl -s --connect-timeout 5 "${OLLAMA_ENDPOINT}/api/tags" >/dev/null 2>&1; then
            log "Ollama is accessible at ${OLLAMA_ENDPOINT} ✅"
            return 0
        fi
        
        retry_count=$((retry_count + 1))
        if [ $retry_count -lt $max_retries ]; then
            warn "Ollama not accessible (attempt ${retry_count}/${max_retries}), retrying in 5 seconds..."
            sleep 5
        fi
    done
    
    error "Ollama not accessible at ${OLLAMA_ENDPOINT}"
    info "Please ensure Ollama is running with: ollama serve"
    info "Or set OLLAMA_ENDPOINT to the correct URL"
    return 1
}

# Function to check and download model
check_and_download_model() {
    log "Checking Granite model availability..."
    
    # Check if model is already available
    if curl -s "${OLLAMA_ENDPOINT}/api/tags" | jq -r '.models[]?.name' 2>/dev/null | grep -q "^${OLLAMA_MODEL}$"; then
        log "Granite model '${OLLAMA_MODEL}' is available ✅"
        return 0
    fi
    
    warn "Granite model '${OLLAMA_MODEL}' not found, downloading..."
    info "This may take several minutes depending on your internet connection..."
    
    # Download the model
    local download_start=$(date +%s)
    
    if curl -s -X POST "${OLLAMA_ENDPOINT}/api/pull" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"${OLLAMA_MODEL}\"}" | grep -q "success"; then
        
        # Wait for model to be available
        local wait_count=0
        local max_wait=60  # 10 minutes max wait
        
        while [ $wait_count -lt $max_wait ]; do
            if curl -s "${OLLAMA_ENDPOINT}/api/tags" | jq -r '.models[]?.name' 2>/dev/null | grep -q "^${OLLAMA_MODEL}$"; then
                local download_end=$(date +%s)
                local download_time=$((download_end - download_start))
                log "Granite model downloaded successfully in ${download_time} seconds ✅"
                return 0
            fi
            
            sleep 10
            wait_count=$((wait_count + 1))
            info "Waiting for model download to complete... (${wait_count}0s elapsed)"
        done
        
        error "Timeout waiting for model download to complete"
        return 1
    else
        error "Failed to initiate model download"
        return 1
    fi
}

# Function to test basic Ollama functionality
test_ollama_functionality() {
    log "Testing basic Ollama functionality..."
    
    local test_prompt="Hello"
    local response
    
    response=$(curl -s -X POST "${OLLAMA_ENDPOINT}/api/generate" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"${OLLAMA_MODEL}\",
            \"prompt\": \"${test_prompt}\",
            \"stream\": false
        }" --max-time 30)
    
    if echo "$response" | jq -e '.response' >/dev/null 2>&1; then
        local actual_response=$(echo "$response" | jq -r '.response')
        log "Ollama is functioning correctly ✅"
        info "Test response: '$(echo "$actual_response" | head -c 50)...'"
        return 0
    else
        error "Ollama test failed"
        error "Response: $response"
        return 1
    fi
}

# Function to test SLM integration specific functionality
test_slm_integration() {
    log "Testing SLM integration functionality..."
    
    # Test with a realistic alert prompt
    local alert_prompt='<|system|>
You are a Kubernetes operations expert. Respond with valid JSON only.
<|user|>
Analyze this alert and recommend an action:
Alert: HighMemoryUsage
Severity: warning
Description: Pod using 95% memory
Namespace: production

Available actions: scale_deployment, restart_pod, increase_resources, notify_only

Respond with JSON: {"action": "...", "confidence": 0.85}
<|assistant|>'

    local response
    response=$(curl -s -X POST "${OLLAMA_ENDPOINT}/api/generate" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"${OLLAMA_MODEL}\",
            \"prompt\": $(echo "$alert_prompt" | jq -Rs .),
            \"stream\": false
        }" --max-time 60)
    
    if echo "$response" | jq -e '.response' >/dev/null 2>&1; then
        local slm_response=$(echo "$response" | jq -r '.response')
        log "SLM integration test successful ✅"
        info "SLM response preview: '$(echo "$slm_response" | head -c 100)...'"
        
        # Try to extract JSON from response
        if echo "$slm_response" | jq . >/dev/null 2>&1; then
            log "Response contains valid JSON ✅"
        else
            warn "Response does not contain valid JSON (this may be normal)"
        fi
        
        return 0
    else
        error "SLM integration test failed"
        error "Response: $response"
        return 1
    fi
}

# Function to check network connectivity for model downloads
check_network_connectivity() {
    log "Checking network connectivity..."
    
    # Test connectivity to Ollama registry
    if curl -s --connect-timeout 10 https://registry.ollama.ai >/dev/null 2>&1; then
        log "Network connectivity to Ollama registry ✅"
    else
        warn "Cannot reach Ollama registry - model downloads may fail"
    fi
    
    # Test general internet connectivity
    if curl -s --connect-timeout 5 https://google.com >/dev/null 2>&1; then
        log "General internet connectivity ✅"
    else
        warn "Limited internet connectivity detected"
    fi
}

# Function to check Go test setup
check_go_test_setup() {
    log "Checking Go test setup..."
    
    # Check if we can build the integration tests
    if go test -c -tags=integration ./test/integration/... >/dev/null 2>&1; then
        log "Integration tests compile successfully ✅"
    else
        error "Integration tests fail to compile"
        info "Run: go test -c -tags=integration ./test/integration/..."
        return 1
    fi
    
    # Check test dependencies
    if go mod verify >/dev/null 2>&1; then
        log "Go module dependencies verified ✅"
    else
        warn "Go module verification failed - running go mod tidy..."
        go mod tidy
    fi
}

# Function to estimate test duration
estimate_test_duration() {
    log "Estimating test duration..."
    
    local num_test_cases=7  # From our fixtures
    local avg_response_time=10  # seconds
    local setup_time=30
    local buffer_time=60
    
    local estimated_duration=$(( num_test_cases * avg_response_time + setup_time + buffer_time ))
    local estimated_minutes=$(( estimated_duration / 60 ))
    
    info "Estimated test duration: ${estimated_minutes} minutes"
    info "This includes ${num_test_cases} test cases with ~${avg_response_time}s average response time"
}

# Main validation function
main() {
    echo ""
    log "=== Prometheus Alerts SLM Integration Test Validation ==="
    echo ""
    
    local validation_start=$(date +%s)
    
    # Run all validation checks
    local failed_checks=()
    
    if ! check_required_tools; then
        failed_checks+=("required_tools")
    fi
    
    if ! check_system_resources; then
        failed_checks+=("system_resources")
    fi
    
    if ! check_network_connectivity; then
        # Don't fail on network issues, just warn
        true
    fi
    
    if ! check_ollama_connectivity; then
        failed_checks+=("ollama_connectivity")
    fi
    
    if ! check_and_download_model; then
        failed_checks+=("model_availability")
    fi
    
    if ! test_ollama_functionality; then
        failed_checks+=("ollama_functionality")
    fi
    
    if ! test_slm_integration; then
        failed_checks+=("slm_integration")
    fi
    
    if ! check_go_test_setup; then
        failed_checks+=("go_test_setup")
    fi
    
    estimate_test_duration
    
    local validation_end=$(date +%s)
    local validation_time=$((validation_end - validation_start))
    
    echo ""
    if [ ${#failed_checks[@]} -eq 0 ]; then
        log "🚀 All validation checks passed! (completed in ${validation_time}s)"
        echo ""
        log "Ready for integration testing. Run with:"
        info "  make test-integration-local    # With Docker Compose"
        info "  make test-integration          # With local Ollama"
        info "  SKIP_SLOW_TESTS=true make test-integration  # Skip performance tests"
        echo ""
    else
        error "❌ Validation failed. Failed checks: ${failed_checks[*]}"
        echo ""
        info "Please resolve the issues above and try again."
        return 1
    fi
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Validates prerequisites for Prometheus Alerts SLM integration testing."
        echo ""
        echo "Environment variables:"
        echo "  OLLAMA_ENDPOINT    Ollama API endpoint (default: http://localhost:11434)"
        echo "  OLLAMA_MODEL       Model name to use (default: granite3.1-dense:8b)"
        echo ""
        echo "This script checks:"
        echo "  - System resources (memory, disk, CPU)"
        echo "  - Required tools (curl, jq, go)"
        echo "  - Ollama connectivity and model availability"
        echo "  - Basic SLM functionality"
        echo "  - Go test compilation"
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac