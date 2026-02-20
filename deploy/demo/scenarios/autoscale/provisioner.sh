#!/usr/bin/env bash
# Host-side provisioner agent for Scenario #126
# Simulates a cloud autoscaler (Karpenter/NAP/cluster-autoscaler) for Kind.
# Watches for ScaleRequest ConfigMap and provisions new nodes via Podman + kubeadm.
#
# This script runs OUTSIDE Kubernetes, started by run.sh.
set -euo pipefail

echo "[provisioner] Watching for ScaleRequest in kubernaut-system..."

while true; do
  STATUS=$(kubectl get cm scale-request -n kubernaut-system \
    -o jsonpath='{.data.status}' 2>/dev/null || echo "none")

  if [ "$STATUS" = "pending" ]; then
    CLUSTER=$(kubectl get cm scale-request -n kubernaut-system \
      -o jsonpath='{.data.cluster_name}')
    IMAGE=$(kubectl get cm scale-request -n kubernaut-system \
      -o jsonpath='{.data.node_image}')
    NEW_NODE="${CLUSTER}-worker-$(date +%s)"

    echo "[provisioner] Provisioning node: $NEW_NODE (image: $IMAGE)"

    podman run -d --privileged \
      --name "$NEW_NODE" \
      --hostname "$NEW_NODE" \
      --network kind \
      "$IMAGE"

    echo "[provisioner] Container created. Waiting for kubelet bootstrap..."
    sleep 5

    echo "[provisioner] Obtaining join command from control plane..."
    JOIN_CMD=$(podman exec "${CLUSTER}-control-plane" \
      kubeadm token create --print-join-command)

    echo "[provisioner] Joining node to cluster..."
    podman exec "$NEW_NODE" $JOIN_CMD

    echo "[provisioner] Waiting for node to become Ready..."
    kubectl wait --for=condition=Ready "node/$NEW_NODE" --timeout=120s

    echo "[provisioner] Labeling node as workload node..."
    kubectl label node "$NEW_NODE" kubernaut.ai/workload-node=true

    kubectl patch cm scale-request -n kubernaut-system \
      --type=merge -p '{"data":{"status":"fulfilled","node_name":"'"$NEW_NODE"'"}}'

    echo "[provisioner] Node $NEW_NODE provisioned and ready."
  fi

  sleep 3
done
