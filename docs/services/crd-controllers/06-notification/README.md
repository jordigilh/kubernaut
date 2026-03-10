# Notification Controller

**Version**: v1.6.0
**Status**: ✅ Production-Ready (358 tests, 100% pass rate, 18/18 BRs Complete)
**Health/Ready Port**: 8081 (`/healthz`, `/readyz` - no auth required)
**Metrics Port**: 9186 (`/metrics` - with auth filter, DD-TEST-001 compliant)
**CRD**: NotificationRequest
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Controller**: NotificationRequestReconciler
**Priority**: **P0 - CRITICAL** (Essential for remediation feedback)
**Effort**: 2 weeks (complete)

---

## 🗂️ Documentation Index

| Document | Purpose | Lines | Status |
|----------|---------|-------|--------|
| **[Overview](./overview.md)** | Service purpose, architecture, key features | ~298 | ✅ Complete |
| **[API Specification](./api-specification.md)** | NotificationRequest CRD types, validation, examples | ~571 | ✅ Complete |
| **[Controller Implementation](./controller-implementation.md)** | Reconciler logic, phase handling, delivery orchestration | ~594 | ✅ Complete |
| **[Audit Trace Specification](./audit-trace-specification.md)** | ADR-034 unified audit table integration | ~500 | ✅ Complete |
| **[Testing Strategy](./testing-strategy.md)** | Unit/Integration/E2E tests, defense-in-depth patterns | ~1,425 | ✅ Complete |
| **[Security Configuration](./security-configuration.md)** | RBAC, data sanitization, container security | ~852 | ✅ Complete |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, correlation IDs, metrics | ~541 | ✅ Complete |
| **[Database Integration](./database-integration.md)** | ADR-034 audit storage, fire-and-forget pattern | ~606 | ✅ Complete |
| **[Integration Points](./integration-points.md)** | RemediationOrchestrator coordination, external channels | ~549 | ✅ Complete |
| **[Implementation Checklist](./implementation-checklist.md)** | APDC-TDD phases, tasks, validation steps | ~339 | ✅ Complete |
| **[Business Requirements](./BUSINESS_REQUIREMENTS.md)** | 17 BRs with acceptance criteria and test mapping | ~691 | ✅ Complete |

**Total**: ~6,913 lines across 11 core specification documents
**Status**: ✅ **100% Complete** - Production-ready with comprehensive documentation

**Additional Documentation**:
- **[Production Runbooks](./runbooks/)** - HIGH_FAILURE_RATE.md, STUCK_NOTIFICATIONS.md
- **[Testing Documentation](./testing/)** - BR-COVERAGE-MATRIX.md, TEST-EXECUTION-SUMMARY.md
- **[Implementation Guides](./implementation/)** - Phase-by-phase implementation plans

---

## 📁 File Organization

```
06-notification/
├── 📄 README.md (you are here)              - Service index & navigation
├── 📘 overview.md                           - High-level architecture ✅ (298 lines)
├── 🔧 api-specification.md                  - CRD type definitions ✅ (571 lines)
├── ⚙️  controller-implementation.md         - Reconciler logic ✅ (594 lines)
├── 📝 audit-trace-specification.md          - ADR-034 audit integration ✅ (500 lines)
├── 🧪 testing-strategy.md                   - Test patterns ✅ (1,425 lines)
├── 🔒 security-configuration.md             - Security & sanitization ✅ (852 lines)
├── 📊 observability-logging.md              - Logging & metrics ✅ (541 lines)
├── 💾 database-integration.md               - Audit storage ✅ (606 lines)
├── 🔗 integration-points.md                 - Service coordination ✅ (549 lines)
├── ✅ implementation-checklist.md           - APDC-TDD phases ✅ (339 lines)
├── 📋 BUSINESS_REQUIREMENTS.md              - 17 BRs with test mapping ✅ (691 lines)
├── 📚 runbooks/                             - Production operational guides
│   ├── HIGH_FAILURE_RATE.md                - Failure rate >10% runbook
│   └── STUCK_NOTIFICATIONS.md              - Stuck notifications >10min runbook
├── 🧪 testing/                              - Test documentation
│   ├── BR-COVERAGE-MATRIX.md               - BR-to-test traceability
│   └── TEST-EXECUTION-SUMMARY.md           - Test execution guide
└── 📁 implementation/                       - Implementation phase guides
    ├── IMPLEMENTATION_PLAN_V1.0.md         - Original implementation plan
    ├── IMPLEMENTATION_PLAN_V3.0.md         - ADR-034 audit integration
    └── design/                              - Design documents
        └── ERROR_HANDLING_PHILOSOPHY.md    - Retry & circuit breaker design
```

**Legend**:
- ✅ = Complete documentation
- 📋 = Core specification document
- 🧪 = Test-related documentation
- 📚 = Operational documentation

---

## 🏗️ Implementation Structure

### **Binary Location**
- **Directory**: `cmd/notification/`
- **Entry Point**: `cmd/notification/main.go`
- **Build Command**: `go build -o bin/notification-controller ./cmd/notification`

### **Controller Location**
- **Controller**: `internal/controller/notification/notificationrequest_controller.go`
- **CRD Types**: `api/notification/v1alpha1/notificationrequest_types.go`

### **Business Logic**
- **Package**: `pkg/notification/`
  - `delivery/` - Channel-specific delivery implementations (console, slack, file)
  - `status/` - CRD status management
  - `sanitization/` - Secret pattern redaction (22 patterns)
  - `retry/` - Exponential backoff & circuit breakers
  - `metrics/` - Prometheus metrics
- **Tests**:
  - `test/unit/notification/` - 225 unit tests
  - `test/integration/notification/` - 112 integration tests
  - `test/e2e/notification/` - 21 E2E tests (Kind-based, 100% pass rate, OpenAPI audit client integration)

**See Also**: [cmd/ directory structure](../../../../cmd/README.md) for complete binary organization.

---

## 🚀 Quick Start

### **For New Developers**
1. **Understand the Service**: Start with [Overview](./overview.md) (5 min read)
2. **Review the CRD**: See [API Specification](./api-specification.md) (10 min read)
3. **Understand Delivery Flow**: Read controller phase transitions (5 min read)

### **For Implementers**
1. **Check Integration**: Start with [Integration Points](./integration-points.md)
2. **Follow Checklist**: Use [Implementation Checklist](./implementation-checklist.md)
3. **Review Patterns**: Reference [Controller Implementation](./controller-implementation.md)

### **For Operators**
1. **Deployment**: See [Production Deployment Guide](./PRODUCTION_DEPLOYMENT_GUIDE.md)
2. **Troubleshooting**: Check [Production Runbooks](./runbooks/)
3. **Monitoring**: Review [Observability & Logging](./observability-logging.md)

---

## 📋 Prerequisites

### Required

| Dependency | Version | Purpose |
|------------|---------|---------|
| **Kubernetes** | 1.27+ | CRD and controller-runtime support |
| **kubectl** | Latest | CRD management and debugging |
| **Data Storage Service** | v1.0+ | ADR-034 unified audit table |

### Optional

| Dependency | Purpose |
|------------|---------|
| **Slack Webhook** | Slack channel delivery (v1.0) |
| **Email SMTP Server** | Email delivery (v2.0) |
| **PagerDuty API Key** | PagerDuty integration (v2.0) |

### Deployment Namespace

**Default Namespace**: `kubernaut-notifications` (where controller and NotificationRequest CRDs are deployed)

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-notifications
```

---

## 📋 Overview

The Notification Controller is a Kubernetes CRD controller that delivers notifications to multiple channels (console, Slack, email) with guaranteed delivery, complete audit trails, and graceful degradation.

### **Key Features**

- ✅ **Zero Data Loss**: CRD-based persistence to etcd before delivery
- ✅ **Complete Audit Trail**: Every delivery attempt recorded in CRD status + ADR-034 unified audit table
- ✅ **Fire-and-Forget Audit**: <1ms audit overhead, zero impact on notification delivery
- ✅ **Automatic Retry**: Exponential backoff (30s → 480s, max 5 attempts)
- ✅ **At-Least-Once Delivery**: Kubernetes reconciliation loop guarantees
- ✅ **Graceful Degradation**: Per-channel circuit breakers + audit DLQ fallback
- ✅ **Data Sanitization**: 22 secret patterns redacted automatically
- ✅ **Observability**: 10 Prometheus metrics + structured logging + audit correlation
- ✅ **Security**: Non-root user, minimum RBAC permissions

### **Supported Channels**

| Channel | Status | Priority | Use Case |
|---------|--------|----------|----------|
| **Console** | ✅ V1.0 | P0 | Development, debugging, audit trail |
| **Slack** | ✅ V1.0 | P0 | Team notifications, workflow updates |
| **Email** | ⏸️ V2.0 | P1 | Formal notifications, external stakeholders |
| **Teams** | ⏸️ V2.0 | P1 | Microsoft-centric organizations |
| **SMS** | ⏸️ V2.0 | P2 | Critical on-call alerts |
| **Webhook** | ⏸️ V2.0 | P2 | Custom integrations |

---

## 🏗️ Architecture

### **CRD-Based Declarative Approach**

```
┌─────────────────────────────────────────────────────────────────┐
│                    NotificationRequest CRD                       │
│  (Stored in etcd - Zero data loss guarantee)                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│              Notification Controller (Reconciler)                │
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐     │
│  │ Status       │    │   Delivery   │    │    Retry     │     │
│  │ Manager      │───▶│   Services   │◀───│    Policy    │     │
│  └──────────────┘    └──────────────┘    └──────────────┘     │
│         │                    │                    │             │
│         │                    ▼                    │             │
│         │         ┌──────────────────┐           │             │
│         │         │   Sanitizer      │           │             │
│         │         └──────────────────┘           │             │
│         │                    │                    │             │
│         ▼                    ▼                    ▼             │
│  ┌──────────────────────────────────────────────────────┐     │
│  │              CRD Status (Audit Trail)                 │     │
│  │   - Phase: Pending → Sending → Sent/Failed           │     │
│  │   - DeliveryAttempts: [console, slack]               │     │
│  │   - CompletionTime, Reason, Message                  │     │
│  └──────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
      ┌───────────────────────┴───────────────────────┐
      │                                                │
      ▼                                                ▼
┌──────────────┐                              ┌──────────────┐
│   Console    │                              │    Slack     │
│   Delivery   │                              │   Delivery   │
│   Service    │                              │   Service    │
└──────────────┘                              └──────────────┘
      │                                                │
      ▼                                                ▼
┌──────────────┐                              ┌──────────────┐
│   Console    │                              │ Slack API    │
│    Output    │                              │  (Webhook)   │
└──────────────┘                              └──────────────┘
```

### **Component Responsibilities**

| Component | Responsibility | File |
|-----------|---------------|------|
| **NotificationRequest CRD** | Persistent storage of notification data | `api/notification/v1alpha1/notificationrequest_types.go` |
| **Reconciler** | Main controller logic, orchestrates delivery | `internal/controller/notification/notificationrequest_controller.go` |
| **Status Manager** | Updates CRD status, records attempts | `pkg/notification/status/manager.go` |
| **Console Delivery** | Logs to console output | `pkg/notification/delivery/console.go` |
| **Slack Delivery** | Sends to Slack webhook | `pkg/notification/delivery/slack.go` |
| **File Delivery** | E2E test validation (HostPath volumes) | `pkg/notification/delivery/file.go` |
| **Sanitizer** | Redacts 22+ secret patterns from content (DD-005) | `pkg/shared/sanitization/` |
| **Retry Policy** | Exponential backoff + circuit breaker | `pkg/notification/retry/policy.go` |
| **Metrics** | 10 Prometheus metrics | `pkg/notification/metrics/metrics.go` |
| **Audit Helpers** | ADR-034 audit event creation | `internal/controller/notification/audit.go` |

---

## 🚀 Quick Start

### **Installation**

```bash
# 1. Install NotificationRequest CRD
kubectl apply -f config/crd/bases/kubernaut.ai_notificationrequests.yaml

# 2. Deploy notification controller
kubectl apply -k deploy/notification/

# 3. (Optional) Create Slack webhook secret
kubectl create secret generic notification-slack-webhook \
  --from-literal=webhook-url="https://hooks.slack.com/services/YOUR/WEBHOOK/URL" \
  -n kubernaut-system

# 4. Verify controller is running
kubectl get pods -n kubernaut-system
kubectl logs -f deployment/notification-controller -n kubernaut-system
```

### **Creating a Notification**

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: example-notification
  namespace: kubernaut-system
spec:
  subject: "Production Alert: High Memory Usage"
  body: "Service api-server has exceeded 90% memory threshold"
  type: escalation
  priority: high
  channels:
    - console
    - slack
```

```bash
kubectl apply -f notification.yaml
kubectl get notificationrequest example-notification -n kubernaut-system -o yaml
```

**Expected Output**:
```yaml
status:
  phase: Sent
  deliveryAttempts:
    - channel: console
      timestamp: "2025-12-02T10:30:00Z"
      status: success
    - channel: slack
      timestamp: "2025-12-02T10:30:01Z"
      status: success
  completionTime: "2025-12-02T10:30:01Z"
```

---

## 📊 Business Requirements Compliance

| BR | Description | Implementation | Coverage |
|----|-------------|---------------|----------|
| **BR-NOT-050** | Data Loss Prevention | CRD persistence to etcd | 90% |
| **BR-NOT-051** | Complete Audit Trail | DeliveryAttempts array + ADR-034 unified audit | 100% |
| **BR-NOT-052** | Automatic Retry | Exponential backoff (5 attempts) | 95% |
| **BR-NOT-053** | At-Least-Once Delivery | Kubernetes reconciliation | 85% |
| **BR-NOT-054** | Observability | 10 Prometheus metrics + audit correlation | 95% |
| **BR-NOT-055** | Graceful Degradation | Per-channel circuit breakers + audit DLQ | 100% |
| **BR-NOT-056** | CRD Lifecycle | Phase state machine | 95% |
| **BR-NOT-057** | Priority Handling | All priorities processed | 95% |
| **BR-NOT-058** | TLS Security | Valid certificates only (permanent failures) | 100% |
| **BR-NOT-062** | Unified Audit Table | ADR-034 unified `audit_events` table | 100% |
| **BR-NOT-063** | Audit Graceful Degradation | Fire-and-forget + DLQ fallback | 100% |
| **BR-NOT-064** | Event Correlation | Remediation ID propagation | 100% |
| **BR-NOT-065** | Channel Routing | Spec-field-based routing (DD-WE-004, Issue #91) | 100% |
| **BR-NOT-066** | Alertmanager Config | Alertmanager-compatible routing format | 100% |
| **BR-NOT-067** | Config Hot-Reload | ConfigMap hot-reload without restart | 100% |
| **BR-NOT-068** | Multi-Channel Fanout | Single notification to multiple channels | 100% |
| **BR-NOT-069** | Routing Rule Visibility | `RoutingResolved` + `Ready` conditions | 100% |

**Overall BR Coverage**: **96.9%** ✅ (17/17 BRs, avg 96.9%)

See [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) for detailed BR specifications.

---

## 🔧 Configuration

### **Environment Variables**

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `SLACK_WEBHOOK_URL` | Slack webhook URL | No | None (disables Slack) |
| `E2E_FILE_OUTPUT` | File output directory for E2E tests | No | None (disables file delivery) |

**Note**: Slack webhook URL should be provided via Kubernetes Secret for production deployments.

### **Controller Arguments**

| Argument | Description | Default |
|----------|-------------|---------|
| `--leader-elect` | Enable leader election | `false` |
| `--metrics-bind-address` | Metrics server address | `:9186` |
| `--health-probe-bind-address` | Health probe address | `:8081` |

**Port Allocation** (per DD-TEST-001):
- **Health/Ready**: 8081 (no conflicts)
- **Metrics**: 9186 (NodePort 30186 in E2E tests)

---

## 📈 Observability

### **Prometheus Metrics**

The controller exposes 10 comprehensive Prometheus metrics on `:9186/metrics`:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `notification_requests_total` | Counter | type, priority, phase | Total notification requests by phase |
| `notification_delivery_attempts_total` | Counter | channel, status | Total delivery attempts by channel |
| `notification_delivery_duration_seconds` | Histogram | channel | Delivery duration by channel |
| `notification_retry_count` | Counter | channel, reason | Retry count by channel and reason |
| `notification_circuit_breaker_state` | Gauge | channel | Circuit breaker state (0=closed, 1=open, 2=half-open) |
| `notification_reconciliation_duration_seconds` | Histogram | - | Reconciliation duration |
| `notification_reconciliation_errors_total` | Counter | error_type | Reconciliation errors by type |
| `notification_active_notifications` | Gauge | phase | Active notifications by phase |
| `notification_channel_health_score` | Gauge | channel | Channel health score (0-100) |

### **Health Probes**

- **Liveness**: `GET /healthz` (port 8081)
- **Readiness**: `GET /readyz` (port 8081)

### **Structured Logging**

All logs are structured JSON with correlation fields:
- `timestamp`: ISO 8601 timestamp
- `level`: log level (info, warn, error)
- `msg`: log message
- `notification`: NotificationRequest name
- `phase`: current phase
- `channel`: delivery channel (if applicable)
- `correlation_id`: Remediation ID for audit trail

See [Observability & Logging](./observability-logging.md) for complete logging standards.

---

## 🔒 Security

### **RBAC Permissions**

The controller requires **minimum permissions**:

| Resource | Verbs | Rationale |
|----------|-------|-----------|
| `notificationrequests` | get, list, watch, update, patch | Read and update CRDs |
| `notificationrequests/status` | get, update, patch | Update CRD status |
| `secrets` | get, list, watch | Read Slack webhook URL (read-only) |
| `events` | create, patch | Record Kubernetes events |

**No administrative permissions required.**

### **Container Security**

- **Non-root user**: Runs as UID 65532
- **No privilege escalation**: `allowPrivilegeEscalation: false`
- **Seccomp profile**: `RuntimeDefault`
- **Dropped capabilities**: ALL
- **Read-only root filesystem**: `readOnlyRootFilesystem: true`

### **Data Sanitization**

The controller automatically redacts **22 secret patterns** from notification content:
- Kubernetes Secrets
- AWS credentials (access keys, secret keys)
- GCP credentials (service account JSON)
- Azure credentials (connection strings)
- Database passwords (PostgreSQL, MySQL, MongoDB)
- API keys (generic, provider-specific)
- OAuth tokens (Bearer, GitHub, GitLab)
- Private keys (SSH, TLS certificates)
- JWT tokens
- And more...

See [Security Configuration](./security-configuration.md) for complete sanitization pattern list.

---

## 🧪 Testing

### **Test Coverage** (v1.4.0)

| Test Type | Files | Coverage | Status |
|-----------|-------|----------|--------|
| **Unit Tests** | 12 | 70%+ code coverage | ✅ 100% passing |
| **Integration Tests** | 18 | >50% BR coverage | ✅ 100% passing |
| **E2E Tests** | 4 | <10% BR coverage | ✅ 100% passing |
| **TOTAL** | **35 files** | **96.9% BR coverage** | **✅ Production-Ready** |

### **Test Execution**

```bash
# Unit tests
make test-unit-notification
go test ./test/unit/notification/... -v

# Integration tests
make test-integration-notification
go test ./test/integration/notification/... -v

# E2E tests (requires Kind cluster)
make test-e2e-notification
go test ./test/e2e/notification/... -v

# All notification tests
make test-notification-all

# With coverage
go test ./test/unit/notification/... -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### **E2E Testing Infrastructure** (DD-NOT-002 v3.0)

**Kind-Based E2E Tests**:
- ✅ Real Kubernetes cluster (Kind)
- ✅ HostPath volumes for file delivery validation
- ✅ NodePort metrics exposure (30186 → localhost:9186)
- ✅ Parallel test execution (4 concurrent processes)
- ✅ Zero flaky tests (process-isolated temp directories)

See [Testing Strategy](./testing-strategy.md) for complete test patterns and anti-patterns.

---

## 🔄 Retry and Error Handling

### **Exponential Backoff**

Failed deliveries are automatically retried with exponential backoff:

| Attempt | Delay | Total Elapsed |
|---------|-------|---------------|
| 1 | 0s | 0s |
| 2 | 30s | 30s |
| 3 | 60s | 90s |
| 4 | 120s | 210s |
| 5 | 240s | 450s |
| Max | 480s | 930s (15.5 min) |

**Max Attempts**: 5 per channel

### **Error Classification**

| Error Type | Example | Retryable | Action |
|------------|---------|-----------|--------|
| **Transient** | 503 Service Unavailable | ✅ Yes | Retry with backoff |
| **Transient** | Network timeout | ✅ Yes | Retry with backoff |
| **Transient** | 429 Rate Limit | ✅ Yes | Retry with longer backoff |
| **Permanent** | 401 Unauthorized | ❌ No | Mark as failed |
| **Permanent** | 404 Not Found | ❌ No | Mark as failed |
| **Permanent** | Invalid webhook URL | ❌ No | Mark as failed |
| **Permanent** | TLS certificate error | ❌ No | Mark as failed (BR-NOT-058) |

### **Circuit Breaker**

Per-channel circuit breakers prevent cascading failures:

- **Closed**: Normal operation
- **Open**: 5+ failures → stop requests for 60s
- **Half-Open**: Allow 1 test request → Close if success, Open if failure

**Isolation**: Console delivery continues even if Slack circuit breaker is open (BR-NOT-055).

See [Controller Implementation](./controller-implementation.md) for retry policy details.

---

## 🛠️ Troubleshooting

### **Production Runbooks**

For operational issues, refer to comprehensive production runbooks:

- **[HIGH_FAILURE_RATE.md](./runbooks/HIGH_FAILURE_RATE.md)** - Notification failure rate >10%
  - **Trigger**: `(failed_deliveries / total_deliveries) > 0.10` for 5 minutes
  - **Symptoms**: Operators not receiving notifications, workflow updates missing
  - **Common causes**: Invalid Slack webhook (60%), rate limiting (30%), network issues (10%)
  - **MTTR Target**: 15 minutes

- **[STUCK_NOTIFICATIONS.md](./runbooks/STUCK_NOTIFICATIONS.md)** - Notifications stuck >10 minutes
  - **Trigger**: P99 delivery latency >600 seconds
  - **Symptoms**: Delayed notifications, stuck in "Sending" phase
  - **Common causes**: Slack API slow (50%), reconciliation blocked (30%), status conflicts (15%)
  - **MTTR Target**: 20 minutes

Both runbooks include:
- ✅ Prometheus alert definitions (ready to deploy)
- ✅ Diagnostic queries (kubectl + PromQL)
- ✅ Root cause analysis decision trees
- ✅ Step-by-step remediation procedures
- ✅ Automation strategies (auto-remediation, escalation)
- ✅ Success criteria and validation steps

### **Common Issues**

#### **Controller Not Starting**

```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app=notification-controller

# Check controller logs
kubectl logs -f deployment/notification-controller -n kubernaut-system

# Check RBAC permissions
kubectl auth can-i get notificationrequests --as=system:serviceaccount:kubernaut-system:notification-controller
```

#### **Notifications Not Delivered**

```bash
# Check NotificationRequest status
kubectl get notificationrequest <name> -n kubernaut-system -o yaml

# Check delivery attempts
kubectl get notificationrequest <name> -n kubernaut-system -o jsonpath='{.status.deliveryAttempts}'

# Check controller logs for errors
kubectl logs -f deployment/notification-controller -n kubernaut-system | grep ERROR
```

#### **Slack Delivery Failing**

```bash
# Verify Slack webhook secret exists
kubectl get secret notification-slack-webhook -n kubernaut-system

# Test webhook URL manually
curl -X POST -H 'Content-Type: application/json' \
  -d '{"text":"Test message"}' \
  https://hooks.slack.com/services/YOUR/WEBHOOK/URL

# Check circuit breaker state
kubectl logs -f deployment/notification-controller -n kubernaut-system | grep "circuit breaker"
```

---

## 🤝 Integration

### **RemediationOrchestrator Integration**

The `RemediationOrchestrator` is the **primary consumer** of NotificationRequest CRDs (per ADR-017).

**Trigger Events**:
- Workflow execution failed (manual review required)
- Workflow skipped (ResourceBusy, PreviousExecutionFailed)
- Remediation completed (success/failure)
- Approval required (human-in-the-loop)

**Integration Example**:
```go
// In RemediationOrchestrator reconciler
notification := &notificationv1alpha1.NotificationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("remediation-%s", remediation.Name),
        Namespace: "kubernaut-system",
        OwnerReferences: []metav1.OwnerReference{
            {
                APIVersion: remediation.APIVersion,
                Kind:       remediation.Kind,
                Name:       remediation.Name,
                UID:        remediation.UID,
                Controller: pointer.Bool(true),
            },
        },
    },
    Spec: notificationv1alpha1.NotificationRequestSpec{
        Subject:  fmt.Sprintf("Remediation %s", remediation.Status.Phase),
        Body:     buildNotificationBody(remediation),
        Type:     notificationv1alpha1.NotificationTypeEscalation,
        Priority: determinePriority(remediation.Spec.Severity),
        Channels: []notificationv1alpha1.Channel{
            notificationv1alpha1.ChannelConsole,
            notificationv1alpha1.ChannelSlack,
        },
    },
}
err := r.Create(ctx, notification)
```

**Integration Documentation**:
- [Integration Points](./integration-points.md) - Complete integration patterns
- [RO Notification Integration Plan](../05-remediationorchestrator/NOTIFICATION_INTEGRATION_PLAN.md) - RO-specific integration
- [ADR-017: NotificationRequest CRD Creator](../../../architecture/decisions/ADR-017-notification-crd-creator.md) - Responsibility definition

**Cross-Team Q&A**:
- WE→Notification Questions - 5 questions answered (internal development reference, removed in v1.0)
- [DD-RO-001: Notification Cancellation Handling](../05-remediationorchestrator/DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md) - Deletion behavior

---

## 📊 Performance

### **Resource Usage**

| Resource | Request | Limit |
|----------|---------|-------|
| **CPU** | 100m | 200m |
| **Memory** | 64Mi | 128Mi |

### **Throughput**

- **Notification Processing**: ~100 notifications/second (theoretical, CRD reconciliation limited)
- **Actual Throughput**: Limited by Slack webhook rate limits (~1/second)
- **Reconciliation Interval**: Every 10 seconds (Kubernetes default)

### **Latency**

| Operation | Typical Latency | Max Latency |
|-----------|----------------|-------------|
| **Console Delivery** | <100ms | <500ms |
| **Slack Delivery** | ~1-3s | ~10s |
| **Audit Write** | <1ms | <5ms (fire-and-forget) |
| **Status Update** | <500ms | <2s |

**Audit Performance** (BR-NOT-063):
- Fire-and-forget pattern: <1ms overhead
- Zero impact on notification delivery latency
- DLQ fallback: 100% audit event capture

---

## 📝 Version History

### **Version 1.5.0** (2025-12-11) - **PENDING IMPLEMENTATION**
- 📋 **BR-NOT-069**: Routing Rule Visibility via Kubernetes Conditions
  - **Status**: ✅ Approved for Kubernaut V1.0 (December 2025)
  - **Description**: Expose routing rule resolution via `RoutingResolved` condition in CRD status
  - **Effort**: 3 hours implementation time
  - **Value**: kubectl-based routing diagnostics without log access
  - **Related**: BR-NOT-065 (Routing Rules), BR-NOT-066 (Config Format)
- ✅ **API Specification**: Updated to v2.3 with NotificationRequest CRD types and Conditions documentation
- 📄 **Business Requirements**: Updated to 18 BRs total (17 implemented, 1 approved for V1.0)
- 🔗 **Handoff Response**: Created RESPONSE_NO_CONDITIONS_IMPLEMENTATION.md with implementation plan

### **Version 1.4.0** (2025-12-06)
- ✅ **Skip-Reason Label Routing**: Added `kubernaut.ai/skip-reason` routing label (DD-WE-004 integration)
- ✅ **Label Constants**: Implemented `LabelSkipReason` and skip reason value constants
- ✅ **API Specification**: Updated to v2.1 with routing labels section
- ✅ **Day 13 Enhancement**: Complete implementation plan for skip-reason routing tests, config, and runbooks
- ✅ **Cross-Team**: Acknowledged DD-WE-004 v1.4 with implementation details
- 📋 **Scheduled**: Day 13 - Skip-reason routing tests, example config, runbook

### **Version 1.3.0** (2025-12-02)
- ✅ **Documentation Standardization**: README restructured to match service template (RO, WE pattern)
- ✅ **Documentation Index**: Added comprehensive doc navigation with line counts
- ✅ **File Organization**: Visual tree with all core specification docs
- ✅ **Implementation Structure**: Added binary/controller/pkg location guide
- ✅ **Enhanced Quick Start**: Separated guidance for developers/implementers/operators
- ✅ **Cross-Team Q&A**: Completed 5 WorkflowExecution team questions with integration examples

### **Version 1.2.0** (2025-12-01)
- ✅ **Comprehensive Test Implementation**: 249 tests (140 unit + 97 integration + 12 E2E) - 100% pass rate
- ✅ **E2E Kind-Based Testing**: 12 E2E tests with Kind cluster (file delivery + metrics validation)
- ✅ **DD-TEST-001 Compliance**: Notification metrics on dedicated ports (30186/9186) to prevent collisions
- ✅ **Production-Ready Validation**: All 3 test tiers passing, stable in CI/CD, zero flaky tests
- ✅ **HostPath File Delivery**: Clean E2E file validation without production image pollution
- ✅ **Parallel Test Execution**: 4 concurrent processes for race condition detection
- 📊 **Test Coverage**: Unit 70%+, Integration >50%, E2E <10% (defense-in-depth pyramid)

### **Version 1.1.0** (2025-11-21)
- ✅ **ADR-034 Unified Audit Table Integration**: All notification events written to unified `audit_events` table
- ✅ **Fire-and-Forget Audit Writes**: <1ms audit overhead, zero impact on delivery performance
- ✅ **Zero Audit Loss**: DLQ fallback with Redis Streams ensures no audit data lost
- ✅ **End-to-End Correlation**: Query complete workflow trail via `correlation_id`
- ✅ **Enhanced Testing**: Expanded integration test coverage for audit scenarios
- ✅ **BR-NOT-062, BR-NOT-063, BR-NOT-064 Complete**: Unified audit table + graceful degradation + correlation

### **Version 1.0.1** (2025-10-20)
- ✅ **Enhanced 5-category error handling** with exponential backoff
- ✅ **Added EventuallyWithRetry anti-flaky patterns** to integration tests
- ✅ **Created 4 edge case test categories** (rate limiting, config changes, large payloads, concurrent delivery)
- ✅ **Documented 2 production runbooks** with Prometheus automation
- 📊 **Expected improvements**: >99% notification success rate, <1% test flakiness, -50% delivery MTTR

### **Version 1.0.0** (2025-10-12)
- Initial production-ready release
- 9/9 Business Requirements implemented
- 85 unit test scenarios, 5 integration test scenarios
- Complete CRD controller implementation
- Security hardening and observability

---

## 🔮 Future Enhancements

### **V1.x Enhancements (In Progress)**

1. **Spec-Field-Based Routing** (BR-NOT-065, BR-NOT-066) - ✅ **COMPLETE**:
   - Alertmanager-compatible routing configuration
   - Spec-field-based channel selection (`spec.type`, `spec.severity`, `spec.metadata["skip-reason"]`)
   - ConfigMap hot-reload for routing rules
   - Multi-channel fanout

2. **Skip-Reason Routing** (DD-WE-004) - ✅ **COMPLETE**:
   - `spec.metadata["skip-reason"]` attribute routing
   - `PreviousExecutionFailed` → PagerDuty (CRITICAL)
   - `ExhaustedRetries` → Slack #ops (HIGH)
   - `ResourceBusy`/`RecentlyRemediated` → Console (LOW)

### **V2.0 Planned Features**

1. **Additional Channels** (BR-NOT-067, BR-NOT-068):
   - Email (SMTP) with HTML templates
   - PagerDuty incidents
   - Microsoft Teams with Adaptive Cards
   - SMS via Twilio/SNS
   - Custom webhooks
   - Note: BR-NOT-069 moved to V1.0 (Kubernetes Conditions)

3. **Performance Optimization**:
   - Batch notification delivery
   - Connection pooling for external services
   - Caching for frequently accessed secrets

4. **Enhanced Observability**:
   - Grafana dashboards (pre-built)
   - Alert rules for SLO violations
   - Distributed tracing with OpenTelemetry

---

## 📞 Support

### **Getting Help**

- **Documentation**: Start with [Overview](./overview.md) for architecture understanding
- **Issues**: Check [Troubleshooting](#-troubleshooting) section and production runbooks
- **Testing**: See [Testing Strategy](./testing-strategy.md) for test patterns
- **Integration**: Review [Integration Points](./integration-points.md) for service coordination

### **Contributing**

This controller follows strict **APDC-Enhanced TDD** methodology:

1. **Analysis**: Understand business context and existing patterns
2. **Plan**: Design implementation with clear success criteria
3. **Do**: Execute TDD (RED → GREEN → REFACTOR)
4. **Check**: Validate business outcomes and quality

See [Implementation Checklist](./implementation-checklist.md) for APDC-TDD workflow.

---

## 📄 License

See [LICENSE](../../../../../LICENSE) file in repository root.

---

## ✅ Production Readiness Status

### **Implementation** ✅
- [x] 100% BR coverage (17/17 BRs implemented)
- [x] All channels operational (console, slack, file for E2E)
- [x] Exponential backoff retry with circuit breakers
- [x] Data sanitization (22 secret patterns)
- [x] ADR-034 unified audit table integration
- [x] Channel routing with hot-reload (BR-NOT-065 to BR-NOT-068)

### **Testing** ✅
- [x] 12 unit test files (70%+ coverage)
- [x] 18 integration test files (>50% coverage - microservices architecture)
- [x] 4 E2E test files (<10% coverage - Kind-based)
- [x] Zero flaky tests
- [x] 100% pass rate in CI/CD
- [x] Parallel execution (4 concurrent processes)

### **Security** ✅
- [x] Non-root container (UID 65532)
- [x] Minimum RBAC permissions
- [x] Read-only root filesystem
- [x] Seccomp profile: RuntimeDefault
- [x] No privilege escalation
- [x] Secret sanitization (22 patterns)

### **Observability** ✅
- [x] 10 Prometheus metrics
- [x] Health probes (/healthz, /readyz)
- [x] Structured logging with correlation IDs
- [x] ADR-034 audit events
- [x] 2 production runbooks with automation

### **Documentation** ✅
- [x] 11 core specification documents (6,913 lines)
- [x] Production runbooks (2)
- [x] Test documentation (BR coverage, execution guide)
- [x] Integration examples for RO
- [x] Troubleshooting guides

### **Deployment** ✅
- [x] Kubernetes manifests (production-ready)
- [x] Dockerfile (UBI9-based, security-hardened)
- [x] ConfigMap for controller configuration
- [x] Secret management patterns
- [x] DD-TEST-001 port compliance (9186/30186)

**Status**: **✅ 100% PRODUCTION-READY** (v1.2.0, December 1, 2025)

**Next Steps**: Ready for V1.0 integration with Remediation Orchestrator

---

**Version**: v1.4.0
**Last Updated**: December 6, 2025
**Status**: ✅ Production-Ready + Day 13 Enhancement Scheduled
**Maintainer**: Notification Service Team

