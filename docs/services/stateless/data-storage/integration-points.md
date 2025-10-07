# Data Storage Service - Integration Points

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API Service (Write-Focused)
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## Table of Contents

1. [Integration Overview](#integration-overview)
2. [Upstream Services (Writers)](#upstream-services-writers)
3. [Downstream Services (Databases)](#downstream-services-databases)
4. [Integration Patterns](#integration-patterns)
5. [Error Handling](#error-handling)
6. [Data Flow Diagrams](#data-flow-diagrams)

---

## Integration Overview

### **Service Position in Architecture**

Data Storage Service acts as the **centralized write gateway** in the Kubernaut architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                    Upstream Services                        │
│  (Write audit data to Data Storage Service)                │
│                                                             │
│  • Gateway Service                                          │
│  • AI Analysis Controller                                   │
│  • Workflow Execution Controller                            │
│  • Kubernetes Executor Controller                           │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ HTTP POST /api/v1/audit/*
                     │ (Bearer Token Authentication)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│              Data Storage Service (Port 8080)               │
│                                                             │
│  1. Authenticate with Kubernetes TokenReviewer              │
│  2. Authorize service account for write operations          │
│  3. Validate audit data against schema                      │
│  4. Generate vector embedding                               │
│  5. Write to PostgreSQL (audit trail)                       │
│  6. Write to Vector DB (embeddings)                         │
│  7. Return 201 Created response                             │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ SQL INSERT (PostgreSQL)
                     │ pgvector INSERT (Vector DB)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                 Downstream Services                         │
│  (Data Storage writes to databases)                         │
│                                                             │
│  • PostgreSQL (Audit Trail - Port 5432)                     │
│  • Vector DB / pgvector (Embeddings - Port 5433)            │
└─────────────────────────────────────────────────────────────┘
```

---

## Upstream Services (Writers)

### **1. Gateway Service**

**Purpose**: Writes remediation request audit trail after CRD creation

**Integration Pattern**: HTTP POST
**Authentication**: Bearer Token (gateway-sa)
**Endpoint**: `POST /api/v1/audit/remediation`

#### **Write Flow**

```go
// In Gateway Service
package gateway

import (
    "context"
    "time"

    "go.uber.org/zap"
)

func (g *GatewayService) CreateRemediationRequest(ctx context.Context, signal *NormalizedSignal) error {
    // 1. Create RemediationRequest CRD
    rr, err := g.k8sClient.Create(ctx, remediationRequest)
    if err != nil {
        return err
    }

    // 2. Write audit trail to Data Storage
    auditReq := &datastorage.RemediationAuditRequest{
        RemediationRequestID: rr.Name,
        AlertName:            signal.AlertName,
        Namespace:            signal.Namespace,
        Cluster:              g.clusterName,
        Priority:             rr.Spec.Priority,
        CreatedAt:            time.Now(),
    }

    _, err = g.dataStorageClient.WriteRemediationAudit(ctx, auditReq)
    if err != nil {
        g.logger.Error("Failed to write audit trail",
            zap.String("remediation_request_id", rr.Name),
            zap.Error(err),
        )
        // Continue - audit failure should not block remediation
    }

    return nil
}
```

#### **Client Implementation**

```go
package datastorage

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "go.uber.org/zap"
)

type DataStorageClient struct {
    baseURL    string
    httpClient *http.Client
    token      string
    logger     *zap.Logger
}

func NewDataStorageClient(baseURL, token string, logger *zap.Logger) *DataStorageClient {
    return &DataStorageClient{
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: 10 * time.Second},
        token:      token,
        logger:     logger,
    }
}

func (c *DataStorageClient) WriteRemediationAudit(ctx context.Context, req *RemediationAuditRequest) (*WriteResponse, error) {
    payload, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal audit request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST",
        fmt.Sprintf("%s/api/v1/audit/remediation", c.baseURL),
        bytes.NewReader(payload),
    )
    if err != nil {
        return nil, err
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("failed to write audit: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    var writeResp WriteResponse
    if err := json.NewDecoder(resp.Body).Decode(&writeResp); err != nil {
        return nil, err
    }

    return &writeResp, nil
}
```

#### **Data Flow**

```
Gateway Service
    │
    │ 1. Signal received
    │ 2. RemediationRequest CRD created
    │
    └──> POST /api/v1/audit/remediation
         {
           "remediationRequestID": "rr-abc123",
           "alertName": "HighMemoryUsage",
           "namespace": "production",
           "cluster": "us-west-2",
           "priority": "P0"
         }
         │
         ▼
    Data Storage Service
         │
         │ 3. Validate request
         │ 4. Generate embedding
         │ 5. Write to PostgreSQL
         │ 6. Write embedding to Vector DB
         │
         └──> 201 Created
              {
                "auditID": "audit-xyz789",
                "status": "persisted"
              }
```

---

### **2. AI Analysis Controller**

**Purpose**: Writes remediation decision audit trail

**Integration Pattern**: HTTP POST
**Authentication**: Bearer Token (aianalysis-controller-sa)
**Endpoint**: `POST /api/v1/audit/remediation`

#### **Write Flow**

```go
// In AI Analysis Controller
package controllers

import (
    "context"
    "time"

    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    "go.uber.org/zap"
)

func (c *AIAnalysisController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    // ... AI analysis logic ...

    // Write audit trail for AI decision
    auditReq := &datastorage.RemediationAuditRequest{
        RemediationRequestID: rr.Name,
        AlertName:            rr.Spec.AlertName,
        Namespace:            rr.Spec.Namespace,
        Cluster:              rr.Spec.Cluster,
        ActionType:           aiDecision.RecommendedAction,
        Confidence:           aiDecision.Confidence,
        Reasoning:            aiDecision.Reasoning,
        CreatedAt:            time.Now(),
    }

    _, err := c.dataStorageClient.WriteRemediationAudit(ctx, auditReq)
    if err != nil {
        c.logger.Error("Failed to write AI decision audit",
            zap.String("remediation_request_id", rr.Name),
            zap.Error(err),
        )
    }

    return reconcile.Result{}, nil
}
```

---

### **3. Workflow Execution Controller**

**Purpose**: Writes workflow step execution audit trail

**Integration Pattern**: HTTP POST
**Authentication**: Bearer Token (workflowexecution-controller-sa)
**Endpoint**: `POST /api/v1/audit/workflow`

#### **Write Flow**

```go
// In Workflow Execution Controller
package controllers

import (
    "context"
    "time"

    "go.uber.org/zap"
)

func (c *WorkflowExecutionController) executeStep(ctx context.Context, step *WorkflowStep) error {
    startTime := time.Now()

    // Execute step
    err := step.Execute(ctx)

    // Write audit trail for step execution
    auditReq := &datastorage.WorkflowAuditRequest{
        WorkflowID:   step.WorkflowID,
        StepName:     step.Name,
        StepStatus:   getStepStatus(err),
        Duration:     time.Since(startTime),
        ErrorMessage: getErrorMessage(err),
        ExecutedAt:   startTime,
    }

    _, writeErr := c.dataStorageClient.WriteWorkflowAudit(ctx, auditReq)
    if writeErr != nil {
        c.logger.Error("Failed to write workflow audit",
            zap.String("workflow_id", step.WorkflowID),
            zap.String("step_name", step.Name),
            zap.Error(writeErr),
        )
    }

    return err
}
```

#### **Client Method**

```go
package datastorage

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

func (c *DataStorageClient) WriteWorkflowAudit(ctx context.Context, req *WorkflowAuditRequest) (*WriteResponse, error) {
    payload, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal workflow audit: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST",
        fmt.Sprintf("%s/api/v1/audit/workflow", c.baseURL),
        bytes.NewReader(payload),
    )
    if err != nil {
        return nil, err
    }

    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("failed to write workflow audit: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
    }

    var writeResp WriteResponse
    if err := json.NewDecoder(resp.Body).Decode(&writeResp); err != nil {
        return nil, err
    }

    return &writeResp, nil
}
```

---

### **4. Kubernetes Executor Controller**

**Purpose**: Writes Kubernetes action execution audit trail

**Integration Pattern**: HTTP POST
**Authentication**: Bearer Token (kubernetes-executor-sa)
**Endpoint**: `POST /api/v1/audit/remediation`

#### **Write Flow**

```go
// In Kubernetes Executor Controller
package controllers

import (
    "context"
    "time"

    "go.uber.org/zap"
)

func (c *KubernetesExecutorController) executeAction(ctx context.Context, action *KubernetesAction) error {
    startTime := time.Now()

    // Execute Kubernetes action
    result, err := c.k8sClient.Execute(ctx, action)

    // Write audit trail for action execution
    auditReq := &datastorage.RemediationAuditRequest{
        RemediationRequestID: action.RemediationRequestID,
        ActionType:           action.Type,
        TargetResource:       action.TargetResource,
        Status:               getActionStatus(err),
        Duration:             time.Since(startTime),
        ErrorMessage:         getErrorMessage(err),
        ExecutedAt:           startTime,
    }

    _, writeErr := c.dataStorageClient.WriteRemediationAudit(ctx, auditReq)
    if writeErr != nil {
        c.logger.Error("Failed to write action audit",
            zap.String("remediation_request_id", action.RemediationRequestID),
            zap.String("action_type", action.Type),
            zap.Error(writeErr),
        )
    }

    return err
}
```

---

## Downstream Services (Databases)

### **1. PostgreSQL (Audit Trail)**

**Purpose**: Persistent storage for audit records

**Connection**: SQL over SSL/TLS
**Port**: 5432
**Database**: `kubernaut_audit`

#### **Schema**

```sql
CREATE TABLE remediation_audit (
    id VARCHAR(255) PRIMARY KEY,
    remediation_request_id VARCHAR(255) NOT NULL,
    alert_name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    cluster VARCHAR(255) NOT NULL,
    action_type VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    confidence FLOAT,
    reasoning TEXT,
    duration_ms INT,
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    INDEX idx_remediation_request_id (remediation_request_id),
    INDEX idx_alert_name (alert_name),
    INDEX idx_cluster_namespace (cluster, namespace),
    INDEX idx_created_at (created_at DESC)
);

CREATE TABLE workflow_audit (
    id VARCHAR(255) PRIMARY KEY,
    workflow_id VARCHAR(255) NOT NULL,
    step_name VARCHAR(255) NOT NULL,
    step_status VARCHAR(50) NOT NULL,
    duration_ms INT NOT NULL,
    error_message TEXT,
    executed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    INDEX idx_workflow_id (workflow_id),
    INDEX idx_executed_at (executed_at DESC)
);
```

#### **Write Pattern**

```go
package storage

import (
    "context"
    "fmt"

    "go.uber.org/zap"
)

func (s *DataStorageService) WriteRemediationAudit(ctx context.Context, audit *RemediationAudit) error {
    query := `
        INSERT INTO remediation_audit (
            id, remediation_request_id, alert_name, namespace, cluster,
            action_type, status, confidence, reasoning, duration_ms,
            error_message, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    `

    _, err := s.db.ExecContext(ctx, query,
        audit.ID,
        audit.RemediationRequestID,
        audit.AlertName,
        audit.Namespace,
        audit.Cluster,
        audit.ActionType,
        audit.Status,
        audit.Confidence,
        audit.Reasoning,
        audit.DurationMs,
        audit.ErrorMessage,
        audit.CreatedAt,
    )

    if err != nil {
        s.logger.Error("Failed to write remediation audit to PostgreSQL",
            zap.String("audit_id", audit.ID),
            zap.Error(err),
        )
        return fmt.Errorf("postgresql write failed: %w", err)
    }

    s.logger.Info("Remediation audit persisted to PostgreSQL",
        zap.String("audit_id", audit.ID),
        zap.String("remediation_request_id", audit.RemediationRequestID),
    )

    return nil
}
```

---

### **2. Vector DB / pgvector (Embeddings)**

**Purpose**: Semantic similarity search for historical incidents

**Connection**: SQL over SSL/TLS (PostgreSQL with pgvector extension)
**Port**: 5433
**Database**: `kubernaut_vectors`

#### **Schema**

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE audit_embeddings (
    audit_id VARCHAR(255) PRIMARY KEY,
    embedding vector(768) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- HNSW index for fast similarity search
CREATE INDEX ON audit_embeddings USING hnsw (embedding vector_cosine_ops);
```

#### **Write Pattern**

```go
package storage

import (
    "context"
    "fmt"
    "strings"
    "time"

    "go.uber.org/zap"
)

func (s *DataStorageService) WriteEmbedding(ctx context.Context, auditID string, embedding *Embedding) error {
    query := `
        INSERT INTO audit_embeddings (audit_id, embedding, created_at)
        VALUES ($1, $2, $3)
    `

    // Convert embedding to pgvector format
    vectorStr := fmt.Sprintf("[%s]", strings.Join(floatsToStrings(embedding.Vector), ","))

    _, err := s.vectorDB.ExecContext(ctx, query,
        auditID,
        vectorStr,
        time.Now(),
    )

    if err != nil {
        s.logger.Error("Failed to write embedding to Vector DB",
            zap.String("audit_id", auditID),
            zap.Error(err),
        )
        return fmt.Errorf("vector db write failed: %w", err)
    }

    s.logger.Info("Embedding persisted to Vector DB",
        zap.String("audit_id", auditID),
        zap.Int("vector_size", len(embedding.Vector)),
    )

    return nil
}
```

#### **Similarity Search Pattern**

```go
package storage

import (
    "context"
    "fmt"
    "strings"
)

func (s *DataStorageService) FindSimilarIncidents(ctx context.Context, embedding *Embedding, limit int) ([]*SimilarIncident, error) {
    query := `
        SELECT audit_id, 1 - (embedding <=> $1) AS similarity
        FROM audit_embeddings
        ORDER BY embedding <=> $1
        LIMIT $2
    `

    vectorStr := fmt.Sprintf("[%s]", strings.Join(floatsToStrings(embedding.Vector), ","))

    rows, err := s.vectorDB.QueryContext(ctx, query, vectorStr, limit)
    if err != nil {
        return nil, fmt.Errorf("similarity search failed: %w", err)
    }
    defer rows.Close()

    var incidents []*SimilarIncident
    for rows.Next() {
        var incident SimilarIncident
        if err := rows.Scan(&incident.AuditID, &incident.Similarity); err != nil {
            return nil, err
        }
        incidents = append(incidents, &incident)
    }

    return incidents, nil
}
```

---

## Integration Patterns

### **Pattern 1: Synchronous Write with Best-Effort Guarantee**

**Used by**: Gateway, AI Analysis, Workflow Execution, Kubernetes Executor

**Characteristics**:
- Audit write failure does **NOT** block primary operation
- Errors are logged but not propagated
- Primary operation success is independent of audit persistence

**Rationale**: Audit trail is important but **not critical** for remediation success.

```go
package gateway

import (
    "context"

    "go.uber.org/zap"
)

// Primary operation
err := createRemediationRequest(ctx, rr)
if err != nil {
    return err // PRIMARY OPERATION FAILED
}

// Best-effort audit write
_, auditErr := dataStorageClient.WriteAudit(ctx, audit)
if auditErr != nil {
    logger.Error("Audit write failed", zap.Error(auditErr))
    // Continue - do not block primary operation
}

return nil // PRIMARY OPERATION SUCCEEDED
```

---

### **Pattern 2: Atomic Database Transaction**

**Used by**: Data Storage Service internal writes

**Characteristics**:
- PostgreSQL write and Vector DB write are in separate transactions
- If PostgreSQL succeeds but Vector DB fails, PostgreSQL is **NOT** rolled back
- Partial success is acceptable (audit trail more important than embedding)

**Rationale**: Audit trail (PostgreSQL) is more critical than embeddings (Vector DB).

```go
package storage

import (
    "context"
    "fmt"

    "go.uber.org/zap"
)

func (s *DataStorageService) PersistAudit(ctx context.Context, audit *RemediationAudit) error {
    // 1. Write to PostgreSQL (CRITICAL)
    if err := s.WriteRemediationAudit(ctx, audit); err != nil {
        return fmt.Errorf("critical: postgresql write failed: %w", err)
    }

    // 2. Generate embedding
    embedding, err := s.GenerateEmbedding(audit)
    if err != nil {
        s.logger.Warn("Embedding generation failed",
            zap.String("audit_id", audit.ID),
            zap.Error(err),
        )
        return nil // PostgreSQL succeeded, continue
    }

    // 3. Write embedding to Vector DB (BEST-EFFORT)
    if err := s.WriteEmbedding(ctx, audit.ID, embedding); err != nil {
        s.logger.Warn("Vector DB write failed",
            zap.String("audit_id", audit.ID),
            zap.Error(err),
        )
        // Continue - PostgreSQL succeeded
    }

    return nil
}
```

---

## Error Handling

### **Client-Side Error Handling**

```go
package gateway

import (
    "context"
    "time"

    "go.uber.org/zap"
)

func (g *GatewayService) writeAuditWithRetry(ctx context.Context, audit *datastorage.RemediationAuditRequest) {
    maxRetries := 3
    backoff := 100 * time.Millisecond

    for attempt := 1; attempt <= maxRetries; attempt++ {
        _, err := g.dataStorageClient.WriteRemediationAudit(ctx, audit)
        if err == nil {
            return // Success
        }

        g.logger.Warn("Audit write failed, retrying",
            zap.Int("attempt", attempt),
            zap.Int("max_retries", maxRetries),
            zap.Error(err),
        )

        if attempt < maxRetries {
            time.Sleep(backoff)
            backoff *= 2 // Exponential backoff
        }
    }

    g.logger.Error("Audit write failed after all retries",
        zap.String("audit_id", audit.RemediationRequestID),
    )
}
```

### **Server-Side Error Responses**

```go
// HTTP 400 - Bad Request
{
    "error": "validation_failed",
    "message": "RemediationRequestID is required",
    "field": "remediationRequestID"
}

// HTTP 401 - Unauthorized
{
    "error": "authentication_failed",
    "message": "Token validation failed"
}

// HTTP 403 - Forbidden
{
    "error": "authorization_failed",
    "message": "Service account not authorized for write operations"
}

// HTTP 429 - Rate Limit Exceeded
{
    "error": "rate_limit_exceeded",
    "message": "Too many requests. Limit: 100 req/s per service"
}

// HTTP 500 - Internal Server Error
{
    "error": "internal_error",
    "message": "Failed to persist audit trail",
    "correlation_id": "req-abc123"
}

// HTTP 503 - Service Unavailable
{
    "error": "service_unavailable",
    "message": "PostgreSQL connection failed"
}
```

---

## Data Flow Diagrams

### **Complete Audit Write Flow**

```
┌──────────────────────────────────────────────────────────────────┐
│ Step 1: Upstream Service (e.g., Gateway)                        │
│   - Primary operation completes (create RemediationRequest CRD)  │
│   - Prepare audit request                                        │
│   - Send POST /api/v1/audit/remediation                          │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ HTTP POST (Bearer Token)
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 2: Data Storage Service - Authentication                   │
│   - Extract Bearer token from Authorization header              │
│   - Validate token with Kubernetes TokenReviewer                │
│   - Extract service account identity                             │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Valid Token
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 3: Data Storage Service - Authorization                    │
│   - Check service account against authorized list               │
│   - Verify write permission for audit type                      │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Authorized
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 4: Data Storage Service - Validation                       │
│   - Validate required fields (ID, RemediationRequestID, etc.)   │
│   - Validate field formats (timestamps, UUIDs, etc.)            │
│   - Validate business rules (status, action types, etc.)        │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Valid
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 5: Data Storage Service - Embedding Generation             │
│   - Extract features from audit data                            │
│   - Generate 768-dimensional embedding vector                   │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Embedding
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 6: Data Storage Service - PostgreSQL Write                 │
│   - Prepare SQL INSERT with prepared statement                  │
│   - Execute INSERT to remediation_audit table                   │
│   - Commit transaction                                           │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ PostgreSQL Success
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 7: Data Storage Service - Vector DB Write (Best-Effort)    │
│   - Convert embedding to pgvector format                        │
│   - Execute INSERT to audit_embeddings table                    │
│   - Commit transaction                                           │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ Success (or logged failure)
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 8: Data Storage Service - Response                         │
│   - Format 201 Created response                                 │
│   - Return audit ID and status                                  │
│   - Log successful write                                        │
└─────────────────────────┬────────────────────────────────────────┘
                          │
                          │ HTTP 201 Created
                          ▼
┌──────────────────────────────────────────────────────────────────┐
│ Step 9: Upstream Service                                        │
│   - Receive success response                                    │
│   - Log audit write success (or failure)                        │
│   - Continue with primary workflow                              │
└──────────────────────────────────────────────────────────────────┘
```

---

## Reference Documentation

- **API Specification**: `docs/services/stateless/data-storage/api-specification.md`
- **Security Configuration**: `docs/services/stateless/data-storage/security-configuration.md`
- **Testing Strategy**: `docs/services/stateless/data-storage/testing-strategy.md`
- **TokenReviewer Auth**: `docs/architecture/KUBERNETES_TOKENREVIEWER_AUTH.md`
- **Service Dependency Map**: `docs/architecture/SERVICE_DEPENDENCY_MAP.md`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Integration Status**: Design complete, implementation pending

