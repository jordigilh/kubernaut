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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

var _ = Describe("Rego Identity Integration — #774, BR-AI-085", Label("integration", "rego", "identity"), func() {
	var (
		evaluator *rego.Evaluator
		evalCtx   context.Context
		cancel    context.CancelFunc
	)

	BeforeEach(func() {
		policyPath := filepath.Join("testdata", "policies", "identity_approval.rego")
		evaluator = rego.NewEvaluator(rego.Config{
			PolicyPath: policyPath,
		}, logr.Discard())

		evalCtx, cancel = context.WithCancel(context.Background())
		err := evaluator.StartHotReload(evalCtx)
		Expect(err).NotTo(HaveOccurred(), "Identity policy should load successfully")
	})

	AfterEach(func() {
		if evaluator != nil {
			evaluator.Stop()
		}
		if cancel != nil {
			cancel()
		}
	})

	Describe("IT-KA-774-003: End-to-end identity chain — buildPolicyInput -> Rego evaluator sees identity", func() {
		It("should auto-approve when PolicyInput has SRE group identity", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				TargetResource: rego.TargetResourceInput{
					Kind:      "Pod",
					Name:      "api-server-abc123",
					Namespace: "production",
				},
				Identity: &rego.IdentityInput{
					User:   "jane@example.com",
					Groups: []string{"sre-team", "sre"},
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(),
				"SRE group in identity should auto-approve via identity_approval.rego")
			Expect(result.Reason).To(ContainSubstring("SRE"))
		})

		It("should require approval when PolicyInput identity has no SRE group", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				TargetResource: rego.TargetResourceInput{
					Kind:      "Pod",
					Name:      "api-server-abc123",
					Namespace: "production",
				},
				Identity: &rego.IdentityInput{
					User:   "dev@example.com",
					Groups: []string{"engineering", "dev"},
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"non-SRE user should require approval")
		})

		It("should require approval when PolicyInput has nil identity (autonomous flow)", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "warning", Environment: "staging"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.90},
				TargetResource: rego.TargetResourceInput{
					Kind:      "Deployment",
					Name:      "worker",
					Namespace: "staging",
				},
				Identity: nil,
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"autonomous flow (nil identity) should default to require approval")
		})
	})
})
