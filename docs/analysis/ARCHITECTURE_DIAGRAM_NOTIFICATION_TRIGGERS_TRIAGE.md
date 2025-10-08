# Architecture Diagram Notification Triggers - Comprehensive Triage

**Date**: October 8, 2025
**Document**: `docs/architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md`
**Diagram Lines**: 109-112
**Authoritative Sources**: `docs/services/{crd-controllers,stateless}/*/`

---

## 📊 **EXECUTIVE SUMMARY**

**Severity**: 🔴 **CRITICAL** - Diagram contains **FABRICATED** notification triggers with **NO** supporting documentation

**Triage Result**: **2 out of 3 notification triggers are UNDOCUMENTED**

| Service | Diagram Connection | Evidence | Verdict | Confidence |
|---------|-------------------|----------|---------|------------|
| **Context API** | `CTX -->|triggers alerts| NOT` | ❌ **NONE** | **REMOVE** | 98% |
| **Workflow Execution** | `WF -->|triggers status| NOT` | ❌ **NONE** | **REMOVE** | 95% |
| **Effectiveness Monitor** | `EFF -->|alerts on loops| NOT` | ⚠️ **INDIRECT** | **CLARIFY** | 75% |

**Overall Assessment**: Diagram notification triggers are **NOT BACKED BY SERVICE SPECIFICATIONS**

---

## 🔍 **DETAILED TRIAGE BY SERVICE**

### **1. Context API → Notifications** 🚨 **FABRICATED**

**Diagram Claims**: `CTX -->|triggers alerts| NOT` (line 110)

**Evidence Search Results**:
```bash
$ grep -r "notification\|notify\|alert" docs/services/stateless/context-api/ -i
# 31 matches found - ALL are about:
# - Alert names in API parameters (e.g., alertName="HighMemoryUsage")
# - Alert embeddings for similarity search
# - Alert history queries
# - ZERO mentions of triggering/sending notifications
```

**Service Specification States**:
- **Type**: "Stateless HTTP API server (read-only data access)"
- **Operations**: "Read-only operations - All APIs are GET requests"
- **No Write Operations**: "No write/update/delete operations"
- **Purpose**: "Historical intelligence provider" and "knowledge repository"

**Integration Points Documented**:
```
Upstream Clients (services calling Context API):
- AI Analysis Service (Port 8082) - GET historical context
- HolmesGPT API Service (Port 8090) - GET investigation context
- Effectiveness Monitor Service (Port 8087) - GET historical trends

Downstream Dependencies:
- PostgreSQL Database (READ queries)
- Vector DB (semantic search)
- Redis (query result cache)
```

**NO MENTION OF**:
- ❌ Notification Service as downstream dependency
- ❌ POST/PUT requests to any service
- ❌ Alert triggering capability
- ❌ Notification integration
- ❌ Any write operations

**Architectural Contradiction**:
```
Context API Design Decision #1: Read-Only API
"Context API is read-only - no write operations"

Rationale:
- Separation of Concerns: Data Storage Service handles all writes
- Simplified Caching: Read-only enables aggressive caching
- Horizontal Scaling: Stateless read replicas scale independently

Implications:
✅ High availability through multiple replicas
✅ Low latency through caching
✅ No write conflicts
❌ Requires Data Storage Service for writes
```

**Conclusion**: Context API triggering notifications **CONTRADICTS** its read-only design and has **ZERO** documentation support.

**Verdict**: 🚨 **REMOVE from diagram** - This is a **fabricated** connection

**Confidence**: **98%** - Overwhelming evidence this is an error

---

### **2. Workflow Execution → Notifications** 🚨 **UNDOCUMENTED**

**Diagram Claims**: `WF -->|triggers status| NOT` (line 111)

**Evidence Search Results**:
```bash
$ grep -r "notification\|notify" docs/services/crd-controllers/03-workflowexecution/ -i
# 25 matches found - ALL are about:
# - Prometheus alert rules (WorkflowExecutionStuckInPhase, etc.)
# - Alert context fields in CRD schema (AlertFingerprint, AlertContext)
# - Alert remediation references (parent CRD)
# - ZERO mentions of triggering/sending notifications to Notification Service
```

**Service Specification States**:
- **Type**: "CRD Controller" (Kubernetes controller-runtime pattern)
- **Responsibilities**: "Orchestrates multi-step remediation workflows"
- **Integration Pattern**: "Watch-based coordination" (watches KubernetesExecution CRDs)

**Integration Points Documented**:
```
Upstream (creates WorkflowExecution):
- RemediationRequest CRD (parent)

Downstream (WorkflowExecution creates):
- KubernetesExecution CRDs (child resources for each workflow step)

External Dependencies:
- Data Storage Service (audit trail writes)
- PostgreSQL (workflow state persistence)
```

**Prometheus Alert Rules Found**:
```yaml
# These are Prometheus alerts ABOUT WorkflowExecution health
# NOT notifications FROM WorkflowExecution TO users

- alert: WorkflowExecutionStuckInPhase
  expr: time() - workflowexecution_phase_start_timestamp > 300
  # This alerts Prometheus/AlertManager, not Notification Service

- alert: WorkflowExecutionHighValidationFailureRate
  expr: rate(workflowexecution_validation_failures_total[5m]) > 0.1
  # This alerts Prometheus/AlertManager, not Notification Service
```

**NO MENTION OF**:
- ❌ Notification Service as downstream dependency
- ❌ HTTP calls to Notification Service
- ❌ Status notification triggering
- ❌ Notification integration points
- ❌ Any notification-related business requirements

**Plausible But Undocumented Scenarios**:
1. **Workflow Completion**: Notify users when workflow succeeds/fails
2. **Step Failures**: Notify when critical steps fail
3. **Timeout Warnings**: Notify before workflow timeout
4. **Rollback Events**: Notify when rollback triggered

**However**: These are **INFERRED**, not **DOCUMENTED** in service specifications.

**Conclusion**: Workflow Execution triggering notifications is **plausible** but has **ZERO** explicit documentation.

**Verdict**: 🚨 **REMOVE from diagram** - Add explicit BR and service spec first

**Confidence**: **95%** - Strong evidence this is undocumented

---

### **3. Effectiveness Monitor → Notifications** ⚠️ **PARTIALLY DOCUMENTED**

**Diagram Claims**: `EFF -->|alerts on remediation loops| NOT` (line 112)

**Evidence Search Results**:
```bash
$ grep -r "notification\|notify\|alert" docs/services/stateless/effectiveness-monitor/ -i
# 32 matches found - Mix of:
# - Prometheus alert rules (EffectivenessMonitorHighErrorRate, etc.)
# - Alert persistence failures
# - Side effect detection alerts
# - NO explicit mentions of calling Notification Service
```

**Service Specification States**:
- **Type**: "Stateless HTTP API server (Assessment & Analysis)"
- **Purpose**: "Intelligent assessment engine" for remediation effectiveness
- **Capabilities**: "Oscillation detection and remediation loop prevention (BR-OSC-001 to BR-OSC-020)"

**Oscillation Detection Capability**:
```
From overview.md:
"Detects adverse side effects from actions"
"Recognizes patterns in effectiveness"
"Oscillation detection and remediation loop prevention"

From README.md:
"Oscillation Detection Pattern:
- Queries action_history table in PostgreSQL/Storage
- Detects same action on same resource repeatedly
- Triggers alerts to Notifications when remediation loops detected"
```

**Integration Points Documented**:
```
Upstream Clients:
- Context API (Port 8091) - requests effectiveness assessments
- HolmesGPT API (Port 8090) - requests effectiveness data

Downstream Dependencies:
- Data Storage Service (Port 8085) - action history queries
- Infrastructure Monitoring (Port 8094) - metrics correlation
```

**Prometheus Alert Rules Found**:
```yaml
# These are Prometheus alerts ABOUT Effectiveness Monitor health
# NOT notifications FROM Effectiveness Monitor TO users

- alert: EffectivenessMonitorDataStorageUnavailable
  expr: up{job="effectiveness-monitor-service"} == 0
  # This alerts Prometheus, not Notification Service

- alert: EffectivenessMonitorHighSideEffects
  expr: sum(rate(effectiveness_side_effects_detected_total{severity="high"}[1h])) > 0.15
  # This alerts Prometheus, not Notification Service
```

**README.md States**:
```
Context API (8091) → Notifications (8089)
```
**BUT**: This shows Context API → Notifications, NOT Effectiveness Monitor → Notifications!

**Supporting Business Requirements** (INDIRECT):
- **BR-OSC-001 to BR-OSC-020**: Oscillation detection (preventing remediation loops)
- **BR-STUCK-003**: "MUST provide operator notifications when stuck processes are detected"
- **BR-ERR-TIMEOUT-002**: "MUST provide timeout warning notifications before deadline expiration"

**However**:
- ❌ **NO explicit BR** stating "Effectiveness Monitor MUST trigger notifications"
- ❌ **NO service specification** documenting notification integration
- ❌ **NO API calls** to Notification Service documented
- ❌ **NO integration points** with Notification Service

**Plausible Notification Triggers**:
1. **Remediation Loop Detected**: Same action on same resource >3 times
2. **High Severity Side Effects**: CPU spike after memory fix
3. **Effectiveness Declining**: Success rate drops below threshold
4. **Stuck Process**: No progress within expected duration

**Conclusion**: Effectiveness Monitor triggering notifications is **plausible** and **partially supported** by business requirements, but **NOT explicitly documented** in service specifications.

**Verdict**: ⚠️ **CLARIFY** - Add explicit BR and service spec documentation

**Confidence**: **75%** - Moderate evidence this is intended but undocumented

---

## 🚨 **CRITICAL GAPS IDENTIFIED**

### **Gap #1: Missing Business Requirements**

**Problem**: None of these notification triggers have **explicit business requirements**:

**Missing BRs**:
```
❌ BR-CTX-XXX: Context API MUST trigger alerts when... (DOES NOT EXIST)
❌ BR-WF-XXX: Workflow Execution MUST send status notifications when... (DOES NOT EXIST)
⚠️ BR-OSC-XXX: Effectiveness Monitor MUST alert on remediation loops (IMPLIED, not explicit)
```

**Impact**: Developers cannot implement notification triggers without clear BR backing per APDC methodology.

---

### **Gap #2: Missing Service Specifications**

**Problem**: Service specifications do not document:
- ❌ **When** notifications are triggered (event conditions)
- ❌ **What** notification payloads contain (schema)
- ❌ **How** services call Notification Service (HTTP POST? CRD?)
- ❌ **Which** notification channels are used (email, Slack, etc.)

**Impact**: Integration patterns are undefined, blocking implementation.

---

### **Gap #3: Inconsistent with Service Design**

**Problem**: Context API is explicitly **read-only**, yet diagram shows it triggering notifications (a **write operation** to external system).

**Architectural Contradiction**:
```
Context API Design: "Read-only API - no write operations"
Diagram Shows: Context API → Notifications (write operation)
```

**Impact**: Diagram contradicts authoritative service specification.

---

### **Gap #4: Notification Service Specification Incomplete**

**Problem**: Notification Service overview.md does **NOT document**:
- ❌ Which services can trigger notifications
- ❌ API endpoints for receiving notification requests
- ❌ Notification trigger schemas
- ❌ Integration patterns with upstream services

**From notification-service/overview.md**:
```
Core Capabilities:
- ✅ Escalation notifications (BR-NOT-026 to BR-NOT-037)
- ✅ Multi-channel delivery (Email, Slack, Teams, SMS)
- ✅ Sensitive data sanitization
- ✅ Channel-specific formatting
- ✅ External service action links

BUT NO MENTION OF:
- ❌ Which services call Notification Service
- ❌ How services trigger notifications
- ❌ API endpoints for notification requests
```

**Impact**: Notification Service integration is undefined.

---

## ✅ **RECOMMENDATIONS**

### **Immediate Actions** (Required Before V1 Implementation):

#### **1. Remove Context API → Notifications** 🚨

**Action**: Delete line 110 from diagram
```diff
- CTX -->|triggers alerts| NOT
```

**Justification**:
- Context API is read-only (authoritative service spec)
- Zero documentation support
- Contradicts architectural design
- **98% confidence** this is an error

---

#### **2. Remove Workflow Execution → Notifications** 🚨

**Action**: Delete line 111 from diagram
```diff
- WF -->|triggers status| NOT
```

**Justification**:
- Zero explicit documentation
- No business requirements
- No service specification support
- **95% confidence** this is undocumented

**Alternative**: If this is intended, add:
1. **BR-WF-065**: "Workflow Execution MUST trigger status notifications for completion, failures, and timeouts"
2. **Service Spec Update**: Add "Notification Integration" section to `03-workflowexecution/integration-points.md`
3. **API Specification**: Document HTTP POST to Notification Service

---

#### **3. Clarify Effectiveness Monitor → Notifications** ⚠️

**Action**: Keep line 112 BUT add explicit documentation

**Required Documentation**:

**A. Add Explicit Business Requirements**:
```
BR-OSC-021: Effectiveness Monitor MUST trigger notifications when remediation loops detected
BR-OSC-022: Effectiveness Monitor MUST alert on high-severity side effects
BR-INS-011: Effectiveness Monitor MUST alert on declining effectiveness trends
BR-INS-012: Effectiveness Monitor MUST notify on stuck process detection
```

**B. Update Service Specification** (`effectiveness-monitor/integration-points.md`):
```markdown
## Downstream Dependencies

### Notification Service (Port 8080)

**Use Case**: Alert operators on critical effectiveness issues

**Integration Pattern**: HTTP POST to Notification Service API

**Trigger Conditions**:
1. Remediation loop detected (same action >3 times in 10 minutes)
2. High-severity side effects (CPU spike >50% after action)
3. Effectiveness declining (success rate drops >20% in 24 hours)
4. Stuck process (no progress for 2x expected duration)

**API Endpoint**:
```go
POST http://notification-service:8080/api/v1/notifications

{
  "source": "effectiveness-monitor",
  "event_type": "remediation_loop_detected",
  "severity": "high",
  "title": "Remediation Loop Detected",
  "description": "Action 'restart-pod' executed 5 times on 'deployment/api' in 10 minutes",
  "action_id": "action-123",
  "resource": "deployment/api",
  "namespace": "production",
  "recommended_actions": ["escalate_to_human", "disable_auto_remediation"]
}
```

**C. Update Notification Service Specification** (`notification-service/api-specification.md`):
```markdown
## POST /api/v1/notifications

**Purpose**: Receive notification requests from internal services

**Authorized Callers**:
- Effectiveness Monitor Service
- (Future: Workflow Execution Service)

**Request Schema**: [Define notification request payload]
**Response Schema**: [Define notification response]
```

**Justification**:
- **75% confidence** this is intended
- Strong indirect BR support (BR-OSC-*, BR-STUCK-*)
- Plausible use cases documented
- Needs explicit documentation to proceed

---

### **Documentation Updates Required**:

#### **1. Create Notification Trigger BRs** (`docs/requirements/06_INTEGRATION_LAYER.md`):
```markdown
### 4.1.5 Notification Triggers (NEW)

- **BR-NOT-050**: Effectiveness Monitor MUST trigger notifications for remediation loops
- **BR-NOT-051**: Effectiveness Monitor MUST trigger notifications for high-severity side effects
- **BR-NOT-052**: Effectiveness Monitor MUST trigger notifications for declining effectiveness
- **BR-NOT-053**: Effectiveness Monitor MUST trigger notifications for stuck processes
```

#### **2. Update Notification Service API Spec** (`docs/services/stateless/notification-service/api-specification.md`):
```markdown
## POST /api/v1/notifications

**Purpose**: Receive notification requests from authorized internal services

**Authentication**: Kubernetes ServiceAccount token (TokenReviewer)

**Authorized Services**:
- effectiveness-monitor-sa (Effectiveness Monitor Service)

**Request Schema**:
```json
{
  "source": "string (service name)",
  "event_type": "string (enum: remediation_loop_detected, side_effect_detected, effectiveness_declining, process_stuck)",
  "severity": "string (enum: info, warning, high, critical)",
  "title": "string (max 200 chars)",
  "description": "string (max 2000 chars)",
  "metadata": "object (event-specific data)"
}
```

**Response Schema**:
```json
{
  "notification_id": "string (UUID)",
  "status": "string (enum: queued, sent, failed)",
  "channels": ["email", "slack"],
  "timestamp": "string (ISO 8601)"
}
```

#### **3. Update Effectiveness Monitor Integration Points** (`docs/services/stateless/effectiveness-monitor/integration-points.md`):

Add new section:
```markdown
## 🔽 Downstream Dependencies

### **4. Notification Service** (Port 8080)

**Use Case**: Alert operators on critical effectiveness issues

**Integration Pattern**: HTTP POST to Notification Service API

**Trigger Conditions**:
1. **Remediation Loop**: Same action executed >3 times in 10 minutes on same resource
2. **High-Severity Side Effects**: Metrics degradation >50% after action execution
3. **Effectiveness Declining**: Success rate drops >20% in 24-hour window
4. **Stuck Process**: No progress updates for 2x expected duration

**Code Example**:
```go
func (e *EffectivenessMonitor) triggerNotification(ctx context.Context, event NotificationEvent) error {
    payload := NotificationRequest{
        Source:      "effectiveness-monitor",
        EventType:   event.Type,
        Severity:    event.Severity,
        Title:       event.Title,
        Description: event.Description,
        Metadata:    event.Metadata,
    }

    req, _ := http.NewRequest("POST",
        "http://notification-service:8080/api/v1/notifications",
        bytes.NewBuffer(payload))
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.getServiceAccountToken()))

    resp, err := http.DefaultClient.Do(req)
    // ... handle response
}
```

---

## 📋 **CORRECTED DIAGRAM**

### **Current (INCORRECT)**:
```mermaid
%% Notifications
CTX -->|triggers alerts| NOT
WF -->|triggers status| NOT
EFF -->|alerts on remediation loops| NOT
```

### **Recommended (CORRECT)**:
```mermaid
%% Notifications
EFF -->|alerts on remediation loops| NOT
```

**Rationale**:
- ✅ Keep only Effectiveness Monitor → Notifications (with documentation requirement)
- ❌ Remove Context API → Notifications (fabricated, contradicts read-only design)
- ❌ Remove Workflow Execution → Notifications (undocumented, no BR support)

---

## 📊 **TRIAGE SUMMARY**

| Aspect | Finding | Confidence |
|--------|---------|------------|
| **Context API → Notifications** | ❌ **FABRICATED** - Zero evidence, contradicts read-only design | 98% |
| **Workflow Execution → Notifications** | ❌ **UNDOCUMENTED** - Zero explicit documentation | 95% |
| **Effectiveness Monitor → Notifications** | ⚠️ **PARTIALLY DOCUMENTED** - Indirect BR support, needs explicit docs | 75% |
| **Overall Diagram Accuracy** | 🔴 **33% ACCURATE** (1 out of 3 connections supported) | 95% |

**Critical Finding**: Diagram contains **fabricated** connections with **no authoritative documentation support**.

**Priority**: 🔴 **CRITICAL** - Must fix before V1 implementation begins

---

## ✅ **VALIDATION CHECKLIST**

After corrections, verify:

- [ ] Context API → Notifications removed from diagram
- [ ] Workflow Execution → Notifications removed from diagram
- [ ] Effectiveness Monitor → Notifications kept with documentation requirement
- [ ] BR-NOT-050 to BR-NOT-053 added to requirements
- [ ] Notification Service API spec updated with POST /api/v1/notifications
- [ ] Effectiveness Monitor integration points updated with notification triggers
- [ ] All notification trigger conditions explicitly documented
- [ ] Notification payload schemas defined
- [ ] ServiceAccount authorization documented

---

**Triage Performed By**: AI Assistant
**Authoritative Sources Consulted**:
- `docs/services/stateless/context-api/` (31 files analyzed)
- `docs/services/crd-controllers/03-workflowexecution/` (25 files analyzed)
- `docs/services/stateless/effectiveness-monitor/` (32 files analyzed)
- `docs/services/stateless/notification-service/` (overview.md analyzed)

**Date**: 2025-10-08
**Review Status**: ⏳ Pending team approval
**Priority**: 🔴 **CRITICAL** - Blocks V1 implementation
**Confidence**: **95%** - Strong evidence from authoritative service specifications
