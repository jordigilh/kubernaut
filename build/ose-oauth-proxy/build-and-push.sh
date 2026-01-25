#!/bin/bash
set -euo pipefail

# Multi-arch ose-oauth-proxy Build and Push Script
# Strategy:
#   - Pull existing AMD64 image from upstream (quay.io/openshift/origin-oauth-proxy)
#   - Build ARM64 from source (adds ARM64 support)
#   - Create multi-arch manifest
# Target: quay.io/jordigilh/ose-oauth-proxy:latest

REGISTRY="quay.io/jordigilh"
IMAGE_NAME="ose-oauth-proxy"
TAG="latest"
UPSTREAM="quay.io/openshift/origin-oauth-proxy:latest"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo "════════════════════════════════════════════════════════════════════════"
echo "🏗️  Multi-arch ose-oauth-proxy Build"
echo "════════════════════════════════════════════════════════════════════════"
echo ""
echo "Strategy:"
echo "  • Build ARM64 from source (native, using Red Hat UBI)"
echo "  • Pull AMD64 from upstream (reuse existing image)"
echo "  • Create multi-arch manifest"
echo ""
echo "ARM64 Base Images (Red Hat UBI):"
echo "  • Builder: registry.access.redhat.com/ubi9/go-toolset:1.24"
echo "  • Runtime: registry.access.redhat.com/ubi9/ubi-minimal:latest"
echo ""
echo "AMD64 Source: ${UPSTREAM}"
echo ""
echo "Target: ${FULL_IMAGE}"
echo "Upstream: ${UPSTREAM}"
echo ""

# Check if logged in to quay.io
if ! podman login quay.io --get-login &>/dev/null; then
    echo "❌ Not logged in to quay.io"
    echo "Run: podman login quay.io"
    exit 1
fi

# Build ARM64 from source (native build on ARM64 Mac)
echo "📦 Building ARM64 image from source (native, Red Hat UBI)..."
podman build \
    --platform linux/arm64 \
    --tag ${FULL_IMAGE}-arm64 \
    -f Dockerfile \
    .

# Pull AMD64 from upstream (fallback since we can't cross-compile on ARM64 without emulation)
echo ""
echo "📥 Pulling AMD64 image from upstream (no AMD64 emulation available)..."
podman pull --platform linux/amd64 ${UPSTREAM}
podman tag ${UPSTREAM} ${FULL_IMAGE}-amd64

# Push both architecture-specific images
echo ""
echo "🚀 Pushing images to registry..."
podman push ${FULL_IMAGE}-arm64
podman push ${FULL_IMAGE}-amd64

# Create and push multi-arch manifest
echo ""
echo "📋 Creating multi-arch manifest..."
podman manifest rm ${FULL_IMAGE} 2>/dev/null || true
podman manifest create ${FULL_IMAGE}
podman manifest add ${FULL_IMAGE} ${FULL_IMAGE}-arm64
podman manifest add ${FULL_IMAGE} ${FULL_IMAGE}-amd64
podman manifest push ${FULL_IMAGE} --all

echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo "✅ Multi-arch image pushed successfully!"
echo "════════════════════════════════════════════════════════════════════════"
echo ""
echo "Image: ${FULL_IMAGE}"
echo "Architectures:"
echo "  - linux/amd64 (from upstream: ${UPSTREAM})"
echo "  - linux/arm64 (built from source with Red Hat UBI)"
echo ""
echo "Verify:"
echo "  podman manifest inspect ${FULL_IMAGE}"
echo ""
echo "Use in E2E tests:"
echo "  Image is configured in test/infrastructure/datastorage.go"
echo ""

