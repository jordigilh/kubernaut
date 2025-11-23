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

package gateway

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Redis Connection Debug", func() {
	var (
		ctx         context.Context
		redisClient *RedisTestClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		redisClient = SetupRedisTestClient(ctx)

		// Clean Redis state before each test to prevent OOM
		if redisClient != nil && redisClient.Client != nil {
			err := redisClient.Client.FlushDB(ctx).Err()
			Expect(err).ToNot(HaveOccurred(), "Failed to flush Redis before test")
		}
	})

	It("should connect to Redis successfully", func() {
		// Verify Redis client was created
		Expect(redisClient).ToNot(BeNil(), "Redis client should be created")
		Expect(redisClient.Client).ToNot(BeNil(), "Redis client connection should exist")

		// Test PING command
		result, err := redisClient.Client.Ping(ctx).Result()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Redis PING failed: %v", err))
		Expect(result).To(Equal("PONG"), "Redis should respond with PONG")

		GinkgoWriter.Printf("✅ Redis connection successful: %s\n", result)
	})

	It("should be able to set and get a test key", func() {
		Expect(redisClient).ToNot(BeNil())
		Expect(redisClient.Client).ToNot(BeNil())

		// Set a test key
		err := redisClient.Client.Set(ctx, "test:debug:key", "test-value", 0).Err()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to SET key: %v", err))

		// Get the test key
		value, err := redisClient.Client.Get(ctx, "test:debug:key").Result()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to GET key: %v", err))
		Expect(value).To(Equal("test-value"), "Value should match")

		// Clean up
		err = redisClient.Client.Del(ctx, "test:debug:key").Err()
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Println("✅ Redis SET/GET operations successful")
	})

	It("should verify Redis DB number", func() {
		Expect(redisClient).ToNot(BeNil())
		Expect(redisClient.Client).ToNot(BeNil())

		// Get Redis client options to verify DB number
		// Each parallel process uses a different DB: DB 2 + processID
		processID := GinkgoParallelProcess()
		expectedDB := 2 + processID
		opts := redisClient.Client.Options()
		Expect(opts.DB).To(Equal(expectedDB), fmt.Sprintf("Redis should be using DB %d for process %d", expectedDB, processID))

		GinkgoWriter.Printf("✅ Redis DB number confirmed: %d (process %d)\n", opts.DB, processID)
	})

	It("should verify Redis address", func() {
		Expect(redisClient).ToNot(BeNil())
		Expect(redisClient.Client).ToNot(BeNil())

		// Get Redis client options to verify address
		opts := redisClient.Client.Options()
		expectedAddr := fmt.Sprintf("localhost:%d", suiteRedisPort)
		Expect(opts.Addr).To(Equal(expectedAddr), fmt.Sprintf("Redis should be at %s", expectedAddr))

		GinkgoWriter.Printf("✅ Redis address confirmed: %s\n", opts.Addr)
	})
})
