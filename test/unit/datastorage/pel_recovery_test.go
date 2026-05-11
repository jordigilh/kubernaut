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

package datastorage

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UT-DS-1048-P5: PEL Recovery (AU-2)", func() {
	Describe("UT-DS-1048-P5-050: Two-phase startup and PEL sweep contract", func() {
		It("should expose AU-2 PEL recovery tuning (BR-AUDIT-001)", func() {
			Expect(server.PelRecoveryClaimInterval).To(Equal(30 * time.Second))
			Expect(server.PelRecoveryMinIdleTime).To(Equal(60 * time.Second))
			Expect(server.PelRecoveryClaimCount).To(Equal(int64(10)))

			Expect(server.PelRecoveryClaimInterval > 0).To(BeTrue())
			Expect(server.PelRecoveryMinIdleTime > 0).To(BeTrue())
			Expect(server.PelRecoveryClaimCount).To(BeNumerically(">", 0))
		})
	})

	Describe("UT-DS-1048-P5-053: Poison message threshold", func() {
		It("should define PelRecoveryMaxDeliveries at 5 (BR-AUDIT-001 integration follows in IT tier)", func() {
			Expect(server.PelRecoveryMaxDeliveries).To(Equal(5))
		})
	})
})
