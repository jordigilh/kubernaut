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

    VOL_NAME="kind-${NEW_NODE}-var"
    podman volume create "$VOL_NAME" >/dev/null 2>&1

    podman run -d --privileged \
      --security-opt unmask=all \
      --name "$NEW_NODE" \
      --hostname "$NEW_NODE" \
      --network kind \
      --tmpfs /run:rprivate,nosuid,nodev,tmpcopyup \
      --tmpfs /tmp:rprivate,nosuid,nodev,tmpcopyup \
      -v "${VOL_NAME}:/var" \
      -v /lib/modules:/lib/modules:ro \
      -v /dev/mapper:/dev/mapper \
      --entrypoint /usr/local/bin/entrypoint \
      "$IMAGE" \
      /sbin/init

    echo "[provisioner] Container created. Waiting for kubelet bootstrap..."
    sleep 10

    echo "[provisioner] Obtaining join command from control plane..."
    JOIN_CMD=$(podman exec "${CLUSTER}-control-plane" \
      kubeadm token create --print-join-command)

    echo "[provisioner] Joining node to cluster..."
    podman exec "$NEW_NODE" $JOIN_CMD --ignore-preflight-errors=SystemVerification

    echo "[provisioner] Waiting for node to register..."
    for i in $(seq 1 30); do
      kubectl get "node/$NEW_NODE" >/dev/null 2>&1 && break
      sleep 2
    done

    echo "[provisioner] Waiting for node to become Ready..."
    kubectl wait --for=condition=Ready "node/$NEW_NODE" --timeout=180s

    echo "[provisioner] Labeling node as workload node..."
    kubectl label node "$NEW_NODE" kubernaut.ai/managed=true

    kubectl patch cm scale-request -n kubernaut-system \
      --type=merge -p '{"data":{"status":"fulfilled","node_name":"'"$NEW_NODE"'"}}'

    echo "[provisioner] Node $NEW_NODE provisioned and ready."
  fi

  sleep 3
done
