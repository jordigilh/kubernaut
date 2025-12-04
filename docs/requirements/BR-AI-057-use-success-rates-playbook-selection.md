# BR-AI-057: Use Success Rates for Playbook Selection

**Business Requirement ID**: BR-AI-057
**Category**: AI/LLM Service
**Priority**: P0
**Target Version**: V1
**Status**: ✅ Approved
**Date**: November 5, 2025
**Last Updated**: November 11, 2025

**⚠️ IMPORTANT CLARIFICATION (2025-11-11)**: "AI Service" in this BR refers to **HolmesGPT API service** (on behalf of the LLM), NOT AIAnalysis Controller. The LLM queries Context API via tool calls, following the LLM-driven tool call pattern defined in DD-CONTEXT-003 and DD-CONTEXT-005.

**Related Architectural Decisions**:
- [DD-CONTEXT-003: Context Enrichment Placement (LLM-Driven Tool Call Pattern)](../architecture/decisions/DD-CONTEXT-003-Context-Enrichment-Placement.md)
- [DD-CONTEXT-005: Minimal LLM Response Schema (Filter Before LLM)](../architecture/decisions/DD-CONTEXT-005-minimal-llm-response-schema.md)
- [DD-CONTEXT-006: Signal Processor Recovery Data Source](../architecture/decisions/DD-CONTEXT-006-signal-processor-recovery-data-source.md)

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces **data-driven playbook selection** as the primary AI capability (90% of Hybrid Model). The **HolmesGPT API service** (on behalf of the LLM) must query historical playbook data from the Context API and use this data to select the most effective playbook for each incident type.

**Current Limitations**:
- ❌ AI lacks access to historical remediation effectiveness data
- ❌ Playbook selection is based on heuristics or random selection
- ❌ AI cannot learn from past remediation successes/failures
- ❌ No data-driven optimization of playbook selection
- ❌ Cannot validate if selected playbook historically works for this incident type

**Impact**:
- Suboptimal playbook selection (low success rate)
- AI cannot fulfill ADR-033 Hybrid Model (90% catalog-based selection)
- No continuous improvement through data-driven learning
- Users lack confidence in AI-selected remediations

---

## 🎯 **Business Objective**

**Enable HolmesGPT API (on behalf of the LLM) to query playbook data from Context API via tool calls and use this data to select the most effective playbook for each remediation request.**

**Architectural Pattern**: LLM-driven tool call pattern (DD-CONTEXT-003)
- AIAnalysis Controller sends investigation request to HolmesGPT API
- HolmesGPT API exposes Context API as a tool to the LLM
- LLM decides when to query Context API for playbooks
- LLM receives minimal 4-field response (DD-CONTEXT-005)
- LLM selects playbook based on `confidence` score (semantic + label matching)

### **Success Criteria**
1. ✅ LLM queries Context API for playbooks via HolmesGPT API tool call
2. ✅ Context API returns playbooks matching incident description (semantic search)
3. ✅ LLM selects playbook with highest `confidence` score
4. ✅ LLM includes confidence score in recommendation
5. ✅ LLM logs playbook selection rationale (data-driven decision)
6. ✅ 90%+ of AI remediation decisions use data-driven playbook selection (ADR-033 target)
7. ✅ Measurable improvement in remediation success rate (10%+ increase)

---

## 📊 **Use Cases**

### **Use Case 1: Data-Driven Playbook Selection for Known Incident**

**Scenario**: LLM receives `pod-oom-killer` alert and selects playbook based on Context API data.

**Current Flow** (Without BR-AI-057):
```
1. AIAnalysis Controller sends investigation request to HolmesGPT API
2. HolmesGPT API sends prompt to LLM with incident context
3. ❌ LLM has no playbook data
4. ❌ LLM selects based on general knowledge (may be incorrect)
5. Low remediation success rate
```

**Desired Flow with BR-AI-057** (LLM-Driven Tool Call Pattern):
```
1. AIAnalysis Controller sends investigation request to HolmesGPT API
2. HolmesGPT API sends prompt to LLM with incident context:
   - Incident: pod-oom-killer
   - Description: "High memory usage causing pod restarts"
   - Environment: production
   - Priority: P0
3. ✅ LLM decides to query for playbooks (tool call)
4. ✅ LLM calls tool: get_playbooks(description="High memory usage causing pod restarts",
                                    labels=["kubernaut.ai/incident-type:pod-oom-killer",
                                            "kubernaut.ai/environment:production",
                                            "kubernaut.ai/priority:P0"])
5. ✅ HolmesGPT API queries Context API:
   GET /api/v1/context/playbooks?description=High+memory+usage+causing+pod+restarts
                                 &labels=kubernaut.ai/incident-type:pod-oom-killer
                                 &labels=kubernaut.ai/environment:production
                                 &labels=kubernaut.ai/priority:P0
6. ✅ Context API returns (minimal 4-field schema per DD-CONTEXT-005):
   {
     "playbooks": [
       {
         "playbook_id": "pod-oom-recovery",
         "version": "v1.2",
         "description": "Increases memory limits and restarts pod",
         "confidence": 0.92  // semantic (0.88) + label match (1.0) + historical (0.95)
       },
       {
         "playbook_id": "pod-force-delete-recovery",
         "version": "v1.0",
         "description": "Force deletes pod and recreates with higher limits",
         "confidence": 0.78  // semantic (0.85) + label match (0.95) + historical (0.50)
       }
     ]
   }
7. ✅ LLM receives playbook list and reasons about selection
8. ✅ LLM selects pod-oom-recovery v1.2 (highest confidence: 0.92)
9. ✅ LLM logs decision: "Selected pod-oom-recovery v1.2 (confidence: 0.92, best semantic match)"
10. ✅ Higher remediation success rate (data-driven selection)
```

**Key Differences from Old Approach**:
- ✅ LLM decides when to query (not pre-enriched)
- ✅ Minimal response schema (4 fields only)
- ✅ `confidence` score (NOT `success_rate` - avoids feedback loop per DD-CONTEXT-005)
- ✅ Filtering via query parameters (environment, priority) before LLM

---

### **Use Case 2: New Playbook - Graceful Degradation**

**Scenario**: LLM queries for playbooks for a new incident type with limited historical data.

**Desired Flow with BR-AI-057** (LLM-Driven Tool Call Pattern):
```
1. AIAnalysis Controller sends investigation request to HolmesGPT API
2. LLM calls tool: get_playbooks(description="rare database failure",
                                 labels=["kubernaut.ai/incident-type:database-failure"])
3. ✅ Context API returns playbooks (including new ones):
   {
     "playbooks": [
       {
         "playbook_id": "database-recovery",
         "version": "v1.0",
         "description": "Restarts database and validates connections",
         "confidence": 0.65  // semantic (0.80) + label match (1.0) + historical (0.0 - new playbook)
       }
     ]
   }
4. ✅ LLM receives playbook with lower confidence (new playbook, no historical data)
5. ✅ LLM reasons about the playbook based on description and current situation
6. ✅ LLM selects database-recovery v1.0 with caveat
7. ✅ LLM logs decision: "Selected database-recovery v1.0 (confidence: 0.65, new playbook with no execution history)"
8. ✅ Operator receives recommendation with confidence score
```

**Key Point**: Context API ALWAYS includes new playbooks (per DD-CONTEXT-005). The `confidence` score reflects lack of historical data, but the LLM can still reason about the playbook based on its description.

---

### **Use Case 3: Multiple Playbook Comparison**

**Scenario**: LLM queries for multiple playbooks and compares them to select the best one.

**Desired Flow with BR-AI-057** (LLM-Driven Tool Call Pattern):
```
1. AIAnalysis Controller sends investigation request to HolmesGPT API
2. LLM calls tool: get_playbooks(description="pod memory pressure",
                                 labels=["kubernaut.ai/incident-type:pod-oom-killer",
                                         "kubernaut.ai/environment:production"])
3. ✅ Context API returns multiple playbooks:
   {
     "playbooks": [
       {
         "playbook_id": "pod-oom-recovery",
         "version": "v1.2",
         "description": "Increases memory limits and restarts pod",
         "confidence": 0.92
       },
       {
         "playbook_id": "pod-oom-vertical-scaling",
         "version": "v1.0",
         "description": "Scales pod vertically with higher memory allocation",
         "confidence": 0.88
       },
       {
         "playbook_id": "pod-force-delete-recovery",
         "version": "v1.0",
         "description": "Force deletes pod and recreates",
         "confidence": 0.75
       }
     ]
   }
4. ✅ LLM receives multiple options and reasons about them
5. ✅ LLM considers confidence scores AND incident context
6. ✅ LLM selects pod-oom-recovery v1.2 (highest confidence + best match for description)
7. ✅ LLM logs decision: "Selected pod-oom-recovery v1.2 (confidence: 0.92, best match for memory pressure incident)"
8. ✅ Data-driven selection with LLM reasoning
```

**Key Point**: LLM can query once and receive multiple playbooks, then reason about which is best for the current situation. This is more efficient than multiple queries and allows LLM to use its reasoning capabilities.

---

## 🔧 **Functional Requirements**

### **FR-AI-057-01: HolmesGPT API Tool for Context API Playbook Query**

**Requirement**: HolmesGPT API SHALL expose Context API as a tool to the LLM for playbook queries.

**Implementation Example** (HolmesGPT API - Python):
```python
# holmesgpt-api/src/tools/context_playbook_tool.py

from holmesgpt import Tool, ToolParameter

class ContextPlaybookTool(Tool):
    """Tool for LLM to query Context API for remediation playbooks."""

    def __init__(self, context_api_client):
        super().__init__(
            name="get_playbooks",
            description=(
                "Query Context API for remediation playbooks matching the incident. "
                "Use this when you need to find playbooks to remediate an incident. "
                "Provide incident description and relevant labels (environment, priority, etc.)."
            ),
            parameters=[
                ToolParameter(
                    name="description",
                    type="string",
                    description="Incident description for semantic search",
                    required=True
                ),
                ToolParameter(
                    name="labels",
                    type="array",
                    description="Label filters (e.g., kubernaut.ai/environment:production)",
                    required=False
                ),
                ToolParameter(
                    name="min_confidence",
                    type="float",
                    description="Minimum confidence score (0.0-1.0)",
                    required=False,
                    default=0.7
                ),
                ToolParameter(
                    name="max_results",
                    type="integer",
                    description="Maximum playbooks to return",
                    required=False,
                    default=10
                )
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

        # Add label filters
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
        return response.json()  # {"playbooks": [{"playbook_id", "version", "description", "confidence"}]}

# Register tool with HolmesGPT SDK
holmes_client.register_tool(ContextPlaybookTool(context_api_client))
```

**Acceptance Criteria**:
- ✅ Tool exposed to LLM via HolmesGPT SDK
- ✅ LLM can call tool with incident description and labels
- ✅ Tool queries Context API with proper parameters
- ✅ Tool returns minimal 4-field schema (per DD-CONTEXT-005)
- ✅ Tool handles Context API errors gracefully
- ✅ Tool logs all queries for observability

---

### **FR-AI-057-02: LLM Playbook Selection Based on Confidence**

**Requirement**: LLM SHALL select playbooks based on `confidence` score returned by Context API.

**Note**: Confidence calculation happens in **Context API**, not in HolmesGPT API or AIAnalysis Controller. Context API calculates confidence as:
- **Semantic Similarity** (0.0-1.0): How well playbook description matches incident description
- **Label Match** (0.0-1.0): How many required labels match
- **Historical Performance** (0.0-1.0): Success rate from Effectiveness Monitor (NOT exposed to LLM per DD-CONTEXT-005)

**Confidence Formula** (in Context API):
```
confidence = (semantic_similarity * 0.4) + (label_match * 0.4) + (historical_performance * 0.2)
```

**LLM Selection Logic**:
```python
# In HolmesGPT API - LLM reasoning
def select_playbook(playbooks: list) -> dict:
    """LLM selects playbook from Context API results."""

    # LLM receives playbooks sorted by confidence (highest first)
    # LLM reasons about:
    # 1. Confidence score (primary factor)
    # 2. Playbook description match to incident
    # 3. Current incident context

    # Example LLM reasoning:
    # "I received 3 playbooks. The highest confidence (0.92) is 'pod-oom-recovery v1.2'
    #  which matches the incident description 'High memory usage causing pod restarts'.
    #  I will select this playbook."

    selected = playbooks[0]  # Highest confidence

    return {
        "playbook_id": selected["playbook_id"],
        "version": selected["version"],
        "confidence": selected["confidence"],
        "reasoning": f"Selected {selected['playbook_id']} (confidence: {selected['confidence']:.2f})"
    }
```

**Acceptance Criteria**:
- ✅ LLM receives playbooks with `confidence` score
- ✅ LLM selects playbook with highest confidence (typically)
- ✅ LLM can override confidence if incident context suggests different playbook
- ✅ LLM logs selection reasoning
- ✅ Confidence score NOT based on `success_rate` directly (avoids feedback loop per DD-CONTEXT-005)

---

### **FR-AI-057-03: Graceful Degradation for No Playbooks**

**Requirement**: HolmesGPT API tool SHALL handle cases where Context API returns no playbooks.

**Fallback Strategy**:
1. **No playbooks found**: Context API returns empty list
2. **LLM receives empty list**: LLM reasons about lack of playbooks
3. **LLM recommendation**: Recommend manual investigation or suggest creating new playbook

**Implementation** (HolmesGPT API - Python):
```python
def execute(self, description: str, labels: list = None,
            min_confidence: float = 0.7, max_results: int = 10):
    """Execute tool call to query Context API for playbooks."""

    # Query Context API
    response = self.context_api_client.get(
        "/api/v1/context/playbooks",
        params=params,
        timeout=2.0
    )

    if response.status_code != 200:
        # Context API error - return error to LLM
        return {
            "error": f"Context API unavailable: {response.status_code}",
            "playbooks": [],
            "message": "Context API is unavailable. Consider manual investigation."
        }

    result = response.json()

    if len(result.get("playbooks", [])) == 0:
        # No playbooks found - return empty list to LLM
        return {
            "playbooks": [],
            "total_results": 0,
            "message": "No playbooks found matching criteria. Consider manual investigation or creating new playbook."
        }

    # Return playbooks to LLM
    return result
```

**LLM Reasoning with No Playbooks**:
```
LLM receives: {"playbooks": [], "message": "No playbooks found..."}

LLM reasoning:
"I queried Context API for playbooks matching 'rare database failure' but found no results.
This incident type may be new or uncommon. I recommend manual investigation by the operator
to determine the root cause and create a new playbook for future incidents."

LLM response:
{
  "recommendation": "manual_investigation",
  "reasoning": "No playbooks found for this incident type",
  "suggested_actions": ["investigate logs", "check database status", "create new playbook"]
}
```

**Acceptance Criteria**:
- ✅ Tool returns empty list if no playbooks found
- ✅ Tool provides helpful message to LLM
- ✅ LLM can reason about lack of playbooks
- ✅ LLM recommends manual investigation or playbook creation
- ✅ Logs all "no playbooks" cases for observability

---

## 📈 **Non-Functional Requirements**

### **NFR-AI-057-01: Performance**

- ✅ Playbook selection latency <500ms (including Context API queries)
- ✅ Parallel queries to Context API for multiple playbooks
- ✅ Cache success rates for 5 minutes (reduce Context API load)

### **NFR-AI-057-02: Reliability**

- ✅ Graceful degradation if Context API unavailable (use fallback)
- ✅ Timeout for Context API queries (2 seconds)
- ✅ Retry logic for transient failures (3 retries with exponential backoff)

### **NFR-AI-057-03: Observability**

- ✅ Log every playbook selection decision with rationale
- ✅ Prometheus metrics: `ai_playbook_selections_total{incident_type="...", playbook_id="...", selection_method="data_driven|fallback"}`
- ✅ Track success rate usage: `ai_success_rate_queries_total{status="success|error"}`

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Playbook Catalog (defines data-driven selection as primary AI capability)
- ✅ BR-INTEGRATION-008: Context API exposes incident-type success rate endpoint
- ✅ BR-PLAYBOOK-001: Playbook Catalog provides available playbooks

### **Downstream Impacts**
- ✅ BR-REMEDIATION-016: RemediationExecutor uses AI-selected playbook
- ✅ BR-EFFECTIVENESS-002: Effectiveness Monitor tracks AI selection accuracy

---

## 🚀 **Implementation Phases**

### **Phase 1: Context API Client** (Day 10 - 4 hours)
- Implement Context API client for success rate queries
- Add retry logic and timeout handling
- Add unit tests

### **Phase 2: Selection Logic** (Day 11 - 4 hours)
- Implement `SelectPlaybook()` with success rate comparison
- Implement confidence calculation
- Add fallback strategies

### **Phase 3: Integration** (Day 12 - 4 hours)
- Integrate with existing AI recommendation flow
- Add logging and metrics
- Integration tests with real Context API

### **Phase 4: Monitoring** (Day 13 - 2 hours)
- Add Prometheus metrics
- Add alerting for high fallback usage
- Dashboard for playbook selection analytics

**Total Estimated Effort**: 14 hours (1.75 days)

---

## 📊 **Success Metrics**

### **Data-Driven Selection Rate**
- **Target**: 90%+ of remediation decisions use data-driven playbook selection
- **Measure**: `ai_playbook_selections_total{selection_method="data_driven"}` / total selections

### **Remediation Success Rate Improvement**
- **Target**: 10%+ increase in overall remediation success rate
- **Measure**: Compare success rate before/after BR-AI-057 implementation

### **Context API Query Success**
- **Target**: 95%+ Context API queries succeed
- **Measure**: `ai_success_rate_queries_total{status="success"}` / total queries

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Hardcoded Playbook Priority**

**Approach**: Maintain hardcoded priority list of playbooks per incident type

**Rejected Because**:
- ❌ Cannot learn from historical data
- ❌ Requires manual updates for every infrastructure change
- ❌ No continuous improvement

---

### **Alternative 2: AI Learns Without Success Rates**

**Approach**: AI uses LLM reasoning without historical data

**Rejected Because**:
- ❌ LLM lacks context-specific remediation effectiveness knowledge
- ❌ Cannot validate effectiveness of selected playbook
- ❌ Lower success rate than data-driven approach

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P0 priority (core of ADR-033 Hybrid Model)
**Rationale**: Enables 90% of AI decisions to be data-driven (ADR-033 target)
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-INTEGRATION-008: Context API exposes incident-type success rate endpoint
- BR-PLAYBOOK-001: Playbook Catalog provides available playbooks
- BR-STORAGE-031-01: Data Storage incident-type success rate API
- BR-STORAGE-031-02: Data Storage playbook success rate API

### **Related Documents**
- [ADR-033: Remediation Playbook Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

