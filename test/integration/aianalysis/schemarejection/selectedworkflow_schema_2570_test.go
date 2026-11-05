/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
*/

package schemarejection

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	rwv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-ORCH-025 selected parameter schema immutability", func() {
	DescribeTable("IT-AA-2570-027: frozen schema rejects mutation and accepts idempotent status writes", func(initial, changed []rwv1.RemediationWorkflowParameter) {
		ctx := context.Background()
		c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		analysis := newSchemaRejectionITAnalysis("default", "schema-2570", "Pending")
		analysis.Name = ""
		analysis.GenerateName = "schema-2570-"
		analysis.Finalizers = nil
		Expect(c.Create(ctx, analysis)).To(Succeed())
		DeferCleanup(func() { Expect(c.Delete(ctx, analysis)).To(Succeed()) })
		analysis.Status.Phase = "Pending"
		now := metav1.Now()
		analysis.Status.EnsureRCAResult().SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowSnapshot: sharedtypes.WorkflowSnapshot{WorkflowID: "wf", WorkflowName: "wf", ActionType: "RestartPod", Version: "v1", ExecutionBundle: "example:v1"},
			Confidence:       0.9, Rationale: "test", SelectedAt: &now, ParameterSchema: initial,
		}
		Expect(c.Status().Update(ctx, analysis)).To(Succeed())
		key := client.ObjectKeyFromObject(analysis)
		Expect(c.Get(ctx, key, analysis)).To(Succeed())
		Expect(analysis.Status.RCAResult.SelectedWorkflow.ParameterSchema).To(Equal(initial),
			"API-server status persistence must retain the selected workflow parameter schema")
		Expect(c.Status().Update(ctx, analysis)).To(Succeed(), "unchanged schema must allow retries")
		Expect(c.Get(ctx, key, analysis)).To(Succeed())
		original := analysis.Status.RCAResult.SelectedWorkflow.DeepCopy()
		analysis.Status.RCAResult.SelectedWorkflow.ParameterSchema = changed
		err = c.Status().Update(ctx, analysis)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "schema mutation must be denied: %v", err)
		Expect(c.Get(ctx, key, analysis)).To(Succeed())
		Expect(analysis.Status.RCAResult.SelectedWorkflow.ParameterSchema).To(Equal(original.ParameterSchema))
	},
		Entry("absent to empty", nil, []rwv1.RemediationWorkflowParameter{}),
		Entry("empty to absent", []rwv1.RemediationWorkflowParameter{}, nil),
		Entry("empty to populated", []rwv1.RemediationWorkflowParameter{}, []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "string"}}),
		Entry("change visible field with default", []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "string", Default: &apiextensionsv1.JSON{Raw: []byte(`"safe"`)}}}, []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "string", Description: "changed", Default: &apiextensionsv1.JSON{Raw: []byte(`"safe"`)}}}),
		Entry("change scalar default", []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "string", Default: &apiextensionsv1.JSON{Raw: []byte(`"safe"`)}}}, []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "string", Default: &apiextensionsv1.JSON{Raw: []byte(`"fast"`)}}}),
		Entry("change nested default", []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "array", Default: &apiextensionsv1.JSON{Raw: []byte(`[{"nested":"safe"}]`)}}}, []rwv1.RemediationWorkflowParameter{{Name: "MODE", Type: "array", Default: &apiextensionsv1.JSON{Raw: []byte(`[{"nested":"fast"}]`)}}}),
	)
})
