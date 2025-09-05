package validation

import (
	"strings"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validation", func() {
	Describe("ValidateResourceReference", func() {
		Context("with valid resource reference", func() {
			It("should pass validation", func() {
				ref := actionhistory.ResourceReference{
					Namespace: "production",
					Kind:      "Deployment",
					Name:      "webapp",
				}

				err := ValidateResourceReference(ref)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when namespace is invalid", func() {
			Context("when namespace is empty", func() {
				It("should return validation error", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "",
						Kind:      "Deployment",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("namespace is required"))
				})
			})

			Context("when namespace is too long", func() {
				It("should return validation error", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "a-very-long-namespace-name-that-exceeds-the-kubernetes-limit-of-sixty-three-characters",
						Kind:      "Deployment",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("namespace must be 63 characters or less"))
				})
			})

			Context("when namespace has invalid characters", func() {
				It("should return validation error for uppercase", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "Production",
						Kind:      "Deployment",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("namespace must be a valid Kubernetes namespace name"))
				})

				It("should return validation error for special characters", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "prod_env",
						Kind:      "Deployment",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("namespace must be a valid Kubernetes namespace name"))
				})
			})
		})

		Context("when kind is invalid", func() {
			Context("when kind is empty", func() {
				It("should return validation error", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("kind is required"))
				})
			})

			Context("when kind is too long", func() {
				It("should return validation error", func() {
					// Create a string longer than 100 characters
					longKind := strings.Repeat("A", 101)
					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      longKind,
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("kind must be 100 characters or less"))
				})
			})

			Context("when kind has invalid format", func() {
				It("should return validation error for lowercase start", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "deployment",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("kind must be a valid Kubernetes resource kind"))
				})

				It("should return validation error for special characters", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment-V1",
						Name:      "webapp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("kind must be a valid Kubernetes resource kind"))
				})
			})
		})

		Context("when name is invalid", func() {
			Context("when name is empty", func() {
				It("should return validation error", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      "",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("name is required"))
				})
			})

			Context("when name is too long", func() {
				It("should return validation error", func() {
					longName := ""
					for i := 0; i < 260; i++ {
						longName += "a"
					}

					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      longName,
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("name must be 253 characters or less"))
				})
			})

			Context("when name has invalid characters", func() {
				It("should return validation error for uppercase", func() {
					ref := actionhistory.ResourceReference{
						Namespace: "production",
						Kind:      "Deployment",
						Name:      "WebApp",
					}

					err := ValidateResourceReference(ref)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("name must be a valid Kubernetes resource name"))
				})
			})
		})

		Context("with multiple validation errors", func() {
			It("should return combined validation errors", func() {
				ref := actionhistory.ResourceReference{
					Namespace: "",
					Kind:      "",
					Name:      "",
				}

				err := ValidateResourceReference(ref)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("namespace is required"))
				Expect(err.Error()).To(ContainSubstring("kind is required"))
				Expect(err.Error()).To(ContainSubstring("name is required"))
			})
		})
	})

	Describe("ValidateStringInput", func() {
		Context("with valid input", func() {
			It("should pass validation", func() {
				err := ValidateStringInput("field", "validinput123", 100)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when input is too long", func() {
			It("should return validation error", func() {
				err := ValidateStringInput("field", "toolong", 5)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be 5 characters or less"))
			})
		})

		Context("when input contains SQL injection patterns", func() {
			It("should detect UNION attacks", func() {
				err := ValidateStringInput("field", "'; UNION SELECT * FROM users --", 100)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains potentially unsafe characters"))
			})

			It("should detect script injection", func() {
				err := ValidateStringInput("field", "<script>alert('xss')</script>", 100)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains potentially unsafe characters"))
			})

			It("should detect SQL comments", func() {
				err := ValidateStringInput("field", "input-- comment", 100)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains potentially unsafe characters"))
			})
		})

		Context("when input contains control characters", func() {
			It("should detect control characters", func() {
				controlChar := string(rune(0x01)) // SOH control character
				err := ValidateStringInput("field", "input"+controlChar, 100)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains invalid control characters"))
			})

			It("should allow valid whitespace", func() {
				err := ValidateStringInput("field", "input\twith\nlines\r", 100)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("ValidateActionType", func() {
		Context("with valid action types", func() {
			validActions := []string{
				"scale_deployment",
				"increase_resources",
				"restart_deployment",
				"rollback_deployment",
				"create_hpa",
			}

			for _, action := range validActions {
				action := action // Capture loop variable
				It("should accept "+action, func() {
					err := ValidateActionType(action)
					Expect(err).NotTo(HaveOccurred())
				})
			}
		})

		Context("with invalid action types", func() {
			It("should reject unknown actions", func() {
				err := ValidateActionType("delete_everything")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("is not a recognized action type"))
			})

			It("should reject actions with SQL injection", func() {
				err := ValidateActionType("scale'; DROP TABLE users; --")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains potentially unsafe characters"))
			})
		})
	})

	Describe("ValidateTimeRange", func() {
		Context("with valid time ranges", func() {
			validRanges := []string{"1h", "24h", "7d", "30d", "60m"}

			for _, timeRange := range validRanges {
				timeRange := timeRange // Capture loop variable
				It("should accept "+timeRange, func() {
					err := ValidateTimeRange(timeRange)
					Expect(err).NotTo(HaveOccurred())
				})
			}
		})

		Context("with invalid time ranges", func() {
			It("should reject invalid format", func() {
				err := ValidateTimeRange("invalid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be in format like"))
			})

			It("should reject SQL injection attempts", func() {
				err := ValidateTimeRange("1h';DROP")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("contains potentially unsafe characters"))
			})
		})
	})

	Describe("ValidateWindowMinutes", func() {
		Context("with valid window minutes", func() {
			It("should accept valid ranges", func() {
				validWindows := []int{1, 60, 120, 1440, 10080}

				for _, window := range validWindows {
					err := ValidateWindowMinutes(window)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("with invalid window minutes", func() {
			It("should reject zero", func() {
				err := ValidateWindowMinutes(0)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be greater than 0"))
			})

			It("should reject negative values", func() {
				err := ValidateWindowMinutes(-1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be greater than 0"))
			})

			It("should reject too large values", func() {
				err := ValidateWindowMinutes(20000)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be 7 days (10080 minutes) or less"))
			})
		})
	})

	Describe("ValidateLimit", func() {
		Context("with valid limits", func() {
			It("should accept valid ranges", func() {
				validLimits := []int{1, 50, 100, 1000, 10000}

				for _, limit := range validLimits {
					err := ValidateLimit(limit)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("with invalid limits", func() {
			It("should reject zero", func() {
				err := ValidateLimit(0)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be greater than 0"))
			})

			It("should reject negative values", func() {
				err := ValidateLimit(-1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be greater than 0"))
			})

			It("should reject too large values", func() {
				err := ValidateLimit(50000)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be 10000 or less"))
			})
		})
	})

	Describe("SanitizeForLogging", func() {
		Context("with clean input", func() {
			It("should return input unchanged", func() {
				input := "clean input text"
				result := SanitizeForLogging(input)
				Expect(result).To(Equal(input))
			})
		})

		Context("with control characters", func() {
			It("should replace control characters", func() {
				controlChar := string(rune(0x01))
				input := "text" + controlChar + "more"
				result := SanitizeForLogging(input)
				Expect(result).To(Equal("text?more"))
			})

			It("should preserve valid whitespace", func() {
				input := "text\twith\nlines\r"
				result := SanitizeForLogging(input)
				Expect(result).To(Equal(input))
			})
		})

		Context("with long input", func() {
			It("should truncate long strings", func() {
				longInput := ""
				for i := 0; i < 300; i++ {
					longInput += "a"
				}

				result := SanitizeForLogging(longInput)
				Expect(len(result)).To(Equal(200))
				Expect(result).To(HaveSuffix("..."))
			})
		})
	})
})
