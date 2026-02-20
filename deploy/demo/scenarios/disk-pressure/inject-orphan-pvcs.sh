#!/usr/bin/env bash
# Create orphaned PVCs that simulate leftover storage from completed batch jobs
set -euo pipefail

NAMESPACE="demo-disk"

echo "==> Creating orphaned PVCs in ${NAMESPACE}..."

for i in 1 2 3 4 5; do
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
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Mi
  storageClassName: standard
YAML
done

echo "==> Created 5 orphaned PVCs (batch-output-job-1 through 5)."
echo "    These simulate PVCs left behind by completed batch Jobs."
echo "    Watch: kubectl get pvc -n ${NAMESPACE}"
