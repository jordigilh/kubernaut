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
package aianalysis_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

var _ = Describe("UT-AA-668-001: AnalyzingHandler WithConfidenceThreshold", func() {
	It("BR-AI-011: should set operator-configurable confidence threshold", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		spy := newSpyEvaluator(ctx, "testdata/policies/always_approve.rego")
		m := metrics.NewMetrics()
		log := ctrl.Log.WithName("test")

		threshold := 0.85
		handler := handlers.NewAnalyzingHandler(spy, log, m, &noopAnalyzingAuditClient{}).
			WithConfidenceThreshold(&threshold)

		Expect(handler).NotTo(BeNil())
		Expect(handler.Name()).To(Equal("analyzing"))
	})

	It("BR-AI-011: should accept nil threshold to use policy default", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		spy := newSpyEvaluator(ctx, "testdata/policies/always_approve.rego")
		m := metrics.NewMetrics()
		log := ctrl.Log.WithName("test")

		handler := handlers.NewAnalyzingHandler(spy, log, m, &noopAnalyzingAuditClient{}).
			WithConfidenceThreshold(nil)

		Expect(handler).NotTo(BeNil())
	})
})

// #2204 (2026-08-20): UT-AA-668-003 (WithSessionPollInterval) removed along
// with the option itself -- see pkg/aianalysis/handlers/constants.go's
// "SESSION CONFIGURATION" doc comment for the rationale (AgentSession watch
// is the completion signal; the backstop is now deadline-derived, not a
// configurable poll cadence).
