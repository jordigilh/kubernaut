"""
Kubernetes Context Provider for HolmesGPT.

This module migrates functionality from the deprecated MCP Bridge by providing
rich Kubernetes context to HolmesGPT investigations. It replaces the complex
multi-turn conversation approach with direct context injection.
"""

import asyncio
import logging
import time
from datetime import datetime, timedelta, timezone
from typing import Dict, List, Optional, Any

import psutil
from kubernetes import client as k8s_client
from kubernetes.client.rest import ApiException

from app.config import Settings
from app.models.requests import AlertData, ContextData


class KubernetesContextProvider:
    """Provides Kubernetes cluster context for HolmesGPT investigations."""

    def __init__(self, settings: Settings):
        self.settings = settings
        self.logger = logging.getLogger(__name__)
        self._k8s_client = None
        self._last_context_refresh = None
        self._context_cache = {}

    async def get_investigation_context(self, alert: AlertData) -> Dict[str, Any]:
        """
        Get comprehensive Kubernetes context for HolmesGPT investigation.

        This replaces the MCP Bridge tools with direct context provision:
        - get_pod_status -> pod_status
        - check_node_capacity -> cluster_capacity
        - get_recent_events -> recent_events
        - check_resource_quotas -> resource_quotas
        """
        context = {}

        try:
            namespace = alert.labels.get("namespace") or alert.labels.get("pod_namespace", "default")

            # Gather all context in parallel (replaces sequential MCP tool calls)
            tasks = [
                self._get_pod_status_context(namespace, alert),
                self._get_cluster_capacity_context(),
                self._get_recent_events_context(namespace),
                self._get_resource_quotas_context(namespace),
                self._get_workload_context(namespace, alert),
            ]

            results = await asyncio.gather(*tasks, return_exceptions=True)

            # Merge results
            for result in results:
                if isinstance(result, dict):
                    context.update(result)
                elif isinstance(result, Exception):
                    self.logger.warning(f"Context gathering failed: {result}")

            # Add metadata
            context["context_timestamp"] = datetime.now(timezone.utc).isoformat()
            context["context_source"] = "kubernetes_context_provider"

            return context

        except Exception as e:
            self.logger.error(f"Failed to gather Kubernetes context: {e}")
            return {"error": f"Context gathering failed: {str(e)}"}

    async def _get_pod_status_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Replaces MCP get_pod_status tool."""
        try:
            if not self._k8s_client:
                await self._init_k8s_client()

            v1 = k8s_client.CoreV1Api(self._k8s_client)

            # Get pods in namespace
            pods = v1.list_namespaced_pod(namespace=namespace)

            pod_info = []
            for pod in pods.items:
                pod_data = {
                    "name": pod.metadata.name,
                    "phase": pod.status.phase,
                    "ready": self._is_pod_ready(pod),
                    "restart_count": sum(container.restart_count or 0 for container in (pod.status.container_statuses or [])),
                    "created": pod.metadata.creation_timestamp.isoformat() if pod.metadata.creation_timestamp else None,
                    "node": pod.spec.node_name,
                    "resources": self._extract_pod_resources(pod),
                    "conditions": [{"type": cond.type, "status": cond.status, "reason": cond.reason}
                                 for cond in (pod.status.conditions or [])]
                }

                # Add container statuses
                if pod.status.container_statuses:
                    pod_data["containers"] = []
                    for container in pod.status.container_statuses:
                        container_data = {
                            "name": container.name,
                            "ready": container.ready,
                            "restart_count": container.restart_count or 0,
                            "state": self._get_container_state(container)
                        }
                        pod_data["containers"].append(container_data)

                pod_info.append(pod_data)

            return {
                "pod_status": {
                    "namespace": namespace,
                    "total_pods": len(pod_info),
                    "pods": pod_info,
                    "summary": self._summarize_pod_health(pod_info)
                }
            }

        except ApiException as e:
            self.logger.warning(f"Failed to get pod status: {e}")
            return {"pod_status": {"error": str(e)}}

    async def _get_cluster_capacity_context(self) -> Dict[str, Any]:
        """Replaces MCP check_node_capacity tool."""
        try:
            if not self._k8s_client:
                await self._init_k8s_client()

            v1 = k8s_client.CoreV1Api(self._k8s_client)
            nodes = v1.list_node()

            cluster_capacity = {
                "total_nodes": len(nodes.items),
                "ready_nodes": 0,
                "total_cpu": 0,
                "total_memory": 0,
                "allocatable_cpu": 0,
                "allocatable_memory": 0,
                "nodes": []
            }

            for node in nodes.items:
                node_ready = any(cond.type == "Ready" and cond.status == "True"
                               for cond in (node.status.conditions or []))

                if node_ready:
                    cluster_capacity["ready_nodes"] += 1

                # Parse resource quantities
                capacity = node.status.capacity or {}
                allocatable = node.status.allocatable or {}

                cpu_capacity = self._parse_cpu_quantity(capacity.get("cpu", "0"))
                memory_capacity = self._parse_memory_quantity(capacity.get("memory", "0"))
                cpu_allocatable = self._parse_cpu_quantity(allocatable.get("cpu", "0"))
                memory_allocatable = self._parse_memory_quantity(allocatable.get("memory", "0"))

                cluster_capacity["total_cpu"] += cpu_capacity
                cluster_capacity["total_memory"] += memory_capacity
                cluster_capacity["allocatable_cpu"] += cpu_allocatable
                cluster_capacity["allocatable_memory"] += memory_allocatable

                node_data = {
                    "name": node.metadata.name,
                    "ready": node_ready,
                    "cpu_capacity": cpu_capacity,
                    "memory_capacity": memory_capacity,
                    "cpu_allocatable": cpu_allocatable,
                    "memory_allocatable": memory_allocatable,
                    "node_info": {
                        "os": node.status.node_info.operating_system if node.status.node_info else "unknown",
                        "kernel": node.status.node_info.kernel_version if node.status.node_info else "unknown",
                        "container_runtime": node.status.node_info.container_runtime_version if node.status.node_info else "unknown"
                    }
                }

                cluster_capacity["nodes"].append(node_data)

            return {"cluster_capacity": cluster_capacity}

        except ApiException as e:
            self.logger.warning(f"Failed to get cluster capacity: {e}")
            return {"cluster_capacity": {"error": str(e)}}

    async def _get_recent_events_context(self, namespace: str) -> Dict[str, Any]:
        """Replaces MCP get_recent_events tool."""
        try:
            if not self._k8s_client:
                await self._init_k8s_client()

            v1 = k8s_client.CoreV1Api(self._k8s_client)

            # Get events from last 1 hour
            events = v1.list_namespaced_event(namespace=namespace)

            recent_events = []
            cutoff_time = datetime.now(timezone.utc) - timedelta(hours=1)

            for event in events.items:
                if event.first_timestamp and event.first_timestamp.replace(tzinfo=None) > cutoff_time:
                    event_data = {
                        "type": event.type,
                        "reason": event.reason,
                        "message": event.message,
                        "object": f"{event.involved_object.kind}/{event.involved_object.name}",
                        "timestamp": event.first_timestamp.isoformat() if event.first_timestamp else None,
                        "count": event.count or 1
                    }
                    recent_events.append(event_data)

            # Sort by timestamp
            recent_events.sort(key=lambda x: x["timestamp"], reverse=True)

            return {
                "recent_events": {
                    "namespace": namespace,
                    "total_events": len(recent_events),
                    "events": recent_events[:50],  # Limit to 50 most recent
                    "warning_count": len([e for e in recent_events if e["type"] == "Warning"]),
                    "normal_count": len([e for e in recent_events if e["type"] == "Normal"])
                }
            }

        except ApiException as e:
            self.logger.warning(f"Failed to get recent events: {e}")
            return {"recent_events": {"error": str(e)}}

    async def _get_resource_quotas_context(self, namespace: str) -> Dict[str, Any]:
        """Replaces MCP check_resource_quotas tool."""
        try:
            if not self._k8s_client:
                await self._init_k8s_client()

            v1 = k8s_client.CoreV1Api(self._k8s_client)

            try:
                quotas = v1.list_namespaced_resource_quota(namespace=namespace)

                quota_info = []
                for quota in quotas.items:
                    quota_data = {
                        "name": quota.metadata.name,
                        "hard_limits": dict(quota.status.hard or {}),
                        "used": dict(quota.status.used or {}),
                        "utilization": {}
                    }

                    # Calculate utilization percentages
                    for resource, hard_limit in quota_data["hard_limits"].items():
                        used_amount = quota_data["used"].get(resource, "0")
                        try:
                            if resource.endswith("cpu"):
                                hard_val = self._parse_cpu_quantity(hard_limit)
                                used_val = self._parse_cpu_quantity(used_amount)
                            elif resource.endswith("memory"):
                                hard_val = self._parse_memory_quantity(hard_limit)
                                used_val = self._parse_memory_quantity(used_amount)
                            else:
                                hard_val = float(hard_limit)
                                used_val = float(used_amount)

                            if hard_val > 0:
                                quota_data["utilization"][resource] = (used_val / hard_val) * 100
                        except (ValueError, TypeError):
                            quota_data["utilization"][resource] = 0

                    quota_info.append(quota_data)

                return {
                    "resource_quotas": {
                        "namespace": namespace,
                        "quotas": quota_info,
                        "has_quotas": len(quota_info) > 0
                    }
                }

            except ApiException as e:
                if e.status == 404:
                    return {"resource_quotas": {"namespace": namespace, "quotas": [], "has_quotas": False}}
                raise

        except ApiException as e:
            self.logger.warning(f"Failed to get resource quotas: {e}")
            return {"resource_quotas": {"error": str(e)}}

    async def _get_workload_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get additional workload context (deployments, services, etc.)."""
        try:
            if not self._k8s_client:
                await self._init_k8s_client()

            apps_v1 = k8s_client.AppsV1Api(self._k8s_client)

            # Get deployments
            deployments = apps_v1.list_namespaced_deployment(namespace=namespace)

            workload_info = {
                "deployments": [],
                "total_deployments": len(deployments.items)
            }

            for deployment in deployments.items:
                dep_data = {
                    "name": deployment.metadata.name,
                    "replicas": deployment.spec.replicas or 0,
                    "ready_replicas": deployment.status.ready_replicas or 0,
                    "available_replicas": deployment.status.available_replicas or 0,
                    "updated_replicas": deployment.status.updated_replicas or 0,
                    "strategy": deployment.spec.strategy.type if deployment.spec.strategy else "Unknown"
                }
                workload_info["deployments"].append(dep_data)

            return {"workloads": workload_info}

        except ApiException as e:
            self.logger.warning(f"Failed to get workload context: {e}")
            return {"workloads": {"error": str(e)}}

    async def _init_k8s_client(self):
        """Initialize Kubernetes client."""
        try:
            # Try in-cluster config first, then local kubeconfig
            from kubernetes import config
            try:
                config.load_incluster_config()
                self.logger.info("Using in-cluster Kubernetes configuration")
            except:
                config.load_kube_config()
                self.logger.info("Using local kubeconfig")

            self._k8s_client = k8s_client.ApiClient()

        except Exception as e:
            self.logger.error(f"Failed to initialize Kubernetes client: {e}")
            raise

    def _is_pod_ready(self, pod) -> bool:
        """Check if pod is ready."""
        if not pod.status.conditions:
            return False
        return any(cond.type == "Ready" and cond.status == "True"
                  for cond in pod.status.conditions)

    def _extract_pod_resources(self, pod) -> Dict[str, Any]:
        """Extract resource requests and limits from pod."""
        resources = {"requests": {}, "limits": {}}

        if not pod.spec.containers:
            return resources

        for container in pod.spec.containers:
            if container.resources:
                if container.resources.requests:
                    for resource, value in container.resources.requests.items():
                        resources["requests"][resource] = resources["requests"].get(resource, 0) + self._parse_resource_quantity(resource, value)

                if container.resources.limits:
                    for resource, value in container.resources.limits.items():
                        resources["limits"][resource] = resources["limits"].get(resource, 0) + self._parse_resource_quantity(resource, value)

        return resources

    def _get_container_state(self, container) -> str:
        """Get container state summary."""
        if container.state.running:
            return "running"
        elif container.state.waiting:
            return f"waiting: {container.state.waiting.reason}"
        elif container.state.terminated:
            return f"terminated: {container.state.terminated.reason}"
        return "unknown"

    def _summarize_pod_health(self, pods: List[Dict]) -> Dict[str, int]:
        """Summarize pod health statistics."""
        summary = {
            "running": 0,
            "pending": 0,
            "failed": 0,
            "succeeded": 0,
            "unknown": 0,
            "ready": 0,
            "not_ready": 0
        }

        for pod in pods:
            phase = pod.get("phase", "unknown").lower()
            if phase in summary:
                summary[phase] += 1
            else:
                summary["unknown"] += 1

            if pod.get("ready"):
                summary["ready"] += 1
            else:
                summary["not_ready"] += 1

        return summary

    def _parse_cpu_quantity(self, cpu_str: str) -> float:
        """Parse CPU quantity to cores."""
        if not cpu_str:
            return 0.0

        cpu_str = str(cpu_str).lower()

        if cpu_str.endswith('m'):
            return float(cpu_str[:-1]) / 1000
        elif cpu_str.endswith('u'):
            return float(cpu_str[:-1]) / 1000000
        elif cpu_str.endswith('n'):
            return float(cpu_str[:-1]) / 1000000000
        else:
            return float(cpu_str)

    def _parse_memory_quantity(self, memory_str: str) -> int:
        """Parse memory quantity to bytes."""
        if not memory_str:
            return 0

        memory_str = str(memory_str).upper()

        multipliers = {
            'K': 1024, 'KI': 1024,
            'M': 1024*1024, 'MI': 1024*1024,
            'G': 1024*1024*1024, 'GI': 1024*1024*1024,
            'T': 1024*1024*1024*1024, 'TI': 1024*1024*1024*1024,
            'KB': 1000, 'MB': 1000*1000, 'GB': 1000*1000*1000, 'TB': 1000*1000*1000*1000
        }

        for suffix, multiplier in multipliers.items():
            if memory_str.endswith(suffix):
                return int(float(memory_str[:-len(suffix)]) * multiplier)

        return int(memory_str)

    def _parse_resource_quantity(self, resource_type: str, value: str) -> float:
        """Parse resource quantity based on type."""
        if "cpu" in resource_type.lower():
            return self._parse_cpu_quantity(value)
        elif "memory" in resource_type.lower():
            return self._parse_memory_quantity(value)
        else:
            try:
                return float(value)
            except ValueError:
                return 0.0

    # Additional context methods for backwards compatibility with tests
    async def _get_service_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get service context for the namespace."""
        try:
            if not self._k8s_client:
                await self._ensure_client()

            v1_api = k8s_client.CoreV1Api(self._k8s_client)
            services = v1_api.list_namespaced_service(namespace=namespace)

            service_list = []
            for service in services.items:
                service_info = {
                    "name": service.metadata.name,
                    "type": service.spec.type,
                    "cluster_ip": service.spec.cluster_ip,
                    "ports": [
                        {
                            "name": port.name,
                            "port": port.port,
                            "target_port": port.target_port,
                            "protocol": port.protocol
                        } for port in (service.spec.ports or [])
                    ]
                }
                service_list.append(service_info)

            return {"services": service_list}

        except Exception as e:
            self.logger.error(f"Failed to get service context: {e}")
            return {"services": [], "error": str(e)}

    async def _get_configmap_secrets_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get ConfigMaps and Secrets context for the namespace."""
        try:
            if not self._k8s_client:
                await self._ensure_client()

            v1_api = k8s_client.CoreV1Api(self._k8s_client)

            # Get ConfigMaps
            configmaps = v1_api.list_namespaced_config_map(namespace=namespace)
            cm_list = [
                {
                    "name": cm.metadata.name,
                    "data_keys": list(cm.data.keys()) if cm.data else []
                } for cm in configmaps.items
            ]

            # Get Secrets
            secrets = v1_api.list_namespaced_secret(namespace=namespace)
            secret_list = [
                {
                    "name": secret.metadata.name,
                    "type": secret.type,
                    "data_keys": list(secret.data.keys()) if secret.data else []
                } for secret in secrets.items
            ]

            return {
                "configmaps": cm_list,
                "secrets": secret_list
            }

        except Exception as e:
            self.logger.error(f"Failed to get configmap/secrets context: {e}")
            return {"configmaps": [], "secrets": [], "error": str(e)}

    async def _get_network_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get network context including network policies."""
        try:
            if not self._k8s_client:
                await self._ensure_client()

            networking_api = k8s_client.NetworkingV1Api(self._k8s_client)

            # Get Network Policies
            policies = networking_api.list_namespaced_network_policy(namespace=namespace)
            policy_list = [
                {
                    "name": policy.metadata.name,
                    "pod_selector": policy.spec.pod_selector.match_labels if policy.spec.pod_selector and policy.spec.pod_selector.match_labels else {}
                } for policy in policies.items
            ]

            return {"network_policies": policy_list}

        except Exception as e:
            self.logger.error(f"Failed to get network context: {e}")
            return {"network_policies": [], "error": str(e)}

    async def _get_volume_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get volume context including PVCs."""
        try:
            if not self._k8s_client:
                await self._ensure_client()

            v1_api = k8s_client.CoreV1Api(self._k8s_client)

            # Get PersistentVolumeClaims
            pvcs = v1_api.list_namespaced_persistent_volume_claim(namespace=namespace)
            pvc_list = [
                {
                    "name": pvc.metadata.name,
                    "status": pvc.status.phase,
                    "capacity": pvc.status.capacity.get("storage") if pvc.status.capacity else None,
                    "access_modes": pvc.spec.access_modes
                } for pvc in pvcs.items
            ]

            return {"persistent_volume_claims": pvc_list}

        except Exception as e:
            self.logger.error(f"Failed to get volume context: {e}")
            return {"persistent_volume_claims": [], "error": str(e)}

    async def _get_events_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get events context (alias for _get_recent_events_context)."""
        return await self._get_recent_events_context(namespace)

    async def _get_rbac_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get RBAC context including roles and bindings."""
        try:
            if not self._k8s_client:
                await self._ensure_client()

            rbac_api = k8s_client.RbacAuthorizationV1Api(self._k8s_client)

            # Get Roles
            roles = rbac_api.list_namespaced_role(namespace=namespace)
            role_list = [
                {
                    "name": role.metadata.name,
                    "rules_count": len(role.rules) if role.rules else 0
                } for role in roles.items
            ]

            # Get RoleBindings
            role_bindings = rbac_api.list_namespaced_role_binding(namespace=namespace)
            binding_list = [
                {
                    "name": binding.metadata.name,
                    "role_ref": binding.role_ref.name,
                    "subjects_count": len(binding.subjects) if binding.subjects else 0
                } for binding in role_bindings.items
            ]

            return {
                "roles": role_list,
                "role_bindings": binding_list
            }

        except Exception as e:
            self.logger.error(f"Failed to get RBAC context: {e}")
            return {"roles": [], "role_bindings": [], "error": str(e)}

    async def _get_node_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Get node context (alias for _get_cluster_capacity_context)."""
        return await self._get_cluster_capacity_context()

    async def gather_context(self, namespace: str, alert: AlertData) -> Dict[str, Any]:
        """Gather comprehensive context (alias for get_investigation_context)."""
        return await self.get_investigation_context(alert)

