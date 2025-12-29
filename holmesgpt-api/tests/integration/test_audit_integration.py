"""
Integration tests for HAPI audit events with Data Storage service.

Business Requirement: BR-AUDIT-005 (Audit Trail)
Design Decisions:
  - ADR-032: Mandatory Audit Requirements
  - ADR-034: Unified Audit Table Design
  - ADR-038: Asynchronous Buffered Audit Ingestion

Tests verify:
1. All 4 HAPI audit event types are sent to DS service
2. Events are stored correctly in DS PostgreSQL database
3. Stored events match sent events (schema validation)
4. ADR-032 compliance (service fails if audit unavailable)

Test Coverage:
- llm_request event
- llm_response event
- llm_tool_call event
- workflow_validation_attempt event
"""

import pytest
import requests
import time
from typing import Dict, Any, List

# Data Storage OpenAPI client
import sys
sys.path.insert(0, 'src/clients')
from datastorage import ApiClient, Configuration
from datastorage.api.audit_write_api_api import AuditWriteAPIApi
from datastorage.models.audit_event_request import AuditEventRequest

# HAPI audit events
from src.audit.events import (
    create_llm_request_event,
    create_llm_response_event,
    create_tool_call_event,
    create_validation_attempt_event,
)


@pytest.fixture
def data_storage_client(data_storage_url):
    """Create Data Storage OpenAPI client for audit verification."""
    config = Configuration(host=data_storage_url)
    api_client = ApiClient(configuration=config)
    return AuditWriteAPIApi(api_client)


@pytest.fixture
def hapi_base_url():
    """HAPI service base URL."""
    return "http://localhost:18120"


def wait_for_audit_flush(seconds: int = 2):
    """
    Wait for buffered audit to flush to Data Storage.

    Per ADR-038: Buffered audit has configurable flush interval.
    Wait to ensure async buffer has flushed to database.
    """
    time.sleep(seconds)


# ========================================
# TOMBSTONE: DELETED ANTI-PATTERN TESTS
# ========================================
#
# **DELETED**: December 26, 2025
#
# **WHY DELETED**:
# These tests followed the WRONG PATTERN: They manually created audit events using
# audit helper functions (create_llm_request_event, create_llm_response_event, etc.)
# and directly called Data Storage API (auditStore.StoreAudit() equivalent) to test
# audit infrastructure, NOT HAPI business logic.
#
# **What they tested** (audit infrastructure):
# - ✅ Audit event helper functions build events correctly
# - ✅ Data Storage API accepts audit events
# - ✅ Data Storage persists events to PostgreSQL
# - ✅ ADR-034 field compliance for events
#
# **What they did NOT test** (HAPI business logic):
# - ❌ HAPI emits audits during incident analysis
# - ❌ HAPI emits audits during recovery analysis
# - ❌ HAPI audit trail captures LLM interactions
# - ❌ HAPI audit trail captures workflow validation attempts
#
# **Tests Deleted** (6 tests, ~340 lines):
# 1. TestLLMRequestAuditEvent::test_llm_request_event_stored_in_ds (lines 68-127)
#    - Manually created llm_request event
#    - Directly called Data Storage API
#    - Verified DS accepted event (not HAPI behavior)
#
# 2. TestLLMResponseAuditEvent::test_llm_response_event_stored_in_ds (lines 139-179)
#    - Manually created llm_response event
#    - Directly called Data Storage API
#    - Verified DS persistence (not HAPI behavior)
#
# 3. TestLLMToolCallAuditEvent::test_llm_tool_call_event_stored_in_ds (lines 191-231)
#    - Manually created llm_tool_call event
#    - Directly called Data Storage API
#    - Verified DS schema validation (not HAPI behavior)
#
# 4. TestWorkflowValidationAuditEvent::test_workflow_validation_event_stored_in_ds (lines 244-288)
#    - Manually created workflow_validation_attempt event
#    - Directly called Data Storage API
#    - Verified DS accepts validation events (not HAPI behavior)
#
# 5. TestWorkflowValidationAuditEvent::test_workflow_validation_final_attempt_with_human_review (lines 290-325)
#    - Manually created validation event with human review
#    - Directly called Data Storage API
#    - Verified DS handles final attempt (not HAPI behavior)
#
# 6. TestAuditEventSchemaValidation::test_all_event_types_have_required_adr034_fields (lines 338-408)
#    - Manually created all 4 event types
#    - Directly called Data Storage API
#    - Verified ADR-034 compliance (infrastructure, not HAPI)
#
# All tests used this FORBIDDEN pattern:
# ```python
# # ❌ WRONG PATTERN
# event = create_llm_request_event(...)  # Manually create event
# audit_request = AuditEventRequest(**event)
# response = data_storage_client.create_audit_event(audit_request)  # Direct DS API call
# assert response.status == "accepted"  # Verify DS API works (not HAPI)
# ```
#
# These tests belonged in:
# - `src/audit/test_events_unit.py` (audit helper unit tests)
# - `datastorage/test_audit_api_integration_test.go` (DS API integration tests)
#
# **REPLACED WITH**: test_hapi_audit_flow_integration.py (flow-based tests)
#
# New tests follow CORRECT pattern:
# ```python
# # ✅ CORRECT PATTERN
# response = call_hapi_incident_analyze(hapi_url, incident_request)  # Trigger business op
# time.sleep(3)  # Wait for buffered audit flush
# events = query_audit_events(data_storage_url, remediation_id)  # Verify audits emitted
# assert "llm_request" in [e.event_type for e in events]  # Verify HAPI behavior
# ```
#
# **Authority**:
# - TESTING_GUIDELINES.md: "Anti-Pattern: Direct Audit Infrastructure Testing"
# - System-Wide Triage: docs/handoff/AUDIT_INFRASTRUCTURE_TESTING_ANTI_PATTERN_TRIAGE_DEC_26_2025.md
# - HAPI Triage: docs/handoff/HAPI_AUDIT_INTEGRATION_ANTI_PATTERN_TRIAGE_DEC_26_2025.md
#
# **Reference Implementations** (flow-based pattern):
# - SignalProcessing: test/integration/signalprocessing/audit_integration_test.go
# - Gateway: test/integration/gateway/audit_integration_test.go
# - HAPI E2E: holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py
# - HAPI Integration: holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py
#
# ========================================
# NEXT STEPS
# ========================================
#
# 1. Use flow-based tests: test_hapi_audit_flow_integration.py
# 2. Create tracking issue: "Implement additional flow-based audit tests"
# 3. Consider adding tests for:
#    - Audit buffering behavior (ADR-038)
#    - Audit graceful degradation (DS unavailable)
#    - Audit correlation_id propagation
#
# ========================================
#
# Helper functions and fixtures above remain for potential reuse.
# All anti-pattern tests deleted.
# Use test_hapi_audit_flow_integration.py for flow-based audit testing.
#
# ========================================
