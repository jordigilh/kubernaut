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

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

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
echo "    New RS has spec_replicas>0 but status_replicas=0 (FailedCreate)."
echo ""
echo "  Expected pipeline:"
echo "    Alert -> Gateway -> SP -> AIAnalysis (policy constraint, no workflow)"
echo "    -> ManualReviewRequired (escalation to human)"
