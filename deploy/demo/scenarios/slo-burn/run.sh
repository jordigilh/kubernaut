#!/usr/bin/env bash
# SLO Error Budget Burn Demo -- Automated Runner
# Scenario #128: Error budget burning -> proactive rollback to preserve SLO
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Kubernaut services deployed (HAPI with real LLM backend)
#   - Prometheus with nginx metrics exporter
#
# Usage: ./deploy/demo/scenarios/slo-burn/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-slo"

echo "============================================="
echo " SLO Error Budget Burn Demo (#128)"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace, API gateway, and traffic generator..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/configmap.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy blackbox-exporter and Prometheus alerting rules
echo "==> Step 2: Deploying blackbox-exporter, Probe, and SLO burn rate alerting rules..."
kubectl apply -f "${SCRIPT_DIR}/manifests/blackbox-exporter.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for initial deployments to be healthy
echo "==> Step 3: Waiting for deployments..."
kubectl wait --for=condition=Available deployment/api-gateway \
  -n "${NAMESPACE}" --timeout=120s
kubectl wait --for=condition=Available deployment/traffic-gen \
  -n "${NAMESPACE}" --timeout=120s
kubectl wait --for=condition=Available deployment/blackbox-exporter \
  -n "${NAMESPACE}" --timeout=60s
echo "  api-gateway, traffic-gen, and blackbox-exporter are healthy."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 4: Register workflow in DataStorage (placeholder)
echo "==> Step 4: Workflow registration..."
echo "  TODO: Build and push proactive-rollback-v1 OCI bundle, register via DataStorage API."
echo "  For now, ensure the workflow is pre-seeded in the catalog."
echo ""

# Step 5: Let healthy traffic establish baseline (~30s)
echo "==> Step 5: Establishing healthy traffic baseline (30s)..."
sleep 30
echo "  Baseline established. Error rate should be ~0%."
echo ""

# Step 6: Inject bad config
echo "==> Step 6: Injecting bad deployment (500 errors on /api/)..."
bash "${SCRIPT_DIR}/inject-bad-config.sh"
echo ""

# Step 7: Wait for alert
echo "==> Step 7: Waiting for SLO burn rate alert to fire (~5 min)..."
echo "  Check Prometheus: http://localhost:9190/alerts"
echo "  The KubernautSLOBudgetBurning alert should appear within 5 minutes."
echo ""

# Step 8: Monitor
echo "==> Step 8: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (SLOBudgetBurning) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM correlates error spike with recent deploy -> selects rollback"
echo "    WE rolls back deployment -> EM verifies error rate within SLO"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl rollout history deployment/api-gateway -n ${NAMESPACE}"
echo "    # Verify error rate drops (check Prometheus)"
