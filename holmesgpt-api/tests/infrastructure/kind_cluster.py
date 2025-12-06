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
Kind Cluster Management for E2E Tests

Provides programmatic Kind cluster creation/deletion using Podman,
matching the approach used by Go services in test/infrastructure/.

Per TESTING_GUIDELINES.md section 4:
- E2E tests must use all real services EXCEPT the LLM
- LLM is mocked due to cost
- If Data Storage is unavailable, E2E tests should FAIL, not skip

Per DD-TEST-001 v1.2:
- HolmesGPT API: Host Port 8088, NodePort 30088, Metrics 9188/30188
- Uses Podman (not Docker) per ADR-016
"""

import os
import subprocess
import time
from pathlib import Path
from typing import Optional


class KindCluster:
    """
    Manages Kind cluster lifecycle for E2E testing.

    Uses Podman as container runtime (per ADR-016).

    Usage:
        cluster = KindCluster("holmesgpt-e2e")
        cluster.create()
        try:
            # Run tests
            pass
        finally:
            cluster.delete()
    """

    CLUSTER_NAME = "holmesgpt-e2e"

    def __init__(
        self,
        name: str = CLUSTER_NAME,
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
        Create Kind cluster using Podman.

        Args:
            timeout_seconds: Timeout for cluster creation

        Raises:
            RuntimeError: If cluster creation fails
        """
        if self.exists():
            print(f"⚠️  Kind cluster '{self.name}' already exists, reusing...")
            self._export_kubeconfig()
            return

        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
        print(f"HolmesGPT API E2E Cluster Setup")
        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
        print(f"📦 Creating Kind cluster '{self.name}'...")
        print(f"   Config: {self.config_path}")
        print(f"   Kubeconfig: {self.kubeconfig_path}")

        if not os.path.exists(self.config_path):
            raise RuntimeError(f"Kind config file not found: {self.config_path}")

        cmd = [
            "kind", "create", "cluster",
            "--name", self.name,
            "--config", self.config_path,
        ]

        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout_seconds
        )

        if result.returncode != 0:
            print(f"❌ Kind create failed:\n{result.stderr}")
            raise RuntimeError(f"Failed to create Kind cluster: {result.stderr}")

        # Export kubeconfig (kind create --kubeconfig doesn't always work reliably)
        self._export_kubeconfig()

        print(f"✅ Kind cluster '{self.name}' created successfully")

    def _export_kubeconfig(self) -> None:
        """Export kubeconfig to file."""
        # Use Output() to avoid capturing stderr (contains "enabling experimental podman provider")
        result = subprocess.run(
            ["kind", "get", "kubeconfig", "--name", self.name],
            capture_output=True,
            text=True
        )

        if result.returncode != 0:
            raise RuntimeError(f"Failed to get kubeconfig: {result.stderr}")

        # Ensure directory exists
        os.makedirs(self.kubeconfig_dir, exist_ok=True)

        # Write kubeconfig (stdout only, not stderr)
        with open(self.kubeconfig_path, 'w') as f:
            f.write(result.stdout)
        os.chmod(self.kubeconfig_path, 0o600)

        # Set KUBECONFIG for subprocess commands
        os.environ["KUBECONFIG"] = self.kubeconfig_path

        print(f"   ✅ Kubeconfig written to {self.kubeconfig_path}")

    def delete(self) -> None:
        """Delete Kind cluster."""
        if not self.exists():
            print(f"⚠️  Kind cluster '{self.name}' does not exist, nothing to delete")
            return

        print(f"🧹 Deleting Kind cluster '{self.name}'...")

        result = subprocess.run(
            ["kind", "delete", "cluster", "--name", self.name],
            capture_output=True,
            text=True
        )

        if result.returncode != 0:
            print(f"⚠️  Warning: Failed to delete Kind cluster: {result.stderr}")
        else:
            print(f"✅ Kind cluster '{self.name}' deleted")

        # Clean up kubeconfig
        if os.path.exists(self.kubeconfig_path):
            os.remove(self.kubeconfig_path)

    def load_image(self, image: str) -> None:
        """
        Load Podman image into Kind cluster.

        Per ADR-016: Uses podman save + kind load image-archive pattern.

        Args:
            image: Image name (e.g., "localhost/kubernaut-datastorage:e2e-test")
        """
        print(f"📦 Loading image '{image}' into Kind cluster '{self.name}'...")

        tar_path = f"/tmp/{self.name}-image.tar"

        # Save image to tar using Podman
        save_cmd = subprocess.run(
            ["podman", "save", image, "-o", tar_path],
            capture_output=True,
            text=True
        )

        if save_cmd.returncode != 0:
            raise RuntimeError(f"Failed to save image: {save_cmd.stderr}")

        # Load image into Kind
        load_cmd = subprocess.run(
            ["kind", "load", "image-archive", tar_path, "--name", self.name],
            capture_output=True,
            text=True
        )

        if load_cmd.returncode != 0:
            raise RuntimeError(f"Failed to load image into Kind: {load_cmd.stderr}")

        # Clean up tar file
        if os.path.exists(tar_path):
            os.remove(tar_path)

        print(f"   ✅ Image '{image}' loaded into Kind cluster")

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
        print("⏳ Waiting for cluster nodes to be ready...")

        deadline = time.time() + timeout_seconds
        while time.time() < deadline:
            result = self.kubectl(
                "get", "nodes",
                "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}",
                check=False
            )

            if result.returncode == 0 and "True" in result.stdout:
                print("   ✅ Cluster nodes are ready")
                return

            time.sleep(5)

        raise TimeoutError(f"Cluster not ready after {timeout_seconds} seconds")


def find_workspace_root() -> str:
    """
    Find workspace root by looking for go.mod (matches Go services pattern).

    Returns:
        Path to workspace root

    Raises:
        RuntimeError: If workspace root not found
    """
    current = Path.cwd()

    while current != current.parent:
        if (current / "go.mod").exists():
            return str(current)
        current = current.parent

    raise RuntimeError("Could not find workspace root (go.mod)")


def build_image(
    image_name: str,
    dockerfile: str,
    context_dir: str,
    build_args: Optional[dict] = None
) -> None:
    """
    Build image using Podman (per ADR-016).

    Follows the pattern from test/infrastructure/datastorage.go:
    - Working directory set to context_dir (workspace root)
    - Dockerfile path is relative to context_dir
    - Context is "." (current directory after cwd change)

    Args:
        image_name: Target image name
        dockerfile: Path to Dockerfile (relative to context_dir)
        context_dir: Build context directory (workspace root)
        build_args: Optional build arguments
    """
    print(f"🔨 Building image '{image_name}'...")

    # Verify dockerfile exists relative to context_dir
    dockerfile_path = os.path.join(context_dir, dockerfile)
    if not os.path.exists(dockerfile_path):
        raise RuntimeError(f"Dockerfile not found: {dockerfile_path}")

    print(f"   Dockerfile: {dockerfile}")
    print(f"   Context: {context_dir}")

    # Match Go pattern: run from context_dir with relative dockerfile path
    cmd = ["podman", "build", "-t", image_name, "-f", dockerfile, "."]

    if build_args:
        for key, value in build_args.items():
            cmd.extend(["--build-arg", f"{key}={value}"])

    # Run from context_dir (workspace root) - matching Go's buildCmd.Dir = workspaceRoot
    result = subprocess.run(cmd, capture_output=True, text=True, cwd=context_dir)

    if result.returncode != 0:
        print(f"❌ Build failed:\n{result.stderr}")
        raise RuntimeError(f"Podman build failed: {result.stderr}")

    print(f"   ✅ Image '{image_name}' built successfully")

