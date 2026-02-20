#!/usr/bin/env bash
# Inject restrictive Linkerd AuthorizationPolicy to block all traffic
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Injecting deny-all AuthorizationPolicy..."
kubectl apply -f "${SCRIPT_DIR}/manifests/deny-policy.yaml"

echo "==> AuthorizationPolicy applied. Linkerd proxy will deny all inbound traffic."
echo "    Health checks will fail, pods may become NotReady."
echo "==> Watch: kubectl get pods -n demo-mesh-failure -w"
echo "==> Check Linkerd: linkerd viz stat deploy -n demo-mesh-failure"
