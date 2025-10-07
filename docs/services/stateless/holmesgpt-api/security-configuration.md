# HolmesGPT API Service - Security Configuration

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP Service (Python REST API)
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Security Overview](#security-overview)
2. [Authentication](#authentication)
3. [Authorization](#authorization)
4. [LLM API Security](#llm-api-security)
5. [Network Security](#network-security)
6. [Secrets Management](#secrets-management)
7. [Audit Logging](#audit-logging)
8. [Security Checklist](#security-checklist)

---

## Security Overview

### **Security Principles**

HolmesGPT API Service implements defense-in-depth security for AI-powered investigations:

1. **Authentication**: Kubernetes TokenReviewer for service identity validation
2. **Authorization**: RBAC for investigation request permission
3. **LLM API Security**: API keys stored in Kubernetes secrets
4. **Network Security**: Network policies, TLS encryption
5. **Audit Trail**: Comprehensive logging of all investigation requests

### **Threat Model**

| Threat | Mitigation |
|--------|------------|
| **Unauthorized Investigations** | TokenReviewer authentication + RBAC |
| **LLM API Key Exposure** | Kubernetes secrets, no hardcoded credentials |
| **Prompt Injection** | Input sanitization, prompt templates |
| **Data Exfiltration** | Read-only K8s access, audit logging |
| **Man-in-the-Middle** | mTLS for all service-to-service communication |

---

## Authentication

### **Kubernetes TokenReviewer**

**Implementation**: `src/auth/token_reviewer.py`

```python
from kubernetes import client, config
from typing import Optional
import logging

logger = logging.getLogger(__name__)

class TokenReviewer:
    """Kubernetes TokenReviewer for service authentication."""

    def __init__(self):
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()

        self.auth_api = client.AuthenticationV1Api()

    def validate_token(self, token: str) -> Optional[dict]:
        """
        Validate bearer token with Kubernetes TokenReviewer.

        Args:
            token: Bearer token from Authorization header

        Returns:
            User info dict if authenticated, None otherwise
        """
        try:
            # Create TokenReview request
            token_review = client.V1TokenReview(
                spec=client.V1TokenReviewSpec(token=token)
            )

            # Call TokenReview API
            result = self.auth_api.create_token_review(body=token_review)

            if not result.status.authenticated:
                logger.warning(
                    "Token authentication failed",
                    extra={"error": result.status.error}
                )
                return None

            logger.info(
                "Token validated successfully",
                extra={
                    "username": result.status.user.username,
                    "groups": result.status.user.groups,
                }
            )

            return {
                "username": result.status.user.username,
                "uid": result.status.user.uid,
                "groups": result.status.user.groups or [],
            }

        except Exception as e:
            logger.error("Token review failed", extra={"error": str(e)})
            return None
```

### **FastAPI Middleware**

```python
from fastapi import FastAPI, Request, HTTPException
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
import logging

logger = logging.getLogger(__name__)

class AuthenticationMiddleware(BaseHTTPMiddleware):
    """FastAPI middleware for bearer token authentication."""

    def __init__(self, app: FastAPI, token_reviewer: TokenReviewer):
        super().__init__(app)
        self.token_reviewer = token_reviewer

    async def dispatch(self, request: Request, call_next):
        # Skip authentication for health checks
        if request.url.path in ["/healthz", "/readyz"]:
            return await call_next(request)

        # Extract Bearer token
        auth_header = request.headers.get("Authorization")
        if not auth_header:
            logger.warning("Missing Authorization header")
            return JSONResponse(
                status_code=401,
                content={"error": "unauthorized", "message": "Missing Authorization header"}
            )

        if not auth_header.startswith("Bearer "):
            logger.warning("Invalid Authorization header format")
            return JSONResponse(
                status_code=401,
                content={"error": "unauthorized", "message": "Invalid Authorization header format"}
            )

        token = auth_header[7:]  # Remove "Bearer " prefix

        # Validate token with Kubernetes
        user_info = self.token_reviewer.validate_token(token)
        if not user_info:
            logger.warning("Token validation failed")
            return JSONResponse(
                status_code=401,
                content={"error": "unauthorized", "message": "Token validation failed"}
            )

        # Store user info in request state for authorization
        request.state.user_info = user_info

        return await call_next(request)
```

---

## Authorization

### **RBAC Configuration**

**ServiceAccount**: `holmesgpt-api-sa`

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-role
rules:
# Read-only access for cluster investigation
- apiGroups: [""]
  resources: ["pods", "services", "nodes", "events", "configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["holmesgpt-llm-credentials"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: holmesgpt-api-rolebinding
subjects:
- kind: ServiceAccount
  name: holmesgpt-api-sa
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: holmesgpt-api-role
  apiGroup: rbac.authorization.k8s.io
```

### **Authorization Logic**

```python
from fastapi import Request, HTTPException
import logging

logger = logging.getLogger(__name__)

AUTHORIZED_SERVICE_ACCOUNTS = [
    "system:serviceaccount:kubernaut-system:aianalysis-controller-sa",
    "system:serviceaccount:kubernaut-system:workflowexecution-controller-sa",
]

def check_authorization(request: Request):
    """
    Check if authenticated user is authorized for investigation requests.

    Args:
        request: FastAPI request with user_info in state

    Raises:
        HTTPException: If user is not authorized
    """
    user_info = getattr(request.state, "user_info", None)
    if not user_info:
        logger.error("User info not found in request state")
        raise HTTPException(
            status_code=403,
            detail={"error": "forbidden", "message": "User info not available"}
        )

    username = user_info.get("username", "")

    # Check if service account is authorized
    if username not in AUTHORIZED_SERVICE_ACCOUNTS:
        logger.warning(
            "User not authorized for investigation",
            extra={"username": username, "path": request.url.path}
        )
        raise HTTPException(
            status_code=403,
            detail={
                "error": "forbidden",
                "message": "Service account not authorized for investigation requests"
            }
        )

    logger.info(
        "User authorized for investigation",
        extra={"username": username}
    )
```

### **FastAPI Dependency**

```python
from fastapi import Depends, Request

async def get_authorized_user(request: Request) -> dict:
    """FastAPI dependency for authorization."""
    check_authorization(request)
    return request.state.user_info

# Usage in endpoint
@app.post("/api/v1/investigate")
async def investigate(
    request: InvestigationRequest,
    user_info: dict = Depends(get_authorized_user)
):
    # User is authenticated and authorized
    logger.info(
        "Investigation request received",
        extra={"username": user_info["username"], "alert": request.alert_name}
    )
    # ... investigation logic ...
```

---

## LLM API Security

### **LLM API Key Management**

**Kubernetes Secret**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: holmesgpt-llm-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  openai_api_key: <OPENAI_API_KEY_FROM_VAULT>
  anthropic_api_key: <ANTHROPIC_API_KEY_FROM_VAULT>
  provider: "openai"  # or "anthropic", "ollama"
```

### **Loading LLM Credentials**

```python
from kubernetes import client, config
import os
import logging

logger = logging.getLogger(__name__)

class LLMCredentialsLoader:
    """Load LLM API credentials from Kubernetes secrets."""

    def __init__(self):
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()

        self.core_api = client.CoreV1Api()

    def load_credentials(self) -> dict:
        """
        Load LLM credentials from Kubernetes secret.

        Returns:
            Dict with API keys and provider info
        """
        try:
            secret = self.core_api.read_namespaced_secret(
                name="holmesgpt-llm-credentials",
                namespace="kubernaut-system"
            )

            credentials = {
                "openai_api_key": secret.data.get("openai_api_key", b"").decode("utf-8"),
                "anthropic_api_key": secret.data.get("anthropic_api_key", b"").decode("utf-8"),
                "provider": secret.data.get("provider", b"openai").decode("utf-8"),
            }

            logger.info("LLM credentials loaded successfully")
            return credentials

        except Exception as e:
            logger.error("Failed to load LLM credentials", extra={"error": str(e)})
            raise

# Initialize LLM client with credentials
credentials_loader = LLMCredentialsLoader()
llm_credentials = credentials_loader.load_credentials()

# Configure LLM provider
if llm_credentials["provider"] == "openai":
    os.environ["OPENAI_API_KEY"] = llm_credentials["openai_api_key"]
elif llm_credentials["provider"] == "anthropic":
    os.environ["ANTHROPIC_API_KEY"] = llm_credentials["anthropic_api_key"]
```

### **Prompt Injection Prevention**

```python
import re
from typing import str

class PromptSanitizer:
    """Sanitize user input to prevent prompt injection."""

    # Dangerous patterns that could be prompt injection
    DANGEROUS_PATTERNS = [
        r"ignore\s+previous\s+instructions",
        r"system:\s*",
        r"assistant:\s*",
        r"<\|im_start\|>",
        r"<\|im_end\|>",
    ]

    @classmethod
    def sanitize(cls, user_input: str) -> str:
        """
        Sanitize user input for safe inclusion in prompts.

        Args:
            user_input: Raw user input

        Returns:
            Sanitized input safe for prompts
        """
        # Remove dangerous patterns
        sanitized = user_input
        for pattern in cls.DANGEROUS_PATTERNS:
            sanitized = re.sub(pattern, "", sanitized, flags=re.IGNORECASE)

        # Limit length
        max_length = 2000
        if len(sanitized) > max_length:
            sanitized = sanitized[:max_length]

        return sanitized.strip()

# Usage in investigation
def create_investigation_prompt(alert_name: str, context: str) -> str:
    """Create safe investigation prompt."""
    safe_alert_name = PromptSanitizer.sanitize(alert_name)
    safe_context = PromptSanitizer.sanitize(context)

    prompt = f"""Analyze the following Kubernetes alert and provide remediation recommendations:

Alert: {safe_alert_name}
Context: {safe_context}

Provide a structured analysis including root cause and recommended actions."""

    return prompt
```

---

## Network Security

### **Network Policies**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: holmesgpt-api-netpol
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: holmesgpt-api
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow from authorized services only
  - from:
    - podSelector:
        matchLabels:
          app: aianalysis-controller
    - podSelector:
        matchLabels:
          app: workflowexecution-controller
    ports:
    - protocol: TCP
      port: 8080
  # Allow Prometheus metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9090
  egress:
  # Allow to Kubernetes API server
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: kube-apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow to LLM providers (OpenAI, Anthropic)
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443
  # Allow DNS
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: UDP
      port: 53
```

---

## Secrets Management

### **Environment Variable Loading**

```python
import os
from typing import Optional

class Config:
    """Application configuration with security defaults."""

    # LLM Configuration
    LLM_PROVIDER: str = os.getenv("LLM_PROVIDER", "openai")
    OPENAI_API_KEY: Optional[str] = os.getenv("OPENAI_API_KEY")
    ANTHROPIC_API_KEY: Optional[str] = os.getenv("ANTHROPIC_API_KEY")

    # Kubernetes Configuration
    K8S_NAMESPACE: str = os.getenv("K8S_NAMESPACE", "kubernaut-system")

    # Security Configuration
    ENABLE_AUTH: bool = os.getenv("ENABLE_AUTH", "true").lower() == "true"

    @classmethod
    def validate(cls):
        """Validate configuration on startup."""
        if cls.ENABLE_AUTH:
            if cls.LLM_PROVIDER == "openai" and not cls.OPENAI_API_KEY:
                raise ValueError("OPENAI_API_KEY required when LLM_PROVIDER=openai")
            if cls.LLM_PROVIDER == "anthropic" and not cls.ANTHROPIC_API_KEY:
                raise ValueError("ANTHROPIC_API_KEY required when LLM_PROVIDER=anthropic")

# Validate on startup
Config.validate()
```

### **Secret Rotation**

```python
from datetime import datetime, timedelta
import logging

logger = logging.getLogger(__name__)

class CredentialsCache:
    """Cache LLM credentials with automatic refresh."""

    def __init__(self, loader: LLMCredentialsLoader, ttl_minutes: int = 60):
        self.loader = loader
        self.ttl = timedelta(minutes=ttl_minutes)
        self.credentials = None
        self.last_refresh = None

    def get_credentials(self) -> dict:
        """Get credentials with automatic refresh."""
        now = datetime.now()

        if self.credentials is None or self.last_refresh is None:
            # Initial load
            self.credentials = self.loader.load_credentials()
            self.last_refresh = now
            logger.info("LLM credentials loaded (initial)")
        elif now - self.last_refresh > self.ttl:
            # Refresh credentials
            try:
                self.credentials = self.loader.load_credentials()
                self.last_refresh = now
                logger.info("LLM credentials refreshed")
            except Exception as e:
                logger.error("Failed to refresh credentials, using cached", extra={"error": str(e)})

        return self.credentials
```

---

## Audit Logging

### **Security Event Logging**

```python
import logging
import json
from datetime import datetime

logger = logging.getLogger(__name__)

def log_security_event(event_type: str, **kwargs):
    """Log security events with structured logging."""
    log_data = {
        "timestamp": datetime.utcnow().isoformat(),
        "event_type": event_type,
        "category": "security",
        **kwargs
    }
    logger.info(f"Security event: {event_type}", extra=log_data)

# Usage examples
def handle_investigation_request(request: InvestigationRequest, user_info: dict):
    log_security_event(
        "investigation_request_received",
        username=user_info["username"],
        alert_name=request.alert_name,
        namespace=request.namespace,
    )

    # ... investigation logic ...

    log_security_event(
        "investigation_completed",
        username=user_info["username"],
        alert_name=request.alert_name,
        status="success",
    )

def handle_authentication_failure(error: str):
    log_security_event(
        "authentication_failed",
        error=error,
        source_ip=request.client.host,
    )
```

---

## Security Checklist

### **Pre-Deployment**

- [ ] TokenReviewer authentication implemented and tested
- [ ] RBAC roles and bindings created (holmesgpt-api-sa)
- [ ] LLM API keys stored in Kubernetes secrets (not hardcoded)
- [ ] Network policies restrict ingress to authorized services
- [ ] NetworkPolicies configured for ingress/egress control
- [ ] Prompt injection prevention implemented
- [ ] Input sanitization tested with malicious inputs
- [ ] Security event logging implemented for all requests
- [ ] Secrets rotation policy documented

### **Runtime Security**

- [ ] Monitor failed authentication attempts (alert if > 10/min)
- [ ] Monitor unauthorized access attempts (alert immediately)
- [ ] Audit logs reviewed regularly for suspicious activity
- [ ] LLM API keys never logged
- [ ] Sensitive data sanitized in logs

### **LLM Security**

- [ ] Prompt templates use parameterization (no string concatenation)
- [ ] User input sanitized before inclusion in prompts
- [ ] LLM responses validated before returning to client
- [ ] Rate limiting enforced for LLM API calls
- [ ] Cost monitoring for LLM API usage

---

## Reference Documentation

- **TokenReviewer Auth**: `docs/architecture/KUBERNETES_TOKENREVIEWER_AUTH.md`
- **Logging Standard**: `docs/architecture/LOGGING_STANDARD.md`
- **API Specification**: `docs/services/stateless/holmesgpt-api/api-specification.md`
- **Overview**: `docs/services/stateless/holmesgpt-api/overview.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Security Review**: Pending

