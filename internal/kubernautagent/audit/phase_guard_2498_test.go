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

package audit

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"
)

var _ = Describe("DD-KA-2496 phase-rejection audit (BR-AUDIT-005, AU-2/AU-3, CC7.2)", func() {
	It("UT-KA-2498-003: the typed DS payload retains server phase and standardized rejection details", func() {
		event := NewEvent(EventTypeLLMToolCall, "rr-audit-2498")
		event.EventOutcome = OutcomeFailure
		event.Data["tool_call_index"] = 0
		event.Data["tool_name"] = "get_namespaced_resource_context"
		event.Data["tool_result"] = `{"status":"error","error":"tool not allowed in phase"}`
		event.Data["phase"] = "workflow_discovery"
		event.Data["error_details"] = &sharedaudit.ErrorDetails{
			Message: "tool not allowed in phase", Code: "ERR_TOOL_PHASE_DENIED", Component: "kubernautagent", RetryPossible: false,
		}

		payload, ok := buildEventData(event)
		Expect(ok).To(BeTrue())
		raw, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())
		var decoded map[string]interface{}
		Expect(json.Unmarshal(raw, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKeyWithValue("phase", "workflow_discovery"))
		Expect(decoded).To(HaveKeyWithValue("error_details", SatisfyAll(
			HaveKeyWithValue("code", "ERR_TOOL_PHASE_DENIED"),
			HaveKeyWithValue("component", "kubernautagent"),
			HaveKeyWithValue("retry_possible", false),
		)))
	})
})
