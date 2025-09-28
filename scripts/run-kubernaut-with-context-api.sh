#!/bin/bash
# scripts/run-kubernaut-with-context-api.sh
# Local development script for Kubernaut with Context API integration
# Following development guideline: integrate with existing code and reuse patterns

set -e

# Configuration
CONFIG_FILE="${1:-config/local-llm.yaml}"
DEFAULT_LOG_LEVEL="info"
DRY_RUN="${DRY_RUN:-false}"

echo "🚀 Starting Kubernaut Full Stack with Context API Integration..."
echo "======================================================"
echo ""

# Validate config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "❌ Configuration file not found: $CONFIG_FILE"
    echo "Available config files:"
    ls -1 config/*.yaml 2>/dev/null || echo "  No config files found in config/"
    exit 1
fi

echo "📋 Configuration:"
echo "  • Config File: $CONFIG_FILE"
echo "  • Log Level: ${LOG_LEVEL:-$DEFAULT_LOG_LEVEL}"
echo "  • Dry Run: $DRY_RUN"
echo ""

echo "📡 Services Starting:"
echo "  • Main Service:  :8080 (webhooks, health, ready)"
echo "  • Context API:   :8091 (HolmesGPT integration endpoints)"
echo "  • Metrics:       :9090 (Prometheus metrics)"
echo ""

# Check if Context API is enabled in config
if ! grep -q "context_api:" "$CONFIG_FILE" || ! grep -A5 "context_api:" "$CONFIG_FILE" | grep -q "enabled: true"; then
    echo "⚠️  Warning: Context API may not be enabled in $CONFIG_FILE"
    echo "   Make sure the config includes:"
    echo "   ai_services:"
    echo "     context_api:"
    echo "       enabled: true"
    echo "       port: 8091"
    echo ""
fi

echo "🔗 Service URLs (after startup):"
echo "  • Main Health:     http://localhost:8080/health"
echo "  • Main Ready:      http://localhost:8080/ready"
echo "  • Context API:     http://localhost:8091/api/v1/context/health"
echo "  • Prometheus:      http://localhost:9090/metrics"
echo ""

echo "💡 Integration Testing:"
echo "  • Test Context API: curl http://localhost:8091/api/v1/context/health"
echo "  • Test Integration: ./scripts/test-holmesgpt-integration.sh"
echo ""

echo "🏃 Starting application..."
echo "----------------------------------------"

# Set log level from environment or default
export LOG_LEVEL="${LOG_LEVEL:-$DEFAULT_LOG_LEVEL}"

# Build the application first (following development guidelines)
echo "🔨 Building application..."
go build -o bin/kubernaut ./cmd/kubernaut

# Start the application with proper flags
echo "▶️  Starting Kubernaut with Context API..."
exec ./bin/kubernaut \
    --config="$CONFIG_FILE" \
    --log-level="$LOG_LEVEL" \
    $([ "$DRY_RUN" = "true" ] && echo "--dry-run")
