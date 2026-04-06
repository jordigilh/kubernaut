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
package mockllm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Scenario Catalog Validation", func() {

	var registry *scenarios.Registry

	BeforeEach(func() {
		registry = scenarios.DefaultRegistry()
	})

	DescribeTable("UT-MOCK-026: Each scenario config matches Python MOCK_SCENARIOS values",
		func(name, expectedSignal, expectedSeverity, expectedWorkflowID string, expectedConfidence float64, rootCauseSubstr string) {
			s, ok := registry.Get(name)
			Expect(ok).To(BeTrue(), "scenario %q not found in registry", name)

			withCfg, ok := s.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue(), "scenario %q does not implement ScenarioWithConfig", name)

			cfg := withCfg.Config()
			Expect(cfg.SignalName).To(Equal(expectedSignal))
			Expect(cfg.Severity).To(Equal(expectedSeverity))
			Expect(cfg.Confidence).To(BeNumerically("~", expectedConfidence, 0.01))
			Expect(cfg.RootCause).To(ContainSubstring(rootCauseSubstr))
			if expectedWorkflowID != "" {
				Expect(cfg.WorkflowID).To(Equal(expectedWorkflowID))
			} else {
				Expect(cfg.WorkflowID).To(BeEmpty())
			}
		},
		Entry("UT-MOCK-026-001: oomkilled",
			"oomkilled", "OOMKilled", "critical",
			uuid.DeterministicUUID("oomkill-increase-memory-v1"), 0.95,
			"memory limits"),
		Entry("UT-MOCK-026-002: crashloop",
			"crashloop", "CrashLoopBackOff", "high",
			uuid.DeterministicUUID("crashloop-config-fix-v1"), 0.95,
			"invalid configuration directive"),
		Entry("UT-MOCK-026-003: node_not_ready",
			"node_not_ready", "NodeNotReady", "critical",
			uuid.DeterministicUUID("node-drain-reboot-v1"), 0.90,
			"disk pressure"),
		Entry("UT-MOCK-026-004: test_signal",
			"test_signal", "TestSignal", "critical",
			uuid.DeterministicUUID("test-signal-handler-v1"), 0.90,
			"graceful shutdown"),
		Entry("UT-MOCK-026-005: no_workflow_found",
			"no_workflow_found", "MOCK_NO_WORKFLOW_FOUND", "critical",
			"", 0.0,
			"No suitable workflow"),
		Entry("UT-MOCK-026-006: low_confidence",
			"low_confidence", "MOCK_LOW_CONFIDENCE", "critical",
			uuid.DeterministicUUID("generic-restart-v1"), 0.35,
			"human judgment"),
		Entry("UT-MOCK-026-007: problem_resolved",
			"problem_resolved", "MOCK_PROBLEM_RESOLVED", "low",
			"", 0.85,
			"self-resolved"),
		Entry("UT-MOCK-026-008: rca_incomplete",
			"rca_incomplete", "MOCK_RCA_INCOMPLETE", "critical",
			uuid.DeterministicUUID("generic-restart-v1"), 0.88,
			"affected resource could not be determined"),
		Entry("UT-MOCK-026-009: oomkilled_predictive",
			"oomkilled_predictive", "OOMKilled", "critical",
			uuid.DeterministicUUID("oomkill-increase-memory-v1"), 0.88,
			"Predicted OOMKill"),
		Entry("UT-MOCK-026-010: cert_not_ready",
			"cert_not_ready", "CertManagerCertNotReady", "critical",
			uuid.DeterministicUUID("fix-certificate-v1"), 0.92,
			"Certificate stuck"),
	)
})
