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

package datastorage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
)

var _ = Describe("UT-DS-1048-P5: Redis TLS Configuration", func() {
	Describe("UT-DS-1048-P5-060: RedisTLSConfig parsing", func() {
		It("should have all TLS fields accessible", func() {
			cfg := config.RedisTLSConfig{
				Enabled:            true,
				CertFile:           "/etc/redis/tls.crt",
				KeyFile:            "/etc/redis/tls.key",
				CAFile:             "/etc/redis/ca.crt",
				InsecureSkipVerify: false,
			}
			Expect(cfg.Enabled).To(BeTrue())
			Expect(cfg.CertFile).To(Equal("/etc/redis/tls.crt"))
			Expect(cfg.KeyFile).To(Equal("/etc/redis/tls.key"))
			Expect(cfg.CAFile).To(Equal("/etc/redis/ca.crt"))
			Expect(cfg.InsecureSkipVerify).To(BeFalse())
		})
	})

	Describe("UT-DS-1048-P5-061: TLS enabled without cert paths", func() {
		It("should fail validation when TLS enabled without caFile and insecureSkipVerify is false", func() {
			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
				Database: config.DatabaseConfig{
					Host: "localhost", Port: 5432, Name: "test", User: "test",
					StatementTimeout: "30s", LockTimeout: "10s",
				},
				Redis: config.RedisConfig{
					Addr: "localhost:6379",
					TLS: config.RedisTLSConfig{
						Enabled: true,
					},
				},
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis TLS enabled"))
		})
	})

	Describe("UT-DS-1048-P5-062: TLS disabled returns nil config", func() {
		It("should return nil TLS config when disabled", func() {
			cfg := config.RedisTLSConfig{Enabled: false}
			tlsCfg, err := cfg.BuildTLSConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsCfg).To(BeNil())
		})
	})

	Describe("UT-DS-1048-P5-064: InsecureSkipVerify propagated", func() {
		It("should propagate InsecureSkipVerify when TLS enabled", func() {
			cfg := config.RedisTLSConfig{
				Enabled:            true,
				InsecureSkipVerify: true,
			}
			tlsCfg, err := cfg.BuildTLSConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsCfg).NotTo(BeNil())
			Expect(tlsCfg.InsecureSkipVerify).To(BeTrue())
		})
	})

	Describe("UT-DS-1048-P5-065: TLS validation passes with valid config", func() {
		It("should pass validation when TLS enabled with caFile", func() {
			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
				Database: config.DatabaseConfig{
					Host: "localhost", Port: 5432, Name: "test", User: "test",
					StatementTimeout: "30s", LockTimeout: "10s",
				},
				Redis: config.RedisConfig{
					Addr: "localhost:6379",
					TLS: config.RedisTLSConfig{
						Enabled: true,
						CAFile:  "/etc/redis/ca.crt",
					},
				},
			}
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation when TLS enabled with insecureSkipVerify", func() {
			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
				Database: config.DatabaseConfig{
					Host: "localhost", Port: 5432, Name: "test", User: "test",
					StatementTimeout: "30s", LockTimeout: "10s",
				},
				Redis: config.RedisConfig{
					Addr: "localhost:6379",
					TLS: config.RedisTLSConfig{
						Enabled:            true,
						InsecureSkipVerify: true,
					},
				},
			}
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
