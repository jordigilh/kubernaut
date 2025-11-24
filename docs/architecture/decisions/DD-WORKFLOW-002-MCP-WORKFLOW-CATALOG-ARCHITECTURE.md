# DD-WORKFLOW-002: MCP Workflow Catalog Architecture

**Date**: November 14, 2025
**Status**: ✅ APPROVED
**Deciders**: Architecture Team
**Version**: 2.0

---

## Changelog

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
    "query": {
      "type": "string",
      "required": true,
      "description": "Structured query: '<signal_type> <severity> [optional_keywords]' (e.g., 'OOMKilled critical', 'CrashLoopBackOff high configuration')",
      "example": "OOMKilled critical"
    },

    "label.signal-type": {
      "type": "string",
      "required": false,
      "description": "Exact match filter for signal type (canonical Kubernetes event reason)",
      "example": "OOMKilled"
    },
    "label.severity": {
      "type": "string",
      "required": false,
      "enum": ["critical", "high", "medium", "low"],
      "description": "Exact match filter for RCA severity assessment",
      "example": "critical"
    },
    "label.environment": {
      "type": "string",
      "required": false,
      "enum": ["production", "staging", "development", "test"],
      "description": "Exact match filter for environment (pass-through from input)",
      "example": "production"
    },
    "label.priority": {
      "type": "string",
      "required": false,
      "enum": ["P0", "P1", "P2", "P3"],
      "description": "Exact match filter for business priority (pass-through from input)",
      "example": "P0"
    },
    "label.risk-tolerance": {
      "type": "string",
      "required": false,
      "enum": ["low", "medium", "high"],
      "description": "Exact match filter for risk tolerance (pass-through from input)",
      "example": "low"
    },
    "label.business-category": {
      "type": "string",
      "required": false,
      "description": "Exact match filter for business category (pass-through from input)",
      "example": "revenue-critical"
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
    "items": {
      "workflow_id": "string",
      "version": "string",
      "title": "string",
      "description": "string",
      "signal_types": "array",
      "similarity_score": "number (0.0-1.0)",
      "estimated_duration": "string",
      "success_rate": "number (0.0-1.0)"
    }
  }
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
      "type": "string",
      "required": true,
      "description": "Playbook identifier"
    },
    "version": {
      "type": "string",
      "required": true,
      "description": "Playbook version (semantic version)"
    }
  },

  "returns": {
    "workflow_id": "string",
    "version": "string",
    "title": "string",
    "description": "string",
    "signal_types": "array",
    "steps": "array",
    "prerequisites": "array",
    "rollback_steps": "array",
    "estimated_duration": "string",
    "risk_level": "string",
    "success_rate": "number"
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
  label.signal-type: "OOMKilled"
  label.severity: "critical"
  label.environment: "production"
  label.business-category: "payments"
  label.risk-tolerance: "low"
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
    "label.signal-type": "OOMKilled",
    "label.severity": "critical",
    "label.environment": "production",
    "label.business-category": "payments",
    "label.risk-tolerance": "low",
    "top_k": 10
  }
  ↓
Data Storage executes two-phase search:
  Phase 1: Exact label matching (SQL WHERE clause)
  Phase 2: Semantic ranking (pgvector similarity)

  SELECT *, 1 - (embedding <=> $1) AS confidence
  FROM workflow_catalog
  WHERE status = 'active'
    AND labels->>'signal-type' = 'OOMKilled'
    AND labels->>'severity' = 'critical'
    AND labels->>'environment' = 'production'
    AND labels->>'business-category' = 'payments'
    AND labels->>'risk-tolerance' = 'low'
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
{
  "workflows": [
    {
      "workflow_id": "increase-memory-conservative-oom",
      "version": "v1.2",
      "title": "OOMKilled Remediation - Conservative Memory Increase",
      "description": "OOMKilled critical: Increases memory limits conservatively without restart",
      "labels": {
        "signal-type": "OOMKilled",
        "severity": "critical",
        "environment": "production",
        "business-category": "payments",
        "risk-tolerance": "low"
      },
      "confidence": 0.95,
      "estimated_duration": "10 minutes",
      "success_rate": 0.92
    },
    {
      "workflow_id": "scale-horizontal-oom-recovery",
      "version": "v2.0",
      "title": "OOMKilled Remediation - Horizontal Scaling",
      "description": "OOMKilled critical: Add replicas to distribute load before increasing memory",
      "labels": {
        "signal-type": "OOMKilled",
        "severity": "critical",
        "environment": "production",
        "business-category": "payments",
        "risk-tolerance": "medium"
      },
      "confidence": 0.88,
      "estimated_duration": "15 minutes",
      "success_rate": 0.85
    }
  ],
  "total_results": 2
}

// Step 5: LLM selects workflow based on confidence and risk tolerance match
{
  "selected_workflow": {
    "workflow_id": "increase-memory-conservative-oom",
    "version": "v1.2"
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

**Document Version**: 2.0
**Last Updated**: November 22, 2025
**Status**: ✅ APPROVED (MCP Architecture + DD-LLM-001 Query Format)
**Next Review**: After Embedding Service V1.0 deployment

**Breaking Changes in v2.0**: Query format changed from free-text to structured format per DD-LLM-001. All implementations must use `<signal_type> <severity>` query format with `label.*` parameters for exact matching.

