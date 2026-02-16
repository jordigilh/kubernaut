# DD-WORKFLOW-002: MCP Workflow Catalog Architecture

> **SUPERSEDED** by [DD-WORKFLOW-016](./DD-WORKFLOW-016-action-type-workflow-indexing.md) (Action-Type Workflow Catalog Indexing, February 2026).
>
> DD-WORKFLOW-016 replaces the single `search_workflow_catalog` tool with a three-step discovery protocol (`list_available_actions`, `list_workflows`, `get_workflow`). The `signal_type`-based querying, semantic search with pgvector embeddings, and Embedding Service architecture defined in this document are no longer valid. The `action_type` taxonomy (DD-WORKFLOW-016) replaces `signal_type` as the primary catalog indexing key. Refer to DD-WORKFLOW-016 for the current HAPI toolset and DS endpoint design.

**Date**: November 14, 2025
**Status**: **SUPERSEDED** by DD-WORKFLOW-016
**Deciders**: Architecture Team
**Version**: 3.3
**Related**: DD-WORKFLOW-012 (Workflow Immutability), ADR-034 (Unified Audit Table), DD-WORKFLOW-014 (Workflow Selection Audit Trail), DD-CONTRACT-001 (AIAnalysis ↔ WorkflowExecution Alignment)

---

## 🔗 **Workflow Immutability Reference**

**CRITICAL**: This DD's semantic search relies on workflow immutability.

**Authority**: **DD-WORKFLOW-012: Workflow Immutability Constraints**
- Workflow embeddings are immutable (cannot change after creation)
- Semantic search results remain consistent for same workflow version
- Description and labels cannot be updated (would invalidate embeddings)

**Cross-Reference**: All semantic search operations assume workflow immutability per DD-WORKFLOW-012.

---

---

## Changelog

### Version 3.3 (2025-11-30)
- **BREAKING**: Standardized all filter field names to **snake_case** for consistency
- Changed `signal-type` → `signal_type` in all filter parameters and examples
- Changed `label.signal-type` → `filters.signal_type` parameter naming convention
- All filter fields now use snake_case: `signal_type`, `severity`, `environment`, `priority`, `custom_labels`
- **Rationale**: snake_case is the standard JSON API convention, aligns with Python (HolmesGPT-API), and matches Kubernetes JSON field naming
- **Impact**: Data Storage must update Go struct JSON tags from `json:"signal-type"` to `json:"signal_type"`
- **Notification**: See `HANDOFF_FILTER_NAMING_STANDARDIZATION.md` for Data Storage team

### Version 3.2 (2025-11-30)
- **BREAKING**: Changed `custom_labels` type from `map[string]string` to `map[string][]string`
- Custom labels now use **subdomain-based structure**: key = subdomain, value = array of label values
- Label values can be **boolean** (`"cost-constrained"`) or **key-value** (`"name=payments"`)
- Query logic: Multiple values within same subdomain are **ORed**, different subdomains are **ANDed**
- SignalProcessing strips `*.kubernaut.io/` prefix - Data Storage receives clean subdomain keys
- Cross-reference: DD-WORKFLOW-001 v1.8, DD-WORKFLOW-004 v2.2, HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md v1.1
- **Impact**: HolmesGPT-API and Data Storage must use new structure

### Version 3.1 (2025-11-30)
- **NEW**: Added `custom_labels` parameter for customer-defined label filtering
- Custom labels are customer-defined via Rego policies (per HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0)
- Custom labels use hard WHERE filter (not boost/penalty scoring) per DD-WORKFLOW-004 v2.1
- Label prefixes: `kubernaut.io/*`, `constraint.kubernaut.io/*`, `custom.kubernaut.io/*`
- Cross-reference: DD-WORKFLOW-001 v1.8 (5 mandatory labels + customer-derived, snake_case)
- **Impact**: Enables filtering by arbitrary customer-defined labels in workflow search
- **SUPERSEDED BY v3.2**: Structure changed to `map[string][]string`

### Version 3.0 (2025-11-29)
- **BREAKING**: `workflow_id` is now a UUID (auto-generated primary key)
- **BREAKING**: Removed `version` from `search_workflow_catalog` response - LLM only needs `workflow_id`
- **BREAKING**: Removed `estimated_duration` from response - field is circumstantial and irrelevant to LLM
- **BREAKING**: Response structure is now FLAT (no nested `workflow` object) per DD-WORKFLOW-002 contract
- **BREAKING**: Renamed `name` to `title` in search response for clarity
- **BREAKING**: Changed `signal_types` (array) to `signal_type` (string) - one workflow per signal type
- `workflow_name` and `version` remain as metadata fields (for humans), not in search response
- Database uses `UNIQUE (workflow_name, version)` constraint to prevent duplicates
- Updated DD-WORKFLOW-012 cross-reference for UUID primary key design
- **Impact**: All consumers must update to use UUID `workflow_id` as single identifier

### Version 2.4 (2025-11-28)
- **BREAKING**: Added `container_image` and `container_digest` to `search_workflow_catalog` response contract
- HolmesGPT-API now resolves `workflow_id` → `container_image` during MCP search (per DD-CONTRACT-001 v1.2)
- RO no longer needs to call Data Storage for workflow resolution - passes through from AIAnalysis
- Added cross-reference to DD-CONTRACT-001 (AIAnalysis ↔ WorkflowExecution Alignment)
- **Impact**: Data Storage must return `container_image` and `container_digest` in workflow search results

### Version 2.3 (2025-11-27)
- **API DESIGN**: Clarified that `remediation_id` is passed in JSON request body (not HTTP header)
- Added cross-reference to DD-WORKFLOW-014 v2.1 for transport mechanism decision
- Updated implementation notes to show JSON body approach for consistency
- **Impact**: Implementation clarification - JSON body is the standard transport

### Version 2.2 (2025-11-27)
- **CRITICAL CLARIFICATION**: Clarified `remediation_id` usage constraint - field is MANDATORY but for CORRELATION/AUDIT ONLY
- Added `usage_note` to `remediation_id` parameter: "Pass-through value from investigation context. Do not interpret or use in search logic."
- Explicitly documented that LLM must NOT use `remediation_id` for RCA analysis or workflow matching
- Updated description to emphasize: "⚠️ IMPORTANT: This field is for CORRELATION/AUDIT ONLY - do NOT use for RCA analysis or workflow matching"
- **Impact**: No functional change - clarifies existing behavior for LLM implementers

### Version 2.1 (2025-11-27)
- **MANDATORY**: Added `remediation_id` as required parameter for audit trail correlation
- Added cross-references to ADR-034 (Unified Audit Table) and DD-WORKFLOW-014 (Workflow Selection Audit Trail)
- Updated MCP tool contract to require `remediation_id` for all workflow search requests
- Ensures workflow selection audit events can be correlated with remediation requests
- **Impact**: All MCP clients must provide `remediation_id` when calling `search_workflow_catalog`

### Version 2.0 (2025-11-22)
- **BREAKING**: Updated all query examples to use structured format per DD-LLM-001
- Changed query format from free-text to `<signal_type> <severity> [keywords]`
- Added `label.*` parameters for exact label matching (Phase 1 filtering)
- Updated Data Flow Example to show structured query + label filters
- Added cross-references to DD-LLM-001 for query format specification
- Clarified that semantic search uses two-phase filtering (exact labels + embedding similarity)

### Version 1.0 (2025-11-14)
- Initial version defining MCP architecture and service responsibilities
- Defined `search_workflow_catalog` and `get_playbook_details` tools
- Established semantic search flow

---

## Context

The Kubernaut architecture requires a way for the LLM (via HolmesGPT API) to search and retrieve remediation workflows based on investigation findings. The workflow catalog needs to be accessible to the LLM without tight coupling between services.

### Problem Statement

- **Need**: LLM must access workflow catalog for remediation selection
- **Challenge**: LLM outputs natural language, not structured queries
- **Requirement**: Semantic search (not just exact label matching)
- **Constraint**: Maintain service boundaries and independence

---

## Decision

**Use MCP (Model Context Protocol) to expose the workflow catalog as tools that the LLM can call.**

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ HolmesGPT API                                                   │
│ - LLM orchestration (Claude 3.5 Sonnet)                        │
│ - Calls MCP tools                                               │
└─────────────────────────────────────────────────────────────────┘
                              ↓
                    MCP Tool Call (JSON-RPC)
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Embedding Service (MCP Server)                                 │
│ - Implements MCP protocol                                       │
│ - Exposes workflow catalog tools                                │
│ - Generates embeddings                                          │
│ - Calls Data Storage REST API                                   │
└─────────────────────────────────────────────────────────────────┘
                              ↓
                    REST API Call (HTTP)
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Data Storage Service                                            │
│ - PostgreSQL + pgvector                                         │
│ - Workflow catalog storage                                      │
│ - Semantic search execution                                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## MCP Tool Specifications

### Tool 1: search_workflow_catalog

**Purpose**: Search for workflows using structured query and label filters

**Query Format**: Per DD-LLM-001, queries must use structured format: `<signal_type> <severity> [optional_keywords]`

```json
{
  "name": "search_workflow_catalog",
  "description": "Search the workflow catalog using structured query format '<signal_type> <severity>' with optional label filters for exact matching.",

  "parameters": {
    "remediation_id": {
      "type": "string",
      "required": true,
      "description": "Remediation request ID for audit correlation. MANDATORY for audit trail tracking per ADR-034. ⚠️ IMPORTANT: This field is for CORRELATION/AUDIT ONLY - do NOT use for RCA analysis or workflow matching. Simply propagate from request context to workflow search for traceability.",
      "example": "req-2025-10-06-abc123",
      "usage_note": "Pass-through value from investigation context. Do not interpret or use in search logic.",
      "transport": "JSON body (not HTTP header) - per DD-WORKFLOW-014 v2.1"
    },

    "query": {
      "type": "string",
      "required": true,
      "description": "Structured query: '<signal_type> <severity> [optional_keywords]' (e.g., 'OOMKilled critical', 'CrashLoopBackOff high configuration')",
      "example": "OOMKilled critical"
    },

    "signal_type": {
      "type": "string",
      "required": false,
      "description": "Exact match filter for signal type (canonical Kubernetes event reason)",
      "example": "OOMKilled"
    },
    "severity": {
      "type": "string",
      "required": false,
      "enum": ["critical", "high", "medium", "low"],
      "description": "Exact match filter for RCA severity assessment",
      "example": "critical"
    },
    "environment": {
      "type": "string",
      "required": false,
      "enum": ["production", "staging", "development", "test"],
      "description": "Exact match filter for environment (pass-through from input)",
      "example": "production"
    },
    "priority": {
      "type": "string",
      "required": false,
      "enum": ["P0", "P1", "P2", "P3"],
      "description": "Exact match filter for business priority (pass-through from input)",
      "example": "P0"
    },
    "risk_tolerance": {
      "type": "string",
      "required": false,
      "enum": ["low", "medium", "high"],
      "description": "Exact match filter for risk tolerance (pass-through from input)",
      "example": "low"
    },
    "business_category": {
      "type": "string",
      "required": false,
      "description": "Exact match filter for business category (pass-through from input)",
      "example": "revenue-critical"
    },

    "custom_labels": {
      "type": "object",
      "required": false,
      "description": "Customer-defined labels for exact match filtering (v3.2: subdomain-based structure). Keys are subdomains (e.g., 'constraint', 'team'), values are arrays of label strings. Labels are defined via Rego policies in Signal Processing. Uses hard WHERE filter (not scoring). Multiple values in same subdomain are ORed; different subdomains are ANDed. See DD-WORKFLOW-001 v1.8 and HANDOFF_CUSTOM_LABELS_QUERY_STRUCTURE.md.",
      "structure": "map[string][]string (subdomain → values)",
      "example": {
        "constraint": ["cost-constrained", "stateful-safe"],
        "team": ["name=payments"],
        "region": ["zone=us-east-1"]
      },
      "label_types": {
        "boolean": "key only (e.g., 'cost-constrained') - presence = true",
        "key_value": "key=value (e.g., 'name=payments') - explicit value"
      },
      "query_logic": {
        "within_subdomain": "OR (any value matches)",
        "between_subdomains": "AND (all subdomains must match)"
      },
      "notes": [
        "Subdomains are customer-defined (no validation by Data Storage)",
        "SignalProcessing strips *.kubernaut.io/ prefix before passing to downstream",
        "Empty arrays are ignored (no filter for that subdomain)",
        "False booleans are omitted by SignalProcessing (not passed)"
      ]
    },

    "top_k": {
      "type": "integer",
      "default": 10,
      "minimum": 1,
      "maximum": 50,
      "description": "Number of results to return"
    }
  },

  "returns": {
    "type": "array",
    "description": "FLAT array of workflow results (no nested objects) - v3.0",
    "items": {
      "workflow_id": "string (UUID - auto-generated primary key, single identifier for all operations)",
      "title": "string (human-readable workflow name)",
      "description": "string (workflow description)",
      "signal_type": "string (the signal type this workflow handles, e.g., 'OOMKilled')",
      "container_image": "string (OCI bundle reference, e.g., quay.io/kubernaut/workflow-oomkill:v1.0.0)",
      "container_digest": "string (sha256 digest for audit, e.g., sha256:abc123...)",
      "confidence": "number (0.0-1.0, semantic similarity score)"
    }
  },
  "notes": [
    "container_image and container_digest are resolved by Data Storage from the workflow catalog (DD-WORKFLOW-009)",
    "HolmesGPT-API passes these through to AIAnalysis.status.selectedWorkflow (DD-CONTRACT-001 v1.2)",
    "RO does NOT need to call Data Storage for workflow resolution - passes through from AIAnalysis"
  ]
}
```

### Tool 2: get_playbook_details

**Purpose**: Get complete details of a specific playbook

```json
{
  "name": "get_playbook_details",
  "description": "Get full details of a specific workflow including steps, prerequisites, and rollback procedures",

  "parameters": {
    "workflow_id": {
      "type": "string (UUID)",
      "required": true,
      "description": "Workflow UUID (from search_workflow_catalog response)"
    }
  },

  "returns": {
    "workflow_id": "string (UUID)",
    "workflow_name": "string (human-readable name)",
    "version": "string (human metadata, informational only)",
    "title": "string",
    "description": "string",
    "signal_type": "string (the signal type this workflow handles)",
    "steps": "array",
    "prerequisites": "array",
    "rollback_steps": "array",
    "risk_level": "string",
    "content": "string (full workflow-schema.yaml)"
  }
}
```

---

## Service Responsibilities

### Embedding Service (MCP Server)

**Responsibilities**:
1. Implement MCP protocol server
2. Expose workflow catalog tools to HolmesGPT API
3. Receive MCP tool calls from LLM
4. Generate embeddings from natural language queries
5. Call Data Storage REST API for semantic search
6. Return ranked playbooks to LLM

**Technology**:
- Python microservice (Flask/FastAPI)
- `mcp` Python library for MCP protocol
- sentence-transformers for embedding generation
- HTTP client for Data Storage integration

---

### Data Storage Service

**Responsibilities**:
1. Store workflow catalog (PostgreSQL)
2. Generate and store workflow embeddings (pgvector)
3. Execute semantic search queries
4. Apply optional label filters
5. Return ranked playbooks

**Technology**:
- Go microservice
- PostgreSQL 16+ with pgvector extension
- REST API endpoints

---

### HolmesGPT API

**Responsibilities**:
1. Orchestrate LLM investigation workflow
2. Call MCP tools (search_workflow_catalog, get_playbook_details)
3. Parse LLM responses
4. Select best workflow based on LLM reasoning

**Technology**:
- Python microservice
- Claude 3.5 Sonnet via Anthropic SDK
- MCP tool calling via Anthropic's tool use API

**Note**: HolmesGPT API implementation details (prompts, Claude integration) are documented in `docs/services/stateless/holmesgpt-api/` and will be reviewed with AIAnalysis service implementation.

---

## Semantic Search Flow

### Step 1: LLM Investigation
```
HolmesGPT API receives alert
  ↓
LLM investigates using Kubernetes tools
  ↓
LLM determines root cause
```

### Step 2: MCP Tool Call
```
LLM decides to search workflows
  ↓
Calls search_workflow_catalog MCP tool (per DD-LLM-001 format)
  query: "OOMKilled critical"
  filters:
    signal_type: "OOMKilled"
    severity: "critical"
    environment: "production"
    priority: "P0"
    custom_labels:
      constraint: ["cost-constrained"]
      team: ["name=payments"]
```

### Step 3: Embedding Generation
```
Embedding Service receives MCP call
  ↓
Generates 384-dim embedding from structured query "OOMKilled critical"
  ↓
Prepares label filters for Phase 1 exact matching
```

### Step 4: Two-Phase Semantic Search (per DD-LLM-001)
```
Embedding Service calls Data Storage REST API
  POST /api/v1/workflows/search
  {
    "query": "OOMKilled critical",
    "embedding": [0.123, -0.456, ...],
    "filters": {
      "signal_type": "OOMKilled",
      "severity": "critical",
      "environment": "production",
      "priority": "P0",
      "custom_labels": {
        "constraint": ["cost-constrained"],
        "team": ["name=payments"]
      }
    },
    "top_k": 10
  }
  ↓
Data Storage executes two-phase search:
  Phase 1: Exact label matching (SQL WHERE clause)
  Phase 2: Semantic ranking (pgvector similarity)

  SELECT *, 1 - (embedding <=> $1) AS confidence
  FROM workflow_catalog
  WHERE status = 'active'
    AND labels->>'signal_type' = 'OOMKilled'
    AND labels->>'severity' = 'critical'
    AND labels->>'environment' = 'production'
    AND labels->>'priority' = 'P0'
    -- Custom labels filtering (DD-HAPI-001: auto-appended by HolmesGPT-API)
    AND custom_labels->'constraint' ? 'cost-constrained'
    AND custom_labels->'team' ? 'name=payments'
  ORDER BY embedding <=> $1
  LIMIT 10
  ↓
Returns ranked workflows with 90-95% confidence scores
```

### Step 5: Workflow Selection
```
Embedding Service returns playbooks to LLM
  ↓
LLM reviews playbooks
  ↓
LLM selects best workflow with reasoning
```

---

## Data Flow Example

```json
// Step 1: LLM calls MCP tool (per DD-LLM-001 structured format)
{
  "tool": "search_workflow_catalog",
  "parameters": {
    "query": "OOMKilled critical",
    "label.signal-type": "OOMKilled",
    "label.severity": "critical",
    "label.environment": "production",
    "label.business-category": "payments",
    "label.risk-tolerance": "low",
    "top_k": 5
  }
}

// Step 2: Embedding Service generates embedding from structured query
embedding = generate_embedding("OOMKilled critical")  // [0.123, -0.456, ..., 0.789]

// Step 3: Embedding Service calls Data Storage with structured query + label filters
POST http://data-storage:8085/api/v1/workflows/search
{
  "query": "OOMKilled critical",
  "embedding": [0.123, -0.456, ..., 0.789],
  "label.signal-type": "OOMKilled",
  "label.severity": "critical",
  "label.environment": "production",
  "label.business-category": "payments",
  "label.risk-tolerance": "low",
  "top_k": 5
}

// Step 4: Data Storage returns workflows with high confidence (90-95%)
// DD-WORKFLOW-002 v3.0: FLAT response structure with UUID workflow_id
{
  "workflows": [
    {
      "workflow_id": "550e8400-e29b-41d4-a716-446655440000",
      "title": "OOMKilled Remediation - Conservative Memory Increase",
      "description": "OOMKilled critical: Increases memory limits conservatively without restart",
      "signal_type": "OOMKilled",
      "container_image": "quay.io/kubernaut/workflow-oom-conservative:v1.2.0",
      "container_digest": "sha256:abc123def456...",
      "confidence": 0.95
    },
    {
      "workflow_id": "660f9500-f30c-52e5-b827-557766551111",
      "title": "OOMKilled Remediation - Horizontal Scaling",
      "description": "OOMKilled critical: Add replicas to distribute load before increasing memory",
      "signal_type": "OOMKilled",
      "container_image": "quay.io/kubernaut/workflow-oom-horizontal:v2.0.0",
      "container_digest": "sha256:def789ghi012...",
      "confidence": 0.88
    }
  ],
  "total_results": 2
}

// Step 5: LLM selects workflow based on confidence
// DD-WORKFLOW-002 v3.0: Only workflow_id (UUID) needed for selection
{
  "selected_workflow": {
    "workflow_id": "550e8400-e29b-41d4-a716-446655440000"
  },
  "reasoning": "MCP search with structured query 'OOMKilled critical' and exact label matching returned this workflow with 95% confidence. Workflow respects low risk tolerance by avoiding service restart."
}
```

---

## Business Requirements

### BR-WORKFLOW-020: MCP Workflow Catalog Tool
**Priority**: P0 (CRITICAL)
**Description**: Data Storage Service MUST expose workflow catalog as MCP tools

**Acceptance Criteria**:
- ✅ Implement `search_workflow_catalog` MCP tool
- ✅ Implement `get_playbook_details` MCP tool
- ✅ Support natural language queries
- ✅ Return structured JSON responses

---

### BR-WORKFLOW-021: Semantic Search with Filters
**Priority**: P0 (CRITICAL)
**Description**: Workflow search MUST combine semantic similarity with optional filters

**Acceptance Criteria**:
- ✅ Generate embeddings from natural language queries
- ✅ Use pgvector cosine similarity for ranking
- ✅ Apply optional filters (signal_types, business_category, etc.)
- ✅ Support exclude_keywords for negative filtering

---

### BR-WORKFLOW-022: Investigation Audit Trail
**Priority**: P0 (CRITICAL)
**Description**: System MUST capture audit trail of MCP tool calls and workflow selection

**Acceptance Criteria**:
- ✅ Record all MCP tool calls (search_workflow_catalog, get_playbook_details)
- ✅ Record LLM reasoning for workflow selection
- ✅ Record similarity scores and match reasons
- ✅ Store audit trail in Data Storage Service

---

### BR-EMBEDDING-006: MCP Server Implementation
**Priority**: P0 (CRITICAL)
**Description**: Embedding Service MUST implement MCP protocol server

**Acceptance Criteria**:
- ✅ Expose `search_workflow_catalog` MCP tool
- ✅ Expose `get_playbook_details` MCP tool
- ✅ Generate embeddings from query text
- ✅ Call Data Storage REST API
- ✅ Return ranked playbooks to LLM

---

## Consequences

### Positive

1. **Service Independence**: HolmesGPT API, Embedding Service, and Data Storage remain loosely coupled
2. **LLM Flexibility**: LLM can use natural language queries, not rigid structures
3. **Standard Protocol**: MCP is a standardized protocol for LLM tool integration
4. **Semantic Search**: Finds relevant playbooks even if labels don't match exactly
5. **Optional Filtering**: LLM can apply filters when relevant, but not required

### Negative

1. **Additional Service**: Embedding Service adds operational complexity
2. **Network Latency**: MCP call + REST API call adds ~100-300ms latency
3. **MCP Protocol Dependency**: Requires MCP library and protocol understanding

### Mitigations

1. **Caching**: Cache workflow embeddings (Redis or materialized view)
2. **Monitoring**: Track MCP tool call latency and success rates
3. **Fallback**: Provide simple label-based search if semantic search fails

---

## Implementation Notes

### MCP Protocol

- **Standard**: Model Context Protocol (https://modelcontextprotocol.io/)
- **Transport**: JSON-RPC over HTTP
- **Library**: `mcp` Python package
- **Tool Calling**: Compatible with Anthropic Claude tool use API

### Embedding Generation

- **Model**: sentence-transformers `all-MiniLM-L6-v2`
- **Dimensions**: 384
- **Latency**: ~100-200ms per query
- **Caching**: Redis cache for workflow embeddings

### Semantic Search

- **Database**: PostgreSQL 16+ with pgvector 0.5.1+
- **Index**: IVFFlat index on embedding column
- **Similarity**: Cosine similarity (`<=>` operator)
- **Latency**: ~50-100ms per query

---

## Related Documents

- **[DD-LLM-001](./adr-041-llm-contract/DD-LLM-001-mcp-search-taxonomy.md)** - ⭐ **REQUIRED**: MCP query format specification and parameter taxonomy
- [DD-WORKFLOW-001](./DD-WORKFLOW-001-mandatory-label-schema.md) - Mandatory label schema for workflows
- [DD-EMBEDDING-001](./DD-EMBEDDING-001-embedding-service-implementation.md) - Embedding Service design
- [DD-STORAGE-008](../services/stateless/data-storage/implementation/DD-STORAGE-008-WORKFLOW-CATALOG-SCHEMA.md) - Workflow catalog schema
- [AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.4](../services/stateless/data-storage/implementation/AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.4.md) - Data Storage implementation

---

**Document Version**: 3.2
**Last Updated**: November 30, 2025
**Status**: ✅ APPROVED (MCP Architecture + UUID Primary Key + Flat Response + Subdomain Custom Labels)
**Next Review**: After custom labels implementation in Data Storage

**Breaking Changes in v3.2**:
- `custom_labels` type changed from `map[string]string` to `map[string][]string`
- Keys are now subdomains (e.g., `constraint`) not full labels (e.g., `constraint.kubernaut.io/cost-constrained`)
- Values are arrays of strings (boolean or key=value format)

**Breaking Changes in v3.0**:
- `workflow_id` is now UUID (auto-generated)
- Response is FLAT (no nested objects)
- Removed `version`, `estimated_duration` from search response
- Changed `signal_types` (array) to `signal_type` (string)
- Renamed `name` to `title`

