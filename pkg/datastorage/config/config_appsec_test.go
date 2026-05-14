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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("ServerConfig AppSec fields (#1048 Phase 4)", func() {

	Describe("GetMaxBodySize", func() {
		It("UT-DS-1048-CF-001: should return 5 MiB default when empty", func() {
			cfg := config.ServerConfig{}
			Expect(cfg.GetMaxBodySize()).To(BeNumerically("==", 5*1024*1024))
		})

		It("UT-DS-1048-CF-002: should parse integer value as bytes", func() {
			cfg := config.ServerConfig{MaxBodySize: "10485760"} // 10 MiB
			Expect(cfg.GetMaxBodySize()).To(BeNumerically("==", 10485760))
		})

		It("UT-DS-1048-CF-003: should clamp below minimum to 1 MiB", func() {
			cfg := config.ServerConfig{MaxBodySize: "100"}
			Expect(cfg.GetMaxBodySize()).To(BeNumerically("==", 1*1024*1024))
		})

		It("UT-DS-1048-CF-004: should clamp above maximum to 50 MiB", func() {
			cfg := config.ServerConfig{MaxBodySize: "999999999"}
			Expect(cfg.GetMaxBodySize()).To(BeNumerically("==", 50*1024*1024))
		})

		It("UT-DS-1048-CF-005: should return default for unparseable value", func() {
			cfg := config.ServerConfig{MaxBodySize: "invalid"}
			Expect(cfg.GetMaxBodySize()).To(BeNumerically("==", 5*1024*1024))
		})
	})

	Describe("GetCORSAllowedOrigins", func() {
		It("UT-DS-1048-CF-006: should return empty when not configured (deny-all default)", func() {
			cfg := config.ServerConfig{}
			Expect(cfg.GetCORSAllowedOrigins()).To(BeEmpty())
		})

		It("UT-DS-1048-CF-007: should return configured origins", func() {
			cfg := config.ServerConfig{CORSAllowedOrigins: []string{"https://app.example.com", "https://admin.example.com"}}
			origins := cfg.GetCORSAllowedOrigins()
			Expect(origins).To(HaveLen(2))
			Expect(origins).To(ContainElement("https://app.example.com"))
		})
	})

	Describe("Backward compatibility with removed fields", func() {
		It("UT-DS-1048-CF-008: should parse YAML with removed fields without error", func() {
			yamlData := `
server:
  port: 8080
  host: "0.0.0.0"
logging:
  level: info
  format: json
database:
  host: localhost
  port: 5432
  name: testdb
  user: test
  sslMode: disable
  secretsFile: /tmp/db-secret.yaml
  passwordKey: password
redis:
  addr: localhost:6379
  db: 3
  dlqStreamName: "my-stream"
  dlqConsumerGroup: "my-group"
  secretsFile: /tmp/redis-secret.yaml
  passwordKey: password
`
			var cfg config.Config
			err := yaml.Unmarshal([]byte(yamlData), &cfg)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Server.Port).To(Equal(8080))
			Expect(cfg.Redis.Addr).To(Equal("localhost:6379"))
		})
	})
})
