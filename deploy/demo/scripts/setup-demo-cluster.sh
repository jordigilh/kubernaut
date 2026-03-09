#!/usr/bin/env bash
# Setup the full Kubernaut demo environment in a Kind cluster.
#
# Installs: Kind cluster, monitoring stack, Kubernaut platform (Helm),
# infrastructure dependencies (cert-manager, metrics-server, Linkerd,
# blackbox-exporter, Gitea, ArgoCD), and seeds the workflow catalog.
#
# Usage:
#   ./deploy/demo/scripts/setup-demo-cluster.sh
#   ./deploy/demo/scripts/setup-demo-cluster.sh --create-cluster
#   ./deploy/demo/scripts/setup-demo-cluster.sh --skip-infra
#   ./deploy/demo/scripts/setup-demo-cluster.sh --kind-config path/to/config.yaml
#
# After setup, run any scenario directly:
#   ./deploy/demo/scenarios/crashloop/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="${SCRIPT_DIR}/../scenarios"

CREATE_FLAG=""
SKIP_INFRA=false
KIND_CONFIG="${SCENARIOS_DIR}/kind-config-multinode.yaml"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --create-cluster)
            CREATE_FLAG="--create-cluster"
            shift
            ;;
        --skip-infra)
            SKIP_INFRA=true
            shift
            ;;
        --kind-config)
            KIND_CONFIG="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--create-cluster] [--skip-infra] [--kind-config PATH]"
            echo ""
            echo "Options:"
            echo "  --create-cluster   Force-recreate the Kind cluster (deletes existing)"
            echo "  --skip-infra       Skip optional infrastructure (cert-manager, Gitea, etc.)"
            echo "  --kind-config PATH Override Kind cluster config (default: singlenode)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

TOTAL_START=$(date +%s)

echo "============================================="
echo " Kubernaut Demo Environment Setup"
echo "============================================="
echo ""

# ── 1. Kind cluster ─────────────────────────────────────────────────────────

echo "==> Phase 1: Kind cluster"
# shellcheck source=kind-helper.sh
source "${SCRIPT_DIR}/kind-helper.sh"
ensure_kind_cluster "${KIND_CONFIG}" "${CREATE_FLAG}"
echo ""

# ── 2. Monitoring stack ─────────────────────────────────────────────────────

echo "==> Phase 2: Monitoring stack"
# shellcheck source=monitoring-helper.sh
source "${SCRIPT_DIR}/monitoring-helper.sh"
ensure_monitoring_stack
echo ""

# ── 3. Infrastructure dependencies (non-Kubernaut) ──────────────────────────

if [ "$SKIP_INFRA" = false ]; then
    echo "==> Phase 3: Infrastructure dependencies"

    echo "--- cert-manager ---"
    ensure_cert_manager
    echo ""

    echo "--- metrics-server ---"
    ensure_metrics_server
    echo ""

    echo "--- Linkerd ---"
    ensure_linkerd
    echo ""

    echo "--- blackbox-exporter ---"
    ensure_blackbox_exporter
    echo ""
else
    echo "==> Phase 3: Skipping infrastructure dependencies (--skip-infra)"
    echo ""
fi

# ── 4. Kubernaut platform ───────────────────────────────────────────────────
# Install before Gitea/ArgoCD because the Helm chart creates the
# kubernaut-workflows namespace that ArgoCD setup provisions secrets into.

echo "==> Phase 4: Kubernaut platform (Helm)"
# shellcheck source=platform-helper.sh
source "${SCRIPT_DIR}/platform-helper.sh"
ensure_platform
echo ""

# ── 4b. GitOps infrastructure (depends on kubernaut-workflows namespace) ────

if [ "$SKIP_INFRA" = false ]; then
    echo "==> Phase 4b: GitOps infrastructure"

    echo "--- Gitea ---"
    if kubectl get namespace gitea &>/dev/null; then
        echo "  Gitea already installed."
    else
        bash "${SCENARIOS_DIR}/gitops/scripts/setup-gitea.sh"
    fi
    echo ""

    echo "--- ArgoCD ---"
    if kubectl get namespace argocd &>/dev/null; then
        echo "  ArgoCD already installed."
    else
        bash "${SCENARIOS_DIR}/gitops/scripts/setup-argocd.sh"
    fi
    echo ""
fi

# ── 5. Seed action types + workflow catalog ─────────────────────────────────

echo "==> Phase 5a: Seeding ActionType CRDs (must exist before workflows)"
bash "${SCRIPT_DIR}/seed-action-types.sh" --continue-on-error --skip-wait
echo ""

echo "==> Phase 5b: Seeding workflow catalog"

DS_PORT_FORWARD_PID=""
cleanup_port_forward() {
    if [ -n "$DS_PORT_FORWARD_PID" ]; then
        kill "$DS_PORT_FORWARD_PID" 2>/dev/null || true
    fi
}
trap cleanup_port_forward EXIT

if ! curl -sf -o /dev/null --connect-timeout 2 "http://localhost:30081/health" 2>/dev/null; then
    echo "  Starting DataStorage port-forward..."
    kubectl port-forward -n kubernaut-system svc/data-storage-service 30081:8080 >/dev/null 2>&1 &
    DS_PORT_FORWARD_PID=$!
    sleep 3
fi

bash "${SCRIPT_DIR}/seed-workflows.sh" --continue-on-error
echo ""

# ── 6. Final validation ────────────────────────────────────────────────────

echo "==> Phase 6: Final readiness validation"
echo ""

NAMESPACES=("kubernaut-system" "monitoring")
if [ "$SKIP_INFRA" = false ]; then
    NAMESPACES+=("cert-manager" "linkerd" "gitea" "argocd")
fi

all_ready=true
for ns in "${NAMESPACES[@]}"; do
    if ! kubectl get namespace "$ns" &>/dev/null; then
        echo "  WARNING: namespace ${ns} does not exist"
        all_ready=false
        continue
    fi
    local_deps=$(kubectl get deployments -n "$ns" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)
    if [ -z "$local_deps" ]; then
        continue
    fi
    for dep in $local_deps; do
        ready=$(kubectl get deployment "$dep" -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
        desired=$(kubectl get deployment "$dep" -n "$ns" -o jsonpath='{.spec.replicas}' 2>/dev/null)
        if [ "${ready:-0}" != "${desired:-1}" ]; then
            echo "  WARNING: ${ns}/${dep} not ready (${ready:-0}/${desired:-1})"
            all_ready=false
        fi
    done
done

TOTAL_END=$(date +%s)
TOTAL_DURATION=$((TOTAL_END - TOTAL_START))
total_mins=$((TOTAL_DURATION / 60))
total_secs=$((TOTAL_DURATION % 60))

echo ""
echo "============================================="
if [ "$all_ready" = true ]; then
    echo " Demo environment ready! (${total_mins}m ${total_secs}s)"
else
    echo " Demo environment setup complete with warnings (${total_mins}m ${total_secs}s)"
fi
echo "============================================="
echo ""
echo "Run any scenario:"
echo "  bash deploy/demo/scenarios/crashloop/run.sh"
echo ""
echo "Or use the orchestrator:"
echo "  bash deploy/demo/scripts/run-scenario.sh --scenario crashloop --auto-approve"
