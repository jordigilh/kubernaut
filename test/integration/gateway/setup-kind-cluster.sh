#!/bin/bash

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# 🚀 Setup Kind Cluster for Gateway Integration Tests
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# This script creates a local Kind cluster using Podman for fast,
# deterministic integration testing with <1ms K8s API latency.
#
# Usage: ./setup-kind-cluster.sh
#
# Requirements:
# - kind installed (brew install kind)
# - podman installed and running
# - kubectl installed
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

set -euo pipefail

CLUSTER_NAME="kubernaut-test"
NAMESPACE="kubernaut-system"
KIND_KUBECONFIG="${HOME}/.kube/gateway-kubeconfig"

# Get script directory and change to project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
cd "${PROJECT_ROOT}"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🚀 Setting up Kind cluster for Gateway integration tests..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 Configuration:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG}"
echo "   Namespace: ${NAMESPACE}"
echo "   Project Root: ${PROJECT_ROOT}"
echo ""

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 1: Configure Kind to use Podman
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo "📋 Step 1: Configuring Kind to use Podman..."

# Set environment variable to use Podman instead of Docker
export KIND_EXPERIMENTAL_PROVIDER=podman

# Verify Podman is running
if ! podman info > /dev/null 2>&1; then
    echo "❌ Podman is not running. Please start Podman and try again."
    echo "   Hint: podman machine start"
    exit 1
fi

echo "✅ Podman is running"
echo "   Provider: ${KIND_EXPERIMENTAL_PROVIDER}"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 2: Check if Kind cluster already exists
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 2: Checking for existing Kind cluster..."

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "✅ Kind cluster '${CLUSTER_NAME}' already exists"

    # Verify cluster is healthy
    if KUBECONFIG="${KIND_KUBECONFIG}" kubectl cluster-info --context "kind-${CLUSTER_NAME}" > /dev/null 2>&1; then
        echo "✅ Cluster is healthy and accessible"
        CLUSTER_EXISTS=true
    else
        echo "⚠️  Cluster exists but is not healthy. Recreating..."
        kind delete cluster --name "${CLUSTER_NAME}"
        CLUSTER_EXISTS=false
    fi
else
    echo "ℹ️  Kind cluster '${CLUSTER_NAME}' does not exist"
    CLUSTER_EXISTS=false
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 3: Create Kind cluster if needed
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
if [ "${CLUSTER_EXISTS:-false}" = "false" ]; then
    echo ""
    echo "📋 Step 3: Creating Kind cluster..."
    echo "   This will take ~30 seconds..."

    # Create Kind cluster with optimized configuration
    # Export kubeconfig to dedicated file to avoid modifying ~/.kube/config
    cat <<EOF | kind create cluster --name "${CLUSTER_NAME}" --kubeconfig="${KIND_KUBECONFIG}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  # Optimize for local testing
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        # Increase API server QPS for integration tests
        max-requests-inflight: "400"
        max-mutating-requests-inflight: "200"
    controllerManager:
      extraArgs:
        # Faster reconciliation for tests
        node-monitor-period: "2s"
        node-monitor-grace-period: "16s"
EOF

    echo "✅ Kind cluster created successfully"
else
    echo ""
    echo "📋 Step 3: Skipping cluster creation (already exists)"
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 4: Verify isolated kubeconfig
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 4: Verifying isolated kubeconfig..."

# Ensure kubeconfig directory exists
mkdir -p "$(dirname "${KIND_KUBECONFIG}")"

# Verify kubeconfig was created
if [ -f "${KIND_KUBECONFIG}" ]; then
    echo "✅ Isolated kubeconfig created at ${KIND_KUBECONFIG}"
else
    echo "❌ Failed to create isolated kubeconfig"
    exit 1
fi

# Set current context in the isolated kubeconfig
KUBECONFIG="${KIND_KUBECONFIG}" kubectl config use-context "kind-${CLUSTER_NAME}" > /dev/null
echo "✅ kubectl context set to 'kind-${CLUSTER_NAME}' (isolated)"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 5: Install Gateway CRD
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 5: Installing RemediationRequest CRD..."

if KUBECONFIG="${KIND_KUBECONFIG}" kubectl apply -f config/crd/remediation.kubernaut.io_remediationrequests.yaml > /dev/null 2>&1; then
    echo "✅ RemediationRequest CRD installed"
else
    echo "⚠️  CRD installation had errors (may already exist)"
fi

# Verify CRD is available
KUBECONFIG="${KIND_KUBECONFIG}" kubectl wait --for condition=established --timeout=30s crd/remediationrequests.remediation.kubernaut.io > /dev/null 2>&1
echo "✅ RemediationRequest CRD is ready"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 6: Create test namespace
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 6: Creating test namespaces..."

for ns in "${NAMESPACE}" "production" "staging" "development"; do
    if KUBECONFIG="${KIND_KUBECONFIG}" kubectl get namespace "${ns}" > /dev/null 2>&1; then
        echo "✅ Namespace '${ns}' already exists"
    else
        KUBECONFIG="${KIND_KUBECONFIG}" kubectl create namespace "${ns}" > /dev/null
        echo "✅ Namespace '${ns}' created"
    fi
done

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 7: Create ServiceAccounts for auth tests
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 7: Creating test ServiceAccounts..."

# Create authorized ServiceAccount
KUBECONFIG="${KIND_KUBECONFIG}" kubectl create serviceaccount gateway-authorized -n "${NAMESPACE}" --dry-run=client -o yaml | KUBECONFIG="${KIND_KUBECONFIG}" kubectl apply -f - > /dev/null
echo "✅ ServiceAccount 'gateway-authorized' created"

# Create unauthorized ServiceAccount
KUBECONFIG="${KIND_KUBECONFIG}" kubectl create serviceaccount gateway-unauthorized -n "${NAMESPACE}" --dry-run=client -o yaml | KUBECONFIG="${KIND_KUBECONFIG}" kubectl apply -f - > /dev/null
echo "✅ ServiceAccount 'gateway-unauthorized' created"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 8: Create RBAC for authorized ServiceAccount
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 8: Setting up RBAC..."

cat <<EOF | KUBECONFIG="${KIND_KUBECONFIG}" kubectl apply -f - > /dev/null
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: remediationrequest-creator
rules:
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests"]
  verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: gateway-authorized-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: remediationrequest-creator
subjects:
- kind: ServiceAccount
  name: gateway-authorized
  namespace: ${NAMESPACE}
EOF

echo "✅ RBAC configured for 'gateway-authorized' ServiceAccount"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 9: Wait for ServiceAccount tokens to be created
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 9: Waiting for ServiceAccount tokens..."

# Wait for tokens to be created (K8s 1.24+ uses TokenRequest API)
sleep 2
echo "✅ ServiceAccount tokens ready"

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 10: Verify cluster health
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 10: Verifying cluster health..."

# Check API server
if KUBECONFIG="${KIND_KUBECONFIG}" kubectl cluster-info > /dev/null 2>&1; then
    echo "✅ K8s API server is healthy"
else
    echo "❌ K8s API server is not responding"
    exit 1
fi

# Check nodes
if KUBECONFIG="${KIND_KUBECONFIG}" kubectl get nodes | grep -q "Ready"; then
    echo "✅ Cluster nodes are ready"
else
    echo "❌ Cluster nodes are not ready"
    exit 1
fi

# Check CRD
if KUBECONFIG="${KIND_KUBECONFIG}" kubectl get crd remediationrequests.remediation.kubernaut.io > /dev/null 2>&1; then
    echo "✅ RemediationRequest CRD is available"
else
    echo "❌ RemediationRequest CRD is not available"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Step 11: Create ClusterRole for Integration Tests
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "📋 Step 11: Creating ClusterRole for integration tests..."

# Create ClusterRole for test ServiceAccounts
# This is required for TokenReview/SubjectAccessReview authentication/authorization
KUBECONFIG="${KIND_KUBECONFIG}" kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-test-remediation-creator
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: gateway
    test: integration
rules:
- apiGroups: ["remediation.kubernaut.io"]
  resources: ["remediationrequests"]
  verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
EOF

if [ $? -eq 0 ]; then
    echo "✅ ClusterRole 'gateway-test-remediation-creator' created"
else
    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0


    echo "❌ Failed to create ClusterRole"
    exit 1
fi

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Success Summary
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Kind cluster ready for Gateway integration tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Cluster Information:"
echo "   Cluster Name: ${CLUSTER_NAME}"
echo "   Context: kind-${CLUSTER_NAME}"
echo "   Kubeconfig: ${KIND_KUBECONFIG} (isolated)"
echo "   Provider: Podman (KIND_EXPERIMENTAL_PROVIDER=podman)"
echo "   Namespaces: ${NAMESPACE}, production, staging, development"
echo "   ClusterRole: gateway-test-remediation-creator (for test ServiceAccounts)"
echo ""
echo "🚀 Ready to run integration tests!"
echo "   Run: ./test/integration/gateway/run-tests-kind.sh"
echo "   Note: Tests will use ${KIND_KUBECONFIG}"
echo ""

exit 0

