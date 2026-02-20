#!/usr/bin/env bash
# Cleanup for Stuck Rollout Demo (#130)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Stuck Rollout demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-rollout --ignore-not-found

echo "==> Cleanup complete."
