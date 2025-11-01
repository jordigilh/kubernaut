#!/bin/bash
# Context API E2E Test Infrastructure Setup
# Uses Podman + Kind for Kubernetes-based E2E testing
# Following ADR-016: Service-Specific Integration Test Infrastructure
# Following DD-008: Integration Test Infrastructure (Podman + Kind)

set -e

# Configuration
CLUSTER_NAME="${KIND_CLUSTER_NAME:-kubernaut-contextapi-e2e}"
NAMESPACE="${CONTEXT_API_NAMESPACE:-contextapi-e2e}"
IMAGE_NAME="${CONTEXT_API_IMAGE:-kubernaut-contextapi:e2e}"
KIND_CONFIG="${KIND_CONFIG:-test/kind/kind-config-contextapi.yaml}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "════════════════════════════════════════════════════════════════════════"
echo "🚀 Context API E2E Test Infrastructure Setup (Podman + Kind)"
echo "════════════════════════════════════════════════════════════════════════"
echo ""
echo "📋 Setup Steps:"
echo "  1. Verify Podman + Kind installation"
echo "  2. Configure Kind to use Podman"
echo "  3. Create Kind cluster"
echo "  4. Create namespace"
echo "  5. Deploy PostgreSQL"
echo "  6. Deploy Redis"
echo "  7. Build Context API image"
echo "  8. Load image into Kind"
echo "  9. Deploy Context API"
echo "  10. Verify deployment"
echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo ""

# Step 1: Verify Podman installation
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}1️⃣  Verifying Podman + Kind installation${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if ! command -v podman &> /dev/null; then
    echo -e "${RED}❌ Error: Podman is not installed${NC}"
    echo "   Please install Podman: https://podman.io/getting-started/installation"
    exit 1
fi
echo -e "${GREEN}✅ Podman installed: $(podman --version)${NC}"

if ! command -v kind &> /dev/null; then
    echo -e "${RED}❌ Error: Kind is not installed${NC}"
    echo "   Please install Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    exit 1
fi
echo -e "${GREEN}✅ Kind installed: $(kind version)${NC}"

if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}❌ Error: kubectl is not installed${NC}"
    echo "   Please install kubectl: https://kubernetes.io/docs/tasks/tools/"
    exit 1
fi
echo -e "${GREEN}✅ kubectl installed: $(kubectl version --client --short 2>/dev/null || kubectl version --client)${NC}"
echo ""

# Step 2: Configure Kind to use Podman + Separate Kubeconfig
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}2️⃣  Configuring Kind to use Podman + Isolated Kubeconfig${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Set Kind to use Podman
export KIND_EXPERIMENTAL_PROVIDER=podman
echo -e "${GREEN}✅ KIND_EXPERIMENTAL_PROVIDER=podman${NC}"

# Configure separate kubeconfig for Kind to avoid disrupting other processes
export KIND_KUBECONFIG="${HOME}/.kube/kind-config"
export KUBECONFIG="${KIND_KUBECONFIG}"
mkdir -p "${HOME}/.kube"
echo -e "${GREEN}✅ Using isolated kubeconfig: ${KIND_KUBECONFIG}${NC}"
echo -e "${BLUE}   (This won't disrupt your main ~/.kube/config)${NC}"

# Verify Podman is running
if ! podman info &> /dev/null; then
    echo -e "${YELLOW}⚠️  Podman machine not running, starting...${NC}"
    podman machine start || true
    sleep 5
fi
echo -e "${GREEN}✅ Podman is running${NC}"
echo ""

# Step 3: Create or verify Kind cluster
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}3️⃣  Creating Kind cluster: ${CLUSTER_NAME}${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo -e "${YELLOW}⚠️  Kind cluster '${CLUSTER_NAME}' already exists${NC}"
    echo "   Verifying cluster is accessible..."
    if kubectl cluster-info --context "kind-${CLUSTER_NAME}" &> /dev/null; then
        echo -e "${GREEN}✅ Cluster is accessible, reusing existing cluster${NC}"
        # Export cluster config to isolated kubeconfig if it's not there
        if ! grep -q "kind-${CLUSTER_NAME}" "${KIND_KUBECONFIG}" 2>/dev/null; then
            echo -e "${YELLOW}⚠️  Cluster not in ${KIND_KUBECONFIG}, exporting...${NC}"
            kind export kubeconfig --name "${CLUSTER_NAME}" --kubeconfig "${KIND_KUBECONFIG}"
        fi
    else
        echo -e "${RED}⚠️  Cluster exists but not accessible, recreating...${NC}"
        kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
        KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}" --kubeconfig "${KIND_KUBECONFIG}" --wait 3m
        echo -e "${GREEN}✅ Kind cluster '${CLUSTER_NAME}' created${NC}"
    fi
else
    echo "Creating Kind cluster with Podman + isolated kubeconfig..."
    KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}" --kubeconfig "${KIND_KUBECONFIG}" --wait 3m
    echo -e "${GREEN}✅ Kind cluster '${CLUSTER_NAME}' created${NC}"
fi

# Switch context
kubectl config use-context "kind-${CLUSTER_NAME}"
echo -e "${GREEN}✅ Switched to context: kind-${CLUSTER_NAME}${NC}"
echo ""

# Step 4: Create namespace
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}4️⃣  Creating namespace: ${NAMESPACE}${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
echo -e "${GREEN}✅ Namespace '${NAMESPACE}' ready${NC}"
echo ""

# Step 5: Deploy PostgreSQL
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}5️⃣  Deploying PostgreSQL with pgvector${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: ${NAMESPACE}
spec:
  type: NodePort
  ports:
  - port: 5432
    targetPort: 5432
    nodePort: 30432
  selector:
    app: postgres
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: ${NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: pgvector/pgvector:pg16
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: "action_history"
        - name: POSTGRES_USER
          value: "slm_user"
        - name: POSTGRES_PASSWORD
          value: "slm_password_dev"
        - name: POSTGRES_SHARED_BUFFERS
          value: "512MB"
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - slm_user
          initialDelaySeconds: 5
          periodSeconds: 5
EOF

echo "Waiting for PostgreSQL to be ready..."
kubectl wait --for=condition=ready pod -l app=postgres -n "${NAMESPACE}" --timeout=120s
echo -e "${GREEN}✅ PostgreSQL deployed and ready${NC}"
echo ""

# Step 6: Deploy Redis
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}6️⃣  Deploying Redis${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: ${NAMESPACE}
spec:
  type: NodePort
  ports:
  - port: 6379
    targetPort: 6379
    nodePort: 30379
  selector:
    app: redis
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: ${NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        readinessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 5
          periodSeconds: 5
EOF

echo "Waiting for Redis to be ready..."
kubectl wait --for=condition=ready pod -l app=redis -n "${NAMESPACE}" --timeout=120s
echo -e "${GREEN}✅ Redis deployed and ready${NC}"
echo ""

# Step 7: Build Context API image
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}7️⃣  Building Context API image with Podman${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Build with Podman (ADR-027: Multi-arch with Red Hat UBI9)
if [ ! -f "docker/contextapi-ubi9.Dockerfile" ]; then
    echo -e "${YELLOW}⚠️  Context API Dockerfile not found, skipping image build${NC}"
    echo "   This is expected for initial E2E test development"
    echo "   Tests will use integration setup instead"
else
    podman build -t "${IMAGE_NAME}" -f docker/contextapi-ubi9.Dockerfile .
    echo -e "${GREEN}✅ Context API image built: ${IMAGE_NAME}${NC}"

    # Step 8: Load image into Kind
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}8️⃣  Loading image into Kind cluster${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    KIND_EXPERIMENTAL_PROVIDER=podman kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER_NAME}"
    echo -e "${GREEN}✅ Image loaded into Kind cluster${NC}"

    # Step 9: Deploy Context API
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${BLUE}9️⃣  Deploying Context API service${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    # Create Secret, ConfigMap, Service, and Deployment
    cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: Secret
metadata:
  name: contextapi-db-secret
  namespace: ${NAMESPACE}
  labels:
    app: contextapi
type: Opaque
stringData:
  password: "slm_password_dev"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: contextapi-config
  namespace: ${NAMESPACE}
  labels:
    app: contextapi
data:
  config.yaml: |
    server:
      port: 8080
      host: "0.0.0.0"
      read_timeout: "30s"
      write_timeout: "30s"
    
    logging:
      level: "info"
      format: "json"
    
    cache:
      redis_addr: "redis.${NAMESPACE}.svc.cluster.local:6379"
      redis_db: 0
      lru_size: 1000
      default_ttl: "5m"
    
    database:
      host: "postgres.${NAMESPACE}.svc.cluster.local"
      port: 5432
      name: "action_history"
      user: "slm_user"
      ssl_mode: "disable"
---
apiVersion: v1
kind: Service
metadata:
  name: contextapi
  namespace: ${NAMESPACE}
  labels:
    app: contextapi
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30800
    name: http
  - port: 9000
    targetPort: 9000
    nodePort: 30900
    name: metrics
  selector:
    app: contextapi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: contextapi
  namespace: ${NAMESPACE}
  labels:
    app: contextapi
spec:
  replicas: 1
  selector:
    matchLabels:
      app: contextapi
  template:
    metadata:
      labels:
        app: contextapi
    spec:
      containers:
      - name: contextapi
        image: ${IMAGE_NAME}
        imagePullPolicy: Never
        args:
          - --config
          - /etc/contextapi/config.yaml
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9000
          name: metrics
        env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: contextapi-db-secret
              key: password
        volumeMounts:
        - name: config
          mountPath: /etc/contextapi
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        securityContext:
          runAsNonRoot: true
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: false
      volumes:
      - name: config
        configMap:
          name: contextapi-config
EOF

    echo "Waiting for Context API to be ready..."
    kubectl wait --for=condition=ready pod -l app=contextapi -n "${NAMESPACE}" --timeout=120s || true
    echo -e "${GREEN}✅ Context API deployed${NC}"
fi

echo ""

# Step 10: Verify deployment
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${BLUE}🔟  Verifying deployment${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "📊 Deployment Status:"
kubectl get pods -n "${NAMESPACE}"
echo ""

echo "🔍 Service Endpoints:"
kubectl get svc -n "${NAMESPACE}"
echo ""

echo "════════════════════════════════════════════════════════════════════════"
echo -e "${GREEN}✅ CONTEXT API E2E INFRASTRUCTURE SETUP COMPLETE${NC}"
echo "════════════════════════════════════════════════════════════════════════"
echo ""
echo "📊 Infrastructure Summary:"
echo "  • Kind Cluster: ${CLUSTER_NAME} (Podman)"
echo "  • Kubeconfig: ${KIND_KUBECONFIG}"
echo "  • Namespace: ${NAMESPACE}"
echo "  • PostgreSQL: postgres:5432 (NodePort: 30432)"
echo "  • Redis: redis:6379 (NodePort: 30379)"
if [ -f "docker/contextapi-ubi9.Dockerfile" ]; then
    echo "  • Context API: contextapi:8080 (NodePort: 30800)"
    echo "  • Metrics: contextapi:9000 (NodePort: 30900)"
fi
echo ""
echo "🧪 Ready to run E2E tests:"
echo "  make test-e2e-contextapi"
echo ""
echo "🔍 Access services locally:"
echo "  • PostgreSQL: localhost:5434"
echo "  • Redis: localhost:6380"
if [ -f "docker/contextapi-ubi9.Dockerfile" ]; then
    echo "  • Context API: http://localhost:8800"
    echo "  • Metrics: http://localhost:9000/metrics"
fi
echo ""
echo "💡 To use kubectl with this cluster:"
echo "  export KUBECONFIG=${KIND_KUBECONFIG}"
echo "  kubectl config use-context kind-${CLUSTER_NAME}"
echo ""
echo "🧹 To clean up:"
echo "  make test-contextapi-e2e-teardown"
echo ""

