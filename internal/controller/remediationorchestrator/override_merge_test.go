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

package controller_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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
			WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID:            "wf-ai-001",
				WorkflowName:          "wf-ai-001",
				ActionType:            "RestartPod",
				Version:               "1.0.0",
				ExecutionBundle:       "ai-bundle:v1.0@sha256:aaa",
				ExecutionBundleDigest: "sha256:aaa",
				ExecutionEngine:       "tekton",
				EngineConfig:          &apiextensionsv1.JSON{Raw: []byte(`{"pipelineRef":"restart-pipeline"}`)},
				ServiceAccountName:    "ai-sa",
			},
			Confidence: 0.72,
			Parameters: typedParametersFromStrings(map[string]string{
				"NAMESPACE": defaultFixture,
				"POD_NAME":  "app-pod-1",
			}),
			Rationale: "AI recommended pod restart for OOMKill recovery",
		}
	})

	Describe("UT-RO-594-001: workflow switching is rejected", func() {
		It("rejects a workflow-name override and preserves the reviewed workflow", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			resolved, applied, err := override.ResolveWorkflow(ctx, reader, &remediationv1.WorkflowOverride{WorkflowName: "drain-restart"}, aiWorkflow, namespace)
			Expect(err).To(MatchError("workflow switching is not permitted"))
			Expect(applied).To(BeFalse())
			Expect(resolved).To(BeNil())
		})
	})

	Describe("BR-ORCH-025: partial reviewer parameter overrides (#2570)", func() {
		It("UT-RO-2570-096 preserves omitted original parameter values", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			changes := &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"POD_NAME": "reviewed-pod"})}

			resolved, _, err := override.ResolveWorkflow(ctx, reader, changes, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Parameters).To(Equal(typedParametersFromStrings(map[string]string{"NAMESPACE": defaultFixture, "POD_NAME": "reviewed-pod"})))
			Expect(aiWorkflow.Parameters).To(HaveKeyWithValue("POD_NAME", typedParameterValue("app-pod-1")))
		})

		It("UT-RO-2570-097 treats an explicit empty override as a no-op", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			changes := &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{})}

			resolved, _, err := override.ResolveWorkflow(ctx, reader, changes, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Parameters).To(Equal(aiWorkflow.Parameters))
		})

		It("UT-RO-2570-103 rejects keys absent from the original parameters", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			changes := &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"NEW_KEY": "value"})}
			_, _, err := override.ResolveWorkflow(ctx, reader, changes, aiWorkflow, namespace)
			Expect(err).To(HaveOccurred())
			Expect(aiWorkflow.Parameters).NotTo(HaveKey("NEW_KEY"))
		})

		It("UT-RO-2570-104 rejects changes to target resource parameters", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			aiWorkflow.Parameters["TARGET_RESOURCE_NAME"] = typedParameterValue("original")
			changes := &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"TARGET_RESOURCE_NAME": "different"})}
			_, _, err := override.ResolveWorkflow(ctx, reader, changes, aiWorkflow, namespace)
			Expect(err).To(HaveOccurred())
			Expect(aiWorkflow.Parameters["TARGET_RESOURCE_NAME"]).To(Equal(typedParameterValue("original")))
		})

		It("UT-RO-2570-105 preserves an unchanged target resource parameter", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			aiWorkflow.Parameters["TARGET_RESOURCE_NAME"] = typedParameterValue("original")
			changes := &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"TARGET_RESOURCE_NAME": "original"})}
			resolved, _, err := override.ResolveWorkflow(ctx, reader, changes, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Parameters["TARGET_RESOURCE_NAME"]).To(Equal(typedParameterValue("original")))
		})
	})

	Describe("UT-RO-594-002: Override params only → AIA workflow + override params", func() {
		It("should keep workflow data and overlay existing parameters", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{
					"POD_NAME": "reviewed-pod",
				}),
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.WorkflowID).To(Equal("wf-ai-001"))
			Expect(resolved.Version).To(Equal("1.0.0"))
			Expect(resolved.ExecutionBundle).To(Equal("ai-bundle:v1.0@sha256:aaa"))
			Expect(resolved.ExecutionEngine).To(Equal("tekton"))
			Expect(resolved.Parameters).To(HaveLen(2))
			Expect(resolved.Parameters).To(HaveKeyWithValue("POD_NAME", typedParameterValue("reviewed-pod")))
			Expect(resolved.Parameters).To(HaveKeyWithValue("NAMESPACE", typedParameterValue(defaultFixture)))
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
			Expect(resolved.Parameters).To(HaveKeyWithValue("NAMESPACE", typedParameterValue(defaultFixture)))
			Expect(resolved.Parameters).To(HaveKeyWithValue("POD_NAME", typedParameterValue("app-pod-1")))
			Expect(resolved.Confidence).To(Equal(0.72))
		})
	})

	Describe("UT-RO-594-004: parameter-only override", func() {
		It("keeps the reviewed workflow and overlays parameters", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()
			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{
					"POD_NAME": "reviewed-pod",
				}),
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.WorkflowID).To(Equal("wf-ai-001"))
			Expect(resolved.Version).To(Equal("1.0.0"))
			Expect(resolved.Parameters).To(HaveLen(2))
			Expect(resolved.Parameters).To(HaveKeyWithValue("POD_NAME", typedParameterValue("reviewed-pod")))
		})
	})

	Describe("UT-RO-594-005: Override rationale preserved", func() {
		It("should use override rationale when provided", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{"POD_NAME": "reviewed-pod"}),
				Rationale:  "operator prefers drain-restart over AI recommendation",
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())

			Expect(resolved.Rationale).To(Equal("operator prefers drain-restart over AI recommendation"))
		})

		It("should preserve AIA rationale when override rationale is empty", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{"POD_NAME": "reviewed-pod"}),
				Rationale:  "",
			}

			resolved, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Rationale).To(Equal("AI recommended pod restart for OOMKill recovery"))
		})
	})

	Describe("UT-RO-594-006: empty or nil overrides preserve original params", func() {
		It("should preserve params when override.Parameters is empty map", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{}),
			}

			resolved, applied, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(applied).To(BeTrue())
			Expect(resolved.Parameters).To(Equal(aiWorkflow.Parameters))
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
			Expect(resolved.Parameters).To(HaveKeyWithValue("NAMESPACE", typedParameterValue(defaultFixture)))
		})
	})

	Describe("UT-RO-594-007: Override present → OperatorOverride event emitted", func() {
		It("should return overrideApplied=true when override is present", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{"POD_NAME": "reviewed-pod"}),
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

	Describe("UT-RO-594-009: parameter override preserves workflow identity", func() {
		It("should return a resolved workflow with the reviewed identity", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{"POD_NAME": "reviewed-pod"}),
			}

			resolved, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())

			Expect(resolved.WorkflowID).To(Equal("wf-ai-001"))
			Expect(resolved.Version).To(Equal("1.0.0"))
			Expect(resolved.Parameters["POD_NAME"]).To(Equal(typedParameterValue("reviewed-pod")))
		})
	})

	Describe("UT-RO-594-010: workflow switching is rejected before lookup", func() {
		It("should reject a deleted workflow name without a catalog lookup", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				WorkflowName: "deleted-workflow",
			}

			_, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("workflow switching is not permitted"))
			Expect(override.IsOverrideNotFoundError(err)).To(BeFalse())
		})
	})

	Describe("DeepCopy safety (G4)", func() {
		It("should not mutate the original aiWorkflow when override is applied", func() {
			reader := fake.NewClientBuilder().WithScheme(scheme).Build()

			overrideSpec := &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{"POD_NAME": "reviewed-pod"}),
			}

			originalID := aiWorkflow.WorkflowID
			originalParams := len(aiWorkflow.Parameters)

			resolved, _, err := override.ResolveWorkflow(ctx, reader, overrideSpec, aiWorkflow, namespace)
			Expect(err).NotTo(HaveOccurred())

			// Resolved keeps the reviewed identity and receives the override.
			Expect(resolved.WorkflowID).To(Equal(originalID))

			// Original should be untouched
			Expect(aiWorkflow.WorkflowID).To(Equal(originalID))
			Expect(aiWorkflow.Parameters).To(HaveLen(originalParams))
			Expect(aiWorkflow.Parameters["POD_NAME"]).To(Equal(typedParameterValue("app-pod-1")))
		})
	})
})
