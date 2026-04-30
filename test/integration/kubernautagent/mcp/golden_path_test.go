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

package mcp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

type goldenPathRunner struct {
	response string
	delay    time.Duration
}

func (r *goldenPathRunner) RunInteractiveTurn(_ context.Context, _ []mcptools.LLMMessage, _ string) (string, error) {
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	return r.response, nil
}

type goldenPathRecon struct{}

func (r *goldenPathRecon) Reconstruct(_ context.Context, _, _ string) ([]mcpinternal.ConversationTurn, error) {
	return []mcpinternal.ConversationTurn{
		{Role: "user", Content: "prior question", Timestamp: time.Now().Add(-5 * time.Minute)},
		{Role: "assistant", Content: "prior answer", Timestamp: time.Now().Add(-4 * time.Minute)},
	}, nil
}

type goldenPathAutoMgr struct {
	cancelled bool
}

func (m *goldenPathAutoMgr) FindByRemediationID(_ string) (string, bool) { return "auto-001", true }
func (m *goldenPathAutoMgr) CancelInvestigation(_ string) error {
	m.cancelled = true
	return nil
}

var _ = Describe("Golden Path Lifecycle — IT-KA-GOLDEN-001", func() {
	var (
		tool    *mcptools.InvestigateTool
		runner  *goldenPathRunner
		autoMgr *goldenPathAutoMgr
		logger  *slog.Logger
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		runner = &goldenPathRunner{response: "The OOM was caused by memory leak in deployment/foo", delay: 50 * time.Millisecond}
		recon := &goldenPathRecon{}
		autoMgr = &goldenPathAutoMgr{}

		scheme := k8sruntime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		sessMgr := mcpinternal.NewLeaseSessionManagerConcrete(fakeClient, "kubernaut", logger)
		tool = mcptools.NewInvestigateTool(sessMgr, runner, recon, autoMgr)
	})

	Describe("IT-KA-GOLDEN-001: Full lifecycle: status → takeover → message → status → complete", func() {
		It("should complete the full interactive lifecycle without errors or goroutine leaks", func() {
			goroutinesBefore := runtime.NumGoroutine()
			user := mcpinternal.UserInfo{Username: "alice@example.com"}
			ctx := context.Background()

			// Step 1: Status → autonomous
			statusResult, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID: "rr-golden-001", Action: mcptools.ActionStatus,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			var status mcptools.StatusOutput
			Expect(json.Unmarshal([]byte(statusResult.Response), &status)).To(Succeed())
			Expect(status.Mode).To(Equal(mcptools.StatusModeAutonomous))

			// Step 2: Takeover (cancels autonomous)
			takeoverResult, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID: "rr-golden-001", Action: mcptools.ActionTakeover,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(takeoverResult.Status).To(Equal("takeover_started"))
			Expect(takeoverResult.SessionID).NotTo(BeEmpty())
			Expect(autoMgr.cancelled).To(BeTrue())
			Expect(takeoverResult.Response).To(ContainSubstring("2 prior turns"))

			// Step 3: Message
			msgResult, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID: "rr-golden-001", Action: mcptools.ActionMessage, Message: "what caused the OOM?",
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(msgResult.Status).To(Equal("message_received"))
			Expect(msgResult.Response).To(ContainSubstring("OOM"))

			// Step 4: Status → interactive
			statusResult2, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID: "rr-golden-001", Action: mcptools.ActionStatus,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			var status2 mcptools.StatusOutput
			Expect(json.Unmarshal([]byte(statusResult2.Response), &status2)).To(Succeed())
			Expect(status2.Mode).To(Equal(mcptools.StatusModeInteractive))
			Expect(status2.Driver).To(Equal("alice@example.com"))

			// Step 5: Complete
			completeResult, err := tool.Handle(ctx, mcptools.InvestigateInput{
				RRID: "rr-golden-001", Action: mcptools.ActionComplete,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(completeResult.Status).To(Equal("completed"))

			// Step 6: Goroutine leak check
			time.Sleep(100 * time.Millisecond)
			goroutinesAfter := runtime.NumGoroutine()
			Expect(goroutinesAfter - goroutinesBefore).To(BeNumerically("<=", 5))
		})
	})

	Describe("IT-KA-TAKE-002: Rapid connect/disconnect does not leak goroutines", func() {
		It("should not leak goroutines after repeated takeover/release cycles", func() {
			user := mcpinternal.UserInfo{Username: "bob@example.com"}
			ctx := context.Background()
			goroutinesBefore := runtime.NumGoroutine()

			for i := 0; i < 5; i++ {
				_, err := tool.Handle(ctx, mcptools.InvestigateInput{
					RRID: "rr-leak-001", Action: mcptools.ActionTakeover,
				}, user)
				Expect(err).NotTo(HaveOccurred())

				_, err = tool.Handle(ctx, mcptools.InvestigateInput{
					RRID: "rr-leak-001", Action: mcptools.ActionComplete,
				}, user)
				Expect(err).NotTo(HaveOccurred())
			}

			time.Sleep(200 * time.Millisecond)
			goroutinesAfter := runtime.NumGoroutine()
			Expect(goroutinesAfter - goroutinesBefore).To(BeNumerically("<=", 5))
		})
	})
})
