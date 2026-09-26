package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kind image source retention [BR-FLEET-054]", func() {
	It("UT-INFRA-AF-FLEET-2462-010 [BR-FLEET-054]: prunes the Podman image after a normal Kind load", func() {
		expectImageLoadRetention(false, true)
	})

	It("UT-INFRA-AF-FLEET-2462-011 [BR-FLEET-054]: retains a shared image for the second AF cluster", func() {
		expectImageLoadRetention(true, false)
	})
})

func expectImageLoadRetention(retainImage, expectPrune bool) {
	tempDir, err := os.MkdirTemp("", "e2e-image-load-cli-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(os.RemoveAll(tempDir)).To(Succeed())
	})

	callLog := filepath.Join(tempDir, "calls.log")
	podmanStub := `#!/bin/sh
set -eu
case "$1" in
  save)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-o" ]; then
        shift
        : > "$1"
        break
      fi
      shift
    done
    printf 'save\n' >> "$IMAGE_LOAD_CALL_LOG"
    ;;
  rmi)
    printf 'rmi\n' >> "$IMAGE_LOAD_CALL_LOG"
    ;;
  *)
    printf 'unexpected podman operation: %s\n' "$1" >&2
    exit 1
    ;;
esac
`
	kindStub := `#!/bin/sh
set -eu
printf 'kind-load\n' >> "$IMAGE_LOAD_CALL_LOG"
`
	for name, contents := range map[string]string{"podman": podmanStub, "kind": kindStub} {
		path := filepath.Join(tempDir, name)
		Expect(os.WriteFile(path, []byte(contents), 0o755)).To(Succeed())
	}

	originalPath := os.Getenv("PATH")
	originalCallLog, hadCallLog := os.LookupEnv("IMAGE_LOAD_CALL_LOG")
	Expect(os.Setenv("PATH", tempDir+string(os.PathListSeparator)+originalPath)).To(Succeed())
	Expect(os.Setenv("IMAGE_LOAD_CALL_LOG", callLog)).To(Succeed())
	DeferCleanup(func() {
		Expect(os.Setenv("PATH", originalPath)).To(Succeed())
		if hadCallLog {
			Expect(os.Setenv("IMAGE_LOAD_CALL_LOG", originalCallLog)).To(Succeed())
		} else {
			Expect(os.Unsetenv("IMAGE_LOAD_CALL_LOG")).To(Succeed())
		}
	})

	serviceName := "image-retention-" + strconv.Itoa(os.Getpid())
	imageName := "localhost/apifrontend:fixture"
	if retainImage {
		err = LoadImageToKindRetainingImage(context.Background(), imageName, serviceName, "test-cluster", io.Discard)
	} else {
		err = LoadImageToKind(context.Background(), imageName, serviceName, "test-cluster", io.Discard)
	}
	Expect(err).NotTo(HaveOccurred())

	calls, err := os.ReadFile(callLog)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(calls)).To(ContainSubstring("save\n"))
	Expect(string(calls)).To(ContainSubstring("kind-load\n"))
	if expectPrune {
		Expect(string(calls)).To(ContainSubstring("rmi\n"))
	} else {
		Expect(string(calls)).NotTo(ContainSubstring("rmi\n"), fmt.Sprintf("shared image %s must remain available", imageName))
	}
}
