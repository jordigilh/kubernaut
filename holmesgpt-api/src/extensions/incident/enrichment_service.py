#
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
#

"""
EnrichmentService — Phase 2 of the #529 Three-Phase RCA Architecture.

Business Requirements:
- BR-HAPI-264: Post-RCA infrastructure label detection
- BR-HAPI-261: Owner chain resolution for LLM-provided affectedResource

Design Decisions:
- ADR-055 v1.5: EnrichmentService is sole authoritative source for enrichment
- ADR-056 v1.7: Label detection via EnrichmentService in Phase 2

Given an affectedResource from the LLM's Phase 1 response, this service:
1. Resolves the K8s owner chain to the root owner (e.g., Pod -> Deployment)
2. Detects infrastructure labels for the resolved root owner
3. Fetches remediation history from DataStorage for the root owner
4. Retries infrastructure calls with exponential backoff (3 retries, 1s/2s/4s)
5. Fails hard with EnrichmentFailure (rca_incomplete) after retry exhaustion
"""

import asyncio
import logging
from dataclasses import dataclass, field
from typing import Any, Callable, Coroutine, Dict, List, Optional

logger = logging.getLogger(__name__)

RETRY_DELAYS = [1.0, 2.0, 4.0]


@dataclass
class EnrichmentResult:
    """Result of Phase 2 enrichment."""
    root_owner: Optional[Dict[str, str]] = None
    detected_labels: Optional[Dict[str, Any]] = None
    remediation_history: Optional[Dict[str, Any]] = None


class EnrichmentFailure(Exception):
    """Raised when enrichment fails after retry exhaustion."""

    def __init__(self, reason: str, detail: str = ""):
        self.reason = reason
        self.detail = detail
        super().__init__(f"{reason}: {detail}")


class EnrichmentService:
    """HAPI-driven Phase 2 enrichment service."""

    def __init__(
        self,
        k8s_client: Any,
        ds_client: Any,
        label_detector: Optional[Callable[..., Coroutine]] = None,
    ):
        self._k8s = k8s_client
        self._ds = ds_client
        self._label_detector = label_detector

    async def enrich(self, affected_resource: Dict[str, str]) -> EnrichmentResult:
        """Run Phase 2 enrichment for the given affectedResource."""
        kind = affected_resource["kind"]
        name = affected_resource["name"]
        namespace = affected_resource.get("namespace", "")

        owner_chain = await self._retry(
            lambda: self._k8s.resolve_owner_chain(kind, name, namespace),
            "resolve_owner_chain",
        )

        root_owner = owner_chain[-1] if owner_chain else {"kind": kind, "name": name, "namespace": namespace}

        detected_labels = None
        if self._label_detector:
            try:
                detected_labels = await self._label_detector(root_owner)
            except Exception:
                logger.warning({"event": "label_detection_failed", "root_owner": root_owner})

        remediation_history = None
        try:
            remediation_history = await self._retry(
                lambda: self._ds.query_remediation_history(
                    target_kind=root_owner["kind"],
                    target_name=root_owner["name"],
                    target_namespace=root_owner.get("namespace", ""),
                ),
                "query_remediation_history",
            )
        except EnrichmentFailure:
            raise
        except Exception:
            logger.warning({"event": "history_fetch_failed", "root_owner": root_owner})

        return EnrichmentResult(
            root_owner=root_owner,
            detected_labels=detected_labels,
            remediation_history=remediation_history,
        )

    async def _retry(self, fn: Callable, operation: str) -> Any:
        """Retry an async operation with exponential backoff."""
        last_error: Optional[Exception] = None
        for attempt, delay in enumerate(RETRY_DELAYS):
            try:
                return await fn()
            except Exception as e:
                last_error = e
                logger.warning({
                    "event": "retry",
                    "operation": operation,
                    "attempt": attempt + 1,
                    "delay_s": delay,
                    "error": str(e),
                })
                await asyncio.sleep(delay)

        raise EnrichmentFailure(
            reason="rca_incomplete",
            detail=f"{operation} failed after {len(RETRY_DELAYS)} retries: {last_error}",
        )
