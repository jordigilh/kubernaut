#!/bin/sh
set -e

: "${TARGET_RESOURCE_NAME:?TARGET_RESOURCE_NAME is required}"
: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"
: "${GIT_REPO_URL:?GIT_REPO_URL is required}"
: "${GIT_USERNAME:?GIT_USERNAME is required}"
: "${GIT_PASSWORD:?GIT_PASSWORD is required}"

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

BRANCH="${GIT_BRANCH:-main}"
git checkout "${BRANCH}"

CURRENT_COMMIT=$(git rev-parse HEAD)
echo "Current commit: ${CURRENT_COMMIT}"

echo "Reverting HEAD commit..."
git revert HEAD --no-edit

echo "Pushing revert to ${BRANCH}..."
git push origin "${BRANCH}"

NEW_COMMIT=$(git rev-parse HEAD)
echo "Revert commit: ${NEW_COMMIT}"

echo "Waiting for ArgoCD to sync (30s)..."
sleep 30

echo "=== Phase 3: Verify ==="
CERT_READY=$(kubectl get certificate "${TARGET_RESOURCE_NAME}" -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
echo "Certificate Ready status: ${CERT_READY}"

if [ "${CERT_READY}" = "True" ]; then
  echo "=== SUCCESS: Git commit reverted (${CURRENT_COMMIT} -> ${NEW_COMMIT}), Certificate is Ready ==="
else
  echo "WARNING: Certificate still not Ready after git revert. ArgoCD may need more time to sync."
  echo "Waiting additional 30s..."
  sleep 30
  CERT_READY=$(kubectl get certificate "${TARGET_RESOURCE_NAME}" -n "${TARGET_NAMESPACE}" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
  if [ "${CERT_READY}" = "True" ]; then
    echo "=== SUCCESS: Certificate became Ready after extended wait ==="
  else
    echo "ERROR: Certificate still not Ready after revert + 60s wait"
    exit 1
  fi
fi
