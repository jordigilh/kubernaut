# TRIAGE: HAPI E2E Tests and OpenAPI Client Gaps

**Date**: 2025-12-13
**Priority**: 🚨 **HIGH** - Critical Testing Gap
**Status**: 📋 TRIAGED - Ready for Implementation

---

## 🎯 Executive Summary

**Critical Gaps Identified**:
1. ❌ HAPI has **NO standalone E2E tests** for recovery endpoint
2. ❌ Integration tests use `requests` library instead of OpenAPI-generated client
3. ❌ No automated validation that OpenAPI spec matches runtime behavior

**Impact**:
- AA team caught recovery endpoint bug, not HAPI tests
- Integration tests don't validate OpenAPI contract compliance
- Breaking changes to API can go undetected

---

## 📊 Current Test Status

### Unit Tests: 560/575 (97%) ✅
**Coverage**: Business logic, mocking, transformations
**Limitation**: Don't validate real HTTP/OpenAPI contract

### Integration Tests: 9 files ⚠️
**Status**: Using `requests.post()` instead of OpenAPI client
**Files**:
1. `test_recovery_dd003_integration.py` - Recovery endpoint
2. `test_custom_labels_integration_dd_hapi_001.py` - Custom labels
3. `test_mock_llm_mode_integration.py` - Mock LLM mode (13 tests)
4. `test_workflow_catalog_data_storage.py` - Data Storage integration
5. `test_workflow_catalog_data_storage_integration.py` - Workflow catalog
6. `conftest.py` - Fixtures

**Compliance**: ✅ Use real services (podman-compose)
**Gap**: ❌ Don't use OpenAPI client

### E2E Tests: 9 files ⚠️
**Status**: Exist but unclear if they validate OpenAPI contracts
**Files**:
1. `test_audit_pipeline_e2e.py`
2. `test_mock_llm_edge_cases_e2e.py`
3. `test_real_llm_integration.py`
4. `test_workflow_catalog_container_image_integration.py`
5. `test_workflow_catalog_data_storage_integration.py`
6. `test_workflow_catalog_e2e.py`
7. `test_workflow_selection_e2e.py`
8. `conftest.py`
9. `TEST_PLAN_WORKFLOW_CATALOG_EDGE_CASES.md`

**Gap**: ❌ No dedicated recovery endpoint E2E tests

---

## 🔍 Detailed Analysis

### Gap 1: No HAPI OpenAPI Client for Integration Tests

**Current State**:
```python
# tests/integration/test_recovery_dd003_integration.py
response = requests.post(
    f"{hapi_service_url}/api/v1/recovery/analyze",
    json=sample_recovery_request_with_previous_execution
)
```

**Problem**:
- Doesn't validate OpenAPI spec compliance
- Manual JSON construction (error-prone)
- No type safety
- Doesn't catch spec/code mismatches

**Should Be**:
```python
# Using OpenAPI-generated client
from holmesgpt_api_client import ApiClient, RecoveryApi
from holmesgpt_api_client.models import RecoveryRequest

client = ApiClient(configuration=Configuration(host=hapi_service_url))
recovery_api = RecoveryApi(client)

response = recovery_api.recovery_analyze_endpoint(
    recovery_request=RecoveryRequest(
        remediation_id="test-001",
        signal_type="OOMKilled",
        # ... typed fields
    )
)
```

**Benefits**:
- ✅ Type-safe API calls
- ✅ Validates OpenAPI spec at runtime
- ✅ Catches spec/code mismatches immediately
- ✅ Same client AA team uses (Go equivalent)

### Gap 2: No HAPI OpenAPI Client Generation

**Current State**: ❌ No HAPI Python client exists

**Need**:
1. Generate Python client from HAPI's OpenAPI spec
2. Use in integration and E2E tests
3. Validate spec matches runtime behavior

**Implementation**:
```bash
# holmesgpt-api/scripts/generate-hapi-client.sh
#!/bin/bash
set -e

OPENAPI_SPEC="/local/holmesgpt-api/api/openapi.json"
OUTPUT_DIR="/local/holmesgpt-api/tests/clients/holmesgpt_api_client"

echo "🔧 Generating Python client for HAPI from ${OPENAPI_SPEC}..."

podman run --rm -v ${PWD}:/local:z openapitools/openapi-generator-cli generate \
  -i "${OPENAPI_SPEC}" \
  -g python \
  -o "${OUTPUT_DIR}" \
  --package-name "holmesgpt_api_client" \
  --additional-properties=packageVersion=1.0.0

echo "✅ HAPI Python client generated"
```

### Gap 3: No Recovery Endpoint E2E Tests

**Current E2E Tests**: Focus on workflow catalog, audit pipeline, mock LLM
**Missing**: Recovery endpoint end-to-end validation

**Need**:
```python
# tests/e2e/test_recovery_endpoint_e2e.py
"""
E2E tests for recovery endpoint validating:
1. OpenAPI spec compliance
2. selected_workflow field presence
3. recovery_analysis field presence
4. Integration with Data Storage
5. Mock LLM mode behavior
"""

@pytest.mark.e2e
class TestRecoveryEndpointE2E:
    """
    E2E tests using OpenAPI-generated client to validate:
    - API contract compliance
    - Field presence and types
    - End-to-end recovery flow
    """

    def test_recovery_endpoint_returns_selected_workflow_e2e(
        self, hapi_client, data_storage_client
    ):
        """
        E2E: Recovery endpoint returns selected_workflow via OpenAPI client

        This test would have caught the missing field bug!
        """
        # Arrange: Create incident
        incident_response = hapi_client.incident_api.incident_analyze_endpoint(...)

        # Act: Request recovery
        recovery_response = hapi_client.recovery_api.recovery_analyze_endpoint(
            recovery_request=RecoveryRequest(
                remediation_id="e2e-001",
                previous_workflow_id=incident_response.selected_workflow.workflow_id,
                signal_type="OOMKilled",
                namespace="production"
            )
        )

        # Assert: Validate OpenAPI contract
        assert recovery_response.selected_workflow is not None, \
            "E2E: selected_workflow must be present (BR-AI-080)"
        assert recovery_response.recovery_analysis is not None, \
            "E2E: recovery_analysis must be present (BR-AI-081)"
        assert recovery_response.selected_workflow.workflow_id is not None
```

---

## 📋 Implementation Plan

### Phase 1: Generate HAPI OpenAPI Client (2-3 hours)

**Priority**: 🚨 HIGH
**Blocking**: Integration test migration

#### Tasks:
1. ✅ Regenerate HAPI OpenAPI spec (DONE)
2. Create client generation script
3. Generate Python client from HAPI spec
4. Fix import paths (like DS client)
5. Add to test dependencies

#### Deliverables:
- `holmesgpt-api/scripts/generate-hapi-client.sh`
- `holmesgpt-api/tests/clients/holmesgpt_api_client/` (generated)
- Updated `requirements-test.txt`

#### Success Criteria:
```python
from holmesgpt_api_client import ApiClient, RecoveryApi
from holmesgpt_api_client.models import RecoveryResponse

# Can instantiate and use
client = ApiClient(configuration=Configuration(host="http://localhost:18120"))
recovery_api = RecoveryApi(client)
```

### Phase 2: Migrate Integration Tests (4-6 hours)

**Priority**: 🚨 HIGH
**Depends On**: Phase 1

#### Files to Migrate (6 files):
1. `test_recovery_dd003_integration.py` (3 tests)
2. `test_custom_labels_integration_dd_hapi_001.py` (8 tests)
3. `test_mock_llm_mode_integration.py` (13 tests)
4. `test_workflow_catalog_data_storage.py` (varies)
5. `test_workflow_catalog_data_storage_integration.py` (varies)
6. `conftest.py` (fixtures)

#### Migration Pattern:
```python
# BEFORE (requests)
response = requests.post(
    f"{hapi_service_url}/api/v1/recovery/analyze",
    json={"remediation_id": "test-001", ...}
)
data = response.json()

# AFTER (OpenAPI client)
from holmesgpt_api_client import ApiClient, RecoveryApi
from holmesgpt_api_client.models import RecoveryRequest

client = ApiClient(configuration=Configuration(host=hapi_service_url))
recovery_api = RecoveryApi(client)

response = recovery_api.recovery_analyze_endpoint(
    recovery_request=RecoveryRequest(
        remediation_id="test-001",
        ...
    )
)
# response is typed RecoveryResponse object
```

#### Success Criteria:
- All integration tests use OpenAPI client
- No `requests.post()` calls to HAPI endpoints
- Tests validate OpenAPI spec compliance
- All integration tests pass

### Phase 3: Create Recovery Endpoint E2E Tests (3-4 hours)

**Priority**: 🔴 MEDIUM-HIGH
**Depends On**: Phase 2

#### New Test File:
`tests/e2e/test_recovery_endpoint_e2e.py`

#### Test Cases (8 tests):
1. **Happy Path**: Recovery returns selected_workflow and recovery_analysis
2. **Field Validation**: All required fields present and typed correctly
3. **Previous Execution**: Context properly passed and processed
4. **Detected Labels**: Labels correctly included in recovery analysis
5. **Mock LLM Mode**: Mock responses match OpenAPI spec
6. **Error Handling**: API errors properly formatted
7. **Integration**: Recovery integrates with Data Storage
8. **Workflow Selection**: Selected workflow is valid and executable

#### Success Criteria:
- E2E tests use OpenAPI client
- Tests validate full recovery flow
- Tests catch spec/code mismatches
- **This test suite would have caught the missing fields bug**

### Phase 4: Automated Spec Validation (2-3 hours)

**Priority**: 🔴 MEDIUM
**Prevents**: Future spec/code mismatches

#### Script: `scripts/validate-openapi-spec.py`
```python
#!/usr/bin/env python3
"""
Validate HAPI OpenAPI spec matches Pydantic models

Run before commit to catch spec/code mismatches
"""
import json
from src.main import app
from src.models.recovery_models import RecoveryResponse
from pydantic import BaseModel

def validate_spec():
    # Generate spec from app
    spec = app.openapi()

    # Get RecoveryResponse schema
    recovery_schema = spec['components']['schemas']['RecoveryResponse']

    # Get Pydantic model fields
    model_fields = RecoveryResponse.model_fields.keys()
    spec_fields = recovery_schema['properties'].keys()

    # Check for missing fields
    missing_in_spec = set(model_fields) - set(spec_fields)
    missing_in_model = set(spec_fields) - set(model_fields)

    if missing_in_spec:
        print(f"❌ Fields in model but not in spec: {missing_in_spec}")
        return False

    if missing_in_model:
        print(f"⚠️  Fields in spec but not in model: {missing_in_model}")

    print("✅ OpenAPI spec matches Pydantic models")
    return True

if __name__ == "__main__":
    import sys
    sys.exit(0 if validate_spec() else 1)
```

#### Integration:
```bash
# .git/hooks/pre-commit
#!/bin/bash
python3 scripts/validate-openapi-spec.py || exit 1
```

#### Success Criteria:
- Pre-commit hook validates spec
- Catches missing fields before commit
- Prevents spec/code drift

---

## 📊 Estimated Effort

| Phase | Priority | Effort | Dependencies |
|-------|----------|--------|--------------|
| **Phase 1: Generate Client** | 🚨 HIGH | 2-3 hours | None |
| **Phase 2: Migrate Integration** | 🚨 HIGH | 4-6 hours | Phase 1 |
| **Phase 3: E2E Tests** | 🔴 MEDIUM-HIGH | 3-4 hours | Phase 2 |
| **Phase 4: Spec Validation** | 🔴 MEDIUM | 2-3 hours | None |
| **Total** | | **11-16 hours** | |

**Timeline**: 2-3 days (with testing and validation)

---

## 🎯 Success Metrics

### Immediate (Phase 1-2)
- ✅ HAPI Python OpenAPI client generated
- ✅ All integration tests use OpenAPI client
- ✅ No `requests.post()` to HAPI endpoints
- ✅ Integration tests pass

### Short Term (Phase 3)
- ✅ Recovery endpoint E2E tests created
- ✅ E2E tests validate OpenAPI contract
- ✅ **E2E tests would catch missing field bugs**

### Long Term (Phase 4)
- ✅ Pre-commit hook validates spec
- ✅ Spec/code drift prevented
- ✅ Consumer teams notified of spec changes

---

## 🚀 Quick Start

### Step 1: Generate HAPI Client (NOW)
```bash
cd holmesgpt-api
./scripts/generate-hapi-client.sh
```

### Step 2: Update One Integration Test (Proof of Concept)
```bash
# Migrate test_recovery_dd003_integration.py
# Validate it works
pytest tests/integration/test_recovery_dd003_integration.py -v
```

### Step 3: Migrate Remaining Tests
```bash
# Batch migrate all integration tests
# Run full integration suite
make test-integration-holmesgpt
```

### Step 4: Create E2E Tests
```bash
# Create tests/e2e/test_recovery_endpoint_e2e.py
# Run E2E suite
make test-e2e-holmesgpt
```

---

## 📞 Team Coordination

### HAPI Team Actions
1. Generate HAPI OpenAPI client
2. Migrate integration tests
3. Create recovery E2E tests
4. Add spec validation to CI/CD

### AA Team Impact
- ✅ Already using Go client (no changes needed)
- ℹ️ Will benefit from improved HAPI test coverage
- ℹ️ Fewer bugs reaching AA team E2E tests

### DS Team Impact
- ℹ️ No impact (DS client already uses OpenAPI)
- ℹ️ Can adopt similar E2E test pattern

---

## 🎓 Lessons Learned

1. **OpenAPI clients are not optional** - They validate contracts at runtime
2. **Integration tests must use real clients** - Not raw HTTP
3. **E2E tests catch what integration tests miss** - Defense in depth
4. **Spec validation must be automated** - Manual processes fail
5. **Consumer teams are your best testers** - AA team caught this bug

---

## ✅ Acceptance Criteria

**Phase 1 Complete When**:
- [ ] HAPI Python client generated
- [ ] Client can be imported and instantiated
- [ ] Client has RecoveryApi with recovery_analyze_endpoint

**Phase 2 Complete When**:
- [ ] All 6 integration test files migrated
- [ ] No `requests.post()` to HAPI endpoints
- [ ] All integration tests pass
- [ ] Tests validate OpenAPI types

**Phase 3 Complete When**:
- [ ] Recovery E2E test file created
- [ ] 8 E2E test cases implemented
- [ ] Tests use OpenAPI client
- [ ] Tests validate selected_workflow and recovery_analysis fields

**Phase 4 Complete When**:
- [ ] Spec validation script created
- [ ] Pre-commit hook added
- [ ] Validation runs in CI/CD
- [ ] Documentation updated

---

**Created**: 2025-12-13
**Priority**: HIGH
**Status**: TRIAGED - Ready for Implementation
**Estimated Effort**: 11-16 hours (2-3 days)

---

**RECOMMENDATION**: Start with Phase 1 immediately to unblock Phase 2


