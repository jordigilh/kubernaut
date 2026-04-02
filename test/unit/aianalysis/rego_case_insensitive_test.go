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

package aianalysis

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// Issue #604: Approval Rego uses case-sensitive environment matching
// Authority: BR-AI-013 (approval scenarios), BR-AI-011 (policy evaluation)
// Related: #595 (DS case-insensitive matching — same class, different layer)
var _ = Describe("Rego Case-Insensitive Matching (#604)", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	getTestdataPath := func(subpath string) string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "testdata", subpath)
	}

	getHelmDefaultPath := func() string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "..", "..", "..", "charts", "kubernaut", "files", "defaults", "approval.rego")
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	// UT-AIA-604-001 through 004: Helm default approval Rego with PascalCase environments
	Describe("Helm default approval Rego", func() {
		var evaluator *rego.Evaluator

		BeforeEach(func() {
			evaluator = rego.NewEvaluator(rego.Config{
				PolicyPath: getHelmDefaultPath(),
			}, logr.Discard())

			err := evaluator.StartHotReload(ctx)
			Expect(err).NotTo(HaveOccurred(), "Helm default Rego must exist and compile")
		})

		DescribeTable("UT-AIA-604: case-insensitive environment matching",
			func(env string, confidence float64, remediationTarget *rego.RemediationTargetInput, expectedApproval bool, description string) {
				input := &rego.PolicyInput{
					Environment:       env,
					Confidence:        confidence,
					RemediationTarget: remediationTarget,
					FailedDetections:  []string{},
					Warnings:          []string{},
				}

				result, err := evaluator.Evaluate(ctx, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Degraded).To(BeFalse(), "Should not be degraded")
				Expect(result.ApprovalRequired).To(Equal(expectedApproval), description)
			},
			Entry("UT-AIA-604-001: PascalCase 'Production' + low confidence → approval required",
				"Production", 0.6,
				&rego.RemediationTargetInput{Kind: "Deployment", Name: "api-gw", Namespace: "prod"},
				true,
				"PascalCase 'Production' from SP LabelDetector must trigger production approval gate"),
			Entry("UT-AIA-604-002: UPPER 'PRODUCTION' + low confidence → approval required",
				"PRODUCTION", 0.6,
				&rego.RemediationTargetInput{Kind: "Deployment", Name: "api-gw", Namespace: "prod"},
				true,
				"Uppercase 'PRODUCTION' must also trigger production approval gate"),
			Entry("UT-AIA-604-003: Mixed 'pRoDuCtIoN' + low confidence → approval required",
				"pRoDuCtIoN", 0.6,
				&rego.RemediationTargetInput{Kind: "Deployment", Name: "api-gw", Namespace: "prod"},
				true,
				"Any case variation of 'production' must trigger production approval gate"),
			Entry("UT-AIA-604-004: PascalCase 'Staging' + high confidence → auto-approve",
				"Staging", 0.9,
				&rego.RemediationTargetInput{Kind: "Deployment", Name: "api-gw", Namespace: "staging"},
				false,
				"PascalCase 'Staging' is non-production — should auto-approve"),
		)
	})

	// UT-AIA-604-005: Unit test fixture severity case-insensitivity
	Describe("Unit test fixture severity matching", func() {
		var evaluator *rego.Evaluator

		BeforeEach(func() {
			evaluator = rego.NewEvaluator(rego.Config{
				PolicyPath: getTestdataPath("policies/approval.rego"),
			}, logr.Discard())

			err := evaluator.StartHotReload(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AIA-604-005: PascalCase 'Critical' severity is recognized", func() {
			input := &rego.PolicyInput{
				Severity:    "Critical",
				Environment: "production",
				RemediationTarget: &rego.RemediationTargetInput{
					Kind: "Deployment", Name: "api", Namespace: "production",
				},
				Confidence:       0.9,
				FailedDetections: []string{},
				Warnings:         []string{},
			}

			result, err := evaluator.Evaluate(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Degraded).To(BeFalse())
			// High confidence production + clean state → auto-approve
			// (severity doesn't gate approval currently, but `is_high_severity`
			// must compile and match for future use)
			Expect(result.ApprovalRequired).To(BeFalse(),
				"PascalCase 'Critical' should not cause evaluation errors")
		})
	})
})
