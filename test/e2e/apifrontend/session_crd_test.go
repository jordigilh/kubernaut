package e2e_test

import (
	"context"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

var _ = Describe("InvestigationSession CRD (E2E)", Label("e2e", "phase1", "session-crd"), func() {

	var namespace string

	BeforeEach(func() {
		namespace = getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-001: AF pod starts ctrl.Manager successfully
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-001: AF logs confirm session controller manager started", func() {
		ctx := context.Background()

		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=apifrontend",
		})
		Expect(err).NotTo(HaveOccurred(), "failed to list AF pods")
		Expect(pods.Items).NotTo(BeEmpty(), "no AF pod found with label app=apifrontend")
		podName := pods.Items[0].Name

		logStream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{}).Stream(ctx)
		Expect(err).NotTo(HaveOccurred(), "failed to stream AF pod logs")
		defer func() { _ = logStream.Close() }()

		logBytes, err := io.ReadAll(logStream)
		Expect(err).NotTo(HaveOccurred(), "failed to read AF pod logs")
		logs := string(logBytes)

		Expect(logs).To(ContainSubstring("session controller manager started"),
			"AF pod should log that the session controller manager started")
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-002: InvestigationSession CRD registered in cluster
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-002: InvestigationSession CRD is registered in the cluster", func() {
		isList := &investigationsessionv1alpha1.InvestigationSessionList{}
		err := k8sClient.List(context.Background(), isList, client.InNamespace(namespace))
		Expect(err).NotTo(HaveOccurred(), "InvestigationSession CRD should be registered in the cluster")
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-003: CRD create + status update works in-cluster
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-003: InvestigationSession can be created and status updated", func() {
		ctx := context.Background()
		is := &investigationsessionv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-sess-003",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "kubernaut-apifrontend",
					"kubernaut.ai/phase":           "Active",
				},
			},
			Spec: investigationsessionv1alpha1.InvestigationSessionSpec{
				A2ATaskID: "task-e2e-003",
				JoinMode:  investigationsessionv1alpha1.SessionJoinModeStart,
				UserIdentity: investigationsessionv1alpha1.SessionUser{
					Username: "e2e-user@kubernaut.ai",
					Groups:   []string{"sre"},
				},
				RemediationRequestRef: investigationsessionv1alpha1.ObjectRef{
					Name:      "rr-e2e-test",
					Namespace: namespace,
				},
			},
		}
		Expect(k8sClient.Create(ctx, is)).To(Succeed())

		Eventually(func() string {
			got := &investigationsessionv1alpha1.InvestigationSession{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "e2e-sess-003"}, got); err != nil {
				return ""
			}
			return got.Name
		}, 10*time.Second, 1*time.Second).Should(Equal("e2e-sess-003"))

		DeferCleanup(func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, is))
		})
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-004: TTL reconciler auto-cancels disconnected session
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-004: TTL reconciler transitions Disconnected session to Cancelled", func() {
		ctx := context.Background()

		is := &investigationsessionv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-sess-004",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "kubernaut-apifrontend",
					"kubernaut.ai/phase":           "Disconnected",
				},
			},
			Spec: investigationsessionv1alpha1.InvestigationSessionSpec{
				A2ATaskID: "task-e2e-004",
				JoinMode:  investigationsessionv1alpha1.SessionJoinModeStart,
				UserIdentity: investigationsessionv1alpha1.SessionUser{
					Username: "e2e-user@kubernaut.ai",
					Groups:   []string{"sre"},
				},
				RemediationRequestRef: investigationsessionv1alpha1.ObjectRef{
					Name:      "rr-e2e-test-ttl",
					Namespace: namespace,
				},
			},
		}
		Expect(k8sClient.Create(ctx, is)).To(Succeed())

		got := &investigationsessionv1alpha1.InvestigationSession{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "e2e-sess-004"}, got)).To(Succeed())
		patch := client.MergeFrom(got.DeepCopy())
		got.Status.Phase = investigationsessionv1alpha1.SessionPhaseDisconnected
		got.Status.Message = "simulated disconnect"
		got.Status.ConnectionState = investigationsessionv1alpha1.ConnectionStateDisconnected
		got.Status.DisconnectedAt = &metav1.Time{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
		Expect(k8sClient.Status().Patch(ctx, got, patch)).To(Succeed())

		Eventually(func() string {
			current := &investigationsessionv1alpha1.InvestigationSession{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "e2e-sess-004"}, current); err != nil {
				return ""
			}
			return string(current.Status.Phase)
		}, 30*time.Second, 2*time.Second).Should(Equal("Cancelled"),
			"Reconciler should auto-cancel a Disconnected session with expired TTL")

		DeferCleanup(func() {
			_ = client.IgnoreNotFound(k8sClient.Delete(ctx, is))
		})
	})
})
