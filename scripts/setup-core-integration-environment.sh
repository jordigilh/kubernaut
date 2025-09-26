#!/bin/bash
set -euo pipefail

# Kubernaut Core Services Integration Environment Setup
# This script sets up a complete integration testing environment for core services
# Usage: ./scripts/setup-core-integration-environment.sh [options]

# Script metadata
readonly SCRIPT_NAME="setup-core-integration-environment.sh"
readonly VERSION="1.0"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Default configuration
readonly DEFAULT_KIND_CLUSTER_NAME="kubernaut-test"
readonly DEFAULT_LLM_ENDPOINT="http://192.168.1.169:8080"
readonly DEFAULT_HOLMESGPT_ENDPOINT="http://localhost:3000"
readonly DEFAULT_CONFIG_FILE="${PROJECT_ROOT}/config/integration-testing.yaml"

# Environment detection
readonly IS_CURSOR_SESSION="${CURSOR_SESSION:-false}"
readonly IS_CI="${CI:-false}"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# Display usage information
usage() {
    cat << EOF
${SCRIPT_NAME} v${VERSION}

Sets up core services integration testing environment with persistent configuration.

USAGE:
    ${SCRIPT_NAME} [OPTIONS]

OPTIONS:
    -h, --help              Show this help message
    -v, --verbose           Enable verbose output
    -c, --config FILE       Use specific config file (default: ${DEFAULT_CONFIG_FILE})
    --cursor                Setup for Cursor IDE (enables SSH tunnel mode)
    --ci                    Setup for CI/CD environment (uses mocks)
    --clean                 Clean existing environment before setup
    --cluster-name NAME     Kind cluster name (default: ${DEFAULT_KIND_CLUSTER_NAME})
    --skip-llm             Skip LLM connectivity setup
    --dry-run              Show what would be done without executing

EXAMPLES:
    # Standard setup for local development
    ${SCRIPT_NAME}

    # Setup for Cursor IDE with SSH tunnel
    ${SCRIPT_NAME} --cursor

    # Clean setup with verbose output
    ${SCRIPT_NAME} --clean --verbose

    # CI/CD setup with mocks
    ${SCRIPT_NAME} --ci

ENVIRONMENT:
    The script creates a persistent environment configuration that includes:
    - Kind cluster with Prometheus and AlertManager
    - Docker containers for databases and services
    - Environment variable configuration
    - Integration test validation

EOF
}

# Parse command line arguments
parse_args() {
    VERBOSE=false
    CURSOR_MODE="${IS_CURSOR_SESSION}"
    CI_MODE="${IS_CI}"
    CLEAN_MODE=false
    DRY_RUN=false
    SKIP_LLM=false
    KIND_CLUSTER_NAME="${DEFAULT_KIND_CLUSTER_NAME}"
    CONFIG_FILE="${DEFAULT_CONFIG_FILE}"

    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -c|--config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            --cursor)
                CURSOR_MODE=true
                shift
                ;;
            --ci)
                CI_MODE=true
                shift
                ;;
            --clean)
                CLEAN_MODE=true
                shift
                ;;
            --cluster-name)
                KIND_CLUSTER_NAME="$2"
                shift 2
                ;;
            --skip-llm)
                SKIP_LLM=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    local missing_tools=()

    # Check required tools
    for tool in kind kubectl docker podman-compose jq curl; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done

    if [[ ${#missing_tools[@]} -ne 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_error "Please install missing tools and try again"
        exit 1
    fi

    # Check Docker/Podman is running
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker and try again."
        exit 1
    fi

    log_success "Prerequisites check passed"
}

# Clean existing environment
clean_environment() {
    if [[ "${CLEAN_MODE}" == "true" ]]; then
        log_info "Cleaning existing environment..."

        # Stop integration containers
        log_info "Stopping integration containers..."
        docker-compose -f "${PROJECT_ROOT}/test/integration/docker-compose.integration.yml" down --remove-orphans || true

        # Delete Kind cluster if it exists
        if kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
            log_info "Deleting existing Kind cluster: ${KIND_CLUSTER_NAME}"
            kind delete cluster --name "${KIND_CLUSTER_NAME}"
        fi

        log_success "Environment cleaned"
    fi
}

# Setup Kind cluster with monitoring
setup_kind_cluster() {
    log_info "Setting up Kind cluster: ${KIND_CLUSTER_NAME}"

    if kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
        log_warning "Kind cluster ${KIND_CLUSTER_NAME} already exists"
        return 0
    fi

    # Create Kind cluster
    cat << 'EOF' > /tmp/kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8081
    protocol: TCP
  - containerPort: 30090
    hostPort: 9091
    protocol: TCP
  - containerPort: 30093
    hostPort: 9094
    protocol: TCP
  - containerPort: 30800
    hostPort: 8801
    protocol: TCP
- role: worker
- role: worker
EOF

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would create Kind cluster with config"
        return 0
    fi

    kind create cluster --name "${KIND_CLUSTER_NAME}" --config /tmp/kind-config.yaml
    rm /tmp/kind-config.yaml

    # Wait for cluster to be ready
    kubectl wait --for=condition=Ready nodes --all --timeout=300s --context "kind-${KIND_CLUSTER_NAME}"

    log_success "Kind cluster created successfully"
}

# Deploy monitoring stack
deploy_monitoring() {
    log_info "Deploying monitoring stack..."

    local context="kind-${KIND_CLUSTER_NAME}"

    # Create monitoring namespace
    kubectl create namespace monitoring --context "${context}" --dry-run=client -o yaml | kubectl apply -f - --context "${context}"

    # Deploy Prometheus
    kubectl apply -f "${PROJECT_ROOT}/test/manifests/monitoring/" --context "${context}"

    # Wait for deployments
    kubectl wait --for=condition=Available deployment/prometheus -n monitoring --timeout=300s --context "${context}"
    kubectl wait --for=condition=Available deployment/alertmanager -n monitoring --timeout=300s --context "${context}"

    log_success "Monitoring stack deployed"
}

# Deploy kubernaut integration service
deploy_kubernaut_service() {
    log_info "Deploying kubernaut integration service..."

    local context="kind-${KIND_CLUSTER_NAME}"

    # Create e2e-test namespace
    kubectl create namespace e2e-test --context "${context}" --dry-run=client -o yaml | kubectl apply -f - --context "${context}"

    # Deploy kubernaut integration service
    kubectl apply -f "${PROJECT_ROOT}/test/manifests/kubernaut-integration-service.yaml" --context "${context}"

    # Wait for deployment
    kubectl wait --for=condition=Available deployment/kubernaut-integration -n e2e-test --timeout=300s --context "${context}"

    log_success "Kubernaut integration service deployed"
}

# Setup Docker services
setup_docker_services() {
    log_info "Setting up Docker services..."

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY RUN] Would start Docker services"
        return 0
    fi

    # Start integration services
    cd "${PROJECT_ROOT}"
    docker-compose -f test/integration/docker-compose.integration.yml up -d

    # Wait for services to be healthy
    log_info "Waiting for services to be healthy..."
    local max_attempts=60
    local attempt=0

    while [[ $attempt -lt $max_attempts ]]; do
        if docker-compose -f test/integration/docker-compose.integration.yml ps | grep -q "healthy"; then
            break
        fi
        sleep 5
        attempt=$((attempt + 1))
    done

    if [[ $attempt -eq $max_attempts ]]; then
        log_warning "Some services may not be fully healthy yet"
    fi

    log_success "Docker services started"
}

# Configure environment variables
configure_environment() {
    log_info "Configuring environment variables..."

    local env_file="${PROJECT_ROOT}/.env.integration"

    # Determine LLM endpoint based on mode
    local llm_endpoint="${DEFAULT_LLM_ENDPOINT}"
    local holmesgpt_llm_base_url="${DEFAULT_LLM_ENDPOINT}"

    if [[ "${CURSOR_MODE}" == "true" ]]; then
        llm_endpoint="http://localhost:8080"
        holmesgpt_llm_base_url="http://localhost:8080"
        log_info "Cursor mode: Using SSH tunnel endpoint"
    elif [[ "${CI_MODE}" == "true" ]]; then
        llm_endpoint="mock://localhost:8080"
        holmesgpt_llm_base_url="mock://localhost:8080"
        log_info "CI mode: Using mock LLM endpoint"
    fi

    # Create environment file
    cat > "${env_file}" << EOF
# Kubernaut Core Services Integration Environment
# Generated by ${SCRIPT_NAME} on $(date)
# Mode: $([ "${CURSOR_MODE}" == "true" ] && echo "Cursor" || ([ "${CI_MODE}" == "true" ] && echo "CI" || echo "Standard"))

# Kubernetes Configuration
KUBECONFIG=${HOME}/.kube/config
KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME}
USE_FAKE_K8S_CLIENT=false
SKIP_K8S_INTEGRATION=false

# LLM Configuration
LLM_ENDPOINT=${llm_endpoint}
LLM_MODEL=ggml-org/gpt-oss-20b-GGUF
LLM_PROVIDER=ramalama
LLM_MAX_TOKENS=16000
LLM_MAX_CONTEXT_SIZE=8192
LLM_TEMPERATURE=0.1
LLM_TIMEOUT=60s

# HolmesGPT Configuration
HOLMESGPT_ENDPOINT=${DEFAULT_HOLMESGPT_ENDPOINT}
HOLMESGPT_API_URL=${DEFAULT_HOLMESGPT_ENDPOINT}
HOLMESGPT_LLM_BASE_URL=${holmesgpt_llm_base_url}

# Monitoring Configuration
PROMETHEUS_ENDPOINT=http://localhost:9091
ALERTMANAGER_ENDPOINT=http://localhost:9094

# Database Configuration
DB_HOST=localhost
DB_PORT=5433
DB_NAME=action_history
DB_USER=slm_user
DB_PASSWORD=slm_password_dev

# Vector Database Configuration
VECTOR_DB_HOST=localhost
VECTOR_DB_PORT=5434
VECTOR_DB_NAME=vector_store
VECTOR_DB_USER=vector_user
VECTOR_DB_PASSWORD=vector_password_dev

# Redis Configuration
REDIS_HOST=localhost
REDIS_PORT=6380
REDIS_PASSWORD=integration_redis_password

# Context API Configuration
KUBERNAUT_CONTEXT_API_URL=http://localhost:8091
KUBERNAUT_CONTEXT_API_TIMEOUT=30

# Test Configuration
SKIP_SLOW_TESTS=false
SKIP_INTEGRATION=false
USE_MOCK_LLM=${CI_MODE}
TEST_TIMEOUT=30m
LOG_LEVEL=info
EOF

    log_success "Environment configuration created: ${env_file}"

    # Display environment summary
    if [[ "${VERBOSE}" == "true" ]]; then
        log_info "Environment summary:"
        echo "  Kind Cluster: ${KIND_CLUSTER_NAME}"
        echo "  LLM Endpoint: ${llm_endpoint}"
        echo "  HolmesGPT: ${DEFAULT_HOLMESGPT_ENDPOINT}"
        echo "  Config File: ${CONFIG_FILE}"
        echo "  Environment File: ${env_file}"
    fi
}

# Validate environment
validate_environment() {
    log_info "Validating integration environment..."

    local validation_errors=()

    # Check Kind cluster
    if ! kubectl cluster-info --context "kind-${KIND_CLUSTER_NAME}" &> /dev/null; then
        validation_errors+=("Kind cluster not accessible")
    fi

    # Check Docker services
    local required_containers=("kubernaut-integration-postgres" "kubernaut-integration-vectordb" "kubernaut-integration-redis" "kubernaut-context-api" "kubernaut-holmesgpt-api")
    for container in "${required_containers[@]}"; do
        if ! docker ps --format "{{.Names}}" | grep -q "^${container}$"; then
            validation_errors+=("Container ${container} not running")
        fi
    done

    # Check services in cluster
    if ! kubectl get pods -n monitoring --context "kind-${KIND_CLUSTER_NAME}" | grep -q "Running"; then
        validation_errors+=("Monitoring pods not running")
    fi

    if ! kubectl get pods -n e2e-test --context "kind-${KIND_CLUSTER_NAME}" | grep -q "Running"; then
        validation_errors+=("Kubernaut integration pod not running")
    fi

    # Report validation results
    if [[ ${#validation_errors[@]} -eq 0 ]]; then
        log_success "Environment validation passed"
        return 0
    else
        log_error "Environment validation failed:"
        for error in "${validation_errors[@]}"; do
            log_error "  - ${error}"
        done
        return 1
    fi
}

# Create persistent configuration
create_persistent_config() {
    log_info "Creating persistent configuration..."

    # Create Makefile target for easy environment setup
    local makefile_target="${PROJECT_ROOT}/scripts/integration-env-shortcuts.mk"

    cat > "${makefile_target}" << EOF
# Kubernaut Integration Environment Shortcuts
# Generated by ${SCRIPT_NAME} on $(date)
# Include this in your main Makefile: include scripts/integration-env-shortcuts.mk

.PHONY: setup-integration-env clean-integration-env validate-integration-env run-integration-tests

# Setup complete integration environment
setup-integration-env:
	@echo "🚀 Setting up core services integration environment..."
	@./scripts/setup-core-integration-environment.sh

# Clean integration environment
clean-integration-env:
	@echo "🧹 Cleaning integration environment..."
	@./scripts/setup-core-integration-environment.sh --clean

# Setup for Cursor IDE
setup-integration-env-cursor:
	@echo "🎯 Setting up integration environment for Cursor IDE..."
	@./scripts/setup-core-integration-environment.sh --cursor

# Validate integration environment
validate-integration-env:
	@echo "✅ Validating integration environment..."
	@./scripts/setup-core-integration-environment.sh --dry-run

# Run core integration tests
run-integration-tests:
	@echo "🧪 Running core services integration tests..."
	@source .env.integration 2>/dev/null || true && \\
	go test -v -tags=integration ./test/integration/core_integration/ -timeout=30m

# Run specific integration test suites
run-integration-tests-ai:
	@source .env.integration 2>/dev/null || true && \\
	go test -v -tags=integration ./test/integration/ai/ -timeout=20m

run-integration-tests-external:
	@source .env.integration 2>/dev/null || true && \\
	go test -v -tags=integration ./test/integration/external_services/ -timeout=15m

# Quick integration test (skip slow tests)
run-integration-tests-quick:
	@source .env.integration 2>/dev/null || true && \\
	SKIP_SLOW_TESTS=true go test -v -tags=integration ./test/integration/core_integration/ -timeout=10m
EOF

    log_success "Persistent configuration created: ${makefile_target}"

    # Create environment activation script
    local activate_script="${PROJECT_ROOT}/scripts/activate-integration-env.sh"

    cat > "${activate_script}" << 'EOF'
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
else
    echo "❌ Integration environment file not found: ${ENV_FILE}"
    echo "   Run: ./scripts/setup-core-integration-environment.sh"
    return 1
fi
EOF

    chmod +x "${activate_script}"
    log_success "Environment activation script created: ${activate_script}"
}

# Display success message
display_success_message() {
    log_success "🎉 Core services integration environment setup complete!"
    echo ""
    echo "📋 Environment Summary:"
    echo "   Kind Cluster: ${KIND_CLUSTER_NAME}"
    echo "   Configuration: ${CONFIG_FILE}"
    echo "   Environment File: ${PROJECT_ROOT}/.env.integration"
    echo ""
    echo "🚀 Quick Start:"
    echo "   # Load environment variables"
    echo "   source ./scripts/activate-integration-env.sh"
    echo ""
    echo "   # Run core integration tests"
    echo "   go test -v -tags=integration ./test/integration/core_integration/ -timeout=30m"
    echo ""
    echo "🔧 Management Commands:"
    echo "   make setup-integration-env       # Setup environment"
    echo "   make clean-integration-env       # Clean environment"
    echo "   make run-integration-tests       # Run all tests"
    echo "   make run-integration-tests-quick # Run quick tests"
    echo ""

    if [[ "${CURSOR_MODE}" == "true" ]]; then
        echo "💡 Cursor IDE Mode Enabled:"
        echo "   SSH Tunnel Required: ssh -L 8080:localhost:8080 user@192.168.1.169"
        echo "   LLM Endpoint: http://localhost:8080"
        echo ""
    fi

    echo "📚 Documentation:"
    echo "   Integration Config: config/integration-testing.yaml"
    echo "   Environment Template: Available in script"
    echo "   Test Examples: test/integration/core_integration/"
}

# Main function
main() {
    log_info "Starting Kubernaut Core Services Integration Environment Setup"

    parse_args "$@"
    check_prerequisites
    clean_environment

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "DRY RUN MODE - No changes will be made"
    fi

    setup_kind_cluster
    deploy_monitoring
    deploy_kubernaut_service
    setup_docker_services
    configure_environment

    if [[ "${DRY_RUN}" != "true" ]]; then
        validate_environment
    fi

    create_persistent_config
    display_success_message

    log_success "Setup completed successfully!"
}

# Run main function
main "$@"
