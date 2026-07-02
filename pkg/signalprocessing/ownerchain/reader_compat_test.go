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

package ownerchain_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
)

func TestOwnerchainReaderCompat(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OwnerChain Reader Compatibility Suite")
}

var _ = Describe("OwnerChain Builder client.Reader compatibility (BR-INTEGRATION-054)", func() {
	It("UT-SP-054-003 [AC-4]: accepts client.Reader for remote cluster support", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		logger := zap.New(zap.UseDevMode(true))

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

		// RED: This call passes client.Reader, not client.Client.
		// NewBuilder currently requires client.Client, so this will fail to compile.
		var reader client.Reader = fakeClient
		builder := ownerchain.NewBuilder(reader, logger)
		Expect(builder).ToNot(BeNil())

		chain, err := builder.Build(context.Background(), "default", "Pod", "test-pod")
		Expect(err).ToNot(HaveOccurred())
		Expect(chain).To(BeEmpty(), "pod with no owner references should have empty chain")
	})
})
