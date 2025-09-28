#!/bin/bash

# Development Environment Cleanup Script
# Destroys all components except LLM model at localhost:8080
#
# Components cleaned:
# - Kind Kubernetes cluster
# - PostgreSQL containers
# - Vector Database containers
# - Redis containers
# - Container networks and volumes
# - Built binaries
# - Environment configuration

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"
CLUSTER_NAME="kubernaut-dev"

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

# Clean up Kubernetes cluster
cleanup_kubernetes() {
    log_step "Cleaning up Kubernetes cluster..."

    # Check if cluster exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_info "Deleting Kind cluster: ${CLUSTER_NAME}"
        kind delete cluster --name="${CLUSTER_NAME}"
        log_success "Kind cluster deleted"
    else
        log_warning "Kind cluster '${CLUSTER_NAME}' not found, skipping"
    fi

    # Clean up kubectl contexts
    if kubectl config get-contexts -o name 2>/dev/null | grep -q "kind-${CLUSTER_NAME}"; then
        log_info "Removing kubectl context: kind-${CLUSTER_NAME}"
        kubectl config delete-context "kind-${CLUSTER_NAME}" >/dev/null 2>&1 || log_warning "Failed to delete kubectl context"
    fi
}

# Clean up databases and Redis
cleanup_databases() {
    log_step "Cleaning up databases and Redis..."

    cd "$PROJECT_ROOT"

    # Use the existing bootstrap script to stop services
    local bootstrap_script="test/integration/scripts/bootstrap-integration-tests.sh"

    if [ -f "$bootstrap_script" ]; then
        log_info "Stopping database services..."
        chmod +x "$bootstrap_script"
        "$bootstrap_script" stop || log_warning "Database cleanup script failed, continuing with manual cleanup"
    else
        log_warning "Database bootstrap script not found, performing manual cleanup"
    fi

    # Manual cleanup of containers if bootstrap script failed
    local containers=(
        "kubernaut-integration-postgres"
        "kubernaut-integration-vectordb"
        "kubernaut-integration-redis"
        "kubernaut-postgres-test"
        "kubernaut-vector-test"
    )

    for container in "${containers[@]}"; do
        if podman ps -a --format "{{.Names}}" | grep -q "^${container}$"; then
            log_info "Removing container: $container"
            podman stop "$container" >/dev/null 2>&1 || true
            podman rm "$container" >/dev/null 2>&1 || true
        fi
    done

    # Clean up volumes
    local volumes=(
        "kubernaut-postgres-data"
        "kubernaut-vector-data"
        "postgres_integration_data"
        "vector_integration_data"
        "redis_integration_data"
    )

    for volume in "${volumes[@]}"; do
        if podman volume exists "$volume" 2>/dev/null; then
            log_info "Removing volume: $volume"
            podman volume rm "$volume" >/dev/null 2>&1 || log_warning "Failed to remove volume: $volume"
        fi
    done

    # Clean up networks
    local networks=(
        "kubernaut-test-network"
        "kubernaut-integration-test"
    )

    for network in "${networks[@]}"; do
        if podman network exists "$network" 2>/dev/null; then
            log_info "Removing network: $network"
            podman network rm "$network" >/dev/null 2>&1 || log_warning "Failed to remove network: $network"
        fi
    done

    log_success "Database and Redis cleanup completed"
}

# Clean up container registry
cleanup_registry() {
    log_step "Cleaning up container registry..."

    local registry_name="kind-registry"

    if podman ps -a --format "{{.Names}}" | grep -q "^${registry_name}$"; then
        log_info "Removing container registry: $registry_name"
        podman stop "$registry_name" >/dev/null 2>&1 || true
        podman rm "$registry_name" >/dev/null 2>&1 || true
        log_success "Container registry removed"
    else
        log_warning "Container registry not found, skipping"
    fi
}

# Clean up built binaries
cleanup_binaries() {
    log_step "Cleaning up built binaries..."

    cd "$PROJECT_ROOT"

    if [ -d "bin" ]; then
        log_info "Removing built binaries from bin/ directory"
        rm -rf bin/
        log_success "Built binaries removed"
    else
        log_warning "bin/ directory not found, skipping"
    fi

    # Clean up other generated files
    local generated_files=(
        "main"
        "kubernaut"
        "kubernaut"
        "kubernaut"
        "mcp-server"
        "test-context-performance"
    )

    for file in "${generated_files[@]}"; do
        if [ -f "$file" ]; then
            log_info "Removing generated file: $file"
            rm -f "$file"
        fi
    done
}

# Clean up environment configuration
cleanup_environment_config() {
    log_step "Cleaning up environment configuration..."

    local env_file="${PROJECT_ROOT}/.env.development"
    local connection_file="/tmp/kubernaut-container-connections.env"

    if [ -f "$env_file" ]; then
        log_info "Removing environment configuration: $env_file"
        rm -f "$env_file"
    fi

    if [ -f "$connection_file" ]; then
        log_info "Removing connection configuration: $connection_file"
        rm -f "$connection_file"
    fi

    # Clean up temporary files
    rm -rf /tmp/milestone1-validation-* 2>/dev/null || true
    rm -rf /tmp/kind-config-*.yaml 2>/dev/null || true

    log_success "Environment configuration cleaned"
}

# Verify cleanup completion
verify_cleanup() {
    log_step "Verifying cleanup completion..."

    local cleanup_issues=false

    # Check for remaining containers
    local remaining_containers=$(podman ps -a --format "{{.Names}}" | grep -E "(kubernaut|integration)" | head -5)
    if [ -n "$remaining_containers" ]; then
        log_warning "Some containers may still exist:"
        echo "$remaining_containers" | while read container; do
            echo "  - $container"
        done
        cleanup_issues=true
    fi

    # Check for remaining volumes
    local remaining_volumes=$(podman volume ls --format "{{.Name}}" | grep -E "(kubernaut|integration)" | head -5)
    if [ -n "$remaining_volumes" ]; then
        log_warning "Some volumes may still exist:"
        echo "$remaining_volumes" | while read volume; do
            echo "  - $volume"
        done
        cleanup_issues=true
    fi

    # Check Kind clusters
    local remaining_clusters=$(kind get clusters 2>/dev/null | grep -v "^No kind clusters" | head -3)
    if [ -n "$remaining_clusters" ]; then
        log_warning "Some Kind clusters may still exist:"
        echo "$remaining_clusters" | while read cluster; do
            echo "  - $cluster"
        done
    fi

    if [ "$cleanup_issues" = false ]; then
        log_success "Cleanup verification passed - environment is clean"
    else
        log_warning "Some cleanup issues detected (see above)"
        echo ""
        echo "Manual cleanup commands:"
        echo "  podman ps -a | grep kubernaut | awk '{print \$1}' | xargs podman rm -f"
        echo "  podman volume ls | grep kubernaut | awk '{print \$2}' | xargs podman volume rm"
        echo "  kind get clusters | xargs -I {} kind delete cluster --name {}"
    fi
}

# Show cleanup summary
show_cleanup_summary() {
    echo ""
    echo "🧹 Development Environment Cleanup Complete!"
    echo "============================================="
    echo ""
    echo "📋 Components Cleaned:"
    echo "  ✓ Kind Kubernetes cluster (${CLUSTER_NAME})"
    echo "  ✓ PostgreSQL containers and volumes"
    echo "  ✓ Vector Database containers and volumes"
    echo "  ✓ Redis containers and volumes"
    echo "  ✓ Container networks"
    echo "  ✓ Built binaries and artifacts"
    echo "  ✓ Environment configuration files"
    echo ""
    echo "🔧 What Remains:"
    echo "  • LLM service at localhost:8080 (preserved)"
    echo "  • Source code and configuration files"
    echo "  • System dependencies (podman, kind, etc.)"
    echo ""
    echo "🚀 To setup environment again:"
    echo "  ./scripts/bootstrap-dev-environment.sh"
    echo ""
    echo "💡 Pro tip: Your LocalAI model is still running and ready for the next bootstrap!"
    echo ""
}

# Main execution
main() {
    local start_time=$(date +%s)

    echo "🧹 Kubernaut Development Environment Cleanup"
    echo "============================================"
    echo ""
    echo "This script will clean up:"
    echo "  ✓ Kind Kubernetes cluster"
    echo "  ✓ PostgreSQL and Vector Database containers"
    echo "  ✓ Redis cache container"
    echo "  ✓ Container networks and volumes"
    echo "  ✓ Built binaries"
    echo "  ✓ Environment configuration"
    echo ""
    echo "⚠️  LLM service at localhost:8080 will be preserved"
    echo ""

    # Handle command line arguments
    case "${1:-}" in
        --help|-h)
            echo "Usage: $0 [--help] [--force]"
            echo ""
            echo "Options:"
            echo "  --help    Show this help message"
            echo "  --force   Skip confirmation prompts"
            echo ""
            echo "This script cleans up the complete development environment"
            echo "created by bootstrap-dev-environment.sh, except for the"
            echo "LLM service which continues running."
            exit 0
            ;;
        --force)
            log_warning "Force mode enabled, skipping confirmation"
            ;;
        "")
            # Ask for confirmation
            echo -n "Proceed with cleanup? [y/N]: "
            read -r response
            if [[ ! "$response" =~ ^[Yy]$ ]]; then
                log_info "Cleanup cancelled by user"
                exit 0
            fi
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac

    echo ""
    log_info "Starting cleanup process..."

    cleanup_kubernetes
    cleanup_databases
    cleanup_registry
    cleanup_binaries
    cleanup_environment_config
    verify_cleanup
    show_cleanup_summary

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_success "🎉 Cleanup completed successfully in ${duration} seconds!"
}

# Execute main function with all arguments
main "$@"
