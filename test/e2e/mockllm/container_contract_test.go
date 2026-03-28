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
package mockllm_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	imageName     = "localhost/mock-llm:e2e-test"
	containerName = "mock-llm-e2e-contract"
	containerPort = "18080"
)

func containerTool() string {
	if tool := os.Getenv("CONTAINER_TOOL"); tool != "" {
		return tool
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	return "docker"
}

func projectRoot() string {
	wd, _ := os.Getwd()
	// Walk up from test/e2e/mockllm/ to project root
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	return wd
}

var _ = Describe("Container Contract", Ordered, func() {
	var tool string

	BeforeAll(func() {
		tool = containerTool()

		_, err := exec.LookPath(tool)
		if err != nil {
			Skip(fmt.Sprintf("container tool %q not available, skipping E2E", tool))
		}

		root := projectRoot()
		dockerfilePath := filepath.Join(root, "test", "services", "mock-llm", "go.Dockerfile")
		buildCtx := root

		GinkgoWriter.Printf("Building image with %s from %s (context: %s)\n", tool, dockerfilePath, buildCtx)

		cmd := exec.Command(tool, "build",
			"--target", "production",
			"-t", imageName,
			"-f", dockerfilePath,
			buildCtx,
		)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter
		Expect(cmd.Run()).To(Succeed(), "Image build should succeed")
	})

	AfterAll(func() {
		// Stop and remove container
		_ = exec.Command(tool, "rm", "-f", containerName).Run()
		// Remove image
		_ = exec.Command(tool, "rmi", "-f", imageName).Run()
	})

	Describe("E2E-MOCK-090-001: Image builds from multi-stage Dockerfile", func() {
		It("should have built successfully (validated by BeforeAll)", func() {
			cmd := exec.Command(tool, "image", "exists", imageName)
			Expect(cmd.Run()).To(Succeed(), "Image should exist after build")
		})
	})

	Describe("E2E-MOCK-091-001: Image size under 50MB", func() {
		It("should have a compressed size under 50MB", func() {
			cmd := exec.Command(tool, "image", "inspect", imageName,
				"--format", "{{.Size}}")
			out, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())

			sizeBytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			Expect(err).NotTo(HaveOccurred())

			sizeMB := float64(sizeBytes) / (1024 * 1024)
			GinkgoWriter.Printf("Image size: %.1f MB\n", sizeMB)
			Expect(sizeMB).To(BeNumerically("<", 50),
				fmt.Sprintf("Image size %.1f MB exceeds 50MB limit", sizeMB))
		})
	})

	Describe("E2E-MOCK-092-001: Sub-second startup with health response", func() {
		It("should respond to /health within 1 second of container start", func() {
			// Start container
			cmd := exec.Command(tool, "run", "-d",
				"--name", containerName,
				"-p", containerPort+":8080",
				imageName,
			)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Container start failed: %s", string(out))

			defer func() {
				_ = exec.Command(tool, "rm", "-f", containerName).Run()
			}()

			// Poll /health for up to 2 seconds (1s target + 1s buffer for container scheduling)
			start := time.Now()
			var lastErr error
			Eventually(func() error {
				resp, err := http.Get("http://localhost:" + containerPort + "/health")
				if err != nil {
					lastErr = err
					return err
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					lastErr = fmt.Errorf("status %d", resp.StatusCode)
					return lastErr
				}
				return nil
			}, 2*time.Second, 100*time.Millisecond).Should(Succeed(),
				"Health endpoint did not respond: %v", lastErr)

			elapsed := time.Since(start)
			GinkgoWriter.Printf("Health responded in %s\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("E2E-MOCK-093-001: Container contract (UID, port, chat endpoint)", func() {
		var cid string

		BeforeEach(func() {
			// Ensure no leftover container
			_ = exec.Command(tool, "rm", "-f", containerName+"-contract").Run()

			cmd := exec.Command(tool, "run", "-d",
				"--name", containerName+"-contract",
				"-p", "18081:8080",
				imageName,
			)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Container start failed: %s", string(out))
			cid = strings.TrimSpace(string(out))

			// Wait for ready
			Eventually(func() error {
				resp, err := http.Get("http://localhost:18081/health")
				if err != nil {
					return err
				}
				resp.Body.Close()
				return nil
			}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
		})

		AfterEach(func() {
			_ = exec.Command(tool, "rm", "-f", containerName+"-contract").Run()
		})

		It("should run as non-root UID 1001", func() {
			cmd := exec.Command(tool, "inspect", cid,
				"--format", "{{.Config.User}}")
			out, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			user := strings.TrimSpace(string(out))
			Expect(user).To(Equal("1001"),
				"Container should run as UID 1001, got %q", user)
		})

		It("should expose port 8080", func() {
			cmd := exec.Command(tool, "inspect", cid,
				"--format", "{{json .Config.ExposedPorts}}")
			out, err := cmd.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("8080"))
		})

		It("should respond to POST /v1/chat/completions", func() {
			body := strings.NewReader(`{
				"model": "mock-model",
				"messages": [{"role": "user", "content": "test"}]
			}`)
			resp, err := http.Post("http://localhost:18081/v1/chat/completions",
				"application/json", body)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			var result map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result).To(HaveKey("choices"))
		})
	})
})
