#!/bin/bash
# Setup envtest for integration testing with real Kubernetes API
# This provides a real Kubernetes API server for integration tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

# Setup envtest binaries
setup_envtest() {
    log_info "Setting up envtest for real Kubernetes API integration testing..."

    cd "$PROJECT_ROOT"

    # Install setup-envtest if not available
    if ! command -v setup-envtest &> /dev/null; then
        log_info "Installing setup-envtest..."
        go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    fi

    # Create bin directory
    mkdir -p bin

    # Setup envtest binaries
    log_info "Setting up Kubernetes test binaries..."
    KUBEBUILDER_ASSETS=$(setup-envtest use --bin-dir ./bin -p path)

    if [ -z "$KUBEBUILDER_ASSETS" ]; then
        log_error "Failed to setup envtest binaries"
        exit 1
    fi

    log_success "Envtest binaries available at: $KUBEBUILDER_ASSETS"

    # Update integration environment
    log_info "Updating integration environment configuration..."

    # Remove old KUBEBUILDER_ASSETS entries
    if [ -f .env.integration ]; then
        grep -v "KUBEBUILDER_ASSETS" .env.integration > .env.integration.tmp || true
        mv .env.integration.tmp .env.integration
    fi

    # Add new configuration
    cat >> .env.integration << EOF

# Real Kubernetes API (envtest) Configuration
KUBEBUILDER_ASSETS=${KUBEBUILDER_ASSETS}
USE_FAKE_K8S_CLIENT=false
USE_REAL_CLUSTER=true
USE_ENVTEST=true
EOF

    log_success "Integration environment updated"
}

# Verify setup
verify_setup() {
    log_info "Verifying envtest setup..."

    cd "$PROJECT_ROOT"
    source .env.integration

    if [ -z "${KUBEBUILDER_ASSETS:-}" ]; then
        log_error "KUBEBUILDER_ASSETS not set in environment"
        exit 1
    fi

    if [ ! -d "$KUBEBUILDER_ASSETS" ]; then
        log_error "KUBEBUILDER_ASSETS directory does not exist: $KUBEBUILDER_ASSETS"
        exit 1
    fi

    # Check for required binaries
    for binary in kube-apiserver etcd kubectl; do
        if [ ! -f "$KUBEBUILDER_ASSETS/$binary" ]; then
            log_error "Required binary not found: $KUBEBUILDER_ASSETS/$binary"
            exit 1
        fi
    done

    log_success "Envtest setup verified successfully"
}

# Main function
main() {
    echo "🚀 Setting up envtest for real Kubernetes API integration testing"
    echo "================================================================="
    echo ""

    setup_envtest
    verify_setup

    echo ""
    log_success "🎉 Envtest setup completed!"
    echo ""
    echo "Configuration:"
    echo "  KUBEBUILDER_ASSETS: $(source .env.integration && echo $KUBEBUILDER_ASSETS)"
    echo "  Real Kubernetes API: Enabled"
    echo "  Fake K8s Client: Disabled"
    echo ""
    echo "Next steps:"
    echo "  1. source .env.integration"
    echo "  2. make test-integration-dev"
    echo ""
    echo "The integration tests will now use a real Kubernetes API server"
    echo "provided by envtest instead of fake clients."
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Sets up envtest for real Kubernetes API integration testing."
        echo ""
        echo "This provides a real Kubernetes API server for integration tests"
        echo "without requiring a full cluster like Kind."
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac
