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

package main

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("shutdownTimeout", func() {
	// UT-KA-1329-004: shutdownTimeout returns configured value (CM-3)
	It("returns the configured value", func() {
		cfg := kaconfig.DefaultConfig()
		cfg.Runtime.Shutdown.DrainSeconds = 3

		Expect(shutdownTimeout(cfg)).To(Equal(3 * time.Second))
	})

	// UT-KA-1329-005: shutdownTimeout returns 30s default on zero (CM-3)
	It("defaults to 30s when unset", func() {
		cfg := kaconfig.DefaultConfig()
		cfg.Runtime.Shutdown.DrainSeconds = 0

		Expect(shutdownTimeout(cfg)).To(Equal(30 * time.Second))
	})
})
