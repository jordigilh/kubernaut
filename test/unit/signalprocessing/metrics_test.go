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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
package signalprocessing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Metrics Suite")
}

// Unit Test: Metrics package implementation correctness
var _ = Describe("Metrics", func() {
	var (
		registry *prometheus.Registry
		m        *metrics.Metrics
	)

	BeforeEach(func() {
		registry = prometheus.NewRegistry()
		m = metrics.NewMetrics(registry)
	})

	// Test 1: Metrics creation
	It("should create metrics with all required counters and gauges", func() {
		Expect(m).NotTo(BeNil())
		Expect(m.ProcessingTotal).NotTo(BeNil())
		Expect(m.ProcessingDuration).NotTo(BeNil())
		Expect(m.EnrichmentErrors).NotTo(BeNil())
	})
})
