#!/bin/sh
set -e

: "${RELEASE_NAME:?RELEASE_NAME is required}"
: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"

echo "=== Phase 1: Validate ==="
echo "Checking Helm release ${RELEASE_NAME} in ${TARGET_NAMESPACE}..."

CURRENT_REV=$(helm history "${RELEASE_NAME}" -n "${TARGET_NAMESPACE}" --max 1 -o json | \
  jq -r '.[0].revision')
echo "Current Helm revision: ${CURRENT_REV}"

if [ "${CURRENT_REV}" -le 1 ]; then
  echo "ERROR: No previous revision to roll back to (current rev: ${CURRENT_REV})"
  exit 1
fi

RELEASE_STATUS=$(helm status "${RELEASE_NAME}" -n "${TARGET_NAMESPACE}" -o json | \
  jq -r '.info.status')
echo "Release status: ${RELEASE_STATUS}"

CRASH_PODS=$(kubectl get pods -n "${TARGET_NAMESPACE}" \
  --field-selector=status.phase!=Running -o name 2>/dev/null | wc -l | tr -d ' ')
echo "Non-running pods: ${CRASH_PODS}"
echo "Validated: Helm release has rollback history."

echo "=== Phase 2: Action ==="
PREV_REV=$((CURRENT_REV - 1))
echo "Rolling back Helm release ${RELEASE_NAME} to revision ${PREV_REV}..."
helm rollback "${RELEASE_NAME}" "${PREV_REV}" -n "${TARGET_NAMESPACE}" --wait --timeout 120s

echo "=== Phase 3: Verify ==="
NEW_REV=$(helm history "${RELEASE_NAME}" -n "${TARGET_NAMESPACE}" --max 1 -o json | \
  jq -r '.[0].revision')
echo "New Helm revision: ${NEW_REV}"

NEW_STATUS=$(helm status "${RELEASE_NAME}" -n "${TARGET_NAMESPACE}" -o json | \
  jq -r '.info.status')
echo "Release status: ${NEW_STATUS}"

READY=$(kubectl get deployment worker -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
DESIRED=$(kubectl get deployment worker -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "0")
echo "Replicas: ${READY}/${DESIRED} ready"

if [ "${READY}" = "${DESIRED}" ] && [ "${NEW_STATUS}" = "deployed" ]; then
  echo "=== SUCCESS: Helm release rolled back (rev ${CURRENT_REV} -> ${NEW_REV}), all replicas ready ==="
else
  echo "ERROR: Rollback may not have fully succeeded (status=${NEW_STATUS}, ready=${READY}/${DESIRED})"
  exit 1
fi
