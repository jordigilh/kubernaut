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

package config_test

import (
	"github.com/jordigilh/kubernaut/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// DATASTORAGE HEALTHURL CONFIG UNIT TESTS
// BR-AUDIT-005 v2.0 | Issue #1985 | DD-PLATFORM-010
// Test ID: IT-AUDIT-1985-002
//
// DD-PLATFORM-010: DataStorage's cross-service readiness probe is an
// unauthenticated /readyz route on the SAME main API port as the
// audit-write API (DataStorageConfig.URL) -- HealthURL and URL differ only
// by path, never by port. A DataStorageProber still needs its own,
// distinct HealthURL to probe -- reusing URL bare would hit the
// auth-protected /api/v1 routes, not the unauthenticated /readyz path.
// ========================================

var _ = Describe("DataStorageConfig.HealthURL (IT-AUDIT-1985-002, BR-AUDIT-005, DD-PLATFORM-010)", func() {
	It("exposes a HealthURL field distinct from URL by path, sharing URL's port", func() {
		cfg := config.DataStorageConfig{
			URL:       "https://data-storage-service:8080",
			HealthURL: "https://data-storage-service:8080/readyz",
		}

		Expect(cfg.HealthURL).To(Equal("https://data-storage-service:8080/readyz"))
		Expect(cfg.HealthURL).NotTo(Equal(cfg.URL), "HealthURL must target the unauthenticated /readyz path, not the bare audit-write API URL")
	})

	It("defaults HealthURL to a usable /readyz path, distinct from the bare main API URL", func() {
		cfg := config.DefaultDataStorageConfig()

		Expect(cfg.HealthURL).NotTo(BeEmpty(), "a usable default HealthURL is required so services fail closed instead of silently skipping the readiness gate")
		Expect(cfg.HealthURL).NotTo(Equal(cfg.URL))
	})

	It("fails validation when HealthURL is empty", func() {
		cfg := config.DefaultDataStorageConfig()
		cfg.HealthURL = ""

		err := config.ValidateDataStorageConfig(&cfg)

		Expect(err).To(HaveOccurred(), "an empty HealthURL must fail config validation, not silently disable the #1985 readiness gate")
		Expect(err.Error()).To(ContainSubstring("healthUrl"))
	})
})
