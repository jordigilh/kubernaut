#!/usr/bin/env bash
# Proactive Memory Exhaustion Demo -- Automated Runner
# Scenario #129: predict_linear detects OOM trend -> graceful restart
#
# The 'leaker' sidecar allocates ~1MB every 5 seconds (~12MB/min) via a
# memory-backed emptyDir. predict_linear projects OOM within 30 minutes,
# triggering ContainerMemoryExhaustionPredicted. The LLM selects
# GracefulRestart (rolling restart) to reset memory before OOM.
#
# Prerequisites:
#   - Kind cluster (kubernaut-demo) with platform installed
#   - Prometheus with kube-state-metrics and cAdvisor scraping
#
# Usage: ./deploy/demo/scenarios/memory-leak/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-memory-leak"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "memory-leak"

echo "============================================="
echo " Proactive Memory Exhaustion Demo (#129)"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace and leaky-app deployment..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying predict_linear alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for deployment to be healthy
echo "==> Step 3: Waiting for leaky-app to be ready..."
kubectl wait --for=condition=Available deployment/leaky-app \
  -n "${NAMESPACE}" --timeout=120s
echo "  leaky-app is running (2 pods with leaker sidecar)."
kubectl get pods -n "${NAMESPACE}"
echo ""

echo "==> Step 4: Memory leak building (~12MB/min per pod)."
echo "    predict_linear will fire once it projects OOM within 30 minutes,"
echo "    typically after 5-7 minutes of trend data."
echo ""
echo "  Expected pipeline:"
echo "    Alert (ContainerMemoryExhaustionPredicted) -> Gateway -> SP -> AIAnalysis"
echo "    -> LLM selects GracefulRestart -> WFE (rolling restart) -> EA (memory reset)"
