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

package config_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DataStorage Config Unit Tests")
}

var _ = Describe("DataStorage Configuration", func() {
	Context("when loading production config file", func() {
		It("should correctly parse camelCase YAML keys for database pool settings", func() {
			// BUG FIX VERIFICATION: Ensure snake_case → camelCase migration succeeded
			// Bug discovered: 2026-01-18 - Config files used max_open_conns (snake_case)
			// which was silently ignored by yaml.Unmarshal (struct tags use camelCase)
			// This test verifies the fix is correct and the config loads properly

			configPath := "../../../../config/data-storage.yaml"

			// Verify file exists
			_, err := os.Stat(configPath)
			Expect(err).ToNot(HaveOccurred(), "Config file should exist at %s", configPath)

			// Load config
			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred(), "Config should load without errors")

			// ASSERT: Database pool settings are correctly loaded (NOT zero/default values)
			// If snake_case keys were still used, these would be 0 (zero value)
			Expect(cfg.Database.MaxOpenConns).To(Equal(25),
				"maxOpenConns should be 25 (from camelCase YAML key)")
			Expect(cfg.Database.MaxIdleConns).To(Equal(5),
				"maxIdleConns should be 5 (from camelCase YAML key)")
			Expect(cfg.Database.ConnMaxLifetime).To(Equal("5m"),
				"connMaxLifetime should be '5m' (from camelCase YAML key)")
			Expect(cfg.Database.ConnMaxIdleTime).To(Equal("10m"),
				"connMaxIdleTime should be '10m' (from camelCase YAML key)")

			// ASSERT: Other database settings also loaded correctly
			Expect(cfg.Database.SSLMode).To(Equal("disable"),
				"sslMode should be 'disable' (from camelCase YAML key)")

			// ASSERT: Server settings loaded correctly
			Expect(cfg.Server.Port).To(Equal(8080),
				"server port should be 8080")

			GinkgoWriter.Printf("✅ Config loaded successfully with correct pool settings:\n")
			GinkgoWriter.Printf("   maxOpenConns: %d\n", cfg.Database.MaxOpenConns)
			GinkgoWriter.Printf("   maxIdleConns: %d\n", cfg.Database.MaxIdleConns)
			GinkgoWriter.Printf("   connMaxLifetime: %s\n", cfg.Database.ConnMaxLifetime)
			GinkgoWriter.Printf("   connMaxIdleTime: %s\n", cfg.Database.ConnMaxIdleTime)
		})
	})

	Context("when loading integration testing config file", func() {
		It("should correctly parse camelCase YAML keys", func() {
			configPath := "../../../../config/integration-testing.yaml"

			// Load config
			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred(), "Integration testing config should load without errors")

			// ASSERT: Database pool settings are correctly loaded
			Expect(cfg.Database.MaxOpenConns).To(Equal(25),
				"integration-testing.yaml maxOpenConns should be 25")
			Expect(cfg.Database.MaxIdleConns).To(Equal(5),
				"integration-testing.yaml maxIdleConns should be 5")
			Expect(cfg.Database.ConnMaxLifetime).To(Equal("1h"),
				"integration-testing.yaml connMaxLifetime should be '1h'")
		})
	})

	Context("when config has invalid YAML keys", func() {
		It("should result in zero values for unmatched snake_case keys", func() {
			// This test documents the bug behavior (snake_case silently ignored)
			// Create a temporary config file with snake_case keys
			tmpFile, err := os.CreateTemp("", "bad-config-*.yaml")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(tmpFile.Name())

			badConfig := `
server:
  port: 8080
  host: "0.0.0.0"
logging:
  level: "info"
  format: "json"
database:
  host: "localhost"
  port: 5432
  name: "test_db"
  user: "test_user"
  max_open_conns: 25   # ❌ Wrong: snake_case (should be maxOpenConns)
  max_idle_conns: 5    # ❌ Wrong: snake_case (should be maxIdleConns)
redis:
  addr: "localhost:6379"
`
			_, err = tmpFile.WriteString(badConfig)
			Expect(err).ToNot(HaveOccurred())
			tmpFile.Close()

			// Load config with snake_case keys
			cfg, err := config.LoadFromFile(tmpFile.Name())
			Expect(err).ToNot(HaveOccurred(), "Config should load without YAML parse errors")

			// ASSERT: Pool settings are ZERO because snake_case keys don't match struct tags
			Expect(cfg.Database.MaxOpenConns).To(Equal(0),
				"snake_case 'max_open_conns' should be silently ignored (zero value)")
			Expect(cfg.Database.MaxIdleConns).To(Equal(0),
				"snake_case 'max_idle_conns' should be silently ignored (zero value)")

			GinkgoWriter.Printf("⚠️  Demonstrated bug: snake_case keys result in zero values\n")
			GinkgoWriter.Printf("   max_open_conns (snake_case) → MaxOpenConns = %d (WRONG!)\n",
				cfg.Database.MaxOpenConns)
		})
	})
})
