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
Resource Context Toolset for HolmesGPT SDK.

ADR-055: LLM-Driven Context Enrichment (Post-RCA)
ADR-056 v1.4: DetectedLabels computation for the RCA target resource.

After the LLM performs Root Cause Analysis and identifies the affected resource,
it calls get_resource_context to:
  1. Resolve the owner chain via K8s ownerReferences traversal
  2. Identify the root managing resource (e.g., Pod -> RS -> Deployment)
  3. Compute a canonical spec hash (SHA-256) for the root owner
  4. Query DataStorage for remediation history filtered by root owner + spec hash
  5. Detect infrastructure labels for the RCA target (one-shot, ADR-056 v1.4)
  6. Conditionally return detected_infrastructure for LLM RCA reassessment

Returns to the LLM:
  - root_owner: Identity of the root managing resource (kind, name, namespace)
  - remediation_history: Past remediations for that resource
  - detected_infrastructure (conditional): Active infrastructure labels for
    RCA reassessment, included only on the first call when active labels exist.
"""

import asyncio
import logging
from typing import Any, Callable, Dict, List, Optional

from holmes.core.tools import (
    Tool,
    Toolset,
    StructuredToolResult,
    StructuredToolResultStatus,
    ToolParameter,
    ToolsetStatusEnum,
)

logger = logging.getLogger(__name__)


class GetResourceContextTool(Tool):
    """LLM-callable tool that fetches remediation context for a K8s resource.

    The LLM calls this after RCA to get the root managing resource,
    remediation history, and infrastructure context for the identified
    target resource. Owner chain traversal, spec hash computation, and
    label detection are internal to the tool.
    """

    def __init__(
        self,
        k8s_client: Any,
        history_fetcher: Optional[Callable] = None,
        session_state: Optional[Dict[str, Any]] = None,
    ):
        super().__init__(
            name="get_resource_context",
            description=(
                "Get remediation context for a Kubernetes resource. Call this "
                "AFTER identifying the affected resource during Root Cause "
                "Analysis. The tool resolves the root managing resource (e.g., "
                "a Pod's managing Deployment), returns its remediation "
                "history, and detects infrastructure characteristics. "
                "If infrastructure characteristics are detected (e.g., GitOps "
                "management, PDB protection), review them to reassess your "
                "RCA strategy before selecting a workflow."
            ),
            parameters={
                "kind": ToolParameter(
                    description="Kubernetes resource kind (e.g., Pod, Deployment, Node)",
                    type="string",
                    required=True,
                ),
                "name": ToolParameter(
                    description="Resource name",
                    type="string",
                    required=True,
                ),
                "namespace": ToolParameter(
                    description="Resource namespace (empty for cluster-scoped resources)",
                    type="string",
                    required=False,
                ),
            },
            additional_instructions=(
                "If the response includes 'detected_infrastructure', review "
                "the infrastructure labels and consider whether they change "
                "your root cause analysis or remediation approach. For "
                "example, gitOpsManaged=true may mean reverting via Git "
                "instead of direct kubectl patching. You may call this tool "
                "again for a different resource if you reassess the target, "
                "but labels will not be re-detected."
            ),
        )
        object.__setattr__(self, "_k8s_client", k8s_client)
        object.__setattr__(self, "_history_fetcher", history_fetcher)
        object.__setattr__(self, "_session_state", session_state)

    def get_parameterized_one_liner(self, params: Dict) -> str:
        kind = params.get("kind", "?")
        name = params.get("name", "?")
        namespace = params.get("namespace", "")
        return f"Get resource context for {kind}/{namespace}/{name}"

    def _invoke(self, params: Dict, user_approved: bool = False) -> StructuredToolResult:
        """Execute the resource context lookup (sync wrapper for async tool).

        Uses asyncio.run() to create a fresh event loop because the HolmesGPT
        SDK invokes tools in a ThreadPoolExecutor thread where no event loop
        exists.  asyncio.get_event_loop() raises RuntimeError in that context.
        """
        kind = params.get("kind", "")
        name = params.get("name", "")
        namespace = params.get("namespace", "")
        return asyncio.run(self._invoke_async(kind, name, namespace))

    async def _invoke_async(self, kind: str, name: str, namespace: str = "") -> StructuredToolResult:
        """Async implementation of resource context lookup."""
        try:
            owner_chain = await self._k8s_client.resolve_owner_chain(kind, name, namespace)

            root_owner = owner_chain[-1] if owner_chain else {"kind": kind, "name": name, "namespace": namespace}

            spec_hash = await self._k8s_client.compute_spec_hash(
                root_owner["kind"], root_owner["name"], root_owner.get("namespace", "")
            )

            history = []
            if self._history_fetcher:
                try:
                    history = self._history_fetcher(
                        resource_kind=root_owner["kind"],
                        resource_name=root_owner["name"],
                        resource_namespace=root_owner.get("namespace", ""),
                        current_spec_hash=spec_hash,
                    )
                except Exception as e:
                    logger.warning({
                        "event": "remediation_history_fetch_failed",
                        "resource": f"{kind}/{name}",
                        "error": str(e),
                    })

            result_data = {
                "root_owner": root_owner,
                "remediation_history": history,
            }

            detected_infra = await self._detect_labels_if_needed(
                kind, name, namespace, owner_chain
            )
            if detected_infra is not None:
                result_data["detected_infrastructure"] = detected_infra

            logger.info({
                "event": "resource_context_resolved",
                "resource": f"{kind}/{namespace}/{name}",
                "chain_length": len(owner_chain),
                "root_owner": f"{root_owner['kind']}/{root_owner['name']}",
                "has_spec_hash": bool(spec_hash),
                "history_count": len(history),
                "has_detected_infrastructure": detected_infra is not None,
            })

            return StructuredToolResult(
                status=StructuredToolResultStatus.SUCCESS,
                data=result_data,
            )

        except Exception as e:
            logger.error({
                "event": "resource_context_failed",
                "resource": f"{kind}/{namespace}/{name}",
                "error": str(e),
            })
            return StructuredToolResult(
                status=StructuredToolResultStatus.ERROR,
                data={"error": str(e)},
            )

    async def _detect_labels_if_needed(
        self,
        kind: str,
        name: str,
        namespace: str,
        owner_chain: List[Dict[str, str]],
    ) -> Optional[Dict[str, Any]]:
        """One-shot label detection: detect on first call, skip on subsequent calls.

        Returns a detected_infrastructure dict (with labels + note) when active
        labels are found on the first call. Returns None when labels are all
        defaults, when session_state is unavailable, or on subsequent calls.
        """
        if self._session_state is None:
            return None

        if "detected_labels" in self._session_state:
            return None

        try:
            from src.detection.labels import LabelDetector

            k8s_context = await self._build_k8s_context(kind, name, namespace, owner_chain)
            detector = LabelDetector(self._k8s_client)
            labels = await detector.detect_labels(k8s_context, owner_chain)

            if labels is None:
                self._session_state["detected_labels"] = {}
                return None

            self._session_state["detected_labels"] = labels

            if self._has_active_labels(labels):
                display_labels = {
                    k: v for k, v in labels.items() if k != "failedDetections"
                }
                return {
                    "labels": display_labels,
                    "note": (
                        "Infrastructure characteristics detected for the RCA target. "
                        "Consider whether these affect your root cause analysis or "
                        "remediation strategy before selecting a workflow."
                    ),
                }

            return None

        except Exception as e:
            logger.warning({
                "event": "label_detection_failed",
                "resource": f"{kind}/{namespace}/{name}",
                "error": str(e),
            })
            self._session_state["detected_labels"] = {}
            return None

    async def _build_k8s_context(
        self, kind: str, name: str, namespace: str, owner_chain: List[Dict[str, str]]
    ) -> Dict[str, Any]:
        """Build k8s_context dict for LabelDetector from K8s resource metadata.

        DD-HAPI-018 v1.2: Handles workload targets (Pod, Deployment) and
        non-workload targets (PodDisruptionBudget). For PDB targets, resolves
        pod context from the PDB's spec.selector.matchLabels.
        """
        k8s_context: Dict[str, Any] = {"namespace": namespace}

        if kind == "Pod":
            pod = await self._k8s_client._get_resource_metadata("Pod", name, namespace)
            if pod is not None:
                k8s_context["pod_details"] = {
                    "name": name,
                    "labels": pod.metadata.labels or {},
                    "annotations": pod.metadata.annotations or {},
                }

        elif kind == "PodDisruptionBudget":
            await self._build_pdb_target_context(name, namespace, k8s_context)

        for entry in owner_chain:
            if entry["kind"] == "Deployment":
                deploy = await self._k8s_client._get_resource_metadata(
                    "Deployment", entry["name"], entry.get("namespace", "")
                )
                if deploy is not None:
                    k8s_context["deployment_details"] = {
                        "name": entry["name"],
                        "labels": deploy.metadata.labels or {},
                        "annotations": deploy.metadata.annotations or {},
                    }
                    # DD-HAPI-018: PDB selectors match Pod labels. When target is a
                    # Deployment, use pod template labels for PDB detection (Pods
                    # created by the Deployment inherit these labels).
                    if "pod_details" not in k8s_context and hasattr(
                        deploy, "spec"
                    ) and deploy.spec is not None:
                        template = getattr(deploy.spec, "template", None)
                        if template is not None:
                            meta = getattr(template, "metadata", None)
                            if meta is not None:
                                pod_labels = getattr(meta, "labels", None) or {}
                                if pod_labels:
                                    k8s_context["pod_details"] = {
                                        "name": entry["name"],
                                        "labels": pod_labels,
                                        "annotations": getattr(
                                            meta, "annotations", None
                                        )
                                        or {},
                                    }
                break

        ns_meta = await self._k8s_client.get_namespace_metadata(namespace)
        if ns_meta is not None:
            k8s_context["namespace_labels"] = ns_meta.get("labels", {})
            k8s_context["namespace_annotations"] = ns_meta.get("annotations", {})
        else:
            k8s_context["namespace_labels"] = {}
            k8s_context["namespace_annotations"] = {}

        return k8s_context

    async def _build_pdb_target_context(
        self, pdb_name: str, namespace: str, k8s_context: Dict[str, Any]
    ) -> None:
        """Resolve pod context when the RCA target is a PodDisruptionBudget.

        DD-HAPI-018 v1.2: Lists PDBs in namespace, finds the target by name,
        reads its spec.selector.matchLabels, lists matching pods, and populates
        pod_details from the first matched pod.
        """
        try:
            pdbs, error = await self._k8s_client.list_pdbs(namespace)
            if error:
                logger.warning({
                    "event": "pdb_target_list_failed",
                    "pdb_name": pdb_name,
                    "namespace": namespace,
                    "error": error,
                })
                return

            target_pdb = None
            for pdb in pdbs:
                if pdb.metadata.name == pdb_name:
                    target_pdb = pdb
                    break

            if target_pdb is None:
                logger.info({
                    "event": "pdb_target_not_found",
                    "pdb_name": pdb_name,
                    "namespace": namespace,
                })
                return

            selector = getattr(target_pdb.spec, "selector", None)
            if selector is None:
                return

            match_labels = getattr(selector, "match_labels", None) or {}
            if not match_labels:
                return

            pods, pod_error = await self._k8s_client.list_pods_by_selector(
                namespace, match_labels
            )
            if pod_error or not pods:
                return

            first_pod = pods[0]
            k8s_context["pod_details"] = {
                "name": first_pod.metadata.name,
                "labels": first_pod.metadata.labels or {},
                "annotations": first_pod.metadata.annotations or {},
            }

        except Exception as e:
            logger.warning({
                "event": "pdb_target_context_failed",
                "pdb_name": pdb_name,
                "namespace": namespace,
                "error": str(e),
            })

    @staticmethod
    def _has_active_labels(labels: Dict[str, Any]) -> bool:
        """Check if any label is active (boolean True or non-empty string)."""
        for key, value in labels.items():
            if key == "failedDetections":
                continue
            if isinstance(value, bool) and value is True:
                return True
            if isinstance(value, str) and value:
                return True
        return False


class ResourceContextToolset(Toolset):
    """Toolset providing get_resource_context to the LLM.

    ADR-055: Registered alongside WorkflowDiscoveryToolset in the
    incident tool registry.
    ADR-056 v1.4: Computes DetectedLabels for the RCA target and stores
    in session_state for downstream workflow discovery tools.
    """

    def __init__(
        self,
        k8s_client: Any = None,
        history_fetcher: Optional[Callable] = None,
        session_state: Optional[Dict[str, Any]] = None,
    ):
        tool = GetResourceContextTool(
            k8s_client=k8s_client,
            history_fetcher=history_fetcher,
            session_state=session_state,
        )

        super().__init__(
            name="resource_context",
            description="Fetch remediation context (root owner + history + infrastructure labels) for a K8s resource",
            docs_url="",
            icon_url="",
            prerequisites=[],
            tools=[tool],
            enabled=True,
            status=ToolsetStatusEnum.ENABLED,
        )

    def get_example_config(self) -> Dict[str, Any]:
        return {
            "resource_context": {
                "enabled": True,
                "description": "ADR-055/ADR-056: Post-RCA resource context enrichment with label detection",
            }
        }
