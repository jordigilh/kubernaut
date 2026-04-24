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
Remediation history DataStorage client wrapper with graceful degradation.

BR-HAPI-016: Remediation history context for LLM prompt enrichment.
DD-HAPI-016 v1.1: HAPI queries DS GET /api/v1/remediation-history/context.
DD-AUTH-014: ServiceAccount authentication for DataStorage access.

This module provides a type-safe wrapper around the generated OpenAPI client for
querying the remediation history endpoint on the DataStorage service. It uses the
same ServiceAccountAuthPoolManager singleton as other HAPI DS clients (workflow
catalog, buffered store) for connection reuse and ServiceAccount token injection.

Graceful degradation: if DS is unavailable or returns an error, the function
returns None and the LLM prompt is constructed without remediation history.
"""

import logging
import os
from typing import Any, Dict, Optional

from datastorage.api.remediation_history_api_api import RemediationHistoryAPIApi
from datastorage.api_client import ApiClient
from datastorage.configuration import Configuration
from datastorage.exceptions import ApiException

logger = logging.getLogger(__name__)


def create_remediation_history_api(
    app_config: Optional[Dict[str, Any]] = None,
) -> Optional[RemediationHistoryAPIApi]:
    """Create a RemediationHistoryAPIApi with ServiceAccount auth pool.

    Follows the same pattern as workflow_catalog.py (DD-AUTH-014):
    1. Resolve DS URL from app_config or DATA_STORAGE_URL env var
    2. Get shared ServiceAccountAuthPoolManager singleton
    3. Configure ApiClient with auth pool and timeout

    Args:
        app_config: Optional application configuration dict. If provided,
            may contain 'data_storage_url' key. Falls back to
            DATA_STORAGE_URL environment variable.

    Returns:
        Configured RemediationHistoryAPIApi, or None if DS URL is not available.
    """
    # Resolve DataStorage URL (same precedence as workflow_catalog.py)
    ds_url = None
    if app_config:
        ds_url = app_config.get("data_storage_url")
    if not ds_url:
        ds_url = os.getenv("DATA_STORAGE_URL", "")
    if not ds_url:
        logger.debug("DataStorage URL not configured, remediation history API unavailable")
        return None

    http_timeout = int(os.getenv("DATA_STORAGE_TIMEOUT", "60"))

    try:
        # DD-AUTH-014: Use singleton pool manager for ServiceAccount token injection
        from datastorage_pool_manager import get_shared_datastorage_pool_manager

        auth_pool = get_shared_datastorage_pool_manager()
        config = Configuration(host=ds_url)
        config.timeout = http_timeout
        api_client = ApiClient(configuration=config)
        api_client.rest_client.pool_manager = auth_pool

        logger.info(
            "Remediation history API client created",
            extra={"ds_url": ds_url, "timeout": http_timeout},
        )
        return RemediationHistoryAPIApi(api_client)

    except Exception as e:
        logger.warning(
            "Failed to create remediation history API client: %s",
            str(e),
        )
        return None


def query_remediation_history(
    api: Optional[RemediationHistoryAPIApi],
    target_kind: str,
    target_name: str,
    target_namespace: str,
    current_spec_hash: str,
) -> Optional[Dict[str, Any]]:
    """Query the DataStorage remediation history context endpoint.

    Uses the generated OpenAPI client (RemediationHistoryAPIApi) for type-safe
    requests with ServiceAccount authentication via the shared pool manager.

    Args:
        api: RemediationHistoryAPIApi instance with auth pool configured,
             or None if DataStorage is not configured.
        target_kind: Kubernetes resource kind (e.g., "Deployment").
        target_name: Kubernetes resource name.
        target_namespace: Kubernetes resource namespace.
        current_spec_hash: SHA-256 hash of the current target resource spec.

    Returns:
        The remediation history context dict from DS (camelCase keys), or None
        on any failure. None indicates graceful degradation -- the LLM prompt
        will be built without remediation history context.
    """
    if api is None:
        logger.debug("RemediationHistoryAPIApi not configured, skipping remediation history query")
        return None

    try:
        context = api.get_remediation_history_context(
            target_kind=target_kind,
            target_name=target_name,
            target_namespace=target_namespace,
            current_spec_hash=current_spec_hash,
        )
        # to_dict() returns camelCase aliases matching prompt builder expectations
        return context.to_dict()

    except ApiException as e:
        logger.warning(
            "DataStorage returned error for remediation history query",
            extra={"status": e.status, "reason": e.reason},
        )
        return None

    except (ConnectionError, ConnectionRefusedError) as e:
        logger.warning(
            "Cannot connect to DataStorage for remediation history",
            extra={"error": str(e)},
        )
        return None

    except Exception as e:
        logger.warning(
            "Unexpected error querying remediation history: %s",
            str(e),
        )
        return None


def fetch_remediation_history_for_request(
    api: Optional[RemediationHistoryAPIApi],
    request_data: Dict[str, Any],
    current_spec_hash: str,
) -> Optional[Dict[str, Any]]:
    """Convenience function to fetch remediation history for an incident request.

    Extracts resource_kind, resource_name, resource_namespace from request_data
    and delegates to query_remediation_history. Used by analyze_incident to
    wire remediation history into prompt construction.

    BR-HAPI-016: Remediation history context for LLM prompt enrichment.

    Args:
        api: RemediationHistoryAPIApi instance, or None if DS not configured.
        request_data: Incident request data dict containing
            resource_kind, resource_name, resource_namespace.
        current_spec_hash: SHA-256 canonical hash of the current target resource
            spec. Empty string means spec hash is unavailable (skip query).

    Returns:
        Remediation history context dict, or None on any failure or missing data.
    """
    if not current_spec_hash:
        logger.debug("No current_spec_hash available, skipping remediation history query")
        return None

    target_kind = request_data.get("resource_kind", "")
    target_name = request_data.get("resource_name", "")
    target_namespace = request_data.get("resource_namespace", "")

    if not all([target_kind, target_name, target_namespace]):
        logger.debug(
            "Incomplete target resource info, skipping remediation history query",
            extra={
                "target_kind": target_kind,
                "target_name": target_name,
                "target_namespace": target_namespace,
            },
        )
        return None

    return query_remediation_history(
        api=api,
        target_kind=target_kind,
        target_name=target_name,
        target_namespace=target_namespace,
        current_spec_hash=current_spec_hash,
    )
