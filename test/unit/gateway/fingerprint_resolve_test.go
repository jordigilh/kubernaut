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

package gateway

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("ResolveFingerprint - Shared fingerprint resolution (Issue #228)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	resource := types.ResourceIdentifier{
		Namespace: "prod",
		Kind:      "Pod",
		Name:      "payment-api-789",
	}

	Describe("Nil resolver", func() {
		It("should return CalculateOwnerFingerprint(resource) when resolver is nil", func() {
			fingerprint := types.ResolveFingerprint(ctx, nil, resource)

			expected := types.CalculateOwnerFingerprint(resource)
			Expect(fingerprint).To(Equal(expected),
				"Nil resolver should produce resource-level fingerprint (no owner resolution)")
		})
	})

	Describe("Successful owner resolution", func() {
		It("should return owner-level fingerprint when resolver succeeds", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "Deployment", "payment-api", nil
				},
			}

			fingerprint := types.ResolveFingerprint(ctx, resolver, resource)

			expected := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "payment-api",
			})
			Expect(fingerprint).To(Equal(expected),
				"Successful resolution should produce owner-level fingerprint")
		})
	})

	Describe("Failed owner resolution", func() {
		It("should fall back to resource-level fingerprint when resolver returns error", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "", "", fmt.Errorf("RBAC: forbidden")
				},
			}

			fingerprint := types.ResolveFingerprint(ctx, resolver, resource)

			expected := types.CalculateOwnerFingerprint(resource)
			Expect(fingerprint).To(Equal(expected),
				"Failed resolution should fall back to resource-level fingerprint (reason excluded)")
		})
	})

	Describe("Partial owner resolution (empty fields)", func() {
		It("should fall back when resolver returns empty ownerKind", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "", "payment-api", nil
				},
			}

			fingerprint := types.ResolveFingerprint(ctx, resolver, resource)

			expected := types.CalculateOwnerFingerprint(resource)
			Expect(fingerprint).To(Equal(expected),
				"Empty ownerKind should trigger fallback to resource-level fingerprint")
		})
	})

	Describe("Cross-adapter consistency", func() {
		It("should produce identical fingerprints for same resource regardless of adapter origin", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "Deployment", "payment-api", nil
				},
			}

			fp1 := types.ResolveFingerprint(ctx, resolver, resource)
			fp2 := types.ResolveFingerprint(ctx, resolver, resource)

			Expect(fp1).To(Equal(fp2),
				"Same resource + resolver should always produce the same fingerprint (deterministic)")

			fpNilResolver := types.ResolveFingerprint(ctx, nil, resource)
			Expect(fp1).ToNot(Equal(fpNilResolver),
				"Owner-resolved fingerprint should differ from resource-level fingerprint "+
					"(Deployment:payment-api vs Pod:payment-api-789)")
		})
	})
})
