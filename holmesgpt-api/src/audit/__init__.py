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
Audit module for HolmesGPT API

Business Requirement: BR-AUDIT-005 (Workflow Selection Audit Trail)
Design Decisions:
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

This module provides the Python equivalent of pkg/audit/ (Go implementation).

Key Components:
  - BufferedAuditStore: Async buffered audit event storage
  - AuditConfig: Configuration for buffer size, batch size, etc.

Usage:
    from src.audit import BufferedAuditStore, AuditConfig

    store = BufferedAuditStore(
        data_storage_url="http://data-storage:8080",
        config=AuditConfig()
    )

    # Non-blocking: Returns immediately
    store.store_audit({"event_id": "...", "event_type": "..."})

    # Graceful shutdown: Flushes remaining events
    store.close()
"""

# Standard library imports
# Third-party imports
# Local imports
from src.audit.buffered_store import BufferedAuditStore, AuditConfig  # noqa: E402
from src.audit.events import (  # noqa: E402
    create_llm_request_event,
    create_llm_response_event,
    create_tool_call_event,
    create_validation_attempt_event,
    create_aiagent_response_complete_event,
    create_hapi_response_complete_event,  # backward compat alias
)
from src.audit.factory import get_audit_store  # noqa: E402

__all__ = [
    "BufferedAuditStore",
    "AuditConfig",
    "get_audit_store",
    "create_llm_request_event",
    "create_llm_response_event",
    "create_tool_call_event",
    "create_validation_attempt_event",
    "create_aiagent_response_complete_event",
    "create_hapi_response_complete_event",  # backward compat alias
]

