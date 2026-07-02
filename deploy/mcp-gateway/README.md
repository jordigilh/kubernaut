# MCP Gateway Development Manifests
#
# These manifests provision the Envoy AI Gateway MCP Gateway for local
# development and integration testing with Kind clusters.
#
# Prerequisites:
#   - Envoy Gateway installed (CRDs: Gateway, Backend, MCPRoute)
#   - Envoy AI Gateway controller or standalone aigw binary
#   - DEX IdP for OAuth2 token issuance (E2E testing)
#
# Usage:
#   kubectl apply -k deploy/mcp-gateway/
#
# Note: This is for development ONLY. Production deployment is managed
# by the kubernaut-operator repository.
#
# Migration: These manifests replace the previous Kuadrant-based deployment
# which required Istio + Authorino. Envoy AI Gateway provides equivalent
# functionality (tool prefixing, OAuth, CEL authorization) without Istio.
