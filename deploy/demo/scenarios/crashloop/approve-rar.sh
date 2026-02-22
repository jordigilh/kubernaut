#!/usr/bin/env bash
# Approve the pending RemediationApprovalRequest for the crashloop demo
set -euo pipefail

NAMESPACE="demo-crashloop"

RAR_NAME=$(kubectl get rar -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}')
echo "==> Approving RAR: $RAR_NAME"

kubectl patch rar "$RAR_NAME" -n "$NAMESPACE" \
  --subresource=status --type=merge \
  -p '{"status":{"decision":"Approved","decidedBy":"demo-operator","reason":"Approved for demo"}}'

echo "==> RAR approved."
