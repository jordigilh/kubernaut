#!/usr/bin/env bash
# ════════════════════════════════════════════════════════════════════════════════
# Generic Service Image Build Script
# Per DD-TEST-001: Unique Container Image Tags for Multi-Team Testing
# ════════════════════════════════════════════════════════════════════════════════
#
# This script builds container images for ANY service with unique tags.
# Replaces service-specific build scripts with a single reusable utility.
#
# Compatibility: bash 3.2+ (macOS default bash compatible)
#
# Usage:
#   ./scripts/build-service-image.sh SERVICE_NAME [OPTIONS]
#
# Examples:
#   ./scripts/build-service-image.sh notification
#   ./scripts/build-service-image.sh signalprocessing --kind
#   ./scripts/build-service-image.sh datastorage --tag custom-123
#
# ════════════════════════════════════════════════════════════════════════════════

set -euo pipefail

# ────────────────────────────────────────────────────────────────────────────────
# Configuration
# ────────────────────────────────────────────────────────────────────────────────

# Check for --help first (before requiring service name)
if [[ "${1:-}" == "--help" ]] || [[ "${1:-}" == "-h" ]]; then
    cat <<EOF
════════════════════════════════════════════════════════════════════════════════
Generic Service Image Build Script (DD-TEST-001)
════════════════════════════════════════════════════════════════════════════════

Usage: $0 SERVICE_NAME [OPTIONS]

Services:
  gateway                 Gateway Service
  notification            Notification Controller
  signalprocessing        SignalProcessing Controller
  remediationorchestrator RemediationOrchestrator Controller
  workflowexecution       WorkflowExecution Controller
  aianalysis              AIAnalysis Controller
  datastorage             DataStorage Service
  hapi                    HolmesGPT API Service

Options:
  --kind                  Load image into Kind cluster (forces single-arch)
  --push                  Push image to registry
  --tag TAG               Set custom image tag (default: auto-generated)
  --cleanup               Clean up image after operation
  --multi-arch            Build multi-architecture image (amd64 + arm64)
  --single-arch           Build single-architecture image (default)
  --cluster NAME          Kind cluster name (default: {service}-test)
  --help, -h              Show this help message

Environment Variables:
  IMAGE_TAG               Override auto-generated tag
  KIND_CLUSTER_NAME       Kind cluster name
  MULTI_ARCH              Enable multi-arch builds (true/false)
  PLATFORMS               Target platforms (default: linux/amd64,linux/arm64)
  TARGETARCH              Target architecture for single-arch (default: host arch)

Tag Format (DD-TEST-001):
  {service}-{user}-{git-hash}-{timestamp}
  Example: notification-jordi-abc123f-1734278400

Examples:
  # Build notification with auto-generated unique tag
  $0 notification

  # Build and load into Kind cluster for testing
  $0 signalprocessing --kind

  # Build with custom tag and push to registry
  $0 datastorage --tag v1.0.0 --push

  # Build for integration tests with automatic cleanup
  $0 aianalysis --kind --cleanup

  # Multi-arch build for production
  $0 workflowexecution --multi-arch --push

════════════════════════════════════════════════════════════════════════════════
EOF
    exit 0
fi

# Service name (REQUIRED - first argument)
if [[ $# -lt 1 ]]; then
    echo "❌ Error: Service name required"
    echo "Usage: $0 SERVICE_NAME [OPTIONS]"
    echo ""
    echo "Available services:"
    echo "  gateway, notification, signalprocessing, remediationorchestrator,"
    echo "  workflowexecution, aianalysis, datastorage, hapi"
    echo ""
    echo "Run '$0 --help' for more information"
    exit 1
fi

SERVICE_NAME="$1"
shift  # Remove service name from arguments

# Service-to-Dockerfile mapping (DD-TEST-001 standard paths)
# Uses case statement for bash 3.2+ compatibility (macOS default bash)
get_dockerfile_path() {
    case "$1" in
        gateway)
            echo "docker/gateway-ubi9.Dockerfile"
            ;;
        notification)
            echo "docker/notification-controller.Dockerfile"
            ;;
        signalprocessing)
            echo "docker/signalprocessing-controller.Dockerfile"
            ;;
        remediationorchestrator)
            echo "docker/remediationorchestrator-controller.Dockerfile"
            ;;
        workflowexecution)
            echo "docker/workflowexecution-controller.Dockerfile"
            ;;
        aianalysis)
            echo "docker/aianalysis-controller.Dockerfile"
            ;;
        datastorage)
            echo "docker/data-storage.Dockerfile"
            ;;
        hapi)
            echo "holmesgpt-api/Dockerfile"
            ;;
        *)
            return 1
            ;;
    esac
}

# Get Dockerfile path for service
DOCKERFILE=$(get_dockerfile_path "$SERVICE_NAME")

# Validate service name
if [[ -z "$DOCKERFILE" ]]; then
    echo "❌ Error: Unknown service: $SERVICE_NAME"
    echo "Available services: gateway, notification, signalprocessing, remediationorchestrator,"
    echo "                    workflowexecution, aianalysis, datastorage, hapi"
    exit 1
fi

# Tag generation (DD-TEST-001 format: {service}-{user}-{git-hash}-{timestamp})
USER_TAG=$(whoami)
GIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
TIMESTAMP=$(date +%s)
DEFAULT_IMAGE_TAG="${SERVICE_NAME}-${USER_TAG}-${GIT_HASH}-${TIMESTAMP}"

# Allow IMAGE_TAG override
IMAGE_TAG="${IMAGE_TAG:-$DEFAULT_IMAGE_TAG}"
FULL_IMAGE="${SERVICE_NAME}:${IMAGE_TAG}"

# Kind cluster configuration
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-${SERVICE_NAME}-test}"

# Multi-architecture build configuration (ADR-027)
MULTI_ARCH="${MULTI_ARCH:-false}"  # Default to single-arch for tests
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"

# Host architecture detection
HOST_ARCH=$(uname -m)
case "$HOST_ARCH" in
    x86_64) HOST_ARCH="amd64" ;;
    aarch64|arm64) HOST_ARCH="arm64" ;;
    armv7l) HOST_ARCH="arm" ;;
    *) HOST_ARCH="amd64" ;;
esac
TARGETARCH="${TARGETARCH:-$HOST_ARCH}"

# ────────────────────────────────────────────────────────────────────────────────
# Colors for Output
# ────────────────────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_debug() { echo -e "${BLUE}[DEBUG]${NC} $1"; }

# ────────────────────────────────────────────────────────────────────────────────
# Command Line Arguments
# ────────────────────────────────────────────────────────────────────────────────

LOAD_TO_KIND=false
PUSH_TO_REGISTRY=false
CLEANUP_AFTER=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --kind)
            LOAD_TO_KIND=true
            MULTI_ARCH=false  # Kind requires single-arch
            shift
            ;;
        --push)
            PUSH_TO_REGISTRY=true
            shift
            ;;
        --tag)
            IMAGE_TAG="$2"
            FULL_IMAGE="${SERVICE_NAME}:${IMAGE_TAG}"
            shift 2
            ;;
        --cleanup)
            CLEANUP_AFTER=true
            shift
            ;;
        --multi-arch)
            MULTI_ARCH=true
            shift
            ;;
        --single-arch)
            MULTI_ARCH=false
            shift
            ;;
        --cluster)
            KIND_CLUSTER_NAME="$2"
            shift 2
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Run '$0 --help' for usage information"
            exit 1
            ;;
    esac
done

# ────────────────────────────────────────────────────────────────────────────────
# Prerequisites Validation
# ────────────────────────────────────────────────────────────────────────────────

log_info "Validating prerequisites..."

# Detect container tool
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

# Validate Kind if needed
if [[ "$LOAD_TO_KIND" == true ]] && ! command -v kind &> /dev/null; then
    log_error "Kind is not installed but --kind flag was specified"
    exit 1
fi

# Validate Dockerfile exists
if [[ ! -f "$DOCKERFILE" ]]; then
    log_error "Dockerfile not found: $DOCKERFILE"
    exit 1
fi

# ────────────────────────────────────────────────────────────────────────────────
# Build Container Image
# ────────────────────────────────────────────────────────────────────────────────

log_info ""
log_info "════════════════════════════════════════════════════════════════════════"
log_info "🔨 Building ${SERVICE_NAME} Image (DD-TEST-001)"
log_info "════════════════════════════════════════════════════════════════════════"
log_info "Service:      $SERVICE_NAME"
log_info "Image:        $FULL_IMAGE"
log_info "Dockerfile:   $DOCKERFILE"

if [[ "$MULTI_ARCH" == "true" ]]; then
    log_info "Build Type:   Multi-architecture"
    log_info "Platforms:    $PLATFORMS"
    log_info "────────────────────────────────────────────────────────────────────────"

    $CONTAINER_TOOL build \
        --platform "$PLATFORMS" \
        -t "$FULL_IMAGE" \
        -f "$DOCKERFILE" \
        .

    BUILD_RESULT=$?
    IMAGE_SIZE="multi-arch"
else
    log_info "Build Type:   Single-architecture"
    log_info "Architecture: $TARGETARCH"
    log_info "────────────────────────────────────────────────────────────────────────"

    $CONTAINER_TOOL build \
        --platform "linux/$TARGETARCH" \
        -t "$FULL_IMAGE" \
        -f "$DOCKERFILE" \
        .

    BUILD_RESULT=$?
    IMAGE_SIZE=$($CONTAINER_TOOL images "$FULL_IMAGE" --format "{{.Size}}" 2>/dev/null || echo "unknown")
fi

if [[ $BUILD_RESULT -ne 0 ]]; then
    log_error "❌ Image build failed"
    exit 1
fi

log_info "✅ Image built successfully: $FULL_IMAGE"
log_info "Size: $IMAGE_SIZE"

# Save image tag for later use (e.g., by tests)
echo "IMAGE_TAG=$IMAGE_TAG" > ".last-image-tag-${SERVICE_NAME}.env"
log_debug "Saved IMAGE_TAG to .last-image-tag-${SERVICE_NAME}.env"

# ────────────────────────────────────────────────────────────────────────────────
# Load into Kind Cluster
# ────────────────────────────────────────────────────────────────────────────────

if [[ "$LOAD_TO_KIND" == true ]]; then
    log_info ""
    log_info "📦 Loading image into Kind cluster: $KIND_CLUSTER_NAME"

    # Set KIND_EXPERIMENTAL_PROVIDER for Podman support (if not already set)
    # This is critical for Kind to use Podman instead of Docker
    export KIND_EXPERIMENTAL_PROVIDER=${KIND_EXPERIMENTAL_PROVIDER:-podman}

    # Check if Kind cluster exists (with retry for race conditions)
    # Race condition: In parallel tests, cluster may be created by another goroutine
    # and not immediately visible to `kind get clusters`
    log_debug "Checking if Kind cluster exists (with 30s retry window)..."
    CLUSTER_FOUND=false
    for i in {1..6}; do
        if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
            CLUSTER_FOUND=true
            log_debug "✅ Cluster found: $KIND_CLUSTER_NAME (attempt $i)"
            break
        fi
        if [[ $i -lt 6 ]]; then
            log_debug "⏳ Cluster not found yet, waiting 5s (attempt $i/6)..."
            sleep 5
        fi
    done

    if [[ "$CLUSTER_FOUND" == false ]]; then
        log_error "Kind cluster '$KIND_CLUSTER_NAME' does not exist after 30s"
        log_info "Available clusters:"
        kind get clusters 2>/dev/null || log_info "  (none)"
        log_info ""
        log_info "Create it with: kind create cluster --name $KIND_CLUSTER_NAME"
        exit 1
    fi

    # Handle Podman differently from Docker
    if [[ "$CONTAINER_TOOL" == "podman" ]]; then
        log_info "Using Podman: saving image to tar and loading into Kind..."

        TEMP_TAR="/tmp/${SERVICE_NAME}-${IMAGE_TAG}.tar"
        $CONTAINER_TOOL save -o "$TEMP_TAR" "$FULL_IMAGE"

        if [[ $? -ne 0 ]]; then
            log_error "❌ Failed to save Podman image to tar"
            exit 1
        fi

        # Load image into Kind (KIND_EXPERIMENTAL_PROVIDER already set above)
        kind load image-archive "$TEMP_TAR" --name "$KIND_CLUSTER_NAME"
        LOAD_RESULT=$?

        rm -f "$TEMP_TAR"
        log_debug "Cleaned up temporary tar file"
        
        # CRITICAL: Remove Podman image immediately after Kind load to free disk space
        # Problem: Image exists in both Podman storage AND Kind = 2x disk usage
        # Solution: Once in Kind, we don't need the Podman copy anymore
        if [[ $LOAD_RESULT -eq 0 ]]; then
            log_info "🗑️  Removing Podman image to free disk space..."
            $CONTAINER_TOOL rmi -f "$FULL_IMAGE" 2>/dev/null
            if [[ $? -eq 0 ]]; then
                log_info "✅ Podman image removed: $FULL_IMAGE"
            else
                log_warn "⚠️  Failed to remove Podman image (non-fatal)"
            fi
        fi
    else
        # Docker can load directly
        kind load docker-image "$FULL_IMAGE" --name "$KIND_CLUSTER_NAME"
        LOAD_RESULT=$?
    fi

    if [[ $LOAD_RESULT -eq 0 ]]; then
        log_info "✅ Image loaded into Kind cluster: $KIND_CLUSTER_NAME"
    else
        log_error "❌ Failed to load image into Kind cluster"
        exit 1
    fi
fi

# ────────────────────────────────────────────────────────────────────────────────
# Push to Registry
# ────────────────────────────────────────────────────────────────────────────────

if [[ "$PUSH_TO_REGISTRY" == true ]]; then
    log_info ""
    log_info "📤 Pushing image to registry: $FULL_IMAGE"

    if [[ "$MULTI_ARCH" == "true" ]]; then
        log_info "Pushing multi-arch manifest list..."
        $CONTAINER_TOOL manifest push "$FULL_IMAGE" docker://"$FULL_IMAGE" || \
        $CONTAINER_TOOL push "$FULL_IMAGE"
    else
        $CONTAINER_TOOL push "$FULL_IMAGE"
    fi

    if [[ $? -eq 0 ]]; then
        log_info "✅ Image pushed to registry: $FULL_IMAGE"
    else
        log_error "❌ Failed to push image to registry"
        exit 1
    fi
fi

# ────────────────────────────────────────────────────────────────────────────────
# Cleanup
# ────────────────────────────────────────────────────────────────────────────────

if [[ "$CLEANUP_AFTER" == true ]]; then
    log_info ""
    log_info "🧹 Cleaning up image: $FULL_IMAGE"
    $CONTAINER_TOOL rmi "$FULL_IMAGE" 2>/dev/null || true
    rm -f ".last-image-tag-${SERVICE_NAME}.env"
    log_info "✅ Cleanup complete"
fi

# ────────────────────────────────────────────────────────────────────────────────
# Summary
# ────────────────────────────────────────────────────────────────────────────────

log_info ""
log_info "════════════════════════════════════════════════════════════════════════"
log_info "Build Summary (DD-TEST-001)"
log_info "════════════════════════════════════════════════════════════════════════"
log_info "Service:      $SERVICE_NAME"
log_info "Image:        $FULL_IMAGE"
log_info "Tag Format:   {service}-{user}-{git-hash}-{timestamp}"
if [[ "$MULTI_ARCH" == "true" ]]; then
    log_info "Multi-Arch:   true"
    log_info "Platforms:    $PLATFORMS"
else
    log_info "Multi-Arch:   false (single-arch)"
    log_info "Architecture: $TARGETARCH"
fi
log_info "Size:         $IMAGE_SIZE"
log_info "Kind Loaded:  $LOAD_TO_KIND"
log_info "Pushed:       $PUSH_TO_REGISTRY"
log_info "Cleaned Up:   $CLEANUP_AFTER"
log_info "════════════════════════════════════════════════════════════════════════"
log_info ""
log_info "✅ Build complete!"
log_info ""
log_info "Image tag saved to: .last-image-tag-${SERVICE_NAME}.env"
log_info "Use in tests: export IMAGE_TAG=$IMAGE_TAG"

exit 0

