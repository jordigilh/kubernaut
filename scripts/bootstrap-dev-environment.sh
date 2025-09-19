#!/bin/bash

# Development Environment Bootstrap Script
# Sets up complete integration testing environment except LLM model
#
# Components setup:
# - Kind Kubernetes cluster with Prometheus/AlertManager
# - PostgreSQL with pgvector extension
# - Vector Database (separate PostgreSQL instance)
# - Redis cache
# - Kubernaut application build
# - Waits for LLM model at localhost:8080

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"
CLUSTER_NAME="kubernaut-dev"
LLM_ENDPOINT="http://192.168.1.169:8080"
LLM_WAIT_TIMEOUT=60

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

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    local missing_deps=()

    # Check required commands
    for cmd in podman kind kubectl go; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done

    # Check podman-compose
    if ! command -v podman-compose &> /dev/null; then
        log_warning "podman-compose not found, will attempt to install..."
        if command -v pip3 &> /dev/null; then
            pip3 install podman-compose || missing_deps+=("podman-compose")
        else
            missing_deps+=("podman-compose")
        fi
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        echo ""
        echo "Installation commands:"
        echo "  macOS: brew install podman kind kubectl go"
        echo "  Linux: sudo apt-get install podman golang-go"
        echo "         go install sigs.k8s.io/kind@latest"
        echo "  podman-compose: pip3 install podman-compose"
        exit 1
    fi

    # Check podman machine (macOS)
    if [[ "$OSTYPE" == "darwin"* ]]; then
        if ! podman machine list 2>/dev/null | grep -q "Currently running"; then
            log_info "Starting Podman machine..."
            podman machine start || {
                log_warning "Podman machine not initialized, initializing..."
                podman machine init
                podman machine start
            }
        fi
    fi

    log_success "All prerequisites satisfied"
}

# Setup databases (PostgreSQL + Vector + Redis)
setup_databases() {
    log_step "Setting up databases (PostgreSQL + Vector DB + Redis)..."

    cd "$PROJECT_ROOT"

    # Use the existing bootstrap script for databases
    local bootstrap_script="test/integration/scripts/bootstrap-integration-tests.sh"

    if [ ! -f "$bootstrap_script" ]; then
        log_error "Database bootstrap script not found: $bootstrap_script"
        exit 1
    fi

    # Make script executable and run it
    chmod +x "$bootstrap_script"

    log_info "Starting database services..."
    if ! "$bootstrap_script" start; then
        log_error "Failed to start database services"
        exit 1
    fi

    log_success "Database services started successfully"
}

# Setup Kind cluster with monitoring
setup_kubernetes() {
    log_step "Setting up Kubernetes cluster with monitoring stack..."

    cd "$PROJECT_ROOT"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_warning "Kind cluster '${CLUSTER_NAME}' already exists, recreating..."
        kind delete cluster --name="${CLUSTER_NAME}"
    fi

    # Use Docker as provider (not Podman)
    unset KIND_EXPERIMENTAL_PROVIDER

    # Create cluster with custom name
    log_info "Creating Kind cluster: ${CLUSTER_NAME}"

    # Create temporary config with custom cluster name
    local temp_config="/tmp/kind-config-${CLUSTER_NAME}.yaml"
    sed "s/name: .*/name: ${CLUSTER_NAME}/" test/kind/kind-config.yaml > "$temp_config"

    kind create cluster \
        --name="${CLUSTER_NAME}" \
        --config="$temp_config" \
        --wait=300s

    # Clean up temp config
    rm -f "$temp_config"

    # Set kubectl context
    kubectl config use-context "kind-${CLUSTER_NAME}"

    # Wait for cluster to be ready
    log_info "Waiting for cluster nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=300s

    # Deploy monitoring stack
    log_info "Deploying Prometheus monitoring stack..."

    # Create monitoring namespace
    kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

    # Apply monitoring manifests if they exist
    if [ -d "test/manifests/monitoring" ]; then
        kubectl apply -f test/manifests/monitoring/

        # Wait for monitoring components
        log_info "Waiting for monitoring components..."
        kubectl wait --for=condition=Available deployment/prometheus -n monitoring --timeout=300s || log_warning "Prometheus deployment timeout"
        kubectl wait --for=condition=Available deployment/alertmanager -n monitoring --timeout=300s || log_warning "AlertManager deployment timeout"
    else
        log_warning "Monitoring manifests not found, skipping Prometheus setup"
    fi

    # Create test namespace
    kubectl create namespace e2e-test --dry-run=client -o yaml | kubectl apply -f -

    # Create service account for tests
    kubectl create serviceaccount kubernaut-test -n e2e-test --dry-run=client -o yaml | kubectl apply -f -
    kubectl create clusterrolebinding kubernaut-test-admin \
        --clusterrole=cluster-admin \
        --serviceaccount=e2e-test:kubernaut-test \
        --dry-run=client -o yaml | kubectl apply -f -

    log_success "Kubernetes cluster and monitoring stack ready"
}

# Build Kubernaut application
build_kubernaut() {
    log_step "Building Kubernaut application..."

    cd "$PROJECT_ROOT"

    # Build main application
    log_info "Building main application..."
    if ! go build -o bin/kubernaut ./cmd/dynamic-toolset-server; then
        log_error "Failed to build main Kubernaut application"
        exit 1
    fi

    # Build other binaries if needed
    log_info "Building additional components..."

    # Build test context API
    if [ -d "cmd/test-context-performance" ]; then
        go build -o bin/test-context-performance ./cmd/test-context-performance || log_warning "Failed to build test-context-performance"
    fi

    # Build prometheus alerts SLM
    if [ -d "cmd/prometheus-alerts-slm" ]; then
        go build -o bin/prometheus-alerts-slm ./cmd/prometheus-alerts-slm || log_warning "Failed to build prometheus-alerts-slm"
    fi

    # Build MCP server
    if [ -d "cmd/mcp-server" ]; then
        go build -o bin/mcp-server ./cmd/mcp-server || log_warning "Failed to build mcp-server"
    fi

    log_success "Kubernaut application built successfully"
}

# Wait for LLM model to be available
wait_for_llm() {
    log_step "Waiting for LLM model at ${LLM_ENDPOINT}..."

    local start_time=$(date +%s)
    local end_time=$((start_time + LLM_WAIT_TIMEOUT))

    while [ $(date +%s) -lt $end_time ]; do
        if curl -s --connect-timeout 5 "${LLM_ENDPOINT}/v1/models" >/dev/null 2>&1; then
            log_success "LLM model is available at ${LLM_ENDPOINT}"

            # Verify we can get models list
            local models_response=$(curl -s "${LLM_ENDPOINT}/v1/models" 2>/dev/null || echo '{}')
            if echo "$models_response" | grep -q '"data"'; then
                log_info "Available models: $(echo "$models_response" | jq -r '.data[].id // "unknown"' 2>/dev/null | head -3 | tr '\n' ', ' | sed 's/,$//')"
            fi
            return 0
        fi

        local remaining=$((end_time - $(date +%s)))
        echo -n "."
        sleep 2
    done

    echo ""
    log_error "LLM model not available at ${LLM_ENDPOINT} after ${LLM_WAIT_TIMEOUT} seconds"
    echo ""
    echo "Please ensure LocalAI is running with a model loaded:"
    echo "  1. Install LocalAI:"
    echo "     curl -L https://github.com/mudler/LocalAI/releases/latest/download/local-ai-\$(uname -s)-\$(uname -m) -o local-ai"
    echo "     chmod +x local-ai"
    echo ""
    echo "  2. Download a model (e.g., Granite):"
    echo "     mkdir -p models"
    echo "     curl -L https://huggingface.co/ibm-granite/granite-3.0-8b-instruct-gguf/resolve/main/granite-3.0-8b-instruct.Q4_K_M.gguf -o models/granite-3.0-8b-instruct.gguf"
    echo "     cp localai-config/granite-3.0-8b-instruct.yaml models/"
    echo ""
    echo "  3. Start LocalAI:"
    echo "     ./local-ai --models-path ./models --address localhost:8080"
    echo ""
    exit 1
}

# Generate environment configuration
generate_environment_config() {
    log_step "Generating environment configuration..."

    local config_file="${PROJECT_ROOT}/.env.development"

    cat > "$config_file" << EOF
# Kubernaut Development Environment Configuration
# Generated: $(date -Iseconds)
# Source this file before running tests: source .env.development

# Database Configuration
export DB_HOST=localhost
export DB_PORT=5433
export DB_NAME=action_history
export DB_USER=slm_user
export DB_PASSWORD=slm_password_dev
export DB_SSL_MODE=disable

# Vector Database Configuration
export VECTOR_DB_HOST=localhost
export VECTOR_DB_PORT=5434
export VECTOR_DB_NAME=vector_store
export VECTOR_DB_USER=vector_user
export VECTOR_DB_PASSWORD=vector_password_dev

# Redis Configuration
export REDIS_HOST=localhost
export REDIS_PORT=6380
export REDIS_PASSWORD=integration_redis_password

# LLM Configuration
export LLM_ENDPOINT=${LLM_ENDPOINT}
export LLM_MODEL=granite-3.0-8b-instruct
export LLM_PROVIDER=localai
export USE_MOCK_LLM=false

# Test Configuration
export USE_CONTAINER_DB=true
export USE_REAL_K8S=true
export SKIP_INTEGRATION=false
export SKIP_SLOW_TESTS=false
export TEST_TIMEOUT=120s

# Kubernetes Configuration
export KUBECONFIG=\$(kind get kubeconfig --name=${CLUSTER_NAME})
export KUBE_CONTEXT=kind-${CLUSTER_NAME}

# Development Tools
export CLUSTER_NAME=${CLUSTER_NAME}
export PROJECT_ROOT=${PROJECT_ROOT}
EOF

    log_success "Environment configuration saved to: $config_file"
}

# Verify environment setup
verify_environment() {
    log_step "Verifying environment setup..."

    local verification_failed=false

    # Check databases
    log_info "Testing database connections..."

    # Test main database
    if PGPASSWORD=slm_password_dev psql -h localhost -p 5433 -U slm_user -d action_history -c "SELECT 1;" >/dev/null 2>&1; then
        log_success "✓ Main database connection successful"
    else
        log_error "✗ Main database connection failed"
        verification_failed=true
    fi

    # Test vector database
    if PGPASSWORD=vector_password_dev psql -h localhost -p 5434 -U vector_user -d vector_store -c "SELECT 1;" >/dev/null 2>&1; then
        log_success "✓ Vector database connection successful"
    else
        log_error "✗ Vector database connection failed"
        verification_failed=true
    fi

    # Test Redis
    if echo "PING" | redis-cli -h localhost -p 6380 -a integration_redis_password --no-auth-warning >/dev/null 2>&1; then
        log_success "✓ Redis connection successful"
    else
        log_error "✗ Redis connection failed"
        verification_failed=true
    fi

    # Test Kubernetes
    if kubectl get nodes >/dev/null 2>&1; then
        local node_count=$(kubectl get nodes --no-headers | wc -l | xargs)
        log_success "✓ Kubernetes cluster accessible (${node_count} nodes)"
    else
        log_error "✗ Kubernetes cluster not accessible"
        verification_failed=true
    fi

    # Test LLM
    if curl -s "${LLM_ENDPOINT}/v1/models" >/dev/null 2>&1; then
        log_success "✓ LLM service accessible"
    else
        log_error "✗ LLM service not accessible"
        verification_failed=true
    fi

    # Test application builds
    if [ -f "${PROJECT_ROOT}/bin/kubernaut" ]; then
        log_success "✓ Kubernaut application built"
    else
        log_error "✗ Kubernaut application not built"
        verification_failed=true
    fi

    if [ "$verification_failed" = true ]; then
        log_error "Environment verification failed"
        exit 1
    fi

    log_success "Environment verification completed successfully"
}

# Show final status and usage information
show_usage_info() {
    echo ""
    echo "🎉 Development Environment Bootstrap Complete!"
    echo "=============================================="
    echo ""
    echo "📋 Services Running:"
    echo "  • PostgreSQL (main):    localhost:5433/action_history"
    echo "  • PostgreSQL (vector):  localhost:5434/vector_store"
    echo "  • Redis Cache:          localhost:6380"
    echo "  • Kubernetes Cluster:   kind-${CLUSTER_NAME}"
    echo "  • LLM Service:          ${LLM_ENDPOINT}"
    echo ""
    echo "🔧 Environment Setup:"
    echo "  source .env.development"
    echo ""
    echo "🧪 Run Integration Tests:"
    echo "  make test-integration              # Run all integration tests"
    echo "  ./scripts/run-tests.sh            # Alternative test runner"
    echo "  ./scripts/run-tests.sh --category infrastructure  # Specific category"
    echo ""
    echo "🔍 Service Management:"
    echo "  kubectl get pods -A               # Check all pods"
    echo "  kubectl port-forward svc/prometheus 9090:9090 -n monitoring  # Access Prometheus"
    echo "  ./test/integration/scripts/bootstrap-integration-tests.sh status  # Database status"
    echo ""
    echo "🧹 Cleanup Environment:"
    echo "  ./scripts/cleanup-dev-environment.sh"
    echo ""
    echo "📁 Configuration saved to: ${PROJECT_ROOT}/.env.development"
    echo ""
}

# Main execution
main() {
    local start_time=$(date +%s)

    echo "🚀 Kubernaut Development Environment Bootstrap"
    echo "=============================================="
    echo ""
    echo "This script will setup:"
    echo "  ✓ PostgreSQL with pgvector extension"
    echo "  ✓ Vector Database (separate PostgreSQL)"
    echo "  ✓ Redis Cache"
    echo "  ✓ Kind Kubernetes cluster"
    echo "  ✓ Prometheus monitoring stack"
    echo "  ✓ Kubernaut application build"
    echo "  ✓ Wait for LLM model at localhost:8080"
    echo ""

    # Handle command line arguments
    case "${1:-}" in
        --help|-h)
            echo "Usage: $0 [--help]"
            echo ""
            echo "Bootstraps complete Kubernaut development environment."
            echo ""
            echo "Prerequisites:"
            echo "  - podman (with podman-compose)"
            echo "  - kind"
            echo "  - kubectl"
            echo "  - go"
            echo "  - LocalAI running at localhost:8080"
            echo ""
            echo "After bootstrap, run tests with:"
            echo "  make test-integration"
            echo "  ./scripts/run-tests.sh"
            echo ""
            echo "Clean up with:"
            echo "  ./scripts/cleanup-dev-environment.sh"
            exit 0
            ;;
        *)
            # Continue with main execution
            ;;
    esac

    check_prerequisites
    setup_databases
    setup_kubernetes
    build_kubernaut
    wait_for_llm
    generate_environment_config
    verify_environment
    show_usage_info

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_success "🎉 Bootstrap completed successfully in ${duration} seconds!"
    echo ""
    echo "Next steps:"
    echo "  1. source .env.development"
    echo "  2. make test-integration"
}

# Execute main function with all arguments
main "$@"
