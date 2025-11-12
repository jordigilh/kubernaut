# DD-AUDIT-003: Service Audit Trace Requirements

**Status**: ✅ **APPROVED** (Production Standard)
**Date**: November 8, 2025
**Last Reviewed**: November 8, 2025
**Confidence**: 95%
**Authority Level**: SYSTEM-WIDE - Defines audit requirements for all 11 services

---

## 🎯 **Overview**

This design decision establishes **which services MUST generate audit traces** and **which services should NOT**, providing clear guidance for audit implementation across all Kubernaut services.

**Key Principle**: Audit traces are mandatory for business-critical operations, compliance requirements, and state-changing operations. Read-only and configuration services use application logs instead.

**Scope**: All 11 Kubernaut services (4 CRD controllers + 7 stateless services).

**Decision Summary**:
- ✅ **6 services MUST** generate audit traces (P0 - business-critical)
- ✅ **2 services SHOULD** generate audit traces (P1 - operational visibility)
- ⚠️ **3 services NO audit** traces needed (read-only/configuration)

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Requirements](#requirements)
3. [Decision](#decision)
4. [Service-by-Service Analysis](#service-by-service-analysis)
5. [Implementation Priority](#implementation-priority)
6. [Audit Event Volume Estimates](#audit-event-volume-estimates)
7. [Industry Precedents](#industry-precedents)
8. [Related Decisions](#related-decisions)

---

## 🎯 **Context & Problem**

### **Challenge**

Kubernaut consists of 11 microservices with different responsibilities. Not all services require audit traces:

1. ⚠️ **Over-Auditing Risk**: Auditing read-only operations wastes storage and degrades performance
2. ⚠️ **Under-Auditing Risk**: Missing audit traces for business-critical operations violates compliance
3. ⚠️ **Inconsistent Standards**: No clear guidance on which services should audit
4. ⚠️ **Cost Impact**: Unnecessary audit traces increase storage costs

### **Business Impact**

- **Compliance**: SOC 2, ISO 27001, GDPR require audit trails for specific operations
- **Debugging**: Audit traces critical for troubleshooting business-critical failures
- **Cost Optimization**: Avoiding unnecessary audit traces reduces storage costs
- **Performance**: Over-auditing can impact service performance

### **Key Question**

**Which of the 11 Kubernaut services should generate audit traces?**

---

## 📋 **Requirements**

### **Audit Trace Decision Criteria**

| Criterion | Description | Weight |
|-----------|-------------|--------|
| **Business-Critical Operations** | Creates/modifies/deletes business resources | ⭐⭐⭐⭐⭐ |
| **Compliance Requirements** | SOC 2, ISO 27001, GDPR audit trail | ⭐⭐⭐⭐⭐ |
| **External Interactions** | Receives external signals, sends external notifications | ⭐⭐⭐⭐ |
| **State Changes** | Modifies CRDs, database records, or external systems | ⭐⭐⭐⭐ |
| **Debugging Value** | Critical for troubleshooting and root cause analysis | ⭐⭐⭐ |
| **Performance Impact** | High-volume operations that need monitoring | ⭐⭐⭐ |

### **Service Inventory**

**Stateless Services** (7):
1. Gateway Service
2. Data Storage Service
3. Context API Service
4. HolmesGPT API Service
5. Dynamic Toolset Service
6. Notification Service
7. Effectiveness Monitor Service

**CRD Controllers** (4):
8. Signal Processing Controller
9. AI Analysis Controller
10. Remediation Execution Controller
11. Remediation Orchestrator Controller

---

## ✅ **Decision**

**APPROVED**: **8 out of 11 services** generate audit traces

**Breakdown**:
- ✅ **6 services MUST** (P0): Gateway, AI Analysis, Remediation Execution, Notification, Data Storage, Effectiveness Monitor
- ✅ **2 services SHOULD** (P1): Signal Processing, Remediation Orchestrator
- ⚠️ **3 services NO audit**: Context API, HolmesGPT API, Dynamic Toolset

**Rationale**:
1. **Compliance Alignment**: P0 services meet SOC 2, ISO 27001, GDPR requirements
2. **Industry Standards**: Matches audit patterns from AWS, Google, Kubernetes
3. **Cost Optimization**: Avoids auditing read-only operations (industry standard)
4. **Performance**: Minimizes audit overhead while maintaining compliance

---

## 📊 **Service-by-Service Analysis**

### **P0: MUST Generate Audit Traces (6 Services)**

---

#### 1. Gateway Service ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Signal ingestion is the entry point for all remediations
- ✅ **Compliance**: External signal ingestion requires audit trail (SOC 2, ISO 27001)
- ✅ **External Interactions**: Receives Prometheus alerts, K8s events, custom webhooks
- ✅ **State Changes**: Creates `RemediationRequest` CRDs
- ✅ **Debugging Value**: Critical for tracing signal flow
- ✅ **Performance Impact**: High-volume (1000+ signals/day)

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `gateway.signal.received` | Signal received from external source | P0 |
| `gateway.signal.deduplicated` | Duplicate signal detected | P0 |
| `gateway.signal.storm_detected` | Storm detection triggered | P0 |
| `gateway.crd.created` | RemediationRequest CRD created | P0 |
| `gateway.crd.creation_failed` | CRD creation failed | P0 |

**Industry Precedent**: AWS EventBridge, Google Cloud Pub/Sub, Azure Event Grid

**Expected Volume**: 1,000 events/day, 30 MB/month

---

#### 2. AI Analysis Controller ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: AI recommendations drive remediation actions
- ✅ **Compliance**: AI decision-making requires audit trail (AI Act, SOC 2)
- ✅ **External Interactions**: Calls HolmesGPT API (external LLM)
- ✅ **State Changes**: Updates `AIAnalysis` CRD with recommendations
- ✅ **Debugging Value**: Critical for understanding AI decisions
- ✅ **Cost Tracking**: LLM API costs need monitoring

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `ai-analysis.investigation.started` | AI investigation started | P0 |
| `ai-analysis.llm.request_sent` | LLM API request sent | P0 |
| `ai-analysis.llm.response_received` | LLM API response received | P0 |
| `ai-analysis.recommendation.generated` | Remediation recommendation generated | P0 |
| `ai-analysis.crd.updated` | AIAnalysis CRD updated | P0 |
| `ai-analysis.llm.request_failed` | LLM API request failed | P0 |

**Industry Precedent**: OpenAI API logs, Anthropic Claude logs, AWS Bedrock audit logs

**Expected Volume**: 500 events/day, 15 MB/month

---

#### 3. Remediation Execution Controller ⭐⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Executes remediation actions (kubectl apply, scale, delete)
- ✅ **Compliance**: Kubernetes operations require audit trail (SOC 2, ISO 27001)
- ✅ **State Changes**: Modifies Kubernetes resources (Deployments, Pods, ConfigMaps)
- ✅ **Safety**: Critical for rollback and incident investigation
- ✅ **Debugging Value**: Essential for understanding what actions were executed

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `execution.workflow.started` | Tekton workflow started | P0 |
| `execution.action.executed` | Kubernetes action executed | P0 |
| `execution.action.succeeded` | Action succeeded | P0 |
| `execution.action.failed` | Action failed | P0 |
| `execution.workflow.completed` | Workflow completed | P0 |
| `execution.rollback.triggered` | Rollback triggered | P0 |

**Industry Precedent**: Kubernetes Audit Logs, Argo Workflows audit, Tekton Pipelines logs

**Expected Volume**: 2,000 events/day, 60 MB/month

---

#### 4. Notification Service ⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Compliance**: Notification delivery requires audit trail (SOC 2)
- ✅ **External Interactions**: Sends Slack messages, emails, PagerDuty alerts
- ✅ **State Changes**: Updates `Notification` CRD status
- ✅ **Debugging Value**: Critical for troubleshooting notification failures
- ✅ **SLA Tracking**: Notification delivery time monitoring

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `notification.message.sent` | Notification sent to external channel | P0 |
| `notification.message.delivered` | Notification delivered successfully | P0 |
| `notification.message.failed` | Notification delivery failed | P0 |
| `notification.crd.updated` | Notification CRD status updated | P1 |

**Industry Precedent**: PagerDuty audit logs, Slack audit logs, SendGrid event webhooks

**Expected Volume**: 500 events/day, 15 MB/month

---

#### 5. Data Storage Service ⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces (internal only)

**Rationale**:
- ✅ **Business-Critical**: Central data access layer (ADR-032)
- ✅ **Compliance**: Database operations require audit trail (SOC 2, ISO 27001)
- ✅ **State Changes**: All PostgreSQL writes go through this service
- ✅ **Debugging Value**: Critical for data integrity troubleshooting
- ✅ **Performance Monitoring**: Database query performance tracking

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `data-storage.audit.write` | Audit event written to PostgreSQL | P0 |
| `data-storage.audit.batch_written` | Audit batch written (from async buffer) | P0 |
| `data-storage.audit.write_failed` | Audit write failed | P0 |
| `data-storage.query.executed` | Query executed (internal monitoring) | P2 |

**Note**: Data Storage Service audits are **internal** (service health monitoring), not business operations.

**Industry Precedent**: AWS RDS audit logs, Google Cloud SQL audit logs

**Expected Volume**: 5,000 events/day, 150 MB/month

---

#### 6. Effectiveness Monitor Service ⭐⭐⭐⭐

**Status**: ✅ **MUST** generate audit traces

**Rationale**:
- ✅ **Business-Critical**: Tracks remediation effectiveness (learning loop)
- ✅ **Compliance**: Effectiveness metrics require audit trail (SOC 2)
- ✅ **State Changes**: Updates effectiveness scores, triggers retraining
- ✅ **Debugging Value**: Critical for understanding AI learning
- ✅ **ML Observability**: Model performance tracking

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `effectiveness.assessment.started` | Effectiveness assessment started | P0 |
| `effectiveness.score.calculated` | Effectiveness score calculated | P0 |
| `effectiveness.learning.triggered` | Learning feedback triggered | P0 |
| `effectiveness.crd.updated` | Effectiveness CRD updated | P1 |

**Industry Precedent**: MLflow tracking, Weights & Biases audit logs, Kubeflow Pipelines logs

**Expected Volume**: 500 events/day, 15 MB/month

---

### **P1: SHOULD Generate Audit Traces (2 Services)**

---

#### 7. Signal Processing Controller ⭐⭐⭐

**Status**: ✅ **SHOULD** generate audit traces (operational visibility)

**Rationale**:
- ✅ **State Changes**: Enriches signals with Kubernetes context
- ✅ **Debugging Value**: Useful for troubleshooting enrichment failures
- ⚠️ **Not Business-Critical**: Enrichment is supplementary (not core operation)
- ⚠️ **Low Volume**: Only runs once per signal

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `signal-processing.enrichment.started` | Signal enrichment started | P1 |
| `signal-processing.enrichment.completed` | Signal enrichment completed | P1 |
| `signal-processing.enrichment.failed` | Signal enrichment failed | P1 |
| `signal-processing.crd.updated` | SignalProcessing CRD updated | P2 |

**Recommendation**: ✅ Generate audit traces for operational visibility, but P1 priority (not P0).

**Expected Volume**: 1,000 events/day, 30 MB/month

---

#### 8. Remediation Orchestrator Controller ⭐⭐⭐

**Status**: ✅ **SHOULD** generate audit traces (coordination visibility)

**Rationale**:
- ✅ **State Changes**: Coordinates lifecycle across 4 CRD controllers
- ✅ **Debugging Value**: Useful for troubleshooting coordination issues
- ⚠️ **Not Business-Critical**: Orchestration is coordination (not core operation)
- ⚠️ **Low Volume**: Only runs once per remediation

**Audit Events**:

| Event Type | Description | Priority |
|------------|-------------|----------|
| `orchestrator.lifecycle.started` | Remediation lifecycle started | P1 |
| `orchestrator.phase.transitioned` | Phase transition (signal → AI → execution → notification) | P1 |
| `orchestrator.lifecycle.completed` | Remediation lifecycle completed | P1 |
| `orchestrator.crd.updated` | RemediationRequest CRD updated | P2 |

**Recommendation**: ✅ Generate audit traces for coordination visibility, but P1 priority (not P0).

**Expected Volume**: 1,000 events/day, 30 MB/month

---

### **NO Audit Traces Needed (3 Services)**

---

#### 9. Context API Service ❌

**Status**: ⚠️ **NO** audit traces needed

**Rationale**:
- ❌ **Read-Only**: Only queries historical data (no state changes)
- ❌ **No External Interactions**: Internal service (called by AI Analysis)
- ❌ **No Compliance Requirements**: Read operations don't require audit trail
- ❌ **Low Debugging Value**: Query failures are transient (retry handles)
- ✅ **Alternative**: Application logs sufficient for debugging

**Why No Audit Traces**:
- Context API is a **data provider** (like a database read replica)
- Industry standard: Read-only APIs don't generate audit traces
- Example: AWS S3 GET requests are NOT audited (only PUT/DELETE)

**Alternative Observability**:
- ✅ Application logs (structured logging)
- ✅ Prometheus metrics (query latency, cache hit rate)
- ✅ OpenTelemetry traces (request tracing)

**Industry Precedent**: AWS S3 GET (not audited), Google Cloud Storage read (not audited)

**⚠️ Future Consideration: PII Access Tracking**:

**Issue**: Context API handles PII data (incident details). GDPR Article 30 *may* require PII access tracking.

**Current Decision**: NO audit traces (v1.0)
- ✅ Not required for SOC 2, ISO 27001
- ✅ Industry standard: Read-only APIs not audited
- ✅ Cost optimization: Avoids high-volume audit traces

**Escape Hatch** (v2.0+):

If compliance requirements change (GDPR Article 30, HIPAA), Context API can enable optional PII access auditing:

```go
// pkg/contextapi/server.go
type Config struct {
    // Optional: Enable PII access audit (default: false)
    EnablePIIAccessAudit bool
}

func (s *Server) handleQuery(ctx context.Context, query *Query) (*QueryResult, error) {
    result, err := s.executeQuery(ctx, query)

    // Optional PII access audit (disabled by default)
    if s.config.EnablePIIAccessAudit && s.auditStore != nil {
        event := audit.NewAuditEvent()
        event.EventType = "context-api.pii.accessed"
        event.EventCategory = "data_access"
        event.EventAction = "read"
        event.EventOutcome = "success"
        event.IsSensitive = true // Mark as PII access
        _ = s.auditStore.StoreAudit(ctx, event)
    }

    return result, err
}
```

**Configuration**:
```yaml
# v1.0: No audit (default)
context_api:
  enable_pii_access_audit: false

# v2.0+: Enable if GDPR/HIPAA required
context_api:
  enable_pii_access_audit: true
```

**Impact if Enabled**:
- ⚠️ High audit volume (1000+ events/day)
- ⚠️ Storage cost increase (~30 MB/month)
- ✅ GDPR Article 30 compliance
- ✅ HIPAA compliance (if applicable)

**Recommendation**: Monitor compliance requirements and enable if needed in v2.0+.

---

#### 10. HolmesGPT API Service ❌

**Status**: ⚠️ **NO** audit traces needed (delegated to AI Analysis Controller)

**Rationale**:
- ❌ **Wrapper Service**: Thin wrapper around HolmesGPT SDK
- ❌ **No State Changes**: Only proxies requests to external LLM
- ❌ **Audit Responsibility**: AI Analysis Controller audits LLM interactions
- ✅ **Alternative**: Application logs + metrics sufficient

**Why No Audit Traces**:
- HolmesGPT API is a **proxy** (like an API gateway)
- Audit responsibility belongs to the **caller** (AI Analysis Controller)
- Industry standard: Proxy services don't duplicate audit traces

**Alternative Observability**:
- ✅ Application logs (request/response logging)
- ✅ Prometheus metrics (LLM latency, error rate)
- ✅ OpenTelemetry traces (distributed tracing)

**Industry Precedent**: AWS API Gateway (not audited), Nginx reverse proxy (not audited)

---

#### 11. Dynamic Toolset Service ❌

**Status**: ⚠️ **NO** audit traces needed

**Rationale**:
- ❌ **Configuration Service**: Only serves HolmesGPT toolset configuration
- ❌ **Read-Only**: No state changes (configuration is static)
- ❌ **No External Interactions**: Internal service (called by HolmesGPT API)
- ❌ **No Compliance Requirements**: Configuration reads don't require audit trail
- ✅ **Alternative**: Application logs sufficient

**Why No Audit Traces**:
- Dynamic Toolset is a **configuration provider** (like a ConfigMap)
- Industry standard: Configuration reads don't generate audit traces
- Example: Kubernetes ConfigMap reads are NOT audited

**Alternative Observability**:
- ✅ Application logs (configuration requests)
- ✅ Prometheus metrics (request count, latency)

**Industry Precedent**: Kubernetes ConfigMap reads (not audited), Consul KV reads (not audited)

---

## 📊 **Summary Table**

| Service | Audit Traces? | Priority | Rationale |
|---------|--------------|----------|-----------|
| **Gateway Service** | ✅ **MUST** | P0 | Business-critical signal ingestion |
| **AI Analysis Controller** | ✅ **MUST** | P0 | Business-critical AI recommendations |
| **Remediation Execution Controller** | ✅ **MUST** | P0 | Business-critical Kubernetes operations |
| **Notification Service** | ✅ **MUST** | P0 | Compliance-required notification delivery |
| **Data Storage Service** | ✅ **MUST** | P0 | Internal audit write monitoring |
| **Effectiveness Monitor Service** | ✅ **MUST** | P0 | Business-critical learning loop |
| **Signal Processing Controller** | ✅ **SHOULD** | P1 | Operational visibility (enrichment) |
| **Remediation Orchestrator Controller** | ✅ **SHOULD** | P1 | Operational visibility (coordination) |
| **Context API Service** | ❌ **NO** | N/A | Read-only, no state changes |
| **HolmesGPT API Service** | ❌ **NO** | N/A | Proxy, audit delegated to caller |
| **Dynamic Toolset Service** | ❌ **NO** | N/A | Configuration, read-only |

**Total**: **8 out of 11 services** generate audit traces (73%)

---

## 🎯 **Implementation Priority**

### Phase 1: P0 Services (MUST) - 6 Services

**Timeline**: Sprint 1-2 (2 weeks)

**Services**:
1. Gateway Service (Week 1)
2. Data Storage Service (Week 1)
3. AI Analysis Controller (Week 2)
4. Remediation Execution Controller (Week 2)
5. Notification Service (Week 2)
6. Effectiveness Monitor Service (Week 2)

**Effort**: 6 hours (1 hour per service)

**Implementation**:
- Use `pkg/audit/` shared library (see [DD-AUDIT-002](./DD-AUDIT-002-audit-shared-library-design.md))
- Follow async buffered pattern (see [ADR-035](./ADR-038-async-buffered-audit-ingestion.md))
- Store in unified audit table (see [ADR-034](./ADR-034-unified-audit-table-design.md))

---

### Phase 2: P1 Services (SHOULD) - 2 Services

**Timeline**: Sprint 3 (1 week)

**Services**:
1. Signal Processing Controller (Week 3)
2. Remediation Orchestrator Controller (Week 3)

**Effort**: 2 hours (1 hour per service)

**Implementation**:
- Same as Phase 1 (use shared library)
- Lower priority (can be deferred if needed)

---

### Phase 3: No Audit Traces - 3 Services

**Services**:
1. Context API Service (application logs only)
2. HolmesGPT API Service (application logs only)
3. Dynamic Toolset Service (application logs only)

**Effort**: 0 hours (no audit implementation needed)

**Alternative Observability**:
- Structured logging (see [DD-005](./DD-005-OBSERVABILITY-STANDARDS.md))
- Prometheus metrics
- OpenTelemetry traces

---

## 📋 **Audit Event Volume Estimates**

### Expected Audit Event Volume (Production)

| Service | Events/Day | Events/Month | Storage/Month |
|---------|-----------|--------------|---------------|
| **Gateway Service** | 1,000 | 30,000 | 30 MB |
| **AI Analysis Controller** | 500 | 15,000 | 15 MB |
| **Remediation Execution Controller** | 2,000 | 60,000 | 60 MB |
| **Notification Service** | 500 | 15,000 | 15 MB |
| **Data Storage Service** | 5,000 | 150,000 | 150 MB |
| **Effectiveness Monitor Service** | 500 | 15,000 | 15 MB |
| **Signal Processing Controller** | 1,000 | 30,000 | 30 MB |
| **Remediation Orchestrator Controller** | 1,000 | 30,000 | 30 MB |
| **TOTAL** | **11,500** | **345,000** | **345 MB** |

**Storage Cost**: ~$0.35/month (PostgreSQL storage at $0.10/GB)

**Retention**: 90 days (default), 7 years (compliance)

**Assumptions**:
- Average event size: 1 KB
- Production load: 1000 remediations/day
- Event compression: None (conservative estimate)

---

## 📚 **Industry Precedents**

### Services That Audit

| Kubernaut Service | Industry Equivalent | Audited? | Rationale |
|-------------------|---------------------|----------|-----------|
| Gateway Service | AWS EventBridge | ✅ Yes | Signal ingestion is business-critical |
| AI Analysis Controller | OpenAI API | ✅ Yes | AI decisions require audit trail |
| Remediation Execution Controller | Kubernetes API | ✅ Yes | Infrastructure changes require audit |
| Notification Service | PagerDuty | ✅ Yes | Notification delivery is compliance-required |
| Data Storage Service | AWS RDS | ✅ Yes | Database operations require audit |
| Effectiveness Monitor Service | MLflow | ✅ Yes | ML model performance requires tracking |

---

### Services That Don't Audit

| Kubernaut Service | Industry Equivalent | Audited? | Rationale |
|-------------------|---------------------|----------|-----------|
| Context API Service | AWS S3 GET | ❌ No | Read-only operations not audited |
| HolmesGPT API Service | AWS API Gateway | ❌ No | Proxy services don't audit (caller audits) |
| Dynamic Toolset Service | Kubernetes ConfigMap | ❌ No | Configuration reads not audited |

**Key Insight**: Industry standard is to audit **state-changing operations** and **external interactions**, NOT read-only or configuration operations.

---

## 🎯 **Confidence Assessment**

**Overall Confidence**: 95%

**Breakdown**:
- **P0 Services (MUST)**: 100% confidence (business-critical, compliance-required)
- **P1 Services (SHOULD)**: 90% confidence (operational visibility, not critical)
- **No Audit Services**: 95% confidence (read-only, no state changes)

**Why 95% (not 100%)**:
- 5% uncertainty: Potential future requirements for Context API (e.g., PII access tracking)
  - **Mitigation**: Re-evaluate if Context API adds write operations or PII access

---

## 🔗 **Related Decisions**

**Audit Architecture**:
- **ADR-034**: [Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) - Database schema for audit events
- **ADR-035**: [Asynchronous Buffered Audit Ingestion](./ADR-038-async-buffered-audit-ingestion.md) - How services write audit traces
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Distributed audit pattern (services write their own traces)
- **DD-AUDIT-002**: [Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md) - Implementation details for `pkg/audit/` shared library

**Supporting Decisions**:
- **ADR-032**: [Data Access Layer Isolation](./ADR-032-data-access-layer-isolation.md) - All DB access via Data Storage Service
- **DD-005**: [Observability Standards](./DD-005-OBSERVABILITY-STANDARDS.md) - Alternative observability for non-audited services
- **DD-007**: [Graceful Shutdown Pattern](./DD-007-kubernetes-aware-graceful-shutdown.md) - Ensures audit flush before shutdown

---

## 📝 **Implementation Checklist**

### Per-Service Implementation (8 services)

- [ ] **Add audit store initialization**
  ```go
  auditStore := audit.NewBufferedStore(dsClient, audit.DefaultConfig(), logger)
  ```

- [ ] **Replace custom audit calls**
  ```go
  auditStore.StoreAudit(ctx, event) // Non-blocking
  ```

- [ ] **Add graceful shutdown**
  ```go
  auditStore.Close() // Flushes remaining events
  ```

- [ ] **Define event types** (per service table above)

- [ ] **Add Prometheus metrics**
  - `audit_events_buffered_total`
  - `audit_events_dropped_total`
  - `audit_events_written_total`

- [ ] **Test integration**
  - Unit tests (buffering, batching, retry)
  - Integration tests (PostgreSQL roundtrip)

---

## 📊 **Success Metrics**

**Compliance Metrics**:
- ✅ 100% of P0 services generate audit traces
- ✅ 100% of business-critical operations audited
- ✅ 100% of external interactions audited

**Performance Metrics**:
- ✅ <1% audit event drop rate
- ✅ <5% audit batch failure rate
- ✅ <1ms latency impact on business operations

**Cost Metrics**:
- ✅ Storage cost <$1/month (345 MB/month)
- ✅ Zero audit traces for read-only operations

---

**Maintained By**: Kubernaut Architecture Team
**Last Updated**: November 8, 2025
**Review Cycle**: Annually or when new services are added

