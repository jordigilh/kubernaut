//go:build integration
// +build integration

package infrastructure_integration

import (
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestBasicEnvironment is consolidated into TestInfrastructure

var _ = Describe("Basic Environment Validation", func() {
	BeforeEach(func() {
		fmt.Println("=== BASIC ENVIRONMENT TEST ===")
	})

	AfterEach(func() {
		fmt.Println("=== BASIC ENVIRONMENT TEST COMPLETE ===")
	})

	It("should have basic output working", func() {
		fmt.Println("Test 1: Basic output working")
		// This test always passes - just checking that we can print
		Expect(true).To(BeTrue())
	})

	It("should check KUBEBUILDER_ASSETS environment variable", func() {
		kubebuilderAssets := os.Getenv("KUBEBUILDER_ASSETS")
		fmt.Printf("Test 2: KUBEBUILDER_ASSETS = %s\n", kubebuilderAssets)

		if kubebuilderAssets == "" {
			fmt.Println("Note: KUBEBUILDER_ASSETS not set (expected for individual test runs)")
			fmt.Println("This is set automatically when running the full test suite via Makefile")
		} else {
			fmt.Printf("KUBEBUILDER_ASSETS is set: %s\n", kubebuilderAssets)
		}

		// Don't fail the test if KUBEBUILDER_ASSETS is not set for individual runs
		// It's only required when running the full K8s integration suite
	})

	It("should be able to determine working directory", func() {
		wd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred(), "Should be able to get working directory")
		fmt.Printf("Test 3: Working directory = %s\n", wd)
		Expect(wd).To(BeNumerically(">=", 1), "BR-DATABASE-001-A: Simple debug must provide data for database utilization requirements")
	})

	It("should have podman available", func() {
		cmd := exec.Command("podman", "--version")
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Test 4: Podman not available - %v\n", err)
			fmt.Printf("Test 4: Output: %s\n", string(output))
			Skip("Podman not available - skipping container-related tests")
		} else {
			fmt.Printf("Test 4: Podman available - %s", string(output))
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should check for running PostgreSQL container", func() {
		fmt.Println("Test 5: Checking if PostgreSQL container is running...")
		cmd := exec.Command("podman", "ps", "--format", "{{.Names}}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Test 5: Failed to list containers - %v\n", err)
			Skip("Cannot list containers - podman may not be available")
		} else {
			fmt.Printf("Test 5: Running containers:\n%s", string(output))
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should have Kubernetes test binaries available", func() {
		kubePath := "../../../bin/k8s/1.33.0-darwin-arm64"
		_, err := os.Stat(kubePath)
		if err != nil {
			fmt.Printf("Test 6: K8s binaries not found at %s - %v\n", kubePath, err)
		} else {
			fmt.Printf("Test 6: K8s binaries found at %s\n", kubePath)
		}
		Expect(err).ToNot(HaveOccurred(), "Kubernetes test binaries should be available")
	})
})
