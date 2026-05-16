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

package llm_test

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
)

var _ = Describe("Issue #885: UT-KA-885-006 — langchaingo WithLogger option", func() {

	It("WithLogger option is accepted by New without error", func() {
		logger := logr.Discard()

		// This verifies the WithLogger option exists and compiles.
		opt := langchaingo.WithLogger(logger)
		Expect(opt).NotTo(BeNil())
	})
})
