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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// #243 / F1 / F8: Parameter Filtering Integration Tests
// ========================================
// Authority: #243 (Defense-in-depth parameter filtering), F1/F8 (coverage gap)
// Pattern: Real envtest K8s API + configurable testWorkflowQuerier
//          + real executor — verifies the full reconciler→resolveSchemaMetadata
//          →CreateOptions→executor→filtered resource pipeline.
// ========================================

var _ = Describe("#243: Parameter Filtering", Label("integration", "243"), func() {

	AfterEach(func() {
		testWorkflowQuerier.Deps = nil
		testWorkflowQuerier.ParamNames = nil
	})

	Context("Job executor with DeclaredParameterNames", func() {

		It("IT-WE-243-001: should strip undeclared params from Job env vars", func() {
			testWorkflowQuerier.ParamNames = map[string]bool{
				"NAMESPACE": true,
				"REPLICAS":  true,
			}

			wfe := createUniqueJobWFE("pf-job-strip", "default/deployment/pf-job-strip-app")
			wfe.Spec.Parameters = map[string]string{
				"NAMESPACE":    "default",
				"REPLICAS":     "3",
				"HALLUCINATED": "should-be-stripped",
				"INJECTED":     "malicious-value",
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created")

			envMap := make(map[string]string)
			for _, env := range job.Spec.Template.Spec.Containers[0].Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(envMap).To(HaveKeyWithValue("REPLICAS", "3"))
			Expect(envMap).To(HaveKey("TARGET_RESOURCE"), "System-injected TARGET_RESOURCE should always be present")
			Expect(envMap).NotTo(HaveKey("HALLUCINATED"), "Undeclared param should be stripped")
			Expect(envMap).NotTo(HaveKey("INJECTED"), "Undeclared param should be stripped")
		})

		It("IT-WE-243-002: should pass all params through when ParamNames is nil (no schema, backward compat)", func() {
			testWorkflowQuerier.ParamNames = nil

			wfe := createUniqueJobWFE("pf-job-nil", "default/deployment/pf-job-nil-app")
			wfe.Spec.Parameters = map[string]string{
				"NAMESPACE": "default",
				"CUSTOM":    "value",
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created")

			envMap := make(map[string]string)
			for _, env := range job.Spec.Template.Spec.Containers[0].Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(envMap).To(HaveKeyWithValue("CUSTOM", "value"), "All params should pass through when no schema")
		})

		It("IT-WE-243-003: should strip all user params when DeclaredParameterNames is empty map", func() {
			testWorkflowQuerier.ParamNames = map[string]bool{}

			wfe := createUniqueJobWFE("pf-job-empty", "default/deployment/pf-job-empty-app")
			wfe.Spec.Parameters = map[string]string{
				"NAMESPACE": "default",
				"REPLICAS":  "3",
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created")

			envMap := make(map[string]string)
			for _, env := range job.Spec.Template.Spec.Containers[0].Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap).To(HaveKey("TARGET_RESOURCE"), "System-injected TARGET_RESOURCE should always be present")
			Expect(envMap).NotTo(HaveKey("NAMESPACE"), "All user params should be stripped with empty declared map")
			Expect(envMap).NotTo(HaveKey("REPLICAS"), "All user params should be stripped with empty declared map")
		})
	})

	Context("Tekton executor with DeclaredParameterNames", func() {

		It("IT-WE-243-010: should strip undeclared params from PipelineRun params", func() {
			testWorkflowQuerier.ParamNames = map[string]bool{
				"NAMESPACE": true,
				"REPLICAS":  true,
			}

			wfe := createUniqueWFEWithParams("pf-tekton-strip", "default/deployment/pf-tekton-strip-app",
				map[string]string{
					"NAMESPACE":    "default",
					"REPLICAS":     "3",
					"HALLUCINATED": "should-be-stripped",
				})
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupWFE(wfe)

			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "PipelineRun should be created")

			paramMap := make(map[string]string)
			for _, p := range pr.Spec.Params {
				paramMap[p.Name] = p.Value.StringVal
			}

			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(paramMap).To(HaveKeyWithValue("REPLICAS", "3"))
			Expect(paramMap).To(HaveKey("TARGET_RESOURCE"), "System-injected TARGET_RESOURCE should always be present")
			Expect(paramMap).NotTo(HaveKey("HALLUCINATED"), "Undeclared param should be stripped")
		})

		It("IT-WE-243-011: should pass all params through when ParamNames is nil (backward compat)", func() {
			testWorkflowQuerier.ParamNames = nil

			wfe := createUniqueWFEWithParams("pf-tekton-nil", "default/deployment/pf-tekton-nil-app",
				map[string]string{
					"NAMESPACE": "default",
					"CUSTOM":    "value",
				})
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupWFE(wfe)

			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "PipelineRun should be created")

			paramMap := make(map[string]string)
			for _, p := range pr.Spec.Params {
				paramMap[p.Name] = p.Value.StringVal
			}

			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(paramMap).To(HaveKeyWithValue("CUSTOM", "value"), "All params should pass through when no schema")
		})
	})
})
