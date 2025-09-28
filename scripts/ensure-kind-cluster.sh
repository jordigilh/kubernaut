#!/bin/bash
# Ensure Kind cluster is running for integration tests
# This script MUST be run before integration tests to guarantee real cluster

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CLUSTER_NAME="kubernaut-integration"
REQUIRED_NODES=2

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

# Check if Docker Desktop is running (not Podman)
check_docker_desktop() {
    log_info "Checking Docker Desktop status..."

    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed"
        log_error "Please install Docker Desktop from: https://www.docker.com/products/docker-desktop"
        exit 1
    fi

    # Check if Docker daemon is running
    if ! docker info &> /dev/null; then
        log_error "Docker Desktop is not running"
        log_error "Please start Docker Desktop and try again"
        exit 1
    fi

    # Check if we're using Docker Desktop (not Podman)
    local docker_context=$(docker context show 2>/dev/null || echo "unknown")
    local docker_endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || echo "unknown")

    if [[ "$docker_endpoint" == *"podman"* ]]; then
        log_error "Docker is configured to use Podman socket"
        log_error "Please reset Docker to use Docker Desktop:"
        log_error "  unset DOCKER_HOST"
        log_error "  docker context use default"
        exit 1
    fi

    log_success "Docker Desktop is running and configured correctly"
}

# Check if Kind is available
check_kind() {
    log_info "Checking Kind availability..."

    if ! command -v kind &> /dev/null; then
        log_error "Kind is not installed"
        log_error "Please install Kind:"
        log_error "  brew install kind"
        log_error "  # or"
        log_error "  go install sigs.k8s.io/kind@latest"
        exit 1
    fi

    log_success "Kind is available"
}

# Check if kubectl is available
check_kubectl() {
    log_info "Checking kubectl availability..."

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed"
        log_error "Please install kubectl:"
        log_error "  brew install kubectl"
        exit 1
    fi

    log_success "kubectl is available"
}

# Create Kind cluster with retry logic
create_kind_cluster() {
    log_info "Creating Kind cluster: ${CLUSTER_NAME}"

    cd "$PROJECT_ROOT"

    # Ensure we're using Docker (not Podman)
    unset DOCKER_HOST || true
    unset KIND_EXPERIMENTAL_PROVIDER || true

    # Create cluster with retries
    local max_retries=3
    local retry=0

    while [ $retry -lt $max_retries ]; do
        log_info "Attempt $((retry + 1))/$max_retries to create Kind cluster..."

        if kind create cluster --name="${CLUSTER_NAME}" --config="test/kind/kind-config-simple.yaml"; then
            log_success "Kind cluster created successfully"
            break
        else
            retry=$((retry + 1))
            if [ $retry -lt $max_retries ]; then
                log_warning "Failed to create cluster, retrying in 10 seconds..."
                sleep 10

                # Clean up any partial cluster
                kind delete cluster --name="${CLUSTER_NAME}" 2>/dev/null || true
            else
                log_error "Failed to create Kind cluster after $max_retries attempts"
                log_error "Please check Docker Desktop is running and try again"
                exit 1
            fi
        fi
    done
}

# Validate cluster is running and accessible
validate_cluster() {
    log_info "Validating Kind cluster..."

    # Check if cluster exists
    if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_error "Kind cluster '${CLUSTER_NAME}' does not exist"
        return 1
    fi

    # Set kubectl context
    kubectl config use-context "kind-${CLUSTER_NAME}" || {
        log_error "Failed to set kubectl context"
        return 1
    }

    # Wait for nodes to be ready
    log_info "Waiting for cluster nodes to be ready..."
    if ! kubectl wait --for=condition=Ready nodes --all --timeout=300s; then
        log_error "Cluster nodes did not become ready within timeout"
        return 1
    fi

    # Verify we have the expected number of nodes
    local node_count=$(kubectl get nodes --no-headers | wc -l | xargs)
    if [ "$node_count" -lt "$REQUIRED_NODES" ]; then
        log_error "Expected at least $REQUIRED_NODES nodes, but found $node_count"
        return 1
    fi

    # Test basic cluster functionality
    log_info "Testing cluster functionality..."
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cluster is not responding to API requests"
        return 1
    fi

    # Create a test namespace to verify write permissions
    local test_ns="kind-validation-test"
    if kubectl create namespace "$test_ns" &> /dev/null; then
        kubectl delete namespace "$test_ns" &> /dev/null
        log_success "Cluster validation passed"
    else
        log_error "Cannot create resources in cluster"
        return 1
    fi

    return 0
}

# Update integration environment for Kind cluster
update_integration_env() {
    log_info "Updating integration environment for Kind cluster..."

    cd "$PROJECT_ROOT"

    # Get kubeconfig path
    local kubeconfig_path="${HOME}/.kube/config"

    # Remove old Kind-related entries
    if [ -f .env.integration ]; then
        grep -v -E "(KIND_CLUSTER|KUBECONFIG|USE_FAKE_K8S_CLIENT|USE_REAL_CLUSTER|USE_ENVTEST)" .env.integration > .env.integration.tmp || true
        mv .env.integration.tmp .env.integration
    fi

    # Add Kind cluster configuration
    cat >> .env.integration << EOF

# Kind Cluster Configuration - REAL CLUSTER REQUIRED
KIND_CLUSTER_NAME=${CLUSTER_NAME}
KUBECONFIG=${kubeconfig_path}
USE_FAKE_K8S_CLIENT=false
USE_REAL_CLUSTER=true
USE_KIND_CLUSTER=true
KUBERNETES_CONTEXT=kind-${CLUSTER_NAME}

# Integration Test Validation
REQUIRE_REAL_CLUSTER=true
FAIL_ON_FAKE_CLUSTER=true
EOF

    log_success "Integration environment updated for Kind cluster"
}

# Main function
main() {
    echo "🚀 Ensuring Kind Cluster for Integration Tests"
    echo "=============================================="
    echo ""
    echo "This script ensures a real Kind Kubernetes cluster is running"
    echo "Integration tests WILL FAIL if not running on a real cluster"
    echo ""

    # Check prerequisites
    check_docker_desktop
    check_kind
    check_kubectl

    # Check if cluster already exists and is healthy
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_info "Kind cluster '${CLUSTER_NAME}' already exists, validating..."
        if validate_cluster; then
            log_success "Existing cluster is healthy"
            update_integration_env
            show_cluster_info
            return 0
        else
            log_warning "Existing cluster is unhealthy, recreating..."
            kind delete cluster --name="${CLUSTER_NAME}"
        fi
    fi

    # Create new cluster
    create_kind_cluster

    # Validate the new cluster
    if ! validate_cluster; then
        log_error "Failed to validate newly created cluster"
        exit 1
    fi

    # Update environment
    update_integration_env

    # Show cluster information
    show_cluster_info

    echo ""
    log_success "🎉 Kind cluster is ready for integration tests!"
    echo ""
    echo "IMPORTANT: Integration tests are now configured to FAIL if not running on a real cluster"
}

# Show cluster information
show_cluster_info() {
    echo ""
    echo "📊 Cluster Information:"
    echo "  Name: ${CLUSTER_NAME}"
    echo "  Context: kind-${CLUSTER_NAME}"
    echo "  Nodes: $(kubectl get nodes --no-headers | wc -l | xargs)"
    echo "  Status: $(kubectl get nodes --no-headers | awk '{print $2}' | sort -u | tr '\n' ' ')"
    echo ""
    echo "🔧 Cluster Details:"
    kubectl get nodes -o wide
    echo ""
    echo "Next steps:"
    echo "  1. source .env.integration"
    echo "  2. make test-integration-dev"
    echo ""
    echo "To delete cluster:"
    echo "  kind delete cluster --name=${CLUSTER_NAME}"
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Ensures a Kind Kubernetes cluster is running for integration tests."
        echo "Integration tests will FAIL if not running on a real cluster."
        echo ""
        echo "Prerequisites:"
        echo "  - Docker Desktop running (not Podman)"
        echo "  - kind installed (brew install kind)"
        echo "  - kubectl installed (brew install kubectl)"
        echo ""
        echo "This script will:"
        echo "  1. Validate Docker Desktop is running"
        echo "  2. Create Kind cluster if needed"
        echo "  3. Configure integration tests to require real cluster"
        echo "  4. Make tests FAIL if fake cluster is detected"
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac
