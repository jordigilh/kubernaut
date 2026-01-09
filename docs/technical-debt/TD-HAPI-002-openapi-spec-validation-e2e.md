# TD-HAPI-002: OpenAPI Spec Validation in E2E Tests

**Created**: December 30, 2025  
**Priority**: High (V1.0 implementation)  
**Effort**: 1-2 days  
**Owner**: HAPI Team  
**Status**: Approved for next branch  

---

## 🎯 PROBLEM STATEMENT

### Current State (V1.0 - Missing Validation)

**Issue**: HAPI E2E tests don't validate responses against the OpenAPI specification.

**Example Bug This Would Have Caught**:
```python
# Bug in recovery/endpoint.py (now fixed)
result = await analyze_recovery(request_data)  # Returns dict
logger.info(f"needs_human_review={result.needs_human_review}")  # AttributeError!
# ❌ HTTP 500 - dict doesn't have attribute access

# OpenAPI spec says:
# response_model=RecoveryResponse (Pydantic model)

# If we had spec validation, test would fail:
# ❌ Response type mismatch: dict != RecoveryResponse
```

### Problems with Current Testing

1. **No Response Schema Validation**
   - Tests check status codes (200, 500)
   - Tests check presence of fields (`assert "incident_id" in response`)
   - Tests DON'T validate response matches OpenAPI schema

2. **Type Mismatches Go Undetected**
   - Dict vs Pydantic model mismatches
   - Missing required fields
   - Wrong field types (string vs int)
   - Extra fields not in spec

3. **OpenAPI Spec Drift**
   - Spec says `needs_human_review: bool`
   - Implementation might return `string` or omit field
   - Tests pass because they don't validate schema

4. **Manual Testing Required**
   - Developers must manually verify OpenAPI spec
   - Integration issues found late (in AIAnalysis tests)
   - Production risks from schema violations

---

## ✅ IDEAL STATE (V1.0 Next Branch)

### OpenAPI-Validated E2E Tests

```python
# File: holmesgpt-api/tests/e2e/test_openapi_compliance.py

import pytest
from openapi_core import Spec
from openapi_core.validation.request import openapi_request_validator
from openapi_core.validation.response import openapi_response_validator

class TestOpenAPICompliance:
    """Validate all HAPI endpoints against OpenAPI specification"""
    
    @pytest.fixture(scope="class")
    def openapi_spec(self):
        """Load OpenAPI spec for validation"""
        import json
        with open("api/openapi.json") as f:
            spec_dict = json.load(f)
        return Spec.from_dict(spec_dict)
    
    def test_incident_endpoint_response_matches_spec(self, hapi_client, openapi_spec):
        """BR-HAPI-002: Incident response must match OpenAPI schema"""
        request_data = {
            "incident_id": "test-001",
            "remediation_id": "test-remediation-001",
            "signal_context": {
                "signal_type": "CrashLoopBackOff",
                "severity": "high"
            }
        }
        
        response = hapi_client.post("/api/v1/incident/analyze", json=request_data)
        
        # Validate response matches OpenAPI spec
        result = openapi_response_validator.validate(
            spec=openapi_spec,
            response=response
        )
        
        # This will FAIL if:
        # - Response type is dict instead of IncidentResponse
        # - Missing required fields (incident_id, selected_workflow, etc.)
        # - Wrong field types (confidence: str instead of float)
        # - Extra fields not in spec
        assert result.errors == [], f"OpenAPI validation errors: {result.errors}"
        
        # Additional type checks
        data = response.json()
        assert isinstance(data["needs_human_review"], bool), "needs_human_review must be bool"
        assert isinstance(data["analysis_confidence"], float), "confidence must be float"
        assert 0.0 <= data["analysis_confidence"] <= 1.0, "confidence must be 0.0-1.0"
    
    def test_recovery_endpoint_response_matches_spec(self, hapi_client, openapi_spec):
        """BR-HAPI-002: Recovery response must match OpenAPI schema"""
        request_data = {
            "incident_id": "test-001",
            "remediation_id": "test-remediation-001",
            "signal_type": "MOCK_NO_WORKFLOW_FOUND",
            "is_recovery_attempt": True,
            "recovery_attempt_number": 1
        }
        
        response = hapi_client.post("/api/v1/recovery/analyze", json=request_data)
        
        # Validate response matches OpenAPI spec
        result = openapi_response_validator.validate(
            spec=openapi_spec,
            response=response
        )
        
        assert result.errors == [], f"OpenAPI validation errors: {result.errors}"
        
        # This test would have caught the dict vs model bug!
        data = response.json()
        assert isinstance(data["needs_human_review"], bool), "needs_human_review must be bool"
        assert isinstance(data["can_recover"], bool), "can_recover must be bool"
        assert isinstance(data["analysis_confidence"], float), "confidence must be float"
    
    def test_health_endpoint_matches_spec(self, hapi_client, openapi_spec):
        """Health endpoint must match OpenAPI schema"""
        response = hapi_client.get("/health/ready")
        
        result = openapi_response_validator.validate(
            spec=openapi_spec,
            response=response
        )
        
        assert result.errors == [], f"OpenAPI validation errors: {result.errors}"
    
    @pytest.mark.parametrize("endpoint,method", [
        ("/api/v1/incident/analyze", "POST"),
        ("/api/v1/recovery/analyze", "POST"),
        ("/health/ready", "GET"),
        ("/health/live", "GET"),
    ])
    def test_all_endpoints_match_openapi_spec(
        self,
        hapi_client,
        openapi_spec,
        endpoint,
        method
    ):
        """Comprehensive OpenAPI validation for all endpoints"""
        # Generate valid request data for endpoint
        request_data = self._generate_valid_request(endpoint)
        
        # Make request
        if method == "POST":
            response = hapi_client.post(endpoint, json=request_data)
        else:
            response = hapi_client.get(endpoint)
        
        # Validate against OpenAPI spec
        result = openapi_response_validator.validate(
            spec=openapi_spec,
            response=response
        )
        
        assert result.errors == [], (
            f"OpenAPI validation failed for {method} {endpoint}: "
            f"{result.errors}"
        )
```

---

## 📋 IMPLEMENTATION PLAN

### Phase 1: Add OpenAPI Validation Library

**Dependencies**:
```python
# holmesgpt-api/requirements.txt
openapi-core>=0.18.0        # OpenAPI 3.x validation
openapi-spec-validator>=0.7.0  # Schema validation
jsonschema>=4.20.0          # JSON schema validation
```

**Purpose**:
- Validate requests match OpenAPI request schemas
- Validate responses match OpenAPI response schemas
- Detect type mismatches (dict vs model, string vs int)
- Catch missing required fields

### Phase 2: Create OpenAPI Compliance Test Suite

**Location**: `holmesgpt-api/tests/e2e/test_openapi_compliance.py`

**Test Structure**:
```python
class TestOpenAPICompliance:
    # Fixture: Load OpenAPI spec
    # Test 1: Incident endpoint response validation
    # Test 2: Recovery endpoint response validation  
    # Test 3: Health endpoint response validation
    # Test 4: Parametrized test for all endpoints
    # Test 5: Request validation (invalid requests rejected)
    # Test 6: Error response validation (HTTP 400, 500)
```

### Phase 3: Add Schema Validation Helpers

**Location**: `holmesgpt-api/tests/utils/openapi_validator.py`

```python
# File: tests/utils/openapi_validator.py

from typing import Dict, Any, List
from openapi_core import Spec
from openapi_core.validation.response import openapi_response_validator

class HAPIOpenAPIValidator:
    """Utility for validating HAPI responses against OpenAPI spec"""
    
    def __init__(self, spec_path: str = "api/openapi.json"):
        import json
        with open(spec_path) as f:
            spec_dict = json.load(f)
        self.spec = Spec.from_dict(spec_dict)
    
    def validate_response(
        self,
        response,
        endpoint: str,
        method: str = "POST"
    ) -> List[str]:
        """
        Validate response against OpenAPI spec
        
        Returns:
            List of validation errors (empty if valid)
        """
        result = openapi_response_validator.validate(
            spec=self.spec,
            response=response
        )
        return [str(e) for e in result.errors]
    
    def assert_valid_response(
        self,
        response,
        endpoint: str,
        method: str = "POST"
    ):
        """Assert response is valid per OpenAPI spec"""
        errors = self.validate_response(response, endpoint, method)
        assert not errors, (
            f"OpenAPI validation failed for {method} {endpoint}:\n"
            f"{chr(10).join(errors)}"
        )
    
    def validate_field_types(self, response_data: Dict[str, Any]) -> List[str]:
        """
        Validate field types match expected types
        
        Catches issues like:
        - needs_human_review: string instead of bool
        - analysis_confidence: string instead of float
        """
        errors = []
        
        # Validate boolean fields
        for field in ["needs_human_review", "can_recover"]:
            if field in response_data:
                if not isinstance(response_data[field], bool):
                    errors.append(
                        f"{field} must be bool, got {type(response_data[field])}"
                    )
        
        # Validate float fields
        for field in ["analysis_confidence", "confidence"]:
            if field in response_data:
                if not isinstance(response_data[field], (int, float)):
                    errors.append(
                        f"{field} must be float, got {type(response_data[field])}"
                    )
                elif not 0.0 <= response_data[field] <= 1.0:
                    errors.append(
                        f"{field} must be 0.0-1.0, got {response_data[field]}"
                    )
        
        return errors
```

### Phase 4: Integrate with Existing E2E Tests

**Update existing tests**:
```python
# File: holmesgpt-api/tests/e2e/test_mock_llm_edge_cases_e2e.py

from tests.utils.openapi_validator import HAPIOpenAPIValidator

class TestRecoveryEdgeCases:
    @pytest.fixture(scope="class")
    def openapi_validator(self):
        return HAPIOpenAPIValidator()
    
    def test_no_recovery_workflow_returns_human_review(
        self,
        hapi_recovery_api,
        openapi_validator  # Add validator
    ):
        request_data = make_recovery_request("MOCK_NO_WORKFLOW_FOUND")
        recovery_request = RecoveryRequest(**request_data)
        
        response = hapi_recovery_api.recovery_analyze_endpoint_api_v1_recovery_analyze_post(
            recovery_request=recovery_request,
            _request_timeout=30
        )
        
        # Validate against OpenAPI spec
        openapi_validator.assert_valid_response(
            response,
            "/api/v1/recovery/analyze",
            "POST"
        )
        
        # Original assertions still work
        data = response.model_dump()
        assert data["needs_human_review"] is True
        # ...
```

### Phase 5: Add OpenAPI Spec Drift Detection

**Pre-commit Hook**:
```bash
#!/bin/bash
# .git/hooks/pre-commit

# Regenerate OpenAPI spec and check for changes
cd holmesgpt-api
python3 -c "
from src.main import app
import json

# Export OpenAPI spec
spec = app.openapi()
with open('api/openapi.json', 'w') as f:
    json.dump(spec, f, indent=2)
"

# Check if spec changed without corresponding code changes
if git diff --cached --name-only | grep -q "src/models/\|src/extensions/"; then
    if git diff api/openapi.json | grep -q "^+"; then
        echo "⚠️  OpenAPI spec changed - ensure it's intentional"
        echo "Review changes: git diff api/openapi.json"
    fi
fi
```

---

## 🎯 ACCEPTANCE CRITERIA

### Must Have (V1.0 Next Branch)

- [ ] OpenAPI validation library installed (`openapi-core`)
- [ ] `test_openapi_compliance.py` test suite created
- [ ] OpenAPI validator utility (`openapi_validator.py`) implemented
- [ ] All HAPI endpoints validated against OpenAPI spec:
  - [ ] `/api/v1/incident/analyze` (POST)
  - [ ] `/api/v1/recovery/analyze` (POST)
  - [ ] `/health/ready` (GET)
  - [ ] `/health/live` (GET)
- [ ] Type validation for all response fields
- [ ] Required field validation
- [ ] Invalid field rejection (extra fields not in spec)
- [ ] All existing E2E tests still passing
- [ ] New OpenAPI tests integrated into CI/CD

### Nice to Have (Future)

- [ ] Request schema validation (reject invalid requests)
- [ ] Error response schema validation (HTTP 400, 500)
- [ ] OpenAPI spec drift detection in pre-commit
- [ ] Automated spec regeneration on model changes
- [ ] Performance impact analysis (validation overhead)

---

## 🧪 TESTING STRATEGY

### Unit Tests (OpenAPI Validator)
- Test validator correctly identifies type mismatches
- Test validator catches missing required fields
- Test validator allows valid responses
- Test validator rejects invalid responses

### Integration Tests
- Validate OpenAPI spec itself is valid
- Validate all endpoints are documented in spec
- Validate all models are in spec

### E2E Tests (The Main Goal)
- Every endpoint response validated against spec
- All edge cases (mock scenarios) validated
- Happy path and error path validation

---

## 📊 BUGS THIS WOULD HAVE CAUGHT

### Bug 1: Dict vs Pydantic Model (Current Branch)
```python
# Bug: recovery/endpoint.py returned dict instead of RecoveryResponse
result = await analyze_recovery(...)  # Returns dict
return result  # FastAPI expects RecoveryResponse

# OpenAPI validator would detect:
# ❌ Response type mismatch: dict != RecoveryResponse
# ❌ Cannot access attributes on dict
```

### Bug 2: Missing needs_human_review Field (Previous)
```python
# Bug: OpenAPI spec missing needs_human_review
# RecoveryResponse model has field, but spec doesn't

# OpenAPI validator would detect:
# ❌ Field 'needs_human_review' in response but not in spec
# ❌ Spec drift: model != spec
```

### Bug 3: Wrong Field Type
```python
# Bug: analysis_confidence returned as string instead of float
{
    "analysis_confidence": "0.85"  # String!
}

# OpenAPI validator would detect:
# ❌ Field 'analysis_confidence' type mismatch: string != float
```

### Bug 4: Missing Required Field
```python
# Bug: incident_id omitted from response
{
    # "incident_id": "...",  # Missing!
    "can_recover": true
}

# OpenAPI validator would detect:
# ❌ Required field 'incident_id' missing from response
```

---

## 🔒 RISKS & MITIGATION

### Risk 1: Performance Impact
**Concern**: OpenAPI validation adds latency to tests

**Mitigation**:
- Validation only in E2E tests (not production)
- Cache parsed OpenAPI spec (parse once, reuse)
- Run validation in parallel where possible
- Expected overhead: <100ms per test

### Risk 2: False Positives
**Concern**: Validator rejects valid responses

**Mitigation**:
- Use well-maintained library (`openapi-core`)
- Test validator itself with known-good responses
- Provide clear error messages for debugging
- Allow validator configuration if needed

### Risk 3: Spec Drift
**Concern**: Model changes don't update OpenAPI spec

**Mitigation**:
- OpenAPI spec generated from Pydantic models (automatic)
- Pre-commit hook to detect spec changes
- CI/CD validation of spec vs implementation
- Document spec update process

---

## 📚 RELATED DOCUMENTS

- [Bug Report](../../docs/shared/HAPI_RECOVERY_DICT_VS_MODEL_BUG.md) - Dict vs model bug this would catch
- [BR-HAPI-002](../../docs/requirements/BR-HAPI-002-response-schemas.md) - Response schema requirements
- [DD-API-001](../../docs/architecture/decisions/DD-API-001-openapi-client-mandate.md) - OpenAPI client usage
- [TESTING_GUIDELINES.md](../../TESTING_GUIDELINES.md) - Testing strategy

---

## 💡 IMPLEMENTATION TIPS

### 1. Start with One Endpoint
```python
# Start simple - validate one endpoint first
def test_incident_endpoint_basic_validation(hapi_client):
    response = hapi_client.post("/api/v1/incident/analyze", json={...})
    
    # Basic validation
    assert response.status_code == 200
    data = response.json()
    
    # Type validation
    assert isinstance(data["needs_human_review"], bool)
    assert isinstance(data["analysis_confidence"], float)
```

### 2. Incrementally Add Validation
```python
# Then add OpenAPI validator
from tests.utils.openapi_validator import HAPIOpenAPIValidator

def test_incident_endpoint_openapi_validation(hapi_client, openapi_validator):
    response = hapi_client.post("/api/v1/incident/analyze", json={...})
    
    # OpenAPI validation
    openapi_validator.assert_valid_response(response, "/api/v1/incident/analyze")
```

### 3. Extend to All Endpoints
```python
# Finally, parametrize for all endpoints
@pytest.mark.parametrize("endpoint,method,request_data", [
    ("/api/v1/incident/analyze", "POST", {...}),
    ("/api/v1/recovery/analyze", "POST", {...}),
    ("/health/ready", "GET", None),
])
def test_all_endpoints_openapi_compliant(
    hapi_client, openapi_validator, endpoint, method, request_data
):
    # ...
```

---

## ✅ SUCCESS METRICS

### Before Implementation (Current)
- OpenAPI validation coverage: 0%
- Type mismatch bugs: Detected late (integration tests)
- Response schema issues: Detected in production
- Developer confidence: Low (manual verification)

### After Implementation (Target)
- OpenAPI validation coverage: 100% (all endpoints)
- Type mismatch bugs: Detected immediately (E2E tests)
- Response schema issues: Prevented before merge
- Developer confidence: High (automated validation)

---

## 🚀 IMPLEMENTATION TIMELINE

### Sprint 1 (Day 1): Foundation
- Install `openapi-core` dependency
- Create `openapi_validator.py` utility
- Write unit tests for validator

### Sprint 2 (Day 2): E2E Integration
- Create `test_openapi_compliance.py`
- Implement incident endpoint validation
- Implement recovery endpoint validation
- Implement health endpoint validation

### Sprint 3 (Day 3-4): Complete Coverage
- Integrate validator with existing E2E tests
- Add parametrized tests for all endpoints
- Verify all tests pass
- Update CI/CD pipeline

### Sprint 4 (Day 5): Optional Enhancements
- Pre-commit hook for spec drift detection
- Request validation (invalid requests)
- Error response validation

---

## ✅ SIGN-OFF

**Technical Debt Owner**: HAPI Team  
**Approved By**: Technical Lead  
**Implementation Branch**: `feature/openapi-validation` (next after current branch merge)  
**Target Release**: V1.0  

---

**Status**: 📋 **Documented** - Ready for implementation in next branch








