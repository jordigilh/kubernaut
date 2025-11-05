package contextapi

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
)

var _ = Describe("Vector Helper Functions", func() {
	Context("VectorToString", func() {
		It("should convert embedding to PostgreSQL vector format", func() {
			embedding := []float32{0.1, 0.2, 0.3}

			result, err := query.VectorToString(embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("[0.1,0.2,0.3]"))
		})

		It("should handle single element vector", func() {
			embedding := []float32{0.5}

			result, err := query.VectorToString(embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("[0.5]"))
		})

		It("should handle large vectors", func() {
			embedding := make([]float32, 1536) // OpenAI embedding size
			for i := range embedding {
				embedding[i] = float32(i) * 0.001
			}

			result, err := query.VectorToString(embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HavePrefix("[0,"))
			Expect(result).To(HaveSuffix("]"))
		})

		It("should return error for nil embedding", func() {
			result, err := query.VectorToString(nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be nil"))
			Expect(result).To(BeEmpty())
		})

		It("should return error for empty embedding", func() {
			embedding := []float32{}

			result, err := query.VectorToString(embedding)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be empty"))
			Expect(result).To(BeEmpty())
		})
	})

	Context("StringToVector", func() {
		It("should parse PostgreSQL vector format to embedding", func() {
			vectorStr := "[0.1,0.2,0.3]"

			result, err := query.StringToVector(vectorStr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(BeNumerically("~", 0.1, 0.001))
			Expect(result[1]).To(BeNumerically("~", 0.2, 0.001))
			Expect(result[2]).To(BeNumerically("~", 0.3, 0.001))
		})

		It("should handle single element vector", func() {
			vectorStr := "[0.5]"

			result, err := query.StringToVector(vectorStr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(BeNumerically("~", 0.5, 0.001))
		})

		It("should handle vectors with spaces", func() {
			vectorStr := "[0.1, 0.2, 0.3]"

			result, err := query.StringToVector(vectorStr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(3))
			Expect(result[0]).To(BeNumerically("~", 0.1, 0.001))
		})

		It("should handle empty vector", func() {
			vectorStr := "[]"

			result, err := query.StringToVector(vectorStr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(0))
		})

		It("should return error for empty string", func() {
			result, err := query.StringToVector("")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot be empty"))
			Expect(result).To(BeNil())
		})

		It("should return error for invalid format", func() {
			vectorStr := "[0.1,invalid,0.3]"

			result, err := query.StringToVector(vectorStr)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse"))
			Expect(result).To(BeNil())
		})
	})

	Context("Round-trip conversion", func() {
		It("should preserve values through conversion cycle", func() {
			original := []float32{0.1, 0.2, 0.3, 0.4, 0.5}

			// Convert to string
			vectorStr, err := query.VectorToString(original)
			Expect(err).ToNot(HaveOccurred())

			// Convert back to vector
			result, err := query.StringToVector(vectorStr)
			Expect(err).ToNot(HaveOccurred())

			// Verify values match
			Expect(result).To(HaveLen(len(original)))
			for i := range original {
				Expect(result[i]).To(BeNumerically("~", original[i], 0.001))
			}
		})

		It("should handle negative values", func() {
			original := []float32{-0.5, 0.0, 0.5}

			vectorStr, err := query.VectorToString(original)
			Expect(err).ToNot(HaveOccurred())

			result, err := query.StringToVector(vectorStr)
			Expect(err).ToNot(HaveOccurred())

			Expect(result[0]).To(BeNumerically("~", -0.5, 0.001))
			Expect(result[1]).To(BeNumerically("~", 0.0, 0.001))
			Expect(result[2]).To(BeNumerically("~", 0.5, 0.001))
		})

		It("should handle very small values", func() {
			original := []float32{0.000001, 0.000002, 0.000003}

			vectorStr, err := query.VectorToString(original)
			Expect(err).ToNot(HaveOccurred())

			result, err := query.StringToVector(vectorStr)
			Expect(err).ToNot(HaveOccurred())

			for i := range original {
				Expect(result[i]).To(BeNumerically("~", original[i], 0.0000001))
			}
		})
	})
})
