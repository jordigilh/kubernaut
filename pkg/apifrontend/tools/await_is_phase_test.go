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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newTypedIS(ns, name, rrName string, phase isv1alpha1.SessionPhase) *isv1alpha1.InvestigationSession {
	is := &isv1alpha1.InvestigationSession{
		ObjectMeta: objMeta(ns, name),
		Spec: isv1alpha1.InvestigationSessionSpec{
			RemediationRequestRef: isv1alpha1.ObjectRef{Name: rrName},
			A2ATaskID:             name,
			UserIdentity: isv1alpha1.SessionUser{
				Username: "admin",
			},
			JoinMode: isv1alpha1.SessionJoinModeTakeover,
		},
	}
	if phase != "" {
		is.Status.Phase = phase
	}
	return is
}

func newISTypedClient(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(isTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

var _ = Describe("AwaitISPhaseActive — BR-INTERACTIVE-010 IS phase polling", func() {

	Describe("UT-AF-IS-POLL-001: returns true immediately when IS is already Active", func() {
		It("should detect Active phase without delay", func() {
			is := newTypedIS("kubernaut-system", "isess-001", "rr-poll-001", isv1alpha1.SessionPhaseActive)
			tc := newISTypedClient(is)

			start := time.Now()
			result := tools.AwaitISPhaseActive(context.Background(), tc, "kubernaut-system", "rr-poll-001")
			elapsed := time.Since(start)

			Expect(result).To(BeTrue())
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("UT-AF-IS-POLL-002: returns false on timeout when phase stays empty", func() {
		It("should timeout when IS exists but phase is never set to Active", func() {
			is := newTypedIS("kubernaut-system", "isess-002", "rr-poll-002", "")
			tc := newISTypedClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			start := time.Now()
			result := tools.AwaitISPhaseActive(ctx, tc, "kubernaut-system", "rr-poll-002")
			elapsed := time.Since(start)

			Expect(result).To(BeFalse())
			Expect(elapsed).To(BeNumerically(">=", 1500*time.Millisecond))
		})
	})

	Describe("UT-AF-IS-POLL-003: returns false when no IS CRD exists", func() {
		It("should timeout gracefully when no IS exists for the given RR", func() {
			tc := newISTypedClient()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := tools.AwaitISPhaseActive(ctx, tc, "kubernaut-system", "rr-missing")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-004: returns false with nil client", func() {
		It("should return false immediately when client is nil", func() {
			result := tools.AwaitISPhaseActive(context.Background(), nil, "kubernaut-system", "rr-nil")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-005: returns false with empty namespace", func() {
		It("should return false immediately when namespace is empty", func() {
			tc := newISTypedClient()
			result := tools.AwaitISPhaseActive(context.Background(), tc, "", "rr-empty-ns")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-006: returns false with empty RR name", func() {
		It("should return false immediately when RR name is empty", func() {
			tc := newISTypedClient()
			result := tools.AwaitISPhaseActive(context.Background(), tc, "kubernaut-system", "")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-007: ignores IS CRDs for different RR", func() {
		It("should not match an Active IS belonging to a different RR", func() {
			is := newTypedIS("kubernaut-system", "isess-other", "rr-other", isv1alpha1.SessionPhaseActive)
			tc := newISTypedClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := tools.AwaitISPhaseActive(ctx, tc, "kubernaut-system", "rr-mine")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-008: ignores terminal phase IS CRDs", func() {
		It("should not match a Completed IS for the same RR", func() {
			is := newTypedIS("kubernaut-system", "isess-done", "rr-done", isv1alpha1.SessionPhaseCompleted)
			tc := newISTypedClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			result := tools.AwaitISPhaseActive(ctx, tc, "kubernaut-system", "rr-done")
			Expect(result).To(BeFalse())
		})
	})

	Describe("UT-AF-IS-POLL-009: detects Active phase set asynchronously", func() {
		It("should detect when IS phase transitions to Active during polling", func() {
			is := newTypedIS("kubernaut-system", "isess-async", "rr-async", "")
			tc := newISTypedClient(is)

			go func() {
				time.Sleep(1 * time.Second)
				var existing isv1alpha1.InvestigationSession
				_ = tc.Get(context.Background(), crclient.ObjectKey{Namespace: "kubernaut-system", Name: "isess-async"}, &existing)
				existing.Status.Phase = isv1alpha1.SessionPhaseActive
				_ = tc.Status().Update(context.Background(), &existing)
			}()

			start := time.Now()
			result := tools.AwaitISPhaseActive(context.Background(), tc, "kubernaut-system", "rr-async")
			elapsed := time.Since(start)

			Expect(result).To(BeTrue())
			Expect(elapsed).To(BeNumerically(">=", 800*time.Millisecond))
			Expect(elapsed).To(BeNumerically("<", 5*time.Second))
		})
	})

	Describe("UT-AF-IS-POLL-010: respects parent context cancellation", func() {
		It("should return false when parent context is cancelled", func() {
			is := newTypedIS("kubernaut-system", "isess-cancel", "rr-cancel", "")
			tc := newISTypedClient(is)

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			start := time.Now()
			result := tools.AwaitISPhaseActive(ctx, tc, "kubernaut-system", "rr-cancel")
			elapsed := time.Since(start)

			Expect(result).To(BeFalse())
			Expect(elapsed).To(BeNumerically("<", 3*time.Second))
		})
	})
})
