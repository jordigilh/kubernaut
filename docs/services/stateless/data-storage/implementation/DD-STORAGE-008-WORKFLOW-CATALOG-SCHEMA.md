# DD-STORAGE-008: Workflow Catalog Schema for Data Storage Service

**Date**: November 13, 2025
**Status**: ✅ **APPROVED** (with mitigations for critical validation gaps)
**Decision Maker**: Kubernaut Data Storage Team
**Authority**: ADR-033 (Workflow Catalog), DD-CONTEXT-005 (Minimal LLM Response Schema), DD-WORKFLOW-003 (Parameterized Actions)
**Affects**: Data Storage Service V1.0 MVP, HolmesGPT API integration
**Version**: 1.2 (added parameters field for DD-WORKFLOW-003)

---

## 🎯 **Context**

**Problem**: Data Storage Service needs a schema for storing workflow catalog metadata to support:
1. **Semantic search** (DD-CONTEXT-005 "Filter Before LLM" pattern)
2. **Label-based filtering** (environment, priority, risk_tolerance, business_category)
3. **Version management** (multiple versions of same playbook)
4. **Lifecycle management** (enable/disable playbooks without losing historical data)
5. **Historical tracking** (maintain all versions for audit and rollback)

**User Requirement**: "We should have a way to disable playbooks and keep historical versions"

**Authoritative Sources**:
- **ADR-033**: Remediation Workflow Catalog (defines playbook structure)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (defines query/response requirements)

---

## ✅ **Decision**

**APPROVED**: Implement `workflow_catalog` table with composite primary key (playbook_id, version), lifecycle status management, and **semantic version validation**.

**Confidence**: 98% (increased from 95% after triage and mitigation approval)

**Critical Mitigations Approved**:
1. ✅ **Semantic version validation** using `golang.org/x/mod/semver` (see "Version Validation & Traceability" section)
2. ✅ **Version increment enforcement** (new version must be > current latest)
3. ✅ **Immutability enforcement** with explicit validation and clear error messages
4. ✅ **Version history API** for traceability (get specific version, diff versions)
5. ✅ **Change metadata** (version_notes, change_summary, approved_by)

---

## 📊 **Schema Design**

### **Table: workflow_catalog**

```sql
CREATE TABLE workflow_catalog (
    -- Identity (Composite Primary Key)
    playbook_id VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL,           -- MUST be semantic version (e.g., v1.0.0, v1.2.3)

    -- Metadata
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    owner VARCHAR(255),                      -- Team or user responsible
    maintainer VARCHAR(255),                 -- Contact email

    -- Content
    content TEXT NOT NULL,                   -- Full playbook YAML/JSON (Tekton Task)
    content_hash VARCHAR(64) NOT NULL,       -- SHA-256 hash for integrity

    -- Labels (JSONB for flexible filtering)
    labels JSONB NOT NULL,                   -- DD-CONTEXT-005 label matching

    -- Parameters (JSONB for Tekton PipelineRun parameter schema)
    parameters JSONB,                        -- DD-WORKFLOW-003 parameter schema (v1.0: manual SQL insert, v1.1: extracted from container)

    -- Semantic Search
    embedding vector(384),                   -- sentence-transformers/all-MiniLM-L6-v2

    -- Lifecycle Management (User Requirement: disable + keep history)
    status VARCHAR(20) NOT NULL DEFAULT 'active',  -- 'active', 'disabled', 'deprecated', 'archived'
    disabled_at TIMESTAMP WITH TIME ZONE,
    disabled_by VARCHAR(255),
    disabled_reason TEXT,

    -- Version Management (User Requirement: traceability + immutability)
    is_latest_version BOOLEAN NOT NULL DEFAULT false,
    previous_version VARCHAR(50),            -- Link to previous version
    deprecation_notice TEXT,                 -- Reason for deprecation

    -- Version Change Metadata (TRIAGE MITIGATION: approved)
    version_notes TEXT,                      -- Release notes / changelog
    change_summary TEXT,                     -- Auto-generated summary of changes
    approved_by VARCHAR(255),                -- Who approved this version
    approved_at TIMESTAMP WITH TIME ZONE,    -- When was this version approved

    -- Success Metrics (from ADR-033)
    expected_success_rate DECIMAL(4,3),      -- Expected success rate (0.000-1.000)
    expected_duration_seconds INTEGER,       -- Expected execution time
    actual_success_rate DECIMAL(4,3),        -- Calculated from execution history
    total_executions INTEGER DEFAULT 0,      -- Number of times executed
    successful_executions INTEGER DEFAULT 0, -- Number of successful executions

    -- Audit Trail
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    updated_by VARCHAR(255),

    -- Constraints
    PRIMARY KEY (playbook_id, version),      -- IMMUTABILITY: Cannot overwrite existing version
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived')),
    CHECK (expected_success_rate IS NULL OR (expected_success_rate >= 0 AND expected_success_rate <= 1)),
    CHECK (actual_success_rate IS NULL OR (actual_success_rate >= 0 AND actual_success_rate <= 1)),
    CHECK (total_executions >= 0),
    CHECK (successful_executions >= 0 AND successful_executions <= total_executions)
);

-- Indexes for Query Performance
CREATE INDEX idx_workflow_catalog_status ON workflow_catalog(status);
CREATE INDEX idx_workflow_catalog_latest ON workflow_catalog(playbook_id, is_latest_version) WHERE is_latest_version = true;
CREATE INDEX idx_workflow_catalog_labels ON workflow_catalog USING GIN (labels);
CREATE INDEX idx_workflow_catalog_embedding ON workflow_catalog USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
CREATE INDEX idx_workflow_catalog_created_at ON workflow_catalog(created_at DESC);
CREATE INDEX idx_workflow_catalog_success_rate ON workflow_catalog(actual_success_rate DESC) WHERE status = 'active';

-- Trigger for updated_at
CREATE OR REPLACE FUNCTION update_workflow_catalog_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workflow_catalog_updated_at
    BEFORE UPDATE ON workflow_catalog
    FOR EACH ROW
    EXECUTE FUNCTION update_workflow_catalog_updated_at();
```

---

## 🔑 **Key Design Decisions**

### **Decision 1: Composite Primary Key (playbook_id, version)**

**Rationale**: Supports multiple versions of same playbook while maintaining historical data.

**Benefits**:
- ✅ Multiple versions coexist (e.g., `pod-oom-recovery` v1.0, v1.1, v1.2)
- ✅ Historical versions preserved (never deleted)
- ✅ Version-specific queries (get specific version)
- ✅ Latest version tracking (via `is_latest_version` flag)

**Example**:
```sql
-- All versions of pod-oom-recovery
SELECT * FROM workflow_catalog WHERE playbook_id = 'pod-oom-recovery';

-- Latest version only
SELECT * FROM workflow_catalog WHERE playbook_id = 'pod-oom-recovery' AND is_latest_version = true;

-- Specific version
SELECT * FROM workflow_catalog WHERE playbook_id = 'pod-oom-recovery' AND version = 'v1.2';
```

**Confidence**: 98% (industry standard pattern)

**Triage Mitigation**: Composite PK prevents overwrites at database level, but application-level validation provides clear error messages (see DD-STORAGE-008-VERSION-TRACEABILITY-TRIAGE Fix 2).

---

### **Decision 2: Lifecycle Status Management**

**Rationale**: Support disabling playbooks without losing historical data (user requirement).

**Status Values**:
- `active`: Playbook is available for AI selection and execution
- `disabled`: Playbook is temporarily disabled (can be re-enabled)
- `deprecated`: Playbook is marked for removal (use newer version)
- `archived`: Playbook is permanently retired (historical reference only)

**Status Transitions**:
```
active → disabled → active (temporary disable/enable)
active → deprecated → archived (permanent retirement)
active → archived (immediate retirement)
```

**Metadata Captured on Disable**:
- `disabled_at`: When was it disabled
- `disabled_by`: Who disabled it (user/operator)
- `disabled_reason`: Why was it disabled (free text)

**Query Behavior**:
```sql
-- Active playbooks only (default for AI selection)
SELECT * FROM workflow_catalog WHERE status = 'active' AND is_latest_version = true;

-- Include disabled playbooks (for admin UI)
SELECT * FROM workflow_catalog WHERE status IN ('active', 'disabled');

-- Historical analysis (include all statuses)
SELECT * FROM workflow_catalog;
```

**Benefits**:
- ✅ Disable playbooks without data loss
- ✅ Audit trail of who/when/why disabled
- ✅ Re-enable if needed (e.g., false alarm)
- ✅ Historical data preserved for analysis

**Confidence**: 95% (meets user requirement)

---

### **Decision 3: Labels as JSONB (DD-CONTEXT-005 Requirement)**

**Rationale**: Support flexible label-based filtering for "Filter Before LLM" pattern.

**Required Labels** (from DD-CONTEXT-005):
```json
{
  "kubernaut.io/environment": "production",
  "kubernaut.io/priority": "P0",
  "kubernaut.io/risk-tolerance": "low",
  "kubernaut.io/business-category": "payment-service",
  "kubernaut.io/incident-type": "pod-oom-killer"
}
```

**Query Pattern**:
```sql
-- Filter by environment
SELECT * FROM workflow_catalog
WHERE status = 'active'
  AND labels->>'kubernaut.io/environment' = 'production';

-- Filter by multiple labels
SELECT * FROM workflow_catalog
WHERE status = 'active'
  AND labels->>'kubernaut.io/environment' = 'production'
  AND labels->>'kubernaut.io/priority' = 'P0'
  AND labels->>'kubernaut.io/risk-tolerance' = 'low';

-- GIN index supports JSONB queries efficiently
```

**Benefits**:
- ✅ Flexible label schema (add new labels without migration)
- ✅ Efficient querying (GIN index)
- ✅ DD-CONTEXT-005 compliance (label matching)

**Confidence**: 100% (authoritative requirement)

---

### **Decision 4: Embedding for Semantic Search**

**Rationale**: Support semantic search for incident description → playbook matching.

**Embedding Strategy**:
- **Model**: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
- **Source**: Playbook `description` + `name` (concatenated)
- **Generation**: Python embedding service (external)
- **Storage**: pgvector column with HNSW index

**Query Pattern**:
```sql
-- Semantic search (cosine similarity)
SELECT
    playbook_id,
    version,
    description,
    1 - (embedding <=> $1) AS similarity
FROM workflow_catalog
WHERE status = 'active'
  AND is_latest_version = true
  AND embedding IS NOT NULL
ORDER BY embedding <=> $1
LIMIT 10;
```

**Confidence**: 95% (DD-CONTEXT-005 requirement)

---

### **Decision 5: Success Metrics Tracking**

**Rationale**: Support AI selection based on historical success rates (ADR-033).

**Metrics Stored**:
- `expected_success_rate`: Baseline expectation (set during registration)
- `expected_duration_seconds`: Baseline execution time
- `actual_success_rate`: Calculated from execution history
- `total_executions`: Total number of times executed
- `successful_executions`: Number of successful executions

**Calculation**:
```sql
-- Update success rate after execution
UPDATE workflow_catalog
SET
    total_executions = total_executions + 1,
    successful_executions = successful_executions + CASE WHEN $success THEN 1 ELSE 0 END,
    actual_success_rate = (successful_executions + CASE WHEN $success THEN 1 ELSE 0 END)::DECIMAL / (total_executions + 1)
WHERE playbook_id = $playbook_id AND version = $version;
```

**AI Selection Query**:
```sql
-- Prefer playbooks with high success rate and statistical significance
SELECT * FROM workflow_catalog
WHERE status = 'active'
  AND is_latest_version = true
  AND total_executions >= 10  -- Statistical significance threshold
ORDER BY actual_success_rate DESC, total_executions DESC
LIMIT 10;
```

**Confidence**: 90% (ADR-033 requirement)

---

## 📋 **Go Model Definition**

```go
// pkg/datastorage/models/playbook.go
package models

import (
    "time"
    "encoding/json"
)

// Playbook represents a remediation workflow in the catalog
type Playbook struct {
    // Identity
    PlaybookID string `json:"playbook_id" db:"playbook_id"`
    Version    string `json:"version" db:"version"`

    // Metadata
    Name        string  `json:"name" db:"name"`
    Description string  `json:"description" db:"description"`
    Owner       *string `json:"owner,omitempty" db:"owner"`
    Maintainer  *string `json:"maintainer,omitempty" db:"maintainer"`

    // Content
    Content     string `json:"content" db:"content"`
    ContentHash string `json:"content_hash" db:"content_hash"`

    // Labels (JSONB)
    Labels map[string]string `json:"labels" db:"labels"`

    // Semantic Search
    Embedding []float32 `json:"embedding,omitempty" db:"embedding"`

    // Lifecycle Management
    Status         PlaybookStatus `json:"status" db:"status"`
    DisabledAt     *time.Time     `json:"disabled_at,omitempty" db:"disabled_at"`
    DisabledBy     *string        `json:"disabled_by,omitempty" db:"disabled_by"`
    DisabledReason *string        `json:"disabled_reason,omitempty" db:"disabled_reason"`

    // Version Management
    IsLatestVersion   bool    `json:"is_latest_version" db:"is_latest_version"`
    PreviousVersion   *string `json:"previous_version,omitempty" db:"previous_version"`
    DeprecationNotice *string `json:"deprecation_notice,omitempty" db:"deprecation_notice"`

    // Success Metrics
    ExpectedSuccessRate   *float64 `json:"expected_success_rate,omitempty" db:"expected_success_rate"`
    ExpectedDurationSecs  *int     `json:"expected_duration_seconds,omitempty" db:"expected_duration_seconds"`
    ActualSuccessRate     *float64 `json:"actual_success_rate,omitempty" db:"actual_success_rate"`
    TotalExecutions       int      `json:"total_executions" db:"total_executions"`
    SuccessfulExecutions  int      `json:"successful_executions" db:"successful_executions"`

    // Audit Trail
    CreatedAt time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
    CreatedBy *string    `json:"created_by,omitempty" db:"created_by"`
    UpdatedBy *string    `json:"updated_by,omitempty" db:"updated_by"`
}

// PlaybookStatus represents the lifecycle status of a playbook
type PlaybookStatus string

const (
    PlaybookStatusActive     PlaybookStatus = "active"
    PlaybookStatusDisabled   PlaybookStatus = "disabled"
    PlaybookStatusDeprecated PlaybookStatus = "deprecated"
    PlaybookStatusArchived   PlaybookStatus = "archived"
)

// PlaybookFilters for querying playbooks
type PlaybookFilters struct {
    PlaybookID      *string            // Filter by playbook ID
    Status          []PlaybookStatus   // Filter by status (default: active only)
    LatestOnly      bool               // Only return latest versions
    Labels          map[string]string  // Label matching (DD-CONTEXT-005)
    MinSuccessRate  *float64           // Minimum actual success rate
    MinExecutions   *int               // Minimum execution count (statistical significance)
}

// PlaybookSearchParams for semantic search
type PlaybookSearchParams struct {
    Query           string             // Incident description for semantic search
    Labels          map[string]string  // Label filters (DD-CONTEXT-005)
    MinConfidence   float64            // Minimum similarity threshold (0.0-1.0)
    MaxResults      int                // Limit number of results
    IncludeDisabled bool               // Include disabled playbooks (default: false)
}

// PlaybookSearchResult represents a semantic search result
type PlaybookSearchResult struct {
    Playbook   *Playbook `json:"playbook"`
    Confidence float64   `json:"confidence"` // Semantic similarity score (0.0-1.0)
}
```

---

## 🔧 **REST API Endpoints**

### **POST /api/v1/playbooks** - Create/Update Playbook

**Request**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "name": "Pod OOM Recovery",
  "description": "Increases memory limits and restarts pod on OOM",
  "owner": "sre-team",
  "maintainer": "sre@company.com",
  "content": "<Tekton Task YAML>",
  "labels": {
    "kubernaut.io/environment": "production",
    "kubernaut.io/priority": "P0",
    "kubernaut.io/risk-tolerance": "low",
    "kubernaut.io/incident-type": "pod-oom-killer"
  },
  "expected_success_rate": 0.90,
  "expected_duration_seconds": 180
}
```

**Response**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "status": "active",
  "is_latest_version": true,
  "created_at": "2025-11-13T10:00:00Z"
}
```

---

### **GET /api/v1/playbooks/search** - Semantic Search

**Request**:
```
GET /api/v1/playbooks/search?query=pod+keeps+crashing&label.environment=production&label.priority=P0&min_confidence=0.7&max_results=10
```

**Response** (DD-CONTEXT-005 format):
```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod on OOM",
      "confidence": 0.92
    },
    {
      "playbook_id": "pod-crash-loop-recovery",
      "version": "v2.0",
      "description": "Diagnoses and fixes pod crash loops",
      "confidence": 0.85
    }
  ],
  "total_results": 2
}
```

---

### **PATCH /api/v1/playbooks/{playbook_id}/{version}/disable** - Disable Playbook

**Request**:
```json
{
  "disabled_by": "operator@company.com",
  "disabled_reason": "High failure rate in production (60% failures in last 24h)"
}
```

**Response**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "status": "disabled",
  "disabled_at": "2025-11-13T15:30:00Z",
  "disabled_by": "operator@company.com",
  "disabled_reason": "High failure rate in production (60% failures in last 24h)"
}
```

---

### **PATCH /api/v1/playbooks/{playbook_id}/{version}/enable** - Re-enable Playbook

**Request**:
```json
{
  "enabled_by": "operator@company.com"
}
```

**Response**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.2",
  "status": "active",
  "disabled_at": null,
  "disabled_by": null,
  "disabled_reason": null
}
```

---

### **GET /api/v1/playbooks/{playbook_id}/versions** - List All Versions

**Request**:
```
GET /api/v1/playbooks/pod-oom-recovery/versions?include_disabled=true
```

**Response**:
```json
{
  "playbook_id": "pod-oom-recovery",
  "versions": [
    {
      "version": "v1.2",
      "status": "active",
      "is_latest_version": true,
      "actual_success_rate": 0.92,
      "total_executions": 150,
      "created_at": "2025-10-01T10:00:00Z"
    },
    {
      "version": "v1.1",
      "status": "deprecated",
      "is_latest_version": false,
      "actual_success_rate": 0.85,
      "total_executions": 200,
      "deprecation_notice": "Replaced by v1.2 with improved memory analysis",
      "created_at": "2025-08-15T10:00:00Z"
    },
    {
      "version": "v1.0",
      "status": "archived",
      "is_latest_version": false,
      "actual_success_rate": 0.78,
      "total_executions": 100,
      "created_at": "2025-06-01T10:00:00Z"
    }
  ],
  "total_versions": 3
}
```

---

## 📊 **Lifecycle Management Workflows**

### **Workflow 1: Create New Version**

```sql
-- Step 1: Create new version
INSERT INTO workflow_catalog (
    playbook_id, version, name, description, content, labels,
    status, is_latest_version, previous_version
) VALUES (
    'pod-oom-recovery', 'v1.2', 'Pod OOM Recovery',
    'Increases memory limits and restarts pod', '<content>',
    '{"kubernaut.io/environment": "production"}',
    'active', true, 'v1.1'
);

-- Step 2: Mark previous version as not latest
UPDATE workflow_catalog
SET is_latest_version = false
WHERE playbook_id = 'pod-oom-recovery' AND version = 'v1.1';
```

---

### **Workflow 2: Disable Playbook (Temporary)**

```sql
UPDATE workflow_catalog
SET
    status = 'disabled',
    disabled_at = NOW(),
    disabled_by = 'operator@company.com',
    disabled_reason = 'High failure rate in production'
WHERE playbook_id = 'pod-oom-recovery' AND version = 'v1.2';
```

---

### **Workflow 3: Re-enable Playbook**

```sql
UPDATE workflow_catalog
SET
    status = 'active',
    disabled_at = NULL,
    disabled_by = NULL,
    disabled_reason = NULL
WHERE playbook_id = 'pod-oom-recovery' AND version = 'v1.2';
```

---

### **Workflow 4: Deprecate Playbook (Permanent)**

```sql
-- Step 1: Mark old version as deprecated
UPDATE workflow_catalog
SET
    status = 'deprecated',
    deprecation_notice = 'Replaced by v1.2 with improved memory analysis',
    is_latest_version = false
WHERE playbook_id = 'pod-oom-recovery' AND version = 'v1.1';

-- Step 2: Create new version (see Workflow 1)
```

---

### **Workflow 5: Archive Playbook (Permanent Retirement)**

```sql
UPDATE workflow_catalog
SET
    status = 'archived',
    is_latest_version = false
WHERE playbook_id = 'pod-oom-recovery' AND version = 'v1.0';
```

---

## 🎯 **Query Patterns for AI Selection**

### **Pattern 1: Active Playbooks Only (Default)**

```sql
SELECT * FROM workflow_catalog
WHERE status = 'active' AND is_latest_version = true;
```

---

### **Pattern 2: Semantic Search with Label Filtering (DD-CONTEXT-005)**

```sql
SELECT
    playbook_id,
    version,
    description,
    1 - (embedding <=> $query_embedding) AS confidence
FROM workflow_catalog
WHERE status = 'active'
  AND is_latest_version = true
  AND labels->>'kubernaut.io/environment' = 'production'
  AND labels->>'kubernaut.io/priority' = 'P0'
  AND labels->>'kubernaut.io/risk-tolerance' = 'low'
  AND 1 - (embedding <=> $query_embedding) >= 0.7
ORDER BY embedding <=> $query_embedding
LIMIT 10;
```

---

### **Pattern 3: Success Rate Filtering**

```sql
SELECT * FROM workflow_catalog
WHERE status = 'active'
  AND is_latest_version = true
  AND total_executions >= 10  -- Statistical significance
  AND actual_success_rate >= 0.80
ORDER BY actual_success_rate DESC, total_executions DESC;
```

---

## ✅ **Benefits**

### **User Requirement: Disable Playbooks**
- ✅ **Temporary disable**: Set status='disabled' (can re-enable)
- ✅ **Permanent retirement**: Set status='deprecated' or 'archived'
- ✅ **Audit trail**: Capture who/when/why disabled
- ✅ **Historical data preserved**: Never delete playbook records

### **Version Management**
- ✅ **Multiple versions coexist**: Composite primary key (playbook_id, version)
- ✅ **Latest version tracking**: `is_latest_version` flag
- ✅ **Version history**: `previous_version` link
- ✅ **Historical analysis**: All versions queryable

### **DD-CONTEXT-005 Compliance**
- ✅ **Label-based filtering**: JSONB labels with GIN index
- ✅ **Semantic search**: pgvector embedding with HNSW index
- ✅ **Minimal response**: 4 fields (playbook_id, version, description, confidence)

### **ADR-033 Compliance**
- ✅ **Success rate tracking**: actual_success_rate calculated from executions
- ✅ **Statistical significance**: total_executions for confidence
- ✅ **AI selection**: Query by success rate and execution count

---

## 📊 **Confidence Assessment**

**Overall Confidence**: **95%**

**Breakdown**:
- **Schema design**: 98% (industry standard composite key + lifecycle management)
- **Lifecycle management**: 95% (meets user requirement for disable + historical data)
- **DD-CONTEXT-005 compliance**: 100% (authoritative requirement)
- **ADR-033 compliance**: 90% (success metrics tracking)
- **Version management**: 98% (standard pattern)

**Why 95% (not 100%)**:
- 5% uncertainty: Potential need for additional metadata fields not yet discovered
  - **Mitigation**: JSONB labels allow adding new metadata without schema changes

---

## 🔗 **Related Decisions**

- **ADR-033**: Remediation Workflow Catalog (defines playbook structure)
- **DD-CONTEXT-005**: Minimal LLM Response Schema (defines query/response requirements)
- **ADR-034**: Unified Audit Table Design (similar lifecycle management pattern)
- **DD-STORAGE-006**: V1.0 No-Cache Decision (no playbook embedding caching in V1.0)
- **DD-STORAGE-007**: Redis Requirement Reassessment (Redis mandatory for DLQ)
- **DD-011**: PostgreSQL Version Requirements (semver validation pattern)

---

## 📋 **Implementation Roadmap**

### **🎯 V1.0 MVP Scope** (Foundation - SQL-Only Management)

**Goal**: Establish workflow catalog foundation with SQL-only management (no REST API for writes, no CRD controller)

**Duration**: 3 days (20 hours)

**V1.0 Deliverables**:
- ✅ `workflow_catalog` table with full schema (including version validation fields for future use)
- ✅ Go models and repository (READ-ONLY operations)
- ✅ Semantic search endpoint: `GET /api/v1/playbooks/search`
- ✅ Version listing endpoint: `GET /api/v1/playbooks/{id}/versions`
- ✅ Embedding generation pipeline (NO CACHING per DD-STORAGE-006)
- ✅ Integration tests for search and version listing

**V1.0 Limitations** (Deferred to V1.1):
- ❌ **NO** playbook creation/update REST API (SQL-only management)
- ❌ **NO** version validation enforcement (manual SQL management)
- ❌ **NO** lifecycle management API (disable/enable via SQL only)
- ❌ **NO** embedding caching (per DD-STORAGE-006)
- ❌ **NO** version diff API

---

#### **V1.0 Phase 1: Schema & Models** (Day 1, 6 hours)

**Scope**: Database schema and Go models (read-only operations)

- [ ] Create migration: `migrations/013_workflow_catalog.sql`
  - [ ] Full schema with all fields (including version validation metadata for V1.1)
  - [ ] Indexes for search, labels, embeddings (HNSW)
  - [ ] Triggers for updated_at
  - [ ] Comments documenting V1.0 vs V1.1 fields
- [ ] Create Go model: `pkg/datastorage/models/playbook.go`
  - [ ] Playbook struct with all fields
  - [ ] PlaybookStatus enum (active/disabled/deprecated/archived)
  - [ ] PlaybookSearchParams struct
  - [ ] PlaybookSearchResult struct
- [ ] Create repository: `pkg/datastorage/repository/playbook_repository.go`
  - [ ] `GetLatestVersion()` - Read latest version
  - [ ] `GetVersion()` - Read specific version
  - [ ] `ListVersions()` - List all versions
  - [ ] `SearchPlaybooks()` - Semantic search with label filtering
  - [ ] **NOTE**: NO create/update methods in V1.0 (SQL-only management)

**Deliverable**: Schema and read-only repository

---

#### **V1.0 Phase 2: Semantic Search API** (Day 2, 8 hours)

**Scope**: Read-only REST API for semantic search and version listing

- [ ] Implement `GET /api/v1/playbooks/search` (semantic search)
  - [ ] Query parameter: `query` (incident description)
  - [ ] Query parameters: `label.*` (label filtering per DD-CONTEXT-005)
  - [ ] Query parameter: `min_confidence` (similarity threshold, default: 0.7)
  - [ ] Query parameter: `max_results` (limit, default: 10)
  - [ ] Response: DD-CONTEXT-005 format (playbook_id, version, description, confidence)
  - [ ] Filter: status='active' AND is_latest_version=true (default)
- [ ] Implement `GET /api/v1/playbooks/{id}/versions` (list all versions)
  - [ ] Query parameter: `include_disabled` (default: false)
  - [ ] Response: Version list with metadata (status, success_rate, created_at)
- [ ] **NOTE**: NO create/update/disable/enable endpoints in V1.0

**Deliverable**: Read-only REST API for search and version listing

---

#### **V1.0 Phase 3: Embedding Generation** (Day 3, 6 hours)

**Scope**: Real-time embedding generation (NO CACHING per DD-STORAGE-006)

- [ ] Python embedding service (sentence-transformers)
  - [ ] HTTP server with `/embed` endpoint
  - [ ] Model: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
  - [ ] Input: Playbook description + name (concatenated)
  - [ ] Output: 384-dimensional vector
- [ ] Go HTTP client: `pkg/datastorage/embedding/client.go`
  - [ ] `GenerateEmbedding(text string) ([]float32, error)`
  - [ ] **NO CACHING** (per DD-STORAGE-006)
  - [ ] Timeout: 5 seconds per request
- [ ] Playbook embedding pipeline: `pkg/datastorage/embedding/playbook_pipeline.go`
  - [ ] Generate embeddings on-the-fly during search
  - [ ] **NO CACHING** (per DD-STORAGE-006)
  - [ ] Acceptable latency: 2.5s for 50 playbooks (per DD-STORAGE-006)
- [ ] Integration tests
  - [ ] Test semantic search with real embeddings
  - [ ] Test label filtering
  - [ ] Test version listing

**Deliverable**: Real-time embedding generation (no caching)

---

### **🚀 V1.1 Enhancements** (REST API + CRD Controller + Caching)

**Goal**: Add playbook management REST API with version validation and embedding caching

**Duration**: 4 days (28 hours)

**V1.1 Deliverables**:
- ✅ Playbook creation/update REST API with version validation
- ✅ Lifecycle management API (disable/enable)
- ✅ Version diff API
- ✅ CRD controller for playbook management
- ✅ Embedding caching with CRD-triggered invalidation
- ✅ Comprehensive integration tests

---

#### **V1.1 Phase 1: Version Validation** (Day 1, 8 hours)

**Scope**: Semantic version validation for playbook creation/update

- [ ] Create `pkg/datastorage/playbook/version_validator.go`
  - [ ] `ValidateVersionFormat()` using `golang.org/x/mod/semver`
  - [ ] `CompareVersions()` for version comparison (-1/0/1)
  - [ ] `IsValidIncrement()` to enforce increment requirement
  - [ ] Custom errors: `ErrVersionAlreadyExists`, `ErrVersionNotIncremented`
- [ ] Update `PlaybookRepository` with validation
  - [ ] `CreatePlaybook()` with version validation
  - [ ] `VersionExists()` check
  - [ ] Transaction for atomic latest version update
- [ ] Unit tests for version validator (TDD)
  - [ ] Test valid semver formats (v1.0.0, v1.2.3, v2.0.0-alpha)
  - [ ] Test invalid formats (1.0, vv1.0.0, abc)
  - [ ] Test version increment validation
  - [ ] Test immutability enforcement

**Deliverable**: Version validation library with comprehensive tests

---

#### **V1.1 Phase 2: Playbook Management REST API** (Day 2, 10 hours)

**Scope**: Create/update/disable/enable playbooks with version validation

- [ ] Implement `POST /api/v1/playbooks` (create/update playbook)
  - [ ] **CRITICAL**: Validate semantic version format (semver)
  - [ ] **CRITICAL**: Validate version increment (must be > current latest)
  - [ ] **CRITICAL**: Prevent overwriting existing versions (immutability)
  - [ ] Return clear error messages:
    - [ ] 400: Invalid version format
    - [ ] 400: Version not incremented
    - [ ] 409: Version already exists (immutable)
  - [ ] Invalidate embedding cache on create/update
- [ ] Implement `PATCH /api/v1/playbooks/{id}/{version}/disable`
  - [ ] Capture disabled_by, disabled_reason, disabled_at
  - [ ] Invalidate embedding cache on disable
- [ ] Implement `PATCH /api/v1/playbooks/{id}/{version}/enable`
  - [ ] Clear disabled metadata
  - [ ] Invalidate embedding cache on enable
- [ ] Implement `GET /api/v1/playbooks/{id}/versions/{version}` (get specific version)
- [ ] Implement `GET /api/v1/playbooks/{id}/versions/{v1}/diff/{v2}` (compare versions)

**Deliverable**: Full playbook management REST API with version validation

---

#### **V1.1 Phase 3: Embedding Caching** (Day 3, 6 hours)

**Scope**: Redis-backed embedding cache with CRD-triggered invalidation

- [ ] Implement embedding cache in `pkg/datastorage/embedding/cache.go`
  - [ ] Redis key: `embedding:playbook:{id}:{version}`
  - [ ] TTL: 24 hours (configurable)
  - [ ] Cache hit/miss metrics
- [ ] Update embedding pipeline to use cache
  - [ ] Check cache before generating embedding
  - [ ] Store embedding in cache after generation
  - [ ] Latency improvement: 2.5s → ~50ms (50× faster)
- [ ] Implement cache invalidation
  - [ ] Invalidate on playbook create/update
  - [ ] Invalidate on playbook disable/enable
  - [ ] CRD controller webhook for automatic invalidation

**Deliverable**: Embedding caching with CRD-triggered invalidation

---

#### **V1.1 Phase 4: Integration Tests** (Day 4, 4 hours)

**Scope**: Comprehensive integration tests for V1.1 features

- [ ] Test playbook CRUD operations with version validation
  - [ ] Test version format validation (invalid semver rejected)
  - [ ] Test version increment validation (v0.9 after v1.0 rejected)
  - [ ] Test immutability (duplicate version rejected with 409)
- [ ] Test lifecycle management (disable/enable)
  - [ ] Test disable captures metadata (who/when/why)
  - [ ] Test re-enable clears metadata
- [ ] Test version history API
  - [ ] Test get specific version
  - [ ] Test diff between versions
- [ ] Test embedding cache
  - [ ] Test cache hit/miss
  - [ ] Test cache invalidation on create/update/disable/enable

**Deliverable**: Comprehensive integration tests for V1.1

---

## 📊 **V1.0 vs V1.1 Feature Matrix**

| Feature | V1.0 MVP | V1.1 Enhancements |
|---------|----------|-------------------|
| **Workflow Catalog Table** | ✅ Full schema | ✅ Same schema |
| **Semantic Search API** | ✅ `GET /search` | ✅ Same |
| **Version Listing API** | ✅ `GET /versions` | ✅ Same |
| **Embedding Generation** | ✅ Real-time (no cache) | ✅ Cached (24h TTL) |
| **Playbook Management** | ❌ SQL-only | ✅ REST API (`POST /playbooks`) |
| **Version Validation** | ❌ Manual SQL | ✅ Automated (semver) |
| **Lifecycle Management** | ❌ SQL-only | ✅ REST API (`PATCH /disable`, `/enable`) |
| **Version Diff API** | ❌ Not available | ✅ `GET /diff/{v1}/{v2}` |
| **CRD Controller** | ❌ Not available | ✅ Playbook CRD |
| **Cache Invalidation** | ❌ N/A (no cache) | ✅ CRD-triggered |

---

## 🎯 **V1.0 MVP Success Criteria**

**Must Have** (Blocking):
1. ✅ `workflow_catalog` table exists with full schema
2. ✅ `GET /api/v1/playbooks/search` returns semantically similar playbooks
3. ✅ `GET /api/v1/playbooks/{id}/versions` lists all versions
4. ✅ Embedding generation works (real-time, no cache)
5. ✅ Integration tests pass for search and version listing
6. ✅ Latency acceptable: 2.5s for 50 playbooks (per DD-STORAGE-006)

**Nice to Have** (Non-Blocking):
- ⚠️ Playbook management via SQL (manual process, documented)
- ⚠️ Version validation via manual review (no automation)

---

## 🚀 **V1.1 Enhancement Success Criteria**

**Must Have** (Blocking):
1. ✅ `POST /api/v1/playbooks` validates version format and increment
2. ✅ Version immutability enforced (duplicate version rejected with 409)
3. ✅ `PATCH /disable` and `/enable` work with audit trail
4. ✅ Embedding cache reduces latency from 2.5s to ~50ms (50× improvement)
5. ✅ CRD controller triggers cache invalidation on playbook changes
6. ✅ Integration tests pass for all V1.1 features

---

**Document Version**: 1.2 (updated with V1.0/V1.1 scope separation)
**Last Updated**: November 13, 2025
**Status**: ✅ **APPROVED** (98% confidence with clear V1.0/V1.1 separation)
**Next Review**: After V1.0 MVP implementation

