#!/bin/bash

set -euo pipefail

CLUSTER_NAME="kubernaut-test"
REGISTRY_NAME="kind-registry"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

# Check if kind is available
check_kind() {
    if ! command -v kind &> /dev/null; then
        warn "KinD is not installed, skipping Kind cluster cleanup"
        return 1
    fi
    return 0
}

# Check if podman is available
check_podman() {
    if ! command -v podman &> /dev/null; then
        warn "Podman is not installed, skipping registry cleanup"
        return 1
    fi
    return 0
}

# Delete Kind cluster
cleanup_cluster() {
    log "Cleaning up KinD cluster: ${CLUSTER_NAME}"

    # Check if cluster exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log "Deleting KinD cluster: ${CLUSTER_NAME}"
        kind delete cluster --name="${CLUSTER_NAME}"
        log "KinD cluster deleted successfully"
    else
        log "KinD cluster ${CLUSTER_NAME} does not exist, skipping"
    fi
}

# Cleanup local registry
cleanup_registry() {
    log "Cleaning up local registry: ${REGISTRY_NAME}"

    # Check if registry container exists
    if podman ps -a --format "{{.Names}}" | grep -q "^${REGISTRY_NAME}$"; then
        log "Stopping and removing registry container: ${REGISTRY_NAME}"
        podman stop "${REGISTRY_NAME}" 2>/dev/null || true
        podman rm "${REGISTRY_NAME}" 2>/dev/null || true
        log "Registry container cleaned up successfully"
    else
        log "Registry container ${REGISTRY_NAME} does not exist, skipping"
    fi
}

# Cleanup bootstrap directory
cleanup_bootstrap() {
    log "Cleaning up bootstrap directory..."

    if [[ -d "/tmp/kind-bootstrap" ]]; then
        rm -rf /tmp/kind-bootstrap
        log "Bootstrap directory cleaned up"
    else
        log "Bootstrap directory does not exist, skipping"
    fi
}

# Cleanup kubectl context
cleanup_kubectl_context() {
    log "Cleaning up kubectl context..."

    local context_name="kind-${CLUSTER_NAME}"
    if kubectl config get-contexts --no-headers | grep -q "${context_name}"; then
        kubectl config delete-context "${context_name}" 2>/dev/null || true
        log "kubectl context ${context_name} removed"
    else
        log "kubectl context ${context_name} does not exist, skipping"
    fi
}

# Force cleanup (remove all kind clusters and registries)
force_cleanup() {
    log "Performing force cleanup of all Kind resources..."

    if check_kind; then
        log "Deleting all Kind clusters..."
        kind get clusters 2>/dev/null | xargs -r -I {} kind delete cluster --name={}
    fi

    if check_podman; then
        log "Stopping all kind-registry containers..."
        podman ps -a --format "{{.Names}}" | grep kind-registry | xargs -r -I {} podman rm -f {}
    fi

    cleanup_bootstrap

    log "Force cleanup completed"
}

# Main execution
main() {
    log "🧹 Cleaning up KinD cluster and related resources..."
    log "Strategy: Kind for CI/CD and local testing, OCP for E2E"
    echo ""

    if check_kind; then
        cleanup_cluster
        cleanup_kubectl_context
    fi

    if check_podman; then
        cleanup_registry
    fi

    cleanup_bootstrap

    log "✅ KinD cluster cleanup completed!"
    echo ""
    log "📋 Cleanup Summary:"
    echo "  ├── Kind cluster: ${CLUSTER_NAME} removed"
    echo "  ├── Registry: ${REGISTRY_NAME} removed"
    echo "  ├── Bootstrap: /tmp/kind-bootstrap removed"
    echo "  └── kubectl context: kind-${CLUSTER_NAME} removed"
    echo ""
    log "🚀 Ready for fresh setup:"
    echo "  make test-integration-kind     # Local development with Kind"
    echo "  make test-ci                   # CI/CD with mocked LLM"
    echo "  make test-e2e-ocp             # Production E2E with OCP"
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--force|--help]"
        echo ""
        echo "Cleans up KinD cluster and related resources for integration testing."
        echo ""
        echo "Options:"
        echo "  --force     Force cleanup of all Kind clusters and registries"
        echo "  --help      Show this help message"
        echo ""
        echo "Default behavior:"
        echo "  - Removes the specific cluster: ${CLUSTER_NAME}"
        echo "  - Removes the registry: ${REGISTRY_NAME}"
        echo "  - Cleans up bootstrap directory and kubectl context"
        exit 0
        ;;
    --force)
        force_cleanup
        ;;
    *)
        main "$@"
        ;;
esac