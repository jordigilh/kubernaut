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

package signalprocessing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

var _ = Describe("Classifier", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// ENVIRONMENT CLASSIFIER TESTS (BR-SP-070)
	// ========================================
	Describe("EnvironmentClassifier", func() {
		var envClassifier *classifier.EnvironmentClassifier

		BeforeEach(func() {
			envClassifier = classifier.NewEnvironmentClassifier(ctrl.Log.WithName("test"))
		})

		// Test 1: Should classify from namespace labels (highest priority)
		It("should classify environment from namespace labels", func() {
			result := envClassifier.Classify(ctx, map[string]string{
				"environment": "production",
			}, map[string]string{})

			Expect(result.Environment).To(Equal("production"))
			Expect(result.Source).To(Equal("namespace-label"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.9))
		})

		// Test 2: Should fall back to signal labels
		It("should classify environment from signal labels when namespace has no label", func() {
			result := envClassifier.Classify(ctx, map[string]string{}, map[string]string{
				"environment": "staging",
			})

			Expect(result.Environment).To(Equal("staging"))
			Expect(result.Source).To(Equal("signal-label"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.7))
		})

		// Test 3: Should return unknown as fallback
		It("should return unknown when no environment label exists", func() {
			result := envClassifier.Classify(ctx, map[string]string{}, map[string]string{})

			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Source).To(Equal("default"))
			Expect(result.Confidence).To(Equal(0.0))
		})

		// Test 4: Should handle various environment names (dynamic taxonomy)
		DescribeTable("should accept any environment value",
			func(envValue string) {
				result := envClassifier.Classify(ctx, map[string]string{
					"environment": envValue,
				}, map[string]string{})

				Expect(result.Environment).To(Equal(envValue))
			},
			Entry("production", "production"),
			Entry("prod", "prod"),
			Entry("staging", "staging"),
			Entry("development", "development"),
			Entry("dev", "dev"),
			Entry("qa", "qa"),
			Entry("canary", "canary"),
			Entry("qa-eu", "qa-eu"),
		)
	})

	// ========================================
	// PRIORITY CLASSIFIER TESTS (BR-SP-071)
	// ========================================
	Describe("PriorityClassifier", func() {
		var priorityClassifier *classifier.PriorityClassifier

		BeforeEach(func() {
			priorityClassifier = classifier.NewPriorityClassifier(ctrl.Log.WithName("test"))
		})

		// Test 1: Critical severity + production = P0
		It("should assign P0 for critical severity in production", func() {
			result := priorityClassifier.Classify(ctx, "critical", "production")

			Expect(result.Priority).To(Equal("P0"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.9))
		})

		// Test 2: Critical severity + staging = P1
		It("should assign P1 for critical severity in staging", func() {
			result := priorityClassifier.Classify(ctx, "critical", "staging")

			Expect(result.Priority).To(Equal("P1"))
		})

		// Test 3: Warning severity + production = P1
		It("should assign P1 for warning severity in production", func() {
			result := priorityClassifier.Classify(ctx, "warning", "production")

			Expect(result.Priority).To(Equal("P1"))
		})

		// Test 4: Info severity = P3
		It("should assign P3 for info severity", func() {
			result := priorityClassifier.Classify(ctx, "info", "production")

			Expect(result.Priority).To(Equal("P3"))
		})

		// Test 5: Unknown environment = lower priority
		It("should lower priority for unknown environment", func() {
			result := priorityClassifier.Classify(ctx, "warning", "unknown")

			Expect(result.Priority).To(Equal("P2"))
		})

		// Test 6: Development always uses P3 (unless critical)
		DescribeTable("should use lower priority for development",
			func(severity, expectedPriority string) {
				result := priorityClassifier.Classify(ctx, severity, "development")
				Expect(result.Priority).To(Equal(expectedPriority))
			},
			Entry("critical in dev is P2", "critical", "P2"),
			Entry("warning in dev is P3", "warning", "P3"),
			Entry("info in dev is P3", "info", "P3"),
		)
	})
})

