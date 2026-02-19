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
- policy/v1: list PodDisruptionBudgets (ADR-056 label detection)
- autoscaling/v2: list HorizontalPodAutoscalers (ADR-056 label detection)
- networking.k8s.io/v1: list NetworkPolicies (ADR-056 label detection)
- core/v1: get Namespaces (ADR-056 label detection - namespace labels/annotations)
"""

import asyncio
import logging
from typing import Any, Dict, List, Optional, Tuple

from kubernetes import client as k8s_client
from kubernetes import config as k8s_config
from kubernetes.client.rest import ApiException

from src.utils.canonical_hash import canonical_spec_hash

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
    """Lightweight K8s client for owner resolution, spec hash, and label detection queries.

    Designed for in-cluster usage with a ServiceAccount that has limited RBAC.
    All blocking K8s API calls are executed via asyncio.to_thread().

    ADR-056 extensions: list_pdbs, list_hpas, list_network_policies return
    (items, error_str|None) tuples for LabelDetector error tracking.
    """

    def __init__(self):
        """Initialize the K8s client from in-cluster config."""
        self._initialized = False
        self._apps_v1: Optional[k8s_client.AppsV1Api] = None
        self._core_v1: Optional[k8s_client.CoreV1Api] = None
        self._policy_v1: Optional[k8s_client.PolicyV1Api] = None
        self._autoscaling_v2: Optional[k8s_client.AutoscalingV2Api] = None
        self._networking_v1: Optional[k8s_client.NetworkingV1Api] = None

    def _ensure_initialized(self):
        """Lazy initialization of the K8s client."""
        if self._initialized:
            return
        try:
            k8s_config.load_incluster_config()
            self._init_api_clients()
            self._initialized = True
            logger.info("K8s client initialized (in-cluster config)")
        except k8s_config.ConfigException:
            logger.warning(
                "K8s in-cluster config not available; "
                "falling back to kubeconfig (dev mode)"
            )
            try:
                k8s_config.load_kube_config()
                self._init_api_clients()
                self._initialized = True
                logger.info("K8s client initialized (kubeconfig fallback)")
            except Exception as e:
                logger.error("Failed to initialize K8s client: %s", e)
                raise

    def _init_api_clients(self):
        """Create all K8s API client instances."""
        self._apps_v1 = k8s_client.AppsV1Api()
        self._core_v1 = k8s_client.CoreV1Api()
        self._policy_v1 = k8s_client.PolicyV1Api()
        self._autoscaling_v2 = k8s_client.AutoscalingV2Api()
        self._networking_v1 = k8s_client.NetworkingV1Api()

    def _list_pdbs_sync(self, namespace: str) -> Tuple[List[Any], Optional[str]]:
        """Synchronous LIST of PodDisruptionBudgets in a namespace.

        Returns (items, error_string). error_string is None on success.
        """
        self._ensure_initialized()
        try:
            result = self._policy_v1.list_namespaced_pod_disruption_budget(
                namespace=namespace
            )
            return result.items, None
        except ApiException as e:
            logger.warning("PDB list failed in %s: %s", namespace, e)
            return [], str(e)
        except Exception as e:
            logger.error("Unexpected error listing PDBs in %s: %s", namespace, e)
            return [], str(e)

    async def list_pdbs(self, namespace: str) -> Tuple[List[Any], Optional[str]]:
        """Async LIST of PodDisruptionBudgets in a namespace.

        ADR-056: Used by LabelDetector for pdbProtected detection.
        Returns (items, error_string). error_string is None on success.
        """
        return await asyncio.to_thread(self._list_pdbs_sync, namespace)

    def _list_hpas_sync(self, namespace: str) -> Tuple[List[Any], Optional[str]]:
        """Synchronous LIST of HorizontalPodAutoscalers in a namespace.

        Returns (items, error_string). error_string is None on success.
        """
        self._ensure_initialized()
        try:
            result = self._autoscaling_v2.list_namespaced_horizontal_pod_autoscaler(
                namespace=namespace
            )
            return result.items, None
        except ApiException as e:
            logger.warning("HPA list failed in %s: %s", namespace, e)
            return [], str(e)
        except Exception as e:
            logger.error("Unexpected error listing HPAs in %s: %s", namespace, e)
            return [], str(e)

    async def list_hpas(self, namespace: str) -> Tuple[List[Any], Optional[str]]:
        """Async LIST of HorizontalPodAutoscalers in a namespace.

        ADR-056: Used by LabelDetector for hpaEnabled detection.
        Returns (items, error_string). error_string is None on success.
        """
        return await asyncio.to_thread(self._list_hpas_sync, namespace)

    def _list_network_policies_sync(
        self, namespace: str
    ) -> Tuple[List[Any], Optional[str]]:
        """Synchronous LIST of NetworkPolicies in a namespace.

        Returns (items, error_string). error_string is None on success.
        """
        self._ensure_initialized()
        try:
            result = self._networking_v1.list_namespaced_network_policy(
                namespace=namespace
            )
            return result.items, None
        except ApiException as e:
            logger.warning("NetworkPolicy list failed in %s: %s", namespace, e)
            return [], str(e)
        except Exception as e:
            logger.error(
                "Unexpected error listing NetworkPolicies in %s: %s", namespace, e
            )
            return [], str(e)

    async def list_network_policies(
        self, namespace: str
    ) -> Tuple[List[Any], Optional[str]]:
        """Async LIST of NetworkPolicies in a namespace.

        ADR-056: Used by LabelDetector for networkIsolated detection.
        Returns (items, error_string). error_string is None on success.
        """
        return await asyncio.to_thread(self._list_network_policies_sync, namespace)

    def _get_namespace_metadata_sync(
        self, name: str
    ) -> Optional[Dict[str, Any]]:
        """Synchronous GET of namespace labels and annotations.

        ADR-056: Used to build k8s_context for LabelDetector (GitOps and
        service mesh detection inspect namespace-level labels/annotations).
        Returns dict with 'labels' and 'annotations' keys, or None on error.
        """
        self._ensure_initialized()
        try:
            ns = self._core_v1.read_namespace(name=name)
            return {
                "labels": ns.metadata.labels or {},
                "annotations": ns.metadata.annotations or {},
            }
        except ApiException as e:
            logger.warning("Namespace %s not found or inaccessible: %s", name, e)
            return None
        except Exception as e:
            logger.error("Unexpected error fetching namespace %s: %s", name, e)
            return None

    async def get_namespace_metadata(
        self, name: str
    ) -> Optional[Dict[str, Any]]:
        """Async GET of namespace labels and annotations.

        ADR-056: Used by get_resource_context to build k8s_context for
        LabelDetector. Returns dict with 'labels' and 'annotations' keys,
        or None on error.
        """
        return await asyncio.to_thread(self._get_namespace_metadata_sync, name)

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

    def _get_resource_metadata_sync(
        self, kind: str, name: str, namespace: str
    ) -> Optional[Any]:
        """Synchronous GET of a K8s resource (for metadata/ownerReferences).

        Returns the full resource object, or None if not found.
        """
        self._ensure_initialized()

        try:
            if kind == "Deployment":
                return self._apps_v1.read_namespaced_deployment(name, namespace)
            elif kind == "StatefulSet":
                return self._apps_v1.read_namespaced_stateful_set(name, namespace)
            elif kind == "DaemonSet":
                return self._apps_v1.read_namespaced_daemon_set(name, namespace)
            elif kind == "ReplicaSet":
                return self._apps_v1.read_namespaced_replica_set(name, namespace)
            elif kind == "Pod":
                return self._core_v1.read_namespaced_pod(name, namespace)
            elif kind == "Node":
                return self._core_v1.read_node(name)
            else:
                logger.warning("Unsupported kind %s for metadata lookup", kind)
                return None
        except ApiException as e:
            if e.status == 404:
                logger.warning("Resource not found: %s/%s in %s", kind, name, namespace)
            else:
                logger.error("K8s API error: %s/%s in %s: %s", kind, name, namespace, e)
            return None
        except Exception as e:
            logger.error("Unexpected error: %s/%s in %s: %s", kind, name, namespace, e)
            return None

    async def _get_resource_metadata(
        self, kind: str, name: str, namespace: str
    ) -> Optional[Any]:
        """Async-safe GET of a K8s resource (for metadata/ownerReferences)."""
        return await asyncio.to_thread(
            self._get_resource_metadata_sync, kind, name, namespace
        )

    async def resolve_owner_chain(
        self, kind: str, name: str, namespace: str = ""
    ) -> List[Dict[str, str]]:
        """Resolve the ownership chain for a K8s resource via ownerReferences.

        ADR-055: Traverses the K8s API to build the owner chain from the given
        resource up to the root controller.

        Algorithm:
        - GET the resource, extract ownerReferences
        - For each ownerReference, recursively GET until no more owners
        - Return the full chain [resource, parent, grandparent, ...]

        Returns empty list if resource is not found.
        Max depth of 10 to prevent infinite loops.
        """
        chain: List[Dict[str, str]] = []
        current_kind = kind
        current_name = name
        current_ns = namespace

        for _ in range(10):
            obj = await self._get_resource_metadata(current_kind, current_name, current_ns)
            if obj is None:
                break

            ns = getattr(obj.metadata, "namespace", None) or ""
            chain.append({
                "kind": current_kind,
                "name": current_name,
                "namespace": ns,
            })

            owner_refs = getattr(obj.metadata, "owner_references", None)
            if not owner_refs:
                break

            owner = owner_refs[0]
            current_kind = owner.kind
            current_name = owner.name

        return chain

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
