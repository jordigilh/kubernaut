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

package ownerchain

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// Issue #1674 (nilnil sentinel-error refactor), Batch 2: getControllerOwner is
// unexported; its "no controller owner" branch is already covered end-to-end
// through the public Build() API (pkg/signalprocessing/ownerchain_builder_test.go
// OC-EC-01/OC-EC-04), which continues to pass unchanged after this refactor.
// This file adds a direct, sentinel-level check on the unexported method.
// BR-SP-100.
// ========================================
var _ = Describe("getControllerOwner (Issue #1674 Batch 2)", func() {
	It("UT-SP-1674-001: returns ErrNoControllerOwner for a resource with no controller owner reference", func() {
		s := runtime.NewScheme()
		Expect(scheme.AddToScheme(s)).To(Succeed())
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-pod", Namespace: "default"},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(pod).Build()
		b := NewBuilder(fakeClient, kubelog.NewLogger(kubelog.DevelopmentOptions()))

		ref, err := b.getControllerOwner(context.Background(), "default", "Pod", "orphan-pod")

		Expect(errors.Is(err, ErrNoControllerOwner)).To(BeTrue())
		Expect(ref).To(BeNil())
	})
})
