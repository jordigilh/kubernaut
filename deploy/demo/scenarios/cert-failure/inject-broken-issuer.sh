#!/usr/bin/env bash
# Inject cert-manager failure by deleting the CA Secret backing the ClusterIssuer
set -euo pipefail

echo "==> Deleting CA Secret that backs the ClusterIssuer..."
kubectl delete secret demo-ca-key-pair -n cert-manager

echo "==> Triggering certificate re-issuance to force failure detection..."
kubectl delete secret demo-app-tls -n demo-cert-failure --ignore-not-found

echo "==> Renewing certificate to trigger immediate re-issuance attempt..."
kubectl cert-manager renew demo-app-cert -n demo-cert-failure 2>/dev/null || \
  kubectl annotate certificate demo-app-cert -n demo-cert-failure \
    cert-manager.io/issuing-trigger="manual-$(date +%s)" --overwrite

echo "==> CA Secret deleted. cert-manager will fail to issue demo-app-cert."
echo "    ClusterIssuer 'demo-selfsigned-ca' can no longer sign certificates."
echo "==> Watch: kubectl get certificate -n demo-cert-failure -w"
