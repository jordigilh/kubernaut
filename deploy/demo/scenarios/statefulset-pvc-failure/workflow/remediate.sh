#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking StatefulSet ${TARGET_STATEFULSET} in ${TARGET_NAMESPACE}..."

READY=$(kubectl get statefulset "${TARGET_STATEFULSET}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
DESIRED=$(kubectl get statefulset "${TARGET_STATEFULSET}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.spec.replicas}')
echo "Replicas: ${READY}/${DESIRED} ready"

if [ "${READY}" = "${DESIRED}" ]; then
  echo "All replicas are ready. No action needed."
  exit 0
fi

STORAGE_CLASS=$(kubectl get statefulset "${TARGET_STATEFULSET}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.spec.volumeClaimTemplates[0].spec.storageClassName}' 2>/dev/null || echo "")
STORAGE_SIZE=$(kubectl get statefulset "${TARGET_STATEFULSET}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.spec.volumeClaimTemplates[0].spec.resources.requests.storage}')
VCT_NAME=$(kubectl get statefulset "${TARGET_STATEFULSET}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.spec.volumeClaimTemplates[0].metadata.name}')
echo "VolumeClaimTemplate: name=${VCT_NAME}, size=${STORAGE_SIZE}, storageClass=${STORAGE_CLASS:-default}"

PENDING_PODS=$(kubectl get pods -n "${TARGET_NAMESPACE}" -l "app=${TARGET_STATEFULSET}" \
  --field-selector=status.phase=Pending -o name 2>/dev/null || echo "")

if [ -z "${PENDING_PODS}" ]; then
  echo "No Pending pods found. Issue may be different than expected."
  PENDING_PODS=$(kubectl get pods -n "${TARGET_NAMESPACE}" -l "app=${TARGET_STATEFULSET}" \
    --field-selector=status.phase!=Running -o name 2>/dev/null || echo "none")
fi
echo "Stuck pods: ${PENDING_PODS}"

if [ -n "${TARGET_PVC:-}" ]; then
  MISSING_PVC="${TARGET_PVC}"
else
  MISSING_PVC=""
  for i in $(seq 0 $((DESIRED - 1))); do
    PVC_NAME="${VCT_NAME}-${TARGET_STATEFULSET}-${i}"
    if ! kubectl get pvc "${PVC_NAME}" -n "${TARGET_NAMESPACE}" &>/dev/null; then
      MISSING_PVC="${PVC_NAME}"
      break
    fi
  done
fi

if [ -z "${MISSING_PVC:-}" ]; then
  echo "ERROR: Could not identify missing PVC"
  exit 1
fi
echo "Missing PVC: ${MISSING_PVC}"
echo "Validated: StatefulSet has missing PVC causing stuck pod."

echo "=== Phase 2: Action ==="
echo "Recreating PVC ${MISSING_PVC}..."

SC_SPEC=""
if [ -n "${STORAGE_CLASS}" ]; then
  SC_SPEC="storageClassName: ${STORAGE_CLASS}"
fi

kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${MISSING_PVC}
  namespace: ${TARGET_NAMESPACE}
  labels:
    app: ${TARGET_STATEFULSET}
spec:
  accessModes: ["ReadWriteOnce"]
  ${SC_SPEC}
  resources:
    requests:
      storage: ${STORAGE_SIZE}
EOF

echo "PVC recreated. Deleting stuck pod to trigger reschedule..."
POD_INDEX=$(echo "${MISSING_PVC}" | grep -o '[0-9]*$')
STUCK_POD="${TARGET_STATEFULSET}-${POD_INDEX}"
kubectl delete pod "${STUCK_POD}" -n "${TARGET_NAMESPACE}" --ignore-not-found --grace-period=0

echo "Waiting for StatefulSet to reconcile (30s)..."
sleep 30

echo "=== Phase 3: Verify ==="
NEW_READY=$(kubectl get statefulset "${TARGET_STATEFULSET}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
echo "Replicas: ${NEW_READY}/${DESIRED} ready"

PVC_STATUS=$(kubectl get pvc "${MISSING_PVC}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
echo "PVC ${MISSING_PVC} status: ${PVC_STATUS}"

if [ "${NEW_READY}" = "${DESIRED}" ]; then
  echo "=== SUCCESS: PVC recreated, all ${DESIRED} StatefulSet replicas ready ==="
else
  echo "WARNING: Not all replicas ready yet (${NEW_READY}/${DESIRED}). May need more time."
  exit 1
fi
