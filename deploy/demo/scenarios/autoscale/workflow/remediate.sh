#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
PENDING=$(kubectl get pods -n "$TARGET_NAMESPACE" -l "app=$TARGET_APP" \
  --field-selector=status.phase=Pending -o name 2>/dev/null | wc -l | tr -d ' ')
echo "Pending pods: $PENDING"
if [ "$PENDING" -eq 0 ]; then
  echo "No pending pods found. Self-recovered or already remediated."
  exit 0
fi

echo "=== Phase 2: Action ==="
echo "Creating ScaleRequest for 1 additional node..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: scale-request
  namespace: kubernaut-system
  labels:
    kubernaut.ai/scale-request: "true"
data:
  requested_nodes: "1"
  status: "pending"
  cluster_name: "$CLUSTER_NAME"
  node_image: "kindest/node:v1.34.3"
EOF

echo "=== Phase 3: Verify ==="
echo "Waiting for provisioner to fulfill scale request..."
TIMEOUT=180
ELAPSED=0
while [ "$ELAPSED" -lt "$TIMEOUT" ]; do
  STATUS=$(kubectl get cm scale-request -n kubernaut-system \
    -o jsonpath='{.data.status}' 2>/dev/null || echo "pending")
  if [ "$STATUS" = "fulfilled" ]; then
    echo "Scale request fulfilled!"
    break
  fi
  sleep 5
  ELAPSED=$((ELAPSED + 5))
done

if [ "$STATUS" != "fulfilled" ]; then
  echo "ERROR: Scale request not fulfilled within ${TIMEOUT}s"
  exit 1
fi

echo "Waiting for pods to become Ready..."
kubectl wait --for=condition=Ready pod -l "app=$TARGET_APP" \
  -n "$TARGET_NAMESPACE" --timeout=120s

RUNNING=$(kubectl get pods -n "$TARGET_NAMESPACE" -l "app=$TARGET_APP" \
  --field-selector=status.phase=Running -o name | wc -l | tr -d ' ')
echo "Running pods: $RUNNING"
echo "=== SUCCESS: Cluster scaled, pods scheduled ==="
