#!/bin/bash
# Local Kubernaut Setup with Remote OCP Cluster Integration
#
# This script sets up Kubernaut locally to manage a remote OpenShift cluster
# Architecture:
# - OCP Cluster: helios08 (remote)
# - AI Model: localhost:8080 (local)
# - Kubernaut: localhost (local, connects to remote cluster)
# - Tests: localhost (local)

set -euo pipefail

# Configuration
REMOTE_HOST="${1:-helios08}"
REMOTE_USER="${2:-root}"
REMOTE_PATH="/root/kubernaut-e2e"
LOCAL_KUBECONFIG="./kubeconfig-remote"
AI_MODEL_URL="http://localhost:8080"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# Banner
echo -e "${CYAN}"
cat << "EOF"
 _   _       _          _     _  _         _                         _
| | | |_   _| |__  _ __(_) __| || | ___   | |    ___   ___  __ _  | |
| |_| | | | | '_ \| '__| |/ _` || |/ _ \  | |   / _ \ / __|/ _` | | |
|  _  | |_| | |_) | |  | | (_| ||  (_) | | |__| (_) | (__| (_| | | |
|_| |_|\__, |_.__/|_|  |_|\__,_||_|\___/  |_____\___/ \___|\__,_| |_|
       |___/
  ____                      _          ____ _           _
 |  _ \ ___ _ __ ___   ___ | |_ ___   / ___| |_   _ ___| |_ ___ _ __
 | |_) / _ \ '_ ` _ \ / _ \| __/ _ \ | |   | | | | / __| __/ _ \ '__|
 |  _ <  __/ | | | | | (_) | ||  __/ | |___| | |_| \__ \ ||  __/ |
 |_| \_\___|_| |_| |_|\___/ \__\___|  \____|_|\__,_|___/\__\___|_|

EOF
echo -e "${NC}"

log_info "Hybrid Kubernaut E2E Setup"
log_info "Remote OCP Cluster: ${REMOTE_USER}@${REMOTE_HOST}"
log_info "Local AI Model: ${AI_MODEL_URL}"
log_info "Local Kubernaut connecting to remote cluster"

# Validate requirements
validate_local_requirements() {
    log_header "Validating Local Requirements"

    # Check if AI model is running locally
    if curl -s "${AI_MODEL_URL}/health" >/dev/null 2>&1 || curl -s "${AI_MODEL_URL}" >/dev/null 2>&1; then
        log_success "AI model detected at ${AI_MODEL_URL}"
    else
        log_warning "AI model not detected at ${AI_MODEL_URL}"
        log_info "Please ensure oss-gpt:20b is running locally on port 8080"
        log_info "You can start it with: ollama serve or LocalAI setup"
    fi

    # Check SSH connectivity to remote cluster
    if ssh -o ConnectTimeout=10 -o BatchMode=yes "${REMOTE_USER}@${REMOTE_HOST}" "echo 'Remote cluster accessible'" 2>/dev/null; then
        log_success "SSH connection to remote cluster verified"
    else
        log_error "Cannot connect to remote cluster at ${REMOTE_USER}@${REMOTE_HOST}"
        log_info "Please ensure SSH key authentication is configured"
        return 1
    fi

    # Check if Go is available for Kubernaut build
    if command -v go >/dev/null 2>&1; then
        log_success "Go compiler available for Kubernaut build"
    else
        log_error "Go compiler not found - required for building Kubernaut"
        return 1
    fi

    # Check if kubectl/oc is available
    if command -v oc >/dev/null 2>&1; then
        log_success "OpenShift CLI (oc) available"
    elif command -v kubectl >/dev/null 2>&1; then
        log_success "Kubernetes CLI (kubectl) available"
    else
        log_error "Neither oc nor kubectl found - required for cluster management"
        return 1
    fi
}

# Retrieve kubeconfig from remote cluster
retrieve_remote_kubeconfig() {
    log_header "Retrieving Remote Cluster Configuration"

    # Check if cluster exists on remote host
    if ! ssh "${REMOTE_USER}@${REMOTE_HOST}" "test -f ${REMOTE_PATH}/kubeconfig" 2>/dev/null; then
        log_error "Remote cluster kubeconfig not found at ${REMOTE_HOST}:${REMOTE_PATH}/kubeconfig"
        log_info "Please ensure the OCP cluster is deployed on ${REMOTE_HOST}"
        log_info "You can deploy it with: make deploy-cluster-remote"
        return 1
    fi

    # Copy kubeconfig from remote host
    log_info "Copying kubeconfig from remote cluster..."
    scp "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/kubeconfig" "${LOCAL_KUBECONFIG}"

    # Get cluster info to validate connection
    export KUBECONFIG="${LOCAL_KUBECONFIG}"

    log_info "Testing connection to remote cluster..."
    if oc get nodes 2>/dev/null | head -5; then
        log_success "Successfully connected to remote OpenShift cluster"

        # Get cluster details
        CLUSTER_VERSION=$(oc get clusterversion -o jsonpath='{.items[0].status.desired.version}' 2>/dev/null || echo "unknown")
        NODE_COUNT=$(oc get nodes --no-headers 2>/dev/null | wc -l || echo "unknown")
        CLUSTER_URL=$(oc whoami --show-server 2>/dev/null || echo "unknown")

        log_info "Cluster Version: ${CLUSTER_VERSION}"
        log_info "Node Count: ${NODE_COUNT}"
        log_info "Cluster URL: ${CLUSTER_URL}"
    else
        log_error "Failed to connect to remote cluster"
        return 1
    fi
}

# Setup local Kubernaut environment
setup_local_kubernaut() {
    log_header "Setting Up Local Kubernaut Environment"

    # Navigate to project root
    PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
    cd "${PROJECT_ROOT}"

    log_info "Building Kubernaut locally..."
    if make build; then
        log_success "Kubernaut build completed"
    else
        log_error "Kubernaut build failed"
        return 1
    fi

    # Create local configuration for remote cluster
    LOCAL_CONFIG_DIR="./local-config-remote"
    mkdir -p "${LOCAL_CONFIG_DIR}"

    # Copy kubeconfig to standard location
    cp "${LOCAL_KUBECONFIG}" "${LOCAL_CONFIG_DIR}/kubeconfig"

    # Create Kubernaut configuration for hybrid setup
    cat > "${LOCAL_CONFIG_DIR}/kubernaut-config.yaml" << EOF
# Kubernaut Configuration for Hybrid Deployment
# Local Kubernaut connecting to Remote OCP Cluster
cluster:
  kubeconfig: "${LOCAL_CONFIG_DIR}/kubeconfig"
  type: "openshift"
  remote: true

ai:
  model_url: "${AI_MODEL_URL}"
  model_type: "oss-gpt"
  local: true

storage:
  vector_db: "postgresql"
  connection_string: "postgres://kubernaut:password@localhost:5432/kubernaut_vectors"

monitoring:
  prometheus_url: "http://localhost:9090"
  grafana_url: "http://localhost:3000"

logging:
  level: "info"
  format: "json"

network:
  architecture: "hybrid"
  cluster_location: "remote"
  ai_location: "local"
  tests_location: "local"
EOF

    log_success "Local Kubernaut configuration created"

    # Set environment variables for hybrid setup
    export KUBECONFIG="${LOCAL_CONFIG_DIR}/kubeconfig"
    export KUBERNAUT_CONFIG="${LOCAL_CONFIG_DIR}/kubernaut-config.yaml"
    export AI_MODEL_URL="${AI_MODEL_URL}"

    log_info "Environment configured for hybrid deployment"
}

# Setup local vector database
setup_local_vector_db() {
    log_header "Setting Up Local Vector Database"

    # Check if PostgreSQL is running locally
    if pgrep postgres >/dev/null 2>&1 || brew services list | grep -q "postgresql.*started"; then
        log_success "PostgreSQL service detected"
    else
        log_info "Starting PostgreSQL service..."
        if command -v brew >/dev/null 2>&1; then
            brew services start postgresql || log_warning "Failed to start PostgreSQL via brew"
        elif command -v systemctl >/dev/null 2>&1; then
            sudo systemctl start postgresql || log_warning "Failed to start PostgreSQL via systemctl"
        else
            log_warning "Please start PostgreSQL manually"
        fi
    fi

    # Create database and user for Kubernaut
    log_info "Setting up Kubernaut vector database..."

    # Create database setup script
    cat > "./setup-vector-db.sql" << EOF
-- Kubernaut Vector Database Setup
CREATE DATABASE kubernaut_vectors;
CREATE USER kubernaut WITH PASSWORD 'password';
GRANT ALL PRIVILEGES ON DATABASE kubernaut_vectors TO kubernaut;

-- Connect to the new database
\c kubernaut_vectors;

-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Grant usage on public schema
GRANT ALL ON SCHEMA public TO kubernaut;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO kubernaut;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO kubernaut;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO kubernaut;

-- Test vector functionality
SELECT vector_dims('[1,2,3]'::vector);
EOF

    # Execute database setup
    if command -v psql >/dev/null 2>&1; then
        psql -h localhost -U postgres -f ./setup-vector-db.sql 2>/dev/null || \
        psql -h localhost -U "$(whoami)" -f ./setup-vector-db.sql 2>/dev/null || \
        log_warning "Vector database setup may need manual configuration"

        rm -f ./setup-vector-db.sql
        log_success "Vector database setup completed"
    else
        log_warning "psql not found - please set up vector database manually"
    fi
}

# Setup local monitoring stack
setup_local_monitoring() {
    log_header "Setting Up Local Monitoring Stack"

    MONITORING_DIR="./local-monitoring"
    mkdir -p "${MONITORING_DIR}"

    # Create Prometheus configuration for hybrid setup
    cat > "${MONITORING_DIR}/prometheus.yml" << EOF
# Prometheus Configuration for Hybrid Kubernaut Deployment
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "kubernaut-rules.yml"

scrape_configs:
  # Local Kubernaut metrics
  - job_name: 'kubernaut-local'
    static_configs:
      - targets: ['localhost:8081']
    scrape_interval: 5s

  # Local AI model metrics (if available)
  - job_name: 'ai-model-local'
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 10s
    metrics_path: '/metrics'

  # Remote cluster metrics (via port-forward)
  - job_name: 'openshift-cluster-remote'
    static_configs:
      - targets: ['localhost:9091']  # Port-forwarded from remote cluster
    scrape_interval: 30s

  # PostgreSQL vector database
  - job_name: 'postgres-local'
    static_configs:
      - targets: ['localhost:5432']
    scrape_interval: 30s
EOF

    # Create Kubernaut alerting rules
    cat > "${MONITORING_DIR}/kubernaut-rules.yml" << EOF
groups:
  - name: kubernaut-hybrid
    rules:
      - alert: KubernautDown
        expr: up{job="kubernaut-local"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Kubernaut is down"
          description: "Local Kubernaut instance has been down for more than 1 minute"

      - alert: RemoteClusterUnreachable
        expr: up{job="openshift-cluster-remote"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Remote OpenShift cluster unreachable"
          description: "Cannot reach remote OpenShift cluster for more than 2 minutes"

      - alert: AIModelUnresponsive
        expr: up{job="ai-model-local"} == 0
        for: 30s
        labels:
          severity: warning
        annotations:
          summary: "AI model unresponsive"
          description: "Local AI model at localhost:8080 is not responding"
EOF

    log_success "Local monitoring configuration created"
    log_info "To start monitoring:"
    log_info "  1. Install Prometheus and Grafana locally"
    log_info "  2. Use configuration in ${MONITORING_DIR}/"
    log_info "  3. Set up port-forwarding to remote cluster metrics"
}

# Create test configuration for hybrid setup
setup_test_configuration() {
    log_header "Setting Up Test Configuration"

    TEST_CONFIG_DIR="./test-config-hybrid"
    mkdir -p "${TEST_CONFIG_DIR}"

    # Create test environment configuration
    cat > "${TEST_CONFIG_DIR}/e2e-test-config.yaml" << EOF
# E2E Test Configuration for Hybrid Architecture
test_environment:
  architecture: "hybrid"
  cluster:
    location: "remote"
    host: "${REMOTE_HOST}"
    kubeconfig: "${LOCAL_CONFIG_DIR}/kubeconfig"

  ai_model:
    location: "local"
    url: "${AI_MODEL_URL}"
    model: "oss-gpt:20b"

  kubernaut:
    location: "local"
    config: "${LOCAL_CONFIG_DIR}/kubernaut-config.yaml"

  vector_db:
    location: "local"
    type: "postgresql"
    connection: "postgres://kubernaut:password@localhost:5432/kubernaut_vectors"

test_scenarios:
  network_isolation:
    description: "Test that cluster cannot reach AI model directly"
    validates: "Network topology is correctly isolated"

  local_ai_integration:
    description: "Test local Kubernaut can access local AI model"
    validates: "AI integration works from local environment"

  remote_cluster_management:
    description: "Test local Kubernaut can manage remote cluster"
    validates: "Remote cluster operations work correctly"

  end_to_end_remediation:
    description: "Test complete remediation flow across hybrid architecture"
    validates: "Business requirements are met in hybrid setup"
EOF

    # Create network validation script
    cat > "${TEST_CONFIG_DIR}/validate-network-topology.sh" << 'EOF'
#!/bin/bash
# Network Topology Validation for Hybrid Architecture

set -euo pipefail

echo "=== Validating Hybrid Network Topology ==="

# Test 1: Local AI model accessibility from localhost
echo "1. Testing local AI model access..."
if curl -s http://localhost:8080/health >/dev/null 2>&1; then
    echo "   ✓ AI model accessible from localhost"
else
    echo "   ✗ AI model not accessible from localhost"
fi

# Test 2: Remote cluster accessibility from localhost
echo "2. Testing remote cluster access..."
if oc get nodes >/dev/null 2>&1; then
    echo "   ✓ Remote cluster accessible from localhost"
else
    echo "   ✗ Remote cluster not accessible from localhost"
fi

# Test 3: Verify cluster CANNOT reach localhost (expected to fail)
echo "3. Testing cluster isolation from localhost AI model..."
if oc run test-pod --image=curlimages/curl --restart=Never --rm -i --tty -- curl -s --connect-timeout 5 http://host.docker.internal:8080 >/dev/null 2>&1; then
    echo "   ✗ SECURITY ISSUE: Cluster can reach localhost (unexpected!)"
else
    echo "   ✓ Cluster properly isolated from localhost (expected)"
fi

echo "=== Network topology validation complete ==="
EOF

    chmod +x "${TEST_CONFIG_DIR}/validate-network-topology.sh"

    log_success "Test configuration for hybrid architecture created"
}

# Create startup script for hybrid environment
create_startup_script() {
    log_header "Creating Hybrid Environment Startup Script"

    cat > "./start-hybrid-kubernaut.sh" << EOF
#!/bin/bash
# Startup script for Hybrid Kubernaut Environment

set -euo pipefail

echo "Starting Hybrid Kubernaut Environment..."

# Set environment variables
export KUBECONFIG="${LOCAL_CONFIG_DIR}/kubeconfig"
export KUBERNAUT_CONFIG="${LOCAL_CONFIG_DIR}/kubernaut-config.yaml"
export AI_MODEL_URL="${AI_MODEL_URL}"

# Validate connections
echo "Validating connections..."

# Check AI model
if curl -s "\${AI_MODEL_URL}/health" >/dev/null 2>&1; then
    echo "✓ AI model available at \${AI_MODEL_URL}"
else
    echo "✗ AI model not available - please start oss-gpt:20b on localhost:8080"
    exit 1
fi

# Check remote cluster
if oc get nodes >/dev/null 2>&1; then
    echo "✓ Remote cluster accessible"
else
    echo "✗ Remote cluster not accessible - check kubeconfig and network"
    exit 1
fi

# Start Kubernaut
echo "Starting local Kubernaut with remote cluster configuration..."
./bin/kubernaut --config "\${KUBERNAUT_CONFIG}" &

echo "Hybrid Kubernaut environment started!"
echo "  - AI Model: \${AI_MODEL_URL}"
echo "  - Remote Cluster: \$(oc whoami --show-server)"
echo "  - Vector DB: localhost:5432"
echo ""
echo "Run tests with: make test-e2e-hybrid"
EOF

    chmod +x "./start-hybrid-kubernaut.sh"
    log_success "Startup script created: ./start-hybrid-kubernaut.sh"
}

# Main execution
main() {
    log_info "Setting up Hybrid Kubernaut Environment"
    log_info "Remote Cluster: ${REMOTE_USER}@${REMOTE_HOST}"
    log_info "Local AI Model: ${AI_MODEL_URL}"

    # Validate requirements
    validate_local_requirements

    # Retrieve remote cluster configuration
    retrieve_remote_kubeconfig

    # Setup local components
    setup_local_kubernaut
    setup_local_vector_db
    setup_local_monitoring
    setup_test_configuration
    create_startup_script

    log_success "Hybrid Kubernaut environment setup completed!"

    # Display next steps
    log_header "Next Steps"
    echo -e "${GREEN}1. Start the environment:${NC} ./start-hybrid-kubernaut.sh"
    echo -e "${GREEN}2. Validate network topology:${NC} ./test-config-hybrid/validate-network-topology.sh"
    echo -e "${GREEN}3. Run hybrid tests:${NC} make test-e2e-hybrid"
    echo -e "${GREEN}4. Monitor with:${NC} Prometheus (local) + Remote cluster metrics"

    log_header "Architecture Summary"
    echo -e "${CYAN}Hybrid Deployment Architecture:${NC}"
    echo -e "  🖥️  OCP Cluster: ${REMOTE_HOST} (remote)"
    echo -e "  🤖 AI Model: localhost:8080 (local)"
    echo -e "  🔧 Kubernaut: localhost (local, manages remote cluster)"
    echo -e "  🧪 Tests: localhost (local)"
    echo -e "  📊 Vector DB: localhost:5432 (local)"
    echo -e "  📈 Monitoring: localhost (local)"

    log_info "Environment ready for hybrid E2E testing!"
}

# Execute main function
main "$@"
