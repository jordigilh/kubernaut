# Phase 2b: HAPI Audit Client Migration to OpenAPI

**Date**: 2025-12-13
**Priority**: 🚨 **CRITICAL** - Production Code Path
**Estimated Effort**: 30-45 minutes

---

## 🎯 Objective

Migrate HAPI's audit client from manual `requests.post()` to Data Storage OpenAPI client for:
- ✅ Type safety (typed models)
- ✅ Contract validation (OpenAPI spec compliance)
- ✅ Consistency with integration test patterns
- ✅ Production reliability

---

## 🔍 Current State (Deprecated)

### File: `src/audit/buffered_store.py`

**Line 332**: Uses `requests.post()` directly
```python
response = requests.post(
    f"{self._url}/api/v1/audit/events",
    json=event,
    timeout=self._config.http_timeout_seconds
)
response.raise_for_status()
```

**Issues**:
- ❌ No type safety
- ❌ No contract validation
- ❌ Manual JSON construction
- ❌ Same deprecated pattern we're fixing in tests
- ❌ Same deprecated pattern used by 6 Go services

---

## ✅ Target State (OpenAPI Client)

### Use Generated OpenAPI Client

**Available Resources**:
- ✅ Client: `src/clients/datastorage/api/audit_write_api_api.py`
- ✅ Model: `src/clients/datastorage/models/audit_event_request.py`
- ✅ Response: `src/clients/datastorage/models/audit_event_response.py`
- ✅ Docs: `src/clients/docs/AuditWriteAPIApi.md`

**Target Code**:
```python
from src.clients.datastorage import ApiClient, Configuration
from src.clients.datastorage.api.audit_write_api_api import AuditWriteAPIApi
from src.clients.datastorage.models.audit_event_request import AuditEventRequest

# In __init__
config = Configuration(host=self._url)
api_client = ApiClient(configuration=config)
self._audit_api = AuditWriteAPIApi(api_client)

# In _write_single_event_with_retry
audit_request = AuditEventRequest(**event)
response = self._audit_api.create_audit_event(
    audit_event_request=audit_request
)
```

---

## 📋 Implementation Plan

### Step 1: Update Imports (2 min)
```python
# Add to src/audit/buffered_store.py
import sys
sys.path.insert(0, 'src/clients')

from datastorage import ApiClient, Configuration
from datastorage.api.audit_write_api_api import AuditWriteAPIApi
from datastorage.models.audit_event_request import AuditEventRequest
from datastorage.exceptions import ApiException
```

### Step 2: Initialize OpenAPI Client in `__init__` (5 min)
```python
def __init__(self, data_storage_url: str, config: AuditConfig):
    # ... existing code ...

    # Initialize OpenAPI client for audit writes
    api_config = Configuration(host=data_storage_url)
    api_config.timeout = config.http_timeout_seconds
    self._api_client = ApiClient(configuration=api_config)
    self._audit_api = AuditWriteAPIApi(self._api_client)

    logger.info(f"BR-AUDIT-005: Initialized OpenAPI audit client - url={data_storage_url}")
```

### Step 3: Update `_write_single_event_with_retry` (15 min)
```python
def _write_single_event_with_retry(self, event: Dict[str, Any]) -> bool:
    """
    Write single event with exponential backoff retry using OpenAPI client

    ADR-034: Uses typed AuditEventRequest model

    Args:
        event: ADR-034 compliant audit event

    Returns:
        True if written successfully, False if dropped
    """
    for attempt in range(1, self._config.max_retries + 1):
        try:
            # Create typed request from event dict
            audit_request = AuditEventRequest(**event)

            # Call OpenAPI endpoint (type-safe)
            response = self._audit_api.create_audit_event(
                audit_event_request=audit_request
            )

            logger.debug(
                f"✅ Audit event written via OpenAPI - "
                f"event_id={response.event_id}, "
                f"event_type={event.get('event_type')}"
            )
            return True

        except ApiException as e:
            event_type = event.get("event_type", "unknown")
            correlation_id = event.get("correlation_id", "")

            logger.warning(
                f"⚠️ DD-AUDIT-002: OpenAPI audit write failed - "
                f"attempt={attempt}/{self._config.max_retries}, "
                f"event_type={event_type}, correlation_id={correlation_id}, "
                f"status={e.status}, error={e.reason}"
            )

            if attempt < self._config.max_retries:
                backoff = attempt * attempt
                time.sleep(backoff)

        except Exception as e:
            # Handle validation errors (Pydantic), etc.
            event_type = event.get("event_type", "unknown")
            correlation_id = event.get("correlation_id", "")

            logger.error(
                f"❌ DD-AUDIT-002: Unexpected error in audit write - "
                f"event_type={event_type}, correlation_id={correlation_id}, "
                f"error={e}"
            )
            # Don't retry on validation errors
            return False

    # Final failure: Drop event
    event_type = event.get("event_type", "unknown")
    correlation_id = event.get("correlation_id", "")

    logger.error(
        f"❌ DD-AUDIT-002: Dropping audit event after max retries - "
        f"event_type={event_type}, correlation_id={correlation_id}, "
        f"max_retries={self._config.max_retries}"
    )
    return False
```

### Step 4: Update Tests (10 min)
- `tests/unit/test_audit_buffered_store.py` - Mock OpenAPI client instead of requests
- Verify audit events still work with OpenAPI client

### Step 5: Cleanup (5 min)
- Remove `import requests` if no longer needed
- Update docstrings to mention OpenAPI client
- Add migration note in CHANGELOG

---

## ✅ Benefits

### Type Safety
- ✅ Pydantic validation catches invalid events before sending
- ✅ IDE autocomplete for audit event fields
- ✅ Compile-time field checking

### Contract Validation
- ✅ OpenAPI spec enforced at runtime
- ✅ Breaking changes caught immediately
- ✅ Consistent with Data Storage expectations

### Consistency
- ✅ Same pattern as integration tests
- ✅ Same pattern recommended for 6 Go services
- ✅ Follows HAPI's own best practices

### Error Handling
- ✅ `ApiException` provides structured error info
- ✅ Better retry logic based on HTTP status codes
- ✅ Validation errors don't retry (optimization)

---

## 🧪 Testing Strategy

### Unit Tests
Update `tests/unit/test_audit_buffered_store.py`:
```python
from unittest.mock import patch, MagicMock

@patch('src.audit.buffered_store.AuditWriteAPIApi')
def test_write_single_event_success(mock_audit_api):
    """Verify audit events written via OpenAPI client"""
    # Arrange
    mock_response = MagicMock()
    mock_response.event_id = "evt-123"
    mock_audit_api.return_value.create_audit_event.return_value = mock_response

    # Act
    store = BufferedAuditStore("http://ds:8080", config)
    result = store._write_single_event_with_retry(event)

    # Assert
    assert result is True
    mock_audit_api.return_value.create_audit_event.assert_called_once()
```

### Integration Tests
- Existing integration tests should continue to work
- Audit events sent during integration tests will use OpenAPI client
- Verify audit pipeline E2E still functions

---

## 📊 Migration Checklist

### Implementation
- [ ] Add OpenAPI client imports
- [ ] Initialize `AuditWriteAPIApi` in `__init__`
- [ ] Convert `_write_single_event_with_retry` to use OpenAPI client
- [ ] Handle `ApiException` and validation errors
- [ ] Remove `requests` dependency

### Testing
- [ ] Update unit test mocks to use OpenAPI client
- [ ] Run unit tests: `pytest tests/unit/test_audit_buffered_store.py -v`
- [ ] Run integration tests: `make test-integration-holmesgpt`
- [ ] Verify audit events appear in Data Storage

### Verification
- [ ] No `requests.post()` calls to audit endpoints
- [ ] Audit events use typed `AuditEventRequest`
- [ ] Error handling improved with `ApiException`
- [ ] Logs show "OpenAPI audit client" messages

---

## 🎯 Success Criteria

**Phase 2b Complete When**:
- ✅ HAPI audit client uses Data Storage OpenAPI client
- ✅ All audit events sent via `AuditWriteAPIApi`
- ✅ Unit tests updated and passing
- ✅ Integration tests passing
- ✅ No `requests.post()` to audit endpoints

**Confidence Impact**: +1% (97% → 98%)
- Production code uses OpenAPI client
- Audit trail reliability improved
- Consistency with test patterns

---

## 🔗 Related Work

### Parallel Efforts
- **Phase 2**: Integration test migration (test code)
- **Phase 2b**: Audit client migration (production code) ← **THIS**
- **6 Go Services**: Need similar migration (handoff docs created)

### Dependencies
- None - can be done in parallel with Phase 2

### Blockers
- None - OpenAPI client already generated and working

---

**Created**: 2025-12-13
**Status**: 🚧 IN PROGRESS
**Estimated Time**: 30-45 minutes
**Priority**: CRITICAL (production code path)


