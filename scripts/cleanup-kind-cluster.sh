#!/bin/bash

set -euo pipefail

CLUSTER_NAME="prometheus-alerts-slm-test"
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

# Clean up kind cluster
cleanup_cluster() {
    log "Cleaning up KinD cluster: ${CLUSTER_NAME}"
    
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        kind delete cluster --name="${CLUSTER_NAME}"
        log "KinD cluster deleted"
    else
        warn "KinD cluster ${CLUSTER_NAME} not found"
    fi
}

# Clean up registry
cleanup_registry() {
    log "Cleaning up local registry: ${REGISTRY_NAME}"
    
    if command -v podman &> /dev/null; then
        if podman ps --format "{{.Names}}" | grep -q "^${REGISTRY_NAME}$"; then
            podman stop "${REGISTRY_NAME}"
            log "Registry stopped"
        fi
        
        if podman ps -a --format "{{.Names}}" | grep -q "^${REGISTRY_NAME}$"; then
            podman rm "${REGISTRY_NAME}"
            log "Registry container removed"
        fi
    else
        warn "Podman not found, skipping registry cleanup"
    fi
}

# Clean up kubectl context
cleanup_kubectl() {
    log "Cleaning up kubectl context"
    
    local context_name="kind-${CLUSTER_NAME}"
    
    if kubectl config get-contexts -o name | grep -q "^${context_name}$"; then
        # Switch to different context if currently using the one we're deleting
        local current_context
        current_context=$(kubectl config current-context 2>/dev/null || echo "")
        
        if [[ "${current_context}" == "${context_name}" ]]; then
            # Try to switch to default context
            if kubectl config get-contexts -o name | grep -q "^default$"; then
                kubectl config use-context default
            else
                warn "No default context found, you may need to set kubectl context manually"
            fi
        fi
        
        kubectl config delete-context "${context_name}" 2>/dev/null || warn "Failed to delete context ${context_name}"
        kubectl config delete-cluster "kind-${CLUSTER_NAME}" 2>/dev/null || warn "Failed to delete cluster config"
        kubectl config delete-user "kind-${CLUSTER_NAME}" 2>/dev/null || warn "Failed to delete user config"
        
        log "Kubectl context cleaned up"
    else
        warn "Kubectl context ${context_name} not found"
    fi
}

# Main execution
main() {
    log "Cleaning up KinD cluster and related resources..."
    
    cleanup_cluster
    cleanup_registry
    cleanup_kubectl
    
    log "Cleanup complete!"
    echo ""
    log "Current kubectl contexts:"
    kubectl config get-contexts 2>/dev/null || warn "No kubectl contexts found"
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Cleans up the KinD cluster and related resources created for e2e testing."
        echo ""
        echo "This will:"
        echo "  - Delete the KinD cluster: ${CLUSTER_NAME}"
        echo "  - Stop and remove the local registry: ${REGISTRY_NAME}"
        echo "  - Clean up kubectl contexts and cluster configurations"
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac