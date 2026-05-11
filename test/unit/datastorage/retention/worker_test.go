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

package retention_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/retention"
)

// BR-AUDIT-009: Retention policies for audit data
var _ = Describe("UT-DS-1048-P5: Retention Worker", func() {
	Describe("UT-DS-1048-P5-010: Worker disabled", func() {
		It("should not start when enabled is false", func() {
			w := retention.NewWorker(nil, retention.Config{
				Enabled: false,
			}, GinkgoLogr)
			w.Start(context.Background())
			w.Stop()
		})
	})

	Describe("UT-DS-1048-P5-012: Worker Stop", func() {
		It("should stop cleanly even if never started", func() {
			w := retention.NewWorker(nil, retention.Config{
				Enabled: false,
			}, GinkgoLogr)
			w.Stop()
		})
	})

	Describe("PurgeSQLBatched", func() {
		It("should contain LIMIT clause", func() {
			Expect(retention.PurgeSQLBatched).To(ContainSubstring("LIMIT"))
		})

		It("should filter legal_hold = FALSE", func() {
			Expect(retention.PurgeSQLBatched).To(ContainSubstring("legal_hold = FALSE"))
		})
	})
})
