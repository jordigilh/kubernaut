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
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// WORKFLOW DISCOVERY HANDLER UNIT TESTS
// ========================================
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
//
// Test Plan: docs/testing/DD-HAPI-017/TEST_PLAN.md
// Test IDs: UT-DS-017-001-001 through UT-DS-017-001-009, UT-DS-017-005-001
//
// Strategy: Unit tests for HTTP handler parsing and formatting logic.
// Repository integration is tested separately in IT-DS-017-001-* tests.
// ========================================

var _ = Describe("Workflow Discovery Handler Unit Tests", func() {

	// ========================================
	// UT-DS-017-001-003: ListActions handler -- context filter parameters parsed
	// ========================================
	Describe("parseDiscoveryFilters", func() {
		Context("UT-DS-017-001-003: context filter parameters parsed correctly", func() {
			It("should parse all mandatory context filter query parameters", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?severity=critical&component=pod&environment=production&priority=P0", nil)

				// Act
				filters, err := server.ParseDiscoveryFilters(req)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(filters).ToNot(BeNil())
				Expect(filters.Severity).To(Equal("critical"))
				Expect(filters.Component).To(Equal("pod"))
				Expect(filters.Environment).To(Equal("production"))
				Expect(filters.Priority).To(Equal("P0"))
			})

			It("should parse remediation_id query parameter", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?severity=critical&component=pod&environment=production&priority=P0&remediation_id=rem-uuid-123", nil)

				// Act
				filters, err := server.ParseDiscoveryFilters(req)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(filters.RemediationID).To(Equal("rem-uuid-123"))
			})

			It("should parse custom_labels JSON query parameter", func() {
				// Arrange
				customLabelsJSON := url.QueryEscape(`{"constraint":["cost-constrained"],"team":["payments"]}`)
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?severity=critical&component=pod&environment=production&priority=P0&custom_labels="+customLabelsJSON, nil)

				// Act
				filters, err := server.ParseDiscoveryFilters(req)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(filters.CustomLabels).To(HaveKey("constraint"))
				Expect(filters.CustomLabels["constraint"]).To(ContainElement("cost-constrained"))
				Expect(filters.CustomLabels["team"]).To(ContainElement("payments"))
			})

			It("should return error for invalid custom_labels JSON", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?custom_labels=invalid-json", nil)

				// Act
				_, err := server.ParseDiscoveryFilters(req)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid custom_labels JSON"))
			})

			It("should return empty filters when no query params provided", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions", nil)

				// Act
				filters, err := server.ParseDiscoveryFilters(req)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(filters.Severity).To(BeEmpty())
				Expect(filters.Component).To(BeEmpty())
				Expect(filters.Environment).To(BeEmpty())
				Expect(filters.Priority).To(BeEmpty())
			})
		})
	})

	// ========================================
	// UT-DS-017-001-002/006: Pagination parsing
	// ========================================
	Describe("parsePagination", func() {
		Context("UT-DS-017-001-002: pagination parameters parsed correctly", func() {
			It("should use default values when no pagination params", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions", nil)

				// Act
				offset, limit := server.ParsePagination(req)

				// Assert
				Expect(offset).To(Equal(0))
				Expect(limit).To(Equal(models.DefaultPaginationLimit)) // 10
			})

			It("should parse valid offset and limit", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?offset=20&limit=5", nil)

				// Act
				offset, limit := server.ParsePagination(req)

				// Assert
				Expect(offset).To(Equal(20))
				Expect(limit).To(Equal(5))
			})

			It("should cap limit at maximum", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?limit=500", nil)

				// Act
				_, limit := server.ParsePagination(req)

				// Assert
				Expect(limit).To(Equal(models.MaxPaginationLimit)) // 100
			})

			It("should default negative offset to 0", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?offset=-5", nil)

				// Act
				offset, _ := server.ParsePagination(req)

				// Assert
				Expect(offset).To(Equal(0))
			})

			It("should default non-numeric limit to default", func() {
				// Arrange
				req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/actions?limit=abc", nil)

				// Act
				_, limit := server.ParsePagination(req)

				// Assert
				Expect(limit).To(Equal(models.DefaultPaginationLimit))
			})
		})
	})

	// ========================================
	// UT-DS-017-005-001: remediationId propagated in handler
	// ========================================
	Describe("WorkflowDiscoveryFilters", func() {
		Context("UT-DS-017-005-001: remediationId propagated correctly", func() {
			It("should detect when context filters are present", func() {
				filters := &models.WorkflowDiscoveryFilters{
					Severity:    "critical",
					Component:   "pod",
					Environment: "production",
					Priority:    "P0",
				}
				Expect(filters.HasContextFilters()).To(BeTrue())
			})

			It("should detect when no context filters are present", func() {
				filters := &models.WorkflowDiscoveryFilters{
					RemediationID: "rem-uuid-123",
				}
				Expect(filters.HasContextFilters()).To(BeFalse())
			})

			It("should detect partial context filters", func() {
				filters := &models.WorkflowDiscoveryFilters{
					Severity: "critical",
				}
				Expect(filters.HasContextFilters()).To(BeTrue())
			})
		})
	})
})
