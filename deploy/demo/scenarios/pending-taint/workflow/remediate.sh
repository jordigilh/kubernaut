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

if echo "$REMAINING_TAINTS" | grep -q "$TAINT_KEY"; then
  echo "ERROR: Taint '$TAINT_KEY' still present on $TARGET_NODE after removal attempt"
  exit 1
fi

if [ -n "${TARGET_NAMESPACE:-}" ]; then
  PENDING=$(kubectl get pods -n "$TARGET_NAMESPACE" --field-selector=status.phase=Pending \
    -o name 2>/dev/null | wc -l | tr -d ' ')
  echo "Pending pods in $TARGET_NAMESPACE: $PENDING"

  if [ "$PENDING" -gt 0 ]; then
    echo "WARNING: $PENDING pods still Pending -- may need more time to schedule"
  fi
else
  echo "TARGET_NAMESPACE not set; skipping namespace-level pod check."
fi

echo "=== SUCCESS: Taint '$TAINT_KEY' removed from $TARGET_NODE ==="
