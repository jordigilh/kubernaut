#!/usr/bin/env bash
# Duplicate Alert Suppression Demo -- Automated Runner
# Scenario #170: Alert storm (5 pods crash) -> Gateway deduplicates to 1 RR
#
# Demonstrates Gateway-level deduplication via OwnerResolver:
# All 5 pods belong to the same Deployment, so they share one fingerprint.
# 5 alerts arrive, but only 1 RemediationRequest is created (OccurrenceCount=5).
#
# Prerequisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/duplicate-alert-suppression/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-alert-storm"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "crashloop"

echo "============================================="
echo " Duplicate Alert Suppression Demo (#170)"
echo " 5 Crashing Pods -> 1 RemediationRequest"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace, healthy config, and api-gateway (5 replicas)..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/configmap.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying CrashLoop detection alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for all 5 replicas to be healthy..."
kubectl wait --for=condition=Available deployment/api-gateway \
  -n "${NAMESPACE}" --timeout=120s
echo "  All 5 pods are running."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 4: Establish baseline
echo "==> Step 4: Establishing healthy baseline (20s)..."
sleep 20
echo ""

# Step 5: Inject bad config (all 5 pods crash)
echo "==> Step 5: Injecting invalid config (all 5 pods will CrashLoop)..."
bash "${SCRIPT_DIR}/inject-bad-config.sh"
echo ""

# Step 6: Wait for alerts
echo "==> Step 6: Waiting for CrashLoop alerts to fire (~2-3 min)..."
echo "  5 pods are crashing, but they all belong to Deployment/api-gateway."
echo "  The Gateway OwnerResolver maps each pod alert to the Deployment."
echo "  All 5 alerts share fingerprint: SHA256(demo-alert-storm:deployment:api-gateway)"
echo ""

# Step 7: Show deduplication
echo "==> Step 7: Monitoring deduplication status..."
echo "  Check periodically with: bash ${SCRIPT_DIR}/show-dedup-status.sh"
echo ""
echo "  Monitor pipeline:"
echo "    kubectl get rr,aa,we -n ${NAMESPACE} -w"
echo ""
echo "  Expected:"
echo "    - 1 RR created (NOT 5)"
echo "    - RR.status.deduplication.occurrenceCount increases to 5"
echo "    - Pipeline proceeds: AA -> RO -> WE (rollback) -> EM"
echo "    - All 5 pods recover after single rollback"
echo ""
echo "==> To verify:"
echo "    bash ${SCRIPT_DIR}/show-dedup-status.sh"
echo "    kubectl get pods -n ${NAMESPACE}"
