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

// Package spike_s9 validates the backward-compatible ScopeChecker interface extension
// and federated client abstraction design for multi-cluster scope checking.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s9

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSpikeS9(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S9 — ScopeChecker Interface Design")
}

// --- Current interface (for reference, matches pkg/shared/scope/checker.go) ---

type ScopeChecker interface {
	IsManaged(ctx context.Context, namespace, kind, name string) (bool, error)
}

// --- Option A: Context-based cluster routing ---
// ClusterID is carried in context; existing callers don't need to change signature.
// FederatedScopeChecker extracts it from context.

type clusterIDKey struct{}

func ContextWithClusterID(ctx context.Context, clusterID string) context.Context {
	return context.WithValue(ctx, clusterIDKey{}, clusterID)
}

func ClusterIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(clusterIDKey{}).(string)
	return v
}

// ContextAwareFederatedChecker implements ScopeChecker and extracts clusterID from context.
type ContextAwareFederatedChecker struct {
	local       ScopeChecker
	remoteCheck func(ctx context.Context, clusterID, namespace, kind, name string) (bool, error)
}

func (c *ContextAwareFederatedChecker) IsManaged(ctx context.Context, namespace, kind, name string) (bool, error) {
	clusterID := ClusterIDFromContext(ctx)
	if clusterID == "" {
		return c.local.IsManaged(ctx, namespace, kind, name)
	}
	return c.remoteCheck(ctx, clusterID, namespace, kind, name)
}

// --- Option B: Extended interface (new method, old callers unaffected) ---

type FederatedScopeChecker interface {
	ScopeChecker
	IsManagedOnCluster(ctx context.Context, clusterID, namespace, kind, name string) (bool, error)
}

type federatedCheckerImpl struct {
	local       ScopeChecker
	remoteCheck func(ctx context.Context, clusterID, namespace, kind, name string) (bool, error)
}

func (f *federatedCheckerImpl) IsManaged(ctx context.Context, namespace, kind, name string) (bool, error) {
	return f.local.IsManaged(ctx, namespace, kind, name)
}

func (f *federatedCheckerImpl) IsManagedOnCluster(ctx context.Context, clusterID, namespace, kind, name string) (bool, error) {
	if clusterID == "" {
		return f.local.IsManaged(ctx, namespace, kind, name)
	}
	return f.remoteCheck(ctx, clusterID, namespace, kind, name)
}

// --- Mocks for testing ---

type alwaysManagedLocal struct{}

func (a *alwaysManagedLocal) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

type neverManagedLocal struct{}

func (n *neverManagedLocal) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}

// --- Tests ---

var _ = Describe("Spike S9 — ScopeChecker Interface Design", func() {
	var (
		ctx             context.Context
		remoteCalls     atomic.Int32
		remoteCheckFunc func(ctx context.Context, clusterID, namespace, kind, name string) (bool, error)
	)

	BeforeEach(func() {
		ctx = context.Background()
		remoteCalls.Store(0)
		//nolint:unparam // must match the remoteCheck field type func(...) (bool, error);
		// S9-013 below exercises a sibling closure returning a non-nil error for the same field.
		remoteCheckFunc = func(_ context.Context, clusterID, _, _, _ string) (bool, error) {
			remoteCalls.Add(1)
			if clusterID == "prod-east" {
				return true, nil
			}
			return false, nil
		}
	})

	Describe("Option A: Context-based cluster routing", func() {
		It("S9-001: empty context routes to local checker", func() {
			checker := &ContextAwareFederatedChecker{
				local:       &alwaysManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			managed, err := checker.IsManaged(ctx, "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(remoteCalls.Load()).To(Equal(int32(0)))
		})

		It("S9-002: context with clusterID routes to remote", func() {
			checker := &ContextAwareFederatedChecker{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			remoteCtx := ContextWithClusterID(ctx, "prod-east")
			managed, err := checker.IsManaged(remoteCtx, "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(remoteCalls.Load()).To(Equal(int32(1)))
		})

		It("S9-003: existing callers (no context change) remain backward-compatible", func() {
			checker := &ContextAwareFederatedChecker{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			// Existing GW code: checker.IsManaged(ctx, ns, kind, name)
			// ctx has no clusterID → always local
			managed, err := checker.IsManaged(ctx, "production", "Pod", "api-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeFalse())
			Expect(remoteCalls.Load()).To(Equal(int32(0)))
		})

		It("S9-004: satisfies ScopeChecker interface — drop-in replacement", func() {
			var checker ScopeChecker = &ContextAwareFederatedChecker{
				local:       &alwaysManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			managed, err := checker.IsManaged(ctx, "ns", "Pod", "x")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	Describe("Option B: Extended interface (IsManagedOnCluster)", func() {
		It("S9-005: IsManaged delegates to local (backward-compatible)", func() {
			checker := &federatedCheckerImpl{
				local:       &alwaysManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			managed, err := checker.IsManaged(ctx, "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(remoteCalls.Load()).To(Equal(int32(0)))
		})

		It("S9-006: IsManagedOnCluster with clusterID routes to remote", func() {
			checker := &federatedCheckerImpl{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			managed, err := checker.IsManagedOnCluster(ctx, "prod-east", "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(remoteCalls.Load()).To(Equal(int32(1)))
		})

		It("S9-007: IsManagedOnCluster with empty clusterID routes to local", func() {
			checker := &federatedCheckerImpl{
				local:       &alwaysManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			managed, err := checker.IsManagedOnCluster(ctx, "", "default", "Deployment", "nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
			Expect(remoteCalls.Load()).To(Equal(int32(0)))
		})

		It("S9-008: satisfies ScopeChecker interface — drop-in for existing callers", func() {
			var checker ScopeChecker = &federatedCheckerImpl{
				local:       &alwaysManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			managed, err := checker.IsManaged(ctx, "ns", "Pod", "x")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("S9-009: callers needing cluster-awareness type-assert to FederatedScopeChecker", func() {
			var base ScopeChecker = &federatedCheckerImpl{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}
			// Caller that knows about federation can type-assert
			federated, ok := base.(FederatedScopeChecker)
			Expect(ok).To(BeTrue())
			managed, err := federated.IsManagedOnCluster(ctx, "prod-east", "default", "Pod", "x")
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})
	})

	Describe("Comparison: Option A vs Option B", func() {
		It("S9-010: Option A — GW caller pattern (cluster from signal)", func() {
			checker := &ContextAwareFederatedChecker{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}

			// GW pattern: signal has clusterID, inject into context before scope check
			type signal struct {
				ClusterID string
				Namespace string
				Kind      string
				Name      string
			}
			sig := signal{ClusterID: "prod-east", Namespace: "default", Kind: "Deployment", Name: "nginx"}

			scopeCtx := ContextWithClusterID(ctx, sig.ClusterID)
			managed, err := checker.IsManaged(scopeCtx, sig.Namespace, sig.Kind, sig.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("S9-011: Option B — GW caller pattern (explicit method call)", func() {
			checker := &federatedCheckerImpl{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}

			type signal struct {
				ClusterID string
				Namespace string
				Kind      string
				Name      string
			}
			sig := signal{ClusterID: "prod-east", Namespace: "default", Kind: "Deployment", Name: "nginx"}

			managed, err := checker.IsManagedOnCluster(ctx, sig.ClusterID, sig.Namespace, sig.Kind, sig.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("S9-012: Option A — RO caller pattern (cluster from RR spec)", func() {
			checker := &ContextAwareFederatedChecker{
				local:       &neverManagedLocal{},
				remoteCheck: remoteCheckFunc,
			}

			// RO pattern: RR spec has clusterID
			type rrSpec struct {
				ClusterID       string
				TargetNamespace string
				TargetKind      string
				TargetName      string
			}
			rr := rrSpec{ClusterID: "prod-east", TargetNamespace: "production", TargetKind: "Deployment", TargetName: "payment-api"}

			scopeCtx := ContextWithClusterID(ctx, rr.ClusterID)
			managed, err := checker.IsManaged(scopeCtx, rr.TargetNamespace, rr.TargetKind, rr.TargetName)
			Expect(err).ToNot(HaveOccurred())
			Expect(managed).To(BeTrue())
		})

		It("S9-013: Option A — error from remote propagates correctly", func() {
			failingRemote := func(_ context.Context, _, _, _, _ string) (bool, error) {
				return false, fmt.Errorf("valkey connection refused")
			}
			checker := &ContextAwareFederatedChecker{
				local:       &alwaysManagedLocal{},
				remoteCheck: failingRemote,
			}

			remoteCtx := ContextWithClusterID(ctx, "prod-west")
			_, err := checker.IsManaged(remoteCtx, "default", "Pod", "x")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("valkey connection refused"))
		})
	})
})
