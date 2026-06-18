#!/usr/bin/env bash
# Deploy the K8s MCP server on a workload cluster
# Prerequisite: manifests/01-mcp-server-rbac.yaml applied

set -euo pipefail

NAMESPACE="${MCP_NAMESPACE:-spike-mcp-write}"
SA_NAME="${MCP_SA_NAME:-mcp-server-sa}"

helm upgrade -i kubernetes-mcp-server \
  oci://ghcr.io/containers/charts/kubernetes-mcp-server \
  -n "$NAMESPACE" \
  --set openshift=true \
  --set serviceAccount.create=false \
  --set serviceAccount.name="$SA_NAME" \
  --set ingress.enabled=false \
  --set service.type=ClusterIP \
  --set service.port=8080

echo "MCP server deployed. Verify:"
echo "  kubectl -n $NAMESPACE get pods -l app.kubernetes.io/name=kubernetes-mcp-server"
echo "  kubectl -n $NAMESPACE port-forward svc/kubernetes-mcp-server 8080:8080"
