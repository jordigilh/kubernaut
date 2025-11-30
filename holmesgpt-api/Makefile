# HolmesGPT API Makefile
#
# Business Requirement: BR-HAPI-250 - Workflow Catalog Integration
# Design Decision: DD-WORKFLOW-002 v2.4 - Generated models with container_image
#
# Usage:
#   make generate-models  - Regenerate Data Storage client models from OpenAPI spec
#   make test             - Run all tests
#   make test-unit        - Run unit tests only
#   make test-integration - Run integration tests only
#   make lint             - Run linter
#   make clean            - Clean generated files

.PHONY: generate-models test test-unit test-integration lint clean help

# OpenAPI spec location
OPENAPI_SPEC := ../docs/services/stateless/data-storage/openapi/v3.yaml
MODELS_OUTPUT := src/clients/datastorage/models.py

# Default target
help:
	@echo "HolmesGPT API Build Targets"
	@echo ""
	@echo "  make generate-models  - Regenerate Data Storage client models from OpenAPI spec"
	@echo "  make test             - Run all tests with coverage"
	@echo "  make test-unit        - Run unit tests only"
	@echo "  make test-integration - Run integration tests only"
	@echo "  make lint             - Run linter (ruff)"
	@echo "  make clean            - Clean generated files"
	@echo ""

# Generate Pydantic models from OpenAPI spec
# DD-WORKFLOW-002 v2.4: Ensures container_image and container_digest are included
generate-models:
	@echo "📦 Installing datamodel-code-generator..."
	pip3 install datamodel-code-generator --index-url https://pypi.org/simple/ --quiet
	@echo "🔄 Generating models from OpenAPI spec: $(OPENAPI_SPEC)"
	python3 -m datamodel_code_generator \
		--input $(OPENAPI_SPEC) \
		--output $(MODELS_OUTPUT) \
		--output-model-type pydantic_v2.BaseModel \
		--use-double-quotes \
		--use-annotated \
		--field-constraints \
		--use-standard-collections \
		--target-python-version 3.11 \
		--encoding utf-8
	@echo "✅ Models generated: $(MODELS_OUTPUT)"
	@echo "⚠️  Note: DataStorageError class is in client.py (not auto-generated)"

# Run all tests with coverage
test:
	python3 -m pytest tests/ -v --tb=short --cov=src --cov-report=html -p 4

# Run unit tests only (parallel execution per DD-TEST-002)
test-unit:
	python3 -m pytest tests/unit/ -v --tb=short --cov=src -p 4

# Run integration tests only
test-integration:
	python3 -m pytest tests/integration/ -v --tb=short

# Run linter
lint:
	@echo "🔍 Running ruff linter..."
	ruff check src/ tests/
	@echo "✅ Linting complete"

# Clean generated files
clean:
	rm -rf htmlcov/
	rm -rf .pytest_cache/
	rm -rf __pycache__/
	find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@echo "🧹 Cleaned generated files"

