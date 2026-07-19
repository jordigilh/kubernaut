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

// IT-AW-1511-001: CRD `cluster` labels round-trip through unmodified Authwebhook (BR-FLEET-003, #1511, AU-3).
//
// Authority: docs/tests/1511/TEST_PLAN.md, docs/requirements/BR-FLEET-003-cluster-scoped-workflow-targeting.md
//
// This test proves the "generic pass-through" claim in DD-FLEET-002: the AW
// RemediationWorkflow handler requires ZERO code changes for the new `cluster`
// label dimension because it marshals the entire CRD spec (including the new
// `Labels.Cluster` field) into its own admitted audit event's workflow_content
// (#1661 Change 8c/DD-WORKFLOW-018: AW no longer forwards to a DS catalog at
// all -- its own audit trail is the durable record of what was admitted).
// `workflowschema`/`schema.Parser.ExtractLabels` (already unit-tested in
// UT-DS-1511-002/003) remain responsible for DS's own discovery-side
// extraction once Change 5 (informer-backed cache) lands.
var _ = Describe("IT-AW-1511-001: RemediationWorkflow cluster label round-trip (BR-FLEET-003)", Label("integration", "authwebhook", "fleet"), func() {

	var (
		rwHandler *authwebhook.RemediationWorkflowHandler
		atHandler *authwebhook.ActionTypeHandler
	)

	BeforeEach(func() {
		logger := ctrl.Log.WithName("rw-cluster-it")
		rwDSClient := authwebhook.NewDSClientAdapterFromClient(dsClient, logger.WithName("rw-ds"))
		rwHandler = authwebhook.NewRemediationWorkflowHandler(rwDSClient, auditStore, k8sClient)
		atHandler = authwebhook.NewActionTypeHandler(auditStore, k8sClient)
	})

	uniqueName := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	buildAT := func(name string) *atv1alpha1.ActionType {
		return &atv1alpha1.ActionType{
			TypeMeta: metav1.TypeMeta{APIVersion: "kubernaut.ai/v1alpha1", Kind: "ActionType"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: name,
				Description: atv1alpha1.ActionTypeDescription{
					What:      "IT cluster label round-trip test action type",
					WhenToUse: "For BR-FLEET-003 / #1511 integration testing",
				},
			},
		}
	}

	buildRWWithCluster := func(name, actionType string, cluster []string) *rwv1alpha1.RemediationWorkflow {
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
					What:      "IT cluster label round-trip test workflow",
					WhenToUse: "For BR-FLEET-003 / #1511 integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Pod"},
					Priority:    "P1",
					Cluster:     cluster,
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

	rwAdmissionRequest := func(rw *rwv1alpha1.RemediationWorkflow, uid string) admission.Request {
		rwJSON, err := json.Marshal(rw)
		Expect(err).ToNot(HaveOccurred())

		return admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: types.UID(uid),
				Kind: metav1.GroupVersionKind{
					Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationWorkflow",
				},
				Name:      rw.Name,
				Namespace: rw.Namespace,
				Operation: admissionv1.Create,
				UserInfo: authv1.UserInfo{
					Username: "it-cluster-user@kubernaut.ai",
					UID:      "it-cluster-uid",
					Groups:   []string{"system:masters"},
				},
				Object: runtime.RawExtension{Raw: rwJSON},
			},
		}
	}

	// fetchContentLabelsByCorrelationID reads back the admitted audit event's
	// workflow_content.labels for the given admission UID. #1661 Change 8c:
	// AW no longer forwards to a DS catalog, so AW's own audit trail (not
	// DS's ListWorkflows) is the durable, queryable record of what a CRD's
	// spec.labels contained at admission time.
	fetchContentLabelsByCorrelationID := func(correlationID string) ogenclient.RemediationWorkflowContentLabels {
		flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
		defer flushCancel()
		Expect(auditStore.Flush(flushCtx)).To(Succeed(), "audit store flush should succeed")

		var payload ogenclient.RemediationWorkflowWebhookAuditPayload
		Eventually(func() bool {
			events, err := queryAuditEvents(dsClient, correlationID, nil)
			if err != nil || len(events) == 0 {
				return false
			}
			p, ok := events[0].EventData.GetRemediationWorkflowWebhookAuditPayload()
			if !ok || !p.WorkflowContent.IsSet() {
				return false
			}
			payload = p
			return true
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
			fmt.Sprintf("expected an admitted audit event with workflow_content for correlation_id=%s", correlationID))
		return payload.WorkflowContent.Value.Labels
	}

	It("IT-AW-1511-001a: cluster labels present on the CRD survive to the admitted audit event's workflow_content unmodified", func() {
		actionType := uniqueActionType("ITClusterCreate")
		// #1661: Creates the ActionType CRD in etcd (Active), required by AW's
		// own RW-to-ActionType existence gate.
		createActiveActionTypeCRD(ctx, k8sClient, atHandler, buildAT(actionType), uniqueName("at-setup-cluster"))

		rwName := uniqueName("it-rw-cluster")
		rw := buildRWWithCluster(rwName, actionType, []string{"production", "staging-eu"})

		createUID := uniqueName("rw-cluster-create")
		resp := rwHandler.Handle(ctx, rwAdmissionRequest(rw, createUID))
		Expect(resp.Allowed).To(BeTrue(), "RW CREATE with cluster labels should be allowed (zero AW code changes): %s", resp.Result)

		labels := fetchContentLabelsByCorrelationID(createUID)
		Expect(labels.Cluster).To(ConsistOf("production", "staging-eu"),
			"cluster labels submitted on the CRD must survive unmodified into the admitted audit event's workflow_content")
		// Regression guard: sibling mandatory dimensions are unaffected by the new field.
		Expect(labels.Priority).To(BeEquivalentTo("P1"))
	})

	It("IT-AW-1511-001b: absent cluster labels round-trip as empty (backward compatible, non-fleet)", func() {
		actionType := uniqueActionType("ITNoClusterCreate")
		createActiveActionTypeCRD(ctx, k8sClient, atHandler, buildAT(actionType), uniqueName("at-setup-nocluster"))

		rwName := uniqueName("it-rw-nocluster")
		rw := buildRWWithCluster(rwName, actionType, nil) // non-fleet: no cluster labels set

		createUID := uniqueName("rw-nocluster-create")
		resp := rwHandler.Handle(ctx, rwAdmissionRequest(rw, createUID))
		Expect(resp.Allowed).To(BeTrue(), "RW CREATE without cluster labels should be allowed: %s", resp.Result)

		labels := fetchContentLabelsByCorrelationID(createUID)
		Expect(labels.Cluster).To(BeEmpty(),
			"non-fleet workflows must round-trip with empty cluster labels, not an error or fabricated value")
	})
})
