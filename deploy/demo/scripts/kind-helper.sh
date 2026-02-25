#!/usr/bin/env bash
# Shared Kind cluster helpers for demo scenarios
# Source this file from run.sh: source "$(dirname "$0")/../../scripts/kind-helper.sh"

CLUSTER_NAME="${CLUSTER_NAME:-kubernaut-demo}"

# Ensure the Kind cluster exists with the required topology.
# Usage: ensure_kind_cluster <kind-config-path> [--create-cluster]
#
# Behavior:
#   - If --create-cluster is passed: recreates the cluster (deletes existing first)
#   - If cluster exists: validates topology against the config
#   - If cluster doesn't exist: auto-creates it
ensure_kind_cluster() {
    local config_path="$1"
    local create_flag="${2:-}"

    if [ "$create_flag" = "--create-cluster" ]; then
        echo "==> Recreating Kind cluster '${CLUSTER_NAME}' from ${config_path}..."
        kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
        kind create cluster --name "${CLUSTER_NAME}" --config "${config_path}"
        echo "  Cluster created."
        return 0
    fi

    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        echo "==> Kind cluster '${CLUSTER_NAME}' exists. Validating topology..."
        validate_topology "${config_path}"
        return $?
    else
        echo "==> Kind cluster '${CLUSTER_NAME}' not found. Creating..."
        kind create cluster --name "${CLUSTER_NAME}" --config "${config_path}"
        echo "  Cluster created."
        return 0
    fi
}

# Validate that the running cluster has the expected node topology
validate_topology() {
    local config_path="$1"

    local needs_worker=false
    if grep -q 'role: worker' "$config_path" 2>/dev/null; then
        needs_worker=true
    fi

    if $needs_worker; then
        local worker_count
        worker_count=$(kubectl get nodes -l kubernaut.ai/managed=true --no-headers 2>/dev/null | wc -l | tr -d ' ')
        if [ "$worker_count" -eq 0 ]; then
            echo "WARNING: Scenario requires a worker node with label kubernaut.ai/managed=true"
            echo "  Found 0 matching nodes. The scenario may not work correctly."
            echo ""
            echo "  Recreate the cluster with:"
            echo "    kind create cluster --name ${CLUSTER_NAME} --config ${config_path}"
            return 1
        fi
        echo "  Topology OK: ${worker_count} worker node(s) with kubernaut.ai/managed=true"
    else
        echo "  Topology OK: single-node cluster sufficient"
    fi
    return 0
}
