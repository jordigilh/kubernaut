#!/usr/bin/env bash
# Seed ActionType CRDs in the cluster.
# Applies all ActionType YAML files from deploy/action-types/ and waits
# for the admission webhook to register them in Data Storage.
#
# Usage:
#   ./deploy/demo/scripts/seed-action-types.sh
#   ./deploy/demo/scripts/seed-action-types.sh --continue-on-error
#   ./deploy/demo/scripts/seed-action-types.sh --skip-wait
#
# BR-WORKFLOW-007: ActionType CRD lifecycle management

set -euo pipefail

CONTINUE_ON_ERROR=false
SKIP_WAIT=false
WAIT_TIMEOUT=60

while [[ $# -gt 0 ]]; do
    case "$1" in
        --continue-on-error)
            CONTINUE_ON_ERROR=true
            shift
            ;;
        --skip-wait)
            SKIP_WAIT=true
            shift
            ;;
        --timeout)
            WAIT_TIMEOUT="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--continue-on-error] [--skip-wait] [--timeout SECONDS]"
            echo ""
            echo "Options:"
            echo "  --continue-on-error  Skip failures and report summary at end"
            echo "  --skip-wait          Don't wait for .status.registered == true"
            echo "  --timeout SECONDS    Wait timeout per CRD (default: 60)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACTION_TYPES_DIR="${SCRIPT_DIR}/../../action-types"

if [ ! -d "$ACTION_TYPES_DIR" ]; then
    echo "ERROR: ActionType CRD directory not found: ${ACTION_TYPES_DIR}"
    exit 1
fi

echo "==> Seeding ActionType CRDs from ${ACTION_TYPES_DIR}"

ok_count=0
fail_count=0
failed_names=()

for yaml_file in "${ACTION_TYPES_DIR}"/*.yaml; do
    name=$(basename "$yaml_file" .yaml)
    echo -n "  ${name}: "

    if kubectl apply -f "$yaml_file" 2>/dev/null; then
        echo "APPLIED"
        ok_count=$((ok_count + 1))
    else
        echo "FAILED"
        fail_count=$((fail_count + 1))
        failed_names+=("${name}")
        if [ "$CONTINUE_ON_ERROR" = false ]; then
            echo ""
            echo "ERROR: ActionType CRD apply failed. Use --continue-on-error to skip failures."
            exit 1
        fi
    fi
done

echo ""
echo "==> Applied: ${ok_count} ActionType CRDs, ${fail_count} failed"

if [ "$SKIP_WAIT" = false ] && [ "$ok_count" -gt 0 ]; then
    echo "==> Waiting for ActionType CRDs to register in Data Storage (timeout: ${WAIT_TIMEOUT}s each)..."
    registered=0
    not_ready=0

    for yaml_file in "${ACTION_TYPES_DIR}"/*.yaml; do
        name=$(basename "$yaml_file" .yaml)
        echo -n "  ${name}: "

        if kubectl wait --for=jsonpath='{.status.registered}'=true \
            "actiontype/${name}" -n kubernaut-system \
            --timeout="${WAIT_TIMEOUT}s" 2>/dev/null; then
            echo "REGISTERED"
            registered=$((registered + 1))
        else
            echo "NOT READY (timeout)"
            not_ready=$((not_ready + 1))
        fi
    done

    echo ""
    echo "==> Registration: ${registered} registered, ${not_ready} not ready"
fi

if [ "$fail_count" -gt 0 ]; then
    echo "    Failed ActionTypes:"
    for name in "${failed_names[@]}"; do
        echo "      - ${name}"
    done
fi

echo "==> Verify: kubectl get at -n kubernaut-system"

[ "$fail_count" -eq 0 ]
