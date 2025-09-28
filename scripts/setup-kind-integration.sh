#!/bin/bash
# Setup Kind cluster for integration testing
# This script creates a real Kubernetes cluster using Kind for integration tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CLUSTER_NAME="kubernaut-integration"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites for Kind cluster..."

    if ! command -v kind &> /dev/null; then
        log_error "Kind is not installed. Install with: brew install kind"
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Install with: brew install kubectl"
        exit 1
    fi

    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not running"
        exit 1
    fi

    # Check if Docker is running
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker Desktop"
        exit 1
    fi

    log_success "All prerequisites satisfied"
}

# Setup Kind cluster
setup_kind_cluster() {
    log_info "Setting up Kind cluster: ${CLUSTER_NAME}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_warning "Kind cluster '${CLUSTER_NAME}' already exists"
        read -p "Do you want to recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Deleting existing cluster..."
            kind delete cluster --name="${CLUSTER_NAME}"
        else
            log_info "Using existing cluster"
            return 0
        fi
    fi

    # Create cluster with simple configuration
    log_info "Creating Kind cluster with configuration..."
    cd "$PROJECT_ROOT"

    # Use Docker as provider (not Podman)
    unset KIND_EXPERIMENTAL_PROVIDER || true

    if kind create cluster --name="${CLUSTER_NAME}" --config="test/kind/kind-config-simple.yaml"; then
        log_success "Kind cluster created successfully"
    else
        log_error "Failed to create Kind cluster"
        exit 1
    fi

    # Wait for cluster to be ready
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=300s

    log_success "Kind cluster is ready"
}

# Configure kubectl context
configure_kubectl() {
    log_info "Configuring kubectl context..."

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

# Update integration environment
update_integration_env() {
    log_info "Updating integration environment configuration..."

    cd "$PROJECT_ROOT"

    # Update .env.integration with Kind cluster configuration
    cat >> .env.integration << EOF

# Kind Cluster Configuration
USE_KIND_CLUSTER=true
KIND_CLUSTER_NAME=${CLUSTER_NAME}
KUBECONFIG=${HOME}/.kube/config
USE_FAKE_K8S_CLIENT=false
USE_REAL_CLUSTER=true
EOF

    log_success "Integration environment updated"
}

# Main function
main() {
    echo "🚀 Setting up Kind cluster for integration testing"
    echo "================================================="
    echo ""

    check_prerequisites
    setup_kind_cluster
    configure_kubectl
    update_integration_env

    echo ""
    log_success "🎉 Kind cluster setup completed!"
    echo ""
    echo "Cluster Information:"
    echo "  Name: ${CLUSTER_NAME}"
    echo "  Context: kind-${CLUSTER_NAME}"
    echo "  Nodes: $(kubectl get nodes --no-headers | wc -l | xargs)"
    echo ""
    echo "Next steps:"
    echo "  1. source .env.integration"
    echo "  2. make test-integration-dev"
    echo ""
    echo "To delete the cluster:"
    echo "  kind delete cluster --name=${CLUSTER_NAME}"
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Sets up a Kind Kubernetes cluster for integration testing."
        echo ""
        echo "Prerequisites:"
        echo "  - Docker Desktop running"
        echo "  - kind installed (brew install kind)"
        echo "  - kubectl installed (brew install kubectl)"
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac
