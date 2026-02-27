#!/usr/bin/env bash
# Approve the most recent pending RemediationApprovalRequest.
# Usage: bash deploy/demo/scripts/approve-rar.sh
set -euo pipefail

PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

RAR_NAME=$(kubectl get remediationapprovalrequests -n "$PLATFORM_NS" \
  -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null)

if [ -z "$RAR_NAME" ]; then
  echo "==> No RemediationApprovalRequest found in ${PLATFORM_NS}"
  exit 1
fi

echo "==> Approving RAR: $RAR_NAME"

kubectl patch remediationapprovalrequest "$RAR_NAME" -n "$PLATFORM_NS" \
  --subresource=status --type=merge \
  -p '{"status":{"decision":"Approved","decidedBy":"demo-operator","decisionMessage":"Approved for demo"}}'

echo "==> RAR approved."
