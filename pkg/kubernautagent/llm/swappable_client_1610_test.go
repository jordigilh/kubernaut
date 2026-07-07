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

package llm_test

import (
	"strconv"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("SwappableClient.Pin — atomic client+model+params snapshot (#1610)", func() {

	Describe("UT-KA-1610-001: Pin never observes a torn read across concurrent Swap", func() {
		It("should return a Client/ModelName/RuntimeParams triple from the same Swap version every time", func() {
			const iterations = 5000

			original := &recordingClient{id: "0"}
			sc, err := llm.NewSwappableClient(original, "model-0", llm.RuntimeParams{Temperature: 0})
			Expect(err).NotTo(HaveOccurred())

			var mismatches atomic.Int64
			var wg sync.WaitGroup
			stop := make(chan struct{})

			// Writer: swaps in a new client/model/params triple on every
			// iteration, each uniquely identifiable by index.
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer close(stop)
				for i := 1; i <= iterations; i++ {
					id := strconv.Itoa(i)
					_ = sc.Swap(&recordingClient{id: id}, "model-"+id, llm.RuntimeParams{Temperature: float64(i)})
				}
			}()

			// Readers: repeatedly Pin() and verify the returned triple is
			// internally consistent (ModelName/RuntimeParams describe the
			// exact same Swap version as the returned Client).
			for w := 0; w < 8; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						snap := sc.Pin()
						rc, ok := snap.Client.(*recordingClient)
						if !ok {
							mismatches.Add(1)
							continue
						}
						idx, convErr := strconv.Atoi(rc.id)
						if convErr != nil {
							mismatches.Add(1)
							continue
						}
						if snap.ModelName != "model-"+rc.id || snap.RuntimeParams.Temperature != float64(idx) {
							mismatches.Add(1)
						}
					}
				}()
			}

			wg.Wait()
			Expect(mismatches.Load()).To(BeZero(),
				"Pin() must return a mutually consistent (Client, ModelName, RuntimeParams) triple even under concurrent Swap")
		})
	})
})
