package sanitization_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
)

// trackingStage records calls and transforms input for verification.
type trackingStage struct {
	name    string
	calls   []string
	transform func(string) string
}

func (s *trackingStage) Name() string { return s.name }
func (s *trackingStage) Sanitize(_ context.Context, input string) (string, error) {
	s.calls = append(s.calls, input)
	if s.transform != nil {
		return s.transform(input), nil
	}
	return input + "[" + s.name + "]", nil
}

var _ = Describe("Kubernaut Agent Sanitization Pipeline Unit — #433", func() {

	Describe("UT-KA-433-035: Sanitization pipeline executes G4 before I1 in correct order", func() {
		It("should execute stages in G4 → I1 order", func() {
			g4 := &trackingStage{name: "G4"}
			i1 := &trackingStage{name: "I1"}
			pipeline := sanitization.NewPipeline(g4, i1)
			Expect(pipeline).NotTo(BeNil(), "NewPipeline should return a non-nil pipeline")

			result, err := pipeline.Run(context.Background(), "raw tool output")
			Expect(err).NotTo(HaveOccurred())

			By("G4 should have been called first with raw input")
			Expect(g4.calls).To(HaveLen(1))
			Expect(g4.calls[0]).To(Equal("raw tool output"))

			By("I1 should have been called second with G4's output")
			Expect(i1.calls).To(HaveLen(1))
			Expect(i1.calls[0]).To(Equal("raw tool output[G4]"))

			By("Final result should show both stages applied in order")
			Expect(result).To(Equal("raw tool output[G4][I1]"))
		})

		It("should expose stage names in correct order", func() {
			g4 := &trackingStage{name: "G4"}
			i1 := &trackingStage{name: "I1"}
			pipeline := sanitization.NewPipeline(g4, i1)
			Expect(pipeline).NotTo(BeNil())
			Expect(pipeline.StageNames()).To(Equal([]string{"G4", "I1"}))
		})
	})
})
