/*
Copyright 2025 Jordi Gil.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// DD-WE-006: Dependency Resolution Integration Tests
// ========================================
// Authority: DD-WE-006, BR-WE-014, BR-WORKFLOW-004
// Pattern: Real envtest K8s API + real DependencyValidator
// + configurable testWorkflowQuerier + real executor
//
// These tests verify the full reconciler→resolveDependencies→executor
// pipeline with real K8s objects (Secrets, ConfigMaps) in envtest.
// ========================================

var _ = Describe("DD-WE-006: Dependency Resolution", Label("integration", "dd-we-006"), func() {

	AfterEach(func() {
		testWorkflowQuerier.Deps = nil
	})

	Context("Job execution backend with schema-declared dependencies", func() {

		It("IT-WE-006-001: should mount secret volumes when workflow declares secret dependencies", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "it-gitea-creds-001",
					Namespace: WorkflowExecutionNS,
				},
				Data: map[string][]byte{
					"username": []byte("kubernaut"),
					"password": []byte("s3cret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, secret) }()

			testWorkflowQuerier.Deps = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "it-gitea-creds-001"}},
			}

			wfe := createUniqueJobWFE("depres-001", "default/deployment/dep-test-001")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created for WFE with valid secret dependency")
			Expect(job.Name).ToNot(BeEmpty(), "Job should have a valid name")

			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
				HaveField("Name", "secret-it-gitea-creds-001"),
			), "Job should have a volume for the declared secret")

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "secret-it-gitea-creds-001"),
				HaveField("MountPath", "/run/kubernaut/secrets/it-gitea-creds-001"),
				HaveField("ReadOnly", true),
			)), "Container should mount secret at convention path, read-only")
		})

		It("IT-WE-006-002: should mount configMap volumes when workflow declares configMap dependencies", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "it-remediation-config-002",
					Namespace: WorkflowExecutionNS,
				},
				Data: map[string]string{
					"config.yaml": "remediation: {retries: 3}",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cm) }()

			testWorkflowQuerier.Deps = &models.WorkflowDependencies{
				ConfigMaps: []models.ResourceDependency{{Name: "it-remediation-config-002"}},
			}

			wfe := createUniqueJobWFE("depres-002", "default/deployment/dep-test-002")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created for WFE with valid configMap dependency")
			Expect(job.Name).ToNot(BeEmpty(), "Job should have a valid name")

			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
				HaveField("Name", "configmap-it-remediation-config-002"),
			), "Job should have a volume for the declared configMap")

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "configmap-it-remediation-config-002"),
				HaveField("MountPath", "/run/kubernaut/configmaps/it-remediation-config-002"),
				HaveField("ReadOnly", true),
			)), "Container should mount configMap at convention path, read-only")
		})

		It("IT-WE-006-003: should mark WFE Failed with ConfigurationError when dependency Secret is missing", func() {
			testWorkflowQuerier.Deps = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "nonexistent-secret-003"}},
			}

			wfe := createUniqueJobWFE("depres-003", "default/deployment/dep-test-003")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			failedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, "Failed", 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "WFE should transition to Failed when dependency is missing")

			Expect(failedWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(failedWFE.Status.FailureDetails.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonConfigurationError),
				"failure reason should be ConfigurationError for missing dependency")
			Expect(failedWFE.Status.FailureDetails.Message).To(ContainSubstring("nonexistent-secret-003"),
				"failure message should name the missing resource")
		})

		It("IT-WE-006-004: should mark WFE Failed when dependency Secret has empty data", func() {
			emptySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "it-empty-secret-004",
					Namespace: WorkflowExecutionNS,
				},
				Data: map[string][]byte{},
			}
			Expect(k8sClient.Create(ctx, emptySecret)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, emptySecret) }()

			testWorkflowQuerier.Deps = &models.WorkflowDependencies{
				Secrets: []models.ResourceDependency{{Name: "it-empty-secret-004"}},
			}

			wfe := createUniqueJobWFE("depres-004", "default/deployment/dep-test-004")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			failedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, "Failed", 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "WFE should transition to Failed when dependency has empty data")

			Expect(failedWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(failedWFE.Status.FailureDetails.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonConfigurationError))
		})

		It("IT-WE-006-005: should create Job without dependency volumes when querier returns nil", func() {
			testWorkflowQuerier.Deps = nil

			wfe := createUniqueJobWFE("depres-005", "default/deployment/dep-test-005")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created for WFE without dependencies")
			Expect(job.Name).ToNot(BeEmpty(), "Job should have a valid name")

			for _, v := range job.Spec.Template.Spec.Volumes {
				Expect(v.Name).ToNot(HavePrefix("secret-"),
					"no secret volumes should be present when no dependencies declared")
				Expect(v.Name).ToNot(HavePrefix("configmap-"),
					"no configMap volumes should be present when no dependencies declared")
			}
		})
	})
})
