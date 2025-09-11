#!/bin/bash
# Complete End-to-End Testing Environment Setup for Kubernaut
# Orchestrates the full testing environment including OCP cluster, AI model, storage, chaos testing, and monitoring

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Default values - can be overridden via environment variables
CLUSTER_NAME="${CLUSTER_NAME:-kubernaut-e2e}"
CONFIG_FILE="${CONFIG_FILE:-kcli-baremetal-params.yml}"
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
| |/ /    | |                               | |   |  ___/ ___||  ___| |_   _|          | | (_)
| ' /_   _| |__   ___ _ __ _ __   __ _ _   _  | |_  | |__ \___ \| |__     | | ___  ___  | |_ _ _ __   __ _
|  <| | | | '_ \ / _ \ '__| '_ \ / _` | | | | | __| |  __| ___) |  __|    | |/ _ \/ __| | __| | '_ \ / _` |
| . \ |_| | |_) |  __/ |  | | | | (_| | |_| | | |_  | |___\____/| |___    | |  __/\__ \ | |_| | | | | (_| |
|_|\_\__,_|_.__/ \___|_|  |_| |_|\__,_|\__,_|  \__| \____/     \____/    \_|\___||___/  \__|_|_| |_|\__, |
                                                                                                     __/ |
    _____                _                                      _                                   |___/
   |  ___|              (_)                                    | |
   | |__ _ ____   ___ __ _ _ __ ___  _ __  _ __ ___   ___ _ __   | |_
   |  __| '_ \ \ / / '__| | '__/ _ \| '_ \| '_ ` _ \ / _ \ '_ \  | __|
   | |__| | | \ V /| |  | | | | (_) | | | | | | | | |  __/ | | | |_
   \____/_| |_|\_/ |_|  |_|_|  \___/|_| |_|_| |_| |_|\___|_| |_|\__|
EOF
echo -e "${NC}"

log_info "Starting Kubernaut Complete E2E Testing Environment Setup"
log_info "Cluster: ${CLUSTER_NAME}"
log_info "AI Model: ${AI_MODEL_NAME} @ ${AI_MODEL_ENDPOINT}"
log_info "Duration: ${ENVIRONMENT_DURATION}"
echo ""

# Cleanup function
cleanup_on_exit() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Setup failed with exit code $exit_code"
        log_info "Check logs and run cleanup if needed: ./cleanup-e2e-environment.sh"
    fi
}

trap cleanup_on_exit EXIT

# Step 1: Validate Prerequisites
validate_prerequisites() {
    progress "Validating Prerequisites"

    # Check if validation script exists and run it
    if [[ -f "${SCRIPT_DIR}/validate-baremetal-setup.sh" ]]; then
        log_info "Running comprehensive validation..."
        chmod +x "${SCRIPT_DIR}/validate-baremetal-setup.sh"
        if "${SCRIPT_DIR}/validate-baremetal-setup.sh" "${CONFIG_FILE}"; then
            log_success "All prerequisites validated"
        else
            log_error "Prerequisite validation failed"
            exit 1
        fi
    else
        log_warning "Validation script not found, running basic checks..."

        # Basic prerequisite checks
        if ! command -v kcli &> /dev/null; then
            log_error "KCLI is not installed"
            exit 1
        fi

        if ! command -v oc &> /dev/null; then
            log_warning "OpenShift CLI (oc) not found - will be installed automatically"
        fi

        # Check pull secret and SSH key
        PULL_SECRET_PATH=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" | sed "s|~|$HOME|" || echo "")
        SSH_KEY_PATH=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/${CONFIG_FILE}" | sed "s|~|$HOME|" || echo "")

        if [[ ! -f "$PULL_SECRET_PATH" ]]; then
            log_error "Pull secret not found: $PULL_SECRET_PATH"
            log_info "Download from: https://console.redhat.com/openshift/install/pull-secret"
            exit 1
        fi

        if [[ ! -f "$SSH_KEY_PATH" ]]; then
            log_error "SSH key not found: $SSH_KEY_PATH"
            log_info "Generate with: ssh-keygen -t rsa -b 4096"
            exit 1
        fi

        log_success "Basic prerequisites validated"
    fi
}

# Step 2: Deploy OpenShift Cluster
deploy_ocp_cluster() {
    progress "Deploying OpenShift Container Platform Cluster"

    # Check if cluster already exists
    if kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
        log_warning "Cluster '${CLUSTER_NAME}' already exists!"
        read -p "Do you want to use the existing cluster? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Using existing cluster"
            # Set kubeconfig for existing cluster
            export KUBECONFIG="$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
            return 0
        else
            log_info "Deleting existing cluster..."
            kcli delete cluster "${CLUSTER_NAME}" --yes
        fi
    fi

    # Deploy new cluster
    if [[ -f "${SCRIPT_DIR}/deploy-kcli-cluster.sh" ]]; then
        log_info "Deploying cluster using KCLI..."
        chmod +x "${SCRIPT_DIR}/deploy-kcli-cluster.sh"

        # Run deployment
        if "${SCRIPT_DIR}/deploy-kcli-cluster.sh" "${CLUSTER_NAME}" "${CONFIG_FILE}"; then
            log_success "OpenShift cluster deployed successfully"
        else
            log_error "Cluster deployment failed"
            exit 1
        fi
    else
        log_error "Cluster deployment script not found"
        exit 1
    fi

    # Set kubeconfig
    export KUBECONFIG="$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"

    # Verify cluster access
    if oc whoami &>/dev/null; then
        log_success "Cluster access verified"
        log_info "Cluster nodes: $(oc get nodes --no-headers | wc -l)"
    else
        log_error "Cannot access cluster"
        exit 1
    fi
}

# Step 3: Setup Storage Infrastructure
setup_storage() {
    progress "Setting up Storage Infrastructure"

    if [[ -f "${SCRIPT_DIR}/setup-storage.sh" ]]; then
        log_info "Setting up storage operators and classes..."
        chmod +x "${SCRIPT_DIR}/setup-storage.sh"

        if "${SCRIPT_DIR}/setup-storage.sh"; then
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

    if [[ -f "${SCRIPT_DIR}/setup-litmus-chaos.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-litmus-chaos.sh"
        if "${SCRIPT_DIR}/setup-litmus-chaos.sh"; then
            log_success "LitmusChaos setup completed"
        else
            log_error "LitmusChaos setup failed"
            exit 1
        fi
    else
        log_info "Creating LitmusChaos setup script..."
        cat > "${SCRIPT_DIR}/setup-litmus-chaos.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Installing LitmusChaos for E2E Testing"

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

log_success "LitmusChaos installation complete!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-litmus-chaos.sh"
        "${SCRIPT_DIR}/setup-litmus-chaos.sh"
    fi
}

# Step 5: Setup Vector Database
setup_vector_database() {
    progress "Setting up Vector Database (PostgreSQL with pgvector)"

    if [[ -f "${SCRIPT_DIR}/setup-vector-database.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-vector-database.sh"
        if "${SCRIPT_DIR}/setup-vector-database.sh" --type "$VECTOR_DB_TYPE"; then
            log_success "Vector database setup completed"
        else
            log_error "Vector database setup failed"
            exit 1
        fi
    else
        log_info "Creating vector database setup script..."
        cat > "${SCRIPT_DIR}/setup-vector-database.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

DB_TYPE="${1:-postgresql}"

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Setting up Vector Database: $DB_TYPE"

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

log_success "Vector database setup complete!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-vector-database.sh"
        "${SCRIPT_DIR}/setup-vector-database.sh" --type "$VECTOR_DB_TYPE"
    fi
}

# Step 6: Setup AI Model Integration
setup_ai_model() {
    progress "Setting up AI Model Integration (${AI_MODEL_NAME})"

    if [[ -f "${SCRIPT_DIR}/setup-ai-model.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-ai-model.sh"
        if "${SCRIPT_DIR}/setup-ai-model.sh" --endpoint "$AI_MODEL_ENDPOINT" --model "$AI_MODEL_NAME"; then
            log_success "AI model integration setup completed"
        else
            log_error "AI model setup failed"
            exit 1
        fi
    else
        log_info "Creating AI model setup script..."
        cat > "${SCRIPT_DIR}/setup-ai-model.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

ENDPOINT="${ENDPOINT:-http://localhost:8080}"
MODEL="${MODEL:-gpt-oss:20b}"

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

while [[ $# -gt 0 ]]; do
  case $1 in
    --endpoint)
      ENDPOINT="$2"
      shift 2
      ;;
    --model)
      MODEL="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

log_info "Setting up AI Model: $MODEL @ $ENDPOINT"

# Test AI model connectivity
log_info "Testing AI model connectivity..."
if curl -s --connect-timeout 10 "$ENDPOINT/v1/models" > /tmp/models.json 2>/dev/null; then
    log_success "AI model endpoint is reachable"

    # Check if the specific model is available
    if grep -q "$MODEL" /tmp/models.json 2>/dev/null; then
        log_success "Model '$MODEL' is available"
    else
        log_error "Model '$MODEL' not found"
        log_info "Available models:"
        if command -v jq >/dev/null 2>&1; then
            jq -r '.data[].id' /tmp/models.json 2>/dev/null || cat /tmp/models.json
        else
            cat /tmp/models.json
        fi
        exit 1
    fi
else
    log_error "AI model endpoint unreachable: $ENDPOINT"
    log_info "Please ensure the AI model is running at $ENDPOINT"
    exit 1
fi

# Create AI model configuration for Kubernaut
cat > /tmp/ai-model-config.yaml << CONFIG
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-ai-config
  namespace: kubernaut-system
data:
  ai.yaml: |
    llm:
      endpoint: "$ENDPOINT"
      provider: "localai"
      model: "$MODEL"
      api_key: ""
      timeout: 30s
      retry_count: 3
      temperature: 0.3
      max_tokens: 500
      max_context_size: 2000

    holmesgpt:
      enabled: true
      mode: "container"
      endpoint: "http://localhost:8090"
      container_image: "kubernaut/holmesgpt-api:latest"
      deployment_target: "local"  # Options: local, cluster
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
CONFIG

oc apply -f /tmp/ai-model-config.yaml

log_success "AI model integration setup complete!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-ai-model.sh"
        "${SCRIPT_DIR}/setup-ai-model.sh" --endpoint "$AI_MODEL_ENDPOINT" --model "$AI_MODEL_NAME"
    fi
}

# Step 7: Deploy Kubernaut
deploy_kubernaut() {
    progress "Deploying Kubernaut Application"

    if [[ -f "${PROJECT_ROOT}/Makefile" ]]; then
        log_info "Building and deploying Kubernaut..."

        # Build Kubernaut
        cd "$PROJECT_ROOT"
        if make build; then
            log_success "Kubernaut built successfully"
        else
            log_error "Kubernaut build failed"
            exit 1
        fi

        # Deploy to cluster
        if [[ -f "${SCRIPT_DIR}/deploy-kubernaut.sh" ]]; then
            chmod +x "${SCRIPT_DIR}/deploy-kubernaut.sh"
            if "${SCRIPT_DIR}/deploy-kubernaut.sh"; then
                log_success "Kubernaut deployed successfully"
            else
                log_error "Kubernaut deployment failed"
                exit 1
            fi
        else
            log_info "Creating Kubernaut deployment script..."
            cat > "${SCRIPT_DIR}/deploy-kubernaut.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Deploying Kubernaut to OpenShift cluster"

# Create deployment manifests
cat > /tmp/kubernaut-deployment.yaml << DEPLOY
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
        image: kubernaut:latest
        imagePullPolicy: IfNotPresent
        env:
        - name: KUBECONFIG
          value: "/etc/kubernetes/kubeconfig"
        - name: LOG_LEVEL
          value: "info"
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
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
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
oc apply -f /tmp/kubernaut-deployment.yaml

# Wait for deployment to be ready
log_info "Waiting for Kubernaut deployment to be ready..."
oc wait --for=condition=Available deployment/kubernaut -n kubernaut-system --timeout=300s

log_success "Kubernaut deployment complete!"
EOF
            chmod +x "${SCRIPT_DIR}/deploy-kubernaut.sh"
            "${SCRIPT_DIR}/deploy-kubernaut.sh"
        fi
    else
        log_warning "Makefile not found, skipping Kubernaut build"
    fi
}

# Step 8: Setup Monitoring Stack
setup_monitoring() {
    if [[ "$ENABLE_MONITORING" != "true" ]]; then
        log_info "Monitoring stack disabled, skipping..."
        return 0
    fi

    progress "Setting up Monitoring Stack (Prometheus/Grafana)"

    if [[ -f "${SCRIPT_DIR}/setup-monitoring-stack.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-monitoring-stack.sh"
        if "${SCRIPT_DIR}/setup-monitoring-stack.sh"; then
            log_success "Monitoring stack setup completed"
        else
            log_warning "Monitoring setup had issues, but continuing..."
        fi
    else
        log_info "Creating monitoring stack setup script..."
        cat > "${SCRIPT_DIR}/setup-monitoring-stack.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Setting up monitoring stack for E2E testing"

# Create monitoring namespace
oc create namespace kubernaut-monitoring --dry-run=client -o yaml | oc apply -f -

# Check if cluster monitoring is available
if oc get prometheus-operator -n openshift-monitoring &>/dev/null; then
    log_success "OpenShift cluster monitoring detected"

    # Create ServiceMonitor for Kubernaut
    cat <<MONITOR | oc apply -f -
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kubernaut-metrics
  namespace: kubernaut-system
  labels:
    app: kubernaut
spec:
  selector:
    matchLabels:
      app: kubernaut
  endpoints:
  - port: http
    interval: 30s
    path: /metrics
MONITOR

    log_success "Kubernaut monitoring integration complete"
else
    log_info "Installing basic Prometheus for monitoring..."
    # Basic Prometheus deployment for environments without cluster monitoring
    cat <<PROMETHEUS | oc apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: kubernaut-monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: prometheus-config
          mountPath: /etc/prometheus
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
      volumes:
      - name: prometheus-config
        configMap:
          name: prometheus-config
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: kubernaut-monitoring
spec:
  selector:
    app: prometheus
  ports:
  - port: 9090
    targetPort: 9090
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: kubernaut-monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
    scrape_configs:
    - job_name: 'kubernaut'
      static_configs:
      - targets: ['kubernaut-service.kubernaut-system:8080']
    - job_name: 'kubernetes-pods'
      kubernetes_sd_configs:
      - role: pod
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
PROMETHEUS

    log_success "Basic Prometheus monitoring setup complete"
fi

log_success "Monitoring stack setup complete!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-monitoring-stack.sh"
        "${SCRIPT_DIR}/setup-monitoring-stack.sh"
    fi
}

# Step 9: Setup Test Data and Patterns
setup_test_data() {
    if [[ "$ENABLE_TEST_DATA" != "true" ]]; then
        log_info "Test data injection disabled, skipping..."
        return 0
    fi

    progress "Setting up Test Data and Pattern Injection"

    if [[ -f "${SCRIPT_DIR}/setup-test-data.sh" ]]; then
        chmod +x "${SCRIPT_DIR}/setup-test-data.sh"
        if "${SCRIPT_DIR}/setup-test-data.sh"; then
            log_success "Test data setup completed"
        else
            log_warning "Test data setup had issues, but continuing..."
        fi
    else
        log_info "Creating test data setup script..."
        cat > "${SCRIPT_DIR}/setup-test-data.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Setting up test data and patterns"

# Create test applications namespace
oc create namespace kubernaut-test-apps --dry-run=client -o yaml | oc apply -f -

# Deploy test applications for chaos testing
cat <<TESTAPPS | oc apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-intensive-app
  namespace: kubernaut-test-apps
  labels:
    app: memory-intensive-app
    test-category: resource-exhaustion
spec:
  replicas: 2
  selector:
    matchLabels:
      app: memory-intensive-app
  template:
    metadata:
      labels:
        app: memory-intensive-app
    spec:
      containers:
      - name: memory-app
        image: nginx:alpine
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        env:
        - name: STRESS_MEMORY
          value: "100M"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cpu-intensive-app
  namespace: kubernaut-test-apps
  labels:
    app: cpu-intensive-app
    test-category: resource-exhaustion
spec:
  replicas: 2
  selector:
    matchLabels:
      app: cpu-intensive-app
  template:
    metadata:
      labels:
        app: cpu-intensive-app
    spec:
      containers:
      - name: cpu-app
        image: nginx:alpine
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "500m"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: database-app
  namespace: kubernaut-test-apps
  labels:
    app: database-app
    test-category: stateful-workload
spec:
  replicas: 1
  selector:
    matchLabels:
      app: database-app
  template:
    metadata:
      labels:
        app: database-app
    spec:
      containers:
      - name: postgres
        image: postgres:13-alpine
        env:
        - name: POSTGRES_DB
          value: testdb
        - name: POSTGRES_USER
          value: testuser
        - name: POSTGRES_PASSWORD
          value: testpass
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-data
          mountPath: /var/lib/postgresql/data
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      volumes:
      - name: postgres-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: database-service
  namespace: kubernaut-test-apps
spec:
  selector:
    app: database-app
  ports:
  - port: 5432
    targetPort: 5432
TESTAPPS

# Wait for test applications to be ready
log_info "Waiting for test applications to be ready..."
oc wait --for=condition=Available deployment/memory-intensive-app -n kubernaut-test-apps --timeout=120s
oc wait --for=condition=Available deployment/cpu-intensive-app -n kubernaut-test-apps --timeout=120s
oc wait --for=condition=Available deployment/database-app -n kubernaut-test-apps --timeout=120s

log_success "Test applications deployed successfully"

# Create test patterns in vector database
log_info "Injecting test patterns into vector database..."

# Create a job to inject test patterns
cat <<PATTERNS | oc apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: pattern-injection
  namespace: kubernaut-system
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: pattern-injector
        image: postgres:13-alpine
        command:
        - /bin/bash
        - -c
        - |
          export PGPASSWORD=kubernaut123

          # Wait for database to be ready
          until pg_isready -h postgresql-vector -p 5432 -U kubernaut; do
            echo "Waiting for database..."
            sleep 2
          done

          # Create tables for patterns
          psql -h postgresql-vector -U kubernaut -d kubernaut -c "
          CREATE EXTENSION IF NOT EXISTS vector;

          CREATE TABLE IF NOT EXISTS patterns (
            id SERIAL PRIMARY KEY,
            pattern_id VARCHAR(255) UNIQUE,
            alert_type VARCHAR(255),
            action_type VARCHAR(255),
            success_rate FLOAT,
            embedding vector(384),
            metadata JSONB,
            created_at TIMESTAMP DEFAULT NOW()
          );

          -- Insert sample patterns
          INSERT INTO patterns (pattern_id, alert_type, action_type, success_rate, embedding, metadata) VALUES
          ('pattern-mem-001', 'HighMemoryUsage', 'increase_resources', 0.95, ARRAY[0.1,0.2,0.3]::vector, '{\"cluster_size\": \"medium\", \"workload_type\": \"web\"}'),
          ('pattern-cpu-001', 'HighCPUUsage', 'scale_deployment', 0.88, ARRAY[0.2,0.3,0.4]::vector, '{\"cluster_size\": \"large\", \"workload_type\": \"compute\"}'),
          ('pattern-disk-001', 'DiskSpaceRunningOut', 'cleanup_workload', 0.92, ARRAY[0.3,0.4,0.5]::vector, '{\"storage_type\": \"persistent\", \"cleanup_strategy\": \"logs\"}'),
          ('pattern-pod-001', 'PodCrashLoopBackOff', 'restart_pod', 0.75, ARRAY[0.4,0.5,0.6]::vector, '{\"error_type\": \"config\", \"restart_count\": 3}'),
          ('pattern-net-001', 'NetworkUnavailable', 'restart_network', 0.82, ARRAY[0.5,0.6,0.7]::vector, '{\"network_plugin\": \"ovn\", \"impact_scope\": \"cluster\"}');
          "

          echo "Test patterns injected successfully"
PATTERNS

# Wait for pattern injection to complete
log_info "Waiting for pattern injection to complete..."
oc wait --for=condition=Complete job/pattern-injection -n kubernaut-system --timeout=120s

log_success "Test data and patterns setup complete!"
EOF
        chmod +x "${SCRIPT_DIR}/setup-test-data.sh"
        "${SCRIPT_DIR}/setup-test-data.sh"
    fi
}

# Step 10: Validate Complete Environment
validate_environment() {
    progress "Validating Complete E2E Environment"

    log_info "Running comprehensive environment validation..."

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
    if oc get storageclass &>/dev/null && oc get pv &>/dev/null; then
        log_success "Storage infrastructure is ready"
        log_info "Storage classes: $(oc get storageclass --no-headers | wc -l)"
    else
        log_warning "Storage infrastructure may have issues"
    fi

    # Check Kubernaut deployment
    if oc get deployment kubernaut -n kubernaut-system &>/dev/null; then
        if oc wait --for=condition=Available deployment/kubernaut -n kubernaut-system --timeout=30s &>/dev/null; then
            log_success "Kubernaut application is running"
        else
            log_warning "Kubernaut application may not be fully ready"
        fi
    else
        log_warning "Kubernaut application not found"
    fi

    # Check vector database
    if oc get deployment postgresql-vector -n kubernaut-system &>/dev/null; then
        if oc wait --for=condition=Available deployment/postgresql-vector -n kubernaut-system --timeout=30s &>/dev/null; then
            log_success "Vector database is running"
        else
            log_warning "Vector database may not be ready"
        fi
    else
        log_warning "Vector database not found"
    fi

    # Check chaos testing
    if [[ "$ENABLE_CHAOS_TESTING" == "true" ]]; then
        if oc get namespace chaos-testing &>/dev/null && oc get chaosexperiments -n chaos-testing &>/dev/null; then
            log_success "Chaos testing framework is ready"
        else
            log_warning "Chaos testing framework may not be ready"
        fi
    fi

    # Check AI model connectivity
    if curl -s --connect-timeout 5 "$AI_MODEL_ENDPOINT/v1/models" &>/dev/null; then
        log_success "AI model endpoint is accessible"
    else
        log_warning "AI model endpoint may not be accessible: $AI_MODEL_ENDPOINT"
    fi

    # Check test applications
    if [[ "$ENABLE_TEST_DATA" == "true" ]]; then
        if oc get namespace kubernaut-test-apps &>/dev/null; then
            READY_APPS=$(oc get deployments -n kubernaut-test-apps --no-headers | awk '$2==$4{print $1}' | wc -l)
            TOTAL_APPS=$(oc get deployments -n kubernaut-test-apps --no-headers | wc -l)
            if [[ $READY_APPS -eq $TOTAL_APPS ]]; then
                log_success "Test applications are ready ($READY_APPS/$TOTAL_APPS)"
            else
                log_warning "Some test applications may not be ready ($READY_APPS/$TOTAL_APPS)"
            fi
        fi
    fi

    log_success "Environment validation completed"
}

# Step 11: Create Test Execution Scripts
create_test_scripts() {
    progress "Creating Test Execution Scripts"

    # Create test runner script
    cat > "${SCRIPT_DIR}/run-e2e-tests.sh" << 'EOF'
#!/bin/bash
# E2E Test Execution Script
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Test configuration
TEST_SUITE="${1:-all}"
CHAOS_ENABLED="${CHAOS_ENABLED:-true}"
DURATION="${DURATION:-30m}"

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

log_info "Running E2E tests: $TEST_SUITE"

# Set environment variables for tests
export KUBECONFIG="${KUBECONFIG:-$HOME/.kcli/clusters/kubernaut-e2e/auth/kubeconfig}"
export LLM_ENDPOINT="http://localhost:8080"
export LLM_MODEL="gpt-oss:20b"
export LLM_PROVIDER="localai"

cd "$PROJECT_ROOT"

case $TEST_SUITE in
  "all")
    log_info "Running all E2E test suites..."
    make test-e2e-use-cases || true
    if [[ "$CHAOS_ENABLED" == "true" ]]; then
      make test-e2e-chaos || true
    fi
    make test-e2e-stress || true
    ;;
  "use-cases")
    log_info "Running E2E use case tests..."
    make test-e2e-use-cases
    ;;
  "chaos")
    if [[ "$CHAOS_ENABLED" == "true" ]]; then
      log_info "Running chaos engineering tests..."
      make test-e2e-chaos
    else
      log_error "Chaos testing is disabled"
      exit 1
    fi
    ;;
  "stress")
    log_info "Running AI model stress tests..."
    make test-e2e-stress
    ;;
  *)
    log_error "Unknown test suite: $TEST_SUITE"
    log_info "Available suites: all, use-cases, chaos, stress"
    exit 1
    ;;
esac

log_success "E2E test execution completed!"
EOF
    chmod +x "${SCRIPT_DIR}/run-e2e-tests.sh"

    # Create individual use case test scripts
    for i in {1..10}; do
        cat > "${SCRIPT_DIR}/run-use-case-${i}.sh" << EOF
#!/bin/bash
# Use Case $i Test Execution Script
set -euo pipefail

SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="\$(cd "\${SCRIPT_DIR}/../../.." && pwd)"

log_info() { echo -e "\033[0;34m[INFO]\033[0m \$1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m \$1"; }

export KUBECONFIG="\${KUBECONFIG:-\$HOME/.kcli/clusters/kubernaut-e2e/auth/kubeconfig}"
export LLM_ENDPOINT="http://localhost:8080"
export LLM_MODEL="gpt-oss:20b"

log_info "Running Use Case $i test..."

cd "\$PROJECT_ROOT"
make test-e2e-use-case-$i

log_success "Use Case $i test completed!"
EOF
        chmod +x "${SCRIPT_DIR}/run-use-case-${i}.sh"
    done

    log_success "Test execution scripts created"
}

# Step 12: Setup Auto-cleanup
setup_auto_cleanup() {
    progress "Setting up Auto-cleanup (Duration: $ENVIRONMENT_DURATION)"

    if [[ "$AUTO_CLEANUP" == "true" ]]; then
        log_info "Scheduling automatic cleanup in $ENVIRONMENT_DURATION"

        # Convert duration to seconds
        DURATION_SECONDS=$(echo "$ENVIRONMENT_DURATION" | sed 's/h/*3600+/g; s/m/*60+/g; s/s/+/g' | sed 's/+$//' | bc)

        # Create cleanup script
        cat > "${SCRIPT_DIR}/auto-cleanup.sh" << 'EOF'
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }

log_info "Auto-cleanup triggered"

# Run cleanup
if [[ -f "${SCRIPT_DIR}/cleanup-e2e-environment.sh" ]]; then
    chmod +x "${SCRIPT_DIR}/cleanup-e2e-environment.sh"
    "${SCRIPT_DIR}/cleanup-e2e-environment.sh"
else
    log_info "Cleanup script not found, manual cleanup may be required"
fi

log_success "Auto-cleanup completed"
EOF
        chmod +x "${SCRIPT_DIR}/auto-cleanup.sh"

        # Schedule cleanup in background
        (sleep "$DURATION_SECONDS" && "${SCRIPT_DIR}/auto-cleanup.sh") &
        CLEANUP_PID=$!
        echo "$CLEANUP_PID" > /tmp/kubernaut-e2e-cleanup.pid

        log_success "Auto-cleanup scheduled (PID: $CLEANUP_PID)"
        log_info "To cancel: kill $CLEANUP_PID"
    else
        log_info "Auto-cleanup disabled"
    fi
}

# Print environment information
print_environment_info() {
    log_header "E2E Testing Environment Ready!"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  KUBERNAUT E2E TESTING ENVIRONMENT${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo -e "Cluster Name:        ${CLUSTER_NAME}"
    echo -e "AI Model:           ${AI_MODEL_NAME} @ ${AI_MODEL_ENDPOINT}"
    echo -e "Vector Database:    ${VECTOR_DB_TYPE} with pgvector"
    echo -e "Chaos Testing:      $([ "$ENABLE_CHAOS_TESTING" = "true" ] && echo "Enabled" || echo "Disabled")"
    echo -e "Monitoring:         $([ "$ENABLE_MONITORING" = "true" ] && echo "Enabled" || echo "Disabled")"
    echo -e "Test Data:          $([ "$ENABLE_TEST_DATA" = "true" ] && echo "Injected" || echo "Disabled")"
    echo -e "Environment Duration: ${ENVIRONMENT_DURATION}"
    echo -e "Auto-cleanup:       $([ "$AUTO_CLEANUP" = "true" ] && echo "Enabled" || echo "Disabled")"

    echo -e "\n${BLUE}Access Information:${NC}"
    if [[ -f "$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig" ]]; then
        echo -e "Kubeconfig:         $HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
        echo -e "Console URL:        $(oc get routes console -n openshift-console -o jsonpath='{.spec.host}' 2>/dev/null || echo 'Not available')"
        if [[ -f "$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeadmin-password" ]]; then
            echo -e "Admin Password:     $(cat "$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeadmin-password")"
        fi
    fi

    echo -e "\n${BLUE}Test Execution:${NC}"
    echo -e "Run all tests:      ./run-e2e-tests.sh all"
    echo -e "Run use cases:      ./run-e2e-tests.sh use-cases"
    echo -e "Run chaos tests:    ./run-e2e-tests.sh chaos"
    echo -e "Run stress tests:   ./run-e2e-tests.sh stress"
    echo -e "Run specific case:  ./run-use-case-1.sh"

    echo -e "\n${BLUE}Environment Commands:${NC}"
    echo -e "Set kubeconfig:     export KUBECONFIG=$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
    echo -e "Check cluster:      oc get nodes"
    echo -e "Check Kubernaut:    oc get pods -n kubernaut-system"
    echo -e "View logs:          oc logs -f deployment/kubernaut -n kubernaut-system"
    echo -e "Cleanup:            ./cleanup-e2e-environment.sh"

    echo -e "${GREEN}========================================${NC}\n"

    log_success "Complete E2E Testing Environment Setup Completed Successfully!"
    log_info "Environment is ready for Top 10 E2E Use Case testing"

    if [[ "$AUTO_CLEANUP" == "true" ]]; then
        log_warning "Environment will auto-cleanup in $ENVIRONMENT_DURATION"
        log_info "To cancel auto-cleanup: kill $(cat /tmp/kubernaut-e2e-cleanup.pid 2>/dev/null || echo 'PID-FILE-NOT-FOUND')"
    fi
}

# Main execution function
main() {
    log_info "Starting Kubernaut Complete E2E Testing Environment Setup"
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
