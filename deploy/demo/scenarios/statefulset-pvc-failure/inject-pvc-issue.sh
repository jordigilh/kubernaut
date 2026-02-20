#!/usr/bin/env bash
# Inject PVC failure by deleting the PV backing a StatefulSet pod's PVC
set -euo pipefail

NAMESPACE="demo-statefulset"
TARGET_POD="kv-store-2"
TARGET_PVC="data-kv-store-2"

echo "==> Injecting PVC failure for ${TARGET_POD}..."

PV_NAME=$(kubectl get pvc "${TARGET_PVC}" -n "${NAMESPACE}" \
  -o jsonpath='{.spec.volumeName}')
echo "  PVC ${TARGET_PVC} is bound to PV ${PV_NAME}"

echo "  Deleting pod ${TARGET_POD} to force re-mount..."
kubectl delete pod "${TARGET_POD}" -n "${NAMESPACE}" --grace-period=0 --force 2>/dev/null || true

echo "  Deleting PVC ${TARGET_PVC}..."
kubectl delete pvc "${TARGET_PVC}" -n "${NAMESPACE}" --force 2>/dev/null || true

echo "  Deleting PV ${PV_NAME}..."
kubectl delete pv "${PV_NAME}" --force 2>/dev/null || true

echo "==> PVC ${TARGET_PVC} deleted. Pod ${TARGET_POD} will be stuck in Pending."
echo "==> Watch: kubectl get pods -n ${NAMESPACE} -w"
