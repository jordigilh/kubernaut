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

package transport

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

var _ = Describe("K8s URL Parser — UT-KA-898-003, BR-INTERACTIVE-003", func() {

	DescribeTable("should parse K8s API URLs into resource/verb/namespace/name",
		func(method, path, expectedVerb, expectedResource, expectedNamespace, expectedName string) {
			info := transport.ParseK8sURL(method, path)
			Expect(info.Verb).To(Equal(expectedVerb), "verb mismatch")
			Expect(info.Resource).To(Equal(expectedResource), "resource mismatch")
			Expect(info.Namespace).To(Equal(expectedNamespace), "namespace mismatch")
			Expect(info.ResourceName).To(Equal(expectedName), "resource name mismatch")
		},
		Entry("core API: GET namespaced resource with name",
			"GET", "/api/v1/namespaces/default/pods/my-pod",
			"get", "pods", "default", "my-pod"),
		Entry("core API: POST creates resource (no name)",
			"POST", "/api/v1/namespaces/kube-system/services",
			"create", "services", "kube-system", ""),
		Entry("named API group: DELETE namespaced resource",
			"DELETE", "/apis/apps/v1/namespaces/prod/deployments/nginx",
			"delete", "deployments", "prod", "nginx"),
		Entry("CRD path: GET custom resource",
			"GET", "/apis/kubernaut.ai/v1alpha1/namespaces/ns1/remediationrequests/rr-1",
			"get", "remediationrequests", "ns1", "rr-1"),
		Entry("cluster-scoped: GET nodes (no namespace)",
			"GET", "/api/v1/nodes",
			"get", "nodes", "", ""),
		Entry("cluster-scoped: GET specific node",
			"GET", "/api/v1/nodes/worker-1",
			"get", "nodes", "", "worker-1"),
		Entry("subresource: GET pod log",
			"GET", "/api/v1/namespaces/default/pods/my-pod/log",
			"get", "pods/log", "default", "my-pod"),
		Entry("subresource: PATCH pod status",
			"PATCH", "/api/v1/namespaces/default/pods/my-pod/status",
			"patch", "pods/status", "default", "my-pod"),
		Entry("PUT updates resource",
			"PUT", "/api/v1/namespaces/default/configmaps/my-cm",
			"update", "configmaps", "default", "my-cm"),
	)
})
