#!/usr/bin/env bash
# Cleanup for PDB Deadlock Demo (#124)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up PDB Deadlock demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-pdb --ignore-not-found

echo "==> Cleanup complete."
