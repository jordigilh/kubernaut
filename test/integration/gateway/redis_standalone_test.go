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
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// TestRedisConnectivity is a standalone test to verify Redis is accessible
// This test runs independently of the Ginkgo suite to avoid BeforeSuite delays
func TestRedisConnectivity(t *testing.T) {
	ctx := context.Background()

	// Test 1: Connect to localhost:6379
	t.Run("Connect to localhost:6379", func(t *testing.T) {
		client := goredis.NewClient(&goredis.Options{
			Addr:         "localhost:6379",
			Password:     "",
			DB:           2,
			PoolSize:     20,
			MinIdleConns: 5,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
		defer client.Close()

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		result, err := client.Ping(pingCtx).Result()
		if err != nil {
			t.Fatalf("❌ Redis PING failed: %v", err)
		}

		if result != "PONG" {
			t.Fatalf("❌ Expected PONG, got: %s", result)
		}

		t.Logf("✅ Redis connection successful: %s", result)
		t.Logf("✅ Redis address: %s", client.Options().Addr)
		t.Logf("✅ Redis DB: %d", client.Options().DB)
	})

	// Test 2: SET/GET operations
	t.Run("SET/GET operations", func(t *testing.T) {
		client := goredis.NewClient(&goredis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       2,
		})
		defer client.Close()

		// Set a test key
		err := client.Set(ctx, "test:standalone:key", "test-value", 10*time.Second).Err()
		if err != nil {
			t.Fatalf("❌ Failed to SET key: %v", err)
		}

		// Get the test key
		value, err := client.Get(ctx, "test:standalone:key").Result()
		if err != nil {
			t.Fatalf("❌ Failed to GET key: %v", err)
		}

		if value != "test-value" {
			t.Fatalf("❌ Expected 'test-value', got: %s", value)
		}

		// Clean up
		err = client.Del(ctx, "test:standalone:key").Err()
		if err != nil {
			t.Logf("⚠️  Failed to clean up test key: %v", err)
		}

		t.Log("✅ Redis SET/GET operations successful")
	})

	// Test 3: Verify deduplication service can connect
	t.Run("Deduplication service Redis connection", func(t *testing.T) {
		client := goredis.NewClient(&goredis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       2,
		})
		defer client.Close()

		// Test the exact key pattern used by deduplication service
		testKey := "gateway:dedup:fingerprint:test-fingerprint-123"
		err := client.Set(ctx, testKey, "test-data", 5*time.Minute).Err()
		if err != nil {
			t.Fatalf("❌ Failed to SET dedup key: %v", err)
		}

		// Verify we can read it back
		exists, err := client.Exists(ctx, testKey).Result()
		if err != nil {
			t.Fatalf("❌ Failed to check key existence: %v", err)
		}

		if exists != 1 {
			t.Fatalf("❌ Key should exist, got: %d", exists)
		}

		// Clean up
		err = client.Del(ctx, testKey).Err()
		if err != nil {
			t.Logf("⚠️  Failed to clean up dedup key: %v", err)
		}

		t.Log("✅ Deduplication service Redis pattern works")
	})

	// Test 4: Verify storm detection service can connect
	t.Run("Storm detection service Redis connection", func(t *testing.T) {
		client := goredis.NewClient(&goredis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       2,
		})
		defer client.Close()

		// Test the exact key pattern used by storm detection service
		testKey := "gateway:storm:production"
		err := client.Set(ctx, testKey, "10", 1*time.Minute).Err()
		if err != nil {
			t.Fatalf("❌ Failed to SET storm key: %v", err)
		}

		// Verify we can read it back
		value, err := client.Get(ctx, testKey).Result()
		if err != nil {
			t.Fatalf("❌ Failed to GET storm key: %v", err)
		}

		if value != "10" {
			t.Fatalf("❌ Expected '10', got: %s", value)
		}

		// Clean up
		err = client.Del(ctx, testKey).Err()
		if err != nil {
			t.Logf("⚠️  Failed to clean up storm key: %v", err)
		}

		t.Log("✅ Storm detection service Redis pattern works")
	})
}


