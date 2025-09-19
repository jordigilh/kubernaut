#!/bin/bash

# =============================================================================
# Ramalama Server Stop Script for Mac Studio (Headless Testing Environment)
# Gracefully stops the ramalama server started by start-ramalama-server.sh
# =============================================================================

set -euo pipefail

# Configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly LOG_DIR="${PROJECT_ROOT}/logs"
readonly PID_FILE="${LOG_DIR}/ramalama-server.pid"
readonly LOG_FILE="${LOG_DIR}/ramalama-server.log"
readonly ERROR_LOG="${LOG_DIR}/ramalama-server.error.log"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Functions
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

stop_server_by_pid() {
    if [[ ! -f "${PID_FILE}" ]]; then
        log_warn "PID file not found: ${PID_FILE}"
        return 1
    fi

    local pid
    pid=$(cat "${PID_FILE}")

    if [[ -z "${pid}" ]]; then
        log_error "PID file is empty"
        return 1
    fi

    if ! kill -0 "${pid}" 2>/dev/null; then
        log_warn "Process with PID ${pid} is not running"
        rm -f "${PID_FILE}"
        return 1
    fi

    log_info "Stopping ramalama server (PID: ${pid})..."

    # Try graceful shutdown first
    kill -TERM "${pid}"

    # Wait for graceful shutdown
    local count=0
    local max_wait=30

    while kill -0 "${pid}" 2>/dev/null && [[ ${count} -lt ${max_wait} ]]; do
        sleep 1
        ((count++))
        if [[ $((count % 5)) -eq 0 ]]; then
            log_info "Waiting for graceful shutdown... (${count}/${max_wait}s)"
        fi
    done

    # Force kill if still running
    if kill -0 "${pid}" 2>/dev/null; then
        log_warn "Process did not stop gracefully, force killing..."
        kill -9 "${pid}"
        sleep 2

        if kill -0 "${pid}" 2>/dev/null; then
            log_error "Failed to stop process ${pid}"
            return 1
        fi
    fi

    # Clean up PID file
    rm -f "${PID_FILE}"
    log_success "Server stopped successfully"
    return 0
}

stop_server_by_port() {
    log_info "Looking for ramalama processes on port 8080..."

    local pids
    pids=$(lsof -ti:8080 2>/dev/null || true)

    if [[ -z "${pids}" ]]; then
        log_info "No processes found listening on port 8080"
        return 1
    fi

    log_info "Found processes on port 8080: ${pids}"

    for pid in ${pids}; do
        local process_name
        process_name=$(ps -p "${pid}" -o comm= 2>/dev/null || echo "unknown")

        log_info "Stopping process ${pid} (${process_name})..."

        # Try graceful shutdown first
        kill -TERM "${pid}" 2>/dev/null || true

        # Wait a moment
        sleep 2

        # Force kill if still running
        if kill -0 "${pid}" 2>/dev/null; then
            log_warn "Force killing process ${pid}..."
            kill -9 "${pid}" 2>/dev/null || true
        fi

        # Verify it's stopped
        if kill -0 "${pid}" 2>/dev/null; then
            log_error "Failed to stop process ${pid}"
        else
            log_success "Stopped process ${pid}"
        fi
    done

    return 0
}

stop_ramalama_containers() {
    log_info "Checking for ramalama containers..."

    # Check if ramalama is running any containers
    if command -v podman &> /dev/null; then
        local containers
        containers=$(podman ps --filter "label=com.github.containers.ramalama" --format "{{.ID}}" 2>/dev/null || true)

        if [[ -n "${containers}" ]]; then
            log_info "Found ramalama containers: ${containers}"
            for container in ${containers}; do
                log_info "Stopping container ${container}..."
                podman stop "${container}" || log_warn "Failed to stop container ${container}"
            done
        else
            log_info "No ramalama containers found"
        fi
    fi

    if command -v docker &> /dev/null; then
        local containers
        containers=$(docker ps --filter "label=com.github.containers.ramalama" --format "{{.ID}}" 2>/dev/null || true)

        if [[ -n "${containers}" ]]; then
            log_info "Found ramalama containers: ${containers}"
            for container in ${containers}; do
                log_info "Stopping container ${container}..."
                docker stop "${container}" || log_warn "Failed to stop container ${container}"
            done
        else
            log_info "No ramalama containers found"
        fi
    fi
}

cleanup_logs() {
    if [[ -f "${LOG_FILE}" ]]; then
        log_info "Archiving server logs..."
        mv "${LOG_FILE}" "${LOG_FILE}.$(date +%Y%m%d_%H%M%S)" 2>/dev/null || true
    fi

    if [[ -f "${ERROR_LOG}" ]]; then
        mv "${ERROR_LOG}" "${ERROR_LOG}.$(date +%Y%m%d_%H%M%S)" 2>/dev/null || true
    fi
}

show_final_status() {
    log_info "Final status check..."

    # Check if anything is still listening on port 8080
    if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null 2>&1; then
        log_warn "Something is still listening on port 8080:"
        lsof -Pi :8080 -sTCP:LISTEN
    else
        log_success "Port 8080 is now free"
    fi

    # Show any remaining ramalama processes
    local remaining_processes
    remaining_processes=$(pgrep -f "ramalama" 2>/dev/null || true)

    if [[ -n "${remaining_processes}" ]]; then
        log_warn "Remaining ramalama processes found:"
        ps -f -p "${remaining_processes}" 2>/dev/null || true
    else
        log_success "No ramalama processes found"
    fi
}

main() {
    log_info "Stopping ramalama server..."

    local stopped=false

    # Try to stop using PID file first
    if stop_server_by_pid; then
        stopped=true
    fi

    # If PID file method failed, try port-based approach
    if ! $stopped; then
        if stop_server_by_port; then
            stopped=true
        fi
    fi

    # Stop any ramalama containers
    stop_ramalama_containers

    # Clean up logs
    cleanup_logs

    # Show final status
    show_final_status

    if $stopped; then
        log_success "Ramalama server shutdown complete!"
    else
        log_warn "No ramalama server was found running"
    fi

    echo
    echo "📊 Summary:"
    echo "   PID file: ${PID_FILE}"
    echo "   Log directory: ${LOG_DIR}"
    echo "   To start again: ${SCRIPT_DIR}/start-ramalama-server.sh"
}

# Handle signals
trap 'log_error "Script interrupted"; exit 130' INT TERM

# Run main function
main "$@"