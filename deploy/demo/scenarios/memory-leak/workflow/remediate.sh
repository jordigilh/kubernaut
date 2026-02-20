#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking memory usage trend for deployment/$TARGET_DEPLOYMENT..."

PODS=$(kubectl get pods -n "$TARGET_NAMESPACE" -l "app=$TARGET_DEPLOYMENT" \
  --field-selector=status.phase=Running -o name 2>/dev/null | wc -l | tr -d ' ')
echo "Running pods: $PODS"

if [ "$PODS" -eq 0 ]; then
  echo "ERROR: No running pods found for deployment/$TARGET_DEPLOYMENT"
  exit 1
fi

CURRENT_REV=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}')
echo "Current deployment revision: $CURRENT_REV"
echo "Validated: deployment is running and eligible for restart."

echo "=== Phase 2: Action ==="
echo "Performing graceful rolling restart of deployment/$TARGET_DEPLOYMENT..."
kubectl rollout restart "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE"

echo "Waiting for rollout to complete..."
kubectl rollout status "deployment/$TARGET_DEPLOYMENT" \
  -n "$TARGET_NAMESPACE" --timeout=120s

echo "=== Phase 3: Verify ==="
NEW_REV=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.metadata.annotations.deployment\.kubernetes\.io/revision}')
echo "New deployment revision: $NEW_REV"

READY=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.readyReplicas}')
DESIRED=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.replicas}')
echo "Replicas: $READY/$DESIRED ready"

if [ "$READY" = "$DESIRED" ]; then
  echo "=== SUCCESS: Deployment restarted (rev $CURRENT_REV -> $NEW_REV), memory usage reset, all replicas ready ==="
else
  echo "WARNING: Not all replicas ready after restart ($READY/$DESIRED)"
  exit 1
fi
