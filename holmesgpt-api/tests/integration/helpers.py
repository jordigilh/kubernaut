"""
Copyright 2026 Jordi Gil.

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
Shared helpers for HAPI integration tests.

Extracted from test_hapi_audit_flow_integration.py to allow reuse
across all integration test modules.
"""

import time


def query_audit_events_with_retry(
    audit_store,
    correlation_id: str,
    event_category: str,
    event_type=None,
    min_expected_events: int = 1,
    timeout_seconds: int = 30,
    poll_interval: float = 1.0,
    limit: int = 100
):
    """
    Query Data Storage for audit events with explicit flush support.

    Eliminates async race conditions by flushing the audit buffer
    before querying, similar to Go integration tests.

    Pattern: flush() -> poll with retry -> assert

    Timeout Alignment (Feb 1, 2026):
    - Polling: 30s (matches Go AIAnalysis INT: Eventually(30*time.Second, 1*time.Second))
    - Poll interval: 1.0s (matches DataStorage 1-second batch flush interval)
    - Flush: 10s (matches Go Gateway/AuthWebhook suite_test.go)

    Pattern Difference from Go:
    - Go AIAnalysis: Flush on EACH retry (controller may buffer during polling)
    - HAPI Python: Flush ONCE (analyze_incident() completes before polling)
    Rationale: Direct function calls complete before polling, so single flush is sufficient.

    Architectural Fixes (Jan 31, 2026 - User Feedback):
    1. No direct DS access: Uses audit_store._audit_api (not separate DS client)
    2. Proper filtering: Requires event_category (mandatory) + event_type (optional)
    3. Pagination: Supports limit parameter (default 100, max 1000)
    4. Correct event_category: "aiagent" (AI Agent Provider, not "analysis")

    Args:
        audit_store: BufferedAuditStore instance (MANDATORY per ADR-032)
        correlation_id: Correlation ID for audit correlation
        event_category: Event category (ADR-034 mandatory field, "aiagent" for HAPI)
        event_type: Optional event type filter (e.g., "aiagent.response.complete")
        min_expected_events: Minimum number of events expected (default 1)
        timeout_seconds: Maximum time to wait for events (default 30s, aligned with Go)
        poll_interval: Time between polling attempts (default 1.0s)
        limit: Maximum events per query (1-1000, default 100)

    Returns:
        List of AuditEvent Pydantic models

    Raises:
        AssertionError: If events don't appear within timeout or audit_store not provided
    """
    # ADR-032: Audit is MANDATORY - fail if audit_store not provided
    if audit_store is None:
        raise AssertionError(
            "audit_store is required (ADR-032: Audit is MANDATORY). "
            "Ensure audit_store fixture is provided to test."
        )

    print(f"Flushing audit buffer before querying (correlation_id={correlation_id})...")
    # Increased timeout from 5s to 10s for parallel test execution (4 workers)
    # With connection pool contention, flush may take longer
    success = audit_store.flush(timeout=10.0)
    if not success:
        raise AssertionError(
            "Audit flush timeout - events may not be persisted. "
            "This indicates a problem with the audit buffer or DataStorage connection pool."
        )
    print("Audit buffer flushed successfully")

    start_time = time.time()
    attempts = 0

    while time.time() - start_time < timeout_seconds:
        attempts += 1

        # Query using audit_store's internal authenticated client (no separate DS access)
        if attempts == 1:
            print(f"AUDIT DEBUG: Querying DataStorage with:")
            print(f"   correlation_id={correlation_id}")
            print(f"   event_category={event_category}")
            print(f"   event_type={event_type}")
            print(f"   limit={limit}")

        response = audit_store._audit_api.query_audit_events(
            correlation_id=correlation_id,
            event_category=event_category,
            event_type=event_type,
            limit=limit,
            _request_timeout=5
        )
        events = response.data if response.data else []

        if len(events) >= min_expected_events:
            elapsed = time.time() - start_time
            print(f"Found {len(events)} audit events after {elapsed:.2f}s ({attempts} attempts)")
            if events:
                print(f"AUDIT DEBUG: Event types found: {[e.event_type for e in events]}")
            return events

        if attempts % 5 == 0:  # Log every 5 attempts
            elapsed = time.time() - start_time
            print(f"Waiting for audit events... {len(events)}/{min_expected_events} found after {elapsed:.2f}s")
            if events:
                print(f"   Event types so far: {[e.event_type for e in events]}")

        time.sleep(poll_interval)

    # Timeout - fail with diagnostic info
    elapsed = time.time() - start_time
    final_response = audit_store._audit_api.query_audit_events(
        correlation_id=correlation_id,
        event_category=event_category,
        event_type=event_type,
        limit=limit,
        _request_timeout=5
    )
    final_events = final_response.data if final_response.data else []
    raise AssertionError(
        f"Timeout waiting for audit events: expected >={min_expected_events}, "
        f"got {len(final_events)} after {elapsed:.2f}s ({attempts} attempts). "
        f"ADR-038: Buffered audit flush may be delayed. "
        f"Filters: correlation_id={correlation_id}, event_category={event_category}, event_type={event_type}"
    )
