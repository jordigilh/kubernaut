# MCP Gateway Development Manifests
#
# These manifests provision the Kuadrant MCP Gateway for local development
# and integration testing with Kind clusters.
#
# Prerequisites:
#   - Kuadrant operator installed (out-of-scope for kubernaut chart)
#   - Istio service mesh with Gateway API support
#   - DEX IdP for OAuth2 token issuance (E2E testing)
#
# Usage:
#   kubectl apply -k deploy/mcp-gateway/
#
# Note: This is for development ONLY. Production deployment is managed
# by the kubernaut-operator repository.
