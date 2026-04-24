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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
)

// TDD Phase: RED — Issue #594 RO Merge Logic Tests
// BR-ORCH-032: RO resolves final workflow spec from RAR override or AIA
// BR-ORCH-033: K8s OperatorOverride event emitted when override applied
//
// These tests validate the ResolveWorkflow merge function that
// determines the final SelectedWorkflow spec: RAR override (if present)
// takes precedence over AIA defaults. WE creator is agnostic.

var _ = Describe("BR-ORCH-032: RO Override Merge Logic (#594)", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		aiWorkflow *aianalysisv1.SelectedWorkflow
		namespace  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-system"

		scheme = runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = remediationv1.AddToScheme(scheme)
		_ = remediationworkflowv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)

		aiWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:            "wf-ai-001",
			ActionType:            "RestartPod",
			Version:               "1.0.0",
			ExecutionBundle:       "ai-bundle:v1.0@sha256:aaa",
			ExecutionBundleDigest: "sha256:aaa",
			Confidence:            0.72,
			Parameters: map[string]string{
				"NAMESPACE": "default",
				"POD_NAME":  "app-pod-1",
			},
			Rationale:          "AI recommended pod restart for OOMKill recovery",
			ExecutionEngine:    "tekton",
			EngineConfig:       &apiextensionsv1.JSON{Raw: []byte(`{"pipelineRef":"restart-pipeline"}`)},
			ServiceAccountName: "ai-sa",
		}
	})

	buildRW := func(name string) *remediationworkflowv1.RemediationWorkflow {
		return &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version:    "2.0.0",
				ActionType: "DrainRestart",
				Description: remediationworkflowv1.RemediationWorkflowDescription{
					What:      "Drains and restarts node",
					WhenToUse: "When node is unhealthy",
				},
				Labels: remediationworkflowv1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"Node"},
					Priority:    "P1",
				},
				Execution: remediationworkflowv1.RemediationWorkflowExecution{
					Engine:             "job",
					Bundle:             "override-bundle:v2.0@sha256:bbb",
					BundleDigest:       "sha256:bbb",
					EngineConfig:       &apiextensionsv1.JSON{Raw: []byte(`{"image":"drain-restart:v2"}`)},
					ServiceAccountName: "override-sa",
				},
				Parameters: []remediationworkflowv1.RemediationWorkflowParameter{
					{Name: "TIMEOUT", Type: "string", Required: true, Description: "timeout"},
				},
			},
			Status: remediationworkflowv1.RemediationWorkflowStatus{
				WorkflowID:    "wf-override-002",
				CatalogStatus: sharedtypes.CatalogStatusActive,
			},
		}
	}

	Describe("UT-RO-594-001: Override workflowName → resolved spec uses RW data; Confidence preserved", func() {
		It("should replace workflow fields from RW while preserving AIA confidence", func() {
			rw := buildRW("drain-restart")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.WorkflowID).To(Equal("wf-override-002"))
			Expect(resolved.ActionType).To(Equal("DrainRestart"))
			Expect(resolved.Version).To(Equal("2.0.0"))
			Expect(resolved.ExecutionBundle).To(Equal("override-bundle:v2.0@sha256:bbb"))
			Expect(resolved.ExecutionBundleDigest).To(Equal("sha256:bbb"))
			Expect(resolved.ExecutionEngine).To(Equal("job"))
			Expect(resolved.ServiceAccountName).To(Equal("override-sa"))
			Expect(resolved.EngineConfig).NotTo(BeNil())
			Expect(string(resolved.EngineConfig.Raw)).To(ContainSubstring("drain-restart:v2"))

			// G2: Confidence preserved from AIA
			Expect(resolved.Confidence).To(Equal(0.72))

			// Parameters from AIA (override.Parameters is nil)
			Expect(resolved.Parameters).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(resolved.Parameters).To(HaveKeyWithValue("POD_NAME", "app-pod-1"))
		})
	})

	Describe("UT-RO-594-002: Override params only → AIA workflow + override params", func() {
		It("should keep AIA workflow data but replace parameters", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: map[string]string{
					"TIMEOUT": "60s",
					"FORCE":   "true",
				},
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.WorkflowID).To(Equal("wf-ai-001"))
			Expect(resolved.Version).To(Equal("1.0.0"))
			Expect(resolved.ExecutionBundle).To(Equal("ai-bundle:v1.0@sha256:aaa"))
			Expect(resolved.ExecutionEngine).To(Equal("tekton"))
			Expect(resolved.Parameters).To(HaveLen(2))
			Expect(resolved.Parameters).To(HaveKeyWithValue("TIMEOUT", "60s"))
			Expect(resolved.Parameters).To(HaveKeyWithValue("FORCE", "true"))
		})
	})

	Describe("UT-RO-594-003: No override → AIA SelectedWorkflow unmodified", func() {
		It("should return AIA workflow unchanged when override is nil", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, nil, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeFalse())

			Expect(resolved.WorkflowID).To(Equal("wf-ai-001"))
			Expect(resolved.Version).To(Equal("1.0.0"))
			Expect(resolved.ExecutionBundle).To(Equal("ai-bundle:v1.0@sha256:aaa"))
			Expect(resolved.Parameters).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(resolved.Parameters).To(HaveKeyWithValue("POD_NAME", "app-pod-1"))
			Expect(resolved.Confidence).To(Equal(0.72))
		})
	})

	Describe("UT-RO-594-004: Override both workflowName + params → both overridden", func() {
		It("should use RW workflow data AND override parameters", func() {
			rw := buildRW("drain-restart")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
				Parameters: map[string]string{
					"DRAIN_TIMEOUT": "120s",
				},
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.WorkflowID).To(Equal("wf-override-002"))
			Expect(resolved.Version).To(Equal("2.0.0"))
			Expect(resolved.Parameters).To(HaveLen(1))
			Expect(resolved.Parameters).To(HaveKeyWithValue("DRAIN_TIMEOUT", "120s"))
		})
	})

	Describe("UT-RO-594-005: Override rationale preserved", func() {
		It("should use override rationale when provided", func() {
			rw := buildRW("drain-restart")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
				Rationale:    "operator prefers drain-restart over AI recommendation",
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.Rationale).To(Equal("operator prefers drain-restart over AI recommendation"))
		})

		It("should preserve AIA rationale when override rationale is empty", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: map[string]string{"TIMEOUT": "30s"},
				Rationale:  "",
			}

			resolved, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Rationale).To(Equal("AI recommended pod restart for OOMKill recovery"))
		})
	})

	Describe("UT-RO-594-006: Params {} → empty; params nil → AIA params", func() {
		It("should replace with empty params when override.Parameters is empty map", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: map[string]string{},
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())
			Expect(resolved.Parameters).To(BeEmpty())
		})

		It("should preserve AIA params when override.Parameters is nil", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Rationale: "just a rationale, no param change",
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())
			Expect(resolved.Parameters).To(HaveLen(2))
			Expect(resolved.Parameters).To(HaveKeyWithValue("NAMESPACE", "default"))
		})
	})

	Describe("UT-RO-594-007: Override present → OperatorOverride event emitted", func() {
		It("should return overrideApplied=true when override is present", func() {
			rw := buildRW("drain-restart")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
			}

			_, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue(), "overrideApplied should be true when override is non-nil")
		})
	})

	Describe("UT-RO-594-008: No override → no OperatorOverride event", func() {
		It("should return overrideApplied=false when override is nil", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			_, applied, err := override.ResolveWorkflow(ctx, reader, nil, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeFalse(), "overrideApplied should be false when no override")
		})
	})

	Describe("UT-RO-594-009: Override applied → SelectedWorkflowRef reflects override", func() {
		It("should return a resolved workflow that reflects the overridden RW data", func() {
			rw := buildRW("drain-restart")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
			}

			resolved, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())

			// SelectedWorkflowRef would be built from this resolved spec:
			Expect(resolved.WorkflowID).To(Equal("wf-override-002"), "SelectedWorkflowRef.WorkflowID should reflect override")
			Expect(resolved.Version).To(Equal("2.0.0"), "SelectedWorkflowRef.Version should reflect override")
			Expect(resolved.ExecutionBundle).To(Equal("override-bundle:v2.0@sha256:bbb"), "SelectedWorkflowRef.ExecutionBundle should reflect override")
			Expect(resolved.ExecutionBundleDigest).To(Equal("sha256:bbb"), "SelectedWorkflowRef.ExecutionBundleDigest should reflect override")
		})
	})

	Describe("UT-RO-594-010: Override RW deleted (NotFound) → error for FailurePhaseApproval", func() {
		It("should return an error when the override RW is not found", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "deleted-workflow",
			}

			_, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deleted-workflow"))

			// R10: The error should be identifiable as a NotFound override error
			// so the reconciler can call transitionToFailed with FailurePhaseApproval
			Expect(override.IsOverrideNotFoundError(err)).To(BeTrue(),
				"error should be identifiable as override-not-found for FailurePhaseApproval transition")
		})
	})

	Describe("DeepCopy safety (G4)", func() {
		It("should not mutate the original aiWorkflow when override is applied", func() {
			rw := buildRW("drain-restart")
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
				Parameters:   map[string]string{"NEW_PARAM": "value"},
			}

			originalID := aiWorkflow.WorkflowID
			originalParams := len(aiWorkflow.Parameters)

			resolved, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())

			// Resolved should have override data
			Expect(resolved.WorkflowID).To(Equal("wf-override-002"))

			// Original should be untouched
			Expect(aiWorkflow.WorkflowID).To(Equal(originalID))
			Expect(aiWorkflow.Parameters).To(HaveLen(originalParams))
		})
	})
})
