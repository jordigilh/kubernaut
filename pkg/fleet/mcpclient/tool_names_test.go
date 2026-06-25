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

package mcpclient_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

var _ = Describe("UT-FLEET-TOOL-001 [SC-7]: ClusterTool constructs correct gateway-prefixed tool name for multi-cluster routing (BR-INTEGRATION-054)", func() {
	DescribeTable("constructs {clusterID}__{toolName} format",
		func(clusterID, tool, expected string) {
			Expect(mcpclient.ClusterTool(clusterID, tool)).To(Equal(expected))
		},
		Entry("standard cluster + get", "prod-east", "resources_get", "prod-east__resources_get"),
		Entry("standard cluster + list", "prod-west", "resources_list", "prod-west__resources_list"),
		Entry("standard cluster + create_or_update", "staging", "resources_create_or_update", "staging__resources_create_or_update"),
		Entry("standard cluster + delete", "dev-cluster", "resources_delete", "dev-cluster__resources_delete"),
		Entry("cluster with dots", "cluster.us-east-1.prod", "resources_get", "cluster.us-east-1.prod__resources_get"),
		Entry("cluster with underscores", "my_cluster_01", "resources_list", "my_cluster_01__resources_list"),
		Entry("empty clusterID", "", "resources_get", "__resources_get"),
	)
})
