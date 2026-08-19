package tools_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

const awaitSessionTestNamespace = "default"

// newTypedAgentSessionWithSessionID mirrors newTypedAgentSession
// (await_agentsession_interactive_test.go) but additionally sets
// Status.SessionID -- the field under test here, distinct from
// Status.Interactive.
func newTypedAgentSessionWithSessionID(name, rrName, sessionID string) *agentsessionv1.AgentSession {
	as := newTypedAgentSession(name, rrName, false)
	as.Status.SessionID = sessionID
	return as
}

var _ = Describe("kubernaut_await_session", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("HandleAwaitSession validation", func() {
		It("UT-AF-1293-SC8-003: returns error when client is nil", func() {
			_, err := tools.HandleAwaitSession(ctx, nil, tools.AwaitSessionArgs{
				Namespace: awaitSessionTestNamespace,
				RRName:    "rr-test",
			})
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-1293-SC8-004: returns error when namespace is empty", func() {
			tc := newAgentSessionTypedClient()
			_, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: "",
				RRName:    "rr-test",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1293-SC8-005: returns error when rr_name is empty", func() {
			tc := newAgentSessionTypedClient()
			_, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: awaitSessionTestNamespace,
				RRName:    "",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_name is required"))
		})
	})

	Describe("HandleAwaitSession fast-path", func() {
		It("UT-AF-1293-006: returns immediately when session already exists", func() {
			as := newTypedAgentSessionWithSessionID("as-ready", "rr-ready", "session-xyz")
			as.Namespace = awaitSessionTestNamespace
			tc := newAgentSessionTypedClient(as)

			start := time.Now()
			result, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: awaitSessionTestNamespace,
				RRName:    "rr-ready",
			})
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("ready"))
			Expect(result.SessionID).To(Equal("session-xyz"))
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("HandleAwaitSession watch path (CHAR-AF-1532)", func() {
		It("observes a session ID that appears via a watch event after the initial list misses it", func() {
			as := newTypedAgentSessionWithSessionID("as-watch", "rr-watch", "")
			as.Namespace = awaitSessionTestNamespace
			tc := newAgentSessionTypedClient(as)

			go func() {
				defer GinkgoRecover()
				// Use an independent context rather than the closure-captured
				// spec-level ctx: this goroutine can still be running (e.g.
				// during the trailing Create below) after HandleAwaitSession
				// returns and the spec completes, at which point the next
				// spec's BeforeEach reassigns the shared ctx variable — a
				// real data race under -race.
				bgCtx := context.Background()
				time.Sleep(200 * time.Millisecond)
				var updated agentsessionv1.AgentSession
				Expect(tc.Get(bgCtx, crclient.ObjectKey{Namespace: awaitSessionTestNamespace, Name: "as-watch"}, &updated)).To(Succeed())
				updated.Status.SessionID = "session-via-watch"
				Expect(tc.Status().Update(bgCtx, &updated)).To(Succeed())

				// Unrelated AgentSession events (different RR name) must be
				// ignored by the watch loop rather than mistakenly matched.
				other := newTypedAgentSessionWithSessionID("as-watch-other", "rr-watch-other", "session-should-be-ignored")
				other.Namespace = awaitSessionTestNamespace
				Expect(tc.Create(bgCtx, other)).To(Succeed())
			}()

			result, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: awaitSessionTestNamespace,
				RRName:    "rr-watch",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("ready"))
			Expect(result.SessionID).To(Equal("session-via-watch"))
		})

		It("times out when the watch never observes a session ID for this RR", func() {
			orig := tools.AwaitSessionTimeout
			tools.AwaitSessionTimeout = 500 * time.Millisecond
			defer func() { tools.AwaitSessionTimeout = orig }()

			as := newTypedAgentSessionWithSessionID("as-timeout", "rr-timeout", "")
			as.Namespace = awaitSessionTestNamespace
			tc := newAgentSessionTypedClient(as)

			result, err := tools.HandleAwaitSession(ctx, tc, tools.AwaitSessionArgs{
				Namespace: awaitSessionTestNamespace,
				RRName:    "rr-timeout",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("timeout"))
		})
	})

	Describe("HandleAwaitSession list filtering", func() {
		It("UT-AF-1293-007: list ignores AgentSession for different RR name", func() {
			as := newTypedAgentSessionWithSessionID("as-other", "rr-other", "session-other")
			as.Namespace = awaitSessionTestNamespace
			tc := newAgentSessionTypedClient(as)

			var asList agentsessionv1.AgentSessionList
			err := tc.List(ctx, &asList, crclient.InNamespace(awaitSessionTestNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(asList.Items).To(HaveLen(1))

			var found string
			for _, item := range asList.Items {
				if item.Spec.RemediationRequestRef.Name != "rr-mine" {
					continue
				}
				if item.Status.SessionID != "" {
					found = item.Status.SessionID
				}
			}
			Expect(found).To(BeEmpty())
		})

		It("UT-AF-1293-008: list skips AgentSession with empty session ID", func() {
			as := newTypedAgentSessionWithSessionID("as-nosession", "rr-nosession", "")
			as.Namespace = awaitSessionTestNamespace
			tc := newAgentSessionTypedClient(as)

			var asList agentsessionv1.AgentSessionList
			err := tc.List(ctx, &asList, crclient.InNamespace(awaitSessionTestNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(asList.Items).To(HaveLen(1))

			var found string
			for _, item := range asList.Items {
				if item.Spec.RemediationRequestRef.Name != "rr-nosession" {
					continue
				}
				if item.Status.SessionID != "" {
					found = item.Status.SessionID
				}
			}
			Expect(found).To(BeEmpty())
		})
	})
})
