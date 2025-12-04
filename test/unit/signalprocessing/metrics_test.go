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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

var _ = Describe("Metrics", func() {
	var m *metrics.Metrics

	BeforeEach(func() {
		m = metrics.NewMetrics()
	})

	// Test 1: NewMetrics should create all required metrics
	// DD-005: Observability Standards
	It("should create all required metrics", func() {
		Expect(m).NotTo(BeNil())
		Expect(m.ReconciliationTotal).NotTo(BeNil())
		Expect(m.ReconciliationDuration).NotTo(BeNil())
		Expect(m.EnrichmentDuration).NotTo(BeNil())
		Expect(m.RegoEvaluationDuration).NotTo(BeNil())
		Expect(m.RegoHotReloadTotal).NotTo(BeNil())
		Expect(m.CategorizationConfidence).NotTo(BeNil())
	})
})

