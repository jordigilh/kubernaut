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
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// ========================================
// OCI IMAGE PULLER (DD-WORKFLOW-017)
// ========================================
// Authority: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// Business Requirement: BR-WORKFLOW-001 (Workflow Registry Management)
// ========================================
//
// ImagePuller abstracts OCI image pulling to enable:
// - Production: Pull real images via crane
// - Testing: Mock puller returns pre-built images
//
// ========================================

// ImagePuller abstracts OCI image pulling operations.
// Implementations must be safe for concurrent use.
type ImagePuller interface {
	// Pull retrieves an OCI image by reference (e.g., "quay.io/org/workflow:v1.0.0").
	// Returns the image and its SHA-256 digest.
	Pull(ctx context.Context, ref string) (v1.Image, string, error)
}

// CraneImagePuller implements ImagePuller using google/go-containerregistry (crane).
type CraneImagePuller struct {
	logger logr.Logger
	opts   []crane.Option
}

// NewCraneImagePuller creates a new CraneImagePuller with optional crane options.
func NewCraneImagePuller(logger logr.Logger, opts ...crane.Option) *CraneImagePuller {
	return &CraneImagePuller{
		logger: logger,
		opts:   opts,
	}
}

// Pull retrieves an OCI image by reference using crane.
// Returns the v1.Image, the SHA-256 digest string, and any error.
func (p *CraneImagePuller) Pull(ctx context.Context, ref string) (v1.Image, string, error) {
	p.logger.V(1).Info("Pulling OCI image", "ref", ref)

	// crane.Pull respects context via crane.WithContext
	opts := append([]crane.Option{crane.WithContext(ctx)}, p.opts...)

	img, err := crane.Pull(ref, opts...)
	if err != nil {
		return nil, "", fmt.Errorf("pull image %q: %w", ref, err)
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, "", fmt.Errorf("compute digest for %q: %w", ref, err)
	}

	digestStr := digest.String()
	p.logger.V(1).Info("OCI image pulled successfully", "ref", ref, "digest", digestStr)

	return img, digestStr, nil
}
