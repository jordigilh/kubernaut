#!/usr/bin/env bash
# Cleanup for SLO Error Budget Burn Demo (#128)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up SLO Error Budget Burn demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-slo --ignore-not-found

echo "==> Cleanup complete."
