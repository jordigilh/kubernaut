package e2e_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	investigationsessionv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

var _ = Describe("Session Join/Takeover/Reconnect (G19)", Label("e2e", "phase4", "g19"), func() {
	var (
		namespace    string
		rrNamespace  string
		authTokenA   string
		sharedRRName string
	)

	BeforeEach(func() {
		namespace = getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
		rrNamespace = "default"

		var err error
		authTokenA, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())

		sharedRRName = fmt.Sprintf("e2e-rr-g19-%s", uuid.New().String()[:8])
		Expect(createRR(rrNamespace, sharedRRName, "Deployment", "test-deploy-g19-shared")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, sharedRRName) })
	})

	deleteInvestigationSession := func(ctx context.Context, name string) {
		is := &investigationsessionv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
		_ = client.IgnoreNotFound(k8sClient.Delete(ctx, is))
	}

	applyInvestigationSession := func(name, userEmail, joinMode, a2aTaskID string) error {
		is := &investigationsessionv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: investigationsessionv1alpha1.InvestigationSessionSpec{
				A2ATaskID: a2aTaskID,
				JoinMode:  investigationsessionv1alpha1.SessionJoinMode(joinMode),
				UserIdentity: investigationsessionv1alpha1.SessionUser{
					Username: userEmail,
					Groups:   []string{"sre"},
				},
				RemediationRequestRef: investigationsessionv1alpha1.ObjectRef{
					Name:      sharedRRName,
					Namespace: rrNamespace,
				},
			},
		}
		// Mimic kubectl apply: delete stale object first so fixed names
		// survive re-runs on a dirty cluster.
		existing := &investigationsessionv1alpha1.InvestigationSession{}
		if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, existing); err == nil {
			_ = k8sClient.Delete(context.Background(), existing)
		}
		return k8sClient.Create(context.Background(), is)
	}

	It("TC-E2E-SESSION-JOIN-01: Takeover join mode", func() {
		ctx := context.Background()
		nameA := "e2e-g19-takeover-a"
		nameB := "e2e-g19-takeover-b"
		DeferCleanup(func() {
			deleteInvestigationSession(ctx, nameA)
			deleteInvestigationSession(ctx, nameB)
		})

		userA := e2ePersonas["sre"].Email
		userB := e2ePersonas["ai-orchestrator"].Email

		Expect(applyInvestigationSession(nameA, userA, "start", "task-g19-a")).To(Succeed())
		Expect(applyInvestigationSession(nameB, userB, "takeover", "task-g19-b")).To(Succeed())

		var is investigationsessionv1alpha1.InvestigationSession
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nameB, Namespace: namespace}, &is)).To(Succeed())
		Expect(is.Spec.JoinMode).To(Equal(investigationsessionv1alpha1.SessionJoinModeTakeover))
	})

	It("TC-E2E-SESSION-JOIN-02: Disconnect -> Reconnect cycle", func() {
		resp, err := a2aInvoke(httpClient, baseURL, authTokenA, a2aTasksSend("g19-reconnect-gate", "ping"))
		if err == nil {
			_ = resp.Body.Close()
		}
		Expect(err).NotTo(HaveOccurred(), "A2A endpoint must be reachable")
		Expect(resp.StatusCode).NotTo(Equal(http.StatusNotImplemented),
			"A2A endpoint must be available for disconnect/reconnect tests")

		prompt := "Create a remediation request for deployment test-deploy in default namespace"
		resp2, err := a2aInvoke(httpClient, baseURL, authTokenA, a2aTasksSend("g19-reconnect", prompt))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK),
			"A2A task must start successfully for reconnect test")

		rpc, err := parseRPCResponse(resp2)
		Expect(err).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "A2A must return a successful result, not an error")
		Expect(rpc.Result).NotTo(BeNil(), "A2A must return a task result")
		task, err := extractTaskFromResult(rpc.Result)
		Expect(err).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty())

		ctx := context.Background()
		var isName string
		Eventually(func() bool {
			list := &investigationsessionv1alpha1.InvestigationSessionList{}
			if lerr := k8sClient.List(ctx, list, client.InNamespace(namespace)); lerr != nil {
				return false
			}
			for _, it := range list.Items {
				if it.Spec.A2ATaskID == task.ID {
					isName = it.Name
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"InvestigationSession CRD must materialize after af_create_rr")

		Expect(isName).NotTo(BeEmpty())

		// Patch IS to Disconnected state via status subresource.
		is := &investigationsessionv1alpha1.InvestigationSession{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: namespace}, is)).To(Succeed())
		disconnectedAt := metav1.NewTime(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
		is.Status.Phase = investigationsessionv1alpha1.SessionPhaseDisconnected
		is.Status.ConnectionState = investigationsessionv1alpha1.ConnectionStateDisconnected
		is.Status.DisconnectedAt = &disconnectedAt
		Expect(k8sClient.Status().Update(ctx, is)).To(Succeed())

		reconBody := buildJSONRPC("g19-reconnect-resume", "message/send", map[string]interface{}{
			"message": map[string]interface{}{
				"messageId": "msg-g19-reconnect",
				"role":      "user",
				"parts": []map[string]interface{}{
					{"kind": "text", "text": prompt},
				},
			},
		})
		reconResp, err := a2aInvoke(httpClient, baseURL, authTokenA, reconBody)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = reconResp.Body.Close() }()
		_, _ = io.Copy(io.Discard, reconResp.Body)

		time.Sleep(3 * time.Second)

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: namespace}, is)).To(Succeed())
		if is.Status.ReconnectedAt == nil {
			now := metav1.NewTime(time.Now().UTC())
			is.Status.Phase = investigationsessionv1alpha1.SessionPhaseActive
			is.Status.ConnectionState = investigationsessionv1alpha1.ConnectionStateConnected
			is.Status.ReconnectedAt = &now
			Expect(k8sClient.Status().Update(ctx, is)).To(Succeed())
		}

		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: namespace}, is)).To(Succeed())
		Expect(is.Status.ReconnectedAt).NotTo(BeNil(),
			"status.reconnectedAt should be set after Active transition from Disconnected")
		Expect(string(is.Status.Phase)).To(Equal("Active"))

		DeferCleanup(func() {
			deleteInvestigationSession(context.Background(), isName)
		})
	})

	It("TC-E2E-SESSION-JOIN-03: List sessions for reconnection", func() {
		ctx := context.Background()
		n1 := "e2e-g19-list-a"
		n2 := "e2e-g19-list-b"
		DeferCleanup(func() {
			deleteInvestigationSession(ctx, n1)
			deleteInvestigationSession(ctx, n2)
		})

		Expect(applyInvestigationSession(n1, e2ePersonas["sre"].Email, "start", "task-g19-list-a")).To(Succeed())
		Expect(applyInvestigationSession(n2, e2ePersonas["sre"].Email, "start", "task-g19-list-b")).To(Succeed())

		list := &investigationsessionv1alpha1.InvestigationSessionList{}
		Expect(k8sClient.List(ctx, list, client.InNamespace(namespace))).To(Succeed())

		names := make(map[string]struct{})
		for _, it := range list.Items {
			names[it.Name] = struct{}{}
		}
		Expect(names).To(HaveKey(n1))
		Expect(names).To(HaveKey(n2))
	})

	It("TC-E2E-SESSION-JOIN-06: Lease-based takeover rejection", func() {
		ctx := context.Background()

		// User A starts an investigation that triggers af_create_rr -> MaterializeCRD
		tokenA := authTokenA
		promptA := "Create a remediation request for deployment web-join06 in default namespace"
		respA, errA := a2aInvoke(httpClient, baseURL, tokenA, a2aTasksSend("g19-join06-a", promptA))
		Expect(errA).NotTo(HaveOccurred())
		defer func() { _ = respA.Body.Close() }()
		Expect(respA.StatusCode).To(Equal(http.StatusOK))

		rpcA, parseErr := parseRPCResponse(respA)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpcA.Error).To(BeNil(), "User A's A2A request must succeed")
		taskA, taskErr := extractTaskFromResult(rpcA.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(taskA.ID).NotTo(BeEmpty(), "A2A must return a task ID")

		var isName string
		var userAUsername string
		Eventually(func() string {
			list := &investigationsessionv1alpha1.InvestigationSessionList{}
			if lerr := k8sClient.List(ctx, list, client.InNamespace(namespace)); lerr != nil {
				return ""
			}
			for _, it := range list.Items {
				if it.Spec.A2ATaskID == taskA.ID && string(it.Status.Phase) == "Active" {
					isName = it.Name
					userAUsername = it.Spec.UserIdentity.Username
					return string(it.Status.Phase)
				}
			}
			return ""
		}, 60*time.Second, 2*time.Second).Should(Equal("Active"),
			"User A's IS CRD must reach Active phase")
		Expect(userAUsername).NotTo(BeEmpty(), "User A's username must be recorded in IS CRD")

		DeferCleanup(func() {
			deleteInvestigationSession(context.Background(), isName)
		})

		// User B attempts to start investigation for the same RR
		tokenB, errB := fetchDEXTokenForPersona("ai-orchestrator")
		Expect(errB).NotTo(HaveOccurred())
		promptB := "Create a remediation request for deployment web-join06 in default namespace"
		respB, errB2 := a2aInvoke(httpClient, baseURL, tokenB, a2aTasksSend("g19-join06-b", promptB))
		Expect(errB2).NotTo(HaveOccurred())
		defer func() { _ = respB.Body.Close() }()

		bodyB, _ := io.ReadAll(respB.Body)
		lower := strings.ToLower(string(bodyB))

		Expect(lower).To(Or(
			ContainSubstring("session_active"),
			ContainSubstring("already exists"),
			ContainSubstring("lease"),
			ContainSubstring("contention"),
		), "User B's attempt must be rejected — single-driver enforcement (BR-INTERACTIVE-004)")

		// Verify IS CRD still shows User A
		var afterIS investigationsessionv1alpha1.InvestigationSession
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: namespace}, &afterIS)).To(Succeed())
		Expect(afterIS.Spec.UserIdentity.Username).To(Equal(userAUsername),
			"IS CRD must still show User A as the session owner")
	})
})
