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

package kubernautagent

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// E2E-KA-1390-001: Nil-result resilience — session completes with nil result →
// GET /result returns HTTP 200 with structured synthetic result → no 409 loop.
var _ = Describe("E2E-KA-1390-001: Nil-Result Resilience", Label("e2e", "ka", "1390"), func() {

	It("should return structured result for completed session with nil result [SC-24, SI-13]", func() {
		// Submit an investigation that will complete. After the mock LLM
		// completes, the session reaches a terminal state. If the mock
		// scenario returns an empty/nil result, GetResult must still
		// return HTTP 200 with a synthetic result body.
		req := &agentclient.IncidentRequest{
			IncidentID:        "test-nil-result-1390",
			RemediationID:     "rem-nil-result-1390",
			SignalName:        "CrashLoopBackOff",
			Severity:          "warning",
			SignalSource:      "kubernetes",
			ResourceNamespace: "default",
			ResourceKind:      "Pod",
			ResourceName:      "nil-result-pod",
			ErrorMessage:      "Container exited with code 137",
			Environment:       "staging",
			Priority:          "P2",
			RiskTolerance:     "high",
			BusinessCategory:  "standard",
			ClusterName:       "e2e-test",
		}

		By("submitting investigation")
		sessionID, err := sessionClient.SubmitInvestigation(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		Expect(sessionID).ToNot(BeEmpty())

		By("waiting for session to reach terminal state")
		Eventually(func() string {
			result, pollErr := sessionClient.PollSession(ctx, sessionID)
			if pollErr != nil || result == nil {
				return "error"
			}
			return result.Status
		}, 2*time.Minute, 2*time.Second).Should(
			BeElementOf("completed", "failed", "cancelled"),
			"session must reach terminal state",
		)

		By("fetching result — must return 200, not 409")
		result, err := sessionClient.GetSessionResult(ctx, sessionID)
		Expect(err).ToNot(HaveOccurred(),
			"GET /result must return 200 for terminal sessions — nil-result 409 loop prevented")
		Expect(result).ToNot(BeNil(), "response must contain a structured result body")
		Expect(result.Analysis).ToNot(BeEmpty(),
			"synthetic result must include non-empty analysis field")
	})
})
