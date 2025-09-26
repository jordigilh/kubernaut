#!/bin/bash
# HolmesGPT REST API Server Entrypoint
# Secure container initialization and service startup

set -e
set -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] INFO: $1${NC}" >&2
}

log_warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARN: $1${NC}" >&2
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}" >&2
}

log_debug() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] DEBUG: $1${NC}" >&2
    fi
}

# Environment validation
validate_environment() {
    log "🔍 Validating environment configuration..."

    local validation_errors=()

    # Required environment variables
    local required_vars=(
        "HOLMESGPT_LLM_PROVIDER"
        "HOLMESGPT_LLM_MODEL"
    )

    for var in "${required_vars[@]}"; do
        if [[ -z "${!var:-}" ]]; then
            validation_errors+=("Required environment variable $var is not set")
        fi
    done

    # Validate LLM provider
    case "${HOLMESGPT_LLM_PROVIDER:-}" in
        "openai"|"anthropic"|"local_llm"|"azure_openai"|"ollama"|"ramalama")
            log_debug "LLM provider ${HOLMESGPT_LLM_PROVIDER} is valid"
            ;;
        "")
            validation_errors+=("HOLMESGPT_LLM_PROVIDER is required")
            ;;
        *)
            validation_errors+=("Invalid LLM provider: ${HOLMESGPT_LLM_PROVIDER}")
            ;;
    esac

    # Validate ports
    if [[ -n "${HOLMESGPT_PORT:-}" ]] && ! [[ "${HOLMESGPT_PORT}" =~ ^[0-9]+$ ]]; then
        validation_errors+=("HOLMESGPT_PORT must be a number")
    fi

    if [[ -n "${HOLMESGPT_METRICS_PORT:-}" ]] && ! [[ "${HOLMESGPT_METRICS_PORT}" =~ ^[0-9]+$ ]]; then
        validation_errors+=("HOLMESGPT_METRICS_PORT must be a number")
    fi

    # Report validation results
    if [[ ${#validation_errors[@]} -gt 0 ]]; then
        log_error "Environment validation failed:"
        for error in "${validation_errors[@]}"; do
            log_error "  - $error"
        done
        exit 1
    fi

    log "✅ Environment validation passed"
}

# System checks
check_system_requirements() {
    log "🔧 Checking system requirements..."

    # Check Python version
    local python_version
    python_version=$(python3.11 --version 2>/dev/null || echo "Python not found")
    log "Python version: $python_version"

    # Check HolmesGPT installation
    if command -v holmes >/dev/null 2>&1; then
        local holmes_version
        holmes_version=$(holmes --version 2>/dev/null || echo "Version check failed")
        log "HolmesGPT version: $holmes_version"
    else
        log_error "HolmesGPT CLI not found in PATH"
        exit 1
    fi

    # Check required Python packages
    local required_packages=("fastapi" "uvicorn" "pydantic" "kubernetes")
    for package in "${required_packages[@]}"; do
        if python3.11 -c "import $package" 2>/dev/null; then
            log_debug "Package $package: OK"
        else
            log_error "Required Python package '$package' not found"
            exit 1
        fi
    done

    log "✅ System requirements check passed"
}

# Network connectivity tests
test_connectivity() {
    log "🌐 Testing network connectivity..."

    # Test LLM provider connectivity
    if [[ -n "${HOLMESGPT_LLM_BASE_URL:-}" ]]; then
        log "Testing LLM connectivity to ${HOLMESGPT_LLM_BASE_URL}"

        if timeout 10 python3.11 -c "
import requests
import sys
try:
    response = requests.get('${HOLMESGPT_LLM_BASE_URL}/v1/models', timeout=5, headers={'Authorization': 'Bearer ${HOLMESGPT_LLM_API_KEY:-}'})
    print(f'LLM Status: {response.status_code}')
    if response.status_code == 200:
        sys.exit(0)
    else:
        sys.exit(1)
except Exception as e:
    print(f'LLM Error: {e}')
    sys.exit(1)
" 2>/dev/null; then
            log "✅ LLM connectivity verified"
        else
            log_warn "⚠️ LLM connectivity test failed (service may still work)"
        fi
    fi

    # Test Context API connectivity
    if [[ -n "${KUBERNAUT_CONTEXT_API_URL:-}" ]]; then
        log "Testing Context API connectivity to ${KUBERNAUT_CONTEXT_API_URL}"

        if timeout 5 python3.11 -c "
import requests
import sys
try:
    response = requests.get('${KUBERNAUT_CONTEXT_API_URL}/api/v1/context/health', timeout=3)
    print(f'Context API Status: {response.status_code}')
    if response.status_code == 200:
        sys.exit(0)
    else:
        sys.exit(1)
except Exception as e:
    print(f'Context API Error: {e}')
    sys.exit(1)
" 2>/dev/null; then
            log "✅ Context API connectivity verified"
        else
            log_warn "⚠️ Context API connectivity test failed"
        fi
    fi

    # Test Kubernetes API connectivity
    if [[ -f "${KUBECONFIG:-/root/.kube/config}" ]]; then
        log "Testing Kubernetes API connectivity"
        if timeout 5 python3.11 -c "
from kubernetes import client, config
try:
    config.load_incluster_config()
except:
    try:
        config.load_kube_config('${KUBECONFIG:-/root/.kube/config}')
    except Exception as e:
        print(f'Kubernetes config error: {e}')
        exit(1)

v1 = client.CoreV1Api()
try:
    v1.list_namespace(limit=1)
    print('Kubernetes API: OK')
except Exception as e:
    print(f'Kubernetes API error: {e}')
    exit(1)
" 2>/dev/null; then
            log "✅ Kubernetes API connectivity verified"
        else
            log_warn "⚠️ Kubernetes API connectivity test failed"
        fi
    fi
}

# Configuration setup
setup_configuration() {
    log "⚙️ Setting up configuration..."

    # Create configuration directories
    mkdir -p /app/config /tmp/app /var/log/holmesgpt

    # Generate HolmesGPT configuration from environment
    cat > /app/config/holmes-config.yaml << EOF
# HolmesGPT Configuration (Auto-generated)
llm:
  provider: "${HOLMESGPT_LLM_PROVIDER:-openai}"
  model: "${HOLMESGPT_LLM_MODEL:-gpt-4}"
  api_key: "${HOLMESGPT_LLM_API_KEY:-}"
  base_url: "${HOLMESGPT_LLM_BASE_URL:-}"
  timeout: ${HOLMESGPT_LLM_TIMEOUT:-60}
  max_tokens: ${HOLMESGPT_LLM_MAX_TOKENS:-4000}
  temperature: ${HOLMESGPT_LLM_TEMPERATURE:-0.3}

toolsets:
$(for toolset in ${HOLMESGPT_TOOLSETS:-kubernetes prometheus internet}; do echo "  - \"$toolset\""; done)

kubernetes:
  kubeconfig: "${KUBECONFIG:-/root/.kube/config}"
  incluster: ${KUBERNETES_IN_CLUSTER:-true}
  namespace: "${KUBERNETES_NAMESPACE:-default}"

prometheus:
  url: "${PROMETHEUS_URL:-http://prometheus:9090}"

cache:
  enabled: ${HOLMESGPT_CACHE_ENABLED:-true}
  ttl: ${HOLMESGPT_CACHE_TTL:-300}

logging:
  level: "${LOG_LEVEL:-INFO}"
  format: "${LOG_FORMAT:-json}"

performance:
  max_concurrent_investigations: ${HOLMESGPT_MAX_CONCURRENT:-10}
  investigation_timeout: ${HOLMESGPT_INVESTIGATION_TIMEOUT:-300}
EOF

    log "✅ Configuration setup completed"
}

# Security checks
run_security_checks() {
    log "🔒 Running security checks..."

    # Check file permissions
    if [[ -w "/etc/passwd" ]] || [[ -w "/etc/shadow" ]] || [[ -w "/etc/group" ]]; then
        log_error "Security violation: System files are writable"
        exit 1
    fi

    # Check if running as root
    if [[ "$(id -u)" -eq 0 ]]; then
        log_error "Security violation: Running as root is not allowed"
        exit 1
    fi

    # Check environment for sensitive data in logs
    if [[ "${DEBUG:-false}" == "true" ]]; then
        log_warn "Debug mode is enabled - sensitive data may appear in logs"
    fi

    log "✅ Security checks passed"
}

# Main startup function
main() {
    echo -e "${CYAN}"
    cat << 'EOF'
╔══════════════════════════════════════════════════════════════╗
║              HolmesGPT REST API Server v1.0.0               ║
║            Source-built with Red Hat UBI Base              ║
╚══════════════════════════════════════════════════════════════╝
EOF
    echo -e "${NC}"

    # Startup checks
    validate_environment
    run_security_checks
    check_system_requirements
    setup_configuration
    test_connectivity

    log "🚀 Starting HolmesGPT REST API Server"
    log "   - Host: ${HOLMESGPT_HOST:-0.0.0.0}"
    log "   - Port: ${HOLMESGPT_PORT:-8090}"
    log "   - Metrics Port: ${HOLMESGPT_METRICS_PORT:-9091}"
    log "   - LLM Provider: ${HOLMESGPT_LLM_PROVIDER:-unknown}"
    log "   - Debug Mode: ${DEBUG:-false}"

    # Export environment variables for the application
    export HOLMES_CONFIG_PATH="/app/config/holmes-config.yaml"
    export PYTHONPATH="/usr/local/lib/python3.11/site-packages:/app/src"

    # Start the application
    log "🎯 Launching application..."
    exec "$@"
}

# Error handler
handle_error() {
    local exit_code=$?
    log_error "Startup failed with exit code $exit_code"
    exit $exit_code
}

# Set error trap
trap handle_error ERR

# Run main function
main "$@"
