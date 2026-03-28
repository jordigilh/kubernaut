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

package evaluator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
)

// Unified test policy containing all 4 rule domains.
// Mirrors the production policy structure operators will deploy.
const testPolicy = `package signalprocessing

import rego.v1

# ========== Environment ==========
default environment := {"environment": "unknown", "source": "default"}

environment := {"environment": "production", "source": "namespace-labels"} if {
    input.namespace.labels["env"] == "production"
}
environment := {"environment": "staging", "source": "namespace-labels"} if {
    input.namespace.labels["env"] == "staging"
}
environment := {"environment": "development", "source": "namespace-labels"} if {
    input.namespace.labels["env"] == "development"
}

# ========== Severity ==========
default severity := "unknown"

severity := "critical" if {
    input.signal.severity == "critical"
}
severity := "high" if {
    input.signal.severity == "high"
}
severity := "medium" if {
    input.signal.severity == "medium"
}
severity := "low" if {
    input.signal.severity == "low"
}

# ========== Priority ==========
default priority := {"priority": "P3", "policy_name": "default"}

priority := {"priority": "P0", "policy_name": "production-critical"} if {
    environment.environment == "production"
    severity == "critical"
}
priority := {"priority": "P1", "policy_name": "production-high"} if {
    environment.environment == "production"
    severity == "high"
}
priority := {"priority": "P2", "policy_name": "staging-any"} if {
    environment.environment == "staging"
}

# ========== Custom Labels ==========
default labels := {}

labels := {"risk-tolerance": ["low"]} if {
    input.namespace.labels["kubernaut.ai/risk-tolerance"] == "low"
}
`

var _ = Describe("Unified Evaluator", func() {
	var (
		eval *evaluator.Evaluator
		ctx  context.Context
	)

	BeforeEach(func() {
		logger := zap.New(zap.UseDevMode(true))
		eval = evaluator.New("/tmp/test-policy.rego", logger)
		ctx = context.Background()
	})

	Describe("UT-EVAL-001: LoadPolicy", func() {
		It("should load a valid unified policy", func() {
			err := eval.LoadPolicy(testPolicy)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a policy with syntax errors", func() {
			err := eval.LoadPolicy(`package signalprocessing
this is not valid rego
`)
			Expect(err).To(HaveOccurred())
		})

		It("should replace previous policy on reload", func() {
			err := eval.LoadPolicy(testPolicy)
			Expect(err).NotTo(HaveOccurred())

			newPolicy := `package signalprocessing
import rego.v1
default environment := {"environment": "always-prod", "source": "override"}
default severity := "critical"
default priority := {"priority": "P0", "policy_name": "override"}
default labels := {}
`
			err = eval.LoadPolicy(newPolicy)
			Expect(err).NotTo(HaveOccurred())

			result, err := eval.EvaluateEnvironment(ctx, evaluator.PolicyInput{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(signalprocessingv1alpha1.Environment("always-prod")))
		})
	})

	Describe("UT-EVAL-010: EvaluateEnvironment", func() {
		BeforeEach(func() {
			Expect(eval.LoadPolicy(testPolicy)).To(Succeed())
		})

		DescribeTable("BR-SP-051: should classify environment from namespace labels",
			func(labels map[string]string, expectedEnv signalprocessingv1alpha1.Environment, expectedSource string) {
				input := evaluator.PolicyInput{
					Namespace: types.NamespaceContext{
						Name:   "test-ns",
						Labels: labels,
					},
				}
				result, err := eval.EvaluateEnvironment(ctx, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Environment).To(Equal(expectedEnv))
				Expect(result.Source).To(Equal(expectedSource))
			},
			Entry("production namespace", map[string]string{"env": "production"}, signalprocessingv1alpha1.EnvironmentProduction, "namespace-labels"),
			Entry("staging namespace", map[string]string{"env": "staging"}, signalprocessingv1alpha1.EnvironmentStaging, "namespace-labels"),
			Entry("development namespace", map[string]string{"env": "development"}, signalprocessingv1alpha1.EnvironmentDevelopment, "namespace-labels"),
			Entry("unlabeled namespace (default)", map[string]string{}, signalprocessingv1alpha1.EnvironmentUnknown, "default"),
		)

		It("should fail when policy not loaded", func() {
			unloaded := evaluator.New("/tmp/nope.rego", zap.New(zap.UseDevMode(true)))
			_, err := unloaded.EvaluateEnvironment(ctx, evaluator.PolicyInput{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not loaded"))
		})
	})

	Describe("UT-EVAL-020: EvaluateSeverity", func() {
		BeforeEach(func() {
			Expect(eval.LoadPolicy(testPolicy)).To(Succeed())
		})

		DescribeTable("BR-SP-105: should determine normalized severity",
			func(inputSeverity, expectedSeverity string) {
				input := evaluator.PolicyInput{
					Signal: evaluator.SignalInput{Severity: inputSeverity},
				}
				result, err := eval.EvaluateSeverity(ctx, input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Severity).To(Equal(expectedSeverity))
				Expect(result.Source).To(Equal("rego-policy"))
			},
			Entry("critical", "critical", "critical"),
			Entry("high", "high", "high"),
			Entry("medium", "medium", "medium"),
			Entry("low", "low", "low"),
			Entry("unmapped (default)", "Warning", "unknown"),
		)

		It("should fail when policy not loaded", func() {
			unloaded := evaluator.New("/tmp/nope.rego", zap.New(zap.UseDevMode(true)))
			_, err := unloaded.EvaluateSeverity(ctx, evaluator.PolicyInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-EVAL-030: EvaluatePriority", func() {
		BeforeEach(func() {
			Expect(eval.LoadPolicy(testPolicy)).To(Succeed())
		})

		It("BR-SP-070: should assign P0 for production + critical", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name:   "prod-ns",
					Labels: map[string]string{"env": "production"},
				},
				Signal: evaluator.SignalInput{Severity: "critical"},
			}
			result, err := eval.EvaluatePriority(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal(signalprocessingv1alpha1.PriorityP0))
			Expect(result.PolicyName).To(Equal("production-critical"))
		})

		It("BR-SP-070: should assign P1 for production + high", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name:   "prod-ns",
					Labels: map[string]string{"env": "production"},
				},
				Signal: evaluator.SignalInput{Severity: "high"},
			}
			result, err := eval.EvaluatePriority(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal(signalprocessingv1alpha1.PriorityP1))
			Expect(result.PolicyName).To(Equal("production-high"))
		})

		It("BR-SP-070: should assign P2 for staging (any severity)", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name:   "staging-ns",
					Labels: map[string]string{"env": "staging"},
				},
				Signal: evaluator.SignalInput{Severity: "low"},
			}
			result, err := eval.EvaluatePriority(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal(signalprocessingv1alpha1.PriorityP2))
		})

		It("BR-SP-070: should assign P3 as default", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name:   "dev-ns",
					Labels: map[string]string{"env": "development"},
				},
				Signal: evaluator.SignalInput{Severity: "low"},
			}
			result, err := eval.EvaluatePriority(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal(signalprocessingv1alpha1.PriorityP3))
			Expect(result.PolicyName).To(Equal("default"))
		})

		It("should fail when policy not loaded", func() {
			unloaded := evaluator.New("/tmp/nope.rego", zap.New(zap.UseDevMode(true)))
			_, err := unloaded.EvaluatePriority(ctx, evaluator.PolicyInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-EVAL-040: EvaluateCustomLabels", func() {
		BeforeEach(func() {
			Expect(eval.LoadPolicy(testPolicy)).To(Succeed())
		})

		It("BR-SP-102: should extract custom labels", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name:   "test-ns",
					Labels: map[string]string{"kubernaut.ai/risk-tolerance": "low"},
				},
			}
			labels, err := eval.EvaluateCustomLabels(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(HaveKeyWithValue("risk-tolerance", []string{"low"}))
		})

		It("BR-SP-102: should return empty map when no labels match", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name: "test-ns",
				},
			}
			labels, err := eval.EvaluateCustomLabels(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(BeEmpty())
		})

		It("should return empty map when policy not loaded", func() {
			unloaded := evaluator.New("/tmp/nope.rego", zap.New(zap.UseDevMode(true)))
			labels, err := unloaded.EvaluateCustomLabels(ctx, evaluator.PolicyInput{})
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(BeEmpty())
		})
	})

	Describe("UT-EVAL-050: BuildInput", func() {
		It("should build input from KubernetesContext and SignalData", func() {
			k8sCtx := &types.KubernetesContext{
				Namespace: &types.NamespaceContext{
					Name:   "prod-ns",
					Labels: map[string]string{"env": "production"},
				},
				Workload: &types.WorkloadDetails{
					Kind: "Deployment",
					Name: "my-app",
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Type:     "PodCrashLoop",
				Source:   "alertmanager",
			}

			input := evaluator.BuildInput(k8sCtx, signal)
			Expect(input.Namespace.Name).To(Equal("prod-ns"))
			Expect(input.Namespace.Labels["env"]).To(Equal("production"))
			Expect(input.Signal.Severity).To(Equal("critical"))
			Expect(input.Signal.Type).To(Equal("PodCrashLoop"))
			Expect(input.Workload.Kind).To(Equal("Deployment"))
		})

		It("should handle nil KubernetesContext gracefully", func() {
			signal := &signalprocessingv1alpha1.SignalData{Severity: "low"}
			input := evaluator.BuildInput(nil, signal)
			Expect(input.Namespace.Name).To(BeEmpty())
			Expect(input.Signal.Severity).To(Equal("low"))
		})

		It("should handle nil signal gracefully", func() {
			k8sCtx := &types.KubernetesContext{
				Namespace: &types.NamespaceContext{Name: "ns"},
			}
			input := evaluator.BuildInput(k8sCtx, nil)
			Expect(input.Namespace.Name).To(Equal("ns"))
			Expect(input.Signal.Severity).To(BeEmpty())
		})
	})

	Describe("UT-EVAL-060: Cross-rule references (ADR-060 key feature)", func() {
		BeforeEach(func() {
			Expect(eval.LoadPolicy(testPolicy)).To(Succeed())
		})

		It("priority should internally reference environment without Go sequencing", func() {
			input := evaluator.PolicyInput{
				Namespace: types.NamespaceContext{
					Name:   "prod-ns",
					Labels: map[string]string{"env": "production"},
				},
				Signal: evaluator.SignalInput{Severity: "critical"},
			}

			priority, err := eval.EvaluatePriority(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(priority.Priority).To(Equal(signalprocessingv1alpha1.PriorityP0))
			Expect(priority.PolicyName).To(Equal("production-critical"))
		})
	})

	Describe("UT-EVAL-070: Label sanitization (BR-SP-104)", func() {
		It("should strip reserved prefixes from labels", func() {
			policy := `package signalprocessing
import rego.v1
default environment := {"environment": "unknown", "source": "default"}
default severity := "unknown"
default priority := {"priority": "P3", "policy_name": "default"}
labels := {"kubernaut.ai/internal": ["secret"], "safe-key": ["ok"]}
`
			Expect(eval.LoadPolicy(policy)).To(Succeed())

			labels, err := eval.EvaluateCustomLabels(ctx, evaluator.PolicyInput{})
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(HaveKey("kubernaut.ai/internal"))
			Expect(labels).To(HaveKeyWithValue("safe-key", []string{"ok"}))
		})
	})
})
