# DD-STORAGE-012: Critical Label Filtering for Safety-Critical Playbook Selection

**Date**: November 13, 2025
**Status**: 🚧 **DEFERRED TO V1.1** (awaiting V1.0 feedback)
**Decision Maker**: Kubernaut Data Storage Team
**Authority**: PoC results from `scripts/poc-label-embedding-test.py`
**Affects**: Data Storage Service V1.1, Playbook Catalog semantic search
**Version**: 1.0

---

## 🎯 **Context**

**Problem**: Custom labels have weak influence in sentence-transformers embeddings, making safety-critical playbook selection unreliable.

**PoC Results** (from `scripts/poc-label-embedding-test.py`):
- **Label influence**: 0.001-0.004 similarity change (WEAK - < 0.02 threshold)
- **Content influence**: 0.125 similarity change (STRONG - > 0.05 threshold)
- **Ratio**: Content has **100× more influence** than labels
- **Critical failure**: Wrong labels can score higher than correct labels (Test 4: 0.8735 vs 0.8683)

**Business Impact**: Safety-critical scenarios where wrong playbook selection could violate constraints:

### **Use Case: Cost-Constrained Namespace**

**Scenario**:
```
Namespace: cost-management
Constraint: Cannot increase memory (cost/hardware limit)

Playbook A (Cost-Optimized):
  - Actions: Restart pod, clear cache, optimize memory usage
  - Labels: cost-tier=budget, memory-scaling=disabled
  - Safe: ✅ Respects cost constraints

Playbook B (Default):
  - Actions: Increase memory limits, restart pod
  - Labels: cost-tier=standard, memory-scaling=enabled
  - Unsafe: ❌ Violates cost constraints

Query: "pod OOMKilled in cost-management namespace"
Expected: Playbook A (cost-optimized) MUST be selected
Current Risk: Playbook B could be selected (higher semantic similarity)
```

**Other Safety-Critical Scenarios**:
- **Data residency**: EU playbooks for GDPR compliance
- **Compliance level**: SOX-compliant playbooks for financial services
- **Hardware tier**: GPU-required playbooks for ML workloads
- **Network isolation**: Air-gapped playbooks for secure environments

---

## 🔍 **Problem Statement**

**Current V1.0 Approach** (Hybrid: Hard Filter Mandatory + Semantic Search):
```sql
SELECT * FROM playbook_catalog
WHERE status = 'active'
  -- Hard filter on mandatory labels
  AND labels->>'kubernaut.io/environment' = 'production'
  AND labels->>'kubernaut.io/priority' = 'P0'
  AND labels->>'kubernaut.io/incident-type' = 'pod-oom-killer'
  
  -- Semantic search (custom labels encoded in embedding)
  AND 1 - (embedding <=> $query_embedding) >= 0.7
ORDER BY embedding <=> $query_embedding
LIMIT 10;
```

**Gap**: Custom labels (cost-tier, memory-scaling, data-residency) are NOT hard-filtered, only semantically matched with weak influence (0.004).

**Risk**: Safety-critical custom labels may not reliably filter playbooks, leading to constraint violations.

---

## 📊 **Options Analysis**

### **Option 1: Mandatory Label Promotion**

**Strategy**: Promote critical custom labels to mandatory labels (hard-filtered)

**Implementation**:
```sql
SELECT * FROM playbook_catalog
WHERE status = 'active'
  -- Mandatory labels (hard-filtered)
  AND labels->>'kubernaut.io/environment' = 'production'
  AND labels->>'kubernaut.io/priority' = 'P0'
  AND labels->>'kubernaut.io/incident-type' = 'pod-oom-killer'
  
  -- Promoted critical labels (hard-filtered)
  AND labels->>'kubernaut.io/cost-tier' = 'budget'
  AND labels->>'kubernaut.io/memory-scaling' = 'disabled'
  
  -- Semantic search
  AND 1 - (embedding <=> $query_embedding) >= 0.7
ORDER BY embedding <=> $query_embedding
LIMIT 10;
```

**Pros**:
- ✅ **100% reliable**: Hard filtering guarantees correct playbook
- ✅ **Safety-critical**: Cost/hardware constraints enforced at DB level
- ✅ **Performance**: GIN index for fast filtering
- ✅ **Simple**: No model changes, just SQL WHERE clause
- ✅ **Deterministic**: No probabilistic behavior

**Cons**:
- ❌ **Schema rigidity**: Adding new constraints requires updating mandatory label list
- ❌ **Rego policy complexity**: More labels to populate
- ❌ **Namespace pollution**: `kubernaut.io/*` namespace grows with organization-specific labels
- ❌ **Not scalable**: Every organization has different critical labels

**Confidence**: **99%** (technically sound, but not scalable)

**Recommendation**: ❌ **NOT RECOMMENDED** (violates namespace separation principle)

---

### **Option 2: Label Weighting in Embedding**

**Strategy**: Repeat critical labels multiple times in embedding text to increase influence

**Implementation**:
```python
def generate_playbook_embedding_weighted(playbook):
    text_parts = [
        playbook.name,
        playbook.description,
    ]
    
    # Repeat critical labels 10× to increase influence
    critical_labels = ['cost-tier', 'memory-scaling', 'data-residency']
    for key, value in playbook.labels.items():
        if any(critical in key for critical in critical_labels):
            # Repeat 10× to increase weight
            for _ in range(10):
                text_parts.append(f"{key}: {value}")
        elif not key.startswith('kubernaut.io/'):
            # Normal weight for non-critical custom labels
            text_parts.append(f"{key}: {value}")
    
    text = '\n'.join(text_parts)
    embedding = model.encode(text)
    return embedding
```

**Example Embedding Input**:
```
Pod OOM Recovery (Cost-Optimized)
Restarts pod and clears cache without increasing memory
mycompany.com/cost-tier: budget
mycompany.com/cost-tier: budget
mycompany.com/cost-tier: budget
... (10 times total)
mycompany.com/memory-scaling: disabled
mycompany.com/memory-scaling: disabled
... (10 times total)
```

**Pros**:
- ✅ **Flexible**: No schema changes
- ✅ **Tunable**: Can adjust repetition count per label
- ✅ **Namespace clean**: No `kubernaut.io/*` pollution
- ✅ **Organization-specific**: Each org defines critical labels

**Cons**:
- ❌ **Unproven**: Needs empirical testing (repetition may not work)
- ❌ **Fragile**: Depends on model behavior (sentence-transformers may normalize)
- ❌ **Not safety-critical**: Still probabilistic, not deterministic
- ❌ **Debugging**: Hard to understand why playbook matched
- ❌ **PoC needed**: Requires validation before implementation

**Confidence**: **50%** (needs PoC validation)

**Recommendation**: ⚠️ **EXPERIMENTAL** (requires PoC before V1.1 consideration)

**PoC Test Plan**:
```python
# Test: Does repetition increase label influence?
playbook_repeated = """Pod OOM Recovery
cost-tier: budget
cost-tier: budget
... (10× total)"""

playbook_normal = """Pod OOM Recovery
cost-tier: budget"""

query_budget = """pod OOM
cost-tier: budget"""

query_standard = """pod OOM
cost-tier: standard"""

# Expected: Repeated labels should increase mismatch penalty
sim_repeated_match = cosine_similarity(encode(playbook_repeated), encode(query_budget))
sim_repeated_mismatch = cosine_similarity(encode(playbook_repeated), encode(query_standard))
sim_normal_match = cosine_similarity(encode(playbook_normal), encode(query_budget))
sim_normal_mismatch = cosine_similarity(encode(playbook_normal), encode(query_standard))

# Hypothesis: (sim_repeated_match - sim_repeated_mismatch) > (sim_normal_match - sim_normal_mismatch)
# i.e., repetition increases label influence
```

---

### **Option 3: Multi-Stage Filtering** (RECOMMENDED)

**Strategy**: Hard filter on critical custom labels, then semantic search within filtered set

**Implementation**:
```sql
-- PostgreSQL function for dynamic critical label filtering
CREATE OR REPLACE FUNCTION search_playbooks_with_critical_labels(
    query_embedding vector(384),
    mandatory_labels JSONB,
    critical_labels JSONB,
    min_confidence DECIMAL,
    max_results INT
) RETURNS TABLE (
    playbook_id VARCHAR,
    version VARCHAR,
    description TEXT,
    labels JSONB,
    confidence DECIMAL
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        p.playbook_id,
        p.version,
        p.description,
        p.labels,
        (1 - (p.embedding <=> query_embedding))::DECIMAL AS confidence
    FROM playbook_catalog p
    WHERE p.status = 'active'
      AND p.is_latest_version = true
      
      -- Hard filter on mandatory labels
      AND p.labels @> mandatory_labels
      
      -- Hard filter on critical custom labels (if present)
      AND (
          critical_labels IS NULL 
          OR jsonb_object_keys(critical_labels) = '{}'
          OR p.labels @> critical_labels
      )
      
      -- Semantic search threshold
      AND (1 - (p.embedding <=> query_embedding)) >= min_confidence
    ORDER BY p.embedding <=> query_embedding
    LIMIT max_results;
END;
$$ LANGUAGE plpgsql;
```

**Usage**:
```sql
SELECT * FROM search_playbooks_with_critical_labels(
    $query_embedding,
    '{"kubernaut.io/environment": "production", "kubernaut.io/priority": "P0"}'::jsonb,
    '{"mycompany.com/cost-tier": "budget", "mycompany.com/memory-scaling": "disabled"}'::jsonb,
    0.7,
    10
);
```

**Architecture**:

**Step 1: Critical Label Registry** (ConfigMap)
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: critical-label-registry
  namespace: kubernaut-system
data:
  critical_labels: |
    # Labels that MUST be hard-filtered (safety-critical)
    - mycompany.com/cost-tier
    - mycompany.com/memory-scaling
    - mycompany.com/data-residency
    - mycompany.com/compliance-level
    - mycompany.com/hardware-tier
```

**Step 2: Rego Policy Populates Critical Labels**
```rego
package kubernaut.categorization

# Mandatory labels (always hard-filtered)
environment := "production" { ... }
priority := "P0" { ... }
incident_type := "pod-oom-killer" { ... }

# Critical custom labels (safety-critical, hard-filtered)
critical_labels["mycompany.com/cost-tier"] := tier {
    namespace_labels := data.kubernetes.namespaces[input.namespace].labels
    tier := namespace_labels["cost-tier"]
}

critical_labels["mycompany.com/cost-tier"] := "standard" {
    # Default if not set
    not data.kubernetes.namespaces[input.namespace].labels["cost-tier"]
}

critical_labels["mycompany.com/memory-scaling"] := scaling {
    namespace_labels := data.kubernetes.namespaces[input.namespace].labels
    scaling := namespace_labels["memory-scaling"]
}

critical_labels["mycompany.com/memory-scaling"] := "enabled" {
    # Default: memory scaling allowed
    not data.kubernetes.namespaces[input.namespace].labels["memory-scaling"]
}

# Optional custom labels (semantic influence only, not hard-filtered)
optional_labels["mycompany.com/team"] := team {
    namespace_labels := data.kubernetes.namespaces[input.namespace].labels
    team := namespace_labels["team"]
}

optional_labels["mycompany.com/region"] := region {
    node_labels := data.kubernetes.nodes[input.node].labels
    region := node_labels["topology.kubernetes.io/region"]
}
```

**Step 3: RemediationRequest CRD Stores Both**
```yaml
apiVersion: kubernaut.io/v1alpha1
kind: RemediationRequest
status:
  environmentClassification:
    environment: "production"
    businessPriority: "P0"
    
    # Critical custom labels (hard-filtered)
    criticalLabels:
      mycompany.com/cost-tier: "budget"
      mycompany.com/memory-scaling: "disabled"
      mycompany.com/data-residency: "eu"
    
    # Optional custom labels (semantic influence only)
    optionalLabels:
      mycompany.com/team: "platform-engineering"
      mycompany.com/region: "us-east-1"
```

**Step 4: Go Client**
```go
type PlaybookSearchParams struct {
    QueryEmbedding  []float32
    
    // Mandatory labels (always hard-filtered)
    MandatoryLabels map[string]string
    
    // Critical custom labels (hard-filtered if present)
    CriticalLabels  map[string]string
    
    // Optional custom labels (encoded in embedding, not filtered)
    OptionalLabels  map[string]string
    
    MinConfidence   float64
    MaxResults      int
}

func (r *playbookRepository) SearchPlaybooks(ctx context.Context, params *PlaybookSearchParams) ([]*Playbook, error) {
    mandatoryJSON, _ := json.Marshal(params.MandatoryLabels)
    criticalJSON, _ := json.Marshal(params.CriticalLabels)
    
    rows, err := r.db.QueryContext(ctx, `
        SELECT * FROM search_playbooks_with_critical_labels($1, $2, $3, $4, $5)
    `, pgvector.NewVector(params.QueryEmbedding), mandatoryJSON, criticalJSON, params.MinConfidence, params.MaxResults)
    
    // ... parse results
}
```

**Pros**:
- ✅ **Safety**: Critical labels hard-filtered (deterministic)
- ✅ **Flexibility**: Can specify critical labels per query
- ✅ **Performance**: GIN index + HNSW index
- ✅ **Backward compatible**: NULL handling for critical labels
- ✅ **Namespace clean**: No `kubernaut.io/*` pollution
- ✅ **Organization-specific**: Each org defines critical labels
- ✅ **Configurable**: Critical label registry per organization
- ✅ **Debuggable**: Can see which labels were hard-filtered

**Cons**:
- ⚠️ **Query complexity**: Need to know which labels are critical
- ⚠️ **NULL handling**: Need to handle optional critical labels
- ⚠️ **CRD schema change**: RemediationRequest needs `criticalLabels` field
- ⚠️ **Rego policy complexity**: Need to separate critical vs optional labels
- ⚠️ **Configuration overhead**: Critical label registry maintenance

**Confidence**: **98%**

**Recommendation**: ✅ **RECOMMENDED** (preferred for V1.1)

---

### **Option 4: Label Hierarchy with Fallback**

**Strategy**: Define label hierarchy and fallback behavior

**Implementation**:
```yaml
# Label Hierarchy Configuration
label_hierarchy:
  tier1_critical:  # MUST match (no playbooks if no match)
    - kubernaut.io/environment
    - kubernaut.io/priority
    - kubernaut.io/incident-type
  
  tier2_critical:  # SHOULD match (fallback to tier1 if no match)
    - mycompany.com/cost-tier
    - mycompany.com/memory-scaling
    - mycompany.com/data-residency
  
  tier3_optional:  # NICE to match (semantic influence only)
    - mycompany.com/team
    - mycompany.com/region
```

**Query Logic**:
```go
func searchPlaybooksWithFallback(ctx context.Context, params *SearchParams) ([]*Playbook, error) {
    // Try tier1 + tier2 (most restrictive)
    playbooks, err := searchWithLabels(ctx, params, tier1Labels + tier2Labels)
    if len(playbooks) > 0 {
        return playbooks, nil
    }
    
    // Fallback: tier1 only (less restrictive)
    log.Warn("No playbooks found with tier2 labels, falling back to tier1 only",
        "tier2_labels", tier2Labels)
    playbooks, err = searchWithLabels(ctx, params, tier1Labels)
    if len(playbooks) > 0 {
        return playbooks, nil
    }
    
    // No playbooks found
    return nil, fmt.Errorf("no playbooks found matching tier1 labels")
}
```

**Pros**:
- ✅ **Safety**: Critical labels always enforced
- ✅ **Graceful degradation**: Fallback if no exact match
- ✅ **Configurable**: Can adjust hierarchy per organization
- ✅ **Flexible**: Can add more tiers as needed

**Cons**:
- ⚠️ **Complexity**: Requires fallback logic
- ⚠️ **Debugging**: Harder to understand which tier was used
- ⚠️ **Unexpected behavior**: Fallback may return unexpected playbooks
- ⚠️ **Performance**: Multiple queries if tier2 has no matches
- ⚠️ **Audit trail**: Need to log which tier was used

**Confidence**: **95%**

**Recommendation**: ⚠️ **ALTERNATIVE** (consider if Option 3 proves too complex)

---

## 📊 **Comparison Matrix**

| Criteria | Option 1: Promotion | Option 2: Weighting | Option 3: Multi-Stage | Option 4: Hierarchy |
|----------|---------------------|---------------------|----------------------|---------------------|
| **Safety** | 100% | 50% | 100% | 100% |
| **Flexibility** | 40% | 90% | 95% | 90% |
| **Performance** | 100% | 95% | 98% | 85% |
| **Complexity** | 60% | 70% | 85% | 75% |
| **Scalability** | 40% | 90% | 95% | 90% |
| **Debuggability** | 90% | 40% | 95% | 70% |
| **Namespace Clean** | 30% | 100% | 100% | 100% |
| **Confidence** | 99% | 50% | 98% | 95% |
| **Recommendation** | ❌ NO | ⚠️ EXPERIMENTAL | ✅ YES | ⚠️ ALTERNATIVE |

---

## 🎯 **Recommended Decision Path**

### **Phase 1: V1.0 Release** (Current)

**Approach**: Hybrid (Hard Filter Mandatory + Semantic Search)
- Mandatory labels: `kubernaut.io/environment`, `kubernaut.io/priority`, `kubernaut.io/incident-type`
- Custom labels: Encoded in embedding (weak influence, acceptable for V1.0)

**Rationale**: Gather real-world feedback on label matching behavior before implementing complex critical label filtering.

**V1.0 Success Criteria**:
- ✅ Semantic search works for 90%+ of use cases
- ✅ Mandatory labels reliably filter playbooks
- ⚠️ Custom labels have weak influence (known limitation)

---

### **Phase 2: V1.0 Feedback Collection** (Post-Release)

**Questions to Answer**:
1. How often do users need safety-critical custom labels?
2. Which custom labels are most critical (cost-tier, data-residency, compliance-level)?
3. Are there use cases where weak label influence causes problems?
4. What is the acceptable complexity for critical label configuration?

**Feedback Mechanisms**:
- User interviews
- Support tickets
- Playbook selection accuracy metrics
- Constraint violation incidents

---

### **Phase 3: V1.1 Implementation** (After Feedback)

**Decision**: Implement **Option 3 (Multi-Stage Filtering)** if feedback confirms need

**Triggers for V1.1 Implementation**:
- ✅ Users report constraint violations (cost, compliance, data-residency)
- ✅ Users request safety-critical label filtering
- ✅ Playbook selection accuracy < 90% due to label mismatch

**Alternative**: If feedback shows weak need, defer to V2.0 or consider Option 2 (Weighting) with PoC

---

## 📋 **Implementation Checklist (V1.1)**

**If Option 3 is approved after V1.0 feedback:**

### **Database Changes**
- [ ] Create PostgreSQL function `search_playbooks_with_critical_labels`
- [ ] Add migration for function creation
- [ ] Test JSONB containment performance with large label sets
- [ ] Verify GIN index optimization

### **CRD Schema Changes**
- [ ] Add `criticalLabels` field to RemediationRequest CRD status
- [ ] Add `optionalLabels` field to RemediationRequest CRD status
- [ ] Update CRD validation
- [ ] Update CRD documentation

### **Rego Policy Changes**
- [ ] Create critical label registry ConfigMap
- [ ] Update Rego policies to populate `criticalLabels` vs `optionalLabels`
- [ ] Add default values for critical labels
- [ ] Test Rego policy evaluation

### **Data Storage Service Changes**
- [ ] Update `PlaybookSearchParams` struct with `CriticalLabels` field
- [ ] Update `SearchPlaybooks` repository method
- [ ] Update HTTP API to accept critical labels
- [ ] Add integration tests for critical label filtering

### **Signal Processing Controller Changes**
- [ ] Update controller to read critical label registry
- [ ] Update controller to populate `criticalLabels` in RemediationRequest status
- [ ] Add unit tests for critical label population

### **Documentation**
- [ ] Update DD-STORAGE-008 with critical label filtering
- [ ] Update playbook catalog guide with critical label examples
- [ ] Create operator guide for critical label registry configuration
- [ ] Update API documentation

### **Testing**
- [ ] Unit tests for PostgreSQL function
- [ ] Integration tests for critical label filtering
- [ ] E2E tests for cost-constrained scenario
- [ ] Performance tests for JSONB containment

---

## 🔗 **Related Decisions**

- **DD-STORAGE-008**: Playbook Catalog Schema (label architecture)
- **DD-STORAGE-010**: Data Storage V1.0 Implementation Plan (V1.0 hybrid approach)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (label matching requirements)
- **PoC**: `scripts/poc-label-embedding-test.py` (empirical evidence for weak label influence)

---

## 📊 **PoC Evidence**

**File**: `scripts/poc-label-embedding-test.py`

**Key Findings**:
- **Test 1 (Label Matching)**: ✅ PASS - Environment labels DO influence similarity
- **Test 2 (Label Weight)**: ❌ FAIL - Priority label has WEAK influence (0.0014 difference)
- **Test 3 (Partial Labels)**: ✅ PASS - Partial label matching works
- **Test 4 (Label vs Content)**: ❌ FAIL - Wrong labels scored HIGHER than correct labels
- **Test 5 (Custom Labels)**: ✅ PASS - Custom labels DO influence similarity

**Quantitative Analysis**:
- Environment mismatch: 0.0011 similarity change (WEAK)
- Priority mismatch: 0.0014 similarity change (WEAK)
- Content mismatch: 0.1250 similarity change (STRONG)
- Custom label mismatch: 0.0040 similarity change (WEAK)

**Conclusion**: Labels have 100× less influence than content, making safety-critical filtering unreliable without hard filtering.

---

## 🎯 **Next Steps**

1. ✅ **DD-STORAGE-012 Created** (this document)
2. 🚧 **V1.0 Release**: Implement hybrid approach (mandatory labels hard-filtered)
3. 🚧 **V1.0 Feedback**: Collect user feedback on label matching behavior
4. 🚧 **V1.1 Decision**: Approve Option 3 if feedback confirms need
5. 🚧 **V1.1 Implementation**: Execute checklist if approved

---

**Document Version**: 1.0
**Last Updated**: November 13, 2025
**Status**: 🚧 **DEFERRED TO V1.1** (awaiting V1.0 feedback)
**Next Review**: After V1.0 release (3-6 months)
**Decision Authority**: Kubernaut Architecture Team + User Feedback

