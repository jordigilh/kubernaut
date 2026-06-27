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

package scopecache_test

import (
	"context"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

// UT-FLEET-VALKEY-READ: ValkeyCacheReader I/O tests
// Authority: BR-FLEET-002 (Fleet Metadata Caching)
// FedRAMP: SC-8 (Transmission Confidentiality and Integrity) -- cache layer I/O
var _ = Describe("UT-FLEET-VALKEY-READ: ValkeyCacheReader", func() {
	var (
		ctx    context.Context
		mr     *miniredis.Miniredis
		reader *scopecache.ValkeyCacheReader
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		mr, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		reader = scopecache.NewValkeyCacheReader(mr.Addr())
	})

	AfterEach(func() {
		reader.Close()
		mr.Close()
	})

	Describe("Exists", func() {
		It("UT-FLEET-VALKEY-READ-001: should return true for existing key", func() {
			_ = mr.Set("fleet:cluster-a:v1:Pod:default:nginx", "1")

			exists, err := reader.Exists(ctx, "fleet:cluster-a:v1:Pod:default:nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("UT-FLEET-VALKEY-READ-002: should return false for non-existing key", func() {
			exists, err := reader.Exists(ctx, "fleet:nonexistent:v1:Pod:default:ghost")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Describe("Ping", func() {
		It("UT-FLEET-VALKEY-READ-003: should succeed when server is reachable", func() {
			err := reader.Ping(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-FLEET-VALKEY-READ-004: should fail when server is unreachable", func() {
			mr.Close()
			err := reader.Ping(ctx)
			Expect(err).To(HaveOccurred())
		})
	})
})
