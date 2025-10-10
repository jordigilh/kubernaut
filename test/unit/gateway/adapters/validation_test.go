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

package adapters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

// TDD Principle: Test BUSINESS REQUIREMENTS, not implementation
// BR-GATEWAY-003: Validate incoming webhook payloads and reject invalid data

var _ = Describe("BR-GATEWAY-003: Payload Validation", func() {
	var (
		adapter *adapters.PrometheusAdapter
		ctx     context.Context
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter()
		ctx = context.Background()
	})

	// Following testing-strategy.md guidance: Use DescribeTable for validation tests
	// "BEST PRACTICE: Use Ginkgo's DescribeTable for environment classification and storm detection testing"
	// Validation tests have consistent logic (reject invalid payload) with varying inputs
	//
	// NOTE: Tests simulate full HTTP handler flow (Parse + Validate)
	// This matches BR-GATEWAY-003: "Validate incoming webhook payloads and reject invalid data"
	DescribeTable("should reject invalid payloads",
		func(testCase string, payload []byte, expectedErrorSubstring string) {
			// Parse payload
			signal, err := adapter.Parse(ctx, payload)

			// For parsing errors (malformed JSON, missing alerts), Parse() returns error
			if err != nil {
				Expect(err).To(HaveOccurred(),
					"BR-003: Must reject invalid payload at Parse stage: %s", testCase)
				if expectedErrorSubstring != "" {
					Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring),
						"BR-003: Error message should indicate validation failure type")
				}
				return
			}

			// For validation errors (missing required fields), Validate() returns error
			err = adapter.Validate(signal)
			Expect(err).To(HaveOccurred(),
				"BR-003: Must reject invalid payload at Validate stage: %s", testCase)
			if expectedErrorSubstring != "" {
				Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring),
					"BR-003: Error message should indicate validation failure type")
			}
		},
		// Malformed JSON (caught by Parse)
		Entry("malformed JSON syntax",
			"invalid JSON structure",
			[]byte(`{"alerts": [{"labels": {"alertname": "Test"]}`), // Missing closing brace
			"invalid"),
		Entry("empty payload",
			"empty payload provides no actionable data",
			[]byte(``),
			""),
		Entry("null payload",
			"null is not a valid AlertManager webhook",
			[]byte(`null`),
			""),

		// Missing required fields (caught by Parse)
		Entry("missing alerts array",
			"AlertManager webhook must contain alerts array",
			[]byte(`{"version": "4", "status": "firing"}`),
			"alert"),
		Entry("empty alerts array",
			"at least one alert must be present",
			[]byte(`{"alerts": []}`),
			"alert"),

		// Missing alertname (caught by Validate)
		// Note: Parse succeeds, but Validate fails because alertname is required
		Entry("missing alertname label",
			"alertname is required for identification",
			[]byte(`{"alerts": [{"labels": {"namespace": "prod", "pod": "api-1"}}]}`),
			"alertName"),

		// NOTE: Namespace is NOT required per prometheus_adapter.go:154
		//   "// Note: Namespace can be empty for cluster-scoped alerts (e.g., node alerts)"
		// So we don't test for missing namespace

		// Incorrect structure (caught by Parse)
		Entry("alerts array contains non-object",
			"each alert must be a structured object",
			[]byte(`{"alerts": ["string instead of object"]}`),
			""),
		Entry("labels is not an object",
			"labels must be key-value pairs",
			[]byte(`{"alerts": [{"labels": "string instead of object"}]}`),
			""),
	)
})
