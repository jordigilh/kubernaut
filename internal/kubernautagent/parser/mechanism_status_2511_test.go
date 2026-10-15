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
})
