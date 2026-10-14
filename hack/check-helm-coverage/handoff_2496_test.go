package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jordigilh/kubernaut/hack/internal/helmschema"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHandoffConfigCoverage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handoff Config Coverage")
}

var _ = Describe("KA chart coverage attribution [BR-PLATFORM-011, BR-KA-016]", func() {
	It("UT-PLATFORM-2496-CI-001: recognizes the actual handoff-budget assertions without an allowlist", func() {
		chart := filepath.Join("..", "..", "charts", "kubernaut")
		body, err := os.ReadFile(filepath.Join(chart, "values.schema.json"))
		Expect(err).NotTo(HaveOccurred())
		schema, err := helmschema.ParseSchema(body)
		Expect(err).NotTo(HaveOccurred())
		corpus, err := BuildAssertionCorpus(filepath.Join(chart, "tests"))
		Expect(err).NotTo(HaveOccurred())

		Expect(IsCovered("maxEvidenceHandoffBytes", corpus["kubernautAgent"])).To(BeTrue())
		Expect(ComputeGaps(CollectSchemaLeaves(schema), corpus)).NotTo(ContainElement(Leaf{
			Service: "kubernautAgent", Path: "investigation.maxEvidenceHandoffBytes",
		}))
		Expect(IsCovered("maxEvidenceHandoffBytes", corpus["gateway"])).To(BeFalse(),
			"KA assertions must not count as another service's coverage")
	})
})
