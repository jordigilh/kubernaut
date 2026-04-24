# Git Hooks for HAPI Service

**Phase**: Phase 4 - Automated Spec Validation
**Business Requirement**: BR-HAPI-260 (API Contract Validation)

---

## Available Hooks

### `pre-commit`
Validates OpenAPI spec matches Pydantic models before commit.

**Purpose**:
- Prevents spec/code drift
- Catches missing fields before commit
- Forces spec regeneration after model changes

**Installation**:
```bash
# From holmesgpt-api directory
ln -sf ../../.git-hooks/pre-commit .git/hooks/pre-commit
```

**Behavior**:
- Runs only when Pydantic models (`src/models/*.py`) are modified
- Validates spec matches models
- Fails commit if validation fails
- Provides fix instructions

**Bypass** (use sparingly):
```bash
git commit --no-verify
```

---

## Manual Validation

Run validation script manually:
```bash
python3 scripts/validate-openapi-spec.py
```

---

## CI/CD Integration

Add to `.github/workflows/hapi-ci.yml`:
```yaml
- name: Validate OpenAPI Spec
  run: |
    cd holmesgpt-api
    python3 scripts/validate-openapi-spec.py
```

---

## Troubleshooting

### Hook Not Running
```bash
# Check if hook is installed
ls -la .git/hooks/pre-commit

# Install if missing
ln -sf ../../.git-hooks/pre-commit .git/hooks/pre-commit
```

### Validation Fails
```bash
# Regenerate spec
python3 scripts/generate-openapi-spec.py

# Stage updated spec
git add api/openapi.json

# Commit again
git commit
```

---

**Created**: 2025-12-13
**Authority**: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md (Phase 4)


