#!/bin/bash

# Kind Integration Environment Cleanup Script
# Cleans up kubernaut integration environment using Kind cluster
#
# This script will:
# - Remove all kubernaut services from Kind cluster
# - Optionally delete the Kind cluster
# - Clean up generated configuration files
# - Remove built container images

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"
CLUSTER_NAME="kubernaut-integration"
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

# Check if Kind cluster exists
check_cluster_exists() {
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        return 0
    else
        return 1
    fi
}

# Remove services from Kind cluster
remove_services() {
    log_step "Removing kubernaut services from Kind cluster..."

    if ! check_cluster_exists; then
        log_warning "Kind cluster '${CLUSTER_NAME}' does not exist"
        return 0
    fi

    # Set kubectl context
    kubectl config use-context "kind-${CLUSTER_NAME}" 2>/dev/null || {
        log_warning "Cannot set kubectl context for Kind cluster"
        return 0
    }

    cd "$PROJECT_ROOT"

    # Remove services using Kustomize
    if [ -f "deploy/integration/kustomization.yaml" ]; then
        log_info "Removing services using Kustomize..."
        kubectl delete -k deploy/integration/ --ignore-not-found=true --timeout=60s
        log_success "Services removed successfully"
    else
        log_warning "Kustomization file not found, removing namespace directly"
        kubectl delete namespace ${NAMESPACE} --ignore-not-found=true --timeout=60s
    fi

    # Wait for namespace to be fully deleted
    log_info "Waiting for namespace to be fully deleted..."
    kubectl wait --for=delete namespace/${NAMESPACE} --timeout=120s 2>/dev/null || {
        log_warning "Namespace deletion timeout, but continuing..."
    }
}

# Clean up container images
cleanup_images() {
    log_step "Cleaning up container images..."

    local images=(
        "kubernaut/webhook-service:latest"
        "kubernaut/ai-service:latest"
        "kubernaut/holmesgpt-api:latest"
    )

    for image in "${images[@]}"; do
        if docker image inspect "$image" &> /dev/null; then
            log_info "Removing image: $image"
            docker rmi "$image" 2>/dev/null || log_warning "Failed to remove image: $image"
        fi
    done

    # Clean up dangling images
    log_info "Cleaning up dangling images..."
    docker image prune -f &> /dev/null || log_warning "Failed to prune dangling images"

    log_success "Container images cleaned up"
}

# Delete Kind cluster
delete_cluster() {
    log_step "Deleting Kind cluster..."

    if check_cluster_exists; then
        log_info "Deleting Kind cluster: ${CLUSTER_NAME}"
        kind delete cluster --name="${CLUSTER_NAME}"
        log_success "Kind cluster deleted successfully"
    else
        log_info "Kind cluster '${CLUSTER_NAME}' does not exist"
    fi
}

# Clean up configuration files
cleanup_config() {
    log_step "Cleaning up configuration files..."

    cd "$PROJECT_ROOT"

    local config_files=(
        ".env.kind-integration"
        ".env.development"  # Legacy file that might conflict
    )

    for file in "${config_files[@]}"; do
        if [ -f "$file" ]; then
            log_info "Removing configuration file: $file"
            rm -f "$file"
        fi
    done

    log_success "Configuration files cleaned up"
}

# Clean up build artifacts
cleanup_build_artifacts() {
    log_step "Cleaning up build artifacts..."

    cd "$PROJECT_ROOT"

    # Clean Go build cache
    if command -v go &> /dev/null; then
        log_info "Cleaning Go build cache..."
        go clean -cache -modcache -testcache 2>/dev/null || log_warning "Failed to clean Go cache"
    fi

    # Remove binary files
    if [ -d "bin" ]; then
        log_info "Removing binary files..."
        rm -rf bin/
    fi

    log_success "Build artifacts cleaned up"
}

# Reset kubectl context
reset_kubectl_context() {
    log_step "Resetting kubectl context..."

    # Get current context
    local current_context
    current_context=$(kubectl config current-context 2>/dev/null || echo "")

    if [ "$current_context" = "kind-${CLUSTER_NAME}" ]; then
        log_info "Resetting kubectl context from Kind cluster"

        # Try to switch to a different context
        local available_contexts
        available_contexts=$(kubectl config get-contexts -o name 2>/dev/null | grep -v "kind-${CLUSTER_NAME}" | head -1 || echo "")

        if [ -n "$available_contexts" ]; then
            kubectl config use-context "$available_contexts"
            log_success "Switched kubectl context to: $available_contexts"
        else
            log_warning "No other kubectl contexts available"
        fi
    fi

    # Remove Kind cluster context from kubeconfig
    kubectl config delete-context "kind-${CLUSTER_NAME}" 2>/dev/null || log_info "Kind context already removed"
    kubectl config delete-cluster "kind-${CLUSTER_NAME}" 2>/dev/null || log_info "Kind cluster config already removed"
    kubectl config delete-user "kind-${CLUSTER_NAME}" 2>/dev/null || log_info "Kind user config already removed"
}

# Show cleanup summary
show_cleanup_summary() {
    log_step "Cleanup summary"

    echo ""
    echo "🧹 Kind Integration Environment Cleanup Complete!"
    echo "==============================================="
    echo ""
    echo "✅ Completed cleanup tasks:"
    echo "  • Removed kubernaut services from Kind cluster"
    echo "  • Deleted Kind cluster (if requested)"
    echo "  • Cleaned up container images"
    echo "  • Removed configuration files"
    echo "  • Cleaned up build artifacts"
    echo "  • Reset kubectl context"
    echo ""
    echo "🔄 To recreate the environment:"
    echo "  make bootstrap-dev-kind"
    echo ""
    echo "💡 Alternative environments:"
    echo "  • Docker-compose (deprecated): make bootstrap-dev-compose"
    echo "  • Production deployment: kubectl apply -k deploy/"
    echo ""
}

# Main execution
main() {
    local delete_cluster_flag=false
    local cleanup_images_flag=false
    local cleanup_build_flag=false

    echo "🧹 Kubernaut Kind Integration Environment Cleanup"
    echo "==============================================="
    echo ""

    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --delete-cluster)
                delete_cluster_flag=true
                shift
                ;;
            --cleanup-images)
                cleanup_images_flag=true
                shift
                ;;
            --cleanup-build)
                cleanup_build_flag=true
                shift
                ;;
            --all)
                delete_cluster_flag=true
                cleanup_images_flag=true
                cleanup_build_flag=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Cleans up kubernaut Kind integration environment."
                echo ""
                echo "Options:"
                echo "  --delete-cluster    Delete the Kind cluster completely"
                echo "  --cleanup-images    Remove built container images"
                echo "  --cleanup-build     Clean up build artifacts and caches"
                echo "  --all               Perform complete cleanup (all options)"
                echo "  --help, -h          Show this help message"
                echo ""
                echo "Default behavior (no options):"
                echo "  • Remove services from Kind cluster"
                echo "  • Clean up configuration files"
                echo "  • Reset kubectl context"
                echo "  • Keep Kind cluster and images for faster restart"
                echo ""
                echo "Examples:"
                echo "  $0                    # Basic cleanup, keep cluster"
                echo "  $0 --delete-cluster   # Remove services and delete cluster"
                echo "  $0 --all              # Complete cleanup"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done

    echo "Cleanup configuration:"
    echo "  • Remove services: ✓"
    echo "  • Delete cluster: $([ "$delete_cluster_flag" = true ] && echo "✓" || echo "✗")"
    echo "  • Cleanup images: $([ "$cleanup_images_flag" = true ] && echo "✓" || echo "✗")"
    echo "  • Cleanup build: $([ "$cleanup_build_flag" = true ] && echo "✓" || echo "✗")"
    echo ""

    # Confirm destructive operations
    if [ "$delete_cluster_flag" = true ]; then
        read -p "⚠️  This will delete the Kind cluster completely. Continue? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Cleanup cancelled"
            exit 0
        fi
    fi

    # Execute cleanup steps
    remove_services
    cleanup_config
    reset_kubectl_context

    if [ "$cleanup_images_flag" = true ]; then
        cleanup_images
    fi

    if [ "$cleanup_build_flag" = true ]; then
        cleanup_build_artifacts
    fi

    if [ "$delete_cluster_flag" = true ]; then
        delete_cluster
    fi

    show_cleanup_summary

    log_success "🎉 Cleanup completed successfully!"
}

# Execute main function
main "$@"
