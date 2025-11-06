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

package contextapi

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/datastorage"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"go.uber.org/zap"
)

// ========================================
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// TDD RED Phase: AggregationService Unit Tests
// ========================================
//
// BEHAVIOR: AggregationService aggregates success rate data from Data Storage Service
// CORRECTNESS: Caching works correctly, Data Storage client is called with correct parameters
//
// ADR-033: Context API becomes Aggregation Layer for AI/LLM Service
// ADR-032: No direct PostgreSQL access - use Data Storage REST API
// ========================================

var _ = Describe("AggregationService", func() {
	var (
		aggregationService *query.AggregationService
		mockDSClient       *MockDataStorageClient
		mockCache          cache.CacheManager
		logger             *zap.Logger
		ctx                context.Context
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		ctx = context.Background()
		mockDSClient = NewMockDataStorageClient()

		// Create L2 LRU cache only for unit tests (no Redis)
		cfg := &cache.Config{
			RedisAddr:  "localhost:6379", // Not used in unit tests
			LRUSize:    1000,
			DefaultTTL: 5 * time.Minute,
		}
		var err error
		mockCache, err = cache.NewCacheManager(cfg, logger)
		Expect(err).ToNot(HaveOccurred())

		aggregationService = query.NewAggregationService(mockDSClient, mockCache, logger)
	})

	// ========================================
	// BR-INTEGRATION-008: Incident-Type Success Rate
	// ========================================

	Describe("GetSuccessRateByIncidentType", func() {
		Context("when Data Storage returns success rate data", func() {
			BeforeEach(func() {
				// Mock Data Storage client response
				mockDSClient.SetIncidentTypeResponse(&dsmodels.IncidentTypeSuccessRateResponse{
					IncidentType:         "pod-oom-killer",
					TimeRange:            "7d",
					TotalExecutions:      100,
					SuccessfulExecutions: 90,
					FailedExecutions:     10,
					SuccessRate:          90.0,
					Confidence:           "high",
					MinSamplesMet:        true,
				}, nil)
			})

			It("should return aggregated success rate for incident type", func() {
				// ACT: Call aggregation service
				result, err := aggregationService.GetSuccessRateByIncidentType(ctx, "pod-oom-killer", "7d", 5)

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Verify response data
				Expect(result).ToNot(BeNil())
				Expect(result.IncidentType).To(Equal("pod-oom-killer"))
				Expect(result.TotalExecutions).To(Equal(100))
				Expect(result.SuccessfulExecutions).To(Equal(90))
				Expect(result.FailedExecutions).To(Equal(10))
				Expect(result.SuccessRate).To(Equal(90.0))
				Expect(result.Confidence).To(Equal("high"))
				Expect(result.MinSamplesMet).To(BeTrue())
			})

			It("should call Data Storage client with correct parameters", func() {
				// ACT
				_, err := aggregationService.GetSuccessRateByIncidentType(ctx, "pod-oom-killer", "7d", 5)

				// ASSERT: Client was called
				Expect(err).ToNot(HaveOccurred())
				Expect(mockDSClient.GetIncidentTypeCallCount()).To(Equal(1))

				// CORRECTNESS: Verify parameters
				incidentType, timeRange, minSamples := mockDSClient.GetIncidentTypeLastCall()
				Expect(incidentType).To(Equal("pod-oom-killer"))
				Expect(timeRange).To(Equal("7d"))
				Expect(minSamples).To(Equal(5))
			})
		})

		Context("when incident_type is empty", func() {
			It("should return validation error", func() {
				_, err := aggregationService.GetSuccessRateByIncidentType(ctx, "", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("incident_type cannot be empty"))
			})
		})

		Context("when Data Storage client returns error", func() {
			BeforeEach(func() {
				mockDSClient.SetIncidentTypeResponse(nil, errors.New("database connection failed"))
			})

			It("should return the error from Data Storage", func() {
				_, err := aggregationService.GetSuccessRateByIncidentType(ctx, "pod-oom-killer", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database connection failed"))
			})
		})

		Context("when cache contains valid data", func() {
			BeforeEach(func() {
				// Pre-populate cache
				cacheKey := "incident_type:pod-oom-killer:7d:5"
				cachedData := &dsmodels.IncidentTypeSuccessRateResponse{
					IncidentType:         "pod-oom-killer",
					TimeRange:            "7d",
					TotalExecutions:      100,
					SuccessfulExecutions: 90,
					FailedExecutions:     10,
					SuccessRate:          90.0,
					Confidence:           "high",
					MinSamplesMet:        true,
				}
				err := mockCache.Set(ctx, cacheKey, cachedData)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return cached data without calling Data Storage", func() {
				// ACT
				result, err := aggregationService.GetSuccessRateByIncidentType(ctx, "pod-oom-killer", "7d", 5)

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())

				// CORRECTNESS: Data Storage client was NOT called (cache hit)
				Expect(mockDSClient.GetIncidentTypeCallCount()).To(Equal(0))
			})
		})
	})

	// ========================================
	// BR-INTEGRATION-009: Playbook Success Rate
	// ========================================

	Describe("GetSuccessRateByPlaybook", func() {
		Context("when Data Storage returns playbook success rate", func() {
			BeforeEach(func() {
				mockDSClient.SetPlaybookResponse(&dsmodels.PlaybookSuccessRateResponse{
					PlaybookID:           "pod-oom-recovery",
					PlaybookVersion:      "v1.2",
					TimeRange:            "30d",
					TotalExecutions:      200,
					SuccessfulExecutions: 180,
					FailedExecutions:     20,
					SuccessRate:          90.0,
					Confidence:           "high",
					MinSamplesMet:        true,
				}, nil)
			})

			It("should return aggregated success rate for playbook", func() {
				// ACT
				result, err := aggregationService.GetSuccessRateByPlaybook(ctx, "pod-oom-recovery", "v1.2", "30d", 5)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(result.PlaybookVersion).To(Equal("v1.2"))
				Expect(result.SuccessRate).To(Equal(90.0))
				Expect(result.Confidence).To(Equal("high"))
			})

			It("should call Data Storage client with playbook parameters", func() {
				// ACT
				_, err := aggregationService.GetSuccessRateByPlaybook(ctx, "pod-oom-recovery", "v1.2", "30d", 5)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(mockDSClient.GetPlaybookCallCount()).To(Equal(1))

				// CORRECTNESS: Verify parameters
				playbookID, playbookVersion, timeRange, minSamples := mockDSClient.GetPlaybookLastCall()
				Expect(playbookID).To(Equal("pod-oom-recovery"))
				Expect(playbookVersion).To(Equal("v1.2"))
				Expect(timeRange).To(Equal("30d"))
				Expect(minSamples).To(Equal(5))
			})
		})

		Context("when playbook_id is empty", func() {
			It("should return validation error", func() {
				_, err := aggregationService.GetSuccessRateByPlaybook(ctx, "", "v1.2", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("playbook_id cannot be empty"))
			})
		})

		Context("when Data Storage client returns error", func() {
			BeforeEach(func() {
				mockDSClient.SetPlaybookResponse(nil, errors.New("playbook not found"))
			})

			It("should return the error", func() {
				_, err := aggregationService.GetSuccessRateByPlaybook(ctx, "unknown-playbook", "v1.0", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("playbook not found"))
			})
		})

		Context("when cache contains playbook data", func() {
			BeforeEach(func() {
				cacheKey := "playbook:pod-oom-recovery:v1.2:30d:5"
				cachedData := &dsmodels.PlaybookSuccessRateResponse{
					PlaybookID:           "pod-oom-recovery",
					PlaybookVersion:      "v1.2",
					TimeRange:            "30d",
					TotalExecutions:      200,
					SuccessfulExecutions: 180,
					FailedExecutions:     20,
					SuccessRate:          90.0,
					Confidence:           "high",
					MinSamplesMet:        true,
				}
				err := mockCache.Set(ctx, cacheKey, cachedData)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return cached data without calling Data Storage", func() {
				result, err := aggregationService.GetSuccessRateByPlaybook(ctx, "pod-oom-recovery", "v1.2", "30d", 5)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(mockDSClient.GetPlaybookCallCount()).To(Equal(0))
			})
		})
	})

	// ========================================
	// BR-INTEGRATION-010: Multi-Dimensional Success Rate
	// ========================================

	Describe("GetSuccessRateMultiDimensional", func() {
		Context("when Data Storage returns multi-dimensional data", func() {
			BeforeEach(func() {
				mockDSClient.SetMultiDimensionalResponse(&dsmodels.MultiDimensionalSuccessRateResponse{
					Dimensions: dsmodels.QueryDimensions{
						IncidentType:    "pod-oom-killer",
						PlaybookID:      "pod-oom-recovery",
						PlaybookVersion: "v1.2",
						ActionType:      "increase_memory",
					},
					TimeRange:            "7d",
					TotalExecutions:      50,
					SuccessfulExecutions: 45,
					FailedExecutions:     5,
					SuccessRate:          90.0,
					Confidence:           "medium",
					MinSamplesMet:        true,
				}, nil)
			})

			It("should return multi-dimensional success rate", func() {
				// ACT
				result, err := aggregationService.GetSuccessRateMultiDimensional(ctx, &datastorage.MultiDimensionalQuery{
					IncidentType:    "pod-oom-killer",
					PlaybookID:      "pod-oom-recovery",
					PlaybookVersion: "v1.2",
					ActionType:      "increase_memory",
					TimeRange:       "7d",
					MinSamples:      5,
				})

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Dimensions.IncidentType).To(Equal("pod-oom-killer"))
				Expect(result.Dimensions.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(result.Dimensions.ActionType).To(Equal("increase_memory"))
				Expect(result.SuccessRate).To(Equal(90.0))
			})

			It("should call Data Storage client with all dimensions", func() {
				// ACT
				query := &datastorage.MultiDimensionalQuery{
					IncidentType:    "pod-oom-killer",
					PlaybookID:      "pod-oom-recovery",
					PlaybookVersion: "v1.2",
					ActionType:      "increase_memory",
					TimeRange:       "7d",
					MinSamples:      5,
				}
				_, err := aggregationService.GetSuccessRateMultiDimensional(ctx, query)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(mockDSClient.GetMultiDimensionalCallCount()).To(Equal(1))

				// CORRECTNESS: Verify query parameters
				lastQuery := mockDSClient.GetMultiDimensionalLastCall()
				Expect(lastQuery.IncidentType).To(Equal("pod-oom-killer"))
				Expect(lastQuery.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(lastQuery.ActionType).To(Equal("increase_memory"))
			})
		})

		Context("when no dimensions are specified", func() {
			It("should return validation error", func() {
				_, err := aggregationService.GetSuccessRateMultiDimensional(ctx, &datastorage.MultiDimensionalQuery{
					TimeRange:  "7d",
					MinSamples: 5,
				})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("at least one dimension"))
			})
		})

		Context("when Data Storage client returns error", func() {
			BeforeEach(func() {
				mockDSClient.SetMultiDimensionalResponse(nil, errors.New("insufficient data"))
			})

			It("should return the error", func() {
				_, err := aggregationService.GetSuccessRateMultiDimensional(ctx, &datastorage.MultiDimensionalQuery{
					IncidentType: "rare-error",
					TimeRange:    "7d",
					MinSamples:   5,
				})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("insufficient data"))
			})
		})

		Context("when cache contains multi-dimensional data", func() {
			BeforeEach(func() {
				cacheKey := "multi:pod-oom-killer:pod-oom-recovery:v1.2:increase_memory:7d:5"
				cachedData := &dsmodels.MultiDimensionalSuccessRateResponse{
					Dimensions: dsmodels.QueryDimensions{
						IncidentType:    "pod-oom-killer",
						PlaybookID:      "pod-oom-recovery",
						PlaybookVersion: "v1.2",
						ActionType:      "increase_memory",
					},
					TimeRange:            "7d",
					TotalExecutions:      50,
					SuccessfulExecutions: 45,
					FailedExecutions:     5,
					SuccessRate:          90.0,
					Confidence:           "medium",
					MinSamplesMet:        true,
				}
				err := mockCache.Set(ctx, cacheKey, cachedData)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return cached data without calling Data Storage", func() {
				result, err := aggregationService.GetSuccessRateMultiDimensional(ctx, &datastorage.MultiDimensionalQuery{
					IncidentType:    "pod-oom-killer",
					PlaybookID:      "pod-oom-recovery",
					PlaybookVersion: "v1.2",
					ActionType:      "increase_memory",
					TimeRange:       "7d",
					MinSamples:      5,
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(mockDSClient.GetMultiDimensionalCallCount()).To(Equal(0))
			})
		})
	})
})

// ========================================
// Mock Data Storage Client
// ========================================

type MockDataStorageClient struct {
	// Incident Type
	incidentTypeResponse      *dsmodels.IncidentTypeSuccessRateResponse
	incidentTypeError         error
	incidentTypeCallCount     int
	incidentTypeLastIncident  string
	incidentTypeLastTimeRange string
	incidentTypeLastMinSamples int

	// Playbook
	playbookResponse           *dsmodels.PlaybookSuccessRateResponse
	playbookError              error
	playbookCallCount          int
	playbookLastID             string
	playbookLastVersion        string
	playbookLastTimeRange      string
	playbookLastMinSamples     int

	// Multi-Dimensional
	multiDimensionalResponse  *dsmodels.MultiDimensionalSuccessRateResponse
	multiDimensionalError     error
	multiDimensionalCallCount int
	multiDimensionalLastQuery *datastorage.MultiDimensionalQuery
}

func NewMockDataStorageClient() *MockDataStorageClient {
	return &MockDataStorageClient{}
}

// Incident Type methods
func (m *MockDataStorageClient) GetSuccessRateByIncidentType(ctx context.Context, incidentType, timeRange string, minSamples int) (*dsmodels.IncidentTypeSuccessRateResponse, error) {
	m.incidentTypeCallCount++
	m.incidentTypeLastIncident = incidentType
	m.incidentTypeLastTimeRange = timeRange
	m.incidentTypeLastMinSamples = minSamples
	return m.incidentTypeResponse, m.incidentTypeError
}

func (m *MockDataStorageClient) SetIncidentTypeResponse(response *dsmodels.IncidentTypeSuccessRateResponse, err error) {
	m.incidentTypeResponse = response
	m.incidentTypeError = err
}

func (m *MockDataStorageClient) GetIncidentTypeCallCount() int {
	return m.incidentTypeCallCount
}

func (m *MockDataStorageClient) GetIncidentTypeLastCall() (string, string, int) {
	return m.incidentTypeLastIncident, m.incidentTypeLastTimeRange, m.incidentTypeLastMinSamples
}

// Playbook methods
func (m *MockDataStorageClient) GetSuccessRateByPlaybook(ctx context.Context, playbookID, playbookVersion, timeRange string, minSamples int) (*dsmodels.PlaybookSuccessRateResponse, error) {
	m.playbookCallCount++
	m.playbookLastID = playbookID
	m.playbookLastVersion = playbookVersion
	m.playbookLastTimeRange = timeRange
	m.playbookLastMinSamples = minSamples
	return m.playbookResponse, m.playbookError
}

func (m *MockDataStorageClient) SetPlaybookResponse(response *dsmodels.PlaybookSuccessRateResponse, err error) {
	m.playbookResponse = response
	m.playbookError = err
}

func (m *MockDataStorageClient) GetPlaybookCallCount() int {
	return m.playbookCallCount
}

func (m *MockDataStorageClient) GetPlaybookLastCall() (string, string, string, int) {
	return m.playbookLastID, m.playbookLastVersion, m.playbookLastTimeRange, m.playbookLastMinSamples
}

// Multi-Dimensional methods
func (m *MockDataStorageClient) GetSuccessRateMultiDimensional(ctx context.Context, query *datastorage.MultiDimensionalQuery) (*dsmodels.MultiDimensionalSuccessRateResponse, error) {
	m.multiDimensionalCallCount++
	m.multiDimensionalLastQuery = query
	return m.multiDimensionalResponse, m.multiDimensionalError
}

func (m *MockDataStorageClient) SetMultiDimensionalResponse(response *dsmodels.MultiDimensionalSuccessRateResponse, err error) {
	m.multiDimensionalResponse = response
	m.multiDimensionalError = err
}

func (m *MockDataStorageClient) GetMultiDimensionalCallCount() int {
	return m.multiDimensionalCallCount
}

func (m *MockDataStorageClient) GetMultiDimensionalLastCall() *datastorage.MultiDimensionalQuery {
	return m.multiDimensionalLastQuery
}
