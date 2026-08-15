package auth_test

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

var _ = Describe("ConsoleAccessAuthorizationCheckGate", func() {
	var (
		ctx      context.Context
		fakeK8s  *k8sfake.Clientset
		gate     *auth.ConsoleAccessAuthorizationCheckGate
		sarCalls atomic.Int32
	)

	countingReactor := func(allowed bool) k8stesting.ReactionFunc {
		return func(action k8stesting.Action) (bool, runtime.Object, error) {
			sarCalls.Add(1)
			createAction := action.(k8stesting.CreateAction)
			sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
			sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: allowed}
			return true, sar, nil
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		fakeK8s = k8sfake.NewSimpleClientset()
		sarCalls.Store(0)
	})

	Describe("UT-AF-2148-001: auth-only mode allows any non-empty user without running the authorization check", func() {
		It("should return true without calling the SAR API", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", countingReactor(false))
			checker := auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
			gate = auth.NewConsoleAccessAuthorizationCheckGate(checker)

			allowed, err := gate.CheckConsoleAccess(ctx, "alice", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue(), "#2148: auth-only mode grants console access to any authenticated user")
			Expect(sarCalls.Load()).To(Equal(int32(0)), "#2148: auth-only mode must not call the SAR API")
		})
	})

	Describe("UT-AF-2148-002: auth-only mode still fail-closes on empty user", func() {
		It("should reject an empty user without calling the SAR API", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", countingReactor(true))
			checker := auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
			gate = auth.NewConsoleAccessAuthorizationCheckGate(checker)

			allowed, err := gate.CheckConsoleAccess(ctx, "", []string{"sre"})
			Expect(err).To(HaveOccurred(), "#2148: authentication is still mandatory in auth-only mode")
			Expect(allowed).To(BeFalse(), "#2148: fail-closed on empty user")
			Expect(sarCalls.Load()).To(Equal(int32(0)), "#2148: empty-user rejection must not reach the SAR API")
		})
	})

	Describe("UT-AF-2148-003: auth-only mode does not affect per-tool Check", func() {
		It("should still call the SAR API and honor its allow/deny result for Check", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", countingReactor(false))
			checker := auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
			gate = auth.NewConsoleAccessAuthorizationCheckGate(checker)

			allowed, err := gate.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse(), "#2148: per-tool authorization must remain unconditionally fail-closed")
			Expect(sarCalls.Load()).To(Equal(int32(1)), "#2148: Check must still call the SAR API")
		})
	})

	Describe("UT-AF-2148-004: ConsoleAccessAuthorizationCheckGate satisfies both ToolAuthorizer and ConsoleAuthorizer", func() {
		It("should be usable as both interfaces", func() {
			checker := auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
			gate = auth.NewConsoleAccessAuthorizationCheckGate(checker)

			var _ auth.ToolAuthorizer = gate
			var _ auth.ConsoleAuthorizer = gate
		})
	})
})
