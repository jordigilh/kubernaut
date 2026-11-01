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

package creator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

var _ = Describe("AgentSession enrichment JSON identity", func() {
	It("UT-AA-KA-2583-004: treats equivalent escaped strings and numbers as identical", func() {
		persisted := &apiextensionsv1.JSON{Raw: []byte(`{"escaped":"\u0061","number":1}`)}
		expected := &apiextensionsv1.JSON{Raw: []byte(`{"escaped":"a","number":1.0}`)}

		Expect(equivalentJSON(persisted, expected)).To(BeTrue())
	})

	It("UT-AA-KA-2583-005: rejects malformed scalar JSON instead of treating it as equivalent", func() {
		persisted := &apiextensionsv1.JSON{Raw: []byte(`{"number":1e}`)}
		expected := &apiextensionsv1.JSON{Raw: []byte(`{"number":1}`)}

		Expect(equivalentJSON(persisted, expected)).To(BeFalse())
	})
})
