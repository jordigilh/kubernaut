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

package infrastructure

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// DD-TESTING-003: E2E must-gather runs the production image as a local
// podman container joined to the cluster's "kind" podman network -- these
// tests cover the pure, exec-free pieces of that mechanism (arg-building,
// kubeconfig-output cleanup) per this package's convention of separating
// exec.Command wrappers from unit-testable logic (see checkKindVersionOutput
// vs. validateKindVersion in kind_cluster_helpers_test.go).
var _ = Describe("buildMustGatherPodmanArgs", func() {
	It("UT-INFRA-MUSTGATHER-001: joins the kind network and mounts kubeconfig + output read-write", func() {
		args := buildMustGatherPodmanArgs(RunMustGatherImageOptions{
			ClusterName: "gateway-e2e",
			Image:       "localhost/must-gather:e2e",
			OutputDir:   "/tmp/mg-output",
		}, "/tmp/mg-kubeconfig-internal.yaml")

		Expect(args).To(ContainElement("--network"))
		Expect(args).To(ContainElement(DefaultMustGatherKindNetwork))
		Expect(args).To(ContainElement("-v"))
		Expect(args).To(ContainElement("/tmp/mg-kubeconfig-internal.yaml:/kubeconfig/kubeconfig-internal.yaml:ro,Z"))
		Expect(args).To(ContainElement("/tmp/mg-output:/must-gather:Z"))
		Expect(args).To(ContainElement("localhost/must-gather:e2e"))
		Expect(args).To(ContainElement("--dest-dir=/must-gather"))
	})

	It("UT-INFRA-MUSTGATHER-002: defaults namespace/workflow-namespace/since/network when unset", func() {
		args := buildMustGatherPodmanArgs(RunMustGatherImageOptions{
			ClusterName: "gateway-e2e",
			Image:       "localhost/must-gather:e2e",
			OutputDir:   "/tmp/mg-output",
		}, "/tmp/mg-kubeconfig-internal.yaml")

		Expect(args).To(ContainElement("--namespace=kubernaut-system"))
		Expect(args).To(ContainElement("--workflow-namespace=kubernaut-workflows"))
		Expect(args).To(ContainElement("--since=24h"))
		Expect(args).To(ContainElement(DefaultMustGatherKindNetwork))
	})

	It("UT-INFRA-MUSTGATHER-003: honors caller-supplied namespace/workflow-namespace/since/network overrides", func() {
		args := buildMustGatherPodmanArgs(RunMustGatherImageOptions{
			ClusterName:       "fleet-e2e",
			Image:             "localhost/must-gather:e2e",
			OutputDir:         "/tmp/mg-output",
			Network:           "custom-net",
			Namespace:         "custom-kubernaut-ns",
			WorkflowNamespace: "custom-workflows-ns",
			Since:             "48h",
		}, "/tmp/mg-kubeconfig-internal.yaml")

		Expect(args).To(ContainElement("custom-net"))
		Expect(args).To(ContainElement("--namespace=custom-kubernaut-ns"))
		Expect(args).To(ContainElement("--workflow-namespace=custom-workflows-ns"))
		Expect(args).To(ContainElement("--since=48h"))
	})

	It("UT-INFRA-MUSTGATHER-004: --rm ensures the container never lingers after collection", func() {
		args := buildMustGatherPodmanArgs(RunMustGatherImageOptions{
			ClusterName: "gateway-e2e",
			Image:       "localhost/must-gather:e2e",
			OutputDir:   "/tmp/mg-output",
		}, "/tmp/mg-kubeconfig-internal.yaml")

		Expect(args).To(ContainElement("--rm"))
	})

	It("UT-INFRA-MUSTGATHER-011: passes each ExtraNamespaces entry as a repeated --extra-namespace flag (Issue #2036/#2194)", func() {
		// Fleet's Kuadrant/Envoy AI Gateway lanes deploy mesh/gateway infra
		// (mcp-system, istio-system, envoy-gateway-system, ...) outside the
		// Helm release namespace -- must-gather's --extra-namespace flag
		// (cmd/must-gather/gather.sh) is how a caller reaches those.
		args := buildMustGatherPodmanArgs(RunMustGatherImageOptions{
			ClusterName:     "fleet-e2e",
			Image:           "localhost/must-gather:e2e",
			OutputDir:       "/tmp/mg-output",
			ExtraNamespaces: []string{"mcp-system", "istio-system"},
		}, "/tmp/mg-kubeconfig-internal.yaml")

		Expect(args).To(ContainElement("--extra-namespace=mcp-system"))
		Expect(args).To(ContainElement("--extra-namespace=istio-system"))
	})

	It("UT-INFRA-MUSTGATHER-012: omits --extra-namespace entirely when ExtraNamespaces is unset (default, single-cluster suites)", func() {
		args := buildMustGatherPodmanArgs(RunMustGatherImageOptions{
			ClusterName: "gateway-e2e",
			Image:       "localhost/must-gather:e2e",
			OutputDir:   "/tmp/mg-output",
		}, "/tmp/mg-kubeconfig-internal.yaml")

		for _, arg := range args {
			Expect(arg).NotTo(HavePrefix("--extra-namespace"))
		}
	})
})

var _ = Describe("stripKindProviderBanner", func() {
	DescribeTable("removing kind's provider-selection banner ahead of the actual kubeconfig YAML",
		func(raw string, expectErr bool, expectedPrefix string) {
			cleaned, err := stripKindProviderBanner([]byte(raw))
			if expectErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(string(cleaned)).To(HavePrefix(expectedPrefix))
		},
		Entry("UT-INFRA-MUSTGATHER-005: strips the podman-provider banner lines",
			"using podman due to KIND_EXPERIMENTAL_PROVIDER\nenabling experimental podman provider\napiVersion: v1\nkind: Config\n",
			false, "apiVersion: v1"),
		Entry("UT-INFRA-MUSTGATHER-006: passes through clean output unchanged",
			"apiVersion: v1\nkind: Config\n",
			false, "apiVersion: v1"),
		Entry("UT-INFRA-MUSTGATHER-007: errors when no apiVersion field is present at all",
			"some unrelated kind CLI error output\n",
			true, ""),
	)
})

var _ = Describe("RunMustGatherImage input validation", func() {
	DescribeTable("UT-AF-2501-005 [BR-PLATFORM-001.6a]: rejects insecure collector authorities before invoking tools",
		func(authority string) {
			err := RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
				ClusterName: "owned-2501", Image: "test-owned-image", OutputDir: GinkgoT().TempDir(),
				DataStorage: &MustGatherDataStorageOptions{URL: authority},
			}, GinkgoWriter)
			Expect(err).To(MatchError(ContainSubstring("requires an https authority without credentials")))
		}, Entry("plaintext", "http://localhost:8080"), Entry("embedded credentials", "https://user:password@localhost:8080"), Entry("missing authority", "https:///path"))

	It("UT-AF-2501-005 [BR-PLATFORM-001.6a]: refuses to mount credentials from the archive tree", func() {
		dir := GinkgoT().TempDir()
		ca, token := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "token")
		Expect(os.WriteFile(ca, []byte("test-owned-ca"), 0600)).To(Succeed())
		Expect(os.WriteFile(token, []byte("test-owned-token"), 0600)).To(Succeed())
		err := RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
			ClusterName: "owned-2501", Image: "test-owned-image", OutputDir: dir,
			DataStorage: &MustGatherDataStorageOptions{URL: "https://localhost:8080", CAFile: ca, TokenFile: token},
		}, GinkgoWriter)
		Expect(err).To(MatchError(ContainSubstring("must remain outside the output directory")))
	})

	It("IT-AF-2501-002: actual runner supplies file mounts, TLS authority and external connection target", func() {
		dir := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		Expect(os.WriteFile(filepath.Join(dir, "kind"), []byte("#!/bin/bash\nprintf 'apiVersion: v1\\nkind: Config\\n'\n"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "podman"), []byte("#!/bin/bash\nprintf '%s\\n' \"$@\"\n"), 0755)).To(Succeed())
		ca, token := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "token")
		Expect(os.WriteFile(ca, []byte("synthetic CA"), 0600)).To(Succeed())
		Expect(os.WriteFile(token, []byte("credential-must-not-be-in-argv-2501"), 0600)).To(Succeed())
		var log bytes.Buffer
		Expect(RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
			ClusterName: "owned-2501", Image: "test-owned-image", OutputDir: filepath.Join(dir, "output"),
			DataStorage: &MustGatherDataStorageOptions{URL: "https://data-storage-service.kubernaut-system.svc:8080",
				ConnectTo: "data-storage-service.kubernaut-system.svc:8080:10.89.0.5:30089", CAFile: ca, TokenFile: token},
		}, &log)).To(Succeed())
		Expect(log.String()).To(ContainSubstring(ca + ":/datastorage/ca.crt:ro,Z"))
		Expect(log.String()).To(ContainSubstring(token + ":/datastorage/token:ro,Z"))
		Expect(log.String()).To(ContainSubstring("DATASTORAGE_TOKEN_FILE=/datastorage/token"))
		Expect(log.String()).To(ContainSubstring("DATASTORAGE_CA_FILE=/datastorage/ca.crt"))
		Expect(log.String()).To(ContainSubstring("DATASTORAGE_CONNECT_TO=data-storage-service.kubernaut-system.svc:8080:10.89.0.5:30089"))
		Expect(log.String()).ToNot(ContainSubstring("credential-must-not-be-in-argv-2501"))
		Expect(strings.Index(log.String(), "DATASTORAGE_URL=")).To(BeNumerically("<", strings.Index(log.String(), "test-owned-image\n")), "container environment must precede the image")
	})
	It("UT-INFRA-MUSTGATHER-008: rejects a missing ClusterName before shelling out", func() {
		err := RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
			Image:     "localhost/must-gather:e2e",
			OutputDir: "/tmp/mg-output",
		}, GinkgoWriter)
		Expect(err).To(MatchError(ContainSubstring("ClusterName")))
	})

	It("UT-INFRA-MUSTGATHER-009: rejects a missing Image before shelling out", func() {
		err := RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
			ClusterName: "gateway-e2e",
			OutputDir:   "/tmp/mg-output",
		}, GinkgoWriter)
		Expect(err).To(MatchError(ContainSubstring("Image")))
	})

	It("UT-INFRA-MUSTGATHER-010: rejects a missing OutputDir before shelling out", func() {
		err := RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
			ClusterName: "gateway-e2e",
			Image:       "localhost/must-gather:e2e",
		}, GinkgoWriter)
		Expect(err).To(MatchError(ContainSubstring("OutputDir")))
	})
})
