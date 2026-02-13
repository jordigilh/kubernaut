# BR-WORKFLOW-001: Workflow Registry Management

> **SUPERSEDED** by [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) (Workflow Lifecycle Component Interactions, February 2026).
>
> DD-WORKFLOW-017 replaces the registry model defined here with OCI pullspec-only registration, `action_type`-based indexing (DD-WORKFLOW-016), and a three-step discovery protocol. The `/playbooks` API paths, full JSON registration payload, and `incident_type`-based querying defined in this document are no longer valid. Refer to DD-WORKFLOW-017 for the current authoritative workflow lifecycle design.

**Business Requirement ID**: BR-WORKFLOW-001
**Category**: Workflow Catalog Service
**Priority**: P0
**Target Version**: V1
**Status**: **SUPERSEDED** by DD-WORKFLOW-017
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 introduces the **Remediation Workflow Catalog** with a Hybrid Model (90% catalog selection + 9% chaining + 1% manual). The Workflow Catalog Service must provide a centralized registry to store, retrieve, and manage remediation workflows and their versions.

**Current Limitations**:
- ❌ No centralized workflow registry (playbooks scattered across code/configs)
- ❌ No versioning system for playbooks
- ❌ No API to query available playbooks
- ❌ AI cannot discover which playbooks exist for a given incident type
- ❌ No workflow metadata (description, author, tags, status)
- ❌ Cannot deprecate old playbook versions

**Impact**:
- AI cannot implement ADR-033 Hybrid Model (90% catalog selection)
- No single source of truth for remediation workflows
- Teams cannot discover existing playbooks before creating duplicates
- No versioning strategy for playbook improvements
- Playbook management is manual and error-prone

---

## 🎯 **Business Objective**

**Provide a centralized workflow registry with versioning, metadata, and query APIs to enable ADR-033 catalog-based remediation.**

### **Success Criteria**
1. ✅ Workflow Catalog Service provides REST API to create/read/update playbooks
2. ✅ Each playbook has unique ID + version (e.g., `pod-oom-recovery:v1.2`)
3. ✅ Playbooks include metadata: description, tags, incident_types, author, status
4. ✅ API supports querying playbooks by incident_type
5. ✅ API supports listing all versions of a playbook
6. ✅ Playbook status lifecycle: draft → active → deprecated
7. ✅ 100% of remediation workflows are registered in catalog (target: 20+ playbooks)

---

## 📊 **Use Cases**

### **Use Case 1: AI Discovers Playbooks for Incident Type**

**Scenario**: AI receives `pod-oom-killer` alert and needs to find all available playbooks for this incident type.

**Current Flow** (Without BR-WORKFLOW-001):
```
1. AI receives pod-oom-killer alert
2. No centralized workflow registry exists
3. ❌ AI cannot discover available playbooks
4. ❌ AI falls back to manual escalation (not catalog selection)
5. ❌ ADR-033 Hybrid Model cannot function
```

**Desired Flow with BR-WORKFLOW-001**:
```
1. AI receives pod-oom-killer alert
2. AI queries Workflow Catalog:
   GET /api/v1/playbooks?incident_type=pod-oom-killer&status=active
3. Response:
   [
     {
       "playbook_id": "pod-oom-recovery",
       "version": "v1.2",
       "description": "Increases memory limits and restarts pod",
       "incident_types": ["pod-oom-killer", "container-memory-pressure"],
       "status": "active",
       "success_rate": 0.89  // from Data Storage historical data
     },
     {
       "playbook_id": "pod-oom-vertical-scaling",
       "version": "v1.0",
       "description": "Triggers VPA recommendation",
       "incident_types": ["pod-oom-killer"],
       "status": "active",
       "success_rate": 0.75
     }
   ]
4. ✅ AI selects pod-oom-recovery v1.2 (highest success rate)
5. ✅ ADR-033 catalog-based selection operational
```

---

### **Use Case 2: Team Registers New Playbook Version**

**Scenario**: Team creates `pod-oom-recovery v1.3` with improved memory allocation algorithm.

**Current Flow**:
```
1. Team creates new playbook version
2. No workflow registry exists
3. ❌ Must manually update AI configuration
4. ❌ No version history tracking
5. ❌ Other teams don't discover new version
```

**Desired Flow with BR-WORKFLOW-001**:
```
1. Team creates playbook YAML:
   apiVersion: playbook.kubernaut.ai/v1
   kind: RemediationPlaybook
   metadata:
     name: pod-oom-recovery
     version: v1.3
   spec:
     description: "Improved memory allocation with 20% headroom"
     incidentTypes:
       - pod-oom-killer
       - container-memory-pressure
     steps:
       - action: increase_memory
         parameters:
           headroom: 1.2  # 20% increase from v1.2
       - action: restart_pod
2. Team registers via API:
   POST /api/v1/playbooks
   Body: {playbook YAML content}
3. Workflow Catalog validates and stores playbook
4. ✅ AI automatically discovers v1.3 in next query
5. ✅ Team can compare v1.3 vs v1.2 effectiveness using Data Storage API
6. ✅ Centralized version history maintained
```

---

### **Use Case 3: Deprecate Old Playbook Version**

**Scenario**: `pod-oom-recovery v1.1` has 40% success rate. Team wants to deprecate it in favor of v1.2 (89% success rate).

**Current Flow**:
```
1. Team identifies v1.1 as ineffective
2. No playbook status management
3. ❌ AI might still select v1.1
4. ❌ Manual coordination required to remove v1.1
5. ❌ No audit trail of deprecation decision
```

**Desired Flow with BR-WORKFLOW-001**:
```
1. Team queries playbook versions:
   GET /api/v1/playbooks/pod-oom-recovery/versions
2. Response shows v1.1 (40%) vs v1.2 (89%) success rates
3. Team deprecates v1.1:
   PATCH /api/v1/playbooks/pod-oom-recovery/versions/v1.1
   Body: {"status": "deprecated", "reason": "Low success rate, superseded by v1.2"}
4. Workflow Catalog updates status
5. ✅ AI excludes deprecated playbooks from queries (status=active only)
6. ✅ Audit trail: who deprecated, when, why
7. ✅ v1.1 still queryable for historical analysis (not deleted)
```

---

## 🔧 **Functional Requirements**

### **FR-PLAYBOOK-001-01: Playbook Data Model**

**Requirement**: Workflow Catalog Service SHALL store playbooks with comprehensive metadata.

**Data Model**:
```go
package playbookcatalog

type RemediationPlaybook struct {
    // Identity
    PlaybookID   string   `json:"playbook_id" db:"playbook_id"`        // e.g., "pod-oom-recovery"
    Version      string   `json:"version" db:"version"`                // e.g., "v1.2"

    // Metadata
    Description  string   `json:"description" db:"description"`
    Author       string   `json:"author" db:"author"`                  // user@company.com
    CreatedAt    time.Time `json:"created_at" db:"created_at"`
    UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`

    // Classification
    IncidentTypes []string  `json:"incident_types" db:"incident_types"` // ["pod-oom-killer", ...]
    Tags         []string  `json:"tags" db:"tags"`                     // ["memory", "kubernetes", "pod"]

    // Status
    Status       PlaybookStatus `json:"status" db:"status"`            // draft, active, deprecated
    StatusReason string   `json:"status_reason,omitempty" db:"status_reason"`
    DeprecatedAt *time.Time `json:"deprecated_at,omitempty" db:"deprecated_at"`
    DeprecatedBy string   `json:"deprecated_by,omitempty" db:"deprecated_by"`

    // Playbook Definition
    Steps        []PlaybookStep `json:"steps" db:"steps"`              // JSON array of steps
}

type PlaybookStatus string

const (
    PlaybookStatusDraft      PlaybookStatus = "draft"      // Under development
    PlaybookStatusActive     PlaybookStatus = "active"     // Available for AI selection
    PlaybookStatusDeprecated PlaybookStatus = "deprecated" // Superseded, exclude from AI
)

type PlaybookStep struct {
    StepNumber  int               `json:"step_number"`
    Action      string            `json:"action"`           // "increase_memory", "restart_pod"
    Parameters  map[string]interface{} `json:"parameters"`
    Description string            `json:"description"`
}
```

**Acceptance Criteria**:
- ✅ Playbook ID + Version combination is unique (primary key)
- ✅ `incident_types` array supports multiple incident type mappings
- ✅ `status` field restricted to 3 values (draft, active, deprecated)
- ✅ `steps` array preserves execution order
- ✅ Deprecation audit trail (deprecated_at, deprecated_by, status_reason)

---

### **FR-PLAYBOOK-001-02: Create Workflow API**

**Requirement**: Workflow Catalog Service SHALL provide REST API to register new playbooks.

**Endpoint Specification**:
```http
POST /api/v1/playbooks

Request Body:
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.3",
  "description": "Improved memory allocation with 20% headroom",
  "author": "sre-team@company.com",
  "incident_types": ["pod-oom-killer", "container-memory-pressure"],
  "tags": ["memory", "kubernetes", "pod"],
  "status": "draft",  // New playbooks start as draft
  "steps": [
    {
      "step_number": 1,
      "action": "increase_memory",
      "parameters": {"headroom": 1.2},
      "description": "Increase memory limits by 20%"
    },
    {
      "step_number": 2,
      "action": "restart_pod",
      "parameters": {},
      "description": "Restart pod to apply new limits"
    }
  ]
}

Response (201 Created):
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.3",
  "status": "draft",
  "created_at": "2025-11-05T10:00:00Z",
  "message": "Playbook registered successfully"
}
```

**Acceptance Criteria**:
- ✅ Returns 201 Created for successful registration
- ✅ Returns 409 Conflict if playbook_id + version already exists
- ✅ Returns 400 Bad Request for invalid data (missing required fields, invalid status)
- ✅ Validates `steps` array has at least 1 step
- ✅ Auto-populates `created_at` and `updated_at` timestamps

---

### **FR-PLAYBOOK-001-03: Query Playbooks by Incident Type**

**Requirement**: Workflow Catalog Service SHALL provide API to query playbooks by incident type and status.

**Endpoint Specification**:
```http
GET /api/v1/playbooks?incident_type={type}&status={status}

Query Parameters:
- incident_type (string, optional): Filter by incident type (e.g., "pod-oom-killer")
- status (string, optional, default: "active"): Filter by status (draft, active, deprecated)
- tags (string, optional): Comma-separated tags (e.g., "memory,kubernetes")

Response (200 OK):
[
  {
    "playbook_id": "pod-oom-recovery",
    "version": "v1.2",
    "description": "Increases memory limits and restarts pod",
    "incident_types": ["pod-oom-killer", "container-memory-pressure"],
    "tags": ["memory", "kubernetes", "pod"],
    "status": "active",
    "created_at": "2025-10-01T10:00:00Z"
  },
  {
    "playbook_id": "pod-oom-vertical-scaling",
    "version": "v1.0",
    "description": "Triggers VPA recommendation",
    "incident_types": ["pod-oom-killer"],
    "tags": ["memory", "vpa"],
    "status": "active",
    "created_at": "2025-10-15T14:00:00Z"
  }
]
```

**Acceptance Criteria**:
- ✅ Returns empty array if no playbooks match criteria
- ✅ Filters by incident_type (case-insensitive)
- ✅ Filters by status (defaults to "active" only)
- ✅ Supports multiple tags filtering (AND logic)
- ✅ Response time <100ms for typical queries (<1000 playbooks)

---

### **FR-PLAYBOOK-001-04: List Playbook Versions**

**Requirement**: Workflow Catalog Service SHALL provide API to list all versions of a playbook.

**Endpoint Specification**:
```http
GET /api/v1/playbooks/{playbook_id}/versions

Response (200 OK):
[
  {
    "playbook_id": "pod-oom-recovery",
    "version": "v1.2",
    "status": "active",
    "created_at": "2025-10-01T10:00:00Z",
    "success_rate": 0.89  // from Data Storage (optional integration)
  },
  {
    "playbook_id": "pod-oom-recovery",
    "version": "v1.1",
    "status": "deprecated",
    "deprecated_at": "2025-10-15T10:00:00Z",
    "deprecated_by": "sre-lead@company.com",
    "status_reason": "Low success rate, superseded by v1.2",
    "created_at": "2025-09-01T10:00:00Z",
    "success_rate": 0.40
  }
]
```

**Acceptance Criteria**:
- ✅ Returns all versions (active + deprecated) for comparison
- ✅ Ordered by `created_at` DESC (newest first)
- ✅ Includes deprecation metadata if status=deprecated
- ✅ Optionally includes success_rate from Data Storage API (if available)

---

### **FR-PLAYBOOK-001-05: Update Playbook Status**

**Requirement**: Workflow Catalog Service SHALL provide API to update playbook status (activate, deprecate).

**Endpoint Specification**:
```http
PATCH /api/v1/playbooks/{playbook_id}/versions/{version}

Request Body (Activate):
{
  "status": "active"
}

Request Body (Deprecate):
{
  "status": "deprecated",
  "status_reason": "Superseded by v1.3 with improved algorithm"
}

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.1",
  "status": "deprecated",
  "deprecated_at": "2025-11-05T10:00:00Z",
  "deprecated_by": "user@company.com",
  "status_reason": "Superseded by v1.3 with improved algorithm"
}
```

**Acceptance Criteria**:
- ✅ Auto-populates `deprecated_at` and `deprecated_by` when status=deprecated
- ✅ Requires `status_reason` when deprecating (mandatory audit trail)
- ✅ Returns 404 Not Found if playbook version doesn't exist
- ✅ Validates status transitions (draft→active, active→deprecated, no reactivation of deprecated)

---

## 📈 **Non-Functional Requirements**

### **NFR-PLAYBOOK-001-01: Performance**

- ✅ Query response time <100ms for 95th percentile
- ✅ Support 1000+ playbooks in registry
- ✅ Index on (playbook_id, version) for fast lookups

### **NFR-PLAYBOOK-001-02: Scalability**

- ✅ Handle 50 concurrent API requests
- ✅ Support 100+ versions per playbook
- ✅ Efficient filtering by incident_type (indexed array column)

### **NFR-PLAYBOOK-001-03: Data Integrity**

- ✅ Unique constraint on (playbook_id, version)
- ✅ Foreign key validation for incident_types (optional)
- ✅ Immutable playbook versions (no updates to steps after creation)

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Remediation Workflow Catalog (defines workflow catalog architecture)
- ✅ PostgreSQL database (for playbook storage)

### **Downstream Impacts**
- ✅ BR-AI-057: AI queries workflow catalog for incident-type-specific playbooks
- ✅ BR-REMEDIATION-016: RemediationExecutor populates playbook_id/version in audits
- ✅ BR-WORKFLOW-002: Playbook versioning and deprecation (extends this BR)

---

## 🚀 **Implementation Phases**

### **Phase 1: Data Model & Schema** (Day 3 - 4 hours)
- Define PostgreSQL schema for `remediation_playbooks` table
- Add indexes on (playbook_id, version), (incident_types), (status)
- Create migration script

### **Phase 2: CRUD APIs** (Day 4-5 - 8 hours)
- Implement POST /api/v1/playbooks
- Implement GET /api/v1/playbooks?incident_type=...
- Implement GET /api/v1/playbooks/{id}/versions
- Implement PATCH /api/v1/playbooks/{id}/versions/{version}

### **Phase 3: Testing** (Day 6 - 6 hours)
- Unit tests: CRUD operations, validation logic
- Integration tests: Full API with real PostgreSQL
- Test edge cases: duplicate versions, invalid status transitions

### **Phase 4: OpenAPI Spec** (Day 7 - 2 hours)
- Document all endpoints in OpenAPI 3.0 spec
- Add request/response examples

**Total Estimated Effort**: 20 hours (2.5 days)

---

## 📊 **Success Metrics**

### **Registry Coverage**
- **Target**: 20+ playbooks registered in catalog
- **Measure**: Count of active playbooks

### **API Usage**
- **Target**: 1000+ AI queries per day
- **Measure**: Track endpoint request count

### **Version Adoption**
- **Target**: 90% of playbooks have 2+ versions
- **Measure**: Count versions per playbook

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Store Playbooks in Git Repository**

**Approach**: Use Git as workflow registry (YAML files in repo)

**Rejected Because**:
- ❌ No REST API for runtime queries
- ❌ Slow for AI to query (requires git clone/pull)
- ❌ Harder to implement status lifecycle (active/deprecated)

---

### **Alternative 2: Embed Playbooks in AI Service**

**Approach**: Hardcode playbooks in AI Service configuration

**Rejected Because**:
- ❌ No centralized registry (each service has own copy)
- ❌ Requires redeployment for new playbooks
- ❌ No version management

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P0 priority (foundation for ADR-033 workflow catalog)
**Rationale**: Required for AI catalog-based selection (90% of Hybrid Model)
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Workflow Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-WORKFLOW-002: Playbook versioning and deprecation
- BR-WORKFLOW-003: Playbook metadata API
- BR-AI-057: AI uses workflow catalog for selection
- BR-REMEDIATION-016: Populate playbook_id/version in audits

### **Related Documents**
- [ADR-033: Remediation Workflow Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

