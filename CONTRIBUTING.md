# Contributing to kubernaut-apifrontend

## Branch Conventions

| Branch | Purpose |
|--------|---------|
| `main` | Protected. Requires PR review + passing CI. |
| `feature/<issue>-<short-desc>` | Feature work |
| `fix/<issue>-<short-desc>` | Bug fixes |
| `chore/<short-desc>` | Maintenance (deps, CI, docs) |

## Development Workflow

1. Create a branch from `main`
2. Write tests first (TDD Red → Green → Refactor)
3. Ensure `make lint` and `make test-unit` pass locally
4. Push and open a PR against `main`

## Commit Style

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(bridge): add timeout metric distinction for tool calls
fix(auth): handle nil user identity in RBAC check
test(bridge): add tier-3 concurrency tests
docs(adr): document RBAC runtime enforcement decision
chore(deps): bump go.uber.org/zap to v1.27.1
```

## Pull Request Requirements

- All CI checks must pass (build, lint, test, security, maturity)
- Minimum 80% test coverage on changed packages
- At least one approving review
- Squash merge preferred for single-concern PRs

## Testing

We use [Ginkgo](https://onsi.github.io/ginkgo/) + [Gomega](https://onsi.github.io/gomega/) with the `-race` flag always enabled.

```bash
make test-unit                     # All unit tests
make test-bridge                   # MCP bridge tests only
GINKGO_LABEL=tier1 make test-bridge  # Filter by tier
make test-all                      # Unit + integration
```

### Test Tiers

| Tier | Scope | Labels |
|------|-------|--------|
| 1 | Happy-path tool dispatch | `tier1, bridge` |
| 2 | RBAC enforcement | `tier2, bridge` |
| 3 | Error paths, timeouts, panics | `tier3, bridge` |
| 4 | Observability (metrics, audit, logs) | `tier4, bridge` |
| 5 | Concurrency, adversarial inputs | `tier5, bridge` |

### Anti-patterns to Avoid

- No `time.Sleep` in tests (use Eventually/Consistently)
- No shared mutable state between specs (use BeforeEach)
- No testing internal implementation details (test behavior)
- Reference: [100 Go Mistakes](https://github.com/teivah/100-go-mistakes)

## Code Generation

After modifying CRD types in `api/`:

```bash
make generate
make verify-generate  # CI runs this to ensure generated code is committed
```

## Logging

- Application logging: `logr` interface backed by `zapr` (zap)
- Bridge internal: `*slog.Logger` (JSON output)
- Never log secrets, tokens, or full request bodies
- Use structured fields: `"tool", toolName, "user", username, "error", err`

## Security

- All string inputs from external sources must be validated (`internal/validate/`)
- Errors returned to clients must be redacted (`internal/security/redact.go`)
- No `os.Getenv` in production code — all config via files
- Run `gosec` and `govulncheck` locally before pushing security-sensitive changes
