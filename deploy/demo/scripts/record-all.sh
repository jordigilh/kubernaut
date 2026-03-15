#!/usr/bin/env bash
# Batch-record all demo scenarios using the three-tape architecture.
#
# Scenarios are grouped by infrastructure requirements. Within each group,
# scenarios are recorded sequentially (each cleans up before the next starts).
#
# Usage:
#   bash deploy/demo/scripts/record-all.sh              # Record all groups
#   bash deploy/demo/scripts/record-all.sh A             # Record only Group A
#   bash deploy/demo/scripts/record-all.sh A stuck-rollout  # Record one scenario from Group A
#
# After recording, splice each scenario:
#   bash deploy/demo/scripts/splice-demo.sh <scenario-name>
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
BASE="${REPO_ROOT}/deploy/demo/scenarios"

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/kubernaut-demo-config}"

GROUP_FILTER="${1:-ALL}"
SCENARIO_FILTER="${2:-}"

record_scenario() {
  local name="$1"
  local dir="${BASE}/${name}"
  if [ ! -f "${dir}/recording/record.sh" ]; then
    echo "SKIP: ${name} (no recording/record.sh)"
    return
  fi
  if [ -n "${SCENARIO_FILTER}" ] && [ "${SCENARIO_FILTER}" != "${name}" ]; then
    return
  fi
  echo ""
  echo "╔══════════════════════════════════════════════════╗"
  echo "  RECORDING: ${name}"
  echo "╚══════════════════════════════════════════════════╝"
  echo ""
  bash "${dir}/recording/record.sh"
  echo ""
  echo "  ✓ ${name} recording complete"
  echo ""
}

# ── Group A: Basic Kind ──────────────────────────────────
if [ "${GROUP_FILTER}" = "ALL" ] || [ "${GROUP_FILTER}" = "A" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  GROUP A — Basic Kind (9 scenarios)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  record_scenario stuck-rollout
  record_scenario crashloop-helm
  record_scenario slo-burn
  record_scenario hpa-maxed
  record_scenario statefulset-pvc-failure
  record_scenario network-policy-block
  record_scenario duplicate-alert-suppression
  record_scenario memory-leak
  record_scenario memory-escalation
fi

# ── Group B: Multi-Node Kind + Podman ────────────────────
if [ "${GROUP_FILTER}" = "ALL" ] || [ "${GROUP_FILTER}" = "B" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  GROUP B — Multi-Node Kind (4 scenarios)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  record_scenario pdb-deadlock
  record_scenario pending-taint
  record_scenario node-notready
  record_scenario autoscale
fi

# ── Group C: GitOps (Gitea + ArgoCD) ────────────────────
if [ "${GROUP_FILTER}" = "ALL" ] || [ "${GROUP_FILTER}" = "C" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  GROUP C — GitOps (2 scenarios)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  record_scenario cert-failure-gitops
  record_scenario gitops-drift
fi

# ── Group D: cert-manager ────────────────────────────────
if [ "${GROUP_FILTER}" = "ALL" ] || [ "${GROUP_FILTER}" = "D" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  GROUP D — cert-manager (1 scenario)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  record_scenario cert-failure
fi

# ── Group E: Linkerd ─────────────────────────────────────
if [ "${GROUP_FILTER}" = "ALL" ] || [ "${GROUP_FILTER}" = "E" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  GROUP E — Linkerd (1 scenario)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  record_scenario mesh-routing-failure
fi

# ── Group F: Escalation ──────────────────────────────────
if [ "${GROUP_FILTER}" = "ALL" ] || [ "${GROUP_FILTER}" = "F" ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  GROUP F — Escalation (2 scenarios)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  record_scenario orphaned-pvc-no-action
  record_scenario resource-quota-exhaustion
fi

echo ""
echo "════════════════════════════════════════════════════════"
echo "  All recordings complete!"
echo ""
echo "  Next: for each scenario, create splice.conf and run:"
echo "    bash deploy/demo/scripts/splice-demo.sh <scenario>"
echo "════════════════════════════════════════════════════════"
