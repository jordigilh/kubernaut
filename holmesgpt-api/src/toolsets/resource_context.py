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

After the LLM performs Root Cause Analysis and identifies the affected resource,
it calls get_resource_context to fetch remediation history. The tool internally:
  1. Resolves the owner chain via K8s ownerReferences traversal
  2. Identifies the root managing resource (e.g., Pod -> RS -> Deployment)
  3. Computes a canonical spec hash (SHA-256) for the root owner
  4. Queries DataStorage for remediation history filtered by root owner + spec hash

Returns to the LLM:
  - root_owner: Identity of the root managing resource (kind, name, namespace)
  - remediation_history: Past remediations for that resource

Owner chain traversal and spec hash computation are internal implementation
details not exposed to the LLM. The LLM only needs to know what resource
was resolved as the root owner and what remediations have been tried before.
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

    The LLM calls this after RCA to get the root managing resource and
    remediation history for the identified target resource. Owner chain
    traversal and spec hash computation are internal to the tool.
    """

    def __init__(
        self,
        k8s_client: Any,
        history_fetcher: Optional[Callable] = None,
    ):
        super().__init__(
            name="get_resource_context",
            description=(
                "Get remediation context for a Kubernetes resource. Call this "
                "AFTER identifying the affected resource during Root Cause "
                "Analysis. The tool resolves the root managing resource (e.g., "
                "a Pod's managing Deployment) and returns its remediation "
                "history. Use the root_owner in your response as the "
                "affectedResource, and the history to avoid repeating "
                "recently failed remediations."
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
            additional_instructions="",
        )
        object.__setattr__(self, "_k8s_client", k8s_client)
        object.__setattr__(self, "_history_fetcher", history_fetcher)

    def get_parameterized_one_liner(self, params: Dict) -> str:
        kind = params.get("kind", "?")
        name = params.get("name", "?")
        namespace = params.get("namespace", "")
        return f"Get resource context for {kind}/{namespace}/{name}"

    def _invoke(self, params: Dict, user_approved: bool = False) -> StructuredToolResult:
        """Execute the resource context lookup (sync wrapper for async tool)."""
        import asyncio
        kind = params.get("kind", "")
        name = params.get("name", "")
        namespace = params.get("namespace", "")
        return asyncio.get_event_loop().run_until_complete(
            self._invoke_async(kind, name, namespace)
        )

    async def _invoke_async(self, kind: str, name: str, namespace: str = "") -> StructuredToolResult:
        """Async implementation of resource context lookup."""
        try:
            # Step 1 (internal): Resolve owner chain via K8s API
            owner_chain = await self._k8s_client.resolve_owner_chain(kind, name, namespace)

            # Step 2 (internal): Determine root owner (last entry in chain, or the resource itself)
            root_owner = owner_chain[-1] if owner_chain else {"kind": kind, "name": name, "namespace": namespace}

            # Step 3 (internal): Compute spec hash for root owner
            spec_hash = await self._k8s_client.compute_spec_hash(
                root_owner["kind"], root_owner["name"], root_owner.get("namespace", "")
            )

            # Step 4 (internal): Fetch remediation history for root owner + spec hash
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

            # Return only what the LLM needs: root owner identity + history
            result_data = {
                "root_owner": root_owner,
                "remediation_history": history,
            }

            logger.info({
                "event": "resource_context_resolved",
                "resource": f"{kind}/{namespace}/{name}",
                "chain_length": len(owner_chain),
                "root_owner": f"{root_owner['kind']}/{root_owner['name']}",
                "has_spec_hash": bool(spec_hash),
                "history_count": len(history),
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


class ResourceContextToolset(Toolset):
    """Toolset providing get_resource_context to the LLM.

    ADR-055: Registered alongside WorkflowDiscoveryToolset in both
    incident and recovery tool registries.
    """

    def __init__(
        self,
        k8s_client: Any = None,
        history_fetcher: Optional[Callable] = None,
    ):
        tool = GetResourceContextTool(
            k8s_client=k8s_client,
            history_fetcher=history_fetcher,
        )

        super().__init__(
            name="resource_context",
            description="Fetch remediation context (root owner + history) for a K8s resource",
            docs_url="",
            icon_url="",
            prerequisites=[],
            tools=[tool],
            enabled=True,
        )

    def get_example_config(self) -> Dict[str, Any]:
        return {
            "resource_context": {
                "enabled": True,
                "description": "ADR-055: Post-RCA resource context enrichment",
            }
        }
