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

package mcp_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("SessionRateLimiter — PR4 SEC-06 BR-INTERACTIVE-006", func() {

	Describe("UT-KA-RATE-001: Rate limiter rejects messages exceeding max_messages_per_minute", func() {
		It("should reject the message with ErrRateLimited after limit is reached", func() {
			rl := mcpinternal.NewSessionRateLimiter(5, 65536)

			for i := 0; i < 5; i++ {
				err := rl.Allow("sess-rl-001", 100)
				Expect(err).NotTo(HaveOccurred())
			}

			err := rl.Allow("sess-rl-001", 100)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, mcpinternal.ErrRateLimited)).To(BeTrue())
		})
	})

	Describe("UT-KA-RATE-002: Rate limiter rejects oversized messages", func() {
		It("should reject messages exceeding max_message_size_bytes", func() {
			rl := mcpinternal.NewSessionRateLimiter(30, 1024)

			err := rl.Allow("sess-rl-002", 2048)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, mcpinternal.ErrRateLimited)).To(BeTrue())
		})
	})
})
