#!/bin/bash
# HolmesGPT REST API Multi-Architecture Container Build Script
# Builds for both linux/amd64 and linux/arm64 using Podman

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
IMAGE_NAME="${IMAGE_NAME:-quay.io/jordigilh/holmesgpt-api}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
DOCKERFILE_PATH="${PROJECT_ROOT}/docker/holmesgpt-api/Dockerfile"
BUILD_CONTEXT="${PROJECT_ROOT}"

# Build configuration
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"
PUSH_IMAGE="${PUSH_IMAGE:-false}"
NO_CACHE="${NO_CACHE:-false}"
SECURITY_SCAN="${SECURITY_SCAN:-true}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging functions
log() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')] $1${NC}"
}

log_warn() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')] WARNING: $1${NC}"
}

log_error() {
    echo -e "${RED}[$(date +'%H:%M:%S')] ERROR: $1${NC}"
}

log_info() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')] INFO: $1${NC}"
}

log_debug() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "${CYAN}[$(date +'%H:%M:%S')] DEBUG: $1${NC}"
    fi
}

# Function to show usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Build HolmesGPT REST API container image for multiple architectures

OPTIONS:
    -i, --image NAME        Container image name (default: quay.io/jordigilh/holmesgpt-api)
    -t, --tag TAG           Image tag (default: latest)
    -p, --platforms LIST    Target platforms (default: linux/amd64,linux/arm64)
    --push                  Push images to registry after build
    --no-cache              Build without using cache
    --no-security-scan      Skip security scanning
    -h, --help              Show this help message

EXAMPLES:
    $0                                  # Build with defaults
    $0 --tag v1.0.0 --push            # Build and push tagged version
    $0 --platforms linux/amd64        # Build for single architecture
    $0 --no-cache --no-security-scan  # Fast build without security checks

ENVIRONMENT VARIABLES:
    IMAGE_NAME              Container image name
    IMAGE_TAG               Image tag
    PLATFORMS               Target platforms
    PUSH_IMAGE              Push after build (true/false)
    NO_CACHE                Disable build cache (true/false)
    SECURITY_SCAN           Enable security scanning (true/false)
    DEBUG                   Enable debug output (true/false)
EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -i|--image)
                IMAGE_NAME="$2"
                shift 2
                ;;
            -t|--tag)
                IMAGE_TAG="$2"
                shift 2
                ;;
            -p|--platforms)
                PLATFORMS="$2"
                shift 2
                ;;
            --push)
                PUSH_IMAGE="true"
                shift
                ;;
            --no-cache)
                NO_CACHE="true"
                shift
                ;;
            --no-security-scan)
                SECURITY_SCAN="false"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

# Check prerequisites
check_prerequisites() {
    log "🔍 Checking prerequisites..."

    # Check podman
    if ! command -v podman &> /dev/null; then
        log_error "Podman is not installed or not in PATH"
        exit 1
    fi

    # Check podman version and buildx support
    local podman_version
    podman_version=$(podman --version | cut -d' ' -f3)
    log_info "Podman version: $podman_version"

    # Check if running on macOS and warn about cross-compilation
    if [[ "$OSTYPE" == "darwin"* ]] && [[ "$PLATFORMS" == *"linux/"* ]]; then
        log_warn "Cross-compilation on macOS may require QEMU emulation"
        log_warn "Consider using --platforms linux/$(uname -m) for native builds"
    fi

    # Check Dockerfile exists
    if [[ ! -f "$DOCKERFILE_PATH" ]]; then
        log_error "Dockerfile not found: $DOCKERFILE_PATH"
        exit 1
    fi

    # Check build context
    if [[ ! -d "$BUILD_CONTEXT" ]]; then
        log_error "Build context directory not found: $BUILD_CONTEXT"
        exit 1
    fi

    # Check HolmesGPT submodule
    local holmesgpt_source="${PROJECT_ROOT}/dependencies/holmesgpt"
    if [[ ! -d "$holmesgpt_source" ]] || [[ ! -f "$holmesgpt_source/README.md" ]]; then
        log_error "HolmesGPT source not found. Initialize submodule:"
        log_error "  git submodule update --init --recursive"
        exit 1
    fi

    log "✅ Prerequisites check passed"
}

# Initialize HolmesGPT submodule
init_holmesgpt_source() {
    log "📥 Initializing HolmesGPT source..."

    cd "$PROJECT_ROOT"

    # Initialize submodule if not already done
    if [[ ! -f "dependencies/holmesgpt/README.md" ]]; then
        log_info "Initializing HolmesGPT submodule..."
        git submodule update --init --recursive dependencies/holmesgpt
    fi

    # Get latest stable version
    cd dependencies/holmesgpt
    local current_branch
    current_branch=$(git branch --show-current)

    if [[ "$current_branch" != "master" ]]; then
        log_info "Checking out latest master branch..."
        git checkout master
        git pull origin master
    fi

    # Get version info
    local holmesgpt_version
    holmesgpt_version=$(git describe --tags --abbrev=0 2>/dev/null || echo "development")
    log_info "HolmesGPT version: $holmesgpt_version"

    cd "$PROJECT_ROOT"
    log "✅ HolmesGPT source initialized"
}

# Run security scan
run_security_scan() {
    if [[ "$SECURITY_SCAN" != "true" ]]; then
        log_info "Skipping security scan (disabled)"
        return 0
    fi

    log "🛡️ Running security scan..."

    # Check if security scanning tools are available
    local scan_tools=("trivy" "grype")
    local available_scanners=()

    for tool in "${scan_tools[@]}"; do
        if command -v "$tool" &> /dev/null; then
            available_scanners+=("$tool")
        fi
    done

    if [[ ${#available_scanners[@]} -eq 0 ]]; then
        log_warn "No security scanners found. Install trivy or grype for security scanning"
        return 0
    fi

    # Run available scanners on the built image
    local full_image_name="${IMAGE_NAME}:${IMAGE_TAG}"

    for scanner in "${available_scanners[@]}"; do
        log_info "Running $scanner security scan..."
        case "$scanner" in
            "trivy")
                trivy image --exit-code 1 --severity HIGH,CRITICAL "$full_image_name" || {
                    log_error "Trivy security scan failed with critical/high vulnerabilities"
                    return 1
                }
                ;;
            "grype")
                grype "$full_image_name" --fail-on high || {
                    log_error "Grype security scan failed with high vulnerabilities"
                    return 1
                }
                ;;
        esac
    done

    log "✅ Security scan passed"
}

# Build container image
build_image() {
    log "🏗️ Building container image..."

    local full_image_name="${IMAGE_NAME}:${IMAGE_TAG}"
    local build_args=()

    # Build arguments
    build_args+=("--file" "$DOCKERFILE_PATH")
    build_args+=("--tag" "$full_image_name")

    # Multi-platform support
    IFS=',' read -ra PLATFORM_ARRAY <<< "$PLATFORMS"
    if [[ ${#PLATFORM_ARRAY[@]} -gt 1 ]]; then
        log_info "Multi-platform build for: $PLATFORMS"
        build_args+=("--platform" "$PLATFORMS")
        build_args+=("--manifest" "$full_image_name")
    else
        log_info "Single platform build for: $PLATFORMS"
        build_args+=("--platform" "$PLATFORMS")
    fi

    # Cache options
    if [[ "$NO_CACHE" == "true" ]]; then
        build_args+=("--no-cache")
        log_info "Cache disabled"
    fi

    # Add build metadata
    build_args+=("--label" "org.opencontainers.image.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)")
    build_args+=("--label" "org.opencontainers.image.version=${IMAGE_TAG}")
    build_args+=("--label" "org.opencontainers.image.source=https://github.com/jordigilh/kubernaut")
    build_args+=("--label" "org.opencontainers.image.revision=$(git rev-parse HEAD)")

    # Build the image
    log_info "Running: podman build ${build_args[*]} $BUILD_CONTEXT"

    if ! podman build "${build_args[@]}" "$BUILD_CONTEXT"; then
        log_error "Container build failed"
        return 1
    fi

    log "✅ Container build completed"
}

# Push image to registry
push_image() {
    if [[ "$PUSH_IMAGE" != "true" ]]; then
        log_info "Skipping image push (disabled)"
        return 0
    fi

    log "📤 Pushing image to registry..."

    local full_image_name="${IMAGE_NAME}:${IMAGE_TAG}"

    # Check if logged in to registry
    local registry_host
    registry_host=$(echo "$IMAGE_NAME" | cut -d'/' -f1)

    if ! podman login --get-login "$registry_host" &>/dev/null; then
        log_warn "Not logged in to registry $registry_host"
        log_info "Run: podman login $registry_host"
        return 1
    fi

    # Push the image
    if ! podman push "$full_image_name"; then
        log_error "Failed to push image"
        return 1
    fi

    log "✅ Image pushed successfully"
    log_info "Image available at: $full_image_name"
}

# Generate build summary
generate_summary() {
    log "📊 Build Summary"
    echo "----------------------------------------"
    echo "Image Name:      $IMAGE_NAME"
    echo "Image Tag:       $IMAGE_TAG"
    echo "Platforms:       $PLATFORMS"
    echo "Build Context:   $BUILD_CONTEXT"
    echo "Dockerfile:      $DOCKERFILE_PATH"
    echo "No Cache:        $NO_CACHE"
    echo "Security Scan:   $SECURITY_SCAN"
    echo "Push Image:      $PUSH_IMAGE"
    echo "----------------------------------------"

    # Image information
    local full_image_name="${IMAGE_NAME}:${IMAGE_TAG}"
    if podman image exists "$full_image_name"; then
        echo "Built Image Info:"
        podman image inspect "$full_image_name" --format "Size: {{.Size}}" 2>/dev/null || true
        podman image inspect "$full_image_name" --format "Created: {{.Created}}" 2>/dev/null || true
    fi
}

# Cleanup function
cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Build failed with exit code $exit_code"

        # Clean up partial builds on failure
        local full_image_name="${IMAGE_NAME}:${IMAGE_TAG}"
        if podman image exists "$full_image_name"; then
            log_info "Cleaning up partial build..."
            podman rmi "$full_image_name" 2>/dev/null || true
        fi
    fi
    exit $exit_code
}

# Main function
main() {
    # Set up error handling
    trap cleanup EXIT ERR

    echo -e "${CYAN}"
    cat << 'EOF'
╔══════════════════════════════════════════════════════════════╗
║        HolmesGPT REST API Multi-Architecture Builder        ║
║              Source-based Build with Podman                 ║
╚══════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"

    # Parse arguments and run build
    parse_args "$@"
    check_prerequisites
    init_holmesgpt_source
    build_image
    run_security_scan
    push_image
    generate_summary

    log "🎉 Build completed successfully!"
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
