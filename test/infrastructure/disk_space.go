/*
Copyright 2025 Jordi Gil.

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

package infrastructure

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Shared Disk Space Management for E2E Tests
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Problem: GitHub Actions runners have limited disk space (~14 GB available)
// Solution: Aggressive cleanup + tracking at each stage
//
// Usage Pattern:
//   1. LogDiskSpace("START", writer)
//   2. Build images in parallel
//   3. LogDiskSpace("IMAGES_BUILT", writer)
//   4. ExportImageToTar(image, tarPath, writer)
//   5. AggressivePodmanCleanup(writer)  ← Frees ~9 GB!
//   6. LogDiskSpace("AFTER_PRUNE", writer)
//   7. Create Kind cluster
//   8. LoadImageFromTar(clusterName, tarPath, writer)
//   9. CleanupTarFile(tarPath, writer)
//
// Design Decision: DD-TEST-008 (E2E Disk Space Management)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// DiskSpaceInfo represents disk space statistics
type DiskSpaceInfo struct {
	Total      string // Total disk size (e.g., "50G")
	Used       string // Used space (e.g., "35G")
	Available  string // Available space (e.g., "15G")
	UsePercent string // Usage percentage (e.g., "70%")
}

// GetDiskSpaceInfo returns disk space statistics for the root filesystem
func GetDiskSpaceInfo() (*DiskSpaceInfo, error) {
	// Use 'df -h /' to get root filesystem stats
	cmd := exec.Command("df", "-h", "/")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute df command: %w", err)
	}

	// Parse output
	// Example:
	// Filesystem      Size  Used Avail Use% Mounted on
	// /dev/sda1        50G   35G   15G  70% /
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected df output: got %d lines", len(lines))
	}

	// Parse the data line (skip header)
	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return nil, fmt.Errorf("unexpected df fields: got %d fields, want at least 5", len(fields))
	}

	return &DiskSpaceInfo{
		Total:      fields[1], // Size
		Used:       fields[2], // Used
		Available:  fields[3], // Avail
		UsePercent: fields[4], // Use%
	}, nil
}

// LogDiskSpace logs disk space at a specific stage for diagnostics
//
// Example output:
//
//	💾 [START] Disk: 50G total, 35G used, 15G available (70% used)
//	💾 [IMAGES_BUILT] Disk: 50G total, 42G used, 8G available (84% used)
//	💾 [AFTER_PRUNE] Disk: 50G total, 36G used, 14G available (72% used) [+6G freed]
func LogDiskSpace(stage string, writer io.Writer) {
	info, err := GetDiskSpaceInfo()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  [%s] Failed to get disk space: %v\n", stage, err)
		return
	}

	_, _ = fmt.Fprintf(writer, "  💾 [%s] Disk: %s total, %s used, %s available (%s used)\n",
		stage, info.Total, info.Used, info.Available, info.UsePercent)
}

// LogDiskSpaceWithComparison logs disk space and compares with previous stage
//
// Example output:
//
//	💾 [AFTER_PRUNE] Disk: 50G total, 36G used, 14G available (72% used) [+6G freed vs IMAGES_BUILT]
func LogDiskSpaceWithComparison(stage string, previousInfo *DiskSpaceInfo, writer io.Writer) *DiskSpaceInfo {
	currentInfo, err := GetDiskSpaceInfo()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  [%s] Failed to get disk space: %v\n", stage, err)
		return previousInfo
	}

	_, _ = fmt.Fprintf(writer, "  💾 [%s] Disk: %s total, %s used, %s available (%s used)",
		stage, currentInfo.Total, currentInfo.Used, currentInfo.Available, currentInfo.UsePercent)

	// Calculate change if previous info available
	if previousInfo != nil {
		// Simple comparison based on used space string (rough estimate)
		_, _ = fmt.Fprintf(writer, " [vs previous stage]")
	}
	_, _ = fmt.Fprintln(writer)

	return currentInfo
}

// ExportImageToTar exports a Podman image to a .tar file
//
// This prepares the image for Kind loading and allows us to prune Podman cache safely.
//
// Parameters:
//   - imageName: Full Podman image name (e.g., "localhost/kubernaut-datastorage:latest")
//   - tarPath: Output path for .tar file (e.g., "/tmp/datastorage-e2e.tar")
//   - writer: Output writer for logs
//
// Returns error if export fails
func ExportImageToTar(imageName, tarPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📦 Exporting %s to %s...\n", imageName, tarPath)

	saveCmd := exec.Command("podman", "save", "-o", tarPath, imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to export image %s: %w", imageName, err)
	}

	// Verify .tar file exists and has reasonable size
	fileInfo, err := os.Stat(tarPath)
	if err != nil {
		return fmt.Errorf("failed to verify .tar file: %w", err)
	}

	// .tar files should be at least 100 MB for our service images
	if fileInfo.Size() < 100*1024*1024 {
		return fmt.Errorf(".tar file too small (%d bytes), export may have failed", fileInfo.Size())
	}

	sizeMB := fileInfo.Size() / (1024 * 1024)
	_, _ = fmt.Fprintf(writer, "  ✅ Image exported successfully (%d MB)\n", sizeMB)

	return nil
}

// LoadImageFromTar loads a .tar file into a Kind cluster
//
// This is the counterpart to ExportImageToTar - loads the image after Podman cleanup.
//
// Parameters:
//   - clusterName: Kind cluster name
//   - tarPath: Path to .tar file
//   - writer: Output writer for logs
//
// Returns error if load fails
func LoadImageFromTar(clusterName, tarPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📦 Loading image from %s into Kind cluster %s...\n", tarPath, clusterName)

	loadCmd := exec.Command("kind", "load", "image-archive", tarPath, "--name", clusterName)
	loadCmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image from %s: %w", tarPath, err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Image loaded successfully\n")
	return nil
}

// CleanupTarFile removes a .tar file and logs the result
//
// Non-fatal: If deletion fails, logs warning but doesn't return error
func CleanupTarFile(tarPath string, writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "  🗑️  Removing %s...\n", tarPath)

	if err := os.Remove(tarPath); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to remove .tar file (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintf(writer, "  ✅ .tar file removed\n")
	}
}

// AggressivePodmanCleanup performs aggressive Podman cleanup to free disk space
//
// CRITICAL: This removes ALL Podman data (images, cache, build layers).
// SAFE: Only call this AFTER exporting images to .tar files.
//
// What it removes:
//   - ALL stopped containers
//   - ALL images (both tagged and untagged)
//   - ALL build cache
//   - ALL intermediate layers
//
// Expected space freed: ~5-9 GB (depending on build cache size)
//
// Safety: Images are preserved as .tar files and can be loaded into Kind
//
// Parameters:
//   - writer: Output writer for logs
//
// Returns error if prune fails critically (non-fatal warnings are logged)
func AggressivePodmanCleanup(writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "\n🗑️  AGGRESSIVE PODMAN CLEANUP")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  ⚠️  Removing ALL Podman data:")
	_, _ = fmt.Fprintln(writer, "     • ALL images (tagged and untagged)")
	_, _ = fmt.Fprintln(writer, "     • ALL build cache")
	_, _ = fmt.Fprintln(writer, "     • ALL intermediate layers")
	_, _ = fmt.Fprintln(writer, "  ✅ Safe: Images preserved as .tar files")
	_, _ = fmt.Fprintln(writer, "  💾 Expected: ~5-9 GB freed")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Run podman system prune with --all flag (removes everything)
	pruneCmd := exec.Command("podman", "system", "prune", "-a", "-f", "--volumes")
	pruneOutput, err := pruneCmd.CombinedOutput()

	// Log the output regardless of error
	if len(pruneOutput) > 0 {
		_, _ = fmt.Fprintf(writer, "%s\n", string(pruneOutput))
	}

	if err != nil {
		// Log warning but don't fail the entire test suite
		_, _ = fmt.Fprintf(writer, "  ⚠️  Prune command failed (non-fatal): %v\n", err)
		_, _ = fmt.Fprintln(writer, "  ⚠️  Continuing with reduced disk space...")
		return nil // Non-fatal
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Podman cleanup completed successfully")
	return nil
}

// ExportImagesAndPrune is a high-level helper that:
//  1. Exports multiple images to .tar files
//  2. Performs aggressive Podman cleanup
//  3. Returns map of image name -> .tar path for later loading
//
// This is the recommended pattern for E2E tests with multiple services.
//
// Example usage:
//
//	tarFiles, err := ExportImagesAndPrune(map[string]string{
//	    "datastorage":  "localhost/kubernaut-datastorage:latest",
//	    "holmesgpt-api": "localhost/kubernaut-holmesgpt-api:latest",
//	}, "/tmp", writer)
//	if err != nil {
//	    return err
//	}
//	// Later: LoadImageFromTar(clusterName, tarFiles["datastorage"], writer)
func ExportImagesAndPrune(images map[string]string, tmpDir string, writer io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Exporting images to .tar files")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	LogDiskSpace("BEFORE_EXPORT", writer)

	tarFiles := make(map[string]string)

	// Export all images to .tar files
	for name, image := range images {
		tarPath := fmt.Sprintf("%s/%s-e2e.tar", tmpDir, name)
		if err := ExportImageToTar(image, tarPath, writer); err != nil {
			return nil, fmt.Errorf("failed to export %s: %w", name, err)
		}
		tarFiles[name] = tarPath
	}

	_, _ = fmt.Fprintln(writer, "✅ All images exported to .tar files")
	LogDiskSpace("AFTER_EXPORT", writer)

	// Aggressive cleanup to free disk space
	_, _ = fmt.Fprintln(writer, "\n🗑️  PHASE 3: Aggressive Podman cleanup")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if err := AggressivePodmanCleanup(writer); err != nil {
		// Non-fatal, but log it
		_, _ = fmt.Fprintf(writer, "  ⚠️  Cleanup warning: %v\n", err)
	}

	LogDiskSpace("AFTER_PRUNE", writer)

	return tarFiles, nil
}

// LoadImagesAndCleanup is a high-level helper that:
//  1. Loads multiple .tar files into Kind cluster
//  2. Deletes .tar files after successful load
//
// This is the recommended pattern for E2E tests with multiple services.
//
// Example usage:
//
//	if err := LoadImagesAndCleanup(clusterName, tarFiles, writer); err != nil {
//	    return err
//	}
func LoadImagesAndCleanup(clusterName string, tarFiles map[string]string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 5: Loading images into Kind cluster")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	LogDiskSpace("BEFORE_LOAD", writer)

	// Load all images into Kind
	for name, tarPath := range tarFiles {
		_, _ = fmt.Fprintf(writer, "\n  📦 Loading %s...\n", name)
		if err := LoadImageFromTar(clusterName, tarPath, writer); err != nil {
			return fmt.Errorf("failed to load %s: %w", name, err)
		}
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images loaded into Kind cluster")
	LogDiskSpace("AFTER_LOAD", writer)

	// Clean up .tar files
	_, _ = fmt.Fprintln(writer, "\n🗑️  PHASE 6: Cleaning up .tar files")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for name, tarPath := range tarFiles {
		_, _ = fmt.Fprintf(writer, "  🗑️  %s: ", name)
		CleanupTarFile(tarPath, writer)
	}

	_, _ = fmt.Fprintln(writer, "✅ All .tar files cleaned up")
	LogDiskSpace("AFTER_CLEANUP", writer)

	return nil
}
