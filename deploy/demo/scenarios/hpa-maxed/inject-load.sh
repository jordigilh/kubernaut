#!/usr/bin/env bash
# Generate CPU load to push HPA to maxReplicas
# Uses a busybox pod to send concurrent requests to the api-frontend service
set -euo pipefail

NAMESPACE="demo-hpa"

echo "==> Starting CPU load generator..."

kubectl run -n "${NAMESPACE}" load-generator --rm -i --restart=Never \
  --image=busybox:1.36 -- sh -c \
  'echo "Generating load..."; i=0; while [ $i -lt 500000 ]; do wget -q -O- http://api-frontend.demo-hpa.svc/ > /dev/null 2>&1 & i=$((i+1)); if [ $((i % 20)) -eq 0 ]; then wait; fi; done; echo "Load complete"' &

echo "==> Load generator running in background."
echo "    Watch HPA: kubectl get hpa -n ${NAMESPACE} -w"
echo "    Once currentReplicas == maxReplicas (3), the alert will fire after 2 min."
