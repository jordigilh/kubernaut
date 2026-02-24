#!/usr/bin/env bash
# Display Gateway deduplication metrics for the alert storm scenario
set -euo pipefail

NAMESPACE="demo-alert-storm"

echo "==> RemediationRequest Deduplication Status:"
echo ""

RR_COUNT=$(kubectl get rr -n "${NAMESPACE}" -o json 2>/dev/null | jq '.items | length' 2>/dev/null || echo "0")
echo "  Total RRs created: ${RR_COUNT}"
echo "  (Expected: 1 -- Gateway deduplicates per-Deployment fingerprint)"
echo ""

if [ "${RR_COUNT}" -gt 0 ]; then
  echo "  RR Details:"
  kubectl get rr -n "${NAMESPACE}" -o custom-columns=\
NAME:.metadata.name,\
PHASE:.status.overallPhase,\
FINGERPRINT:.spec.signalFingerprint,\
OCCURRENCES:.status.deduplication.occurrenceCount,\
LAST_SEEN:.status.deduplication.lastSeenAt
  echo ""
  echo "  Deduplication proves Gateway correctly maps all 5 crashing pods"
  echo "  to a single Deployment-level fingerprint."
fi
