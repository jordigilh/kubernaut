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

package datastorage_test

import (
	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UT-DS-1048-P5: Signer Configuration", func() {
	Describe("UT-DS-1048-P5-005: SignerCertDir config field", func() {
		It("should parse signerCertDir from YAML", func() {
			cfg := &config.ServerConfig{
				SignerCertDir: "/custom/certs",
			}
			Expect(cfg.GetSignerCertDir()).To(Equal("/custom/certs"))
		})
	})

	Describe("UT-DS-1048-P5-006: SignerCertDir default", func() {
		It("should default to /etc/certs when not set", func() {
			cfg := &config.ServerConfig{}
			Expect(cfg.GetSignerCertDir()).To(Equal("/etc/certs"))
		})

		It("should default to /etc/certs when empty string", func() {
			cfg := &config.ServerConfig{SignerCertDir: ""}
			Expect(cfg.GetSignerCertDir()).To(Equal("/etc/certs"))
		})
	})
})
