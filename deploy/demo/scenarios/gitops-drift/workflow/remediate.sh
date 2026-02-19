#!/bin/sh
# GitOps Revert Remediation Script
#
# Authority: DD-WORKFLOW-003 (Parameterized Remediation Actions)
# Scenario: #125 -- GitOps drift remediation
#
# Pattern: Validate -> Action -> Verify (DD-WORKFLOW-003)
#
# Parameters (env vars):
#   GIT_REPO_URL          - URL of the Git repository
#   GIT_BRANCH            - Branch to revert on (default: main)
#   TARGET_NAMESPACE      - Namespace of the affected workload
#   TARGET_RESOURCE_NAME  - Name of the affected resource
#
set -e

GIT_BRANCH="${GIT_BRANCH:-main}"
WORK_DIR="/tmp/gitops-revert"

echo "=== Phase 1: Validate ==="
echo "Checking for crashing pods in namespace ${TARGET_NAMESPACE}..."

CRASH_PODS=$(kubectl get pods -n "${TARGET_NAMESPACE}" \
  --field-selector=status.phase!=Running,status.phase!=Succeeded \
  -o name 2>/dev/null | wc -l | tr -d ' ')

if [ "${CRASH_PODS}" -eq 0 ]; then
  echo "No crashing pods found. Verifying restart count..."
  RESTARTING=$(kubectl get pods -n "${TARGET_NAMESPACE}" \
    -o jsonpath='{range .items[*]}{.status.containerStatuses[*].restartCount}{"\n"}{end}' 2>/dev/null \
    | awk '{s+=$1} END {print s+0}')
  if [ "${RESTARTING}" -eq 0 ]; then
    echo "No issues detected, nothing to do"
    exit 0
  fi
  echo "Found pods with restarts: ${RESTARTING} total restarts"
fi

echo "Validated: workload in ${TARGET_NAMESPACE} has issues"

echo "=== Phase 2: Action ==="
echo "Cloning repository: ${GIT_REPO_URL}"
rm -rf "${WORK_DIR}"
git clone --branch "${GIT_BRANCH}" --depth 5 "${GIT_REPO_URL}" "${WORK_DIR}"
cd "${WORK_DIR}"

LAST_COMMIT=$(git log --oneline -1)
echo "Last commit: ${LAST_COMMIT}"

echo "Reverting last commit..."
git config user.email "kubernaut@kubernaut.ai"
git config user.name "Kubernaut Remediation"
git revert --no-edit HEAD

echo "Pushing revert..."
git push origin "${GIT_BRANCH}"
echo "Revert pushed successfully"

echo "=== Phase 3: Verify ==="
echo "Waiting for ArgoCD to sync (up to 120s)..."
TIMEOUT=120
ELAPSED=0
while [ "${ELAPSED}" -lt "${TIMEOUT}" ]; do
  READY=$(kubectl get pods -n "${TARGET_NAMESPACE}" -l app="${TARGET_RESOURCE_NAME}" \
    --field-selector=status.phase=Running \
    -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' 2>/dev/null \
    | grep -c "True" || echo "0")

  if [ "${READY}" -gt 0 ]; then
    echo "Pods are healthy after revert (${READY} ready)"
    break
  fi

  sleep 5
  ELAPSED=$((ELAPSED + 5))
  echo "  Waiting... (${ELAPSED}s)"
done

if [ "${ELAPSED}" -ge "${TIMEOUT}" ]; then
  echo "WARNING: Pods not yet healthy after ${TIMEOUT}s, ArgoCD sync may still be in progress"
  exit 1
fi

echo "=== SUCCESS: GitOps revert completed, workload restored ==="
