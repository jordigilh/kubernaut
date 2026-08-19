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

package alignment

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

// BR-INTEGRATION-1489, DD-FLEET-005: cluster-transparent tool exposure removes
// the "{clusterID}__tool" name prefix that parseClusterIDFromToolName relied
// on for shadow-agent audit attribution. attributionClusterID is the pure
// decision helper that fixes this by preferring the context's ClusterID
// (already carried by every fleet investigation via audit.WithClusterID),
// falling back to the legacy name-parsing convention only when the context
// carries none — never a regression for pre-DD-FLEET-005 callers.
var _ = Describe("SubmitToolStep cluster attribution (BR-INTEGRATION-1489, DD-FLEET-005)", Label("fleet", "unit"), func() {

	Describe("UT-KA-FLEET-019 [AU-3/CC8.1]: attributionClusterID decision logic", func() {
		It("prefers the context's ClusterID over the tool name prefix when both are present", func() {
			ctx := audit.WithClusterID(context.Background(), "context-cluster")

			got := attributionClusterID(ctx, "prefix-cluster__resources_get")
			Expect(got).To(Equal("context-cluster"),
				"UT-KA-FLEET-019: context ClusterID must win — it reflects the investigation's real target, "+
					"not an incidental name convention")
		})

		It("falls back to parsing the tool name prefix when the context carries no ClusterID", func() {
			got := attributionClusterID(context.Background(), "prefix-cluster__resources_get")
			Expect(got).To(Equal("prefix-cluster"),
				"UT-KA-FLEET-019: absent context ClusterID must fall back to the legacy name-parsing convention")
		})

		It("attributes correctly for a generically-named tool call once the context carries the ClusterID "+
			"(the actual regression DD-FLEET-005 introduces and this fixes)", func() {
			ctx := audit.WithClusterID(context.Background(), "prod-east")

			got := attributionClusterID(ctx, "kubectl_get_by_name")
			Expect(got).To(Equal("prod-east"),
				"UT-KA-FLEET-019: a generic tool name has no prefix to parse — only the context fix "+
					"attributes it correctly under full name transparency")
		})

		It("returns empty when neither the context nor the tool name carry a cluster identity", func() {
			got := attributionClusterID(context.Background(), "kubectl_get_by_name")
			Expect(got).To(BeEmpty())
		})
	})
})
