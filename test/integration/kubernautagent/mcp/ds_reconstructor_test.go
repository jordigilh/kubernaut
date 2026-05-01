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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("DSContextReconstructor IT — BR-INTERACTIVE-009", Label("integration", "reconstructor"), func() {

	var logger *slog.Logger

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	Describe("IT-KA-RECON-001: reconstruct returns empty when no events exist", func() {
		It("should return zero conversation turns for a fresh session", func() {
			mockDS := newMockDSServer()
			defer mockDS.Close()

			dsClient, err := ogenclient.NewClient(mockDS.Server.URL)
			Expect(err).NotTo(HaveOccurred())

			recon := mcpinternal.NewDSContextReconstructor(dsClient, 5*time.Second, logger)
			turns, err := recon.Reconstruct(context.Background(), "rr-new", "sess-new")
			Expect(err).NotTo(HaveOccurred())
			Expect(turns).To(BeEmpty())
		})
	})

	Describe("IT-KA-RECON-002: reconstruct is best-effort on DS failure", func() {
		It("should return empty turns (not error) when DS returns 500", func() {
			failingDS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer failingDS.Close()

			dsClient, err := ogenclient.NewClient(failingDS.URL)
			Expect(err).NotTo(HaveOccurred())

			recon := mcpinternal.NewDSContextReconstructor(dsClient, 5*time.Second, logger)
			turns, err := recon.Reconstruct(context.Background(), "rr-fail", "sess-fail")
			Expect(err).NotTo(HaveOccurred(), "best-effort: should not propagate DS errors")
			Expect(turns).To(BeEmpty())
		})
	})

	Describe("IT-KA-RECON-003: reconstruct respects context cancellation", func() {
		It("should return empty turns when context is already cancelled", func() {
			mockDS := newMockDSServer()
			defer mockDS.Close()

			dsClient, err := ogenclient.NewClient(mockDS.Server.URL)
			Expect(err).NotTo(HaveOccurred())

			recon := mcpinternal.NewDSContextReconstructor(dsClient, 5*time.Second, logger)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			turns, err := recon.Reconstruct(ctx, "rr-cancelled", "sess-cancelled")
			Expect(err).NotTo(HaveOccurred(), "best-effort: should return empty on cancellation")
			Expect(turns).To(BeEmpty())
		})
	})

	Describe("IT-KA-RECON-004: reconstruct respects timeout", func() {
		It("should return empty turns when DS is slow and timeout fires", func() {
			slowDS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))
			defer slowDS.Close()

			dsClient, err := ogenclient.NewClient(slowDS.URL)
			Expect(err).NotTo(HaveOccurred())

			recon := mcpinternal.NewDSContextReconstructor(dsClient, 100*time.Millisecond, logger)
			turns, err := recon.Reconstruct(context.Background(), "rr-slow", "sess-slow")
			Expect(err).NotTo(HaveOccurred(), "best-effort: should return empty on timeout")
			Expect(turns).To(BeEmpty())
		})
	})
})
