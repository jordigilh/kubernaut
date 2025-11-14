# DD-PLAYBOOK-002: MCP Playbook Catalog Architecture

**Date**: November 14, 2025
**Status**: ✅ APPROVED
**Deciders**: Architecture Team
**Version**: 1.0

---

## Context

The Kubernaut architecture requires a way for the LLM (via HolmesGPT API) to search and retrieve remediation playbooks based on investigation findings. The playbook catalog needs to be accessible to the LLM without tight coupling between services.

### Problem Statement

- **Need**: LLM must access playbook catalog for remediation selection
- **Challenge**: LLM outputs natural language, not structured queries
- **Requirement**: Semantic search (not just exact label matching)
- **Constraint**: Maintain service boundaries and independence

---

## Decision

**Use MCP (Model Context Protocol) to expose the playbook catalog as tools that the LLM can call.**

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
│ - Exposes playbook catalog tools                                │
│ - Generates embeddings                                          │
│ - Calls Data Storage REST API                                   │
└─────────────────────────────────────────────────────────────────┘
                              ↓
                    REST API Call (HTTP)
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Data Storage Service                                            │
│ - PostgreSQL + pgvector                                         │
│ - Playbook catalog storage                                      │
│ - Semantic search execution                                     │
└─────────────────────────────────────────────────────────────────┘
```

---

## MCP Tool Specifications

### Tool 1: search_playbook_catalog

**Purpose**: Search for playbooks using natural language query and optional filters

```json
{
  "name": "search_playbook_catalog",
  "description": "Search the playbook catalog using natural language. Provide a description of the problem and desired remediation approach.",

  "parameters": {
    "query": {
      "type": "string",
      "required": true,
      "description": "Natural language description of the problem, root cause, and desired remediation",
      "example": "Memory leak from unclosed database connections causing OOMKilled"
    },

    "filters": {
      "type": "object",
      "required": false,
      "description": "Optional filters to narrow search results",
      "properties": {
        "signal_types": {
          "type": "array",
          "items": {"type": "string"},
          "description": "Filter by signal types (e.g., ['MemoryLeak', 'OOMKilled'])"
        },
        "business_category": {
          "type": "string",
          "description": "Filter by business category (e.g., 'payments', 'auth')"
        },
        "risk_tolerance": {
          "type": "string",
          "enum": ["low", "medium", "high"],
          "description": "Filter by risk tolerance level"
        },
        "environment": {
          "type": "string",
          "enum": ["production", "staging", "development"],
          "description": "Filter by environment"
        },
        "exclude_keywords": {
          "type": "array",
          "items": {"type": "string"},
          "description": "Keywords to exclude from results"
        }
      }
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
      "playbook_id": "string",
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
  "description": "Get full details of a specific playbook including steps, prerequisites, and rollback procedures",

  "parameters": {
    "playbook_id": {
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
    "playbook_id": "string",
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
2. Expose playbook catalog tools to HolmesGPT API
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
1. Store playbook catalog (PostgreSQL)
2. Generate and store playbook embeddings (pgvector)
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
2. Call MCP tools (search_playbook_catalog, get_playbook_details)
3. Parse LLM responses
4. Select best playbook based on LLM reasoning

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
LLM decides to search playbooks
  ↓
Calls search_playbook_catalog MCP tool
  query: "Memory leak from unclosed connections..."
  filters: {business_category: "payments", risk_tolerance: "low"}
```

### Step 3: Embedding Generation
```
Embedding Service receives MCP call
  ↓
Generates 384-dim embedding from query text
  ↓
Applies optional filters
```

### Step 4: Semantic Search
```
Embedding Service calls Data Storage REST API
  POST /api/v1/playbooks/search
  {
    "embedding": [0.123, -0.456, ...],
    "filters": {business_category: "payments", ...},
    "top_k": 10
  }
  ↓
Data Storage executes pgvector similarity search
  SELECT * FROM playbook_catalog
  ORDER BY embedding <=> $1::vector
  WHERE business_category = 'payments'
  LIMIT 10
  ↓
Returns ranked playbooks
```

### Step 5: Playbook Selection
```
Embedding Service returns playbooks to LLM
  ↓
LLM reviews playbooks
  ↓
LLM selects best playbook with reasoning
```

---

## Data Flow Example

```json
// Step 1: LLM calls MCP tool
{
  "tool": "search_playbook_catalog",
  "parameters": {
    "query": "Memory leak from unclosed database connections causing OOMKilled. Need restart and connection pool timeout configuration.",
    "filters": {
      "signal_types": ["MemoryLeak", "DatabaseConnectionLeak", "OOMKilled"],
      "business_category": "payments",
      "risk_tolerance": "low"
    },
    "top_k": 5
  }
}

// Step 2: Embedding Service generates embedding
embedding = generate_embedding(query)  // [0.123, -0.456, ..., 0.789]

// Step 3: Embedding Service calls Data Storage
POST http://data-storage:8085/api/v1/playbooks/search
{
  "embedding": [0.123, -0.456, ..., 0.789],
  "filters": {
    "signal_types": ["MemoryLeak", "DatabaseConnectionLeak", "OOMKilled"],
    "business_category": "payments",
    "risk_tolerance": "low"
  },
  "top_k": 5
}

// Step 4: Data Storage returns playbooks
{
  "playbooks": [
    {
      "playbook_id": "database-connection-leak-001",
      "version": "1.2.0",
      "title": "Database Connection Leak Remediation",
      "description": "Addresses memory leaks caused by unclosed database connections...",
      "signal_types": ["MemoryLeak", "DatabaseConnectionLeak", "OOMKilled"],
      "similarity_score": 0.94,
      "estimated_duration": "15 minutes",
      "success_rate": 0.92
    },
    {
      "playbook_id": "memory-leak-generic-001",
      "version": "1.0.0",
      "title": "Generic Memory Leak Investigation",
      "description": "General memory leak troubleshooting...",
      "signal_types": ["MemoryLeak"],
      "similarity_score": 0.78,
      "estimated_duration": "20 minutes",
      "success_rate": 0.85
    }
  ]
}

// Step 5: LLM selects playbook
{
  "selected_playbook": {
    "playbook_id": "database-connection-leak-001",
    "version": "1.2.0"
  },
  "reasoning": "This playbook directly addresses the root cause (unclosed connections) with a proven 92% success rate..."
}
```

---

## Business Requirements

### BR-PLAYBOOK-020: MCP Playbook Catalog Tool
**Priority**: P0 (CRITICAL)
**Description**: Data Storage Service MUST expose playbook catalog as MCP tools

**Acceptance Criteria**:
- ✅ Implement `search_playbook_catalog` MCP tool
- ✅ Implement `get_playbook_details` MCP tool
- ✅ Support natural language queries
- ✅ Return structured JSON responses

---

### BR-PLAYBOOK-021: Semantic Search with Filters
**Priority**: P0 (CRITICAL)
**Description**: Playbook search MUST combine semantic similarity with optional filters

**Acceptance Criteria**:
- ✅ Generate embeddings from natural language queries
- ✅ Use pgvector cosine similarity for ranking
- ✅ Apply optional filters (signal_types, business_category, etc.)
- ✅ Support exclude_keywords for negative filtering

---

### BR-PLAYBOOK-022: Investigation Audit Trail
**Priority**: P0 (CRITICAL)
**Description**: System MUST capture audit trail of MCP tool calls and playbook selection

**Acceptance Criteria**:
- ✅ Record all MCP tool calls (search_playbook_catalog, get_playbook_details)
- ✅ Record LLM reasoning for playbook selection
- ✅ Record similarity scores and match reasons
- ✅ Store audit trail in Data Storage Service

---

### BR-EMBEDDING-006: MCP Server Implementation
**Priority**: P0 (CRITICAL)
**Description**: Embedding Service MUST implement MCP protocol server

**Acceptance Criteria**:
- ✅ Expose `search_playbook_catalog` MCP tool
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

1. **Caching**: Cache playbook embeddings (Redis or materialized view)
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
- **Caching**: Redis cache for playbook embeddings

### Semantic Search

- **Database**: PostgreSQL 16+ with pgvector 0.5.1+
- **Index**: IVFFlat index on embedding column
- **Similarity**: Cosine similarity (`<=>` operator)
- **Latency**: ~50-100ms per query

---

## Related Documents

- [DD-PLAYBOOK-001](./DD-PLAYBOOK-001-mandatory-label-schema.md) - Mandatory label schema
- [DD-EMBEDDING-001](./DD-EMBEDDING-001-embedding-service-implementation.md) - Embedding Service design
- [DD-STORAGE-008](../services/stateless/data-storage/implementation/DD-STORAGE-008-PLAYBOOK-CATALOG-SCHEMA.md) - Playbook catalog schema
- [AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.4](../services/stateless/data-storage/implementation/AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.4.md) - Data Storage implementation

---

**Document Version**: 1.0
**Last Updated**: November 14, 2025
**Status**: ✅ APPROVED (MCP Architecture)
**Next Review**: After Embedding Service V1.0 deployment

