#!/bin/bash
# Debug Script for HolmesGPT-API E2E Deployment Failure
# Issue: HAPI pod crash-looping in Kind cluster with ExitCode=2
# Date: 2025-12-29

set -e

CLUSTER_NAME="aianalysis-e2e"
NAMESPACE="kubernaut-system"
APP_LABEL="holmesgpt-api"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔍 HolmesGPT-API E2E Deployment Debug Script"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check if cluster exists
if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "❌ Kind cluster '${CLUSTER_NAME}' not found"
    echo "Creating cluster and deploying infrastructure..."
    echo ""

    # Run E2E test setup but keep cluster alive on failure
    echo "🚀 Running E2E test setup (will fail at HAPI deployment)..."
    cd "$(git rev-parse --show-toplevel)"

    # Set environment variable to keep cluster on failure
    export KEEP_CLUSTER_ON_FAILURE=true

    # Run tests and capture failure (expected)
    timeout 600 ginkgo -v --timeout=15m --fail-fast ./test/e2e/aianalysis/... 2>&1 | tee /tmp/hapi-debug-setup.log || true

    echo ""
    echo "⚠️  Setup completed (failure expected)"
    echo ""
fi

# Verify cluster is running
if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "❌ Failed to create or find cluster. Exiting."
    exit 1
fi

echo "✅ Kind cluster '${CLUSTER_NAME}' is running"
echo ""

# Set kubeconfig
export KUBECONFIG="$(kind get kubeconfig --name=${CLUSTER_NAME} 2>/dev/null | grep -v 'enabling experimental')"
if [ -z "$KUBECONFIG" ]; then
    kind get kubeconfig --name=${CLUSTER_NAME} > /tmp/kind-config-${CLUSTER_NAME}
    export KUBECONFIG="/tmp/kind-config-${CLUSTER_NAME}"
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📦 STEP 1: Check HAPI Pod Status"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

kubectl get pods -n ${NAMESPACE} -l app=${APP_LABEL} -o wide || echo "⚠️  No HAPI pods found"
echo ""

# Get pod name
POD_NAME=$(kubectl get pods -n ${NAMESPACE} -l app=${APP_LABEL} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$POD_NAME" ]; then
    echo "❌ No HAPI pod found in namespace ${NAMESPACE}"
    echo ""
    echo "Checking all namespaces..."
    kubectl get pods --all-namespaces -l app=${APP_LABEL}
    echo ""

    echo "Checking deployment status..."
    kubectl get deployment -n ${NAMESPACE} ${APP_LABEL} -o yaml 2>/dev/null || echo "⚠️  No deployment found"
    exit 1
fi

echo "✅ Found HAPI pod: ${POD_NAME}"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📋 STEP 2: Pod Description and Events"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

kubectl describe pod ${POD_NAME} -n ${NAMESPACE}
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📜 STEP 3: Container Logs (Current and Previous)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Current logs:"
echo "─────────────"
kubectl logs ${POD_NAME} -n ${NAMESPACE} -c ${APP_LABEL} --tail=100 2>&1 || echo "⚠️  No current logs available"
echo ""

echo "Previous logs (if crashed):"
echo "───────────────────────────"
kubectl logs ${POD_NAME} -n ${NAMESPACE} -c ${APP_LABEL} --previous --tail=100 2>&1 || echo "⚠️  No previous logs available"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🗂️  STEP 4: ConfigMap Verification"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Checking ConfigMap..."
kubectl get configmap -n ${NAMESPACE} holmesgpt-api-config -o yaml 2>&1 || echo "⚠️  ConfigMap not found"
echo ""

echo "Verifying ConfigMap mount in pod spec..."
kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.volumes[?(@.name=="config")]}' | jq '.' 2>/dev/null || echo "⚠️  Config volume not found"
echo ""

kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.containers[0].volumeMounts[?(@.name=="config")]}' | jq '.' 2>/dev/null || echo "⚠️  Config mount not found"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🖼️  STEP 5: Image Verification in Kind"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Images in Kind cluster matching 'holmesgpt'..."
docker exec ${CLUSTER_NAME}-control-plane crictl images | grep -i holmesgpt || echo "⚠️  No holmesgpt images found in Kind"
echo ""

echo "Image used by pod:"
kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.containers[0].image}'
echo ""
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔧 STEP 6: Container Environment and Args"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Container args:"
kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.containers[0].args}' | jq '.' 2>/dev/null
echo ""

echo "Container environment:"
kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.spec.containers[0].env}' | jq '.' 2>/dev/null
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🐚 STEP 7: Exec into Container (if running)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

POD_STATUS=$(kubectl get pod ${POD_NAME} -n ${NAMESPACE} -o jsonpath='{.status.phase}')
echo "Pod status: ${POD_STATUS}"

if [ "$POD_STATUS" = "Running" ]; then
    echo ""
    echo "Checking config file in container..."
    kubectl exec ${POD_NAME} -n ${NAMESPACE} -c ${APP_LABEL} -- ls -la /etc/holmesgpt/ 2>&1 || echo "⚠️  Cannot exec into container"
    echo ""

    echo "Config file contents:"
    kubectl exec ${POD_NAME} -n ${NAMESPACE} -c ${APP_LABEL} -- cat /etc/holmesgpt/config.yaml 2>&1 || echo "⚠️  Cannot read config file"
    echo ""

    echo "Python version:"
    kubectl exec ${POD_NAME} -n ${NAMESPACE} -c ${APP_LABEL} -- python --version 2>&1 || echo "⚠️  Cannot check Python version"
    echo ""

    echo "Checking if HAPI module is importable:"
    kubectl exec ${POD_NAME} -n ${NAMESPACE} -c ${APP_LABEL} -- python -c "import holmesgpt_api; print('✅ Module imported successfully')" 2>&1 || echo "⚠️  Cannot import holmesgpt_api module"
else
    echo "⚠️  Pod not running (status: ${POD_STATUS}), cannot exec"
fi
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔍 STEP 8: Check Dependencies (PostgreSQL, Data Storage)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "PostgreSQL pods:"
kubectl get pods -n ${NAMESPACE} -l app=postgresql -o wide || echo "⚠️  No PostgreSQL pods found"
echo ""

echo "Data Storage pods:"
kubectl get pods -n ${NAMESPACE} -l app=datastorage -o wide || echo "⚠️  No Data Storage pods found"
echo ""

echo "Data Storage service:"
kubectl get svc -n ${NAMESPACE} datastorage || echo "⚠️  Data Storage service not found"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 STEP 9: Recent Events in Namespace"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

kubectl get events -n ${NAMESPACE} --sort-by='.lastTimestamp' | tail -20
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Debug Information Collection Complete"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 Summary:"
echo "  - Pod Name: ${POD_NAME}"
echo "  - Pod Status: ${POD_STATUS}"
echo "  - Cluster: ${CLUSTER_NAME}"
echo "  - Namespace: ${NAMESPACE}"
echo ""
echo "🔧 Next Steps:"
echo "  1. Review logs above for Python errors or missing dependencies"
echo "  2. Check ConfigMap content matches expected format"
echo "  3. Verify image contains HAPI application code"
echo "  4. Test HAPI standalone: scripts/test-hapi-standalone.sh"
echo ""
echo "🗑️  To clean up cluster: kind delete cluster --name ${CLUSTER_NAME}"
echo ""



