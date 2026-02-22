#!/usr/bin/env bash
# Build and push demo scenario workflow images to quay.io/kubernaut-cicd/test-workflows
#
# Two-image split per scenario (eliminates circular digest dependency):
#   1. Execution image (<name>:v1.0.0)        -- remediate.sh + tools, run by WE as K8s Job
#   2. Schema image    (<name>-schema:v1.0.0)  -- workflow-schema.yaml only, pulled by DataStorage
#
# The exec image is built first, pushed, and its manifest list digest is embedded
# into workflow-schema.yaml before building the schema image.
#
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
# ADR-043: OCI images include /workflow-schema.yaml for catalog registration
#
# Usage:
#   ./build-demo-workflows.sh                    # Build and push multi-arch (amd64 + arm64)
#   ./build-demo-workflows.sh --local            # Build local-only (no push, current arch)
#   ./build-demo-workflows.sh --scenario NAME    # Build a single scenario
#   ./build-demo-workflows.sh --scenario crashloop --seed
#
# Prerequisites:
#   - podman login quay.io (for push)
#   - podman with multi-arch manifest support
#   - jq (for digest extraction)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="${SCRIPT_DIR}/../scenarios"
SCHEMA_DOCKERFILE="${SCENARIOS_DIR}/Dockerfile.schema"
REGISTRY="quay.io/kubernaut-cicd/test-workflows"
VERSION="v1.0.0"
LOCAL_ONLY=false
SINGLE_SCENARIO=""
SEED_AFTER=false

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
        --seed)
            SEED_AFTER=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--local] [--scenario NAME] [--version TAG] [--seed]"
            echo ""
            echo "Options:"
            echo "  --local            Build for current arch only (no push)"
            echo "  --scenario NAME    Build a single scenario (e.g., crashloop)"
            echo "  --version TAG      Override version tag (default: v1.0.0)"
            echo "  --seed             Register workflow(s) in DataStorage after push"
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
with open(f, 'w') as fh: fh.write(content)
" "${schema_file}" "${new_bundle}"
}

# build_and_push_multiarch builds both arch images, creates a manifest list, pushes it,
# and prints the manifest list digest to stdout.
# All podman build/push output goes to stderr so only the digest reaches stdout.
# Args: $1=full_ref $2=dockerfile $3=context_dir
build_and_push_multiarch() {
    local ref="$1" dockerfile="$2" context="$3"

    podman manifest rm "${ref}" &>/dev/null || true
    podman manifest create "${ref}" >/dev/null

    podman build --platform linux/amd64 -t "${ref}-amd64" -f "${dockerfile}" "${context}" >&2
    podman build --platform linux/arm64 -t "${ref}-arm64" -f "${dockerfile}" "${context}" >&2

    podman manifest add "${ref}" "${ref}-amd64" >/dev/null
    podman manifest add "${ref}" "${ref}-arm64" >/dev/null

    podman manifest push "${ref}" "docker://${ref}" >&2

    # Compute manifest list digest from the registry (skopeo returns raw bytes, hash them)
    skopeo inspect --raw "docker://${ref}" 2>/dev/null | \
        python3 -c "import sys,hashlib; data=sys.stdin.buffer.read(); print('sha256:'+hashlib.sha256(data).hexdigest())"
}

# build_local builds for the current arch only and prints the image digest.
# Args: $1=full_ref $2=dockerfile $3=context_dir
build_local() {
    local ref="$1" dockerfile="$2" context="$3"
    podman build -t "${ref}" -f "${dockerfile}" "${context}" >&2
    podman inspect "${ref}" --format '{{.Digest}}' 2>/dev/null || echo ""
}

# update_bundle_digest writes the exec image digest into workflow-schema.yaml
# Replaces the bundle line precisely, discarding any trailing garbage.
# Args: $1=schema_file $2=registry/image_name $3=digest
update_bundle_digest() {
    local schema_file="$1" image_ref="$2" digest="$3"
    local new_bundle="${image_ref}@${digest}"
    python3 -c "
import re, sys
f, new = sys.argv[1], sys.argv[2]
with open(f) as fh: content = fh.read()
# Replace the bundle line and any non-YAML garbage that may follow it
# (from previous broken runs where build output leaked into the file).
# Match from 'bundle: ...' up to the next blank line or YAML key.
content = re.sub(
    r'(  bundle: ).*?(?=\n\n|\nparameters:|\ndetectedLabels:|\Z)',
    r'\g<1>' + new,
    content,
    flags=re.DOTALL
)
with open(f, 'w') as fh: fh.write(content)
" "${schema_file}" "${new_bundle}"
}

build_count=0
skip_count=0
seeded_schemas=()

for entry in "${WORKFLOWS[@]}"; do
    SCENARIO="${entry%%:*}"
    IMAGE_NAME="${entry#*:}"
    BUILD_DIR="${SCENARIOS_DIR}/${SCENARIO}/workflow"
    EXEC_REF="${REGISTRY}/${IMAGE_NAME}:${VERSION}"
    SCHEMA_REF="${REGISTRY}/${IMAGE_NAME}-schema:${VERSION}"
    SCHEMA_FILE="${BUILD_DIR}/workflow-schema.yaml"

    if [ -n "$SINGLE_SCENARIO" ] && [ "$SCENARIO" != "$SINGLE_SCENARIO" ]; then
        skip_count=$((skip_count + 1))
        continue
    fi

    if [ ! -f "${BUILD_DIR}/Dockerfile.exec" ]; then
        echo "SKIP: ${SCENARIO} -- no Dockerfile.exec at ${BUILD_DIR}/Dockerfile.exec"
        skip_count=$((skip_count + 1))
        continue
    fi

    if [ ! -f "${SCHEMA_FILE}" ]; then
        echo "ERROR: ${SCENARIO} -- missing workflow-schema.yaml (required by ADR-043)"
        exit 1
    fi

    echo "==> ${IMAGE_NAME} (scenario: ${SCENARIO})"

    # Step 1: Build and push execution image
    echo "  [exec] Building ${EXEC_REF}..."
    if $LOCAL_ONLY; then
        EXEC_DIGEST=$(build_local "${EXEC_REF}" "${BUILD_DIR}/Dockerfile.exec" "${BUILD_DIR}")
        echo "  [exec] Built (local arch only)"
    else
        EXEC_DIGEST=$(build_and_push_multiarch "${EXEC_REF}" "${BUILD_DIR}/Dockerfile.exec" "${BUILD_DIR}")
        echo "  [exec] Pushed. Digest: ${EXEC_DIGEST}"
    fi

    # Step 2: Update workflow-schema.yaml with exec image digest
    if [ -n "${EXEC_DIGEST}" ]; then
        update_bundle_digest "${SCHEMA_FILE}" "${REGISTRY}/${IMAGE_NAME}" "${EXEC_DIGEST}"
        echo "  [schema] Updated execution.bundle digest in workflow-schema.yaml"
    else
        echo "  [schema] WARNING: Could not extract digest, schema not updated"
    fi

    # Step 3: Build and push schema image
    echo "  [schema] Building ${SCHEMA_REF}..."
    if $LOCAL_ONLY; then
        build_local "${SCHEMA_REF}" "${SCHEMA_DOCKERFILE}" "${BUILD_DIR}" > /dev/null
        echo "  [schema] Built (local arch only)"
    else
        build_and_push_multiarch "${SCHEMA_REF}" "${SCHEMA_DOCKERFILE}" "${BUILD_DIR}" > /dev/null
        echo "  [schema] Pushed."
    fi

    seeded_schemas+=("${SCHEMA_REF}")
    build_count=$((build_count + 1))
    echo ""
done

echo "============================================"
echo "Built: ${build_count} scenario(s) (exec + schema images each)"
if [ "$skip_count" -gt 0 ]; then
    echo "Skipped: ${skip_count}"
fi
if ! $LOCAL_ONLY; then
    echo "Pushed to: ${REGISTRY}"
fi
echo "============================================"

if $SEED_AFTER && ! $LOCAL_ONLY && [ "${#seeded_schemas[@]}" -gt 0 ]; then
    echo ""
    echo "==> Seeding workflow(s) in DataStorage..."
    SA_TOKEN=$(kubectl create token holmesgpt-api-sa -n kubernaut-system --duration=10m 2>/dev/null || echo "")
    DS_URL="${DATASTORAGE_URL:-http://localhost:30081}"
    AUTH_HEADER=""
    if [ -n "$SA_TOKEN" ]; then
        AUTH_HEADER="-H \"Authorization: Bearer ${SA_TOKEN}\""
    fi

    for schema_ref in "${seeded_schemas[@]}"; do
        echo -n "  Registering ${schema_ref}... "
        HTTP_CODE=$(eval curl -s -o /dev/null -w '%{http_code}' -X POST "${DS_URL}/api/v1/workflows" \
            -H "Content-Type: application/json" \
            ${AUTH_HEADER} \
            -d "'{ \"schemaImage\": \"${schema_ref}\" }'")
        echo "HTTP ${HTTP_CODE}"
    done
fi

echo ""
echo "Next steps:"
echo "  Seed the workflows in DataStorage (if not using --seed):"
echo "    ./deploy/demo/scripts/seed-workflows.sh"
