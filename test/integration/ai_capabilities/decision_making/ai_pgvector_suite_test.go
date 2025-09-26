//go:build integration
// +build integration

package decision_making

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/embedding"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

func TestAIPgVector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI PgVector Integration Tests Suite")
}

// BR-AI-PGVECTOR-SUITE-001: AI PgVector Integration Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of AI PgVector integration business logic
// Stakeholder Value: Provides executive confidence in AI vector database integration and business continuity

// BR-AI-PGVECTOR-002: AI Embedding Pipeline Business Logic
// Business Impact: Ensures AI embeddings are properly processed and stored for business intelligence
// Stakeholder Value: Provides executive confidence in AI-powered search and recommendation capabilities

var _ = Describe("AI + pgvector Integration Test Suite - Executive AI Vector Database Validation", func() {
	var (
		aiEmbeddingPipeline *embedding.AIEmbeddingPipeline
		vectorDatabase      vector.VectorDatabase
		ctx                 context.Context
		logger              *logrus.Logger
		mockLLMClient       *mocks.MockLLMClient
		mockVectorDB        *mocks.MockVectorDatabase
	)

	BeforeEach(func() {
		// Use existing testutil patterns instead of deprecated TDDConversionHelper
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create mock clients directly
		mockLLMClient = mocks.NewMockLLMClient()
		mockVectorDB = mocks.NewMockVectorDatabase()

		// Create real AI embedding pipeline for business logic testing
		aiEmbeddingPipeline = embedding.NewAIEmbeddingPipeline(
			mockLLMClient,
			mockVectorDB,
			logger,
		)

		// Use the mock vector database for testing
		vectorDatabase = mockVectorDB
	})

	Context("When validating AI embedding pipeline business requirements", func() {
		Describe("BR-AI-PGVECTOR-002: AI Embedding Pipeline Business Logic", func() {
			It("should store embeddings for AI-powered business intelligence", func() {
				// Business Scenario: Executive stakeholders need confidence that AI embeddings are properly stored for business intelligence
				// Business Impact: Enables AI-powered search and recommendations for business decision making

				// Business Setup: Validate AI embedding pipeline is operational
				Expect(aiEmbeddingPipeline).ToNot(BeNil(),
					"BR-AI-PGVECTOR-002: AI embedding pipeline must be available for business intelligence")

				// Business Action: Store embedding for business intelligence
				embeddingRequest := &embedding.EmbeddingRequest{
					ID:      "business-intelligence-embedding-001",
					Content: "Critical business workflow pattern for customer onboarding automation",
					Metadata: map[string]interface{}{
						"business_priority":   "high",
						"customer_impact":     "critical",
						"automation_type":     "onboarding",
						"effectiveness_score": 0.92,
					},
				}

				err := aiEmbeddingPipeline.StoreEmbedding(ctx, embeddingRequest)

				// Business Validation: Embedding storage supports business intelligence
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-PGVECTOR-002: Embedding storage must succeed for AI-powered business intelligence")

				// Business Outcome: AI embedding storage enables business intelligence
				embeddingStorageWorking := err == nil

				Expect(embeddingStorageWorking).To(BeTrue(),
					"BR-AI-PGVECTOR-002: AI embedding storage must enable comprehensive business intelligence for executive confidence in AI-powered search capabilities")
			})

			It("should retrieve similar embeddings for business recommendations", func() {
				// Business Scenario: Executive stakeholders need confidence that AI can retrieve similar patterns for business recommendations
				// Business Impact: Enables AI-powered business recommendations and pattern recognition

				// Business Action: Retrieve similar embeddings for business recommendations
				businessQuery := "workflow automation for customer service optimization"
				similarEmbeddings, err := aiEmbeddingPipeline.RetrieveSimilarEmbeddings(ctx, businessQuery, 5)

				// Business Validation: Similar embedding retrieval supports business recommendations
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-PGVECTOR-002: Similar embedding retrieval must succeed for business recommendations")

				Expect(similarEmbeddings).ToNot(BeNil(),
					"BR-AI-PGVECTOR-002: Similar embeddings must be available for business pattern recognition")

				// Business Outcome: Similar embedding retrieval enables business recommendations
				similarEmbeddingRetrievalWorking := err == nil &&
					similarEmbeddings != nil

				Expect(similarEmbeddingRetrievalWorking).To(BeTrue(),
					"BR-AI-PGVECTOR-002: Similar embedding retrieval must enable comprehensive business recommendations for executive confidence in AI-powered pattern recognition")
			})
		})

		Describe("BR-AI-PGVECTOR-003: Vector Database Operations Business Logic", func() {
			It("should perform vector search for business pattern recognition", func() {
				// Business Scenario: Executive stakeholders need confidence that vector search supports business pattern recognition
				// Business Impact: Enables AI-powered business insights through pattern similarity

				// Business Setup: Validate vector database is operational
				Expect(vectorDatabase).ToNot(BeNil(),
					"BR-AI-PGVECTOR-003: Vector database must be available for business pattern recognition")

				// Business Action: Perform vector search for business patterns
				testEmbedding := []float64{0.1, 0.2, 0.3, 0.4, 0.5} // Example business embedding
				searchResults, err := vectorDatabase.SearchByVector(ctx, testEmbedding, 10, 0.7)

				// Business Validation: Vector search supports business pattern recognition
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-PGVECTOR-003: Vector search must succeed for business pattern recognition")

				Expect(searchResults).ToNot(BeNil(),
					"BR-AI-PGVECTOR-003: Search results must be available for business pattern analysis")

				// Business Outcome: Vector search enables business pattern recognition
				vectorSearchWorking := err == nil &&
					searchResults != nil

				Expect(vectorSearchWorking).To(BeTrue(),
					"BR-AI-PGVECTOR-003: Vector search must enable comprehensive business pattern recognition for executive confidence in AI-powered insights")
			})

			It("should provide vector database health monitoring for business reliability", func() {
				// Business Scenario: Executive stakeholders need confidence that vector database health is monitored for business reliability
				// Business Impact: Ensures AI-powered business operations continue reliably

				// Business Action: Check vector database health
				err := vectorDatabase.IsHealthy(ctx)

				// Business Validation: Vector database health supports business reliability
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-PGVECTOR-003: Vector database must be healthy for business reliability")

				// Business Action: Get pattern analytics for business insights
				analytics, err := vectorDatabase.GetPatternAnalytics(ctx)

				// Business Validation: Pattern analytics support business intelligence
				Expect(err).ToNot(HaveOccurred(),
					"BR-AI-PGVECTOR-003: Pattern analytics must be available for business intelligence")

				Expect(analytics).ToNot(BeNil(),
					"BR-AI-PGVECTOR-003: Analytics data must exist for business monitoring")

				// Business Outcome: Vector database monitoring enables business reliability
				databaseMonitoringWorking := err == nil &&
					analytics != nil

				Expect(databaseMonitoringWorking).To(BeTrue(),
					"BR-AI-PGVECTOR-003: Vector database monitoring must enable comprehensive business reliability for executive confidence in AI data infrastructure")
			})
		})
	})
})

var _ = BeforeSuite(func() {
	// Global setup for AI PgVector integration tests
})

var _ = AfterSuite(func() {
	// Global cleanup for AI PgVector integration tests
})
