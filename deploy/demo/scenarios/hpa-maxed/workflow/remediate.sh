#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking HPA $TARGET_HPA status..."

CURRENT_MAX=$(kubectl get hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.maxReplicas}')
CURRENT_REPLICAS=$(kubectl get hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.currentReplicas}')
echo "Current maxReplicas: $CURRENT_MAX"
echo "Current replicas: $CURRENT_REPLICAS"

if [ "$CURRENT_REPLICAS" -lt "$CURRENT_MAX" ]; then
  echo "WARNING: HPA is not at ceiling ($CURRENT_REPLICAS < $CURRENT_MAX)"
  echo "Proceeding anyway as the alert indicated maxed-out state."
fi

# Calculate new max: use parameter or default to current + 2
if [ -n "${NEW_MAX_REPLICAS:-}" ] && [ "$NEW_MAX_REPLICAS" -gt "$CURRENT_MAX" ]; then
  TARGET_MAX="$NEW_MAX_REPLICAS"
else
  TARGET_MAX=$((CURRENT_MAX + 2))
fi

echo "Validated: will raise maxReplicas from $CURRENT_MAX to $TARGET_MAX"

echo "=== Phase 2: Action ==="
echo "Patching HPA $TARGET_HPA maxReplicas to $TARGET_MAX..."
kubectl patch hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  --type=merge -p "{\"spec\":{\"maxReplicas\":$TARGET_MAX}}"

echo "Waiting for HPA to scale (30s)..."
sleep 30

echo "=== Phase 3: Verify ==="
NEW_MAX=$(kubectl get hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.maxReplicas}')
NEW_REPLICAS=$(kubectl get hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.currentReplicas}')
DESIRED=$(kubectl get hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.desiredReplicas}')
echo "New maxReplicas: $NEW_MAX"
echo "Current replicas: $NEW_REPLICAS"
echo "Desired replicas: $DESIRED"

TARGET_DEPLOY=$(kubectl get hpa "$TARGET_HPA" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.scaleTargetRef.name}')
READY=$(kubectl get "deployment/$TARGET_DEPLOY" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.readyReplicas}')
echo "Deployment ready replicas: $READY"

if [ "$NEW_MAX" -gt "$CURRENT_MAX" ]; then
  echo "=== SUCCESS: HPA ceiling raised ($CURRENT_MAX -> $NEW_MAX), replicas: $NEW_REPLICAS/$NEW_MAX, deployment ready: $READY ==="
else
  echo "ERROR: HPA maxReplicas was not updated ($NEW_MAX)"
  exit 1
fi
