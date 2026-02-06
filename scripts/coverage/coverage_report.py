#!/usr/bin/env python3
"""
Kubernaut Coverage Report Generator

Replaces the bash+awk coverage reporting with a single Python script that:
- Parses Go coverage profiles (.out files) for line-by-line analysis
- Parses Python pytest-cov reports (.txt files) for module-level analysis
- Performs proper cross-tier line-by-line merging for "All Tiers" column
- Falls back to .pct summary files when full .out data isn't available
- Outputs markdown, table, or json formats

Usage:
    python3 coverage_report.py                          # Full table report
    python3 coverage_report.py --format markdown        # Markdown for PR comments
    python3 coverage_report.py --service gateway        # Single service
    python3 coverage_report.py --format json            # JSON for CI integration
"""

import argparse
import json
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional


# ============================================================================
# Configuration: Service patterns (mirrors .coverage-patterns.yaml)
# ============================================================================

GENERATED_CODE_PATTERNS = ["/ogen-client/", "/mocks/", "/test/"]

# Go services: package pattern, unit-exclude regex, integration-include regex
# File exclusions use `.go:` (Go coverage format: file.go:line)
# Directory exclusions use `/`
GO_SERVICE_CONFIG = {
    "aianalysis": {
        "pkg_pattern": "/pkg/aianalysis/",
        "unit_exclude": r"/(handler\.go:|audit/)",
        "int_include": r"/(handler\.go:|audit/)",
    },
    "authwebhook": {
        "pkg_pattern": "/pkg/authwebhook/",
        "unit_exclude": r"/(notificationrequest_handler|remediationapprovalrequest_handler|remediationrequest_handler|workflowexecution_handler|notificationrequest_validator)\.go:",
        "int_include": r"/(notificationrequest_handler|remediationapprovalrequest_handler|remediationrequest_handler|workflowexecution_handler|notificationrequest_validator)\.go:",
    },
    "datastorage": {
        "pkg_pattern": "/pkg/datastorage/",
        "unit_exclude": r"/(server/|repository/|dlq/|ogen-client/|mocks/|adapter/|query/service\.go:|reconstruction/query\.go:)",
        "int_include": r"/(server/|repository/|dlq/|adapter/|query/service\.go:|reconstruction/query\.go:)",
    },
    "gateway": {
        "pkg_pattern": "/pkg/gateway/",
        "unit_exclude": r"/(server\.go:|k8s/|processing/(crd_creator|distributed_lock|status_updater)/)",
        "int_include": r"/(server\.go:|k8s/|processing/(crd_creator|distributed_lock|status_updater)/)",
    },
    "notification": {
        "pkg_pattern": "/pkg/notification/",
        "unit_exclude": r"/(client\.go:|delivery/|phase/|status/)",
        "int_include": r"/(client\.go:|delivery/|phase/|status/)",
    },
    "remediationorchestrator": {
        "pkg_pattern": "/pkg/remediationorchestrator/",
        "unit_exclude": r"/(creator|handler/(aianalysis|signalprocessing|workflowexecution)|aggregator|status)/",
        "int_include": r"/(creator|handler/(aianalysis|signalprocessing|workflowexecution)|aggregator|status)/",
    },
    "signalprocessing": {
        "pkg_pattern": "/pkg/signalprocessing/",
        "unit_exclude": r"/(audit|cache|enricher|handler|status)/",
        "int_include": r"/(audit|cache|enricher|handler|status)/",
    },
    "workflowexecution": {
        "pkg_pattern": "/pkg/workflowexecution/",
        "unit_exclude": r"/(audit|status)/",
        "int_include": r"/(audit|status)/",
    },
}

# Python holmesgpt-api: module patterns for unit vs integration
PYTHON_UNIT_PATTERNS = [
    r"src/(models|validation|sanitization|toolsets|config)/",
    r"src/audit/buffered_store\.py",
    r"src/errors\.py",
]
PYTHON_INTEGRATION_PATTERNS = [
    r"src/(extensions|middleware|auth|clients)/",
    r"src/main\.py",
    r"src/audit/(events|factory)\.py",
    r"src/metrics/instrumentation\.py",
]


# ============================================================================
# Data types
# ============================================================================

@dataclass
class CoverageEntry:
    """A single Go coverage profile entry."""
    key: str           # file:startLine.startCol,endLine.endCol
    num_stmts: int     # number of statements in this block
    count: int         # execution count (0 = not covered)


@dataclass
class ServiceCoverage:
    """Coverage results for a single service."""
    name: str
    language: str = "go"
    unit: str = "-"
    integration: str = "-"
    e2e: str = "-"
    all_tiers: str = "-"


# ============================================================================
# Go coverage file parser
# ============================================================================

def parse_go_coverage_file(filepath: str) -> list[CoverageEntry]:
    """Parse a Go coverage .out file into a list of CoverageEntry objects."""
    entries = []
    path = Path(filepath)
    if not path.exists() or path.stat().st_size == 0:
        return entries

    with open(path) as f:
        for line in f:
            line = line.strip()
            # Skip mode line (e.g., "mode: set" or "mode: atomic")
            if line.startswith("mode:"):
                continue
            # Format: file:startLine.startCol,endLine.endCol numStmts count
            parts = line.split()
            if len(parts) != 3:
                continue
            try:
                key = parts[0]
                num_stmts = int(parts[1])
                count = int(parts[2])
                entries.append(CoverageEntry(key=key, num_stmts=num_stmts, count=count))
            except (ValueError, IndexError):
                continue
    return entries


def is_generated_code(key: str) -> bool:
    """Check if a coverage entry is for generated code (should be excluded)."""
    for pattern in GENERATED_CODE_PATTERNS:
        if pattern in key:
            return True
    return False


def calculate_coverage(entries: list[CoverageEntry]) -> str:
    """Calculate coverage percentage from a list of entries."""
    total_stmts = 0
    covered_stmts = 0
    for e in entries:
        total_stmts += e.num_stmts
        if e.count > 0:
            covered_stmts += e.num_stmts
    if total_stmts == 0:
        return "0.0%"
    return f"{(covered_stmts / total_stmts) * 100:.1f}%"


def filter_go_entries(
    entries: list[CoverageEntry],
    pkg_pattern: str,
    exclude_pattern: Optional[str] = None,
    include_pattern: Optional[str] = None,
) -> list[CoverageEntry]:
    """Filter Go coverage entries by package and inclusion/exclusion patterns."""
    filtered = []
    for e in entries:
        # Must match package pattern
        if pkg_pattern not in e.key:
            continue
        # Skip generated code
        if is_generated_code(e.key):
            continue
        # Apply exclude pattern (for unit-testable: exclude integration code)
        if exclude_pattern and re.search(exclude_pattern, e.key):
            continue
        # Apply include pattern (for integration-testable: only include integration code)
        if include_pattern and not re.search(include_pattern, e.key):
            continue
        filtered.append(e)
    return filtered


def merge_go_coverage_entries(*entry_lists: list[CoverageEntry]) -> dict[str, CoverageEntry]:
    """
    Merge coverage entries from multiple tiers (unit, integration, e2e).

    For each unique key (file:lines), take the maximum count across all tiers.
    This gives us true accumulated coverage: if ANY tier covers a line, it counts.
    """
    merged: dict[str, CoverageEntry] = {}
    for entries in entry_lists:
        for e in entries:
            if e.key in merged:
                # Take the higher count (if any tier covered it, it's covered)
                if e.count > merged[e.key].count:
                    merged[e.key] = CoverageEntry(
                        key=e.key, num_stmts=e.num_stmts, count=e.count
                    )
            else:
                merged[e.key] = CoverageEntry(
                    key=e.key, num_stmts=e.num_stmts, count=e.count
                )
    return merged


# ============================================================================
# Go service coverage calculation
# ============================================================================

def read_pct_file(filepath: str) -> Optional[str]:
    """Read a .pct summary file and return the percentage, or None if invalid."""
    path = Path(filepath)
    if not path.exists():
        return None
    content = path.read_text().strip()
    # Remove any % sign for normalization
    num = content.replace("%", "").strip()
    # Validate it's a valid number
    try:
        float(num)
        return f"{num}%" if "%" not in content else content
    except ValueError:
        return None  # "N/A" or other non-numeric


def calc_go_service_tier(service: str, tier: str) -> str:
    """Calculate coverage for a Go service at a specific tier."""
    config = GO_SERVICE_CONFIG.get(service)
    if not config:
        return "0.0%"

    covfile = f"coverage_{tier}_{service}.out"
    entries = parse_go_coverage_file(covfile)

    # If no entries, try .pct fallback
    if not entries:
        pct = read_pct_file(f"coverage_{tier}_{service}.pct")
        return pct if pct else "-"

    pkg_pattern = config["pkg_pattern"]

    if tier == "unit":
        filtered = filter_go_entries(
            entries, pkg_pattern, exclude_pattern=config["unit_exclude"]
        )
        return calculate_coverage(filtered)
    elif tier == "integration":
        filtered = filter_go_entries(
            entries, pkg_pattern, include_pattern=config["int_include"]
        )
        return calculate_coverage(filtered)
    elif tier == "e2e":
        # E2E uses full package coverage (no unit/integration filtering)
        filtered = [
            e for e in entries
            if pkg_pattern in e.key and not is_generated_code(e.key)
        ]
        return calculate_coverage(filtered)
    return "-"


def calc_go_service_all_tiers(service: str) -> str:
    """
    Calculate merged All Tiers coverage for a Go service.

    Uses proper line-by-line merging: for each code block, if ANY tier
    covered it, it counts as covered. This eliminates double-counting
    and gives true accumulated coverage.
    """
    config = GO_SERVICE_CONFIG.get(service)
    if not config:
        return "0.0%"

    pkg_pattern = config["pkg_pattern"]

    # Collect entries from all available tiers
    all_entries = []
    has_real_data = False

    for tier in ["unit", "integration", "e2e"]:
        covfile = f"coverage_{tier}_{service}.out"
        entries = parse_go_coverage_file(covfile)
        if entries:
            has_real_data = True
            # Filter to service's package and exclude generated code
            filtered = [
                e for e in entries
                if pkg_pattern in e.key and not is_generated_code(e.key)
            ]
            all_entries.append(filtered)

    if has_real_data and all_entries:
        # Proper line-by-line merge across all tiers
        merged = merge_go_coverage_entries(*all_entries)
        merged_list = list(merged.values())
        return calculate_coverage(merged_list)

    # Fallback: use .pct files (pick the highest)
    max_pct = 0.0
    found_any = False
    for tier in ["unit", "integration", "e2e"]:
        pct = read_pct_file(f"coverage_{tier}_{service}.pct")
        if pct:
            found_any = True
            try:
                val = float(pct.replace("%", ""))
                if val > max_pct:
                    max_pct = val
            except ValueError:
                continue

    if found_any:
        return f"{max_pct:.1f}%"
    return "-"


# ============================================================================
# Python (holmesgpt-api) coverage calculation
# ============================================================================

@dataclass
class PythonModuleCoverage:
    """Coverage data for a single Python module."""
    name: str
    total_stmts: int
    missed_stmts: int

    @property
    def covered_stmts(self) -> int:
        return self.total_stmts - self.missed_stmts


def parse_python_coverage_file(filepath: str) -> list[PythonModuleCoverage]:
    """Parse a Python pytest-cov text report into module coverage data."""
    modules = []
    path = Path(filepath)
    if not path.exists() or path.stat().st_size == 0:
        return modules

    with open(path) as f:
        for line in f:
            line = line.strip()
            # Skip headers, separators, summary
            if (line.startswith("Name") or line.startswith("---") or
                    line.startswith("==") or line.startswith("TOTAL") or
                    not line.startswith("src/")):
                continue

            parts = line.split()
            if len(parts) >= 3:
                try:
                    name = parts[0]
                    total_stmts = int(parts[1])
                    missed_stmts = int(parts[2])
                    modules.append(PythonModuleCoverage(
                        name=name, total_stmts=total_stmts, missed_stmts=missed_stmts
                    ))
                except (ValueError, IndexError):
                    continue
    return modules


def filter_python_modules(
    modules: list[PythonModuleCoverage], patterns: list[str]
) -> list[PythonModuleCoverage]:
    """Filter Python modules matching any of the given regex patterns."""
    filtered = []
    for m in modules:
        for pat in patterns:
            if re.search(pat, m.name):
                filtered.append(m)
                break
    return filtered


def calc_python_coverage(modules: list[PythonModuleCoverage]) -> str:
    """Calculate coverage percentage from Python module data."""
    total = sum(m.total_stmts for m in modules)
    covered = sum(m.covered_stmts for m in modules)
    if total == 0:
        return "0.0%"
    return f"{(covered / total) * 100:.1f}%"


def get_python_total_from_file(filepath: str) -> Optional[str]:
    """Extract TOTAL percentage from a pytest-cov report."""
    path = Path(filepath)
    if not path.exists():
        return None
    with open(path) as f:
        for line in f:
            if line.strip().startswith("TOTAL"):
                parts = line.strip().split()
                if parts:
                    last = parts[-1].replace("%", "")
                    try:
                        return f"{float(last):.1f}%"
                    except ValueError:
                        pass
    return None


def calc_python_service() -> ServiceCoverage:
    """Calculate all coverage tiers for holmesgpt-api (Python service)."""
    svc = ServiceCoverage(name="holmesgpt-api", language="python")

    # Unit coverage
    unit_file = "coverage_unit_holmesgpt-api.txt"
    unit_modules = parse_python_coverage_file(unit_file)
    if unit_modules:
        filtered = filter_python_modules(unit_modules, PYTHON_UNIT_PATTERNS)
        svc.unit = calc_python_coverage(filtered)
    else:
        # Fallback to TOTAL line
        total = get_python_total_from_file(unit_file)
        svc.unit = total if total else "-"

    # Integration coverage
    int_file = "coverage_integration_holmesgpt-api_python.txt"
    int_modules = parse_python_coverage_file(int_file)
    if int_modules:
        filtered = filter_python_modules(int_modules, PYTHON_INTEGRATION_PATTERNS)
        svc.integration = calc_python_coverage(filtered)
    else:
        total = get_python_total_from_file(int_file)
        svc.integration = total if total else "-"

    # E2E coverage (Go-based Ginkgo tests)
    e2e_file = "coverage_e2e_holmesgpt-api.out"
    e2e_entries = parse_go_coverage_file(e2e_file)
    if e2e_entries:
        svc.e2e = calculate_coverage(e2e_entries)
    else:
        pct = read_pct_file("coverage_e2e_holmesgpt-api.pct")
        svc.e2e = pct if pct else "-"

    # All Tiers: Python unit total (can't line-merge Python + Go)
    total = get_python_total_from_file(unit_file)
    svc.all_tiers = total if total else svc.unit

    return svc


# ============================================================================
# Full report generation
# ============================================================================

def generate_all_service_coverage(filter_service: Optional[str] = None) -> list[ServiceCoverage]:
    """Generate coverage data for all services."""
    results = []

    # Python service
    if not filter_service or filter_service == "holmesgpt-api":
        results.append(calc_python_service())

    # Go services
    for service in GO_SERVICE_CONFIG:
        if filter_service and filter_service != service:
            continue

        svc = ServiceCoverage(name=service, language="go")
        svc.unit = calc_go_service_tier(service, "unit")
        svc.integration = calc_go_service_tier(service, "integration")
        svc.e2e = calc_go_service_tier(service, "e2e")
        svc.all_tiers = calc_go_service_all_tiers(service)
        results.append(svc)

    return results


# ============================================================================
# Output formatters
# ============================================================================

def output_markdown(services: list[ServiceCoverage]) -> str:
    """Generate markdown table for GitHub PR comments."""
    lines = [
        "## 📊 Kubernaut Coverage Report (By Test Tier)",
        "",
        "| Service | Unit-Testable | Integration-Testable | E2E | All Tiers |",
        "|---------|---------------|----------------------|-----|-----------|",
    ]

    for svc in services:
        lines.append(f"| {svc.name} | {svc.unit} | {svc.integration} | {svc.e2e} | {svc.all_tiers} |")

    lines.extend([
        "",
        "### 📝 Column Definitions",
        "",
        "- **Unit-Testable**: Pure logic code (config, validators, builders, formatters, classifiers)",
        "- **Integration-Testable**: Integration-only code (handlers, servers, DB adapters, K8s clients)",
        "- **E2E**: End-to-end test coverage (full workflows)",
        "- **All Tiers**: Merged coverage — line-by-line deduplication across all tiers (any tier covering a line counts)",
        "",
        "### 🎯 Quality Targets",
        "",
        "- Unit-Testable: ≥70%",
        "- Integration: ≥60%",
        "- All Tiers: ≥80%",
        "",
        "---",
        "",
        "_Generated by `make coverage-report-markdown` | See [Coverage Analysis Report](docs/testing/COVERAGE_ANALYSIS_REPORT.md) for details_",
    ])

    return "\n".join(lines)


def output_table(services: list[ServiceCoverage]) -> str:
    """Generate terminal-friendly table output."""
    lines = [
        "═" * 115,
        "📊 KUBERNAUT COMPREHENSIVE COVERAGE ANALYSIS (By Test Tier)",
        "═" * 115,
        f"{'Service':<25} {'Unit-Testable':<15} {'Integration':<15} {'E2E':<15} {'All Tiers':<15}",
        "─" * 115,
    ]

    for svc in services:
        lines.append(f"{svc.name:<25} {svc.unit:<15} {svc.integration:<15} {svc.e2e:<15} {svc.all_tiers:<15}")

    lines.extend([
        "─" * 115,
        "",
        "📝 COLUMN DEFINITIONS:",
        "   • Unit-Testable: Coverage of pure logic code (config, validators, builders, formatters, classifiers, etc.)",
        "   • Integration: Coverage of integration-only code (handlers, servers, DB adapters, K8s clients, workers, etc.)",
        "   • E2E: Coverage of any code from E2E tests (usually covers full workflows)",
        "   • All Tiers: Line-by-line merged coverage where ANY tier covering a line counts (true total coverage)",
        "",
        "🎯 QUALITY TARGETS:",
        "   - Unit-Testable: ≥70% (pure logic should be well-tested)",
        "   - Integration: ≥60% (handlers/servers should have good integration coverage)",
        "   - All Tiers: ≥80% (overall coverage goal)",
        "",
        "📈 Run 'make test-tier-unit test-tier-integration test-tier-e2e' to update all coverage files.",
    ])

    return "\n".join(lines)


def output_json(services: list[ServiceCoverage]) -> str:
    """Generate JSON output for CI/CD integration."""
    data = {
        "services": [
            {
                "name": svc.name,
                "language": svc.language,
                "unit_testable": svc.unit,
                "integration": svc.integration,
                "e2e": svc.e2e,
                "all_tiers": svc.all_tiers,
            }
            for svc in services
        ]
    }
    return json.dumps(data, indent=2)


# ============================================================================
# Main
# ============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Generate comprehensive coverage report for all Kubernaut services."
    )
    parser.add_argument(
        "--format", choices=["table", "markdown", "json"], default="table",
        help="Output format (default: table)"
    )
    parser.add_argument(
        "--service", default=None,
        help="Report for specific service only"
    )
    args = parser.parse_args()

    # Change to repo root for coverage file access
    repo_root = Path(__file__).resolve().parent.parent.parent
    os.chdir(repo_root)

    # Generate coverage data
    services = generate_all_service_coverage(filter_service=args.service)

    # Output in requested format
    if args.format == "markdown":
        print(output_markdown(services))
    elif args.format == "json":
        print(output_json(services))
    else:
        print(output_table(services))


if __name__ == "__main__":
    main()
