#!/bin/bash

# HolmesGPT + Kubernaut Hybrid Environment Setup Script
# This script sets up the complete hybrid architecture environment
#
# Architecture:
# [HolmesGPT] → [Kubernetes API] (direct)
# [HolmesGPT] → [Prometheus] (direct)
# [HolmesGPT] → [Kubernaut Context API] (Kubernaut-specific only)
#              ↓
#         [Local LLM]

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
CONFIG_DIR="$HOME/.config/holmesgpt"
LLM_PORT=8080
CONTEXT_API_PORT=8091
HOLMESGPT_PORT=8090

# Logging function
log() {
    echo -e "${CYAN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
check_prerequisites() {
    log "🔍 Checking prerequisites..."

    local missing_deps=()

    # Check required commands
    for cmd in podman curl jq oc; do
        if ! command_exists "$cmd"; then
            missing_deps+=("$cmd")
        fi
    done

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Install missing dependencies and run this script again"
        log_info "  macOS: brew install podman curl jq"
        log_info "  For oc: Download from OpenShift CLI releases"
        exit 1
    fi

    # Check if running on macOS (for podman specifics)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        log_info "Detected macOS - will use x86_64 emulation for containers"
    fi

    # Check OpenShift/Kubernetes access
    if ! oc whoami >/dev/null 2>&1; then
        log_warning "Not authenticated with OpenShift/Kubernetes cluster"
        log_info "You may need to run: oc login <cluster-url>"
    else
        log_success "OpenShift/Kubernetes access verified"
    fi

    log_success "All prerequisites checked"
}

# Check if port is in use
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 0  # Port is in use
    else
        return 1  # Port is free
    fi
}

# Wait for service to be ready
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=${3:-30}
    local attempt=1

    log "⏳ Waiting for $service_name to be ready..."

    while [ $attempt -le $max_attempts ]; do
        if curl -s --connect-timeout 2 "$url" >/dev/null 2>&1; then
            log_success "$service_name is ready!"
            return 0
        fi

        echo -n "."
        sleep 2
        ((attempt++))
    done

    log_error "$service_name failed to start after $((max_attempts * 2)) seconds"
    return 1
}

# Setup configuration directory
setup_config_directory() {
    log "📁 Setting up configuration directory..."

    mkdir -p "$CONFIG_DIR"

    # Create HolmesGPT configuration
    cat > "$CONFIG_DIR/config.yaml" << EOF
# HolmesGPT Configuration for Kubernaut Integration
llm:
  provider: "openai-compatible"
  base_url: "http://host.containers.internal:8080/v1"
  model: "ggml-org/gpt-oss-20b-GGUF"
  timeout: 120

api:
  host: "0.0.0.0"
  port: 8090

toolsets:
  - name: "kubernaut-hybrid"
    description: "Kubernaut hybrid toolset with direct K8s/Prometheus access"
    config_file: "/app/config/kubernaut-toolset.yaml"

integration:
  kubernaut_context_api: "http://host.containers.internal:8091"
  prometheus_url: "http://prometheus:9090"

logging:
  level: "info"
  format: "json"

timeouts:
  default: "30s"
  investigation: "300s"
EOF

    # Copy hybrid toolset configuration
    cp "$PROJECT_ROOT/config/holmesgpt-hybrid-toolset.yaml" "$CONFIG_DIR/kubernaut-toolset.yaml"

    log_success "Configuration directory created at $CONFIG_DIR"
}

# Start Local LLM
start_local_llm() {
    log "🤖 Checking Local LLM status..."

    if check_port $LLM_PORT; then
        log_info "Local LLM appears to be running on port $LLM_PORT"

        # Test if it's actually the LLM service
        if curl -s "http://localhost:$LLM_PORT/v1/models" | jq -r '.models[0].name' 2>/dev/null | grep -q "gpt-oss-20b-GGUF"; then
            log_success "Local LLM (gpt-oss-20b-GGUF) is running"
            return 0
        else
            log_warning "Port $LLM_PORT is in use but not by expected LLM service"
        fi
    fi

    log_warning "Local LLM not detected on port $LLM_PORT"
    log_info "Please start your local LLM service manually:"
    log_info "  Example: ramalama serve --port $LLM_PORT ggml-org/gpt-oss-20b-GGUF"
    log_info "  Or: ollama serve (if using Ollama)"

    read -p "Press Enter once your Local LLM is running on port $LLM_PORT..."

    if ! wait_for_service "http://localhost:$LLM_PORT/v1/models" "Local LLM"; then
        log_error "Failed to connect to Local LLM"
        exit 1
    fi
}

# Start Kubernaut Context API
start_context_api() {
    log "🚀 Starting Kubernaut Context API..."

    if check_port $CONTEXT_API_PORT; then
        log_info "Port $CONTEXT_API_PORT is in use, checking if it's Context API..."
        if curl -s "http://localhost:$CONTEXT_API_PORT/api/v1/context/health" | jq -r '.service' 2>/dev/null | grep -q "context-api"; then
            log_success "Kubernaut Context API is already running"
            return 0
        else
            log_error "Port $CONTEXT_API_PORT is in use by another service"
            exit 1
        fi
    fi

    # Check if context-api binary exists
    if [ ! -f "$PROJECT_ROOT/bin/context-api-production" ]; then
        log_error "Context API binary not found at $PROJECT_ROOT/bin/context-api-production"
        log_info "Please build the project first: make build"
        exit 1
    fi

    # Check if config exists
    local config_file="$PROJECT_ROOT/config/dynamic-context-orchestration.yaml"
    if [ ! -f "$config_file" ]; then
        log_error "Context API config not found at $config_file"
        exit 1
    fi

    # Start Context API in background
    log "Starting Context API server..."
    cd "$PROJECT_ROOT"
    ./bin/context-api-production --config "$config_file" &
    CONTEXT_API_PID=$!
    echo $CONTEXT_API_PID > /tmp/kubernaut-context-api.pid

    # Wait for Context API to be ready
    if ! wait_for_service "http://localhost:$CONTEXT_API_PORT/api/v1/context/health" "Kubernaut Context API"; then
        log_error "Failed to start Kubernaut Context API"
        exit 1
    fi

    log_success "Kubernaut Context API started (PID: $CONTEXT_API_PID)"
}

# Start HolmesGPT container
start_holmesgpt() {
    log "🐳 Starting HolmesGPT container..."

    local container_name="holmesgpt-kubernaut-hybrid"
    local image="us-central1-docker.pkg.dev/genuine-flight-317411/devel/holmes:latest"

    # Stop existing container if running
    podman stop "$container_name" 2>/dev/null || true
    podman rm "$container_name" 2>/dev/null || true

    # Pull latest image
    log "Pulling HolmesGPT image..."
    if ! podman pull --platform linux/amd64 "$image"; then
        log_error "Failed to pull HolmesGPT image"
        exit 1
    fi

    # Start HolmesGPT container
    log "Starting HolmesGPT container with hybrid toolset..."

    local platform_flag=""
    if [[ "$OSTYPE" == "darwin"* ]]; then
        platform_flag="--platform linux/amd64"
    fi

    podman run -d \
        --name "$container_name" \
        $platform_flag \
        --network host \
        -v "$CONFIG_DIR:/app/config:ro,Z" \
        -v "$HOME/.kube:/root/.kube:ro,Z" \
        -e HOLMES_LLM_PROVIDER="openai-compatible" \
        -e HOLMES_LLM_BASE_URL="http://host.containers.internal:$LLM_PORT/v1" \
        -e HOLMES_LLM_MODEL="ggml-org/gpt-oss-20b-GGUF" \
        -e HOLMES_API_HOST="0.0.0.0" \
        -e HOLMES_API_PORT="$HOLMESGPT_PORT" \
        -e KUBERNAUT_CONTEXT_API="http://host.containers.internal:$CONTEXT_API_PORT" \
        "$image" \
        bash -c "echo 'HolmesGPT ready for investigations' && sleep infinity"

    log_success "HolmesGPT container started: $container_name"
    log_info "Container is ready for investigations"
}

# Validate the entire setup
validate_setup() {
    log "🧪 Validating complete setup..."

    local validation_passed=true

    # Test 1: Local LLM
    log "Testing Local LLM..."
    if curl -s "http://localhost:$LLM_PORT/v1/models" | jq -r '.models[0].name' 2>/dev/null | grep -q "gpt-oss-20b-GGUF"; then
        log_success "Local LLM: OK"
    else
        log_error "Local LLM: FAILED"
        validation_passed=false
    fi

    # Test 2: Context API
    log "Testing Kubernaut Context API..."
    if curl -s "http://localhost:$CONTEXT_API_PORT/api/v1/context/health" | jq -r '.status' 2>/dev/null | grep -q "healthy"; then
        log_success "Kubernaut Context API: OK"
    else
        log_error "Kubernaut Context API: FAILED"
        validation_passed=false
    fi

    # Test 3: Context Discovery
    log "Testing Context Discovery..."
    if curl -s "http://localhost:$CONTEXT_API_PORT/api/v1/context/discover" >/dev/null 2>&1; then
        log_success "Context Discovery: OK"
    else
        log_warning "Context Discovery: Limited (endpoint may not be fully implemented)"
    fi

    # Test 4: HolmesGPT Container
    log "Testing HolmesGPT container..."
    if podman ps --format "table {{.Names}}" | grep -q "holmesgpt-kubernaut-hybrid"; then
        log_success "HolmesGPT Container: OK"
    else
        log_error "HolmesGPT Container: FAILED"
        validation_passed=false
    fi

    # Test 5: Kubernetes Access
    log "Testing Kubernetes access..."
    if oc get pods -n default >/dev/null 2>&1; then
        log_success "Kubernetes Access: OK"
    else
        log_warning "Kubernetes Access: Limited (check oc login)"
    fi

    if [ "$validation_passed" = true ]; then
        log_success "🎊 All core components validated successfully!"
        return 0
    else
        log_error "❌ Some components failed validation"
        return 1
    fi
}

# Display usage information
show_usage_info() {
    log "📋 Environment Setup Complete!"
    echo ""
    echo -e "${GREEN}🎯 SERVICES RUNNING:${NC}"
    echo "  • Local LLM: http://localhost:$LLM_PORT"
    echo "  • Kubernaut Context API: http://localhost:$CONTEXT_API_PORT"
    echo "  • HolmesGPT Container: holmesgpt-kubernaut-hybrid"
    echo ""
    echo -e "${BLUE}🔧 USAGE EXAMPLES:${NC}"
    echo ""
    echo "1. Run HolmesGPT Investigation:"
    echo "   podman exec -it holmesgpt-kubernaut-hybrid \\"
    echo "     holmes investigate --alert-name 'PodCrashLoop' \\"
    echo "     --namespace 'default' --toolsets /app/config/kubernaut-toolset.yaml"
    echo ""
    echo "2. Test Direct Kubernetes Access:"
    echo "   podman exec -it holmesgpt-kubernaut-hybrid kubectl get pods -n default"
    echo ""
    echo "3. Test Kubernaut Context API:"
    echo "   curl http://localhost:$CONTEXT_API_PORT/api/v1/context/health"
    echo ""
    echo "4. Test Pattern Analysis:"
    echo "   curl 'http://localhost:$CONTEXT_API_PORT/api/v1/context/patterns/test-pattern'"
    echo ""
    echo -e "${YELLOW}📚 DOCUMENTATION:${NC}"
    echo "  • Setup Guide: docs/deployment/HOLMESGPT_HYBRID_SETUP_GUIDE.md"
    echo "  • Architecture: docs/deployment/HOLMESGPT_HYBRID_ARCHITECTURE.md"
    echo ""
    echo -e "${CYAN}🛑 TO STOP ENVIRONMENT:${NC}"
    echo "  ./scripts/stop-holmesgpt-environment.sh"
}

# Cleanup function for graceful shutdown
cleanup() {
    log "🧹 Cleaning up..."

    # Stop HolmesGPT container
    podman stop holmesgpt-kubernaut-hybrid 2>/dev/null || true

    # Stop Context API if we started it
    if [ -f /tmp/kubernaut-context-api.pid ]; then
        local pid=$(cat /tmp/kubernaut-context-api.pid)
        if ps -p $pid > /dev/null 2>&1; then
            log "Stopping Context API (PID: $pid)..."
            kill $pid 2>/dev/null || true
            rm -f /tmp/kubernaut-context-api.pid
        fi
    fi

    log_info "Cleanup completed"
}

# Trap for cleanup on script exit
trap cleanup EXIT INT TERM

# Main execution
main() {
    echo -e "${CYAN}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║           HolmesGPT + Kubernaut Hybrid Environment          ║"
    echo "║                      Setup Script                           ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    check_prerequisites
    setup_config_directory
    start_local_llm
    start_context_api
    start_holmesgpt

    if validate_setup; then
        show_usage_info
        log_success "🚀 Environment is ready for HolmesGPT investigations!"

        # Keep script running to maintain services
        log_info "Press Ctrl+C to stop all services and exit"
        while true; do
            sleep 10
            # Health check
            if ! check_port $CONTEXT_API_PORT; then
                log_warning "Context API appears to have stopped"
                break
            fi
        done
    else
        log_error "Environment setup failed. Check the errors above."
        exit 1
    fi
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
