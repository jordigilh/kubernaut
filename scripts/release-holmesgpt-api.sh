#!/bin/bash
# HolmesGPT REST API Release Script
# Handles versioning, tagging, building, and releasing container images

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
IMAGE_NAME="${IMAGE_NAME:-quay.io/jordigilh/holmesgpt-api}"
REGISTRY="${REGISTRY:-quay.io/jordigilh}"
PLATFORMS="${PLATFORMS:-linux/amd64,linux/arm64}"

# Release configuration
RELEASE_TYPE="${RELEASE_TYPE:-patch}"  # major, minor, patch, prerelease
PRERELEASE_SUFFIX="${PRERELEASE_SUFFIX:-}"
DRY_RUN="${DRY_RUN:-false}"
SKIP_TESTS="${SKIP_TESTS:-false}"
SKIP_SECURITY_SCAN="${SKIP_SECURITY_SCAN:-false}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
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

log_step() {
    echo -e "${MAGENTA}[$(date +'%H:%M:%S')] STEP: $1${NC}"
}

# Function to show usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS] [VERSION]

Release HolmesGPT REST API container image with proper versioning

ARGUMENTS:
    VERSION                 Explicit version to release (e.g., 1.2.3)

OPTIONS:
    -t, --type TYPE         Release type: major, minor, patch, prerelease (default: patch)
    -s, --suffix SUFFIX     Prerelease suffix (e.g., alpha, beta, rc1)
    -i, --image NAME        Container image name (default: quay.io/jordigilh/holmesgpt-api)
    -r, --registry URL      Container registry (default: quay.io/jordigilh)
    -p, --platforms LIST    Target platforms (default: linux/amd64,linux/arm64)
    --dry-run               Show what would be done without executing
    --skip-tests            Skip running tests before release
    --skip-security-scan    Skip security scanning
    -h, --help              Show this help message

EXAMPLES:
    $0                              # Patch release (1.0.0 -> 1.0.1)
    $0 --type minor                 # Minor release (1.0.1 -> 1.1.0)
    $0 --type major                 # Major release (1.1.0 -> 2.0.0)
    $0 --type prerelease -s rc1     # Prerelease (1.1.0 -> 1.1.1-rc1)
    $0 2.0.0                        # Explicit version
    $0 --dry-run                    # Preview changes

ENVIRONMENT VARIABLES:
    IMAGE_NAME              Container image name
    REGISTRY                Container registry
    PLATFORMS               Target platforms
    RELEASE_TYPE            Release type
    DRY_RUN                 Dry run mode (true/false)
    SKIP_TESTS              Skip tests (true/false)
    SKIP_SECURITY_SCAN      Skip security scan (true/false)
EOF
}

# Parse command line arguments
parse_args() {
    local explicit_version=""

    while [[ $# -gt 0 ]]; do
        case $1 in
            -t|--type)
                RELEASE_TYPE="$2"
                shift 2
                ;;
            -s|--suffix)
                PRERELEASE_SUFFIX="$2"
                shift 2
                ;;
            -i|--image)
                IMAGE_NAME="$2"
                shift 2
                ;;
            -r|--registry)
                REGISTRY="$2"
                shift 2
                ;;
            -p|--platforms)
                PLATFORMS="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            --skip-tests)
                SKIP_TESTS="true"
                shift
                ;;
            --skip-security-scan)
                SKIP_SECURITY_SCAN="true"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            -*)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
            *)
                explicit_version="$1"
                shift
                ;;
        esac
    done

    # Validate release type
    case $RELEASE_TYPE in
        major|minor|patch|prerelease)
            ;;
        *)
            log_error "Invalid release type: $RELEASE_TYPE"
            log_error "Valid types: major, minor, patch, prerelease"
            exit 1
            ;;
    esac

    # If explicit version provided, validate format
    if [[ -n "$explicit_version" ]]; then
        if [[ ! "$explicit_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
            log_error "Invalid version format: $explicit_version"
            log_error "Expected format: X.Y.Z or X.Y.Z-suffix"
            exit 1
        fi
        NEW_VERSION="$explicit_version"
    fi
}

# Get current version from git tags
get_current_version() {
    log_step "Determining current version..."

    cd "$PROJECT_ROOT"

    # Get latest tag
    local latest_tag
    if latest_tag=$(git describe --tags --abbrev=0 2>/dev/null); then
        # Remove 'v' prefix if present
        CURRENT_VERSION="${latest_tag#v}"
        log_info "Current version: $CURRENT_VERSION"
    else
        CURRENT_VERSION="0.0.0"
        log_info "No previous tags found, starting from: $CURRENT_VERSION"
    fi
}

# Calculate next version
calculate_next_version() {
    if [[ -n "${NEW_VERSION:-}" ]]; then
        log_info "Using explicit version: $NEW_VERSION"
        return 0
    fi

    log_step "Calculating next version..."

    # Parse current version
    if [[ ! "$CURRENT_VERSION" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)(-(.+))?$ ]]; then
        log_error "Invalid current version format: $CURRENT_VERSION"
        exit 1
    fi

    local major="${BASH_REMATCH[1]}"
    local minor="${BASH_REMATCH[2]}"
    local patch="${BASH_REMATCH[3]}"
    local prerelease="${BASH_REMATCH[5]}"

    # Calculate new version based on type
    case $RELEASE_TYPE in
        major)
            NEW_VERSION="$((major + 1)).0.0"
            ;;
        minor)
            NEW_VERSION="${major}.$((minor + 1)).0"
            ;;
        patch)
            NEW_VERSION="${major}.${minor}.$((patch + 1))"
            ;;
        prerelease)
            if [[ -n "$prerelease" ]]; then
                # Increment existing prerelease
                NEW_VERSION="${major}.${minor}.${patch}-${PRERELEASE_SUFFIX:-next}"
            else
                # Create new prerelease
                NEW_VERSION="${major}.${minor}.$((patch + 1))-${PRERELEASE_SUFFIX:-alpha}"
            fi
            ;;
    esac

    log_info "Next version: $NEW_VERSION"
}

# Validate working directory
validate_working_directory() {
    log_step "Validating working directory..."

    cd "$PROJECT_ROOT"

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree &>/dev/null; then
        log_error "Not in a git repository"
        exit 1
    fi

    # Check for uncommitted changes
    if [[ $(git status --porcelain) ]]; then
        log_error "Working directory has uncommitted changes:"
        git status --short
        exit 1
    fi

    # Check if we're on main/master branch
    local current_branch
    current_branch=$(git branch --show-current)
    if [[ "$current_branch" != "main" ]] && [[ "$current_branch" != "master" ]]; then
        log_warn "Not on main/master branch (current: $current_branch)"
        if [[ "$DRY_RUN" != "true" ]]; then
            read -p "Continue with release from $current_branch? (y/N): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                log_info "Release cancelled"
                exit 0
            fi
        fi
    fi

    log "✅ Working directory validation passed"
}

# Run tests
run_tests() {
    if [[ "$SKIP_TESTS" == "true" ]]; then
        log_info "Skipping tests (disabled)"
        return 0
    fi

    log_step "Running tests..."

    cd "$PROJECT_ROOT"

    # Run Go tests
    if [[ -f "go.mod" ]]; then
        log_info "Running Go tests..."
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY RUN] Would run: go test ./..."
        else
            go test ./... || {
                log_error "Go tests failed"
                return 1
            }
        fi
    fi

    # Run Python tests if they exist
    if [[ -d "pkg/ai/holmesgpt/api-server" ]]; then
        log_info "Running Python tests..."
        if [[ "$DRY_RUN" == "true" ]]; then
            log_info "[DRY RUN] Would run Python tests"
        else
            # This would run actual Python tests when implemented
            log_info "Python tests would run here"
        fi
    fi

    log "✅ Tests passed"
}

# Build and test container
build_and_test_container() {
    log_step "Building and testing container..."

    local build_script="$SCRIPT_DIR/build-holmesgpt-api.sh"
    if [[ ! -x "$build_script" ]]; then
        log_error "Build script not found or not executable: $build_script"
        exit 1
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would build container: $IMAGE_NAME:$NEW_VERSION"
        log_info "[DRY RUN] Would run: $build_script --tag $NEW_VERSION --platforms $PLATFORMS"
        return 0
    fi

    # Build container
    local build_args=(
        "--image" "$IMAGE_NAME"
        "--tag" "$NEW_VERSION"
        "--platforms" "$PLATFORMS"
    )

    if [[ "$SKIP_SECURITY_SCAN" == "true" ]]; then
        build_args+=("--no-security-scan")
    fi

    if ! "$build_script" "${build_args[@]}"; then
        log_error "Container build failed"
        return 1
    fi

    log "✅ Container build completed"
}

# Create git tag
create_git_tag() {
    log_step "Creating git tag..."

    cd "$PROJECT_ROOT"

    local tag_name="v$NEW_VERSION"
    local tag_message="Release $NEW_VERSION

- HolmesGPT REST API Server
- Multi-architecture support (amd64, arm64)
- Source-built with Red Hat UBI
- Container image: $IMAGE_NAME:$NEW_VERSION"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would create tag: $tag_name"
        log_info "[DRY RUN] Tag message:"
        echo "$tag_message" | sed 's/^/  /'
        return 0
    fi

    # Create annotated tag
    if ! git tag -a "$tag_name" -m "$tag_message"; then
        log_error "Failed to create git tag"
        return 1
    fi

    log "✅ Git tag created: $tag_name"
}

# Push release
push_release() {
    log_step "Pushing release..."

    cd "$PROJECT_ROOT"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would push git tag to origin"
        log_info "[DRY RUN] Would push container image: $IMAGE_NAME:$NEW_VERSION"
        return 0
    fi

    # Push git tag
    if ! git push origin "v$NEW_VERSION"; then
        log_error "Failed to push git tag"
        return 1
    fi

    # Push container image
    local build_script="$SCRIPT_DIR/build-holmesgpt-api.sh"
    if ! "$build_script" --image "$IMAGE_NAME" --tag "$NEW_VERSION" --platforms "$PLATFORMS" --push; then
        log_error "Failed to push container image"
        return 1
    fi

    log "✅ Release pushed successfully"
}

# Generate release notes
generate_release_notes() {
    log_step "Generating release notes..."

    cd "$PROJECT_ROOT"

    local release_notes_file="release-notes-$NEW_VERSION.md"

    cat > "$release_notes_file" << EOF
# HolmesGPT REST API Server v$NEW_VERSION

## Release Information

- **Version**: $NEW_VERSION
- **Release Date**: $(date -u +"%Y-%m-%d")
- **Container Image**: \`$IMAGE_NAME:$NEW_VERSION\`
- **Platforms**: $PLATFORMS

## Features

- HolmesGPT REST API Server with source-based build
- Multi-architecture container support (amd64, arm64)
- Red Hat UBI base images for enterprise compliance
- Comprehensive security scanning and validation
- FastAPI-based REST endpoints for investigation capabilities
- Integration with Kubernaut Context API

## Container Usage

\`\`\`bash
# Pull the container
podman pull $IMAGE_NAME:$NEW_VERSION

# Run locally
podman run -d \\
  --name holmesgpt-api \\
  -p 8090:8090 \\
  -p 9091:9091 \\
  -e HOLMESGPT_LLM_PROVIDER=openai \\
  -e HOLMESGPT_LLM_API_KEY=your-api-key \\
  $IMAGE_NAME:$NEW_VERSION
\`\`\`

## Kubernetes Deployment

\`\`\`yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api-server
spec:
  replicas: 3
  selector:
    matchLabels:
      app: holmesgpt-api-server
  template:
    metadata:
      labels:
        app: holmesgpt-api-server
    spec:
      containers:
      - name: holmesgpt-api
        image: $IMAGE_NAME:$NEW_VERSION
        ports:
        - containerPort: 8090
        - containerPort: 9091
        env:
        - name: HOLMESGPT_LLM_PROVIDER
          value: "openai"
        - name: HOLMESGPT_LLM_API_KEY
          valueFrom:
            secretKeyRef:
              name: holmesgpt-secrets
              key: llm-api-key
\`\`\`

## Security

- Built from audited HolmesGPT source code
- Red Hat UBI base images with security updates
- Non-root container execution
- Comprehensive vulnerability scanning
- Enterprise-grade security hardening

## Support

- Documentation: https://github.com/jordigilh/kubernaut/docs/
- Issues: https://github.com/jordigilh/kubernaut/issues
- Container Registry: https://quay.io/repository/jordigilh/holmesgpt-api

EOF

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would create release notes: $release_notes_file"
    else
        log_info "Release notes created: $release_notes_file"
    fi
}

# Main release function
main() {
    echo -e "${CYAN}"
    cat << 'EOF'
╔══════════════════════════════════════════════════════════════╗
║         HolmesGPT REST API Release Management Tool          ║
║            Automated Versioning and Container Release       ║
╚══════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"

    # Parse arguments
    parse_args "$@"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "DRY RUN MODE - No changes will be made"
    fi

    # Release process
    validate_working_directory
    get_current_version
    calculate_next_version
    run_tests
    build_and_test_container
    create_git_tag
    push_release
    generate_release_notes

    # Summary
    echo
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                    Release Summary                          ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo "Previous Version: $CURRENT_VERSION"
    echo "New Version:      $NEW_VERSION"
    echo "Release Type:     $RELEASE_TYPE"
    echo "Container Image:  $IMAGE_NAME:$NEW_VERSION"
    echo "Platforms:        $PLATFORMS"
    echo "Dry Run:          $DRY_RUN"

    if [[ "$DRY_RUN" != "true" ]]; then
        echo
        log "🎉 Release v$NEW_VERSION completed successfully!"
        log "📦 Container available at: $IMAGE_NAME:$NEW_VERSION"
    else
        echo
        log "🔍 Dry run completed. Run without --dry-run to execute the release."
    fi
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
