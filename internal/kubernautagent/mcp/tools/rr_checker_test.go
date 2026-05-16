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

package tools_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

type fakeRRChecker struct {
	exists bool
	err    error
}

func (f *fakeRRChecker) RemediationRequestExists(_ context.Context, _ string) (bool, error) {
	return f.exists, f.err
}

var _ = Describe("RR existence checker — HARM-004 hardening", func() {

	Describe("UT-KA-RR-001: Non-NotFound K8s errors propagate as internal errors", func() {
		It("should wrap the API server error in handleStart", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-rr-err",
					CorrelationID: "rr-api-timeout",
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}
			checker := &fakeRRChecker{exists: false, err: errors.New("API server timeout")}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{}, mcptools.WithRRExistenceChecker(checker))
			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-api-timeout",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validate remediation request"))
			Expect(err.Error()).To(ContainSubstring("API server timeout"))
			var mcpErr *mcptools.MCPError
			Expect(errors.As(err, &mcpErr)).To(BeFalse(),
				"non-NotFound K8s errors should not be wrapped as MCPError — they are internal failures")
		})
	})

	Describe("UT-KA-RR-002: RR existence check passes when RR exists", func() {
		It("should proceed to Takeover without error", func() {
			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-rr-ok",
					CorrelationID: "rr-exists",
				},
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}
			checker := &fakeRRChecker{exists: true, err: nil}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mcptools.NopAutonomousManager{}, mcptools.WithRRExistenceChecker(checker))
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-exists",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
		})
	})
})
