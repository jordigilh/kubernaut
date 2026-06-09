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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("Fix #1384 Bug B — Reconstruction empty-content filter (AU-6, SI-10, CP-10)", func() {

	var (
		ctx    context.Context
		logger logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
	})

	Describe("UT-KA-1384-B01: Reconstruction produces valid LLM prompt with no empty content blocks (SI-10)", func() {
		It("should filter out turns with empty Content before passing to RunReconTurn", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "Investigate the OOMKilled pod", Timestamp: time.Now().Add(-3 * time.Minute)},
					{Role: "assistant", Content: "", Timestamp: time.Now().Add(-2 * time.Minute)},
					{Role: "assistant", Content: "The root cause is memory pressure.", Timestamp: time.Now().Add(-1 * time.Minute)},
				},
			}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logger)
			err := spawner.SpawnReconstruct(ctx, &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-1384-b01",
				SessionID:     "sess-b01",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))

			for i, msg := range runner.receivedMessages {
				Expect(msg.Content).NotTo(BeEmpty(),
					"message[%d] (role=%s) has empty Content — violates SI-10 boundary validation", i, msg.Role)
			}
			Expect(runner.receivedMessages).To(HaveLen(2),
				"expected 2 messages (1 user + 1 non-empty assistant); got %d", len(runner.receivedMessages))
		})
	})

	Describe("UT-KA-1384-B02: Reconstruction preserves all meaningful conversation context (CP-10)", func() {
		It("should retain non-empty turns in original chronological order", func() {
			now := time.Now()
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "First question", Timestamp: now.Add(-4 * time.Minute)},
					{Role: "assistant", Content: "", Timestamp: now.Add(-3 * time.Minute)},
					{Role: "user", Content: "Second question", Timestamp: now.Add(-2 * time.Minute)},
					{Role: "assistant", Content: "Answer to second", Timestamp: now.Add(-1 * time.Minute)},
				},
			}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logger)
			err := spawner.SpawnReconstruct(ctx, &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-1384-b02",
				SessionID:     "sess-b02",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.receivedMessages).To(HaveLen(3))
			Expect(runner.receivedMessages[0].Content).To(Equal("First question"))
			Expect(runner.receivedMessages[1].Content).To(Equal("Second question"))
			Expect(runner.receivedMessages[2].Content).To(Equal("Answer to second"))
		})
	})

	Describe("UT-KA-1384-B05: All-empty audit history produces nil reconstruction (CP-10)", func() {
		It("should return nil messages when all turns have empty Content", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{
				turns: []mcpinternal.ConversationTurn{
					{Role: "assistant", Content: "", Timestamp: time.Now().Add(-2 * time.Minute)},
					{Role: "assistant", Content: "", Timestamp: time.Now().Add(-1 * time.Minute)},
				},
			}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logger)
			err := spawner.SpawnReconstruct(ctx, &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-1384-b05",
				SessionID:     "sess-b05",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))
			Expect(runner.receivedMessages).To(BeNil(),
				"all-empty turns should produce nil messages, not empty slice")
		})
	})

	Describe("UT-KA-1384-B06: Nil input to reconstruction handled safely", func() {
		It("should return nil messages for nil turns input (no panic)", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{turns: nil}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, logger)
			err := spawner.SpawnReconstruct(ctx, &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-1384-b06",
				SessionID:     "sess-b06",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.receivedMessages).To(BeNil())
		})
	})
})
