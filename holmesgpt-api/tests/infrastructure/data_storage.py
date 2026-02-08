"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Data Storage Stack Deployment for HAPI E2E Tests

Deploys the full Data Storage stack to Kind cluster:
- PostgreSQL
- Redis for DLQ
- Data Storage Service

Per DD-TEST-001 v1.2 port allocations:
- Data Storage: 8089/30089
- PostgreSQL: 5488/30488
- Embedding: 8188/30288
- Redis: 6388/30388
"""

import os
import subprocess
import time
from pathlib import Path
from typing import Optional

from .kind_cluster import KindCluster, find_workspace_root, build_image


# Port allocations per DD-TEST-001 v1.2
PORTS = {
    "data_storage": {"host": 8089, "nodeport": 30089, "metrics_nodeport": 30189},
    "postgresql": {"host": 5488, "nodeport": 30488, "container": 5432},
    "redis": {"host": 6388, "nodeport": 30388, "container": 6379},
    "embedding": {"host": 8188, "nodeport": 30288, "container": 8000},
}


class DataStorageDeployment:
    """
    Deploys Data Storage service and dependencies to Kind cluster.

    This matches the Go implementation in test/infrastructure/datastorage.go
    but uses Python subprocess calls to kubectl.
    """

    NAMESPACE = "holmesgpt-e2e"

    def __init__(self, cluster: KindCluster):
        self.cluster = cluster
        self.manifests_dir = Path(__file__).parent / "manifests"

    def deploy(self, skip_embedding: bool = False) -> str:
        """
        Deploy full Data Storage stack to the cluster.

        Args:
            skip_embedding: Skip embedding service (use mock in Data Storage)

        Returns:
            Data Storage URL (e.g., "http://localhost:8089")
        """
        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
        print(f"Deploying Data Storage Stack in Namespace: {self.NAMESPACE}")
        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

        # 1. Build and load Data Storage image
        print("🔨 Building Data Storage Docker image...")
        self._build_data_storage_image()

        # 2. Create namespace
        print(f"📁 Creating namespace {self.NAMESPACE}...")
        self._create_namespace()

        # 3. Deploy PostgreSQL
        print("🐘 Deploying PostgreSQL...")
        self._deploy_postgresql()

        # 4. Deploy Redis for DLQ
        print("📦 Deploying Redis...")
        self._deploy_redis()

        # 5. Apply database migrations
        print("📋 Applying database migrations...")
        self._apply_migrations()

        # 6. Deploy Embedding Service (optional)
        if not skip_embedding:
            print("🧠 Deploying Embedding Service...")
            self._deploy_embedding_service()

        # 7. Deploy Data Storage Service
        print("🚀 Deploying Data Storage Service...")
        self._deploy_data_storage_service(skip_embedding)

        # 8. Wait for all services ready
        print("⏳ Waiting for services to be ready...")
        self._wait_for_ready()

        url = f"http://localhost:{PORTS['data_storage']['host']}"
        print(f"✅ Data Storage stack ready at {url}")
        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

        return url

    def _build_data_storage_image(self) -> None:
        """Build Data Storage image using Podman."""
        workspace_root = find_workspace_root()

        build_image(
            image_name="localhost/kubernaut-datastorage:e2e-test",
            dockerfile="docker/datastorage-ubi9.Dockerfile",
            context_dir=workspace_root
        )

        # Load into Kind cluster
        self.cluster.load_image("localhost/kubernaut-datastorage:e2e-test")

    def _create_namespace(self) -> None:
        """Create test namespace."""
        result = self.cluster.kubectl(
            "create", "namespace", self.NAMESPACE,
            check=False
        )

        if result.returncode != 0 and "AlreadyExists" not in result.stderr:
            raise RuntimeError(f"Failed to create namespace: {result.stderr}")

        print(f"   ✅ Namespace {self.NAMESPACE} ready")

    def _deploy_postgresql(self) -> None:
        """Deploy PostgreSQL."""
        manifest = self._generate_postgresql_manifest()
        self._apply_manifest(manifest, "postgresql")

    def _deploy_redis(self) -> None:
        """Deploy Redis for DLQ."""
        manifest = self._generate_redis_manifest()
        self._apply_manifest(manifest, "redis")

    def _deploy_embedding_service(self) -> None:
        """Deploy Embedding Service (mock for E2E tests)."""
        manifest = self._generate_embedding_manifest()
        self._apply_manifest(manifest, "embedding")

    def _deploy_data_storage_service(self, skip_embedding: bool = False) -> None:
        """Deploy Data Storage Service."""
        manifest = self._generate_data_storage_manifest(skip_embedding)
        self._apply_manifest(manifest, "datastorage")

    def _apply_manifest(self, manifest: str, name: str) -> None:
        """Apply manifest via kubectl."""
        result = subprocess.run(
            ["kubectl", "--kubeconfig", self.cluster.kubeconfig_path,
             "apply", "-f", "-", "-n", self.NAMESPACE],
            input=manifest,
            text=True,
            capture_output=True
        )

        if result.returncode != 0:
            raise RuntimeError(f"Failed to apply {name} manifest: {result.stderr}")

        print(f"   ✅ {name} deployed")

    def _apply_migrations(self) -> None:
        """Apply database migrations."""
        # Wait for PostgreSQL to be ready
        self._wait_for_pod("app=postgresql", timeout=60)

        # Get pod name
        result = self.cluster.kubectl(
            "get", "pods", "-n", self.NAMESPACE,
            "-l", "app=postgresql",
            "-o", "jsonpath={.items[0].metadata.name}"
        )
        pod_name = result.stdout.strip()

        if not pod_name:
            raise RuntimeError("PostgreSQL pod not found")

        # Apply migrations from workspace
        workspace_root = find_workspace_root()
        migrations_dir = Path(workspace_root) / "migrations"

        migrations = [
            "001_initial_schema.sql",
            "002_fix_partitioning.sql",
            "003_stored_procedures.sql",
            "004_add_effectiveness_assessment_due.sql",
            "005_vector_schema.sql",
            "006_effectiveness_assessment.sql",
            "009_update_vector_dimensions.sql",
            "007_add_context_column.sql",
            "008_context_api_compatibility.sql",
            "010_audit_write_api_phase1.sql",
            "011_rename_alert_to_signal.sql",
            "012_adr033_multidimensional_tracking.sql",
            "013_create_audit_events_table.sql",
            "015_create_workflow_catalog_table.sql",
            "017_add_workflow_schema_fields.sql",
            "018_rename_execution_bundle_to_container_image.sql",
            "019_uuid_primary_key.sql",
            "020_add_workflow_label_columns.sql",
            "1000_create_audit_events_partitions.sql",
        ]

        for migration in migrations:
            migration_path = migrations_dir / migration
            if not migration_path.exists():
                print(f"   ⚠️  Skipping {migration} (not found)")
                continue

            content = migration_path.read_text()
            # Remove CONCURRENTLY keyword for test environment
            content = content.replace("CONCURRENTLY ", "")
            # Extract only UP migration
            if "-- +goose Down" in content:
                content = content.split("-- +goose Down")[0]

            # Apply via kubectl exec
            result = subprocess.run(
                ["kubectl", "--kubeconfig", self.cluster.kubeconfig_path,
                 "exec", "-i", "-n", self.NAMESPACE, pod_name, "--",
                 "psql", "-U", "slm_user", "-d", "action_history"],
                input=content,
                text=True,
                capture_output=True
            )

            if result.returncode != 0:
                # Check if error is due to already existing objects
                if "already exists" in result.stderr.lower():
                    print(f"   ✅ {migration} (already applied)")
                    continue
                if "ERROR:" not in result.stderr:
                    print(f"   ✅ {migration} (with notices)")
                    continue
                print(f"   ❌ {migration} failed: {result.stderr}")
            else:
                print(f"   ✅ {migration}")

        print("   ✅ Migrations applied")

    def _wait_for_pod(self, label_selector: str, timeout: int = 60) -> None:
        """Wait for pod with label to be ready."""
        deadline = time.time() + timeout

        while time.time() < deadline:
            result = self.cluster.kubectl(
                "get", "pods", "-n", self.NAMESPACE,
                "-l", label_selector,
                "-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}",
                check=False
            )

            if result.returncode == 0 and "True" in result.stdout:
                return

            time.sleep(2)

        raise TimeoutError(f"Pod with label {label_selector} not ready after {timeout}s")

    def _wait_for_ready(self) -> None:
        """Wait for all services to be ready."""
        services = [
            ("postgresql", "app=postgresql"),
            ("redis", "app=redis"),
            ("datastorage", "app=datastorage"),
        ]

        for name, label in services:
            print(f"   ⏳ Waiting for {name}...")
            self._wait_for_pod(label, timeout=120)
            print(f"   ✅ {name} ready")

    def teardown(self) -> None:
        """Remove Data Storage deployment."""
        print(f"🧹 Removing namespace {self.NAMESPACE}...")
        self.cluster.kubectl(
            "delete", "namespace", self.NAMESPACE,
            "--ignore-not-found",
            "--wait=true",
            "--timeout=60s",
            check=False
        )
        print(f"   ✅ Namespace {self.NAMESPACE} deleted")

    # =========================================================================
    # Manifest Generators
    # =========================================================================

    def _generate_postgresql_manifest(self) -> str:
        """Generate PostgreSQL manifest."""
        return f"""---
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgresql-init
data:
  init.sql: |
    GRANT ALL PRIVILEGES ON DATABASE action_history TO slm_user;
    GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
    GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
    GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
stringData:
  POSTGRES_USER: slm_user
  POSTGRES_PASSWORD: test_password
  POSTGRES_DB: action_history
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  labels:
    app: postgresql
spec:
  type: NodePort
  ports:
  - name: postgresql
    port: 5432
    targetPort: 5432
    nodePort: {PORTS['postgresql']['nodeport']}
    protocol: TCP
  selector:
    app: postgresql
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  labels:
    app: postgresql
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
      - name: postgresql
        image: postgres:16-alpine
        ports:
        - containerPort: 5432
        envFrom:
        - secretRef:
            name: postgresql-secret
        env:
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        volumeMounts:
        - name: postgresql-data
          mountPath: /var/lib/postgresql/data
        - name: postgresql-init
          mountPath: /docker-entrypoint-initdb.d
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user"]
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user"]
          initialDelaySeconds: 30
          periodSeconds: 10
      volumes:
      - name: postgresql-data
        emptyDir: {{}}
      - name: postgresql-init
        configMap:
          name: postgresql-init
"""

    def _generate_redis_manifest(self) -> str:
        """Generate Redis manifest."""
        return f"""---
apiVersion: v1
kind: Service
metadata:
  name: redis
  labels:
    app: redis
spec:
  type: NodePort
  ports:
  - name: redis
    port: 6379
    targetPort: 6379
    nodePort: {PORTS['redis']['nodeport']}
    protocol: TCP
  selector:
    app: redis
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  labels:
    app: redis
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
        image: quay.io/jordigilh/redis:7-alpine
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
            command: ["redis-cli", "ping"]
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          exec:
            command: ["redis-cli", "ping"]
          initialDelaySeconds: 30
          periodSeconds: 10
"""

    def _generate_embedding_manifest(self) -> str:
        """Generate Embedding Service manifest (mock for E2E)."""
        return f"""---
apiVersion: v1
kind: Service
metadata:
  name: embedding
  labels:
    app: embedding
spec:
  type: NodePort
  ports:
  - name: http
    port: 8000
    targetPort: 8000
    nodePort: {PORTS['embedding']['nodeport']}
    protocol: TCP
  selector:
    app: embedding
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: embedding
  labels:
    app: embedding
spec:
  replicas: 1
  selector:
    matchLabels:
      app: embedding
  template:
    metadata:
      labels:
        app: embedding
    spec:
      containers:
      - name: embedding
        image: quay.io/jordigilh/embedding-service:latest
        ports:
        - containerPort: 8000
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        readinessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 10
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
"""

    def _generate_data_storage_manifest(self, skip_embedding: bool = False) -> str:
        """Generate Data Storage Service manifest."""
        embedding_url = "http://embedding:8000" if not skip_embedding else ""

        config_yaml = f"""service:
  name: data-storage
  metricsPort: 9090
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: postgresql.{self.NAMESPACE}.svc.cluster.local
  port: 5432
  name: action_history
  user: slm_user
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: redis.{self.NAMESPACE}.svc.cluster.local:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
"""

        return f"""---
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
data:
  config.yaml: |
{self._indent(config_yaml, 4)}
---
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secret
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
  redis-secrets.yaml: |
    password: ""
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  labels:
    app: datastorage
spec:
  type: NodePort
  ports:
  - name: http
    port: 8080
    targetPort: 8080
    nodePort: {PORTS['data_storage']['nodeport']}
    protocol: TCP
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: {PORTS['data_storage']['metrics_nodeport']}
    protocol: TCP
  selector:
    app: datastorage
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  labels:
    app: datastorage
spec:
  replicas: 1
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: localhost/kubernaut-datastorage:e2e-test
        ports:
        - name: http
          containerPort: 8080
        - name: metrics
          containerPort: 9090
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
"""

    def _indent(self, text: str, spaces: int) -> str:
        """Indent text by specified number of spaces."""
        indent = " " * spaces
        return "\n".join(indent + line if line else line for line in text.split("\n"))


