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

package infrastructure

import (
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

var _ = Describe("localDataStorageImageName", func() {
	It("UT-INFRA-DS-001: returns the local Podman-qualified image reference", func() {
		Expect(localDataStorageImageName("aianalysis-18d7afe3")).To(Equal("localhost/kubernaut/datastorage:aianalysis-18d7afe3"))
	})
})

var _ = Describe("prebuiltCIArtifactImageName", func() {
	It("UT-INFRA-DS-002: matches the DataStorage artifact reference used by CI", func() {
		Expect(prebuiltCIArtifactImageName("datastorage", "aianalysis-18d7afe3")).To(Equal("localhost/datastorage:aianalysis-18d7afe3"),
			"BR-FLEET-054: Fleet-enabled AF setup must reuse the CI-loaded DataStorage image")
	})
})

var _ = Describe("DS E2E burst capacity [BR-DS-006]", Serial, func() {
	DescribeTable("emitted fixtures retain memory and requests while allowing the canonical CPU burst",
		func(expectedCPU string, deploy func() error) {
			manifest := captureKubectlManifest(deploy)
			var deployments int
			for _, document := range strings.Split(manifest, "\n---\n") {
				var deployment appsv1.Deployment
				Expect(yaml.Unmarshal([]byte(document), &deployment)).To(Succeed())
				if deployment.Kind != deploymentKind {
					continue
				}
				deployments++
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				resources := deployment.Spec.Template.Spec.Containers[0].Resources
				Expect(resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse(expectedCPU)))
				Expect(resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("1Gi")))
				Expect(resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("250m")))
				Expect(resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("512Mi")))
			}
			Expect(deployments).To(Equal(1), "capture the actual Deployment emitted to kubectl")
		},
		Entry("UT-INFRA-DS-2496-CPU-001: PostgreSQL can absorb concurrent partition planning", "4", func() error {
			return deployPostgreSQLInNamespace(context.Background(), "burst-fixture", "fixture-kubeconfig", io.Discard)
		}),
		Entry("UT-INFRA-DS-2496-CPU-002: DS can absorb concurrent authenticated writes", "2", func() error {
			return deployDataStorageServiceInNamespace(context.Background(), "burst-fixture", "fixture-kubeconfig", "localhost/datastorage:fixture", io.Discard)
		}),
	)
})
