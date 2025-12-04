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

package workflowexecution_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
)

var _ = Describe("WorkflowExecution Metrics", func() {

	// ========================================
	// Metrics Initialization Tests
	// ========================================
	Describe("InitMetrics", func() {
		It("should initialize without panicking", func() {
			// InitMetrics should be idempotent and safe to call multiple times
			Expect(func() {
				workflowexecution.InitMetrics()
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Phase Transition Metrics Tests
	// ========================================
	Describe("RecordPhaseTransition", func() {
		It("should record phase transitions without error", func() {
			Expect(func() {
				workflowexecution.RecordPhaseTransition("test-namespace", "Pending")
				workflowexecution.RecordPhaseTransition("test-namespace", "Running")
				workflowexecution.RecordPhaseTransition("test-namespace", "Completed")
				workflowexecution.RecordPhaseTransition("test-namespace", "Failed")
				workflowexecution.RecordPhaseTransition("test-namespace", "Skipped")
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Duration Metrics Tests
	// ========================================
	Describe("RecordDuration", func() {
		It("should record execution duration without error", func() {
			startTime := time.Now().Add(-5 * time.Minute)

			Expect(func() {
				workflowexecution.RecordDuration("test-namespace", "test-workflow", "Completed", startTime)
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Skip Metrics Tests (DD-WE-001)
	// ========================================
	Describe("RecordSkip", func() {
		It("should record ResourceBusy skips", func() {
			Expect(func() {
				workflowexecution.RecordSkip("test-namespace", "ResourceBusy")
			}).ToNot(Panic())
		})

		It("should record RecentlyRemediated skips", func() {
			Expect(func() {
				workflowexecution.RecordSkip("test-namespace", "RecentlyRemediated")
			}).ToNot(Panic())
		})
	})

	// ========================================
	// PipelineRun Creation Metrics Tests
	// ========================================
	Describe("RecordPipelineRunCreation", func() {
		It("should record successful PipelineRun creation", func() {
			Expect(func() {
				workflowexecution.RecordPipelineRunCreation("success")
			}).ToNot(Panic())
		})

		It("should record failed PipelineRun creation", func() {
			Expect(func() {
				workflowexecution.RecordPipelineRunCreation("failure")
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Reconcile Metrics Tests
	// ========================================
	Describe("RecordReconcile", func() {
		It("should record successful reconciliation", func() {
			Expect(func() {
				workflowexecution.RecordReconcile("test-namespace", "success")
			}).ToNot(Panic())
		})

		It("should record error reconciliation", func() {
			Expect(func() {
				workflowexecution.RecordReconcile("test-namespace", "error")
			}).ToNot(Panic())
		})

		It("should record requeue reconciliation", func() {
			Expect(func() {
				workflowexecution.RecordReconcile("test-namespace", "requeue")
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Reconcile Duration Metrics Tests
	// ========================================
	Describe("RecordReconcileDuration", func() {
		It("should record reconcile duration", func() {
			Expect(func() {
				workflowexecution.RecordReconcileDuration("test-namespace", 100*time.Millisecond)
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Active Executions Gauge Tests
	// ========================================
	Describe("SetActiveExecutions", func() {
		It("should set active executions count", func() {
			Expect(func() {
				workflowexecution.SetActiveExecutions("test-namespace", 5)
			}).ToNot(Panic())
		})

		It("should allow setting to zero", func() {
			Expect(func() {
				workflowexecution.SetActiveExecutions("test-namespace", 0)
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Resource Lock Check Metrics Tests
	// ========================================
	Describe("RecordResourceLockCheck", func() {
		It("should record lock check duration", func() {
			Expect(func() {
				workflowexecution.RecordResourceLockCheck(1 * time.Millisecond)
			}).ToNot(Panic())
		})
	})

	// ========================================
	// Cooldown Skip Metrics Tests
	// ========================================
	Describe("RecordCooldownSkip", func() {
		It("should record cooldown skips", func() {
			Expect(func() {
				workflowexecution.RecordCooldownSkip("test-namespace", "test-workflow")
			}).ToNot(Panic())
		})
	})
})

