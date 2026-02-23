#!/usr/bin/env bash
# Exhaust ResourceQuota by scaling deployment beyond quota limits
# 3 replicas x 256Mi = 768Mi > 512Mi quota
set -euo pipefail

NAMESPACE="demo-quota"

echo "==> Current ResourceQuota usage:"
kubectl describe quota namespace-quota -n "${NAMESPACE}"
echo ""

echo "==> Scaling api-server to 3 replicas with 256Mi each (768Mi > 512Mi quota)..."
kubectl patch deployment api-server -n "${NAMESPACE}" --type=json -p '[
  {"op": "replace", "path": "/spec/replicas", "value": 3},
  {"op": "replace", "path": "/spec/template/spec/containers/0/resources/requests/memory", "value": "256Mi"},
  {"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value": "256Mi"}
]'

echo "==> Waiting for pods to go Pending..."
sleep 10

echo "==> Pod status (expect some Pending with 'exceeded quota'):"
kubectl get pods -n "${NAMESPACE}"
echo ""
echo "==> Events showing quota exhaustion:"
kubectl get events -n "${NAMESPACE}" --sort-by='.lastTimestamp' | tail -10
echo ""
echo "==> Updated ResourceQuota usage:"
kubectl describe quota namespace-quota -n "${NAMESPACE}"
