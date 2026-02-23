#!/usr/bin/env bash
# Display the AI analysis result in a human-readable format for the demo recording.
set -euo pipefail

NAMESPACE="${1:-demo-crashloop}"

AA_NAME=$(kubectl get aianalyses -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}')

ROOT_CAUSE=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.rootCause}')
SEVERITY=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.rootCauseAnalysis.severity}')
AFFECTED_KIND=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.kind}')
AFFECTED_NAME=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.name}')
AFFECTED_NS=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.namespace}')
CONFIDENCE=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.selectedWorkflow.confidence}')
WORKFLOW_ID=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.selectedWorkflow.workflowId}')
ACTION_TYPE=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.selectedWorkflow.actionType}')
RATIONALE=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.selectedWorkflow.rationale}')
APPROVAL=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvalRequired}')
APPROVAL_REASON=$(kubectl get aianalyses "$AA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.approvalReason}')

printf '\n'
printf '  Root Cause Analysis\n'
printf '  ───────────────────\n'
printf '  Root Cause:       %s\n' "${ROOT_CAUSE:-N/A}"
printf '  Severity:         %s\n' "${SEVERITY:-unknown}"
if [ -n "$AFFECTED_KIND" ]; then
  printf '  Target Resource:  %s/%s (ns: %s)\n' "${AFFECTED_KIND}" "${AFFECTED_NAME}" "${AFFECTED_NS}"
fi
printf '\n'
printf '  Selected Workflow\n'
printf '  ─────────────────\n'
printf '  ID:               %s\n' "${WORKFLOW_ID:-N/A}"
printf '  Action:           %s\n' "${ACTION_TYPE:-N/A}"
printf '  Confidence:       %s\n' "${CONFIDENCE:-N/A}"
printf '  Rationale:        %s\n' "${RATIONALE:-N/A}"
printf '\n'
printf '  Approval Required: %s\n' "${APPROVAL:-false}"
if [ -n "$APPROVAL_REASON" ]; then
  printf '  Reason:            %s\n' "${APPROVAL_REASON}"
fi
printf '\n'
