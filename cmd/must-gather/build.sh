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

# Kubernaut Must-Gather - Build and Push Script
# Builds multi-arch container image and pushes to quay.io/kubernaut

set -euo pipefail

VERSION="${1:-v1.0.0}"
REGISTRY="quay.io/kubernaut"
IMAGE_NAME="must-gather"

echo "=========================================="
echo "Building Kubernaut Must-Gather"
echo "=========================================="
echo "Version: ${VERSION}"
echo "Registry: ${REGISTRY}"
echo "Image: ${IMAGE_NAME}"
echo "=========================================="
echo ""

# Check if logged in to quay.io
echo "Checking quay.io authentication..."
if ! podman login quay.io --get-login > /dev/null 2>&1; then
    echo "Not logged in to quay.io. Please authenticate:"
    podman login quay.io
fi

# Build multi-arch image
echo ""
echo "Building multi-arch image (amd64, arm64)..."
podman build --platform linux/amd64,linux/arm64 \
    -t "${REGISTRY}/${IMAGE_NAME}:${VERSION}" \
    -t "${REGISTRY}/${IMAGE_NAME}:latest" \
    .

echo ""
echo "Build complete!"
echo ""
echo "Images built:"
echo "  - ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
echo "  - ${REGISTRY}/${IMAGE_NAME}:latest"
echo ""

# Push images
read -p "Push images to ${REGISTRY}? (y/n): " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Pushing ${REGISTRY}/${IMAGE_NAME}:${VERSION}..."
    podman push "${REGISTRY}/${IMAGE_NAME}:${VERSION}"

    echo "Pushing ${REGISTRY}/${IMAGE_NAME}:latest..."
    podman push "${REGISTRY}/${IMAGE_NAME}:latest"

    echo ""
    echo "=========================================="
    echo "✅ Images successfully pushed!"
    echo "=========================================="
    echo ""
    echo "Images available at:"
    echo "  - ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
    echo "  - ${REGISTRY}/${IMAGE_NAME}:latest"
    echo ""
    echo "Usage:"
    echo "  oc adm must-gather --image=${REGISTRY}/${IMAGE_NAME}:latest"
else
    echo "Skipping push."
fi

echo ""
echo "Done!"

