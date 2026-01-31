# HolmesGPT API Integration Test Container
# Purpose: Run HAPI integration tests in isolated, reproducible environment
# Pattern: DD-INTEGRATION-001 v2.0 (Go infrastructure + Python tests)
# Matches Dockerfile.e2e Python version for consistency

FROM registry.access.redhat.com/ubi9/python-312:latest

USER root

# Install system dependencies
# Note: --allowerasing handles curl-minimal vs curl conflict in UBI9
RUN dnf install -y --allowerasing \
	git \
	curl \
	which \
	&& dnf clean all

# Set working directory
WORKDIR /workspace

# Copy Python dependencies first for layer caching
# Using requirements-e2e.txt for faster builds (no google-cloud-aiplatform 1.5GB)
COPY holmesgpt-api/requirements-e2e.txt holmesgpt-api/requirements-test.txt ./holmesgpt-api/
COPY dependencies/holmesgpt ./dependencies/holmesgpt

# Copy DataStorage OpenAPI client (referenced in requirements-e2e.txt)
# Destination must match PYTHONPATH: /workspace/holmesgpt-api/datastorage/
# Tests import as: from datastorage import ApiClient
COPY holmesgpt-api/src/clients/datastorage ./holmesgpt-api/datastorage

# Install holmesgpt package first (avoids relative path issues in requirements.txt)
# The requirements-e2e.txt line references "../dependencies/holmesgpt/" which doesn't resolve in container context
RUN pip3.12 install --no-cache-dir --break-system-packages /workspace/dependencies/holmesgpt

# Install remaining Python dependencies
# Filter out the broken relative path line before installing
# Using requirements-e2e.txt (minimal deps) instead of requirements.txt (full deps)
RUN grep -v "../dependencies/holmesgpt" holmesgpt-api/requirements-e2e.txt > /tmp/requirements-filtered.txt && \
	pip3.12 install --no-cache-dir --break-system-packages \
	-r /tmp/requirements-filtered.txt \
	-r holmesgpt-api/requirements-test.txt

# Copy application code
COPY holmesgpt-api/ ./holmesgpt-api/
COPY docs/ ./docs/

# Copy test fixtures and configuration
COPY holmesgpt-api/config.yaml ./holmesgpt-api/config.yaml

# NOTE: OpenAPI client is generated on HOST before docker build (see Makefile)
# Rationale: generate-client.sh requires podman/docker which is not available in container
# Pattern: Matches E2E test approach (DD-API-001 compliance)

# Set environment variables for integration tests
ENV PYTHONPATH=/workspace/holmesgpt-api
ENV CONFIG_FILE=/workspace/holmesgpt-api/config.yaml
ENV MOCK_LLM_MODE=true

# Integration test ports (DD-TEST-001)
ENV HAPI_INTEGRATION_PORT=18120
ENV DS_INTEGRATION_PORT=18098
ENV PG_INTEGRATION_PORT=15439
ENV REDIS_INTEGRATION_PORT=16387
# Use 127.0.0.1 (IPv4) because CI resolves localhost to IPv6 (::1) but services bind to IPv4
# Makefile runs container with --network=host giving direct access to host's network stack
ENV HAPI_URL=http://127.0.0.1:18120
ENV DATA_STORAGE_URL=http://127.0.0.1:18098

# Default command: Run integration tests with pytest
WORKDIR /workspace/holmesgpt-api
CMD ["python3.12", "-m", "pytest", "tests/integration/", "-n", "4", "-v", "--tb=short", "--no-cov"]

