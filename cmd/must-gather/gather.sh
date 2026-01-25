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

# Kubernaut Must-Gather - Main Collection Script
# BR-PLATFORM-001: Enterprise diagnostic collection
# Usage: /usr/bin/gather [--since=24h] [--dest-dir=/must-gather] [--no-sanitize]

set -euo pipefail

# Script directory
SCRIPT_DIR="/usr/share/must-gather"
COLLECTORS_DIR="${SCRIPT_DIR}/collectors"
SANITIZERS_DIR="${SCRIPT_DIR}/sanitizers"
UTILS_DIR="${SCRIPT_DIR}/utils"

# Default configuration
SINCE_DURATION="24h"
DEST_DIR="/must-gather"
SANITIZE_ENABLED="true"
MAX_SIZE_MB=500
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
COLLECTION_NAME="kubernaut-must-gather-${TIMESTAMP}"

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --since=*)
            SINCE_DURATION="${1#*=}"
            shift
            ;;
        --dest-dir=*)
            DEST_DIR="${1#*=}"
            shift
            ;;
        --no-sanitize)
            SANITIZE_ENABLED="false"
            shift
            ;;
        --max-size=*)
            MAX_SIZE_MB="${1#*=}"
            shift
            ;;
        --help|-h)
            echo "Kubernaut Must-Gather Diagnostic Collection Tool"
            echo ""
            echo "Usage: gather [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --since=DURATION      Log collection timeframe (default: 24h)"
            echo "  --dest-dir=PATH       Output directory (default: /must-gather)"
            echo "  --no-sanitize         Disable automatic sanitization (internal use only)"
            echo "  --max-size=MB         Maximum collection size in MB (default: 500)"
            echo "  --help, -h            Show this help message"
            echo ""
            echo "Examples:"
            echo "  gather                           # Default collection (24h logs)"
            echo "  gather --since=48h               # Collect last 48 hours"
            echo "  gather --dest-dir=/tmp/diagnostics"
            exit 0
            ;;
        *)
            echo "Error: Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Kubernaut namespaces (authoritative list - 3 namespaces in V1.0)
# Source: DD-INFRA-001, DD-WE-002
KUBERNAUT_NAMESPACES=(
    "kubernaut-system"        # Core services + CRD controllers
    "kubernaut-notifications" # Notification controller (isolated)
    "kubernaut-workflows"     # Tekton PipelineRuns execution
)

# Export configuration for collectors
export SINCE_DURATION
export DEST_DIR
export SANITIZE_ENABLED
export MAX_SIZE_MB
export KUBERNAUT_NAMESPACES
export COLLECTION_NAME

# Create collection directory structure
COLLECTION_DIR="${DEST_DIR}/${COLLECTION_NAME}"
mkdir -p "${COLLECTION_DIR}"/{cluster-scoped,namespaces,crds,events,datastorage,metrics,logs}

echo "=========================================="
echo "Kubernaut Must-Gather Diagnostic Collection"
echo "=========================================="
echo "Version: 1.0.0"
echo "Collection: ${COLLECTION_NAME}"
echo "Timestamp: $(date --rfc-3339=seconds)"
echo "Since: ${SINCE_DURATION}"
echo "Sanitization: ${SANITIZE_ENABLED}"
echo "Max Size: ${MAX_SIZE_MB}MB"
echo "Namespaces: ${KUBERNAUT_NAMESPACES[*]}"
echo "=========================================="
echo ""

# Capture start time
START_TIME=$(date +%s)

# Version information
echo "Collecting version information..."
kubectl version --output=yaml > "${COLLECTION_DIR}/version-info.yaml" 2>/dev/null || true

# Execute collectors in order
# BR-PLATFORM-001: Collection order optimized for diagnostics

echo ""
echo "Phase 1: CRD Collection (6 types)..."
bash "${COLLECTORS_DIR}/crds.sh" "${COLLECTION_DIR}" || echo "Warning: CRD collection had errors"

echo ""
echo "Phase 2: Service Logs Collection (8 services)..."
bash "${COLLECTORS_DIR}/logs.sh" "${COLLECTION_DIR}" || echo "Warning: Log collection had errors"

echo ""
echo "Phase 3: Kubernetes Events..."
bash "${COLLECTORS_DIR}/events.sh" "${COLLECTION_DIR}" || echo "Warning: Event collection had errors"

echo ""
echo "Phase 4: Cluster State (RBAC, Storage, Network)..."
bash "${COLLECTORS_DIR}/cluster-state.sh" "${COLLECTION_DIR}" || echo "Warning: Cluster state collection had errors"

echo ""
echo "Phase 5: Tekton Resources (PipelineRuns, TaskRuns)..."
bash "${COLLECTORS_DIR}/tekton.sh" "${COLLECTION_DIR}" || echo "Warning: Tekton collection had errors"

echo ""
echo "Phase 6: DataStorage REST API (workflows, audit events)..."
bash "${COLLECTORS_DIR}/datastorage.sh" "${COLLECTION_DIR}" || echo "Warning: DataStorage API collection had errors"

echo ""
echo "Phase 7: Database Infrastructure (PostgreSQL, Redis)..."
bash "${COLLECTORS_DIR}/database.sh" "${COLLECTION_DIR}" || echo "Warning: Database collection had errors"

echo ""
echo "Phase 8: Metrics Collection..."
bash "${COLLECTORS_DIR}/metrics.sh" "${COLLECTION_DIR}" || echo "Warning: Metrics collection had errors"

# Sanitization (if enabled)
if [ "${SANITIZE_ENABLED}" = "true" ]; then
    echo ""
    echo "Phase 9: Sanitizing sensitive data..."
    bash "${SANITIZERS_DIR}/sanitize-all.sh" "${COLLECTION_DIR}" || echo "Warning: Sanitization had errors"
fi

# Generate collection metadata
echo ""
echo "Generating collection metadata..."
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

cat > "${COLLECTION_DIR}/collection-metadata.json" <<EOF
{
  "collection_time": "$(date --rfc-3339=seconds)",
  "kubernaut_version": "v1.0.0",
  "must_gather_version": "v1.0.0",
  "kubernetes_version": "$(kubectl version --short 2>/dev/null | grep Server | cut -d' ' -f3 || echo 'unknown')",
  "cluster_name": "$(kubectl config current-context 2>/dev/null || echo 'unknown')",
  "collection_duration_seconds": ${DURATION},
  "namespaces_collected": $(printf '%s\n' "${KUBERNAUT_NAMESPACES[@]}" | jq -R . | jq -s .),
  "sanitization_enabled": ${SANITIZE_ENABLED},
  "collection_flags": ["--since=${SINCE_DURATION}"],
  "errors": []
}
EOF

# Generate checksums
echo ""
echo "Generating SHA256 checksums..."
bash "${UTILS_DIR}/checksum.sh" "${COLLECTION_DIR}" || echo "Warning: Checksum generation had errors"

# Package into tarball
echo ""
echo "Creating compressed archive..."
ARCHIVE_NAME="${COLLECTION_NAME}.tar.gz"
tar -czf "${DEST_DIR}/${ARCHIVE_NAME}" -C "${DEST_DIR}" "${COLLECTION_NAME}" 2>/dev/null || {
    echo "Warning: Failed to create archive, leaving uncompressed directory"
}

# Final summary
ARCHIVE_SIZE=$(du -h "${DEST_DIR}/${ARCHIVE_NAME}" 2>/dev/null | cut -f1 || echo "N/A")
echo ""
echo "=========================================="
echo "Collection Complete!"
echo "=========================================="
echo "Archive: ${DEST_DIR}/${ARCHIVE_NAME}"
echo "Size: ${ARCHIVE_SIZE}"
echo "Duration: ${DURATION} seconds"
echo "Checksums: ${COLLECTION_DIR}/SHA256SUMS"
echo ""
echo "To extract:"
echo "  tar -xzf ${ARCHIVE_NAME}"
echo ""
echo "To verify checksums:"
echo "  cd ${COLLECTION_NAME} && sha256sum -c SHA256SUMS"
echo "=========================================="

exit 0

