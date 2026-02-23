#!/usr/bin/env bash
# NetworkPolicy Traffic Block Demo -- Automated Runner
# Scenario #138: Deny-all NetworkPolicy -> health checks fail -> fix policy
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-netpol"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "network-policy-block"

echo "============================================="
echo " NetworkPolicy Traffic Block Demo (#138)"
echo "============================================="
echo ""

echo "==> Step 1: Deploying namespace, workload, and baseline NetworkPolicy..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/networkpolicy-allow.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

echo "==> Step 2: Waiting for deployment to be healthy..."
kubectl wait --for=condition=Available deployment/web-frontend \
  -n "${NAMESPACE}" --timeout=120s
kubectl get pods -n "${NAMESPACE}"
echo ""

echo "==> Step 3: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established."
echo ""

echo "==> Step 4: Injecting deny-all NetworkPolicy..."
bash "${SCRIPT_DIR}/inject-deny-all-netpol.sh"
echo ""

echo "==> Step 5: Waiting for KubeDeploymentReplicasMismatch alert (~3-4 min)..."
echo "  Health checks will fail -> pods become NotReady -> restarts begin."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (DeploymentUnavailable) -> Gateway -> SP -> AA (HAPI)"
echo "    LLM detects networkIsolated=true, diagnoses NetworkPolicy block"
echo "    WE removes the deny-all NetworkPolicy"
echo "    EM verifies pods become Ready"
echo ""
echo "==> Verify remediation:"
echo "    kubectl get networkpolicies -n ${NAMESPACE}"
echo "    kubectl get pods -n ${NAMESPACE}"
