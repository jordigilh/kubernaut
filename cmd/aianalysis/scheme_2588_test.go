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

package main

import (
	"testing"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAIAnalysisScheme(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis production scheme wiring")
}

var _ = Describe("Cancellation parent watch startup (BR-ORCH-029)", func() {
	It("IT-AA-2588-010: registers parent RR objects and lists in the production scheme", func() {
		for _, kind := range []string{"RemediationRequest", "RemediationRequestList"} {
			gvk := remediationv1.GroupVersion.WithKind(kind)
			obj, err := scheme.New(gvk)
			Expect(err).NotTo(HaveOccurred(), "the parent watch must resolve %s before manager startup", kind)
			gvks, _, err := scheme.ObjectKinds(obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(gvks).To(ContainElement(gvk))
		}
	})
})
