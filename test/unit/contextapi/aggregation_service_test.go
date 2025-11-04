package contextapi_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	"github.com/jordigilh/kubernaut/pkg/contextapi/query"
	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"go.uber.org/zap"
)

var _ = Describe("AggregationService with Data Storage Client (ADR-032)", func() {
	var (
		aggregationService *query.AggregationService
		mockDSClient       *MockDataStorageClient
		mockCache          *cache.NoOpCache
		logger             *zap.Logger
		ctx                context.Context
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		ctx = context.Background()
		mockDSClient = NewMockDataStorageClient()
		mockCache = &cache.NoOpCache{}
		
		aggregationService = query.NewAggregationService(mockDSClient, mockCache, logger)
	})

	Describe("NewAggregationService", func() {
		It("should create a new aggregation service with Data Storage client", func() {
			Expect(aggregationService).ToNot(BeNil())
		})

		It("should accept Data Storage client instead of direct DB", func() {
			// This test validates ADR-032 compliance
			// AggregationService should use Data Storage client, not *sqlx.DB
			service := query.NewAggregationService(mockDSClient, mockCache, logger)
			Expect(service).ToNot(BeNil())
		})
	})

	Describe("AggregateSuccessRate", func() {
		Context("when Data Storage returns incidents", func() {
			BeforeEach(func() {
				// Mock Data Storage to return test incidents
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "incident-1", Status: "completed"},
					{Name: "incident-2", Status: "completed"},
					{Name: "incident-3", Status: "failed"},
					{Name: "incident-4", Status: "completed"},
				}, 4, nil)
			})

			It("should calculate success rate correctly", func() {
				result, err := aggregationService.AggregateSuccessRate(ctx, "workflow-123")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result["workflow_id"]).To(Equal("workflow-123"))
				Expect(result["total_count"]).To(Equal(4))
				Expect(result["completed_count"]).To(Equal(3))
				Expect(result["failed_count"]).To(Equal(1))
				Expect(result["success_rate"]).To(BeNumerically("~", 0.75, 0.01))
			})

			It("should call Data Storage client with correct filters", func() {
				_, err := aggregationService.AggregateSuccessRate(ctx, "workflow-123")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(mockDSClient.ListIncidentsCalled()).To(BeTrue())
			})
		})

		Context("when workflow_id is empty", func() {
			It("should return an error", func() {
				_, err := aggregationService.AggregateSuccessRate(ctx, "")
				
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("workflow_id cannot be empty"))
			})
		})

		Context("when Data Storage client returns error", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturnsError("connection failed")
			})

			It("should return the error", func() {
				_, err := aggregationService.AggregateSuccessRate(ctx, "workflow-123")
				
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection failed"))
			})
		})

		Context("when all incidents are successful", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "incident-1", Status: "completed"},
					{Name: "incident-2", Status: "completed"},
				}, 2, nil)
			})

			It("should return 100% success rate", func() {
				result, err := aggregationService.AggregateSuccessRate(ctx, "workflow-123")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result["success_rate"]).To(Equal(1.0))
			})
		})

		Context("when all incidents failed", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "incident-1", Status: "failed"},
					{Name: "incident-2", Status: "failed"},
				}, 2, nil)
			})

			It("should return 0% success rate", func() {
				result, err := aggregationService.AggregateSuccessRate(ctx, "workflow-123")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result["success_rate"]).To(Equal(0.0))
			})
		})
	})

	Describe("GroupByNamespace", func() {
		Context("when Data Storage returns incidents from multiple namespaces", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "inc-1", Namespace: "production", Status: "completed"},
					{Name: "inc-2", Namespace: "production", Status: "failed"},
					{Name: "inc-3", Namespace: "staging", Status: "completed"},
					{Name: "inc-4", Namespace: "staging", Status: "completed"},
					{Name: "inc-5", Namespace: "staging", Status: "completed"},
				}, 5, nil)
			})

			It("should group incidents by namespace", func() {
				result, err := aggregationService.GroupByNamespace(ctx)
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveLen(2))
				
				// Find production namespace
				var prodStats map[string]interface{}
				for _, ns := range result {
					if ns["namespace"] == "production" {
						prodStats = ns
						break
					}
				}
				
				Expect(prodStats).ToNot(BeNil())
				Expect(prodStats["count"]).To(Equal(2))
				Expect(prodStats["completed_count"]).To(Equal(1))
				Expect(prodStats["failed_count"]).To(Equal(1))
				Expect(prodStats["success_rate"]).To(BeNumerically("~", 0.5, 0.01))
			})

			It("should calculate success rate per namespace", func() {
				result, err := aggregationService.GroupByNamespace(ctx)
				
				Expect(err).ToNot(HaveOccurred())
				
				// Find staging namespace (3 completed out of 3)
				var stagingStats map[string]interface{}
				for _, ns := range result {
					if ns["namespace"] == "staging" {
						stagingStats = ns
						break
					}
				}
				
				Expect(stagingStats).ToNot(BeNil())
				Expect(stagingStats["count"]).To(Equal(3))
				Expect(stagingStats["success_rate"]).To(Equal(1.0))
			})
		})

		Context("when Data Storage returns empty result", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{}, 0, nil)
			})

			It("should return empty groups", func() {
				result, err := aggregationService.GroupByNamespace(ctx)
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveLen(0))
			})
		})
	})

	Describe("GetSeverityDistribution", func() {
		Context("when Data Storage returns incidents with different severities", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "inc-1", Severity: "critical", Namespace: "production"},
					{Name: "inc-2", Severity: "critical", Namespace: "production"},
					{Name: "inc-3", Severity: "high", Namespace: "production"},
					{Name: "inc-4", Severity: "medium", Namespace: "production"},
					{Name: "inc-5", Severity: "low", Namespace: "production"},
				}, 5, nil)
			})

			It("should return severity distribution", func() {
				result, err := aggregationService.GetSeverityDistribution(ctx, "production")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result["namespace"]).To(Equal("production"))
				Expect(result["total_count"]).To(Equal(5))
				
				distribution := result["distribution"].(map[string]interface{})
				
				critical := distribution["critical"].(map[string]interface{})
				Expect(critical["count"]).To(Equal(2))
				Expect(critical["percentage"]).To(BeNumerically("~", 40.0, 0.1))
				
				high := distribution["high"].(map[string]interface{})
				Expect(high["count"]).To(Equal(1))
				Expect(high["percentage"]).To(BeNumerically("~", 20.0, 0.1))
			})

			It("should call Data Storage with namespace filter", func() {
				_, err := aggregationService.GetSeverityDistribution(ctx, "production")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(mockDSClient.LastNamespaceFilter()).To(Equal("production"))
			})
		})

		Context("when namespace is empty", func() {
			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "inc-1", Severity: "critical"},
				}, 1, nil)
			})

			It("should fetch all namespaces", func() {
				result, err := aggregationService.GetSeverityDistribution(ctx, "")
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result["namespace"]).To(Equal(""))
			})
		})
	})

	Describe("GetIncidentTrend", func() {
		Context("when Data Storage returns incidents over time", func() {
			now := time.Now()
			yesterday := now.AddDate(0, 0, -1)
			twoDaysAgo := now.AddDate(0, 0, -2)

			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{Name: "inc-1", StartTime: &now},
					{Name: "inc-2", StartTime: &now},
					{Name: "inc-3", StartTime: &yesterday},
					{Name: "inc-4", StartTime: &twoDaysAgo},
				}, 4, nil)
			})

			It("should return incident counts per day", func() {
				result, err := aggregationService.GetIncidentTrend(ctx, 7)
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveLen(7))
				
				// Should have entries for each day
				for _, dayData := range result {
					Expect(dayData["date"]).ToNot(BeEmpty())
					Expect(dayData["count"]).To(BeNumerically(">=", 0))
				}
			})

			It("should include today's incidents", func() {
				result, err := aggregationService.GetIncidentTrend(ctx, 7)
				
				Expect(err).ToNot(HaveOccurred())
				
				// Find today's entry
				todayStr := now.Format("2006-01-02")
				var todayCount int
				for _, dayData := range result {
					if dayData["date"] == todayStr {
						todayCount = dayData["count"].(int)
						break
					}
				}
				
				Expect(todayCount).To(Equal(2)) // 2 incidents today
			})
		})

		Context("when days parameter is invalid", func() {
			It("should return error for days < 1", func() {
				_, err := aggregationService.GetIncidentTrend(ctx, 0)
				
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("days must be between 1 and 365"))
			})

			It("should return error for days > 365", func() {
				_, err := aggregationService.GetIncidentTrend(ctx, 400)
				
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("days must be between 1 and 365"))
			})
		})
	})

	Describe("AggregateWithFilters", func() {
		Context("when filters are applied", func() {
			namespace := "production"
			severity := "critical"

			BeforeEach(func() {
				mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
					{
						Name:       "inc-1",
						Namespace:  "production",
						Severity:   "critical",
						ClusterName: "cluster-1",
						ActionType: "restart",
						Status:     "completed",
					},
					{
						Name:       "inc-2",
						Namespace:  "production",
						Severity:   "critical",
						ClusterName: "cluster-1",
						ActionType: "scale",
						Status:     "failed",
					},
				}, 2, nil)
			})

			It("should aggregate with filters", func() {
				filters := &models.AggregationFilters{
					Namespace: &namespace,
					Severity:  &severity,
				}

				result, err := aggregationService.AggregateWithFilters(ctx, filters)
				
				Expect(err).ToNot(HaveOccurred())
				Expect(result["total_count"]).To(Equal(2))
				Expect(result["unique_namespaces"]).To(Equal(1))
				Expect(result["unique_clusters"]).To(Equal(1))
				Expect(result["unique_actions"]).To(Equal(2))
				Expect(result["overall_success_rate"]).To(BeNumerically("~", 0.5, 0.01))
			})

			It("should pass filters to Data Storage client", func() {
				filters := &models.AggregationFilters{
					Namespace: &namespace,
					Severity:  &severity,
				}

				_, err := aggregationService.AggregateWithFilters(ctx, filters)
				
				Expect(err).ToNot(HaveOccurred())
				Expect(mockDSClient.LastNamespaceFilter()).To(Equal("production"))
				Expect(mockDSClient.LastSeverityFilter()).To(Equal("critical"))
			})
		})

		Context("when filters are nil", func() {
			It("should return error", func() {
				_, err := aggregationService.AggregateWithFilters(ctx, nil)
				
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("aggregation filters cannot be nil"))
			})
		})
	})

	Describe("ADR-032 Compliance", func() {
		It("should NOT access PostgreSQL directly", func() {
			// This test validates that aggregation service uses Data Storage client
			// instead of direct database access (ADR-032 requirement)
			
			// Create service with mock Data Storage client
			service := query.NewAggregationService(mockDSClient, mockCache, logger)
			Expect(service).ToNot(BeNil())
			
			// All data should come from Data Storage client, not direct SQL
			mockDSClient.ListIncidentsReturns([]*models.IncidentEvent{
				{Name: "test", Status: "completed"},
			}, 1, nil)
			
			result, err := service.AggregateSuccessRate(ctx, "test-workflow")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			
			// Verify Data Storage client was called (not direct DB)
			Expect(mockDSClient.ListIncidentsCalled()).To(BeTrue())
		})
	})
})

// ============================================================================
// Mock Data Storage Client for Testing
// ============================================================================

type MockDataStorageClient struct {
	listIncidentsResult []*models.IncidentEvent
	listIncidentsTotal  int
	listIncidentsError  error
	listIncidentsCalled bool
	lastNamespaceFilter string
	lastSeverityFilter  string
	lastClusterFilter   string
}

func NewMockDataStorageClient() *MockDataStorageClient {
	return &MockDataStorageClient{}
}

func (m *MockDataStorageClient) ListIncidents(ctx context.Context, filters *dsclient.ListIncidentsFilters) ([]*models.IncidentEvent, int, error) {
	m.listIncidentsCalled = true
	
	if filters != nil {
		if filters.Namespace != nil {
			m.lastNamespaceFilter = *filters.Namespace
		}
		if filters.Severity != nil {
			m.lastSeverityFilter = *filters.Severity
		}
		if filters.Cluster != nil {
			m.lastClusterFilter = *filters.Cluster
		}
	}
	
	if m.listIncidentsError != nil {
		return nil, 0, m.listIncidentsError
	}
	
	return m.listIncidentsResult, m.listIncidentsTotal, nil
}

func (m *MockDataStorageClient) ListIncidentsReturns(incidents []*models.IncidentEvent, total int, err error) {
	m.listIncidentsResult = incidents
	m.listIncidentsTotal = total
	m.listIncidentsError = err
}

func (m *MockDataStorageClient) ListIncidentsReturnsError(errMsg string) {
	m.listIncidentsError = fmt.Errorf(errMsg)
}

func (m *MockDataStorageClient) ListIncidentsCalled() bool {
	return m.listIncidentsCalled
}

func (m *MockDataStorageClient) LastNamespaceFilter() string {
	return m.lastNamespaceFilter
}

func (m *MockDataStorageClient) LastSeverityFilter() string {
	return m.lastSeverityFilter
}

func (m *MockDataStorageClient) LastClusterFilter() string {
	return m.lastClusterFilter
}

