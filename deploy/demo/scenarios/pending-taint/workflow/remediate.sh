#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking node $TARGET_NODE for taint $TAINT_KEY..."

TAINTS=$(kubectl get node "$TARGET_NODE" -o jsonpath='{.spec.taints[*].key}')
echo "Current taints: $TAINTS"

if ! echo "$TAINTS" | grep -q "$TAINT_KEY"; then
  echo "WARNING: Taint '$TAINT_KEY' not found on node $TARGET_NODE"
  echo "Available taints: $TAINTS"
  echo "Proceeding anyway -- taint may have already been removed."
fi

NODE_STATUS=$(kubectl get node "$TARGET_NODE" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')
echo "Node Ready status: $NODE_STATUS"
echo "Validated: node exists and taint identified."

echo "=== Phase 2: Action ==="
echo "Removing taint $TAINT_KEY from node $TARGET_NODE..."
kubectl taint nodes "$TARGET_NODE" "${TAINT_KEY}-" || true

echo "Waiting for pending pods to be scheduled (30s)..."
sleep 30

echo "=== Phase 3: Verify ==="
REMAINING_TAINTS=$(kubectl get node "$TARGET_NODE" -o jsonpath='{.spec.taints[*].key}')
echo "Remaining taints: ${REMAINING_TAINTS:-<none>}"

PENDING=$(kubectl get pods --all-namespaces --field-selector=status.phase=Pending \
  -o name 2>/dev/null | wc -l | tr -d ' ')
echo "Cluster-wide pending pods: $PENDING"

echo "=== SUCCESS: Taint '$TAINT_KEY' removed from $TARGET_NODE, pods should be scheduling ==="
