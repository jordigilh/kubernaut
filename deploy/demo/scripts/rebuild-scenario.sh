#!/usr/bin/env bash
# Unified scenario rebuild orchestrator
#
# Rebuilds workflow images (exec + schema) and/or platform service images
# for a single demo scenario. Eliminates the need to rebuild everything.
#
# Usage:
#   ./rebuild-scenario.sh --scenario crashloop                             # workflow only
#   ./rebuild-scenario.sh --scenario crashloop --services aianalysis,holmesgpt-api  # workflow + services
#   ./rebuild-scenario.sh --scenario crashloop --seed                      # workflow + register in DataStorage
#   ./rebuild-scenario.sh --services aianalysis,holmesgpt-api              # services only (no workflow)
#   ./rebuild-scenario.sh --scenario crashloop --workflow-only             # explicit: workflow, no services
#   ./rebuild-scenario.sh --scenario crashloop --local                     # local build (no push)
#
# Prerequisites:
#   - podman login quay.io (for push)
#   - make (for service image builds)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

SCENARIO=""
SERVICES=""
WORKFLOW_ONLY=false
LOCAL_ONLY=false
SEED_AFTER=false
CREATE_MANIFEST=false
IMAGE_TAG="${IMAGE_TAG:-demo-v1.0}"
VERSION="${VERSION:-v1.0.0}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scenario)
            SCENARIO="$2"
            shift 2
            ;;
        --services)
            SERVICES="$2"
            shift 2
            ;;
        --workflow-only)
            WORKFLOW_ONLY=true
            shift
            ;;
        --local)
            LOCAL_ONLY=true
            shift
            ;;
        --seed)
            SEED_AFTER=true
            shift
            ;;
        --manifest)
            CREATE_MANIFEST=true
            shift
            ;;
        --image-tag)
            IMAGE_TAG="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--scenario NAME] [--services svc1,svc2] [--workflow-only] [--local] [--seed] [--manifest]"
            echo ""
            echo "Options:"
            echo "  --scenario NAME        Build workflow images for this scenario"
            echo "  --services svc1,svc2   Rebuild specific platform service images (comma-separated)"
            echo "  --workflow-only         Only build workflow images (skip services even if --services given)"
            echo "  --local                Local build only (no push to registry)"
            echo "  --seed                 Register workflow in DataStorage after push"
            echo "  --manifest             Create multi-arch manifests for service images (requires both arches pushed)"
            echo "  --image-tag TAG        Platform service image tag (default: demo-v1.0)"
            echo "  --version TAG          Workflow image version tag (default: v1.0.0)"
            echo ""
            echo "Available services: datastorage gateway aianalysis authwebhook notification"
            echo "                    remediationorchestrator signalprocessing workflowexecution"
            echo "                    effectivenessmonitor holmesgpt-api"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

if [ -z "$SCENARIO" ] && [ -z "$SERVICES" ]; then
    echo "ERROR: At least --scenario or --services is required."
    echo "Run with --help for usage."
    exit 1
fi

echo "============================================"
echo "Scenario Rebuild"
echo "============================================"
[ -n "$SCENARIO" ] && echo "Scenario:  ${SCENARIO}"
[ -n "$SERVICES" ] && echo "Services:  ${SERVICES}"
echo "Mode:      $(if $LOCAL_ONLY; then echo 'LOCAL ONLY'; else echo 'BUILD + PUSH'; fi)"
echo ""

# Step 1: Build workflow images (exec + schema)
if [ -n "$SCENARIO" ]; then
    echo "==> Building workflow images for scenario: ${SCENARIO}"
    WORKFLOW_ARGS=(--scenario "${SCENARIO}" --version "${VERSION}")
    if $LOCAL_ONLY; then
        WORKFLOW_ARGS+=(--local)
    fi
    if $SEED_AFTER; then
        WORKFLOW_ARGS+=(--seed)
    fi
    bash "${SCRIPT_DIR}/build-demo-workflows.sh" "${WORKFLOW_ARGS[@]}"
    echo ""
fi

# Step 2: Build platform service images
if [ -n "$SERVICES" ] && ! $WORKFLOW_ONLY; then
    echo "==> Building platform service images: ${SERVICES}"
    IFS=',' read -ra SVC_LIST <<< "$SERVICES"
    for svc in "${SVC_LIST[@]}"; do
        svc=$(echo "$svc" | xargs)  # trim whitespace
        echo "  --- ${svc} ---"

        make -C "${REPO_ROOT}" "image-build-${svc}" IMAGE_TAG="${IMAGE_TAG}"

        if ! $LOCAL_ONLY; then
            make -C "${REPO_ROOT}" "image-push-${svc}" IMAGE_TAG="${IMAGE_TAG}"
            if $CREATE_MANIFEST; then
                make -C "${REPO_ROOT}" "image-manifest-${svc}" IMAGE_TAG="${IMAGE_TAG}"
            fi
        fi
        echo ""
    done
fi

echo "============================================"
echo "Rebuild complete."
echo "============================================"
