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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func TestRedisClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Redis Client Suite")
}

var _ = Describe("Redis Client", func() {
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
	})

	AfterEach(func() {
		if client != nil {
			_ = client.Close()
		}
		if miniRedis != nil {
			miniRedis.Close()
		}
	})

	Describe("NewClient", func() {
		It("should create a new Redis client without connecting", func() {
			opts := &redis.Options{
				Addr: redisAddr,
				DB:   0,
			}
			client = rediscache.NewClient(opts, logger)
			Expect(client).ToNot(BeNil())
			Expect(client.GetClient()).ToNot(BeNil())
		})
	})

	Describe("EnsureConnection", func() {
		Context("when Redis is available", func() {
			It("should establish connection on first call", func() {
				opts := &redis.Options{
					Addr: redisAddr,
					DB:   0,
				}
				client = rediscache.NewClient(opts, logger)

				err := client.EnsureConnection(ctx)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should use fast path on subsequent calls", func() {
				opts := &redis.Options{
					Addr: redisAddr,
					DB:   0,
				}
				client = rediscache.NewClient(opts, logger)

				// First call: slow path (establishes connection)
				err := client.EnsureConnection(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Second call: fast path (already connected)
				start := time.Now()
				err = client.EnsureConnection(ctx)
				duration := time.Since(start)
				Expect(err).ToNot(HaveOccurred())
				// Fast path should be < 1ms (atomic load only)
				Expect(duration).To(BeNumerically("<", 1*time.Millisecond))
			})
		})

		Context("when Redis is unavailable", func() {
			It("should return error without panicking", func() {
				opts := &redis.Options{
					Addr:        "localhost:9999", // Non-existent Redis
					DB:          0,
					DialTimeout: 100 * time.Millisecond,
				}
				client = rediscache.NewClient(opts, logger)

				err := client.EnsureConnection(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("redis unavailable"))
			})
		})

		Context("when called concurrently", func() {
			It("should prevent thundering herd with double-checked locking", func() {
				opts := &redis.Options{
					Addr: redisAddr,
					DB:   0,
				}
				client = rediscache.NewClient(opts, logger)

				// Simulate 10 concurrent goroutines trying to connect
				var wg sync.WaitGroup
				connectionAttempts := 10
				errors := make([]error, connectionAttempts)

				for i := 0; i < connectionAttempts; i++ {
					wg.Add(1)
					go func(index int) {
						defer wg.Done()
						errors[index] = client.EnsureConnection(ctx)
					}(i)
				}

				wg.Wait()

				// All goroutines should succeed (no race conditions)
				for i, err := range errors {
					Expect(err).ToNot(HaveOccurred(), "goroutine %d failed", i)
				}
			})
		})
	})

	Describe("GetClient", func() {
		It("should return underlying go-redis client", func() {
			opts := &redis.Options{
				Addr: redisAddr,
				DB:   0,
			}
			client = rediscache.NewClient(opts, logger)

			redisClient := client.GetClient()
			Expect(redisClient).ToNot(BeNil())

			// Verify we can use the client after EnsureConnection
			err := client.EnsureConnection(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Test direct Redis operation
			err = redisClient.Set(ctx, "test-key", "test-value", 0).Err()
			Expect(err).ToNot(HaveOccurred())

			val, err := redisClient.Get(ctx, "test-key").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal("test-value"))
		})
	})

	Describe("Close", func() {
		It("should close Redis connection successfully", func() {
			opts := &redis.Options{
				Addr: redisAddr,
				DB:   0,
			}
			client = rediscache.NewClient(opts, logger)

			err := client.EnsureConnection(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = client.Close()
			Expect(err).ToNot(HaveOccurred())

			// After close, connection should be marked as disconnected
			// (subsequent EnsureConnection would need to reconnect)
		})
	})

	Describe("Graceful Degradation", func() {
		It("should allow service to start when Redis unavailable", func() {
			opts := &redis.Options{
				Addr:        "localhost:9999", // Non-existent Redis
				DB:          0,
				DialTimeout: 100 * time.Millisecond,
			}
			client = rediscache.NewClient(opts, logger)

			// Service can create client even when Redis unavailable
			Expect(client).ToNot(BeNil())

			// Service discovers Redis unavailable on first operation
			err := client.EnsureConnection(ctx)
			Expect(err).To(HaveOccurred())

			// Service can proceed without cache (graceful degradation)
			// No panic, no crash
		})
	})
})
