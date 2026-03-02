#!/bin/sh
# GitOps Revert Remediation Script
#
# Authority: DD-WORKFLOW-003 (Parameterized Remediation Actions)
# Scenario: #125 -- GitOps drift remediation
#
# DD-WE-006: Git credentials are read from volume-mounted Secret (gitea-repo-creds),
# NOT embedded in GIT_REPO_URL by the LLM. The Secret is provisioned by operators
# in kubernaut-workflows and mounted at /run/kubernaut/secrets/gitea-repo-creds/.
#
# GIT_REPO_URL and GIT_BRANCH are discovered from the ArgoCD Application that
# targets TARGET_NAMESPACE, not provided by the LLM.
#
# Parameters (env vars):
#   TARGET_NAMESPACE      - Namespace of the affected workload
#   TARGET_RESOURCE_NAME  - Name of the affected resource
#
set -e

: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"
: "${TARGET_RESOURCE_NAME:?TARGET_RESOURCE_NAME is required}"

WORK_DIR="/tmp/gitops-revert"

SECRET_DIR="/run/kubernaut/secrets/gitea-repo-creds"
if [ ! -d "${SECRET_DIR}" ]; then
  echo "ERROR: Secret mount not found at ${SECRET_DIR}. Ensure gitea-repo-creds Secret exists in kubernaut-workflows."
  exit 1
fi
GIT_USERNAME=$(cat "${SECRET_DIR}/username")
GIT_PASSWORD=$(cat "${SECRET_DIR}/password")

echo "=== Phase 0: Discover ArgoCD Application ==="
ARGO_APP_JSON=$(kubectl get applications.argoproj.io -n argocd -o json)

GIT_REPO_URL=$(echo "${ARGO_APP_JSON}" | jq -r \
  --arg ns "${TARGET_NAMESPACE}" \
  '.items[] | select(.spec.destination.namespace == $ns) | .spec.source.repoURL' \
  | head -1)

GIT_BRANCH_RAW=$(echo "${ARGO_APP_JSON}" | jq -r \
  --arg ns "${TARGET_NAMESPACE}" \
  '.items[] | select(.spec.destination.namespace == $ns) | .spec.source.targetRevision' \
  | head -1)
GIT_BRANCH="${GIT_BRANCH_RAW}"
[ "${GIT_BRANCH}" = "HEAD" ] || [ -z "${GIT_BRANCH}" ] && GIT_BRANCH="main"

if [ -z "${GIT_REPO_URL}" ] || [ "${GIT_REPO_URL}" = "null" ]; then
  echo "ERROR: No ArgoCD Application found targeting namespace ${TARGET_NAMESPACE}"
  exit 1
fi
echo "Discovered from ArgoCD: repoURL=${GIT_REPO_URL} branch=${GIT_BRANCH}"

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
AUTH_URL=$(echo "${GIT_REPO_URL}" | sed "s|://|://${GIT_USERNAME}:${GIT_PASSWORD}@|")
echo "Cloning repository: ${GIT_REPO_URL}"
rm -rf "${WORK_DIR}"
git clone --branch "${GIT_BRANCH}" --depth 5 "${AUTH_URL}" "${WORK_DIR}"
cd "${WORK_DIR}"

LAST_COMMIT=$(git log --oneline -1)
echo "Last commit: ${LAST_COMMIT}"

echo "Reverting last commit..."
git config user.email "kubernaut@kubernaut.ai"
git config user.name "Kubernaut Remediation"
git revert --no-edit HEAD

echo "Pushing revert..."
git push origin "${GIT_BRANCH}"

NEW_COMMIT=$(git rev-parse HEAD)
echo "Revert commit: ${NEW_COMMIT}"

echo "=== Phase 3: Verify ==="
VERIFY_TIMEOUT="${VERIFY_TIMEOUT:-180}"
POLL_INTERVAL=5
ELAPSED=0

ARGO_APP_NAME=$(echo "${ARGO_APP_JSON}" | jq -r \
  --arg ns "${TARGET_NAMESPACE}" \
  '.items[] | select(.spec.destination.namespace == $ns) | .metadata.name' \
  | head -1)

echo "Waiting for ArgoCD app '${ARGO_APP_NAME}' to sync revision ${NEW_COMMIT} (timeout=${VERIFY_TIMEOUT}s)..."
while [ "${ELAPSED}" -lt "${VERIFY_TIMEOUT}" ]; do
  SYNC_REV=$(kubectl get app "${ARGO_APP_NAME}" -n argocd \
    -o jsonpath='{.status.sync.revision}' 2>/dev/null || echo "")

  if [ "${SYNC_REV}" = "${NEW_COMMIT}" ]; then
    echo "  [${ELAPSED}s] ArgoCD synced revision ${NEW_COMMIT}"
    break
  fi

  echo "  [${ELAPSED}s] ArgoCD sync revision=${SYNC_REV:-pending}, waiting..."
  sleep "${POLL_INTERVAL}"
  ELAPSED=$((ELAPSED + POLL_INTERVAL))
done

if [ "${ELAPSED}" -ge "${VERIFY_TIMEOUT}" ]; then
  echo "ERROR: ArgoCD did not sync revert commit after ${VERIFY_TIMEOUT}s"
  exit 1
fi

echo "Waiting for deployment rollout to complete..."
if kubectl rollout status "deployment/${TARGET_RESOURCE_NAME}" -n "${TARGET_NAMESPACE}" --timeout=120s 2>&1; then
  echo "=== SUCCESS: Git commit reverted (${NEW_COMMIT}), deployment rolled out ==="
else
  echo "ERROR: Deployment rollout did not complete after revert"
  exit 1
fi
