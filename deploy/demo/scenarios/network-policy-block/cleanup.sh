#!/usr/bin/env bash
# Cleanup for NetworkPolicy Traffic Block Demo (#138)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up NetworkPolicy Traffic Block demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/deny-all-netpol.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/networkpolicy-allow.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/deployment.yaml" --ignore-not-found
kubectl delete namespace demo-netpol --ignore-not-found

echo "==> Cleanup complete."
