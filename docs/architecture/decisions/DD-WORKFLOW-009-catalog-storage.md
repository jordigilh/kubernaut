# DD-WORKFLOW-009: Workflow Catalog Storage

**Date**: 2025-11-15
**Status**: Confirmed
**Version**: v1.1
**Related**: DD-WORKFLOW-007, DD-WORKFLOW-008, DD-WORKFLOW-012 (Workflow Immutability)

---

## 🔗 **Workflow Immutability Reference**

**CRITICAL**: This DD defines the PRIMARY KEY that enforces workflow immutability.

**Authority**: **DD-WORKFLOW-012: Workflow Immutability Constraints**
- `PRIMARY KEY (workflow_id, version)` enforces immutability
- Cannot overwrite existing workflow versions
- All schema fields are immutable per DD-WORKFLOW-012

**Cross-Reference**: The composite primary key design is the database-level enforcement mechanism for DD-WORKFLOW-012 immutability constraints.

---

---

## Storage Backend

### Decision: PostgreSQL with pgvector

**Database**: PostgreSQL with pgvector extension
**Managed By**: Workflow Catalog Controller
**Concurrency**: Handled at database level

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Operator Creates RemediationWorkflow CRD                    │
│ kubectl apply -f playbook-oomkill.yaml                      │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ CRD created in cluster
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Workflow Registry Controller (Go Service)                   │
│ - Watches RemediationWorkflow CRDs                          │
│ - Pulls container images                                    │
│ - Extracts /playbook-schema.json                            │
│ - Validates schema                                          │
│ - Calls Data Storage REST API                               │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ HTTP POST /api/v1/playbooks
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service (REST API)                             │
│ - Workflow CRUD endpoints                                   │
│ - Semantic version validation                               │
│ - Database operations                                       │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ SQL queries
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ PostgreSQL + pgvector                                        │
│ - Workflow metadata storage                                 │
│ - Schema storage (JSONB)                                    │
│ - Concurrent access handling                                │
│ - Transaction management                                    │
└─────────────────────────────────────────────────────────────┘
```

---

## Database Schema

### Playbooks Table

```sql
CREATE TABLE playbooks (
    -- Primary identification
    workflow_id VARCHAR(255) PRIMARY KEY,
    version VARCHAR(50) NOT NULL,

    -- Container information
    container_image TEXT NOT NULL,
    container_digest VARCHAR(71) NOT NULL, -- sha256:...

    -- Schema (stored as JSONB for querying)
    parameters JSONB NOT NULL,
    labels JSONB NOT NULL,

    -- Metadata
    title TEXT,
    description TEXT,

    -- Validation status
    validated BOOLEAN NOT NULL DEFAULT false,
    validated_at TIMESTAMP WITH TIME ZONE,

    -- Audit fields
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),

    -- Version constraint: unique workflow_id + version combination
    UNIQUE(workflow_id, version)
);

-- Index for label-based searches
CREATE INDEX idx_playbooks_labels ON playbooks USING GIN (labels);

-- Index for parameter searches
CREATE INDEX idx_playbooks_parameters ON playbooks USING GIN (parameters);

-- Index for validation status
CREATE INDEX idx_playbooks_validated ON playbooks (validated);

-- Index for container digest lookups
CREATE INDEX idx_playbooks_digest ON playbooks (container_digest);
```

### Example Row

```sql
INSERT INTO playbooks (
    workflow_id,
    version,
    container_image,
    container_digest,
    parameters,
    labels,
    title,
    description,
    validated,
    validated_at,
    created_by
) VALUES (
    'oomkill-cost-optimized',
    '1.0.0',
    'quay.io/kubernaut/playbook-oomkill-cost:v1.0.0',
    'sha256:abc123def456...',
    '{
        "parameters": [
            {
                "name": "TARGET_RESOURCE_KIND",
                "type": "string",
                "required": true,
                "enum": ["Deployment", "StatefulSet"]
            },
            {
                "name": "TARGET_RESOURCE_NAME",
                "type": "string",
                "required": true
            }
        ]
    }'::jsonb,
    '{
        "signal_type": "OOMKilled",
        "severity": "high",
        "priority": "P1",
        "business_category": "cost-management"
    }'::jsonb,
    'Cost-Optimized OOMKill Remediation',
    'Remediation for OOMKilled events in cost-sensitive namespaces',
    true,
    NOW(),
    'operator@example.com'
);
```

---

## Concurrency Handling

### Database-Level Concurrency Control

**Mechanism**: PostgreSQL transaction isolation + unique constraints

#### Scenario: Concurrent Registration of Same Playbook

```sql
-- Transaction 1 (Operator A)
BEGIN;
INSERT INTO playbooks (workflow_id, version, ...)
VALUES ('oomkill-cost-optimized', '1.0.0', ...)
ON CONFLICT (workflow_id, version) DO UPDATE
SET
    container_image = EXCLUDED.container_image,
    container_digest = EXCLUDED.container_digest,
    parameters = EXCLUDED.parameters,
    labels = EXCLUDED.labels,
    validated = EXCLUDED.validated,
    validated_at = EXCLUDED.validated_at,
    updated_at = NOW();
COMMIT;

-- Transaction 2 (Operator B) - concurrent
BEGIN;
INSERT INTO playbooks (workflow_id, version, ...)
VALUES ('oomkill-cost-optimized', '1.0.0', ...)
ON CONFLICT (workflow_id, version) DO UPDATE
SET
    container_image = EXCLUDED.container_image,
    container_digest = EXCLUDED.container_digest,
    parameters = EXCLUDED.parameters,
    labels = EXCLUDED.labels,
    validated = EXCLUDED.validated,
    validated_at = EXCLUDED.validated_at,
    updated_at = NOW();
COMMIT;
```

**Result**: PostgreSQL handles serialization, last write wins (deterministic)

#### Scenario: Version Conflict Detection

```sql
-- Check if version already exists before registration
SELECT workflow_id, version, container_digest, updated_at
FROM playbooks
WHERE workflow_id = $1 AND version = $2;

-- If exists and digest different, prompt operator
IF found AND container_digest != $3 THEN
    RAISE NOTICE 'Version % already exists with different digest. Use --force to overwrite.';
END IF;
```

---

## Catalog Controller Implementation

### Registration Endpoint

```go
package controller

import (
    "database/sql"
    "encoding/json"
    "fmt"

    "github.com/lib/pq"
)

type PlaybookCatalogController struct {
    db *sql.DB
}

func (c *PlaybookCatalogController) RegisterPlaybook(req *RegisterRequest) error {
    // 1. Pull image and extract schema (done before DB operation)
    schema, err := c.extractSchema(req.ContainerImage)
    if err != nil {
        return fmt.Errorf("schema extraction failed: %w", err)
    }

    // 2. Validate schema format
    if err := c.validateSchema(schema); err != nil {
        return fmt.Errorf("schema validation failed: %w", err)
    }

    // 3. Get image digest
    digest, err := c.getImageDigest(req.ContainerImage)
    if err != nil {
        return fmt.Errorf("failed to get image digest: %w", err)
    }

    // 4. Insert/update in database (PostgreSQL handles concurrency)
    query := `
        INSERT INTO playbooks (
            workflow_id,
            version,
            container_image,
            container_digest,
            parameters,
            labels,
            title,
            description,
            validated,
            validated_at,
            created_by
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), $10)
        ON CONFLICT (workflow_id, version) DO UPDATE
        SET
            container_image = EXCLUDED.container_image,
            container_digest = EXCLUDED.container_digest,
            parameters = EXCLUDED.parameters,
            labels = EXCLUDED.labels,
            title = EXCLUDED.title,
            description = EXCLUDED.description,
            validated = EXCLUDED.validated,
            validated_at = EXCLUDED.validated_at,
            updated_at = NOW()
        RETURNING workflow_id, version, created_at, updated_at
    `

    parametersJSON, _ := json.Marshal(schema.Parameters)
    labelsJSON, _ := json.Marshal(schema.Labels)

    var result struct {
        PlaybookID string
        Version    string
        CreatedAt  time.Time
        UpdatedAt  time.Time
    }

    err = c.db.QueryRow(
        query,
        schema.PlaybookID,
        schema.Version,
        req.ContainerImage,
        digest,
        parametersJSON,
        labelsJSON,
        schema.Title,
        schema.Description,
        true, // validated
        req.CreatedBy,
    ).Scan(&result.PlaybookID, &result.Version, &result.CreatedAt, &result.UpdatedAt)

    if err != nil {
        return fmt.Errorf("database insert failed: %w", err)
    }

    // Log whether this was insert or update
    if result.CreatedAt.Equal(result.UpdatedAt) {
        log.Info("Playbook registered (new)",
            "workflow_id", result.PlaybookID,
            "version", result.Version)
    } else {
        log.Info("Playbook updated (existing)",
            "workflow_id", result.PlaybookID,
            "version", result.Version)
    }

    return nil
}
```

### Search Endpoint

```go
func (c *PlaybookCatalogController) SearchPlaybooks(criteria *SearchCriteria) ([]*Playbook, error) {
    query := `
        SELECT
            workflow_id,
            version,
            container_image,
            container_digest,
            parameters,
            labels,
            title,
            description,
            validated,
            validated_at
        FROM playbooks
        WHERE validated = true
            AND labels @> $1::jsonb
        ORDER BY updated_at DESC
        LIMIT $2
    `

    // Build label filter
    labelFilter := map[string]interface{}{
        "signal_type": criteria.SignalType,
        "severity":    criteria.Severity,
    }
    labelFilterJSON, _ := json.Marshal(labelFilter)

    rows, err := c.db.Query(query, labelFilterJSON, criteria.Limit)
    if err != nil {
        return nil, fmt.Errorf("search query failed: %w", err)
    }
    defer rows.Close()

    var playbooks []*Playbook
    for rows.Next() {
        var p Playbook
        var parametersJSON, labelsJSON []byte

        err := rows.Scan(
            &p.PlaybookID,
            &p.Version,
            &p.ContainerImage,
            &p.ContainerDigest,
            &parametersJSON,
            &labelsJSON,
            &p.Title,
            &p.Description,
            &p.Validated,
            &p.ValidatedAt,
        )
        if err != nil {
            return nil, err
        }

        json.Unmarshal(parametersJSON, &p.Parameters)
        json.Unmarshal(labelsJSON, &p.Labels)

        playbooks = append(playbooks, &p)
    }

    return playbooks, nil
}
```

---

## Concurrency Scenarios

### Scenario 1: Two Operators Register Same Workflow Simultaneously

**Setup**:
- Operator A: Registers `oomkill-cost-optimized:v1.0.0` at 10:00:00.000
- Operator B: Registers `oomkill-cost-optimized:v1.0.0` at 10:00:00.001

**PostgreSQL Handling**:
```
Transaction A: BEGIN
Transaction B: BEGIN

Transaction A: INSERT ... ON CONFLICT DO UPDATE
Transaction B: INSERT ... ON CONFLICT DO UPDATE (waits for A)

Transaction A: COMMIT (succeeds)
Transaction B: COMMIT (succeeds, updates A's insert)

Result: Last write wins (B's version stored)
```

**Outcome**: Both succeed, B's version is final (deterministic)

### Scenario 2: Version Conflict with Different Digests

**Setup**:
- Existing: `oomkill-cost-optimized:v1.0.0` with digest `sha256:aaa...`
- New: `oomkill-cost-optimized:v1.0.0` with digest `sha256:bbb...`

**Controller Logic**:
```go
// Check existing digest before registration
existingDigest, err := c.getExistingDigest(playbookID, version)
if err == nil && existingDigest != newDigest {
    if !req.Force {
        return fmt.Errorf(
            "version %s already exists with different digest.\n"+
            "Existing: %s\n"+
            "New: %s\n"+
            "Use --force to overwrite",
            version, existingDigest, newDigest,
        )
    }
}
```

**Outcome**: Operator must explicitly use `--force` to overwrite

---

## Benefits of PostgreSQL Storage

### 1. Concurrent Access (100% confidence)
- ✅ ACID transactions
- ✅ Row-level locking
- ✅ Serializable isolation
- ✅ Unique constraints
- ✅ No application-level locking needed

### 2. JSONB Querying (99% confidence)
- ✅ Label-based searches (`labels @> '{"signal_type": "OOMKilled"}'`)
- ✅ Parameter queries
- ✅ GIN indexes for performance
- ✅ Native JSON operators

### 3. Durability (100% confidence)
- ✅ Persistent storage
- ✅ Backup/restore
- ✅ Point-in-time recovery
- ✅ Replication support

### 4. Integration (99% confidence)
- ✅ Same database as other kubernaut services
- ✅ Shared connection pool
- ✅ Transaction coordination
- ✅ Consistent backup strategy

---

## Migration from Mock MCP

### Current (v1.0): Mock MCP Server
```python
# In-memory dictionary
MOCK_PLAYBOOKS = {
    "oomkill-general": {...},
    "oomkill-cost-optimized": {...}
}
```

### Future (v1.1): Workflow Catalog Controller
```go
// PostgreSQL storage
db.Query("SELECT * FROM playbooks WHERE labels @> $1", labelsJSON)
```

### Migration Path
1. Keep Mock MCP for v1.0 compatibility
2. Add Workflow Catalog Controller in v1.1
3. Mock MCP reads from PostgreSQL (via catalog API)
4. Deprecate Mock MCP in v1.2

---

## Configuration

### Database Connection

```yaml
# config/playbook-catalog.yaml
database:
  host: postgres.kubernaut-system.svc.cluster.local
  port: 5432
  database: kubernaut
  user: workflow_catalog
  password: ${DB_PASSWORD}
  ssl_mode: require
  max_connections: 10
  connection_timeout: 30s

catalog:
  table_name: playbooks
  enable_pgvector: true  # For future semantic search
```

### Environment Variables

```bash
export DB_HOST=postgres.kubernaut-system.svc.cluster.local
export DB_PORT=5432
export DB_NAME=kubernaut
export DB_USER=workflow_catalog
export DB_PASSWORD=<secret>
export DB_SSL_MODE=require
```

---

## Summary

**Storage**: PostgreSQL + pgvector
**Concurrency**: Handled at database level (ACID transactions)
**Controller**: Go service with REST API
**Schema**: JSONB for flexible querying

**Confidence**: 100% (database-level concurrency is standard practice)

**Benefits**:
- ✅ No application-level locking needed
- ✅ ACID guarantees
- ✅ Flexible JSONB queries
- ✅ Integration with existing kubernaut infrastructure

**Status**: Confirmed for v1.1 implementation
**Next Step**: Define Workflow Catalog Controller API specification
