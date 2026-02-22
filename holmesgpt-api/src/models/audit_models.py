#
# Copyright 2025 Jordi Gil.
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
Audit Event Data Models - OpenAPI Generated Types

Business Requirement: BR-AUDIT-001 (Unified audit trail)
Design Decision: ADR-034 (Unified Audit Table Design)
Purpose: Type-safe event_data structures for audit events

MIGRATION COMPLETE: All audit payload types are now imported from OpenAPI-generated
DataStorage client. This eliminates duplicate type definitions and ensures
single source of truth from api/openapi/data-storage-v1.yaml.

To regenerate types: make generate-datastorage-client
"""

# Import OpenAPI-generated audit payload types from DataStorage client
# Note: src/clients is in sys.path (see buffered_store.py), so datastorage package is directly importable
from datastorage.models.llm_request_payload import LLMRequestPayload as LLMRequestEventData
from datastorage.models.llm_response_payload import LLMResponsePayload as LLMResponseEventData
from datastorage.models.llm_tool_call_payload import LLMToolCallPayload as LLMToolCallEventData
from datastorage.models.workflow_validation_payload import WorkflowValidationPayload as WorkflowValidationEventData
from datastorage.models.ai_agent_response_payload import AIAgentResponsePayload as HAPIResponseEventData

# Re-export for backward compatibility
__all__ = [
    "LLMRequestEventData",
    "LLMResponseEventData",
    "LLMToolCallEventData",
    "WorkflowValidationEventData",
    "HAPIResponseEventData",
]


