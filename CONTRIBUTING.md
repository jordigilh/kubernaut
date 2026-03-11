# Contributing to Kubernaut

Thank you for your interest in contributing to Kubernaut! This document provides guidelines and instructions for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-username>/kubernaut.git`
3. Create a feature branch: `git checkout -b feature/my-change`
4. Make your changes following the guidelines below
5. Push and open a Pull Request

## Prerequisites

- **Go** 1.25.3+
- **Docker** or **Podman** for container builds
- **Kind** for local Kubernetes cluster testing
- **Helm** 3.x for chart operations
- **kubectl** configured for your cluster

## Development Workflow

Kubernaut follows **strict TDD** with the RED-GREEN-REFACTOR cycle:

1. **RED** -- Write a failing test that defines the expected behavior
2. **GREEN** -- Write the minimal code to make the test pass
3. **REFACTOR** -- Improve code quality while keeping tests green

### Build

```bash
go build ./...
```

### Test

```bash
make test                           # Unit tests
make test-integration-<service>     # Integration tests
make test-e2e-<service>             # E2E tests
```

### Lint

```bash
golangci-lint run --timeout=5m
```

## Code Standards

- Use **Ginkgo/Gomega** BDD framework for all tests (not standard `testing`)
- Handle all errors -- never ignore them
- Wrap errors with context: `fmt.Errorf("description: %w", err)`
- Avoid `any` or `interface{}` unless absolutely necessary
- Follow existing naming conventions and patterns

## Business Requirements

Every code change must map to a business requirement using the format `BR-[CATEGORY]-[NUMBER]` (e.g., `BR-WORKFLOW-001`). Include the BR reference in your PR description.

## Pull Request Process

1. Ensure all tests pass and there are no lint errors
2. Reference the business requirement(s) your change addresses
3. Provide a clear description of what changed and why
4. Update documentation if your change affects public APIs or behavior
5. Request review from a maintainer

## Reporting Issues

Use the [GitHub issue tracker](https://github.com/jordigilh/kubernaut/issues) with the provided templates for bug reports and feature requests.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
