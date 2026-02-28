#!/usr/bin/env bash
# Deploy Gitea (lightweight Git server) for GitOps demo scenarios
# Uses Helm chart with SQLite backend (minimal memory footprint ~200-300MB)
#
# Usage: ./deploy/demo/scenarios/gitops/scripts/setup-gitea.sh
set -euo pipefail

GITEA_NAMESPACE="gitea"
GITEA_ADMIN_USER="kubernaut"
GITEA_ADMIN_PASS="kubernaut123"
REPO_NAME="demo-gitops-repo"

echo "==> Installing Gitea in namespace ${GITEA_NAMESPACE}..."

kubectl create namespace "${GITEA_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

helm repo add gitea-charts https://dl.gitea.com/charts/ 2>/dev/null || true
helm repo update gitea-charts

helm upgrade --install gitea gitea-charts/gitea \
  --namespace "${GITEA_NAMESPACE}" \
  --set gitea.admin.username="${GITEA_ADMIN_USER}" \
  --set gitea.admin.password="${GITEA_ADMIN_PASS}" \
  --set gitea.admin.email="admin@kubernaut.ai" \
  --set persistence.enabled=false \
  --set "gitea.config.database.DB_TYPE=sqlite3" \
  --set postgresql.enabled=false \
  --set postgresql-ha.enabled=false \
  --set redis-cluster.enabled=false \
  --set "resources.requests.memory=128Mi" \
  --set "resources.requests.cpu=50m" \
  --set "resources.limits.memory=512Mi" \
  --set "resources.limits.cpu=500m" \
  --wait --timeout=300s

echo "==> Waiting for Gitea pod to be ready..."
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=gitea \
  -n "${GITEA_NAMESPACE}" --timeout=120s

GITEA_URL="http://gitea-http.${GITEA_NAMESPACE}:3000"

echo "==> Creating repository via Gitea API..."
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_SETUP_PID=$!
sleep 3

curl -sf -X POST "http://localhost:3000/api/v1/user/repos" \
  -u "${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${REPO_NAME}\", \"auto_init\": false, \"private\": false}" \
  -o /dev/null 2>/dev/null || echo "  Repository may already exist"

kill "${PF_SETUP_PID}" 2>/dev/null || true
sleep 1

echo "==> Pushing initial manifests to Gitea..."
WORK_DIR=$(mktemp -d)
cd "${WORK_DIR}"
git init
git config user.email "kubernaut@kubernaut.ai"
git config user.name "Kubernaut Setup"

mkdir -p manifests

cat > manifests/namespace.yaml <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: demo-gitops
  labels:
    kubernaut.ai/environment: staging
    kubernaut.ai/business-unit: platform
    kubernaut.ai/service-owner: sre-team
    kubernaut.ai/criticality: high
    kubernaut.ai/sla-tier: tier-2
EOF

cat > manifests/configmap.yaml <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: demo-gitops
  labels:
    app: web-frontend
data:
  nginx.conf: |
    worker_processes auto;
    error_log /var/log/nginx/error.log warn;
    pid /tmp/nginx.pid;

    events {
        worker_connections 1024;
    }

    http {
        server {
            listen 8080;
            server_name _;

            location / {
                return 200 'healthy\n';
                add_header Content-Type text/plain;
            }

            location /healthz {
                return 200 'ok\n';
                add_header Content-Type text/plain;
            }
        }
    }
EOF

cat > manifests/deployment.yaml <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-frontend
  namespace: demo-gitops
  labels:
    app: web-frontend
    kubernaut.ai/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-frontend
  template:
    metadata:
      labels:
        app: web-frontend
        kubernaut.ai/managed: "true"
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 5
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 3
      volumes:
      - name: config
        configMap:
          name: nginx-config
EOF

cat > manifests/service.yaml <<'EOF'
apiVersion: v1
kind: Service
metadata:
  name: web-frontend
  namespace: demo-gitops
  labels:
    app: web-frontend
    kubernaut.ai/managed: "true"
spec:
  selector:
    app: web-frontend
  ports:
  - port: 8080
    targetPort: 8080
EOF

git add .
git commit -m "Initial deployment: nginx web-frontend with healthy config"

# Port-forward to push (Gitea is cluster-internal)
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_PID=$!
sleep 3

git remote add origin "http://${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}@localhost:3000/${GITEA_ADMIN_USER}/${REPO_NAME}.git"
git push -u origin main --force

kill "${PF_PID}" 2>/dev/null || true
cd /
rm -rf "${WORK_DIR}"

echo "==> Gitea setup complete"
echo "    URL (in-cluster): ${GITEA_URL}"
echo "    Repo: ${GITEA_URL}/${GITEA_ADMIN_USER}/${REPO_NAME}"
