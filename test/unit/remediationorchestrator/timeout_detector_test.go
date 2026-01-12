/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/timeout"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("TimeoutDetector", func() {
	var (
		config   remediationorchestrator.OrchestratorConfig
		detector *timeout.Detector
	)

	BeforeEach(func() {
		config = remediationorchestrator.DefaultConfig()
	})

	Describe("CheckGlobalTimeout", func() {
		BeforeEach(func() {
			detector = timeout.NewDetector(config)
		})

		// Test #2: CheckGlobalTimeout returns true when exceeded (BR-ORCH-027)
		It("should return TimedOut=true when global timeout exceeded", func() {
			// Create RR with creation timestamp 2 hours ago (exceeds 60min default)
			rr := helpers.NewRemediationRequest("test-rr", "default")
			rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

			result := detector.CheckGlobalTimeout(rr)

			Expect(result.TimedOut).To(BeTrue())
			Expect(result.TimedOutPhase).To(Equal("global"))
			Expect(result.Elapsed).To(BeNumerically(">=", 2*time.Hour))
		})

		// Test #3: CheckGlobalTimeout returns false when not exceeded (BR-ORCH-027)
		It("should return TimedOut=false when global timeout not exceeded", func() {
			// Create RR with creation timestamp 5 minutes ago (within 60min default)
			rr := helpers.NewRemediationRequest("test-rr", "default")
			rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-5 * time.Minute))

			result := detector.CheckGlobalTimeout(rr)

			Expect(result.TimedOut).To(BeFalse())
			Expect(result.TimedOutPhase).To(BeEmpty())
		})

		// Test #4: CheckGlobalTimeout uses per-RR override when set (BR-ORCH-027)
		It("should use per-RR timeout override when set", func() {
			// Create RR with creation timestamp 30 minutes ago
			rr := helpers.NewRemediationRequest("test-rr", "default")
			rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-30 * time.Minute))
			// Set per-RR override to 15 minutes (should trigger timeout)
			globalTimeout := metav1.Duration{Duration: 15 * time.Minute}
			rr.Spec.TimeoutConfig = &remediationv1.TimeoutConfig{
				Global: &globalTimeout,
			}

			result := detector.CheckGlobalTimeout(rr)

			Expect(result.TimedOut).To(BeTrue())
			Expect(result.TimedOutPhase).To(Equal("global"))
		})
	})

	Describe("CheckTimeout", func() {
		BeforeEach(func() {
			detector = timeout.NewDetector(config)
		})

		Context("Terminal Phases", func() {
			// Test #5: CheckTimeout skips terminal phases (Completed)
			It("should skip timeout check for Completed phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour)) // Would exceed timeout
				rr.Status.OverallPhase = "Completed"

				result := detector.CheckTimeout(rr)

				Expect(result.TimedOut).To(BeFalse())
			})

			// Test #6: CheckTimeout skips terminal phases (Failed)
			It("should skip timeout check for Failed phase", func() {
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour)) // Would exceed timeout
				rr.Status.OverallPhase = "Failed"

				result := detector.CheckTimeout(rr)

				Expect(result.TimedOut).To(BeFalse())
			})
		})
	})
})
