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

package infrastructure

import (
	"encoding/base64"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Monitoring TLS cache validation", func() {
	It("UT-INFRA-TLS-003 [BR-E2E-001]: rejects a host CA that differs from the Secret", func() {
		caPEM := []byte("-----BEGIN CERTIFICATE-----\ncurrent-ca\n-----END CERTIFICATE-----\n")
		encoded := base64.StdEncoding.EncodeToString(caPEM)

		Expect(monitoringCABytesMatch(caPEM, encoded)).To(BeTrue())
		Expect(monitoringCABytesMatch([]byte("stale-ca"), encoded)).To(BeFalse())
		Expect(monitoringCABytesMatch(caPEM, "not-base64")).To(BeFalse())
	})
})
