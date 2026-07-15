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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/testutil"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// #1111 / #1114: IT audit event coverage for RemediationWorkflow and ActionType
// webhook admission events. All audit events flow through real DS (Podman Postgres).
//
// Business Requirements: BR-AUDIT-005, ADR-058, ADR-059
// Correlation ID: string(req.UID) per buildAuditEnvelope in pkg/authwebhook/audit_helpers.go

var _ = Describe("#1111 RW/AT Webhook Admission Audit Events", Label("integration", "authwebhook", "audit"), func() {

	var (
		rwHandler *authwebhook.RemediationWorkflowHandler
		atHandler *authwebhook.ActionTypeHandler
	)

	BeforeEach(func() {
		logger := ctrl.Log.WithName("rw-at-audit-it")
		rwDSClient := authwebhook.NewDSClientAdapterFromClient(dsClient, logger.WithName("rw-ds"))
		atDSClient := authwebhook.NewDSClientAdapterFromClient(dsClient, logger.WithName("at-ds"))

		rwHandler = authwebhook.NewRemediationWorkflowHandler(rwDSClient, auditStore, k8sClient)
		atHandler = authwebhook.NewActionTypeHandler(atDSClient, auditStore, k8sClient)
	})

	flushAndQuery := func(correlationID, eventType string) []ogenclient.AuditEvent {
		flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
		defer flushCancel()
		err := auditStore.Flush(flushCtx)
		Expect(err).ToNot(HaveOccurred(), "Audit store flush should succeed")

		var events []ogenclient.AuditEvent
		Eventually(func() bool {
			found, qErr := queryAuditEvents(dsClient, correlationID, &eventType)
			if qErr != nil {
				return false
			}
			events = found
			return len(events) > 0
		}, 15*time.Second, 1*time.Second).Should(BeTrue(),
			fmt.Sprintf("Expected %s audit event with correlation_id=%s", eventType, correlationID))
		return events
	}

	uniqueID := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	buildRW := func(name, actionType string) *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubernaut.ai/v1alpha1",
				Kind:       "RemediationWorkflow",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: actionType,
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT audit test workflow",
					WhenToUse: "For webhook audit integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Pod"},
					Priority:    "P1",
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: "job",
					Bundle: testutil.ValidBundleRef,
				},
				Parameters: []rwv1alpha1.RemediationWorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				},
			},
		}
	}

	buildAT := func(name string) *atv1alpha1.ActionType {
		return &atv1alpha1.ActionType{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubernaut.ai/v1alpha1",
				Kind:       "ActionType",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: name,
				Description: atv1alpha1.ActionTypeDescription{
					What:      "IT audit test action type",
					WhenToUse: "For webhook audit integration testing",
				},
			},
		}
	}

	rwAdmissionRequest := func(op admissionv1.Operation, rw *rwv1alpha1.RemediationWorkflow, uid string) admission.Request {
		rwJSON, err := json.Marshal(rw)
		Expect(err).ToNot(HaveOccurred())

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: types.UID(uid),
				Kind: metav1.GroupVersionKind{
					Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationWorkflow",
				},
				Name:      rw.Name,
				Namespace: rw.Namespace,
				Operation: op,
				UserInfo: authv1.UserInfo{
					Username: "it-audit-user@kubernaut.ai",
					UID:      "it-audit-uid",
					Groups:   []string{"system:masters"},
				},
			},
		}
		switch op {
		case admissionv1.Create:
			req.Object = runtime.RawExtension{Raw: rwJSON}
		case admissionv1.Update:
			req.Object = runtime.RawExtension{Raw: rwJSON}
			req.OldObject = runtime.RawExtension{Raw: rwJSON}
		case admissionv1.Delete:
			req.OldObject = runtime.RawExtension{Raw: rwJSON}
		}
		return req
	}

	atAdmissionRequest := func(op admissionv1.Operation, at *atv1alpha1.ActionType, uid string) admission.Request {
		atJSON, err := json.Marshal(at)
		Expect(err).ToNot(HaveOccurred())

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: types.UID(uid),
				Kind: metav1.GroupVersionKind{
					Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
				},
				Name:      at.Name,
				Namespace: at.Namespace,
				Operation: op,
				UserInfo: authv1.UserInfo{
					Username: "it-audit-user@kubernaut.ai",
					UID:      "it-audit-uid",
					Groups:   []string{"system:masters"},
				},
			},
		}
		switch op {
		case admissionv1.Create:
			req.Object = runtime.RawExtension{Raw: atJSON}
		case admissionv1.Update:
			req.Object = runtime.RawExtension{Raw: atJSON}
			req.OldObject = runtime.RawExtension{Raw: atJSON}
		case admissionv1.Delete:
			req.OldObject = runtime.RawExtension{Raw: atJSON}
		}
		return req
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// RemediationWorkflow admission audit events (ADR-058)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("RemediationWorkflow admission audit events (ADR-058)", func() {

		It("IT-AW-1111-001: remediationworkflow.admitted.create persisted to DS", func() {
			uid := uniqueID("rw-create")
			actionType := uniqueActionType("ITScaleCreate")

			// DD-WORKFLOW-016: Register the action type in the taxonomy before
			// creating the RW, otherwise DS rejects with 403 (FK constraint).
			// #1661: Also creates the ActionType CRD in etcd (Active), required
			// by AW's own RW-to-ActionType existence gate.
			createActiveActionTypeCRD(ctx, k8sClient, atHandler, buildAT(actionType), uniqueID("at-setup-create"))

			rw := buildRW(uniqueID("it-rw-create"), actionType)

			resp := rwHandler.Handle(ctx, rwAdmissionRequest(admissionv1.Create, rw, uid))
			Expect(resp.Allowed).To(BeTrue(), "RW CREATE should be allowed: %s", resp.Result)

			events := flushAndQuery(uid, authwebhook.EventTypeRWAdmittedCreate)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(uid))

			// IT-AW-311-001 (#1661 Change 2): workflow_content/content_hash must
			// survive the full round-trip — AW emit -> audit store -> DS Postgres
			// persistence -> query API -> JSONB decode — proving the audit trail
			// alone can reconstruct the exact workflow definition (SOC2 CC8.1).
			payload, ok := events[0].EventData.GetRemediationWorkflowWebhookAuditPayload()
			Expect(ok).To(BeTrue(), "event_data should decode as RemediationWorkflowWebhookAuditPayload")
			Expect(payload.WorkflowContent.IsSet()).To(BeTrue(),
				"workflow_content should be queryable from DS after round-trip persistence")
			Expect(payload.WorkflowContent.Value.Version).To(Equal(rw.Spec.Version))
			Expect(payload.WorkflowContent.Value.ActionType).To(Equal(rw.Spec.ActionType))
			Expect(payload.WorkflowContent.Value.Parameters).To(HaveLen(1))
			Expect(payload.WorkflowContent.Value.Parameters[0].Name).To(Equal("NAMESPACE"))
			Expect(payload.ContentHash.IsSet()).To(BeTrue())
			Expect(payload.ContentHash.Value).To(MatchRegexp("^[0-9a-f]{64}$"),
				"content_hash must be a SHA-256 hex digest (64 lowercase hex chars)")
		})

		It("IT-AW-1111-002: remediationworkflow.admitted.update persisted to DS", func() {
			uid := uniqueID("rw-update")
			actionType := uniqueActionType("ITScaleUpdate")

			createActiveActionTypeCRD(ctx, k8sClient, atHandler, buildAT(actionType), uniqueID("at-setup-update"))

			rw := buildRW(uniqueID("it-rw-update"), actionType)

			resp := rwHandler.Handle(ctx, rwAdmissionRequest(admissionv1.Update, rw, uid))
			Expect(resp.Allowed).To(BeTrue(), "RW UPDATE should be allowed: %s", resp.Result)

			events := flushAndQuery(uid, authwebhook.EventTypeRWAdmittedUpdate)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(uid))
		})

		It("IT-AW-1111-003: remediationworkflow.admitted.delete persisted to DS", func() {
			actionType := uniqueActionType("ITScaleDelete")

			createActiveActionTypeCRD(ctx, k8sClient, atHandler, buildAT(actionType), uniqueID("at-setup-delete"))

			rw := buildRW(uniqueID("it-rw-delete"), actionType)

			createUID := uniqueID("rw-pre-delete")
			createResp := rwHandler.Handle(ctx, rwAdmissionRequest(admissionv1.Create, rw, createUID))
			Expect(createResp.Allowed).To(BeTrue(), "Pre-delete CREATE should succeed")

			deleteUID := uniqueID("rw-delete")
			resp := rwHandler.Handle(ctx, rwAdmissionRequest(admissionv1.Delete, rw, deleteUID))
			Expect(resp.Allowed).To(BeTrue(), "RW DELETE should be allowed")

			events := flushAndQuery(deleteUID, authwebhook.EventTypeRWAdmittedDelete)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(deleteUID))
		})

		It("IT-AW-1111-004: remediationworkflow.admitted.denied persisted on unmarshal failure", func() {
			uid := uniqueID("rw-denied")

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID(uid),
					Kind: metav1.GroupVersionKind{
						Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationWorkflow",
					},
					Name:      "it-rw-denied",
					Namespace: "default",
					Operation: admissionv1.Create,
					UserInfo: authv1.UserInfo{
						Username: "it-audit-user@kubernaut.ai",
						UID:      "it-audit-uid",
					},
					Object: runtime.RawExtension{Raw: []byte(`{invalid json}`)},
				},
			}

			resp := rwHandler.Handle(ctx, req)
			Expect(resp.Allowed).To(BeFalse(), "RW CREATE with malformed JSON should be denied")

			events := flushAndQuery(uid, authwebhook.EventTypeRWAdmittedDenied)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(uid))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// ActionType admission audit events (ADR-059)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("ActionType admission audit events (ADR-059)", func() {

		It("IT-AW-1111-005: actiontype.admitted.create persisted to DS", func() {
			uid := uniqueID("at-create")
			at := buildAT(uniqueID("ITCreate"))

			resp := atHandler.Handle(ctx, atAdmissionRequest(admissionv1.Create, at, uid))
			Expect(resp.Allowed).To(BeTrue(), "AT CREATE should be allowed: %s", resp.Result)

			events := flushAndQuery(uid, authwebhook.EventTypeATAdmittedCreate)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(uid))
		})

		It("IT-AW-1111-006: actiontype.admitted.update persisted to DS", func() {
			atName := uniqueID("ITUpdate")
			at := buildAT(atName)

			createUID := uniqueID("at-pre-update")
			createResp := atHandler.Handle(ctx, atAdmissionRequest(admissionv1.Create, at, createUID))
			Expect(createResp.Allowed).To(BeTrue(), "Pre-update CREATE should succeed")

			updatedAT := buildAT(atName)
			updatedAT.Spec.Description.What = "Updated description for audit test"

			updateUID := uniqueID("at-update")
			oldJSON, err := json.Marshal(at)
			Expect(err).ToNot(HaveOccurred())
			newJSON, err := json.Marshal(updatedAT)
			Expect(err).ToNot(HaveOccurred())
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID(updateUID),
					Kind: metav1.GroupVersionKind{
						Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
					},
					Name:      atName,
					Namespace: "default",
					Operation: admissionv1.Update,
					UserInfo: authv1.UserInfo{
						Username: "it-audit-user@kubernaut.ai",
						UID:      "it-audit-uid",
					},
					Object:    runtime.RawExtension{Raw: newJSON},
					OldObject: runtime.RawExtension{Raw: oldJSON},
				},
			}

			resp := atHandler.Handle(ctx, req)
			Expect(resp.Allowed).To(BeTrue(), "AT UPDATE should be allowed: %s", resp.Result)

			events := flushAndQuery(updateUID, authwebhook.EventTypeATAdmittedUpdate)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(updateUID))
		})

		It("IT-AW-1111-007: actiontype.admitted.delete persisted to DS", func() {
			atName := uniqueID("ITDelete")
			at := buildAT(atName)

			createUID := uniqueID("at-pre-del")
			createResp := atHandler.Handle(ctx, atAdmissionRequest(admissionv1.Create, at, createUID))
			Expect(createResp.Allowed).To(BeTrue(), "Pre-delete CREATE should succeed")

			deleteUID := uniqueID("at-delete")
			resp := atHandler.Handle(ctx, atAdmissionRequest(admissionv1.Delete, at, deleteUID))
			Expect(resp.Allowed).To(BeTrue(), "AT DELETE should be allowed: %s", resp.Result)

			events := flushAndQuery(deleteUID, authwebhook.EventTypeATAdmittedDelete)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(deleteUID))
		})

		// AT denied events: unlike RW which emits denied audit on unmarshal failure,
		// AT handlers only emit denied audit when DS operations fail or business
		// rules reject the request. These tests trigger the correct code paths.

		It("IT-AW-1111-008: actiontype.denied.update persisted on spec.name immutability violation", func() {
			// AT handleUpdate checks spec.name immutability before calling DS.
			// Changing spec.name triggers emitATDeniedAudit directly (line 132).
			uid := uniqueID("at-denied-update")

			oldAT := buildAT("OriginalName")
			newAT := buildAT("ChangedName")

			oldJSON, err := json.Marshal(oldAT)
			Expect(err).ToNot(HaveOccurred())
			newJSON, err := json.Marshal(newAT)
			Expect(err).ToNot(HaveOccurred())

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					UID: types.UID(uid),
					Kind: metav1.GroupVersionKind{
						Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
					},
					Name:      "at-immutable-test",
					Namespace: "default",
					Operation: admissionv1.Update,
					UserInfo: authv1.UserInfo{
						Username: "it-audit-user@kubernaut.ai",
						UID:      "it-audit-uid",
					},
					Object:    runtime.RawExtension{Raw: newJSON},
					OldObject: runtime.RawExtension{Raw: oldJSON},
				},
			}

			resp := atHandler.Handle(ctx, req)
			Expect(resp.Allowed).To(BeFalse(), "AT UPDATE with changed spec.name should be denied")

			events := flushAndQuery(uid, authwebhook.EventTypeATDeniedUpdate)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(uid))
		})

		It("IT-AW-1111-009: actiontype.denied.delete persisted on active dependents", func() {
			// #1661 Change 8d: the K8s-native live RemediationWorkflow list is
			// now the SOLE dependents gate (DS's Postgres-backed tracking is
			// gone). A dependent RW must therefore exist as a real CRD in
			// etcd -- rwHandler.Handle alone (direct invocation, bypassing
			// the real API server) never persists one -- so this test
			// creates the RW via k8sClient.Create directly, mirroring
			// createActiveActionTypeCRD's precedent for the AT side.
			atName := uniqueActionType("ITDeniedDel")
			at := buildAT(atName)
			createActiveActionTypeCRD(ctx, k8sClient, atHandler, at, uniqueID("at-dep-create"))

			rw := buildRW(uniqueID("it-rw-dependent"), atName)
			Expect(k8sClient.Create(ctx, rw)).To(Succeed(), "dependent RW CRD creation should succeed")

			deleteUID := uniqueID("at-denied-del")
			resp := atHandler.Handle(ctx, atAdmissionRequest(admissionv1.Delete, at, deleteUID))
			Expect(resp.Allowed).To(BeFalse(), "AT DELETE should be denied due to active dependents")

			events := flushAndQuery(deleteUID, authwebhook.EventTypeATDeniedDelete)
			Expect(events).To(HaveLen(1))
			Expect(events[0].CorrelationID).To(Equal(deleteUID))
		})

		// IT-AW-1111-010 ("actiontype.denied.create persisted on DS
		// registration failure") is removed: #1661 Change 8d deleted
		// ActionTypeHandler's DS round-trip entirely -- handleCreate no
		// longer calls DS (or performs any local spec.name emptiness check
		// of its own; that format constraint is enforced structurally by
		// the apiserver's kubebuilder:validation:Pattern on
		// ActionTypeSpec.Name, proven by IT-CRD-312-004a/b in
		// crd_pattern_validation_test.go). There is no DS registration
		// failure path left for a direct handler.Handle invocation to
		// exercise.
	})
})
