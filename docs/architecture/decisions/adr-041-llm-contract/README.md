# ADR-041: LLM Prompt and Response Contract

This directory contains the authoritative documentation for the LLM prompt structure and response format used by the HolmesGPT API for Root Cause Analysis (RCA) and workflow selection.

## 📁 Document Index

### Core Contract
- **[ADR-041-llm-prompt-response-contract.md](./ADR-041-llm-prompt-response-contract.md)** (v2.7)
  - Main contract document defining prompt structure and response format
  - Establishes design principles (single workflow per incident, observable facts only)
  - Defines the contract between HolmesGPT API, LLM Provider, and downstream services

### Supporting Decisions
- **[DD-LLM-001-mcp-search-taxonomy.md](./DD-LLM-001-mcp-search-taxonomy.md)** (v1.0)
  - MCP workflow search parameter taxonomy
  - Query format specification: `<signal_type> <severity> [optional_keywords]`
  - Canonical signal type and severity taxonomies
  - Business/policy field pass-through requirements
  - Optimization strategies for high confidence scores (90-95%)

## 🔗 Related Documents

### External Dependencies
- **[DD-WORKFLOW-001](../DD-WORKFLOW-001-mandatory-label-schema.md)** (v1.2)
  - Mandatory workflow label schema (7 labels)
  - Signal type canonical values
  - Label matching rules for MCP search

- **[DD-WORKFLOW-003](../DD-WORKFLOW-003-parameterized-actions.md)** (v2.2)
  - Parameterized workflow actions
  - Parameter schema format (JSON Schema)
  - LLM parameter population requirements

- **[DD-STORAGE-008](../../services/stateless/data-storage/implementation/DD-STORAGE-008-PLAYBOOK-CATALOG-SCHEMA.md)** (v1.2)
  - Workflow catalog database schema
  - MCP search API implementation
  - Semantic search with pgvector

## 📊 Document Relationships

```
ADR-041 (LLM Prompt/Response Contract)
├── DD-LLM-001 (MCP Search Taxonomy)
│   ├── Query Format: <signal_type> <severity>
│   ├── Signal Type Taxonomy → DD-WORKFLOW-001
│   ├── Severity Taxonomy (4 levels)
│   └── Business/Policy Labels → DD-WORKFLOW-001
│
├── DD-WORKFLOW-001 (Label Schema v1.4)
│   ├── 5 Mandatory Labels + DetectedLabels + CustomLabels
│   ├── Label Matching Rules (pass-through principle)
│   └── Workflow Description Format
│
├── DD-WORKFLOW-003 (Parameterized Actions)
│   ├── Parameter Schema (JSON Schema)
│   └── LLM Parameter Population
│
└── DD-STORAGE-008 (Workflow Catalog)
    ├── Database Schema
    ├── MCP Search API
    └── Semantic Search Implementation
```

## 🎯 Key Concepts

### MCP Workflow Search
The LLM searches for workflows using:
1. **Query String**: `<signal_type> <severity> [optional_keywords]`
   - Example: `"OOMKilled critical"`
   - Used for semantic similarity ranking (pgvector embeddings)

2. **Label Filters**: Exact matching on 6 labels
   - `signal_type`: Canonical Kubernetes event (e.g., "OOMKilled")
   - `severity`: RCA severity assessment (critical/high/medium/low)
   - `environment`: production/staging/development (pass-through)
   - `priority`: P0/P1/P2/P3 (pass-through)
   - `risk_tolerance`: low/medium/high (pass-through)
   - `business_category`: revenue-critical/etc (pass-through)

3. **Result**: List of workflows with confidence scores (90-95% for exact matches)

### Field Roles
- **LLM Determines** (from RCA): `signal_type`, `severity`, query construction
- **LLM Pass-Through** (business/policy): `environment`, `priority`, `risk_tolerance`, `business_category`
- **LLM Populates** (workflow parameters): Resource details (namespace, name, etc.)

### Confidence Score Optimization
**Strategy**: Exact label matching + semantic ranking
- **Before**: 60-70% confidence (semantic search only)
- **After**: 90-95% confidence (exact filtering + semantic ranking)

## 🔄 Version History

| Document | Version | Date | Key Changes |
|----------|---------|------|-------------|
| ADR-041 | 2.7 | 2025-11-16 | Simplified to first-time incident prompt |
| DD-LLM-001 | 1.0 | 2025-11-16 | Initial MCP search taxonomy |
| DD-WORKFLOW-001 | 1.2 | 2025-11-16 | Added MCP search clarifications |
| DD-WORKFLOW-003 | 2.2 | 2025-11-15 | Resolved parameter naming and validation |
| DD-STORAGE-008 | 1.2 | 2025-11-13 | Added parameters field |

## 📝 Implementation Status

### v1.0 MVP (Current)
- ✅ ADR-041 prompt structure defined
- ✅ DD-LLM-001 taxonomy defined
- ✅ DD-WORKFLOW-001 label schema updated
- 🔄 HolmesGPT API implementation (in progress)
- ⏳ Unit tests for prompt generation (pending)
- ⏳ Integration tests with real LLM (pending)

### Future Enhancements
- v1.0: Deduplication and storm detection in prompt (Gateway service)
- v1.1: Recovery/retry prompt for failed remediations
- v2.0: Multi-step workflow orchestration
- v2.0: Workflow effectiveness learning

## 🧪 Testing

### Unit Tests
Location: `kubernaut-agent/tests/unit/`
- `test_prompt_generation_adr041.py`: Validates prompt structure against ADR-041
- `test_recovery_analysis.py`: Tests recovery analysis endpoint

### Integration Tests
Location: `kubernaut-agent/tests/integration/` (future)
- LLM query construction validation
- MCP search parameter validation
- Workflow parameter population validation

## 📚 Quick Reference

### For LLM Prompt Engineers
1. Read **ADR-041** for overall prompt structure
2. Read **DD-LLM-001** for MCP search query format
3. Reference **DD-WORKFLOW-001** for valid signal types and severities

### For Backend Developers
1. Read **DD-STORAGE-008** for MCP search API implementation
2. Read **DD-WORKFLOW-001** for label schema and validation
3. Read **DD-WORKFLOW-003** for parameter schema format

### For Workflow Authors
1. Read **DD-WORKFLOW-001** for mandatory labels
2. Read **DD-WORKFLOW-003** for parameter schema
3. Follow description format: `"<signal_type> <severity>: <description>"`

## 🔍 Search Tips

**Finding signal types**: See DD-WORKFLOW-001 section "Valid Values (Authoritative)"
**Finding severity levels**: See DD-LLM-001 section "RCA Severity Taxonomy"
**Finding label schema**: See DD-WORKFLOW-001 section "AUTHORITATIVE LABEL DEFINITIONS"
**Finding query format**: See DD-LLM-001 section "Query Format Specification"

---

**Last Updated**: 2025-11-16
**Maintained By**: Kubernaut Architecture Team
**Questions**: See individual documents for specific topics

