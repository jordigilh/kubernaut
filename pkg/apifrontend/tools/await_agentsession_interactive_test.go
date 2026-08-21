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

package tools_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newTypedAgentSession(name, rrName string, interactive bool) *agentsessionv1.AgentSession {
	return &agentsessionv1.AgentSession{
		ObjectMeta: objMeta("kubernaut-system", name),
		Spec: agentsessionv1.AgentSessionSpec{
			RemediationRequestRef: agentsessionv1.ObjectRef{Name: rrName, Namespace: "kubernaut-system"},
			IncidentID:            name,
			RemediationID:         rrName,
			SignalName:            "OOMKilled",
			Severity:              "critical",
		},
		Status: agentsessionv1.AgentSessionStatus{
			Interactive: interactive,
		},
	}
}

func newAgentSessionTypedClient(objects ...crclient.Object) crclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(newAgentSessionScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

// noWatchAgentSessionClient wraps a crclient.Client without re-exposing
// Watch, so AwaitAgentSessionInteractive's `client.(crclient.WithWatch)`
// type assertion fails and the poll-fallback branch (shared with
// HandleAwaitSession's existing watch-or-poll pattern) is exercised, even
// though the underlying fake client actually does implement WithWatch.
type noWatchAgentSessionClient struct {
	crclient.Client
}

// DD-AA-KA-001 Amendment Gap 1 / Issue #2172: AF must watch
// AgentSession.Status.Interactive directly (KA's own authoritative ack)
// instead of polling the retired IS.Status.Phase=Active signal AA used to
// write. This replaces AwaitISPhaseActive.
var _ = Describe("AwaitAgentSessionInteractive — BR-AA-KA-065.5 AgentSession watch", func() {

	Describe("UT-AF-AS-WATCH-001: returns true immediately when AgentSession is already Interactive", func() {
		It("should detect Interactive=true without delay", func() {
			as := newTypedAgentSession("as-001", "rr-watch-001", true)
			tc := newAgentSessionTypedClient(as)

			start := time.Now()
			interactive, err := tools.AwaitAgentSessionInteractive(context.Background(), tc, "kubernaut-system", "rr-watch-001")
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeTrue())
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("UT-AF-AS-WATCH-002: returns (false, nil) on timeout when Interactive stays false", func() {
		It("should time out without error when AgentSession exists but never becomes Interactive", func() {
			as := newTypedAgentSession("as-002", "rr-watch-002", false)
			tc := newAgentSessionTypedClient(as)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			interactive, err := tools.AwaitAgentSessionInteractive(ctx, tc, "kubernaut-system", "rr-watch-002")

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeFalse())
		})
	})

	Describe("UT-AF-AS-WATCH-003: returns (false, nil) when no AgentSession exists yet", func() {
		It("should time out gracefully when no AgentSession exists for the given RR (fresh-start race)", func() {
			tc := newAgentSessionTypedClient()

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			interactive, err := tools.AwaitAgentSessionInteractive(ctx, tc, "kubernaut-system", "rr-missing")

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeFalse())
		})
	})

	Describe("UT-AF-AS-WATCH-004: returns an error with nil client", func() {
		It("should surface ErrK8sUnavailable immediately when client is nil", func() {
			interactive, err := tools.AwaitAgentSessionInteractive(context.Background(), nil, "kubernaut-system", "rr-nil")

			Expect(interactive).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrK8sUnavailable)).To(BeTrue())
		})
	})

	Describe("UT-AF-AS-WATCH-005: returns an error with empty namespace", func() {
		It("should surface ErrInvalidInput immediately when namespace is empty", func() {
			tc := newAgentSessionTypedClient()
			interactive, err := tools.AwaitAgentSessionInteractive(context.Background(), tc, "", "rr-empty-ns")

			Expect(interactive).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})
	})

	Describe("UT-AF-AS-WATCH-006: returns an error with empty RR name", func() {
		It("should surface ErrInvalidInput immediately when RR name is empty", func() {
			tc := newAgentSessionTypedClient()
			interactive, err := tools.AwaitAgentSessionInteractive(context.Background(), tc, "kubernaut-system", "")

			Expect(interactive).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})
	})

	Describe("UT-AF-AS-WATCH-007: ignores AgentSessions for a different RR", func() {
		It("should not match an Interactive AgentSession belonging to a different RR", func() {
			as := newTypedAgentSession("as-other", "rr-other", true)
			tc := newAgentSessionTypedClient(as)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			interactive, err := tools.AwaitAgentSessionInteractive(ctx, tc, "kubernaut-system", "rr-mine")

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeFalse())
		})
	})

	Describe("UT-AF-AS-WATCH-008: detects Interactive flip set asynchronously (the ack case)", func() {
		It("should detect when Status.Interactive transitions to true during the wait, via watch not poll", func() {
			as := newTypedAgentSession("as-async", "rr-async", false)
			tc := newAgentSessionTypedClient(as)

			go func() {
				time.Sleep(300 * time.Millisecond)
				var existing agentsessionv1.AgentSession
				_ = tc.Get(context.Background(), crclient.ObjectKey{Namespace: "kubernaut-system", Name: "as-async"}, &existing)
				existing.Status.Interactive = true
				_ = tc.Status().Update(context.Background(), &existing)
			}()

			start := time.Now()
			interactive, err := tools.AwaitAgentSessionInteractive(context.Background(), tc, "kubernaut-system", "rr-async")
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeTrue())
			// A poll loop with multi-second backoff would take much longer to
			// notice; a real watch fires promptly after the update.
			Expect(elapsed).To(BeNumerically("<", 5*time.Second))
		})
	})

	Describe("UT-AF-AS-WATCH-009: returns (false, nil) on parent context cancellation", func() {
		It("should time out without error when the parent context is cancelled", func() {
			as := newTypedAgentSession("as-cancel", "rr-cancel", false)
			tc := newAgentSessionTypedClient(as)

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()

			start := time.Now()
			interactive, err := tools.AwaitAgentSessionInteractive(ctx, tc, "kubernaut-system", "rr-cancel")
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeFalse())
			Expect(elapsed).To(BeNumerically("<", 3*time.Second))
		})
	})

	Describe("UT-AF-AS-WATCH-010: falls back to polling for a client without watch support", func() {
		It("should still detect Interactive=true via the poll fallback when the client cannot Watch", func() {
			as := newTypedAgentSession("as-nowatch", "rr-nowatch", true)
			plain := noWatchAgentSessionClient{Client: newAgentSessionTypedClient(as)}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			interactive, err := tools.AwaitAgentSessionInteractive(ctx, plain, "kubernaut-system", "rr-nowatch")

			Expect(err).NotTo(HaveOccurred())
			Expect(interactive).To(BeTrue())
		})
	})
})
