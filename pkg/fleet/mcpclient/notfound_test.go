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

package mcpclient

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// UT-FLEET-MCP-NOTFOUND: asRemoteNotFound (issue #2349, foundational
// occurrence of #2344's fix).
var _ = Describe("asRemoteNotFound", func() {
	gvk := schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}

	It("UT-FLEET-MCP-NOTFOUND-001: recognizes the k8s-standard not-found message shape", func() {
		errText := fmt.Sprintf(`call remote-cluster__resources_get returned error: failed to get resource: jobs.batch %q not found`, "wfe-618ac7d3b8940927")

		err := asRemoteNotFound(errText, gvk, "wfe-618ac7d3b8940927")

		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("UT-FLEET-MCP-NOTFOUND-002: does not misclassify an unrelated error mentioning a different name", func() {
		errText := `call remote-cluster__resources_get returned error: connection refused`

		err := asRemoteNotFound(errText, gvk, "wfe-618ac7d3b8940927")

		Expect(err).ToNot(HaveOccurred())
	})

	It("UT-FLEET-MCP-NOTFOUND-003: does not misclassify a not-found for a different resource name", func() {
		errText := fmt.Sprintf(`failed to get resource: jobs.batch %q not found`, "some-other-job")

		err := asRemoteNotFound(errText, gvk, "wfe-618ac7d3b8940927")

		Expect(err).ToNot(HaveOccurred())
	})
})
