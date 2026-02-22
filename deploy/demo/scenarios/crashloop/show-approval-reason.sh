#!/usr/bin/env bash
# Display the approval reason from the AIAnalysis status, highlighting the rego
# policy match that triggered the RemediationApprovalRequest.
set -euo pipefail

NAMESPACE="${1:-demo-crashloop}"

AA_NAME=$(kubectl get aianalyses -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}')

APPROVAL=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvalRequired}')
REASON=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvalReason}')
ENVIRONMENT=$(kubectl get ns "$NAMESPACE" -o jsonpath='{.metadata.labels.kubernaut\.ai/environment}' 2>/dev/null)

# Extract the rego policy from the live ConfigMap
REGO_POLICY=$(kubectl get configmap aianalysis-policies -n kubernaut-system \
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
printf '  This namespace is labeled environment=production, so the rule\n'
printf '  "require_approval if { input.environment == \"production\" }"\n'
printf '  matched. A RAR must be approved before the workflow can execute.\n'
printf '\n'
