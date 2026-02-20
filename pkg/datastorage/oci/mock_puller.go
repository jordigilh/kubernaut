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

package oci

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// ========================================
// MOCK IMAGE PULLER (DD-WORKFLOW-017)
// ========================================
// Used in unit/integration tests to avoid pulling real OCI images.
// Creates in-memory images with /workflow-schema.yaml content.
// ========================================

// MockImagePuller implements ImagePuller for testing.
// It returns an in-memory OCI image containing /workflow-schema.yaml
// with the provided content.
type MockImagePuller struct {
	schemaContent string
}

// NewMockImagePuller creates a MockImagePuller that returns an image
// containing /workflow-schema.yaml with the given content.
// If content is empty, the image will have no files (simulating missing schema).
func NewMockImagePuller(schemaContent string) *MockImagePuller {
	return &MockImagePuller{schemaContent: schemaContent}
}

// Exists always succeeds for mock images.
func (m *MockImagePuller) Exists(_ context.Context, _ string) error {
	return nil
}

// Pull returns an in-memory OCI image with /workflow-schema.yaml.
func (m *MockImagePuller) Pull(_ context.Context, ref string) (v1.Image, string, error) {
	img := empty.Image

	if m.schemaContent != "" {
		// Build a tar layer containing /workflow-schema.yaml
		layerContent, err := buildTarLayer(SchemaFilePath, []byte(m.schemaContent))
		if err != nil {
			return nil, "", fmt.Errorf("build mock layer: %w", err)
		}

		// Use static.NewLayer to create a materialized layer (not streaming).
		// stream.NewLayer's digest isn't available until consumed, which fails
		// when the extractor calls img.Digest() before reading layers.
		layer := static.NewLayer(layerContent, types.DockerLayer)
		img, err = mutate.AppendLayers(img, layer)
		if err != nil {
			return nil, "", fmt.Errorf("append mock layer: %w", err)
		}
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, "", fmt.Errorf("compute mock digest: %w", err)
	}

	return img, digest.String(), nil
}

// MockImagePullerWithFailingExists implements ImagePuller where Pull() succeeds
// (schema extraction works) but Exists() returns an error (bundle image not found).
// This enables testing the handler's bundle-not-found error path (Step 5c) without
// failing at the schema extraction stage (Step 4).
type MockImagePullerWithFailingExists struct {
	*MockImagePuller
	existsErr error
}

// NewMockImagePullerWithFailingExists creates a mock where Pull returns a valid
// image containing the given schema content, but Exists returns the given error.
func NewMockImagePullerWithFailingExists(schemaContent string, existsErr error) *MockImagePullerWithFailingExists {
	return &MockImagePullerWithFailingExists{
		MockImagePuller: NewMockImagePuller(schemaContent),
		existsErr:       existsErr,
	}
}

// Exists returns the configured error, simulating a missing bundle image.
func (m *MockImagePullerWithFailingExists) Exists(_ context.Context, _ string) error {
	return m.existsErr
}

// FailingMockImagePuller implements ImagePuller and always returns an error.
type FailingMockImagePuller struct {
	errMsg string
}

// NewFailingMockImagePuller creates a mock puller that always fails with the given message.
func NewFailingMockImagePuller(errMsg string) *FailingMockImagePuller {
	return &FailingMockImagePuller{errMsg: errMsg}
}

// Exists always returns an error for the failing mock.
func (m *FailingMockImagePuller) Exists(_ context.Context, ref string) error {
	return fmt.Errorf("image %q not found in registry: %s", ref, m.errMsg)
}

// Pull always returns an error.
func (m *FailingMockImagePuller) Pull(_ context.Context, ref string) (v1.Image, string, error) {
	return nil, "", fmt.Errorf("pull image %q: %s", ref, m.errMsg)
}

// buildTarLayer creates a tar archive containing a single file at the given path.
func buildTarLayer(filePath string, content []byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Remove leading slash for tar entry
	name := filePath
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	header := &tar.Header{
		Name: name,
		Size: int64(len(content)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
