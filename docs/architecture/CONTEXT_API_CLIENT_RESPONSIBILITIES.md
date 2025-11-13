# Context API Client Responsibilities - Crystal Clear

**Date**: November 11, 2025
**Purpose**: Define which services access Context API and their exact responsibilities
**Status**: ✅ Authoritative Reference
**Confidence**: 100%

---

## 🎯 Executive Summary

**Context API is accessed by EXACTLY ONE service:**
1. **HolmesGPT API** (on behalf of LLM) - On-demand playbook queries

**ALL other services are FORBIDDEN from accessing Context API.**

**Note**: Effectiveness Monitor does NOT access Context API. It queries Data Storage Service directly for remediation effectiveness data. Context API has legacy REST endpoints that are no longer used (deprecated after Signal Processor → Context API path was removed per DD-CONTEXT-006). These endpoints will be evaluated for deprecation post-V1.0.

---

## ✅ ALLOWED: HolmesGPT API (LLM Tool Calls)

### Service Information
- **Service Name**: HolmesGPT API
- **Type**: Stateless REST Service
- **Namespace**: `kubernaut-system`
- **Access Pattern**: On-demand (LLM-driven tool calls)
- **Client Type**: HTTP REST Client (Python)

### Responsibilities

#### 1. Expose Context API as LLM Tool
**Purpose**: Allow LLM to query Context API for playbooks when needed

**Implementation**:
```python
# holmesgpt-api/src/tools/context_playbook_tool.py

class ContextPlaybookTool(Tool):
    """Tool for LLM to query Context API for remediation playbooks."""

    def __init__(self, context_api_client):
        super().__init__(
            name="get_playbooks",
            description="Query Context API for remediation playbooks matching the incident.",
            parameters=[
                ToolParameter(name="description", type="string", required=True),
                ToolParameter(name="labels", type="array", required=False),
                ToolParameter(name="min_confidence", type="float", required=False, default=0.7),
                ToolParameter(name="max_results", type="integer", required=False, default=10)
            ]
        )
        self.context_api_client = context_api_client

    def execute(self, description: str, labels: list = None,
                min_confidence: float = 0.7, max_results: int = 10):
        """Execute tool call to query Context API for playbooks."""

        # Build query parameters
        params = {
            "description": description,
            "min_confidence": min_confidence,
            "max_results": max_results
        }

        if labels:
            params["labels"] = labels

        # Query Context API
        response = self.context_api_client.get(
            "/api/v1/context/playbooks",
            params=params,
            timeout=2.0
        )

        if response.status_code != 200:
            return {
                "error": f"Context API error: {response.status_code}",
                "playbooks": []
            }

        # Return minimal 4-field schema (per DD-CONTEXT-005)
        return response.json()
```

#### 2. Query Parameters Construction
**Responsibility**: Build query parameters from LLM tool call arguments

**Query Parameters**:
| Parameter | Type | Required | Source | Example |
|-----------|------|----------|--------|---------|
| `description` | string | Yes | LLM reasoning | "High memory usage causing pod restarts" |
| `labels` | array | No | LLM reasoning | `["kubernaut.io/environment:production"]` |
| `min_confidence` | float | No | LLM reasoning | `0.7` |
| `max_results` | int | No | LLM reasoning | `10` |

**Example Query**:
```http
GET /api/v1/context/playbooks?description=High+memory+usage+causing+pod+restarts&labels=kubernaut.io/environment:production&labels=kubernaut.io/priority:P0&min_confidence=0.7&max_results=10
```

#### 3. Response Handling
**Responsibility**: Return minimal 4-field schema to LLM

**Response Schema** (per DD-CONTEXT-005):
```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "confidence": 0.92
    }
  ],
  "total_results": 1
}
```

**Fields Returned**:
- `playbook_id`: Unique playbook identifier
- `version`: Playbook version
- `description`: Human-readable description
- `confidence`: Composite score (semantic + label + historical)

**Fields NOT Returned** (per DD-CONTEXT-005):
- ❌ `success_rate` (creates feedback loop)
- ❌ `incident_types` (redundant with query)
- ❌ `risk_level` (handled by label filtering)
- ❌ `estimated_duration` (not suitable for LLM reasoning)
- ❌ `prerequisites` (redundant with label filtering)
- ❌ `rollback_available` (part of risk categorization)

#### 4. Error Handling
**Responsibility**: Handle Context API errors gracefully

**Error Scenarios**:
| Scenario | Response | LLM Impact |
|----------|----------|------------|
| Context API unavailable | `{"error": "...", "playbooks": []}` | LLM can recommend manual investigation |
| No playbooks found | `{"playbooks": [], "total_results": 0}` | LLM can suggest creating new playbook |
| Timeout (>2s) | `{"error": "timeout", "playbooks": []}` | LLM can recommend manual investigation |
| Invalid query | `{"error": "...", "playbooks": []}` | LLM can retry with different parameters |

#### 5. Observability
**Responsibility**: Log all tool calls for monitoring

**Metrics**:
- `holmesgpt_context_tool_call_total` (counter) - Total tool calls
- `holmesgpt_context_tool_call_duration_seconds` (histogram) - Query latency
- `holmesgpt_context_tool_call_errors_total` (counter) - Error count
- `holmesgpt_context_tool_call_playbooks_returned` (histogram) - Playbooks per query

**Logs**:
```json
{
  "timestamp": "2025-11-11T10:30:00Z",
  "service": "holmesgpt-api",
  "tool": "get_playbooks",
  "query": {
    "description": "High memory usage",
    "labels": ["kubernaut.io/environment:production"],
    "min_confidence": 0.7
  },
  "response": {
    "playbooks_returned": 2,
    "duration_ms": 45,
    "status": "success"
  }
}
```

### Configuration

**Environment Variables**:
```bash
# Context API endpoint
CONTEXT_API_URL=http://context-api.kubernaut-system.svc.cluster.local:8091

# Timeout for Context API queries
CONTEXT_API_TIMEOUT=2s

# Retry configuration
CONTEXT_API_MAX_RETRIES=3
CONTEXT_API_RETRY_BACKOFF=exponential
```

**Service Discovery**:
```yaml
# Kubernetes Service
apiVersion: v1
kind: Service
metadata:
  name: context-api
  namespace: kubernaut-system
spec:
  selector:
    app: context-api
  ports:
    - name: http
      port: 8091
      targetPort: 8091
```

### Architectural Decisions

**Related DDs**:
- **DD-CONTEXT-003**: Context Enrichment Placement (LLM-Driven Tool Call Pattern)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (Filter Before LLM)
- **BR-AI-057**: Use Success Rates for Playbook Selection (via HolmesGPT API)

**Key Principles**:
1. ✅ **LLM Autonomy**: LLM decides when to query Context API
2. ✅ **On-Demand**: No pre-enrichment, only when LLM needs it
3. ✅ **Minimal Schema**: 4 fields only, no circumstantial data
4. ✅ **Filter Before LLM**: All filtering via query parameters

---

## ❌ DEPRECATED: Effectiveness Monitor (No Longer Uses Context API)

### Service Information
- **Service Name**: Effectiveness Monitor
- **Type**: CRD Controller
- **Namespace**: `kubernaut-system`
- **Access Pattern**: ❌ **NONE** - Does NOT access Context API
- **Data Source**: Data Storage Service (direct queries)

### Why Effectiveness Monitor Does NOT Use Context API

**Correct Architecture**:
```
Effectiveness Monitor
    ↓ (queries remediation effectiveness data)
Data Storage Service REST API
    ↓ (returns success rates, failure patterns, trends)
Effectiveness Monitor
    ↓ (validates remediation success)
    ↓ (updates system for future analysis)
Data Storage Service
    ↓ (stores updated effectiveness data)
```

**Purpose of Effectiveness Monitor**:
1. ✅ **Validate Remediation Success**: Determine if remediation actually resolved the issue
2. ✅ **Update System**: Store effectiveness data for future analysis
3. ✅ **Enable Human Analysis**: Provide data that humans can query via Data Storage REST API
4. ✅ **Track Trends**: Monitor playbook performance over time

**Data Source**: Data Storage Service (NOT Context API)
- Effectiveness Monitor queries Data Storage Service directly
- Data Storage Service provides remediation audit data
- Effectiveness Monitor updates effectiveness metrics in Data Storage Service
- Humans can query Data Storage Service REST API for analytics

### Legacy Context API Endpoints (Deprecated)

**Status**: ⚠️ **DEPRECATED** - No longer used after DD-CONTEXT-006

**Background**:
- Context API previously had REST endpoints for analytics
- These endpoints were used when Signal Processor → Context API path existed
- DD-CONTEXT-006 removed Signal Processor → Context API path
- Endpoints are now unused and will be evaluated for removal post-V1.0

**Legacy Endpoints** (DO NOT USE):
| Endpoint | Original Purpose | Status |
|----------|------------------|--------|
| `GET /api/v1/context/analytics/playbook/{id}` | Playbook effectiveness | ⚠️ DEPRECATED |
| `GET /api/v1/context/analytics/trends` | Historical trends | ⚠️ DEPRECATED |
| `GET /api/v1/context/analytics/failures` | Failure patterns | ⚠️ DEPRECATED |
| `GET /api/v1/context/analytics/success-rates` | Success rate aggregation | ⚠️ DEPRECATED |

**Replacement**: Use Data Storage Service REST API instead

### Correct Data Flow

**Effectiveness Monitor → Data Storage Service**:
```go
// pkg/effectiveness/data_storage_client.go

type DataStorageClient struct {
    baseURL    string
    httpClient *http.Client
    logger     *zap.Logger
}

// GetRemediationEffectiveness queries Data Storage for effectiveness data
func (c *DataStorageClient) GetRemediationEffectiveness(ctx context.Context,
    remediationID string) (*EffectivenessData, error) {

    url := fmt.Sprintf("%s/api/v1/remediation/%s/effectiveness", c.baseURL, remediationID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to query Data Storage: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("Data Storage error: %d", resp.StatusCode)
    }

    var effectiveness EffectivenessData
    if err := json.NewDecoder(resp.Body).Decode(&effectiveness); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &effectiveness, nil
}

// UpdateEffectiveness updates effectiveness data in Data Storage
func (c *DataStorageClient) UpdateEffectiveness(ctx context.Context,
    remediationID string, effectiveness *EffectivenessData) error {

    url := fmt.Sprintf("%s/api/v1/remediation/%s/effectiveness", c.baseURL, remediationID)

    body, err := json.Marshal(effectiveness)
    if err != nil {
        return fmt.Errorf("failed to marshal effectiveness: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("failed to update Data Storage: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("Data Storage error: %d", resp.StatusCode)
    }

    return nil
}
```

### Human Analysis

**Humans CAN**:
- ✅ Query Data Storage Service REST API directly for analytics
- ✅ View effectiveness dashboards (powered by Data Storage Service)
- ✅ Generate reports from Data Storage Service data

**Humans DO NOT**:
- ❌ Query Context API for analytics (deprecated endpoints)
- ❌ Use Effectiveness Monitor as a proxy to Context API

### Configuration

**Environment Variables**:
```bash
# Data Storage Service endpoint (NOT Context API)
DATA_STORAGE_URL=http://data-storage.kubernaut-system.svc.cluster.local:8090

# Query intervals
EFFECTIVENESS_CHECK_INTERVAL=5m
EFFECTIVENESS_UPDATE_INTERVAL=10m

# Timeout
DATA_STORAGE_TIMEOUT=5s
```

### Architectural Decisions

**Related DDs**:
- **DD-CONTEXT-006**: Signal Processor Recovery Data Source (removed Context API dependency)
- **DD-SCHEMA-001**: Data Storage Schema Authority (Data Storage is source of truth)

**Key Principles**:
1. ✅ **Direct Data Access**: Effectiveness Monitor queries Data Storage Service directly
2. ✅ **System Updates**: Updates effectiveness data for future analysis
3. ✅ **Human Analysis**: Humans query Data Storage Service REST API
4. ❌ **NO Context API**: Context API endpoints are deprecated

---

## ❌ FORBIDDEN: AIAnalysis Controller

### Service Information
- **Service Name**: AIAnalysis Controller
- **Type**: CRD Controller
- **Namespace**: `kubernaut-system`
- **Access Pattern**: ❌ **NONE** - Does NOT access Context API

### Why Forbidden

**Architectural Decision**: DD-CONTEXT-003 (Context Enrichment Placement)

**Rationale**:
1. ❌ **Pre-Enrichment Anti-Pattern**: AIAnalysis Controller should NOT pre-enrich LLM prompts with context
2. ✅ **LLM Autonomy**: LLM decides when to query Context API via HolmesGPT API tool
3. ✅ **Token Efficiency**: Avoids unnecessary context in every prompt (36% token cost reduction)
4. ✅ **Flexibility**: LLM can query multiple times, compare alternatives

**Correct Pattern**:
```
AIAnalysis Controller
    ↓ (send investigation request)
HolmesGPT API Service
    ↓ (LLM decides if context needed)
LLM (Claude/Vertex AI)
    ↓ (tool call: get_playbooks)
HolmesGPT API → Context API Service
    ↓ (return playbooks to LLM)
LLM continues investigation with playbooks
```

**Incorrect Pattern** (FORBIDDEN):
```
❌ AIAnalysis Controller → Context API (pre-fetch context)
❌ AIAnalysis Controller → HolmesGPT API (send with pre-enriched context)
❌ LLM receives context without deciding if needed
```

### Monitoring Role (BR-AI-002)

**AIAnalysis Controller DOES**:
- ✅ Monitor HolmesGPT API tool call rates
- ✅ Track investigation quality by context usage
- ✅ Alert on anomalous context usage patterns

**AIAnalysis Controller DOES NOT**:
- ❌ Call Context API directly
- ❌ Pre-enrich LLM prompts with context
- ❌ Fetch playbooks for AI Analysis

**Related DD**: DD-CONTEXT-004 (BR-AI-002 Ownership)

---

## ❌ FORBIDDEN: Signal Processor

### Service Information
- **Service Name**: Signal Processor (RemediationProcessing Controller)
- **Type**: CRD Controller
- **Namespace**: `kubernaut-system`
- **Access Pattern**: ❌ **NONE** - Does NOT access Context API

### Why Forbidden

**Architectural Decision**: DD-CONTEXT-006 (Signal Processor Recovery Data Source)

**Rationale**:
1. ❌ **Circumstantial Historical Data**: Historical recovery data is circumstantial and cannot be reliably used for LLM decision-making
2. ❌ **Causation vs Correlation**: Cannot guarantee same situation without complete environment data
3. ❌ **Feedback Loop**: Exposing historical success/failure rates creates bias
4. ✅ **Current Failure Data**: Remediation Orchestrator embeds current failure data from WorkflowExecution CRD

**Correct Pattern**:
```
Remediation Orchestrator
    ↓ (extracts failure data from WorkflowExecution CRD)
    ↓ (embeds in SignalProcessing CRD spec)
SignalProcessing Controller
    ↓ (enriches with monitoring context - current state)
    ↓ (enriches with business context - current ownership)
    ↓ (NO Context API call for historical recovery data)
AIAnalysis/HolmesGPT API
    ↓ (receives current failure context)
    ↓ (LLM queries Context API for playbooks via tool if needed)
```

**Incorrect Pattern** (FORBIDDEN):
```
❌ SignalProcessing Controller → Context API (query historical recovery data)
❌ SignalProcessing Controller → Embed historical data in CRD
❌ LLM receives circumstantial historical data
```

### What Signal Processor DOES

**Signal Processor DOES**:
- ✅ Enrich with **current** monitoring context (from Data Storage Service)
- ✅ Enrich with **current** business context (from Data Storage Service)
- ✅ Use **current** failure data (embedded by Remediation Orchestrator)

**Signal Processor DOES NOT**:
- ❌ Call Context API for historical recovery data
- ❌ Pre-enrich with historical playbook performance
- ❌ Query for previous failure patterns

**Related DD**: DD-CONTEXT-006 (Signal Processor Recovery Data Source)

---

## ❌ FORBIDDEN: All Other Services

### Services That Do NOT Access Context API

| Service | Type | Why Forbidden |
|---------|------|---------------|
| **Gateway** | CRD Controller | No need for historical data; focuses on ingestion and classification |
| **Remediation Orchestrator** | CRD Controller | Coordinates CRDs; does not need historical data |
| **Workflow Execution** | CRD Controller | Executes playbooks; does not need historical data |
| **Notification** | CRD Controller | Sends notifications; does not need historical data |
| **Data Storage** | Stateless REST | Provides data TO Context API, not FROM Context API |

### General Principle

**Context API is a specialized service for:**
1. ✅ **LLM Tool Calls** (on-demand playbook queries)
2. ✅ **Human Analytics** (periodic effectiveness monitoring)

**All other services should:**
- ✅ Use Data Storage Service for operational data
- ✅ Use CRD specs for context (self-contained CRD pattern)
- ❌ NOT query Context API for historical data

---

## 📊 Summary Table

| Service | Access | Pattern | Purpose | Endpoints | Data Source |
|---------|--------|---------|---------|-----------|-------------|
| **HolmesGPT API** | ✅ YES | LLM tool call | Playbook queries | `GET /playbooks` | Context API |
| **Effectiveness Monitor** | ❌ NO | N/A | Validates remediation success | N/A | Data Storage Service |
| **AIAnalysis Controller** | ❌ NO | N/A | Monitoring only (BR-AI-002) | N/A | N/A |
| **Signal Processor** | ❌ NO | N/A | Uses embedded failure data | N/A | Data Storage Service |
| **Gateway** | ❌ NO | N/A | No historical data needed | N/A | N/A |
| **Remediation Orchestrator** | ❌ NO | N/A | Coordinates CRDs | N/A | N/A |
| **Workflow Execution** | ❌ NO | N/A | Executes playbooks | N/A | N/A |
| **Notification** | ❌ NO | N/A | Sends notifications | N/A | N/A |
| **Data Storage** | ❌ NO | N/A | Provides data TO Context API | N/A | N/A |
| **Humans** | ⚠️ CAN | Direct REST | Analytics queries | Data Storage REST API | Data Storage Service |

**Key Changes**:
- ⚠️ **Effectiveness Monitor**: Does NOT access Context API (uses Data Storage Service instead)
- ⚠️ **Context API Analytics Endpoints**: DEPRECATED (will be evaluated for removal post-V1.0)
- ✅ **Humans**: Can query Data Storage Service REST API directly for analytics

---

## 🔗 Related Documents

- **Diagram**: [context-api-access-patterns.excalidraw](diagrams/context-api-access-patterns.excalidraw)
- **DD-CONTEXT-003**: Context Enrichment Placement (LLM-Driven Tool Call Pattern)
- **DD-CONTEXT-004**: BR-AI-002 Ownership (AIAnalysis monitoring role)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (Filter Before LLM)
- **DD-CONTEXT-006**: Signal Processor Recovery Data Source (no Context API call)
- **BR-AI-057**: Use Success Rates for Playbook Selection (via HolmesGPT API)
- **CONTEXT_API_SERVICE_ACCESS_TRIAGE.md**: Comprehensive triage analysis

---

## 🎯 Validation Checklist

Before implementing any Context API client:

- [ ] Is this service HolmesGPT API?
  - ✅ YES → Proceed with implementation
  - ❌ NO → STOP - Context API access is forbidden

- [ ] If HolmesGPT API:
  - [ ] Implemented as LLM tool (not direct call)?
  - [ ] Returns minimal 4-field schema?
  - [ ] Handles errors gracefully?
  - [ ] Logs all tool calls?

- [ ] If Effectiveness Monitor:
  - [ ] **STOP** - Use Data Storage Service instead
  - [ ] Query Data Storage Service REST API for effectiveness data
  - [ ] Update effectiveness data via Data Storage Service
  - [ ] Do NOT use Context API (deprecated endpoints)

- [ ] If any other service:
  - [ ] **STOP** - Context API access is forbidden
  - [ ] Review DD-CONTEXT-003 and DD-CONTEXT-006
  - [ ] Use Data Storage Service or CRD specs instead

- [ ] If implementing analytics for humans:
  - [ ] Query Data Storage Service REST API (NOT Context API)
  - [ ] Context API analytics endpoints are deprecated

---

**Document Version**: 1.0
**Last Updated**: November 11, 2025
**Status**: ✅ **AUTHORITATIVE REFERENCE**
**Confidence**: **100%** (Crystal clear, no room for doubt)

