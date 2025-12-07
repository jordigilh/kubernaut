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

package datastorage

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// INPUT VALIDATION UNIT TESTS
// 📋 Testing Principle: Behavior + Correctness
// ========================================
var _ = Describe("BR-STORAGE-010: Input Validation", func() {
	var validator *validation.Validator

	BeforeEach(func() {
		logger := kubelog.NewLogger(kubelog.DevelopmentOptions())
		validator = validation.NewValidator(logger)
	})

	// ⭐ TABLE-DRIVEN: Validation test cases
	// BEHAVIOR: Validator accepts valid audit records and rejects invalid ones
	// CORRECTNESS: Required fields must be present, invalid values are rejected
	DescribeTable("should validate RemediationAudit records",
		func(audit *models.RemediationAudit, shouldPass bool, expectedErrorContains string) {
			err := validator.ValidateRemediationAudit(audit)

			if shouldPass {
				Expect(err).ToNot(HaveOccurred(), "validation should pass")
			} else {
				Expect(err).To(HaveOccurred(), "validation should fail")
				Expect(err.Error()).To(ContainSubstring(expectedErrorContains))
			}
		},

		// Valid cases
		Entry("BR-STORAGE-010.1: valid complete audit",
			&models.RemediationAudit{
				Name:                 "test-remediation",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			true, ""),

		Entry("BR-STORAGE-010.2: valid minimal audit",
			&models.RemediationAudit{
				Name:                 "minimal",
				Namespace:            "kube-system",
				Phase:                "processing",
				ActionType:           "restart_deployment",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-456",
				SignalFingerprint:    "alert-def",
				Severity:             "critical",
				Environment:          "staging",
				ClusterName:          "stage-cluster",
				TargetResource:       "pod/test",
				Metadata:             "{}",
			},
			true, ""),

		// Missing required fields
		Entry("BR-STORAGE-010.3: missing name",
			&models.RemediationAudit{
				Name:                 "",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "name is required"),

		Entry("BR-STORAGE-010.4: missing namespace",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            "",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "namespace is required"),

		Entry("BR-STORAGE-010.5: missing phase",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            "default",
				Phase:                "",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "phase is required"),

		Entry("BR-STORAGE-010.6: missing action_type",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "action_type is required"),

		// Invalid phase value
		Entry("BR-STORAGE-010.7: invalid phase value",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            "default",
				Phase:                "invalid-phase",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "invalid phase"),

		// Field length violations
		Entry("BR-STORAGE-010.8: name exceeds maximum length (256 chars)",
			&models.RemediationAudit{
				Name:                 strings.Repeat("a", 256),
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "exceeds maximum length"),

		Entry("BR-STORAGE-010.9: namespace exceeds maximum length (256 chars)",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            strings.Repeat("n", 256),
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "exceeds maximum length"),

		Entry("BR-STORAGE-010.10: action_type exceeds maximum length (101 chars)",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           strings.Repeat("a", 101),
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			false, "exceeds maximum length"),

		// Boundary conditions (valid)
		Entry("BR-STORAGE-010.11: name at maximum length (255 chars) - valid",
			&models.RemediationAudit{
				Name:                 strings.Repeat("a", 255),
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			true, ""),

		Entry("BR-STORAGE-010.12: action_type at maximum length (100 chars) - valid",
			&models.RemediationAudit{
				Name:                 "test",
				Namespace:            "default",
				Phase:                "completed",
				ActionType:           strings.Repeat("a", 100),
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				SignalFingerprint:    "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			true, ""),
	)

	Context("phase validation", func() {
		It("should accept all valid phases", func() {
			validPhases := []string{"pending", "processing", "completed", "failed"}

			for _, phase := range validPhases {
				audit := &models.RemediationAudit{
					Name:                 "test",
					Namespace:            "default",
					Phase:                phase,
					ActionType:           "scale_deployment",
					Status:               "success",
					StartTime:            time.Now(),
					RemediationRequestID: "req-123",
					SignalFingerprint:    "alert-abc",
					Severity:             "high",
					Environment:          "production",
					ClusterName:          "prod-cluster",
					TargetResource:       "deployment/my-app",
					Metadata:             "{}",
				}

				err := validator.ValidateRemediationAudit(audit)
				Expect(err).ToNot(HaveOccurred(), "phase %s should be valid", phase)
			}
		})
	})
})
