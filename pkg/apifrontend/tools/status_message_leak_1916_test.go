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
	"fmt"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// statusLeak1916Scheme registers both AIAnalysis (session-ready check) and
// AgentSession (AwaitAgentSessionInteractive check, #2172) so a single fake
// client can drive either await path independently, without requiring both
// to be registered in each of the package's other single-purpose test
// schemes.
func statusLeak1916Scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = aiav1alpha1.AddToScheme(s)
	_ = agentsessionv1.AddToScheme(s)
	return s
}

func newStatusLeak1916Client(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(statusLeak1916Scheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

// statusTextsFrom extracts the text of every TaskStatusUpdateEvent captured
// by a bridgeQueue, in emission order.
func statusTextsFrom(queue *bridgeQueue) []string {
	var texts []string
	for _, evt := range queue.Events() {
		se, ok := evt.(*a2a.TaskStatusUpdateEvent)
		if !ok {
			continue
		}
		if se.Status.Message == nil || len(se.Status.Message.Parts) == 0 {
			continue
		}
		if tp, ok := se.Status.Message.Parts[0].(a2a.TextPart); ok {
			texts = append(texts, tp.Text)
		}
	}
	return texts
}

var _ = Describe("Interactive investigation status messages — no internal-name leakage (#1916)", func() {

	It("UT-AF-1916-001 [SI-11]: session-ready status omits internal acronym KA", func() {
		// #2170/DD-AA-KA-001: HandleAwaitSession's session-ready signal now
		// comes from AgentSession.Status.SessionID, not
		// AIAnalysis.Status.KASession.ID (see crd_tools_session.go's
		// HandleAwaitSession doc comment).
		tc := newStatusLeak1916Client(newTypedAgentSessionWithSessionID("as-1916-001", "rr-1916-001", "sess-1916-001"))

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1916-001",
					Status:    "started",
					Events:    nil,
					Closer:    func() {},
				}, nil
			},
		}

		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1916-001", "ctx-1916-001", nil)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		_, err := tools.HandleInvestigationMCPWithRegistry(
			ctx, &tools.InvestigateConfig{
				MCPClient: mockMCP,
				Client:    tc,
				Namespace: "kubernaut-system",
			}, tools.InvestigateMCPArgs{RRID: "rr-1916-001"},
			true, "alice",
		)
		Expect(err).NotTo(HaveOccurred())

		texts := statusTextsFrom(queue)
		var readyText string
		for _, t := range texts {
			if strings.HasPrefix(t, "Investigation session ready") {
				readyText = t
				break
			}
		}
		Expect(readyText).NotTo(BeEmpty(), "expected a session-ready status event, got: %v", texts)
		Expect(readyText).To(Equal("Investigation session ready, connecting..."),
			"SI-11: session-ready status must not reference the internal KA acronym")
		Expect(readyText).NotTo(ContainSubstring("KA"))
	})

	It("UT-AF-1916-002 [SI-11]: session-acknowledged status omits internal acronym AA", func() {
		origTimeout := tools.AwaitSessionTimeout
		tools.AwaitSessionTimeout = 10 * time.Millisecond
		defer func() { tools.AwaitSessionTimeout = origTimeout }()

		// Deliberately no AIAnalysis-with-session object: isolates the
		// AgentSession-Interactive path (#2172) from the session-ready path
		// covered by UT-AF-1916-001.
		tc := newStatusLeak1916Client(newTypedAgentSession("as-1916-002", "rr-1916-002", true))

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1916-002",
					Status:    "started",
					Events:    nil,
					Closer:    func() {},
				}, nil
			},
		}

		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1916-002", "ctx-1916-002", nil)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		_, err := tools.HandleInvestigationMCPWithRegistry(
			ctx, &tools.InvestigateConfig{
				MCPClient: mockMCP,
				Client:    tc,
				Namespace: "kubernaut-system",
			}, tools.InvestigateMCPArgs{RRID: "rr-1916-002"},
			true, "alice",
		)
		Expect(err).NotTo(HaveOccurred())

		texts := statusTextsFrom(queue)
		var ackText string
		for _, t := range texts {
			if strings.HasPrefix(t, "Interactive session") {
				ackText = t
				break
			}
		}
		Expect(ackText).NotTo(BeEmpty(), "expected a session-acknowledged status event, got: %v", texts)
		Expect(ackText).To(Equal("Interactive session created, starting investigation..."),
			"SI-11: session-acknowledged status must not reference the internal AA acronym")
		Expect(ackText).NotTo(ContainSubstring("AA"))
	})

	It("UT-AF-1916-003 [SI-11]: session-tracking-failed warning omits internal CRD name IS CRD", func() {
		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-1916-003",
					Status:    "started",
					Events:    nil,
					Closer:    func() {},
				}, nil
			},
		}

		onStarted := func(_ context.Context, _ string, _ string, _ string) error {
			return fmt.Errorf("boom: hook failed")
		}

		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1916-003", "ctx-1916-003", nil)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// No K8s client needed: with Client=nil, the await-session /
		// IS-phase-active block (guarded by "cfg.Client != nil") is
		// skipped, isolating this test to the onStarted-hook-error path
		// only.
		_, err := tools.HandleInvestigationMCPWithRegistry(
			ctx, &tools.InvestigateConfig{
				MCPClient: mockMCP,
				Namespace: "kubernaut-system",
				OnStarted: onStarted,
			}, tools.InvestigateMCPArgs{RRID: "rr-1916-003"},
			true, "alice",
		)
		Expect(err).NotTo(HaveOccurred())

		texts := statusTextsFrom(queue)
		var warnText string
		for _, t := range texts {
			if strings.HasPrefix(t, "Warning:") {
				warnText = t
				break
			}
		}
		Expect(warnText).NotTo(BeEmpty(), "expected a warning status event, got: %v", texts)
		Expect(warnText).To(Equal("Warning: session tracking setup failed (boom: hook failed), investigation continues"),
			"SI-11: session-tracking-failed warning must not reference the internal IS CRD name, but must retain the underlying error")
		Expect(warnText).NotTo(ContainSubstring("IS CRD"))
	})
})
