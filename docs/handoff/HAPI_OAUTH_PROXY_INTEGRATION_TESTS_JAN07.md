# HolmesGPT API OAuth-Proxy Integration Tests

**Date**: January 7, 2026  
**Context**: DD-AUTH-006 Implementation  
**Status**: Integration Tests Work Without Changes

---

## 📋 **Summary**

HolmesGPT API integration tests **require NO changes** after oauth-proxy implementation.

**Reason**: The `get_authenticated_user()` function gracefully handles missing headers by returning "unknown" and logging a warning. Integration tests run without oauth-proxy, so the header is missing, but tests continue to work.

---

## 🔍 **Current Behavior**

### **Integration Test Environment**
- Tests run against HAPI directly (no oauth-proxy sidecar)
- Requests DO NOT include `X-Auth-Request-User` header
- `get_authenticated_user()` returns "unknown"
- Warning logged: "OAuth-proxy should inject this header after authentication"

### **Example Log Output (Integration Tests)**
```json
{
  "event": "missing_user_header",
  "header": "X-Auth-Request-User",
  "path": "/api/v1/incident/analyze",
  "note": "OAuth-proxy should inject this header after authentication"
}
{
  "event": "incident_analysis_requested",
  "user": "unknown",
  "endpoint": "/incident/analyze",
  "purpose": "LLM cost tracking and audit trail"
}
```

### **Production Behavior**
- Requests go through oauth-proxy sidecar
- OAuth-proxy injects `X-Auth-Request-User: system:serviceaccount:kubernaut-system:gateway-sa`
- `get_authenticated_user()` returns actual user identity
- No warning logged

---

## ✅ **Why No Changes Needed**

1. **Graceful Degradation**: Code handles missing header without failing
2. **Test Focus**: Integration tests validate business logic, not auth
3. **Auth is External**: OAuth-proxy handles auth; Python code just logs user
4. **Backward Compatible**: Old tests continue to work

---

## 🔧 **Optional Enhancement (Future)**

If you want integration tests to inject mock headers (like DataStorage tests):

### **Option 1: Add Header to Requests** (Recommended)
```python
# In test file
def test_incident_analysis(hapi_url):
    headers = {"X-Auth-Request-User": "test-service@integration.test"}
    response = requests.post(
        f"{hapi_url}/api/v1/incident/analyze",
        json=incident_data,
        headers=headers  # Mock oauth-proxy header
    )
```

### **Option 2: Create Fixture** (If many tests need it)
```python
# In conftest.py
@pytest.fixture
def integration_headers():
    """Mock oauth-proxy headers for integration tests"""
    return {"X-Auth-Request-User": "test-service@integration.test"}

# In test file
def test_incident_analysis(hapi_url, integration_headers):
    response = requests.post(
        f"{hapi_url}/api/v1/incident/analyze",
        json=incident_data,
        headers=integration_headers
    )
```

---

## 🎯 **Recommendation**

**Do Nothing** - Integration tests work fine as-is.

**Reason**:
- Tests validate business logic (incident analysis, recovery suggestions)
- User attribution is for logging/audit, not business functionality
- Adding mock headers is low-priority enhancement

**When to Add Mock Headers**:
- If you need to test user-specific behavior
- If you want cleaner logs (no "unknown" warnings)
- If you implement user-based rate limiting or quotas

---

## 📚 **Related Documentation**

- **DD-AUTH-006**: HolmesGPT API oauth-proxy integration
- **DD-AUTH-004**: DataStorage oauth-proxy pattern
- **DD-AUTH-005**: Client authentication pattern (8 services)
- **user_context.py**: User extraction implementation

---

## ✅ **Phase 9.6 Status**

**Status**: COMPLETE (no changes needed)  
**Impact**: Integration tests continue to work without modifications  
**Optional Enhancement**: Add mock header fixture (low priority)

