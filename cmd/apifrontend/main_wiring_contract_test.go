package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("API Frontend backend incarnation startup resolution", func() {
	It("UT-AF-2614-005: calls an injected generator exactly once", func() {
		var calls atomic.Int32
		value, err := resolveBackendIncarnation(func() (string, error) {
			calls.Add(1)
			return "01234567-89ab-4cde-8123-456789abcdef", nil
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal("01234567-89ab-4cde-8123-456789abcdef"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})

	It("UT-AF-2614-005: reports generator failures with backend-incarnation context", func() {
		_, err := resolveBackendIncarnation(func() (string, error) {
			return "", errors.New("fixture generator failed")
		})

		Expect(err).To(MatchError(ContainSubstring("backend incarnation")))
	})

	DescribeTable("UT-AF-2614-005: rejects invalid generated values before router construction",
		func(value string) {
			_, err := resolveBackendIncarnation(func() (string, error) { return value, nil })
			Expect(err).To(MatchError(ContainSubstring("backend incarnation")))
		},
		Entry("empty", ""),
		Entry("uppercase", "01234567-89ab-4Cde-8123-456789abcdef"),
		Entry("wrong version", "01234567-89ab-3cde-8123-456789abcdef"),
		Entry("line feed", "01234567-89ab-4cde-8123-456789abcde\n"),
	)

	It("UT-AF-2614-005: nil selects the real canonical UUIDv4 generator", func() {
		value, err := resolveBackendIncarnation(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(value)).To(BeTrue())
	})

	It("IT-AF-2614-004: rejects generator failures before constructing a router/server bundle", func() {
		params := testRouterBuildParams(&allowAllToolAuthorizer{})
		params.BackendIncarnationGenerator = func() (string, error) {
			return "", errors.New("fixture generator failed")
		}

		routerServers, cleanup, err := buildRouterAndServers(context.Background(), params)

		Expect(err).To(MatchError(ContainSubstring("backend incarnation")))
		Expect(routerServers).To(BeNil())
		Expect(cleanup).To(BeNil())
	})

	It("IT-AF-2614-005: production construction resolves once and propagates deterministic replacement values", func() {
		dir := GinkgoTB().TempDir()
		generateSelfSignedCertPair(GinkgoTB(), filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))

		previousCA, hadPreviousCA := os.LookupEnv("TLS_CA_FILE")
		Expect(os.Unsetenv("TLS_CA_FILE")).To(Succeed())
		DeferCleanup(func() {
			if hadPreviousCA {
				Expect(os.Setenv("TLS_CA_FILE", previousCA)).To(Succeed())
			} else {
				Expect(os.Unsetenv("TLS_CA_FILE")).To(Succeed())
			}
		})
		sharedtls.ResetDefaultTransportForTesting()
		DeferCleanup(sharedtls.ResetDefaultTransportForTesting)

		build := func(value string) (*routerAndServers, int32) {
			var calls atomic.Int32
			params := testRouterBuildParams(&allowAllToolAuthorizer{})
			params.HDeps.Cfg = config.DefaultConfig()
			params.HDeps.Cfg.Server.Port = 0
			params.HDeps.Cfg.Server.HealthPort = 0
			params.HDeps.Cfg.Server.MetricsPort = 0
			params.HDeps.Cfg.Server.TLS.CertDir = dir
			params.BackendIncarnationGenerator = func() (string, error) {
				calls.Add(1)
				return value, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			routerServers, cleanup, err := buildRouterAndServers(ctx, params)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				cancel()
				if cleanup != nil {
					cleanup()
				}
			})
			return routerServers, calls.Load()
		}

		nilParams := testRouterBuildParams(&allowAllToolAuthorizer{})
		nilParams.HDeps.Cfg = config.DefaultConfig()
		nilParams.HDeps.Cfg.Server.Port = 0
		nilParams.HDeps.Cfg.Server.HealthPort = 0
		nilParams.HDeps.Cfg.Server.MetricsPort = 0
		nilParams.HDeps.Cfg.Server.TLS.CertDir = dir
		nilContext, nilCancel := context.WithCancel(context.Background())
		nilRouter, nilCleanup, err := buildRouterAndServers(nilContext, nilParams)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			nilCancel()
			if nilCleanup != nil {
				nilCleanup()
			}
		})
		Expect(regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(nilRouter.RouterCfg.PublicRoutes.BackendContract.Incarnation)).To(BeTrue())

		routerA, callsA := build("01234567-89ab-4cde-8123-456789abcdef")
		routerB, callsB := build("11234567-89ab-4cde-8123-456789abcdef")

		Expect(callsA).To(Equal(int32(1)))
		Expect(callsB).To(Equal(int32(1)))
		Expect(routerA.RouterCfg.PublicRoutes.BackendContract.Incarnation).To(Equal("01234567-89ab-4cde-8123-456789abcdef"))
		Expect(routerB.RouterCfg.PublicRoutes.BackendContract.Incarnation).To(Equal("11234567-89ab-4cde-8123-456789abcdef"))
		Expect(routerA.RouterCfg.PublicRoutes.BackendContract.Incarnation).NotTo(Equal(routerB.RouterCfg.PublicRoutes.BackendContract.Incarnation))
	})

	It("UT-AF-2614-006: rejects direct router configuration without a resolved value", func() {
		params := testRouterBuildParams(&allowAllToolAuthorizer{})
		params.BackendIncarnation = ""

		_, router, err := buildRouterConfig(params)

		Expect(router).To(BeNil())
		Expect(err).To(MatchError(ContainSubstring("backend incarnation")))
	})
})
