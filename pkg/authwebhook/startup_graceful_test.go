/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authwebhook_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// gracefulDSClient supports per-workflow error injection for graceful degradation testing.
type gracefulDSClient struct {
	workflowErrors map[string]error // keyed by CRD name; nil means success
	atResult       *authwebhook.ActionTypeRegistrationResult
	rwResult       *authwebhook.WorkflowRegistrationResult
}

func (m *gracefulDSClient) CreateActionType(_ context.Context, name string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
	if m.atResult != nil {
		return m.atResult, nil
	}
	return &authwebhook.ActionTypeRegistrationResult{ActionType: name, Status: "created"}, nil
}

func (m *gracefulDSClient) CreateWorkflowInline(_ context.Context, content, source, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
	if err, ok := m.workflowErrors[source]; ok && err != nil {
		return nil, err
	}
	if m.rwResult != nil {
		return m.rwResult, nil
	}
	return &authwebhook.WorkflowRegistrationResult{
		WorkflowID:   "wf-uuid-001",
		WorkflowName: "test",
		Version:      "1.0.0",
		Status:       "Active",
	}, nil
}

// fakeEventRecorder captures K8s events for assertions.
type fakeEventRecorder struct {
	events []recordedEvent
}

type recordedEvent struct {
	Object    string
	EventType string
	Reason    string
	Message   string
}

func (r *fakeEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.events = append(r.events, recordedEvent{EventType: eventtype, Reason: reason, Message: message})
}

func (r *fakeEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.events = append(r.events, recordedEvent{
		EventType: eventtype,
		Reason:    reason,
		Message:   fmt.Sprintf(messageFmt, args...),
	})
}

func (r *fakeEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	r.events = append(r.events, recordedEvent{
		EventType: eventtype,
		Reason:    reason,
		Message:   fmt.Sprintf(messageFmt, args...),
	})
}

var _ = Describe("StartupReconciler Graceful Degradation (#1246)", func() {

	// UT-AW-1246-001: One RW fails with permanent error, others succeed
	Describe("UT-AW-1246-001: Graceful continuation on individual RW failure", func() {
		It("should start successfully when one RW fails with a permanent error", func() {
			scheme := newTestScheme()
			rw1 := makeWorkflowCRD("wf-good", "ScaleMemory")
			rw2 := makeWorkflowCRD("wf-bad", "MissingAction")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw1, rw2).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-bad": authwebhook.NewPermanentError("workflow registration rejected: action_type 'MissingAction' is not in the taxonomy"),
				},
			}

			recorder := &fakeEventRecorder{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  recorder,
			}

			ctx := context.Background()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred(), "startup should succeed despite individual RW failure")

			// Verify good RW has Active status
			good := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-good"), good)).To(Succeed())
			Expect(string(good.Status.CatalogStatus)).To(Equal("Active"))

			// Verify bad RW has Disabled status + Ready=False condition
			bad := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-bad"), bad)).To(Succeed())
			Expect(string(bad.Status.CatalogStatus)).To(Equal("Disabled"))

			readyCond := findCondition(bad.Status.Conditions, rwv1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil(), "Ready condition should exist on failed RW")
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(rwv1alpha1.ReasonDependencyMissing))
		})
	})

	// UT-AW-1246-002: All RWs fail with permanent errors
	Describe("UT-AW-1246-002: All RWs fail with permanent errors", func() {
		It("should still start successfully and mark all as Disabled", func() {
			scheme := newTestScheme()
			rw1 := makeWorkflowCRD("wf-fail-1", "Bad1")
			rw2 := makeWorkflowCRD("wf-fail-2", "Bad2")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw1, rw2).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-fail-1": authwebhook.NewPermanentError("rejected"),
					"wf-fail-2": authwebhook.NewPermanentError("rejected"),
				},
			}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  &fakeEventRecorder{},
			}

			ctx := context.Background()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred(), "startup must succeed even when all RWs fail")

			for _, name := range []string{"wf-fail-1", "wf-fail-2"} {
				rw := &rwv1alpha1.RemediationWorkflow{}
				Expect(k8sClient.Get(ctx, nsName("default", name), rw)).To(Succeed())
				Expect(string(rw.Status.CatalogStatus)).To(Equal("Disabled"))
			}
		})
	})

	// UT-AW-1246-003: IsPermanentError identifies 400/404 errors
	Describe("UT-AW-1246-003: IsPermanentError for permanent errors", func() {
		It("should return true for PermanentError type", func() {
			permErr := authwebhook.NewPermanentError("action_type not found")
			Expect(authwebhook.IsPermanentError(permErr)).To(BeTrue())
		})
	})

	// UT-AW-1246-004: IsPermanentError identifies transient errors
	Describe("UT-AW-1246-004: IsPermanentError for transient errors", func() {
		It("should return false for generic/network errors", func() {
			transientErr := fmt.Errorf("connection refused")
			Expect(authwebhook.IsPermanentError(transientErr)).To(BeFalse())
		})
	})

	// UT-AW-1246-005: Transient error retries then succeeds
	Describe("UT-AW-1246-005: Transient error triggers retry", func() {
		It("should retry transient errors and succeed when DS recovers", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-transient", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			callCount := 0
			mockDS := &countingDSClient{
				callCounter: &callCount,
				failUntil:   2, // fail first 2 calls, succeed on 3rd
			}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  &fakeEventRecorder{},
			}

			ctx := context.Background()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-transient"), updated)).To(Succeed())
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Active"))
			Expect(callCount).To(BeNumerically(">=", 3))
		})
	})

	// UT-AW-1246-006: K8s event emitted for failed RW
	Describe("UT-AW-1246-006: K8s event emission on failure", func() {
		It("should emit a Warning event on the failed RW CR", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-event", "Bad")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-event": authwebhook.NewPermanentError("rejected"),
				},
			}

			recorder := &fakeEventRecorder{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  recorder,
			}

			ctx := context.Background()
			_ = reconciler.Start(ctx)

			Expect(recorder.events).NotTo(BeEmpty(), "should emit at least one K8s event")
			Expect(recorder.events[0].EventType).To(Equal("Warning"))
			Expect(recorder.events[0].Reason).To(Equal("RegistrationFailed"))
		})
	})

	// UT-AW-1246-009: Successful RW gets Ready=True condition
	Describe("UT-AW-1246-009: Ready=True on successful registration", func() {
		It("should set Ready=True condition on successfully registered RW", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-ready", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  &fakeEventRecorder{},
			}

			ctx := context.Background()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-ready"), updated)).To(Succeed())

			readyCond := findCondition(updated.Status.Conditions, rwv1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil(), "Ready condition should exist")
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal(rwv1alpha1.ReasonRegistered))
		})
	})

	// UT-AW-1246-010: Audit event emitted on permanent failure
	Describe("UT-AW-1246-010: Audit event emission on registration failure", func() {
		It("should emit an audit event with category 'workflow' and typed payload", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-audit", "MissingDep")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-audit": authwebhook.NewPermanentError("action_type 'MissingDep' not in taxonomy"),
				},
			}

			auditMock := &mockAuditStore{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  &fakeEventRecorder{},
				AuditStore:     auditMock,
			}

			ctx := context.Background()
			_ = reconciler.Start(ctx)

			Expect(auditMock.storedEvents).To(HaveLen(1), "should emit exactly one audit event")
			evt := auditMock.storedEvents[0]
			Expect(evt.EventType).To(Equal(authwebhook.EventTypeRWRegistrationFailed))
			Expect(string(evt.EventCategory)).To(Equal("workflow"))
			Expect(evt.EventAction).To(Equal("registration_failed"))
			Expect(evt.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))

			// Validate typed payload
			Expect(evt.EventData.Type).To(Equal(ogenclient.AuthwebhookWorkflowRegistrationFailedPayloadAuditEventRequestEventData))
			payload := evt.EventData.AuthwebhookWorkflowRegistrationFailedPayload
			Expect(payload.WorkflowName).To(Equal("wf-audit"))
			Expect(string(payload.Reason)).To(Equal("DependencyMissing"))
			Expect(payload.Message).To(ContainSubstring("action_type 'MissingDep' not in taxonomy"))
		})
	})

	// UT-AW-1246-011: Audit store nil does not panic
	Describe("UT-AW-1246-011: Nil AuditStore graceful handling", func() {
		It("should not panic when AuditStore is nil and a workflow fails", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-noaudit", "Bad")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-noaudit": authwebhook.NewPermanentError("rejected"),
				},
			}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        5 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  &fakeEventRecorder{},
				AuditStore:     nil, // explicitly nil
			}

			ctx := context.Background()
			Expect(func() { _ = reconciler.Start(ctx) }).NotTo(Panic())
		})
	})

	// UT-AW-1246-012: Context cancellation during retry marks workflow as failed
	Describe("UT-AW-1246-012: Context cancellation marks workflow Disabled", func() {
		It("should mark workflow as Disabled when context is cancelled during retry", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-cancel", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-cancel": fmt.Errorf("connection timeout"),
				},
			}

			auditMock := &mockAuditStore{}
			recorder := &fakeEventRecorder{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        2 * time.Second,
				InitialBackoff: 10 * time.Millisecond,
				EventRecorder:  recorder,
				AuditStore:     auditMock,
			}

			ctx, cancel := context.WithCancel(context.Background())
			// Cancel almost immediately so the retry loop detects ctx.Done()
			go func() {
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()

			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred(), "startup should still succeed gracefully")

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(context.Background(), nsName("default", "wf-cancel"), updated)).To(Succeed())
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Disabled"))

			readyCond := findCondition(updated.Status.Conditions, rwv1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(rwv1alpha1.ReasonDataStorageError))

			Expect(auditMock.storedEvents).To(HaveLen(1), "audit event should be emitted for cancelled workflow")
			Expect(recorder.events).NotTo(BeEmpty(), "K8s event should be emitted")
		})
	})

	// UT-AW-1246-013: Deadline exhaustion marks workflow Disabled with audit
	Describe("UT-AW-1246-013: Deadline exhaustion with audit emission", func() {
		It("should mark workflow as Disabled when deadline is exhausted", func() {
			scheme := newTestScheme()
			rw := makeWorkflowCRD("wf-deadline", "ScaleMemory")

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rw).
				WithStatusSubresource(&atv1alpha1.ActionType{}, &rwv1alpha1.RemediationWorkflow{}).
				Build()

			mockDS := &gracefulDSClient{
				workflowErrors: map[string]error{
					"wf-deadline": fmt.Errorf("service unavailable"),
				},
			}

			auditMock := &mockAuditStore{}
			recorder := &fakeEventRecorder{}

			reconciler := &authwebhook.StartupReconciler{
				K8sClient:      k8sClient,
				DSWorkflow:     mockDS,
				DSActionType:   mockDS,
				Logger:         ctrl.Log.WithName("test"),
				Timeout:        50 * time.Millisecond,
				InitialBackoff: 100 * time.Millisecond, // larger than timeout → immediate deadline breach
				EventRecorder:  recorder,
				AuditStore:     auditMock,
			}

			ctx := context.Background()
			err := reconciler.Start(ctx)
			Expect(err).NotTo(HaveOccurred(), "startup should succeed gracefully")

			updated := &rwv1alpha1.RemediationWorkflow{}
			Expect(k8sClient.Get(ctx, nsName("default", "wf-deadline"), updated)).To(Succeed())
			Expect(string(updated.Status.CatalogStatus)).To(Equal("Disabled"))

			readyCond := findCondition(updated.Status.Conditions, rwv1alpha1.ConditionReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal(rwv1alpha1.ReasonDataStorageError))

			Expect(auditMock.storedEvents).To(HaveLen(1), "audit event should be emitted for deadline exhaustion")
			Expect(recorder.events).NotTo(BeEmpty(), "K8s warning event should be emitted")
		})
	})
})

// countingDSClient fails N times with transient errors, then succeeds.
type countingDSClient struct {
	callCounter *int
	failUntil   int
}

func (m *countingDSClient) CreateActionType(_ context.Context, name string, _ ogenclient.ActionTypeDescription, _ string) (*authwebhook.ActionTypeRegistrationResult, error) {
	return &authwebhook.ActionTypeRegistrationResult{ActionType: name, Status: "created"}, nil
}

func (m *countingDSClient) CreateWorkflowInline(_ context.Context, _, _, _ string) (*authwebhook.WorkflowRegistrationResult, error) {
	*m.callCounter++
	if *m.callCounter <= m.failUntil {
		return nil, fmt.Errorf("connection refused")
	}
	return &authwebhook.WorkflowRegistrationResult{
		WorkflowID:   "wf-uuid-recovered",
		WorkflowName: "test",
		Version:      "1.0.0",
		Status:       "Active",
	}, nil
}

// mockAuditStore captures audit events emitted by the startup reconciler.
type mockAuditStore struct {
	storedEvents []*ogenclient.AuditEventRequest
	storeError   error
}

func (m *mockAuditStore) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	if m.storeError != nil {
		return m.storeError
	}
	m.storedEvents = append(m.storedEvents, event)
	return nil
}

func (m *mockAuditStore) Flush(_ context.Context) error { return nil }
func (m *mockAuditStore) Close() error                  { return nil }

// findCondition looks up a condition by type in the conditions slice.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
