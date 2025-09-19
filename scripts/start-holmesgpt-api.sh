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
    if [[ -f "${PID_FILE}" ]]; then
        local pid
        pid=$(cat "${PID_FILE}")

        if kill -0 "${pid}" 2>/dev/null; then
            log_info "Stopping existing HolmesGPT API service (PID: ${pid})..."
            kill "${pid}"

            # Wait for graceful shutdown
            local count=0
            while kill -0 "${pid}" 2>/dev/null && [[ ${count} -lt 10 ]]; do
                sleep 1
                ((count++))
            done

            # Force kill if still running
            if kill -0 "${pid}" 2>/dev/null; then
                log_warn "Force killing service..."
                kill -9 "${pid}"
            fi

            log_success "Existing service stopped"
        fi

        rm -f "${PID_FILE}"
    fi
}

start_service() {
    log_info "Starting HolmesGPT API service..."

    cd "${HOLMESGPT_DIR}"

    # Set environment variables for LLM connection
    export LLM_ENDPOINT=http://192.168.1.169:8080
    export LLM_MODEL=hf://ggml-org/gpt-oss-20b-GGUF
    export LLM_PROVIDER=ramalama

    # Start service in background
    source venv/bin/activate
    nohup python src/main.py > "${LOG_FILE}" 2>&1 &
    local pid=$!

    # Save PID
    echo "${pid}" > "${PID_FILE}"

    log_info "Service started with PID: ${pid}"
    log_info "Waiting for service to be ready..."

    # Wait for service to start
    local count=0
    local max_wait=30

    while [[ ${count} -lt ${max_wait} ]]; do
        if curl -s "http://localhost:3000/health" >/dev/null 2>&1; then
            log_success "HolmesGPT API is ready and responding!"
            log_info "Service endpoint: http://localhost:3000"
            log_info "Health endpoint: http://localhost:3000/health"
            return 0
        fi

        if ! kill -0 "${pid}" 2>/dev/null; then
            log_error "Service process died unexpectedly"
            log_error "Check error log: ${LOG_FILE}"
            tail -20 "${LOG_FILE}"
            return 1
        fi

        sleep 2
        ((count += 2))
        log_info "Waiting... (${count}/${max_wait}s)"
    done

    log_error "Service failed to start within ${max_wait} seconds"
    log_error "Check logs: ${LOG_FILE}"
    return 1
}

show_service_info() {
    log_success "HolmesGPT API service is running!"
    echo
    echo "📊 Service Information:"
    echo "   Endpoint: http://localhost:3000"
    echo "   Health: http://localhost:3000/health"
    echo "   Docs: http://localhost:3000/docs"
    echo "   LLM Backend: ${LLM_ENDPOINT:-Not configured}"
    echo
    echo "📁 Files:"
    echo "   PID: ${PID_FILE}"
    echo "   Logs: ${LOG_FILE}"
    echo
    echo "🛑 To stop the service:"
    echo "   kill \$(cat ${PID_FILE})"
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