#!/usr/bin/env bash
# Add a NoSchedule taint to the designated taint-target worker node.
# Only one worker is tainted so that WFE remediation jobs can still schedule
# on the other untainted worker.
set -euo pipefail

TARGET_NODE=$(kubectl get nodes -l kubernaut.ai/demo-taint-target=true -o name | head -1)

if [ -z "$TARGET_NODE" ]; then
  echo "ERROR: No worker node with label kubernaut.ai/demo-taint-target=true found."
  echo "Label a worker first: kubectl label node <worker> kubernaut.ai/demo-taint-target=true"
  exit 1
fi

echo "==> Adding NoSchedule taint to ${TARGET_NODE}..."
kubectl taint nodes "${TARGET_NODE}" maintenance=scheduled:NoSchedule --overwrite

echo "==> Taint applied to ${TARGET_NODE}. Pods pinned to this node will remain Pending."
echo "    Watch: kubectl get pods -n demo-taint -w"
