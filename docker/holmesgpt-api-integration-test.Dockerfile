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
COPY holmesgpt-api/requirements.txt holmesgpt-api/requirements-test.txt ./holmesgpt-api/
COPY dependencies/holmesgpt/ ./dependencies/holmesgpt/

# Install holmesgpt package first (avoids relative path issues in requirements.txt)
# The requirements.txt line 37 references "../dependencies/holmesgpt/" which doesn't resolve in container context
RUN pip3.12 install --no-cache-dir --break-system-packages ./dependencies/holmesgpt/

# Install remaining Python dependencies
# Filter out the broken relative path line before installing
RUN grep -v "../dependencies/holmesgpt" holmesgpt-api/requirements.txt > /tmp/requirements-filtered.txt && \
	pip3.12 install --no-cache-dir --break-system-packages \
	-r /tmp/requirements-filtered.txt \
	-r holmesgpt-api/requirements-test.txt

# Copy application code
COPY holmesgpt-api/ ./holmesgpt-api/
COPY docs/ ./docs/

# Copy test fixtures and configuration
COPY holmesgpt-api/config.yaml ./holmesgpt-api/config.yaml

# Set environment variables for integration tests
ENV PYTHONPATH=/workspace/holmesgpt-api
ENV CONFIG_FILE=/workspace/holmesgpt-api/config.yaml
ENV MOCK_LLM_MODE=true

# Integration test ports (DD-TEST-001)
ENV HAPI_INTEGRATION_PORT=18120
ENV DS_INTEGRATION_PORT=18098
ENV PG_INTEGRATION_PORT=15439
ENV REDIS_INTEGRATION_PORT=16387
ENV HAPI_URL=http://host.containers.internal:18120
ENV DATA_STORAGE_URL=http://host.containers.internal:18098

# Default command: Run integration tests with pytest
WORKDIR /workspace/holmesgpt-api
CMD ["python3.12", "-m", "pytest", "tests/integration/", "-n", "4", "-v", "--tb=short", "--no-cov"]

