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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("NewImpersonatingClient — #703", func() {

	baseCfg := func() *rest.Config {
		return &rest.Config{
			Host:        "https://kubernetes.default.svc",
			BearerToken: "sa-token-abc",
		}
	}

	Describe("UT-KA-703-E01: Creates rest.Config with correct ImpersonationConfig", func() {
		It("should set UserName and Groups on the impersonation config", func() {
			cfg := baseCfg()
			result, err := mcp.NewImpersonatingConfig(cfg, "user@company.com", []string{"engineering", "viewers"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Impersonate.UserName).To(Equal("user@company.com"))
			Expect(result.Impersonate.Groups).To(ConsistOf("engineering", "viewers"))
		})
	})

	Describe("UT-KA-703-E02: Rejects empty username", func() {
		It("should return error when username is empty", func() {
			cfg := baseCfg()
			_, err := mcp.NewImpersonatingConfig(cfg, "", []string{"engineering"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("username"))
		})
	})

	Describe("UT-KA-703-E03: Accepts empty groups (groups are optional)", func() {
		It("should succeed with nil groups", func() {
			cfg := baseCfg()
			result, err := mcp.NewImpersonatingConfig(cfg, "user@company.com", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Impersonate.UserName).To(Equal("user@company.com"))
			Expect(result.Impersonate.Groups).To(BeNil())
		})
	})

	Describe("UT-KA-703-E04: Original rest.Config is NOT mutated (deep copy)", func() {
		It("should preserve the original config unchanged", func() {
			cfg := baseCfg()
			originalHost := cfg.Host
			originalToken := cfg.BearerToken

			result, err := mcp.NewImpersonatingConfig(cfg, "user@company.com", []string{"admin"})
			Expect(err).NotTo(HaveOccurred())

			Expect(cfg.Impersonate.UserName).To(BeEmpty(), "original config must not be mutated")
			Expect(cfg.Host).To(Equal(originalHost))
			Expect(cfg.BearerToken).To(Equal(originalToken))
			Expect(result.Host).To(Equal(originalHost))
		})
	})

	Describe("UT-KA-703-E05: BearerToken is preserved for SA auth", func() {
		It("should keep the base config's BearerToken for SA authentication", func() {
			cfg := baseCfg()
			result, err := mcp.NewImpersonatingConfig(cfg, "user@company.com", []string{"viewers"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BearerToken).To(Equal("sa-token-abc"))
		})
	})
})
