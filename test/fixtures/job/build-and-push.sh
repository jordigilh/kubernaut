#!/usr/bin/env bash
# Build and push Job backend test workflow images to quay.io/kubernaut-cicd
#
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
# ADR-043: OCI images include /workflow-schema.yaml for catalog compliance
#
# Usage:
#   ./build-and-push.sh              # Build and push multi-arch (amd64 + arm64)
#   ./build-and-push.sh --local      # Build local-only (no push, current arch only)
#
# Prerequisites:
#   - podman login quay.io (for push)
#   - podman with multi-arch manifest support

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY="quay.io/kubernaut-cicd/test-workflows"
VERSION="v1.0.0"
LOCAL_ONLY=false

if [[ "${1:-}" == "--local" ]]; then
    LOCAL_ONLY=true
fi

echo "============================================"
echo "Building Job Backend Test Workflow Images"
echo "============================================"
echo "Registry: ${REGISTRY}"
echo "Version:  ${VERSION}"
echo "Mode:     $(if $LOCAL_ONLY; then echo 'LOCAL ONLY'; else echo 'MULTI-ARCH + PUSH'; fi)"
echo ""

# Image definitions
IMAGES=(
    "job-hello-world:${SCRIPT_DIR}/hello-world"
    "job-failing:${SCRIPT_DIR}/failing"
)

for entry in "${IMAGES[@]}"; do
    IMAGE_NAME="${entry%%:*}"
    BUILD_DIR="${entry#*:}"
    FULL_REF="${REGISTRY}/${IMAGE_NAME}:${VERSION}"

    echo "Building ${IMAGE_NAME}..."
    echo "  Context: ${BUILD_DIR}"
    echo "  Image:   ${FULL_REF}"

    if $LOCAL_ONLY; then
        # Build for current architecture only (fast, for local testing)
        podman build -t "${FULL_REF}" "${BUILD_DIR}"
        echo "  Built (local arch only)"
    else
        # Build and push multi-arch manifest (amd64 + arm64)
        # Consistent with Tekton bundle multi-arch strategy
        podman manifest create "${FULL_REF}" 2>/dev/null || podman manifest rm "${FULL_REF}" && podman manifest create "${FULL_REF}"

        podman build --platform linux/amd64 -t "${FULL_REF}-amd64" "${BUILD_DIR}"
        podman build --platform linux/arm64 -t "${FULL_REF}-arm64" "${BUILD_DIR}"

        podman manifest add "${FULL_REF}" "${FULL_REF}-amd64"
        podman manifest add "${FULL_REF}" "${FULL_REF}-arm64"

        podman manifest push "${FULL_REF}" "docker://${FULL_REF}"
        echo "  Pushed multi-arch manifest"
    fi
    echo ""
done

echo "============================================"
echo "All Job test workflow images built"
if ! $LOCAL_ONLY; then
    echo "Pushed to: ${REGISTRY}"
fi
echo "============================================"
