# DD-WORKFLOW-014: Workflow Selection Audit Trail

**Date**: 2025-11-27
**Updated**: 2026-02-05
**Status**: ✅ **APPROVED**
**Version**: 3.0
**Authority**: Audit & Compliance
**Related**: DD-WORKFLOW-013 (Scoring Field Population), DD-AUDIT-001 (Audit Responsibility Pattern), ADR-034 (Unified Audit Trail), ADR-038 (Async Buffered Audit), DD-WORKFLOW-016 (Action-Type Indexing), DD-WORKFLOW-017 (Workflow Lifecycle)

---

## 📋 **Changelog**

| Version | Date | Changes |
|---------|------|---------|
| 3.0 | 2026-02-05 | **BREAKING**: Replaced single `workflow.catalog.search_completed` event with three step-specific events aligned with DD-WORKFLOW-016 three-step discovery protocol: `workflow.catalog.actions_listed` (Step 1), `workflow.catalog.workflows_listed` (Step 2), `workflow.catalog.workflow_retrieved` (Step 3). Added `workflow.catalog.selection_validated` for HAPI post-selection validation. All events remain DS-generated (V2.0 architecture preserved). |
| 2.1 | 2025-11-27 | **API DESIGN**: Changed remediation_id transport from HTTP header to JSON body for consistency with existing request patterns |
| 2.0 | 2025-11-27 | **CRITICAL**: Moved audit generation from HolmesGPT API to Data Storage Service for richer context (all workflow metadata, success metrics, version history) |
| 1.0 | 2025-11-27 | Initial design with HolmesGPT API as audit generator |

---

## 🎯 **Purpose**

Define how workflow selection decisions are captured in the audit trail, including scoring breakdown fields for debugging and tuning workflow definitions.

---

## 📋 **Business Requirements**

> **Authority**: [BR-AUDIT-021-030-WORKFLOW-SELECTION-AUDIT-TRAIL.md](../../requirements/BR-AUDIT-021-030-WORKFLOW-SELECTION-AUDIT-TRAIL.md)

This DD implements the following Business Requirements:

| BR ID | Requirement | Status |
|-------|-------------|--------|
| **BR-AUDIT-021** | Mandatory remediation_id propagation from HolmesGPT API | ✅ Implemented |
| **BR-AUDIT-022** | No audit generation in HolmesGPT API | ✅ Implemented |
| **BR-AUDIT-023** | Audit event generation in Data Storage Service | 🔄 Pending |
| **BR-AUDIT-024** | Asynchronous non-blocking audit (ADR-038) | 🔄 Pending |
| **BR-AUDIT-025** | Query metadata capture | 🔄 Pending |
| **BR-AUDIT-026** | Scoring breakdown capture | 🔄 Pending |
| **BR-AUDIT-027** | Workflow metadata capture | 🔄 Pending |
| **BR-AUDIT-028** | Search metadata capture | 🔄 Pending |
| **BR-AUDIT-029** | Audit data retention | 🔄 Pending |
| **BR-AUDIT-030** | Audit query API | 🔄 Pending |

**Use Cases**:
  1. **Debugging**: "Why wasn't my new workflow selected?"
  2. **Tuning**: "How can I improve my workflow's description/labels to rank higher?"
  3. **Compliance**: "What workflows were considered for this remediation?"
  4. **Learning**: "Which workflows are most frequently selected?"

---

## 🚨 **V2.0 ARCHITECTURAL CHANGE: Data Storage Service as Audit Generator**

### **Why This Change?**

**V1.0 (HolmesGPT API)**: Limited context - only sees search response

**V2.0 (Data Storage Service)**: Full context - has access to ALL workflow data

### **Context Comparison**

| Audit Data Point | HolmesGPT API | Data Storage Service |
|------------------|---------------|---------------------|
| Query & filters | ✅ | ✅ |
| remediation_id | ✅ | ✅ |
| Returned workflows | ✅ | ✅ |
| Scoring breakdown | ⚠️ Partial | ✅ Full |
| **All candidates considered** | ❌ | ✅ |
| **Workflow content** | ❌ | ✅ |
| **Content hash** | ❌ | ✅ |
| **Owner/maintainer** | ❌ | ✅ |
| **Version history** | ❌ | ✅ |
| **Success metrics** | ❌ | ✅ |
| **Disabled reason** | ❌ | ✅ |
| **Embedding vector** | ❌ | ✅ |
| **DB query timing** | ❌ | ✅ |

### **Key Benefits of V2.0**

1. **Richer Context**: Data Storage has access to ALL workflow metadata
2. **All Candidates**: Can audit ALL workflows considered (not just top_k)
3. **Success Metrics**: Includes historical success rates for learning/tuning
4. **Version History**: Includes why specific versions were selected
5. **Embedding Quality**: Includes embedding vectors for debugging semantic search
6. **Single Source of Truth**: Audit at data layer (consistent with ADR-032)
7. **Fire-and-Forget**: Uses ADR-038 async buffered pattern (non-blocking)

---

## 🎯 **Audit Event Specification (V2.0)**

### **Event Type**: `workflow.catalog.search_completed`

**When**: After Data Storage Service executes workflow search query

**Who Generates**: **Data Storage Service** (at the data layer)

**Authority**: ADR-032 (Data Access Layer Isolation)
- ✅ Data Storage Service has access to ALL workflow metadata
- ✅ Audit happens at data layer (consistent with other audit events)
- ✅ Uses ADR-038 async buffered pattern (non-blocking)

---

## 📊 **Audit Event Schema (V2.0 - Enhanced)**

### **Core Fields** (Standard Audit Trail)

```json
{
  "event_id": "uuid",
  "event_timestamp": "2025-11-27T10:30:00Z",
  "event_date": "2025-11-27",
  "event_type": "workflow.catalog.search_completed",
  "service": "datastorage",
  "correlation_id": "req-2025-11-27-abc123",
  "user_id": "system",
  "resource_type": "workflow_catalog",
  "resource_id": "search-query-hash",
  "action": "search",
  "outcome": "success",
  "severity": "info",
  "event_data": { /* See below */ }
}
```

---

### **event_data Schema (V2.0 - Enhanced)**

```json
{
  "query": {
    "text": "OOMKilled critical",
    "filters": {
      "signal-type": "OOMKilled",
      "severity": "critical",
      "resource-management": "gitops",
      "environment": "production"
    },
    "top_k": 3,
    "min_similarity": 0.7
  },

  "results": {
    "total_found": 5,
    "returned": 3,
    "candidates_considered": 5,

    "workflows": [
      {
        "workflow_id": "pod-oom-gitops",
        "version": "v1.0.0",
        "title": "Pod OOM GitOps Recovery",
        "rank": 1,

        // ========================================
        // SCORING BREAKDOWN (for debugging/tuning)
        // ========================================
        "scoring": {
          "confidence": 1.0,
          "base_similarity": 0.88,
          "label_boost": 0.18,
          "label_penalty": 0.0,

          "boost_breakdown": {
            "resource-management": 0.10,
            "environment": 0.08
          },
          "penalty_breakdown": {}
        },

        // ========================================
        // V2.0: FULL WORKFLOW METADATA (from database)
        // ========================================
        "labels": {
          "signal-type": "OOMKilled",
          "severity": "critical",
          "resource-management": "gitops",
          "gitops-tool": "argocd",
          "environment": "production"
        },

        // V2.0: Owner/Maintainer info
        "owner": "platform-team",
        "maintainer": "oncall@example.com",

        // V2.0: Version history
        "previous_version": "v0.9.0",
        "version_notes": "Added ArgoCD sync support",
        "approved_by": "security-team",
        "approved_at": "2025-11-20T10:00:00Z",

        // V2.0: Success metrics (for learning)
        "success_metrics": {
          "expected_success_rate": 0.95,
          "actual_success_rate": 0.92,
          "total_executions": 150,
          "successful_executions": 138
        },

        // V2.0: Content hash (for integrity verification)
        "content_hash": "sha256:abc123..."
      },
      {
        "workflow_id": "pod-oom-manual",
        "version": "v1.0.0",
        "title": "Pod OOM Manual Recovery",
        "rank": 2,

        "scoring": {
          "confidence": 0.78,
          "base_similarity": 0.88,
          "label_boost": 0.0,
          "label_penalty": 0.10,

          "boost_breakdown": {},
          "penalty_breakdown": {
            "resource-management": 0.10
          }
        },

        "labels": {
          "signal-type": "OOMKilled",
          "severity": "critical",
          "resource-management": "manual"
        },

        "owner": "sre-team",
        "maintainer": "sre@example.com",

        "success_metrics": {
          "expected_success_rate": 0.85,
          "actual_success_rate": 0.80,
          "total_executions": 50,
          "successful_executions": 40
        },

        "content_hash": "sha256:def456..."
      }
    ]
  },

  // V2.0: Enhanced search metadata
  "search_metadata": {
    "duration_ms": 45,
    "db_query_time_ms": 12,
    "embedding_generation_time_ms": 33,
    "embedding_dimensions": 768,
    "embedding_model": "all-mpnet-base-v2",
    "index_used": "workflow_embedding_idx",
    "cache_hit": false
  }
}
```

---

## 🔄 **Data Flow (V2.0)**

### **Request Flow**

```
HolmesGPT API
  ↓ POST /api/v1/workflows/search
  ↓ Include: remediation_id in JSON request body
  ↓
Data Storage Service
  ↓ Execute two-phase search (labels + pgvector)
  ↓ Calculate hybrid scoring
  ↓ Generate audit event (async buffered - ADR-038)
  ↓ Return search results to HolmesGPT API
  ↓
HolmesGPT API
  ✅ Receives workflows (no audit responsibility)
```

### **Audit Flow (Non-Blocking)**

```
Data Storage Service (after search query)
  ↓ Build audit event with FULL workflow metadata
  ↓ Include: all candidates, success metrics, version history
  ↓ Store in async buffer (ADR-038)
  ↓ Background worker writes to audit_events table
  ↓
PostgreSQL (audit_events table)
  ✅ Complete audit trail with rich context
```

---

## 🔧 **Implementation (V2.0)**

### **Step 1: HolmesGPT API - Pass remediation_id in JSON Body**

```python
# holmesgpt-api/src/toolsets/workflow_catalog.py
# Pass remediation_id in JSON request body (NOT HTTP header)

def _search_workflows(self, query: str, filters: Dict, top_k: int):
    request_data = {
        "query": query,
        "filters": filters,
        "top_k": top_k,
        "remediation_id": self._remediation_id  # For audit correlation
    }

    response = requests.post(
        f"{self._data_storage_url}/api/v1/workflows/search",
        json=request_data,
        timeout=self._http_timeout
    )

    # NO audit responsibility - Data Storage handles it
    return response.json()
```

**Why JSON Body (not HTTP Header)?**
- ✅ **Consistency**: All request data in one place (JSON body)
- ✅ **Validation**: Standard JSON schema validation on server
- ✅ **RESTful**: Request metadata belongs in request body
- ✅ **Simplicity**: No header parsing required on server

---

### **Step 2: Data Storage Service - Generate Audit Event**

```go
// pkg/datastorage/server/workflow_handlers.go
// Generate audit event after search

func (s *Server) SearchWorkflows(c *gin.Context) {
    // Parse request body (includes remediation_id)
    var request models.WorkflowSearchRequest
    if err := c.ShouldBindJSON(&request); err != nil {
        // ... error handling
    }

    // Extract remediation_id from request body
    remediationID := request.RemediationID

    // Execute search
    results, searchMeta, err := s.workflowRepo.SearchByEmbedding(ctx, request)
    if err != nil {
        // ... error handling
    }

    // ========================================
    // V2.0: Generate audit event with FULL context
    // ========================================
    auditEvent := s.buildWorkflowSearchAuditEvent(
        remediationID,
        request,
        results,
        searchMeta,
    )

    // Non-blocking: Uses ADR-038 async buffered pattern
    s.auditStore.StoreAudit(ctx, auditEvent)

    // Return search results
    c.JSON(http.StatusOK, results)
}

func (s *Server) buildWorkflowSearchAuditEvent(
    remediationID string,
    request *models.WorkflowSearchRequest,
    results []*models.WorkflowSearchResult,
    meta *SearchMetadata,
) *audit.AuditEvent {

    // Build workflow entries with FULL metadata
    workflows := make([]map[string]interface{}, len(results))
    for i, r := range results {
        workflows[i] = map[string]interface{}{
            "workflow_id": r.Workflow.WorkflowID,
            "version":     r.Workflow.Version,
            "title":       r.Workflow.Name,
            "rank":        i + 1,

            // Scoring breakdown
            "scoring": map[string]interface{}{
                "confidence":        r.FinalScore,
                "base_similarity":   r.BaseSimilarity,
                "label_boost":       r.LabelBoost,
                "label_penalty":     r.LabelPenalty,
                "boost_breakdown":   r.BoostBreakdown,
                "penalty_breakdown": r.PenaltyBreakdown,
            },

            // V2.0: Full workflow metadata
            "labels":           r.Workflow.Labels,
            "owner":            r.Workflow.Owner,
            "maintainer":       r.Workflow.Maintainer,
            "previous_version": r.Workflow.PreviousVersion,
            "version_notes":    r.Workflow.VersionNotes,
            "approved_by":      r.Workflow.ApprovedBy,
            "approved_at":      r.Workflow.ApprovedAt,
            "content_hash":     r.Workflow.ContentHash,

            // V2.0: Success metrics
            "success_metrics": map[string]interface{}{
                "expected_success_rate":  r.Workflow.ExpectedSuccessRate,
                "actual_success_rate":    r.Workflow.ActualSuccessRate,
                "total_executions":       r.Workflow.TotalExecutions,
                "successful_executions":  r.Workflow.SuccessfulExecutions,
            },
        }
    }

    return audit.NewAuditEvent().
        WithEventType("workflow.catalog.search_completed").
        WithService("datastorage").
        WithCorrelationID(remediationID).
        WithEventData(map[string]interface{}{
            "query": map[string]interface{}{
                "text":           request.Query,
                "filters":        request.Filters,
                "top_k":          request.TopK,
                "min_similarity": request.MinSimilarity,
            },
            "results": map[string]interface{}{
                "total_found":          len(results),
                "returned":             len(results),
                "candidates_considered": meta.CandidatesConsidered,
                "workflows":            workflows,
            },
            "search_metadata": map[string]interface{}{
                "duration_ms":               meta.DurationMs,
                "db_query_time_ms":          meta.DBQueryTimeMs,
                "embedding_generation_time_ms": meta.EmbeddingTimeMs,
                "embedding_dimensions":      768,
                "embedding_model":           "all-mpnet-base-v2",
                "index_used":                meta.IndexUsed,
                "cache_hit":                 meta.CacheHit,
            },
        })
}
```

---

## 🎯 **Use Cases & Benefits**

### **Use Case 1: Debugging Workflow Selection**

**Scenario**: Operator creates new workflow but it's never selected

**V2.0 Audit Query** (includes success metrics):
```sql
SELECT
  event_data->'results'->'workflows'->0->>'workflow_id' AS workflow_id,
  event_data->'results'->'workflows'->0->'scoring'->>'confidence' AS confidence,
  event_data->'results'->'workflows'->0->'success_metrics'->>'actual_success_rate' AS success_rate,
  event_data->'results'->'workflows'->0->>'owner' AS owner
FROM audit_events
WHERE event_type = 'workflow.catalog.search_completed'
  AND event_data->'query'->>'text' LIKE '%OOMKilled%';
```

**V2.0 Insight**: See success rate history to understand if workflow is effective

---

### **Use Case 2: Version History Analysis**

**Scenario**: Understand why a specific version was selected over previous

**V2.0 Audit Query**:
```sql
SELECT
  event_data->'results'->'workflows'->0->>'workflow_id' AS workflow_id,
  event_data->'results'->'workflows'->0->>'version' AS version,
  event_data->'results'->'workflows'->0->>'previous_version' AS previous_version,
  event_data->'results'->'workflows'->0->>'version_notes' AS version_notes,
  event_data->'results'->'workflows'->0->>'approved_by' AS approved_by
FROM audit_events
WHERE event_type = 'workflow.catalog.search_completed'
  AND event_data->'results'->'workflows'->0->>'workflow_id' = 'pod-oom-gitops';
```

**V2.0 Insight**: Complete version history for compliance and debugging

---

### **Use Case 3: Workflow Effectiveness Tracking**

**Scenario**: Identify workflows with declining success rates

**V2.0 Audit Query**:
```sql
SELECT
  jsonb_array_elements(event_data->'results'->'workflows')->>'workflow_id' AS workflow_id,
  AVG((jsonb_array_elements(event_data->'results'->'workflows')->'success_metrics'->>'actual_success_rate')::float) AS avg_success_rate,
  AVG((jsonb_array_elements(event_data->'results'->'workflows')->'success_metrics'->>'expected_success_rate')::float) AS avg_expected_rate
FROM audit_events
WHERE event_type = 'workflow.catalog.search_completed'
  AND event_timestamp >= NOW() - INTERVAL '30 days'
GROUP BY workflow_id
HAVING AVG((jsonb_array_elements(event_data->'results'->'workflows')->'success_metrics'->>'actual_success_rate')::float) < 0.8
ORDER BY avg_success_rate;
```

**V2.0 Insight**: Identify underperforming workflows for review

---

## 📋 **Implementation Checklist (V2.0)**

- [ ] **Phase 1**: Update HolmesGPT API (1 hour)
  - [ ] Pass `remediation_id` in JSON request body
  - [ ] Remove audit event generation code
  - [ ] Remove `BufferedAuditStore` dependency

- [ ] **Phase 2**: Implement Data Storage audit (3 hours)
  - [ ] Add `remediation_id` field to `WorkflowSearchRequest` model
  - [ ] Build audit event with full workflow metadata
  - [ ] Include success metrics and version history
  - [ ] Use ADR-038 async buffered pattern

- [ ] **Phase 3**: Update tests (2 hours)
  - [ ] Remove HolmesGPT API audit tests
  - [ ] Add Data Storage Service audit tests
  - [ ] Verify full metadata in audit events

- [ ] **Phase 4**: Documentation (1 hour)
  - [ ] Update DD-WORKFLOW-014 (this document) ✅
  - [ ] Update implementation plan
  - [ ] Create operator runbook

**Total Effort**: 7 hours

---

## V3.0: Three-Step Discovery Audit Events (DD-WORKFLOW-016)

### Architectural Change

The single `workflow.catalog.search_completed` event (V2.0) is replaced by **four step-specific events** aligned with the three-step discovery protocol defined in DD-WORKFLOW-016. This provides fine-grained auditability for each decision point in the workflow selection process.

**V2.0 architecture is preserved**: All events are generated by the **Data Storage Service** at the data layer (per ADR-032). HAPI does not generate audit events -- it delegates to DS.

### V3.0 Audit Events

| Event Type | Step | When | Generated By |
|-----------|------|------|-------------|
| `workflow.catalog.actions_listed` | Step 1 | DS returns action types for signal context | Data Storage |
| `workflow.catalog.workflows_listed` | Step 2 | DS returns workflows for selected action type | Data Storage |
| `workflow.catalog.workflow_retrieved` | Step 3 | DS returns single workflow parameter schema | Data Storage |
| `workflow.catalog.selection_validated` | Post-selection | DS validates HAPI's re-query of selected workflow | Data Storage |

### Event Schemas

#### `workflow.catalog.actions_listed` (Step 1)

```json
{
  "version": "1.0",
  "service": "data-storage",
  "operation": "actions_listed",
  "status": "success",
  "payload": {
    "remediationId": "rr-2026-02-05-abc123",
    "signalContext": {
      "severity": "critical",
      "component": "deployment",
      "environment": "production",
      "priority": "P0"
    },
    "resultCount": 3,
    "actionTypes": ["ScaleReplicas", "IncreaseCPULimits", "RestartPod"],
    "pagination": {
      "offset": 0,
      "limit": 10,
      "totalCount": 3,
      "hasMore": false
    },
    "queryDurationMs": 12
  }
}
```

#### `workflow.catalog.workflows_listed` (Step 2)

```json
{
  "version": "1.0",
  "service": "data-storage",
  "operation": "workflows_listed",
  "status": "success",
  "payload": {
    "remediationId": "rr-2026-02-05-abc123",
    "actionType": "ScaleReplicas",
    "signalContext": {
      "severity": "critical",
      "component": "deployment",
      "environment": "production",
      "priority": "P0"
    },
    "resultCount": 2,
    "workflows": [
      {
        "workflowId": "550e8400-e29b-41d4-a716-446655440000",
        "workflowName": "scale-conservative",
        "version": "1.0.0",
        "finalScore": 0.92
      },
      {
        "workflowId": "660f9511-f39c-52e5-b827-557766551111",
        "workflowName": "scale-aggressive",
        "version": "1.0.0",
        "finalScore": 0.85
      }
    ],
    "pagination": {
      "offset": 0,
      "limit": 10,
      "totalCount": 2,
      "hasMore": false
    },
    "queryDurationMs": 15
  }
}
```

**Note**: `finalScore` is included in the audit event for debugging and tuning (operators need to understand why certain workflows rank higher). It is **not** exposed to the LLM -- HAPI strips it before rendering.

#### `workflow.catalog.workflow_retrieved` (Step 3)

```json
{
  "version": "1.0",
  "service": "data-storage",
  "operation": "workflow_retrieved",
  "status": "success",
  "payload": {
    "remediationId": "rr-2026-02-05-abc123",
    "workflowId": "550e8400-e29b-41d4-a716-446655440000",
    "workflowName": "scale-conservative",
    "actionType": "ScaleReplicas",
    "version": "1.0.0",
    "securityGateResult": "pass",
    "signalContext": {
      "severity": "critical",
      "component": "deployment",
      "environment": "production",
      "priority": "P0"
    },
    "parameterCount": 3,
    "queryDurationMs": 8
  }
}
```

**Note**: `securityGateResult` is `"pass"` when the workflow matches the signal context filters, or `"fail"` when the workflow_id exists but doesn't match (defense-in-depth per DD-WORKFLOW-016). A `"fail"` result means the LLM attempted to use a workflow outside the allowed context.

#### `workflow.catalog.selection_validated` (Post-Selection)

```json
{
  "version": "1.0",
  "service": "data-storage",
  "operation": "selection_validated",
  "status": "success",
  "payload": {
    "remediationId": "rr-2026-02-05-abc123",
    "workflowId": "550e8400-e29b-41d4-a716-446655440000",
    "workflowName": "scale-conservative",
    "actionType": "ScaleReplicas",
    "version": "1.0.0",
    "validationResult": "valid",
    "parameterValidation": {
      "totalParameters": 3,
      "requiredProvided": 2,
      "optionalProvided": 1,
      "validationErrors": []
    },
    "queryDurationMs": 10
  }
}
```

### V2.0 Event Deprecation

The V2.0 `workflow.catalog.search_completed` event is **deprecated** and replaced by the four V3.0 events above. Existing V2.0 consumers should migrate to querying the V3.0 events by `remediationId` to reconstruct the full selection flow.

---

## 🔗 **Related Documents**

- **DD-WORKFLOW-013**: Scoring Field Population (how fields are calculated)
- **DD-WORKFLOW-004**: Hybrid Weighted Label Scoring (scoring algorithm)
- **DD-WORKFLOW-016**: Action-Type Workflow Catalog Indexing (three-step discovery protocol)
- **DD-WORKFLOW-017**: Workflow Lifecycle Component Interactions (end-to-end lifecycle)
- **DD-AUDIT-001**: Audit Responsibility Pattern (who generates audit events)
- **ADR-034**: Unified Audit Trail Architecture (audit table schema)
- **ADR-038**: Async Buffered Audit Ingestion (non-blocking pattern)
- **ADR-032**: Data Access Layer Isolation (data layer authority)

---

**Status**: ✅ **APPROVED**
**Version**: 3.0
**Confidence**: 95%
**Next Steps**: Implement V3.0 step-specific audit events in Data Storage Service
