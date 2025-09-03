# Multi-stage Dockerfile for HolmesGPT REST API

# Stage 1: Base Python environment
FROM python:3.11-slim as base

# Set environment variables
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    POETRY_VENV_IN_PROJECT=1 \
    POETRY_NO_INTERACTION=1 \
    POETRY_CACHE_DIR=/tmp/poetry_cache

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    git \
    libpq-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# Stage 2: Dependencies installation
FROM base as dependencies

# Install Poetry
RUN pip install poetry==1.7.1

# Copy dependency files
WORKDIR /app
COPY pyproject.toml poetry.lock* ./

# Configure poetry and install dependencies
RUN poetry config virtualenvs.create false \
    && poetry install --no-dev --no-root \
    && rm -rf $POETRY_CACHE_DIR

# Stage 3: Development image
FROM dependencies as development

# Install development dependencies
RUN poetry install --no-root && rm -rf $POETRY_CACHE_DIR

# Copy source code
COPY . .

# Create non-root user
RUN useradd --create-home --shell /bin/bash holmesgpt
RUN chown -R holmesgpt:holmesgpt /app
USER holmesgpt

# Expose ports
EXPOSE 8000 9090

# Development command
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--reload"]

# Stage 4: Production image
FROM dependencies as production

# Install HolmesGPT CLI (adjust URL as needed)
RUN curl -L https://github.com/robusta-dev/holmesgpt/releases/latest/download/holmes-linux-amd64 -o /usr/local/bin/holmes \
    && chmod +x /usr/local/bin/holmes \
    || echo "HolmesGPT CLI not available - will use fallback mode"

# Copy source code
COPY app/ /app/app/
COPY pyproject.toml README.md ./

# Create non-root user
RUN useradd --create-home --shell /bin/bash --uid 1000 holmesgpt \
    && mkdir -p /app/logs /app/data /app/tmp \
    && chown -R holmesgpt:holmesgpt /app

USER holmesgpt

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Expose ports
EXPOSE 8000 9090

# Production command
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "4"]

# Stage 5: Ollama integration image
FROM production as ollama-integration

# Install Ollama
RUN curl -fsSL https://ollama.com/install.sh | sh \
    || echo "Ollama installation failed - continuing without"

# Set up Ollama environment
ENV OLLAMA_HOST=0.0.0.0:11434
ENV HOLMES_OLLAMA_URL=http://localhost:11434

# Expose Ollama port
EXPOSE 11434

# Custom entrypoint for Ollama integration
COPY docker/entrypoint-ollama.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "2"]

