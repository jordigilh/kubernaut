#!/usr/bin/env bash
# Display the approval reason from the AIAnalysis status, highlighting the rego
# policy match that triggered the RemediationApprovalRequest.
# Usage: bash deploy/demo/scripts/show-approval-reason.sh <scenario-namespace>
# Example: bash deploy/demo/scripts/show-approval-reason.sh demo-crashloop
set -euo pipefail

SCENARIO_NS="${1:?Usage: show-approval-reason.sh <scenario-namespace>}"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

AA_NAME=$(kubectl get aianalyses -n "$PLATFORM_NS" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.remediationRequestRef.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "$SCENARIO_NS" | tail -1 | cut -f1)

if [ -z "$AA_NAME" ]; then
  AA_NAME=$(kubectl get aianalyses -n "$PLATFORM_NS" -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null)
fi

APPROVAL=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.approvalRequired}' 2>/dev/null)
REASON=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.approvalReason}' 2>/dev/null)
ENVIRONMENT=$(kubectl get ns "$SCENARIO_NS" -o jsonpath='{.metadata.labels.kubernaut\.ai/environment}' 2>/dev/null)

REGO_POLICY=$(kubectl get configmap aianalysis-policies -n "$PLATFORM_NS" \
  -o jsonpath='{.data.approval\.rego}' 2>/dev/null)

printf '\n'
printf '  ┌─────────────────────────────────────────────────────────┐\n'
printf '  │  Approval Policy Match                                  │\n'
printf '  └─────────────────────────────────────────────────────────┘\n'
printf '\n'
printf '  Approval Required:  %s\n' "${APPROVAL:-false}"
printf '  Policy Reason:      %s\n' "${REASON:-N/A}"
printf '  Namespace Label:    kubernaut.ai/environment=%s\n' "${ENVIRONMENT:-N/A}"
printf '\n'
printf '  Rego Policy (aianalysis-policies ConfigMap):\n'
printf '  ────────────────────────────────────────────\n'
if [ -n "$REGO_POLICY" ]; then
  echo "$REGO_POLICY" | head -8 | while IFS= read -r line; do
    printf '    %s\n' "$line"
  done
  printf '    ...\n'
else
  printf '    (policy not found)\n'
fi
printf '\n'
if [ -n "$ENVIRONMENT" ]; then
  printf '  This namespace is labeled environment=%s, which matched the\n' "$ENVIRONMENT"
  printf '  approval policy. A RAR must be approved before the workflow executes.\n'
fi
printf '\n'
