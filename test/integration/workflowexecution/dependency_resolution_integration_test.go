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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// DD-WE-006 / Issue #1481: Dependency Resolution Integration Tests
// ========================================
// Authority: DD-WE-006, BR-WE-014, BR-WORKFLOW-004, BR-WORKFLOW-008
// Pattern: Real envtest K8s API + CRD-embedded WorkflowRef.Dependencies + real executor
//
// These tests verify the full reconciler→resolveSchemaMetadata→executor
// pipeline with real K8s objects (Secrets, ConfigMaps) in envtest.
//
// Issue #1481 removed the K8sDependencyValidator pre-flight check: a
// schema-declared Secret/ConfigMap dependency that doesn't exist no longer
// blocks the WFE from creating its Job. Instead, BR-WORKFLOW-008 guarantees
// the Job still fails fast and observably:
//  1. ActiveDeadlineSeconds bounds how long a Pod can sit unable to mount the
//     missing volume (envtest has no kubelet, so this is unit-tested in
//     pkg/workflowexecution/executor/job_test.go — verified below only via
//     the Job spec field, not real enforcement).
//  2. JobExecutor.GetStatus() inspects Pod FailedMount/CreateContainerConfigError
//     events to enrich the generic Job condition message with the specific
//     missing resource.
// ========================================

var _ = Describe("DD-WE-006: Dependency Resolution", Label("integration", "dd-we-006"), func() {

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

			wfe := createUniqueJobWFE("depres-001", "default/deployment/dep-test-001")
			wfe.Spec.WorkflowRef.Dependencies = &sharedtypes.WorkflowDependencies{
				Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "it-gitea-creds-001"}},
			}
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

			wfe := createUniqueJobWFE("depres-002", "default/deployment/dep-test-002")
			wfe.Spec.WorkflowRef.Dependencies = &sharedtypes.WorkflowDependencies{
				ConfigMaps: []sharedtypes.WorkflowResourceDependency{{Name: "it-remediation-config-002"}},
			}
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

		It("IT-WE-1481-001 [BR-WORKFLOW-008]: should create the Job despite a missing Secret dependency, set ActiveDeadlineSeconds, and enrich the Failed message from the Pod's FailedMount event", func() {
			wfe := createUniqueJobWFE("depres-1481-001", "default/deployment/dep-test-1481-001")
			wfe.Spec.WorkflowRef.Dependencies = &sharedtypes.WorkflowDependencies{
				Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "nonexistent-secret-1481"}},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			// Issue #1481: registration/admission-time dependency check is gone —
			// the Job must still be created even though the Secret doesn't exist.
			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job must be created even though the declared Secret dependency is missing (#1481)")

			// BR-WORKFLOW-008 (1): Job must carry a deadline so a Pod stuck unable
			// to mount the missing Secret reaches a terminal state instead of
			// hanging in Pending forever.
			Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil(),
				"BR-WORKFLOW-008: Job must have ActiveDeadlineSeconds set")
			Expect(*job.Spec.ActiveDeadlineSeconds).To(BeNumerically(">", 0))

			// EnvTest has no kubelet/job-controller: simulate the FailedMount Pod
			// event + terminal JobFailed condition a real cluster would produce
			// once the deadline elapses.
			const missingSecretMessage = `MountVolume.SetUp failed for volume "secret-nonexistent-secret-1481" : secret "nonexistent-secret-1481" not found`
			Expect(simulateJobFailureWithMissingDependency(job, "FailedMount", missingSecretMessage)).To(Succeed())

			// BR-WORKFLOW-008 (2)+(3): GetStatus() must enrich the generic Job
			// condition message with the specific missing-dependency detail from
			// the Pod event, and MarkFailed must persist it to FailureDetails.
			failedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, "Failed", 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "WFE should reach Failed once the Job reaches a terminal JobFailed condition")

			Expect(failedWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil())
			Expect(failedWFE.Status.FailureDetails.Message).To(ContainSubstring("nonexistent-secret-1481"),
				"BR-WORKFLOW-008: failure message should name the missing resource from the Pod's FailedMount event")
		})

		It("IT-WE-1645-001 [BR-WORKFLOW-008]: should enrich the Failed message from the Pod's image-pull-failure event", func() {
			wfe := createUniqueJobWFE("depres-1645-001", "default/deployment/dep-test-1645-001")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created")

			// EnvTest has no kubelet/job-controller: simulate the kubelet-emitted
			// image-pull-failure Pod event + terminal JobFailed condition a real
			// cluster would produce for an unreachable/nonexistent bundle image
			// (Issue #1642 removed the DataStorage pre-flight check that used to
			// catch some of these earlier, at registration time).
			const badImageMessage = `Failed to pull image "quay.io/kubernaut/nonexistent-1645:v1": rpc error: code = NotFound desc = manifest unknown`
			Expect(simulateJobFailureWithMissingDependency(job, "Failed", badImageMessage)).To(Succeed())

			failedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, "Failed", 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "WFE should reach Failed once the Job reaches a terminal JobFailed condition")

			Expect(failedWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil())
			Expect(failedWFE.Status.FailureDetails.Message).To(ContainSubstring("manifest unknown"),
				"BR-WORKFLOW-008: failure message should surface the specific image-pull failure detail from the Pod event")
		})

		It("IT-WE-006-005: should create Job without dependency volumes when querier returns nil", func() {
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
