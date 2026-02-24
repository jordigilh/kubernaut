#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking deployment/$TARGET_DEPLOYMENT in namespace $TARGET_NAMESPACE..."

# Check deployment exists
if ! kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" >/dev/null 2>&1; then
  echo "ERROR: Deployment $TARGET_DEPLOYMENT not found in namespace $TARGET_NAMESPACE"
  exit 1
fi

# Get current memory limit from first container
CURRENT_MEM=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')

if [ -z "$CURRENT_MEM" ]; then
  echo "ERROR: No memory limit set on first container of deployment/$TARGET_DEPLOYMENT"
  exit 1
fi

# Parse memory value (handle Mi, Gi suffixes)
FACTOR=${MEMORY_INCREASE_FACTOR:-2}
case "$CURRENT_MEM" in
  *Mi)
    NUM=$(echo "$CURRENT_MEM" | sed 's/Mi$//')
    SUFFIX="Mi"
    ;;
  *Gi)
    NUM=$(echo "$CURRENT_MEM" | sed 's/Gi$//')
    SUFFIX="Gi"
    ;;
  *)
    NUM=$(echo "$CURRENT_MEM" | sed 's/[^0-9]//g')
    SUFFIX="Mi"
    ;;
esac

NEW_NUM=$((NUM * FACTOR))
NEW_LIMIT="${NEW_NUM}${SUFFIX}"

echo "Current memory limit: $CURRENT_MEM"
echo "Target memory limit:  $NEW_LIMIT (factor: $FACTOR)"
echo "Validated: deployment exists and memory limit is set."

echo "=== Phase 2: Action ==="
echo "Patching deployment/$TARGET_DEPLOYMENT memory limits and requests to $NEW_LIMIT..."

kubectl patch "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" --type=json -p \
  "[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/resources/limits/memory\",\"value\":\"${NEW_LIMIT}\"},{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/resources/requests/memory\",\"value\":\"${NEW_LIMIT}\"}]"

echo "Waiting for rollout to complete..."
kubectl rollout status "deployment/$TARGET_DEPLOYMENT" \
  -n "$TARGET_NAMESPACE" --timeout=120s

echo "=== Phase 3: Verify ==="
VERIFIED_LIMIT=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')
READY=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.readyReplicas}')
DESIRED=$(kubectl get "deployment/$TARGET_DEPLOYMENT" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.replicas}')

echo "New memory limit: $VERIFIED_LIMIT"
echo "Replicas: $READY/$DESIRED ready"

if [ "$VERIFIED_LIMIT" = "$NEW_LIMIT" ] && [ "$READY" = "$DESIRED" ]; then
  echo "=== SUCCESS: Memory limits increased ($CURRENT_MEM -> $NEW_LIMIT), all replicas ready ==="
else
  echo "ERROR: Verification failed (limit=$VERIFIED_LIMIT expected=$NEW_LIMIT, ready=$READY/$DESIRED)"
  exit 1
fi
