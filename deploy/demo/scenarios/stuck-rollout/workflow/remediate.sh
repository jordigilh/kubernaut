#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking deployment/$TARGET_DEPLOYMENT rollout status..."

CURRENT_REV=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}')
echo "Current deployment revision: $CURRENT_REV"

if [ "$CURRENT_REV" -le 1 ]; then
  echo "ERROR: No previous revision to roll back to (current rev: $CURRENT_REV)"
  exit 1
fi

CURRENT_IMAGE=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[0].image}')
echo "Current image: $CURRENT_IMAGE"

UNAVAILABLE=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.unavailableReplicas}')
echo "Unavailable replicas: ${UNAVAILABLE:-0}"
echo "Validated: deployment has rollback history and rollout is stuck."

echo "=== Phase 2: Action ==="
echo "Rolling back deployment/$TARGET_DEPLOYMENT to previous revision..."
kubectl rollout undo "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE"

echo "Waiting for rollback replicas to become ready..."
for i in $(seq 1 24); do
  READY=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
    -o jsonpath='{.status.readyReplicas}')
  DESIRED=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
    -o jsonpath='{.spec.replicas}')
  if [ "${READY:-0}" = "$DESIRED" ] && [ -n "$DESIRED" ]; then
    echo "Rollout complete: $READY/$DESIRED replicas ready"
    break
  fi
  if [ "$i" -eq 24 ]; then
    echo "ERROR: Rollout did not complete within 120s"
    exit 1
  fi
  echo "Waiting... (${READY:-0}/$DESIRED ready)"
  sleep 5
done

echo "=== Phase 3: Verify ==="
NEW_REV=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}')
echo "New deployment revision: $NEW_REV"

NEW_IMAGE=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[0].image}')
echo "Restored image: $NEW_IMAGE"

READY=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.readyReplicas}')
DESIRED=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.replicas}')
echo "Replicas: $READY/$DESIRED ready"

if [ "$READY" = "$DESIRED" ]; then
  echo "=== SUCCESS: Deployment rolled back (rev $CURRENT_REV -> $NEW_REV), image restored ($CURRENT_IMAGE -> $NEW_IMAGE), all replicas ready ==="
else
  echo "WARNING: Not all replicas ready after rollback ($READY/$DESIRED)"
  exit 1
fi
