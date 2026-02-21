#!/usr/bin/env bash
# StatefulSet PVC Failure Demo -- Automated Runner
# Scenario #137: PVC deleted -> pod stuck Pending -> recreate PVC
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-statefulset"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

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
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (KubeStatefulSetReplicasMismatch) -> Gateway -> SP -> AA (HAPI)"
echo "    LLM detects stateful=true, diagnoses PVC failure"
echo "    WE recreates PVC, deletes stuck pod"
echo "    EM verifies all replicas ready"
echo ""
echo "==> Verify remediation:"
echo "    kubectl get pods -n ${NAMESPACE}"
echo "    kubectl get pvc -n ${NAMESPACE}"
