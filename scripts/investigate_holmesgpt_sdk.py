#!/usr/bin/env python3
"""
HolmesGPT SDK Investigation Script

Purpose: Investigate HolmesGPT SDK capabilities for BR-HAPI-189 Phase 1

Usage:
    python scripts/investigate_holmesgpt_sdk.py

Requirements:
    pip install holmes-ai  # or appropriate package name

Output:
    - Console report of SDK capabilities
    - Investigation findings for Phase 1 documentation
"""

import inspect
import sys
import traceback
from typing import Any, List, Dict
from datetime import datetime


def print_section(title: str):
    """Print formatted section header"""
    print("\n" + "=" * 80)
    print(title)
    print("=" * 80)


def print_subsection(title: str):
    """Print formatted subsection"""
    print(f"\n{title}")
    print("-" * len(title))


def investigate_sdk_import():
    """Task 1: Attempt to import HolmesGPT SDK"""
    print_section("Task 1: SDK Import and Basic Inspection")

    try:
        from holmes import Client
        print("\n✅ SUCCESS: HolmesGPT SDK imported successfully")
        print(f"   Client class: {Client.__module__}.{Client.__name__}")
        print(f"   Client module file: {inspect.getfile(Client)}")

        return Client

    except ImportError as e:
        print(f"\n❌ FAILED: Cannot import HolmesGPT SDK")
        print(f"   Error: {e}")
        print("\n📝 Action Required:")
        print("   1. Install HolmesGPT SDK:")
        print("      pip install holmes-ai  # Verify actual package name")
        print("   2. Check SDK documentation: https://github.com/robusta-dev/holmesgpt")
        return None

    except Exception as e:
        print(f"\n❌ UNEXPECTED ERROR: {e}")
        traceback.print_exc()
        return None


def investigate_client_methods(client_class):
    """Task 2: Inspect Client class methods"""
    print_section("Task 2: Client Class Methods and Signatures")

    if not client_class:
        print("\n⚠️  Skipped: Client class not available")
        return {}

    methods_info = {}

    print("\n📋 Public Methods:")
    for name, method in inspect.getmembers(client_class):
        if not name.startswith('_') and callable(method):
            try:
                sig = inspect.signature(method)
                methods_info[name] = str(sig)
                print(f"  • {name}{sig}")

                # Check for return type annotation
                if sig.return_annotation != inspect.Signature.empty:
                    print(f"    Return type: {sig.return_annotation}")
            except (ValueError, TypeError):
                print(f"  • {name}(...) [signature unavailable]")

    # Special focus on investigate() method
    if hasattr(client_class, 'investigate'):
        print_subsection("🔍 investigate() Method Analysis")
        try:
            sig = inspect.signature(client_class.investigate)
            print(f"   Full signature: investigate{sig}")

            # Parameters
            print(f"\n   Parameters:")
            for param_name, param in sig.parameters.items():
                if param_name != 'self':
                    print(f"     - {param_name}: {param.annotation if param.annotation != inspect.Parameter.empty else 'Any'}")

            # Return type
            if sig.return_annotation != inspect.Signature.empty:
                print(f"\n   ✅ Return type annotated: {sig.return_annotation}")
            else:
                print(f"\n   ⚠️  Return type NOT annotated (returns Any)")

            # Docstring
            if client_class.investigate.__doc__:
                print(f"\n   Docstring:\n{client_class.investigate.__doc__}")

        except Exception as e:
            print(f"   ❌ Error inspecting investigate(): {e}")

    else:
        print("\n❌ CRITICAL: investigate() method NOT found in Client class")

    return methods_info


def investigate_exception_hierarchy():
    """Task 3: Look for custom exception classes"""
    print_section("Task 3: Exception Hierarchy Investigation")

    try:
        import holmes

        # Find all exception/error classes
        exception_classes = []
        for name in dir(holmes):
            if ('error' in name.lower() or 'exception' in name.lower()):
                obj = getattr(holmes, name)
                if inspect.isclass(obj):
                    exception_classes.append((name, obj))

        if exception_classes:
            print("\n✅ Found Exception Classes:")
            for exc_name, exc_class in exception_classes:
                bases = [base.__name__ for base in exc_class.__bases__]
                print(f"\n  • {exc_name}")
                print(f"    Inherits from: {', '.join(bases)}")

                # Check __init__ signature
                try:
                    sig = inspect.signature(exc_class.__init__)
                    print(f"    Constructor: __init__{sig}")

                    # Check for metadata attributes
                    init_params = list(sig.parameters.keys())
                    if 'toolset_name' in init_params:
                        print(f"    ✅ HAS 'toolset_name' parameter!")
                    if 'error_code' in init_params:
                        print(f"    ✅ HAS 'error_code' parameter!")

                except Exception as e:
                    print(f"    ⚠️  Could not inspect __init__: {e}")

                # Check instance attributes
                try:
                    dummy_instance = exc_class("test")
                    attrs = [a for a in dir(dummy_instance) if not a.startswith('_')]
                    if attrs:
                        print(f"    Instance attributes: {', '.join(attrs)}")
                except:
                    pass

        else:
            print("\n⚠️  NO custom exception classes found in holmes module")
            print("   SDK likely uses standard Python exceptions (Exception, RuntimeError, etc.)")
            print("\n📝 Implication: Must use traceback analysis or pattern matching for error classification")

        return exception_classes

    except ImportError:
        print("\n❌ Cannot import holmes module")
        return []

    except Exception as e:
        print(f"\n❌ Error during exception investigation: {e}")
        traceback.print_exc()
        return []


def investigate_toolset_classes():
    """Task 4: Look for toolset-related classes"""
    print_section("Task 4: Toolset Implementation Investigation")

    try:
        import holmes

        # Find toolset-related classes
        toolset_items = []
        for name in dir(holmes):
            if 'toolset' in name.lower():
                obj = getattr(holmes, name)
                toolset_items.append((name, obj, type(obj).__name__))

        if toolset_items:
            print("\n✅ Found Toolset-Related Items:")
            for name, obj, obj_type in toolset_items:
                print(f"  • {name} ({obj_type})")

                # If it's a class, inspect it
                if inspect.isclass(obj):
                    print(f"    Base classes: {[b.__name__ for b in obj.__bases__]}")

                    # Check for callback/hook methods
                    methods = [m for m in dir(obj) if not m.startswith('_')]
                    callback_methods = [m for m in methods if 'callback' in m.lower() or 'hook' in m.lower() or 'on_' in m.lower()]
                    if callback_methods:
                        print(f"    ✅ Callback/hook methods: {', '.join(callback_methods)}")

        else:
            print("\n⚠️  NO toolset classes found in public API")
            print("   Toolsets may be internal implementation details")

        # Check for registration/configuration methods
        print_subsection("🔍 Toolset Registration/Configuration")

        config_methods = []
        for name in dir(holmes):
            if any(keyword in name.lower() for keyword in ['register', 'configure', 'add', 'toolset']):
                obj = getattr(holmes, name)
                if callable(obj):
                    config_methods.append(name)

        if config_methods:
            print(f"  ✅ Found configuration methods: {', '.join(config_methods)}")
        else:
            print(f"  ⚠️  No toolset registration methods found")

        return toolset_items

    except ImportError:
        print("\n❌ Cannot import holmes module")
        return []

    except Exception as e:
        print(f"\n❌ Error during toolset investigation: {e}")
        traceback.print_exc()
        return []


def investigate_return_types():
    """Task 5: Investigate return types and result objects"""
    print_section("Task 5: Investigation Return Types and Metadata")

    try:
        import holmes

        # Look for Result/Response classes
        result_classes = []
        for name in dir(holmes):
            if any(keyword in name.lower() for keyword in ['result', 'response', 'output']):
                obj = getattr(holmes, name)
                if inspect.isclass(obj):
                    result_classes.append((name, obj))

        if result_classes:
            print("\n✅ Found Result/Response Classes:")
            for class_name, class_obj in result_classes:
                print(f"\n  • {class_name}")

                # Inspect class attributes
                if hasattr(class_obj, '__annotations__'):
                    print(f"    Attributes:")
                    for attr_name, attr_type in class_obj.__annotations__.items():
                        print(f"      - {attr_name}: {attr_type}")

                        # Check for critical attributes
                        if 'toolset' in attr_name.lower():
                            print(f"        ✅ CRITICAL: Contains toolset information!")

                # Check for methods
                methods = [m for m in dir(class_obj) if not m.startswith('_') and callable(getattr(class_obj, m))]
                if methods:
                    print(f"    Methods: {', '.join(methods[:5])}{'...' if len(methods) > 5 else ''}")

        else:
            print("\n⚠️  NO result/response classes found in public API")
            print("   investigate() may return dict or generic object")

        return result_classes

    except ImportError:
        print("\n❌ Cannot import holmes module")
        return []

    except Exception as e:
        print(f"\n❌ Error investigating return types: {e}")
        traceback.print_exc()
        return []


def investigate_extension_points():
    """Task 6: Check for extension/plugin mechanisms"""
    print_section("Task 6: Extension Points and Customization")

    try:
        import holmes
        from holmes import Client

        print("\n🔍 Subclassing Capability:")
        try:
            class TestClient(Client):
                pass
            print("  ✅ Client class CAN be subclassed")
            print("     Implementation approach: Create wrapper/subclass for error tracking")
        except TypeError as e:
            print(f"  ❌ Client class CANNOT be subclassed: {e}")

        print("\n🔍 Middleware/Plugin System:")
        plugin_keywords = ['middleware', 'plugin', 'extension', 'hook', 'callback']
        found_plugins = False

        for name in dir(holmes):
            if any(keyword in name.lower() for keyword in plugin_keywords):
                obj = getattr(holmes, name)
                if callable(obj) or inspect.isclass(obj):
                    print(f"  • Found: {name}")
                    found_plugins = True

        if not found_plugins:
            print("  ⚠️  No plugin/middleware system found")
            print("     Implementation approach: Use wrapper pattern for extension")

        print("\n🔍 Monkey-Patching Safety:")
        print("  ⚠️  Monkey-patching is technically possible but NOT recommended")
        print("     Recommendation: Use composition (wrapper) over monkey-patching")

    except ImportError:
        print("\n❌ Cannot import holmes module")

    except Exception as e:
        print(f"\n❌ Error investigating extension points: {e}")
        traceback.print_exc()


def generate_summary(findings: Dict[str, Any]):
    """Generate investigation summary and recommendations"""
    print_section("Investigation Summary and Recommendations")

    print("\n📊 SDK Capabilities Assessment:")

    # Exception handling
    if findings.get('exceptions'):
        print(f"\n  ✅ Custom Exceptions: {len(findings['exceptions'])} found")
        has_toolset_exceptions = any('toolset' in name.lower() for name, _ in findings['exceptions'])
        if has_toolset_exceptions:
            print(f"     ✅ Toolset-specific exceptions available")
        else:
            print(f"     ⚠️  No toolset-specific exceptions")
    else:
        print(f"\n  ⚠️  Custom Exceptions: NONE found")

    # Toolset metadata
    if findings.get('toolsets'):
        print(f"\n  ✅ Toolset Classes: {len(findings['toolsets'])} found")
    else:
        print(f"\n  ⚠️  Toolset Classes: NONE in public API")

    # Return types
    if findings.get('result_classes'):
        print(f"\n  ✅ Result Classes: {len(findings['result_classes'])} found")
    else:
        print(f"\n  ⚠️  Result Classes: NONE found (returns generic types)")

    print("\n" + "=" * 80)
    print("RECOMMENDED IMPLEMENTATION APPROACH")
    print("=" * 80)

    # Determine best approach based on findings
    has_structured_exceptions = findings.get('exceptions') and any(
        'toolset' in name.lower() for name, _ in findings.get('exceptions', [])
    )

    if has_structured_exceptions:
        print("\n✅ SCENARIO A: Use Structured Exceptions")
        print("   SDK provides toolset-specific exceptions")
        print("   Confidence: 95%")
        print("\n   Implementation:")
        print("   - Catch specific exception types (KubernetesToolsetError, etc.)")
        print("   - Extract toolset_name from exception metadata")
        print("   - Record failure with rich error information")

    else:
        print("\n⚠️  SCENARIO B: Implement Multi-Tier Error Classification")
        print("   SDK uses generic exceptions without toolset metadata")
        print("   Confidence: 70-85%")
        print("\n   Implementation Priority:")
        print("   1. Traceback Analysis (85% confidence)")
        print("      - Inspect exception.__traceback__ to identify source library")
        print("   2. Intelligent Pattern Matching (70% confidence)")
        print("      - Use toolset-specific patterns (ports, error codes, API paths)")
        print("   3. Client Wrapper (85% confidence)")
        print("      - Wrap Client class to add structured error tracking")

    print("\n" + "=" * 80)
    print("NEXT STEPS")
    print("=" * 80)
    print("""
1. Document findings in: docs/requirements/BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md

2. Based on findings, implement RECOMMENDED APPROACH:
   - Update BR-HAPI-189 specification with chosen approach
   - Create implementation tasks for Phase 2
   - Develop unit test specifications

3. If SDK lacks structured error handling:
   - Consider submitting feature request/PR to HolmesGPT project
   - Implement wrapper pattern for immediate needs

4. Update service specification:
   - Update 08-holmesgpt-api.md with error handling architecture
   - Document confidence level and rationale
    """)


def main():
    """Main investigation workflow"""
    print("=" * 80)
    print(" HolmesGPT SDK Investigation Report")
    print(" BR-HAPI-189 Phase 1: SDK Capability Assessment")
    print(f" Date: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 80)

    findings = {}

    # Task 1: Import SDK
    client_class = investigate_sdk_import()
    findings['client_class'] = client_class

    if not client_class:
        print("\n" + "=" * 80)
        print("❌ INVESTIGATION HALTED")
        print("=" * 80)
        print("\nCannot proceed without HolmesGPT SDK installation.")
        print("Please install the SDK and re-run this script.")
        return 1

    # Task 2: Inspect Client methods
    findings['methods'] = investigate_client_methods(client_class)

    # Task 3: Exception hierarchy
    findings['exceptions'] = investigate_exception_hierarchy()

    # Task 4: Toolset classes
    findings['toolsets'] = investigate_toolset_classes()

    # Task 5: Return types
    findings['result_classes'] = investigate_return_types()

    # Task 6: Extension points
    investigate_extension_points()

    # Generate summary
    generate_summary(findings)

    print("\n" + "=" * 80)
    print("✅ Investigation Complete")
    print("=" * 80)
    print(f"\nReport generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("Save this output to: docs/requirements/sdk_investigation_report_{date}.txt")

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\n\n⚠️  Investigation interrupted by user")
        sys.exit(130)
    except Exception as e:
        print(f"\n\n❌ FATAL ERROR: {e}")
        traceback.print_exc()
        sys.exit(1)

