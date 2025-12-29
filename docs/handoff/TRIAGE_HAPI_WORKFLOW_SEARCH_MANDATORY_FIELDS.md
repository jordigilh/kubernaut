# TRIAGE: HAPI Integration Test Failures (32 Tests - 400 Bad Request)

**From**: HAPI Team
**To**: HAPI Team
**Date**: 2025-12-12
**Status**: 🔴 **P1 - 32 Integration Tests Failing**
**Related**: [NOTICE_DS_EMBEDDING_REMOVAL_TO_HAPI.md](./NOTICE_DS_EMBEDDING_REMOVAL_TO_HAPI.md), [FOLLOWUP_DS_WORKFLOW_SEARCH_STILL_BROKEN.md](./FOLLOWUP_DS_WORKFLOW_SEARCH_STILL_BROKEN.md)

---

## 📊 **Triage Summary**

| Metric | Value |
|--------|-------|
| **Test Status** | 57/90 passing (63%) → 32 tests failing with 400 errors |
| **Root Cause** | Data Storage now requires 5 mandatory filter fields (was 2) |
| **Impact** | Integration tests blocked until HAPI adapts to new API contract |
| **Fix Complexity** | Medium - update `workflow_catalog.py` filter builder |
| **Estimated Time** | 1-2 hours (code + tests + validation) |

---

## 🔍 **Root Cause Analysis**

### **Issue**: 400 Bad Request on `/api/v1/workflows/search`

**Current Behavior**:
- HAPI sends only 2 mandatory fields: `signal_type`, `severity`
- Data Storage rejects with 400: "filters.component is required"

**Expected Behavior**:
- HAPI must send all 5 mandatory fields as per DD-WORKFLOW-001 v1.6

### **Data Storage API Contract** (Authoritative)

**Source**: `pkg/datastorage/models/workflow.go:176-206`

```go
type WorkflowSearchFilters struct {
    // 5 MANDATORY LABELS (Strict Filtering) - DD-WORKFLOW-001 v1.4
    SignalType  string `json:"signal_type" validate:"required"`
    Severity    string `json:"severity" validate:"required,oneof=critical high medium low"`
    Component   string `json:"component" validate:"required"`       // ← MISSING in HAPI
    Environment string `json:"environment" validate:"required"`     // ← MISSING in HAPI
    Priority    string `json:"priority" validate:"required"`        // ← MISSING in HAPI
}
```

**Validation Logic**: `pkg/datastorage/server/workflow_handlers.go:643-658`

```go
func (h *Handler) validateWorkflowSearchRequest(req *models.WorkflowSearchRequest) error {
    if req.Filters.SignalType == "" {
        return fmt.Errorf("filters.signal_type is required")
    }
    if req.Filters.Severity == "" {
        return fmt.Errorf("filters.severity is required")
    }
    if req.Filters.Component == "" {
        return fmt.Errorf("filters.component is required")  // ← HAPI violation
    }
    if req.Filters.Environment == "" {
        return fmt.Errorf("filters.environment is required")  // ← HAPI violation
    }
    if req.Filters.Priority == "" {
        return fmt.Errorf("filters.priority is required")  // ← HAPI violation
    }
    return nil
}
```

---

## 🛠️ **Required Changes in HAPI**

### **File**: `holmesgpt-api/src/toolsets/workflow_catalog.py`

### **Change 1: Update `_build_filters_from_query` Method**

**Current Code** (`workflow_catalog.py:533-595`):

```python
def _build_filters_from_query(self, query: str, additional_filters: Dict) -> Dict[str, Any]:
    """
    Transform LLM query into WorkflowSearchFilters format
    """
    # Parse structured query per DD-LLM-001
    parts = query.split()
    signal_type = parts[0] if len(parts) > 0 else ""

    # Extract severity from query
    severity = "high"  # Default severity
    query_lower = query.lower()
    for sev in ["critical", "high", "medium", "low"]:
        if sev in query_lower:
            severity = sev
            break

    # Build filters (DD-STORAGE-008 format)
    filters = {
        "signal-type": signal_type,
        "severity": severity
    }
    # ... merge additional_filters ...
    return filters
```

**Updated Code** (with 5 mandatory fields):

```python
def _build_filters_from_query(self, query: str, additional_filters: Dict) -> Dict[str, Any]:
    """
    Transform LLM query into WorkflowSearchFilters format

    Business Requirement: BR-STORAGE-013
    Design Decision: DD-WORKFLOW-001 v1.6 (5 Mandatory Labels)

    Args:
        query: Structured query '<signal_type> <severity> [keywords]' (per DD-LLM-001)
        additional_filters: Additional filters from LLM (optional labels)

    Returns:
        WorkflowSearchFilters dict with 5 mandatory fields:
        - signal_type (REQUIRED)
        - severity (REQUIRED)
        - component (REQUIRED)
        - environment (REQUIRED)
        - priority (REQUIRED)
    """
    # Parse structured query per DD-LLM-001
    parts = query.split()
    signal_type = parts[0] if len(parts) > 0 else ""

    # Extract severity from query
    severity = "high"  # Default severity
    query_lower = query.lower()
    for sev in ["critical", "high", "medium", "low"]:
        if sev in query_lower:
            severity = sev
            break

    # Build filters with 5 MANDATORY fields (DD-WORKFLOW-001 v1.6)
    filters = {
        "signal_type": signal_type,
        "severity": severity,
        # NEW: 3 additional mandatory fields with smart defaults
        "component": self._extract_component_from_rca() or "*",  # Wildcard if unknown
        "environment": self._extract_environment_from_rca() or "*",  # Wildcard if unknown
        "priority": self._map_severity_to_priority(severity),  # Map severity → priority
    }

    # Merge additional filters (optional labels per DD-WORKFLOW-004)
    if additional_filters:
        # Map from LLM filter names to API field names
        filter_mapping = {
            "resource_management": "resource-management",
            "gitops_tool": "gitops-tool",
            "environment": "environment",
            "business_category": "business-category",
            "priority": "priority",
            "risk_tolerance": "risk-tolerance",
            "component": "component",  # Allow LLM to override default
        }

        for llm_key, api_key in filter_mapping.items():
            if llm_key in additional_filters:
                filters[api_key] = additional_filters[llm_key]

    return filters
```

### **Change 2: Add Helper Methods**

```python
def _extract_component_from_rca(self) -> Optional[str]:
    """
    Extract component (pod/deployment/node) from RCA resource.

    Business Requirement: BR-HAPI-250 (Workflow Catalog Search Tool)
    Design Decision: DD-WORKFLOW-001 v1.6 (component is mandatory)

    Returns:
        Component type from RCA resource, or None if not available
    """
    if not self._rca_resource:
        return None

    # Extract kind from RCA resource
    kind = self._rca_resource.get("kind", "").lower()

    # Map Kubernetes kinds to component labels
    kind_mapping = {
        "pod": "pod",
        "deployment": "deployment",
        "replicaset": "deployment",  # ReplicaSets are managed by Deployments
        "statefulset": "statefulset",
        "daemonset": "daemonset",
        "node": "node",
        "service": "service",
        "persistentvolumeclaim": "pvc",
        "persistentvolume": "pv",
    }

    return kind_mapping.get(kind, kind if kind else None)

def _extract_environment_from_rca(self) -> Optional[str]:
    """
    Extract environment from RCA resource namespace.

    Business Requirement: BR-HAPI-250 (Workflow Catalog Search Tool)
    Design Decision: DD-WORKFLOW-001 v1.6 (environment is mandatory)

    Returns:
        Environment inferred from namespace, or None if not available
    """
    if not self._rca_resource:
        return None

    namespace = self._rca_resource.get("namespace", "").lower()

    # Heuristic mapping (customers should use custom_labels for precise control)
    if "prod" in namespace or "production" in namespace:
        return "production"
    elif "stag" in namespace or "staging" in namespace:
        return "staging"
    elif "dev" in namespace or "development" in namespace:
        return "development"
    elif "test" in namespace:
        return "test"

    return None  # Will default to "*" wildcard in caller

def _map_severity_to_priority(self, severity: str) -> str:
    """
    Map severity level to priority level.

    Business Requirement: BR-HAPI-250 (Workflow Catalog Search Tool)
    Design Decision: DD-WORKFLOW-001 v1.6 (priority is mandatory)

    Args:
        severity: critical/high/medium/low

    Returns:
        Priority level: P0/P1/P2/P3
    """
    severity_to_priority = {
        "critical": "P0",
        "high": "P1",
        "medium": "P2",
        "low": "P3",
    }
    return severity_to_priority.get(severity.lower(), "P2")  # Default to P2
```

### **Change 3: Update `SearchWorkflowCatalogTool.__init__`**

**Add RCA resource storage**:

```python
def __init__(
    self,
    data_storage_url: str,
    remediation_id: Optional[str] = None,
    custom_labels: Optional[Dict[str, List[str]]] = None,
    detected_labels: Optional[Dict[str, Any]] = None,
    rca_resource: Optional[Dict[str, Any]] = None,  # ← NEW parameter
    owner_chain: Optional[List[Dict[str, Any]]] = None,
    http_timeout: int = 10
):
    self._data_storage_url = data_storage_url
    self._remediation_id = remediation_id
    self._custom_labels = custom_labels
    self._detected_labels = detected_labels
    self._rca_resource = rca_resource  # ← NEW: Store RCA resource for component/env extraction
    self._owner_chain = owner_chain
    self._http_timeout = http_timeout
```

---

## 🧪 **Test Updates Required**

### **File**: `tests/integration/test_data_storage_label_integration.py`

**Issue**: Tests call `_search_workflows` without passing mandatory fields.

**Example Current Test** (`test_data_storage_label_integration.py:92-107`):

```python
rca_resource = {
    "signal_type": "OOMKilled",
    "kind": "Pod",
    "namespace": "production",
    "name": "api-server-xyz"
}

workflows = tool._search_workflows(
    query="OOMKilled critical memory exhaustion",
    rca_resource=rca_resource,
    filters={},  # ← Empty filters cause 400 errors
    top_k=5
)
```

**Updated Test** (with mandatory fields):

```python
rca_resource = {
    "signal_type": "OOMKilled",
    "kind": "Pod",
    "namespace": "production",
    "name": "api-server-xyz"
}

# Pass mandatory filters explicitly
filters = {
    "component": "pod",         # From rca_resource.kind
    "environment": "production", # From rca_resource.namespace
    "priority": "P0"            # Map from severity=critical
}

workflows = tool._search_workflows(
    query="OOMKilled critical memory exhaustion",
    rca_resource=rca_resource,
    filters=filters,  # ← Now includes 3 mandatory fields (signal_type/severity from query)
    top_k=5
)
```

**Alternative**: Update `SearchWorkflowCatalogTool` to accept `rca_resource` in constructor and extract fields automatically.

---

## 📋 **Implementation Checklist**

### **Phase 1: Core Filter Builder (1-2 hours)**
- [ ] Update `_build_filters_from_query` to include 5 mandatory fields
- [ ] Add `_extract_component_from_rca()` helper method
- [ ] Add `_extract_environment_from_rca()` helper method
- [ ] Add `_map_severity_to_priority()` helper method
- [ ] Update `SearchWorkflowCatalogTool.__init__` to accept `rca_resource`
- [ ] Update all `SearchWorkflowCatalogTool` instantiations to pass `rca_resource`

### **Phase 2: Test Updates (30 minutes)**
- [ ] Update `test_data_storage_label_integration.py` tests to pass mandatory fields
- [ ] Update integration test fixtures to include component/environment/priority
- [ ] Run integration tests against local Data Storage

### **Phase 3: Validation (30 minutes)**
- [ ] Verify 90/90 integration tests passing
- [ ] Check filter logs show all 5 mandatory fields
- [ ] Validate wildcard behavior (component="*", environment="*")
- [ ] Test with real workflow catalog data

---

## 🎯 **Expected Outcomes**

### **After Fix**:
- ✅ 90/90 HAPI integration tests passing (100%)
- ✅ All workflow search requests include 5 mandatory fields
- ✅ Wildcard support for unknown component/environment
- ✅ Smart severity → priority mapping
- ✅ RCA resource context used for component/environment extraction

### **Confidence Assessment**: 85%

**Justification**:
- Authoritative source confirmed (Data Storage Go structs + validation code)
- Clear API contract with validation errors
- Helper methods provide smart defaults for missing context
- Wildcard support prevents over-filtering when context is limited

**Risks**:
- RCA resource may not always have `kind` or `namespace` (mitigated by wildcards)
- Namespace → environment heuristic may not match customer conventions (mitigated by custom_labels override)

---

## 🔗 **Related Documents**

- [NOTICE_DS_EMBEDDING_REMOVAL_TO_HAPI.md](./NOTICE_DS_EMBEDDING_REMOVAL_TO_HAPI.md) - Original notification
- [DD-WORKFLOW-001 v1.6](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - Mandatory label schema
- [FOLLOWUP_DS_WORKFLOW_SEARCH_STILL_BROKEN.md](./FOLLOWUP_DS_WORKFLOW_SEARCH_STILL_BROKEN.md) - Migration 018 fix

---

## 📊 **Progress Tracking**

| Phase | Status | Duration |
|-------|--------|----------|
| Root Cause Analysis | ✅ Complete | 20 minutes |
| Implementation Plan | ✅ Complete | 10 minutes |
| Code Changes | ⏸️  Pending | ~1-2 hours |
| Test Updates | ⏸️  Pending | ~30 minutes |
| Validation | ⏸️  Pending | ~30 minutes |

**Total Estimated Time**: 2-3 hours for complete fix and validation

