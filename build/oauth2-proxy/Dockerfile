# Multi-arch OAuth2 Proxy for Kubernaut E2E Tests
# Builds: quay.io/jordigilh/oauth2-proxy:latest
# Architectures: linux/amd64, linux/arm64

FROM quay.io/oauth2-proxy/oauth2-proxy:v7.5.1

# Metadata
LABEL name="kubernaut-oauth2-proxy" \
      vendor="Kubernaut" \
      version="7.5.1" \
      summary="Multi-arch OAuth2 Proxy for E2E Tests" \
      description="OAuth2 Proxy configured for Kubernaut SOC2 E2E testing with static user injection" \
      maintainer="jgil@redhat.com"

# No additional configuration needed - using upstream image as-is
# Configuration is provided via ConfigMap in E2E tests

