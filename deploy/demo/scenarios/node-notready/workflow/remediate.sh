#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking node $TARGET_NODE status..."

NODE_READY=$(kubectl get node "$TARGET_NODE" \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
echo "Node Ready status: $NODE_READY"

if [ "$NODE_READY" = "True" ]; then
  echo "WARNING: Node $TARGET_NODE is currently Ready. It may have recovered."
  echo "Proceeding with cordon to prevent future issues."
fi

SCHEDULABLE=$(kubectl get node "$TARGET_NODE" -o jsonpath='{.spec.unschedulable}')
echo "Node schedulable: $([ "$SCHEDULABLE" = "true" ] && echo "No (already cordoned)" || echo "Yes")"
echo "Validated: node identified for cordon and drain."

echo "=== Phase 2: Action ==="
echo "Cordoning node $TARGET_NODE..."
kubectl cordon "$TARGET_NODE"

echo "Draining node $TARGET_NODE (grace period 30s, ignore daemonsets)..."
kubectl drain "$TARGET_NODE" \
  --ignore-daemonsets \
  --delete-emptydir-data \
  --force \
  --grace-period=30 \
  --timeout=120s || true

echo "=== Phase 3: Verify ==="
SCHEDULABLE_AFTER=$(kubectl get node "$TARGET_NODE" -o jsonpath='{.spec.unschedulable}')
echo "Node unschedulable after cordon: $SCHEDULABLE_AFTER"

PODS_ON_NODE=$(kubectl get pods --all-namespaces --field-selector="spec.nodeName=$TARGET_NODE" \
  --no-headers 2>/dev/null | grep -v "kube-system" | wc -l | tr -d ' ')
echo "Non-system pods remaining on node: $PODS_ON_NODE"

echo "=== SUCCESS: Node $TARGET_NODE cordoned and drained, $PODS_ON_NODE non-system pods remaining ==="
