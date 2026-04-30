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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

type statusSessionMgr struct {
	driverSession *mcpinternal.InteractiveSession
	driverErr     error
}

func (m *statusSessionMgr) Takeover(_ context.Context, _ string, _ mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	return nil, nil
}
func (m *statusSessionMgr) Release(_ string, _ string) error { return nil }
func (m *statusSessionMgr) GetDriver(_ string) (*mcpinternal.InteractiveSession, error) {
	return m.driverSession, m.driverErr
}
func (m *statusSessionMgr) IsDriverActive(_ string) bool {
	return m.driverSession != nil
}

type statusAutoMgr struct {
	found bool
}

func (m *statusAutoMgr) FindByRemediationID(_ string) (string, bool) { return "auto-sess", m.found }
func (m *statusAutoMgr) CancelInvestigation(_ string) error { return nil }

var _ = Describe("action=status — PR4 PROD-01 BR-INTERACTIVE-002", func() {

	Describe("UT-KA-STATUS-001: action=status returns mode 'autonomous' when no driver active", func() {
		It("should return autonomous mode with no driver info", func() {
			sessMgr := &statusSessionMgr{driverSession: nil}
			autoMgr := &statusAutoMgr{found: true}
			tool := mcptools.NewInvestigateTool(sessMgr, nil, nil, autoMgr)

			input := mcptools.InvestigateInput{
				RRID:   "rr-status-001",
				Action: mcptools.ActionStatus,
			}

			result, err := tool.Handle(context.Background(), input, mcpinternal.UserInfo{Username: "observer@example.com"})
			Expect(err).NotTo(HaveOccurred())

			var status mcptools.StatusOutput
			Expect(json.Unmarshal([]byte(result.Response), &status)).To(Succeed())
			Expect(status.Mode).To(Equal("autonomous"))
			Expect(status.RRID).To(Equal("rr-status-001"))
		})
	})

	Describe("UT-KA-STATUS-002: action=status returns mode 'interactive' + driver identity when driver active", func() {
		It("should return interactive mode with driver username", func() {
			sessMgr := &statusSessionMgr{
				driverSession: &mcpinternal.InteractiveSession{
					SessionID:     "interactive-sess-001",
					CorrelationID: "rr-status-002",
					ActingUser:    mcpinternal.UserInfo{Username: "alice@example.com"},
					StartedAt:     time.Now(),
				},
			}
			autoMgr := &statusAutoMgr{found: true}
			tool := mcptools.NewInvestigateTool(sessMgr, nil, nil, autoMgr)

			input := mcptools.InvestigateInput{
				RRID:   "rr-status-002",
				Action: mcptools.ActionStatus,
			}

			result, err := tool.Handle(context.Background(), input, mcpinternal.UserInfo{Username: "observer@example.com"})
			Expect(err).NotTo(HaveOccurred())

			var status mcptools.StatusOutput
			Expect(json.Unmarshal([]byte(result.Response), &status)).To(Succeed())
			Expect(status.Mode).To(Equal("interactive"))
			Expect(status.Driver).To(Equal("alice@example.com"))
		})
	})

	Describe("UT-KA-STATUS-003: action=status returns 'not_found' when no investigation running", func() {
		It("should return not_found mode when autonomous session doesn't exist and no driver", func() {
			sessMgr := &statusSessionMgr{driverSession: nil}
			autoMgr := &statusAutoMgr{found: false}
			tool := mcptools.NewInvestigateTool(sessMgr, nil, nil, autoMgr)

			input := mcptools.InvestigateInput{
				RRID:   "rr-status-003",
				Action: mcptools.ActionStatus,
			}

			result, err := tool.Handle(context.Background(), input, mcpinternal.UserInfo{Username: "observer@example.com"})
			Expect(err).NotTo(HaveOccurred())

			var status mcptools.StatusOutput
			Expect(json.Unmarshal([]byte(result.Response), &status)).To(Succeed())
			Expect(status.Mode).To(Equal("not_found"))
		})
	})
})
