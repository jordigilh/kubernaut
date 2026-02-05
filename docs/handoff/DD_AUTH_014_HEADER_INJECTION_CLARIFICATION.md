# DD-AUTH-014: X-Auth-Request-User Header Injection - Clarification

**Date**: 2026-01-27
**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)

---

## 🎯 **Question**

> "Why do we need `X-Auth-Request-User` header injection? If we use auth/authz middleware, this is no longer required."

**Answer**: You're correct for HAPI, but **not** for DataStorage.

---

## 🔍 **Key Difference: DataStorage vs HAPI**

### **DataStorage (Go) - Header REQUIRED** ✅

**Reason**: DataStorage handlers **explicitly check** for the header and **return 401** if missing.

**Example**: `pkg/datastorage/server/audit_export_handler.go`
```go
// Extract user from X-Auth-Request-User header (SOC2 user attribution)
user := r.Header.Get("X-Auth-Request-User")
if user == "" {
    respondWithError(w, http.StatusUnauthorized, "Missing X-Auth-Request-User header")
    return
}
```

**Why?**
- Legal hold operations require user attribution for SOC2 compliance
- Handlers depend on the header for `placed_by` / `released_by` fields
- Removing the header would break existing business logic

**Solution**: Auth middleware **must inject** the header

---

### **HAPI (Python) - Header NOT REQUIRED** ✅

**Reason**: HAPI handlers use `get_authenticated_user()` for **logging only**, not access control.

**Example**: `holmesgpt-api/src/middleware/user_context.py` (BEFORE fix)
```python
def get_authenticated_user(request: Request) -> str:
    """Extract user from X-Auth-Request-User header (optional)"""
    user = request.headers.get("X-Auth-Request-User", "unknown")
    return user  # Returns "unknown" if missing, no 401
```

**Why?**
- Handlers use it for **audit enrichment** (logging which SA called the LLM)
- **No access control** - handlers don't check the header
- If missing, logs show `"user": "unknown"` (graceful degradation)

**Solution**: Auth middleware stores validated user in `request.state.user`, and `get_authenticated_user()` reads from there directly

---

## ✅ **Resolution**

### **HAPI Changes** (Applied)

1. **Updated `user_context.py`** to read from `request.state.user` instead of header:
```python
def get_authenticated_user(request: Request) -> str:
    """Extract user from request.state (set by auth middleware)"""
    user = getattr(request.state, "user", None) or "unknown"
    return user
```

2. **Removed header injection** from `src/middleware/auth.py`:
```python
# BEFORE (redundant):
request.state.user = user
request._headers.append((b"x-auth-request-user", user.encode("utf-8")))

# AFTER (clean):
request.state.user = user  # Source of truth
```

3. **Updated documentation** (`AUTH_RESPONSES.md`) to reflect this change

---

### **DataStorage - No Changes** (Kept as-is)

DataStorage keeps header injection because handlers require it:

```go
// pkg/datastorage/server/middleware/auth.go
// Step 5: Inject X-Auth-Request-User header (for SOC2 user attribution)
r.Header.Set("X-Auth-Request-User", user)
next.ServeHTTP(w, r.WithContext(ctx))
```

---

## 📊 **Comparison Table**

| Aspect | DataStorage (Go) | HAPI (Python) |
|--------|------------------|---------------|
| **Header Injection** | ✅ Required | ❌ Not required |
| **Handler Usage** | Access control (returns 401) | Logging only (graceful fallback) |
| **User Source** | `r.Header.Get("X-Auth-Request-User")` | `request.state.user` |
| **If Missing** | 401 Unauthorized | Logs `"unknown"` |
| **Business Logic** | Legal hold requires user attribution | Audit enrichment only |

---

## 🎯 **Why the Difference?**

**DataStorage**:
- Migrated from `ose-oauth-proxy` which **always injected** the header
- Handlers were **written to depend on** the header
- Changing handlers would be a **breaking change** to business logic
- **Solution**: Middleware mimics oauth-proxy behavior

**HAPI**:
- Handlers were written with **graceful fallback** (`"unknown"`)
- User attribution is **optional** (for logging, not access control)
- **Solution**: Direct access to `request.state.user` (cleaner, no header mutation)

---

## 📝 **Lessons Learned**

1. **Header injection is a legacy pattern** from oauth-proxy migration
2. **New services should use `request.state`** directly (cleaner)
3. **Existing services may require header** if handlers check for it
4. **Always distinguish**: Access control vs audit enrichment

---

## 🔗 **Related Documentation**

- [HAPI Implementation Complete](./DD_AUTH_014_HAPI_IMPLEMENTATION_COMPLETE.md)
- [DataStorage Final Summary](./DD_AUTH_014_FINAL_SUMMARY.md)
- [HAPI user_context.py](../../holmesgpt-api/src/middleware/user_context.py)
- [DataStorage audit_export_handler.go](../../pkg/datastorage/server/audit_export_handler.go)

---

**Conclusion**: The user was correct for HAPI. Header injection is only needed when handlers **explicitly check** for it (like DataStorage). HAPI handlers use `request.state.user` for audit enrichment without requiring the header.
