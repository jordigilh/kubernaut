#!/bin/bash
# Validate that integration tests are running on a real Kind cluster
# This script MUST be called by integration tests to ensure real cluster usage

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[CLUSTER-VALIDATION]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[CLUSTER-VALIDATION]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[CLUSTER-VALIDATION]${NC} $1"
}

log_error() {
    echo -e "${RED}[CLUSTER-VALIDATION]${NC} $1"
}

# Validate real cluster is required
validate_real_cluster_requirement() {
    log_info "Validating real cluster requirement..."

    # Check environment variables that enforce real cluster
    if [[ "${REQUIRE_REAL_CLUSTER:-false}" == "true" ]]; then
        log_info "Real cluster is REQUIRED by environment configuration"
    else
        log_warning "REQUIRE_REAL_CLUSTER not set - tests may use fake clients"
    fi

    if [[ "${FAIL_ON_FAKE_CLUSTER:-false}" == "true" ]]; then
        log_info "Tests will FAIL if fake cluster is detected"
    else
        log_warning "FAIL_ON_FAKE_CLUSTER not set - tests may silently use fake clients"
    fi

    if [[ "${USE_FAKE_K8S_CLIENT:-true}" == "false" ]]; then
        log_info "Fake Kubernetes client is DISABLED"
    else
        log_error "USE_FAKE_K8S_CLIENT is not disabled - tests may use fake clients"
        return 1
    fi
}

# Validate Kind cluster exists and is accessible
validate_kind_cluster() {
    log_info "Validating Kind cluster exists and is accessible..."

    # Check if kind command is available
    if ! command -v kind &> /dev/null; then
        log_error "Kind command not found - cannot validate cluster"
        return 1
    fi

    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl command not found - cannot validate cluster"
        return 1
    fi

    # Get expected cluster name
    local cluster_name="${KIND_CLUSTER_NAME:-${CLUSTER_NAME:-kubernaut-dev}}"

    # Check if Kind cluster exists
    if ! kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
        log_error "Kind cluster '${cluster_name}' does not exist"
        log_error "Run 'make bootstrap-dev' to create the required cluster"
        return 1
    fi

    log_success "Kind cluster '${cluster_name}' exists"

    # Check if cluster is accessible
    local context="kind-${cluster_name}"
    if ! kubectl config get-contexts | grep -q "$context"; then
        log_error "kubectl context '${context}' not found"
        return 1
    fi

    # Test cluster connectivity
    if ! kubectl --context="$context" cluster-info &>/dev/null; then
        log_error "Cannot connect to Kind cluster '${cluster_name}'"
        log_error "Cluster may be unhealthy or not running"
        return 1
    fi

    log_success "Kind cluster '${cluster_name}' is accessible and healthy"

    # Validate cluster has nodes
    local node_count=$(kubectl --context="$context" get nodes --no-headers 2>/dev/null | wc -l | xargs)
    if [ "$node_count" -eq 0 ]; then
        log_error "Kind cluster has no nodes"
        return 1
    fi

    log_success "Kind cluster has $node_count node(s)"

    # Check if nodes are ready
    local ready_nodes=$(kubectl --context="$context" get nodes --no-headers 2>/dev/null | grep " Ready " | wc -l | xargs)
    if [ "$ready_nodes" -eq 0 ]; then
        log_error "No nodes are in Ready state"
        return 1
    fi

    log_success "$ready_nodes/$node_count nodes are Ready"

    return 0
}

# Validate current kubectl context points to Kind cluster
validate_kubectl_context() {
    log_info "Validating kubectl context points to Kind cluster..."

    local current_context=$(kubectl config current-context 2>/dev/null || echo "none")

    if [[ "$current_context" != kind-* ]]; then
        log_error "Current kubectl context '$current_context' is not a Kind cluster"
        log_error "Expected context to start with 'kind-'"
        return 1
    fi

    log_success "Current kubectl context '$current_context' is a Kind cluster"

    # Test that we can actually use the cluster
    if ! kubectl get nodes &>/dev/null; then
        log_error "Cannot access nodes in current context"
        return 1
    fi

    log_success "Can successfully access cluster nodes"

    return 0
}

# Main validation function
main() {
    echo "🔍 Validating Real Kind Cluster for Integration Tests"
    echo "===================================================="
    echo ""

    local validation_failed=false

    # Run all validations
    if ! validate_real_cluster_requirement; then
        validation_failed=true
    fi

    if ! validate_kind_cluster; then
        validation_failed=true
    fi

    if ! validate_kubectl_context; then
        validation_failed=true
    fi

    echo ""

    if [ "$validation_failed" = true ]; then
        log_error "❌ REAL CLUSTER VALIDATION FAILED"
        echo ""
        echo "Integration tests REQUIRE a real Kind cluster to run."
        echo "Fake clusters or mock clients are NOT allowed."
        echo ""
        echo "To fix this:"
        echo "  1. Run: make bootstrap-dev"
        echo "  2. Ensure Kind cluster is created successfully"
        echo "  3. Source environment: source .env.development"
        echo "  4. Re-run integration tests"
        echo ""
        exit 1
    else
        log_success "✅ REAL CLUSTER VALIDATION PASSED"
        echo ""
        echo "Integration tests are configured to use a real Kind cluster."
        echo "Fake clients are disabled and real cluster is accessible."
        echo ""
        return 0
    fi
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Validates that integration tests are running on a real Kind cluster."
        echo "This script ensures that:"
        echo "  1. Real cluster is required by environment configuration"
        echo "  2. Kind cluster exists and is accessible"
        echo "  3. kubectl context points to Kind cluster"
        echo "  4. Fake Kubernetes clients are disabled"
        echo ""
        echo "Integration tests MUST call this script to ensure real cluster usage."
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac
