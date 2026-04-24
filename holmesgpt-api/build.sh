#!/bin/bash
# Build script for HolmesGPT API service
# Usage: ./build.sh [TAG]

set -e

# Default values
IMAGE_NAME="kubernaut-holmesgpt-api"
REGISTRY="${REGISTRY:-quay.io/kubernaut}"
TAG="${1:-latest}"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo "========================================
Building HolmesGPT API Service
========================================
Image: ${FULL_IMAGE}
"

# Build from holmesgpt-api directory (self-contained build)
cd "$(dirname "$0")"

# Build image
echo "Building image..."
podman build \
    -t "${FULL_IMAGE}" \
    --label "build.date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    --label "build.version=${TAG}" \
    .

echo "
✅ Build complete!

Image: ${FULL_IMAGE}

To run locally:
  podman run -d -p 8080:8080 \\
    -e DEV_MODE=true \\
    -e AUTH_ENABLED=false \\
    ${FULL_IMAGE}

To push to registry:
  podman push ${FULL_IMAGE}

To run tests:
  podman run --rm ${FULL_IMAGE} pytest -v
"

