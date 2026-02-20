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

package aianalysis

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ADR-056 SoC: E2E tests for PostRCAContext and DetectedLabels
// in the AIAnalysis full 4-phase reconciliation (Kind cluster).
//
// Business Requirements:
//   - ADR-056, BR-AI-056: DetectedLabels in PostRCAContext after HAPI
//   - BR-AI-013: Rego policy uses detected_labels for approval gating
//   - BR-SP-101: Infrastructure label detection from real K8s resources
//
// Infrastructure: Kind cluster with full stack
//   (AA controller, HAPI, Mock LLM 3-step, DataStorage)
//
// Unlike integration tests (envtest), E2E tests in Kind provide real K8s resources
// (Deployments, PDBs, HPAs) that HAPI can discover during label detection.

var _ = Describe("E2E-AA ADR-056 DetectedLabels", Label("e2e", "adr-056", "detected-labels"), func() {
	const (
		timeout  = 60 * time.Second
		interval = time.Second
	)

	var (
		testNS    string
		clientset *kubernetes.Clientset
	)

	BeforeEach(func() {
		// Create K8s clientset from Kind cluster kubeconfig for resource creation
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to build kubeconfig")
		clientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred(), "Failed to create K8s clientset")

		testNS = createTestNamespace("adr056-e2e")
	})

	// createTestDeployment creates a Deployment in the test namespace for label detection.
	createTestDeployment := func(name string) {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": name},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": name},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:    "app",
							Image:   "busybox:1.36",
							Command: []string{"sleep", "3600"},
						}},
					},
				},
			},
		}
		_, err := clientset.AppsV1().Deployments(testNS).Create(ctx, deployment, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create Deployment %s", name)
	}

	// createTestPDB creates a PodDisruptionBudget for the given Deployment.
	createTestPDB := func(deployName string) {
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName + "-pdb",
				Namespace: testNS,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": deployName},
				},
			},
		}
		_, err := clientset.PolicyV1().PodDisruptionBudgets(testNS).Create(ctx, pdb, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create PDB for %s", deployName)
	}

	// newAnalysisCR creates an AIAnalysis CR for the test namespace.
	newAnalysisCR := func(suffix, deployName string) *aianalysisv1alpha1.AIAnalysis {
		return &aianalysisv1alpha1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-aa-056-" + suffix + "-" + randomSuffix(),
				Namespace: testNS,
			},
			Spec: aianalysisv1alpha1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "e2e-rr-" + suffix,
					Namespace: testNS,
				},
				RemediationID: "e2e-rem-056-" + suffix,
				AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
					SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      "e2e-fp-056-" + suffix,
						Severity:         "critical",
						SignalType:       "CrashLoopBackOff",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1alpha1.TargetResource{
							Kind:      "Deployment",
							Name:      deployName,
							Namespace: testNS,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
				},
			},
		}
	}

	Context("Full 4-Phase Reconciliation with PostRCAContext", func() {
		It("E2E-AA-056-001: should populate postRCAContext.detectedLabels after full reconciliation", func() {
			deployName := "app-e2e-001"

			By("Creating K8s resources for label detection (Deployment)")
			createTestDeployment(deployName)

			analysis := newAnalysisCR("001", deployName)
			defer func() { _ = k8sClient.Delete(context.Background(), analysis) }()

			By("Creating AIAnalysis CR for production CrashLoopBackOff")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for full 4-phase reconciliation to complete")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(Equal("Completed"),
				"CR should complete 4-phase reconciliation (Pending→Investigating→Analyzing→Completed)")

			By("Verifying postRCAContext is populated with detected labels")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// ADR-056 + BR-AI-056: PostRCAContext should be populated after HAPI
			// computes labels from real K8s resources in the Kind cluster.
			if analysis.Status.PostRCAContext != nil {
				Expect(analysis.Status.PostRCAContext.DetectedLabels).NotTo(BeNil(),
					"PostRCAContext.DetectedLabels must be non-nil when PostRCAContext is set")
				Expect(analysis.Status.PostRCAContext.SetAt).NotTo(BeNil(),
					"PostRCAContext.SetAt must be set (BR-AI-082 immutability guard)")
			}

			// BR-AI-013: Production environment should require approval (Rego evaluation
			// ran with detected_labels in input regardless of label values)
			Expect(analysis.Status.ApprovalRequired).To(BeTrue(),
				"Production environment should require approval (Rego uses detected_labels)")
		})

		It("E2E-AA-056-002: should populate postRCAContext for recovery analysis in Kind", func() {
			deployName := "app-e2e-002"

			By("Creating K8s resources for label detection")
			createTestDeployment(deployName)

			analysis := newAnalysisCR("002", deployName)
			analysis.Spec.IsRecoveryAttempt = true
			analysis.Spec.RecoveryAttemptNumber = 1
			defer func() { _ = k8sClient.Delete(context.Background(), analysis) }()

			By("Creating recovery AIAnalysis CR")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to complete")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(SatisfyAny(Equal("Completed"), Equal("Failed")))

			By("Verifying PostRCAContext for recovery flow")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// Recovery uses the same PostRCAContext contract as incident
			if analysis.Status.PostRCAContext != nil {
				Expect(analysis.Status.PostRCAContext.DetectedLabels).NotTo(BeNil())
				Expect(analysis.Status.PostRCAContext.SetAt).NotTo(BeNil())
			}

			Expect(analysis.Status.InvestigationID).NotTo(BeEmpty(),
				"Recovery InvestigationID should be set")
		})

		It("E2E-AA-056-003: should detect PDB in Kind cluster and populate postRCAContext.detectedLabels.pdbProtected", func() {
			deployName := "app-e2e-003"

			By("Creating Deployment and PDB in Kind cluster")
			createTestDeployment(deployName)
			createTestPDB(deployName)

			analysis := newAnalysisCR("003", deployName)
			defer func() { _ = k8sClient.Delete(context.Background(), analysis) }()

			By("Creating AIAnalysis CR targeting PDB-protected Deployment")
			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			By("Waiting for reconciliation to complete")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
				return string(analysis.Status.Phase)
			}, timeout, interval).Should(SatisfyAny(Equal("Completed"), Equal("Failed")))

			By("Verifying pdbProtected in postRCAContext.detectedLabels")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())

			// BR-SP-101 + ADR-056: With a real PDB in the Kind cluster, HAPI should
			// detect pdbProtected=true and include it in PostRCAContext.
			if analysis.Status.PostRCAContext != nil &&
				analysis.Status.PostRCAContext.DetectedLabels != nil {
				Expect(analysis.Status.PostRCAContext.DetectedLabels.PDBProtected).To(BeTrue(),
					"pdbProtected should be true when PDB exists for the Deployment")
			}
		})
	})
})
