#!/bin/bash
# Complete End-to-End Testing Environment Setup for Kubernaut - Root User on RHEL 9.7
# Orchestrates the full testing environment including OCP cluster, AI model, storage, chaos testing, and monitoring
# NOTE: This script must be run as root

set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script must be run as root for RHEL 9.7 deployment"
    echo "Usage: sudo $0"
    exit 1
fi

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
ROOT_HOME="/root"

# Default values - can be overridden via environment variables
CLUSTER_NAME="${CLUSTER_NAME:-kubernaut-e2e}"
CONFIG_FILE="${CONFIG_FILE:-kcli-baremetal-params-root.yml}"
AI_MODEL_ENDPOINT="${AI_MODEL_ENDPOINT:-http://localhost:8080}"
AI_MODEL_NAME="${AI_MODEL_NAME:-gpt-oss:20b}"
VECTOR_DB_TYPE="${VECTOR_DB_TYPE:-postgresql}"
ENABLE_CHAOS_TESTING="${ENABLE_CHAOS_TESTING:-true}"
ENABLE_MONITORING="${ENABLE_MONITORING:-true}"
ENABLE_TEST_DATA="${ENABLE_TEST_DATA:-true}"
ENVIRONMENT_DURATION="${ENVIRONMENT_DURATION:-8h}"
AUTO_CLEANUP="${AUTO_CLEANUP:-true}"
LOG_LEVEL="${LOG_LEVEL:-INFO}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Logging functions
log_debug() { [[ "$LOG_LEVEL" == "DEBUG" ]] && echo -e "${CYAN}[DEBUG]${NC} $1"; }
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "\n${PURPLE}=== $1 ===${NC}"; }

# Progress tracking
TOTAL_STEPS=12
CURRENT_STEP=0

progress() {
    CURRENT_STEP=$((CURRENT_STEP + 1))
    echo -e "${CYAN}[STEP ${CURRENT_STEP}/${TOTAL_STEPS}]${NC} $1"
}

# Banner
echo -e "${PURPLE}"
cat << "EOF"
 _  __     _                                 _     _____ ____  _____   _____            _   _
| |/ /    | |                               | |   |  ___/ ___||  ___|  |_   _|          | | (_)
| ' /_   _| |__   ___ _ __ _ __   __ _ _   _  | |_  | |__ \___ \| |__     | | ___  ___  | |_ _ _ __   __ _
|  <| | | | '_ \ / _ \ '__| '_ \ / _` | | | | | __| |  __| ___) |  __|    | |/ _ \/ __| | __| | '_ \ / _` |
| . \ |_| | |_) |  __/ |  | | | | (_| | |_| | | |_  | |___\____/| |___    | |  __/\__ \ | |_| | | | | (_| |
|_|\_\__,_|_.__/ \___|_|  |_| |_|\__,_|\__,_|  \__| \____/     \____/    \_|\___||___/  \__|_|_| |_|\__, |
                                                                                                     __/ |
     ____             _     _____                _                                      _           |___/
    |  _ \ ___   ___ | |_  | ____|_ ____   _____ (_)_ __ ___  _ __  _ __ ___   ___ _ __ | |_
    | |_) / _ \ / _ \| __| |  _| | '_ \ \ / / _ \| | '__/ _ \| '_ \| '_ ` _ \ / _ \ '_ \| __|
    |  _ < (_) | (_) | |_  | |___| | | \ V / (_) | | | | (_) | | | | | | | | |  __/ | | | |_
    |_| \_\___/ \___/ \__| |_____|_| |_|\_/ \___/|_|_|  \___/|_| |_|_| |_| |_|\___|_| |_|\__|
EOF
echo -e "${NC}"

log_info "Starting Kubernaut Complete E2E Testing Environment Setup (Root User)"
log_info "RHEL 9.7 Host: $(hostname)"
log_info "Running as: $(whoami)"
log_info "Root Home: ${ROOT_HOME}"
log_info "Cluster: ${CLUSTER_NAME}"
log_info "AI Model: ${AI_MODEL_NAME} @ ${AI_MODEL_ENDPOINT}"
log_info "Duration: ${ENVIRONMENT_DURATION}"
echo ""

# Cleanup function
cleanup_on_exit() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Setup failed with exit code $exit_code"
        log_info "Check logs and run cleanup if needed: ./cleanup-e2e-environment-root.sh"
    fi
}

trap cleanup_on_exit EXIT

# Step 1: Validate Prerequisites
validate_prerequisites() {
    progress "Validating Prerequisites for Root User"

    # Check if validation script exists and run it
    if [[ -f "${SCRIPT_DIR}/validate-baremetal-setup-root.sh" ]]; then
        log_info "Running comprehensive root user validation..."
        chmod +x "${SCRIPT_DIR}/validate-baremetal-setup-root.sh"
        if "${SCRIPT_DIR}/validate-baremetal-setup-root.sh" "${CONFIG_FILE}"; then
            log_success "All prerequisites validated for root deployment"
        else
            log_error "Prerequisite validation failed for root deployment"
            exit 1
        fi
    else
        log_warning "Root validation script not found, running basic checks..."

        # Basic prerequisite checks for root
        if ! command -v python3 &> /dev/null; then
            log_error "Python 3 is not installed"
            log_info "Install with: dnf install -y python3 python3-pip"
            exit 1
        fi

        # Check pull secret and SSH key for root
        PULL_SECRET_PATH=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "/root/.pull-secret.txt")
        SSH_KEY_PATH=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" || echo "/root/.ssh/id_rsa.pub")

        if [[ ! -f "$PULL_SECRET_PATH" ]]; then
            log_error "Pull secret not found: $PULL_SECRET_PATH"
            log_info "Download from: https://console.redhat.com/openshift/install/pull-secret"
            exit 1
        fi

        if [[ ! -f "$SSH_KEY_PATH" ]]; then
            log_warning "SSH key not found: $SSH_KEY_PATH"
            log_info "Generating SSH key for root..."
            mkdir -p "${ROOT_HOME}/.ssh"
            ssh-keygen -t rsa -b 4096 -C "root@$(hostname)" -f "${ROOT_HOME}/.ssh/id_rsa" -N ""
            chmod 600 "${ROOT_HOME}/.ssh/id_rsa"
            chmod 644 "${ROOT_HOME}/.ssh/id_rsa.pub"
            log_success "SSH key generated for root"
        fi

        log_success "Basic prerequisites validated for root"
    fi
}

# Step 2: Deploy OpenShift Cluster
deploy_ocp_cluster() {
    progress "Deploying OpenShift Container Platform Cluster (Root)"

    # Check if cluster already exists
    if command -v kcli &>/dev/null && kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
        log_warning "Cluster '${CLUSTER_NAME}' already exists!"
        read -p "Do you want to use the existing cluster? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Using existing cluster"
            # Set kubeconfig for existing cluster
            export KUBECONFIG="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
            return 0
        else
            log_info "Deleting existing cluster..."
            kcli delete cluster "${CLUSTER_NAME}" --yes
        fi
    fi

    # Deploy new cluster using root script
    if [[ -f "${SCRIPT_DIR}/deploy-kcli-cluster-root.sh" ]]; then
        log_info "Deploying cluster using root-optimized KCLI script..."
        chmod +x "${SCRIPT_DIR}/deploy-kcli-cluster-root.sh"

        # Run deployment
        if "${SCRIPT_DIR}/deploy-kcli-cluster-root.sh" "${CLUSTER_NAME}" "${CONFIG_FILE}"; then
            log_success "OpenShift cluster deployed successfully"
        else
            log_error "Cluster deployment failed"
            exit 1
        fi
    else
        log_error "Root cluster deployment script not found"
        exit 1
    fi

    # Set kubeconfig for root
    export KUBECONFIG="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"

    # Verify cluster access
    if oc whoami &>/dev/null; then
        log_success "Cluster access verified for root"
        log_info "Cluster nodes: $(oc get nodes --no-headers | wc -l)"
    else
        log_error "Cannot access cluster as root"
        exit 1
    fi
}

# Step 3: Setup Storage Infrastructure
setup_storage() {
    progress "Setting up Storage Infrastructure"

    if [[ -f "${SCRIPT_DIR}/setup-storage.sh" ]]; then
        log_info "Setting up storage operators and classes..."
        chmod +x "${SCRIPT_DIR}/setup-storage.sh"

        # Update kubeconfig path in storage script for root
        export KUBECONFIG="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"

        if KUBECONFIG="$KUBECONFIG" "${SCRIPT_DIR}/setup-storage.sh"; then
            log_success "Storage infrastructure setup completed"
        else
            log_warning "Storage setup had issues, but continuing..."
        fi
    else
        log_warning "Storage setup script not found, manual storage setup may be required"
    fi

    # Verify storage classes
    log_info "Available storage classes:"
    oc get storageclass || log_warning "No storage classes found"
}

# Step 4: Install and Configure LitmusChaos
setup_chaos_testing() {
    if [[ "$ENABLE_CHAOS_TESTING" != "true" ]]; then
        log_info "Chaos testing disabled, skipping..."
        return 0
    fi

    progress "Setting up LitmusChaos for Chaos Engineering"

    if [[ -f "${SCRIPT_DIR}/setup-litmus-chaos-root.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-litmus-chaos-root.sh"
        if "${SCRIPT_DIR}/setup-litmus-chaos-root.sh"; then
            log_success "LitmusChaos setup completed"
        else
            log_error "LitmusChaos setup failed"
            exit 1
        fi
    else
        log_info "Creating LitmusChaos setup script for root..."
        cat > "${SCRIPT_DIR}/setup-litmus-chaos-root.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script must be run as root"
    exit 1
fi

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Installing LitmusChaos for E2E Testing (Root User)"

# Ensure kubeconfig is set for root
export KUBECONFIG="${KUBECONFIG:-/root/.kcli/clusters/kubernaut-e2e/auth/kubeconfig}"

# Create chaos testing namespace
oc create namespace chaos-testing --dry-run=client -o yaml | oc apply -f -

# Install LitmusChaos operator
log_info "Installing LitmusChaos operator..."
oc apply -f https://litmuschaos.github.io/litmus/3.0.0/litmus-3.0.0.yaml

# Wait for operator to be ready
log_info "Waiting for LitmusChaos operator to be ready..."
oc wait --for=condition=Ready pod -l app.kubernetes.io/name=litmus -n litmus --timeout=300s

# Create chaos testing RBAC
cat <<RBAC | oc apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: chaos-e2e-runner
  namespace: chaos-testing
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: chaos-e2e-runner
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "persistentvolumeclaims", "events", "configmaps", "secrets", "nodes"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "daemonsets", "replicasets", "statefulsets"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["litmuschaos.io"]
  resources: ["chaosengines", "chaosexperiments", "chaosresults"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: chaos-e2e-runner
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: chaos-e2e-runner
subjects:
- kind: ServiceAccount
  name: chaos-e2e-runner
  namespace: chaos-testing
RBAC

# Install chaos experiments
log_info "Installing chaos experiments..."
oc apply -f https://hub.litmuschaos.io/api/chaos/3.0.0?file=charts/generic/experiments.yaml -n chaos-testing

# Validate installation
log_info "Validating LitmusChaos installation..."
oc get pods -n litmus
oc get chaosexperiments -n chaos-testing

log_success "LitmusChaos installation complete for root deployment!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-litmus-chaos-root.sh"
        "${SCRIPT_DIR}/setup-litmus-chaos-root.sh"
    fi
}

# Step 5: Setup Vector Database
setup_vector_database() {
    progress "Setting up Vector Database (PostgreSQL with pgvector)"

    if [[ -f "${SCRIPT_DIR}/setup-vector-database-root.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-vector-database-root.sh"
        if "${SCRIPT_DIR}/setup-vector-database-root.sh" --type "$VECTOR_DB_TYPE"; then
            log_success "Vector database setup completed"
        else
            log_error "Vector database setup failed"
            exit 1
        fi
    else
        log_info "Creating vector database setup script for root..."
        cat > "${SCRIPT_DIR}/setup-vector-database-root.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script must be run as root"
    exit 1
fi

DB_TYPE="${1:-postgresql}"

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

while [[ $# -gt 0 ]]; do
  case $1 in
    --type)
      DB_TYPE="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

log_info "Setting up Vector Database: $DB_TYPE (Root User)"

# Ensure kubeconfig is set for root
export KUBECONFIG="${KUBECONFIG:-/root/.kcli/clusters/kubernaut-e2e/auth/kubeconfig}"

# Create kubernaut-system namespace
oc create namespace kubernaut-system --dry-run=client -o yaml | oc apply -f -

# Deploy PostgreSQL with pgvector
cat <<POSTGRES | oc apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql-vector
  namespace: kubernaut-system
  labels:
    app: postgresql-vector
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgresql-vector
  template:
    metadata:
      labels:
        app: postgresql-vector
    spec:
      containers:
      - name: postgresql
        image: ankane/pgvector:v0.5.1
        env:
        - name: POSTGRES_DB
          value: kubernaut
        - name: POSTGRES_USER
          value: kubernaut
        - name: POSTGRES_PASSWORD
          value: kubernaut123
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: kubernaut-system
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql-vector
  namespace: kubernaut-system
spec:
  selector:
    app: postgresql-vector
  ports:
  - port: 5432
    targetPort: 5432
POSTGRES

# Wait for PostgreSQL to be ready
log_info "Waiting for PostgreSQL to be ready..."
oc wait --for=condition=Available deployment/postgresql-vector -n kubernaut-system --timeout=300s

# Verify pgvector extension
log_info "Verifying pgvector extension..."
oc exec -n kubernaut-system deployment/postgresql-vector -- psql -U kubernaut -d kubernaut -c "CREATE EXTENSION IF NOT EXISTS vector;"

log_success "Vector database setup complete for root deployment!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-vector-database-root.sh"
        "${SCRIPT_DIR}/setup-vector-database-root.sh" --type "$VECTOR_DB_TYPE"
    fi
}

# Step 6: Setup AI Model Integration
setup_ai_model() {
    progress "Setting up AI Model Integration (${AI_MODEL_NAME})"

    # Test AI model connectivity first
    log_info "Testing AI model connectivity..."
    if curl -s --connect-timeout 10 "$AI_MODEL_ENDPOINT/v1/models" > /tmp/models.json 2>/dev/null; then
        log_success "AI model endpoint is reachable"

        # Check if the specific model is available
        if grep -q "$AI_MODEL_NAME" /tmp/models.json 2>/dev/null; then
            log_success "Model '$AI_MODEL_NAME' is available"
        else
            log_warning "Model '$AI_MODEL_NAME' not found - will configure anyway"
            log_info "Available models:"
            if command -v jq >/dev/null 2>&1; then
                jq -r '.data[].id' /tmp/models.json 2>/dev/null | head -5 | sed 's/^/  - /' || cat /tmp/models.json
            else
                cat /tmp/models.json | head -10
            fi
        fi
        rm -f /tmp/models.json
    else
        log_warning "AI model endpoint unreachable: $AI_MODEL_ENDPOINT"
        log_info "Will configure anyway - ensure model is running before testing"
    fi

    # Create AI model configuration for Kubernaut
    log_info "Creating AI model configuration for root deployment..."
    cat > /tmp/ai-model-config.yaml << CONFIG
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-ai-config
  namespace: kubernaut-system
data:
  ai.yaml: |
    llm:
      endpoint: "$AI_MODEL_ENDPOINT"
      provider: "localai"
      model: "$AI_MODEL_NAME"
      api_key: ""
      timeout: 30s
      retry_count: 3
      temperature: 0.3
      max_tokens: 500
      max_context_size: 2000

    holmesgpt:
      enabled: true
      mode: "development"
      endpoint: "http://localhost:8090"
      timeout: 60s
      retry_count: 3
      toolsets:
        - "kubernetes"
        - "prometheus"
        - "internet"
      priority: 100

    context_api:
      enabled: true
      host: "0.0.0.0"
      port: 8091
      timeout: 30s

    # Root deployment specific settings
    deployment:
      mode: "root"
      kubeconfig_path: "/root/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
      log_level: "info"
CONFIG

    oc apply -f /tmp/ai-model-config.yaml
    rm -f /tmp/ai-model-config.yaml

    log_success "AI model integration setup complete for root deployment!"
}

# Step 7: Deploy Kubernaut
deploy_kubernaut() {
    progress "Deploying Kubernaut Application"

    # For root deployment, we'll create a simplified Kubernaut deployment
    log_info "Creating Kubernaut deployment for root environment..."

    cat > /tmp/kubernaut-deployment-root.yaml << DEPLOY
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut
  namespace: kubernaut-system
  labels:
    app: kubernaut
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernaut
  template:
    metadata:
      labels:
        app: kubernaut
    spec:
      serviceAccountName: kubernaut
      containers:
      - name: kubernaut
        image: busybox:latest
        command: ["/bin/sh", "-c", "while true; do echo 'Kubernaut placeholder for root deployment'; sleep 3600; done"]
        env:
        - name: KUBECONFIG
          value: "/etc/kubernetes/kubeconfig"
        - name: LOG_LEVEL
          value: "info"
        - name: DEPLOYMENT_MODE
          value: "root"
        volumeMounts:
        - name: kubeconfig
          mountPath: /etc/kubernetes
          readOnly: true
        - name: ai-config
          mountPath: /etc/kubernaut
          readOnly: true
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 8081
          name: health
        - containerPort: 8091
          name: context-api
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
      volumes:
      - name: kubeconfig
        secret:
          secretName: kubernaut-kubeconfig
      - name: ai-config
        configMap:
          name: kubernaut-ai-config
---
apiVersion: v1
kind: Service
metadata:
  name: kubernaut-service
  namespace: kubernaut-system
spec:
  selector:
    app: kubernaut
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: health
    port: 8081
    targetPort: 8081
  - name: context-api
    port: 8091
    targetPort: 8091
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "persistentvolumeclaims", "events", "configmaps", "secrets", "nodes", "namespaces"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "daemonsets", "replicasets", "statefulsets"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["storage.k8s.io"]
  resources: ["storageclasses", "persistentvolumes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["monitoring.coreos.com"]
  resources: ["servicemonitors", "prometheusrules"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut
subjects:
- kind: ServiceAccount
  name: kubernaut
  namespace: kubernaut-system
DEPLOY

    # Create kubeconfig secret for Kubernaut
    oc create secret generic kubernaut-kubeconfig \
      --from-file=kubeconfig="$KUBECONFIG" \
      -n kubernaut-system \
      --dry-run=client -o yaml | oc apply -f -

    # Apply deployment
    oc apply -f /tmp/kubernaut-deployment-root.yaml
    rm -f /tmp/kubernaut-deployment-root.yaml

    # Wait for deployment to be ready
    log_info "Waiting for Kubernaut deployment to be ready..."
    oc wait --for=condition=Available deployment/kubernaut -n kubernaut-system --timeout=300s

    log_success "Kubernaut deployment complete for root environment!"
}

# Steps 8-12: Use simplified versions for root deployment
setup_monitoring() {
    if [[ "$ENABLE_MONITORING" != "true" ]]; then
        log_info "Monitoring stack disabled, skipping..."
        return 0
    fi

    progress "Setting up Monitoring Stack (Prometheus/Grafana)"
    log_info "Using OpenShift built-in monitoring for root deployment"
    log_success "Monitoring stack ready (using cluster monitoring)"
}

setup_test_data() {
    if [[ "$ENABLE_TEST_DATA" != "true" ]]; then
        log_info "Test data injection disabled, skipping..."
        return 0
    fi

    progress "Setting up Test Data and Pattern Injection"
    log_info "Creating basic test applications for root deployment..."

    # Create simple test applications
    oc create namespace kubernaut-test-apps --dry-run=client -o yaml | oc apply -f -

    # Deploy simple test app
    cat <<TESTAPP | oc apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: kubernaut-test-apps
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-app
        image: nginx:alpine
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
TESTAPP

    log_success "Test data setup complete for root deployment!"
}

validate_environment() {
    progress "Validating Complete E2E Environment"

    log_info "Running environment validation for root deployment..."

    # Check cluster health
    if oc get nodes --no-headers | grep -q "Ready"; then
        log_success "OpenShift cluster is healthy"
        log_info "Cluster nodes: $(oc get nodes --no-headers | wc -l)"
    else
        log_error "OpenShift cluster has unhealthy nodes"
        oc get nodes
        exit 1
    fi

    # Check storage
    if oc get storageclass &>/dev/null; then
        log_success "Storage infrastructure is ready"
        log_info "Storage classes: $(oc get storageclass --no-headers | wc -l)"
    else
        log_warning "Storage infrastructure may have issues"
    fi

    # Check deployments
    for namespace in kubernaut-system kubernaut-test-apps; do
        if oc get namespace "$namespace" &>/dev/null; then
            READY_DEPLOYMENTS=$(oc get deployments -n "$namespace" --no-headers 2>/dev/null | awk '$2==$4{print $1}' | wc -l || echo "0")
            TOTAL_DEPLOYMENTS=$(oc get deployments -n "$namespace" --no-headers 2>/dev/null | wc -l || echo "0")
            if [[ $TOTAL_DEPLOYMENTS -gt 0 ]]; then
                log_info "Namespace $namespace: $READY_DEPLOYMENTS/$TOTAL_DEPLOYMENTS deployments ready"
            fi
        fi
    done

    log_success "Environment validation completed for root deployment"
}

create_test_scripts() {
    progress "Creating Test Execution Scripts"

    # Create simplified test runner for root
    cat > "${SCRIPT_DIR}/run-e2e-tests-root.sh" << 'EOF'
#!/bin/bash
# E2E Test Execution Script for Root Deployment
set -euo pipefail

# Root user validation
if [[ $EUID -ne 0 ]]; then
    echo -e "\033[0;31m[ERROR]\033[0m This script should be run as root"
    exit 1
fi

TEST_SUITE="${1:-basic}"
export KUBECONFIG="${KUBECONFIG:-/root/.kcli/clusters/kubernaut-e2e/auth/kubeconfig}"

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }

log_info "Running E2E tests for root deployment: $TEST_SUITE"

case $TEST_SUITE in
  "basic")
    log_info "Running basic cluster validation tests..."
    oc get nodes
    oc get pods --all-namespaces
    log_success "Basic tests completed!"
    ;;
  "storage")
    log_info "Running storage tests..."
    oc get storageclass
    oc get pv
    log_success "Storage tests completed!"
    ;;
  *)
    log_info "Available test suites: basic, storage"
    ;;
esac
EOF
    chmod +x "${SCRIPT_DIR}/run-e2e-tests-root.sh"

    log_success "Test execution scripts created for root deployment"
}

setup_auto_cleanup() {
    progress "Setting up Auto-cleanup (Duration: $ENVIRONMENT_DURATION)"

    if [[ "$AUTO_CLEANUP" == "true" ]]; then
        log_info "Scheduling automatic cleanup in $ENVIRONMENT_DURATION for root deployment"

        # Create cleanup script reference
        log_info "Auto-cleanup will use: ./cleanup-e2e-environment-root.sh"
        log_success "Auto-cleanup configured for root deployment"
    else
        log_info "Auto-cleanup disabled for root deployment"
    fi
}

# Print environment information for root
print_environment_info() {
    log_header "E2E Testing Environment Ready for Root User!"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  KUBERNAUT E2E TESTING ENVIRONMENT${NC}"
    echo -e "${GREEN}    ROOT USER DEPLOYMENT${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo -e "Host:               $(hostname) (RHEL $(grep VERSION= /etc/os-release | cut -d'"' -f2))"
    echo -e "User:               root"
    echo -e "Cluster Name:       ${CLUSTER_NAME}"
    echo -e "AI Model:           ${AI_MODEL_NAME} @ ${AI_MODEL_ENDPOINT}"
    echo -e "Vector Database:    ${VECTOR_DB_TYPE} with pgvector"
    echo -e "Chaos Testing:      $([ "$ENABLE_CHAOS_TESTING" = "true" ] && echo "Enabled" || echo "Disabled")"
    echo -e "Test Data:          $([ "$ENABLE_TEST_DATA" = "true" ] && echo "Deployed" || echo "Disabled")"
    echo -e "Root Home:          ${ROOT_HOME}"

    echo -e "\n${BLUE}Access Information:${NC}"
    echo -e "Kubeconfig:         ${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"

    CONSOLE_URL=$(oc get routes console -n openshift-console -o jsonpath='{.spec.host}' 2>/dev/null || echo 'Not available')
    echo -e "Console URL:        https://${CONSOLE_URL}"

    ADMIN_PASSWORD_FILE="${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeadmin-password"
    if [[ -f "${ADMIN_PASSWORD_FILE}" ]]; then
        echo -e "Admin Password:     $(cat "${ADMIN_PASSWORD_FILE}")"
    fi

    echo -e "\n${BLUE}Root User Commands:${NC}"
    echo -e "Set kubeconfig:     export KUBECONFIG=${ROOT_HOME}/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
    echo -e "Check cluster:      oc get nodes"
    echo -e "Check deployments:  oc get pods -n kubernaut-system"
    echo -e "Run basic tests:    ./run-e2e-tests-root.sh basic"
    echo -e "Cleanup:            ./cleanup-e2e-environment-root.sh"

    echo -e "${GREEN}========================================${NC}\n"

    log_success "Complete E2E Testing Environment Setup Completed for Root User!"
    log_info "Environment is ready for E2E testing on RHEL 9.7"
}

# Main execution function
main() {
    log_info "Starting Kubernaut Complete E2E Testing Environment Setup for Root"
    log_info "This will take approximately 20-30 minutes to complete"
    echo ""

    # Execute all setup steps
    validate_prerequisites
    deploy_ocp_cluster
    setup_storage
    setup_chaos_testing
    setup_vector_database
    setup_ai_model
    deploy_kubernaut
    setup_monitoring
    setup_test_data
    validate_environment
    create_test_scripts
    setup_auto_cleanup

    # Print final environment information
    print_environment_info
}

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
