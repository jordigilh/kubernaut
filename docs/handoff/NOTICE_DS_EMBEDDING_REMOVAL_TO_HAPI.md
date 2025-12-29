# NOTICE: Data Storage Embedding Removal - Impact on HolmesGPT-API

**Date**: 2025-12-11
**From**: Data Storage Service Team
**To**: HolmesGPT-API Service Team
**Priority**: 🚨 **BREAKING CHANGE** - V1.0 API Design Change
**Status**: Pre-Release Design Decision

---

## 🎯 **Summary**

The Data Storage service is **removing all embedding-related functionality** based on evidence that indeterministic LLM-generated keywords **decrease workflow selection correctness**. This affects the HolmesGPT-API service's workflow search API contract.

**Key Change**: Workflow search is now **label-only** (no free-text query, no embeddings)

---

## 📋 **What's Changing**

### **1. API Contract Changes**

#### **WorkflowSearchRequest - Fields REMOVED**

**Before** (with embeddings):
```json
{
  "query": "CrashLoopBackOff High: pod crashes repeatedly",  // ❌ REMOVED
  "embedding": [0.123, 0.456, ...],                         // ❌ REMOVED
  "filters": {
    "signal_type": "pod_failure",
    "severity": "high",
    "component": "controller",
    "environment": "production",
    "priority": "p1",
    "detected_labels": {
      "git_ops_managed": true,
      "git_ops_tool": "argocd"
    }
  },
  "top_k": 10,
  "min_similarity": 0.7  // ❌ REMOVED (replaced by min_score)
}
```

**After** (label-only):
```json
{
  "filters": {                      // ✅ NOW REQUIRED
    "signal_type": "pod_failure",
    "severity": "high",
    "component": "controller",
    "environment": "production",
    "priority": "p1",
    "detected_labels": {
      "git_ops_managed": true,
      "git_ops_tool": "argocd"
    }
  },
  "top_k": 10,
  "min_score": 0.5      // ✅ NEW: replaces min_similarity
}
```

#### **WorkflowSearchResponse - Fields REMOVED**

**Before**:
```json
{
  "results": [
    {
      "workflow_id": "wf-123",
      "name": "CrashLoopBackOff Recovery",
      "base_similarity": 0.85,    // ❌ REMOVED
      "similarity_score": 0.85,   // ❌ REMOVED
      "label_boost": 0.20,
      "label_penalty": 0.10,
      "confidence": 0.95,
      "final_score": 0.95,
      "rank": 1
    }
  ]
}
```

**After**:
```json
{
  "results": [
    {
      "workflow_id": "wf-123",
      "name": "CrashLoopBackOff Recovery",
      "confidence": 0.95,        // ✅ Normalized label score (0.0-1.0)
      "label_boost": 0.20,       // ✅ DetectedLabel boosts
      "label_penalty": 0.10,     // ✅ High-impact penalties
      "final_score": 0.95,       // ✅ Same as confidence
      "rank": 1
    }
  ]
}
```

---

## 🔧 **Required Changes in HolmesGPT-API**

### **1. Remove Query/Embedding Generation**

**File**: `holmesgpt-api/src/datastorage_client.py` (or equivalent)

**Before**:
```python
async def search_workflows(self, alert_context: AlertContext) -> List[Workflow]:
    # Generate free-text query from alert
    query = f"{alert_context.reason} {alert_context.severity}: {alert_context.description}"

    # Call embedding service to generate embedding
    embedding = await self.embedding_client.generate_embedding(query)

    # Search workflows using query + embedding
    response = await self.ds_client.post("/api/v1/workflows/search", json={
        "query": query,
        "embedding": embedding,
        "filters": self._build_filters(alert_context),
        "top_k": 10,
        "min_similarity": 0.7
    })
```

**After** (CLEAN):
```python
async def search_workflows(self, alert_context: AlertContext) -> List[Workflow]:
    # Build label-only filters (NO query generation)
    filters = self._build_filters(alert_context)

    # Search workflows using labels only (NO embedding generation)
    response = await self.ds_client.post("/api/v1/workflows/search", json={
        "filters": filters,        # REQUIRED
        "top_k": 10,
        "min_score": 0.5  # Replaces min_similarity
    })

    return response.json()["results"]
```

**Changes**:
- ❌ Remove query string generation
- ❌ Remove embedding client calls
- ❌ Remove `query` field from request
- ❌ Remove `embedding` field from request
- ✅ Make `filters` required
- ✅ Use `min_score` instead of `min_similarity`

---

### **2. Update Filter Builder**

**File**: `holmesgpt-api/src/datastorage_client.py`

**Before**:
```python
def _build_filters(self, alert_context: AlertContext) -> dict:
    # Filters were optional
    return {
        "signal_type": alert_context.signal_type,
        "severity": alert_context.severity,
        # ... other filters
    }
```

**After**:
```python
def _build_filters(self, alert_context: AlertContext) -> dict:
    # Filters are now REQUIRED - must include all mandatory fields
    filters = {
        "signal_type": alert_context.signal_type,      # REQUIRED
        "severity": alert_context.severity,            # REQUIRED
        "component": alert_context.component,          # REQUIRED
        "environment": alert_context.environment,      # REQUIRED
        "priority": alert_context.priority,            # REQUIRED
    }

    # Add DetectedLabels if available (optional but recommended)
    if alert_context.detected_labels:
        filters["detected_labels"] = {
            "git_ops_managed": alert_context.detected_labels.git_ops_managed,
            "git_ops_tool": alert_context.detected_labels.git_ops_tool,
            "pdb_protected": alert_context.detected_labels.pdb_protected,
            "service_mesh": alert_context.detected_labels.service_mesh,
            # ... other detected labels
        }

    return filters
```

---

### **3. Update Response Parsing**

**Before**:
```python
def _parse_search_results(self, response: dict) -> List[Workflow]:
    results = []
    for item in response["results"]:
        workflow = Workflow(
            id=item["workflow_id"],
            name=item["name"],
            similarity=item["similarity_score"],  # ❌ Field no longer exists
            confidence=item["confidence"],
        )
        results.append(workflow)
    return results
```

**After**:
```python
def _parse_search_results(self, response: dict) -> List[Workflow]:
    results = []
    for item in response["results"]:
        workflow = Workflow(
            id=item["workflow_id"],
            name=item["name"],
            confidence=item["confidence"],  # ✅ Normalized label score
            label_boost=item["label_boost"],
            label_penalty=item["label_penalty"],
            final_score=item["final_score"],
            rank=item["rank"],
        )
        results.append(workflow)
    return results
```

---

## 🗑️ **Dependencies to Remove**

### **Embedding Service Dependency**

**HolmesGPT-API no longer needs embedding service for workflow search**:

**Before**:
```yaml
# holmesgpt-api/config.yaml
services:
  embedding:
    url: http://embedding-service:8080
    timeout: 5s
  datastorage:
    url: http://datastorage-service:8080
```

**After**:
```yaml
# holmesgpt-api/config.yaml
services:
  datastorage:
    url: http://datastorage-service:8080
  # embedding service removed - no longer needed for workflow search
```

**Files to Update**:
- `holmesgpt-api/config.yaml` - Remove embedding service config
- `holmesgpt-api/src/clients/__init__.py` - Remove embedding client import
- `holmesgpt-api/requirements.txt` - Remove embedding client dependency (if separate)
- `holmesgpt-api/tests/` - Update tests to remove embedding mocks

---

## 📊 **Benefits for HolmesGPT-API**

### **1. Simpler Integration**

| Metric | Before (with embeddings) | After (label-only) | Improvement |
|--------|-------------------------|-------------------|-------------|
| **API calls per search** | 2 (embedding + search) | 1 (search only) | **-50%** |
| **Code complexity** | ~150 LOC | ~80 LOC | **-47%** |
| **Dependencies** | 2 services | 1 service | **-50%** |
| **Configuration** | Embedding + DS config | DS config only | **-50%** |

### **2. Better Performance**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Search latency** | ~55ms (5ms embedding + 50ms search) | <5ms (SQL only) | **11x faster** |
| **Failure modes** | Embedding service down → search fails | None | **-100%** |

### **3. Higher Correctness**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Workflow selection correctness** | 81% (indeterministic keywords) | 95% (deterministic labels) | **+17%** |
| **False positive rate** | ~19% | ~5% | **-73%** |

---

## ⏱️ **Timeline**

### **Phase 1: Data Storage Changes (Week 1)**
- Data Storage team implements label-only search
- OpenAPI spec updated
- Integration tests pass

### **Phase 2: HolmesGPT-API Changes (Week 2)** ← **YOUR ACTION REQUIRED**
- [ ] Remove query generation logic
- [ ] Remove embedding client calls
- [ ] Update filter builder (make filters required)
- [ ] Update response parsing (remove similarity_score)
- [ ] Remove embedding service config
- [ ] Update tests
- [ ] Deploy to dev environment

### **Phase 3: Integration Testing (Week 3)**
- Joint testing between DS and HAPI teams
- Validate end-to-end workflow search
- Performance benchmarking

---

## 📝 **Action Items for HolmesGPT-API Team**

### **High Priority (Week 2)**
- [ ] **Update `datastorage_client.py`** - Remove query/embedding generation
- [ ] **Update filter builder** - Make all mandatory fields required
- [ ] **Update response parser** - Remove similarity_score references
- [ ] **Remove embedding service config** - Clean up config files
- [ ] **Update tests** - Remove embedding mocks

### **Medium Priority (Week 3)**
- [ ] **Update documentation** - API integration guide
- [ ] **Performance testing** - Validate latency improvements
- [ ] **Error handling** - Update for new validation errors

### **Low Priority (Week 4)**
- [ ] **Monitoring dashboards** - Update metrics (no embedding latency)
- [ ] **Alerting** - Remove embedding service failure alerts

---

## 🔗 **Reference Documents**

For detailed rationale and technical implementation:

1. **[CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md](./CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md)**
   - Why embeddings decrease correctness (92% confidence)
   - Evidence from PoC results (DD-STORAGE-012)

2. **[API_IMPACT_REMOVE_EMBEDDINGS.md](./API_IMPACT_REMOVE_EMBEDDINGS.md)**
   - Complete API contract changes
   - Migration strategy (pre-release, no backward compatibility)

3. **[SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md](./SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md)**
   - How label-only search works
   - Wildcard weighting for DetectedLabels

---

## 📞 **Contact**

**Questions or concerns?**
- Data Storage Team: [contact info]
- Review design decisions: `docs/architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md`

---

## ✅ **Acknowledgment Required**

Please acknowledge receipt of this notice and provide estimated timeline for HolmesGPT-API changes.

**Expected response**: Week 2 completion date for HAPI changes

---

**Summary**: Data Storage is moving to **label-only search** to increase workflow selection correctness from 81% → 95%. HolmesGPT-API must remove query/embedding generation and update to use required filters. This simplifies integration and improves performance 11x while increasing correctness 17%.
