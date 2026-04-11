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

package cors_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubecors "github.com/jordigilh/kubernaut/pkg/http/cors"
)

var _ = Describe("BR-HTTP-015: shared CORS coverage (issue 668)", func() {
	AfterEach(func() {
		_ = os.Unsetenv("CORS_MAX_AGE")
	})

	Describe("ProductionOptions (BR-HTTP-015)", func() {
		It("returns empty allowed origins and production-safe defaults (BR-HTTP-015)", func() {
			opts := kubecors.ProductionOptions()
			Expect(opts.AllowedOrigins).To(BeEmpty())
			Expect(opts.AllowedMethods).To(Equal([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}))
			Expect(opts.AllowedHeaders).To(Equal([]string{"Accept", "Authorization", "Content-Type", "X-Request-ID"}))
			Expect(opts.ExposedHeaders).To(Equal([]string{"Link", "X-Request-ID", "X-Total-Count"}))
			Expect(opts.AllowCredentials).To(BeFalse())
			Expect(opts.MaxAge).To(Equal(300))
		})
	})

	Describe("FromEnvironment CORS_MAX_AGE parsing (BR-HTTP-015)", func() {
		It("sets MaxAge from a numeric CORS_MAX_AGE string (BR-HTTP-015)", func() {
			Expect(os.Setenv("CORS_MAX_AGE", "7200")).To(Succeed())
			opts := kubecors.FromEnvironment()
			Expect(opts.MaxAge).To(Equal(7200))
		})

		It("leaves MaxAge at default when CORS_MAX_AGE is unset (BR-HTTP-015)", func() {
			opts := kubecors.FromEnvironment()
			Expect(opts.MaxAge).To(Equal(300))
		})

		It("sets MaxAge to zero when CORS_MAX_AGE contains non-digits (BR-HTTP-015)", func() {
			Expect(os.Setenv("CORS_MAX_AGE", "12x3")).To(Succeed())
			opts := kubecors.FromEnvironment()
			Expect(opts.MaxAge).To(Equal(0))
		})
	})
})
