# DD-AUDIT-006: RemediationApprovalRequest Audit Implementation

**Status**: ✅ **APPROVED** - V1.0 Critical Feature
**Date**: February 1, 2026
**Priority**: P0 (SOC 2 Compliance Mandatory)
**Version**: 1.0
**Parent BR**: [BR-AUDIT-006](../../requirements/BR-AUDIT-006-remediation-approval-audit-trail.md)
**Related**: ADR-040, DD-AUDIT-003 v1.6, DD-AUDIT-002, DD-WEBHOOK-001

---

## Context & Problem

### **The Compliance Gap**

RemediationApprovalRequest (RAR) captures approval decisions in CRD status but **does not emit audit events**. This creates a critical SOC 2 compliance gap:

**Current State**:
```
RAR Controller: Decision made → Update CRD status
                                    ↓
                         ❌ NO AUDIT EVENT ❌
```

**SOC 2 Impact**:
- ❌ **CC8.1 Violation**: Cannot prove WHO made approval decision
- ❌ **CC6.8 Violation**: No tamper-evident record (non-repudiation)
- ❌ **AU-2 Violation**: Approval decisions not audited
- ❌ **After CRD deletion (90 days)**: NO EVIDENCE of approval

**Auditor Question**: "Who approved the production remediation on January 15?"
**Answer**: ❌ **NO ANSWER** after 90 days (CRD deleted, no audit trail)

---

### **Why This is a Problem**

1. **SOC 2 Certification FAILS**: Control gap blocks Type II certification
2. **Legal Liability**: Cannot defend approval decisions in court
3. **Forensic Investigation**: Cannot reconstruct approval history after CRD deletion
4. **Accountability**: No proof of WHO made high-risk decisions

---

## Decision

**RemediationApprovalRequest controller MUST emit audit events to DataStorage for all approval decisions**, following the same pattern as AIAnalysis and SignalProcessing controllers.

### **Event Types** (3 events)

| Event Type | Trigger | Priority | Purpose |
|-----------|---------|----------|---------|
| `approval.decision` | Decision made | **P0** | SOC 2 compliance (WHO, WHEN, WHAT, WHY) |
| `approval.request.created` | RAR created | P1 | Context (why approval needed) |
| `approval.timeout` | Timeout | P1 | Operational visibility |

---

## Implementation Pattern

### **Follow Existing Audit Pattern**

**Precedent**: `pkg/aianalysis/audit/` (successful pattern)

**Copy Pattern From**:
```
pkg/aianalysis/audit/
├── audit.go           # Core audit client
├── types.go           # Event type constants
└── audit_test.go      # Unit tests
```

**Apply To**:
```
pkg/remediationapprovalrequest/audit/  ← NEW
├── audit.go           # RAR audit client
├── types.go           # Event type constants  
└── audit_test.go      # Unit tests
```

---

## Detailed Implementation

### **Component 1: Audit Package**

**Location**: `pkg/remediationapprovalrequest/audit/`

#### **File: audit.go**

```go
package audit

import (
	"context"
	
	"github.com/go-logr/logr"
	remediationapprovalrequestv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Event categories
const (
	EventCategoryApproval = "approval"
)

// Event types
const (
	EventTypeApprovalDecision      = "approval.decision"       // P0 - SOC 2 critical
	EventTypeApprovalRequestCreated = "approval.request.created" // P1 - context
	EventTypeApprovalTimeout       = "approval.timeout"        // P1 - operational
)

// Event actions
const (
	EventActionDecisionMade    = "decision_made"
	EventActionRequestCreated  = "request_created"
	EventActionTimeout         = "timeout"
)

// Actor IDs
const (
	ActorTypeService = "service"
	ActorTypeUser    = "user"
	ActorTypeSystem  = "system"
	ActorIDController = "remediationapprovalrequest-controller"
)

// AuditClient handles audit event generation for RemediationApprovalRequest
type AuditClient struct {
	store audit.Store
	log   logr.Logger
}

// NewAuditClient creates a new audit client
func NewAuditClient(store audit.Store, log logr.Logger) *AuditClient {
	return &AuditClient{
		store: store,
		log:   log,
	}
}

// RecordApprovalDecision records approval decision event (P0 - SOC 2 critical)
// Captures WHO, WHEN, WHAT, WHY for compliance and forensic investigation
func (c *AuditClient) RecordApprovalDecision(ctx context.Context, rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest) {
	// Only emit if decision is final
	if rar.Status.Decision == "" {
		return
	}

	// Build structured payload using OpenAPI-generated type
	payload := &ogenclient.RemediationApprovalDecisionPayload{
		EventType:                EventTypeApprovalDecision,
		RemediationRequestName:   rar.Spec.RemediationRequestRef.Name,
		AIAnalysisName:           rar.Spec.AIAnalysisRef.Name,
		Decision:                 string(rar.Status.Decision),
		DecidedBy:                rar.Status.DecidedBy,
		Confidence:               rar.Spec.Confidence,
		WorkflowID:               rar.Spec.RecommendedWorkflow.WorkflowID,
	}

	// Optional fields (use .SetTo() for optional fields)
	if rar.Status.DecidedAt != nil {
		payload.DecidedAt.SetTo(rar.Status.DecidedAt.Time)
	}
	if rar.Status.DecisionMessage != "" {
		payload.DecisionMessage.SetTo(rar.Status.DecisionMessage)
	}
	if rar.Spec.RecommendedWorkflow.WorkflowVersion != "" {
		payload.WorkflowVersion.SetTo(rar.Spec.RecommendedWorkflow.WorkflowVersion)
	}
	if rar.Spec.TargetResource != "" {
		payload.TargetResource.SetTo(rar.Spec.TargetResource)
	}
	if rar.Spec.RequiredBy != nil {
		payload.TimeoutDeadline.SetTo(rar.Spec.RequiredBy.Time)
	}

	// Calculate decision duration
	if rar.Status.DecidedAt != nil {
		durationSeconds := int32(rar.Status.DecidedAt.Sub(rar.CreationTimestamp.Time).Seconds())
		payload.DecisionDurationSeconds.SetTo(durationSeconds)
	}

	// Determine outcome
	var apiOutcome ogenclient.AuditEventRequestEventOutcome
	if rar.Status.Decision == remediationapprovalrequestv1alpha1.ApprovalDecisionApproved {
		apiOutcome = audit.OutcomeSuccess
	} else {
		apiOutcome = audit.OutcomeFailure
	}

	// Build audit event (DD-AUDIT-002: OpenAPI types)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeApprovalDecision)
	audit.SetEventCategory(event, EventCategoryApproval)
	audit.SetEventAction(event, EventActionDecisionMade)
	audit.SetEventOutcome(event, apiOutcome)
	
	// Actor: authenticated user from webhook (SOC 2 CC8.1)
	audit.SetActor(event, ActorTypeUser, rar.Status.DecidedBy)
	
	audit.SetResource(event, "RemediationApprovalRequest", rar.Name)
	
	// Correlation ID: parent RR name (DD-AUDIT-CORRELATION-002)
	audit.SetCorrelationID(event, rar.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, rar.Namespace)
	
	// Set structured payload using union constructor
	event.EventData = ogenclient.NewAuditEventRequestEventDataApprovalDecisionAuditEventRequestEventData(*payload)

	// Fire-and-forget (per DD-AUDIT-002)
	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write approval decision audit event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
			"decision", rar.Status.Decision,
		)
		// Don't fail reconciliation on audit failure (graceful degradation)
	}
}

// RecordApprovalRequestCreated records approval request creation event (P1 - context)
// Captures approval context (why approval needed, deadline, severity)
func (c *AuditClient) RecordApprovalRequestCreated(ctx context.Context, rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest) {
	// Build structured payload
	payload := &ogenclient.RemediationApprovalRequestCreatedPayload{
		EventType:                EventTypeApprovalRequestCreated,
		RemediationRequestName:   rar.Spec.RemediationRequestRef.Name,
		AIAnalysisName:           rar.Spec.AIAnalysisRef.Name,
		Confidence:               rar.Spec.Confidence,
		WorkflowID:               rar.Spec.RecommendedWorkflow.WorkflowID,
		ApprovalReason:           rar.Spec.ApprovalReason,
	}

	if rar.Spec.RequiredBy != nil {
		payload.RequiredBy.SetTo(rar.Spec.RequiredBy.Time)
	}

	// Build audit event
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeApprovalRequestCreated)
	audit.SetEventCategory(event, EventCategoryApproval)
	audit.SetEventAction(event, EventActionRequestCreated)
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, ActorTypeService, "aianalysis-controller") // Creator
	audit.SetResource(event, "RemediationApprovalRequest", rar.Name)
	audit.SetCorrelationID(event, rar.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, rar.Namespace)
	event.EventData = ogenclient.NewAuditEventRequestEventDataApprovalRequestCreatedAuditEventRequestEventData(*payload)

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write approval request created audit event")
	}
}

// RecordApprovalTimeout records approval timeout event (P1 - operational)
// Captures timeout context (deadline, duration)
func (c *AuditClient) RecordApprovalTimeout(ctx context.Context, rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest) {
	// Build structured payload
	payload := &ogenclient.RemediationApprovalTimeoutPayload{
		EventType:                EventTypeApprovalTimeout,
		RemediationRequestName:   rar.Spec.RemediationRequestRef.Name,
		AIAnalysisName:           rar.Spec.AIAnalysisRef.Name,
		TimeoutReason:            "No operator response within deadline",
	}

	if rar.Spec.RequiredBy != nil {
		payload.TimeoutDeadline.SetTo(rar.Spec.RequiredBy.Time)
		durationSeconds := int32(rar.Spec.RequiredBy.Sub(rar.CreationTimestamp.Time).Seconds())
		payload.TimeoutDurationSeconds.SetTo(durationSeconds)
	}

	// Build audit event
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, EventTypeApprovalTimeout)
	audit.SetEventCategory(event, EventCategoryApproval)
	audit.SetEventAction(event, EventActionTimeout)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, ActorTypeSystem, ActorIDController)
	audit.SetResource(event, "RemediationApprovalRequest", rar.Name)
	audit.SetCorrelationID(event, rar.Spec.RemediationRequestRef.Name)
	audit.SetNamespace(event, rar.Namespace)
	event.EventData = ogenclient.NewAuditEventRequestEventDataApprovalTimeoutAuditEventRequestEventData(*payload)

	if err := c.store.StoreAudit(ctx, event); err != nil {
		c.log.Error(err, "Failed to write approval timeout audit event")
	}
}
```

**Estimated LOC**: ~300-400 LOC

---

#### **File: types.go**

```go
package audit

// Event type constants for RemediationApprovalRequest audit events
// Per DD-AUDIT-003 v1.6: Approval lifecycle events

const (
	// Event categories
	CategoryApproval = "approval"

	// Event types (per DD-AUDIT-003 v1.6)
	TypeApprovalDecision      = "approval.decision"       // SOC 2 CC8.1 critical
	TypeApprovalRequestCreated = "approval.request.created" // Context
	TypeApprovalTimeout       = "approval.timeout"        // Operational

	// Event actions
	ActionDecisionMade    = "decision_made"
	ActionRequestCreated  = "request_created"
	ActionTimeout         = "timeout"

	// Actor types
	ActorService = "service"
	ActorUser    = "user"
	ActorSystem  = "system"

	// Actor IDs
	ActorRAR = "remediationapprovalrequest-controller"
)
```

**Estimated LOC**: ~30-40 LOC

---

#### **File: audit_test.go**

```go
package audit_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationapprovalrequestv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Mock audit store for testing
type MockAuditStore struct {
	StoredEvents []ogenclient.AuditEventRequest
	StoreError   error
}

func (m *MockAuditStore) StoreAudit(ctx context.Context, event ogenclient.AuditEventRequest) error {
	if m.StoreError != nil {
		return m.StoreError
	}
	m.StoredEvents = append(m.StoredEvents, event)
	return nil
}

func TestRemediationApprovalRequestAudit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationApprovalRequest Audit Suite")
}

var _ = Describe("RemediationApprovalRequest Audit", func() {
	var (
		ctx         context.Context
		auditClient *audit.AuditClient
		mockStore   *MockAuditStore
		rar         *remediationapprovalrequestv1alpha1.RemediationApprovalRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		logger := logr.Discard()
		auditClient = audit.NewAuditClient(mockStore, logger)

		// Create test RAR
		now := metav1.Now()
		rar = &remediationapprovalrequestv1alpha1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "rar-test-123",
				Namespace:         "production",
				CreationTimestamp: metav1.Time{Time: now.Add(-180 * time.Second)},
			},
			Spec: remediationapprovalrequestv1alpha1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-parent-456",
					Namespace: "production",
				},
				AIAnalysisRef: remediationapprovalrequestv1alpha1.ObjectRef{
					Name: "ai-test-789",
				},
				Confidence: 0.75,
				RecommendedWorkflow: remediationapprovalrequestv1alpha1.RecommendedWorkflowRef{
					WorkflowID:      "oomkill-increase-memory-limits",
					WorkflowVersion: "v1.2.0",
				},
				TargetResource: "payment/deployment/payment-api",
				ApprovalReason: "Confidence below 80% auto-approve threshold",
			},
			Status: remediationapprovalrequestv1alpha1.RemediationApprovalRequestStatus{
				Decision:        remediationapprovalrequestv1alpha1.ApprovalDecisionApproved,
				DecidedBy:       "alice@example.com",
				DecidedAt:       &now,
				DecisionMessage: "Root cause accurate. Safe to proceed.",
			},
		}
	})

	Context("RecordApprovalDecision", func() {
		It("should emit approval.decision event for approved decision", func() {
			// When
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]

			Expect(event.EventType).To(Equal("approval.decision"))
			Expect(event.EventCategory).To(Equal("approval"))
			Expect(event.EventAction).To(Equal("decision_made"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
			Expect(event.ActorType).To(Equal("user"))
			Expect(event.ActorID).To(Equal("alice@example.com"))
			Expect(event.CorrelationID).To(Equal("rr-parent-456"))
			Expect(event.ResourceType).To(Equal("RemediationApprovalRequest"))
			Expect(event.ResourceName).To(Equal("rar-test-123"))
			Expect(event.Namespace).To(Equal("production"))

			// Verify payload
			Expect(event.EventData.IsRemediationApprovalDecisionPayload()).To(BeTrue())
			payload, ok := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Decision).To(Equal("approved"))
			Expect(payload.DecidedBy).To(Equal("alice@example.com"))
			Expect(payload.Confidence).To(Equal(0.75))
			Expect(payload.WorkflowID).To(Equal("oomkill-increase-memory-limits"))
		})

		It("should NOT emit event if decision is empty", func() {
			// Given: Decision not yet made
			rar.Status.Decision = ""

			// When
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then
			Expect(mockStore.StoredEvents).To(HaveLen(0))
		})

		It("should handle rejected decision", func() {
			// Given
			rar.Status.Decision = remediationapprovalrequestv1alpha1.ApprovalDecisionRejected
			rar.Status.DecisionMessage = "Risk too high for production"

			// When
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then
			Expect(mockStore.StoredEvents).To(HaveLen(1))
			event := mockStore.StoredEvents[0]
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))

			payload, _ := event.EventData.GetRemediationApprovalDecisionPayload()
			Expect(payload.Decision).To(Equal("rejected"))
			decisionMsg, _ := payload.DecisionMessage.Get()
			Expect(decisionMsg).To(Equal("Risk too high for production"))
		})

		It("should not fail reconciliation on audit store failure", func() {
			// Given: Mock store returns error
			mockStore.StoreError = errors.New("audit store unavailable")

			// When: RecordApprovalDecision is called
			auditClient.RecordApprovalDecision(ctx, rar)

			// Then: No panic, graceful degradation
			Expect(mockStore.StoredEvents).To(HaveLen(0))
		})
	})
})
```

**Estimated LOC**: ~200-300 LOC

---

### **Component 2: OpenAPI Schema Extension**

**Location**: `api/openapi/data-storage-v1.yaml`

**Add to `components.schemas`**:

```yaml
RemediationApprovalDecisionPayload:
  type: object
  description: Audit payload for approval decision event (SOC 2 CC8.1)
  required:
    - event_type
    - remediation_request_name
    - ai_analysis_name
    - decision
    - decided_by
    - confidence
    - workflow_id
  properties:
    event_type:
      type: string
      enum: [approval.decision]
      description: Discriminator for union type
    remediation_request_name:
      type: string
      description: Parent RemediationRequest name (correlation ID)
    ai_analysis_name:
      type: string
      description: AIAnalysis CRD that required approval
    decision:
      type: string
      enum: [approved, rejected, expired]
      description: Final approval decision
    decided_by:
      type: string
      description: Authenticated username from webhook (SOC 2 CC8.1)
    decided_at:
      type: string
      format: date-time
      description: When decision was made
    decision_message:
      type: string
      description: Optional rationale from operator
    confidence:
      type: number
      format: float
      description: AI confidence score that triggered approval
    workflow_id:
      type: string
      description: Workflow being approved
    workflow_version:
      type: string
      description: Workflow version
    target_resource:
      type: string
      description: Target resource being remediated
    timeout_deadline:
      type: string
      format: date-time
      description: Approval deadline
    decision_duration_seconds:
      type: integer
      description: Time to decision (seconds)
```

**Add to `AuditEventRequestEventData.oneOf`**:

```yaml
- $ref: '#/components/schemas/RemediationApprovalDecisionPayload'
```

**Regenerate Client**:
```bash
make generate-datastorage-client
```

**Estimated LOC**: ~50-80 LOC (YAML)

---

### **Component 3: Controller Integration**

**Location**: `pkg/remediationapprovalrequest/controller/controller.go`

**Add Audit Client**:

```go
type Reconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Log         logr.Logger
	AuditClient *audit.AuditClient  // ← NEW
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize audit client
	auditStore := datastorage.NewAuditStore(...)
	r.AuditClient = audit.NewAuditClient(auditStore, r.Log)

	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationapprovalrequestv1alpha1.RemediationApprovalRequest{}).
		Complete(r)
}
```

**Hook into Reconciliation**:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("remediationapprovalrequest", req.NamespacedName)

	// Fetch RAR
	rar := &remediationapprovalrequestv1alpha1.RemediationApprovalRequest{}
	if err := r.Get(ctx, req.NamespacedName, rar); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Track old decision for change detection
	oldDecision := rar.Status.Decision

	// ... reconciliation logic ...

	// Emit audit event if decision changed (idempotency)
	if oldDecision == "" && rar.Status.Decision != "" {
		r.AuditClient.RecordApprovalDecision(ctx, rar)
	}

	return ctrl.Result{}, nil
}
```

**Estimated LOC**: ~100-150 LOC (integration + tests)

---

## Testing Strategy

### **Unit Tests**

**Location**: `pkg/remediationapprovalrequest/audit/audit_test.go`

**Coverage**: 8 test cases (see audit_test.go above)

---

### **Integration Tests**

**Location**: `test/integration/remediationapprovalrequest/audit_integration_test.go`

**Test Cases**:
1. Approval decision audit event emitted
2. Rejected decision audit event emitted
3. Timeout decision audit event emitted
4. Audit event queryable after CRD deletion
5. Correlation ID matches parent RR
6. Authenticated user captured correctly
7. Fire-and-forget (no reconciliation failure)

---

## Success Criteria

### **Functional**:
- ✅ Audit package created following AIAnalysis pattern
- ✅ `approval.decision` event emitted for all decisions
- ✅ Authenticated user captured from webhook
- ✅ Correlation ID links to parent RR
- ✅ Fire-and-forget (no controller failure)

### **Compliance**:
- ✅ SOC 2 CC8.1 satisfied (user attribution)
- ✅ SOC 2 CC6.8 satisfied (non-repudiation)
- ✅ Audit events queryable for 90-365 days
- ✅ Tamper-evidence (SHA-256 hashing)

### **Testing**:
- ✅ Unit tests: 8/8 passing
- ✅ Integration tests: 7/7 passing
- ✅ E2E tests: Approval workflow includes audit verification

---

## Implementation Checklist

- [ ] Create `pkg/remediationapprovalrequest/audit/` package
  - [ ] `audit.go` - Core audit client
  - [ ] `types.go` - Event type constants
  - [ ] `audit_test.go` - Unit tests (8 test cases)
- [ ] Update OpenAPI schema
  - [ ] Add `RemediationApprovalDecisionPayload`
  - [ ] Add to `AuditEventRequestEventData.oneOf`
  - [ ] Regenerate ogen client
- [ ] Integrate with controller
  - [ ] Add audit client to reconciler
  - [ ] Hook decision change detection
  - [ ] Fire-and-forget pattern
- [ ] Integration tests
  - [ ] Create `test/integration/remediationapprovalrequest/audit_integration_test.go`
  - [ ] 7 test scenarios
- [ ] Documentation updates
  - [ ] Update DD-AUDIT-003 v1.6 (add RAR section)
  - [ ] Update ADR-040 (reference audit events)
  - [ ] Update BR-AUDIT-006 (mark implemented)

---

## Rollout Plan

### **Phase 1: Development** (Days 1-3)
- Day 1: Create audit package + unit tests
- Day 2: Update OpenAPI schema + regenerate client
- Day 3: Integrate with controller + integration tests

### **Phase 2: Testing** (Days 4-5)
- Day 4: Run full test suite (unit + integration)
- Day 5: E2E testing with live cluster

### **Phase 3: Documentation** (Day 6)
- Update DD-AUDIT-003
- Update ADR-040
- Mark BR-AUDIT-006 as implemented

### **Phase 4: Deployment** (Day 7)
- Merge to main
- Deploy to staging
- Verify with SOC 2 checklist

---

## Related Documents

- [BR-AUDIT-006: Remediation Approval Audit Trail](../../requirements/BR-AUDIT-006-remediation-approval-audit-trail.md)
- [ADR-040: RemediationApprovalRequest CRD Architecture](./ADR-040-remediation-approval-request-architecture.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md)
- [DD-AUDIT-002: Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md)
- [DD-WEBHOOK-001: CRD Webhook Requirements Matrix](./DD-WEBHOOK-001-crd-webhook-requirements-matrix.md)

---

## Approval

**Approved By**: Architecture Team, Compliance Team
**Date**: February 1, 2026
**Priority**: P0 - SOC 2 Compliance Mandatory
**Status**: ✅ **APPROVED for V1.0 Implementation**

---

**Document Version**: 1.0
**Last Updated**: February 1, 2026
**Maintained By**: Kubernaut Architecture Team
