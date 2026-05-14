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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
package signalprocessing

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/cache"
)

// Unit Test: TTLCache implementation correctness
// Per IMPLEMENTATION_PLAN_V1.21.md Day 3: Separate cache package
var _ = Describe("TTLCache", func() {

	Describe("NewTTLCache", func() {
		It("should create cache with specified TTL", func() {
			c := cache.NewTTLCache(5 * time.Minute)
			Expect(c).NotTo(BeNil())
		})
	})

	Describe("Get/Set operations", func() {
		var c *cache.TTLCache

		BeforeEach(func() {
			c = cache.NewTTLCache(1 * time.Hour) // Long TTL for tests
		})

		It("should return false for non-existent key", func() {
			_, ok := c.Get("nonexistent")
			Expect(ok).To(BeFalse())
		})

		It("should store and retrieve value", func() {
			c.Set("key1", "value1")

			val, ok := c.Get("key1")
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal("value1"))
		})

		It("should store different types", func() {
			c.Set("string", "hello")
			c.Set("int", 42)
			c.Set("struct", struct{ Name string }{"test"})

			val1, ok1 := c.Get("string")
			Expect(ok1).To(BeTrue())
			Expect(val1).To(Equal("hello"))

			val2, ok2 := c.Get("int")
			Expect(ok2).To(BeTrue())
			Expect(val2).To(Equal(42))

			val3, ok3 := c.Get("struct")
			Expect(ok3).To(BeTrue())
			Expect(val3.(struct{ Name string }).Name).To(Equal("test"))
		})

		It("should overwrite existing key", func() {
			c.Set("key", "original")
			c.Set("key", "updated")

			val, ok := c.Get("key")
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal("updated"))
		})
	})

	Describe("TTL expiration", func() {
		It("should expire entries after TTL", func() {
			c := cache.NewTTLCache(50 * time.Millisecond)

			c.Set("expiring", "value")

			// Should exist immediately
			val, ok := c.Get("expiring")
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal("value"))

			// Wait for expiration
			time.Sleep(100 * time.Millisecond)

			// Should be expired
			_, ok = c.Get("expiring")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("Delete operation", func() {
		It("should delete existing key", func() {
			c := cache.NewTTLCache(1 * time.Hour)

			c.Set("to-delete", "value")
			c.Delete("to-delete")

			_, ok := c.Get("to-delete")
			Expect(ok).To(BeFalse())
		})

		It("should not panic on deleting non-existent key", func() {
			c := cache.NewTTLCache(1 * time.Hour)

			Expect(func() {
				c.Delete("nonexistent")
			}).NotTo(Panic())
		})
	})

	Describe("Clear operation", func() {
		It("should clear all entries", func() {
			c := cache.NewTTLCache(1 * time.Hour)

			c.Set("key1", "value1")
			c.Set("key2", "value2")
			c.Set("key3", "value3")

			c.Clear()

			_, ok1 := c.Get("key1")
			_, ok2 := c.Get("key2")
			_, ok3 := c.Get("key3")

			Expect(ok1).To(BeFalse())
			Expect(ok2).To(BeFalse())
			Expect(ok3).To(BeFalse())
		})
	})

	// ========================================
	// PHASE 4 TDD RED: Issue #1110 SP Readiness Audit
	// Findings: CONC-C3 (bounded growth), CONC-C4 (concurrent safety)
	// BR-SP-001 (cache), DD-PERF-001
	// ========================================

	Describe("Issue #1110 Phase 4: Concurrency and Cache Bounds", func() {

		// CONC-C3 (Medium): TTLCache unbounded growth
		// Authority: BR-SP-001 (cache)
		Describe("CONC-C3: TTLCache bounded growth", func() {
			It("UT-SP-1110-043: 50+ Set/Delete cycles does not grow beyond max entries", func() {
				c := cache.NewTTLCache(1 * time.Hour)

				for i := 0; i < 60; i++ {
					c.Set(fmt.Sprintf("key-%d", i), fmt.Sprintf("value-%d", i))
				}

				Expect(c.Len()).To(BeNumerically("<=", cache.DefaultMaxEntries),
					"CONC-C3: TTLCache MUST NOT grow beyond DefaultMaxEntries")
			})

			It("UT-SP-1110-044: TTLCache evicts expired entries during Set when at capacity", func() {
				c := cache.NewTTLCache(10 * time.Millisecond)

				for i := 0; i < 20; i++ {
					c.Set(fmt.Sprintf("expired-key-%d", i), "value")
				}

				time.Sleep(20 * time.Millisecond)

				c.Set("fresh-key", "fresh-value")

				Expect(c.Len()).To(BeNumerically("<", 20),
					"CONC-C3: TTLCache Set MUST evict expired entries when at capacity")

				val, ok := c.Get("fresh-key")
				Expect(ok).To(BeTrue())
				Expect(val).To(Equal("fresh-value"))
			})
		})

		// CONC-C4 (Low): Concurrent safety verification
		// Authority: DD-PERF-001
		Describe("CONC-C4: TTLCache concurrent safety", func() {
			It("UT-SP-1110-045: Get/Set/Delete under 10+ concurrent goroutines produces no races", func() {
				c := cache.NewTTLCache(1 * time.Hour)
				done := make(chan struct{})

				for i := 0; i < 15; i++ {
					go func(id int) {
						defer GinkgoRecover()
						for j := 0; j < 100; j++ {
							key := fmt.Sprintf("key-%d-%d", id, j)
							c.Set(key, j)
							c.Get(key)
							if j%3 == 0 {
								c.Delete(key)
							}
						}
						done <- struct{}{}
					}(i)
				}

				for i := 0; i < 15; i++ {
					Eventually(done, 5*time.Second).Should(Receive())
				}

				Expect(c.Len()).To(BeNumerically(">=", 0),
					"CONC-C4: TTLCache MUST be safe under concurrent access (run with -race)")
			})
		})
	})
})
