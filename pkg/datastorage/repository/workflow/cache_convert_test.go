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

package workflow

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// UNIT TESTS: CRD -> models label/workflow converters (Issue #1661 Change 6)
// ========================================
// Authority: DD-WORKFLOW-018. These converters let the cache-backed Step 1/2
// discovery path (ListActions, ListWorkflowsByActionType) reuse the existing
// filter/scoring predicates (cache_filter.go) and models.RemediationWorkflow
// response shape without touching Postgres.
//
// RED: none of these symbols exist yet -- this file must fail to compile.
// ========================================

var _ = Describe("crdLabelsToMandatoryLabels (Issue #1661 Change 6)", func() {
	It("UT-DS-1661-609-001: maps every field 1:1 (CRD and models.MandatoryLabels share the same shape)", func() {
		crdLabels := rwv1alpha1.RemediationWorkflowLabels{
			Severity:    []string{"critical", "high"},
			Environment: []string{"production"},
			Component:   []string{"v1/Pod"},
			Priority:    "P1",
			Cluster:     []string{"production", "staging-eu"},
		}
		got := crdLabelsToMandatoryLabels(crdLabels)
		Expect(got.Severity).To(Equal([]string{"critical", "high"}))
		Expect(got.Environment).To(Equal([]string{"production"}))
		Expect(got.Component).To(Equal([]string{"v1/Pod"}))
		Expect(got.Priority).To(Equal("P1"))
		Expect(got.Cluster).To(Equal([]string{"production", "staging-eu"}))
	})

	It("UT-DS-1661-609-002: nil/empty Cluster maps to nil (BR-FLEET-003 exclusion semantics preserved)", func() {
		got := crdLabelsToMandatoryLabels(rwv1alpha1.RemediationWorkflowLabels{Priority: "P2"})
		Expect(got.Cluster).To(BeEmpty())
	})
})

var _ = Describe("crdCustomLabelsToModel (Issue #1661 Change 6)", func() {
	It("UT-DS-1661-610-001: wraps each single CRD value in a one-element slice", func() {
		got := crdCustomLabelsToModel(map[string]string{"team": "payments", "constraint": "cost-constrained"})
		Expect(got["team"]).To(Equal([]string{"payments"}))
		Expect(got["constraint"]).To(Equal([]string{"cost-constrained"}))
	})

	It("UT-DS-1661-610-002: nil input maps to an empty (non-nil) CustomLabels", func() {
		got := crdCustomLabelsToModel(nil)
		Expect(got).ToNot(BeNil())
		Expect(got).To(BeEmpty())
	})
})

var _ = Describe("crdDetectedLabelsToModel (Issue #1661 Change 6)", func() {
	It("UT-DS-1661-611-001: unmarshals the CRD's raw JSON into models.DetectedLabels", func() {
		raw := &apiextensionsv1.JSON{Raw: []byte(`{"gitOpsManaged":true,"gitOpsTool":"argocd"}`)}
		got, err := crdDetectedLabelsToModel(raw)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.GitOpsManaged).To(BeTrue())
		Expect(got.GitOpsTool).To(Equal("argocd"))
	})

	It("UT-DS-1661-611-002: nil input maps to a zero-value DetectedLabels without error", func() {
		got, err := crdDetectedLabelsToModel(nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.IsEmpty()).To(BeTrue())
	})

	It("UT-DS-1661-611-003: malformed JSON returns an error", func() {
		raw := &apiextensionsv1.JSON{Raw: []byte(`{not-json`)}
		_, err := crdDetectedLabelsToModel(raw)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("crdActionTypeToEntry (Issue #1661 Change 6)", func() {
	It("UT-DS-1661-613-001: maps spec.name/description and the supplied workflow count", func() {
		at := &atv1alpha1.ActionType{
			Spec: atv1alpha1.ActionTypeSpec{
				Name: "ScaleReplicas",
				Description: atv1alpha1.ActionTypeDescription{
					What:      "Scales a Deployment/StatefulSet replica count",
					WhenToUse: "When a workload is under-provisioned",
				},
			},
		}
		got := crdActionTypeToEntry(at, 3)
		Expect(got.ActionType).To(Equal("ScaleReplicas"))
		Expect(got.Description.What).To(Equal("Scales a Deployment/StatefulSet replica count"))
		Expect(got.Description.WhenToUse).To(Equal("When a workload is under-provisioned"))
		Expect(got.WorkflowCount).To(Equal(3))
	})
})

var _ = Describe("crdWorkflowToModel (Issue #1661 Change 6)", func() {
	buildRW := func() *rwv1alpha1.RemediationWorkflow {
		return &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-oom-recovery"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: "ScaleReplicas",
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "Recovers a Pod from an OOM condition",
					WhenToUse: "When a Pod is OOMKilled",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Pod"},
					Priority:    "P1",
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine:             "job",
					Bundle:             "quay.io/kubernaut/oom-recovery@sha256:abc123",
					ServiceAccountName: "oom-recovery-sa",
				},
			},
			Status: rwv1alpha1.RemediationWorkflowStatus{
				WorkflowID:    "11111111-1111-1111-1111-111111111111",
				ContentHash:   "deadbeef",
				CatalogStatus: sharedtypes.CatalogStatusActive,
			},
		}
	}

	It("UT-DS-1661-612-001: maps identity/execution fields needed by the discovery response", func() {
		got, err := crdWorkflowToModel(buildRW())
		Expect(err).ToNot(HaveOccurred())
		Expect(got.WorkflowID).To(Equal("11111111-1111-1111-1111-111111111111"))
		Expect(got.WorkflowName).To(Equal("pod-oom-recovery"))
		Expect(got.Name).To(Equal("pod-oom-recovery"), "Name mirrors WorkflowName for CRD-sourced workflows (matches buildWorkflowCore's inline-registration convention)")
		Expect(got.Version).To(Equal("1.0.0"))
		Expect(got.ActionType).To(Equal("ScaleReplicas"))
		Expect(string(got.ExecutionEngine)).To(Equal("job"))
		Expect(got.ExecutionBundle).ToNot(BeNil())
		Expect(*got.ExecutionBundle).To(Equal("quay.io/kubernaut/oom-recovery@sha256:abc123"))
		Expect(got.ServiceAccountName).ToNot(BeNil())
		Expect(*got.ServiceAccountName).To(Equal("oom-recovery-sa"))
		Expect(got.ContentHash).To(Equal("deadbeef"))
		Expect(got.Description.What).To(Equal("Recovers a Pod from an OOM condition"))
		Expect(got.Status).To(Equal("Active"))
	})

	It("UT-DS-1661-612-004: Content is populated with the same clean-CRD marshaling AuthWebhook hashed into ContentHash (models.RemediationWorkflow.Content is a required OpenAPI field; empty Content previously broke GET /workflows/{id} for every CRD-native workflow)", func() {
		rw := buildRW()
		got, err := crdWorkflowToModel(rw)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Content).ToNot(BeEmpty())

		cleanContent, marshalErr := contenthash.MarshalCleanCRDContent(rw)
		Expect(marshalErr).ToNot(HaveOccurred())
		Expect(got.Content).To(Equal(string(cleanContent)),
			"Content must be exactly the clean-CRD marshaling ContentHash was computed from, so sha256(Content) == ContentHash is independently verifiable")
	})

	It("UT-DS-1661-612-003: maps spec.parameters[] into the {\"schema\":{\"parameters\":[...]}} envelope models.RemediationWorkflow.Parameters has always used (#1661 Phase 55 prerequisite -- HandleGetWorkflowByID's documented contract requires it for LLM parameter validation; the envelope matches buildWrappedWorkflowParameters/the OpenAPI object schema, not a bare array -- a bare array breaks the ogen client's map[string]jx.Raw decoder)", func() {
		rw := buildRW()
		rw.Spec.Parameters = []rwv1alpha1.RemediationWorkflowParameter{
			{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
			{Name: "REPLICAS", Type: "integer", Required: false, Description: "Desired replica count", Enum: []string{"1", "2", "3"}},
		}

		got, err := crdWorkflowToModel(rw)
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Parameters).ToNot(BeNil())

		var envelope struct {
			Schema struct {
				Parameters []map[string]interface{} `json:"parameters"`
			} `json:"schema"`
		}
		Expect(json.Unmarshal(*got.Parameters, &envelope)).To(Succeed())
		params := envelope.Schema.Parameters
		Expect(params).To(HaveLen(2))
		Expect(params[0]["name"]).To(Equal("NAMESPACE"))
		Expect(params[0]["type"]).To(Equal("string"))
		Expect(params[0]["required"]).To(Equal(true))
		Expect(params[1]["name"]).To(Equal("REPLICAS"))
		Expect(params[1]["enum"]).To(Equal([]interface{}{"1", "2", "3"}))
	})

	It("UT-DS-1661-612-004: no declared parameters maps to a nil (omitted) parameters field", func() {
		got, err := crdWorkflowToModel(buildRW())
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Parameters).To(BeNil())
	})

	It("UT-DS-1661-612-002: catalog-only fields with no CRD equivalent are zero-valued (not a regression -- confirmed nil today for inline registration too)", func() {
		got, err := crdWorkflowToModel(buildRW())
		Expect(err).ToNot(HaveOccurred())
		Expect(got.Owner).To(BeNil())
		Expect(got.Maintainer).To(BeNil())
		Expect(got.SchemaImage).To(BeNil())
		Expect(got.SchemaDigest).To(BeNil())
		Expect(got.PreviousVersion).To(BeNil())
		Expect(got.DeprecationNotice).To(BeNil())
		Expect(got.VersionNotes).To(BeNil())
		Expect(got.ChangeSummary).To(BeNil())
		Expect(got.ApprovedBy).To(BeNil())
		Expect(got.ApprovedAt).To(BeNil())
		Expect(got.ExpectedSuccessRate).To(BeNil())
		Expect(got.ExpectedDurationSeconds).To(BeNil())
		Expect(got.CreatedBy).To(BeNil())
		Expect(got.UpdatedBy).To(BeNil())
	})
})
