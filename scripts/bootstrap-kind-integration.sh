#!/bin/bash

# Kind Integration Environment Bootstrap Script
# Sets up complete kubernaut integration environment using Kind cluster
#
# Components setup:
# - Kind Kubernetes cluster (1 control-plane + 2 workers)
# - All services deployed as Kubernetes resources
# - PostgreSQL with pgvector extension
# - Redis cache
# - Prometheus + AlertManager monitoring
# - Kubernaut webhook + AI services
# - HolmesGPT integration
# - Waits for external LLM model at 192.168.1.169:8080

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"
CLUSTER_NAME="kubernaut-integration"
LLM_ENDPOINT="http://192.168.1.169:8080"
LLM_WAIT_TIMEOUT=60
NAMESPACE="kubernaut-integration"

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

    # Check Kind
    if ! command -v kind &> /dev/null; then
        missing_deps+=("kind")
    fi

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        missing_deps+=("kubectl")
    fi

    # Check Docker or Podman
    if ! command -v docker &> /dev/null && ! command -v podman &> /dev/null; then
        missing_deps+=("docker or podman")
    fi

    # Check Go
    if ! command -v go &> /dev/null; then
        missing_deps+=("go")
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Install missing dependencies:"
        for dep in "${missing_deps[@]}"; do
            case $dep in
                "kind")
                    echo "  - kind: brew install kind"
                    ;;
                "kubectl")
                    echo "  - kubectl: brew install kubectl"
                    ;;
                "docker or podman")
                    echo "  - docker: brew install docker"
                    echo "  - OR podman: brew install podman"
                    ;;
                "go")
                    echo "  - go: brew install go"
                    ;;
            esac
        done
        exit 1
    fi

    # Check if Docker/Podman is running
    if command -v docker &> /dev/null; then
        if ! docker info &> /dev/null; then
            log_error "Docker is not running. Please start Docker Desktop."
            exit 1
        fi
        log_info "Using Docker as container runtime"
    elif command -v podman &> /dev/null; then
        if ! podman info &> /dev/null; then
            log_error "Podman is not running. Please start podman machine."
            log_info "Run: podman machine start"
            exit 1
        fi
        log_info "Using Podman as container runtime"
        # Set Kind to use Podman
        export KIND_EXPERIMENTAL_PROVIDER=podman
    fi

    log_success "All prerequisites satisfied"
}

# Create Kind cluster
create_kind_cluster() {
    log_step "Setting up Kind cluster: ${CLUSTER_NAME}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_warning "Kind cluster '${CLUSTER_NAME}' already exists"
        log_info "Using existing cluster"
        return 0
    fi

    # Create cluster with configuration
    log_info "Creating Kind cluster with configuration..."
    cd "$PROJECT_ROOT"

    if kind create cluster --name="${CLUSTER_NAME}" --config="test/kind/kind-config-simple.yaml"; then
        log_success "Kind cluster created successfully"
    else
        log_error "Failed to create Kind cluster"
        exit 1
    fi

    # Wait for cluster to be ready
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=300s --context "kind-${CLUSTER_NAME}"

    log_success "Kind cluster is ready"
}

# Configure kubectl context
configure_kubectl() {
    log_step "Configuring kubectl context..."

    # Set the context
    kubectl config use-context "kind-${CLUSTER_NAME}"

    # Verify cluster access
    if kubectl cluster-info &> /dev/null; then
        log_success "kubectl configured successfully"
        kubectl get nodes
    else
        log_error "Failed to configure kubectl"
        exit 1
    fi
}

# Build kubernaut services
build_services() {
    log_step "Building kubernaut services..."

    cd "$PROJECT_ROOT"

    # Build webhook service
    log_info "Building webhook service..."
    if make build-webhook-service; then
        log_success "Webhook service built successfully"
    else
        log_error "Failed to build webhook service"
        exit 1
    fi

    # Build AI service
    log_info "Building AI service..."
    if make build-ai-service; then
        log_success "AI service built successfully"
    else
        log_error "Failed to build AI service"
        exit 1
    fi

    # Build container images for Kind
    log_info "Building container images for Kind..."

    # Build webhook service image
    docker build -t kubernaut/webhook-service:latest -f docker/webhook-service.Dockerfile .
    kind load docker-image kubernaut/webhook-service:latest --name="${CLUSTER_NAME}"

    # Build AI service image
    docker build -t kubernaut/ai-service:latest -f docker/ai-service.Dockerfile .
    kind load docker-image kubernaut/ai-service:latest --name="${CLUSTER_NAME}"

    # Build HolmesGPT API image
    docker build -t kubernaut/holmesgpt-api:latest -f docker/holmesgpt-api/Dockerfile .
    kind load docker-image kubernaut/holmesgpt-api:latest --name="${CLUSTER_NAME}"

    log_success "All services built and loaded into Kind cluster"
}

# Deploy services to Kind cluster
deploy_services() {
    log_step "Deploying services to Kind cluster..."

    cd "$PROJECT_ROOT"

    # Deploy using Kustomize
    log_info "Applying Kubernetes manifests..."
    kubectl apply -k deploy/integration/

    # Wait for namespace to be created
    log_info "Waiting for namespace to be ready..."
    kubectl wait --for=condition=Ready --timeout=60s namespace/${NAMESPACE} || true

    # Wait for deployments to be ready
    log_info "Waiting for deployments to be ready (this may take a few minutes)..."

    # Wait for PostgreSQL first (other services depend on it)
    kubectl wait --for=condition=available --timeout=300s deployment/postgresql -n ${NAMESPACE}
    log_info "PostgreSQL is ready"

    # Wait for Redis
    kubectl wait --for=condition=available --timeout=300s deployment/redis -n ${NAMESPACE}
    log_info "Redis is ready"

    # Wait for monitoring stack
    kubectl wait --for=condition=available --timeout=300s deployment/prometheus -n ${NAMESPACE}
    kubectl wait --for=condition=available --timeout=300s deployment/alertmanager -n ${NAMESPACE}
    log_info "Monitoring stack is ready"

    # Wait for kubernaut services
    kubectl wait --for=condition=available --timeout=300s deployment/webhook-service -n ${NAMESPACE}
    kubectl wait --for=condition=available --timeout=300s deployment/ai-service -n ${NAMESPACE}
    kubectl wait --for=condition=available --timeout=300s deployment/holmesgpt -n ${NAMESPACE}

    log_success "All services deployed successfully"
}

# Wait for external LLM
wait_for_llm() {
    log_step "Checking external LLM availability..."

    log_info "Checking LLM endpoint: ${LLM_ENDPOINT}"

    local count=0
    while [ $count -lt $LLM_WAIT_TIMEOUT ]; do
        if curl -s --connect-timeout 5 "${LLM_ENDPOINT}/health" > /dev/null 2>&1 || \
           curl -s --connect-timeout 5 "${LLM_ENDPOINT}/api/version" > /dev/null 2>&1; then
            log_success "LLM endpoint is available"
            return 0
        fi

        if [ $((count % 10)) -eq 0 ]; then
            log_info "Waiting for LLM at ${LLM_ENDPOINT}... (${count}/${LLM_WAIT_TIMEOUT}s)"
        fi

        sleep 1
        ((count++))
    done

    log_warning "LLM endpoint not available after ${LLM_WAIT_TIMEOUT}s"
    log_info "Services will start but AI functionality may be limited"
    log_info "Ensure LLM is running at ${LLM_ENDPOINT}"
}

# Verify environment
verify_environment() {
    log_step "Verifying environment..."

    # Check cluster status
    log_info "Cluster status:"
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"

    # Check services
    log_info "Service status:"
    kubectl get pods,svc -n ${NAMESPACE}

    # Check service health
    log_info "Checking service health..."

    # Wait a bit for services to fully start
    sleep 10

    # Check if services are responding
    local webhook_ready=false
    local prometheus_ready=false

    # Check webhook service (via NodePort)
    if curl -s --connect-timeout 5 "http://localhost:30800/health" > /dev/null 2>&1; then
        webhook_ready=true
        log_success "Webhook service is responding"
    else
        log_warning "Webhook service not responding yet"
    fi

    # Check Prometheus (via NodePort)
    if curl -s --connect-timeout 5 "http://localhost:30090/-/ready" > /dev/null 2>&1; then
        prometheus_ready=true
        log_success "Prometheus is responding"
    else
        log_warning "Prometheus not responding yet"
    fi

    if [ "$webhook_ready" = true ] && [ "$prometheus_ready" = true ]; then
        log_success "Environment verification completed successfully"
    else
        log_warning "Some services may still be starting up"
        log_info "Use 'make kind-status' to check service status"
    fi
}

# Generate environment configuration
generate_environment_config() {
    log_step "Generating environment configuration..."

    cd "$PROJECT_ROOT"

    cat > .env.kind-integration << EOF
# Kind Integration Environment Configuration
# Generated by bootstrap-kind-integration.sh

# Cluster configuration
KIND_CLUSTER_NAME=${CLUSTER_NAME}
KUBECONFIG=${HOME}/.kube/config
KUBECTL_CONTEXT=kind-${CLUSTER_NAME}

# Service endpoints (NodePort access)
WEBHOOK_SERVICE_URL=http://localhost:30800
PROMETHEUS_URL=http://localhost:30090
ALERTMANAGER_URL=http://localhost:30093
POSTGRESQL_URL=postgresql://slm_user:slm_password_dev@localhost:30432/action_history

# LLM configuration
LLM_ENDPOINT=${LLM_ENDPOINT}
LLM_PROVIDER=ollama
LLM_MODEL=hf://ggml-org/gpt-oss-20b-GGUF

# Testing configuration
USE_KIND_CLUSTER=true
USE_FAKE_K8S_CLIENT=false
INTEGRATION_NAMESPACE=${NAMESPACE}

# Environment type
ENVIRONMENT=integration
DEPLOYMENT_METHOD=kind-cluster
EOF

    log_success "Environment configuration saved to .env.kind-integration"
}

# Show usage information
show_usage_info() {
    log_step "Environment ready!"

    echo ""
    echo "🎉 Kind Integration Environment Bootstrap Complete!"
    echo "=================================================="
    echo ""
    echo "📋 Services Available:"
    echo "  • Webhook Service:    http://localhost:30800"
    echo "  • Prometheus:         http://localhost:30090"
    echo "  • AlertManager:       http://localhost:30093"
    echo "  • PostgreSQL:         localhost:30432"
    echo ""
    echo "🔧 Management Commands:"
    echo "  • Check status:       make kind-status"
    echo "  • View logs:          make kind-logs"
    echo "  • Deploy services:    make kind-deploy"
    echo "  • Remove services:    make kind-undeploy"
    echo "  • Clean up:           make cleanup-dev-kind"
    echo ""
    echo "🧪 Testing Commands:"
    echo "  • Run integration tests:  make test-integration-dev"
    echo "  • Run AI tests:          make test-ai-dev"
    echo "  • Run quick tests:       make test-quick-dev"
    echo ""
    echo "📊 Monitoring:"
    echo "  • kubectl get pods -n ${NAMESPACE}"
    echo "  • kubectl logs -f deployment/webhook-service -n ${NAMESPACE}"
    echo "  • kubectl logs -f deployment/ai-service -n ${NAMESPACE}"
    echo ""
    echo "⚙️  Configuration:"
    echo "  • Environment file:   .env.kind-integration"
    echo "  • Kubectl context:    kind-${CLUSTER_NAME}"
    echo "  • Namespace:          ${NAMESPACE}"
    echo ""
    if [ -f ".env.kind-integration" ]; then
        echo "💡 Load environment: source .env.kind-integration"
    fi
    echo ""
}

# Main execution
main() {
    local start_time
    start_time=$(date +%s)

    echo "🚀 Kubernaut Kind Integration Environment Bootstrap"
    echo "=================================================="
    echo ""
    echo "This script will setup:"
    echo "  ✓ Kind Kubernetes cluster (1 control-plane + 2 workers)"
    echo "  ✓ PostgreSQL with pgvector extension"
    echo "  ✓ Redis cache"
    echo "  ✓ Prometheus + AlertManager monitoring"
    echo "  ✓ Kubernaut webhook + AI services"
    echo "  ✓ HolmesGPT integration"
    echo "  ✓ Wait for external LLM at ${LLM_ENDPOINT}"
    echo ""

    # Handle command line arguments
    case "${1:-}" in
        --help|-h)
            echo "Usage: $0 [--help]"
            echo ""
            echo "Bootstraps complete Kubernaut integration environment using Kind cluster."
            echo ""
            echo "Prerequisites:"
            echo "  - kind (brew install kind)"
            echo "  - kubectl (brew install kubectl)"
            echo "  - docker or podman running"
            echo "  - go (brew install go)"
            echo "  - LLM model running at ${LLM_ENDPOINT}"
            echo ""
            echo "After bootstrap, run tests with:"
            echo "  make test-integration-dev"
            echo ""
            echo "Clean up with:"
            echo "  make cleanup-dev-kind"
            exit 0
            ;;
        *)
            # Continue with main execution
            ;;
    esac

    check_prerequisites
    create_kind_cluster
    configure_kubectl
    build_services
    deploy_services
    wait_for_llm
    verify_environment
    generate_environment_config
    show_usage_info

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_success "🎉 Bootstrap completed successfully in ${duration} seconds!"
    echo ""
    echo "Next steps:"
    echo "  1. source .env.kind-integration"
    echo "  2. make test-integration-dev"
}

# Execute main function
main "$@"
