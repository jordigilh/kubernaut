#!/usr/bin/env python3
"""
API Frontend Coverage Report Generator

Reuses the kubernaut coverage reporting methodology:
- Parses Go coverage profiles (.out files) for line-by-line analysis
- Classifies statements as unit-testable vs integration-testable
- Performs cross-tier line-by-line merging for "All Tiers" column
- Outputs markdown, table, or json formats

Usage:
    python3 coverage_report.py                          # Full table report
    python3 coverage_report.py --format markdown        # Markdown for PR comments
    python3 coverage_report.py --format json            # JSON for CI integration

See docs/testing/TESTING_COVERAGE_METHODOLOGY.md for methodology details.
"""

import argparse
import json
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


# ============================================================================
# Configuration: Package tier classification
# ============================================================================

GENERATED_CODE_PATTERNS = ["/mocks/", "/test/", "/zz_generated"]

# apifrontend is a single service, but we classify internal/ packages into
# unit-testable vs integration-testable using exclude/include regexes.
#
# Unit-Testable: pure logic (no K8s API, no real HTTP, no containers)
#   config, validate, audit, metrics, security, httputil, requestid,
#   logging, streaming (SSE framing), severity (triage pipeline, types),
#   session (state machine), ratelimit, prometheus (registry wiring),
#   tools (business logic — validation, args, helpers),
#   agent (prompt, RBAC role config), resilience (CB config)
#
# Integration-Only: requires real K8s API, containers, or network I/O
#   handler (MCP bridge, HTTP server, SSE dispatch),
#   ka (REST client hitting real KA container),
#   ds (OgenClient hitting real DS),
#   launcher (server bootstrap),
#   controller (envtest reconcilers),
#   tlswiring (real TLS cert loading),
#   auth (TokenReview, SAR via envtest)
#
# Some packages have both unit-testable and integration-only code.
# The patterns below define what to EXCLUDE from unit-testable scope
# (i.e., code that can only be tested at IT/E2E tier).

AF_SERVICE_CONFIG = {
    "apifrontend": {
        "pkg_prefix": "github.com/jordigilh/kubernaut-apifrontend/internal/",
        # Packages that are purely unit-testable (no exclusions needed)
        "unit_only_pkgs": [
            "config", "validate", "audit", "metrics", "security",
            "httputil", "requestid", "logging", "prometheus",
        ],
        # Packages with mixed code: unit_exclude removes IT-only files
        "mixed_pkgs": {
            "tools": {
                "unit_exclude": None,  # all tools logic is unit-testable
            },
            "agent": {
                "unit_exclude": None,  # prompt, RBAC config are unit-testable
            },
            "session": {
                "unit_exclude": None,  # state machine is unit-testable
            },
            "severity": {
                "unit_exclude": r"/llm\.go:",  # GenAI client needs real LLM
            },
            "ratelimit": {
                "unit_exclude": None,
            },
            "resilience": {
                "unit_exclude": None,
            },
            "streaming": {
                "unit_exclude": None,
            },
            "auth": {
                # TokenReview/SAR middleware needs envtest; RBAC config/helpers are UT
                "unit_exclude": r"/(dynamic_impersonation\.go:|middleware\.go:|transport\.go:)",
            },
        },
        # Packages that are integration/E2E only (excluded from UT denominator)
        "integration_only_pkgs": [
            "handler", "ka", "ds", "launcher", "controller", "tlswiring",
        ],
        # Packages excluded from IT-testable denominator because the IT suite
        # deliberately bypasses them (e.g. auth uses pre-injected testUser).
        # These are covered by UT and E2E (real Dex) instead.
        "it_exclude_pkgs": [
            "auth",
        ],
    }
}


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
class TierCoverage:
    """Coverage results for a single tier."""
    name: str
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
            if line.startswith("mode:"):
                continue
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
    return any(pattern in key for pattern in GENERATED_CODE_PATTERNS)


def calculate_coverage(entries: list[CoverageEntry]) -> str:
    """Calculate coverage percentage from a list of entries."""
    total_stmts = sum(e.num_stmts for e in entries)
    if total_stmts == 0:
        return "0.0%"
    covered_stmts = sum(e.num_stmts for e in entries if e.count > 0)
    return f"{(covered_stmts / total_stmts) * 100:.1f}%"


def calculate_coverage_raw(entries: list[CoverageEntry]) -> tuple[int, int]:
    """Return (covered_stmts, total_stmts) for a list of entries."""
    total = sum(e.num_stmts for e in entries)
    covered = sum(e.num_stmts for e in entries if e.count > 0)
    return covered, total


def get_pkg_from_key(key: str, prefix: str) -> Optional[str]:
    """Extract the package name from a coverage key.

    E.g., 'github.com/.../internal/tools/crd_tools.go:10.1,20.1' -> 'tools'
    """
    if prefix not in key:
        return None
    after = key[key.index(prefix) + len(prefix):]
    return after.split("/")[0]


def merge_coverage_entries(*entry_lists: list[CoverageEntry]) -> list[CoverageEntry]:
    """Merge coverage entries from multiple tiers (max count per key)."""
    merged: dict[str, CoverageEntry] = {}
    for entries in entry_lists:
        for e in entries:
            if e.key in merged:
                if e.count > merged[e.key].count:
                    merged[e.key] = CoverageEntry(
                        key=e.key, num_stmts=e.num_stmts, count=e.count
                    )
            else:
                merged[e.key] = CoverageEntry(
                    key=e.key, num_stmts=e.num_stmts, count=e.count
                )
    return list(merged.values())


# ============================================================================
# Tier-scoped filtering
# ============================================================================

def filter_unit_testable(entries: list[CoverageEntry], config: dict) -> list[CoverageEntry]:
    """Filter entries to unit-testable scope only."""
    prefix = config["pkg_prefix"]
    unit_only = set(config["unit_only_pkgs"])
    mixed = config["mixed_pkgs"]
    filtered = []

    for e in entries:
        if is_generated_code(e.key):
            continue
        pkg = get_pkg_from_key(e.key, prefix)
        if pkg is None:
            continue

        if pkg in unit_only:
            filtered.append(e)
        elif pkg in mixed:
            exclude = mixed[pkg].get("unit_exclude")
            if exclude and re.search(exclude, e.key):
                continue
            filtered.append(e)
        # integration_only_pkgs are excluded from UT scope

    return filtered


def filter_integration_testable(entries: list[CoverageEntry], config: dict) -> list[CoverageEntry]:
    """Filter entries to integration-testable scope.

    Integration scope includes:
    - integration_only_pkgs (handler, ka, ds, launcher, controller, tlswiring)
    - Mixed-pkg files that are excluded from unit scope (auth middleware, etc.)

    Packages listed in it_exclude_pkgs are omitted entirely — the IT suite
    bypasses them by design (e.g. auth uses pre-injected identities).
    """
    prefix = config["pkg_prefix"]
    int_only = set(config["integration_only_pkgs"])
    it_exclude = set(config.get("it_exclude_pkgs", []))
    mixed = config["mixed_pkgs"]
    filtered = []

    for e in entries:
        if is_generated_code(e.key):
            continue
        pkg = get_pkg_from_key(e.key, prefix)
        if pkg is None:
            continue
        if pkg in it_exclude:
            continue

        if pkg in int_only:
            filtered.append(e)
        elif pkg in mixed:
            exclude = mixed[pkg].get("unit_exclude")
            if exclude and re.search(exclude, e.key):
                filtered.append(e)
            # Non-excluded mixed code is unit-testable, not IT-only

    return filtered


def filter_all(entries: list[CoverageEntry], config: dict) -> list[CoverageEntry]:
    """Filter entries to all service code (exclude generated)."""
    prefix = config["pkg_prefix"]
    filtered = []
    for e in entries:
        if is_generated_code(e.key):
            continue
        if prefix in e.key:
            filtered.append(e)
    return filtered


# ============================================================================
# Coverage calculation
# ============================================================================

COVERAGE_FILES = {
    "unit": "coverage_unit_apifrontend.out",
    "integration": "coverage_integration_apifrontend.out",
    "e2e": "coverage_e2e_apifrontend.out",
}

# Legacy file names (fallback)
LEGACY_FILES = {
    "unit": "cover.out",
    "integration": "cover-it.out",
    "e2e": "cover-e2e.out",
}


def find_coverage_file(tier: str) -> Optional[str]:
    """Find the coverage profile for a tier, trying canonical then legacy names."""
    canonical = COVERAGE_FILES[tier]
    if Path(canonical).exists():
        return canonical
    legacy = LEGACY_FILES.get(tier)
    if legacy and Path(legacy).exists():
        return legacy
    return None


def calc_tier_coverage(tier: str, scope: str, config: dict) -> str:
    """Calculate coverage for a specific tier and scope."""
    covfile = find_coverage_file(tier)
    if not covfile:
        return "-"

    entries = parse_go_coverage_file(covfile)
    if not entries:
        return "-"

    if scope == "unit":
        filtered = filter_unit_testable(entries, config)
    elif scope == "integration":
        filtered = filter_integration_testable(entries, config)
    elif scope == "all":
        filtered = filter_all(entries, config)
    else:
        return "-"

    return calculate_coverage(filtered)


def calc_all_tiers(config: dict) -> str:
    """Calculate merged All Tiers coverage (line-level OR across tiers)."""
    all_entries_per_tier = []

    for tier in ["unit", "integration", "e2e"]:
        covfile = find_coverage_file(tier)
        if not covfile:
            continue
        entries = parse_go_coverage_file(covfile)
        if entries:
            filtered = filter_all(entries, config)
            all_entries_per_tier.append(filtered)

    if not all_entries_per_tier:
        return "-"

    merged = merge_coverage_entries(*all_entries_per_tier)
    return calculate_coverage(merged)


def calc_per_package_breakdown(config: dict) -> list[dict]:
    """Calculate per-package coverage breakdown for detailed reporting."""
    prefix = config["pkg_prefix"]
    all_pkgs = (
        config["unit_only_pkgs"]
        + list(config["mixed_pkgs"].keys())
        + config["integration_only_pkgs"]
    )

    results = []
    for tier_name, _ in COVERAGE_FILES.items():
        covfile = find_coverage_file(tier_name)
        if not covfile:
            continue
        entries = parse_go_coverage_file(covfile)
        if not entries:
            continue

        pkg_entries: dict[str, list[CoverageEntry]] = {p: [] for p in all_pkgs}
        for e in entries:
            if is_generated_code(e.key):
                continue
            pkg = get_pkg_from_key(e.key, prefix)
            if pkg and pkg in pkg_entries:
                pkg_entries[pkg].append(e)

        for pkg in all_pkgs:
            if pkg_entries[pkg]:
                covered, total = calculate_coverage_raw(pkg_entries[pkg])
                pct = f"{(covered / total) * 100:.1f}%" if total > 0 else "0.0%"
                tier_label = "UT" if pkg in config["unit_only_pkgs"] else (
                    "IT" if pkg in config["integration_only_pkgs"] else "UT+IT"
                )
                results.append({
                    "package": pkg,
                    "tier_source": tier_name,
                    "tier_label": tier_label,
                    "coverage": pct,
                    "covered": covered,
                    "total": total,
                })

    return results


# ============================================================================
# Output formatters
# ============================================================================

def output_markdown(config: dict) -> str:
    """Generate markdown table matching kubernaut's PR comment format."""
    unit_ut = calc_tier_coverage("unit", "unit", config)
    unit_it = calc_tier_coverage("unit", "integration", config)
    it_ut = calc_tier_coverage("integration", "unit", config)
    it_it = calc_tier_coverage("integration", "integration", config)
    e2e_all = calc_tier_coverage("e2e", "all", config)
    all_tiers = calc_all_tiers(config)

    # Best per-scope across tiers
    def best(a: str, b: str) -> str:
        def to_float(s: str) -> float:
            try:
                return float(s.replace("%", ""))
            except (ValueError, AttributeError):
                return -1.0
        va, vb = to_float(a), to_float(b)
        if va < 0 and vb < 0:
            return "-"
        return a if va >= vb else b

    best_ut = best(unit_ut, it_ut)
    best_it = best(unit_it, it_it)

    lines = [
        "## Coverage Report (By Test Tier)",
        "",
        "| Scope | Unit Tests | Integration Tests | E2E | Best |",
        "|-------|-----------|-------------------|-----|------|",
        f"| Unit-Testable | {unit_ut} | {it_ut} | - | {best_ut} |",
        f"| Integration-Testable | {unit_it} | {it_it} | - | {best_it} |",
        f"| All Code | - | - | {e2e_all} | {all_tiers} |",
        "",
        "### Column Definitions",
        "",
        "- **Unit-Testable**: Pure logic code (config, validators, tools, agents, session, severity triage, resilience)",
        "- **Integration-Testable**: I/O-dependent code (MCP handler, KA/DS clients, launcher, controller, TLS); auth excluded — tested by UT+E2E",
        "- **E2E**: End-to-end test coverage (full MCP workflows in Kind cluster)",
        "- **All Code / All Tiers**: Line-by-line merge across all tiers (any tier covering a line counts)",
        "",
        "### Quality Targets",
        "",
        "- Unit-Testable: >= 80%",
        "- Integration-Testable: >= 80%",
        "- All Tiers: >= 80%",
        "",
        "---",
        "",
        "_Generated by `make coverage-report-markdown` | See [Testing Coverage Methodology](docs/testing/TESTING_COVERAGE_METHODOLOGY.md) for details_",
        "",
        "<!-- Sticky Pull Request Commentcoverage-report -->",
    ]
    return "\n".join(lines)


def output_table(config: dict) -> str:
    """Generate terminal-friendly table output with per-package breakdown."""
    unit_ut = calc_tier_coverage("unit", "unit", config)
    unit_it = calc_tier_coverage("unit", "integration", config)
    it_ut = calc_tier_coverage("integration", "unit", config)
    it_it = calc_tier_coverage("integration", "integration", config)
    e2e_all = calc_tier_coverage("e2e", "all", config)
    all_tiers = calc_all_tiers(config)

    lines = [
        "=" * 90,
        "  APIFRONTEND COVERAGE REPORT (By Test Tier)",
        "=" * 90,
        "",
        f"  {'Scope':<25} {'Unit Tests':<15} {'IT Tests':<15} {'E2E':<10} {'All Tiers':<10}",
        "  " + "-" * 75,
        f"  {'Unit-Testable':<25} {unit_ut:<15} {it_ut:<15} {'-':<10} {'-':<10}",
        f"  {'Integration-Testable':<25} {unit_it:<15} {it_it:<15} {'-':<10} {'-':<10}",
        f"  {'All Code':<25} {'-':<15} {'-':<15} {e2e_all:<10} {all_tiers:<10}",
        "",
    ]

    # Per-package breakdown from UT profile
    lines.append("  " + "-" * 75)
    lines.append("  Per-Package Breakdown (from Unit Test profile):")
    lines.append("  " + "-" * 75)
    lines.append(f"  {'Package':<25} {'Tier':<10} {'Coverage':<12} {'Covered/Total':<20}")
    lines.append("  " + "-" * 75)

    breakdown = calc_per_package_breakdown(config)
    ut_breakdown = [b for b in breakdown if b["tier_source"] == "unit"]
    ut_breakdown.sort(key=lambda x: x["package"])

    for b in ut_breakdown:
        lines.append(
            f"  {b['package']:<25} {b['tier_label']:<10} {b['coverage']:<12} "
            f"{b['covered']}/{b['total']}"
        )

    lines.extend([
        "",
        "  " + "=" * 75,
        "  COLUMN DEFINITIONS:",
        "    Unit-Testable: Pure logic (config, validators, tools, agent, session, severity, resilience)",
        "    Integration-Testable: I/O code (handler, ka, ds, launcher, controller, tlswiring); auth excluded — tested by UT+E2E",
        "    All Tiers: Line-by-line merged coverage — any tier covering a line counts",
        "",
        "  QUALITY TARGETS:",
        "    Unit-Testable:        >= 80%",
        "    Integration-Testable: >= 80%",
        "    All Tiers:            >= 80%",
        "",
        "  Run 'make test-unit test-integration-containers test-e2e' to update all coverage files.",
        "  " + "=" * 75,
    ])

    return "\n".join(lines)


def output_json(config: dict) -> str:
    """Generate JSON output for CI/CD integration."""
    data = {
        "service": "apifrontend",
        "tiers": {
            "unit_testable": {
                "from_unit_tests": calc_tier_coverage("unit", "unit", config),
                "from_integration_tests": calc_tier_coverage("integration", "unit", config),
            },
            "integration_testable": {
                "from_unit_tests": calc_tier_coverage("unit", "integration", config),
                "from_integration_tests": calc_tier_coverage("integration", "integration", config),
            },
            "e2e": calc_tier_coverage("e2e", "all", config),
            "all_tiers_merged": calc_all_tiers(config),
        },
        "packages": calc_per_package_breakdown(config),
    }
    return json.dumps(data, indent=2)


# ============================================================================
# Main
# ============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Generate per-tier coverage report for apifrontend."
    )
    parser.add_argument(
        "--format", choices=["table", "markdown", "json"], default="table",
        help="Output format (default: table)"
    )
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parent.parent.parent
    os.chdir(repo_root)

    config = AF_SERVICE_CONFIG["apifrontend"]

    if args.format == "markdown":
        print(output_markdown(config))
    elif args.format == "json":
        print(output_json(config))
    else:
        print(output_table(config))


if __name__ == "__main__":
    main()
