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

package agentsession_test

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Typed investigation outcome mapping — #2521", func() {
	It("UT-AA-KA-2521-001: carries terminal and gate status through AgentSession", func() {
		result := agentsession.MapInvestigationResultToAgentSessionResult(logr.Discard(), &katypes.InvestigationResult{
			TerminalOutcome:         sharedtypes.TerminalOutcomeEvidenceTransportUnavailable,
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure,
			HistoryStatus:           sharedtypes.HistoryStatusTransportUnavailable,
			HumanReviewNeeded:       true,
			HumanReviewReason:       katypes.HumanReviewReasonRCAIncomplete,
		}, "incident-1")

		Expect(result.TerminalOutcome).To(Equal(sharedtypes.TerminalOutcomeEvidenceTransportUnavailable))
		Expect(result.WorkflowDiscoveryStatus).To(Equal(sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure))
		Expect(result.HistoryStatus).To(Equal(sharedtypes.HistoryStatusTransportUnavailable))
		Expect(result.HumanReviewReason).To(Equal(katypes.HumanReviewReasonRCAIncomplete))
	})
})
