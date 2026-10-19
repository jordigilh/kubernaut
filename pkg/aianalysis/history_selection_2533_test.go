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

package aianalysis_test

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

var _ = Describe("Issue #2533 AIAnalysis historical decision audit source", func() {
	It("UT-AA-2533-001: emits bounded workflow rationale alongside repeat justification", func() {
		analysis := buildLowConfidenceFloorTestAnalysis()
		result := selectionResult2496(.84, false)
		mapped := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), result, "rr-aa-2533")
		store := &aiAuditStoreSpy{}
		auditClient := aiaudit.NewAuditClient(store, logr.Discard())

		_, err := handlers.NewResponseProcessor(logr.Discard(), metrics.NewMetrics(), auditClient).
			ProcessAgentSessionResult(context.Background(), analysis, mapped)
		Expect(err).NotTo(HaveOccurred())
		auditClient.RecordAnalysisComplete(context.Background(), analysis)
		if len(store.events) == 0 {
			Fail("expected RecordAnalysisComplete to record an audit event")
		}
		event := store.events[len(store.events)-1]
		Expect(event.EventType).To(Equal(aiaudit.EventTypeAnalysisCompleted))

		raw, err := json.Marshal(event.EventData)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring(`"workflow_rationale":"Selection based on current memory observations"`))
		Expect(string(raw)).To(ContainSubstring(`"repeat_justification":{"evidence_basis":"New memory limit observed"`))
		Expect(string(raw)).To(ContainSubstring(`"prior_outcome_relevance":"Prior Full assessment was ineffective"`))
		Expect(string(raw)).To(ContainSubstring(`"expected_mechanism":"Increase the current limit"`))
		Expect(string(raw)).NotTo(ContainSubstring(`"parameters"`))
	})
})
