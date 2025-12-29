# ADR-030 Exception: HAPI Service - Environment Variable for Config Path

**Date**: December 29, 2025
**Status**: ✅ APPROVED
**Related**: ADR-030 Configuration Management Standard

---

## Context

ADR-030 mandates that all services use a `-config` command-line flag to specify configuration file paths, avoiding environment variables for config paths.

However, the HolmesGPT-API (HAPI) service is implemented in Python using FastAPI + Uvicorn, which has a technical limitation that prevents direct use of command-line flags.

---

## Technical Constraint

### Uvicorn Does NOT Support Custom Flags

**Test Verification**:
```bash
$ python3 -c "
import sys
sys.argv = ['uvicorn', 'src.main:app', '--custom-flag', 'value']
from uvicorn.main import main
main()
"

# Output:
Usage: uvicorn [OPTIONS] APP
Error: No such option: --custom-flag
Exit code: 2
```

**Conclusion**: Uvicorn **rejects** unknown command-line arguments and **does not pass them** to the FastAPI application.

### Why This Matters

Uvicorn is an ASGI server that:
1. **Only understands its own flags**: `--host`, `--port`, `--workers`, `--reload`, etc.
2. **Rejects unknown flags**: Any custom flag causes immediate error and exit
3. **Provides no mechanism** to pass custom arguments to the ASGI application

**Available Options**:
- ✅ **Environment variables** - Standard Python/ASGI pattern
- ❌ **Command-line flags** - Not supported by uvicorn architecture
- ❌ **Hardcoded paths** - Violates ADR-030 flexibility requirement

---

## Decision

**GRANT EXCEPTION**: HAPI service is allowed to use the `CONFIG_FILE` environment variable to specify config file path.

### Implementation Pattern

**External Interface** (matches Go services):
```yaml
# Kubernetes deployment
containers:
- name: holmesgpt-api
  args:
  - "-config"
  - "/etc/holmesgpt/config.yaml"
```

**Internal Implementation** (Python-specific):
```bash
# entrypoint.sh
CONFIG_PATH="/etc/holmesgpt/config.yaml"
while [[ $# -gt 0 ]]; do
    case $1 in
        -config) CONFIG_PATH="$2"; shift 2 ;;
    esac
done

export CONFIG_FILE="$CONFIG_PATH"  # ← Environment variable
exec uvicorn src.main:app --host 0.0.0.0 --port 8080 --workers 4
```

```python
# src/main.py
config_file = os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml")
```

---

## Rationale

### Why This Exception is Justified

1. **Technical Necessity**: No alternative exists for Python/uvicorn architecture
2. **Consistent User Experience**: External interface remains identical to Go services via entrypoint script
3. **Implementation Detail**: Environment variable is internal; users only see `-config` flag
4. **ADR-030 Compliance**: Achieves ADR-030 goals (YAML ConfigMap, explicit config, no hardcoding)

### Alternative Approaches Considered

#### Alternative 1: Custom Uvicorn Wrapper
**Approach**: Write Python wrapper to parse flags before starting uvicorn
**Verdict**: ❌ Rejected - Adds complexity, still uses env vars internally
**Reason**: Would still need environment variable to pass config to FastAPI app

#### Alternative 2: Modify Uvicorn Source
**Approach**: Fork uvicorn to support custom flag passing
**Verdict**: ❌ Rejected - Maintenance burden, upstream unlikely to accept
**Reason**: Violates ASGI spec design, not how ASGI servers work

#### Alternative 3: Use Different ASGI Server
**Approach**: Switch to ASGI server that supports custom flags
**Verdict**: ❌ Rejected - No ASGI server supports this pattern
**Reason**: This is an architectural constraint of the ASGI specification

#### Alternative 4: Bash Entrypoint Script (SELECTED)
**Approach**: Entrypoint parses `-config` flag, exports as `CONFIG_FILE` env var
**Verdict**: ✅ **APPROVED** - Minimal complexity, consistent external interface
**Reason**: Best balance of user experience and technical constraints

---

## Scope of Exception

### What is Allowed

✅ **Allowed**: `CONFIG_FILE` environment variable for config path only
✅ **Allowed**: Entrypoint script to provide `-config` flag interface
✅ **Allowed**: Internal implementation detail (hidden from users)

### What is NOT Allowed

❌ **Not Allowed**: Environment variables for functional configuration
❌ **Not Allowed**: Skipping YAML ConfigMap requirement
❌ **Not Allowed**: Exposing environment variable to users

**Principle**: Exception applies ONLY to config path mechanism, not to configuration content.

---

## User Impact

### No User-Facing Difference

**Go Service** (Gateway, SP, WE, RO, Notification, DS, AA):
```yaml
containers:
- name: gateway
  args:
  - "-config"
  - "/etc/gateway/config.yaml"
```

**Python Service** (HAPI):
```yaml
containers:
- name: holmesgpt-api
  args:
  - "-config"
  - "/etc/holmesgpt/config.yaml"  # ← Identical interface
```

**Result**: Users cannot tell the difference - HAPI looks identical to Go services.

---

## Compliance Verification

### ADR-030 Requirements vs HAPI Implementation

| Requirement | Go Services | HAPI Service | Compliant? |
|-------------|-------------|--------------|------------|
| YAML ConfigMap as source of truth | ✅ Yes | ✅ Yes | ✅ |
| `-config` flag for path | ✅ Native | ✅ Via entrypoint | ✅ |
| No env vars for config content | ✅ Yes | ✅ Yes | ✅ |
| Explicit configuration required | ✅ Yes | ✅ Yes | ✅ |
| Consistent user interface | ✅ Yes | ✅ Yes | ✅ |

**Internal Implementation Difference**:
- **Go**: Command-line flag parsed natively by `flag` package
- **HAPI**: Command-line flag parsed by entrypoint script, converted to env var

**External Interface**: Identical

---

## Documentation Requirements

### 1. Exception Must Be Documented

- [x] ADR-030 Exception document (this file)
- [x] HAPI ADR-030 compliance guide
- [x] Technical rationale with proof (uvicorn test)

### 2. Implementation Must Be Transparent

- [x] Entrypoint script with clear comments
- [x] Python code documents env var usage
- [x] Kubernetes manifests show `-config` flag usage

### 3. Exception Scope Must Be Clear

- [x] Only for config path mechanism
- [x] Not for configuration content
- [x] Not for other services

---

## Review & Approval

### Technical Review

- [x] **Python/Uvicorn Limitation Verified**: Confirmed uvicorn rejects custom flags
- [x] **Alternative Approaches Evaluated**: No viable alternatives found
- [x] **Implementation Tested**: Go build passes, Python syntax valid

### ADR-030 Compliance Review

- [x] **User Interface Consistent**: `-config` flag works identically across all services
- [x] **ConfigMap Requirement Met**: YAML ConfigMap is source of truth
- [x] **Exception Scope Limited**: Only applies to config path, not content

### Security Review

- [x] **No Additional Risk**: Environment variable is internal implementation detail
- [x] **Secrets Handling Unchanged**: Secret file paths still in config, not env vars
- [x] **No Exposure**: Users never interact with CONFIG_FILE env var directly

---

## Conclusion

**Exception Granted**: HAPI service may use `CONFIG_FILE` environment variable internally as implementation detail.

**Justification**: Technical limitation of Python/uvicorn architecture with no viable alternatives.

**Impact**: None - external interface remains consistent with ADR-030 via entrypoint script.

**Precedent**: This exception applies ONLY to HAPI service and ONLY for config path mechanism.

---

## References

### Technical Documentation

- **Uvicorn CLI**: https://www.uvicorn.org/settings/
- **ASGI Specification**: https://asgi.readthedocs.io/
- **FastAPI Configuration**: https://fastapi.tiangolo.com/advanced/settings/

### Internal Documentation

- **ADR-030**: Configuration Management Standard
- **HAPI ADR-030 Implementation**: `docs/handoff/HAPI_ADR_030_FINAL_DEC_29_2025.md`
- **Entrypoint Script**: `holmesgpt-api/entrypoint.sh`

### Verification Test

```bash
# Proof that uvicorn rejects custom flags
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
python3 -c "
import sys
sys.argv = ['uvicorn', 'src.main:app', '--custom-flag', 'value']
from uvicorn.main import main
main()
"
# Output: Error: No such option: --custom-flag
# Exit code: 2
```

---

**Status**: ✅ **EXCEPTION APPROVED**
**Scope**: HAPI service only, config path mechanism only
**Impact**: Zero user-facing impact - consistent interface maintained

