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

package scopecache_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("FMC repair contract (BR-SCOPE-001, BR-SECURITY-016)", func() {
	It("UT-FMC-CACHE-009: validates a complete typed observation set before repair", func() {
		partition := scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-uid-1"}
		Expect(partition.Validate()).To(Succeed())

		observations := scopecache.ValidatedObservationSet{
			Items: []scopecache.RepairObservation{{
				Identity: scopecache.ResourceIdentity{
					ClusterID: "prod-east",
					Group:     "apps",
					Version:   "v1",
					Kind:      "Deployment",
					Namespace: "payments",
					Name:      "checkout",
				},
				APIVersion:    "apps/v1",
				LabelsPresent: true,
				LabelState:    scope.LabelStateTrue,
				TTL:           45 * time.Second,
			}},
			Complete:  true,
			PageCount: 1,
			ItemCount: 1,
		}
		Expect(observations.Validate()).To(Succeed())

		observations.ItemCount = 0
		Expect(observations.Validate()).To(HaveOccurred())
	})

	It("UT-FMC-CACHE-014: repair tokens are non-empty and remain proof-bearing", func() {
		token := scopecache.RepairToken{Value: "repair-token-1"}
		Expect(token.Validate()).To(Succeed())
		Expect(scopecache.RepairToken{}.Validate()).To(HaveOccurred())

		fence := scopecache.WriterFence{
			HolderPodUID:     "pod-uid-1",
			HolderSessionUID: "session-uid-1",
			LeaseEpoch:       3,
			Fence:            3,
		}
		partition := scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-uid-1"}
		start := scopecache.RepairStartResult{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 8,
			WriterFence:              fence,
			Generation:               4,
			Token:                    token,
		}
		request := scopecache.RepairRequest{
			Partition:                start.Partition,
			RegistrationUID:          start.RegistrationUID,
			ExpectedRegistryRevision: start.ExpectedRegistryRevision,
			WriterFence:              start.WriterFence,
			Generation:               start.Generation,
			Token:                    start.Token,
		}
		Expect(request.Token).To(Equal(start.Token))
		Expect(request.Token.Validate()).To(Succeed())
	})

	It("UT-FMC-CACHE-013: classifies an ambiguous repair failure without publishing clean", func() {
		failure := scopecache.RepairFailure{
			Code:      scopecache.RepairFailureAmbiguousWrite,
			Message:   "repair reply was lost",
			Retryable: false,
		}
		Expect(failure.Error()).To(ContainSubstring("ambiguous_write"))
		err := scopecache.RepairFailureError{Failure: failure}
		Expect(err.Error()).To(ContainSubstring("repair reply was lost"))
	})
})
