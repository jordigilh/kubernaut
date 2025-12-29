# Implementation Plan: E2E Testing Infrastructure for HolmesGPT-API

**Version**: 1.2
**Status**: ✅ COMPLETE
**Target Version**: V1.0
**Date**: December 6, 2025
**Effort Estimate**: 1 day

> ⚠️ **V1.0 BLOCKER**: Without this infrastructure, E2E tests cannot run in CI/CD.
> E2E tests require real Data Storage (per TESTING_GUIDELINES.md section 4).

---

## 📊 Port Allocation (per DD-TEST-001 v1.2)

| Component | Host Port | NodePort | Purpose |
|-----------|-----------|----------|---------|
| **HolmesGPT API** | 8088 | 30088 | HAPI service |
| **HolmesGPT Metrics** | 9188 | 30188 | Prometheus metrics |
| **Data Storage** | 8089 | 30089 | Audit trail, workflow catalog API |
| **PostgreSQL + pgvector** | 5488 | 30488 | Workflow catalog storage |
| **Embedding Service** | 8188 | 30288 | Vector embeddings for semantic search |
| **Redis** | 6388 | 30388 | Data Storage DLQ |

**Kind Config Location**: `holmesgpt-api/tests/infrastructure/kind-holmesgpt-config.yaml`

---

## 📋 Overview

### Problem Statement

HAPI's E2E tests currently rely on:
- Mock servers (default)
- Manual cluster setup + environment variables
- Port-forwarding for real service access

Go services use programmatic Kind cluster creation via `test/infrastructure/` package, enabling:
- Reproducible test environments
- CI/CD automation without manual setup
- Isolated test clusters per service

### Goal

Consolidate HAPI's E2E testing infrastructure to use programmatic Kind cluster creation, matching the approach used by Go services.

---

## 🏗️ Architecture

### Target Structure

```
holmesgpt-api/
├── tests/
│   ├── infrastructure/           # NEW: E2E infrastructure utilities
│   │   ├── __init__.py
│   │   ├── kind_cluster.py       # Kind cluster management (Podman)
│   │   ├── data_storage.py       # Data Storage + dependencies deployment
│   │   ├── kind-holmesgpt-config.yaml  # Kind cluster config
│   │   └── manifests/
│   │       ├── namespace.yaml
│   │       ├── postgresql-deployment.yaml   # PostgreSQL + pgvector
│   │       ├── redis-deployment.yaml        # Redis for DLQ
│   │       ├── embedding-service-deployment.yaml  # Vector embeddings
│   │       └── data-storage-deployment.yaml
│   │
│   ├── e2e/
│   │   ├── conftest.py           # UPDATED: Use programmatic cluster
│   │   └── test_audit_pipeline_e2e.py
│   │
│   └── ...
```

### Dependency Stack

```
┌─────────────────────────────────────────────────────────────────┐
│                      HAPI E2E Tests                             │
│                   (Mock LLM only - cost)                        │
├─────────────────────────────────────────────────────────────────┤
│                    HolmesGPT API Service                        │
│                      (8088 / 30088)                             │
├─────────────────────────────────────────────────────────────────┤
│                    Data Storage Service                         │
│                      (8089 / 30089)                             │
│               Workflow Catalog REST API                         │
│                    Audit Trail API                              │
├──────────────────┬──────────────────┬──────────────────────────┤
│  PostgreSQL      │  Embedding Svc   │  Redis                   │
│  + pgvector      │  (8188/30288)    │  (6388/30388)            │
│  (5488/30488)    │                  │  (DLQ)                   │
└──────────────────┴──────────────────┴──────────────────────────┘
```

### Test Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ pytest tests/e2e/ -v                                            │
├─────────────────────────────────────────────────────────────────┤
│ 1. BeforeSuite (session-scoped fixture)                         │
│    ├─ Create Kind cluster "holmesgpt-e2e"                       │
│    ├─ Deploy Data Storage service                               │
│    ├─ Wait for Data Storage health check                        │
│    └─ Set DATA_STORAGE_URL environment variable                 │
├─────────────────────────────────────────────────────────────────┤
│ 2. Run E2E Tests                                                │
│    ├─ test_llm_request_event_persisted                          │
│    ├─ test_llm_response_event_persisted                         │
│    ├─ test_validation_attempt_event_persisted                   │
│    ├─ test_complete_audit_trail_persisted                       │
│    └─ test_validation_retry_events_persisted                    │
├─────────────────────────────────────────────────────────────────┤
│ 3. AfterSuite (cleanup)                                         │
│    └─ Delete Kind cluster (optional, based on --keep-cluster)   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 📝 Implementation Phases

### Phase 1: Kind Cluster Management (2 hours)

**File**: `tests/infrastructure/kind_cluster.py`

```python
"""
Kind cluster management for E2E tests.

Provides programmatic creation/deletion of Kind clusters,
matching the approach used by Go services in test/infrastructure/.
"""

import os
import subprocess
import time
from pathlib import Path
from typing import Optional


class KindCluster:
    """
    Manages Kind cluster lifecycle for E2E testing.

    Usage:
        cluster = KindCluster("holmesgpt-e2e")
        cluster.create()
        try:
            # Run tests
            pass
        finally:
            cluster.delete()
    """

    def __init__(
        self,
        name: str,
        config_path: Optional[str] = None,
        kubeconfig_dir: Optional[str] = None
    ):
        self.name = name
        self.config_path = config_path or self._default_config_path()
        self.kubeconfig_dir = kubeconfig_dir or os.path.expanduser("~/.kube")
        self.kubeconfig_path = f"{self.kubeconfig_dir}/kind-{name}"

    def _default_config_path(self) -> str:
        """Get default Kind config path relative to this file."""
        return str(Path(__file__).parent / "kind-holmesgpt-config.yaml")

    def exists(self) -> bool:
        """Check if cluster already exists."""
        result = subprocess.run(
            ["kind", "get", "clusters"],
            capture_output=True,
            text=True
        )
        return self.name in result.stdout.split()

    def create(self, timeout_seconds: int = 300) -> None:
        """
        Create Kind cluster.

        Args:
            timeout_seconds: Timeout for cluster creation

        Raises:
            RuntimeError: If cluster creation fails
        """
        if self.exists():
            print(f"Kind cluster '{self.name}' already exists, reusing...")
            return

        print(f"Creating Kind cluster '{self.name}'...")
        print(f"  Config: {self.config_path}")
        print(f"  Kubeconfig: {self.kubeconfig_path}")

        cmd = [
            "kind", "create", "cluster",
            "--name", self.name,
            "--kubeconfig", self.kubeconfig_path,
        ]

        if os.path.exists(self.config_path):
            cmd.extend(["--config", self.config_path])

        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout_seconds
        )

        if result.returncode != 0:
            raise RuntimeError(f"Failed to create Kind cluster: {result.stderr}")

        # Set KUBECONFIG for subsequent kubectl commands
        os.environ["KUBECONFIG"] = self.kubeconfig_path

        print(f"Kind cluster '{self.name}' created successfully")

    def delete(self) -> None:
        """Delete Kind cluster."""
        if not self.exists():
            print(f"Kind cluster '{self.name}' does not exist, nothing to delete")
            return

        print(f"Deleting Kind cluster '{self.name}'...")

        result = subprocess.run(
            ["kind", "delete", "cluster", "--name", self.name],
            capture_output=True,
            text=True
        )

        if result.returncode != 0:
            print(f"Warning: Failed to delete Kind cluster: {result.stderr}")
        else:
            print(f"Kind cluster '{self.name}' deleted")

        # Clean up kubeconfig
        if os.path.exists(self.kubeconfig_path):
            os.remove(self.kubeconfig_path)

    def load_image(self, image: str) -> None:
        """
        Load Docker image into Kind cluster.

        Args:
            image: Docker image name (e.g., "ghcr.io/kubernaut/data-storage:latest")
        """
        print(f"Loading image '{image}' into Kind cluster '{self.name}'...")

        result = subprocess.run(
            ["kind", "load", "docker-image", image, "--name", self.name],
            capture_output=True,
            text=True
        )

        if result.returncode != 0:
            raise RuntimeError(f"Failed to load image: {result.stderr}")

    def kubectl(self, *args, check: bool = True) -> subprocess.CompletedProcess:
        """
        Run kubectl command against the cluster.

        Args:
            *args: kubectl arguments
            check: Raise exception on non-zero exit code

        Returns:
            CompletedProcess with stdout/stderr
        """
        cmd = ["kubectl", "--kubeconfig", self.kubeconfig_path, *args]
        return subprocess.run(cmd, capture_output=True, text=True, check=check)

    def wait_for_ready(self, timeout_seconds: int = 120) -> None:
        """Wait for cluster to be ready (all nodes Ready)."""
        print("Waiting for cluster nodes to be ready...")

        deadline = time.time() + timeout_seconds
        while time.time() < deadline:
            result = self.kubectl(
                "get", "nodes",
                "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}",
                check=False
            )

            if result.returncode == 0 and "True" in result.stdout:
                print("Cluster nodes are ready")
                return

            time.sleep(5)

        raise TimeoutError(f"Cluster not ready after {timeout_seconds} seconds")
```

### Phase 2: Kind Cluster Config (30 min)

**File**: `tests/infrastructure/kind-holmesgpt-config.yaml`

```yaml
# Kind cluster configuration for HolmesGPT-API E2E tests
# Mirrors configuration used by Go services in test/infrastructure/

kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

# Single node for simplicity (HAPI doesn't need multi-node)
nodes:
  - role: control-plane
    extraPortMappings:
      # Data Storage service (NodePort)
      - containerPort: 30080
        hostPort: 8080
        protocol: TCP
      # PostgreSQL (for debugging)
      - containerPort: 30432
        hostPort: 5432
        protocol: TCP

# Faster startup
networking:
  disableDefaultCNI: false

# Resource limits for CI environments
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://registry-1.docker.io"]
```

### Phase 3: Data Storage Deployment (1.5 hours)

**File**: `tests/infrastructure/data_storage.py`

```python
"""
Data Storage deployment utilities for E2E tests.
"""

import time
from pathlib import Path
from typing import Optional

from .kind_cluster import KindCluster


class DataStorageDeployment:
    """
    Deploys Data Storage service to Kind cluster for E2E testing.
    """

    NAMESPACE = "kubernaut-e2e"
    SERVICE_NAME = "data-storage"
    HEALTH_ENDPOINT = "/health"

    def __init__(self, cluster: KindCluster):
        self.cluster = cluster
        self.manifests_dir = Path(__file__).parent / "manifests"

    def deploy(self, image: str = "ghcr.io/kubernaut/data-storage:latest") -> str:
        """
        Deploy Data Storage to the cluster.

        Args:
            image: Data Storage Docker image

        Returns:
            Data Storage URL (e.g., "http://localhost:8080")
        """
        print(f"Deploying Data Storage to namespace '{self.NAMESPACE}'...")

        # Create namespace
        self.cluster.kubectl(
            "create", "namespace", self.NAMESPACE,
            check=False  # Ignore if already exists
        )

        # Load image into Kind (if local)
        if not image.startswith("ghcr.io"):
            self.cluster.load_image(image)

        # Apply manifests
        self._apply_manifests(image)

        # Wait for deployment to be ready
        self._wait_for_ready()

        # Return URL (NodePort exposed on localhost:8080)
        return "http://localhost:8080"

    def _apply_manifests(self, image: str) -> None:
        """Apply Kubernetes manifests."""
        # PostgreSQL (in-cluster for testing)
        self.cluster.kubectl(
            "apply", "-f", str(self.manifests_dir / "postgres-deployment.yaml"),
            "-n", self.NAMESPACE
        )

        # Data Storage
        # Use kustomize or envsubst to inject image
        manifest_content = (self.manifests_dir / "data-storage-deployment.yaml").read_text()
        manifest_content = manifest_content.replace("${IMAGE}", image)

        # Apply via stdin
        import subprocess
        subprocess.run(
            ["kubectl", "--kubeconfig", self.cluster.kubeconfig_path,
             "apply", "-f", "-", "-n", self.NAMESPACE],
            input=manifest_content,
            text=True,
            check=True
        )

    def _wait_for_ready(self, timeout_seconds: int = 180) -> None:
        """Wait for Data Storage to be ready."""
        print("Waiting for Data Storage to be ready...")

        deadline = time.time() + timeout_seconds
        while time.time() < deadline:
            result = self.cluster.kubectl(
                "get", "deployment", self.SERVICE_NAME,
                "-n", self.NAMESPACE,
                "-o", "jsonpath={.status.readyReplicas}",
                check=False
            )

            if result.returncode == 0 and result.stdout.strip() == "1":
                print("Data Storage is ready")
                return

            time.sleep(5)

        raise TimeoutError(f"Data Storage not ready after {timeout_seconds} seconds")

    def teardown(self) -> None:
        """Remove Data Storage deployment."""
        print(f"Removing Data Storage from namespace '{self.NAMESPACE}'...")
        self.cluster.kubectl(
            "delete", "namespace", self.NAMESPACE,
            "--ignore-not-found",
            check=False
        )
```

### Phase 4: E2E Conftest Update (1 hour)

**File**: `tests/e2e/conftest.py` (updated)

```python
"""
E2E Test Configuration and Fixtures

Provides session-scoped fixtures for E2E testing with:
- Programmatic Kind cluster creation
- Real Data Storage service
- Mock LLM server (cost constraint)
"""

import os
import sys
import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))

from tests.infrastructure.kind_cluster import KindCluster
from tests.infrastructure.data_storage import DataStorageDeployment


def pytest_addoption(parser):
    """Add custom pytest options."""
    parser.addoption(
        "--keep-cluster",
        action="store_true",
        default=False,
        help="Keep Kind cluster after tests (for debugging)"
    )
    parser.addoption(
        "--reuse-cluster",
        action="store_true",
        default=False,
        help="Reuse existing Kind cluster if available"
    )


def pytest_configure(config):
    """Register custom markers."""
    config.addinivalue_line("markers", "e2e: mark test as E2E test")
    config.addinivalue_line("markers", "requires_data_storage: requires real Data Storage")


# =============================================================================
# SESSION-SCOPED FIXTURES (Created once per test session)
# =============================================================================

@pytest.fixture(scope="session")
def kind_cluster(request):
    """
    Session-scoped Kind cluster for E2E tests.

    Creates cluster at start, deletes at end (unless --keep-cluster).
    """
    cluster = KindCluster("holmesgpt-e2e")

    reuse = request.config.getoption("--reuse-cluster")
    keep = request.config.getoption("--keep-cluster")

    if reuse and cluster.exists():
        print(f"Reusing existing Kind cluster '{cluster.name}'")
    else:
        cluster.create()
        cluster.wait_for_ready()

    yield cluster

    if not keep:
        cluster.delete()
    else:
        print(f"Keeping Kind cluster '{cluster.name}' (--keep-cluster)")


@pytest.fixture(scope="session")
def data_storage_url(kind_cluster):
    """
    Session-scoped Data Storage URL.

    Deploys Data Storage to Kind cluster and returns URL.
    """
    deployment = DataStorageDeployment(kind_cluster)
    url = deployment.deploy()

    # Set environment variable for tests
    os.environ["DATA_STORAGE_URL"] = url

    yield url

    # Cleanup handled by cluster deletion


@pytest.fixture(scope="session", autouse=True)
def setup_e2e_environment(data_storage_url):
    """Set up environment variables for E2E testing."""
    os.environ["LLM_PROVIDER"] = "openai"
    os.environ["LLM_MODEL"] = "mock-model"
    os.environ["OPENAI_API_KEY"] = "mock-key-for-e2e"
    os.environ["DATA_STORAGE_TIMEOUT"] = "30"

    print(f"\n{'='*60}")
    print("E2E Test Environment")
    print(f"{'='*60}")
    print(f"  DATA_STORAGE_URL: {data_storage_url}")
    print(f"  LLM: Mocked (cost constraint)")
    print(f"{'='*60}\n")

    yield


@pytest.fixture(scope="session")
def mock_llm_server_e2e():
    """
    Session-scoped mock LLM server.

    LLM is the ONLY component we mock (due to cost).
    Per TESTING_GUIDELINES.md section 4.
    """
    from tests.mock_llm_server import MockLLMServer

    with MockLLMServer(force_text_response=False) as server:
        os.environ["LLM_ENDPOINT"] = server.url
        yield server
```

### Phase 5: Kubernetes Manifests (30 min)

**File**: `tests/infrastructure/manifests/data-storage-deployment.yaml`

```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: data-storage
spec:
  type: NodePort
  ports:
    - port: 8080
      targetPort: 8080
      nodePort: 30080
  selector:
    app: data-storage
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: data-storage
spec:
  replicas: 1
  selector:
    matchLabels:
      app: data-storage
  template:
    metadata:
      labels:
        app: data-storage
    spec:
      containers:
        - name: data-storage
          image: ${IMAGE}
          ports:
            - containerPort: 8080
          env:
            - name: DATABASE_URL
              value: "postgresql://postgres:postgres@postgres:5432/kubernaut"
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
            initialDelaySeconds: 10
            periodSeconds: 10
```

---

## ✅ Acceptance Criteria

### AC-1: Programmatic Cluster Creation

```gherkin
Given no Kind cluster exists
When pytest tests/e2e/ is executed
Then a Kind cluster "holmesgpt-e2e" SHALL be created automatically
And Data Storage SHALL be deployed to the cluster
And E2E tests SHALL run against real Data Storage
```

### AC-2: Cluster Reuse

```gherkin
Given pytest tests/e2e/ --reuse-cluster is executed
And Kind cluster "holmesgpt-e2e" already exists
Then the existing cluster SHALL be reused
And no new cluster SHALL be created
```

### AC-3: Cleanup

```gherkin
Given E2E tests complete
When --keep-cluster is NOT specified
Then the Kind cluster SHALL be deleted
And kubeconfig file SHALL be removed
```

### AC-4: CI/CD Compatibility

```gherkin
Given the test runs in CI/CD environment
When pytest tests/e2e/ is executed
Then tests SHALL pass without manual intervention
And no pre-existing cluster required
```

---

## 🔗 Related Documents

- [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md) - LLM mocking policy (section 4)
- [Go test/infrastructure/](../../../../../test/infrastructure/) - Reference implementation for Go services
- [test_audit_pipeline_e2e.py](../../../holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py) - E2E tests requiring this infrastructure

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial implementation plan |

