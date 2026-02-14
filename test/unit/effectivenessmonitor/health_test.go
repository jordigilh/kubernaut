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

package effectivenessmonitor

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

var _ = Describe("Health Check Scorer (BR-EM-001)", func() {

	var scorer health.Scorer

	BeforeEach(func() {
		scorer = health.NewScorer()
	})

	// ========================================
	// UT-EM-HC-001: All pods ready, no restarts -> 1.0
	// ========================================
	Describe("Score (UT-EM-HC-001 through UT-EM-HC-006)", func() {

		It("UT-EM-HC-001: should return 1.0 when all pods ready, no restarts", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           3,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(1.0))
			Expect(result.Component).To(Equal(types.ComponentHealth))
		})

		// UT-EM-HC-002: All pods ready, restarts detected -> 0.75
		It("UT-EM-HC-002: should return 0.75 when all pods ready but restarts detected", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           3,
				RestartsSinceRemediation: 2,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.75))
		})

		// UT-EM-HC-003: Partial readiness -> 0.5
		It("UT-EM-HC-003: should return 0.5 when partial readiness", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           2,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.5))
		})

		// UT-EM-HC-004: No pods ready -> 0.0
		It("UT-EM-HC-004: should return 0.0 when no pods are ready", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           0,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
		})

		// UT-EM-HC-005: Target resource not found -> 0.0
		It("UT-EM-HC-005: should return 0.0 when target resource not found", func() {
			status := health.TargetStatus{
				TotalReplicas:           0,
				ReadyReplicas:           0,
				RestartsSinceRemediation: 0,
				TargetExists:           false,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
			Expect(result.Details).To(ContainSubstring("not found"))
		})

		// UT-EM-HC-006: Single replica fully healthy -> 1.0
		It("UT-EM-HC-006: should return 1.0 for single replica fully healthy", func() {
			status := health.TargetStatus{
				TotalReplicas:           1,
				ReadyReplicas:           1,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(*result.Score).To(Equal(1.0))
		})

		// Edge case: zero total replicas (scaled down)
		It("should handle zero total replicas (scaled down)", func() {
			status := health.TargetStatus{
				TotalReplicas:           0,
				ReadyReplicas:           0,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			// When target exists but has 0 replicas, score is 0.0 (no running pods)
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
		})

		// Edge case: partial readiness with restarts
		It("should handle partial readiness with restarts", func() {
			status := health.TargetStatus{
				TotalReplicas:           4,
				ReadyReplicas:           2,
				RestartsSinceRemediation: 5,
				TargetExists:           true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			// Partial readiness takes precedence over restarts -> 0.5
			Expect(*result.Score).To(Equal(0.5))
		})

		// ========================================
		// v2.5: CrashLoopBackOff and OOMKilled detection (DD-017 v2.5)
		// ========================================

		// UT-EM-HC-007: CrashLoopBackOff detected -> 0.0
		It("UT-EM-HC-007: should return 0.0 when CrashLoopBackOff detected", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           2,
				RestartsSinceRemediation: 5,
				TargetExists:           true,
				CrashLoops:             true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
			Expect(result.Details).To(ContainSubstring("CrashLoopBackOff"))
		})

		// UT-EM-HC-008: OOMKilled detected with all pods ready -> 0.25
		It("UT-EM-HC-008: should return 0.25 when OOMKilled detected with all pods ready", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           3,
				RestartsSinceRemediation: 1,
				TargetExists:           true,
				OOMKilled:              true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.25))
			Expect(result.Details).To(ContainSubstring("OOMKilled"))
		})

		// UT-EM-HC-009: Both CrashLoopBackOff and OOMKilled -> 0.0 (CrashLoop takes precedence)
		It("UT-EM-HC-009: should return 0.0 when both CrashLoopBackOff and OOMKilled detected", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           1,
				RestartsSinceRemediation: 10,
				TargetExists:           true,
				CrashLoops:             true,
				OOMKilled:              true,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
			Expect(result.Details).To(ContainSubstring("CrashLoopBackOff"))
		})

		// UT-EM-HC-010: All healthy with no CrashLoop/OOM -> 1.0 (unchanged)
		It("UT-EM-HC-010: should still return 1.0 when healthy with no CrashLoop/OOM", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           3,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
				CrashLoops:             false,
				OOMKilled:              false,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(*result.Score).To(Equal(1.0))
		})

		// ========================================
		// v2.5: Pending pod detection (DD-017 v2.5)
		// ========================================

		// UT-EM-HC-011: All pods pending -> 0.0
		It("UT-EM-HC-011: should return 0.0 when all pods are pending", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           0,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
				PendingCount:           3,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
			Expect(result.Details).To(ContainSubstring("pending"))
		})

		// UT-EM-HC-012: Some pods pending, some ready -> 0.5 (partial readiness)
		It("UT-EM-HC-012: should return 0.5 when some pods pending and some ready", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           2,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
				PendingCount:           1,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.5))
			Expect(result.Details).To(ContainSubstring("pending"))
		})

		// UT-EM-HC-013: CrashLoopBackOff takes precedence over pending
		It("UT-EM-HC-013: CrashLoopBackOff should take precedence over pending", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           0,
				RestartsSinceRemediation: 5,
				TargetExists:           true,
				CrashLoops:             true,
				PendingCount:           2,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
			Expect(result.Details).To(ContainSubstring("CrashLoopBackOff"))
		})

		// UT-EM-HC-014: All ready, zero pending -> 1.0 (unchanged behavior)
		It("UT-EM-HC-014: should return 1.0 when all ready and zero pending", func() {
			status := health.TargetStatus{
				TotalReplicas:           3,
				ReadyReplicas:           3,
				RestartsSinceRemediation: 0,
				TargetExists:           true,
				PendingCount:           0,
			}

			result := scorer.Score(context.Background(), status)
			Expect(result.Assessed).To(BeTrue())
			Expect(*result.Score).To(Equal(1.0))
		})
	})
})
