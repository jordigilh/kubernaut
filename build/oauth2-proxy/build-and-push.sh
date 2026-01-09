#!/bin/bash
set -euo pipefail

# Multi-arch OAuth2 Proxy Build and Push Script
# Strategy: Build ARM64 locally, pull existing AMD64 from upstream
# Target: quay.io/jordigilh/oauth2-proxy:latest

REGISTRY="quay.io/jordigilh"
IMAGE_NAME="oauth2-proxy"
VERSION="v7.5.1"  # Note: upstream uses 'v' prefix
TAG="latest"
UPSTREAM="quay.io/oauth2-proxy/oauth2-proxy:${VERSION}"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${TAG}"

echo "════════════════════════════════════════════════════════════════════════"
echo "🏗️  Multi-arch OAuth2 Proxy Build"
echo "════════════════════════════════════════════════════════════════════════"
echo ""
echo "Strategy:"
echo "  • Build ARM64 locally (native)"
echo "  • Pull AMD64 from upstream (avoid cross-compile)"
echo "  • Create multi-arch manifest"
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

# Build for ARM64 (local machine - native build)
echo "📦 Building ARM64 image (native)..."
podman build \
    --platform linux/arm64 \
    --tag ${FULL_IMAGE}-arm64 \
    -f Dockerfile \
    .

# Pull AMD64 from upstream (avoid cross-compile)
echo ""
echo "📥 Pulling AMD64 image from upstream..."
podman pull --platform linux/amd64 ${UPSTREAM}
podman tag ${UPSTREAM} ${FULL_IMAGE}-amd64

# Push ARM64 first to create the repository on quay.io
echo ""
echo "🚀 Pushing ARM64 image to create repository..."
podman push ${FULL_IMAGE}-arm64

# Create multi-arch manifest
echo ""
echo "📋 Creating multi-arch manifest..."
podman manifest rm ${FULL_IMAGE} 2>/dev/null || true
podman manifest create ${FULL_IMAGE}
podman manifest add ${FULL_IMAGE} ${FULL_IMAGE}-arm64
podman manifest add ${FULL_IMAGE} ${FULL_IMAGE}-amd64

# Push multi-arch manifest (this will update with both architectures)
echo ""
echo "🚀 Pushing multi-arch manifest to registry..."
podman manifest push ${FULL_IMAGE} --all

echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo "✅ Multi-arch image pushed successfully!"
echo "════════════════════════════════════════════════════════════════════════"
echo ""
echo "Image: ${FULL_IMAGE}"
echo "Architectures:"
echo "  - linux/amd64 (from upstream: ${UPSTREAM})"
echo "  - linux/arm64 (built locally)"
echo ""
echo "Verify:"
echo "  podman manifest inspect ${FULL_IMAGE}"
echo ""
echo "Use in E2E tests:"
echo "  Image is already configured in test/infrastructure/datastorage.go"
echo ""

