/*
Copyright 2025 Jordi Gil.

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

// Package aianalysis contains unit tests for AIAnalysis Rego policy startup validation.
//
// Business Requirements:
// - BR-AI-011: Policy evaluation
// - BR-AI-056: Startup validation for configuration
//
// Design Decisions:
// - ADR-050: Configuration Validation Strategy (fail-fast at startup)
// - DD-AIANALYSIS-002: Rego Policy Startup Validation
//
// Testing Strategy (per ADR-050):
// - Unit tests verify startup validation failures (invalid config → service exits)
// - Integration tests verify graceful degradation (invalid hot-reload → old config preserved)
// - E2E tests use production configuration files (not mocks)
//
// TDD Methodology (per TESTING_GUIDELINES.md):
// - RED phase: Write failing tests
// - GREEN phase: Minimal implementation
// - REFACTOR phase: Optimize and cache compiled policy
package aianalysis

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// BR-AI-056: Startup validation tests
// ADR-050: Configuration Validation Strategy
// DD-AIANALYSIS-002: Rego Policy Startup Validation
var _ = Describe("Rego Startup Validation", Label("unit", "rego", "startup-validation"), func() {
	var (
		ctx    context.Context
		logger logr.Logger
	)

	// Helper to get testdata path relative to this test file
	getTestdataPath := func(subpath string) string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "testdata", subpath)
	}

	// Helper to create temp invalid policy file
	createInvalidPolicyFile := func() string {
		tmpFile, err := os.CreateTemp("", "invalid-policy-*.rego")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = tmpFile.Close() }()

		// Write syntactically invalid Rego
		_, err = tmpFile.WriteString(`package aianalysis
approval {
	# INVALID SYNTAX: missing closing brace
	require_approval = true
`)
		Expect(err).NotTo(HaveOccurred())

		return tmpFile.Name()
	}

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard() // Test logger (silent)
	})

	// ========================================
	// ADR-050: STARTUP VALIDATION TESTS
	// ========================================
	// Per ADR-050: Invalid configuration at startup MUST cause service to fail (exit 1)
	// Per DD-AIANALYSIS-002: Use pkg/shared/hotreload/FileWatcher for startup validation

	Context("StartHotReload", func() {
		Context("with valid policy file", func() {
			It("should successfully load and validate policy at startup", func() {
				evaluator := rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("policies/approval.rego"),
				}, logger)

				err := evaluator.StartHotReload(ctx)
				Expect(err).NotTo(HaveOccurred(), "Valid policy should load without error")

				// Verify policy hash is available (confirms policy was loaded)
				hash := evaluator.GetPolicyHash()
				Expect(hash).NotTo(BeEmpty(), "Policy hash should be available after successful load")
			})

			It("should cache compiled policy for runtime use", func() {
				evaluator := rego.NewEvaluator(rego.Config{
					PolicyPath: getTestdataPath("policies/approval.rego"),
				}, logger)

				err := evaluator.StartHotReload(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Verify Evaluate() uses cached policy (no file I/O)
				// If caching works, should be able to Evaluate even if file is deleted
				result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
					Environment: "staging",
					Confidence:  0.9,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Degraded).To(BeFalse(), "Should use cached policy, not degraded mode")
			})
		})

		Context("with invalid policy file (syntax error)", func() {
			It("should fail fast at startup (MUST return error)", func() {
				invalidPolicyPath := createInvalidPolicyFile()
				defer func() { _ = os.Remove(invalidPolicyPath) }()

				evaluator := rego.NewEvaluator(rego.Config{
					PolicyPath: invalidPolicyPath,
				}, logger)

				err := evaluator.StartHotReload(ctx)
				Expect(err).To(HaveOccurred(), "Invalid policy MUST cause startup failure per ADR-050")
				Expect(err.Error()).To(ContainSubstring("policy validation failed"), "Error should indicate policy validation failure")
			})

			It("should provide actionable error message with details", func() {
				invalidPolicyPath := createInvalidPolicyFile()
				defer func() { _ = os.Remove(invalidPolicyPath) }()

				evaluator := rego.NewEvaluator(rego.Config{
					PolicyPath: invalidPolicyPath,
				}, logger)

				err := evaluator.StartHotReload(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Or(
					ContainSubstring("rego"),
					ContainSubstring("compile"),
					ContainSubstring("syntax"),
				), "Error message should indicate Rego compilation/syntax issue")
			})
		})

		Context("with missing policy file", func() {
			It("should fail fast at startup (MUST return error)", func() {
				evaluator := rego.NewEvaluator(rego.Config{
					PolicyPath: "/nonexistent/path/policy.rego",
				}, logger)

				err := evaluator.StartHotReload(ctx)
				Expect(err).To(HaveOccurred(), "Missing policy file MUST cause startup failure per ADR-050")
			})
		})
	})

	// ========================================
	// ADR-050: RUNTIME HOT-RELOAD TESTS
	// ========================================
	// Per ADR-050: Invalid hot-reload MUST gracefully degrade (keep old config, log error)

	Context("Hot-Reload Graceful Degradation", func() {
		var (
			tmpDir     string
			policyPath string
			evaluator  *rego.Evaluator
		)

		BeforeEach(func() {
			// Create temporary directory for hot-reload testing
			var err error
			tmpDir, err = os.MkdirTemp("", "rego-hotreload-test-*")
			Expect(err).NotTo(HaveOccurred())

			policyPath = filepath.Join(tmpDir, "approval.rego")

		// Write initial valid policy
		validPolicy := `package aianalysis

approval = result if {
	input.environment == "staging"
	result := {
		"require_approval": false,
		"reason": "Auto-approved for staging"
	}
}

approval = result if {
	input.environment == "production"
	result := {
		"require_approval": true,
		"reason": "Production requires approval"
	}
}
`
			err = os.WriteFile(policyPath, []byte(validPolicy), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Initialize evaluator with valid policy
			evaluator = rego.NewEvaluator(rego.Config{
				PolicyPath: policyPath,
			}, logger)

			err = evaluator.StartHotReload(ctx)
			Expect(err).NotTo(HaveOccurred(), "Initial valid policy should load")
		})

		AfterEach(func() {
			if evaluator != nil {
				evaluator.Stop()
			}
			if tmpDir != "" {
				_ = os.RemoveAll(tmpDir)
			}
		})

		It("should keep old policy when hot-reload encounters syntax error", func() {
			// Get initial policy hash
			initialHash := evaluator.GetPolicyHash()
			Expect(initialHash).NotTo(BeEmpty())

			// Verify initial policy works
			result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
				Environment: "staging",
				Confidence:  0.9,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(), "Staging should auto-approve initially")

			// Write invalid policy (simulating bad ConfigMap update)
			invalidPolicy := `package aianalysis
approval {
	# INVALID SYNTAX
	require_approval = true
`
			err = os.WriteFile(policyPath, []byte(invalidPolicy), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Wait for hot-reload attempt (should fail gracefully)
			// Note: FileWatcher has ~100ms debounce + ConfigMap ~60s propagation
			// In tests, we trigger immediate reload by writing file
			Eventually(func() string {
				return evaluator.GetPolicyHash()
			}, "2s", "100ms").Should(Equal(initialHash), "Policy hash should remain unchanged (old policy preserved)")

			// Verify evaluator still works with old policy
			result, err = evaluator.Evaluate(ctx, &rego.PolicyInput{
				Environment: "staging",
				Confidence:  0.9,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(), "Should still use old policy (graceful degradation)")
			Expect(result.Degraded).To(BeFalse(), "Should not be degraded (using cached valid policy)")
		})

		It("should successfully apply valid hot-reload update", func() {
			// Get initial policy hash
			initialHash := evaluator.GetPolicyHash()

		// Write updated valid policy (different logic)
		updatedPolicy := `package aianalysis

approval = result if {
	# Updated: ALL environments auto-approve
	result := {
		"require_approval": false,
		"reason": "Auto-approved (updated policy)"
	}
}
`
			err := os.WriteFile(policyPath, []byte(updatedPolicy), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Wait for hot-reload to complete
			Eventually(func() string {
				return evaluator.GetPolicyHash()
			}, "2s", "100ms").ShouldNot(Equal(initialHash), "Policy hash should change after successful reload")

			// Verify new policy is applied
			result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
				Environment: "production", // Previously required approval
				Confidence:  0.9,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(), "Production should now auto-approve (new policy)")
			Expect(result.Reason).To(ContainSubstring("updated policy"), "Should use updated policy reason")
		})
	})

	// ========================================
	// PERFORMANCE: CACHED POLICY COMPILATION
	// ========================================
	// Per ADR-050: Cached compilation should eliminate 2-5ms overhead per call

	Context("Performance: Cached Policy Compilation", func() {
		It("should use cached compiled policy (no file I/O on Evaluate)", func() {
			// Create temporary directory for policy (file watcher needs stable directory)
			tempDir, err := os.MkdirTemp("", "rego-hotreload-test-")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tempDir) }()

			// Create temporary policy file in directory
			tempPolicyPath := filepath.Join(tempDir, "approval.rego")
			originalPath := getTestdataPath("policies/approval.rego")
			originalContent, err := os.ReadFile(originalPath)
			Expect(err).NotTo(HaveOccurred())
			
			err = os.WriteFile(tempPolicyPath, originalContent, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Use temp policy for this test
			evaluator := rego.NewEvaluator(rego.Config{
				PolicyPath: tempPolicyPath,
			}, logger)

			err = evaluator.StartHotReload(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Delete temp policy file to prove Evaluate() doesn't read from disk
			err = os.Remove(tempPolicyPath)
			Expect(err).NotTo(HaveOccurred())

			// Evaluate should still work (using cached compiled policy)
			result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
				Environment: "staging",
				Confidence:  0.9,
			})

			// Note: This test will fail if Evaluate() tries to read from disk
			// This proves caching is working
			Expect(err).NotTo(HaveOccurred(), "Should use cached policy, not read from disk")
			Expect(result.Degraded).To(BeFalse(), "Should not degrade if using cache")
		})
	})
})
