#!/usr/bin/env bash
# Inject a deny-all NetworkPolicy to block all ingress traffic
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Removing allow-web-traffic policy (so deny-all takes full effect)..."
kubectl delete networkpolicy allow-web-traffic -n demo-netpol --ignore-not-found

echo "==> Injecting deny-all NetworkPolicy..."
kubectl apply -f "${SCRIPT_DIR}/manifests/deny-all-netpol.yaml"

echo "==> Deny-all NetworkPolicy applied. All ingress traffic is now blocked."
echo "    Health checks will fail, pods will become NotReady."
echo "==> Watch: kubectl get pods -n demo-netpol -w"
