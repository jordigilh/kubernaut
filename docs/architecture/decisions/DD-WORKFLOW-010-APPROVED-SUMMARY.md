# DD-WORKFLOW-010: MCP Integration Approval Summary

**Date**: 2025-11-15  
**Status**: ✅ **APPROVED**  
**Decision**: Integrate MCP workflow catalog toolset in HolmesGPT API  
**Confidence**: 92%

---

## Decision

**APPROVED**: Integrate MCP workflow catalog search as a toolset within holmesgpt-api instead of maintaining a separate MCP service.

---

## Context

### MVP Validation (Completed)
The Mock MCP Server successfully validated the MVP:
- ✅ **LLM RCA Capability**: Demonstrated LLM can perform Root Cause Analysis using Kubernetes toolsets
- ✅ **Playbook Recommendations**: Demonstrated LLM can recommend remediation based on workflow catalog
- ✅ **End-to-End Flow**: Validated HolmesGPT API → Mock MCP → Workflow recommendations workflow

### Production Architecture (v1.0)
For production, we will:
- **Remove** Mock MCP Server (served its validation purpose)
- **Integrate** MCP workflow toolset directly in holmesgpt-api
- **Leverage** existing Data Storage Service REST API
- **Use** existing Embedding Service for semantic search

---

## Architecture

### Current (MVP Validation)
```
HolmesGPT API → Mock MCP Server (in-memory playbooks)
```

### Approved (v1.0 Production)
```
HolmesGPT API (with integrated workflow toolset)
    ↓
Data Storage Service REST API
    ↓
Embedding Service + PostgreSQL/pgvector
```

---

## Key Benefits

1. **Clean Architecture** (98% confidence)
   - Uses existing Data Storage REST API
   - No direct database coupling
   - No embedding logic duplication

2. **Reduced Complexity** (98% confidence)
   - One less service to deploy and maintain
   - Eliminates unnecessary wrapper layer

3. **Lower Latency** (95% confidence)
   - Removes one HTTP hop
   - 20-100ms improvement per search

4. **Faster Development** (98% confidence)
   - 1.5 days vs. 3-4 days for separate service
   - Just HTTP client wrapper (~100 lines)

5. **Consistent Pattern** (95% confidence)
   - Matches existing toolset integration (Kubernetes, Prometheus)
   - Toolsets are embedded, not separate services

---

## Implementation

### Code Structure
```python
# holmesgpt-api/src/toolsets/playbook_mcp.py
class PlaybookMCPTool:
    def __init__(self, data_storage_url: str):
        self.data_storage_url = data_storage_url
    
    async def search_workflow_catalog(
        self,
        query: str,
        labels: Dict[str, str],
        min_confidence: float = 0.7,
        max_results: int = 5
    ) -> List[Dict[str, Any]]:
        # Calls Data Storage REST API
        response = await httpx.get(
            f"{self.data_storage_url}/api/v1/playbooks/search",
            params={...}
        )
        return response.json()
```

### Integration Points
- **Data Storage Service**: `GET /api/v1/playbooks/search` (already exists)
- **Embedding Service**: Used by Data Storage (already planned)
- **PostgreSQL + pgvector**: Backend storage (already exists)

---

## Testing Strategy

Per TESTING_GUIDELINES.md:

### Unit Tests
- Mock HTTP client
- Test workflow toolset logic
- Fast execution (<100ms)

### Business Requirement Tests
- Real Data Storage Service
- Validate end-to-end workflow search
- Test business value delivery

---

## Timeline

**Effort**: 1.5 days for v1.0 production implementation

### Phase 1: Integration (1 day)
- Create `playbook_mcp.py` toolset
- Implement `search_workflow_catalog` method
- Update `recovery.py` integration
- Remove Mock MCP Server deployment

### Phase 2: Testing (0.5 days)
- Unit tests with mocked HTTP
- Business requirement tests with real service
- End-to-end RCA validation

---

## Risk Assessment

### Low Risks (Mitigated)
1. **Service Separation** (75% confidence)
   - Mitigation: Keep as separate module, follows toolset pattern
   
2. **Testing Complexity** (80% confidence)
   - Mitigation: Follow TESTING_GUIDELINES.md

3. **Reusability** (70% confidence)
   - Mitigation: Other services can call Data Storage REST API directly

### No Risks
- ✅ **Database Coupling**: None (REST API only)
- ✅ **Embedding Coupling**: None (Embedding Service handles it)
- ✅ **Migration Risk**: None (Data Storage API is stable)

---

## Approval Criteria Met

- ✅ **MVP Validated**: Mock MCP Server successfully validated the approach
- ✅ **Clean Architecture**: Uses existing REST APIs, no coupling
- ✅ **Reduced Complexity**: Eliminates unnecessary service layer
- ✅ **Consistent Pattern**: Matches existing toolset integration
- ✅ **Low Risk**: Just HTTP client wrapper
- ✅ **Fast Implementation**: 1.5 days effort

---

## Next Steps

1. ✅ Document approval (this document)
2. ⏳ Implement `playbook_mcp.py` toolset
3. ⏳ Update `recovery.py` integration
4. ⏳ Add unit and BR tests
5. ⏳ Remove Mock MCP Server deployment
6. ⏳ Validate end-to-end RCA with integrated toolset

---

## Related Documents

- **DD-WORKFLOW-010**: Full confidence assessment (498 lines)
- **DD-WORKFLOW-008**: Version roadmap (v1.0, v1.1, v1.2, v2.0)
- **DD-STORAGE-008**: Workflow catalog schema
- **DD-EMBEDDING-001**: Embedding service architecture
- **TESTING_GUIDELINES.md**: Testing strategy

---

**Approved By**: Architecture Team  
**Approval Date**: 2025-11-15  
**Status**: ✅ Ready for v1.0 implementation
