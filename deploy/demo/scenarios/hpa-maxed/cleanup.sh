#!/usr/bin/env bash
# Cleanup for HPA Maxed Out Demo (#123)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up HPA Maxed Out demo..."

# Kill any lingering load generator
kubectl delete pod load-generator -n demo-hpa --ignore-not-found

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-hpa --ignore-not-found

echo "==> Cleanup complete."
