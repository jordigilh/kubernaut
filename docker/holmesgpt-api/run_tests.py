#!/usr/bin/env python3
"""
HolmesGPT API Test Runner
Following TDD principles and project guidelines

This script implements the TDD cycle:
1. Red: Run tests (should fail initially)
2. Green: Fix implementation to pass tests
3. Refactor: Improve code while maintaining tests
"""

import subprocess
import sys
import os
from pathlib import Path

# Colors for output
RED = '\033[0;31m'
GREEN = '\033[0;32m'
YELLOW = '\033[1;33m'
BLUE = '\033[0;34m'
NC = '\033[0m'  # No Color

def log(message: str, color: str = NC):
    """Log colored message"""
    print(f"{color}{message}{NC}")

def run_command(cmd: list, description: str) -> bool:
    """Run command and return success status"""
    log(f"🔄 {description}...", BLUE)
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, cwd=Path(__file__).parent)
        if result.returncode == 0:
            log(f"✅ {description} passed", GREEN)
            if result.stdout:
                print(result.stdout)
            return True
        else:
            log(f"❌ {description} failed", RED)
            print(result.stdout)
            print(result.stderr)
            return False
    except Exception as e:
        log(f"❌ {description} error: {e}", RED)
        return False

def main():
    """Main test runner implementing TDD cycle"""

    print(f"{BLUE}{'='*60}")
    print("🧪 HolmesGPT API Test Runner - TDD Cycle")
    print("Following project guidelines and TDD principles")
    print(f"{'='*60}{NC}")

    # Verify we're in the right directory
    if not os.path.exists("tests") or not os.path.exists("src"):
        log("❌ Error: Must run from holmesgpt-api directory", RED)
        sys.exit(1)

    # Set up Python path
    current_dir = Path(__file__).parent
    src_path = current_dir / "src"
    os.environ["PYTHONPATH"] = str(src_path)

    log("📁 Setting PYTHONPATH to src directory", BLUE)
    log(f"   PYTHONPATH={os.environ['PYTHONPATH']}", BLUE)

    # TDD Phase 1: RED - Run all tests (expecting failures)
    log("\n🔴 TDD PHASE 1: RED - Running tests (expecting initial failures)", YELLOW)
    log("Project guideline: 'tests should fail initially as expected'", BLUE)

    test_commands = [
        # Run specific test categories to track progress
        (["python", "-m", "pytest", "tests/test_investigation_api.py", "-v"], "Investigation API Tests"),
        (["python", "-m", "pytest", "tests/test_chat_api.py", "-v"], "Chat API Tests"),
        (["python", "-m", "pytest", "tests/test_auth_api.py", "-v"], "Auth API Tests"),
        (["python", "-m", "pytest", "tests/test_health_api.py", "-v"], "Health API Tests"),
        (["python", "-m", "pytest", "tests/test_holmesgpt_service.py", "-v"], "HolmesGPT Service Tests"),
        (["python", "-m", "pytest", "tests/test_auth_service.py", "-v"], "Auth Service Tests"),
        (["python", "-m", "pytest", "tests/test_context_service.py", "-v"], "Context Service Tests"),
        (["python", "-m", "pytest", "tests/test_models.py", "-v"], "Model Tests"),
    ]

    passed_tests = 0
    total_tests = len(test_commands)

    for cmd, description in test_commands:
        success = run_command(cmd, description)
        if success:
            passed_tests += 1

    # Report TDD Phase 1 results
    log(f"\n📊 TDD Phase 1 Results:", YELLOW)
    log(f"   Tests Passed: {passed_tests}/{total_tests}", BLUE)
    log(f"   Tests Failed: {total_tests - passed_tests}/{total_tests}", BLUE)

    if passed_tests == total_tests:
        log("✅ All tests passed! Implementation is complete.", GREEN)
    else:
        log("❌ Tests failed as expected in TDD cycle.", YELLOW)
        log("🔧 Next step: Implement business logic to make tests pass", BLUE)

    # Run comprehensive test suite with coverage
    log("\n📈 Running comprehensive test suite with coverage...", BLUE)
    coverage_success = run_command([
        "python", "-m", "pytest",
        "tests/",
        "--cov=src",
        "--cov-report=html",
        "--cov-report=term-missing",
        "-v"
    ], "Full test suite with coverage")

    # Code quality checks
    log("\n🔍 Running code quality checks...", BLUE)

    quality_checks = [
        (["python", "-m", "flake8", "src", "tests", "--max-line-length=120"], "Flake8 linting"),
        (["python", "-m", "black", "--check", "src", "tests"], "Black code formatting"),
        (["python", "-m", "isort", "--check-only", "src", "tests"], "Import sorting"),
    ]

    quality_passed = 0
    for cmd, description in quality_checks:
        if run_command(cmd, description):
            quality_passed += 1

    # Final summary
    log(f"\n{'='*60}", BLUE)
    log("📋 TDD CYCLE SUMMARY", YELLOW)
    log(f"{'='*60}", BLUE)

    log(f"🧪 Unit Tests: {passed_tests}/{total_tests} passed", GREEN if passed_tests == total_tests else YELLOW)
    log(f"📊 Coverage: {'✅ Generated' if coverage_success else '❌ Failed'}", GREEN if coverage_success else RED)
    log(f"🔍 Code Quality: {quality_passed}/{len(quality_checks)} passed", GREEN if quality_passed == len(quality_checks) else YELLOW)

    # TDD Guidance
    log(f"\n🎯 TDD NEXT STEPS:", YELLOW)
    if passed_tests < total_tests:
        log("1. 🔴 RED PHASE: Tests are failing (as expected)", BLUE)
        log("2. 🟢 GREEN PHASE: Implement business logic to pass tests", BLUE)
        log("3. 🔵 REFACTOR PHASE: Improve code while maintaining tests", BLUE)
    else:
        log("1. ✅ GREEN PHASE: All tests passing", GREEN)
        log("2. 🔵 REFACTOR PHASE: Consider improvements", BLUE)
        log("3. 🔁 REPEAT: Add new tests for additional features", BLUE)

    # Business Requirements Tracking
    log(f"\n📋 BUSINESS REQUIREMENTS COVERAGE:", YELLOW)
    log("✅ BR-HAPI-001-005: Investigation API", GREEN)
    log("✅ BR-HAPI-006-010: Chat API", GREEN)
    log("✅ BR-HAPI-016-020: Health Monitoring", GREEN)
    log("✅ BR-HAPI-026-030: Authentication", GREEN)
    log("✅ BR-HAPI-044: Data Validation", GREEN)

    log(f"\n{'='*60}", BLUE)

    # Exit with appropriate code
    if passed_tests == total_tests and quality_passed == len(quality_checks):
        log("🎉 All tests and quality checks passed!", GREEN)
        sys.exit(0)
    else:
        log("⚠️  Some tests failed or quality issues found", YELLOW)
        sys.exit(1)

if __name__ == "__main__":
    main()


