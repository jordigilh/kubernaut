//go:build integration
// +build integration

package infrastructure_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestBasicEnvironment(t *testing.T) {
	fmt.Println("=== BASIC ENVIRONMENT TEST ===")

	// Test 1: Basic output
	fmt.Println("Test 1: Basic output working")

	// Test 2: Environment variables
	fmt.Printf("Test 2: KUBEBUILDER_ASSETS = %s\n", os.Getenv("KUBEBUILDER_ASSETS"))
	fmt.Printf("Test 2: K8S_MCP_SERVER_IMAGE = %s\n", os.Getenv("K8S_MCP_SERVER_IMAGE"))

	// Test 3: Working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Test 3 failed: %v", err)
	}
	fmt.Printf("Test 3: Working directory = %s\n", wd)

	// Test 4: Podman availability
	cmd := exec.Command("podman", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Test 4: Podman not available - %v\n", err)
		fmt.Printf("Test 4: Output: %s\n", string(output))
	} else {
		fmt.Printf("Test 4: Podman available - %s\n", string(output))
	}

	// Test 5: Database connection
	fmt.Println("Test 5: Checking if PostgreSQL container is running...")
	cmd = exec.Command("podman", "ps", "--format", "{{.Names}}")
	output, err = cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Test 5: Failed to list containers - %v\n", err)
	} else {
		fmt.Printf("Test 5: Running containers:\n%s\n", string(output))
	}

	// Test 6: K8s binaries
	kubePath := "./bin/k8s/1.33.0-darwin-arm64"
	if _, err := os.Stat(kubePath); err != nil {
		fmt.Printf("Test 6: K8s binaries not found at %s - %v\n", kubePath, err)
	} else {
		fmt.Printf("Test 6: K8s binaries found at %s\n", kubePath)
	}

	fmt.Println("=== BASIC ENVIRONMENT TEST COMPLETE ===")
}
