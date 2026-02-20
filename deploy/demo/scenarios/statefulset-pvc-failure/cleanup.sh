#!/usr/bin/env bash
# Cleanup for StatefulSet PVC Failure Demo (#137)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up StatefulSet PVC Failure demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete statefulset kv-store -n demo-statefulset --cascade=foreground --ignore-not-found
kubectl delete pvc -l app=kv-store -n demo-statefulset --ignore-not-found
kubectl delete namespace demo-statefulset --ignore-not-found

echo "==> Cleanup complete."
