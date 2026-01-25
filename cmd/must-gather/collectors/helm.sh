#!/bin/bash
# Copyright 2025 Jordi Gil
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Kubernaut Must-Gather - Helm Release Collector (Placeholder)
# BR-PLATFORM-001.6e: Collect Helm release information
# STATUS: V1.0 Placeholder - Helm charts not yet available

set -euo pipefail

COLLECTION_DIR="${1}"
HELM_DIR="${COLLECTION_DIR}/helm"

echo "Checking for Helm releases..."

mkdir -p "${HELM_DIR}"

# Check if Helm is installed
if ! command -v helm &> /dev/null; then
    echo "  Helm CLI not available in must-gather container"
    cat > "${HELM_DIR}/README.txt" <<'EOF'
Helm Collection - Not Available

Helm charts for Kubernaut are not yet available in V1.0.
Kubernaut V1.0 is deployed via Kustomize.

Helm collection will be enabled in a future release when
Helm charts are available.

Current deployment method: Kustomize
See: deploy/ directory in repository
EOF
    exit 0
fi

# Check for Kubernaut Helm releases in known namespaces
HELM_RELEASES=$(helm list --all-namespaces -o json 2>/dev/null | jq -r '.[] | select(.name | contains("kubernaut")) | .name' || echo "")

if [ -z "${HELM_RELEASES}" ]; then
    echo "  No Kubernaut Helm releases found (expected for V1.0 - deployed via Kustomize)"
    cat > "${HELM_DIR}/README.txt" <<'EOF'
Helm Collection - No Releases Found

No Kubernaut Helm releases detected in this cluster.

This is expected for Kubernaut V1.0, which is deployed via Kustomize.

If you believe Helm releases should be present, verify with:
  helm list --all-namespaces | grep kubernaut

Helm collection will be fully implemented when Kubernaut Helm
charts become available.
EOF
    exit 0
fi

# If Helm releases are found (future), collect them
echo "  Found Kubernaut Helm releases, collecting..."

helm list --all-namespaces -o json | jq '.[] | select(.name | contains("kubernaut"))' \
    > "${HELM_DIR}/helm-releases.json" 2>/dev/null || true

while IFS= read -r release; do
    NAMESPACE=$(echo "${release}" | jq -r '.namespace')
    NAME=$(echo "${release}" | jq -r '.name')

    echo "    Collecting Helm release: ${NAME} (${NAMESPACE})"

    # Release history
    helm history "${NAME}" -n "${NAMESPACE}" -o json \
        > "${HELM_DIR}/${NAME}-history.json" 2>/dev/null || true

    # Release values
    helm get values "${NAME}" -n "${NAMESPACE}" -o yaml \
        > "${HELM_DIR}/${NAME}-values.yaml" 2>/dev/null || true

    # Release manifest
    helm get manifest "${NAME}" -n "${NAMESPACE}" \
        > "${HELM_DIR}/${NAME}-manifest.yaml" 2>/dev/null || true
done <<< "${HELM_RELEASES}"

echo "Helm collection complete"

