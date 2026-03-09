#!/usr/bin/env bash
# Resource Quota Exhaustion Demo -- Automated Runner
# Scenario #171: ResourceQuota prevents pod creation -> LLM escalates to human review
#
# No workflow is seeded -- the LLM should recognize this as a policy constraint
# and escalate with needs_human_review: true (ManualReviewRequired).
#
# The alert uses ReplicaSet-level metrics (spec vs status replicas) because
# quota-rejected pods are never created (FailedCreate at admission, never
# reach Pending state).
#
# Prerequisites:
#   - Kind cluster (kubernaut-demo) with platform installed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/resource-quota-exhaustion/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-quota"

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
echo " Resource Quota Exhaustion Demo (#171)"
echo " Policy Constraint -> ManualReviewRequired"
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

echo "==> Step 5: Fault injected. Waiting for KubeResourceQuotaExhausted alert (~1-2 min)."

# Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
