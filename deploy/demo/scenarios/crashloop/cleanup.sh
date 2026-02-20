#!/usr/bin/env bash
# Cleanup for CrashLoopBackOff Demo (#120)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up CrashLoopBackOff demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-crashloop --ignore-not-found

echo "==> Cleanup complete."
