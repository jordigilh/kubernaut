# ════════════════════════════════════════════════════════════════════════════════
# Shared Image Build Utilities
# Per DD-TEST-001: Unique Container Image Tags for Multi-Team Testing
# ════════════════════════════════════════════════════════════════════════════════
#
# This file provides shared functions for generating unique container image tags
# and building service images. All services should include this file.
#
# Usage in service Makefiles:
#   include ../.makefiles/image-build.mk
#
# ════════════════════════════════════════════════════════════════════════════════

# ────────────────────────────────────────────────────────────────────────────────
# Image Tag Generation (DD-TEST-001 Compliant)
# ────────────────────────────────────────────────────────────────────────────────

# Generate unique tag components
USER_TAG := $(shell whoami)
GIT_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
TIMESTAMP := $(shell date +%s)

# Default unique tag format: {service}-{user}-{git-hash}-{timestamp}
# Override with IMAGE_TAG environment variable
define generate_image_tag
$(if $(IMAGE_TAG),$(IMAGE_TAG),$(1)-$(USER_TAG)-$(GIT_HASH)-$(TIMESTAMP))
endef

# Container tool detection (docker or podman)
CONTAINER_TOOL ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null || echo "docker")

# Platform detection for local builds
LOCAL_PLATFORM := linux/$(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

# Multi-architecture platforms (for production builds)
PLATFORMS ?= linux/amd64,linux/arm64

# ────────────────────────────────────────────────────────────────────────────────
# Shared Build Functions
# ────────────────────────────────────────────────────────────────────────────────

# Build service image with unique tag
# Usage: $(call build_service_image,SERVICE_NAME,DOCKERFILE_PATH)
define build_service_image
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🔨 Building $(1) image (DD-TEST-001 compliant)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@IMAGE_TAG=$(call generate_image_tag,$(1)); \
	echo "Service:    $(1)"; \
	echo "Tag:        $$IMAGE_TAG"; \
	echo "Dockerfile: $(2)"; \
	echo "Platform:   $(LOCAL_PLATFORM)"; \
	echo "────────────────────────────────────────────────────────────────────────"; \
	$(CONTAINER_TOOL) build --platform $(LOCAL_PLATFORM) \
		-f $(2) \
		-t $(1):$$IMAGE_TAG \
		. && \
	echo "✅ Image built: $(1):$$IMAGE_TAG" && \
	echo "IMAGE_TAG=$$IMAGE_TAG" > .last-image-tag-$(1).env
endef

# Build multi-arch service image
# Usage: $(call build_service_image_multi,SERVICE_NAME,DOCKERFILE_PATH)
define build_service_image_multi
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🔨 Building $(1) multi-arch image (DD-TEST-001 compliant)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@IMAGE_TAG=$(call generate_image_tag,$(1)); \
	echo "Service:    $(1)"; \
	echo "Tag:        $$IMAGE_TAG"; \
	echo "Dockerfile: $(2)"; \
	echo "Platforms:  $(PLATFORMS)"; \
	echo "────────────────────────────────────────────────────────────────────────"; \
	$(CONTAINER_TOOL) build --platform $(PLATFORMS) \
		-f $(2) \
		-t $(1):$$IMAGE_TAG \
		. && \
	echo "✅ Multi-arch image built: $(1):$$IMAGE_TAG" && \
	echo "IMAGE_TAG=$$IMAGE_TAG" > .last-image-tag-$(1).env
endef

# Load service image into Kind cluster
# Usage: $(call load_image_to_kind,SERVICE_NAME,KIND_CLUSTER_NAME)
define load_image_to_kind
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📦 Loading $(1) image into Kind cluster"
	@echo "════════════════════════════════════════════════════════════════════════"
	@if [ ! -f .last-image-tag-$(1).env ]; then \
		echo "❌ No image tag found for $(1). Build image first."; \
		exit 1; \
	fi; \
	. .last-image-tag-$(1).env; \
	echo "Service: $(1)"; \
	echo "Tag:     $$IMAGE_TAG"; \
	echo "Cluster: $(2)"; \
	echo "────────────────────────────────────────────────────────────────────────"; \
	if ! kind get clusters | grep -q "^$(2)$$"; then \
		echo "❌ Kind cluster '$(2)' does not exist"; \
		exit 1; \
	fi; \
	kind load docker-image $(1):$$IMAGE_TAG --name $(2) && \
	echo "✅ Image loaded: $(1):$$IMAGE_TAG → $(2)"
endef

# Cleanup service image
# Usage: $(call cleanup_service_image,SERVICE_NAME)
define cleanup_service_image
	@echo "🧹 Cleaning up $(1) image..."
	@if [ -f .last-image-tag-$(1).env ]; then \
		. .last-image-tag-$(1).env; \
		echo "Removing $(1):$$IMAGE_TAG"; \
		$(CONTAINER_TOOL) rmi $(1):$$IMAGE_TAG 2>/dev/null || true; \
		rm -f .last-image-tag-$(1).env; \
		echo "✅ Cleanup complete"; \
	else \
		echo "ℹ️  No image to clean up for $(1)"; \
	fi
endef

# ────────────────────────────────────────────────────────────────────────────────
# Service-Specific Targets (Examples)
# ────────────────────────────────────────────────────────────────────────────────
# These can be used as templates for service-specific Makefiles

# Example: Build notification service
# .PHONY: docker-build-notification
# docker-build-notification:
# 	$(call build_service_image,notification,docker/notification-controller.Dockerfile)

# Example: Build and load notification service into Kind
# .PHONY: docker-build-notification-kind
# docker-build-notification-kind: docker-build-notification
# 	$(call load_image_to_kind,notification,notification-test)

# Example: Cleanup notification service image
# .PHONY: docker-clean-notification
# docker-clean-notification:
# 	$(call cleanup_service_image,notification)

# ────────────────────────────────────────────────────────────────────────────────
# Integration Test Targets (DD-TEST-001 Compliant)
# ────────────────────────────────────────────────────────────────────────────────

# Generic integration test target with automatic cleanup
# Usage: $(call run_integration_tests_with_cleanup,SERVICE_NAME,TEST_PATH)
define run_integration_tests_with_cleanup
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Running $(1) Integration Tests (DD-TEST-001 compliant)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@if [ ! -f .last-image-tag-$(1).env ]; then \
		echo "❌ No image tag found. Build image first: make docker-build-$(1)"; \
		exit 1; \
	fi; \
	. .last-image-tag-$(1).env; \
	echo "Service:    $(1)"; \
	echo "Tag:        $$IMAGE_TAG"; \
	echo "Tests:      $(2)"; \
	echo "────────────────────────────────────────────────────────────────────────"; \
	TEST_RESULT=0; \
	IMAGE_TAG=$$IMAGE_TAG go test $(2) -v -timeout 30m || TEST_RESULT=$$?; \
	echo ""; \
	echo "🧹 Cleaning up test image..."; \
	$(CONTAINER_TOOL) rmi $(1):$$IMAGE_TAG 2>/dev/null || true; \
	rm -f .last-image-tag-$(1).env; \
	echo "✅ Cleanup complete"; \
	exit $$TEST_RESULT
endef

# ────────────────────────────────────────────────────────────────────────────────
# Utility Functions
# ────────────────────────────────────────────────────────────────────────────────

# Show current image tag for service
# Usage: $(call show_image_tag,SERVICE_NAME)
define show_image_tag
	@if [ -f .last-image-tag-$(1).env ]; then \
		. .last-image-tag-$(1).env; \
		echo "$(1): $$IMAGE_TAG"; \
	else \
		echo "$(1): No image built"; \
	fi
endef

# Export image tag to environment
# Usage: $(call export_image_tag,SERVICE_NAME)
define export_image_tag
	@if [ -f .last-image-tag-$(1).env ]; then \
		. .last-image-tag-$(1).env; \
		echo "export IMAGE_TAG=$$IMAGE_TAG"; \
	else \
		echo "❌ No image tag found for $(1)"; \
		exit 1; \
	fi
endef

# ────────────────────────────────────────────────────────────────────────────────
# Help Target
# ────────────────────────────────────────────────────────────────────────────────

.PHONY: image-build-help
image-build-help: ## Show image build utilities help
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🔨 Shared Image Build Utilities (DD-TEST-001)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "Tag Format: {service}-{user}-{git-hash}-{timestamp}"
	@echo "Example:    notification-jordi-abc123f-1734278400"
	@echo ""
	@echo "Environment Variables:"
	@echo "  IMAGE_TAG       Override auto-generated tag"
	@echo "  CONTAINER_TOOL  Container tool (docker/podman, auto-detected)"
	@echo "  PLATFORMS       Multi-arch platforms (default: linux/amd64,linux/arm64)"
	@echo ""
	@echo "Functions Available:"
	@echo "  build_service_image(SERVICE,DOCKERFILE)       Build single-arch image"
	@echo "  build_service_image_multi(SERVICE,DOCKERFILE) Build multi-arch image"
	@echo "  load_image_to_kind(SERVICE,CLUSTER)           Load to Kind cluster"
	@echo "  cleanup_service_image(SERVICE)                Clean up image"
	@echo "  show_image_tag(SERVICE)                       Show current tag"
	@echo ""
	@echo "Usage in Makefile:"
	@echo "  include .makefiles/image-build.mk"
	@echo "  docker-build-myservice:"
	@echo "    \$$(call build_service_image,myservice,docker/myservice.Dockerfile)"
	@echo "════════════════════════════════════════════════════════════════════════"

# ════════════════════════════════════════════════════════════════════════════════
# End of Shared Image Build Utilities
# ════════════════════════════════════════════════════════════════════════════════

