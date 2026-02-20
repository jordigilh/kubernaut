#!/usr/bin/env bash
# Inject a non-existent image tag to cause a stuck rollout
set -euo pipefail

NAMESPACE="demo-rollout"

echo "==> Setting checkout-api to a non-existent image tag..."
kubectl set image deployment/checkout-api -n "${NAMESPACE}" \
  api=nginx:99.99.99-doesnotexist

echo "==> Bad image injected. The rollout will stall because:"
echo "    - New pods will have ImagePullBackOff"
echo "    - progressDeadlineSeconds (120s) will be exceeded"
echo "    - The Progressing condition will become False"
echo ""
echo "    Watch: kubectl rollout status deployment/checkout-api -n ${NAMESPACE}"
echo "    Watch: kubectl get pods -n ${NAMESPACE} -w"
