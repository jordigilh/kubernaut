#!/usr/bin/env bash
# VHS tape setup: cleanup + stabilization window + deploy
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

bash "${SCRIPT_DIR}/cleanup.sh" 2>/dev/null || true

kubectl get configmap remediationorchestrator-config -n "${PLATFORM_NS}" -o yaml \
  | sed 's/stabilizationWindow: "5m"/stabilizationWindow: "1m"/' \
  | kubectl apply -f - >/dev/null 2>&1
kubectl rollout restart deploy/remediationorchestrator-controller -n "${PLATFORM_NS}" >/dev/null 2>&1
kubectl rollout status deploy/remediationorchestrator-controller -n "${PLATFORM_NS}" --timeout=120s >/dev/null 2>&1

kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml" >/dev/null 2>&1
kubectl apply -f "${SCRIPT_DIR}/manifests/" >/dev/null 2>&1
kubectl wait --for=condition=Available deployment/web-service -n demo-node --timeout=180s >/dev/null 2>&1
sleep 20
