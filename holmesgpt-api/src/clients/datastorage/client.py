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
Data Storage Service Python Client

Generated from OpenAPI v3 spec: docs/services/stateless/data-storage/openapi/v3.yaml

Business Requirements:
- BR-STORAGE-012: Workflow catalog persistence
- BR-STORAGE-013: Semantic search with hybrid weighted scoring
- BR-WORKFLOW-001: Workflow version management

Design Decisions:
- DD-STORAGE-011: Workflow CRUD API
- DD-WORKFLOW-012: Workflow immutability
- ADR-043: Workflow schema definition standard
"""

import logging
import requests
from typing import List, Optional
from urllib.parse import urljoin

from .models import (
    CreateWorkflowRequest,
    DisableWorkflowRequest,
    RemediationWorkflow,
    WorkflowVersionSummary,
    WorkflowSearchRequest,
    WorkflowSearchResponse,
    RFC7807Error,
)


class DataStorageError(Exception):
    """Exception raised when Data Storage Service returns an error"""

    def __init__(self, message: str, error: RFC7807Error | None = None):
        super().__init__(message)
        self.error = error

logger = logging.getLogger(__name__)


class DataStorageClient:
    """
    Python client for Data Storage Service REST API

    Business Requirements:
    - BR-STORAGE-012: Workflow catalog persistence
    - BR-STORAGE-013: Semantic search with hybrid weighted scoring
    - BR-WORKFLOW-001: Workflow version management

    Design Decisions:
    - DD-STORAGE-011: Workflow CRUD API
    - DD-WORKFLOW-012: Workflow immutability

    Usage:
        client = DataStorageClient("http://data-storage:8080")

        # Search workflows
        response = client.search_workflows(WorkflowSearchRequest(
            query="OOMKilled critical",
            remediation_id="rem-123",
            top_k=5,
        ))

        # Create workflow
        workflow = client.create_workflow(CreateWorkflowRequest(
            workflow_id="oom-recovery",
            version="1.0.0",
            name="OOM Recovery",
            description="Recover from OOMKilled",
            content="...",
            labels={"signal_type": "OOMKilled", "severity": "critical"},
        ))
    """

    def __init__(
        self,
        base_url: str,
        timeout: int = 10,
        headers: Optional[dict] = None,
    ):
        """
        Initialize Data Storage client

        Args:
            base_url: Data Storage Service base URL (e.g., "http://data-storage:8080")
            timeout: HTTP request timeout in seconds (default: 10)
            headers: Optional additional HTTP headers
        """
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.headers = headers or {}
        self.headers.setdefault("Content-Type", "application/json")
        self.headers.setdefault("Accept", "application/json")

        logger.info(f"DataStorageClient initialized: base_url={self.base_url}, timeout={self.timeout}s")

    def _handle_error(self, response: requests.Response) -> None:
        """
        Handle error response from Data Storage Service

        Args:
            response: HTTP response object

        Raises:
            DataStorageError: If response indicates an error
        """
        if response.status_code >= 400:
            try:
                error_data = response.json()
                error = RFC7807Error.model_validate(error_data)
                raise DataStorageError(
                    f"Data Storage Service error: {error.title} - {error.detail}",
                    error=error,
                )
            except (ValueError, KeyError):
                raise DataStorageError(
                    f"Data Storage Service error: HTTP {response.status_code} - {response.text}"
                )

    # ========================================
    # WORKFLOW CRUD OPERATIONS (DD-STORAGE-011)
    # ========================================

    def create_workflow(self, request: CreateWorkflowRequest) -> RemediationWorkflow:
        """
        Create a new workflow in the catalog

        POST /api/v1/workflows

        Business Requirement: BR-STORAGE-012
        Design Decisions: DD-STORAGE-011, ADR-043

        Args:
            request: CreateWorkflowRequest with workflow details

        Returns:
            Created RemediationWorkflow

        Raises:
            DataStorageError: If creation fails (400 for validation, 409 for duplicate)
        """
        url = f"{self.base_url}/api/v1/workflows"

        logger.info(f"Creating workflow: workflow_id={request.workflow_id}, version={request.version}")

        response = requests.post(
            url,
            json=request.model_dump(exclude_none=True),
            headers=self.headers,
            timeout=self.timeout,
        )

        self._handle_error(response)

        workflow = RemediationWorkflow.model_validate(response.json())
        logger.info(f"Workflow created successfully: workflow_id={workflow.workflow_id}")

        return workflow

    def get_workflow(self, workflow_id: str, version: str) -> Optional[RemediationWorkflow]:
        """
        Get a workflow by ID and version

        GET /api/v1/workflows/{workflow_id}/{version}

        Business Requirement: BR-WORKFLOW-001
        Design Decision: DD-STORAGE-011

        Args:
            workflow_id: Unique workflow identifier
            version: Semantic version

        Returns:
            RemediationWorkflow if found, None if not found

        Raises:
            DataStorageError: If request fails (excluding 404)
        """
        url = f"{self.base_url}/api/v1/workflows/{workflow_id}/{version}"

        logger.debug(f"Getting workflow: workflow_id={workflow_id}, version={version}")

        response = requests.get(
            url,
            headers=self.headers,
            timeout=self.timeout,
        )

        if response.status_code == 404:
            return None

        self._handle_error(response)

        return RemediationWorkflow.model_validate(response.json())

    def list_workflow_versions(self, workflow_id: str) -> List[WorkflowVersionSummary]:
        """
        List all versions of a workflow

        GET /api/v1/workflows/{workflow_id}/versions

        Business Requirement: BR-WORKFLOW-001
        Design Decision: DD-STORAGE-011

        Args:
            workflow_id: Unique workflow identifier

        Returns:
            List of WorkflowVersionSummary (empty if none found)

        Raises:
            DataStorageError: If request fails
        """
        url = f"{self.base_url}/api/v1/workflows/{workflow_id}/versions"

        logger.debug(f"Listing workflow versions: workflow_id={workflow_id}")

        response = requests.get(
            url,
            headers=self.headers,
            timeout=self.timeout,
        )

        self._handle_error(response)

        return [WorkflowVersionSummary.model_validate(v) for v in response.json()]

    def disable_workflow(
        self, workflow_id: str, version: str, request: DisableWorkflowRequest
    ) -> RemediationWorkflow:
        """
        Disable a workflow version (soft delete)

        PATCH /api/v1/workflows/{workflow_id}/{version}/disable

        Business Requirement: BR-WORKFLOW-001
        Design Decision: DD-WORKFLOW-012 (no hard delete)

        Args:
            workflow_id: Unique workflow identifier
            version: Semantic version
            request: DisableWorkflowRequest with reason

        Returns:
            Updated RemediationWorkflow

        Raises:
            DataStorageError: If disable fails (400 for missing reason, 404 for not found)
        """
        url = f"{self.base_url}/api/v1/workflows/{workflow_id}/{version}/disable"

        logger.info(f"Disabling workflow: workflow_id={workflow_id}, version={version}, reason={request.reason}")

        response = requests.patch(
            url,
            json=request.model_dump(exclude_none=True),
            headers=self.headers,
            timeout=self.timeout,
        )

        self._handle_error(response)

        workflow = RemediationWorkflow.model_validate(response.json())
        logger.info(f"Workflow disabled successfully: workflow_id={workflow.workflow_id}")

        return workflow

    # ========================================
    # WORKFLOW SEARCH (BR-STORAGE-013)
    # ========================================

    def search_workflows(self, request: WorkflowSearchRequest) -> WorkflowSearchResponse:
        """
        Search for workflows using semantic search

        POST /api/v1/workflows/search

        Business Requirement: BR-STORAGE-013
        Design Decisions:
        - DD-WORKFLOW-004: Hybrid weighted scoring (V1.0: confidence = base_similarity)
        - DD-WORKFLOW-014: remediation_id for audit correlation

        Args:
            request: WorkflowSearchRequest with query and filters

        Returns:
            WorkflowSearchResponse with ranked results

        Raises:
            DataStorageError: If search fails
        """
        url = f"{self.base_url}/api/v1/workflows/search"

        logger.info(
            f"Searching workflows: query='{request.query}', top_k={request.top_k}, "
            f"remediation_id={request.remediation_id or 'not-set'}"
        )

        response = requests.post(
            url,
            json=request.model_dump(exclude_none=True),
            headers=self.headers,
            timeout=self.timeout,
        )

        self._handle_error(response)

        search_response = WorkflowSearchResponse.model_validate(response.json())
        logger.info(f"Search completed: {search_response.total_found} workflows found")

        return search_response

    # ========================================
    # HEALTH CHECK
    # ========================================

    def health_check(self) -> bool:
        """
        Check if Data Storage Service is healthy

        GET /health

        Returns:
            True if healthy, False otherwise
        """
        try:
            response = requests.get(
                f"{self.base_url}/health",
                timeout=5,
            )
            return response.status_code == 200
        except Exception as e:
            logger.warning(f"Health check failed: {e}")
            return False

    def readiness_check(self) -> bool:
        """
        Check if Data Storage Service is ready

        GET /health/ready

        Returns:
            True if ready, False otherwise
        """
        try:
            response = requests.get(
                f"{self.base_url}/health/ready",
                timeout=5,
            )
            return response.status_code == 200
        except Exception as e:
            logger.warning(f"Readiness check failed: {e}")
            return False

