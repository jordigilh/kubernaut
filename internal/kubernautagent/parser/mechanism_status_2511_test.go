/*
Copyright 2026 Jordi Gil.
Licensed under the Apache License, Version 2.0.
*/

package parser_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

// BR-KA-016 / BR-AUDIT-005; DD-KA-2511; FedRAMP AU-3; SOC 2 CC7.2.
var _ = Describe("#2511 causal mechanism status", func() {
	It("UT-KA-2511-006: preserves the verified/probable/unknown mechanism status", func() {
		result, err := parser.NewResultParser().Parse(`{"root_cause_analysis":{"summary":"certificate chain mismatch","causal_mechanism_status":"probable"},"confidence":0.72}`)
		Expect(err).NotTo(HaveOccurred())
		encoded, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).To(ContainSubstring(`"causal_mechanism_status":"probable"`))
	})

	It("UT-KA-2511-007: rejects an invalid mechanism status on the flat parser path", func() {
		result, err := parser.NewResultParser().Parse(`{"rca_summary":"certificate chain mismatch","causal_mechanism_status":"invented","confidence":0.72}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.CausalMechanismStatus).To(BeEmpty())
	})

	It("UT-KA-2511-008: records explicit zero confidence as present", func() {
		result, err := parser.NewResultParser().Parse(`{"root_cause_analysis":{"summary":"uncertain"},"confidence":0}`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Confidence).To(BeZero())
		Expect(result.ConfidenceSet).To(BeTrue())
	})
})
