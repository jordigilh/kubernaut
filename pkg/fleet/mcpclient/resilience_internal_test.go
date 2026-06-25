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

package mcpclient

import (
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UT-FLEET-RES-006 [AC-3]: Resilience layer correctly classifies retryable error patterns (BR-INTEGRATION-065)", func() {
	var rc *ResilientClient

	BeforeEach(func() {
		rc = &ResilientClient{
			logger: logr.Discard(),
		}
	})

	It("returns false for nil error", func() {
		Expect(rc.isRetryableError(nil)).To(BeFalse())
	})

	DescribeTable("isRetryableError classification",
		func(errMsg string, expected bool) {
			Expect(rc.isRetryableError(fmt.Errorf("%s", errMsg))).To(Equal(expected))
		},
		Entry("401 unauthorized is retryable", "HTTP 401 Unauthorized", true),
		Entry("session not found is retryable", "session not found for id abc123", true),
		Entry("connection refused is retryable", "dial tcp 127.0.0.1:1975: connection refused", true),
		Entry("EOF is retryable", "unexpected EOF", true),
		Entry("connection reset is retryable", "read: connection reset by peer", true),
		Entry("generic error is not retryable", "object not found: Pod/nginx", false),
		Entry("permission denied is not retryable", "forbidden: user lacks permission", false),
		Entry("validation error is not retryable", "admission webhook denied the request", false),
	)
})
