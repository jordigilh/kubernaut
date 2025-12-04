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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// =============================================================================
// Unit Tests: Metrics Implementation
// Per TESTING_GUIDELINES.md: These tests validate function/method behavior
// and error handling, not business outcomes.
// =============================================================================

var _ = Describe("Metrics", func() {
	var metrics *Metrics

	BeforeEach(func() {
		// TDD: Metrics struct is defined by tests
		// Reset registry for each test
		metrics = NewMetrics()
	})

	AfterEach(func() {
		// Unregister metrics to avoid conflicts between tests
		if metrics != nil {
			metrics.Unregister()
		}
	})

	Context("when recording phase transitions", func() {
		It("should increment phase transition counter", func() {
			// When: Recording a phase transition
			metrics.RecordPhaseTransition("default", "increase-memory", "Pending", "Running")

			// Then: Counter should be incremented
			count := testutil.ToFloat64(metrics.PhaseTransitions.WithLabelValues("default", "increase-memory", "Pending", "Running"))
			Expect(count).To(Equal(1.0))
		})

		It("should track different phases separately", func() {
			// When: Recording multiple different transitions
			metrics.RecordPhaseTransition("default", "wf-1", "Pending", "Running")
			metrics.RecordPhaseTransition("default", "wf-1", "Running", "Completed")
			metrics.RecordPhaseTransition("default", "wf-2", "Pending", "Skipped")

			// Then: Each transition is tracked separately
			Expect(testutil.ToFloat64(metrics.PhaseTransitions.WithLabelValues("default", "wf-1", "Pending", "Running"))).To(Equal(1.0))
			Expect(testutil.ToFloat64(metrics.PhaseTransitions.WithLabelValues("default", "wf-1", "Running", "Completed"))).To(Equal(1.0))
			Expect(testutil.ToFloat64(metrics.PhaseTransitions.WithLabelValues("default", "wf-2", "Pending", "Skipped"))).To(Equal(1.0))
		})
	})

	Context("when recording execution duration", func() {
		It("should observe duration in histogram", func() {
			// When: Recording execution duration
			metrics.RecordExecutionDuration("default", "increase-memory", "Completed", 30*time.Second)

			// Then: Histogram should have observation
			// Note: We can't easily check histogram values, but we can check it doesn't panic
			Expect(func() {
				metrics.RecordExecutionDuration("default", "increase-memory", "Completed", 45*time.Second)
			}).NotTo(Panic())
		})
	})

	Context("when recording PipelineRun creation", func() {
		It("should increment creation success counter", func() {
			// When: Recording successful creation
			metrics.RecordPipelineRunCreation("default", "increase-memory", true)

			// Then: Success counter should be incremented
			count := testutil.ToFloat64(metrics.PipelineRunCreations.WithLabelValues("default", "increase-memory", "success"))
			Expect(count).To(Equal(1.0))
		})

		It("should increment creation failure counter", func() {
			// When: Recording failed creation
			metrics.RecordPipelineRunCreation("default", "increase-memory", false)

			// Then: Failure counter should be incremented
			count := testutil.ToFloat64(metrics.PipelineRunCreations.WithLabelValues("default", "increase-memory", "failure"))
			Expect(count).To(Equal(1.0))
		})
	})

	Context("when recording skipped executions", func() {
		It("should increment skip counter with reason", func() {
			// When: Recording skipped execution due to ResourceBusy
			metrics.RecordSkipped("default", "increase-memory", "ResourceBusy")

			// Then: Skip counter should be incremented
			count := testutil.ToFloat64(metrics.SkippedTotal.WithLabelValues("default", "increase-memory", "ResourceBusy"))
			Expect(count).To(Equal(1.0))
		})

		It("should track different skip reasons separately", func() {
			// When: Recording skips with different reasons
			metrics.RecordSkipped("default", "wf-1", "ResourceBusy")
			metrics.RecordSkipped("default", "wf-1", "RecentlyRemediated")
			metrics.RecordSkipped("default", "wf-1", "RecentlyRemediated")

			// Then: Each reason is tracked separately
			Expect(testutil.ToFloat64(metrics.SkippedTotal.WithLabelValues("default", "wf-1", "ResourceBusy"))).To(Equal(1.0))
			Expect(testutil.ToFloat64(metrics.SkippedTotal.WithLabelValues("default", "wf-1", "RecentlyRemediated"))).To(Equal(2.0))
		})
	})

	Context("when recording failed executions", func() {
		It("should increment failure counter with reason", func() {
			// When: Recording failed execution
			metrics.RecordFailed("default", "increase-memory", "OOMKilled")

			// Then: Failure counter should be incremented
			count := testutil.ToFloat64(metrics.FailedTotal.WithLabelValues("default", "increase-memory", "OOMKilled"))
			Expect(count).To(Equal(1.0))
		})
	})

	Context("when recording completed executions", func() {
		It("should increment completed counter", func() {
			// When: Recording completed execution
			metrics.RecordCompleted("default", "increase-memory")

			// Then: Completed counter should be incremented
			count := testutil.ToFloat64(metrics.CompletedTotal.WithLabelValues("default", "increase-memory"))
			Expect(count).To(Equal(1.0))
		})
	})

	Context("when tracking active executions gauge", func() {
		It("should track active execution count", func() {
			// When: Setting active count
			metrics.SetActiveExecutions(5)

			// Then: Gauge should reflect the count
			count := testutil.ToFloat64(metrics.ActiveExecutions)
			Expect(count).To(Equal(5.0))
		})

		It("should update when executions change", func() {
			// When: Updating active count
			metrics.SetActiveExecutions(3)
			metrics.SetActiveExecutions(7)
			metrics.SetActiveExecutions(2)

			// Then: Gauge should reflect latest count
			count := testutil.ToFloat64(metrics.ActiveExecutions)
			Expect(count).To(Equal(2.0))
		})
	})

	Context("when recording reconcile operations", func() {
		It("should increment reconcile counter", func() {
			// When: Recording successful reconcile
			metrics.RecordReconcile("default", true)

			// Then: Success counter should be incremented
			count := testutil.ToFloat64(metrics.ReconcileTotal.WithLabelValues("default", "success"))
			Expect(count).To(Equal(1.0))
		})

		It("should record reconcile errors separately", func() {
			// When: Recording failed reconcile
			metrics.RecordReconcile("default", false)

			// Then: Error counter should be incremented
			count := testutil.ToFloat64(metrics.ReconcileTotal.WithLabelValues("default", "error"))
			Expect(count).To(Equal(1.0))
		})

		It("should observe reconcile duration", func() {
			// When: Recording reconcile duration
			metrics.RecordReconcileDuration(100 * time.Millisecond)

			// Then: Should not panic
			Expect(func() {
				metrics.RecordReconcileDuration(50 * time.Millisecond)
			}).NotTo(Panic())
		})
	})
})

