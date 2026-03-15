#!/usr/bin/env bash
# generate-image-list.sh — Extract all container images from the Kubernaut Helm chart.
#
# Outputs one image per line, suitable for feeding into oc mirror, skopeo, or
# an ImageSetConfiguration. Requires `helm` and optionally `yq` on PATH.
#
# Usage:
#   ./hack/airgap/generate-image-list.sh                           # default values
#   ./hack/airgap/generate-image-list.sh -f values-ocp.yaml        # with OCP overlay
#   ./hack/airgap/generate-image-list.sh --set global.image.tag=1.0.0

set -euo pipefail

CHART_DIR="$(cd "$(dirname "$0")/../../charts/kubernaut" && pwd)"

HELM_ARGS=("$@")

echo "# Kubernaut Helm chart image list" >&2
echo "# Chart: ${CHART_DIR}" >&2
echo "# Extra args: ${HELM_ARGS[*]:-<none>}" >&2
echo "#" >&2

helm template airgap-inventory "${CHART_DIR}" \
  --set postgresql.auth.password=placeholder \
  "${HELM_ARGS[@]}" 2>/dev/null |
  grep -E '^\s+image:\s+' |
  sed 's/.*image:\s*//' |
  sed 's/^"//' | sed 's/"$//' |
  sort -u
