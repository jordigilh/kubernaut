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

package investigator

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// White-box (package investigator, not investigator_test) because these
// specs exercise anomalyDetectorFor and pruneAnomalyDetectors directly,
// which are intentionally unexported: they are Investigator-internal
// multiplexing/lifecycle concerns, not part of the public API (#1892
// design decision: keep counting logic (AnomalyDetector) and
// multiplexing/lifecycle (this file) separately testable).

func newScopeTestInvestigator(cfg AnomalyConfig) *Investigator {
	return New(Config{
		Logger: logr.Discard(),
		Pipeline: Pipeline{
			AnomalyDetector: NewAnomalyDetector(cfg, nil),
		},
	})
}

var _ = Describe("Kubernaut Agent Investigator anomaly detector scoping (#1892)", func() {

	Describe("UT-KA-1892-003: anomalyDetectorFor memoizes per correlationID", func() {
		It("returns the same instance for repeated lookups of one correlationID and a distinct instance for another", func() {
			inv := newScopeTestInvestigator(AnomalyConfig{MaxToolCallsPerTool: 10, MaxTotalToolCalls: 100, MaxRepeatedFailures: 10})

			a1 := inv.anomalyDetectorFor("rr-a")
			a2 := inv.anomalyDetectorFor("rr-a")
			Expect(a1).To(BeIdenticalTo(a2),
				"repeated lookups for the same correlationID must resolve to the same detector instance")

			b1 := inv.anomalyDetectorFor("rr-b")
			Expect(b1).NotTo(BeIdenticalTo(a1),
				"a different correlationID must resolve to a distinct detector instance")
		})
	})

	Describe("IT-KA-1892-001: concurrent investigations never observe each other's Reset()/counters (#1892)", func() {
		It("keeps rr-b's per-tool counter exhausted despite a concurrent Reset()/CheckToolCall storm on rr-a", func() {
			cfg := AnomalyConfig{MaxToolCallsPerTool: 2, MaxTotalToolCalls: 100, MaxRepeatedFailures: 100}
			inv := newScopeTestInvestigator(cfg)

			detA := inv.anomalyDetectorFor("rr-a")
			detB := inv.anomalyDetectorFor("rr-b")
			Expect(detA).NotTo(BeIdenticalTo(detB), "rr-a and rr-b must get distinct detector instances")

			args := json.RawMessage(`{}`)
			// rr-b consumes its full per-tool budget (2) for kubectl_describe up front.
			Expect(detB.CheckToolCall("kubectl_describe", args).Allowed).To(BeTrue())
			Expect(detB.CheckToolCall("kubectl_describe", args).Allowed).To(BeTrue())

			// Concurrently hammer rr-a's detector with Reset() and CheckToolCall,
			// simulating rr-a's own phase-transition Reset() firing while rr-b is
			// mid-flight on the same KA pod. Against the pre-fix pod-wide singleton
			// this would reset/mutate rr-b's shared counters too.
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				for i := 0; i < 50; i++ {
					inv.anomalyDetectorFor("rr-a").Reset()
				}
			}()
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				for i := 0; i < 50; i++ {
					inv.anomalyDetectorFor("rr-a").CheckToolCall("kubectl_describe", args)
				}
			}()
			wg.Wait()

			// rr-b's per-tool budget for kubectl_describe must still read as
			// exhausted, unaffected by any of rr-a's concurrent Reset() calls.
			result := detB.CheckToolCall("kubectl_describe", args)
			Expect(result.Allowed).To(BeFalse(),
				"rr-b's own per-tool budget must remain exhausted; a shared singleton would have been reset by rr-a's concurrent Reset() calls")
			Expect(result.Reason).To(ContainSubstring("per-tool"))
		})
	})

	Describe("UT-KA-1892-002: pruneAnomalyDetectors removes idle entries and keeps recently-touched ones", func() {
		It("evicts only entries whose lastAccess predates the cutoff", func() {
			inv := newScopeTestInvestigator(DefaultAnomalyConfig())

			staleA := inv.anomalyDetectorFor("stale-a")
			time.Sleep(20 * time.Millisecond)
			freshC := inv.anomalyDetectorFor("fresh-c")

			removed := inv.pruneAnomalyDetectors(10 * time.Millisecond)
			Expect(removed).To(Equal(1), "only the idle entry older than the cutoff should be pruned")

			// A pruned correlationID gets a brand-new clone on next access.
			Expect(inv.anomalyDetectorFor("stale-a")).NotTo(BeIdenticalTo(staleA),
				"stale-a should have been evicted and re-created fresh on next access")
			// A recently-touched entry survives the sweep untouched.
			Expect(inv.anomalyDetectorFor("fresh-c")).To(BeIdenticalTo(freshC),
				"fresh-c was touched after the cutoff and must not be pruned")
		})
	})
})
