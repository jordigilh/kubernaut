# Makefile.e2e-coverage.mk
# Reusable E2E Coverage Targets for Go Services
#
# This file provides standardized E2E coverage collection following DD-TEST-007.
# Include this file in your main Makefile and use the define-e2e-coverage-target function.
#
# Reference: docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md
#
# Usage in Makefile:
#   include Makefile.e2e-coverage.mk
#   $(eval $(call define-e2e-coverage-target,notification,notification,4))
#
# This generates: test-e2e-notification-coverage target

##@ E2E Coverage Collection (DD-TEST-007)

# Function: define-e2e-coverage-target
# Args:
#   $(1) = service name (e.g., notification, datastorage, gateway)
#   $(2) = service directory name in test/e2e/ (usually same as service name)
#   $(3) = number of parallel processes (default: 4)
#
# Example: $(eval $(call define-e2e-coverage-target,notification,notification,4))
define define-e2e-coverage-target
.PHONY: test-e2e-$(1)-coverage
test-e2e-$(1)-coverage: ## Run $(1) E2E tests with coverage collection (DD-TEST-007)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📊 $(shell echo $(1) | sed 's/\b\(.\)/\u\1/g') Service - E2E Coverage Collection (DD-TEST-007)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Collecting coverage from:"
	@echo "   • Binary profiling (Go 1.20+) during E2E execution"
	@echo "   • Graceful shutdown triggers coverage data write"
	@echo "   • Coverage directory: ./test/e2e/$(2)/coverdata/"
	@echo ""
	@echo "🏗️  Step 1: Building $(1) image with coverage instrumentation..."
	@echo "   Setting E2E_COVERAGE=true to enable GOFLAGS=-cover in Dockerfile"
	@echo ""
	@$$(MAKE) E2E_COVERAGE=true test-e2e-$(1)
	@echo ""
	@echo "📊 Step 2: Generating coverage reports..."
	@./scripts/generate-e2e-coverage.sh $(1) ./test/e2e/$(2)/coverdata ./test/e2e/$(2)
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "✅ E2E Coverage Collection Complete for $(1)"
	@echo "════════════════════════════════════════════════════════════════════════"
endef

# Quick reference for defining coverage targets:
#
# In your main Makefile, add these lines after including this file:
#
# # Define E2E coverage targets
# $(eval $(call define-e2e-coverage-target,notification,notification,4))
# $(eval $(call define-e2e-coverage-target,datastorage,datastorage,4))
# $(eval $(call define-e2e-coverage-target,gateway,gateway,4))
# $(eval $(call define-e2e-coverage-target,signalprocessing,signalprocessing,4))
# $(eval $(call define-e2e-coverage-target,workflowexecution,workflowexecution,4))
# $(eval $(call define-e2e-coverage-target,remediationorchestrator,remediationorchestrator,4))
# $(eval $(call define-e2e-coverage-target,aianalysis,aianalysis,4))
# $(eval $(call define-e2e-coverage-target,toolset,toolset,4))
#
# This generates targets like:
#   make test-e2e-notification-coverage
#   make test-e2e-datastorage-coverage
#   etc.



