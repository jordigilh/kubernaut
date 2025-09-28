#!/bin/bash

# =============================================================================
# HolmesGPT API Startup Script
# Integrated into the development bootstrap process
# =============================================================================

set -euo pipefail

# Configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly HOLMESGPT_DIR="${PROJECT_ROOT}/docker/holmesgpt-api"
readonly PID_FILE="${HOLMESGPT_DIR}/holmesgpt-api.pid"
readonly LOG_FILE="${HOLMESGPT_DIR}/holmesgpt-api.log"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

check_dependencies() {
    log_info "Checking HolmesGPT API dependencies..."

    if [[ ! -d "${HOLMESGPT_DIR}" ]]; then
        log_error "HolmesGPT API directory not found: ${HOLMESGPT_DIR}"
        exit 1
    fi

    # Check if virtual environment exists
    if [[ ! -d "${HOLMESGPT_DIR}/venv" ]]; then
        log_info "Creating Python virtual environment..."
        cd "${HOLMESGPT_DIR}"
        python3 -m venv venv
    fi

    # Install required dependencies
    log_info "Installing required dependencies..."
    cd "${HOLMESGPT_DIR}"
    source venv/bin/activate
    pip install -q fastapi uvicorn structlog prometheus_client 2>/dev/null || {
        log_warn "Some dependencies may already be installed"
    }

    log_success "Dependencies check completed"
}

stop_existing_service() {
    # Stop any existing HolmesGPT containers
    local existing_containers
    existing_containers=$(podman ps -q --filter "name=holmesgpt-api-dev" 2>/dev/null || true)

    if [[ -n "${existing_containers}" ]]; then
        log_info "Stopping existing HolmesGPT API containers..."
        podman stop ${existing_containers} >/dev/null 2>&1 || true
        podman rm ${existing_containers} >/dev/null 2>&1 || true
        log_success "Existing containers stopped"
    fi

    # Clean up PID file if it exists
    if [[ -f "${PID_FILE}" ]]; then
        local container_id
        container_id=$(cat "${PID_FILE}")

        # Try to stop container by ID if it's still running
        if podman ps -q --filter "id=${container_id}" | grep -q "${container_id}" 2>/dev/null; then
            log_info "Stopping container ${container_id}..."
            podman stop "${container_id}" >/dev/null 2>&1 || true
            podman rm "${container_id}" >/dev/null 2>&1 || true
        fi

        rm -f "${PID_FILE}"
    fi
}

start_service() {
    log_info "Starting HolmesGPT API container service..."

    # Use environment variables from bootstrap script or set defaults
    local llm_base_url="${HOLMESGPT_LLM_BASE_URL:-${LLM_ENDPOINT:-http://localhost:8010}}"
    local llm_provider="${HOLMESGPT_LLM_PROVIDER:-${LLM_PROVIDER:-ramalama}}"
    local llm_model="${HOLMESGPT_LLM_MODEL:-${LLM_MODEL:-oss-gpt:20b}}"
    local api_port="${HOLMESGPT_PORT:-8090}"
    local log_level="${HOLMESGPT_LOG_LEVEL:-INFO}"

    log_info "Configuration:"
    log_info "  LLM Base URL: ${llm_base_url}"
    log_info "  LLM Provider: ${llm_provider}"
    log_info "  LLM Model: ${llm_model}"
    log_info "  API Port: ${api_port}"

    # Stop any existing HolmesGPT containers
    podman stop holmesgpt-api-dev 2>/dev/null || true
    podman rm holmesgpt-api-dev 2>/dev/null || true

    # Start containerized HolmesGPT API
    local container_id
    container_id=$(podman run -d \
        --name holmesgpt-api-dev \
        --network host \
        -e HOLMESGPT_LLM_BASE_URL="${llm_base_url}" \
        -e HOLMESGPT_LLM_PROVIDER="${llm_provider}" \
        -e HOLMESGPT_LLM_MODEL="${llm_model}" \
        -e HOLMESGPT_PORT="${api_port}" \
        -e HOLMESGPT_LOG_LEVEL="${log_level}" \
        holmesgpt-api:localhost-8010 2>/dev/null)

    if [[ -z "${container_id}" ]]; then
        log_error "Failed to start HolmesGPT API container"
        return 1
    fi

    # Save container ID instead of PID
    echo "${container_id}" > "${PID_FILE}"

    log_info "Container started with ID: ${container_id}"
    log_info "Waiting for service to be ready..."

    # Wait for service to start
    local count=0
    local max_wait=30

    while [[ ${count} -lt ${max_wait} ]]; do
        if curl -s "http://localhost:${api_port}/health" >/dev/null 2>&1; then
            log_success "HolmesGPT API is ready and responding!"
            log_info "Service endpoint: http://localhost:${api_port}"
            log_info "Health endpoint: http://localhost:${api_port}/health"
            return 0
        fi

        # Check if container is still running
        if ! podman ps -q --filter "id=${container_id}" | grep -q "${container_id}"; then
            log_error "Container died unexpectedly"
            log_error "Container logs:"
            podman logs "${container_id}" | tail -20
            return 1
        fi

        sleep 2
        ((count += 2))
        log_info "Waiting... (${count}/${max_wait}s)"
    done

    log_error "Service failed to start within ${max_wait} seconds"
    log_error "Container logs:"
    podman logs "${container_id}" | tail -20
    return 1
}

show_service_info() {
    local api_port="${HOLMESGPT_PORT:-8090}"
    local llm_base_url="${HOLMESGPT_LLM_BASE_URL:-${LLM_ENDPOINT:-http://localhost:8010}}"

    log_success "HolmesGPT API container service is running!"
    echo
    echo "📊 Service Information:"
    echo "   Endpoint: http://localhost:${api_port}"
    echo "   Health: http://localhost:${api_port}/health"
    echo "   Docs: http://localhost:${api_port}/docs"
    echo "   LLM Backend: ${llm_base_url}"
    echo
    echo "🐳 Container Information:"
    echo "   Container Name: holmesgpt-api-dev"
    echo "   Container ID: $(cat ${PID_FILE} 2>/dev/null || echo 'Not found')"
    echo "   Image: holmesgpt-api:localhost-8010"
    echo
    echo "🛑 To stop the service:"
    echo "   podman stop holmesgpt-api-dev"
    echo "   # or: podman stop \$(cat ${PID_FILE})"
}

main() {
    log_info "Starting HolmesGPT API service for integration testing..."

    check_dependencies
    stop_existing_service
    start_service
    show_service_info

    log_success "HolmesGPT API startup complete!"
}

# Handle signals
trap 'log_error "Script interrupted"; exit 130' INT TERM

# Run main function
main "$@"