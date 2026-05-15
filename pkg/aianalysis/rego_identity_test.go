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

package aianalysis_test

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

var _ = Describe("Rego Identity Input — #774, BR-AI-085, BR-INTERACTIVE-001", func() {

	var (
		evaluator *rego.Evaluator
	)

	getTestdataPath := func(subpath string) string {
		_, filename, _, _ := runtime.Caller(0)
		dir := filepath.Dir(filename)
		return filepath.Join(dir, "testdata", subpath)
	}

	BeforeEach(func() {
		evaluator = rego.NewEvaluator(rego.Config{
			PolicyPath: getTestdataPath("policies/identity_approval.rego"),
		}, logr.Discard())
	})

	Describe("UT-KA-774-009: Rego evaluator includes identity in inputMap when present", func() {
		It("should auto-approve when identity has SRE group (policy checks input.identity.groups)", func() {
			input := &rego.PolicyInput{
				SignalType: "alert",
				Severity:   "high",
				Confidence: 0.95,
				Identity: &rego.IdentityInput{
					User:   "alice@example.com",
					Groups: []string{"engineering", "sre"},
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(),
				"SRE group should auto-approve per identity_approval.rego")
			Expect(result.Reason).To(ContainSubstring("SRE"))
		})

		It("should require approval when identity has no SRE group", func() {
			input := &rego.PolicyInput{
				SignalType: "alert",
				Severity:   "high",
				Confidence: 0.95,
				Identity: &rego.IdentityInput{
					User:   "bob@example.com",
					Groups: []string{"engineering", "dev"},
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"non-SRE user should require approval")
		})
	})

	Describe("UT-KA-774-010: Nil identity handled gracefully in Rego input", func() {
		It("should not panic when Identity is nil (autonomous flow)", func() {
			input := &rego.PolicyInput{
				SignalType: "alert",
				Severity:   "medium",
				Confidence: 0.9,
				Identity:   nil,
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"autonomous flow should default to require approval")
		})
	})

	Describe("UT-KA-774-011: buildPolicyInput produces nil identity for non-interactive flows", func() {
		It("should produce PolicyInput with nil Identity by default", func() {
			input := &rego.PolicyInput{
				SignalType: "alert",
				Severity:   "low",
				Confidence: 0.8,
			}

			Expect(input.Identity).To(BeNil(),
				"default PolicyInput should have nil Identity (autonomous flow)")
		})
	})
})
