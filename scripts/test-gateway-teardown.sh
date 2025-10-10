#!/bin/bash
# Gateway Integration Test Environment Teardown
#
# This script deletes the Kind cluster created for Gateway integration tests.
#
# Usage: ./scripts/test-gateway-teardown.sh
# Or:    make test-gateway-teardown

set -euo pipefail

CLUSTER_NAME="kubernaut-gateway-test"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_info "Deleting Kind cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
log_success "Cluster deleted"

# Clean up temp files
rm -f /tmp/test-gateway-token.txt
log_success "Temp files removed"

echo ""
log_success "Gateway test environment cleaned up"

