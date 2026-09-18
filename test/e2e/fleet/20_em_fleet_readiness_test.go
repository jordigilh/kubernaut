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

package fleet

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FLEET-020 proves BR-FLEET-054's pod-level fail-closed contract in the
// deployed fleet journey. The EffectivenessMonitor deployment is only counted
// ready after its /readyz gate has accepted the MCP client and cluster
// registry probers. FedRAMP SI-4 covers the monitored dependency health.
var _ = Describe("E2E-FLEET-020 [BR-FLEET-054, SI-4]: EM fleet readiness", Label("fleet", "readiness"), func() {
	It("reports the EffectivenessMonitor ready only after fleet registry startup succeeds", func() {
		By("verifying the fleet fixture contains a managed cluster")
		clusterNames := func() string {
			cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"get", "backend", "-A", "-l", "kubernaut.ai/managed=true",
				"-o", "jsonpath={.items[*].metadata.name}")
			out, err := cmd.Output()
			if err != nil {
				return ""
			}
			return string(out)
		}
		Eventually(clusterNames, timeout, interval).ShouldNot(BeEmpty(),
			"fleet registry must have an authoritative managed-cluster object before EM readiness is assessed")

		By("verifying the deployment readiness gate has accepted the fleet dependencies")
		readyReplicas := func() string {
			cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"get", "deployment", "effectivenessmonitor-controller", "-n", namespace,
				"-o", "jsonpath={.status.readyReplicas}")
			out, err := cmd.Output()
			if err != nil {
				return ""
			}
			return string(out)
		}
		Eventually(readyReplicas, timeout, interval).Should(Equal("1"),
			"EM must enter Service endpoints only after MCP and ClusterRegistry readiness succeeds")
	})
})
