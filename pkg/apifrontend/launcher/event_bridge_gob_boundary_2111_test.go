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

package launcher_test

import (
	"bytes"
	"context"
	"encoding/gob"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

// #2110/#2111 root cause: a2a-go@v0.3.15's task manager gob-encodes every
// TaskArtifactUpdateEvent's artifact for its internal deep-copy fan-out
// (internal/utils.DeepCopy). encoding/gob requires any concrete type stored
// behind an interface{} to be gob.Register'd first -- a2a-go's own
// internal/taskstore/store.go registers map[string]any and []any at init(),
// but nothing in THIS repo ever validates that data handed to
// EventBridge.EmitArtifact only ever contains those (or other gob-safe
// primitive) types.
//
// The #2112/#2113 fix closed the ONE call site that broke
// (canonicalGroundedRCA) by changing its return type. That fix was correct
// but not sufficient: EmitArtifact is the single choke point all THREE
// current artifact producers (present_decision's emitDecisionEvent,
// crd_tools.go's progress snapshot, ka_investigate_mcp.go's RCA artifact)
// share, and nothing there stops a *future* caller from reintroducing the
// exact same crash class by assigning a struct pointer into any nested
// field of the data map. These tests exercise EmitArtifact itself --
// the shared boundary -- rather than any one caller's data shape, and
// reproduce a2a-go's actual gob round-trip (not just the call to
// EmitArtifact) so a regression here fails for the same reason production
// did: a live gob decode error, not just a type assertion.
var _ = Describe("EmitArtifact gob-safety boundary (#2110/#2111 hardening)", func() {
	// registerA2AGobTypes mirrors a2a-go@v0.3.15's internal/taskstore/
	// store.go init(). gob.Register is idempotent, so calling this
	// alongside that real init() (triggered transitively elsewhere in the
	// test binary) is safe.
	registerA2AGobTypes := func() {
		gob.Register(map[string]any{})
		gob.Register([]any{})
	}

	// simulateA2AGoDeepCopy reproduces a2a-go's internal/utils.DeepCopy: a
	// gob encode/decode round-trip of the artifact DataPart's Data field,
	// which is the literal operation that crashed in production.
	simulateA2AGoDeepCopy := func(data map[string]any) error {
		registerA2AGobTypes()
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(data); err != nil {
			return err
		}
		var out map[string]any
		return gob.NewDecoder(&buf).Decode(&out)
	}

	// unregisteredStruct is deliberately never gob.Register'd anywhere,
	// standing in for any future struct type a contributor might
	// accidentally assign into an artifact's data map (the exact mistake
	// #2110/#2111 made with *tools.RCAData).
	type unregisteredStruct struct {
		Foo string
		Bar int
	}

	It("UT-AF-2111-001 (regression): a struct pointer nested anywhere in EmitArtifact's data must not reach a2a-go's gob pipeline unsanitized", func() {
		queue := &fakeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2111-001", "ctx-2111-001", nil)

		data := map[string]any{
			"type":    "test_artifact",
			"summary": "some artifact",
			"payload": &unregisteredStruct{Foo: "leaked", Bar: 7},
		}
		err := launcher.EmitArtifactForTest(ctx, data, "fallback text", nil)
		Expect(err).NotTo(HaveOccurred(), "EmitArtifact itself must not error on unsafe input -- it must sanitize, not propagate the crash")

		Expect(queue.events).To(HaveLen(1))
		Expect(launcher.LastArtifactDataForTest(queue.events[0])).To(
			WithTransform(simulateA2AGoDeepCopy, Succeed()),
			"the emitted artifact's DataPart.Data must survive a2a-go's real gob deep-copy round-trip -- "+
				"a raw struct pointer here is exactly what crashed every grounded present_decision call in v1.5.6-rc4",
		)
	})

	It("UT-AF-2111-002: legitimate JSON-shaped values (strings, floats, bools, nested maps/slices, nil) are preserved through EmitArtifact", func() {
		queue := &fakeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2111-002", "ctx-2111-002", nil)

		data := map[string]any{
			"type":         "investigation_summary",
			"summary":      "OOMKill detected",
			"confidence":   0.92,
			"critical":     true,
			"missing":      nil,
			"causal_chain": []any{"MemoryPressure", "Evicted"},
			"rca": map[string]any{
				"severity": "critical",
				"target":   "pod/checkout-service",
			},
		}
		err := launcher.EmitArtifactForTest(ctx, data, "fallback", nil)
		Expect(err).NotTo(HaveOccurred())

		got := launcher.LastArtifactDataForTest(queue.events[0])
		Expect(got["type"]).To(Equal("investigation_summary"))
		Expect(got["summary"]).To(Equal("OOMKill detected"))
		Expect(got["confidence"]).To(Equal(0.92))
		Expect(got["critical"]).To(Equal(true))
		Expect(got["missing"]).To(BeNil())
		Expect(got["causal_chain"]).To(Equal([]any{"MemoryPressure", "Evicted"}))
		rca, ok := got["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal("critical"))
		Expect(rca["target"]).To(Equal("pod/checkout-service"))

		Expect(simulateA2AGoDeepCopy(got)).To(Succeed())
	})

	It("UT-AF-2111-003 (defense-in-depth, independent of phase_guard.go): even if a future regression reintroduces the exact #2110 *tools.RCAData-shaped struct pointer into present_decision's rca argument, EmitArtifact's own boundary must still make it gob-safe", func() {
		queue := &fakeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2111-003", "ctx-2111-003", nil)

		type rcaShapedStruct struct {
			Severity       string
			Confidence     float64
			ToolCallsCount int
			LLMTurns       int
		}
		data := map[string]any{
			"type":       "decision",
			"session_id": "sess-2110",
			"rca":        &rcaShapedStruct{Severity: "critical", Confidence: 0.9, ToolCallsCount: 7, LLMTurns: 3},
		}
		err := launcher.EmitArtifactForTest(ctx, data, "fallback", nil)
		Expect(err).NotTo(HaveOccurred())

		got := launcher.LastArtifactDataForTest(queue.events[0])
		Expect(simulateA2AGoDeepCopy(got)).To(Succeed(),
			"this must pass even without phase_guard.go's canonicalGroundedRCA fix -- the boundary guard is the "+
				"backstop that closes the bug class regardless of which upstream caller regresses")
	})

	It("UT-AF-2111-004: data that cannot be made JSON-safe degrades gracefully to a text-only artifact instead of failing the whole task", func() {
		queue := &fakeQueue{}
		m := &spyBridgeMetrics{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2111-004", "ctx-2111-004", m)

		data := map[string]any{
			"type":    "test_artifact",
			"channel": make(chan int), // unmarshalable by encoding/json
		}
		err := launcher.EmitArtifactForTest(ctx, data, "fallback text", nil)
		Expect(err).NotTo(HaveOccurred(), "unsanitizable data must not fail EmitArtifact -- the task must survive")

		Expect(queue.events).To(HaveLen(1))
		evt := queue.events[0]
		Expect(launcher.ArtifactHasDataPartForTest(evt)).To(BeFalse(),
			"the unsafe DataPart must be dropped entirely rather than emitted half-sanitized")
		Expect(launcher.ArtifactTextFallbackForTest(evt)).To(Equal("fallback text"),
			"the human-readable fallback must still reach the client even when structured data is dropped")
		Expect(m.failuresInc).To(Equal(1),
			"dropping unsafe artifact data must be observable via the existing write-failure counter (SI-4)")
	})
})
