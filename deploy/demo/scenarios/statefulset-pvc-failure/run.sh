#!/usr/bin/env bash
# StatefulSet PVC Failure Demo -- Automated Runner
# Scenario #137: PVC deleted -> pod stuck Pending -> recreate PVC
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-statefulset"

APPROVE_MODE="--auto-approve"
SKIP_VALIDATE=""
for _arg in "$@"; do
    case "$_arg" in
        --auto-approve)  APPROVE_MODE="--auto-approve" ;;
        --interactive)   APPROVE_MODE="--interactive" ;;
        --no-validate)   SKIP_VALIDATE=true ;;
    esac
done

# shellcheck source=../../scripts/platform-helper.sh
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
require_demo_ready

echo "============================================="
echo " StatefulSet PVC Failure Demo (#137)"
echo "============================================="
echo ""

echo "==> Step 1: Deploying namespace and StatefulSet..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/statefulset.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

echo "==> Step 2: Waiting for all StatefulSet pods to be ready..."
kubectl rollout status statefulset/kv-store -n "${NAMESPACE}" --timeout=180s
kubectl get pods -n "${NAMESPACE}"
echo ""

echo "==> Step 3: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established."
echo ""

echo "==> Step 4: Injecting PVC failure..."
bash "${SCRIPT_DIR}/inject-pvc-issue.sh"
echo ""

echo "==> Step 5: Waiting for KubeStatefulSetReplicasMismatch alert (~3-4 min)..."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""
# Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
