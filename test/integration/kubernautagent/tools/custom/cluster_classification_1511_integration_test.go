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

package custom_test

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// IT-KA-1511-001: the `cluster` discovery param reaches KA's own workflow
// catalog cache through the production dispatch path (BR-FLEET-003, #1511,
// AU-3). #1677 Phase 2e (DD-WORKFLOW-019): the discovery tools no longer
// round-trip through DataStorage -- filtering (including cluster-based
// filtering, ported in Phase 2b's cache_filter.go) now happens against KA's
// own informer-backed cache. This test proves the KA -> workflowcatalog.
// Catalog dispatch succeeds end-to-end with the ClusterClassification signal
// field present, i.e. the wiring itself is correct and introduces no
// regression for non-fleet callers.
var _ = Describe("IT-KA-1511-001: cluster classification param reaches KA's workflow catalog (BR-FLEET-003)", Label("integration", "fleet"), func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(wfCatalog).NotTo(BeNil(), "workflow catalog must be initialized by SynchronizedBeforeSuite")

		reg = registry.New()
		allTools := custom.NewAllTools(wfCatalog, nil, logr.Discard())
		for _, t := range allTools {
			reg.Register(t)
		}
	})

	clusterToolCtx := func(cluster string) context.Context {
		return katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:              "critical",
			ResourceKind:          "Deployment",
			Environment:           "production",
			Priority:              "P0",
			ClusterClassification: cluster,
		})
	}

	It("IT-KA-1511-001a: list_available_actions succeeds against real DS when ClusterClassification is set", func() {
		result, err := reg.Execute(clusterToolCtx("production"), "list_available_actions",
			json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred(), "the cluster param must not break the real DS discovery call")
		Expect(result).NotTo(BeEmpty())
		Expect(result).To(ContainSubstring("actionTypes"))
	})

	It("IT-KA-1511-001b: list_workflows succeeds against real DS when ClusterClassification is set", func() {
		result, err := reg.Execute(clusterToolCtx("production"), "list_workflows",
			json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
		Expect(err).NotTo(HaveOccurred(), "the cluster param must not break the real DS discovery call")
		Expect(result).NotTo(BeEmpty())
		Expect(result).To(ContainSubstring("workflows"))
	})

	It("IT-KA-1511-001c: no ClusterClassification (non-fleet) is a zero behavioral change against real DS", func() {
		result, err := reg.Execute(clusterToolCtx(""), "list_available_actions",
			json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeEmpty())
		Expect(result).To(ContainSubstring("actionTypes"))
	})

	It("IT-KA-1511-002: dispatch returns only cluster-compatible workflows and actions (BR-FLEET-003)", func() {
		scheme, err := workflowcatalog.NewScheme()
		Expect(err).NotTo(HaveOccurred())
		workflow := func(name, id, action, cluster string) *rwv1alpha1.RemediationWorkflow {
			return &rwv1alpha1.RemediationWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
				Spec:       rwv1alpha1.RemediationWorkflowSpec{Version: "1.0.0", ActionType: action, Labels: rwv1alpha1.RemediationWorkflowLabels{Severity: []string{"critical"}, Component: []string{"deployment"}, Environment: []string{"production"}, Priority: "P0", Cluster: []string{cluster}}},
				Status:     rwv1alpha1.RemediationWorkflowStatus{WorkflowID: id, CatalogStatus: sharedtypes.CatalogStatusActive},
			}
		}
		action := func(name string) *atv1alpha1.ActionType {
			return &atv1alpha1.ActionType{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Spec: atv1alpha1.ActionTypeSpec{Name: name}, Status: atv1alpha1.ActionTypeStatus{CatalogStatus: sharedtypes.CatalogStatusActive}}
		}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			workflow("matching", "11111111-1111-1111-1111-111111111111", "RestartPod", "production"),
			workflow("mismatched", "22222222-2222-2222-2222-222222222222", "RestartPod", "staging"),
			workflow("other-action", "33333333-3333-3333-3333-333333333333", "ScaleReplicas", "staging"),
			action("RestartPod"), action("ScaleReplicas"),
		).WithIndex(&rwv1alpha1.RemediationWorkflow{}, ".spec.actionType", func(obj client.Object) []string {
			return []string{obj.(*rwv1alpha1.RemediationWorkflow).Spec.ActionType}
		}).Build()
		catalog := workflowcatalog.NewCatalog(workflowcatalog.NewCacheFromReader(reader), logr.Discard())
		localReg := registry.New()
		for _, tool := range custom.NewAllTools(catalog, nil, logr.Discard()) {
			localReg.Register(tool)
		}
		actions, err := localReg.Execute(clusterToolCtx("production"), "list_available_actions", json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred())
		var actionResult struct {
			ActionTypes []struct {
				ActionType string `json:"actionType"`
			} `json:"actionTypes"`
		}
		Expect(json.Unmarshal([]byte(actions), &actionResult)).To(Succeed())
		Expect(actionResult.ActionTypes).To(HaveLen(1))
		Expect(actionResult.ActionTypes[0].ActionType).To(Equal("RestartPod"))
		workflows, err := localReg.Execute(clusterToolCtx("production"), "list_workflows", json.RawMessage(`{"action_type":"RestartPod"}`))
		Expect(err).NotTo(HaveOccurred())
		var workflowResult struct {
			Workflows []struct {
				WorkflowID string `json:"workflowId"`
			} `json:"workflows"`
		}
		Expect(json.Unmarshal([]byte(workflows), &workflowResult)).To(Succeed())
		Expect(workflowResult.Workflows).To(HaveLen(1))
		Expect(workflowResult.Workflows[0].WorkflowID).To(Equal("11111111-1111-1111-1111-111111111111"))
	})
})
