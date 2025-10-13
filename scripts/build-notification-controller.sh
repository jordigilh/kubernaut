#!/usr/bin/env bash
# Build script for Notification Controller
# Builds Docker image and optionally loads into KIND cluster

set -euo pipefail

# Configuration
IMAGE_NAME="kubernaut-notification"
IMAGE_TAG="${IMAGE_TAG:-latest}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-notification-test}"
DOCKERFILE="docker/notification-controller.Dockerfile"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse command line arguments
LOAD_TO_KIND=false
PUSH_TO_REGISTRY=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --kind)
            LOAD_TO_KIND=true
            shift
            ;;
        --push)
            PUSH_TO_REGISTRY=true
            shift
            ;;
        --tag)
            IMAGE_TAG="$2"
            FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --kind          Load image into KIND cluster"
            echo "  --push          Push image to registry"
            echo "  --tag TAG       Set image tag (default: latest)"
            echo "  --help          Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  IMAGE_TAG           Image tag (default: latest)"
            echo "  KIND_CLUSTER_NAME   KIND cluster name (default: notification-test)"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate prerequisites
log_info "Validating prerequisites..."

if ! command -v docker &> /dev/null; then
    log_error "Docker is not installed or not in PATH"
    exit 1
fi

if [[ "$LOAD_TO_KIND" == true ]] && ! command -v kind &> /dev/null; then
    log_error "KIND is not installed but --kind flag was specified"
    exit 1
fi

# Check if Dockerfile exists
if [[ ! -f "$DOCKERFILE" ]]; then
    log_error "Dockerfile not found: $DOCKERFILE"
    exit 1
fi

# Build Docker image
log_info "Building Docker image: $FULL_IMAGE"
log_info "Dockerfile: $DOCKERFILE"

docker build \
    -t "$FULL_IMAGE" \
    -f "$DOCKERFILE" \
    .

if [[ $? -eq 0 ]]; then
    log_info "✅ Docker image built successfully: $FULL_IMAGE"
else
    log_error "❌ Docker image build failed"
    exit 1
fi

# Get image size
IMAGE_SIZE=$(docker images "$FULL_IMAGE" --format "{{.Size}}")
log_info "Image size: $IMAGE_SIZE"

# Load into KIND cluster if requested
if [[ "$LOAD_TO_KIND" == true ]]; then
    log_info "Loading image into KIND cluster: $KIND_CLUSTER_NAME"
    
    # Check if KIND cluster exists
    if ! kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
        log_warn "KIND cluster '$KIND_CLUSTER_NAME' does not exist"
        log_info "Create it with: kind create cluster --name $KIND_CLUSTER_NAME"
        exit 1
    fi
    
    kind load docker-image "$FULL_IMAGE" --name "$KIND_CLUSTER_NAME"
    
    if [[ $? -eq 0 ]]; then
        log_info "✅ Image loaded into KIND cluster: $KIND_CLUSTER_NAME"
    else
        log_error "❌ Failed to load image into KIND cluster"
        exit 1
    fi
fi

# Push to registry if requested
if [[ "$PUSH_TO_REGISTRY" == true ]]; then
    log_info "Pushing image to registry: $FULL_IMAGE"
    
    docker push "$FULL_IMAGE"
    
    if [[ $? -eq 0 ]]; then
        log_info "✅ Image pushed to registry: $FULL_IMAGE"
    else
        log_error "❌ Failed to push image to registry"
        exit 1
    fi
fi

# Summary
log_info ""
log_info "========================================="
log_info "Build Summary"
log_info "========================================="
log_info "Image:        $FULL_IMAGE"
log_info "Size:         $IMAGE_SIZE"
log_info "KIND Loaded:  $LOAD_TO_KIND"
log_info "Pushed:       $PUSH_TO_REGISTRY"
log_info "========================================="
log_info ""
log_info "Next steps:"
log_info "  1. Deploy controller: kubectl apply -k deploy/notification/"
log_info "  2. Verify deployment: kubectl get pods -n kubernaut-notifications"
log_info "  3. Check logs: kubectl logs -f deployment/notification-controller -n kubernaut-notifications"

exit 0

