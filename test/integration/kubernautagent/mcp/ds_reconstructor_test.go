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

var _ = Describe("DSContextReconstructor IT — BR-INTERACTIVE-009", Label("integration", "reconstructor"), func() {

	var logger logr.Logger

	BeforeEach(func() {
		logger = logr.Discard()
	})

	Describe("IT-KA-RECON-001: reconstruct returns empty when no events exist", func() {
		It("should return zero conversation turns for a fresh session", func() {
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")

			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
			turns, err := recon.Reconstruct(context.Background(), "rr-new", "sess-new")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(BeEmpty())
		})
	})

	Describe("IT-KA-RECON-003: reconstruct respects context cancellation", func() {
		It("should return empty turns when context is already cancelled", func() {
			Expect(sharedDSClient).NotTo(BeNil(), "shared DS client must be initialized by suite")

			recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			turns, err := recon.Reconstruct(ctx, "rr-cancelled", "sess-cancelled")
			Expect(err).NotTo(HaveOccurred(), "best-effort: should return empty on cancellation")
			Expect(turns).To(BeEmpty())
		})
	})
})
