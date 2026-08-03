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
	"errors"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ErrorBoundary", func() {
	Describe("known MCPError codes pass through unchanged", func() {
		It("returns the same *MCPError when the handler error already is one", func() {
			err := tools.ErrorBoundary(logr.Discard(), "kubernaut_investigate", tools.ErrCodeRateLimited)
			Expect(err).To(Equal(tools.ErrCodeRateLimited))
		})
	})

	Describe("unknown errors are redacted to internal_error (H3/SEC-5)", func() {
		It("replaces a raw, non-MCPError with the generic internal error", func() {
			err := tools.ErrorBoundary(logr.Discard(), "kubernaut_investigate", fmt.Errorf("some internal detail: db timeout"))
			Expect(err).To(Equal(tools.ErrCodeInternalError))
		})
	})

	Describe("IT-KA-1889-001: tool budget exhaustion survives the full production wrap chain (#1889 gap 1)", func() {
		It("reaches ErrorBoundary as tool_budget_exhausted, not internal_error, through the exact wraps production applies", func() {
			// Step 1: adapters.ExtractContent, exactly as InvestigatorRunnerAdapter.RunInteractiveTurn calls it.
			_, extractErr := adapters.ExtractContent(&investigator.ExhaustedResult{Reason: investigator.ReasonToolBudgetExhausted})
			Expect(extractErr).To(HaveOccurred())

			// Step 2: the InvestigateTool.handleMessage wrap (investigate.go:656):
			// fmt.Errorf("interactive turn failed: %w", err).
			turnErr := fmt.Errorf("interactive turn failed: %w", extractErr)

			// Step 3: the registration.go dispatch wrap that every kubernaut_investigate
			// tool call passes through.
			boundaryErr := tools.ErrorBoundary(logr.Discard(), "kubernaut_investigate", turnErr)

			var mcpErr *tools.MCPError
			Expect(errors.As(boundaryErr, &mcpErr)).To(BeTrue(),
				"tool budget exhaustion must survive both wrap layers as a typed MCPError, not be redacted to internal_error")
			Expect(mcpErr.Code).To(Equal("tool_budget_exhausted"))
			Expect(boundaryErr).NotTo(Equal(tools.ErrCodeInternalError),
				"this is exactly the #1889 regression: redaction to a generic, undiagnosable error")
		})
	})
})
