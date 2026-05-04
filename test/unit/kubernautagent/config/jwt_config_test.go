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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("JWTProviderConfig Validation — #1009", func() {

	validBaseYAML := func(interactiveBlock string) []byte {
		return []byte(`
runtime:
  server:
    rateLimit:
      requestsPerSecond: 5
      burst: 10
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 40
` + interactiveBlock)
	}

	Describe("UT-KA-1009-025: Valid JWTProviderConfig with all required fields passes validation", func() {
		It("should pass validation with a complete jwtProviders configuration", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
      claimMappings:
        username: "preferred_username"
        groups: "groups"
  jwtInteractiveGroup: "kubernaut-interactive-users"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Interactive.JWTProviders).To(HaveLen(1))
			Expect(cfg.Interactive.JWTProviders[0].Name).To(Equal("keycloak"))
			Expect(cfg.Interactive.JWTProviders[0].Issuer).To(Equal("https://keycloak.example.com/realms/kubernaut"))
			Expect(cfg.Interactive.JWTProviders[0].JWKSURL).To(Equal("https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"))
			Expect(cfg.Interactive.JWTProviders[0].Audience).To(Equal("kubernaut-agent"))
			Expect(cfg.Interactive.JWTProviders[0].ClaimMappings.Username).To(Equal("preferred_username"))
			Expect(cfg.Interactive.JWTProviders[0].ClaimMappings.Groups).To(Equal("groups"))
			Expect(cfg.Interactive.JWTInteractiveGroup).To(Equal("kubernaut-interactive-users"))
		})
	})

	Describe("UT-KA-1009-026: JWTProviderConfig without issuer URL fails validation", func() {
		It("should return an error when issuer is empty", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("issuer"))
		})
	})

	Describe("UT-KA-1009-027: JWTProviderConfig without JWKS URL fails validation", func() {
		It("should return an error when jwksURL is empty", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("jwksURL"))
		})
	})

	Describe("UT-KA-1009-028: JWTProviderConfig without audience fails validation", func() {
		It("should return an error when audience is empty", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audience"))
		})
	})

	Describe("UT-KA-1009-029: ClaimMappings defaults applied when not specified", func() {
		It("should default username to 'preferred_username' and groups to 'groups'", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Interactive.JWTProviders[0].ClaimMappings.Username).To(Equal("preferred_username"))
			Expect(cfg.Interactive.JWTProviders[0].ClaimMappings.Groups).To(Equal("groups"))
		})
	})

	Describe("UT-KA-1009-030: InteractiveConfig validates all providers", func() {
		It("should validate each provider in the jwtProviders list", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "valid-provider"
      issuer: "https://issuer-1.example.com"
      jwksURL: "https://issuer-1.example.com/.well-known/jwks.json"
      audience: "kubernaut-agent"
    - name: "invalid-provider"
      issuer: ""
      jwksURL: "https://issuer-2.example.com/.well-known/jwks.json"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("issuer"))
			Expect(err.Error()).To(ContainSubstring("invalid-provider"))
		})
	})

	Describe("UT-KA-1009-031: Duplicate issuer URLs across providers rejected", func() {
		It("should return an error when two providers share the same issuer URL", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "provider-1"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
    - name: "provider-2"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate"))
			Expect(err.Error()).To(ContainSubstring("issuer"))
		})
	})

	Describe("UT-KA-1009-032: Empty jwtProviders list is valid (Pattern A only)", func() {
		It("should pass validation when no JWT providers are configured", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.JWTProviders).To(BeEmpty())
		})
	})

	Describe("UT-KA-1009-033: jwtProviders ignored when interactive.enabled=false", func() {
		It("should not validate jwtProviders when interactive mode is disabled", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: false
  jwtProviders:
    - name: "invalid"
      issuer: ""
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-1009-034: JWKS URL exceeding max length rejected", func() {
		It("should return an error when jwksURL exceeds 2048 characters", func() {
			longPath := ""
			for i := 0; i < 2100; i++ {
				longPath += "a"
			}
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "https://keycloak.example.com/` + longPath + `"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds maximum length"))
		})
	})

	Describe("UT-KA-1009-035: Syntactically invalid JWKS URL rejected", func() {
		It("should return an error when jwksURL is not a valid URL", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "://missing-scheme"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid jwksURL"))
		})
	})

	Describe("UT-KA-1009-036: JWKS URL with unsupported scheme rejected", func() {
		It("should return an error when jwksURL uses a non-HTTP(S) scheme", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/realms/kubernaut"
      jwksURL: "ftp://keycloak.example.com/certs"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scheme must be"))
		})
	})

	Describe("UT-KA-1009-037: JWKS URL with HTTP scheme accepted for dev/test", func() {
		It("should accept http:// JWKS URLs without error", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "dev-keycloak"
      issuer: "http://localhost:8080/realms/kubernaut"
      jwksURL: "http://localhost:8080/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-1009-038: Issuer URL exceeding max length rejected", func() {
		It("should return an error when issuer exceeds 2048 characters", func() {
			longPath := ""
			for i := 0; i < 2100; i++ {
				longPath += "a"
			}
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 5
  jwtProviders:
    - name: "keycloak"
      issuer: "https://keycloak.example.com/` + longPath + `"
      jwksURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/certs"
      audience: "kubernaut-agent"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds maximum length"))
		})
	})
})
