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
	"context"
	"fmt"
	"io"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
)

// ========================================
// OCI SCHEMA EXTRACTOR (DD-WORKFLOW-017)
// ========================================
// Authority: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// Business Requirement: BR-WORKFLOW-001 (Workflow Registry Management)
// ========================================
//
// SchemaExtractor pulls an OCI image, locates /workflow-schema.yaml
// in the image layers, parses and validates it, and returns the
// parsed schema along with the image digest.
//
// ========================================

// SchemaFilePath is the expected location of workflow-schema.yaml in OCI images.
const SchemaFilePath = "/workflow-schema.yaml"

// ExtractionResult holds the result of extracting a workflow schema from an OCI image.
type ExtractionResult struct {
	// Schema is the parsed and validated workflow schema.
	Schema *models.WorkflowSchema

	// Digest is the SHA-256 digest of the OCI image.
	Digest string

	// RawContent is the raw YAML content of the workflow-schema.yaml file.
	RawContent string
}

// SchemaExtractor extracts and validates workflow-schema.yaml from OCI images.
type SchemaExtractor struct {
	puller ImagePuller
	parser *schema.Parser
}

// NewSchemaExtractor creates a new SchemaExtractor.
func NewSchemaExtractor(puller ImagePuller, parser *schema.Parser) *SchemaExtractor {
	return &SchemaExtractor{
		puller: puller,
		parser: parser,
	}
}

// ExtractFromImage pulls an OCI image and extracts the workflow-schema.yaml.
// Returns the parsed schema, the image digest, and the raw YAML content.
//
// Error conditions:
// - Image pull failure (network, auth, not found)
// - /workflow-schema.yaml not found in any layer
// - Invalid YAML content
// - Schema validation failure (missing required fields/labels)
func (e *SchemaExtractor) ExtractFromImage(ctx context.Context, imageRef string) (*ExtractionResult, error) {
	// Step 1: Pull the OCI image
	img, digest, err := e.puller.Pull(ctx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("pull image %q: %w", imageRef, err)
	}

	// Step 2: Find /workflow-schema.yaml in image layers
	content, err := findFileInImage(img, SchemaFilePath)
	if err != nil {
		return nil, err
	}

	// Step 3: Parse and validate the schema
	parsedSchema, err := e.parser.ParseAndValidate(content)
	if err != nil {
		return nil, fmt.Errorf("validate workflow-schema.yaml from %q: %w", imageRef, err)
	}

	return &ExtractionResult{
		Schema:     parsedSchema,
		Digest:     digest,
		RawContent: content,
	}, nil
}

// ValidateBundleExists checks that the execution.bundle image reference exists in the registry.
// Called after schema extraction to provide early feedback on typos or missing images
// rather than failing at workflow execution time.
func (e *SchemaExtractor) ValidateBundleExists(ctx context.Context, bundleRef string) error {
	return e.puller.Exists(ctx, bundleRef)
}

// findFileInImage searches all layers of an OCI image for the given file path.
// Returns the file content as a string, or an error if not found.
func findFileInImage(img v1.Image, filePath string) (string, error) {
	layers, err := img.Layers()
	if err != nil {
		return "", fmt.Errorf("read image layers: %w", err)
	}

	// Normalize the search path (remove leading slash for tar comparison)
	searchPath := strings.TrimPrefix(filePath, "/")

	// Search layers in reverse order (top layer first, most likely to contain the file)
	for i := len(layers) - 1; i >= 0; i-- {
		content, found, err := findFileInLayer(layers[i], searchPath)
		if err != nil {
			return "", fmt.Errorf("read layer %d: %w", i, err)
		}
		if found {
			return content, nil
		}
	}

	return "", fmt.Errorf("%s not found in image layers", filePath)
}

// findFileInLayer searches a single layer for a file by path.
func findFileInLayer(layer v1.Layer, searchPath string) (string, bool, error) {
	rc, err := layer.Uncompressed()
	if err != nil {
		return "", false, fmt.Errorf("uncompress layer: %w", err)
	}
	defer func() { _ = rc.Close() }()

	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, fmt.Errorf("read tar entry: %w", err)
		}

		// Match the file path (handle with or without leading ./ or /)
		name := strings.TrimPrefix(header.Name, "./")
		name = strings.TrimPrefix(name, "/")

		if name == searchPath && header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return "", false, fmt.Errorf("read file %q: %w", searchPath, err)
			}
			return string(data), true, nil
		}
	}

	return "", false, nil
}
