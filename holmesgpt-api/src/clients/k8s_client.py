# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Lightweight Kubernetes client for HAPI spec hash computation and resource access.

ADR-055: Pre-computation of root owner from owner chain removed. Context
enrichment (owner chain resolution, spec hash, remediation history) is now
performed post-RCA by the LLM via the get_resource_context tool.

Design Decisions:
- DD-EM-002: Canonical spec hash cross-language compatibility
- Uses in-cluster ServiceAccount (requires RBAC for apps/v1 GET)
- Async-safe via asyncio.to_thread() for blocking K8s API calls

RBAC Requirements (ServiceAccount):
- apps/v1: get on Deployments, StatefulSets, DaemonSets, ReplicaSets
- core/v1: get on Pods, Nodes (for bare resource fallback)
"""

import asyncio
import logging
from typing import Any, Dict, Optional

from kubernetes import client as k8s_client
from kubernetes import config as k8s_config
from kubernetes.client.rest import ApiException

from utils.canonical_hash import canonical_spec_hash

logger = logging.getLogger(__name__)

# API group mapping for K8s resource kinds
_API_GROUP_MAP = {
    "Deployment": ("apps", "v1"),
    "StatefulSet": ("apps", "v1"),
    "DaemonSet": ("apps", "v1"),
    "ReplicaSet": ("apps", "v1"),
    "Pod": ("", "v1"),
    "Node": ("", "v1"),
    "Service": ("", "v1"),
}


class K8sResourceClient:
    """Lightweight K8s client for owner resolution and spec hash computation.

    Designed for in-cluster usage with a ServiceAccount that has limited RBAC.
    All blocking K8s API calls are executed via asyncio.to_thread().
    """

    def __init__(self):
        """Initialize the K8s client from in-cluster config."""
        self._initialized = False
        self._apps_v1: Optional[k8s_client.AppsV1Api] = None
        self._core_v1: Optional[k8s_client.CoreV1Api] = None

    def _ensure_initialized(self):
        """Lazy initialization of the K8s client."""
        if self._initialized:
            return
        try:
            k8s_config.load_incluster_config()
            self._apps_v1 = k8s_client.AppsV1Api()
            self._core_v1 = k8s_client.CoreV1Api()
            self._initialized = True
            logger.info("K8s client initialized (in-cluster config)")
        except k8s_config.ConfigException:
            logger.warning(
                "K8s in-cluster config not available; "
                "falling back to kubeconfig (dev mode)"
            )
            try:
                k8s_config.load_kube_config()
                self._apps_v1 = k8s_client.AppsV1Api()
                self._core_v1 = k8s_client.CoreV1Api()
                self._initialized = True
                logger.info("K8s client initialized (kubeconfig fallback)")
            except Exception as e:
                logger.error("Failed to initialize K8s client: %s", e)
                raise

    def _get_resource_spec_sync(
        self, kind: str, name: str, namespace: str
    ) -> Optional[Dict[str, Any]]:
        """Synchronous GET of a K8s resource's .spec (runs in thread pool).

        Returns the .spec dict, or None if the resource is not found or
        the kind is not supported.
        """
        self._ensure_initialized()

        try:
            if kind == "Deployment":
                obj = self._apps_v1.read_namespaced_deployment(name, namespace)
            elif kind == "StatefulSet":
                obj = self._apps_v1.read_namespaced_stateful_set(name, namespace)
            elif kind == "DaemonSet":
                obj = self._apps_v1.read_namespaced_daemon_set(name, namespace)
            elif kind == "ReplicaSet":
                obj = self._apps_v1.read_namespaced_replica_set(name, namespace)
            elif kind == "Pod":
                obj = self._apps_v1 and self._core_v1.read_namespaced_pod(
                    name, namespace
                )
            elif kind == "Node":
                obj = self._core_v1.read_node(name)
            else:
                logger.warning(
                    "Unsupported kind %s for spec hash computation", kind
                )
                return None

            # Convert the .spec to a dict
            if hasattr(obj, "spec") and obj.spec is not None:
                return k8s_client.ApiClient().sanitize_for_serialization(
                    obj.spec
                )
            return None

        except ApiException as e:
            if e.status == 404:
                logger.warning(
                    "Resource not found: %s/%s in %s", kind, name, namespace
                )
            else:
                logger.error(
                    "K8s API error getting %s/%s in %s: %s",
                    kind,
                    name,
                    namespace,
                    e,
                )
            return None
        except Exception as e:
            logger.error(
                "Unexpected error getting %s/%s in %s: %s",
                kind,
                name,
                namespace,
                e,
            )
            return None

    async def get_resource_spec(
        self, kind: str, name: str, namespace: str = ""
    ) -> Optional[Dict[str, Any]]:
        """Async-safe GET of a K8s resource's .spec.

        Executes the blocking K8s API call in a thread pool.
        """
        return await asyncio.to_thread(
            self._get_resource_spec_sync, kind, name, namespace
        )

    async def compute_spec_hash(
        self, kind: str, name: str, namespace: str = ""
    ) -> str:
        """Compute the canonical spec hash for a K8s resource.

        Returns "sha256:<hex>" on success, or "" on failure.
        """
        spec = await self.get_resource_spec(kind, name, namespace)
        if spec is None:
            return ""
        return canonical_spec_hash(spec)


# Module-level singleton (lazy-initialized)
_k8s_client: Optional[K8sResourceClient] = None


def get_k8s_client() -> K8sResourceClient:
    """Get the module-level K8s client singleton."""
    global _k8s_client
    if _k8s_client is None:
        _k8s_client = K8sResourceClient()
    return _k8s_client
