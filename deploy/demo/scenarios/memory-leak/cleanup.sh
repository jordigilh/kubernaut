#!/usr/bin/env bash
# Cleanup for Proactive Memory Exhaustion Demo (#129)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Memory Leak demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-memory-leak --ignore-not-found

echo "==> Cleanup complete."
