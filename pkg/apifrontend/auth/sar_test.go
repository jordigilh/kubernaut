package auth_test

import (
	"context"
	"errors"
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

var _ = Describe("SARChecker", func() {
	var (
		ctx      context.Context
		fakeK8s  *k8sfake.Clientset
		checker  *auth.SARChecker
		sarCalls atomic.Int32
		lastSAR  *authorizationv1.SubjectAccessReview
	)

	//nolint:unparam // must match k8stesting.ReactionFunc signature required by PrependReactor
	allowAll := func(action k8stesting.Action) (bool, runtime.Object, error) {
		sarCalls.Add(1)
		createAction := action.(k8stesting.CreateAction)
		sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
		lastSAR = sar.DeepCopy()
		sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
		return true, sar, nil
	}

	//nolint:unparam // must match k8stesting.ReactionFunc signature required by PrependReactor
	denyAll := func(action k8stesting.Action) (bool, runtime.Object, error) {
		sarCalls.Add(1)
		createAction := action.(k8stesting.CreateAction)
		sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
		lastSAR = sar.DeepCopy()
		sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: false, Reason: "RBAC denied"}
		return true, sar, nil
	}

	//nolint:unparam // action unused and runtime.Object always nil, but must match k8stesting.ReactionFunc signature required by PrependReactor
	failAll := func(_ k8stesting.Action) (bool, runtime.Object, error) {
		sarCalls.Add(1)
		return true, nil, errors.New("api server unreachable")
	}

	BeforeEach(func() {
		ctx = context.Background()
		fakeK8s = k8sfake.NewSimpleClientset()
		sarCalls.Store(0)
		lastSAR = nil
	})

	Describe("UT-AF-1221-001: Cache hit returns cached result without API call", func() {
		It("should return cached result on second call without additional API call", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", allowAll)
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed1, err1 := checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
			Expect(err1).NotTo(HaveOccurred())
			Expect(allowed1).To(BeTrue(), "AC 4: first call should query SAR and return allowed")
			Expect(sarCalls.Load()).To(Equal(int32(1)), "AC 4: first call should trigger one SAR API call")

			allowed2, err2 := checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
			Expect(err2).NotTo(HaveOccurred())
			Expect(allowed2).To(BeTrue(), "AC 4: cached result should be returned")
			Expect(sarCalls.Load()).To(Equal(int32(1)), "AC 4: second call should NOT trigger another SAR API call")
		})
	})

	Describe("UT-AF-1221-002: Cache miss calls API and stores result", func() {
		It("should call API on first access and cache the result", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", allowAll)
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed, err := checker.Check(ctx, "bob", []string{"cicd"}, "kubernaut_watch")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue(), "AC 4: cache miss should call API and return result")
			Expect(sarCalls.Load()).To(Equal(int32(1)), "AC 4: exactly one SAR call on cache miss")
		})
	})

	Describe("UT-AF-1221-003: Cache TTL expiry triggers fresh API call", func() {
		It("should call API again after TTL expires", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", allowAll)
			checker = auth.NewSARChecker(fakeK8s, 50*time.Millisecond, logr.Discard())

			_, err1 := checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
			Expect(err1).NotTo(HaveOccurred())
			Expect(sarCalls.Load()).To(Equal(int32(1)))

			Eventually(func() int32 {
				_, _ = checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
				return sarCalls.Load()
			}, 500*time.Millisecond, 20*time.Millisecond).Should(BeNumerically(">=", int32(2)),
				"AC 4: after TTL expiry a fresh SAR call should be made")
		})
	})

	Describe("UT-AF-1221-004: API error returns (false, err) — fail-closed", func() {
		It("should return false and an error when SAR API fails", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", failAll)
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed, err := checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
			Expect(err).To(HaveOccurred(), "AC 5: API error should be propagated")
			Expect(allowed).To(BeFalse(), "AC 5: fail-closed — API error must deny access")
		})
	})

	Describe("UT-AF-1221-005: Denied user returns (false, nil)", func() {
		It("should return false without error when SAR denies access", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", denyAll)
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed, err := checker.Check(ctx, "viewer", []string{"observability"}, "kubernaut_approve")
			Expect(err).NotTo(HaveOccurred(), "AC 1: denial is not an error")
			Expect(allowed).To(BeFalse(), "AC 1: user without permission should be denied")
		})
	})

	Describe("UT-AF-1221-006: Allowed user returns (true, nil)", func() {
		It("should return true without error when SAR allows access", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", allowAll)
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed, err := checker.Check(ctx, "sre-admin", []string{"sre"}, "kubernaut_approve")
			Expect(err).NotTo(HaveOccurred(), "AC 1: allowed access is not an error")
			Expect(allowed).To(BeTrue(), "AC 1: user with permission should be allowed")
		})
	})

	Describe("UT-AF-1221-007: Cache key uniqueness", func() {
		It("should produce different cache results for different user/group/tool combinations", func() {
			callCount := atomic.Int32{}
			fakeK8s.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				callCount.Add(1)
				createAction := action.(k8stesting.CreateAction)
				sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
				sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
				return true, sar, nil
			})
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			_, _ = checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_approve")
			_, _ = checker.Check(ctx, "bob", []string{"sre"}, "kubernaut_approve")
			_, _ = checker.Check(ctx, "alice", []string{"cicd"}, "kubernaut_approve")
			_, _ = checker.Check(ctx, "alice", []string{"sre"}, "kubernaut_watch")

			Expect(callCount.Load()).To(Equal(int32(4)),
				"AC 4: four distinct user/group/tool combos should produce four SAR calls")
		})
	})

	Describe("UT-AF-1221-008: Groups propagated to SAR spec", func() {
		It("should include user and groups in the SAR request", func() {
			fakeK8s.PrependReactor("create", "subjectaccessreviews", allowAll)
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			_, err := checker.Check(ctx, "alice@corp.com", []string{"sre", "system:authenticated"}, "kubernaut_approve")
			Expect(err).NotTo(HaveOccurred())

			Expect(lastSAR).NotTo(BeNil(), "AC 3: SAR request should have been captured")
			Expect(lastSAR.Spec.User).To(Equal("alice@corp.com"), "AC 3: user must be propagated to SAR")
			Expect(lastSAR.Spec.Groups).To(ConsistOf("sre", "system:authenticated"), "AC 3: groups must be propagated to SAR")
			Expect(lastSAR.Spec.ResourceAttributes).NotTo(BeNil())
			Expect(lastSAR.Spec.ResourceAttributes.Verb).To(Equal("use"), "verb must be 'use'")
			Expect(lastSAR.Spec.ResourceAttributes.Group).To(Equal("kubernaut.ai"), "apiGroup must be 'kubernaut.ai'")
			Expect(lastSAR.Spec.ResourceAttributes.Resource).To(Equal("tools"), "resource must be 'tools'")
			Expect(lastSAR.Spec.ResourceAttributes.Name).To(Equal("kubernaut_approve"), "resourceName must be the tool name")
		})
	})

	Describe("UT-AF-1221-009: Empty user returns error", func() {
		It("should reject empty user as input validation", func() {
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed, err := checker.Check(ctx, "", []string{"sre"}, "kubernaut_approve")
			Expect(err).To(HaveOccurred(), "AC 5: empty user must be rejected")
			Expect(allowed).To(BeFalse(), "AC 5: fail-closed on invalid input")
		})
	})

	Describe("UT-AF-1221-010: Empty tool name returns error", func() {
		It("should reject empty tool name as input validation", func() {
			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed, err := checker.Check(ctx, "alice", []string{"sre"}, "")
			Expect(err).To(HaveOccurred(), "AC 5: empty tool name must be rejected")
			Expect(allowed).To(BeFalse(), "AC 5: fail-closed on invalid input")
		})
	})
})
