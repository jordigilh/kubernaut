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
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const (
	fleetRedisContainerName = "fleet_valkey_it_1"
	fleetRedisPort          = 16390
)

func TestFleet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Scope Cache Integration Suite")
}

var valkeyAddr string

var _ = SynchronizedBeforeSuite(func() []byte {
	By("Starting Valkey (Redis) container for fleet scope cache IT")
	cfg := infrastructure.RedisConfig{
		ContainerName: fleetRedisContainerName,
		Port:          fleetRedisPort,
	}
	infrastructure.CleanupContainers([]string{fleetRedisContainerName}, GinkgoWriter)
	Expect(infrastructure.StartRedis(cfg, GinkgoWriter)).To(Succeed(),
		"Failed to start Valkey container")
	Expect(infrastructure.WaitForRedisReady(fleetRedisContainerName, GinkgoWriter)).To(Succeed(),
		"Valkey failed to become ready")

	addr := fmt.Sprintf("127.0.0.1:%d", fleetRedisPort)
	return []byte(addr)
}, func(data []byte) {
	valkeyAddr = string(data)
	fmt.Fprintf(os.Stdout, "Fleet IT using Valkey at %s\n", valkeyAddr)
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	By("Stopping Valkey container")
	infrastructure.CleanupContainers([]string{fleetRedisContainerName}, GinkgoWriter)
})
