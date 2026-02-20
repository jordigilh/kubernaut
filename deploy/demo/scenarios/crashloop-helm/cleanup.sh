#!/usr/bin/env bash
# Cleanup for Helm CrashLoopBackOff Demo (#135)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Helm CrashLoopBackOff demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
helm uninstall demo-crashloop-helm -n demo-crashloop-helm 2>/dev/null || true
kubectl delete namespace demo-crashloop-helm --ignore-not-found

echo "==> Cleanup complete."
