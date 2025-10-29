#!/bin/bash

set -euo pipefail

CLUSTER_NAME="kubernaut-test"
KIND_CONFIG="${KIND_CONFIG:-test/kind/kind-config.yaml}"
REGISTRY_NAME="kind-registry"
REGISTRY_PORT="5001"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
}

# Check if podman is available
check_podman() {
    if ! command -v podman &> /dev/null; then
        error "Podman is not installed or not in PATH"
        exit 1
    fi

    if ! podman system connection list --format=json | grep -q "Name"; then
        error "No podman system connections found. Please set up podman."
        exit 1
    fi

    log "Podman is available and configured"
}

# Check if kind is available
check_kind() {
    if ! command -v kind &> /dev/null; then
        error "KinD is not installed. Please install kind:"
        echo "  go install sigs.k8s.io/kind@latest"
        echo "  or visit: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
        exit 1
    fi
    log "KinD is available"
}

# Setup local registry with podman
setup_registry() {
    log "Setting up local registry with podman..."

    # Check if registry is already running
    if podman ps --format "{{.Names}}" | grep -q "^${REGISTRY_NAME}$"; then
        log "Registry ${REGISTRY_NAME} is already running"
        return 0
    fi

    # Remove existing registry container if it exists
    if podman ps -a --format "{{.Names}}" | grep -q "^${REGISTRY_NAME}$"; then
        log "Removing existing registry container..."
        podman rm -f "${REGISTRY_NAME}"
    fi

    # Start registry
    podman run -d \
        --name "${REGISTRY_NAME}" \
        --restart=always \
        -p "127.0.0.1:${REGISTRY_PORT}:5000" \
        docker.io/library/registry:2

    log "Local registry started on localhost:${REGISTRY_PORT}"
}

# Create kind cluster
create_cluster() {
    log "Creating KinD cluster: ${CLUSTER_NAME}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        warn "Cluster ${CLUSTER_NAME} already exists. Deleting first..."
        kind delete cluster --name="${CLUSTER_NAME}"
    fi

    # Ensure kind config exists
    if [[ ! -f "${KIND_CONFIG}" ]]; then
        error "KinD config file not found: ${KIND_CONFIG}"
        exit 1
    fi

    # Set podman as the provider for kind
    export KIND_EXPERIMENTAL_PROVIDER=podman

    # Create cluster
    kind create cluster \
        --name="${CLUSTER_NAME}" \
        --config="${KIND_CONFIG}" \
        --wait=300s

    log "KinD cluster created successfully"
}

# Connect registry to kind network
connect_registry() {
    log "Connecting registry to kind network..."

    # Get the kind network name
    local kind_network
    kind_network=$(podman network ls --format "{{.Name}}" | grep kind || echo "kind")

    # Connect registry to kind network if not already connected
    if ! podman inspect "${REGISTRY_NAME}" --format="{{.NetworkSettings.Networks}}" | grep -q "${kind_network}"; then
        podman network connect "${kind_network}" "${REGISTRY_NAME}" || {
            warn "Failed to connect registry to kind network, continuing anyway..."
        }
    fi

    log "Registry connected to kind network"
}

# Configure kubectl context
configure_kubectl() {
    log "Configuring kubectl context..."

    # Export kubeconfig to standard location for integration tests
    local kubeconfig_dir="${HOME}/.kube"
    local kubeconfig_file="${kubeconfig_dir}/kind-config"
    
    # Create .kube directory if it doesn't exist
    mkdir -p "${kubeconfig_dir}"
    
    # Export kubeconfig to file
    kind get kubeconfig --name="${CLUSTER_NAME}" > "${kubeconfig_file}"
    chmod 600 "${kubeconfig_file}"
    log "Kubeconfig saved to: ${kubeconfig_file}"

    # Set kubectl context to the new cluster
    KUBECONFIG="${kubeconfig_file}" kubectl cluster-info --context "kind-${CLUSTER_NAME}"
    KUBECONFIG="${kubeconfig_file}" kubectl config use-context "kind-${CLUSTER_NAME}"

    # Wait for cluster to be ready
    log "Waiting for cluster to be ready..."
    KUBECONFIG="${kubeconfig_file}" kubectl wait --for=condition=Ready nodes --all --timeout=300s

    log "Cluster is ready!"
}

# Deploy monitoring stack
deploy_monitoring_stack() {
    log "Deploying Prometheus monitoring stack..."

    local kubeconfig_file="${HOME}/.kube/kind-config"

    # Apply monitoring manifests
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/namespace.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/prometheus-rbac.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/prometheus-config.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/alert-rules.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/prometheus-deployment.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/alertmanager-config.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/alertmanager-deployment.yaml
    KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/monitoring/kube-state-metrics.yaml

    log "Waiting for monitoring stack to be ready..."

    # Wait for Prometheus to be ready
    KUBECONFIG="${kubeconfig_file}" kubectl wait --for=condition=Available deployment/prometheus -n monitoring --timeout=300s
    log "Prometheus is ready"

    # Wait for AlertManager to be ready
    KUBECONFIG="${kubeconfig_file}" kubectl wait --for=condition=Available deployment/alertmanager -n monitoring --timeout=300s
    log "AlertManager is ready"

    # Wait for kube-state-metrics to be ready
    KUBECONFIG="${kubeconfig_file}" kubectl wait --for=condition=Available deployment/kube-state-metrics -n monitoring --timeout=300s
    log "Kube-state-metrics is ready"

    log "Monitoring stack deployed successfully"
}

# Deploy test prerequisites
deploy_prerequisites() {
    log "Deploying test prerequisites..."

    local kubeconfig_file="${HOME}/.kube/kind-config"

    # Create test namespace
    KUBECONFIG="${kubeconfig_file}" kubectl create namespace e2e-test --dry-run=client -o yaml | KUBECONFIG="${kubeconfig_file}" kubectl apply -f -

    # Apply test manifests if they exist
    if [[ -f "test/manifests/test-deployment.yaml" ]]; then
        KUBECONFIG="${kubeconfig_file}" kubectl apply -f test/manifests/test-deployment.yaml -n e2e-test
        log "Test deployment applied"
    fi

    # Create RBAC for kubernaut
    KUBECONFIG="${kubeconfig_file}" kubectl create serviceaccount kubernaut -n e2e-test --dry-run=client -o yaml | KUBECONFIG="${kubeconfig_file}" kubectl apply -f -

    # Grant necessary permissions for testing
    KUBECONFIG="${kubeconfig_file}" kubectl create clusterrolebinding kubernaut-admin \
        --clusterrole=cluster-admin \
        --serviceaccount=e2e-test:kubernaut \
        --dry-run=client -o yaml | KUBECONFIG="${kubeconfig_file}" kubectl apply -f -

    log "Prerequisites deployed successfully"
}

# Create bootstrap directory with test files
setup_bootstrap_directory() {
    log "Setting up bootstrap directory for integration testing..."

    # Remove existing bootstrap directory if it exists
    if [[ -d "/tmp/kind-bootstrap" ]]; then
        log "Removing existing bootstrap directory..."
        rm -rf /tmp/kind-bootstrap
    fi

    # Create bootstrap directory with proper permissions
    mkdir -p /tmp/kind-bootstrap
    chmod 755 /tmp/kind-bootstrap

    # Create test alert payload for integration testing
    cat > /tmp/kind-bootstrap/test-alert.json << 'EOF'
{
  "alerts": [
    {
      "status": "firing",
      "labels": {
        "alertname": "HighMemoryUsage",
        "severity": "warning",
        "namespace": "e2e-test",
        "pod": "test-app-pod"
      },
      "annotations": {
        "description": "Memory usage is above 80%",
        "summary": "High memory usage detected"
      },
      "startsAt": "2024-01-01T00:00:00Z"
    }
  ]
}
EOF

    # Create integration test configuration
    cat > /tmp/kind-bootstrap/integration-config.yaml << 'EOF'
# Integration Test Configuration for Kind Cluster
cluster:
  name: kubernaut-test
  type: kind
  real_k8s: true

database:
  host: localhost
  port: 5433
  name: action_history
  use_container: true

vector_db:
  host: localhost
  port: 5434
  name: vector_store
  use_container: true

llm:
  endpoint: http://localhost:8080
  model: granite3.1-dense:8b
  provider: localai
  mock_in_ci: true

monitoring:
  prometheus_port: 9090
  alertmanager_port: 9093
  enable_scraping: true
EOF

    log "Bootstrap directory prepared at /tmp/kind-bootstrap"
}

# Deploy vector database components for integration testing
deploy_vector_db_support() {
    log "Deploying vector database support for integration testing..."

    local kubeconfig_file="${HOME}/.kube/kind-config"

    # Create vector DB namespace
    KUBECONFIG="${kubeconfig_file}" kubectl create namespace vector-db --dry-run=client -o yaml | KUBECONFIG="${kubeconfig_file}" kubectl apply -f -

    # Deploy pgvector support (ConfigMap for initialization)
    cat <<EOF | KUBECONFIG="${kubeconfig_file}" kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: vector-db-init
  namespace: vector-db
data:
  init.sql: |
    -- Initialize vector database for integration testing
    CREATE EXTENSION IF NOT EXISTS vector;
    CREATE EXTENSION IF NOT EXISTS hstore;

    -- Create tables for action patterns and embeddings
    CREATE TABLE IF NOT EXISTS action_patterns (
        id SERIAL PRIMARY KEY,
        pattern_name VARCHAR(255) NOT NULL,
        embedding vector(1536),
        metadata hstore,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE INDEX IF NOT EXISTS action_patterns_embedding_idx
    ON action_patterns USING ivfflat (embedding vector_cosine_ops);
EOF

    log "Vector database support deployed"
}

# Main execution
main() {
    log "Setting up KinD cluster for CI/CD and local integration testing..."
    log "🎯 Strategy: Kind for CI/CD and local testing, OCP for e2e tests"
    echo ""

    check_podman
    check_kind
    setup_bootstrap_directory
    setup_registry
    create_cluster
    connect_registry
    configure_kubectl
    deploy_monitoring_stack
    deploy_vector_db_support
    deploy_prerequisites

    local kubeconfig_file="${HOME}/.kube/kind-config"

    log "KinD cluster setup complete!"
    echo ""
    log "🏗️ Cluster Architecture:"
    echo "  ├── Control plane: 1 node (with monitoring stack)"
    echo "  ├── Workers: 2 nodes (for multi-node testing)"
    echo "  ├── Monitoring: Prometheus + AlertManager"
    echo "  ├── Vector DB: pgvector support"
    echo "  └── Bootstrap: Integration test components"
    echo ""
    log "Cluster info:"
    KUBECONFIG="${kubeconfig_file}" kubectl cluster-info --context "kind-${CLUSTER_NAME}"
    echo ""
    log "Monitoring services:"
    KUBECONFIG="${kubeconfig_file}" kubectl get pods,svc -n monitoring
    echo ""
    log "Test environment:"
    KUBECONFIG="${kubeconfig_file}" kubectl get pods,svc -n e2e-test
    echo ""
    log "Vector DB support:"
    KUBECONFIG="${kubeconfig_file}" kubectl get configmap -n vector-db
    echo ""
    log "🚀 Ready for Integration Testing:"
    echo "  • Real Kubernetes API via Kind"
    echo "  • Real PostgreSQL + Vector DB (containerized)"
    echo "  • Local LLM at port 8080 (configurable)"
    echo "  • Full monitoring stack"
    echo ""
    log "Configuration:"
    echo "  export KUBECONFIG=${kubeconfig_file}"
    echo "  kubectl config use-context kind-${CLUSTER_NAME}"
    echo ""
    log "Access services:"
    echo "  KUBECONFIG=${kubeconfig_file} kubectl port-forward svc/prometheus 9090:9090 -n monitoring"
    echo "  KUBECONFIG=${kubeconfig_file} kubectl port-forward svc/alertmanager 9093:9093 -n monitoring"
    echo ""
    log "Run tests:"
    echo "  make test-integration-kind    # Integration tests with Kind"
    echo "  make test-ci                  # CI tests with mocked LLM"
    echo ""
    log "Clean up:"
    echo "  ./scripts/cleanup-kind-cluster.sh"
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [--help]"
        echo ""
        echo "Sets up a KinD cluster for e2e testing using Podman as the container runtime."
        echo ""
        echo "Prerequisites:"
        echo "  - podman (configured and running)"
        echo "  - kind (https://kind.sigs.k8s.io/)"
        echo "  - kubectl"
        exit 0
        ;;
    *)
        main "$@"
        ;;
esac