# HolmesGPT API Service - Observability & Logging

**Version**: 1.0
**Last Updated**: October 16, 2025
**Service Type**: Stateless HTTP API Service (AI Investigation & Analysis)
**Logging Library**: Python `logging` with JSON formatter
**Framework**: FastAPI

---

## 📋 Overview

Comprehensive observability strategy for HolmesGPT API Service, covering:
- **Structured Logging** (Python logging with JSON encoding)
- **Prometheus Metrics** (token count, cost tracking, investigation duration)
- **Health Probes** (liveness, readiness with LLM connectivity)
- **Distributed Tracing** (correlation ID propagation)
- **Alert Rules** (Prometheus AlertManager)

---

## 📊 Structured Logging

### **Logging Library: Python logging**

The HolmesGPT API uses Python's built-in `logging` library with JSON formatting for structured, machine-readable logs.

**Dependencies**:
```python
# requirements.txt
python-json-logger==2.0.7  # JSON formatter for structured logging
```

### **Logger Initialization**

```python
# holmesgpt-api/src/config/logging.py
import logging
import sys
from pythonjsonlogger import jsonlogger

def setup_logging(log_level: str = "INFO") -> logging.Logger:
    """Configure structured JSON logging"""

    logger = logging.getLogger("holmesgpt_api")
    logger.setLevel(getattr(logging, log_level.upper()))

    # Remove existing handlers
    logger.handlers.clear()

    # Console handler with JSON formatter
    handler = logging.StreamHandler(sys.stdout)

    # Custom JSON formatter with correlation ID
    formatter = jsonlogger.JsonFormatter(
        '%(timestamp)s %(level)s %(name)s %(correlation_id)s %(message)s',
        rename_fields={
            "levelname": "level",
            "asctime": "timestamp"
        },
        timestamp=True
    )

    handler.setFormatter(formatter)
    logger.addHandler(handler)

    return logger
```

### **Application Initialization**

```python
# holmesgpt-api/src/main.py
from fastapi import FastAPI
from src.config.logging import setup_logging
from src.config.settings import settings

# Initialize logger
logger = setup_logging(log_level=settings.log_level)

# Create FastAPI app
app = FastAPI(
    title="HolmesGPT API",
    version="1.0.0",
    description="AI-powered Kubernetes investigation service"
)

@app.on_event("startup")
async def startup_event():
    logger.info(
        "HolmesGPT API Service starting",
        extra={
            "version": "v1.0",
            "port": 8080,
            "log_level": settings.log_level,
            "llm_provider": settings.llm_provider
        }
    )
```

---

### **Log Levels**

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | LLM provider unavailable, authentication failure |
| **WARN** | Recoverable errors, degraded mode | Rate limit hit, Context API timeout (graceful degradation) |
| **INFO** | Normal operations, state transitions | Investigation requests, completions, token usage |
| **DEBUG** | Detailed flow for troubleshooting | LLM prompts (sanitized), tool calls, context enrichment |

---

### **Correlation ID Propagation**

**FastAPI Middleware**:

```python
# holmesgpt-api/src/api/middleware/correlation.py
import uuid
from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
import logging

logger = logging.getLogger("holmesgpt_api")

class CorrelationIDMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        # Extract or generate correlation ID
        correlation_id = request.headers.get("X-Correlation-ID", str(uuid.uuid4()))

        # Store in request state for downstream access
        request.state.correlation_id = correlation_id

        # Log incoming request
        logger.info(
            "Incoming request",
            extra={
                "correlation_id": correlation_id,
                "method": request.method,
                "path": request.url.path,
                "client_ip": request.client.host if request.client else "unknown"
            }
        )

        # Process request
        response = await call_next(request)

        # Add correlation ID to response headers
        response.headers["X-Correlation-ID"] = correlation_id

        return response
```

**Add to FastAPI App**:

```python
# holmesgpt-api/src/main.py
from src.api.middleware.correlation import CorrelationIDMiddleware

app.add_middleware(CorrelationIDMiddleware)
```

---

### **Investigation Lifecycle Logging**

#### **1. Investigation Request**

```python
# holmesgpt-api/src/api/v1/investigate.py
import logging
import time
from fastapi import APIRouter, Request, HTTPException
from src.models.requests import InvestigationRequest
from src.services.holmesgpt_client import HolmesGPTClient

logger = logging.getLogger("holmesgpt_api")
router = APIRouter()

@router.post("/api/v1/investigate")
async def investigate(request: Request, req: InvestigationRequest):
    correlation_id = request.state.correlation_id
    start_time = time.time()

    logger.info(
        "Investigation request received",
        extra={
            "correlation_id": correlation_id,
            "investigation_id": req.context.get("investigation_id"),
            "priority": req.context.get("priority"),
            "environment": req.context.get("environment"),
            "llm_provider": req.llmProvider,
            "llm_model": req.llmModel
        }
    )

    try:
        # Perform investigation
        client = HolmesGPTClient(logger)
        result = await client.investigate(req, correlation_id)

        duration = time.time() - start_time

        logger.info(
            "Investigation completed successfully",
            extra={
                "correlation_id": correlation_id,
                "investigation_id": req.context.get("investigation_id"),
                "duration_seconds": round(duration, 3),
                "input_tokens": result.get("metrics", {}).get("input_tokens"),
                "output_tokens": result.get("metrics", {}).get("output_tokens"),
                "total_cost": result.get("metrics", {}).get("cost_dollars"),
                "recommendations_count": len(result.get("recommendations", []))
            }
        )

        return result

    except Exception as e:
        duration = time.time() - start_time

        logger.error(
            "Investigation failed",
            extra={
                "correlation_id": correlation_id,
                "investigation_id": req.context.get("investigation_id"),
                "duration_seconds": round(duration, 3),
                "error": str(e),
                "error_type": type(e).__name__
            },
            exc_info=True
        )

        raise HTTPException(status_code=500, detail=str(e))
```

**Example Log Entry (Success)**:

```json
{
  "timestamp": "2025-10-16T14:23:15.432Z",
  "level": "INFO",
  "name": "holmesgpt_api",
  "correlation_id": "abc123-def456",
  "message": "Investigation completed successfully",
  "investigation_id": "mem-api-srv-abc123",
  "duration_seconds": 2.145,
  "input_tokens": 287,
  "output_tokens": 512,
  "total_cost": 0.0423,
  "recommendations_count": 3
}
```

---

#### **2. Token Count and Cost Tracking**

```python
# holmesgpt-api/src/services/token_tracker.py
import logging
from typing import Dict, Optional

logger = logging.getLogger("holmesgpt_api")

class TokenTracker:
    """Track token usage and cost per investigation"""

    # Token costs per provider (per 1K tokens)
    TOKEN_COSTS = {
        "openai": {
            "gpt-4": {"input": 0.03, "output": 0.06},
            "gpt-4-turbo": {"input": 0.01, "output": 0.03},
            "gpt-3.5-turbo": {"input": 0.0015, "output": 0.002}
        },
        "anthropic": {
            "claude-3-opus": {"input": 0.015, "output": 0.075},
            "claude-3-sonnet": {"input": 0.003, "output": 0.015}
        }
    }

    def calculate_cost(
        self,
        provider: str,
        model: str,
        input_tokens: int,
        output_tokens: int,
        correlation_id: str
    ) -> Dict[str, float]:
        """Calculate cost and log token usage"""

        try:
            costs = self.TOKEN_COSTS.get(provider, {}).get(model)
            if not costs:
                logger.warn(
                    "Unknown provider/model for cost calculation",
                    extra={
                        "correlation_id": correlation_id,
                        "provider": provider,
                        "model": model
                    }
                )
                return {"cost_dollars": 0.0}

            input_cost = (input_tokens / 1000) * costs["input"]
            output_cost = (output_tokens / 1000) * costs["output"]
            total_cost = input_cost + output_cost

            logger.info(
                "Token usage calculated",
                extra={
                    "correlation_id": correlation_id,
                    "provider": provider,
                    "model": model,
                    "input_tokens": input_tokens,
                    "output_tokens": output_tokens,
                    "total_tokens": input_tokens + output_tokens,
                    "input_cost": round(input_cost, 4),
                    "output_cost": round(output_cost, 4),
                    "total_cost": round(total_cost, 4)
                }
            )

            return {
                "input_tokens": input_tokens,
                "output_tokens": output_tokens,
                "total_tokens": input_tokens + output_tokens,
                "cost_dollars": round(total_cost, 4)
            }

        except Exception as e:
            logger.error(
                "Cost calculation failed",
                extra={
                    "correlation_id": correlation_id,
                    "error": str(e)
                },
                exc_info=True
            )
            return {"cost_dollars": 0.0}
```

---

#### **3. Authentication and Authorization Logging**

```python
# holmesgpt-api/src/api/middleware/auth.py
import logging
from fastapi import Request, HTTPException
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials

logger = logging.getLogger("holmesgpt_api")
security = HTTPBearer()

async def verify_token(request: Request, credentials: HTTPAuthorizationCredentials):
    """Verify Kubernetes ServiceAccount token"""
    correlation_id = request.state.correlation_id
    token = credentials.credentials

    logger.debug(
        "Authenticating request",
        extra={
            "correlation_id": correlation_id,
            "client_ip": request.client.host if request.client else "unknown"
        }
    )

    try:
        # Verify token with Kubernetes API
        # ... token verification logic ...

        logger.info(
            "Authentication successful",
            extra={
                "correlation_id": correlation_id,
                "service_account": "extracted_sa_name",
                "namespace": "extracted_namespace"
            }
        )

        return True

    except Exception as e:
        logger.error(
            "Authentication failed",
            extra={
                "correlation_id": correlation_id,
                "client_ip": request.client.host if request.client else "unknown",
                "error": str(e),
                "error_type": type(e).__name__
            }
        )

        raise HTTPException(status_code=401, detail="Authentication failed")
```

**Example Log Entry (Auth Failure)**:

```json
{
  "timestamp": "2025-10-16T14:25:10.123Z",
  "level": "ERROR",
  "name": "holmesgpt_api",
  "correlation_id": "xyz789",
  "message": "Authentication failed",
  "client_ip": "10.0.1.45",
  "error": "Invalid token signature",
  "error_type": "AuthenticationError"
}
```

---

#### **4. Rate Limiting Logging**

```python
# holmesgpt-api/src/api/middleware/ratelimit.py
import logging
from fastapi import Request, HTTPException
from slowapi import Limiter
from slowapi.util import get_remote_address

logger = logging.getLogger("holmesgpt_api")

limiter = Limiter(key_func=get_remote_address)

def rate_limit_exceeded_handler(request: Request, exc):
    """Log rate limit violations"""
    correlation_id = getattr(request.state, "correlation_id", "unknown")

    logger.warn(
        "Rate limit exceeded",
        extra={
            "correlation_id": correlation_id,
            "client_ip": request.client.host if request.client else "unknown",
            "path": request.url.path,
            "method": request.method,
            "limit": "100/minute"
        }
    )

    return HTTPException(status_code=429, detail="Rate limit exceeded")
```

---

#### **5. LLM Provider Integration Logging**

```python
# holmesgpt-api/src/services/holmesgpt_client.py
import logging
import time
from typing import Dict, Any
from holmes import HolmesGPT

logger = logging.getLogger("holmesgpt_api")

class HolmesGPTClient:
    def __init__(self, logger: logging.Logger):
        self.logger = logger
        self.client = HolmesGPT()

    async def investigate(self, request: Dict[str, Any], correlation_id: str) -> Dict[str, Any]:
        """Perform investigation using HolmesGPT SDK"""

        start = time.time()

        self.logger.debug(
            "Calling LLM provider",
            extra={
                "correlation_id": correlation_id,
                "provider": request.get("llmProvider"),
                "model": request.get("llmModel"),
                "max_tokens": request.get("maxTokens"),
                "temperature": request.get("temperature")
            }
        )

        try:
            # Call HolmesGPT SDK
            result = await self.client.investigate_async(
                context=request.get("context"),
                llm_provider=request.get("llmProvider"),
                llm_model=request.get("llmModel"),
                toolsets=request.get("toolsets", [])
            )

            duration = time.time() - start

            self.logger.info(
                "LLM provider call successful",
                extra={
                    "correlation_id": correlation_id,
                    "provider": request.get("llmProvider"),
                    "model": request.get("llmModel"),
                    "duration_seconds": round(duration, 3),
                    "input_tokens": result.get("usage", {}).get("prompt_tokens"),
                    "output_tokens": result.get("usage", {}).get("completion_tokens")
                }
            )

            return result

        except Exception as e:
            duration = time.time() - start

            self.logger.error(
                "LLM provider call failed",
                extra={
                    "correlation_id": correlation_id,
                    "provider": request.get("llmProvider"),
                    "model": request.get("llmModel"),
                    "duration_seconds": round(duration, 3),
                    "error": str(e),
                    "error_type": type(e).__name__
                },
                exc_info=True
            )

            raise
```

---

## 📈 Prometheus Metrics

### **Metrics Exposition**

```python
# holmesgpt-api/src/utils/metrics.py
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST
from fastapi import Response

# Investigation metrics
investigation_requests_total = Counter(
    'holmesgpt_investigations_total',
    'Total investigation requests',
    ['status', 'priority', 'environment']
)

investigation_duration_seconds = Histogram(
    'holmesgpt_investigation_duration_seconds',
    'Investigation request duration',
    ['priority', 'environment'],
    buckets=[0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0]
)

# Token and cost metrics
investigation_tokens_total = Counter(
    'holmesgpt_investigation_tokens_total',
    'Total tokens used in investigations',
    ['type', 'provider', 'model']  # type: input/output
)

investigation_cost_dollars_total = Counter(
    'holmesgpt_investigation_cost_dollars_total',
    'Total cost of investigations in dollars',
    ['provider', 'model']
)

# Authentication metrics
auth_failures_total = Counter(
    'holmesgpt_auth_failures_total',
    'Total authentication failures',
    ['reason']
)

# Rate limit metrics
rate_limit_hits_total = Counter(
    'holmesgpt_rate_limit_hits_total',
    'Total rate limit violations',
    ['client_ip']
)

# LLM provider metrics
llm_calls_total = Counter(
    'holmesgpt_llm_calls_total',
    'Total LLM provider calls',
    ['provider', 'model', 'status']
)

llm_call_duration_seconds = Histogram(
    'holmesgpt_llm_call_duration_seconds',
    'LLM provider call duration',
    ['provider', 'model'],
    buckets=[0.5, 1.0, 2.0, 5.0, 10.0]
)

# Context API integration metrics
context_api_calls_total = Counter(
    'holmesgpt_context_api_calls_total',
    'Total Context API calls (when LLM requests tool)',
    ['tool_name', 'status']
)

@app.get("/metrics")
async def metrics():
    """Expose Prometheus metrics"""
    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)
```

### **Metrics Usage Example**

```python
# holmesgpt-api/src/api/v1/investigate.py (updated)
from src.utils.metrics import (
    investigation_requests_total,
    investigation_duration_seconds,
    investigation_tokens_total,
    investigation_cost_dollars_total
)

@router.post("/api/v1/investigate")
async def investigate(request: Request, req: InvestigationRequest):
    priority = req.context.get("priority", "unknown")
    environment = req.context.get("environment", "unknown")

    with investigation_duration_seconds.labels(
        priority=priority,
        environment=environment
    ).time():
        try:
            result = await client.investigate(req, correlation_id)

            # Record success
            investigation_requests_total.labels(
                status="success",
                priority=priority,
                environment=environment
            ).inc()

            # Record token usage
            metrics = result.get("metrics", {})
            investigation_tokens_total.labels(
                type="input",
                provider=req.llmProvider,
                model=req.llmModel
            ).inc(metrics.get("input_tokens", 0))

            investigation_tokens_total.labels(
                type="output",
                provider=req.llmProvider,
                model=req.llmModel
            ).inc(metrics.get("output_tokens", 0))

            # Record cost
            investigation_cost_dollars_total.labels(
                provider=req.llmProvider,
                model=req.llmModel
            ).inc(metrics.get("cost_dollars", 0))

            return result

        except Exception as e:
            investigation_requests_total.labels(
                status="error",
                priority=priority,
                environment=environment
            ).inc()

            raise
```

---

## 🏥 Health Probes

### **Liveness Probe**

```python
# holmesgpt-api/src/api/v1/health.py
import logging
from fastapi import APIRouter, Response

logger = logging.getLogger("holmesgpt_api")
router = APIRouter()

@router.get("/health")
async def health_check():
    """Kubernetes liveness probe - basic service health"""
    logger.debug("Liveness probe check")
    return {"status": "healthy", "service": "holmesgpt-api"}
```

### **Readiness Probe**

```python
@router.get("/ready")
async def readiness_check():
    """Kubernetes readiness probe - check dependencies"""
    logger.debug("Readiness probe check")

    checks = {}
    ready = True

    # Check LLM provider connectivity
    try:
        # Quick health check to LLM provider
        llm_healthy = await check_llm_provider()
        checks["llm_provider"] = "ok" if llm_healthy else "degraded"
        if not llm_healthy:
            ready = False
    except Exception as e:
        logger.error(f"LLM provider check failed: {e}")
        checks["llm_provider"] = "unavailable"
        ready = False

    # Check Context API availability (optional)
    try:
        context_api_healthy = await check_context_api()
        checks["context_api"] = "ok" if context_api_healthy else "degraded"
        # Don't fail readiness if Context API is down (graceful degradation)
    except Exception as e:
        logger.warn(f"Context API check failed: {e}")
        checks["context_api"] = "degraded"

    status_code = 200 if ready else 503

    return Response(
        content={"status": "ready" if ready else "not_ready", "checks": checks},
        status_code=status_code
    )
```

---

## 🔔 Alert Rules

### **Prometheus AlertManager Rules**

```yaml
# deploy/kubernetes/alerts.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: holmesgpt-api-alerts
  namespace: kubernaut-system
spec:
  groups:
  - name: holmesgpt-api
    interval: 30s
    rules:

    # High error rate
    - alert: HolmesGPTAPIHighErrorRate
      expr: |
        rate(holmesgpt_investigations_total{status="error"}[5m])
        /
        rate(holmesgpt_investigations_total[5m])
        > 0.05
      for: 5m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "HolmesGPT API error rate above 5%"
        description: "Error rate is {{ $value | humanizePercentage }}"

    # High investigation latency
    - alert: HolmesGPTAPIHighLatency
      expr: |
        histogram_quantile(0.95,
          rate(holmesgpt_investigation_duration_seconds_bucket[5m])
        ) > 5.0
      for: 10m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "HolmesGPT API p95 latency above 5s"
        description: "p95 latency is {{ $value }}s"

    # Token cost anomaly
    - alert: HolmesGPTAPITokenCostAnomaly
      expr: |
        rate(holmesgpt_investigation_cost_dollars_total[1h])
        > 1.5 * rate(holmesgpt_investigation_cost_dollars_total[24h] offset 1d)
      for: 30m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "HolmesGPT API token cost anomaly detected"
        description: "Hourly cost is 50% higher than daily average"

    # Authentication failures
    - alert: HolmesGPTAPIAuthFailures
      expr: |
        rate(holmesgpt_auth_failures_total[5m]) > 10
      for: 5m
      labels:
        severity: warning
        service: holmesgpt-api
      annotations:
        summary: "High authentication failure rate"
        description: "Auth failures: {{ $value }} req/s"

    # LLM provider failures
    - alert: HolmesGPTAPILLMProviderDown
      expr: |
        rate(holmesgpt_llm_calls_total{status="error"}[5m])
        /
        rate(holmesgpt_llm_calls_total[5m])
        > 0.1
      for: 5m
      labels:
        severity: critical
        service: holmesgpt-api
      annotations:
        summary: "LLM provider failure rate above 10%"
        description: "Failure rate: {{ $value | humanizePercentage }}"
```

---

## 📊 Grafana Dashboard Queries

### **Investigation Rate**

```promql
# Requests per second
rate(holmesgpt_investigations_total[5m])

# By status
sum by (status) (rate(holmesgpt_investigations_total[5m]))
```

### **Token Usage Trends**

```promql
# Tokens per minute
rate(holmesgpt_investigation_tokens_total[1m]) * 60

# By type (input/output)
sum by (type) (rate(holmesgpt_investigation_tokens_total[5m]))
```

### **Cost Tracking**

```promql
# Cost per hour
rate(holmesgpt_investigation_cost_dollars_total[1h]) * 3600

# Daily cost
sum(increase(holmesgpt_investigation_cost_dollars_total[24h]))
```

### **Latency Percentiles**

```promql
# p50, p95, p99
histogram_quantile(0.50, rate(holmesgpt_investigation_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(holmesgpt_investigation_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(holmesgpt_investigation_duration_seconds_bucket[5m]))
```

---

## 🔍 Troubleshooting

### **Common Log Queries**

**Find slow investigations**:
```json
{
  "level": "INFO",
  "message": "Investigation completed successfully",
  "duration_seconds": {">": 5.0}
}
```

**Find high-cost investigations**:
```json
{
  "level": "INFO",
  "message": "Token usage calculated",
  "total_cost": {">": 0.10}
}
```

**Find authentication failures**:
```json
{
  "level": "ERROR",
  "message": "Authentication failed"
}
```

**Find LLM provider errors**:
```json
{
  "level": "ERROR",
  "message": "LLM provider call failed"
}
```

---

## ✅ Observability Checklist

- [ ] Structured JSON logging configured
- [ ] Correlation ID middleware enabled
- [ ] Token count and cost tracking implemented
- [ ] Authentication logging in place
- [ ] Rate limit logging configured
- [ ] Prometheus metrics exposed at `/metrics`
- [ ] Health probes (`/health`, `/ready`) implemented
- [ ] Alert rules deployed to Prometheus
- [ ] Grafana dashboard created
- [ ] Log aggregation (e.g., ELK, Loki) configured

---

## 📚 References

- Python logging: https://docs.python.org/3/library/logging.html
- python-json-logger: https://github.com/madzak/python-json-logger
- Prometheus Python client: https://github.com/prometheus/client_python
- FastAPI middleware: https://fastapi.tiangolo.com/tutorial/middleware/


