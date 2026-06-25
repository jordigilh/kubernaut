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

package fmc_test

import (
	"context"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
)

// UT-FLEET-VALKEY-WRITE: ValkeyWriter I/O tests
// Authority: BR-FLEET-002 (Fleet Metadata Caching)
// FedRAMP: SC-8 (Transmission Confidentiality and Integrity) -- cache layer writes
var _ = Describe("UT-FLEET-VALKEY-WRITE: ValkeyWriter", func() {
	var (
		ctx    context.Context
		mr     *miniredis.Miniredis
		writer *fmc.ValkeyWriter
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		mr, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		writer = fmc.NewValkeyWriter(mr.Addr())
	})

	AfterEach(func() {
		writer.Close()
		mr.Close()
	})

	Describe("Set", func() {
		It("UT-FLEET-VALKEY-WRITE-001: should write key with TTL", func() {
			err := writer.Set(ctx, "fleet:cluster-a:v1:Pod:default:nginx", 30*time.Second)
			Expect(err).ToNot(HaveOccurred())

			Expect(mr.Exists("fleet:cluster-a:v1:Pod:default:nginx")).To(BeTrue())
			val, _ := mr.Get("fleet:cluster-a:v1:Pod:default:nginx")
			Expect(val).To(Equal("1"))
		})
	})

	Describe("Close", func() {
		It("UT-FLEET-VALKEY-WRITE-002: should close without error", func() {
			err := writer.Close()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
