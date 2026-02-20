#!/bin/sh
set -e

: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"

echo "=== Phase 1: Validate ==="
echo "Checking NetworkPolicies in namespace ${TARGET_NAMESPACE}..."

POLICIES=$(kubectl get networkpolicies -n "${TARGET_NAMESPACE}" -o json)
POLICY_COUNT=$(echo "${POLICIES}" | grep -c '"name"' || echo "0")
echo "Found ${POLICY_COUNT} NetworkPolicies"

if [ -n "${TARGET_POLICY:-}" ]; then
  OFFENDING_POLICY="${TARGET_POLICY}"
  echo "Target policy specified: ${OFFENDING_POLICY}"
else
  OFFENDING_POLICY=$(kubectl get networkpolicies -n "${TARGET_NAMESPACE}" \
    -l injected-by=kubernaut-demo -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

  if [ -z "${OFFENDING_POLICY}" ]; then
    echo "ERROR: Could not identify a deny-all NetworkPolicy (expected label injected-by=kubernaut-demo)"
    kubectl get networkpolicies -n "${TARGET_NAMESPACE}" -o wide
    exit 1
  fi
  echo "Auto-detected offending policy: ${OFFENDING_POLICY}"
fi

UNAVAILABLE=$(kubectl get deployment -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.metadata.name}{": unavailable="}{.status.unavailableReplicas}{"\n"}{end}' 2>/dev/null)
echo "Deployment status: ${UNAVAILABLE}"
echo "Validated: Deny-all NetworkPolicy '${OFFENDING_POLICY}' found."

echo "=== Phase 2: Action ==="
echo "Removing NetworkPolicy '${OFFENDING_POLICY}'..."
kubectl delete networkpolicy "${OFFENDING_POLICY}" -n "${TARGET_NAMESPACE}"

echo "Waiting for pods to recover (20s)..."
sleep 20

echo "=== Phase 3: Verify ==="
REMAINING=$(kubectl get networkpolicies -n "${TARGET_NAMESPACE}" \
  -o name 2>/dev/null | wc -l | tr -d ' ')
echo "Remaining NetworkPolicies: ${REMAINING}"

READY=$(kubectl get deployment -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.status.readyReplicas}{end}' 2>/dev/null || echo "0")
DESIRED=$(kubectl get deployment -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.spec.replicas}{end}' 2>/dev/null || echo "0")
echo "Deployment replicas: ${READY}/${DESIRED} ready"

UNAVAILABLE=$(kubectl get deployment -n "${TARGET_NAMESPACE}" \
  -o jsonpath='{range .items[*]}{.status.unavailableReplicas}{end}' 2>/dev/null || echo "0")
echo "Unavailable replicas: ${UNAVAILABLE:-0}"

if [ "${READY}" = "${DESIRED}" ]; then
  echo "=== SUCCESS: NetworkPolicy '${OFFENDING_POLICY}' removed, all ${DESIRED} replicas ready ==="
else
  echo "WARNING: Not all replicas ready yet (${READY}/${DESIRED}). May need more time."
  exit 1
fi
