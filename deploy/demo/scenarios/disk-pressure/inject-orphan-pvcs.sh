#!/usr/bin/env bash
# Create orphaned PVCs that simulate leftover storage from completed batch jobs.
#
# StorageClass "standard" (local-path) uses WaitForFirstConsumer, so PVCs stay
# Pending until a pod mounts them. We create short-lived pods that bind each PVC,
# then delete the pods — leaving behind Bound PVCs that no running pod uses.
set -euo pipefail

NAMESPACE="demo-disk"
PVC_COUNT=5

echo "==> Creating ${PVC_COUNT} PVCs and temporary binder pods in ${NAMESPACE}..."

for i in $(seq 1 "$PVC_COUNT"); do
  kubectl apply -f - <<YAML
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: batch-output-job-${i}
  namespace: ${NAMESPACE}
  labels:
    app: batch-job
    batch-run: "completed"
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 100Mi
  storageClassName: standard
---
apiVersion: v1
kind: Pod
metadata:
  name: binder-${i}
  namespace: ${NAMESPACE}
  labels:
    role: pvc-binder
spec:
  restartPolicy: Never
  containers:
  - name: touch
    image: busybox:1.36
    command: ["sh", "-c", "echo bound > /data/marker && sleep 3600"]
    volumeMounts:
    - name: vol
      mountPath: /data
  volumes:
  - name: vol
    persistentVolumeClaim:
      claimName: batch-output-job-${i}
YAML
done

echo "==> Waiting for all binder pods to be Running (PVCs binding)..."
for i in $(seq 1 "$PVC_COUNT"); do
  kubectl wait --for=condition=Ready pod/binder-${i} -n "${NAMESPACE}" --timeout=120s
done

echo "==> PVCs are Bound. Deleting binder pods to orphan the PVCs..."
for i in $(seq 1 "$PVC_COUNT"); do
  kubectl delete pod binder-${i} -n "${NAMESPACE}" --wait=false
done

kubectl wait --for=delete pod -l role=pvc-binder -n "${NAMESPACE}" --timeout=60s 2>/dev/null || true

echo "==> Created ${PVC_COUNT} orphaned Bound PVCs (batch-output-job-1 through ${PVC_COUNT})."
echo "    These simulate PVCs left behind by completed batch Jobs."
echo "    Verify: kubectl get pvc -n ${NAMESPACE}"
