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

package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	metadatafake "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

var _ = Describe("Issue #2521 production K8s tool wiring", func() {
	It("IT-KA-2521-005: registerK8sTools reaches the bounded list path", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "production-boundary", Namespace: "prod"},
			Data:       map[string]string{"payload": strings.Repeat("raw-bulk-body ", 100)},
		}
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		dynamicClient := fake.NewSimpleDynamicClient(scheme, configMap)
		metadataClient := metadatafake.NewSimpleMetadataClient(scheme, configMap)
		clientset, err := kubernetes.NewForConfig(&rest.Config{Host: "http://127.0.0.1"})
		Expect(err).NotTo(HaveOccurred())
		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
		mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
		infra := &k8sInfra{
			kubeConfig:     &rest.Config{Host: "http://127.0.0.1"},
			clientset:      clientset,
			dynClient:      dynamicClient,
			metadataClient: metadataClient,
			mapper:         mapper,
		}

		reg := registry.New()
		registerK8sTools(reg, infra, logr.Discard(), audit.NopAuditStore{})
		result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
			json.RawMessage(`{"kind":"ConfigMap","namespace":"prod"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("production-boundary"))
		Expect(result).NotTo(ContainSubstring("raw-bulk-body"))
		Expect(result).To(ContainSubstring(`"item_count":1`))
	})
})
