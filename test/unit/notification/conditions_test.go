package notification

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification"
)

// BR-NOT-069: Routing Rule Visibility via Kubernetes Conditions
// Test coverage for condition helper functions
var _ = Describe("Notification Conditions", func() {
	var notifReq *notificationv1alpha1.NotificationRequest

	BeforeEach(func() {
		notifReq = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-notification",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Subject:  "Test Notification",
				Body:     "Test body",
			},
			Status: notificationv1alpha1.NotificationRequestStatus{
				Phase: notificationv1alpha1.NotificationPhasePending,
			},
		}
	})

	Describe("SetRoutingResolved", func() {
		Context("when setting condition for the first time", func() {
			It("should create RoutingResolved condition with correct values", func() {
				// BR-NOT-069: Set RoutingResolved condition after routing resolution
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionTrue,
					notification.ReasonRoutingRuleMatched,
					"Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email",
				)

				Expect(notifReq.Status.Conditions).To(HaveLen(1))

				condition := notifReq.Status.Conditions[0]
				Expect(condition.Type).To(Equal(notification.ConditionTypeRoutingResolved))
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(notification.ReasonRoutingRuleMatched))
				Expect(condition.Message).To(ContainSubstring("Matched rule 'production-critical'"))
				Expect(condition.Message).To(ContainSubstring("channels: slack, email"))
				Expect(condition.ObservedGeneration).To(Equal(int64(1)))
				Expect(condition.LastTransitionTime).NotTo(BeZero())
			})
		})

		Context("when updating existing condition with same status", func() {
			It("should update message but preserve LastTransitionTime", func() {
				// Set initial condition
				initialTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
				notifReq.Status.Conditions = []metav1.Condition{
					{
						Type:               notification.ConditionTypeRoutingResolved,
						Status:             metav1.ConditionTrue,
						Reason:             notification.ReasonRoutingRuleMatched,
						Message:            "Old message",
						LastTransitionTime: initialTime,
						ObservedGeneration: 1,
					},
				}

				// Update with same status
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionTrue,
					notification.ReasonRoutingRuleMatched,
					"New message after hot-reload",
				)

				Expect(notifReq.Status.Conditions).To(HaveLen(1))
				condition := notifReq.Status.Conditions[0]
				Expect(condition.Message).To(Equal("New message after hot-reload"))
				Expect(condition.LastTransitionTime).To(Equal(initialTime), "LastTransitionTime should not change when status stays the same")
			})
		})

		Context("when updating existing condition with different status", func() {
			It("should update LastTransitionTime", func() {
				// Set initial condition (True)
				oldTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
				notifReq.Status.Conditions = []metav1.Condition{
					{
						Type:               notification.ConditionTypeRoutingResolved,
						Status:             metav1.ConditionTrue,
						Reason:             notification.ReasonRoutingRuleMatched,
						Message:            "Old status",
						LastTransitionTime: oldTime,
						ObservedGeneration: 1,
					},
				}

				// Update to False (routing failed)
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionFalse,
					notification.ReasonRoutingFailed,
					"Routing failed due to invalid config",
				)

				Expect(notifReq.Status.Conditions).To(HaveLen(1))
				condition := notifReq.Status.Conditions[0]
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(notification.ReasonRoutingFailed))
				Expect(condition.LastTransitionTime).NotTo(Equal(oldTime), "LastTransitionTime should update when status changes")
				Expect(condition.LastTransitionTime.After(oldTime.Time)).To(BeTrue())
			})
		})
	})

	Describe("IsRoutingResolved", func() {
		Context("when RoutingResolved condition is True", func() {
			It("should return true", func() {
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionTrue,
					notification.ReasonRoutingRuleMatched,
					"Matched rule",
				)

				Expect(notification.IsRoutingResolved(notifReq)).To(BeTrue())
			})
		})

		Context("when RoutingResolved condition is False", func() {
			It("should return false", func() {
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionFalse,
					notification.ReasonRoutingFailed,
					"Routing failed",
				)

				Expect(notification.IsRoutingResolved(notifReq)).To(BeFalse())
			})
		})

		Context("when RoutingResolved condition does not exist", func() {
			It("should return false", func() {
				// No conditions set
				Expect(notification.IsRoutingResolved(notifReq)).To(BeFalse())
			})
		})
	})

	Describe("GetRoutingResolved", func() {
		Context("when RoutingResolved condition exists", func() {
			It("should return the condition", func() {
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionTrue,
					notification.ReasonRoutingFallback,
					"No rules matched, using console fallback",
				)

				condition := notification.GetRoutingResolved(notifReq)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Type).To(Equal(notification.ConditionTypeRoutingResolved))
				Expect(condition.Reason).To(Equal(notification.ReasonRoutingFallback))
				Expect(condition.Message).To(ContainSubstring("console fallback"))
			})
		})

		Context("when RoutingResolved condition does not exist", func() {
			It("should return nil", func() {
				// No conditions set
				condition := notification.GetRoutingResolved(notifReq)
				Expect(condition).To(BeNil())
			})
		})
	})

	Describe("Routing Fallback Scenario", func() {
		Context("when no routing rules match", func() {
			It("should set RoutingResolved with Fallback reason", func() {
				// BR-NOT-069: Fallback detection visible via kubectl
				notification.SetRoutingResolved(
					notifReq,
					metav1.ConditionTrue,
					notification.ReasonRoutingFallback,
					"No routing rules matched (labels: type=escalation, severity=low), using console fallback",
				)

				condition := notification.GetRoutingResolved(notifReq)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue), "Fallback is still successful routing")
				Expect(condition.Reason).To(Equal(notification.ReasonRoutingFallback))
				Expect(condition.Message).To(ContainSubstring("No routing rules matched"))
				Expect(condition.Message).To(ContainSubstring("console fallback"))
			})
		})
	})

	// Issue #79 Phase 1a: Generic condition helpers
	Describe("SetCondition", func() {
		Context("when setting an arbitrary condition type", func() {
			It("should create the condition with correct values", func() {
				notification.SetCondition(
					notifReq,
					"TestCondition",
					metav1.ConditionTrue,
					"TestReason",
					"test message",
				)

				Expect(notifReq.Status.Conditions).To(HaveLen(1))
				c := notifReq.Status.Conditions[0]
				Expect(c.Type).To(Equal("TestCondition"))
				Expect(c.Status).To(Equal(metav1.ConditionTrue))
				Expect(c.Reason).To(Equal("TestReason"))
				Expect(c.Message).To(Equal("test message"))
				Expect(c.ObservedGeneration).To(Equal(int64(1)))
			})
		})

		Context("when setting multiple different condition types", func() {
			It("should store each condition independently", func() {
				notification.SetCondition(notifReq, "CondA", metav1.ConditionTrue, "ReasonA", "msg A")
				notification.SetCondition(notifReq, "CondB", metav1.ConditionFalse, "ReasonB", "msg B")

				Expect(notifReq.Status.Conditions).To(HaveLen(2))
			})
		})

		Context("when updating an existing condition", func() {
			It("should update in place without duplicating", func() {
				notification.SetCondition(notifReq, "CondA", metav1.ConditionTrue, "R1", "first")
				notification.SetCondition(notifReq, "CondA", metav1.ConditionFalse, "R2", "second")

				Expect(notifReq.Status.Conditions).To(HaveLen(1))
				c := notifReq.Status.Conditions[0]
				Expect(c.Status).To(Equal(metav1.ConditionFalse))
				Expect(c.Reason).To(Equal("R2"))
			})
		})
	})

	Describe("GetCondition", func() {
		Context("when condition exists", func() {
			It("should return the condition", func() {
				notification.SetCondition(notifReq, "TestCond", metav1.ConditionTrue, "R", "m")

				c := notification.GetCondition(notifReq, "TestCond")
				Expect(c).NotTo(BeNil())
				Expect(c.Type).To(Equal("TestCond"))
			})
		})

		Context("when condition does not exist", func() {
			It("should return nil", func() {
				c := notification.GetCondition(notifReq, "NonExistent")
				Expect(c).To(BeNil())
			})
		})
	})
})
