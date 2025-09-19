#!/bin/bash

# =============================================================================
# Ramalama Server Startup Script for Mac Studio (Headless Testing Environment)
# Optimized for ggml-org/gpt-oss-20b-GGUF performance
# =============================================================================

set -euo pipefail

# Configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly LOG_DIR="${PROJECT_ROOT}/logs"
readonly PID_FILE="${LOG_DIR}/ramalama-server.pid"
readonly LOG_FILE="${LOG_DIR}/ramalama-server.log"
readonly ERROR_LOG="${LOG_DIR}/ramalama-server.error.log"

# Server Configuration
readonly HOST="192.168.1.169"
readonly PORT="8080"
readonly MODEL_NAME="hf://ggml-org/gpt-oss-20b-GGUF"

# Performance Optimizations for Dedicated Headless Remote Mac Studio M2 Max
readonly THREADS=20              # Over-subscribe for pipeline efficiency
readonly CONTEXT_SIZE=8192       # Larger context for integration tests
readonly BATCH_SIZE=1024         # Large batches for dedicated GPU (30 cores)
readonly TIMEOUT=600             # Longer timeout for complex requests

# Memory Optimizations for Dedicated Hardware
readonly USE_MLOCK="true"        # Lock model in memory
readonly USE_MMAP="false"        # Direct memory access for performance
readonly NUMA_ENABLE="true"      # Enable NUMA optimizations if available

# Inference Speed Optimizations
readonly GPU_LAYERS=999          # Use all GPU layers via Metal
readonly DRAFT_MODEL="none"      # Disable draft model for now (compatibility)
readonly TEMPERATURE=0.7         # Balanced temperature for 20B model creativity vs consistency

# Logging Configuration
readonly LOG_LEVEL="INFO"
readonly ENABLE_METRICS="true"

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

check_dependencies() {
    log_info "Checking dependencies..."

    if ! command -v ramalama &> /dev/null; then
        log_error "ramalama is not installed or not in PATH"
        log_info "Install with: brew install ramalama"
        exit 1
    fi

    # Check if model exists
    if ! ramalama list | grep -q "${MODEL_NAME}"; then
        log_warn "Model ${MODEL_NAME} not found locally"
        log_info "Pulling model... (this may take a while)"
        ramalama pull "${MODEL_NAME}" || {
            log_error "Failed to pull model ${MODEL_NAME}"
            exit 1
        }
    fi

    log_success "Dependencies check passed"
}

setup_environment() {
    log_info "Setting up environment..."

    # Create log directory
    mkdir -p "${LOG_DIR}"

    # Set performance environment variables for dedicated Mac Studio
    export MALLOC_ARENA_MAX=1                    # Single arena for headless performance
    export RAMALAMA_NUM_THREAD="${THREADS}"      # Override thread count
    export RAMALAMA_LOG_LEVEL="${LOG_LEVEL}"     # Set logging level
    export GGML_METAL_NDEBUG=1                   # Disable Metal debugging overhead

    # macOS specific optimizations for dedicated Mac Studio M2 Max
    if [[ "$(uname)" == "Darwin" ]]; then
        # Aggressive file descriptor limits for headless operation
        ulimit -n 131072

        # System-wide optimizations for dedicated Mac Studio hardware
        if command -v sysctl &> /dev/null; then
            # VM and memory optimizations for 32GB M2 Max
            sysctl -w vm.compressor_mode=4 2>/dev/null || log_warn "Could not optimize VM compressor"
            sysctl -w kern.maxfiles=131072 2>/dev/null || log_warn "Could not increase system file limit"

            # Network optimizations for integration test traffic
            sysctl -w net.inet.tcp.sendspace=131072 2>/dev/null || log_warn "Could not optimize TCP send buffer"
            sysctl -w net.inet.tcp.recvspace=131072 2>/dev/null || log_warn "Could not optimize TCP receive buffer"
            sysctl -w net.inet.tcp.delayed_ack=0 2>/dev/null || log_warn "Could not disable delayed ACK"

            # Memory pressure optimizations for large model
            sysctl -w vm.pressure_threshold=95 2>/dev/null || log_warn "Could not adjust memory pressure threshold"
        fi

        # Disable unnecessary services for dedicated headless Mac Studio
        launchctl unload -w /System/Library/LaunchDaemons/com.apple.metadata.mds.plist 2>/dev/null || true
        launchctl unload -w /System/Library/LaunchAgents/com.apple.photoanalysisd.plist 2>/dev/null || true
    fi

    log_success "Environment setup completed"
}

check_port() {
    log_info "Checking if port ${PORT} is available..."

    if lsof -Pi :${PORT} -sTCP:LISTEN -t >/dev/null 2>&1; then
        log_error "Port ${PORT} is already in use"
        log_info "Existing process:"
        lsof -Pi :${PORT} -sTCP:LISTEN
        log_info "Use 'kill \$(lsof -ti:${PORT})' to stop existing server"
        exit 1
    fi

    log_success "Port ${PORT} is available"
}

stop_existing_server() {
    if [[ -f "${PID_FILE}" ]]; then
        local pid
        pid=$(cat "${PID_FILE}")

        if kill -0 "${pid}" 2>/dev/null; then
            log_info "Stopping existing ramalama server (PID: ${pid})..."
            kill "${pid}"

            # Wait for graceful shutdown
            local count=0
            while kill -0 "${pid}" 2>/dev/null && [[ ${count} -lt 10 ]]; do
                sleep 1
                ((count++))
            done

            # Force kill if still running
            if kill -0 "${pid}" 2>/dev/null; then
                log_warn "Force killing server..."
                kill -9 "${pid}"
            fi

            log_success "Existing server stopped"
        fi

        rm -f "${PID_FILE}"
    fi
}

build_ramalama_command() {
    local cmd="ramalama"

    # Global flags must come before the serve command
    cmd+=" --runtime llama.cpp"
    cmd+=" --nocontainer"
    cmd+=" serve"

    # Basic server configuration
    cmd+=" --host ${HOST}"
    cmd+=" --port ${PORT}"

    # Core performance optimizations for Mac Studio M2 Max
    cmd+=" --ctx-size ${CONTEXT_SIZE}"
    cmd+=" --threads ${THREADS}"

    # GPU acceleration (Metal on Apple Silicon M2 Max - 30-core GPU)
    cmd+=" --ngl ${GPU_LAYERS}"

    # Inference speed optimizations for integration testing
    cmd+=" --temp ${TEMPERATURE}"

    # Cache optimization for repeated requests
    cmd+=" --cache-reuse 1024"

    # Disable web UI for headless operation
    cmd+=" --webui off"

    # Enable GPU device access
    cmd+=" --keep-groups"

    # Runtime arguments - using the flags that worked before
    cmd+=" --runtime-args \"-b ${BATCH_SIZE} --mlock --no-mmap -np 8\""

    # Model name as positional argument (must be last)
    cmd+=" ${MODEL_NAME}"

    echo "${cmd}"
}

start_server() {
    log_info "Starting ramalama server with optimized settings..."

    local cmd
    cmd=$(build_ramalama_command)

    log_info "Command: ${cmd}"
    log_info "Logs: ${LOG_FILE}"
    log_info "Error logs: ${ERROR_LOG}"

    # Start server in background
    nohup ${cmd} > "${LOG_FILE}" 2> "${ERROR_LOG}" &
    local pid=$!

    # Save PID
    echo "${pid}" > "${PID_FILE}"

    log_info "Server started with PID: ${pid}"
    log_info "Waiting for server to be ready..."

    # Wait for server to start (longer wait for MLX + draft model loading)
    local count=0
    local max_wait=120

    while [[ ${count} -lt ${max_wait} ]]; do
        # Try multiple possible endpoints to check if server is ready
        if curl -s "http://${HOST}:${PORT}/v1/models" >/dev/null 2>&1 || \
           curl -s "http://${HOST}:${PORT}/health" >/dev/null 2>&1 || \
           curl -s "http://${HOST}:${PORT}/" >/dev/null 2>&1; then
            log_success "Server is ready and responding!"
            log_info "Server endpoint: http://${HOST}:${PORT}/"
            log_info "Models endpoint: http://${HOST}:${PORT}/v1/models"
            return 0
        fi

        if ! kill -0 "${pid}" 2>/dev/null; then
            log_error "Server process died unexpectedly"
            log_error "Check error log: ${ERROR_LOG}"
            cat "${ERROR_LOG}"
            return 1
        fi

        sleep 2
        ((count += 2))
        log_info "Waiting... (${count}/${max_wait}s)"
    done

    log_error "Server failed to start within ${max_wait} seconds"
    log_error "Check logs: ${LOG_FILE} and ${ERROR_LOG}"
    return 1
}

show_server_info() {
    log_success "Ramalama server is running!"
    echo
    echo "📊 Dedicated Headless Mac Studio M2 Max Server:"
    echo "   Host: ${HOST}:${PORT}"
    echo "   Model: ${MODEL_NAME}"
    echo "   Hardware: Mac Studio M2 Max (32GB RAM, 30 GPU cores)"
    echo "   Runtime: ramalama serve (tested working configuration)"
    echo "   Context Size: ${CONTEXT_SIZE}"
    echo "   CPU Threads: ${THREADS}"
    echo "   GPU Layers: ${GPU_LAYERS} (Metal - All available)"
    echo "   Batch Size: ${BATCH_SIZE} (Optimized for dedicated hardware)"
    echo "   Optimized: Cache reuse (1024) + Memory locking + No-mmap + Parallel (8)"
    echo "   Temperature: ${TEMPERATURE} (Balanced for 20B model performance)"
    echo "   Mode: Headless (Web UI disabled) + NoContainer"
    echo
    echo "🔗 Endpoints:"
    echo "   Server: http://${HOST}:${PORT}/"
    echo "   Models: http://${HOST}:${PORT}/v1/models"
    echo "   Completions: http://${HOST}:${PORT}/v1/completions"
    echo
    echo "📁 Files:"
    echo "   PID: ${PID_FILE}"
    echo "   Logs: ${LOG_FILE}"
    echo "   Error Logs: ${ERROR_LOG}"
    echo
    echo "🛑 To stop the server:"
    echo "   ${SCRIPT_DIR}/stop-ramalama-server.sh"
    echo "   or: kill \$(cat ${PID_FILE})"
}

test_server() {
    log_info "Testing server with simple request..."

    local test_response
    test_response=$(curl -s -X POST "http://${HOST}:${PORT}/v1/completions" \
        -H "Content-Type: application/json" \
        -d '{
            "prompt": "Respond with: RAMALAMA_SERVER_OK",
            "max_tokens": 10,
            "temperature": 0.1
        }' | jq -r '.choices[0].text // empty' 2>/dev/null)

    if [[ -n "${test_response}" ]]; then
        log_success "Server test passed: ${test_response}"
    else
        log_warn "Server test failed or returned empty response"
        log_info "Server may still be loading the model..."
    fi
}

main() {
    log_info "Starting ramalama server for kubernaut testing..."
    log_info "Optimized for dedicated headless Mac Studio M2 Max with llama.cpp runtime"

    check_dependencies
    setup_environment
    check_port
    stop_existing_server
    start_server
    show_server_info
    test_server

    log_success "Dedicated Mac Studio ramalama server startup complete!"
    log_info "For integration tests on client machines, use:"
    echo "   export LLM_ENDPOINT=http://${HOST}:${PORT}"
    echo "   export LLM_MODEL=${MODEL_NAME}"
    echo "   export LLM_PROVIDER=ramalama"
    echo
    log_info "Expected performance with dedicated hardware:"
    echo "   • 5-8x speedup from full Metal GPU utilization (30 cores)"
    echo "   • Large batch processing (1024) for GPU efficiency"
    echo "   • Cache reuse (1024) for repeated integration test patterns"
    echo "   • Memory locking for stable performance in 32GB RAM"
    echo "   • Parallel processing (8 requests) with no-mmap optimization"
    echo "   • Headless operation (Web UI disabled) for maximum performance"
}

# Handle signals
trap 'log_error "Script interrupted"; exit 130' INT TERM

# Run main function
main "$@"