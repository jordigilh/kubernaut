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

package k8serrors_test

import (
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
)

func TestK8sErrors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8s Error Helpers Suite")
}

// SOC2 Round 2 M-6: Tests for centralized K8s error classification helpers

var _ = Describe("IsIndexerConflict", func() {
	DescribeTable("should correctly classify indexer conflict errors",
		func(err error, expected bool) {
			Expect(k8serrors.IsIndexerConflict(err)).To(Equal(expected))
		},
		Entry("matching error", errors.New("indexer conflict: field index already exists"), true),
		Entry("non-matching error", errors.New("failed to create field index"), false),
		Entry("nil error", nil, false),
	)
})

var _ = Describe("IsNamespaceTerminating", func() {
	DescribeTable("should correctly classify namespace terminating errors",
		func(err error, expected bool) {
			Expect(k8serrors.IsNamespaceTerminating(err)).To(Equal(expected))
		},
		Entry("matching error", errors.New("namespace \"test-ns\" is being terminated"), true),
		Entry("non-matching error", errors.New("failed to create resource"), false),
		Entry("nil error", nil, false),
		Entry("partial match - namespace only", errors.New("namespace not found"), false),
		Entry("partial match - terminated only", errors.New("resource is being terminated"), false),
	)
})
