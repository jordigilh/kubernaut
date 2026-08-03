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

package session_test

import (
	"context"
	"iter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// fakeState is a minimal, self-contained adksession.State implementation
// (deliberately independent of the real adksession.InMemoryService) used to
// control DD-AF-011 (#1899) checkpoint/mode flags without the side effect of
// polluting the Events() count -- setting state via a real session's
// AppendEvent would itself append an event, defeating "does not trigger with
// empty events"-style tests.
type fakeState struct {
	data map[string]any
}

func newFakeState(flags map[string]any) *fakeState {
	data := make(map[string]any, len(flags))
	for k, v := range flags {
		data[k] = v
	}
	return &fakeState{data: data}
}

func (f *fakeState) Get(key string) (any, error) {
	v, ok := f.data[key]
	if !ok {
		return nil, adksession.ErrStateKeyNotExist
	}
	return v, nil
}

func (f *fakeState) Set(key string, value any) error {
	f.data[key] = value
	return nil
}

func (f *fakeState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range f.data {
			if !yield(k, v) {
				return
			}
		}
	}
}

var _ = Describe("Re-invocation Fallback", func() {
	ctx := context.Background()
	inmem := adksession.InMemoryService()

	getEvents := func(events ...*adksession.Event) adksession.Events {
		resp, err := inmem.Create(ctx, &adksession.CreateRequest{
			AppName: "test",
			UserID:  "test",
		})
		Expect(err).NotTo(HaveOccurred())
		for _, evt := range events {
			err := inmem.AppendEvent(ctx, resp.Session, evt)
			Expect(err).NotTo(HaveOccurred())
		}
		getResp, err := inmem.Get(ctx, &adksession.GetRequest{
			AppName:   "test",
			UserID:    "test",
			SessionID: resp.Session.ID(),
		})
		Expect(err).NotTo(HaveOccurred())
		return getResp.Session.Events()
	}

	// activeDriverState is the DD-AF-011 baseline for "mid-investigation, no
	// checkpoint gate active" -- the scenario all the pre-#1899 tests were
	// written against, now made explicit since driver-active is a mandatory
	// precondition for reinvocation.
	activeDriverState := func() adksession.State {
		return newFakeState(map[string]any{session.StateKeyDriverActive: true})
	}

	textEvent := func() *adksession.Event {
		evt := adksession.NewEvent("inv-1")
		evt.Author = "agent"
		evt.Content = genai.NewContentFromText("analysis complete", genai.RoleModel)
		return evt
	}

	toolCallEvent := func() *adksession.Event {
		evt := adksession.NewEvent("inv-1")
		evt.Author = "agent"
		evt.Content = &genai.Content{
			Role: string(genai.RoleModel),
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						Name: "kubectl_list",
						Args: map[string]any{"namespace": "default"},
					},
				},
			},
		}
		return evt
	}

	It("UT-AF-230-001: detects text-only turn end during active investigation", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, activeDriverState(), 0)
		Expect(result).To(BeTrue())
	})

	It("UT-AF-230-002: does not trigger with tool calls", func() {
		events := getEvents(toolCallEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, activeDriverState(), 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-003: does not trigger when terminal", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseCompleted, events, activeDriverState(), 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-004: generates correct synthetic message", func() {
		msg := session.SyntheticMessage()
		Expect(msg).NotTo(BeNil())
		Expect(msg.Role).To(Equal(string(genai.RoleUser)))
		Expect(msg.Parts).To(HaveLen(1))
		Expect(msg.Parts[0].Text).NotTo(BeEmpty())
	})

	It("UT-AF-230-005: tracks reinvocation count", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, activeDriverState(), 1)
		Expect(result).To(BeTrue())
	})

	It("UT-AF-230-006: stops after max reinvocations", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, activeDriverState(), session.MaxReinvocations)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-007: does not trigger when Disconnected", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseDisconnected, events, activeDriverState(), 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-008: does not trigger with empty events", func() {
		events := getEvents()
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, activeDriverState(), 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-1435-010: does not trigger when context is cancelled (#1435)", func() {
		events := getEvents(textEvent())
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		result := session.NeedsReinvocationCtx(cancelledCtx, v1alpha1.SessionPhaseActive, events, activeDriverState(), 0)
		Expect(result).To(BeFalse(),
			"#1435: re-invocation must not fire when context is already cancelled")
	})

	It("UT-AF-1435-011: triggers normally when context is active", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocationCtx(ctx, v1alpha1.SessionPhaseActive, events, activeDriverState(), 0)
		Expect(result).To(BeTrue(),
			"re-invocation should still fire when context is healthy")
	})

	// --- DD-AF-011 (#1899): mode/flag-aware reinvocation gate ---

	It("UT-AF-1899-001: does NOT nudge when no driver session is active (literal #1899 repro)", func() {
		// No af_interactive_driver_active flag at all -- e.g. the user asked
		// a plain question and the model answered in Observation Mode. There
		// was never an investigation to "continue".
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, nil, 0)
		Expect(result).To(BeFalse(),
			"#1899: reinvocation must never nudge investigation into existence when none was ever requested")
	})

	It("UT-AF-1899-002: does NOT nudge while af_phase2_blocked is set, even with an active driver", func() {
		events := getEvents(textEvent())
		state := newFakeState(map[string]any{
			session.StateKeyDriverActive:  true,
			session.StateKeyPhase2Blocked: true,
		})
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, state, 0)
		Expect(result).To(BeFalse(),
			"reinvocation must not push the model past a phase-2 checkpoint the harness deliberately gated")
	})

	It("UT-AF-1899-003: does NOT nudge while af_phase3_blocked is set, even with an active driver", func() {
		events := getEvents(textEvent())
		state := newFakeState(map[string]any{
			session.StateKeyDriverActive:  true,
			session.StateKeyPhase3Blocked: true,
		})
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, state, 0)
		Expect(result).To(BeFalse(),
			"reinvocation must not push the model past a phase-3 checkpoint the harness deliberately gated")
	})

	It("UT-AF-1899-004: DOES nudge with an active driver and no checkpoint blocked (regression: legitimate continuation still works)", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, activeDriverState(), 0)
		Expect(result).To(BeTrue(),
			"a genuinely stalled mid-investigation turn (driver active, no checkpoint gate) must still be nudged")
	})
})
