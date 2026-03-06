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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("ResolveFingerprint - Shared fingerprint resolution (Issue #228)", func() {
	var ctx context.Context
	testLogger := logr.Discard()

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
			fingerprint, err := types.ResolveFingerprint(ctx, nil, resource, testLogger)
			Expect(err).ToNot(HaveOccurred())

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

			fingerprint, err := types.ResolveFingerprint(ctx, resolver, resource, testLogger)
			Expect(err).ToNot(HaveOccurred())

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
		It("should return error when resolver fails (signal must be dropped)", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "", "", fmt.Errorf("RBAC: forbidden")
				},
			}

			fingerprint, err := types.ResolveFingerprint(ctx, resolver, resource, testLogger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("owner resolution failed"))
			Expect(fingerprint).To(BeEmpty(),
				"Failed resolution must return empty fingerprint so signal is dropped")
		})
	})

	Describe("Partial owner resolution (empty fields)", func() {
		It("should return error when resolver returns empty ownerKind", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "", "payment-api", nil
				},
			}

			fingerprint, err := types.ResolveFingerprint(ctx, resolver, resource, testLogger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("owner resolution returned empty"))
			Expect(fingerprint).To(BeEmpty(),
				"Empty ownerKind must return empty fingerprint so signal is dropped")
		})
	})

	Describe("Cross-adapter consistency", func() {
		It("should produce identical fingerprints for same resource regardless of adapter origin", func() {
			resolver := &mockOwnerResolver{
				resolveFunc: func(ctx context.Context, namespace, kind, name string) (string, string, error) {
					return "Deployment", "payment-api", nil
				},
			}

			fp1, err := types.ResolveFingerprint(ctx, resolver, resource, testLogger)
			Expect(err).ToNot(HaveOccurred())
			fp2, err := types.ResolveFingerprint(ctx, resolver, resource, testLogger)
			Expect(err).ToNot(HaveOccurred())

			Expect(fp1).To(Equal(fp2),
				"Same resource + resolver should always produce the same fingerprint (deterministic)")

			fpNilResolver, err := types.ResolveFingerprint(ctx, nil, resource, testLogger)
			Expect(err).ToNot(HaveOccurred())
			Expect(fp1).ToNot(Equal(fpNilResolver),
				"Owner-resolved fingerprint should differ from resource-level fingerprint "+
					"(Deployment:payment-api vs Pod:payment-api-789)")
		})
	})
})
