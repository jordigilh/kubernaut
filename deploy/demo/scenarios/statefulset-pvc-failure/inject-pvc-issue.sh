#!/usr/bin/env bash
# Inject PVC failure by replacing a StatefulSet pod's PVC with a broken one
#
# On Kind (local-path provisioner), simply deleting a PVC is insufficient because
# the StatefulSet controller recreates it faster than we can intervene. To simulate
# a real PVC failure without race conditions:
#   1. Scale the StatefulSet to 2 replicas (removes kv-store-2 and releases its PVC)
#   2. Delete PVC data-kv-store-2 (now unprotected since no pod uses it)
#   3. Create a replacement PVC with a non-existent StorageClass
#   4. Scale back to 3 replicas -> kv-store-2 sees the PVC but it can't bind -> stuck
#
# The remediate.sh workflow detects the non-Bound PVC, deletes it, recreates it
# with the correct StorageClass, and the pod recovers.
set -euo pipefail

NAMESPACE="demo-statefulset"
STATEFULSET="kv-store"
TARGET_PVC="data-kv-store-2"

echo "==> Injecting PVC failure for ${STATEFULSET}..."

STORAGE_SIZE=$(kubectl get pvc "${TARGET_PVC}" -n "${NAMESPACE}" \
  -o jsonpath='{.spec.resources.requests.storage}')
echo "  PVC ${TARGET_PVC} size: ${STORAGE_SIZE}"

echo "  Scaling StatefulSet to 2 replicas (removing kv-store-2)..."
kubectl scale statefulset "${STATEFULSET}" -n "${NAMESPACE}" --replicas=2
kubectl rollout status statefulset/"${STATEFULSET}" -n "${NAMESPACE}" --timeout=60s

echo "  Deleting PVC ${TARGET_PVC}..."
kubectl delete pvc "${TARGET_PVC}" -n "${NAMESPACE}" --wait=true --timeout=30s

echo "  Creating broken PVC (non-existent StorageClass)..."
kubectl create -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${TARGET_PVC}
  namespace: ${NAMESPACE}
  labels:
    app: ${STATEFULSET}
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: broken-storage-class
  resources:
    requests:
      storage: ${STORAGE_SIZE}
EOF

echo "  Scaling StatefulSet back to 3 replicas..."
kubectl scale statefulset "${STATEFULSET}" -n "${NAMESPACE}" --replicas=3

sleep 5
echo ""
echo "  Current state:"
kubectl get pods -n "${NAMESPACE}"
echo ""
kubectl get pvc -n "${NAMESPACE}"
echo ""
echo "==> PVC ${TARGET_PVC} replaced with broken PVC (storageClass: broken-storage-class)."
echo "==> kv-store-2 will be Pending (PVC cannot bind)."
echo "==> Alert will fire after ~3 min."
echo "==> Watch: kubectl get pods,pvc -n ${NAMESPACE} -w"
