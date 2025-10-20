# DD-HOLMESGPT-011: Authentication Strategy for HolmesGPT API

**Date**: October 16, 2025  
**Status**: ✅ Accepted  
**Supersedes**: Custom JWT implementation in initial GREEN phase  
**Related BRs**: BR-HAPI-066, BR-HAPI-067, BR-HAPI-068, BR-HAPI-070

---

## 📋 Context

During GREEN phase implementation, custom JWT token generation and refresh logic was implemented. Upon review, this approach violates core project principles:

- **No speculative code**: Project rule mandates all code must be backed by business requirements
- **Use existing frameworks**: Avoid reinventing authentication when FastAPI provides battle-tested utilities
- **Kubernetes-native**: Kubernaut architecture prefers Kubernetes-native patterns (TokenReviewer API)

**Business Requirements**:
- **BR-HAPI-066**: API key authentication for external access
- **BR-HAPI-067**: JWT tokens for service-to-service communication
- **BR-HAPI-068**: Role-based access control (RBAC)
- **BR-HAPI-070**: Log all authentication attempts

**What's NOT Required**:
- ❌ Custom JWT refresh token implementation
- ❌ Custom token generation
- ❌ Token refresh endpoints

---

## 🎯 Decision

**Use FastAPI's built-in security utilities + Kubernetes TokenReviewer API** instead of custom JWT implementation.

### **Architecture**

```python
from fastapi import Depends, HTTPException, Security
from fastapi.security import APIKeyHeader, HTTPBearer
from typing import Optional

# API Key authentication (BR-HAPI-066)
api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)

# Bearer token authentication (BR-HAPI-067)
bearer_scheme = HTTPBearer(auto_error=False)

async def verify_api_key(api_key: Optional[str] = Security(api_key_header)) -> str:
    """Validate API key against configured keys"""
    if not api_key or api_key not in settings.get("api_keys", []):
        raise HTTPException(status_code=401, detail="Invalid API key")
    return api_key

async def verify_bearer_token(credentials: Optional[HTTPBearer] = Security(bearer_scheme)) -> dict:
    """Validate Bearer token (JWT or Kubernetes ServiceAccount)"""
    if not credentials:
        raise HTTPException(status_code=401, detail="Missing authentication")
    
    token = credentials.credentials
    
    # For Kubernetes ServiceAccount tokens, validate via TokenReviewer API
    # For JWT tokens, use standard JWT validation
    # Implementation details in middleware/auth.py
    
    return {"authenticated": True, "token": token}
```

---

## ✅ Benefits

### **1. Leverages Battle-Tested Code**
- FastAPI security utilities are used by thousands of production applications
- Well-documented, well-tested, community-supported
- Security vulnerabilities are patched by FastAPI maintainers

### **2. Kubernetes-Native for Service-to-Service**
```python
# Use Kubernetes TokenReviewer API for service authentication
from kubernetes import client as k8s_client

def validate_k8s_token(token: str) -> bool:
    """Validate ServiceAccount token via Kubernetes TokenReviewer API"""
    v1 = k8s_client.AuthenticationV1Api()
    review = k8s_client.V1TokenReview(
        spec=k8s_client.V1TokenReviewSpec(token=token)
    )
    result = v1.create_token_review(review)
    return result.status.authenticated
```

**Benefits**:
- ✅ **No secret management**: Tokens auto-mounted in pods
- ✅ **Native RBAC**: Leverages Kubernetes permissions
- ✅ **Audit trail**: All validation logged by Kubernetes API
- ✅ **Token rotation**: Handled automatically by Kubernetes

### **3. Simplifies Codebase**
**Before** (Custom Implementation):
- `~150 lines` of custom JWT code
- `generate_jwt()`, `refresh_token()`, `decode_jwt()` functions
- Custom token validation logic
- Custom error handling

**After** (FastAPI Utilities):
- `~30 lines` using FastAPI security
- Standard dependency injection
- Built-in error handling
- Standard HTTP status codes

### **4. Aligns with Project Principles**
From `00-core-development-methodology.mdc`:
> "Every code change must be backed by at least ONE business requirement"

**Custom JWT refresh**: ❌ No business requirement  
**FastAPI security utilities**: ✅ Implements BR-HAPI-066, BR-HAPI-067

---

## 📊 Comparison

| Feature | Custom JWT | FastAPI + K8s TokenReviewer |
|---------|------------|------------------------------|
| **Lines of Code** | ~150 | ~30 |
| **Security Updates** | Manual | Automatic (FastAPI) |
| **K8s Integration** | Manual | Native (TokenReviewer) |
| **RBAC** | Custom | Native (Kubernetes) |
| **Token Rotation** | Manual | Automatic (K8s) |
| **Audit Trail** | Manual | Native (K8s API) |
| **Business Requirement** | ❌ None | ✅ BR-HAPI-066, 067 |

---

## 🔧 Implementation

### **Phase 1: Middleware Update** (GREEN Phase)
```python
# holmesgpt-api/src/middleware/auth.py

from fastapi import Depends, HTTPException, Security, status
from fastapi.security import APIKeyHeader, HTTPBearer
from typing import Optional, Dict, Any
import logging

logger = logging.getLogger(__name__)

# Security schemes
api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)
bearer_scheme = HTTPBearer(auto_error=False)

async def verify_authentication(
    api_key: Optional[str] = Security(api_key_header),
    bearer: Optional[HTTPBearer] = Security(bearer_scheme),
    settings: Dict[str, Any] = None
) -> Dict[str, Any]:
    """
    Unified authentication verification (BR-HAPI-066, BR-HAPI-067)
    
    Supports:
    - API Key authentication (X-API-Key header)
    - Bearer token authentication (Authorization: Bearer <token>)
    """
    # BR-HAPI-070: Log authentication attempts
    logger.info("Authentication attempt", extra={
        "has_api_key": bool(api_key),
        "has_bearer": bool(bearer)
    })
    
    # Try API key first (BR-HAPI-066)
    if api_key:
        if api_key in settings.get("api_keys", []):
            logger.info("API key authentication successful")
            return {"auth_type": "api_key", "authenticated": True}
        else:
            logger.warning("Invalid API key attempt")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid API key"
            )
    
    # Try Bearer token (BR-HAPI-067)
    if bearer:
        token = bearer.credentials
        # For Kubernetes ServiceAccount tokens, validate via TokenReviewer
        # For other JWT tokens, use standard validation
        is_valid = await validate_token(token, settings)
        if is_valid:
            logger.info("Bearer token authentication successful")
            return {"auth_type": "bearer", "authenticated": True, "token": token}
        else:
            logger.warning("Invalid bearer token attempt")
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Invalid bearer token"
            )
    
    # No valid authentication provided
    logger.warning("No valid authentication provided")
    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Authentication required",
        headers={"WWW-Authenticate": "Bearer"}
    )

async def validate_token(token: str, settings: Dict[str, Any]) -> bool:
    """
    Validate Bearer token (JWT or Kubernetes ServiceAccount)
    
    Business Requirement: BR-HAPI-067
    """
    # Implementation will use:
    # - Kubernetes TokenReviewer API for ServiceAccount tokens
    # - Standard JWT validation for other tokens
    # Details in REFACTOR phase
    return True  # GREEN phase: minimal implementation
```

### **Phase 2: Remove Custom Code** (GREEN Phase)
**Delete**:
- `generate_jwt()` - Not backed by business requirement
- `refresh_token()` - Not backed by business requirement  
- `decode_jwt()` - Replace with FastAPI's JWT utilities if needed

**Update Tests**:
- Remove token refresh tests (no BR)
- Update auth tests to use FastAPI security patterns
- Keep logging and RBAC tests (BR-HAPI-068, BR-HAPI-070)

### **Phase 3: Kubernetes Integration** (REFACTOR Phase)
```python
# Full TokenReviewer integration
from kubernetes import client as k8s_client, config

def validate_k8s_token(token: str) -> bool:
    """
    Validate Kubernetes ServiceAccount token via TokenReviewer API
    
    Used by Kubernaut CRD controllers to authenticate to HolmesGPT API
    """
    try:
        config.load_incluster_config()
        v1 = k8s_client.AuthenticationV1Api()
        
        review = k8s_client.V1TokenReview(
            spec=k8s_client.V1TokenReviewSpec(token=token)
        )
        
        result = v1.create_token_review(review)
        return result.status.authenticated
    except Exception as e:
        logger.error(f"Token validation failed: {e}")
        return False
```

---

## 🎯 Migration Plan

### **GREEN Phase** (Now)
- ✅ Create this design decision document
- ✅ Remove custom JWT code (`generate_jwt`, `refresh_token`, `decode_jwt`)
- ✅ Update `auth.py` middleware to use FastAPI security
- ✅ Update/remove tests for custom auth
- ✅ Keep minimal `validate_token()` stub (returns True)

### **REFACTOR Phase** (Later)
- ⏸️ Implement full TokenReviewer integration for Kubernetes tokens
- ⏸️ Add JWT validation for non-Kubernetes tokens (if needed)
- ⏸️ Implement RBAC permission checking (BR-HAPI-068)
- ⏸️ Add comprehensive security tests

---

## 📚 References

### **Project Rules**
- [00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc): Business requirement mandate
- [02-technical-implementation.mdc](../../.cursor/rules/02-technical-implementation.mdc): Use existing frameworks

### **Business Requirements**
- [13_HOLMESGPT_REST_API_WRAPPER.md](../../requirements/13_HOLMESGPT_REST_API_WRAPPER.md): BR-HAPI-066, 067, 068, 070

### **Architecture Decisions**
- [004-metrics-authentication.md](./004-metrics-authentication.md): TokenReviewer pattern used in CRD controllers

### **FastAPI Documentation**
- [Security](https://fastapi.tiangolo.com/tutorial/security/)
- [OAuth2 with Password (and hashing), Bearer with JWT tokens](https://fastapi.tiangolo.com/tutorial/security/oauth2-jwt/)

---

## ✅ Acceptance Criteria

1. ✅ All authentication backed by business requirements (BR-HAPI-066, 067, 068, 070)
2. ✅ No custom JWT implementation (use FastAPI utilities)
3. ✅ Kubernetes-native for service-to-service (TokenReviewer API in REFACTOR)
4. ✅ Authentication attempts logged (BR-HAPI-070)
5. ✅ Tests validate business requirements, not implementation details

---

**Status**: ✅ **Accepted** - Implementing in GREEN phase  
**Confidence**: 95% - Battle-tested FastAPI utilities + Kubernetes-native patterns  
**Next**: Remove custom auth code, update middleware and tests


