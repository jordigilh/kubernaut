package fullpipeline

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FP-1189-001: RR CRD fixture triggers the full downstream pipeline
// (RO → SP → AA → KA → WE → Notification).
var _ = Describe("AF MCP Path Full Pipeline [E2E-FP-1189-001]", Label("fp", "af", "mcp", "issue-1189"), func() {

	It("should create RR via kubectl and trigger full pipeline execution", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-001")
		}
		_ = resp.Body.Close()

		By("Creating managed test namespace for RR target")
		testNS := fmt.Sprintf("fp-e2e-1189-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testNS,
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), ns, &client.DeleteOptions{})
		})

		By("Deploying target Deployment for the remediation workflow")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "memory-eater",
				Namespace: testNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "memory-eater"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "memory-eater"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "busybox:1.36",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		rrName := "e2e-fp-mcp-rr-001"
		By("Creating RemediationRequest via kubectl CRD fixture")
		fp := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("e2e-%s-Deployment-memory-eater", testNS))))
		manifest := fmt.Sprintf(`apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: %s
  namespace: %s
spec:
  signalName: "KubernetesPodOOMKilled"
  signalFingerprint: "%s"
  signalType: "prometheus"
  severity: "critical"
  firingTime: "2026-01-01T00:00:00Z"
  receivedTime: "2026-01-01T00:00:01Z"
  targetType: "kubernetes"
  targetResource:
    kind: Deployment
    name: memory-eater
    namespace: %s
`, rrName, namespace, fp, testNS)
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "kubectl apply RR: %s", string(out))
		DeferCleanup(func() {
			_, _ = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"delete", "remediationrequest", rrName, "-n", namespace, "--ignore-not-found").CombinedOutput()
		})

		By("Waiting for full pipeline execution (RR → WE completion)")
		foundRR := fpWaitForRR("memory-eater", 120*time.Second)
		Expect(foundRR).To(Equal(rrName))
		GinkgoWriter.Printf("  RemediationRequest created: %s\n", foundRR)

		fpWaitForWEComplete(foundRR, 5*time.Minute)
		GinkgoWriter.Printf("  WorkflowExecution completed for %s\n", foundRR)
	})
})
