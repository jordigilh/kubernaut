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
	"context"
	"errors"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// MockEmbeddingAPIClient for testing
type MockEmbeddingAPIClient struct {
	embedding []float32
	err       error
}

func (m *MockEmbeddingAPIClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embedding, nil
}

// MockCache for testing
type MockCache struct {
	data map[string][]float32
}

func NewMockCache() *MockCache {
	return &MockCache{
		data: make(map[string][]float32),
	}
}

func (m *MockCache) Get(ctx context.Context, key string) ([]float32, error) {
	if emb, ok := m.data[key]; ok {
		return emb, nil
	}
	return nil, errors.New("not found")
}

func (m *MockCache) Set(ctx context.Context, key string, embedding []float32, ttl time.Duration) error {
	m.data[key] = embedding
	return nil
}

var _ = Describe("BR-STORAGE-012: Embedding Generation", func() {
	var (
		pipeline  *embedding.Pipeline
		mockAPI   *MockEmbeddingAPIClient
		mockCache *MockCache
		logger    *zap.Logger
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockCache = NewMockCache()

		// Create 384-dimensional embedding
		testEmbedding := make([]float32, 384)
		for i := range testEmbedding {
			testEmbedding[i] = float32(i) * 0.01
		}

		mockAPI = &MockEmbeddingAPIClient{
			embedding: testEmbedding,
		}

		pipeline = embedding.NewPipeline(mockAPI, mockCache, logger)
	})

	// ⭐ TABLE-DRIVEN: Embedding generation test cases
	DescribeTable("should generate embeddings for different audit types",
		func(audit *models.RemediationAudit, expectedDimension int, shouldSucceed bool) {
			ctx := context.Background()

			result, err := pipeline.Generate(ctx, audit)

			if shouldSucceed {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.Embedding).To(HaveLen(expectedDimension))
				Expect(result.Dimension).To(Equal(expectedDimension))
			} else {
				Expect(err).To(HaveOccurred())
			}
		},

		Entry("BR-STORAGE-012.1: Normal audit with all fields",
			&models.RemediationAudit{
				Name:                 "test-remediation",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-123",
				AlertFingerprint:     "alert-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			384, true),

		Entry("BR-STORAGE-012.2: Audit with minimal fields",
			&models.RemediationAudit{
				Name:                 "minimal",
				Namespace:            "kube-system",
				Phase:                "processing",
				ActionType:           "restart_deployment",
				Status:               "pending",
				StartTime:            time.Now(),
				RemediationRequestID: "req-456",
				AlertFingerprint:     "alert-def",
				Severity:             "critical",
				Environment:          "staging",
				ClusterName:          "stage-cluster",
				TargetResource:       "pod/test",
				Metadata:             "{}",
			},
			384, true),

		Entry("BR-STORAGE-012.3: Audit with very long text",
			&models.RemediationAudit{
				Name:                 strings.Repeat("a", 255),
				Namespace:            strings.Repeat("n", 255),
				Phase:                "completed",
				ActionType:           strings.Repeat("action", 20),
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: strings.Repeat("req", 85),
				AlertFingerprint:     strings.Repeat("alert", 51),
				Severity:             "high",
				Environment:          "production",
				ClusterName:          strings.Repeat("cluster", 36),
				TargetResource:       strings.Repeat("deployment/", 46),
				Metadata:             "{}",
			},
			384, true),

		Entry("BR-STORAGE-012.4: Audit with special characters",
			&models.RemediationAudit{
				Name:                 "test-用户-τεστ",
				Namespace:            "special@#$%",
				Phase:                "failed",
				ActionType:           "scale_deployment",
				Status:               "failure",
				StartTime:            time.Now(),
				RemediationRequestID: "req-!@#$%",
				AlertFingerprint:     "alert-unicode-用户",
				Severity:             "low",
				Environment:          "dev",
				ClusterName:          "cluster-τεστ",
				TargetResource:       "deployment/app-用户",
				Metadata:             "{}",
			},
			384, true),

		Entry("BR-STORAGE-012.5: Audit with empty name",
			&models.RemediationAudit{
				Name:                 "",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-789",
				AlertFingerprint:     "alert-ghi",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/my-app",
				Metadata:             "{}",
			},
			384, true),
	)

	Context("cache behavior", func() {
		It("should cache embeddings and return cache hits", func() {
			ctx := context.Background()

			audit := &models.RemediationAudit{
				Name:                 "cached-test",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-cache-123",
				AlertFingerprint:     "alert-cache-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/cached-app",
				Metadata:             "{}",
			}

			By("generating embedding first time (cache miss)")
			result1, err := pipeline.Generate(ctx, audit)
			Expect(err).ToNot(HaveOccurred())
			Expect(result1.CacheHit).To(BeFalse(), "first call should be cache miss")

			By("generating embedding second time (cache hit)")
			result2, err := pipeline.Generate(ctx, audit)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2.CacheHit).To(BeTrue(), "second call should be cache hit")

			By("verifying embeddings are identical")
			Expect(result2.Embedding).To(Equal(result1.Embedding))
		})
	})

	Context("error handling", func() {
		It("should return error when audit is nil", func() {
			ctx := context.Background()

			result, err := pipeline.Generate(ctx, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audit is nil"))
			Expect(result).To(BeNil())
		})

		It("should return error when API fails", func() {
			ctx := context.Background()

			// Create pipeline with failing API
			failingAPI := &MockEmbeddingAPIClient{
				err: errors.New("API unavailable"),
			}
			failingPipeline := embedding.NewPipeline(failingAPI, mockCache, logger)

			audit := &models.RemediationAudit{
				Name:                 "fail-test",
				Namespace:            "default",
				Phase:                "pending",
				ActionType:           "scale_deployment",
				Status:               "success",
				StartTime:            time.Now(),
				RemediationRequestID: "req-fail-123",
				AlertFingerprint:     "alert-fail-abc",
				Severity:             "high",
				Environment:          "production",
				ClusterName:          "prod-cluster",
				TargetResource:       "deployment/fail-app",
				Metadata:             "{}",
			}

			result, err := failingPipeline.Generate(ctx, audit)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("API unavailable"))
			Expect(result).To(BeNil())
		})
	})
})
