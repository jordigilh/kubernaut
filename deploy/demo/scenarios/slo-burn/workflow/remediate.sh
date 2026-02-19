#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
CURRENT_REV=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}')
echo "Current deployment revision: $CURRENT_REV"

if [ -z "$CURRENT_REV" ] || [ "$CURRENT_REV" -le 1 ] 2>/dev/null; then
  echo "ERROR: No previous revision to roll back to (revision: ${CURRENT_REV:-unknown})"
  exit 1
fi

CM_REF=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.volumes[0].configMap.name}')
echo "Current ConfigMap reference: $CM_REF"
echo "Deployment has rollback target. Proceeding."

echo "=== Phase 2: Action ==="
echo "Rolling back deployment/$TARGET_DEPLOYMENT to previous revision..."
kubectl rollout undo "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE"

echo "Waiting for rollout to complete..."
kubectl rollout status "deployment/$TARGET_DEPLOYMENT" \
  -n "$TARGET_NAMESPACE" --timeout=120s

echo "=== Phase 3: Verify ==="
NEW_REV=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}')
echo "New deployment revision: $NEW_REV"

NEW_CM_REF=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.volumes[0].configMap.name}')
echo "ConfigMap reference after rollback: $NEW_CM_REF"

READY=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.readyReplicas}')
DESIRED=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.replicas}')
echo "Replicas: $READY/$DESIRED ready"

if [ "$READY" = "$DESIRED" ]; then
  echo "=== SUCCESS: Deployment rolled back (rev $CURRENT_REV -> $NEW_REV), ConfigMap reverted ($CM_REF -> $NEW_CM_REF), all replicas ready ==="
else
  echo "WARNING: Not all replicas ready after rollback ($READY/$DESIRED)"
  exit 1
fi
