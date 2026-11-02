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

package authwebhook_test

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type approvalSafetyClock struct{ now time.Time }

func (c approvalSafetyClock) Now() time.Time { return c.now }

var _ = Describe("RAR approval safety admission (BR-ORCH-026)", func() {
	var (
		ctx     context.Context
		handler *authwebhook.RemediationApprovalRequestAuthHandler
		store   *MockAuditStore
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = &MockAuditStore{}
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(store, nil, "kubernaut-system")
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(handler.InjectDecoder(admission.NewDecoder(scheme))).To(Succeed())
	})

	It("IT-AW-2564-005: rejects a decision for a terminating RAR after authentication and attributes the denial", func() {
		deletionTime := metav1.NewTime(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
		rar := safetyRAR(remediationv1.ApprovalDecisionApproved, "operator@example.com", time.Time{})
		rar.DeletionTimestamp = &deletionTime

		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), "operator@example.com")

		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring("terminating"))
		Expect(store.StoredEvents).To(HaveLen(1))
		Expect(store.StoredEvents[0].ActorID.Value).To(Equal("operator@example.com"))
		Expect(store.StoredEvents[0].EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))
	})

	It("IT-AW-2564-005: denied AF delegation records the intermediary as the actor", func() {
		deletionTime := metav1.NewTime(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
		rar := safetyRAR(remediationv1.ApprovalDecisionApproved, "delegated@example.com", time.Time{})
		rar.DeletionTimestamp = &deletionTime
		trustedAF := authwebhook.BuildTrustedAFSA("kubernaut-system")

		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), trustedAF)

		Expect(resp.Allowed).To(BeFalse())
		Expect(store.StoredEvents).To(HaveLen(1))
		Expect(store.StoredEvents[0].ActorType.Value).To(Equal("service"))
		Expect(store.StoredEvents[0].ActorID.Value).To(Equal(trustedAF))
	})

	DescribeTable("UT-AW-2564-011: rejects client-submitted system-only decisions", func(decision remediationv1.ApprovalDecision) {
		rar := safetyRAR(decision, "system", time.Time{})
		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), "operator@example.com")

		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring("system"))
	},
		Entry("Expired", remediationv1.ApprovalDecisionExpired),
		Entry("Cancelled", remediationv1.ApprovalDecisionCancelled),
	)

	It("IT-AW-2564-016: accepts Expired only from the trusted RO service account", func() {
		rar := safetyRAR(remediationv1.ApprovalDecisionExpired, "system", time.Time{})
		trusted := "system:serviceaccount:kubernaut-system:remediationorchestrator-controller"

		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), trusted)

		Expect(resp.Allowed).To(BeTrue())
	})

	It("IT-AW-2564-016: preserves the trusted RO expiry timestamp", func() {
		decidedAt := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
		handler.SetClock(approvalSafetyClock{now: decidedAt.Add(time.Second)})
		rar := safetyRAR(remediationv1.ApprovalDecisionExpired, "system", decidedAt)
		trusted := authwebhook.BuildTrustedROSA("kubernaut-system")

		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), trusted)

		Expect(resp.Allowed).To(BeTrue())
		Expect(store.StoredEvents).To(HaveLen(1))
		payload, ok := store.StoredEvents[0].EventData.GetRemediationApprovalAuditPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.DecidedAt).To(BeTemporally("==", decidedAt),
			"the webhook must not move the trusted RO decision timestamp forward")
	})

	It("IT-AW-2564-015: authenticates before returning a terminal-state response and rejects tuple mutation", func() {
		decidedAt := time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)
		oldRAR := safetyRAR(remediationv1.ApprovalDecisionApproved, "original@example.com", decidedAt)
		oldRAR.Status.DecisionMessage = "original reason"
		newRAR := oldRAR.DeepCopy()
		newRAR.Status.DecidedBy = "attacker@example.com"

		resp := handleSafetyAdmission(ctx, handler, newRAR, oldRAR, "attacker@example.com")

		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring("terminal"))
		Expect(store.StoredEvents).To(HaveLen(1))
	})

	DescribeTable("IT-AW-2564-015: persists terminal approval conditions with the decision", func(decision remediationv1.ApprovalDecision, reason string) {
		rar := safetyRAR(decision, "", time.Time{})
		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), "operator@example.com")

		Expect(resp.Allowed).To(BeTrue())
		conditions := patchedApprovalConditions(resp)
		Expect(meta.FindStatusCondition(conditions, "ApprovalPending")).To(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionFalse)),
		))
		Expect(meta.FindStatusCondition(conditions, "ApprovalDecided")).To(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionTrue)),
			WithTransform(func(condition *metav1.Condition) string { return condition.Reason }, Equal(reason)),
		))
		Expect(meta.FindStatusCondition(conditions, "Ready")).To(SatisfyAll(
			Not(BeNil()),
			WithTransform(func(condition *metav1.Condition) metav1.ConditionStatus { return condition.Status }, Equal(metav1.ConditionTrue)),
		))
	},
		Entry("Approved", remediationv1.ApprovalDecisionApproved, "Approved"),
		Entry("Rejected", remediationv1.ApprovalDecisionRejected, "Rejected"),
	)

	It("IT-AW-2564-015: bounds the rejected condition while retaining the full decision message", func() {
		fullReason := strings.Repeat("拒", 40000)
		rar := safetyRAR(remediationv1.ApprovalDecisionRejected, "", time.Time{})
		rar.Status.DecisionMessage = fullReason

		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), "operator@example.com")

		Expect(resp.Allowed).To(BeTrue())
		condition := meta.FindStatusCondition(patchedApprovalConditions(resp), "ApprovalDecided")
		Expect(condition).NotTo(BeNil())
		Expect([]rune(condition.Message)).To(HaveLen(32768))
		Expect(condition.Message).To(HavePrefix("Rejected by operator@example.com: "))
		Expect(store.StoredEvents).To(HaveLen(1))
		payload, ok := store.StoredEvents[0].EventData.GetRemediationApprovalAuditPayload()
		Expect(ok).To(BeTrue())
		Expect(payload.DecisionMessage).To(Equal(fullReason))
	})

	It("IT-AW-2564-015: trusted RO cannot mutate a decision-bearing terminal condition", func() {
		decidedAt := time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)
		oldRAR := safetyRAR(remediationv1.ApprovalDecisionApproved, "operator@example.com", decidedAt)
		oldRAR.Status.Conditions = []metav1.Condition{{
			Type:               "ApprovalDecided",
			Status:             metav1.ConditionTrue,
			Reason:             "Approved",
			Message:            "approved",
			ObservedGeneration: oldRAR.Generation,
		}}
		newRAR := oldRAR.DeepCopy()
		newRAR.Status.Conditions[0].Status = metav1.ConditionFalse

		resp := handleSafetyAdmission(ctx, handler, newRAR, oldRAR, authwebhook.BuildTrustedROSA("kubernaut-system"))

		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring("terminal"))
	})

	It("IT-AW-2564-015: trusted RO may add a valid AuditRecorded condition", func() {
		decidedAt := time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)
		oldRAR := safetyRAR(remediationv1.ApprovalDecisionApproved, "operator@example.com", decidedAt)
		newRAR := oldRAR.DeepCopy()
		newRAR.Status.Conditions = append(newRAR.Status.Conditions, metav1.Condition{
			Type:               "AuditRecorded",
			Status:             metav1.ConditionTrue,
			Reason:             "AuditSucceeded",
			Message:            "approval audit event accepted by audit buffer",
			ObservedGeneration: newRAR.Generation,
			LastTransitionTime: metav1.Now(),
		})

		resp := handleSafetyAdmission(ctx, handler, newRAR, oldRAR, authwebhook.BuildTrustedROSA("kubernaut-system"))

		Expect(resp.Allowed).To(BeTrue())
	})

	It("IT-AW-2564-015: trusted RO cannot forge AuditRecorded success metadata", func() {
		decidedAt := time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)
		oldRAR := safetyRAR(remediationv1.ApprovalDecisionApproved, "operator@example.com", decidedAt)
		newRAR := oldRAR.DeepCopy()
		newRAR.Status.Conditions = append(newRAR.Status.Conditions, metav1.Condition{
			Type:               "AuditRecorded",
			Status:             metav1.ConditionTrue,
			Reason:             "AuditFailed",
			Message:            "not a successful audit",
			ObservedGeneration: newRAR.Generation,
			LastTransitionTime: metav1.Now(),
		})

		resp := handleSafetyAdmission(ctx, handler, newRAR, oldRAR, authwebhook.BuildTrustedROSA("kubernaut-system"))

		Expect(resp.Allowed).To(BeFalse())
		Expect(resp.Result.Message).To(ContainSubstring("terminal"))
	})

	DescribeTable("UT-AW-2564-011: enforces a strict injected deadline", func(now time.Time, allowed bool) {
		handler.SetClock(approvalSafetyClock{now: now})
		requiredBy := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
		rar := safetyRARWithRequiredBy(remediationv1.ApprovalDecisionApproved, "", requiredBy)

		resp := handleSafetyAdmission(ctx, handler, rar, safetyPendingRAR(rar), "operator@example.com")

		Expect(resp.Allowed).To(Equal(allowed))
	},
		Entry("strictly before", time.Date(2026, 11, 2, 11, 59, 59, 0, time.UTC), true),
		Entry("exactly at", time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), false),
		Entry("after", time.Date(2026, 11, 2, 12, 0, 1, 0, time.UTC), false),
	)
})

func safetyRAR(decision remediationv1.ApprovalDecision, decidedBy string, decidedAt time.Time) *remediationv1.RemediationApprovalRequest {
	rar := safetyRARWithRequiredBy(decision, decidedBy, time.Now().Add(15*time.Minute))
	if !decidedAt.IsZero() {
		t := metav1.NewTime(decidedAt)
		rar.Status.DecidedAt = &t
	}
	return rar
}

func safetyRARWithRequiredBy(decision remediationv1.ApprovalDecision, decidedBy string, requiredBy time.Time) *remediationv1.RemediationApprovalRequest {
	rar := &remediationv1.RemediationApprovalRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "rar-safety", Namespace: "kubernaut-system", UID: "rar-safety-uid"},
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-safety", Namespace: "kubernaut-system"},
			AIAnalysisRef:         remediationv1.ObjectRef{Name: "ai-safety"},
			RequiredBy:            metav1.NewTime(requiredBy),
		},
		Status: remediationv1.RemediationApprovalRequestStatus{Decision: decision, DecidedBy: decidedBy},
	}
	return rar
}

func safetyPendingRAR(rar *remediationv1.RemediationApprovalRequest) *remediationv1.RemediationApprovalRequest {
	pending := rar.DeepCopy()
	pending.Status = remediationv1.RemediationApprovalRequestStatus{}
	return pending
}

func handleSafetyAdmission(ctx context.Context, handler *authwebhook.RemediationApprovalRequestAuthHandler, rar, oldRAR *remediationv1.RemediationApprovalRequest, username string) admission.Response {
	raw, err := json.Marshal(rar)
	Expect(err).NotTo(HaveOccurred())
	oldRaw, err := json.Marshal(oldRAR)
	Expect(err).NotTo(HaveOccurred())

	return handler.Handle(ctx, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       "rar-safety-admission",
		Name:      rar.Name,
		Namespace: rar.Namespace,
		Operation: admissionv1.Update,
		Kind:      metav1.GroupVersionKind{Group: "remediation.kubernaut.ai", Version: "v1alpha1", Kind: "RemediationApprovalRequest"},
		UserInfo:  authv1.UserInfo{Username: username, UID: "safety-user-uid"},
		Object:    runtime.RawExtension{Raw: raw},
		OldObject: runtime.RawExtension{Raw: oldRaw},
	}})
}

func patchedApprovalConditions(resp admission.Response) []metav1.Condition {
	for _, patch := range resp.Patches {
		if patch.Path != "/status/conditions" {
			continue
		}
		raw, err := json.Marshal(patch.Value)
		Expect(err).NotTo(HaveOccurred())
		var conditions []metav1.Condition
		Expect(json.Unmarshal(raw, &conditions)).To(Succeed())
		return conditions
	}
	return nil
}
