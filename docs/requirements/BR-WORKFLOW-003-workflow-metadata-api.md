# BR-WORKFLOW-003: Workflow Metadata API

> **SUPERSEDED** by [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) and [DD-WORKFLOW-012](../architecture/decisions/DD-WORKFLOW-012-workflow-immutability-constraints.md).
>
> DD-WORKFLOW-012 establishes workflow immutability -- metadata updates after registration are not permitted. DD-WORKFLOW-017 defines the authoritative registration flow where all metadata is extracted from the OCI image's `/workflow-schema.yaml` (per ADR-043). The metadata update API (`PATCH .../metadata`) and tag-based filtering defined in this document conflict with the immutability model. Refer to DD-WORKFLOW-016 for the current discovery and filtering design.

**Business Requirement ID**: BR-WORKFLOW-003
**Category**: Workflow Catalog Service
**Priority**: P1
**Target Version**: V1
**Status**: **SUPERSEDED** by DD-WORKFLOW-017, DD-WORKFLOW-012
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires rich workflow metadata (description, tags, author, incident types) to enable discovery, filtering, and AI selection. The Workflow Catalog Service must provide API to query and update workflow metadata without requiring full playbook re-registration.

**Current Limitations**:
- ❌ No API to update workflow metadata (description, tags) without re-registering
- ❌ Cannot query playbooks by tags or author
- ❌ Limited discoverability (only query by incident_type)
- ❌ No metadata versioning (description changes not tracked)

**Impact**:
- Difficult to discover related playbooks (e.g., all "memory" playbooks)
- Cannot filter by author or team
- Metadata updates require full playbook re-registration
- Limited playbook discoverability

---

## 🎯 **Business Objective**

**Provide comprehensive workflow metadata API for querying, filtering, and updating workflow metadata to enhance playbook discoverability and management.**

### **Success Criteria**
1. ✅ API to query playbooks by tags
2. ✅ API to query playbooks by author
3. ✅ API to update workflow metadata (description, tags)
4. ✅ API to retrieve full playbook details (metadata + definition)
5. ✅ Supports pagination for large workflow catalogs (500+ playbooks)
6. ✅ Metadata updates do not affect playbook definition (separation of concerns)

---

## 🔧 **Functional Requirements**

### **FR-PLAYBOOK-003-01: Query Playbooks by Tags**

**API Specification**:
```http
GET /api/v1/playbooks?tags=memory,kubernetes

Response (200 OK):
[
  {
    "playbook_id": "pod-oom-recovery",
    "version": "v1.2",
    "tags": ["memory", "kubernetes", "pod"],
    "description": "Increases memory limits and restarts pod",
    "status": "active"
  },
  {
    "playbook_id": "node-memory-pressure",
    "version": "v1.0",
    "tags": ["memory", "kubernetes", "node"],
    "description": "Evicts pods to relieve node memory pressure",
    "status": "active"
  }
]
```

**Acceptance Criteria**:
- ✅ Filters by tags (AND logic: all tags must match)
- ✅ Returns empty array if no matches
- ✅ Supports pagination (limit, offset)

---

### **FR-PLAYBOOK-003-02: Update Workflow Metadata**

**API Specification**:
```http
PATCH /api/v1/playbooks/{playbook_id}/versions/{version}/metadata

Request Body:
{
  "description": "Updated description with improved memory allocation algorithm",
  "tags": ["memory", "kubernetes", "pod", "vpa"],
  "author": "updated-author@company.com"
}

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Updated description...",
  "tags": ["memory", "kubernetes", "pod", "vpa"],
  "author": "updated-author@company.com",
  "updated_at": "2025-11-05T10:00:00Z"
}
```

**Acceptance Criteria**:
- ✅ Updates only provided fields (partial update)
- ✅ Does NOT modify playbook definition (steps)
- ✅ Returns 404 Not Found if playbook doesn't exist
- ✅ Updates `updated_at` timestamp

---

### **FR-PLAYBOOK-003-03: Get Full Playbook Details**

**API Specification**:
```http
GET /api/v1/playbooks/{playbook_id}/versions/{version}

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "description": "Increases memory limits and restarts pod",
  "author": "sre-team@company.com",
  "created_at": "2025-10-01T10:00:00Z",
  "updated_at": "2025-11-05T10:00:00Z",
  "incident_types": ["pod-oom-killer", "container-memory-pressure"],
  "tags": ["memory", "kubernetes", "pod"],
  "status": "active",
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
```

**Acceptance Criteria**:
- ✅ Returns full workflow metadata + definition
- ✅ Returns 404 Not Found if playbook doesn't exist
- ✅ Includes all fields (metadata + steps)

---

## 📈 **Non-Functional Requirements**

- ✅ Query response time <100ms
- ✅ Update response time <100ms
- ✅ Support 1000+ playbooks in catalog

---

## 🚀 **Implementation Phases**

### **Phase 1: Query by Tags** (Day 10 - 2 hours)
- Implement tag filtering logic
- Add pagination support
- Unit tests

### **Phase 2: Metadata Update** (Day 10 - 2 hours)
- Implement partial update logic
- Validate metadata fields
- Integration tests

### **Phase 3: Full Details API** (Day 11 - 1 hour)
- Implement GET endpoint
- Return complete playbook structure

**Total Estimated Effort**: 5 hours (0.625 days)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority
**Rationale**: Enhances playbook discoverability and management
**Approved By**: Architecture Team

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

