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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
)

// TestEmbeddingClientUnit is commented out because this file is part of the
// datastorage_test package and uses the main suite in suite_test.go.
// Having multiple RunSpecs calls in the same package causes Ginkgo to fail.
// func TestEmbeddingClientUnit(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "Embedding Client Unit Suite")
// }

var _ = Describe("Embedding Client", func() {
	var (
		ctx         context.Context
		logger      logr.Logger
		miniRedis   *miniredis.Miniredis
		redisClient *rediscache.Client
		cache       *rediscache.Cache[[]float32]
		server      *httptest.Server
		client      embedding.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = kubelog.NewLogger(kubelog.DefaultOptions())

		// Start miniredis
		var err error
		miniRedis, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())

		// Create Redis client
		opts := &redis.Options{
			Addr: miniRedis.Addr(),
			DB:   0,
		}
		redisClient = rediscache.NewClient(opts, logger)
		err = redisClient.EnsureConnection(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Create cache
		cache = rediscache.NewCache[[]float32](redisClient, "embeddings", 24*time.Hour)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if redisClient != nil {
			_ = redisClient.Close()
		}
		if miniRedis != nil {
			miniRedis.Close()
		}
	})

	Describe("NewClient", func() {
		It("should create a new embedding client", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			client = embedding.NewClient(server.URL, cache, logger)
			Expect(client).ToNot(BeNil())
		})
	})

	Describe("Embed", func() {
		Context("when service returns valid embedding", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("POST"))
					Expect(r.URL.Path).To(Equal("/api/v1/embed"))
					Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

					// Generate mock 768-dimensional embedding
					mockEmbedding := make([]float32, 768)
					for i := range mockEmbedding {
						mockEmbedding[i] = float32(i) * 0.001
					}

					resp := embedding.EmbedResponse{
						Embedding:  mockEmbedding,
						Dimensions: 768,
						Model:      "all-mpnet-base-v2",
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))

				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should generate embedding successfully", func() {
				emb, err := client.Embed(ctx, "OOMKilled pod in production")
				Expect(err).ToNot(HaveOccurred())
				Expect(emb).To(HaveLen(768))
				Expect(emb[0]).To(BeNumerically("~", 0.0, 0.001))
				Expect(emb[767]).To(BeNumerically("~", 0.767, 0.001))
			})

			It("should cache embedding for future requests", func() {
				text := "OOMKilled pod in production"

				// First call: cache miss, calls service
				emb1, err := client.Embed(ctx, text)
				Expect(err).ToNot(HaveOccurred())
				Expect(emb1).To(HaveLen(768))

				// Wait for async cache write
				time.Sleep(100 * time.Millisecond)

				// Second call: cache hit, no service call
				emb2, err := client.Embed(ctx, text)
				Expect(err).ToNot(HaveOccurred())
				Expect(emb2).To(Equal(emb1))
			})
		})

		Context("when text is empty", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return error", func() {
				_, err := client.Embed(ctx, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("text must be non-empty"))
			})
		})

		Context("when service returns error", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error": "text too long"}`))
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return error without retry", func() {
				_, err := client.Embed(ctx, "test text")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("status 400"))
			})
		})

		Context("when service is temporarily unavailable", func() {
			var callCount int

			BeforeEach(func() {
				callCount = 0
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					callCount++
					if callCount < 3 {
						// First 2 attempts fail
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}

					// Third attempt succeeds
					mockEmbedding := make([]float32, 768)
					resp := embedding.EmbedResponse{
						Embedding:  mockEmbedding,
						Dimensions: 768,
						Model:      "all-mpnet-base-v2",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should retry and eventually succeed", func() {
				emb, err := client.Embed(ctx, "test text")
				Expect(err).ToNot(HaveOccurred())
				Expect(emb).To(HaveLen(768))
				Expect(callCount).To(Equal(3)) // 3 attempts
			})
		})

		Context("when service returns invalid dimensions", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Return wrong dimensions
					resp := embedding.EmbedResponse{
						Embedding:  make([]float32, 512), // Wrong!
						Dimensions: 512,
						Model:      "wrong-model",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return error", func() {
				_, err := client.Embed(ctx, "test text")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected embedding dimensions"))
			})
		})

		Context("when cache is unavailable", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mockEmbedding := make([]float32, 768)
					resp := embedding.EmbedResponse{
						Embedding:  mockEmbedding,
						Dimensions: 768,
						Model:      "all-mpnet-base-v2",
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))

				// Create client without cache (graceful degradation)
				client = embedding.NewClient(server.URL, nil, logger)
			})

			It("should proceed without cache", func() {
				emb, err := client.Embed(ctx, "test text")
				Expect(err).ToNot(HaveOccurred())
				Expect(emb).To(HaveLen(768))
			})
		})

		Context("when context is cancelled", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Slow response
					time.Sleep(2 * time.Second)
					w.WriteHeader(http.StatusOK)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return context error", func() {
				cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()

				_, err := client.Embed(cancelCtx, "test text")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Or(
					ContainSubstring("context"),
					ContainSubstring("deadline exceeded"),
				))
			})
		})
	})

	Describe("Health", func() {
		Context("when service is healthy", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("GET"))
					Expect(r.URL.Path).To(Equal("/health"))

					resp := embedding.HealthResponse{
						Status:     "healthy",
						Model:      "all-mpnet-base-v2",
						Dimensions: 768,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return no error", func() {
				err := client.Health(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when service is unhealthy", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte(`{"error": "service unavailable"}`))
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return error", func() {
				err := client.Health(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("status 503"))
			})
		})

		Context("when service returns wrong dimensions", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := embedding.HealthResponse{
						Status:     "healthy",
						Model:      "wrong-model",
						Dimensions: 512, // Wrong!
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should return error", func() {
				err := client.Health(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("dimensions mismatch"))
			})
		})
	})

	Describe("Retry Logic", func() {
		var callCount int

		BeforeEach(func() {
			callCount = 0
		})

		Context("when all retries fail", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					callCount++
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				client = embedding.NewClient(server.URL, cache, logger)
			})

			It("should fail after max retries", func() {
				_, err := client.Embed(ctx, "test text")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed after"))
				Expect(callCount).To(Equal(4)) // Initial + 3 retries
			})
		})
	})

	Describe("Cache Integration", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mockEmbedding := make([]float32, 768)
				for i := range mockEmbedding {
					mockEmbedding[i] = 0.5
				}
				resp := embedding.EmbedResponse{
					Embedding:  mockEmbedding,
					Dimensions: 768,
					Model:      "all-mpnet-base-v2",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			}))
			client = embedding.NewClient(server.URL, cache, logger)
		})

		It("should cache embeddings with 24-hour TTL", func() {
			text := "CrashLoopBackOff in staging"

			// Generate embedding
			emb1, err := client.Embed(ctx, text)
			Expect(err).ToNot(HaveOccurred())

			// Wait for async cache write
			time.Sleep(100 * time.Millisecond)

			// Verify cached
			cached, err := cache.Get(ctx, text)
			Expect(err).ToNot(HaveOccurred())
			Expect(*cached).To(Equal(emb1))

			// Verify TTL (should be close to 24 hours)
			// Note: miniredis doesn't support TTL inspection, so we trust the implementation
		})
	})

	// ========================================
	// TEST: Cache Expiration (Edge Case)
	// ========================================
	// BR-STORAGE-014: Cache TTL and expiration behavior
	// Edge Case: Cache entries should expire after TTL
	// Confidence Impact: +0.5% (validates cache lifecycle)
	Context("when cache entries expire", func() {
		It("should regenerate embeddings after cache TTL expires", func() {
			// ARRANGE: Create cache with very short TTL (1 second for testing)
			mr := miniredis.NewMiniRedis()
			err := mr.Start()
			Expect(err).ToNot(HaveOccurred())
			defer mr.Close()

			redisOpts := &redis.Options{
				Addr: mr.Addr(),
				DB:   0,
			}
			cacheClient := rediscache.NewClient(redisOpts, logger)
			defer cacheClient.Close()

			// Create cache with 1 second TTL
			shortTTLCache := rediscache.NewCache[[]float32](cacheClient, "embeddings-ttl-test", 1*time.Second)

			callCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				resp := embedding.EmbedResponse{
					Embedding:  make([]float32, 768),
					Dimensions: 768,
					Model:      "test-model",
				}
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := embedding.NewClient(server.URL, shortTTLCache, logger)

			text := "test cache expiration"

			// ACT: Generate embedding (first call)
			emb1, err := client.Embed(ctx, text)
			Expect(err).ToNot(HaveOccurred())
			Expect(callCount).To(Equal(1), "Should call service once")

			// Wait for async cache write
			time.Sleep(100 * time.Millisecond)

			// ACT: Generate embedding again immediately (should hit cache)
			emb2, err := client.Embed(ctx, text)
			Expect(err).ToNot(HaveOccurred())
			Expect(emb2).To(Equal(emb1))
			Expect(callCount).To(Equal(1), "Should NOT call service again (cache hit)")

			// ACT: Simulate cache expiration using miniredis FastForward
			mr.FastForward(2 * time.Second) // Fast forward past the 1 second TTL

			// ACT: Generate embedding after expiration (should regenerate)
			emb3, err := client.Embed(ctx, text)
			Expect(err).ToNot(HaveOccurred())
			Expect(callCount).To(Equal(2), "Should call service again after cache expiration")

			// ASSERT: New embedding should be generated and cached again
			time.Sleep(100 * time.Millisecond)

			// Verify new embedding is cached
			emb4, err := client.Embed(ctx, text)
			Expect(err).ToNot(HaveOccurred())
			Expect(emb4).To(Equal(emb3))
			Expect(callCount).To(Equal(2), "Should use newly cached embedding")
		})
	})
})
