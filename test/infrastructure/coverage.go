package infrastructure

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// CollectE2EBinaryCoverage collects Go coverage data from the Kind node's
// hostPath volume and merges it into a text profile. This implements the
// DD-TEST-007 pattern from kubernaut.
//
// Prerequisites:
//   - AF built with GOFLAGS=-cover
//   - Pod deployed with GOCOVERDIR=/coverdata
//   - Kind node has hostPath /coverdata mounted
//
// Returns the path to the merged coverage profile, or an error.
func CollectE2EBinaryCoverage(clusterName string, writer io.Writer) (string, error) {
	localDir := filepath.Join(os.TempDir(), "af-e2e-coverage")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return "", fmt.Errorf("create local coverage dir: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "Collecting coverage data from Kind node...")

	// Get the Kind node container name
	nodeName := clusterName + "-control-plane"

	// Copy coverage data from the Kind node
	cpCmd := exec.Command("podman", "cp", nodeName+":/coverdata/.", localDir)
	cpCmd.Stdout = writer
	cpCmd.Stderr = writer
	if err := cpCmd.Run(); err != nil {
		return "", fmt.Errorf("podman cp coverage data: %w", err)
	}

	// Check if we got any coverage files
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return "", fmt.Errorf("read coverage dir: %w", err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no coverage data files found in %s", localDir)
	}

	_, _ = fmt.Fprintf(writer, "Found %d coverage data files\n", len(entries))

	// Merge into a text profile
	profilePath := filepath.Join(os.TempDir(), "coverage_e2e_apifrontend_binary.out")
	mergeCmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+localDir, "-o="+profilePath)
	mergeCmd.Stdout = writer
	mergeCmd.Stderr = writer
	if err := mergeCmd.Run(); err != nil {
		return "", fmt.Errorf("go tool covdata textfmt: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "Coverage profile written to %s\n", profilePath)

	// Print summary
	funcCmd := exec.Command("go", "tool", "cover", "-func="+profilePath)
	funcCmd.Stdout = writer
	funcCmd.Stderr = writer
	_ = funcCmd.Run()

	return profilePath, nil
}
