# RemediationOrchestrator - Notification Integration Implementation Plan

**Date**: 2025-10-13  
**Status**: ⏳ **Ready for Implementation** (awaiting RemediationOrchestrator completion)  
**Effort**: **1-2 hours**  
**Confidence**: **90%**  
**Related**: ADR-017 (NotificationRequest CRD Creator)

---

## 📊 **Executive Summary**

Per **ADR-017**, the `RemediationOrchestrator` is responsible for creating `NotificationRequest` CRDs to notify users of:
1. Remediation completion (Success/Failed)
2. Critical events requiring escalation
3. Significant status changes

**Current Status**: RemediationOrchestrator CRD is scaffold-only  
**Implementation Approach**: Add notification creation logic to controller when CRD is complete

---

## 🎯 **Integration Requirements (Per ADR-017)**

### **When to Create Notifications**:

| Event | Priority | Channels | Notification Type |
|-------|----------|----------|-------------------|
| Remediation Success | Medium | Console + Slack | `status-update` |
| Remediation Failed | High | Console + Slack | `escalation` |
| Critical Alert | Critical | Console + Slack | `escalation` |
| Recovery Initiated | Medium | Console | `status-update` |
| Max Retries Exceeded | High | Console + Slack | `escalation` |

---

## 📋 **Implementation Steps**

### **Step 1: Update RemediationOrchestrator Controller** (30-45 min)

**File**: `internal/controller/remediationorchestrator/remediationorchestrator_controller.go`

**Changes Needed**:

#### **1a: Add Imports**
```go
import (
    "fmt"
    
    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)
```

#### **1b: Add RBAC Markers**
```go
// +kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests,verbs=create;get;list;watch
```

#### **1c: Update Reconcile Method**
```go
func (r *RemediationOrchestratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := logf.FromContext(ctx)

    // Fetch RemediationOrchestrator instance
    orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{}
    err := r.Get(ctx, req.NamespacedName, orchestrator)
    if err != nil {
        if errors.IsNotFound(err) {
            log.Info("RemediationOrchestrator not found, likely deleted")
            return ctrl.Result{}, nil
        }
        log.Error(err, "Failed to fetch RemediationOrchestrator")
        return ctrl.Result{}, err
    }

    // ... existing reconciliation logic ...

    // Create notification if appropriate
    if r.shouldNotify(orchestrator) {
        if err := r.createNotification(ctx, orchestrator); err != nil {
            log.Error(err, "Failed to create notification", 
                "orchestrator", orchestrator.Name)
            // Don't fail reconciliation - notification is non-critical
        }
    }

    return ctrl.Result{}, nil
}
```

---

### **Step 2: Add Notification Helper Functions** (15-20 min)

**File**: `internal/controller/remediationorchestrator/notification.go` (NEW)

```go
package remediationorchestrator

import (
    "context"
    "fmt"

    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    remediationorchestratorv1alpha1 "github.com/jordigilh/kubernaut/api/remediationorchestrator/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// shouldNotify determines if a notification should be created for the given orchestrator
func (r *RemediationOrchestratorReconciler) shouldNotify(orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator) bool {
    // Check if we've already created a notification for this phase
    // (avoid duplicate notifications)
    if orchestrator.Status.LastNotifiedPhase == orchestrator.Status.Phase {
        return false
    }

    // Notify on terminal states
    if orchestrator.Status.Phase == "Success" || orchestrator.Status.Phase == "Failed" {
        return true
    }

    // Notify on critical priority
    if orchestrator.Spec.Priority == "critical" {
        return true
    }

    // Notify on escalation events
    if orchestrator.Status.RequiresEscalation {
        return true
    }

    // Notify on max retries exceeded
    if orchestrator.Status.RetryCount >= orchestrator.Spec.MaxRetries {
        return true
    }

    return false
}

// createNotification creates a NotificationRequest CRD for the given orchestrator
func (r *RemediationOrchestratorReconciler) createNotification(
    ctx context.Context,
    orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator,
) error {
    log := logf.FromContext(ctx)

    notification := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("remediation-%s-%s", 
                orchestrator.Name, 
                orchestrator.Status.Phase),
            Namespace: orchestrator.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":       "remediation-notification",
                "app.kubernetes.io/managed-by": "remediationorchestrator",
                "remediation-name":             orchestrator.Name,
                "remediation-phase":            orchestrator.Status.Phase,
            },
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion: orchestrator.APIVersion,
                    Kind:       orchestrator.Kind,
                    Name:       orchestrator.Name,
                    UID:        orchestrator.UID,
                    Controller: boolPtr(true),
                },
            },
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Subject:  formatSubject(orchestrator),
            Body:     formatBody(orchestrator),
            Type:     determineNotificationType(orchestrator),
            Priority: determinePriority(orchestrator),
            Channels: determineChannels(orchestrator),
        },
    }

    log.Info("Creating notification", 
        "notification", notification.Name,
        "remediation", orchestrator.Name,
        "phase", orchestrator.Status.Phase)

    if err := r.Create(ctx, notification); err != nil {
        return fmt.Errorf("failed to create notification: %w", err)
    }

    // Update status to record that we've notified for this phase
    orchestrator.Status.LastNotifiedPhase = orchestrator.Status.Phase
    if err := r.Status().Update(ctx, orchestrator); err != nil {
        log.Error(err, "Failed to update LastNotifiedPhase status")
        // Non-critical error - notification was created successfully
    }

    return nil
}

// formatSubject creates a notification subject line
func formatSubject(orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator) string {
    return fmt.Sprintf("Remediation %s: %s", 
        orchestrator.Status.Phase,
        orchestrator.Name)
}

// formatBody creates a notification body with remediation details
func formatBody(orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator) string {
    return fmt.Sprintf(`Remediation Status Update

**Remediation**: %s
**Status**: %s
**Namespace**: %s
**Alert**: %s
**Priority**: %s

**Details**:
- Retry Count: %d/%d
- Started: %s
- Last Update: %s

**Summary**: %s`,
        orchestrator.Name,
        orchestrator.Status.Phase,
        orchestrator.Namespace,
        orchestrator.Spec.AlertName,
        orchestrator.Spec.Priority,
        orchestrator.Status.RetryCount,
        orchestrator.Spec.MaxRetries,
        orchestrator.Status.StartTime.Format("2006-01-02 15:04:05"),
        orchestrator.Status.LastUpdateTime.Format("2006-01-02 15:04:05"),
        orchestrator.Status.Summary,
    )
}

// determineNotificationType maps remediation state to notification type
func determineNotificationType(orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator) notificationv1alpha1.NotificationType {
    switch orchestrator.Status.Phase {
    case "Failed":
        return notificationv1alpha1.NotificationTypeEscalation
    case "Success":
        return notificationv1alpha1.NotificationTypeStatusUpdate
    default:
        if orchestrator.Status.RequiresEscalation {
            return notificationv1alpha1.NotificationTypeEscalation
        }
        return notificationv1alpha1.NotificationTypeSimple
    }
}

// determinePriority maps remediation priority to notification priority
func determinePriority(orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator) notificationv1alpha1.NotificationPriority {
    switch orchestrator.Spec.Priority {
    case "critical":
        return notificationv1alpha1.NotificationPriorityCritical
    case "high":
        return notificationv1alpha1.NotificationPriorityHigh
    case "low":
        return notificationv1alpha1.NotificationPriorityLow
    default:
        return notificationv1alpha1.NotificationPriorityMedium
    }
}

// determineChannels selects notification channels based on remediation state
func determineChannels(orchestrator *remediationorchestratorv1alpha1.RemediationOrchestrator) []notificationv1alpha1.Channel {
    channels := []notificationv1alpha1.Channel{
        notificationv1alpha1.ChannelConsole, // Always notify console
    }

    // Add Slack for important events
    if orchestrator.Status.Phase == "Failed" ||
       orchestrator.Status.Phase == "Success" ||
       orchestrator.Spec.Priority == "critical" ||
       orchestrator.Status.RequiresEscalation {
        channels = append(channels, notificationv1alpha1.ChannelSlack)
    }

    return channels
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
    return &b
}
```

---

### **Step 3: Add Unit Tests** (30-45 min)

**File**: `internal/controller/remediationorchestrator/notification_test.go` (NEW)

```go
package remediationorchestrator

import (
    "context"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    remediationorchestratorv1alpha1 "github.com/jordigilh/kubernaut/api/remediationorchestrator/v1alpha1"
)

func TestNotificationIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "RemediationOrchestrator Notification Integration Suite")
}

var _ = Describe("Notification Integration (ADR-017)", func() {
    var (
        ctx        context.Context
        reconciler *RemediationOrchestratorReconciler
        scheme     *runtime.Scheme
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme = runtime.NewScheme()
        _ = remediationorchestratorv1alpha1.AddToScheme(scheme)
        _ = notificationv1alpha1.AddToScheme(scheme)

        fakeClient := fake.NewClientBuilder().
            WithScheme(scheme).
            WithStatusSubresource(
                &remediationorchestratorv1alpha1.RemediationOrchestrator{},
                &notificationv1alpha1.NotificationRequest{},
            ).
            Build()

        reconciler = &RemediationOrchestratorReconciler{
            Client: fakeClient,
            Scheme: scheme,
        }
    })

    Context("shouldNotify decision", func() {
        It("should notify on remediation success (BR-NOT Integration)", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                },
                Spec: remediationorchestratorv1alpha1.RemediationOrchestratorSpec{
                    Priority: "medium",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "Success",
                },
            }

            Expect(reconciler.shouldNotify(orchestrator)).To(BeTrue(),
                "Should notify on remediation success")
        })

        It("should notify on remediation failure (BR-NOT Integration)", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "Failed",
                },
            }

            Expect(reconciler.shouldNotify(orchestrator)).To(BeTrue(),
                "Should notify on remediation failure")
        })

        It("should notify on critical priority (BR-NOT Integration)", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                },
                Spec: remediationorchestratorv1alpha1.RemediationOrchestratorSpec{
                    Priority: "critical",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "InProgress",
                },
            }

            Expect(reconciler.shouldNotify(orchestrator)).To(BeTrue(),
                "Should notify on critical priority")
        })

        It("should NOT notify if already notified for this phase", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase:              "Success",
                    LastNotifiedPhase:  "Success",
                },
            }

            Expect(reconciler.shouldNotify(orchestrator)).To(BeFalse(),
                "Should not notify again for same phase")
        })
    })

    Context("createNotification", func() {
        It("should create NotificationRequest with correct fields", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                    UID:       "test-uid-123",
                },
                Spec: remediationorchestratorv1alpha1.RemediationOrchestratorSpec{
                    Priority:  "high",
                    AlertName: "HighCPUUsage",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase:   "Success",
                    Summary: "Remediation completed successfully",
                },
            }

            // Create orchestrator first
            err := reconciler.Create(ctx, orchestrator)
            Expect(err).ToNot(HaveOccurred())

            // Create notification
            err = reconciler.createNotification(ctx, orchestrator)
            Expect(err).ToNot(HaveOccurred())

            // Verify notification was created
            notification := &notificationv1alpha1.NotificationRequest{}
            err = reconciler.Get(ctx, types.NamespacedName{
                Name:      "remediation-test-remediation-Success",
                Namespace: "default",
            }, notification)
            Expect(err).ToNot(HaveOccurred())

            // Verify notification fields
            Expect(notification.Spec.Subject).To(ContainSubstring("Remediation Success"))
            Expect(notification.Spec.Subject).To(ContainSubstring("test-remediation"))
            Expect(notification.Spec.Body).To(ContainSubstring("HighCPUUsage"))
            Expect(notification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityHigh))
            Expect(notification.Spec.Channels).To(ContainElements(
                notificationv1alpha1.ChannelConsole,
                notificationv1alpha1.ChannelSlack,
            ))
        })

        It("should set owner reference for garbage collection", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                    UID:       "test-uid-456",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "Failed",
                },
            }

            err := reconciler.Create(ctx, orchestrator)
            Expect(err).ToNot(HaveOccurred())

            err = reconciler.createNotification(ctx, orchestrator)
            Expect(err).ToNot(HaveOccurred())

            notification := &notificationv1alpha1.NotificationRequest{}
            err = reconciler.Get(ctx, types.NamespacedName{
                Name:      "remediation-test-remediation-Failed",
                Namespace: "default",
            }, notification)
            Expect(err).ToNot(HaveOccurred())

            Expect(notification.OwnerReferences).To(HaveLen(1))
            Expect(notification.OwnerReferences[0].Name).To(Equal("test-remediation"))
            Expect(notification.OwnerReferences[0].UID).To(Equal(types.UID("test-uid-456")))
        })
    })

    Context("notification priority mapping", func() {
        DescribeTable("should map remediation priority to notification priority",
            func(remediationPriority string, expectedNotificationPriority notificationv1alpha1.NotificationPriority) {
                orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                    Spec: remediationorchestratorv1alpha1.RemediationOrchestratorSpec{
                        Priority: remediationPriority,
                    },
                }

                priority := determinePriority(orchestrator)
                Expect(priority).To(Equal(expectedNotificationPriority))
            },
            Entry("critical → critical", "critical", notificationv1alpha1.NotificationPriorityCritical),
            Entry("high → high", "high", notificationv1alpha1.NotificationPriorityHigh),
            Entry("medium → medium", "medium", notificationv1alpha1.NotificationPriorityMedium),
            Entry("low → low", "low", notificationv1alpha1.NotificationPriorityLow),
            Entry("default → medium", "", notificationv1alpha1.NotificationPriorityMedium),
        )
    })

    Context("notification channel selection", func() {
        It("should always include console channel", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "InProgress",
                },
            }

            channels := determineChannels(orchestrator)
            Expect(channels).To(ContainElement(notificationv1alpha1.ChannelConsole))
        })

        It("should include Slack for failures", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "Failed",
                },
            }

            channels := determineChannels(orchestrator)
            Expect(channels).To(ContainElements(
                notificationv1alpha1.ChannelConsole,
                notificationv1alpha1.ChannelSlack,
            ))
        })

        It("should include Slack for critical priority", func() {
            orchestrator := &remediationorchestratorv1alpha1.RemediationOrchestrator{
                Spec: remediationorchestratorv1alpha1.RemediationOrchestratorSpec{
                    Priority: "critical",
                },
                Status: remediationorchestratorv1alpha1.RemediationOrchestratorStatus{
                    Phase: "InProgress",
                },
            }

            channels := determineChannels(orchestrator)
            Expect(channels).To(ContainElement(notificationv1alpha1.ChannelSlack))
        })
    })
})
```

---

## 📊 **Implementation Timeline**

| Step | Task | Effort | Cumulative |
|------|------|--------|------------|
| 1 | Update controller | 30-45 min | 30-45 min |
| 2 | Add helper functions | 15-20 min | 45-65 min |
| 3 | Add unit tests | 30-45 min | 75-110 min |

**Total Effort**: **1.5-2 hours**

**Prerequisites**:
- ✅ NotificationRequest CRD API complete
- ⏳ RemediationOrchestrator CRD fully defined
- ⏳ RemediationOrchestrator controller implemented

---

## ✅ **Success Criteria**

### **Unit Test Validation**:
- [x] shouldNotify() logic tested (6 scenarios)
- [x] createNotification() tested (2 scenarios)
- [x] Priority mapping tested (5 scenarios)
- [x] Channel selection tested (3 scenarios)
- [x] Owner reference validation tested
- [x] Idempotency tested (no duplicate notifications)

### **Integration Validation** (Post-Deployment):
- [ ] RemediationOrchestrator creates notifications on success
- [ ] RemediationOrchestrator creates notifications on failure
- [ ] Notifications have correct priority mapping
- [ ] Notifications include complete remediation details
- [ ] Owner references enable garbage collection
- [ ] No duplicate notifications created

---

## 🎯 **BR Coverage**

This integration enables:
- **BR-NOT-050**: Data loss prevention (notifications persisted to etcd)
- **BR-NOT-051**: Complete audit trail (remediation events recorded)
- **BR-NOT-053**: At-least-once delivery (remediation status guaranteed notification)
- **BR-NOT-056**: CRD lifecycle (notifications follow remediation lifecycle)

---

## 📋 **Deferred Work**

**Status**: ⏳ **Implementation deferred** until RemediationOrchestrator CRD is complete

**Why Deferred**:
1. RemediationOrchestrator CRD currently scaffold-only
2. Need real remediation flow to test notification integration
3. Notification controller deployment deferred until all services complete

**When to Implement**:
- When RemediationOrchestrator CRD spec is defined
- When RemediationOrchestrator controller reconciliation logic is implemented
- When ready to deploy complete system end-to-end

---

## 🔗 **Related Documentation**

- **ADR-017**: NotificationRequest CRD Creator (RemediationOrchestrator responsibility)
- **Notification Service Docs**: `docs/services/crd-controllers/06-notification/`
- **BR-NOT-050 to BR-NOT-058**: Notification service business requirements

---

**Version**: 1.0  
**Date**: 2025-10-13  
**Status**: ⏳ **Ready for implementation** (awaiting RemediationOrchestrator completion)  
**Effort**: 1.5-2 hours  
**Confidence**: 90%

