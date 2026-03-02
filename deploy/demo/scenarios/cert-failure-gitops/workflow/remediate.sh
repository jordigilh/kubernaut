#!/bin/sh
# Fix Certificate GitOps Remediation Script
#
# DD-WE-006: Git credentials are read from volume-mounted Secret (gitea-repo-creds),
# NOT from LLM-provided parameters. The Secret is provisioned by operators in
# kubernaut-workflows and mounted at /run/kubernaut/secrets/gitea-repo-creds/.
#
# GIT_REPO_URL and GIT_BRANCH are discovered from the ArgoCD Application that
# targets TARGET_NAMESPACE, not provided by the LLM.
set -e

: "${TARGET_RESOURCE_NAME:?TARGET_RESOURCE_NAME is required}"
: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"

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
echo "Checking Certificate ${TARGET_RESOURCE_NAME} in ${TARGET_NAMESPACE}..."

CERT_READY=$(kubectl get certificate "${TARGET_RESOURCE_NAME}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
echo "Certificate Ready status: ${CERT_READY}"

if [ "${CERT_READY}" = "True" ]; then
  echo "Certificate is already Ready. No action needed."
  exit 0
fi

echo "Validated: Certificate is not Ready. Proceeding with git revert."

echo "=== Phase 2: Action ==="
WORK_DIR=$(mktemp -d)
trap 'rm -rf "${WORK_DIR}"' EXIT
cd "${WORK_DIR}"

AUTH_URL=$(echo "${GIT_REPO_URL}" | sed "s|://|://${GIT_USERNAME}:${GIT_PASSWORD}@|")
echo "Cloning repository ${GIT_REPO_URL}..."
git clone "${AUTH_URL}" repo
cd repo
git config user.email "kubernaut-remediation@kubernaut.ai"
git config user.name "Kubernaut Remediation"

git checkout "${GIT_BRANCH}"

CURRENT_COMMIT=$(git rev-parse HEAD)
echo "Current commit: ${CURRENT_COMMIT}"

echo "Reverting HEAD commit..."
git revert HEAD --no-edit

echo "Pushing revert to ${GIT_BRANCH}..."
git push origin "${GIT_BRANCH}"

NEW_COMMIT=$(git rev-parse HEAD)
echo "Revert commit: ${NEW_COMMIT}"

echo "=== Phase 3: Verify ==="
VERIFY_TIMEOUT="${VERIFY_TIMEOUT:-180}"
POLL_INTERVAL=10
ELAPSED=0

echo "Polling Certificate status (timeout=${VERIFY_TIMEOUT}s, interval=${POLL_INTERVAL}s)..."
while [ "${ELAPSED}" -lt "${VERIFY_TIMEOUT}" ]; do
  CERT_READY=$(kubectl get certificate "${TARGET_RESOURCE_NAME}" -n "${TARGET_NAMESPACE}" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")

  if [ "${CERT_READY}" = "True" ]; then
    echo "=== SUCCESS: Git commit reverted (${CURRENT_COMMIT} -> ${NEW_COMMIT}), Certificate is Ready (after ${ELAPSED}s) ==="
    exit 0
  fi

  echo "  [${ELAPSED}s] Certificate Ready=${CERT_READY}, waiting..."
  sleep "${POLL_INTERVAL}"
  ELAPSED=$((ELAPSED + POLL_INTERVAL))
done

echo "ERROR: Certificate still not Ready after revert + ${VERIFY_TIMEOUT}s"
exit 1
