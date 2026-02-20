#!/bin/sh
set -e

: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"

echo "=== Phase 1: Validate ==="
echo "Checking AuthorizationPolicies in namespace ${TARGET_NAMESPACE}..."

POLICIES=$(kubectl get authorizationpolicies.policy.linkerd.io \
  -n "${TARGET_NAMESPACE}" -o name 2>/dev/null || echo "")

if [ -z "${POLICIES}" ]; then
  echo "No AuthorizationPolicies found in ${TARGET_NAMESPACE}."
  echo "ERROR: Expected to find a blocking policy but found none."
  exit 1
fi

echo "Found AuthorizationPolicies:"
echo "${POLICIES}"

if [ -n "${TARGET_POLICY:-}" ]; then
  POLICY_NAME="${TARGET_POLICY}"
else
  POLICY_NAME=$(echo "${POLICIES}" | head -1 | sed 's|.*/||')
  echo "No specific policy specified, targeting first found: ${POLICY_NAME}"
fi

echo "Checking pods status..."
NOT_READY=$(kubectl get pods -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.metadata.name}{" ready="}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' 2>/dev/null | grep -c "ready=False" || echo "0")
echo "Pods not ready: ${NOT_READY}"

echo "Validated: AuthorizationPolicy '${POLICY_NAME}' found, likely blocking traffic."

echo "=== Phase 2: Action ==="
echo "Removing AuthorizationPolicy '${POLICY_NAME}'..."
kubectl delete authorizationpolicy.policy.linkerd.io "${POLICY_NAME}" \
  -n "${TARGET_NAMESPACE}"

SERVERS=$(kubectl get servers.policy.linkerd.io \
  -n "${TARGET_NAMESPACE}" -o name 2>/dev/null || echo "")
if [ -n "${SERVERS}" ]; then
  if [ -n "${TARGET_SERVER:-}" ]; then
    echo "Removing Server '${TARGET_SERVER}'..."
    kubectl delete server.policy.linkerd.io "${TARGET_SERVER}" \
      -n "${TARGET_NAMESPACE}" --ignore-not-found
  else
    echo "Removing all Server resources (a Server without an AuthorizationPolicy still denies unauthenticated traffic)..."
    kubectl delete servers.policy.linkerd.io --all \
      -n "${TARGET_NAMESPACE}" --ignore-not-found
  fi
fi

MESH_AUTH=$(kubectl get meshtlsauthentications.policy.linkerd.io \
  -n "${TARGET_NAMESPACE}" -o name 2>/dev/null || echo "")
if [ -n "${MESH_AUTH}" ]; then
  echo "Removing associated MeshTLSAuthentication resources..."
  kubectl delete meshtlsauthentications.policy.linkerd.io --all \
    -n "${TARGET_NAMESPACE}" --ignore-not-found
fi

echo "Waiting for pods to stabilize (15s)..."
sleep 15

echo "=== Phase 3: Verify ==="
REMAINING=$(kubectl get authorizationpolicies.policy.linkerd.io \
  -n "${TARGET_NAMESPACE}" -o name 2>/dev/null || echo "")
echo "Remaining AuthorizationPolicies: ${REMAINING:-<none>}"

REMAINING_SERVERS=$(kubectl get servers.policy.linkerd.io \
  -n "${TARGET_NAMESPACE}" -o name 2>/dev/null || echo "")
echo "Remaining Servers: ${REMAINING_SERVERS:-<none>}"

READY_PODS=$(kubectl get pods -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' 2>/dev/null | grep -c "True" || echo "0")
TOTAL_PODS=$(kubectl get pods -n "${TARGET_NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
echo "Pods ready: ${READY_PODS}/${TOTAL_PODS}"

if [ "${READY_PODS}" -gt 0 ] && [ -z "${REMAINING}" ] && [ -z "${REMAINING_SERVERS}" ]; then
  echo "=== SUCCESS: Policy resources removed, traffic restored, ${READY_PODS}/${TOTAL_PODS} pods ready ==="
else
  echo "WARNING: Remediation may not be fully effective (ready=${READY_PODS}/${TOTAL_PODS}, policies=${REMAINING}, servers=${REMAINING_SERVERS})"
  exit 1
fi
