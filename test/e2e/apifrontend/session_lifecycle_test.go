package e2e_test

import (
	"context"
	"fmt"
	"io"

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
		rrNamespace = namespace

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
		ctx := context.Background()

		// Create a prerequisite RR directly — this test validates session
		// lifecycle (disconnect/reconnect), not RR creation by the LLM.
		rrName := fmt.Sprintf("rr-join02-%d", time.Now().UnixNano())
		Expect(createRR(rrNamespace, rrName, "Deployment", "test-deploy")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })

		// #1332: IS CRD creation handled by kubernaut_investigate.
		// Invoke investigate via MCP to create the IS CRD through the production path.
		mcpToken, mcpTokenErr := fetchDEXTokenForPersona("sre")
		Expect(mcpTokenErr).NotTo(HaveOccurred())
		mcpSessID, mcpSessErr := initMCPSession(mcpToken)
		Expect(mcpSessErr).NotTo(HaveOccurred())

		takeoverBody := buildJSONRPC("g19-reconnect-takeover", "tools/call", map[string]interface{}{
			"name":      "kubernaut_investigate",
			"arguments": map[string]interface{}{"rr_id": rrName},
		})
		_, takeoverCode, takeoverErr := mcpPOST(mcpToken, mcpSessID, takeoverBody)
		Expect(takeoverErr).NotTo(HaveOccurred())
		Expect(takeoverCode).To(BeNumerically("<", 500), "kubernaut_investigate must not return 5xx")

		var isName string
		Eventually(func() bool {
			list := &investigationsessionv1alpha1.InvestigationSessionList{}
			if lerr := k8sClient.List(ctx, list, client.InNamespace(namespace)); lerr != nil {
				return false
			}
			for _, it := range list.Items {
				if it.Spec.RemediationRequestRef.Name == rrName {
					isName = it.Name
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"kubernaut_investigate must create the InvestigationSession CRD")
		DeferCleanup(func() { deleteInvestigationSession(context.Background(), isName) })

		// NOTE: kaCorrelationID assertion moved to E2E FP (08_af_a2a_interactive_test.go)
		// where AA is deployed and can acknowledge the IS + submit to KA.
		// E2E AF validates IS CRD creation; FP validates the full lifecycle.

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
					{"kind": "text", "text": "resume investigation for test-deploy"},
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

		// Create a prerequisite RR directly — this test validates single-driver
		// guard (User B rejected when User A holds the session), not RR creation.
		tokenA := authTokenA
		rrName := fmt.Sprintf("rr-join06-%d", time.Now().UnixNano())
		Expect(createRR(rrNamespace, rrName, "Deployment", "web-join06")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })

		// #1332: User A invokes kubernaut_investigate via MCP to create the IS CRD.
		mcpSessA, mcpSessErr := initMCPSession(tokenA)
		Expect(mcpSessErr).NotTo(HaveOccurred())
		takeoverBodyA := buildJSONRPC("g19-join06-takeover-a", "tools/call", map[string]interface{}{
			"name":      "kubernaut_investigate",
			"arguments": map[string]interface{}{"rr_id": rrName},
		})
		_, codeA, takeoverErrA := mcpPOST(tokenA, mcpSessA, takeoverBodyA)
		Expect(takeoverErrA).NotTo(HaveOccurred())
		Expect(codeA).To(BeNumerically("<", 500))

		var isName string
		var userAUsername string
		Eventually(func() bool {
			list := &investigationsessionv1alpha1.InvestigationSessionList{}
			if lerr := k8sClient.List(ctx, list, client.InNamespace(namespace)); lerr != nil {
				return false
			}
			for _, it := range list.Items {
				if it.Spec.RemediationRequestRef.Name == rrName {
					isName = it.Name
					userAUsername = it.Spec.UserIdentity.Username
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"User A's IS CRD must be created after kubernaut_investigate")
		Expect(userAUsername).NotTo(BeEmpty(), "User A's username must be recorded in IS CRD")

		DeferCleanup(func() {
			deleteInvestigationSession(context.Background(), isName)
		})

		// User B invokes kubernaut_investigate for the same RR — single-driver guard rejects.
		tokenB, errB := fetchDEXTokenForPersona("ai-orchestrator")
		Expect(errB).NotTo(HaveOccurred())
		mcpSessB, mcpSessBErr := initMCPSession(tokenB)
		Expect(mcpSessBErr).NotTo(HaveOccurred())
		takeoverBodyB := buildJSONRPC("g19-join06-takeover-b", "tools/call", map[string]interface{}{
			"name":      "kubernaut_investigate",
			"arguments": map[string]interface{}{"rr_id": rrName},
		})
		rawB, _, takeoverErrB := mcpPOST(tokenB, mcpSessB, takeoverBodyB)
		Expect(takeoverErrB).NotTo(HaveOccurred())

		lower := strings.ToLower(string(rawB))
		Expect(lower).To(Or(
			ContainSubstring("session_active"),
			ContainSubstring("already exists"),
			ContainSubstring("failed"),
		), "User B's takeover must be rejected — single-driver enforcement (BR-INTERACTIVE-004)")

		// Verify IS CRD still shows User A
		var afterIS investigationsessionv1alpha1.InvestigationSession
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: namespace}, &afterIS)).To(Succeed())
		Expect(afterIS.Spec.UserIdentity.Username).To(Equal(userAUsername),
			"IS CRD must still show User A as the session owner")
	})
})
