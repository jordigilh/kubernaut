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

package k8s_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

var _ = Describe("BR-SHARED-001: TruncateMapValues (issue 668)", func() {
	It("returns nil when input is nil (BR-SHARED-001)", func() {
		Expect(k8sutil.TruncateMapValues(nil, 10)).To(BeNil())
	})

	It("returns a new map without mutating the input (BR-SHARED-001)", func() {
		in := map[string]string{"k": "abcdefghij"}
		out := k8sutil.TruncateMapValues(in, 4)
		Expect(in["k"]).To(Equal("abcdefghij"))
		Expect(out["k"]).To(Equal("abcd"))
	})

	It("truncates values longer than maxLength and copies short values unchanged (BR-SHARED-001)", func() {
		in := map[string]string{
			"short": "ab",
			"long":  "0123456789",
		}
		out := k8sutil.TruncateMapValues(in, 5)
		Expect(out["short"]).To(Equal("ab"))
		Expect(out["long"]).To(Equal("01234"))
	})

	It("honours MaxLabelValueLength when used as maxLength (BR-SHARED-001)", func() {
		long := strings.Repeat("x", 80)
		out := k8sutil.TruncateMapValues(map[string]string{"v": long}, k8sutil.MaxLabelValueLength)
		Expect(len(out["v"])).To(Equal(k8sutil.MaxLabelValueLength))
	})
})
