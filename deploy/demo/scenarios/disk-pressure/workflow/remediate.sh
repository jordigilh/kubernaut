#!/bin/sh
set -e

SELECTOR="${LABEL_SELECTOR:-batch-run=completed}"

echo "=== Phase 1: Validate ==="
echo "Scanning for orphaned PVCs in namespace $TARGET_NAMESPACE..."
echo "Label selector: $SELECTOR"

PVCS=$(kubectl get pvc -n "$TARGET_NAMESPACE" -l "$SELECTOR" -o name 2>/dev/null)
PVC_COUNT=$(echo "$PVCS" | grep -c "persistentvolumeclaim" || echo "0")
echo "Found $PVC_COUNT PVCs matching selector."

if [ "$PVC_COUNT" -eq 0 ]; then
  echo "No orphaned PVCs found. Nothing to clean up."
  exit 0
fi

echo "PVCs to delete:"
echo "$PVCS"

# Verify none are mounted by running pods
MOUNTED=0
for pvc in $PVCS; do
  PVC_NAME=$(echo "$pvc" | sed 's|persistentvolumeclaim/||')
  POD_USING=$(kubectl get pods -n "$TARGET_NAMESPACE" \
    -o jsonpath="{.items[?(@.spec.volumes[*].persistentVolumeClaim.claimName=='$PVC_NAME')].metadata.name}" 2>/dev/null || echo "")
  if [ -n "$POD_USING" ]; then
    echo "WARNING: PVC $PVC_NAME is mounted by pod $POD_USING -- skipping"
    MOUNTED=$((MOUNTED + 1))
  fi
done

DELETABLE=$((PVC_COUNT - MOUNTED))
echo "Validated: $DELETABLE PVCs are safe to delete ($MOUNTED in use, skipped)."

echo "=== Phase 2: Action ==="
echo "Deleting $DELETABLE orphaned PVCs..."
DELETED=0
for pvc in $PVCS; do
  PVC_NAME=$(echo "$pvc" | sed 's|persistentvolumeclaim/||')
  POD_USING=$(kubectl get pods -n "$TARGET_NAMESPACE" \
    -o jsonpath="{.items[?(@.spec.volumes[*].persistentVolumeClaim.claimName=='$PVC_NAME')].metadata.name}" 2>/dev/null || echo "")
  if [ -z "$POD_USING" ]; then
    kubectl delete pvc "$PVC_NAME" -n "$TARGET_NAMESPACE"
    DELETED=$((DELETED + 1))
    echo "  Deleted: $PVC_NAME"
  fi
done

echo "=== Phase 3: Verify ==="
REMAINING=$(kubectl get pvc -n "$TARGET_NAMESPACE" -l "$SELECTOR" -o name 2>/dev/null | grep -c "persistentvolumeclaim" || echo "0")
echo "Remaining PVCs with selector '$SELECTOR': $REMAINING"

echo "=== SUCCESS: Deleted $DELETED orphaned PVCs, $REMAINING remaining ==="
