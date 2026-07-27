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

package authwebhook_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
)

// PermanentError/IsPermanentError are generic error-classification helpers.
// Their only production caller (the pre-#1661 Change 8c startup-reconciler
// DS-retry loop) was removed when AW's RW sync stopped calling DS entirely,
// but the classifier itself remains a valid, independently-useful utility
// (also referenced by ds_client.go's now-unreachable-but-still-compiling
// real DS HTTP implementation) -- these two direct-unit tests moved here
// from the now-deleted startup_graceful_test.go (#1246) so coverage of the
// helper itself isn't lost.
var _ = Describe("UT-AW-1246-003/004: IsPermanentError classification", func() {
	It("should return true for PermanentError type", func() {
		permErr := authwebhook.NewPermanentError("action_type not found")
		Expect(authwebhook.IsPermanentError(permErr)).To(BeTrue())
	})

	It("should return false for generic/network errors", func() {
		transientErr := fmt.Errorf("connection refused")
		Expect(authwebhook.IsPermanentError(transientErr)).To(BeFalse())
	})
})
