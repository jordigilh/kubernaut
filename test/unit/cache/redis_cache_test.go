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

package cache

import (
	"context"
	"sync"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

var _ = Describe("Redis Cache", func() {
	var (
		ctx       context.Context
		logger    logr.Logger
		miniRedis *miniredis.Miniredis
		redisAddr string
		client    *rediscache.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = kubelog.NewLogger(kubelog.Options{Development: true, Level: 1})

		// Start miniredis
		var err error
		miniRedis, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		redisAddr = miniRedis.Addr()

		// Create Redis client
		opts := &redis.Options{
			Addr: redisAddr,
			DB:   0,
		}
		client = rediscache.NewClient(opts, logger)
		err = client.EnsureConnection(ctx)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if client != nil {
			_ = client.Close()
		}
		if miniRedis != nil {
			miniRedis.Close()
		}
	})

	Describe("NewCache", func() {
		It("should create a new type-safe cache", func() {
			cache := rediscache.NewCache[string](client, "test", 5*time.Minute)
			Expect(cache).ToNot(BeNil())
		})
	})

	Describe("Get and Set", func() {
		Context("with string type", func() {
			It("should store and retrieve string values", func() {
				cache := rediscache.NewCache[string](client, "strings", 5*time.Minute)

				// Set value
				testValue := "hello world"
				err := cache.Set(ctx, "key1", &testValue)
				Expect(err).ToNot(HaveOccurred())

				// Get value
				retrieved, err := cache.Get(ctx, "key1")
				Expect(err).ToNot(HaveOccurred())
				Expect(*retrieved).To(Equal("hello world"))
			})
		})

		Context("with []float32 type (embeddings)", func() {
			It("should store and retrieve embedding vectors", func() {
				cache := rediscache.NewCache[[]float32](client, "embeddings", 24*time.Hour)

				// Set embedding
				embedding := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
				err := cache.Set(ctx, "text content", &embedding)
				Expect(err).ToNot(HaveOccurred())

				// Get embedding
				retrieved, err := cache.Get(ctx, "text content")
				Expect(err).ToNot(HaveOccurred())
				Expect(*retrieved).To(HaveLen(5))
				Expect((*retrieved)[0]).To(BeNumerically("~", 0.1, 0.001))
				Expect((*retrieved)[4]).To(BeNumerically("~", 0.5, 0.001))
			})
		})

		Context("with struct type", func() {
			type TestStruct struct {
				Name  string
				Count int
				Tags  []string
			}

			It("should store and retrieve struct values", func() {
				cache := rediscache.NewCache[TestStruct](client, "structs", 10*time.Minute)

				// Set struct
				testData := TestStruct{
					Name:  "test",
					Count: 42,
					Tags:  []string{"tag1", "tag2"},
				}
				err := cache.Set(ctx, "struct-key", &testData)
				Expect(err).ToNot(HaveOccurred())

				// Get struct
				retrieved, err := cache.Get(ctx, "struct-key")
				Expect(err).ToNot(HaveOccurred())
				Expect(retrieved.Name).To(Equal("test"))
				Expect(retrieved.Count).To(Equal(42))
				Expect(retrieved.Tags).To(Equal([]string{"tag1", "tag2"}))
			})
		})
	})

	Describe("Cache Miss", func() {
		It("should return ErrCacheMiss for non-existent keys", func() {
			cache := rediscache.NewCache[string](client, "test", 5*time.Minute)

			retrieved, err := cache.Get(ctx, "non-existent-key")
			Expect(err).To(Equal(rediscache.ErrCacheMiss))
			Expect(retrieved).To(BeNil())
		})
	})

	Describe("TTL Expiration", func() {
		It("should expire cache entries after TTL", func() {
			// Create cache with 1-second TTL
			cache := rediscache.NewCache[string](client, "ttl-test", 1*time.Second)

			// Set value
			testValue := "expires soon"
			err := cache.Set(ctx, "ttl-key", &testValue)
			Expect(err).ToNot(HaveOccurred())

			// Verify value exists immediately
			retrieved, err := cache.Get(ctx, "ttl-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrieved).To(Equal("expires soon"))

			// Fast-forward time in miniredis
			miniRedis.FastForward(2 * time.Second)

			// Verify value expired
			retrieved, err = cache.Get(ctx, "ttl-key")
			Expect(err).To(Equal(rediscache.ErrCacheMiss))
			Expect(retrieved).To(BeNil())
		})
	})

	Describe("Key Hashing", func() {
		It("should generate deterministic hashes for same keys", func() {
			cache := rediscache.NewCache[string](client, "hash-test", 5*time.Minute)

			// Set value with key "test-key"
			testValue1 := "value1"
			err := cache.Set(ctx, "test-key", &testValue1)
			Expect(err).ToNot(HaveOccurred())

			// Retrieve with same key (should use same hash)
			retrieved, err := cache.Get(ctx, "test-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrieved).To(Equal("value1"))

			// Overwrite with same key
			testValue2 := "value2"
			err = cache.Set(ctx, "test-key", &testValue2)
			Expect(err).ToNot(HaveOccurred())

			// Verify overwrite worked (same hash)
			retrieved, err = cache.Get(ctx, "test-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrieved).To(Equal("value2"))
		})

		It("should isolate keys by prefix", func() {
			cache1 := rediscache.NewCache[string](client, "prefix1", 5*time.Minute)
			cache2 := rediscache.NewCache[string](client, "prefix2", 5*time.Minute)

			// Set same key in both caches
			value1 := "cache1-value"
			value2 := "cache2-value"
			err := cache1.Set(ctx, "shared-key", &value1)
			Expect(err).ToNot(HaveOccurred())
			err = cache2.Set(ctx, "shared-key", &value2)
			Expect(err).ToNot(HaveOccurred())

			// Verify values are isolated by prefix
			retrieved1, err := cache1.Get(ctx, "shared-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrieved1).To(Equal("cache1-value"))

			retrieved2, err := cache2.Get(ctx, "shared-key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrieved2).To(Equal("cache2-value"))
		})
	})

	Describe("Graceful Degradation", func() {
		Context("when Redis is unavailable", func() {
			It("should return error on Set without panicking", func() {
				// Create client with non-existent Redis
				opts := &redis.Options{
					Addr:        "localhost:9999",
					DB:          0,
					DialTimeout: 100 * time.Millisecond,
				}
				unavailableClient := rediscache.NewClient(opts, logger)
				defer func() { _ = unavailableClient.Close() }()

				cache := rediscache.NewCache[string](unavailableClient, "test", 5*time.Minute)

				testValue := "test"
				err := cache.Set(ctx, "key", &testValue)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("redis connection failed"))
			})

			It("should return error on Get without panicking", func() {
				// Create client with non-existent Redis
				opts := &redis.Options{
					Addr:        "localhost:9999",
					DB:          0,
					DialTimeout: 100 * time.Millisecond,
				}
				unavailableClient := rediscache.NewClient(opts, logger)
				defer func() { _ = unavailableClient.Close() }()

				cache := rediscache.NewCache[string](unavailableClient, "test", 5*time.Minute)

				retrieved, err := cache.Get(ctx, "key")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("redis connection failed"))
				Expect(retrieved).To(BeNil())
			})
		})
	})

	Describe("Concurrent Access", func() {
		It("should handle concurrent Get/Set operations safely", func() {
			cache := rediscache.NewCache[int](client, "concurrent", 5*time.Minute)

			// Concurrent writers
			var wg sync.WaitGroup
			concurrentOps := 10

			for i := 0; i < concurrentOps; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					value := index * 10
					err := cache.Set(ctx, "counter", &value)
					Expect(err).ToNot(HaveOccurred())
				}(i)
			}

			wg.Wait()

			// Verify final value is one of the written values
			retrieved, err := cache.Get(ctx, "counter")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrieved).To(BeNumerically(">=", 0))
			Expect(*retrieved).To(BeNumerically("<", concurrentOps*10))
		})
	})

	Describe("Type Safety", func() {
		It("should enforce type safety at compile time", func() {
			// This test verifies compile-time type safety
			// (if this compiles, type safety is working)

			stringCache := rediscache.NewCache[string](client, "strings", 5*time.Minute)
			intCache := rediscache.NewCache[int](client, "ints", 5*time.Minute)
			floatCache := rediscache.NewCache[[]float32](client, "floats", 5*time.Minute)

			// Set values
			stringVal := "test"
			intVal := 42
			floatVal := []float32{0.1, 0.2}

			err := stringCache.Set(ctx, "key", &stringVal)
			Expect(err).ToNot(HaveOccurred())
			err = intCache.Set(ctx, "key", &intVal)
			Expect(err).ToNot(HaveOccurred())
			err = floatCache.Set(ctx, "key", &floatVal)
			Expect(err).ToNot(HaveOccurred())

			// Get values (type-safe)
			retrievedString, err := stringCache.Get(ctx, "key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrievedString).To(Equal("test"))

			retrievedInt, err := intCache.Get(ctx, "key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrievedInt).To(Equal(42))

			retrievedFloat, err := floatCache.Get(ctx, "key")
			Expect(err).ToNot(HaveOccurred())
			Expect(*retrievedFloat).To(HaveLen(2))
		})
	})
})
