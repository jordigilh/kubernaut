#!/bin/bash
# Gateway Integration Test Environment Setup
#
# This script sets up a minimal Kind cluster for Gateway integration tests.
# Components: Kind cluster + Redis + ServiceAccount token
#
# Usage: ./scripts/test-gateway-setup.sh
# Or:    make test-gateway-setup

set -euo pipefail

# Configuration
CLUSTER_NAME="kubernaut-gateway-test"
NAMESPACE="kubernaut-test"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    log_warning "Kind cluster '${CLUSTER_NAME}' already exists"
    log_info "Skipping cluster creation (use 'make test-gateway-teardown' to delete)"
else
    log_info "Creating Kind cluster with Ingress support: ${CLUSTER_NAME}"
    kind create cluster \
        --name "${CLUSTER_NAME}" \
        --config "${PROJECT_ROOT}/test/kind/kind-config-gateway.yaml" \
        --wait 60s
    log_success "Kind cluster created"
fi

# Set kubectl context
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null 2>&1

# Deploy Nginx Ingress Controller
log_info "Deploying Nginx Ingress Controller"
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml >/dev/null 2>&1
log_success "Nginx Ingress Controller deployed"

# Wait for Ingress Controller to be ready
log_info "Waiting for Ingress Controller to be ready..."
kubectl wait --namespace ingress-nginx \
    --for=condition=ready pod \
    --selector=app.kubernetes.io/component=controller \
    --timeout=120s
log_success "Ingress Controller is ready"

# Deploy Redis with NodePort
log_info "Deploying Redis (NodePort) to ${NAMESPACE}"
kubectl apply -f "${PROJECT_ROOT}/test/fixtures/redis-nodeport.yaml"
log_success "Redis deployment applied"

# Wait for Redis to be ready
log_info "Waiting for Redis pod to be ready..."
kubectl wait --for=condition=ready pod \
    -l app=redis \
    -n "${NAMESPACE}" \
    --timeout=120s
log_success "Redis is ready (accessible at localhost:6379 via NodePort)"

# Install CRDs (required for RemediationRequest creation)
log_info "Installing Kubernaut CRDs"
kubectl apply -f "${PROJECT_ROOT}/config/crd/bases/" >/dev/null 2>&1
log_success "CRDs installed"

# Create ServiceAccount for tests
log_info "Creating test ServiceAccount"
kubectl create serviceaccount test-gateway-sa -n "${NAMESPACE}" 2>/dev/null || true
log_success "ServiceAccount created"

# Create token and save to temp file
log_info "Generating ServiceAccount token"
kubectl create token test-gateway-sa \
    -n "${NAMESPACE}" \
    --duration 24h > /tmp/test-gateway-token.txt
log_success "Token saved to /tmp/test-gateway-token.txt"

# No port-forward needed - Kind NodePort auto-maps to host!

# Display cluster info
echo ""
log_success "Gateway test environment ready!"
echo ""
echo "Cluster: ${CLUSTER_NAME}"
echo "Namespace: ${NAMESPACE}"
echo "Token: /tmp/test-gateway-token.txt"
echo "Redis: localhost:6379 (NodePort 30379, auto-mapped by Kind)"
echo "Ingress: localhost:8080 (HTTP), localhost:8443 (HTTPS)"
echo ""
echo "Architecture:"
echo "  Host → localhost:6379 → Kind NodePort → Redis Pod"
echo "  Host → localhost:8080 → Kind Ingress → Gateway Service → Gateway Pod"
echo ""
echo "To run tests:"
echo "  export TEST_TOKEN=\$(cat /tmp/test-gateway-token.txt)"
echo "  cd test/integration/gateway && ginkgo -v"
echo ""
echo "Or simply:"
echo "  make test-gateway"
echo ""

