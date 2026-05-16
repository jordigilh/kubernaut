/*
Copyright 2026 Jordi Gil.

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

package datastorage_test

import (
	"context"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
)

// ========================================
// PHASE 7: REDIS READINESS CHECK — BEHAVIORAL (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 7.3
// File Under Test: pkg/datastorage/dlq/client.go (HealthCheck)
//
// These tests exercise dlq.Client.HealthCheck against real (miniredis)
// and stopped Redis instances, proving the production code path that
// the readiness handler depends on.
//
// ========================================

var _ = Describe("Phase 7: Redis Readiness Check (TP-1088-P1)", func() {

	Describe("dlq.Client.HealthCheck", func() {

		It("UT-DS-1088-P7-003a: HealthCheck returns nil when Redis is reachable", func() {
			mr := miniredis.RunT(GinkgoT())

			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer rc.Close()

			client, err := dlq.NewClient(rc, logr.Discard(), 1000)
			Expect(err).ToNot(HaveOccurred())

			err = client.HealthCheck(context.Background())
			Expect(err).ToNot(HaveOccurred(),
				"HealthCheck must return nil when Redis responds to PING")
		})

		It("UT-DS-1088-P7-003b: HealthCheck returns error when Redis is unreachable", func() {
			mr := miniredis.RunT(GinkgoT())
			addr := mr.Addr()
			mr.Close()

			rc := redis.NewClient(&redis.Options{Addr: addr})
			defer rc.Close()

			client, err := dlq.NewClient(rc, logr.Discard(), 1000)
			Expect(err).ToNot(HaveOccurred())

			err = client.HealthCheck(context.Background())
			Expect(err).To(HaveOccurred(),
				"HealthCheck must return error when Redis is down")
			Expect(err.Error()).To(ContainSubstring("redis ping failed"),
				"Error must wrap the ping failure with context")
		})
	})
})
