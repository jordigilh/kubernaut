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

package enricher_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
)

func TestReaderFactory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ReaderFactory Suite")
}

var _ = Describe("ReaderFactory (BR-INTEGRATION-054)", func() {
	var (
		ctx         context.Context
		localClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		localClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	Describe("LocalReaderFactory", func() {
		It("UT-SP-054-002a: returns local client for empty ClusterID", func() {
			// RED: NewLocalReaderFactory does not exist yet
			factory := enricher.NewLocalReaderFactory(localClient)
			reader, err := factory.ReaderFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient))
		})

		It("UT-SP-054-002b: returns local client for any ClusterID (local-only mode)", func() {
			factory := enricher.NewLocalReaderFactory(localClient)
			reader, err := factory.ReaderFor(ctx, "remote-cluster-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient),
				"local-only factory always returns local client regardless of clusterID")
		})
	})

	Describe("MCPReaderFactory", func() {
		It("UT-SP-054-002c: returns local client for empty ClusterID", func() {
			// RED: NewMCPReaderFactory does not exist yet
			factory := enricher.NewMCPReaderFactory(localClient, nil)
			reader, err := factory.ReaderFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(reader).To(Equal(localClient))
		})

		It("UT-SP-054-002d: returns error for remote ClusterID when session is nil", func() {
			factory := enricher.NewMCPReaderFactory(localClient, nil)
			_, err := factory.ReaderFor(ctx, "remote-cluster-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP session not available"))
		})
	})
})
