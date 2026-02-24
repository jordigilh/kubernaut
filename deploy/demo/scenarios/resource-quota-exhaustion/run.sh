#!/usr/bin/env bash
# Resource Quota Exhaustion Demo -- Automated Runner
# Scenario: ResourceQuota prevents pod scheduling -> LLM escalates to human review
#
# No workflow is seeded -- the LLM should recognize this as a policy issue
# and escalate with needs_human_review: true
#
# Usage: ./deploy/demo/scenarios/resource-quota-exhaustion/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-quota"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

echo "============================================="
echo " Resource Quota Exhaustion Demo (#171)"
echo "============================================="
echo ""

# Step 1: Deploy namespace, resourcequota, deployment, prometheus-rule
echo "==> Step 1: Deploying namespace, resourcequota, deployment, and prometheus-rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/resourcequota.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 2: Wait for healthy deployment
echo "==> Step 2: Waiting for api-server to be healthy..."
kubectl wait --for=condition=Available deployment/api-server \
  -n "${NAMESPACE}" --timeout=120s
echo "  api-server is running within quota."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 3: Establish baseline
echo "==> Step 3: Establishing baseline (20s)..."
sleep 20
echo "  Baseline established."
echo ""

# Step 4: Exhaust quota
echo "==> Step 4: Exhausting ResourceQuota..."
bash "${SCRIPT_DIR}/exhaust-quota.sh"
echo ""

# Step 5: Print monitoring instructions
echo "==> Step 5: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (KubePodPendingQuotaExhausted) -> Gateway -> SP -> RR -> AA (NeedsHumanReview)"
echo "    LLM distinguishes policy constraints from infrastructure failures"
echo "    -> ManualReviewNotification (escalation to human)"
echo ""
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo "  The KubePodPendingQuotaExhausted alert fires after pods are Pending for 1m."
echo ""
