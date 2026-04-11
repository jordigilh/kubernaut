/*
Copyright 2026 Jordi Gil.

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

package authwebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Covers pkg/authwebhook zero-coverage helpers (audit enum converters via webhook paths,
// BuildRARApprovalAuditEvent, NewDSClientAdapter). Issue / coverage slice 668.

var _ = Describe("BR-AUDIT-006 / BR-WORKFLOW-006 / BR-AUTH-001: AuthWebhook coverage 668", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("BR-AUDIT-006: notification audit ogen enum converters (delete paths)", func() {
		It("BR-AUDIT-006: NotificationRequestDeleteHandler maps Spec.Type and Spec.Priority into NotificationAuditPayload", func() {
			mockStore := &MockAuditStore{}
			handler := authwebhook.NewNotificationRequestDeleteHandler(mockStore)

			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nr-del-668", Namespace: "default", UID: "nr-uid-668",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeCompletion,
					Priority: notificationv1.NotificationPriorityMedium,
					Subject:  "subject",
					Body:     "body",
				},
			}
			raw, err := json.Marshal(nr)
			Expect(err).NotTo(HaveOccurred())

			admReq := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					OldObject: runtime.RawExtension{Raw: raw},
					UserInfo: authv1.UserInfo{
						Username: "operator@kubernaut.ai",
						UID:      "op-668",
						Groups:   []string{"system:authenticated"},
					},
				},
			}

			resp := handler.Handle(ctx, admReq)
			Expect(resp.Allowed).To(BeTrue())
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			payload, ok := mockStore.StoredEvents[0].EventData.GetNotificationAuditPayload()
			Expect(ok).To(BeTrue())

			typ, ok := payload.GetType().Get()
			Expect(ok).To(BeTrue())
			Expect(typ).To(Equal(ogenclient.NotificationAuditPayloadTypeCompletion))

			prio, ok := payload.GetPriority().Get()
			Expect(ok).To(BeTrue())
			Expect(prio).To(Equal(ogenclient.NotificationAuditPayloadPriorityMedium))
		})

		It("BR-AUDIT-006: NotificationRequestValidator maps notification type, priority, and status phase into audit payload", func() {
			mockStore := &MockAuditStore{}
			v := authwebhook.NewNotificationRequestValidator(mockStore)

			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nr-val-668", Namespace: "prod", UID: "nr-uid-val-668",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeManualReview,
					Priority: notificationv1.NotificationPriorityCritical,
					Subject:  "review",
					Body:     "please review",
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase: notificationv1.NotificationPhaseFailed,
				},
			}

			admReq := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					UserInfo: authv1.UserInfo{
						Username: "reviewer@kubernaut.ai",
						UID:      "rev-668",
					},
				},
			}
			ctxWithReq := admission.NewContextWithRequest(ctx, admReq)

			warn, err := v.ValidateDelete(ctxWithReq, nr)
			Expect(err).NotTo(HaveOccurred())
			Expect(warn).To(BeEmpty())
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			payload, ok := mockStore.StoredEvents[0].EventData.GetNotificationAuditPayload()
			Expect(ok).To(BeTrue())

			nt, ok := payload.GetNotificationType().Get()
			Expect(ok).To(BeTrue())
			Expect(nt).To(Equal(ogenclient.NotificationAuditPayloadNotificationTypeManualReview))

			prio, ok := payload.GetPriority().Get()
			Expect(ok).To(BeTrue())
			Expect(prio).To(Equal(ogenclient.NotificationAuditPayloadPriorityCritical))

			fs, ok := payload.GetFinalStatus().Get()
			Expect(ok).To(BeTrue())
			Expect(fs).To(Equal(ogenclient.NotificationAuditPayloadFinalStatusFailed))
		})
	})

	Describe("BR-AUDIT-006: BuildRARApprovalAuditEvent", func() {
		It("BR-AUDIT-006: returns error when RemediationApprovalRequest is nil", func() {
			_, err := authwebhook.BuildRARApprovalAuditEvent(nil, "user@x", "rr-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil"))
		})

		It("BR-AUDIT-006: returns error when authenticated user is empty", func() {
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rar-668"},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					AIAnalysisRef: remediationv1.ObjectRef{Name: "ai-1"},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision:        remediationv1.ApprovalDecisionApproved,
					DecidedAt:       &metav1.Time{Time: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
					DecisionMessage: "ok",
				},
			}
			_, err := authwebhook.BuildRARApprovalAuditEvent(rar, "", "rr-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("authenticated user"))
		})

		It("BR-AUDIT-006: returns error when parent RemediationRequest name is empty", func() {
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rar-668"},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					AIAnalysisRef: remediationv1.ObjectRef{Name: "ai-1"},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision:        remediationv1.ApprovalDecisionRejected,
					DecidedAt:       &metav1.Time{Time: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
					DecisionMessage: "no",
				},
			}
			_, err := authwebhook.BuildRARApprovalAuditEvent(rar, "user@x", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parent RR"))
		})

		It("BR-AUDIT-006: builds audit event with structured RemediationApprovalAuditPayload", func() {
			rar := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rar-build-668"},
				Spec: remediationv1.RemediationApprovalRequestSpec{
					AIAnalysisRef: remediationv1.ObjectRef{Name: "ai-analysis-668"},
				},
				Status: remediationv1.RemediationApprovalRequestStatus{
					Decision:        remediationv1.ApprovalDecisionApproved,
					DecidedAt:       &metav1.Time{Time: time.Date(2026, 4, 2, 15, 30, 0, 0, time.UTC)},
					DecisionMessage: "approved for rollout",
				},
			}

			event, err := authwebhook.BuildRARApprovalAuditEvent(rar, "lead@kubernaut.ai", "rr-parent-668")
			Expect(err).NotTo(HaveOccurred())
			Expect(event).NotTo(BeNil())

			payload, ok := event.EventData.GetRemediationApprovalAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.RequestName).To(Equal("rar-build-668"))
			Expect(payload.Decision).To(Equal(ogenclient.RemediationApprovalAuditPayloadDecisionApproved))
			Expect(payload.DecisionMessage).To(Equal("approved for rollout"))
			Expect(payload.AiAnalysisRef).To(Equal("ai-analysis-668"))
			Expect(payload.DecidedAt).To(BeTemporally("==", rar.Status.DecidedAt.Time))
			Expect(string(payload.EventType)).To(Equal(authwebhook.EventTypeRARDecided))
		})
	})

	Describe("BR-WORKFLOW-006 / BR-AUTH-001: NewDSClientAdapter", func() {
		It("BR-WORKFLOW-006: returns error when base URL is empty", func() {
			_, err := authwebhook.NewDSClientAdapter("", 2*time.Second, logr.Discard())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("baseURL"))
		})

		It("BR-WORKFLOW-006: constructs a non-nil adapter for a valid Data Storage base URL", func() {
			adapter, err := authwebhook.NewDSClientAdapter("http://127.0.0.1:59999", 0, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})
	})
})

// UT-AW-668-003: StartupReconciler and Validator lifecycle (BR-AUTH-001)
var _ = Describe("UT-AW-668-003: StartupReconciler and Validator lifecycle (BR-AUTH-001)", func() {
	It("BR-AUTH-001: NeedLeaderElection should return false for multi-replica consistency", func() {
		r := &authwebhook.StartupReconciler{
			Logger: logr.Discard(),
		}
		Expect(r.NeedLeaderElection()).To(BeFalse())
	})

	It("BR-AUTH-001: NotificationRequestValidator.ValidateCreate should allow all creates", func() {
		v := authwebhook.NewNotificationRequestValidator(nil)
		warnings, err := v.ValidateCreate(context.Background(), &notificationv1.NotificationRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeNil())
	})

	It("BR-AUTH-001: NotificationRequestValidator.ValidateUpdate should allow all updates", func() {
		v := authwebhook.NewNotificationRequestValidator(nil)
		warnings, err := v.ValidateUpdate(context.Background(),
			&notificationv1.NotificationRequest{},
			&notificationv1.NotificationRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(warnings).To(BeNil())
	})
})

// UT-AW-668-004: ForceDisableActionType via HTTP mock (BR-WORKFLOW-006)
var _ = Describe("UT-AW-668-004: ForceDisableActionType via HTTP mock (BR-WORKFLOW-006)", func() {
	It("BR-WORKFLOW-006: returns success for HTTP 200", func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPatch))
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		adapter, err := authwebhook.NewDSClientAdapter(ts.URL, 0, logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		result, err := adapter.ForceDisableActionType(context.Background(), "my-at", "system", []string{"wf-orphan"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disabled).To(BeTrue())
	})

	It("BR-WORKFLOW-006: returns conflict details for HTTP 409", func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = fmt.Fprintf(w, `{"dependentWorkflowCount":2,"dependentWorkflows":["wf-a","wf-b"]}`)
		}))
		defer ts.Close()

		adapter, err := authwebhook.NewDSClientAdapter(ts.URL, 0, logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		result, err := adapter.ForceDisableActionType(context.Background(), "my-at", "system", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disabled).To(BeFalse())
		Expect(result.DependentWorkflowCount).To(Equal(2))
	})

	It("BR-WORKFLOW-006: returns success for HTTP 404 (already gone)", func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		adapter, err := authwebhook.NewDSClientAdapter(ts.URL, 0, logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		result, err := adapter.ForceDisableActionType(context.Background(), "my-at", "system", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Disabled).To(BeTrue())
	})

	It("BR-WORKFLOW-006: returns error for unexpected status codes", func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		adapter, err := authwebhook.NewDSClientAdapter(ts.URL, 0, logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		_, err = adapter.ForceDisableActionType(context.Background(), "my-at", "system", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unexpected status"))
	})

	It("BR-WORKFLOW-006: rejects call when adapter missing baseURL/httpClient", func() {
		adapter, err := authwebhook.NewDSClientAdapter("", 0, logr.Discard())
		Expect(err).To(HaveOccurred())

		if adapter != nil {
			_, err = adapter.ForceDisableActionType(context.Background(), "x", "system", nil)
			Expect(err).To(HaveOccurred())
		}
	})
})

// UT-AW-668-005: ValidateApprovalDecision and forgery detection (BR-AUDIT-006)
var _ = Describe("UT-AW-668-005: Validation and Forgery Detection (BR-AUDIT-006)", func() {
	It("BR-AUDIT-006: ValidateApprovalDecision rejects invalid decisions", func() {
		err := authwebhook.ValidateApprovalDecision(remediationv1.ApprovalDecision("DoSomethingElse"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid decision"))
	})

	It("BR-AUDIT-006: ValidateApprovalDecision accepts Approved", func() {
		Expect(authwebhook.ValidateApprovalDecision(remediationv1.ApprovalDecisionApproved)).NotTo(HaveOccurred())
	})

	It("BR-AUDIT-006: ValidateApprovalDecision accepts Rejected", func() {
		Expect(authwebhook.ValidateApprovalDecision(remediationv1.ApprovalDecisionRejected)).NotTo(HaveOccurred())
	})

	It("BR-AUTH-001: DetectAndLogForgeryAttempt returns true when DecidedBy is user-provided", func() {
		detected := authwebhook.DetectAndLogForgeryAttempt(logr.Discard(), "attacker@evil.com", "admin@kubernaut.ai")
		Expect(detected).To(BeTrue())
	})

	It("BR-AUTH-001: DetectAndLogForgeryAttempt returns false when DecidedBy is empty", func() {
		detected := authwebhook.DetectAndLogForgeryAttempt(logr.Discard(), "", "admin@kubernaut.ai")
		Expect(detected).To(BeFalse())
	})
})
