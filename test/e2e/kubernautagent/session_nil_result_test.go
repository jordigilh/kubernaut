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

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-KA-1390-001: Nil-result resilience — session completes with nil result →
// AgentSession.Status.Result must still be a structured, non-empty body (no
// caller-visible nil/empty-result gap). Trigger mechanism migrated off the
// retired HTTP session endpoints to direct AgentSession CRD creation
// (issue #2190, DD-AA-KA-001) — business assertion is unaffected.
var _ = Describe("E2E-KA-1390-001: Nil-Result Resilience", Label("e2e", "ka", "1390"), func() {

	It("should return structured result for completed session with nil result [SC-24, SI-13]", func() {
		// Create an AgentSession that will complete. If the mock scenario
		// returns an empty/nil result, Status.Result must still be a
		// synthetic, structured, non-empty body once Phase=Completed.
		spec := agentsessionv1.AgentSessionSpec{
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

		By("creating the AgentSession and waiting for a terminal phase")
		result, err := infrastructure.InvestigateViaAgentSession(ctx, k8sClient, sharedNamespace, spec, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(),
			"AgentSession must reach Completed with a structured Result — nil-result gap prevented")
		Expect(result).ToNot(BeNil(), "Status.Result must contain a structured result body")
		Expect(result.Analysis).ToNot(BeEmpty(),
			"synthetic result must include non-empty analysis field")
	})
})
