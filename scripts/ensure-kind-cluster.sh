#!/bin/bash
# Script to ensure a Kind cluster exists for integration testing
# Per ADR-016: Service-Specific Integration Test Infrastructure

set -e

CLUSTER_NAME="${KIND_CLUSTER_NAME:-kubernaut-integration}"
KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"

echo "🔍 Checking for Kind cluster: ${CLUSTER_NAME}..."

# Check if kind is installed
if ! command -v kind &> /dev/null; then
    echo "❌ Error: Kind is not installed"
    echo "   Please install Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    exit 1
fi

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "✅ Kind cluster '${CLUSTER_NAME}' already exists"

    # Verify cluster is accessible
    if kubectl cluster-info --context "kind-${CLUSTER_NAME}" &> /dev/null; then
        echo "✅ Cluster is accessible"
    else
        echo "⚠️  Cluster exists but is not accessible, attempting to recreate..."
        kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
        echo "🔧 Creating Kind cluster: ${CLUSTER_NAME}..."
        kind create cluster --name "${CLUSTER_NAME}" --wait 2m
    fi
else
    echo "🔧 Kind cluster not found, creating: ${CLUSTER_NAME}..."
    kind create cluster --name "${CLUSTER_NAME}" --wait 2m
    echo "✅ Kind cluster '${CLUSTER_NAME}' created successfully"
fi

# Final verification
if ! kubectl cluster-info --context "kind-${CLUSTER_NAME}" &> /dev/null; then
    echo "❌ Error: Kind cluster exists but is not accessible"
    echo "   Try running: kubectl config use-context kind-${CLUSTER_NAME}"
    exit 1
fi

echo "✅ Kind cluster '${CLUSTER_NAME}' is ready for integration testing"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   KUBECONFIG: ${KUBECONFIG}"
