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

# Target architecture configuration
# Default to host architecture for integration tests (avoids cross-compilation)
# Override with TARGETARCH env var for production multi-arch builds
HOST_ARCH=$(uname -m)
case "$HOST_ARCH" in
    x86_64)
        HOST_ARCH="amd64"
        ;;
    aarch64|arm64)
        HOST_ARCH="arm64"
        ;;
    armv7l)
        HOST_ARCH="arm"
        ;;
    *)
        log_warn "Unknown architecture: $HOST_ARCH, defaulting to amd64"
        HOST_ARCH="amd64"
        ;;
esac

# Use TARGETARCH env var if set, otherwise use host architecture
TARGETARCH="${TARGETARCH:-$HOST_ARCH}"

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
            echo "  TARGETARCH          Target architecture (default: host architecture)"
            echo "                      - For integration tests: Uses host arch (arm64/amd64)"
            echo "                      - For production: Set to 'amd64' or 'arm64'"
            echo ""
            echo "Examples:"
            echo "  # Integration tests (uses host architecture)"
            echo "  ./scripts/build-notification-controller.sh --kind"
            echo ""
            echo "  # Production build for amd64"
            echo "  TARGETARCH=amd64 ./scripts/build-notification-controller.sh --push"
            echo ""
            echo "  # Production build for arm64"
            echo "  TARGETARCH=arm64 IMAGE_TAG=v1.0.0-arm64 ./scripts/build-notification-controller.sh --push"
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

# Detect container tool (docker or podman)
CONTAINER_TOOL=""
if command -v docker &> /dev/null; then
    CONTAINER_TOOL="docker"
elif command -v podman &> /dev/null; then
    CONTAINER_TOOL="podman"
else
    log_error "Neither Docker nor Podman is installed or in PATH"
    exit 1
fi

log_info "Using container tool: $CONTAINER_TOOL"

if [[ "$LOAD_TO_KIND" == true ]] && ! command -v kind &> /dev/null; then
    log_error "KIND is not installed but --kind flag was specified"
    exit 1
fi

# Check if Dockerfile exists
if [[ ! -f "$DOCKERFILE" ]]; then
    log_error "Dockerfile not found: $DOCKERFILE"
    exit 1
fi

# Build container image
log_info "Building container image: $FULL_IMAGE"
log_info "Dockerfile: $DOCKERFILE"
log_info "Target Architecture: $TARGETARCH"

$CONTAINER_TOOL build \
    -t "$FULL_IMAGE" \
    -f "$DOCKERFILE" \
    --build-arg TARGETARCH="$TARGETARCH" \
    .

if [[ $? -eq 0 ]]; then
    log_info "✅ Container image built successfully: $FULL_IMAGE"
else
    log_error "❌ Container image build failed"
    exit 1
fi

# Get image size
IMAGE_SIZE=$($CONTAINER_TOOL images "$FULL_IMAGE" --format "{{.Size}}")
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

    # Handle Podman differently from Docker
    if [[ "$CONTAINER_TOOL" == "podman" ]]; then
        log_info "Using Podman: saving image to tar and loading into KIND..."
        
        # Save Podman image to tar file
        TEMP_TAR="/tmp/${IMAGE_NAME}-${IMAGE_TAG}.tar"
        $CONTAINER_TOOL save -o "$TEMP_TAR" "$FULL_IMAGE"
        
        if [[ $? -ne 0 ]]; then
            log_error "❌ Failed to save Podman image to tar"
            exit 1
        fi
        
        # Load tar into KIND
        kind load image-archive "$TEMP_TAR" --name "$KIND_CLUSTER_NAME"
        LOAD_RESULT=$?
        
        # Cleanup tar file
        rm -f "$TEMP_TAR"
        
        if [[ $LOAD_RESULT -eq 0 ]]; then
            log_info "✅ Image loaded into KIND cluster: $KIND_CLUSTER_NAME"
        else
            log_error "❌ Failed to load image into KIND cluster"
            exit 1
        fi
    else
        # Docker can load directly
        kind load docker-image "$FULL_IMAGE" --name "$KIND_CLUSTER_NAME"
        
        if [[ $? -eq 0 ]]; then
            log_info "✅ Image loaded into KIND cluster: $KIND_CLUSTER_NAME"
        else
            log_error "❌ Failed to load image into KIND cluster"
            exit 1
        fi
    fi
fi

# Push to registry if requested
if [[ "$PUSH_TO_REGISTRY" == true ]]; then
    log_info "Pushing image to registry: $FULL_IMAGE"

    $CONTAINER_TOOL push "$FULL_IMAGE"

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
log_info "Architecture: $TARGETARCH"
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


