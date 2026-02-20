#!/usr/bin/env bash
# Cleanup for Disk Pressure / Orphaned PVC Demo (#121)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Disk Pressure demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-disk --ignore-not-found

echo "==> Cleanup complete."
