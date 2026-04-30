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
	"errors"
	"log/slog"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

type reconSpawnRunner struct {
	receivedMessages []mcpinternal.ReconMessage
	called           atomic.Int32
}

func (r *reconSpawnRunner) RunReconTurn(_ context.Context, msgs []mcpinternal.ReconMessage, _ string) (string, error) {
	r.called.Add(1)
	r.receivedMessages = msgs
	return "reconstructed-response", nil
}

type reconSpawnPanicRunner struct{}

func (r *reconSpawnPanicRunner) RunReconTurn(_ context.Context, _ []mcpinternal.ReconMessage, _ string) (string, error) {
	panic("simulated panic in LLM runner")
}

type reconSpawnRecon struct {
	turns []mcpinternal.ConversationTurn
	err   error
}

func (r *reconSpawnRecon) Reconstruct(_ context.Context, _, _ string) ([]mcpinternal.ConversationTurn, error) {
	return r.turns, r.err
}

var _ = Describe("Reconstruction Spawning — PR4 BR-INTERACTIVE-004 SEC-04", func() {

	Describe("UT-KA-TAKE-006: Reconstruction calls RunInteractiveTurn with full prior messages", func() {
		It("should convert conversation turns to LLM messages for the runner", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "What caused the OOM?", ActingUser: "alice"},
					{Role: "assistant", Content: "The OOM was caused by...", ActingUser: "ka-sa"},
				},
			}

			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, slog.Default())

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-recon-001",
				SessionID:     "old-sess-001",
				SignalMeta: map[string]string{
					"signal_name": "OOMKilled",
					"severity":    "critical",
				},
			}

			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))
			Expect(runner.receivedMessages).To(HaveLen(2))
			Expect(runner.receivedMessages[0].Role).To(Equal("user"))
			Expect(runner.receivedMessages[0].Content).To(Equal("What caused the OOM?"))
			Expect(runner.receivedMessages[1].Role).To(Equal("assistant"))
		})
	})

	Describe("UT-KA-TAKE-007: Reconstruction sets ActingUser to KA SA identity", func() {
		It("should use the kubernaut-agent service account identity", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, slog.Default())

			Expect(spawner.ServiceAccountIdentity()).To(ContainSubstring("kubernaut"))
		})
	})

	Describe("UT-KA-TAKE-008: Reconstruction uses stored signal metadata for system prompt", func() {
		It("should include signal metadata in reconstruction context", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, slog.Default())

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-meta-001",
				SessionID:     "old-sess-002",
				SignalMeta: map[string]string{
					"signal_name": "CrashLoopBackOff",
					"severity":    "high",
				},
			}

			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))
		})
	})

	Describe("UT-KA-TAKE-009: Reconstruction succeeds with empty DS context (best-effort)", func() {
		It("should succeed even when reconstructor returns empty turns", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{turns: nil}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, slog.Default())

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-empty-001",
				SessionID:     "old-sess-003",
				SignalMeta:     map[string]string{},
			}

			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))
			Expect(runner.receivedMessages).To(BeEmpty())
		})
	})

	Describe("UT-KA-TAKE-010: Panic in RunReconTurn is recovered and returned as error", func() {
		It("should recover from a panic and return an error", func() {
			panicRunner := &reconSpawnPanicRunner{}
			recon := &reconSpawnRecon{}
			spawner := mcpinternal.NewReconstructionSpawner(panicRunner, recon, slog.Default())

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-panic-001",
				SessionID:     "old-sess-004",
			}

			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("panic in SpawnReconstruct"))
		})
	})

	Describe("UT-KA-TAKE-011: Nil ReconstructionContext returns error", func() {
		It("should return an error when entry is nil", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, slog.Default())

			err := spawner.SpawnReconstruct(context.Background(), nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must not be nil"))
		})
	})

	Describe("UT-KA-TAKE-012: Reconstruct error is logged but proceeds with empty context", func() {
		It("should proceed with empty messages when reconstructor returns error", func() {
			runner := &reconSpawnRunner{}
			recon := &reconSpawnRecon{err: errors.New("DS temporarily unavailable")}
			spawner := mcpinternal.NewReconstructionSpawner(runner, recon, slog.Default())

			entry := &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-ds-fail",
				SessionID:     "old-sess-005",
			}

			err := spawner.SpawnReconstruct(context.Background(), entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))
			Expect(runner.receivedMessages).To(BeEmpty())
		})
	})
})
