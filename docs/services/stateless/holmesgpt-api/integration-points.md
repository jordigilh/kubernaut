# HolmesGPT API Service - Integration Points

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP Service (Python REST API)
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Integration Overview](#integration-overview)
2. [Upstream Services (Investigation Requesters)](#upstream-services-investigation-requesters)
3. [Downstream Services](#downstream-services)
4. [Integration Patterns](#integration-patterns)
5. [Error Handling](#error-handling)
6. [Data Flow Diagrams](#data-flow-diagrams)

---

## Integration Overview

### **Service Position in Architecture**

HolmesGPT API acts as the **AI-powered investigation engine** in the Kubernaut architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                    Upstream Services                        │
│  (Request AI-powered investigations)                        │
│                                                             │
│  • AI Analysis Controller                                   │
│  • Workflow Execution Controller                            │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ HTTP POST /api/v1/investigate
                     │ (Bearer Token Authentication)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│           HolmesGPT API Service (Port 8080)                 │
│                                                             │
│  1. Authenticate with Kubernetes TokenReviewer              │
│  2. Authorize service account for investigation             │
│  3. Load dynamic toolset from ConfigMap                     │
│  4. Retrieve historical context from Context API            │
│  5. Generate investigation prompt                           │
│  6. Call HolmesGPT SDK with toolset                         │
│  7. Parse LLM response                                      │
│  8. Return structured recommendations                       │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ HolmesGPT SDK → LLM Provider
                     │ Kubernetes API (read-only)
                     │ Context API (historical data)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                 Downstream Services                         │
│  (HolmesGPT API calls these services)                       │
│                                                             │
│  • LLM Providers (OpenAI, Anthropic, Ollama)                │
│  • Kubernetes API Server (read-only access)                 │
│  • Context API (historical success rates)                   │
│  • Dynamic Toolset Service (toolset discovery)              │
└─────────────────────────────────────────────────────────────┘
```

---

## Upstream Services (Investigation Requesters)

### **1. AI Analysis Controller**

**Purpose**: Requests AI-powered investigation for remediation decision

**Integration Pattern**: HTTP POST
**Authentication**: Bearer Token (aianalysis-controller-sa)
**Endpoint**: `POST /api/v1/investigate`

#### **Request Flow**

```python
# In AI Analysis Controller (Go)
import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "go.uber.org/zap"
)

type HolmesGPTClient struct {
    baseURL    string
    httpClient *http.Client
    token      string
    logger     *zap.Logger
}

func (c *HolmesGPTClient) Investigate(ctx context.Context, req *InvestigationRequest) (*InvestigationResponse, error) {
    payload, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal investigation request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST",
        fmt.Sprintf("%s/api/v1/investigate", c.baseURL),
        bytes.NewReader(payload),
    )
    if err != nil {
        return nil, err
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("investigation request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    var investigationResp InvestigationResponse
    if err := json.NewDecoder(resp.Body).Decode(&investigationResp); err != nil {
        return nil, err
    }

    return &investigationResp, nil
}
```

#### **Investigation Flow**

```go
// In AI Analysis Controller reconciliation
package controllers

import (
    "context"

    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "go.uber.org/zap"
)

func (c *AIAnalysisController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    // ... load RemediationRequest CRD ...

    // Request AI-powered investigation
    investigationReq := &holmesgpt.InvestigationRequest{
        AlertName: rr.Spec.AlertName,
        Namespace: rr.Spec.Namespace,
        Cluster:   rr.Spec.Cluster,
        Context: map[string]string{
            "severity":  rr.Spec.Priority,
            "timestamp": rr.Spec.Timestamp.String(),
        },
    }

    investigation, err := c.holmesGPTClient.Investigate(ctx, investigationReq)
    if err != nil {
        c.logger.Error("Investigation failed",
            zap.String("remediation_request", rr.Name),
            zap.Error(err),
        )
        // Fallback to rule-based analysis
        return c.fallbackAnalysis(ctx, rr)
    }

    // Use investigation results for remediation decision
    c.logger.Info("Investigation completed",
        zap.String("remediation_request", rr.Name),
        zap.String("root_cause", investigation.RootCause),
        zap.Int("recommended_actions", len(investigation.RecommendedActions)),
    )

    // Update RemediationRequest with AI recommendations
    rr.Status.AIAnalysis = &remediationv1.AIAnalysisStatus{
        RootCause: investigation.RootCause,
        RecommendedAction: investigation.RecommendedActions[0].Action,
        Confidence: investigation.RecommendedActions[0].Confidence,
        Reasoning: investigation.Analysis,
    }

    return reconcile.Result{}, c.Status().Update(ctx, rr)
}
```

---

### **2. Workflow Execution Controller**

**Purpose**: Requests AI-powered investigation during workflow execution

**Integration Pattern**: HTTP POST
**Authentication**: Bearer Token (workflowexecution-controller-sa)
**Endpoint**: `POST /api/v1/investigate`

#### **Request Flow**

```go
// In Workflow Execution Controller
package controllers

import (
    "context"

    "go.uber.org/zap"
)

func (c *WorkflowExecutionController) executeAIAnalysisStep(ctx context.Context, step *WorkflowStep) error {
    investigationReq := &holmesgpt.InvestigationRequest{
        AlertName: step.AlertName,
        Namespace: step.Namespace,
        Cluster:   step.Cluster,
        Context:   step.Context,
    }

    investigation, err := c.holmesGPTClient.Investigate(ctx, investigationReq)
    if err != nil {
        c.logger.Error("Workflow investigation failed",
            zap.String("workflow_id", step.WorkflowID),
            zap.String("step_name", step.Name),
            zap.Error(err),
        )
        return err
    }

    // Store investigation results in workflow context
    step.Results["investigation"] = investigation

    return nil
}
```

---

## Downstream Services

### **1. LLM Providers (OpenAI, Anthropic, Ollama)**

**Purpose**: AI-powered analysis and recommendation generation

**Integration Pattern**: HolmesGPT SDK → LLM API
**Authentication**: API keys from Kubernetes secrets

#### **Python Implementation**

```python
# In HolmesGPT API Service
from holmesgpt import HolmesGPT
import os

class HolmesGPTClient:
    """Client for HolmesGPT SDK integration."""

    def __init__(self, config: Config):
        self.config = config

        # Initialize HolmesGPT with LLM provider
        if config.LLM_PROVIDER == "openai":
            os.environ["OPENAI_API_KEY"] = config.OPENAI_API_KEY
            self.holmes = HolmesGPT(llm_provider="openai", model="gpt-4")
        elif config.LLM_PROVIDER == "anthropic":
            os.environ["ANTHROPIC_API_KEY"] = config.ANTHROPIC_API_KEY
            self.holmes = HolmesGPT(llm_provider="anthropic", model="claude-3-opus")
        else:
            # Local model (Ollama)
            self.holmes = HolmesGPT(llm_provider="ollama", model="llama2")

    async def investigate(self, investigation_request: InvestigationRequest) -> InvestigationResponse:
        """
        Perform AI-powered investigation using HolmesGPT SDK.

        Args:
            investigation_request: Investigation request with alert details

        Returns:
            Structured investigation response with recommendations
        """
        # Generate investigation prompt
        prompt = self._create_investigation_prompt(investigation_request)

        # Call HolmesGPT SDK
        try:
            result = await self.holmes.investigate(
                prompt=prompt,
                context={
                    "namespace": investigation_request.namespace,
                    "cluster": investigation_request.cluster,
                },
                toolset=self.toolset  # Dynamic toolset from ConfigMap
            )

            # Parse response
            return self._parse_investigation_response(result)

        except Exception as e:
            logger.error(f"HolmesGPT investigation failed: {e}")
            raise
```

#### **LLM Provider Fallback**

```python
class LLMProviderManager:
    """Manage LLM provider with fallback."""

    def __init__(self, primary: str, fallback: str):
        self.primary_client = self._create_client(primary)
        self.fallback_client = self._create_client(fallback)
        self.last_used_provider = primary

    async def complete(self, prompt: str) -> str:
        """Complete prompt with fallback on failure."""
        try:
            response = await self.primary_client.complete(prompt)
            self.last_used_provider = "primary"
            return response
        except Exception as e:
            logger.warning(f"Primary LLM provider failed: {e}, using fallback")
            response = await self.fallback_client.complete(prompt)
            self.last_used_provider = "fallback"
            return response
```

---

### **2. Kubernetes API Server (Read-Only)**

**Purpose**: Cluster inspection for investigation

**Integration Pattern**: kubernetes Python client
**Authentication**: ServiceAccount token (in-cluster)

#### **Python Implementation**

```python
from kubernetes import client, config
from typing import List, Dict, Optional

class KubernetesInspector:
    """Kubernetes cluster inspector for investigations."""

    def __init__(self):
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()

        self.core_api = client.CoreV1Api()
        self.apps_api = client.AppsV1Api()

    def list_pods(self, namespace: str, label_selector: Optional[str] = None) -> List[client.V1Pod]:
        """List pods in namespace (read-only)."""
        return self.core_api.list_namespaced_pod(
            namespace=namespace,
            label_selector=label_selector
        ).items

    def get_pod_logs(self, name: str, namespace: str, tail_lines: int = 100) -> str:
        """Get pod logs (read-only)."""
        return self.core_api.read_namespaced_pod_log(
            name=name,
            namespace=namespace,
            tail_lines=tail_lines
        )

    def describe_deployment(self, name: str, namespace: str) -> Dict:
        """Describe deployment (read-only)."""
        deployment = self.apps_api.read_namespaced_deployment(
            name=name,
            namespace=namespace
        )

        return {
            "name": deployment.metadata.name,
            "replicas": deployment.spec.replicas,
            "available_replicas": deployment.status.available_replicas,
            "ready_replicas": deployment.status.ready_replicas,
            "updated_replicas": deployment.status.updated_replicas,
        }
```

---

### **3. Context API (Historical Data)**

**Purpose**: Retrieve historical success rates for similar incidents

**Integration Pattern**: HTTP GET
**Authentication**: Bearer Token (holmesgpt-api-sa)
**Endpoint**: `GET /api/v1/context/success-rate`

#### **Python Implementation**

```python
import httpx
from typing import Optional

class ContextAPIClient:
    """Client for Context API integration."""

    def __init__(self, base_url: str, token: str):
        self.base_url = base_url
        self.token = token
        self.client = httpx.AsyncClient(timeout=10.0)

    async def get_historical_success_rate(
        self,
        alert_name: str,
        action_type: str
    ) -> Optional[float]:
        """
        Get historical success rate for action on alert.

        Args:
            alert_name: Alert name (e.g., "HighMemoryUsage")
            action_type: Action type (e.g., "restart-pod")

        Returns:
            Success rate (0.0-1.0) or None if no data
        """
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json"
        }

        params = {
            "alert_name": alert_name,
            "action_type": action_type
        }

        try:
            response = await self.client.get(
                f"{self.base_url}/api/v1/context/success-rate",
                params=params,
                headers=headers
            )

            if response.status_code == 200:
                data = response.json()
                return data.get("success_rate")
            else:
                logger.warning(f"Context API returned {response.status_code}")
                return None

        except Exception as e:
            logger.error(f"Failed to get historical success rate: {e}")
            return None

# Usage in investigation
async def enhance_investigation_with_context(
    investigation: InvestigationResponse,
    context_client: ContextAPIClient
):
    """Enhance investigation with historical context."""
    for action in investigation.recommended_actions:
        success_rate = await context_client.get_historical_success_rate(
            alert_name=investigation.alert_name,
            action_type=action.action
        )

        if success_rate is not None:
            action.historical_success_rate = success_rate
            # Adjust confidence based on historical data
            action.confidence = (action.confidence + success_rate) / 2
```

---

### **4. Dynamic Toolset Service (Toolset Discovery)**

**Purpose**: Discover and load dynamic toolsets for HolmesGPT

**Integration Pattern**: Kubernetes ConfigMap polling
**Authentication**: ServiceAccount (in-cluster)

#### **Python Implementation**

```python
from kubernetes import client, config, watch
import yaml
import logging

logger = logging.getLogger(__name__)

class ToolsetLoader:
    """Load dynamic toolsets from ConfigMap."""

    def __init__(self, namespace: str = "kubernaut-system", configmap_name: str = "holmesgpt-toolset"):
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()

        self.core_api = client.CoreV1Api()
        self.namespace = namespace
        self.configmap_name = configmap_name
        self.toolset = None

    def load_toolset(self) -> Dict:
        """Load toolset from ConfigMap."""
        try:
            configmap = self.core_api.read_namespaced_config_map(
                name=self.configmap_name,
                namespace=self.namespace
            )

            toolset_yaml = configmap.data.get("toolset.yaml", "{}")
            self.toolset = yaml.safe_load(toolset_yaml)

            logger.info(f"Toolset loaded: {len(self.toolset.get('tools', []))} tools")
            return self.toolset

        except Exception as e:
            logger.error(f"Failed to load toolset: {e}")
            return {}

    def watch_toolset_changes(self, callback):
        """Watch ConfigMap for toolset changes (hot-reload)."""
        w = watch.Watch()

        for event in w.stream(
            self.core_api.list_namespaced_config_map,
            namespace=self.namespace,
            field_selector=f"metadata.name={self.configmap_name}"
        ):
            if event["type"] in ["ADDED", "MODIFIED"]:
                logger.info("Toolset ConfigMap changed, reloading...")
                new_toolset = self.load_toolset()
                callback(new_toolset)
```

---

## Integration Patterns

### **Pattern 1: Synchronous Investigation Request**

**Used by**: AI Analysis Controller, Workflow Execution Controller

**Characteristics**:
- Investigation request is synchronous (waits for response)
- Timeout: 30 seconds (configurable)
- Retry on transient failures (3 attempts with exponential backoff)

```python
# In HolmesGPT API
from fastapi import FastAPI, Request, HTTPException
from pydantic import BaseModel

app = FastAPI()

class InvestigationRequest(BaseModel):
    alert_name: str
    namespace: str
    cluster: str
    context: Dict[str, str]

class InvestigationResponse(BaseModel):
    investigation_id: str
    analysis: str
    root_cause: str
    recommended_actions: List[Dict]

@app.post("/api/v1/investigate", response_model=InvestigationResponse)
async def investigate(
    request: InvestigationRequest,
    req: Request
):
    # Authenticate and authorize (middleware)
    user_info = req.state.user_info

    # Perform investigation
    investigation = await holmes_client.investigate(request)

    # Enhance with historical context
    await enhance_investigation_with_context(investigation, context_client)

    return investigation
```

---

### **Pattern 2: Asynchronous Toolset Hot-Reload**

**Used by**: HolmesGPT API internal process

**Characteristics**:
- ConfigMap changes trigger toolset reload
- No service restart required
- Background process watches for changes

```python
import asyncio

async def toolset_watcher_task(toolset_loader: ToolsetLoader, holmes_client: HolmesGPTClient):
    """Background task to watch toolset changes."""
    def on_toolset_changed(new_toolset: Dict):
        logger.info("Reloading HolmesGPT with new toolset")
        holmes_client.reload_toolset(new_toolset)

    # Start watching in background
    await asyncio.to_thread(
        toolset_loader.watch_toolset_changes,
        callback=on_toolset_changed
    )

# Start background task on application startup
@app.on_event("startup")
async def startup_event():
    asyncio.create_task(toolset_watcher_task(toolset_loader, holmes_client))
```

---

## Error Handling

### **Client-Side Error Handling (Go)**

```go
package holmesgpt

import (
    "context"
    "fmt"
    "time"

    "go.uber.org/zap"
)

func (c *HolmesGPTClient) InvestigateWithRetry(ctx context.Context, req *InvestigationRequest) (*InvestigationResponse, error) {
    maxRetries := 3
    backoff := 1 * time.Second

    for attempt := 1; attempt <= maxRetries; attempt++ {
        investigation, err := c.Investigate(ctx, req)
        if err == nil {
            return investigation, nil
        }

        // Check if error is retryable
        if !isRetryableError(err) {
            return nil, err
        }

        c.logger.Warn("Investigation failed, retrying",
            zap.Int("attempt", attempt),
            zap.Int("max_retries", maxRetries),
            zap.Error(err),
        )

        if attempt < maxRetries {
            time.Sleep(backoff)
            backoff *= 2 // Exponential backoff
        }
    }

    return nil, fmt.Errorf("investigation failed after %d attempts", maxRetries)
}
```

### **Server-Side Error Responses (Python)**

```python
# HTTP 400 - Bad Request
{
    "error": "validation_failed",
    "message": "alert_name is required",
    "field": "alert_name"
}

# HTTP 401 - Unauthorized
{
    "error": "authentication_failed",
    "message": "Token validation failed"
}

# HTTP 403 - Forbidden
{
    "error": "authorization_failed",
    "message": "Service account not authorized for investigations"
}

# HTTP 429 - Rate Limit Exceeded
{
    "error": "rate_limit_exceeded",
    "message": "Too many investigation requests. Limit: 100 req/min per service"
}

# HTTP 500 - Internal Server Error
{
    "error": "investigation_failed",
    "message": "HolmesGPT SDK error: LLM provider timeout",
    "correlation_id": "inv-abc123"
}

# HTTP 503 - Service Unavailable
{
    "error": "service_unavailable",
    "message": "LLM provider unavailable (OpenAI API down)"
}
```

---

## Data Flow Diagrams

### **Complete Investigation Flow**

```
┌──────────────────────────────────────────────────────────────────┐
│ Step 1: AI Analysis Controller                                  │
│   - Remediation decision needed                                  │
│   - Prepare investigation request                                │
│   - Send POST /api/v1/investigate                                │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ HTTP POST (Bearer Token)
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 2: HolmesGPT API - Authentication                          │
│   - Extract Bearer token                                         │
│   - Validate with Kubernetes TokenReviewer                       │
│   - Extract service account identity                             │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Valid Token
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 3: HolmesGPT API - Authorization                           │
│   - Check service account (aianalysis-controller-sa)             │
│   - Verify investigation permission                              │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Authorized
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 4: HolmesGPT API - Toolset Loading                         │
│   - Read toolset from ConfigMap                                  │
│   - Register tools with HolmesGPT SDK                            │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Toolset Loaded
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 5: HolmesGPT API - Historical Context Retrieval            │
│   - Call Context API for historical success rates                │
│   - Enrich investigation request with context                    │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Context Retrieved
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 6: HolmesGPT API - Prompt Generation                       │
│   - Sanitize user input                                          │
│   - Create investigation prompt with template                    │
│   - Include historical context in prompt                         │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Prompt Ready
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 7: HolmesGPT SDK - LLM API Call                            │
│   - Call LLM provider (OpenAI/Anthropic/Ollama)                  │
│   - Execute K8s inspection tools (read-only)                     │
│   - Generate analysis and recommendations                        │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ LLM Response
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 8: HolmesGPT API - Response Parsing                        │
│   - Parse structured response                                    │
│   - Extract root cause and recommendations                       │
│   - Normalize confidence scores                                  │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Structured Response
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 9: HolmesGPT API - Response Enhancement                    │
│   - Adjust confidence based on historical success rates          │
│   - Add metadata (investigation ID, timestamp, tools used)       │
│   - Format final response                                        │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ HTTP 200 OK
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 10: AI Analysis Controller                                 │
│   - Receive investigation response                               │
│   - Use recommendations for remediation decision                 │
│   - Update RemediationRequest CRD with AI analysis               │
└──────────────────────────────────────────────────────────────────┘
```

---

## Reference Documentation

- **API Specification**: `docs/services/stateless/holmesgpt-api/api-specification.md`
- **Security Configuration**: `docs/services/stateless/holmesgpt-api/security-configuration.md`
- **Testing Strategy**: `docs/services/stateless/holmesgpt-api/testing-strategy.md`
- **TokenReviewer Auth**: `docs/architecture/KUBERNETES_TOKENREVIEWER_AUTH.md`
- **Service Dependency Map**: `docs/architecture/SERVICE_DEPENDENCY_MAP.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Integration Status**: Design complete, implementation pending

