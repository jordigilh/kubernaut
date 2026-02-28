#!/usr/bin/env bash
# Exhaust ResourceQuota by scaling deployment beyond quota limits.
#
# Patches the deployment template to require 256Mi per pod (up from 128Mi)
# and sets replicas=3. The template change creates a new ReplicaSet, but
# the old RS pods (128Mi each) still consume quota. With 384Mi already used
# the new RS cannot create a single 256Mi pod (384+256=640 > 512Mi quota).
# The new RS shows spec_replicas>0 but status_replicas=0 (FailedCreate).
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

echo "==> Waiting for ReplicaSet to fail creating pods..."
sleep 10

echo "==> Pod status (old RS pods running, new RS stuck at FailedCreate):"
kubectl get pods -n "${NAMESPACE}"
echo ""
echo "==> ReplicaSet status (new RS has 0 ready):"
kubectl get rs -n "${NAMESPACE}"
echo ""
echo "==> Events showing quota exhaustion:"
kubectl get events -n "${NAMESPACE}" --sort-by='.lastTimestamp' | tail -10
echo ""
echo "==> Updated ResourceQuota usage:"
kubectl describe quota namespace-quota -n "${NAMESPACE}"
