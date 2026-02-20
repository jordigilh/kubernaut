#!/usr/bin/env bash
# Build and push demo scenario workflow images to quay.io/kubernaut-cicd/test-workflows
#
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
# ADR-043: OCI images include /workflow-schema.yaml for catalog registration
#
# Usage:
#   ./build-demo-workflows.sh                    # Build and push multi-arch (amd64 + arm64)
#   ./build-demo-workflows.sh --local            # Build local-only (no push, current arch)
#   ./build-demo-workflows.sh --scenario NAME    # Build a single scenario
#   ./build-demo-workflows.sh --local --scenario gitops-drift
#
# Prerequisites:
#   - podman login quay.io (for push)
#   - podman with multi-arch manifest support
#
# Images built:
#   quay.io/kubernaut-cicd/test-workflows/git-revert-job:v1.0.0          (#125)
#   quay.io/kubernaut-cicd/test-workflows/provision-node-job:v1.0.0      (#126)
#   quay.io/kubernaut-cicd/test-workflows/proactive-rollback-job:v1.0.0  (#128)
#   quay.io/kubernaut-cicd/test-workflows/graceful-restart-job:v1.0.0    (#129)
#   quay.io/kubernaut-cicd/test-workflows/crashloop-rollback-job:v1.0.0 (#120)
#   quay.io/kubernaut-cicd/test-workflows/patch-hpa-job:v1.0.0         (#123)
#   quay.io/kubernaut-cicd/test-workflows/relax-pdb-job:v1.0.0        (#124)
#   quay.io/kubernaut-cicd/test-workflows/remove-taint-job:v1.0.0     (#122)
#   quay.io/kubernaut-cicd/test-workflows/cleanup-pvc-job:v1.0.0      (#121)
#   quay.io/kubernaut-cicd/test-workflows/cordon-drain-job:v1.0.0     (#127)
#   quay.io/kubernaut-cicd/test-workflows/rollback-deployment-job:v1.0.0 (#130)
#   quay.io/kubernaut-cicd/test-workflows/fix-certificate-job:v1.0.0  (#133)
#   quay.io/kubernaut-cicd/test-workflows/fix-certificate-gitops-job:v1.0.0 (#134)
#   quay.io/kubernaut-cicd/test-workflows/helm-rollback-job:v1.0.0    (#135)
#   quay.io/kubernaut-cicd/test-workflows/fix-authz-policy-job:v1.0.0 (#136)
#   quay.io/kubernaut-cicd/test-workflows/fix-statefulset-pvc-job:v1.0.0 (#137)
#   quay.io/kubernaut-cicd/test-workflows/fix-network-policy-job:v1.0.0 (#138)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="${SCRIPT_DIR}/../scenarios"
REGISTRY="quay.io/kubernaut-cicd/test-workflows"
VERSION="v1.0.0"
LOCAL_ONLY=false
SINGLE_SCENARIO=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --local)
            LOCAL_ONLY=true
            shift
            ;;
        --scenario)
            SINGLE_SCENARIO="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--local] [--scenario NAME] [--version TAG]"
            echo ""
            echo "Options:"
            echo "  --local            Build for current arch only (no push)"
            echo "  --scenario NAME    Build a single scenario (e.g., gitops-drift)"
            echo "  --version TAG      Override version tag (default: v1.0.0)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "============================================"
echo "Building Demo Scenario Workflow Images"
echo "============================================"
echo "Registry: ${REGISTRY}"
echo "Version:  ${VERSION}"
echo "Mode:     $(if $LOCAL_ONLY; then echo 'LOCAL ONLY (current arch)'; else echo 'MULTI-ARCH (amd64 + arm64) + PUSH'; fi)"
if [ -n "$SINGLE_SCENARIO" ]; then
    echo "Scenario: ${SINGLE_SCENARIO}"
fi
echo ""

# scenario-dir:image-name mappings
# Format: "scenario_directory:image_name"
WORKFLOWS=(
    "gitops-drift:git-revert-job"
    "autoscale:provision-node-job"
    "slo-burn:proactive-rollback-job"
    "memory-leak:graceful-restart-job"
    "crashloop:crashloop-rollback-job"
    "hpa-maxed:patch-hpa-job"
    "pdb-deadlock:relax-pdb-job"
    "pending-taint:remove-taint-job"
    "disk-pressure:cleanup-pvc-job"
    "node-notready:cordon-drain-job"
    "stuck-rollout:rollback-deployment-job"
    "cert-failure:fix-certificate-job"
    "cert-failure-gitops:fix-certificate-gitops-job"
    "crashloop-helm:helm-rollback-job"
    "mesh-routing-failure:fix-authz-policy-job"
    "statefulset-pvc-failure:fix-statefulset-pvc-job"
    "network-policy-block:fix-network-policy-job"
)

build_count=0
skip_count=0

for entry in "${WORKFLOWS[@]}"; do
    SCENARIO="${entry%%:*}"
    IMAGE_NAME="${entry#*:}"
    BUILD_DIR="${SCENARIOS_DIR}/${SCENARIO}/workflow"
    FULL_REF="${REGISTRY}/${IMAGE_NAME}:${VERSION}"

    if [ -n "$SINGLE_SCENARIO" ] && [ "$SCENARIO" != "$SINGLE_SCENARIO" ]; then
        skip_count=$((skip_count + 1))
        continue
    fi

    if [ ! -f "${BUILD_DIR}/Dockerfile" ]; then
        echo "SKIP: ${SCENARIO} -- no Dockerfile at ${BUILD_DIR}/Dockerfile"
        skip_count=$((skip_count + 1))
        continue
    fi

    if [ ! -f "${BUILD_DIR}/workflow-schema.yaml" ]; then
        echo "ERROR: ${SCENARIO} -- missing workflow-schema.yaml (required by ADR-043)"
        exit 1
    fi

    echo "Building ${IMAGE_NAME} (scenario: ${SCENARIO})..."
    echo "  Context: ${BUILD_DIR}"
    echo "  Image:   ${FULL_REF}"

    if $LOCAL_ONLY; then
        podman build -t "${FULL_REF}" "${BUILD_DIR}"
        echo "  Built (local arch only)"
    else
        podman manifest rm "${FULL_REF}" 2>/dev/null || true
        podman manifest create "${FULL_REF}"

        podman build --platform linux/amd64 -t "${FULL_REF}-amd64" "${BUILD_DIR}"
        podman build --platform linux/arm64 -t "${FULL_REF}-arm64" "${BUILD_DIR}"

        podman manifest add "${FULL_REF}" "${FULL_REF}-amd64"
        podman manifest add "${FULL_REF}" "${FULL_REF}-arm64"

        podman manifest push "${FULL_REF}" "docker://${FULL_REF}"
        echo "  Pushed multi-arch manifest (amd64 + arm64)"
    fi

    build_count=$((build_count + 1))
    echo ""
done

echo "============================================"
echo "Built: ${build_count} workflow image(s)"
if [ "$skip_count" -gt 0 ]; then
    echo "Skipped: ${skip_count}"
fi
if ! $LOCAL_ONLY; then
    echo "Pushed to: ${REGISTRY}"
fi
echo "============================================"

echo ""
echo "Next steps:"
echo "  1. Update workflow-schema.yaml bundle digests with:"
echo "     podman manifest inspect ${REGISTRY}/<image>:${VERSION} | jq -r '.digest'"
echo "  2. Seed the workflows in DataStorage:"
echo "     ./deploy/demo/scripts/seed-workflows.sh"
