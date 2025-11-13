# Context API Deprecation and Unified Audit Implementation Plan

## 🚨 CRITICAL FINDING: Notification Audit Misalignment

**Current State**: `notification_audit` table exists but does NOT follow ADR-034 unified audit design
**Required Action**: Refactor to unified `audit_events` table with event sourcing pattern

---

## Phase 1: Context API Deprecation and Code Salvage (4-6 hours)

### Task 1.1: Triage Context API Semantic Search Implementation
**Finding**: Context API has NO semantic search implementation
- `pkg/contextapi/query/executor.go:454-458` - Returns error "not implemented"
- `pkg/contextapi/query/router.go:87-98` - Stub only, delegates to non-existent implementation
- **Conclusion**: Nothing to migrate - semantic search was never implemented in Context API

### Task 1.2: Triage Context API Tests for Salvageable Coverage
**Files to analyze**:
- `test/integration/contextapi/02_cache_fallback_test.go` - Cache behavior tests
- `test/integration/contextapi/11_aggregation_api_test.go` - Aggregation tests
- `test/integration/contextapi/11_aggregation_edge_cases_test.go` - Edge case tests
- `test/integration/contextapi/13_graceful_shutdown_test.go` - Graceful shutdown tests

**Action**: Compare with Data Storage tests to identify gaps:
- `test/integration/datastorage/` - Check if similar test coverage exists
- Focus on: cache behavior, aggregation edge cases, graceful shutdown patterns
- **Expected outcome**: Likely minimal salvage since Context API was mostly stubs

### Task 1.3: Triage Context API Business Requirements
**Files to analyze**:
- `docs/services/stateless/context-api/BUSINESS_REQUIREMENTS.md`
- `docs/services/stateless/context-api/BR_MAPPING.md`

**Actions**:
1. Identify BRs that are still relevant (semantic search, aggregation)
2. Check if BRs are duplicated in Data Storage BRs
3. Move non-duplicate relevant BRs to Data Storage BR document
4. Mark deprecated BRs as such

### Task 1.4: Update Documentation to Mark Context API as Deprecated
**Files to update**:
- `docs/architecture/SERVICE_DEPENDENCY_MAP.md` - Remove Context API from active services
- `docs/architecture/MICROSERVICES_COMMUNICATION_ARCHITECTURE.md` - Mark as deprecated
- `docs/services/README.md` - Move to deprecated section
- `README.md` - Remove from active services list
- ADRs/DDs referencing Context API - Add deprecation notice

**Template for deprecation notice**:
```
> DEPRECATED (2025-11-13): Context API has been deprecated in favor of Data Storage Service.
> Semantic search functionality moved to Data Storage Service (BR-STORAGE-012, BR-STORAGE-013).
> See: DD-CONTEXT-006 for deprecation rationale.
```

### Task 1.5: Remove Context API Code (After Salvage Complete)
**Files/directories to remove**:
- `pkg/contextapi/` - All Context API business logic
- `cmd/contextapi/` - Main application
- `test/integration/contextapi/` - Integration tests (after salvaging useful patterns)
- `test/unit/contextapi/` - Unit tests
- `test/e2e/contextapi/` - E2E tests

**Exception - Keep shared code**:
- Any models/types used by other services
- Shared utilities if referenced elsewhere

---

## Phase 2: Unified Audit Table Implementation (ADR-034) (12-16 hours)

### Task 2.1: Read and Analyze ADR-034 (Unified Audit Table Design)
**File**: `docs/architecture/decisions/ADR-034-unified-audit-table-design.md`

**Key Requirements**:
- Unified `audit_events` table with event sourcing pattern
- Structured columns + JSONB hybrid (NOT service-specific tables)
- Cross-service correlation tracking (correlation_id, parent_event_id, trace_id)
- Partitioning by event_date for performance
- Industry-standard schema (AWS CloudTrail, Google Cloud Audit Logs pattern)

**Critical Finding**:
- Existing `notification_audit` table (migration 010) does NOT follow ADR-034
- Must be refactored to unified `audit_events` table

### Task 2.2: Analyze Existing Notification Audit Implementation
**Files to analyze**:
- `migrations/010_audit_write_api_phase1.sql` - Current schema
- `pkg/datastorage/repository/notification_audit_repository.go` - Current implementation
- `pkg/datastorage/models/notification_audit.go` - Current models
- `pkg/datastorage/server/audit_handlers.go` - Current HTTP API
- `test/integration/datastorage/schema_validation_test.go` - Current tests

**Extract**:
- What data is currently captured?
- What queries are supported?
- What can be reused vs must be refactored?

### Task 2.3: Create Unified Audit Implementation Plan
**File to create**: `docs/services/stateless/data-storage/implementation/UNIFIED_AUDIT_IMPLEMENTATION_PLAN.md`

**Plan structure**:
1. **Executive Summary**
   - ADR-034 compliance requirement
   - Refactor existing notification_audit + add 5 service audit events

2. **Current State Analysis**
   - Existing notification_audit schema vs ADR-034 schema
   - Gap analysis and migration requirements

3. **Implementation Strategy**:
   - **Phase 2.1**: Create unified `audit_events` table (ADR-034 schema)
   - **Phase 2.2**: Migrate notification_audit data to audit_events
   - **Phase 2.3**: Refactor notification repository to use audit_events
   - **Phase 2.4**: Add audit events for 5 other services:
     1. `signal_processing_audit` → `gateway.signal.*` events
     2. `orchestration_audit` → `orchestrator.*` events
     3. `ai_analysis_audit` → `ai.analysis.*` events
     4. `workflow_execution_audit` → `workflow.execution.*` events
     5. `effectiveness_audit` → `effectiveness.assessment.*` events

4. **Migration Strategy**
   - Data migration from notification_audit to audit_events
   - Backward compatibility during transition
   - Deprecation timeline for old table

5. **Technical Decisions**
   - Event type taxonomy (e.g., `notification.sent`, `notification.failed`)
   - JSONB payload schema for each service
   - Index strategy for common queries

6. **Success Metrics**
   - Zero data loss during migration
   - Query performance maintained or improved
   - All 6 services emitting audit events

7. **Deployment Plan**
   - Phased rollout (notification first, then others)
   - Rollback strategy

8. **Implementation Checklist**
   - Schema creation
   - Data migration
   - Code refactoring
   - Testing (unit, integration, E2E)

**Key Sections**:
- **ADR-034 Compliance**: Explicit mapping to ADR-034 requirements
- **Event Type Taxonomy**: Define event_type for all 6 services
- **JSONB Payload Schemas**: Document event_data structure per service
- **Correlation Tracking**: How remediation_id maps to correlation_id

---

## Phase 3: Playbook Catalog Semantic Search Implementation Plan (8-10 hours)

### Task 3.1: Verify JSON Format Alignment with DD-CONTEXT-005
**File to read**: `docs/architecture/decisions/DD-CONTEXT-005-minimal-llm-response-schema.md`

**Verify**:
- Response schema: `{playbook_id, version, description, confidence}`
- Query parameters: `incident_type, labels, min_confidence, max_results`
- Filter Before LLM pattern compliance
- **CORRECTED Label filters**: `environment`, `priority`, `business_category` (NOT `risk_tolerance` - derived from environment)

**Current alignment