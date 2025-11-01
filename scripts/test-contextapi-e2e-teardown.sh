#!/bin/bash
# Context API E2E Test Infrastructure Teardown
# Cleans up Podman + Kind E2E test infrastructure

set -e

# Configuration
CLUSTER_NAME="${KIND_CLUSTER_NAME:-kubernaut-contextapi-e2e}"
NAMESPACE="${CONTEXT_API_NAMESPACE:-contextapi-e2e}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "════════════════════════════════════════════════════════════════════════"
echo "🧹 Context API E2E Test Infrastructure Teardown"
echo "════════════════════════════════════════════════════════════════════════"
echo ""

# Parse command line arguments
FULL_TEARDOWN=false
if [ "$1" == "--full" ]; then
    FULL_TEARDOWN=true
    echo "Mode: FULL TEARDOWN (delete Kind cluster)"
else
    echo "Mode: PARTIAL TEARDOWN (keep Kind cluster)"
    echo "Use --full flag to delete the Kind cluster"
fi
echo ""

# Check if cluster exists
if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo -e "${YELLOW}⚠️  Kind cluster '${CLUSTER_NAME}' does not exist${NC}"
    exit 0
fi

# Switch to cluster context
kubectl config use-context "kind-${CLUSTER_NAME}" 2>/dev/null || true

# Delete namespace (this will delete all resources)
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🗑️  Deleting namespace: ${NAMESPACE}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if kubectl get namespace "${NAMESPACE}" &> /dev/null; then
    kubectl delete namespace "${NAMESPACE}" --wait=true --timeout=60s
    echo -e "${GREEN}✅ Namespace '${NAMESPACE}' deleted${NC}"
else
    echo -e "${YELLOW}⚠️  Namespace '${NAMESPACE}' does not exist${NC}"
fi
echo ""

# Full teardown: delete Kind cluster
if [ "$FULL_TEARDOWN" == true ]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🗑️  Deleting Kind cluster: ${CLUSTER_NAME}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    KIND_EXPERIMENTAL_PROVIDER=podman kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
    echo -e "${GREEN}✅ Kind cluster '${CLUSTER_NAME}' deleted${NC}"
    echo ""
fi

echo "════════════════════════════════════════════════════════════════════════"
echo -e "${GREEN}✅ CONTEXT API E2E TEARDOWN COMPLETE${NC}"
echo "════════════════════════════════════════════════════════════════════════"
echo ""

if [ "$FULL_TEARDOWN" == false ]; then
    echo "💡 Kind cluster '${CLUSTER_NAME}' is still running"
    echo "   To delete it, run: $0 --full"
    echo ""
fi

