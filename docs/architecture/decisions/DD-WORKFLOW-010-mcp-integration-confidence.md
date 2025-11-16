# DD-WORKFLOW-010: MCP Workflow Catalog Integration in HolmesGPT API - Confidence Assessment

**Date**: 2025-11-15  
**Status**: ✅ Approved  
**Target Version**: v1.0 (MVP)  
**Related**: DD-WORKFLOW-008, DD-WORKFLOW-009, DD-EMBEDDING-001, DD-STORAGE-008

---

## Proposal

**Integrate MCP workflow catalog search logic directly inside holmesgpt-api instead of developing a separate MCP service wrapper.**

**Rationale**: Avoid building an unnecessary service layer on top of existing Data Storage Service REST API.

---

## Current Architecture (v1.0 - Actual)

```
┌─────────────────────────────────────────────────────────────┐
│ HolmesGPT API                                                │
│ - recovery.py (RCA orchestration)                           │
│ - mcp_client.py (calls Mock MCP Server)                     │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ HTTP POST /mcp/tools/search_workflow_catalog
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Mock MCP Server (Python Flask) - TEMPORARY TEST SERVICE     │
│ - In-memory workflow dictionary                             │
│ - Hardcoded search logic with scoring                       │
│ - Returns matching playbooks                                │
└─────────────────────────────────────────────────────────────┘
```

**But v1.0 Already Has (Per DD-STORAGE-008)**:
```
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service (ALREADY EXISTS in v1.0)               │
│ - GET /api/v1/playbooks/search (semantic search)            │
│ - Label-based filtering (JSONB with GIN index)              │
│ - PostgreSQL + pgvector backend                             │
│ - Workflow catalog table with full schema                   │
│ - Lifecycle management (active/disabled/deprecated)         │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Calls for embedding generation
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Embedding Service (PLANNED - DD-EMBEDDING-001)              │
│ - POST /api/v1/embed                                        │
│ - sentence-transformers/all-MiniLM-L6-v2 (384 dims)         │
│ - Python microservice                                       │
└─────────────────────────────────────────────────────────────┘
```

**Current Problem**:
- **Mock MCP Server** is a temporary test service duplicating workflow search logic
- **Data Storage Service** already provides semantic search REST API with label filtering
- **Embedding Service** is planned for v1.0 to generate embeddings
- Mock MCP Server is an **unnecessary wrapper** over existing infrastructure

---

## Proposed Architecture (v1.0 with Integration)

```
┌─────────────────────────────────────────────────────────────┐
│ HolmesGPT API                                                │
│                                                              │
│ ┌──────────────────────────────────────────────────────┐    │
│ │ recovery.py (RCA orchestration)                      │    │
│ └────────────┬─────────────────────────────────────────┘    │
│              │                                               │
│              ▼                                               │
│ ┌──────────────────────────────────────────────────────┐    │
│ │ playbook_mcp.py (NEW - MCP tool implementation)      │    │
│ │ - MCP tool: search_workflow_catalog                  │    │
│ │ - Calls Data Storage REST API                        │    │
│ │ - No database access                                 │    │
│ │ - No embedding generation                            │    │
│ └────────────┬─────────────────────────────────────────┘    │
│              │                                               │
└──────────────┼───────────────────────────────────────────────┘
               │
               │ HTTP GET /api/v1/playbooks/search
               ▼
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service (ALREADY EXISTS)                       │
│ - Semantic search with label filtering                      │
│ - PostgreSQL + pgvector queries                             │
│ - Calls Embedding Service for query embeddings              │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ POST /api/v1/embed
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Embedding Service (PLANNED)                                 │
│ - Generate query embeddings                                 │
└─────────────────────────────────────────────────────────────┘
```

**Proposed State**:
- **Remove** Mock MCP Server as separate service
- **Add** `playbook_mcp.py` module inside holmesgpt-api
- **Call** Data Storage Service REST API (no direct database access)
- **Leverage** existing embedding service (no embedding generation in holmesgpt-api)
- **Eliminate** unnecessary service wrapper

---

## Confidence Assessment

### ✅ **Pros (Very High Confidence: 90-98%)**

#### 1. **Reduced Development Time** (98% confidence)
- **Benefit**: No need to build separate MCP service wrapper
- **Evidence**: 
  - Mock MCP Server is currently ~200 lines of Python
  - Integration would be ~100 lines in `playbook_mcp.py` (just HTTP client calls)
  - Data Storage Service already provides all search logic
  - Embedding Service already handles embedding generation
- **Time Savings**: ~3-4 days of development + deployment work

#### 2. **Lower Latency** (95% confidence)
- **Benefit**: Eliminates one HTTP hop
- **Current**: HolmesGPT API → Mock MCP → Data Storage → Embedding Service
- **Proposed**: HolmesGPT API → Data Storage → Embedding Service
- **Estimated Improvement**: 20-100ms saved per workflow search
- **Impact**: Faster RCA response times

#### 3. **Simplified Deployment** (98% confidence)
- **Benefit**: One less service to deploy, monitor, and maintain
- **Current**: 3 services (holmesgpt-api + mock-mcp-server + data-storage + embedding-service)
- **Proposed**: 3 services (holmesgpt-api + data-storage + embedding-service)
- **Operational Savings**: 
  - Fewer pods to manage
  - Simpler service mesh configuration
  - Reduced resource consumption

#### 4. **Easier Debugging** (90% confidence)
- **Benefit**: Fewer service boundaries to cross
- **Current**: Debug across 4 services with HTTP boundaries
- **Proposed**: Debug across 3 services
- **Developer Experience**: Simpler stack traces, fewer network issues

#### 5. **Consistent with Existing Pattern** (95% confidence)
- **Evidence**: HolmesGPT API already integrates toolsets:
  - Kubernetes toolset (embedded, calls K8s API)
  - Prometheus toolset (embedded, calls Prometheus API)
  - LLM client (embedded, calls Anthropic API)
- **Pattern**: Toolsets are embedded HTTP clients, not separate services
- **Alignment**: Workflow search is another "toolset" calling existing REST API

#### 6. **No Database Coupling** (98% confidence)
- **Benefit**: HolmesGPT API calls REST API, not PostgreSQL directly
- **Architecture**: Clean separation via Data Storage Service
- **Evidence**: Per DD-STORAGE-008, all database access is via REST API
- **Migration-Proof**: v1.1 changes to Data Storage don't affect holmesgpt-api

#### 7. **No Embedding Logic Duplication** (98% confidence)
- **Benefit**: Embedding Service handles all embedding generation
- **Architecture**: HolmesGPT API never generates embeddings
- **Evidence**: Per DD-EMBEDDING-001, Embedding Service is centralized
- **Simplicity**: HolmesGPT API is just an HTTP client

---

### ⚠️ **Cons (Low-Medium Confidence: 70-80%)**

#### 1. **Violates Service Separation Principle** (75% confidence)
- **Risk**: HolmesGPT API takes on workflow search responsibility
- **Current Responsibilities**: RCA orchestration, LLM interaction
- **Added Responsibility**: MCP tool implementation for workflow search
- **Concern**: "God object" anti-pattern
- **Counter-Argument**: 
  - Workflow search is a "tool" for RCA, not a separate domain
  - Similar to how Kubernetes/Prometheus toolsets are embedded
  - Just an HTTP client wrapper, not business logic
- **Severity**: Low (acceptable for MVP, consistent with toolset pattern)

#### 2. **Testing Complexity** (80% confidence)
- **Risk**: Testing workflow search requires Data Storage Service
- **Current**: Mock MCP Server can be mocked easily (HTTP interface)
- **Proposed**: Need to mock Data Storage REST API
- **Mitigation**: 
  - **Unit Tests**: Mock HTTP client (standard practice)
  - **Business Requirement Tests**: Real Data Storage Service (per TESTING_GUIDELINES.md)
  - **Per TESTING_GUIDELINES.md**: Use appropriate mocks for test type
- **Severity**: Low (standard practice, aligns with testing strategy)

#### 3. **Reusability** (70% confidence)
- **Risk**: Workflow search logic only available to HolmesGPT API
- **Alternative**: Separate MCP service could be reused by other services
- **Counter-Argument**:
  - Other services can call Data Storage REST API directly
  - MCP tool wrapper is specific to HolmesGPT's LLM integration
  - No other service currently needs MCP-style workflow search
- **Severity**: Low (no current reusability requirement)

---

## Alternative: Keep Separate MCP Service

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ HolmesGPT API                                                │
│ - recovery.py                                               │
│ - mcp_client.py                                             │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ HTTP POST /mcp/tools/search_workflow_catalog
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ MCP Workflow Catalog Service (Separate Service)             │
│ - MCP server implementation                                 │
│ - Calls Data Storage REST API                               │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ HTTP GET /api/v1/playbooks/search
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service                                         │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ POST /api/v1/embed
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Embedding Service                                            │
└─────────────────────────────────────────────────────────────┘
```

### Pros of Separate Service
- ✅ **Service Separation**: Clear boundaries, single responsibility
- ✅ **Independent Scaling**: Scale MCP service independently
- ✅ **Reusability**: Other services can use MCP Workflow Catalog

### Cons of Separate Service
- ❌ **Higher Development Time**: 3-4 extra days for service infrastructure
- ❌ **Higher Latency**: Additional HTTP hop (20-100ms)
- ❌ **More Operational Complexity**: Additional deployment, monitoring
- ❌ **Overkill for MVP**: MCP service is just a thin wrapper over Data Storage API
- ❌ **Duplicates Existing Pattern**: Kubernetes/Prometheus toolsets are embedded

---

## Recommended Approach

### **Option A: Integrated for v1.0 MVP** ✅ **RECOMMENDED**

**Confidence: 92%**

**Implementation**:
1. Add `playbook_mcp.py` module to holmesgpt-api
2. Implement MCP tool: `search_workflow_catalog`
3. Call Data Storage Service REST API (`GET /api/v1/playbooks/search`)
4. No database access, no embedding generation
5. Remove Mock MCP Server

**Rationale**:
- **MVP Priority**: Speed to market, reduce complexity
- **Low Risk**: Just an HTTP client wrapper (~100 lines)
- **Clean Architecture**: Uses existing Data Storage REST API
- **Consistent Pattern**: Matches existing toolset integration (Kubernetes, Prometheus)
- **No Coupling**: No database or embedding logic in holmesgpt-api

**Code Structure**:
```python
# holmesgpt-api/src/toolsets/playbook_mcp.py

import httpx
from typing import List, Dict, Any
import logging

logger = logging.getLogger(__name__)

class PlaybookMCPTool:
    """
    MCP tool for workflow catalog search.
    Calls Data Storage Service REST API.
    """
    
    def __init__(self, data_storage_url: str):
        self.data_storage_url = data_storage_url
        self.client = httpx.AsyncClient(timeout=30.0)
    
    async def search_workflow_catalog(
        self,
        query: str,
        labels: Dict[str, str],
        min_confidence: float = 0.7,
        max_results: int = 5
    ) -> List[Dict[str, Any]]:
        """
        Search workflow catalog via Data Storage Service.
        
        Args:
            query: Incident description for semantic search
            labels: Label filters (environment, priority, etc.)
            min_confidence: Minimum similarity threshold (0.0-1.0)
            max_results: Maximum number of results
        
        Returns:
            List of matching playbooks with confidence scores
        """
        # Build query parameters
        params = {
            "query": query,
            "min_confidence": min_confidence,
            "max_results": max_results
        }
        
        # Add label filters
        for key, value in labels.items():
            params[f"label.{key}"] = value
        
        # Call Data Storage Service
        url = f"{self.data_storage_url}/api/v1/playbooks/search"
        
        try:
            response = await self.client.get(url, params=params)
            response.raise_for_status()
            
            result = response.json()
            playbooks = result.get("playbooks", [])
            
            logger.info({
                "event": "playbook_search_success",
                "playbooks_found": len(playbooks),
                "query": query,
                "labels": labels
            })
            
            return playbooks
            
        except httpx.RequestError as e:
            logger.error({
                "event": "playbook_search_error",
                "error": str(e),
                "url": url
            })
            return []
        except httpx.HTTPStatusError as e:
            logger.error({
                "event": "playbook_search_http_error",
                "status_code": e.response.status_code,
                "response_text": e.response.text,
                "url": url
            })
            return []
```

**Integration in recovery.py**:
```python
# holmesgpt-api/src/extensions/recovery.py

from toolsets.playbook_mcp import PlaybookMCPTool

async def analyze_recovery(request_data: Dict[str, Any], app_config: Dict[str, Any]):
    # Initialize workflow MCP tool
    data_storage_url = app_config.get("data_storage", {}).get("url")
    playbook_tool = PlaybookMCPTool(data_storage_url)
    
    # Extract search criteria from incident
    labels = {
        "environment": request_data["context"].get("environment", "*"),
        "priority": request_data["context"].get("priority", "*"),
        "risk_tolerance": request_data["context"].get("risk_tolerance", "*"),
        "business_category": request_data["context"].get("business_category", "*"),
    }
    
    # Search playbooks
    playbooks = await playbook_tool.search_workflow_catalog(
        query=request_data.get("description", ""),
        labels=labels,
        min_confidence=0.7,
        max_results=5
    )
    
    # Include playbooks in LLM prompt
    # ... (rest of RCA logic)
```

---

## Risk Mitigation

### Risk 1: Data Storage API Changes
**Mitigation**: Data Storage REST API is stable (v1.0 contract)  
**Effort**: Minimal (just HTTP client updates)  
**Confidence**: 95%

### Risk 2: Testing Complexity
**Mitigation**: Follow TESTING_GUIDELINES.md (mock HTTP for unit tests, real service for BR tests)  
**Effort**: Standard practice  
**Confidence**: 90%

### Risk 3: Service Separation
**Mitigation**: Keep workflow MCP logic in separate module (`playbook_mcp.py`)  
**Effort**: Design decision (no extra work)  
**Confidence**: 98%

---

## Decision Matrix

| Criterion | Integrated (Option A) | Separate Service (Option B) |
|---|---|---|
| **Development Time** | ✅ 1 day | ❌ 3-4 days |
| **Latency** | ✅ Low (2 hops) | ⚠️ Medium (3 hops) |
| **Operational Complexity** | ✅ Low (3 services) | ❌ High (4 services) |
| **Service Separation** | ⚠️ Medium (acceptable) | ✅ High (clean boundaries) |
| **Testing Complexity** | ✅ Low (mock HTTP) | ✅ Low (mock HTTP) |
| **Database Coupling** | ✅ None (REST API only) | ✅ None (REST API only) |
| **Embedding Coupling** | ✅ None (Embedding Service) | ✅ None (Embedding Service) |
| **Reusability** | ⚠️ Low (HolmesGPT only) | ✅ High (any service) |
| **MVP Suitability** | ✅ **Excellent** | ⚠️ Overkill |

---

## Final Recommendation

### ✅ **RECOMMENDED: Integrate MCP Workflow Tool in HolmesGPT API (Option A)**

**Overall Confidence: 92%**

**Justification**:
1. **MVP Priority**: Speed to market is critical for v1.0
2. **Clean Architecture**: Uses existing Data Storage REST API (no database coupling)
3. **No Embedding Logic**: Embedding Service handles all embedding generation
4. **Consistent Pattern**: Matches existing toolset integration (Kubernetes, Prometheus)
5. **Operational Simplicity**: One less service to deploy and monitor
6. **Low Complexity**: Just an HTTP client wrapper (~100 lines)

**Confidence Breakdown**:
- Development time savings: 98%
- Latency improvement: 95%
- Deployment simplification: 98%
- Clean architecture (REST API): 98%
- No database coupling: 98%
- No embedding coupling: 98%
- Testing approach: 90%
- Service separation: 75%
- **Overall**: 92%

**Gap to 100%**:
- 5%: Service separation principle (mitigated by toolset pattern)
- 3%: Reusability (no current requirement for other services)

**Actionable Mitigations**:
1. ✅ Keep workflow MCP logic in separate module (`playbook_mcp.py`)
2. ✅ Use Data Storage REST API only (no database access)
3. ✅ Follow TESTING_GUIDELINES.md for unit vs. BR tests
4. ✅ Document as "toolset" pattern (consistent with Kubernetes/Prometheus)

---

## Implementation Plan (v1.0 MVP)

### Phase 1: Integration (1 day)
- [ ] Create `holmesgpt-api/src/toolsets/playbook_mcp.py`
- [ ] Implement `PlaybookMCPTool` class with `search_workflow_catalog` method
- [ ] Call Data Storage REST API (`GET /api/v1/playbooks/search`)
- [ ] Update `recovery.py` to use `PlaybookMCPTool`
- [ ] Remove Mock MCP Server deployment

### Phase 2: Testing (0.5 days)
- [ ] Add unit tests with mocked HTTP client
- [ ] Add business requirement tests with real Data Storage Service
- [ ] Test end-to-end RCA with workflow search

**Total Effort**: 1.5 days for v1.0

---

## v2.0 Strategic Evaluation (Deferred)

**Per DD-WORKFLOW-008**:
- Evaluate toolset integration strategy (embedded vs. MCP)
- Evaluate HolmesGPT replacement with custom agent
- Analyze performance, maintenance, security implications

**Timeline**: After v1.1 release

---

## Summary

**Recommendation**: Integrate MCP workflow tool in holmesgpt-api for v1.0 MVP  
**Confidence**: 92%  
**Development Time**: 1.5 days (vs. 3-4 days for separate service)  
**Architecture**: Clean (REST API only, no database/embedding coupling)  
**Risk Level**: Low (just HTTP client wrapper)  

**Status**: ✅ Ready for implementation in v1.0
