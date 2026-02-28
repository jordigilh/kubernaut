#!/usr/bin/env bash
# Display the AI analysis result in a human-readable format for demo recordings.
# Usage: bash deploy/demo/scripts/show-ai-analysis.sh <scenario-namespace>
# Example: bash deploy/demo/scripts/show-ai-analysis.sh demo-crashloop
set -euo pipefail

SCENARIO_NS="${1:?Usage: show-ai-analysis.sh <scenario-namespace>}"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

AA_NAME=$(kubectl get aianalyses -n "$PLATFORM_NS" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.remediationRequestRef.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "$SCENARIO_NS" | tail -1 | cut -f1)

if [ -z "$AA_NAME" ]; then
  AA_NAME=$(kubectl get aianalyses -n "$PLATFORM_NS" -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null)
fi

ROOT_CAUSE=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.rootCause}' 2>/dev/null)
SEVERITY=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.rootCauseAnalysis.severity}' 2>/dev/null)
AFFECTED_KIND=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.kind}' 2>/dev/null)
AFFECTED_NAME=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.name}' 2>/dev/null)
AFFECTED_NS=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.namespace}' 2>/dev/null)
CONFIDENCE=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null)
WORKFLOW_ID=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.selectedWorkflow.workflowId}' 2>/dev/null)
ACTION_TYPE=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.selectedWorkflow.actionType}' 2>/dev/null)
RATIONALE=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.selectedWorkflow.rationale}' 2>/dev/null)
APPROVAL=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.approvalRequired}' 2>/dev/null)
APPROVAL_REASON=$(kubectl get aianalyses "$AA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.approvalReason}' 2>/dev/null)

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
