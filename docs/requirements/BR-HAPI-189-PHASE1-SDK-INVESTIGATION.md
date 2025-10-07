# BR-HAPI-189 Phase 1: HolmesGPT SDK Investigation

**Phase**: Phase 1 - SDK Capability Assessment
**Date**: 2025-01-15
**Status**: IN PROGRESS
**Purpose**: Investigate HolmesGPT SDK to inform structured error handling implementation

---

## Investigation Objectives

**Primary Goal**: Determine HolmesGPT SDK's capabilities for structured error tracking to replace string-matching approach in BR-HAPI-189.

**Key Questions to Answer**:
1. Does HolmesGPT SDK provide toolset execution callbacks/hooks?
2. Does HolmesGPT SDK provide custom exception hierarchy?
3. Do investigation results include toolset execution metadata?
4. What error information is available in exceptions?
5. Can we extend/wrap the SDK to add structured error tracking?

---

## Investigation Tasks

### Task 1: SDK Source Code Analysis

**Objective**: Examine HolmesGPT Python SDK source code structure

**Actions**:
```bash
# Clone HolmesGPT repository
git clone https://github.com/robusta-dev/holmesgpt.git
cd holmesgpt

# Examine SDK structure
find . -name "*.py" | grep -E "(client|exception|error|toolset)"

# Look for Python SDK entry points
cat setup.py  # or pyproject.toml
grep -r "class.*Client" --include="*.py"
grep -r "class.*Error\|class.*Exception" --include="*.py"
```

**Expected Findings**:
- Location of `Client` class (main SDK entry point)
- Exception hierarchy (if any)
- Toolset implementation files

**Documentation Location**: `holmesgpt/holmes/` or `holmesgpt/src/`

---

### Task 2: Exception Hierarchy Investigation

**Objective**: Identify custom exception types provided by HolmesGPT SDK

**Actions**:
```bash
# Search for exception definitions
cd holmesgpt
grep -r "class.*Exception\|class.*Error" --include="*.py" -A 5

# Search for exception raising
grep -r "raise.*Error\|raise.*Exception" --include="*.py" | head -20

# Check for toolset-specific exceptions
grep -r "ToolsetError\|KubernetesError\|PrometheusError" --include="*.py"
```

**Questions to Answer**:
- [ ] Does SDK define `ToolsetExecutionError`?
- [ ] Does SDK define toolset-specific exceptions (e.g., `KubernetesToolsetError`)?
- [ ] Do exceptions have metadata attributes (e.g., `toolset_name`, `error_code`)?
- [ ] Are exceptions well-documented?

**Example Expected Structure**:
```python
# Best case: Structured exception hierarchy
class HolmesGPTException(Exception):
    """Base exception for HolmesGPT"""
    pass

class ToolsetExecutionError(HolmesGPTException):
    """Base exception for toolset execution failures"""
    def __init__(self, toolset_name: str, message: str, **kwargs):
        self.toolset_name = toolset_name
        self.error_metadata = kwargs
        super().__init__(message)

class KubernetesToolsetError(ToolsetExecutionError):
    """Kubernetes toolset-specific error"""
    pass

class PrometheusToolsetError(ToolsetExecutionError):
    """Prometheus toolset-specific error"""
    pass
```

---

### Task 3: Client API Investigation

**Objective**: Understand `Client.investigate()` method signature and return types

**Actions**:
```bash
# Find Client class definition
cd holmesgpt
grep -r "class Client" --include="*.py" -A 50

# Find investigate() method
grep -r "def investigate" --include="*.py" -A 30

# Check return type annotations
grep -r "-> .*Result\|-> .*Response" --include="*.py"
```

**Questions to Answer**:
- [ ] What does `investigate()` return? (object type, structure)
- [ ] Does return object include `toolsets_used`?
- [ ] Does return object include toolset execution details?
- [ ] Are there async variants (`async def investigate`)?

**Example Expected Return Object**:
```python
@dataclass
class InvestigationResult:
    """Result of HolmesGPT investigation"""
    analysis: str
    root_cause: Optional[str]
    recommendations: List[str]
    confidence: float

    # CRITICAL: Does it include toolset metadata?
    toolsets_used: List[str]  # e.g., ["kubernetes", "prometheus"]

    # OPTIONAL: Detailed execution info
    toolset_execution_details: Optional[List[ToolsetExecutionDetail]] = None
```

---

### Task 4: Toolset Implementation Investigation

**Objective**: Understand how toolsets are implemented and executed

**Actions**:
```bash
# Find toolset implementations
cd holmesgpt
find . -name "*toolset*.py"
grep -r "class.*Toolset" --include="*.py" -A 20

# Check for callback/hook mechanisms
grep -r "callback\|hook\|register.*toolset" --include="*.py"

# Check for execution context tracking
grep -r "execution.*context\|current.*toolset" --include="*.py"
```

**Questions to Answer**:
- [ ] How are toolsets registered/configured?
- [ ] Is there a callback mechanism for toolset execution?
- [ ] Can we hook into toolset execution lifecycle?
- [ ] How do toolsets report errors?

**Example Expected Toolset Structure**:
```python
class Toolset(ABC):
    """Base class for HolmesGPT toolsets"""

    @abstractmethod
    def execute(self, context: InvestigationContext) -> ToolsetResult:
        """Execute toolset operation"""
        pass

    # CRITICAL: Does it support callbacks?
    def on_error(self, error: Exception):
        """Optional error callback"""
        pass
```

---

### Task 5: SDK Extension Points Investigation

**Objective**: Identify ways to extend SDK for structured error tracking

**Actions**:
```bash
# Check for plugin/extension mechanisms
cd holmesgpt
grep -r "plugin\|extension\|middleware" --include="*.py"

# Check for monkey-patching opportunities
grep -r "__init__.*Client" --include="*.py" -A 10

# Check for decorators/wrappers
grep -r "@.*decorator\|wrap" --include="*.py"
```

**Questions to Answer**:
- [ ] Can we subclass `Client` to add error tracking?
- [ ] Can we wrap toolset execution methods?
- [ ] Are there middleware/plugin extension points?
- [ ] Can we monkey-patch safely for error tracking?

---

## Investigation Results Template

### Finding 1: Exception Hierarchy

**Status**: ⏳ PENDING INVESTIGATION

**What We Found**:
```
[To be filled after investigation]

Example:
- SDK provides base `HolmesGPTException` class
- NO toolset-specific exception classes found
- Exceptions do NOT include toolset_name metadata
- All errors raised as generic Exception with string messages
```

**Implications**:
```
[To be filled]

Example:
- Cannot rely on exception types for toolset classification
- Must use alternative approach (traceback analysis, pattern matching)
- Consider submitting PR to HolmesGPT to add structured exceptions
```

**Code Examples**:
```python
# [Paste actual code found in SDK]
```

---

### Finding 2: Investigation Results Metadata

**Status**: ⏳ PENDING INVESTIGATION

**What We Found**:
```
[To be filled after investigation]

Example:
- investigate() returns InvestigationResult object
- Result includes 'toolsets_used' field (List[str])
- Result does NOT include execution details
- Result does NOT include error information per toolset
```

**Implications**:
```
[To be filled]

Example:
- Can track which toolsets were used on success
- Cannot identify which toolset failed on error
- Must implement external error tracking
```

**Code Examples**:
```python
# [Paste actual InvestigationResult class]
```

---

### Finding 3: Toolset Callbacks/Hooks

**Status**: ⏳ PENDING INVESTIGATION

**What We Found**:
```
[To be filled after investigation]

Example:
- NO callback mechanism found
- NO hook registration API
- Toolsets executed directly without lifecycle hooks
- Cannot register custom error handlers
```

**Implications**:
```
[To be filled]

Example:
- Cannot use callbacks for real-time error tracking
- Must catch exceptions at Client.investigate() level
- Consider contributing callback feature to HolmesGPT
```

---

### Finding 4: Extension Points

**Status**: ⏳ PENDING INVESTIGATION

**What We Found**:
```
[To be filled after investigation]

Example:
- Client class CAN be subclassed
- investigate() method is NOT final/private
- Can wrap Client with decorator pattern
- Can use Python contextlib for execution tracking
```

**Implications**:
```
[To be filled]

Example:
- RECOMMENDED: Create HolmesGPTClientWrapper with error tracking
- Can add structured error handling without modifying SDK
- Maintains compatibility with SDK updates
```

**Implementation Approach**:
```python
# [Paste proposed wrapper implementation]
```

---

## Practical Investigation Script

**Purpose**: Automated script to investigate SDK capabilities

```python
#!/usr/bin/env python3
"""
HolmesGPT SDK Investigation Script

Usage:
    python investigate_holmesgpt_sdk.py

Requirements:
    pip install holmes-ai  # or appropriate package name
"""

import inspect
import sys
from typing import Any

def investigate_sdk():
    """Investigate HolmesGPT SDK capabilities"""

    print("=" * 80)
    print("HolmesGPT SDK Investigation Report")
    print("=" * 80)

    # Task 1: Import SDK
    try:
        from holmes import Client
        print("\n✅ Task 1: SDK Import Successful")
        print(f"   Client class: {Client.__module__}.{Client.__name__}")
    except ImportError as e:
        print(f"\n❌ Task 1: SDK Import Failed: {e}")
        print("   Action: Install HolmesGPT SDK: pip install holmes-ai")
        return

    # Task 2: Inspect Client class
    print("\n" + "=" * 80)
    print("Task 2: Client Class Inspection")
    print("=" * 80)

    print("\nClient Methods:")
    for name, method in inspect.getmembers(Client, predicate=inspect.ismethod):
        if not name.startswith('_'):
            sig = inspect.signature(method)
            print(f"  - {name}{sig}")

    # Task 3: Check for investigate() method
    if hasattr(Client, 'investigate'):
        print("\n✅ Client.investigate() method found")
        sig = inspect.signature(Client.investigate)
        print(f"   Signature: investigate{sig}")

        # Check return type annotation
        if sig.return_annotation != inspect.Signature.empty:
            print(f"   Return type: {sig.return_annotation}")
        else:
            print("   ⚠️  No return type annotation found")
    else:
        print("\n❌ Client.investigate() method NOT found")

    # Task 4: Look for exception classes
    print("\n" + "=" * 80)
    print("Task 4: Exception Hierarchy Investigation")
    print("=" * 80)

    try:
        import holmes
        exception_classes = [
            name for name in dir(holmes)
            if 'error' in name.lower() or 'exception' in name.lower()
        ]

        if exception_classes:
            print("\n✅ Found exception classes:")
            for exc_name in exception_classes:
                exc_class = getattr(holmes, exc_name)
                if inspect.isclass(exc_class):
                    bases = [base.__name__ for base in exc_class.__bases__]
                    print(f"  - {exc_name} (inherits: {', '.join(bases)})")

                    # Check for metadata attributes
                    if hasattr(exc_class, '__init__'):
                        sig = inspect.signature(exc_class.__init__)
                        print(f"    __init__{sig}")
        else:
            print("\n⚠️  No exception classes found in holmes module")
            print("   SDK likely uses standard Python exceptions")
    except Exception as e:
        print(f"\n❌ Error inspecting exceptions: {e}")

    # Task 5: Check for toolset-related classes
    print("\n" + "=" * 80)
    print("Task 5: Toolset Implementation Investigation")
    print("=" * 80)

    try:
        import holmes
        toolset_classes = [
            name for name in dir(holmes)
            if 'toolset' in name.lower()
        ]

        if toolset_classes:
            print("\n✅ Found toolset-related classes:")
            for name in toolset_classes:
                obj = getattr(holmes, name)
                print(f"  - {name} ({type(obj).__name__})")
        else:
            print("\n⚠️  No toolset classes exposed in public API")
    except Exception as e:
        print(f"\n❌ Error inspecting toolsets: {e}")

    # Task 6: Test actual SDK usage (if credentials available)
    print("\n" + "=" * 80)
    print("Task 6: Runtime Behavior Test (Optional)")
    print("=" * 80)
    print("\n⚠️  Skipping runtime test (requires API key and K8s cluster)")
    print("   To test: Set LLM_API_KEY environment variable and run investigate()")

    # Summary
    print("\n" + "=" * 80)
    print("Investigation Summary")
    print("=" * 80)
    print("""
Next Steps:
1. Review findings above
2. Examine SDK source code directly: https://github.com/robusta-dev/holmesgpt
3. Document findings in BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md
4. Determine implementation approach based on SDK capabilities
    """)

if __name__ == "__main__":
    investigate_sdk()
```

**Run Instructions**:
```bash
# Install HolmesGPT SDK
pip install holmes-ai  # Verify actual package name

# Run investigation script
python investigate_holmesgpt_sdk.py > sdk_investigation_report.txt

# Review findings
cat sdk_investigation_report.txt
```

---

## Decision Matrix

Based on investigation findings, choose implementation approach:

| SDK Capability | Implementation Approach | Confidence |
|----------------|------------------------|------------|
| **Structured exceptions with metadata** | Use exception types directly for classification | 95% |
| **Investigation results include toolset metadata** | Use result metadata for success tracking | 90% |
| **Callback/hook mechanism available** | Register callbacks for real-time tracking | 90% |
| **No structured exceptions, generic Exception** | Use traceback analysis + pattern matching | 70% |
| **Can subclass Client** | Create wrapper with error tracking | 85% |
| **Cannot modify/extend SDK** | External error tracking with proxy pattern | 75% |

---

## Recommended Implementation Approaches

### Scenario A: SDK Provides Structured Exceptions ✅ (BEST CASE)

**IF SDK provides**:
- Toolset-specific exception classes (e.g., `KubernetesToolsetError`)
- Exception metadata (e.g., `e.toolset_name`)

**THEN implement**:
```python
try:
    result = await holmes_client.investigate(...)
except KubernetesToolsetError as e:
    toolset_config_service.record_toolset_failure('kubernetes', type(e).__name__, str(e))
except PrometheusToolsetError as e:
    toolset_config_service.record_toolset_failure('prometheus', type(e).__name__, str(e))
```

**Confidence**: 95% (reliable, maintainable)

---

### Scenario B: SDK Provides Generic Exceptions, No Metadata ⚠️ (LIKELY)

**IF SDK provides**:
- Generic `Exception` with error messages
- NO toolset metadata in exceptions

**THEN implement** (in priority order):

**1. Traceback Analysis** (Primary):
```python
import traceback

try:
    result = await holmes_client.investigate(...)
except Exception as e:
    # Analyze which library raised the exception
    tb = traceback.extract_tb(e.__traceback__)
    for frame in tb:
        if 'kubernetes' in frame.filename:
            toolset = 'kubernetes'
            break
        elif 'prometheus' in frame.filename:
            toolset = 'prometheus'
            break

    if toolset:
        toolset_config_service.record_toolset_failure(toolset, type(e).__name__, str(e))
```

**2. Intelligent Pattern Matching** (Fallback):
```python
def classify_error_by_patterns(error: Exception) -> Optional[str]:
    """Enhanced pattern matching with toolset-specific indicators"""
    error_str = str(error).lower()

    # Kubernetes patterns (port, API endpoints, error codes)
    if any(p in error_str for p in [
        'connection refused.*6443',  # K8s API port
        'forbidden.*pods',  # RBAC error
        'unauthorized.*api',
        'kube-apiserver'
    ]):
        return 'kubernetes'

    # Prometheus patterns
    if any(p in error_str for p in [
        'connection refused.*9090',  # Prometheus port
        'promql',
        'query_range',
        'prometheus.*api'
    ]):
        return 'prometheus'

    return None
```

**Confidence**: 70-85% (acceptable with multi-tier approach)

---

### Scenario C: Can Wrap/Extend Client ✅ (RECOMMENDED)

**IF SDK allows**:
- Subclassing `Client`
- Wrapping with decorator pattern

**THEN implement**:
```python
from holmes import Client as HolmesClient
from contextlib import contextmanager

class HolmesGPTClientWrapper:
    """Wrapper with structured error tracking"""

    def __init__(self, client: HolmesClient, toolset_config_service):
        self.client = client
        self.toolset_config_service = toolset_config_service

    @contextmanager
    def _track_toolset_execution(self, toolset_name: str):
        """Context manager for toolset execution tracking"""
        try:
            yield
            # Success: reset failure counter
            self.toolset_config_service.record_toolset_success(toolset_name)
        except Exception as e:
            # Failure: record with metadata
            self.toolset_config_service.record_toolset_failure(
                toolset_name=toolset_name,
                error_type=type(e).__name__,
                error_message=str(e)
            )
            raise

    async def investigate(self, *args, **kwargs):
        """Wrapped investigate with error tracking"""
        try:
            result = await self.client.investigate(*args, **kwargs)

            # Track successful toolset usage
            if hasattr(result, 'toolsets_used'):
                for toolset_name in result.toolsets_used:
                    self.toolset_config_service.record_toolset_success(toolset_name)

            return result

        except Exception as e:
            # Classify and track error
            toolset_name = self._classify_error(e)
            if toolset_name:
                self.toolset_config_service.record_toolset_failure(
                    toolset_name=toolset_name,
                    error_type=type(e).__name__,
                    error_message=str(e)
                )
            raise
```

**Confidence**: 85% (flexible, maintainable)

---

## Phase 1 Deliverables

**Completion Criteria**:
- [ ] Investigation script executed successfully
- [ ] All 5 investigation tasks completed
- [ ] Findings documented in this document
- [ ] Implementation approach selected based on findings
- [ ] Confidence assessment provided for selected approach

**Expected Timeline**: 1-2 days

**Next Phase**: Phase 2 - Implementation based on selected approach

---

## Status Updates

### Update 2025-01-15 - Investigation Started

**Status**: IN PROGRESS
**Current Task**: Task 1 - SDK Source Code Analysis
**Blocker**: None
**Next Steps**: Execute investigation script, document findings

---

## Notes

**Important Considerations**:
1. SDK capabilities may change in future versions - design for flexibility
2. If SDK lacks structured error handling, consider contributing PR to HolmesGPT
3. Wrapper approach provides forward compatibility with SDK updates
4. Document any assumptions made during investigation

**Contact**:
- HolmesGPT GitHub: https://github.com/robusta-dev/holmesgpt
- HolmesGPT Docs: https://docs.robusta.dev/holmesgpt
- SDK Issues: Consider opening GitHub issue for structured error handling feature request

