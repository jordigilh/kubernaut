# HolmesGPT API Makefile
#
# Business Requirement: BR-HAPI-250 - Workflow Catalog Integration
# Design Decision: DD-WORKFLOW-002 v2.4 - Generated models with container_image
# ADR-045: OpenAPI spec export and CI validation
#
# Usage:
#   make generate-models  - Regenerate Data Storage client models from OpenAPI spec
#   make export-openapi   - Export OpenAPI spec from FastAPI app (ADR-045)
#   make validate-openapi - Validate OpenAPI spec consistency (CI)
#   make test             - Run all tests
#   make test-unit        - Run unit tests only
#   make test-integration - Run integration tests only
#   make lint             - Run linter
#   make clean            - Clean generated files

.PHONY: generate-models export-openapi validate-openapi test test-unit test-integration lint clean help

# OpenAPI spec locations (ADR-045)
DS_OPENAPI_SPEC := ../docs/services/stateless/data-storage/openapi/v3.yaml
HAPI_OPENAPI_OUTPUT := api/openapi.json
MODELS_OUTPUT := src/clients/datastorage/models.py

# Default target
help:
	@echo "HolmesGPT API Build Targets"
	@echo ""
	@echo "  make generate-models  - Regenerate Data Storage client models from OpenAPI spec"
	@echo "  make export-openapi   - Export OpenAPI spec from FastAPI app (ADR-045)"
	@echo "  make validate-openapi - Validate OpenAPI spec consistency (CI)"
	@echo "  make test             - Run all tests with coverage"
	@echo "  make test-unit        - Run unit tests only (CI Tier 1)"
	@echo "  make test-integration - Run integration tests only (CI Tier 2)"
	@echo "  make lint             - Run linter (ruff)"
	@echo "  make clean            - Clean generated files"
	@echo ""

# Export OpenAPI spec from FastAPI application (ADR-045)
# This generates the authoritative OpenAPI spec from the running application
export-openapi:
	@echo "📄 Exporting OpenAPI spec from FastAPI app..."
	@mkdir -p api
	python3 -c "from src.main import app; import json; print(json.dumps(app.openapi(), indent=2))" > $(HAPI_OPENAPI_OUTPUT)
	@echo "✅ OpenAPI spec exported: $(HAPI_OPENAPI_OUTPUT)"
	@echo "📊 Schema count: $$(python3 -c \"import json; spec=json.load(open('$(HAPI_OPENAPI_OUTPUT)')); print(len(spec.get('components', {}).get('schemas', {})))\")"

# Validate OpenAPI spec consistency (CI validation per ADR-045)
# Ensures exported spec matches committed spec
validate-openapi: export-openapi
	@echo "🔍 Validating OpenAPI spec consistency..."
	@if [ -f "$(HAPI_OPENAPI_OUTPUT).committed" ]; then \
		diff -q $(HAPI_OPENAPI_OUTPUT) $(HAPI_OPENAPI_OUTPUT).committed > /dev/null 2>&1 || \
		(echo "❌ OpenAPI spec drift detected! Run 'make export-openapi' and commit changes." && exit 1); \
		echo "✅ OpenAPI spec is consistent"; \
	else \
		echo "⚠️  No committed spec found. Copying current as baseline..."; \
		cp $(HAPI_OPENAPI_OUTPUT) $(HAPI_OPENAPI_OUTPUT).committed; \
	fi

# Generate Pydantic models from OpenAPI spec
# DD-WORKFLOW-002 v2.4: Ensures container_image and container_digest are included
generate-models:
	@echo "📦 Installing datamodel-code-generator..."
	pip3 install datamodel-code-generator --index-url https://pypi.org/simple/ --quiet
	@echo "🔄 Generating models from OpenAPI spec: $(DS_OPENAPI_SPEC)"
	python3 -m datamodel_code_generator \
		--input $(DS_OPENAPI_SPEC) \
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
	python3 -m pytest tests/ -v --tb=short --cov=src --cov-report=html

# Run unit tests only (CI Tier 1)
test-unit:
	python3 -m pytest tests/unit/ -v --tb=short --cov=src

# Run integration tests only (CI Tier 2)
test-integration:
	python3 -m pytest tests/integration/ -v --tb=short

# Run E2E tests (CI Tier 3 - requires Go infrastructure)
# Usage: First run `make test-e2e-datastorage` from project root
test-e2e:
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 HolmesGPT API - E2E Tests (requires Go infrastructure)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@curl -s http://localhost:8081/health/ready > /dev/null 2>&1 || \
		(echo "❌ Data Storage not available at localhost:8081" && \
		 echo "   Run 'make test-e2e-datastorage' from project root first" && \
		 exit 1)
	@echo "✅ Data Storage available"
	DATA_STORAGE_URL=http://localhost:8081 MOCK_LLM=true HAPI_USE_GO_INFRA=true \
		python3 -m pytest tests/e2e/ -v --tb=short

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

