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

// BR-AI-001: Handler configuration options for operator-tunable settings
package aianalysis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("UT-AA-668-001: AnalyzingHandler WithConfidenceThreshold", func() {
	It("BR-AI-011: should set operator-configurable confidence threshold", func() {
		mockEval := mocks.NewMockRegoEvaluator()
		m := metrics.NewMetrics()
		log := ctrl.Log.WithName("test")

		threshold := 0.85
		handler := handlers.NewAnalyzingHandler(mockEval, log, m, &noopAnalyzingAuditClient{}).
			WithConfidenceThreshold(&threshold)

		Expect(handler).NotTo(BeNil())
		Expect(handler.Name()).To(Equal("analyzing"))
	})

	It("BR-AI-011: should accept nil threshold to use policy default", func() {
		mockEval := mocks.NewMockRegoEvaluator()
		m := metrics.NewMetrics()
		log := ctrl.Log.WithName("test")

		handler := handlers.NewAnalyzingHandler(mockEval, log, m, &noopAnalyzingAuditClient{}).
			WithConfidenceThreshold(nil)

		Expect(handler).NotTo(BeNil())
	})
})

var _ = Describe("UT-AA-668-003: InvestigatingHandler WithSessionPollInterval", func() {
	It("BR-AI-011: should configure custom session poll interval", func() {
		mockClient := mocks.NewMockAgentClient()
		m := metrics.NewMetrics()
		log := ctrl.Log.WithName("test")

		customInterval := 5 * time.Second
		handler := handlers.NewInvestigatingHandler(
			mockClient, log, m, &noopAuditClient{},
			handlers.WithSessionPollInterval(customInterval),
		)

		Expect(handler).NotTo(BeNil())
	})

	It("BR-AI-011: should use default poll interval when option not provided", func() {
		mockClient := mocks.NewMockAgentClient()
		m := metrics.NewMetrics()
		log := ctrl.Log.WithName("test")

		handler := handlers.NewInvestigatingHandler(mockClient, log, m, &noopAuditClient{})

		Expect(handler).NotTo(BeNil())
	})
})
