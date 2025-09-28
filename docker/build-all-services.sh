#!/bin/bash
# Build all Kubernaut microservices container images
# Usage: ./docker/build-all-services.sh [version] [registry]

set -euo pipefail

# Configuration
VERSION=${1:-"v1.0.0"}
REGISTRY=${2:-"quay.io/jordigilh"}
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Service definitions based on approved architecture (compatible with bash 3.2+)
# Only include services with actual implementations
SERVICES=(
    "gateway-service"      # ✅ Has implementation (renamed from webhook-service)
    "ai-service"          # ✅ Has implementation
    "processor-service"   # ✅ Has implementation
)

# Services with Dockerfiles but no implementations yet (for future development)
PLANNED_SERVICES=(
    "alert-service"
    "workflow-service"
    "executor-service"
    "storage-service"
    "intelligence-service"
    "monitor-service"
    "context-service"
    "notification-service"
)

# Get service port based on approved architecture
get_service_port() {
    local service_name=$1
    case "$service_name" in
        "gateway-service") echo "8080" ;;
        "ai-service") echo "8082" ;;
        "processor-service") echo "8095" ;;
        # Planned services (for future implementation)
        "alert-service") echo "8081" ;;
        "workflow-service") echo "8083" ;;
        "executor-service") echo "8084" ;;
        "storage-service") echo "8085" ;;
        "intelligence-service") echo "8086" ;;
        "monitor-service") echo "8087" ;;
        "context-service") echo "8088" ;;
        "notification-service") echo "8089" ;;
        *) echo "unknown" ;;
    esac
}

# Validate Dockerfile function
validate_dockerfile() {
    local service_name=$1
    local dockerfile="docker/${service_name}.Dockerfile"
    local expected_port=$(get_service_port "$service_name")
    local issues=()

    log_info "Validating ${service_name} Dockerfile..."

    # Check if Dockerfile exists
    if [[ ! -f "$dockerfile" ]]; then
        log_error "Dockerfile not found: $dockerfile"
        return 1
    fi

    # Check if expected port is exposed
    if ! grep -q "EXPOSE.*${expected_port}" "$dockerfile"; then
        issues+=("Port ${expected_port} not exposed")
    fi

    # Check if binary path matches service name
    if ! grep -q "./cmd/${service_name}" "$dockerfile"; then
        issues+=("Binary path doesn't match ./cmd/${service_name}")
    fi

    # Check if health check is defined
    if ! grep -q "HEALTHCHECK" "$dockerfile"; then
        issues+=("No health check defined")
    fi

    # Report issues
    if [[ ${#issues[@]} -gt 0 ]]; then
        log_warning "Issues found in ${service_name}:"
        for issue in "${issues[@]}"; do
            echo "  ⚠️  $issue"
        done
        return 1
    else
        log_success "Dockerfile validation passed for ${service_name}"
        return 0
    fi
}

# Build function for individual service
build_service() {
    local service_name=$1
    local dockerfile="docker/${service_name}.Dockerfile"
    local image_name="${REGISTRY}/${service_name}:${VERSION}"
    local latest_tag="${REGISTRY}/${service_name}:latest"

    log_info "Building ${service_name}..."

    # Check if Dockerfile exists
    if [[ ! -f "$dockerfile" ]]; then
        log_error "Dockerfile not found: $dockerfile"
        return 1
    fi

    # Build the image
    if docker build \
        --file "$dockerfile" \
        --tag "$image_name" \
        --tag "$latest_tag" \
        --build-arg VERSION="$VERSION" \
        --build-arg BUILD_DATE="$BUILD_DATE" \
        --build-arg GIT_COMMIT="$GIT_COMMIT" \
        --label "org.opencontainers.image.version=$VERSION" \
        --label "org.opencontainers.image.created=$BUILD_DATE" \
        --label "org.opencontainers.image.revision=$GIT_COMMIT" \
        --label "org.opencontainers.image.source=https://github.com/jordigilh/kubernaut" \
        .; then
        log_success "Successfully built ${service_name} -> ${image_name}"
        return 0
    else
        log_error "Failed to build ${service_name}"
        return 1
    fi
}

# Push function for individual service
push_service() {
    local service_name=$1
    local image_name="${REGISTRY}/${service_name}:${VERSION}"
    local latest_tag="${REGISTRY}/${service_name}:latest"

    log_info "Pushing ${service_name}..."

    if docker push "$image_name" && docker push "$latest_tag"; then
        log_success "Successfully pushed ${service_name}"
        return 0
    else
        log_error "Failed to push ${service_name}"
        return 1
    fi
}

# Validate prerequisites
validate_prerequisites() {
    log_info "Validating prerequisites..."

    # Check Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi

    # Check Docker daemon
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi

    # Check registry login
    if ! docker info | grep -q "Registry:"; then
        log_warning "Not logged into container registry. Run: docker login ${REGISTRY%%/*}"
    fi

    # Check if we're in the right directory
    if [[ ! -f "go.mod" ]] || [[ ! -d "cmd" ]]; then
        log_error "Please run this script from the kubernaut project root directory"
        exit 1
    fi

    log_success "Prerequisites validated"
}

# Build all services
build_all_services() {
    local failed_builds=()
    local successful_builds=()

    log_info "Building all Kubernaut microservices..."
    log_info "Version: ${VERSION}"
    log_info "Registry: ${REGISTRY}"
    log_info "Build Date: ${BUILD_DATE}"
    log_info "Git Commit: ${GIT_COMMIT}"
    echo

    for service in "${SERVICES[@]}"; do
        if build_service "$service"; then
            successful_builds+=("$service")
        else
            failed_builds+=("$service")
        fi
        echo
    done

    # Report results
    echo "=========================================="
    log_info "Build Summary:"
    echo "=========================================="

    if [[ ${#successful_builds[@]} -gt 0 ]]; then
        log_success "Successfully built ${#successful_builds[@]} services:"
        for service in "${successful_builds[@]}"; do
            echo "  ✅ $service"
        done
        echo
    fi

    if [[ ${#failed_builds[@]} -gt 0 ]]; then
        log_error "Failed to build ${#failed_builds[@]} services:"
        for service in "${failed_builds[@]}"; do
            echo "  ❌ $service"
        done
        echo
        return 1
    fi

    log_success "All services built successfully!"
    return 0
}

# Push all services
push_all_services() {
    local failed_pushes=()
    local successful_pushes=()

    log_info "Pushing all Kubernaut microservices to registry..."
    echo

    for service in "${SERVICES[@]}"; do
        if push_service "$service"; then
            successful_pushes+=("$service")
        else
            failed_pushes+=("$service")
        fi
        echo
    done

    # Report results
    echo "=========================================="
    log_info "Push Summary:"
    echo "=========================================="

    if [[ ${#successful_pushes[@]} -gt 0 ]]; then
        log_success "Successfully pushed ${#successful_pushes[@]} services:"
        for service in "${successful_pushes[@]}"; do
            echo "  ✅ $service -> ${REGISTRY}/${service}:${VERSION}"
        done
        echo
    fi

    if [[ ${#failed_pushes[@]} -gt 0 ]]; then
        log_error "Failed to push ${#failed_pushes[@]} services:"
        for service in "${failed_pushes[@]}"; do
            echo "  ❌ $service"
        done
        echo
        return 1
    fi

    log_success "All services pushed successfully!"
    return 0
}

# Validate all Dockerfiles
validate_all_dockerfiles() {
    local failed_validations=()
    local successful_validations=()

    log_info "Validating all Kubernaut Dockerfiles..."
    echo

    for service in "${SERVICES[@]}"; do
        if validate_dockerfile "$service"; then
            successful_validations+=("$service")
        else
            failed_validations+=("$service")
        fi
        echo
    done

    # Report results
    echo "=========================================="
    log_info "Validation Summary:"
    echo "=========================================="

    if [[ ${#successful_validations[@]} -gt 0 ]]; then
        log_success "Successfully validated ${#successful_validations[@]} Dockerfiles:"
        for service in "${successful_validations[@]}"; do
            echo "  ✅ $service"
        done
        echo
    fi

    if [[ ${#failed_validations[@]} -gt 0 ]]; then
        log_error "Failed to validate ${#failed_validations[@]} Dockerfiles:"
        for service in "${failed_validations[@]}"; do
            echo "  ❌ $service"
        done
        echo
        log_error "Please fix the issues above before building"
        return 1
    fi

    log_success "All Dockerfiles validated successfully!"
    return 0
}

# Generate image list
generate_image_list() {
    local output_file="docker/image-list.txt"

    log_info "Generating image list..."

    {
        echo "# Kubernaut Microservices Container Images"
        echo "# Generated on: $(date)"
        echo "# Version: ${VERSION}"
        echo "# Registry: ${REGISTRY}"
        echo ""

        for service in $(printf '%s\n' "${!SERVICES[@]}" | sort); do
            echo "${REGISTRY}/${service}:${VERSION}"
        done
    } > "$output_file"

    log_success "Image list generated: $output_file"
}

# Main execution
main() {
    local action=${1:-"build"}

    case "$action" in
        "build")
            validate_prerequisites
            build_all_services
            generate_image_list
            ;;
        "push")
            validate_prerequisites
            push_all_services
            ;;
        "build-and-push")
            validate_prerequisites
            if build_all_services; then
                push_all_services
                generate_image_list
            else
                log_error "Build failed, skipping push"
                exit 1
            fi
            ;;
        "validate")
            validate_prerequisites
            validate_all_dockerfiles
            ;;
        "list")
            for service in $(printf '%s\n' "${SERVICES[@]}" | sort); do
                echo "${REGISTRY}/${service}:${VERSION}"
            done
            ;;
        "help"|"-h"|"--help")
            echo "Usage: $0 [action] [version] [registry]"
            echo ""
            echo "Actions:"
            echo "  build          Build all container images (default)"
            echo "  push           Push all container images to registry"
            echo "  build-and-push Build and push all container images"
            echo "  validate       Validate all Dockerfiles for consistency"
            echo "  list           List all image names that would be built"
            echo "  help           Show this help message"
            echo ""
            echo "Arguments:"
            echo "  version        Image version tag (default: v1.0.0)"
            echo "  registry       Container registry (default: quay.io/jordigilh)"
            echo ""
            echo "Examples:"
            echo "  $0 build v1.1.0"
            echo "  $0 push v1.1.0 quay.io/jordigilh"
            echo "  $0 build-and-push v1.2.0"
            echo "  $0 list"
            ;;
        *)
            log_error "Unknown action: $action"
            echo "Run '$0 help' for usage information"
            exit 1
            ;;
    esac
}

# Handle script arguments
if [[ $# -eq 0 ]]; then
    main "build"
elif [[ $1 =~ ^(build|push|build-and-push|validate|list|help|-h|--help)$ ]]; then
    main "$@"
else
    # First argument is version, second is registry
    main "build" "$@"
fi
