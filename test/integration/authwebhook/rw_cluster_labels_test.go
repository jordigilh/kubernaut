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
// `Labels.Cluster` field) and forwards it to DS's inline registration endpoint,
// where `workflowschema`/`schema.Parser.ExtractLabels` (already unit-tested in
// UT-DS-1511-002/003) perform the actual extraction into the `labels` JSONB
// column.
var _ = Describe("IT-AW-1511-001: RemediationWorkflow cluster label round-trip (BR-FLEET-003)", Label("integration", "authwebhook", "fleet"), func() {

	var (
		rwHandler *authwebhook.RemediationWorkflowHandler
		atHandler *authwebhook.ActionTypeHandler
	)

	BeforeEach(func() {
		logger := ctrl.Log.WithName("rw-cluster-it")
		rwDSClient := authwebhook.NewDSClientAdapterFromClient(dsClient, logger.WithName("rw-ds"))
		atDSClient := authwebhook.NewDSClientAdapterFromClient(dsClient, logger.WithName("at-ds"))
		rwHandler = authwebhook.NewRemediationWorkflowHandler(rwDSClient, auditStore, k8sClient)
		atHandler = authwebhook.NewActionTypeHandler(atDSClient, auditStore, k8sClient)
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

	atAdmissionRequest := func(at *atv1alpha1.ActionType, uid string) admission.Request {
		atJSON, err := json.Marshal(at)
		Expect(err).ToNot(HaveOccurred())
		return admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID: types.UID(uid),
				Kind: metav1.GroupVersionKind{
					Group: "kubernaut.ai", Version: "v1alpha1", Kind: "ActionType",
				},
				Name:      at.Name,
				Namespace: at.Namespace,
				Operation: admissionv1.Create,
				UserInfo: authv1.UserInfo{
					Username: "it-cluster-user@kubernaut.ai",
					UID:      "it-cluster-uid",
				},
				Object: runtime.RawExtension{Raw: atJSON},
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

	fetchWorkflowByName := func(name string) ogenclient.RemediationWorkflow {
		var found ogenclient.RemediationWorkflow
		Eventually(func() bool {
			resp, err := dsClient.ListWorkflows(ctx, ogenclient.ListWorkflowsParams{
				WorkflowName: ogenclient.OptString{Value: name, Set: true},
			})
			if err != nil {
				return false
			}
			list, ok := resp.(*ogenclient.WorkflowListResponse)
			if !ok || len(list.Workflows) == 0 {
				return false
			}
			found = list.Workflows[0]
			return true
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
			fmt.Sprintf("expected workflow %q to be discoverable via DS ListWorkflows after AW registration", name))
		return found
	}

	It("IT-AW-1511-001a: cluster labels present on the CRD survive to DS's labels JSONB unmodified", func() {
		actionType := uniqueName("ITClusterCreate")
		atResp := atHandler.Handle(ctx, atAdmissionRequest(buildAT(actionType), uniqueName("at-setup-cluster")))
		Expect(atResp.Allowed).To(BeTrue(), "AT setup CREATE should succeed: %s", atResp.Result)

		rwName := uniqueName("it-rw-cluster")
		rw := buildRWWithCluster(rwName, actionType, []string{"production", "staging-eu"})

		resp := rwHandler.Handle(ctx, rwAdmissionRequest(rw, uniqueName("rw-cluster-create")))
		Expect(resp.Allowed).To(BeTrue(), "RW CREATE with cluster labels should be allowed (zero AW code changes): %s", resp.Result)

		stored := fetchWorkflowByName(rwName)
		Expect(stored.Labels.Cluster).To(ConsistOf("production", "staging-eu"),
			"cluster labels submitted on the CRD must survive unmodified through AW to DS's labels JSONB")
		// Regression guard: sibling mandatory dimensions are unaffected by the new field.
		Expect(stored.Labels.Priority).To(BeEquivalentTo("P1"))
	})

	It("IT-AW-1511-001b: absent cluster labels round-trip as empty (backward compatible, non-fleet)", func() {
		actionType := uniqueName("ITNoClusterCreate")
		atResp := atHandler.Handle(ctx, atAdmissionRequest(buildAT(actionType), uniqueName("at-setup-nocluster")))
		Expect(atResp.Allowed).To(BeTrue(), "AT setup CREATE should succeed: %s", atResp.Result)

		rwName := uniqueName("it-rw-nocluster")
		rw := buildRWWithCluster(rwName, actionType, nil) // non-fleet: no cluster labels set

		resp := rwHandler.Handle(ctx, rwAdmissionRequest(rw, uniqueName("rw-nocluster-create")))
		Expect(resp.Allowed).To(BeTrue(), "RW CREATE without cluster labels should be allowed: %s", resp.Result)

		stored := fetchWorkflowByName(rwName)
		Expect(stored.Labels.Cluster).To(BeEmpty(),
			"non-fleet workflows must round-trip with empty cluster labels, not an error or fabricated value")
	})
})
