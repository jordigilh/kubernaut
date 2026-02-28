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

package routing

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

// stubHistoryClient implements routing.HistoryContextClient for testing DSHistoryAdapter.
var _ routing.HistoryContextClient = (*stubHistoryClient)(nil)

type stubHistoryClient struct {
	capturedParams ogenclient.GetRemediationHistoryContextParams
	response       ogenclient.GetRemediationHistoryContextRes
	err            error
}

func (s *stubHistoryClient) GetRemediationHistoryContext(
	ctx context.Context,
	params ogenclient.GetRemediationHistoryContextParams,
) (ogenclient.GetRemediationHistoryContextRes, error) {
	s.capturedParams = params
	return s.response, s.err
}

var _ = Describe("DSHistoryAdapter (Issue #214)", func() {

	var (
		ctx    context.Context
		stub   *stubHistoryClient
		adapter *routing.DSHistoryAdapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		stub = &stubHistoryClient{}
		adapter = routing.NewDSHistoryAdapter(stub)
	})

	Describe("Parameter mapping", func() {
		It("should map TargetResource and window to ogen params correctly", func() {
			now := time.Now()
			stub.response = &ogenclient.RemediationHistoryContext{
				Tier1: ogenclient.RemediationHistoryTier1{
					Chain: []ogenclient.RemediationHistoryEntry{
						{RemediationUID: "uid-1", CompletedAt: now},
					},
				},
			}

			target := routing.TargetResource{
				Kind:      "Deployment",
				Name:      "payment-api",
				Namespace: "prod",
			}
			window := 4 * time.Hour

			_, err := adapter.GetRemediationHistory(ctx, target, "abc123hash", window)
			Expect(err).ToNot(HaveOccurred())

			Expect(stub.capturedParams.TargetKind).To(Equal("Deployment"))
			Expect(stub.capturedParams.TargetName).To(Equal("payment-api"))
			Expect(stub.capturedParams.TargetNamespace).To(Equal("prod"))
			Expect(stub.capturedParams.CurrentSpecHash).To(Equal("abc123hash"))
			Expect(stub.capturedParams.Tier1Window.IsSet()).To(BeTrue())
			Expect(stub.capturedParams.Tier1Window.Value).To(Equal("4h0m0s"))
		})
	})

	Describe("Tier1.Chain extraction", func() {
		It("should return Tier1.Chain entries from a successful response", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				{RemediationUID: "uid-1", CompletedAt: now.Add(-2 * time.Hour)},
				{RemediationUID: "uid-2", CompletedAt: now.Add(-1 * time.Hour)},
			}
			stub.response = &ogenclient.RemediationHistoryContext{
				Tier1: ogenclient.RemediationHistoryTier1{
					Chain: entries,
				},
			}

			target := routing.TargetResource{Kind: "Deployment", Name: "web", Namespace: "default"}
			result, err := adapter.GetRemediationHistory(ctx, target, "hash1", 24*time.Hour)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result[0].RemediationUID).To(Equal("uid-1"))
			Expect(result[1].RemediationUID).To(Equal("uid-2"))
		})

		It("should return empty slice when Tier1.Chain is empty", func() {
			stub.response = &ogenclient.RemediationHistoryContext{
				Tier1: ogenclient.RemediationHistoryTier1{
					Chain: []ogenclient.RemediationHistoryEntry{},
				},
			}

			target := routing.TargetResource{Kind: "StatefulSet", Name: "db", Namespace: "data"}
			result, err := adapter.GetRemediationHistory(ctx, target, "hash2", 24*time.Hour)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Describe("Error handling", func() {
		It("should propagate transport errors from the ogen client", func() {
			stub.err = fmt.Errorf("connection refused")

			target := routing.TargetResource{Kind: "Deployment", Name: "api", Namespace: "prod"}
			result, err := adapter.GetRemediationHistory(ctx, target, "hash3", 4*time.Hour)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(result).To(BeNil())
		})

		It("should return error for non-success responses (BadRequest)", func() {
			stub.response = &ogenclient.GetRemediationHistoryContextBadRequest{}

			target := routing.TargetResource{Kind: "Deployment", Name: "api", Namespace: "prod"}
			result, err := adapter.GetRemediationHistory(ctx, target, "hash4", 4*time.Hour)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected response type"))
			Expect(result).To(BeNil())
		})

		It("should return error for non-success responses (InternalServerError)", func() {
			stub.response = &ogenclient.GetRemediationHistoryContextInternalServerError{}

			target := routing.TargetResource{Kind: "Deployment", Name: "api", Namespace: "prod"}
			result, err := adapter.GetRemediationHistory(ctx, target, "hash5", 4*time.Hour)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected response type"))
			Expect(result).To(BeNil())
		})
	})
})
