package storage

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// TestServerHelper provides reusable HTTP test server functionality
// Following project guideline: AVOID duplication and REUSE existing code
type TestServerHelper struct {
	embeddingFactory *EmbeddingDataFactory
	dimensions       *BusinessRequirementDimensions
}

// NewTestServerHelper creates a new test server helper
func NewTestServerHelper() *TestServerHelper {
	return &TestServerHelper{
		embeddingFactory: NewEmbeddingDataFactory(),
		dimensions:       NewBusinessRequirementDimensions(),
	}
}

// CreateOpenAITestServer creates a standardized OpenAI mock server
// Business Requirement: BR-VDB-001 - Provide consistent OpenAI API simulation
func (h *TestServerHelper) CreateOpenAITestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Parse request to handle both single and batch requests
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		input := req["input"]
		var embeddings [][]float64

		if inputArray, ok := input.([]interface{}); ok {
			// Batch request
			embeddings = h.embeddingFactory.CreateBatchEmbeddings(
				h.dimensions.OpenAI,
				len(inputArray),
				0.1, // Base seed
			)
		} else {
			// Single request
			embeddings = [][]float64{
				h.embeddingFactory.CreateDeterministicEmbedding(h.dimensions.OpenAI, 0.1),
			}
		}

		response := h.embeddingFactory.CreateOpenAIResponse(embeddings, "text-embedding-3-small")
		json.NewEncoder(w).Encode(response)
	}))
}

// CreateHuggingFaceTestServer creates a standardized HuggingFace mock server
// Business Requirement: BR-VDB-002 - Provide consistent HuggingFace API simulation
func (h *TestServerHelper) CreateHuggingFaceTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Parse request to handle both single and batch requests
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		input := req["inputs"]
		var embeddings [][]float64

		if inputArray, ok := input.([]interface{}); ok {
			// Batch request
			embeddings = h.embeddingFactory.CreateBatchEmbeddings(
				h.dimensions.HuggingFace,
				len(inputArray),
				0.2, // Different base seed from OpenAI
			)
		} else {
			// Single request
			embeddings = [][]float64{
				h.embeddingFactory.CreateDeterministicEmbedding(h.dimensions.HuggingFace, 0.2),
			}
		}

		response := h.embeddingFactory.CreateHuggingFaceResponse(embeddings)
		json.NewEncoder(w).Encode(response)
	}))
}

// CreateRateLimitedOpenAIServer creates an OpenAI server that simulates rate limiting
// Business Requirement: BR-VDB-001 - Test rate limiting and exponential backoff
func (h *TestServerHelper) CreateRateLimitedOpenAIServer(failCount int) *httptest.Server {
	requestCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount <= failCount {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_exceeded",
				},
			})
			return
		}

		// After failures, return success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		embedding := h.embeddingFactory.CreateDeterministicEmbedding(h.dimensions.OpenAI, 0.1)
		response := h.embeddingFactory.CreateOpenAIResponse([][]float64{embedding}, "text-embedding-3-small")
		json.NewEncoder(w).Encode(response)
	}))
}

// ServiceBuilder provides fluent interface for creating test services
// Following project guideline: Ensure functionality aligns with business requirements
type ServiceBuilder struct {
	testContext   *TestContext
	configFactory *ServiceConfigFactory
	apiKeys       *TestAPIKeys
}

// NewServiceBuilder creates a new service builder
func NewServiceBuilder(testContext *TestContext) *ServiceBuilder {
	return &ServiceBuilder{
		testContext:   testContext,
		configFactory: NewServiceConfigFactory(),
		apiKeys:       NewTestAPIKeys(),
	}
}

// BuildOpenAIService creates a fully configured OpenAI service for testing
// Business Requirement: BR-VDB-001 - Provide consistent OpenAI service setup
func (b *ServiceBuilder) BuildOpenAIService(serverURL string) *vector.OpenAIEmbeddingService {
	dimensions := NewBusinessRequirementDimensions()
	config := b.configFactory.CreateOpenAIConfig(serverURL, dimensions.OpenAI)

	return vector.NewOpenAIEmbeddingServiceWithConfig(
		b.apiKeys.OpenAI,
		b.testContext.MockCache,
		b.testContext.Logger,
		config,
	)
}

// BuildHuggingFaceService creates a fully configured HuggingFace service for testing
// Business Requirement: BR-VDB-002 - Provide consistent HuggingFace service setup
func (b *ServiceBuilder) BuildHuggingFaceService(serverURL string) *vector.HuggingFaceEmbeddingService {
	dimensions := NewBusinessRequirementDimensions()
	config := b.configFactory.CreateHuggingFaceConfig(serverURL, dimensions.HuggingFace)

	return vector.NewHuggingFaceEmbeddingServiceWithConfig(
		b.apiKeys.HuggingFace,
		b.testContext.MockCache,
		b.testContext.Logger,
		config,
	)
}

// TestSuite provides complete test suite setup and teardown
// Following project guideline: AVOID duplication and REUSE existing code
type TestSuite struct {
	Context      *TestContext
	ServerHelper *TestServerHelper
	Builder      *ServiceBuilder

	// Server references for cleanup
	OpenAIServer      *httptest.Server
	HuggingFaceServer *httptest.Server
}

// NewTestSuite creates a complete test suite setup
func NewTestSuite() *TestSuite {
	context := NewTestContext()
	serverHelper := NewTestServerHelper()
	builder := NewServiceBuilder(context)

	return &TestSuite{
		Context:      context,
		ServerHelper: serverHelper,
		Builder:      builder,
	}
}

// SetupServers creates and starts test servers
// Business Requirement: Support consistent server setup across tests
func (ts *TestSuite) SetupServers() {
	ts.OpenAIServer = ts.ServerHelper.CreateOpenAITestServer()
	ts.HuggingFaceServer = ts.ServerHelper.CreateHuggingFaceTestServer()
}

// TeardownServers cleans up test servers
// Following project guideline: Ensure proper cleanup
func (ts *TestSuite) TeardownServers() {
	if ts.OpenAIServer != nil {
		ts.OpenAIServer.Close()
		ts.OpenAIServer = nil
	}
	if ts.HuggingFaceServer != nil {
		ts.HuggingFaceServer.Close()
		ts.HuggingFaceServer = nil
	}
}

// GetOpenAIService returns a configured OpenAI service
func (ts *TestSuite) GetOpenAIService() *vector.OpenAIEmbeddingService {
	if ts.OpenAIServer == nil {
		panic("OpenAI server not setup. Call SetupServers() first.")
	}
	return ts.Builder.BuildOpenAIService(ts.OpenAIServer.URL)
}

// GetHuggingFaceService returns a configured HuggingFace service
func (ts *TestSuite) GetHuggingFaceService() *vector.HuggingFaceEmbeddingService {
	if ts.HuggingFaceServer == nil {
		panic("HuggingFace server not setup. Call SetupServers() first.")
	}
	return ts.Builder.BuildHuggingFaceService(ts.HuggingFaceServer.URL)
}
