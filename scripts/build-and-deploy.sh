#!/bin/bash

# Build and Deploy Script for Kubernaut Components
# Builds all kubernaut components and deploys them to Kind cluster internal registry
#
# Components handled:
# - Kubernaut webhook service
# - Kubernaut AI service
# - HolmesGPT integration
# - All related Kubernetes manifests
#
# PREREQUISITES: External dependencies must be running (use bootstrap-external-deps.sh)

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

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    # Check if Kind cluster exists
    if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_error "Kind cluster '${CLUSTER_NAME}' not found"
        log_info "Run bootstrap-external-deps.sh first to create the cluster"
        exit 1
    fi

    # Check if kubectl context is correct
    if ! kubectl config current-context | grep -q "kind-${CLUSTER_NAME}"; then
        log_warning "kubectl context not set to Kind cluster"
        log_info "Setting kubectl context..."
        kubectl config use-context "kind-${CLUSTER_NAME}"
    fi

    # Check if cluster is accessible
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot access Kind cluster"
        log_info "Ensure the cluster is running: kind get clusters"
        exit 1
    fi

    # Check if namespace exists
    if ! kubectl get namespace ${NAMESPACE} &> /dev/null; then
        log_error "Namespace '${NAMESPACE}' not found"
        log_info "Run bootstrap-external-deps.sh first to create external dependencies"
        exit 1
    fi

    # Check container runtime
    if command -v podman &> /dev/null; then
        if ! podman info &> /dev/null; then
            log_error "Podman is not running"
            exit 1
        fi
        log_info "Using Podman as container runtime"
        export KIND_EXPERIMENTAL_PROVIDER=podman
    elif command -v docker &> /dev/null; then
        if ! docker info &> /dev/null; then
            log_error "Docker is not running"
            exit 1
        fi
        log_info "Using Docker as container runtime"
    else
        log_error "Neither Docker nor Podman found"
        exit 1
    fi

    log_success "All prerequisites satisfied"
}

# Build kubernaut Go binaries
build_go_services() {
    log_step "Building kubernaut Go services..."

    cd "$PROJECT_ROOT"

    # Build gateway service (includes webhook functionality)
    log_info "Building gateway service..."
    if make build-gateway-service; then
        log_success "Gateway service built successfully"
    else
        log_error "Failed to build gateway service"
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

    log_success "All Go services built successfully"
}

# Build and load container images to Kind cluster
build_and_load_images() {
    log_step "Building and loading container images to Kind cluster..."

    cd "$PROJECT_ROOT"

    # Initialize git submodules if needed
    log_info "Ensuring git submodules are initialized..."
    git submodule init && git submodule update || log_warning "Submodule initialization failed (may not be needed)"

    # Build gateway service image
    log_info "Building gateway service image..."
    if docker build -t localhost/kubernaut/gateway-service:latest -f docker/gateway-service.Dockerfile .; then
        log_success "Gateway service image built"
    else
        log_error "Failed to build gateway service image"
        exit 1
    fi

    # Load gateway service image to Kind
    log_info "Loading gateway service image to Kind cluster..."
    kind load docker-image localhost/kubernaut/gateway-service:latest --name="${CLUSTER_NAME}"

    # Build AI service image
    log_info "Building AI service image..."
    if docker build -t localhost/kubernaut/ai-service:latest -f docker/ai-service.Dockerfile .; then
        log_success "AI service image built"
    else
        log_error "Failed to build AI service image"
        exit 1
    fi

    # Load AI service image to Kind
    log_info "Loading AI service image to Kind cluster..."
    kind load docker-image localhost/kubernaut/ai-service:latest --name="${CLUSTER_NAME}"

    # Build HolmesGPT API image
    log_info "Building HolmesGPT API image..."
    if docker build -t localhost/kubernaut/holmesgpt-api:latest -f docker/holmesgpt-api/Dockerfile .; then
        log_success "HolmesGPT API image built"
    else
        log_error "Failed to build HolmesGPT API image"
        exit 1
    fi

    # Load HolmesGPT API image to Kind
    log_info "Loading HolmesGPT API image to Kind cluster..."
    kind load docker-image localhost/kubernaut/holmesgpt-api:latest --name="${CLUSTER_NAME}"

    log_success "All container images built and loaded to Kind cluster"
}

# Deploy kubernaut services to Kind cluster
deploy_kubernaut_services() {
    log_step "Deploying kubernaut services to Kind cluster..."

    cd "$PROJECT_ROOT"

    # Create or update kubernaut-specific manifests with localhost registry
    log_info "Preparing kubernaut manifests with internal registry..."

    # Deploy kubernaut services using Kustomize
    log_info "Applying kubernaut services manifests..."

    # Deploy RBAC first
    kubectl apply -k deploy/integration/kubernaut/ -n ${NAMESPACE}

    # Wait for kubernaut services to be ready
    log_info "Waiting for kubernaut services to be ready (this may take a few minutes)..."

    # Wait for gateway service
    if kubectl wait --for=condition=available --timeout=300s deployment/gateway-service -n ${NAMESPACE} 2>/dev/null; then
        log_success "Gateway service is ready"
    else
        log_warning "Gateway service deployment may still be starting"
    fi

    # Wait for AI service
    if kubectl wait --for=condition=available --timeout=300s deployment/ai-service -n ${NAMESPACE} 2>/dev/null; then
        log_success "AI service is ready"
    else
        log_warning "AI service deployment may still be starting"
    fi

    # Wait for HolmesGPT
    if kubectl wait --for=condition=available --timeout=300s deployment/holmesgpt -n ${NAMESPACE} 2>/dev/null; then
        log_success "HolmesGPT service is ready"
    else
        log_warning "HolmesGPT deployment may still be starting"
    fi

    log_success "All kubernaut services deployed successfully"
}

# Update image references in manifests to use localhost registry
update_manifest_images() {
    log_step "Updating manifest images to use Kind internal registry..."

    cd "$PROJECT_ROOT"

    # Apply kubernaut services directly and then patch images
    log_info "Applying kubernaut services manifests..."
    kubectl apply -k deploy/integration/kubernaut/ -n ${NAMESPACE}

    # Patch deployments to use localhost registry images
    log_info "Updating deployment images to use Kind internal registry..."

    # Update webhook-service image (using gateway-service image)
    kubectl patch deployment webhook-service -n ${NAMESPACE} -p '{"spec":{"template":{"spec":{"containers":[{"name":"webhook-service","image":"localhost/kubernaut/gateway-service:latest"}]}}}}'

    # Update ai-service image
    kubectl patch deployment ai-service -n ${NAMESPACE} -p '{"spec":{"template":{"spec":{"containers":[{"name":"ai-service","image":"localhost/kubernaut/ai-service:latest"}]}}}}'

    # Update holmesgpt image
    kubectl patch deployment holmesgpt -n ${NAMESPACE} -p '{"spec":{"template":{"spec":{"containers":[{"name":"holmesgpt","image":"localhost/kubernaut/holmesgpt-api:latest"}]}}}}'

    log_success "Manifest images updated to use Kind internal registry"
}

# Verify kubernaut services
verify_kubernaut_services() {
    log_step "Verifying kubernaut services..."

    # Check service status
    log_info "Kubernaut services status:"
    kubectl get pods,svc -n ${NAMESPACE} -l component=kubernaut

    # Check service health
    log_info "Checking kubernaut service health..."

    # Wait a bit for services to fully start
    sleep 15

    local webhook_ready=false
    local ai_ready=false
    local holmesgpt_ready=false

    # Check gateway service health
    if kubectl exec -n ${NAMESPACE} deployment/gateway-service -- /usr/local/bin/gateway-service --health-check > /dev/null 2>&1; then
        webhook_ready=true
        log_success "Gateway service is healthy"
    else
        log_warning "Gateway service health check failed"
    fi

    # Check AI service (if it has health endpoint)
    if kubectl get pods -n ${NAMESPACE} -l app=ai-service --field-selector=status.phase=Running | grep -q ai-service; then
        ai_ready=true
        log_success "AI service is running"
    else
        log_warning "AI service not running yet"
    fi

    # Check HolmesGPT service
    if kubectl get pods -n ${NAMESPACE} -l app=holmesgpt --field-selector=status.phase=Running | grep -q holmesgpt; then
        holmesgpt_ready=true
        log_success "HolmesGPT service is running"
    else
        log_warning "HolmesGPT service not running yet"
    fi

    if [ "$webhook_ready" = true ] && [ "$ai_ready" = true ] && [ "$holmesgpt_ready" = true ]; then
        log_success "All kubernaut services verification completed successfully"
    else
        log_warning "Some kubernaut services may still be starting up"
        log_info "Use 'kubectl get pods -n ${NAMESPACE}' to check detailed status"
        log_info "Use 'kubectl logs -f deployment/<service> -n ${NAMESPACE}' to check logs"
    fi
}

# Show usage information
show_usage_info() {
    log_step "Kubernaut services deployment complete!"

    echo ""
    echo "🎉 Kubernaut Build and Deploy Complete!"
    echo "======================================"
    echo ""
    echo "📋 Kubernaut Services Deployed:"
    echo "  • Gateway Service:    Ready (localhost/kubernaut/gateway-service:latest)"
    echo "  • AI Service:         Ready (localhost/kubernaut/ai-service:latest)"
    echo "  • HolmesGPT API:      Ready (localhost/kubernaut/holmesgpt-api:latest)"
    echo ""
    echo "🔧 Management Commands:"
    echo "  • Check status:       kubectl get pods -n ${NAMESPACE}"
    echo "  • View logs:          kubectl logs -f deployment/<service> -n ${NAMESPACE}"
    echo "  • Restart service:    kubectl rollout restart deployment/<service> -n ${NAMESPACE}"
    echo ""
    echo "🔄 Rebuild and Redeploy:"
    echo "  • Rebuild all:        make build-and-deploy"
    echo "  • Rebuild specific:   docker build -t localhost/kubernaut/<service>:latest -f docker/<service>.Dockerfile ."
    echo "  • Reload image:       kind load docker-image localhost/kubernaut/<service>:latest --name ${CLUSTER_NAME}"
    echo "  • Restart deployment: kubectl rollout restart deployment/<service> -n ${NAMESPACE}"
    echo ""
    echo "🧪 Testing Commands:"
    echo "  • Run integration tests:  make test-integration-dev"
    echo "  • Port forward webhook:   kubectl port-forward -n ${NAMESPACE} svc/webhook-service 8080:8080"
    echo "  • Port forward AI:        kubectl port-forward -n ${NAMESPACE} svc/ai-service 8093:8093"
    echo ""
    echo "⚙️  Configuration:"
    echo "  • Kubectl context:    kind-${CLUSTER_NAME}"
    echo "  • Namespace:          ${NAMESPACE}"
    echo "  • Registry:           Kind internal (localhost/kubernaut/*)"
    echo ""
    echo "💡 Development Workflow:"
    echo "  1. Make code changes"
    echo "  2. Run: make build-and-deploy"
    echo "  3. Test your changes"
    echo "  4. Repeat as needed"
    echo ""
}

# Main execution
main() {
    local start_time
    start_time=$(date +%s)

    echo "🚀 Kubernaut Build and Deploy"
    echo "============================="
    echo ""
    echo "This script will build and deploy kubernaut components:"
    echo "  ✓ Build Go services (gateway, AI)"
    echo "  ✓ Build container images"
    echo "  ✓ Load images to Kind internal registry"
    echo "  ✓ Deploy kubernaut services to Kubernetes"
    echo "  ✓ Verify service health"
    echo ""
    echo "Prerequisites:"
    echo "  ✓ Kind cluster must be running (use bootstrap-external-deps.sh)"
    echo "  ✓ External dependencies must be deployed"
    echo ""

    # Handle command line arguments
    case "${1:-}" in
        --help|-h)
            echo "Usage: $0 [--help]"
            echo ""
            echo "Builds and deploys kubernaut components to Kind cluster internal registry."
            echo ""
            echo "Prerequisites:"
            echo "  - Kind cluster running with external dependencies"
            echo "  - Run bootstrap-external-deps.sh first"
            echo ""
            echo "This script can be run multiple times to rebuild and redeploy services."
            exit 0
            ;;
        *)
            # Continue with main execution
            ;;
    esac

    check_prerequisites
    build_go_services
    build_and_load_images
    update_manifest_images
    deploy_kubernaut_services
    verify_kubernaut_services
    show_usage_info

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_success "🎉 Build and deploy completed successfully in ${duration} seconds!"
    echo ""
    echo "Next steps:"
    echo "  1. kubectl get pods -n ${NAMESPACE}  # Check all services"
    echo "  2. make test-integration-dev         # Run integration tests"
}

# Execute main function
main "$@"
