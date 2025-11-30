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

This client provides typed access to the Data Storage Service REST API.
"""

from .client import DataStorageClient, DataStorageError
from .models import (
    CreateWorkflowRequest,
    DisableWorkflowRequest,
    RemediationWorkflow,
    WorkflowVersionSummary,
    WorkflowSearchRequest,
    WorkflowSearchFilters,
    WorkflowSearchResponse,
    WorkflowSearchResult,
    RFC7807Error,
)

__all__ = [
    "DataStorageClient",
    "DataStorageError",
    "CreateWorkflowRequest",
    "DisableWorkflowRequest",
    "RemediationWorkflow",
    "WorkflowVersionSummary",
    "WorkflowSearchRequest",
    "WorkflowSearchFilters",
    "WorkflowSearchResponse",
    "WorkflowSearchResult",
    "RFC7807Error",
]

