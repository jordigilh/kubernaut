#!/bin/bash
# Kubernaut Integration Environment Activation Script
# Source this script to load integration environment variables
# Usage: source ./scripts/activate-integration-env.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${PROJECT_ROOT}/.env.integration"

if [[ -f "${ENV_FILE}" ]]; then
    echo "🔧 Loading integration environment from ${ENV_FILE}"
    set -a  # Export all variables
    source "${ENV_FILE}"
    set +a

    echo "✅ Integration environment loaded:"
    echo "   LLM Endpoint: ${LLM_ENDPOINT}"
    echo "   HolmesGPT: ${HOLMESGPT_ENDPOINT}"
    echo "   Prometheus: ${PROMETHEUS_ENDPOINT}"
    echo "   AlertManager: ${ALERTMANAGER_ENDPOINT}"
    echo ""
    echo "🧪 Run integration tests with:"
    echo "   go test -v -tags=integration ./test/integration/core_integration/ -timeout=30m"
    echo ""
    echo "💡 SSH Tunnel Required (for Cursor):"
    echo "   ssh -L 8080:localhost:8080 user@192.168.1.169"
else
    echo "❌ Integration environment file not found: ${ENV_FILE}"
    echo "   Run: ./scripts/setup-core-integration-environment.sh"
    return 1
fi
