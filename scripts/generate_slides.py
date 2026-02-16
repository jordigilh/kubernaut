#!/usr/bin/env python3
"""Generate kubernaut-technical-overview.pptx from hardcoded slide content.

Usage:
    /tmp/pptx-venv/bin/python scripts/generate_slides.py

Output:
    docs/presentations/kubernaut-technical-overview.pptx
"""

from pathlib import Path

from pptx import Presentation
from pptx.util import Inches, Pt, Emu
from pptx.dml.color import RGBColor
from pptx.enum.text import PP_ALIGN, MSO_ANCHOR
from pptx.enum.shapes import MSO_SHAPE

# ---------------------------------------------------------------------------
# Theme colours
# ---------------------------------------------------------------------------
NAVY = RGBColor(0x1B, 0x2A, 0x4A)
DARK_GRAY = RGBColor(0x33, 0x33, 0x33)
LIGHT_GRAY_BG = RGBColor(0xF2, 0xF2, 0xF2)
CODE_BG = RGBColor(0xF5, 0xF5, 0xF0)
WHITE = RGBColor(0xFF, 0xFF, 0xFF)
ACCENT_BLUE = RGBColor(0x2B, 0x57, 0x9A)
TABLE_HEADER_BG = RGBColor(0x1B, 0x2A, 0x4A)
TABLE_ALT_ROW = RGBColor(0xF0, 0xF4, 0xF8)

FONT_BODY = "Calibri"
FONT_CODE = "Courier New"

OUTPUT = Path(__file__).resolve().parent.parent / "docs" / "presentations" / "kubernaut-technical-overview.pptx"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _set_slide_bg(slide, color):
    """Set solid background colour for a slide."""
    bg = slide.background
    fill = bg.fill
    fill.solid()
    fill.fore_color.rgb = color


def _add_title_box(slide, text, left, top, width, height, font_size=28, bold=True, color=NAVY):
    """Add a styled title text box."""
    txBox = slide.shapes.add_textbox(left, top, width, height)
    tf = txBox.text_frame
    tf.word_wrap = True
    p = tf.paragraphs[0]
    p.text = text
    p.font.size = Pt(font_size)
    p.font.bold = bold
    p.font.color.rgb = color
    p.font.name = FONT_BODY
    return txBox


def _add_subtitle_box(slide, text, left, top, width, height, font_size=14, color=DARK_GRAY):
    """Add a subtitle / purpose text box."""
    txBox = slide.shapes.add_textbox(left, top, width, height)
    tf = txBox.text_frame
    tf.word_wrap = True
    p = tf.paragraphs[0]
    p.text = text
    p.font.size = Pt(font_size)
    p.font.color.rgb = color
    p.font.name = FONT_BODY
    p.font.italic = True
    return txBox


def _add_bullets(slide, items, left, top, width, height, font_size=12, color=DARK_GRAY):
    """Add a bulleted list text box."""
    txBox = slide.shapes.add_textbox(left, top, width, height)
    tf = txBox.text_frame
    tf.word_wrap = True
    for i, item in enumerate(items):
        if i == 0:
            p = tf.paragraphs[0]
        else:
            p = tf.add_paragraph()
        p.text = item
        p.font.size = Pt(font_size)
        p.font.color.rgb = color
        p.font.name = FONT_BODY
        p.space_after = Pt(4)
        p.level = 0
        pPr = p._pPr
        if pPr is None:
            from pptx.oxml.ns import qn
            pPr = p._p.get_or_add_pPr()
        # Bullet character
        from pptx.oxml.ns import qn
        buChar = pPr.makeelement(qn("a:buChar"), {"char": "\u2022"})
        # Remove existing bullets
        for existing in pPr.findall(qn("a:buChar")):
            pPr.remove(existing)
        for existing in pPr.findall(qn("a:buNone")):
            pPr.remove(existing)
        pPr.append(buChar)
    return txBox


def _add_code_block(slide, code_text, left, top, width, height, font_size=9):
    """Add a code block with light background."""
    # Background rectangle
    shape = slide.shapes.add_shape(MSO_SHAPE.RECTANGLE, left, top, width, height)
    shape.fill.solid()
    shape.fill.fore_color.rgb = CODE_BG
    shape.line.fill.background()  # no border
    shape.shadow.inherit = False

    # Code text box on top
    txBox = slide.shapes.add_textbox(
        left + Inches(0.15), top + Inches(0.1),
        width - Inches(0.3), height - Inches(0.2),
    )
    tf = txBox.text_frame
    tf.word_wrap = True
    lines = code_text.strip().split("\n")
    for i, line in enumerate(lines):
        if i == 0:
            p = tf.paragraphs[0]
        else:
            p = tf.add_paragraph()
        p.text = line
        p.font.size = Pt(font_size)
        p.font.name = FONT_CODE
        p.font.color.rgb = DARK_GRAY
        p.space_after = Pt(1)
        p.space_before = Pt(0)
    return txBox


def _add_label(slide, text, left, top, width, height, font_size=10, bold=True, color=ACCENT_BLUE):
    """Add a small label (e.g. 'CRD: ...' or 'Architecture: ...')."""
    txBox = slide.shapes.add_textbox(left, top, width, height)
    tf = txBox.text_frame
    tf.word_wrap = True
    p = tf.paragraphs[0]
    p.text = text
    p.font.size = Pt(font_size)
    p.font.bold = bold
    p.font.color.rgb = color
    p.font.name = FONT_BODY
    return txBox


# ---------------------------------------------------------------------------
# Slide builders
# ---------------------------------------------------------------------------

def slide_title(prs):
    """Slide 0: Title slide."""
    slide = prs.slides.add_slide(prs.slide_layouts[6])  # blank
    _set_slide_bg(slide, WHITE)

    _add_title_box(slide, "Kubernaut", Inches(1), Inches(1.8), Inches(8), Inches(0.8),
                   font_size=44, color=NAVY)
    _add_title_box(slide, "Technical Overview", Inches(1), Inches(2.6), Inches(8), Inches(0.6),
                   font_size=28, bold=False, color=ACCENT_BLUE)
    _add_subtitle_box(slide, "Kubernetes-Native Autonomous Remediation Platform",
                      Inches(1), Inches(3.4), Inches(8), Inches(0.5),
                      font_size=16, color=DARK_GRAY)
    _add_label(slide, "Technical Audience  |  February 2026",
               Inches(1), Inches(4.2), Inches(8), Inches(0.4),
               font_size=12, bold=False, color=DARK_GRAY)


def slide_pipeline_overview(prs):
    """Slide 1: Pipeline Overview."""
    slide = prs.slides.add_slide(prs.slide_layouts[6])
    _set_slide_bg(slide, WHITE)

    _add_title_box(slide, "Pipeline Overview", Inches(0.5), Inches(0.3), Inches(9), Inches(0.6))

    _add_subtitle_box(
        slide,
        "When a monitoring alert fires, Kubernaut ingests it, enriches it with cluster context, "
        "asks an LLM to diagnose the root cause and select a remediation workflow, executes that "
        "workflow safely, and then measures whether the fix actually worked.",
        Inches(0.5), Inches(0.9), Inches(9), Inches(0.7),
        font_size=12, color=DARK_GRAY,
    )

    pipeline_diagram = (
        "Alert (Prometheus / K8s Event)\n"
        "  |\n"
        "  v\n"
        "Gateway --> RemediationRequest CRD\n"
        "  |\n"
        "  v\n"
        "Signal Processing --> SignalProcessing CRD\n"
        "  |                   (enrichment + Rego classification)\n"
        "  v\n"
        "AI Analysis --> AIAnalysis CRD\n"
        "  |             (HolmesGPT RCA + workflow selection + Rego approval)\n"
        "  v\n"
        "Remediation Orchestrator --> coordinates lifecycle\n"
        "  |\n"
        "  v\n"
        "Workflow Execution --> WorkflowExecution CRD\n"
        "  |                    (Tekton PipelineRun / K8s Job)\n"
        "  v\n"
        "Effectiveness Monitor --> EffectivenessAssessment CRD\n"
        "  |                       (health + alerts + metrics + spec hash)\n"
        "  v\n"
        "Notification --> NotificationRequest CRD\n"
        "                 (Slack, Email, Console)"
    )
    _add_code_block(slide, pipeline_diagram,
                    Inches(0.8), Inches(1.7), Inches(7.5), Inches(4.5), font_size=10)

    _add_label(
        slide,
        "Supporting: DataStorage (audit + workflow catalog), HolmesGPT-API (LLM), AuthWebhook (SOC2 identity)",
        Inches(0.5), Inches(6.4), Inches(9), Inches(0.4),
        font_size=10, bold=False, color=DARK_GRAY,
    )


def _service_slide(prs, title, purpose, arch, crd, features, code_label=None, code_text=None):
    """Generic service slide builder."""
    slide = prs.slides.add_slide(prs.slide_layouts[6])
    _set_slide_bg(slide, WHITE)

    _add_title_box(slide, title, Inches(0.5), Inches(0.3), Inches(9), Inches(0.6))

    _add_subtitle_box(slide, purpose,
                      Inches(0.5), Inches(0.9), Inches(9), Inches(0.55),
                      font_size=11, color=DARK_GRAY)

    label_y = Inches(1.45)
    labels = []
    if arch:
        labels.append(f"Architecture: {arch}")
    if crd:
        labels.append(f"CRD: {crd}")
    _add_label(slide, "  |  ".join(labels),
               Inches(0.5), label_y, Inches(9), Inches(0.3),
               font_size=10, bold=True, color=ACCENT_BLUE)

    # Decide layout based on whether we have code
    if code_text:
        bullet_width = Inches(4.5)
        bullet_height = Inches(4.5)
        _add_bullets(slide, features,
                     Inches(0.5), Inches(1.8), bullet_width, bullet_height,
                     font_size=10)
        if code_label:
            _add_label(slide, code_label,
                       Inches(5.2), Inches(1.8), Inches(4.5), Inches(0.3),
                       font_size=9, bold=True, color=ACCENT_BLUE)
        _add_code_block(slide, code_text,
                        Inches(5.2), Inches(2.1), Inches(4.5), Inches(4.8),
                        font_size=8)
    else:
        _add_bullets(slide, features,
                     Inches(0.5), Inches(1.8), Inches(9), Inches(5.0),
                     font_size=11)


def slide_gateway(prs):
    _service_slide(prs,
        title="Gateway",
        purpose="Webhook receiver that ingests alerts from monitoring systems, normalizes them, "
                "deduplicates, and creates RemediationRequest CRDs. The single entry point for all signals.",
        arch="HTTP server (stateless, K8s-native)",
        crd="RemediationRequest",
        features=[
            "Pluggable adapters: Prometheus AlertManager, K8s Events",
            "Status-based deduplication via RR fingerprint lookups (no Redis)",
            "Replay prevention: header-first + body-fallback freshness validation",
            "Label-based scope filtering (kubernaut.ai/managed=true)",
            "K8s Lease-based distributed locking for multi-replica safety",
            "Circuit breaker against K8s API cascading failures",
            "Buffered audit events to DataStorage",
            "Graceful shutdown with readiness probe (503 during drain)",
        ],
    )


def slide_signal_processing(prs):
    code = (
        'package signalprocessing.priority\n'
        '\n'
        'import rego.v2\n'
        '\n'
        '# Severity rank: higher = more urgent\n'
        'severity_rank := 3 if {\n'
        '    lower(input.signal.severity) == "critical"\n'
        '}\n'
        'severity_rank := 2 if {\n'
        '    lower(input.signal.severity) == "warning"\n'
        '}\n'
        'severity_rank := 1 if {\n'
        '    lower(input.signal.severity) == "info"\n'
        '}\n'
        'default severity_rank := 0\n'
        '\n'
        '# Environment rank: higher = more sensitive\n'
        'env_rank := 3 if {\n'
        '    lower(input.environment) == "production"\n'
        '}\n'
        'env_rank := 2 if {\n'
        '    lower(input.environment) == "staging"\n'
        '}\n'
        'default env_rank := 0\n'
        '\n'
        '# Combined score -> priority\n'
        'score := severity_rank + env_rank\n'
        '\n'
        'result := {"priority": "P0",\n'
        '           "policy_name": "score-based"}\n'
        '    if { score >= 6 }\n'
        'result := {"priority": "P1",\n'
        '           "policy_name": "score-based"}\n'
        '    if { score >= 4; score < 6 }\n'
        'default result := {"priority": "P3",\n'
        '    "policy_name": "default-catch-all"}'
    )
    _service_slide(prs,
        title="Signal Processing",
        purpose="Enriches raw signals with K8s context (namespace labels, pod status, owner chain, "
                "HPA, PDB) and classifies them using operator-defined Rego policies.",
        arch="Kubernetes controller",
        crd="SignalProcessing",
        features=[
            "Phase flow: Pending > Enriching > Classifying > Categorizing > Completed",
            "K8s enrichment: namespace, pod, deployment, owner chain, node, PDB, HPA",
            "5 Rego classifiers: Environment, Priority, Severity, Business, CustomLabels",
            "Detected labels: GitOps, Helm, PDB-protected, HPA-managed, service mesh",
            "Signal mode: predictive vs reactive",
            "Degraded mode: partial enrichment when target not found",
        ],
        code_label="Rego -- Priority Classification",
        code_text=code,
    )


def slide_ai_analysis(prs):
    code = (
        'package aianalysis.approval\n'
        '\n'
        'import rego.v2\n'
        '\n'
        'default require_approval := false\n'
        '\n'
        '# Production requires manual approval\n'
        'require_approval if {\n'
        '    is_production\n'
        '}\n'
        '\n'
        '# 3+ recovery attempts require approval\n'
        'require_approval if {\n'
        '    is_multiple_recovery\n'
        '}\n'
        '\n'
        '# Production + unvalidated target\n'
        'require_approval if {\n'
        '    is_production\n'
        '    not target_validated\n'
        '}\n'
        '\n'
        '# Production + failed detections\n'
        'require_approval if {\n'
        '    is_production\n'
        '    has_failed_detections\n'
        '}\n'
        '\n'
        '# Production + stateful workload\n'
        'require_approval if {\n'
        '    is_production\n'
        '    is_stateful\n'
        '}'
    )
    _service_slide(prs,
        title="AI Analysis",
        purpose="Runs AI-powered root cause analysis via HolmesGPT-API, selects a remediation "
                "workflow, and evaluates approval requirements via Rego policy. The brain of the pipeline.",
        arch="Kubernetes controller",
        crd="AIAnalysis",
        features=[
            "Phase flow: Pending > Investigating > Analyzing > Completed",
            "HolmesGPT integration: async session (submit > poll > result)",
            "Workflow selection: HAPI returns workflow ID, image, parameters, rationale",
            "Rego approval policy: approved / manual_review_required / denied",
            "LLM identifies affected resource (e.g. Deployment instead of Pod)",
            "Recovery flow: uses history to avoid repeating failed remediations",
            "Execution engine: supports tekton and job backends",
            "WorkflowNotNeeded: LLM can determine problem self-resolved",
        ],
        code_label="Rego -- Approval Policy",
        code_text=code,
    )


def slide_hapi(prs):
    _service_slide(prs,
        title="HolmesGPT-API (HAPI)",
        purpose="Internal Python service (FastAPI) that wraps HolmesGPT SDK and orchestrates "
                "LLM-driven incident analysis, recovery proposals, and workflow discovery.",
        arch="HTTP service (FastAPI, not a controller)",
        crd=None,
        features=[
            "3 API endpoints: incident analysis, recovery analysis, post-execution analysis",
            "3-step workflow discovery protocol against DataStorage:",
            "  1. list_available_actions (what can I do?)",
            "  2. list_workflows (which workflows implement this action?)",
            "  3. get_workflow (fetch schema and parameters)",
            "Remediation history enrichment: past remediations injected into LLM prompt",
            "Three-way hash comparison: detects spec drift since last remediation",
            "Session-based async API: submit investigation > poll for result",
            "Mock LLM mode for deterministic testing",
            "Auth: K8s ServiceAccount tokens (TokenReview + SubjectAccessReview)",
        ],
    )


def slide_remediation_orchestrator(prs):
    _service_slide(prs,
        title="Remediation Orchestrator",
        purpose="Coordinates the full remediation lifecycle by creating and watching child CRDs "
                "(SP, AA, WE, NR, EA). The conductor of the pipeline.",
        arch="Kubernetes controller",
        crd="RemediationRequest (owns all child CRDs via owner references)",
        features=[
            "Phase state machine: Pending > Processing > Analyzing > AwaitingApproval > Executing > Completed/Failed/TimedOut",
            "Creates child CRDs at the right phase transitions",
            "Approval flow: RemediationApprovalRequest when Rego requires human approval",
            "Routing engine: scope blocking, consecutive failure blocking, dedup",
            "Timeout handling: global (1h) + per-phase (Processing 5m, Analyzing 10m, Executing 30m)",
            "Pre-remediation SHA-256 spec hash of AI-resolved target resource",
            "EffectivenessAssessment on all terminal phases (Completed, Failed, TimedOut)",
            "AI-resolved target: uses LLM AffectedResource instead of signal source",
            "Atomic status updates with RetryOnConflict",
            "Full audit trail to DataStorage at every phase transition",
        ],
    )


def slide_workflow_execution(prs):
    _service_slide(prs,
        title="Workflow Execution",
        purpose="Executes remediation workflows via Tekton PipelineRuns or K8s Jobs, monitors progress, "
                "and reports structured success/failure details back to the orchestrator.",
        arch="Kubernetes controller",
        crd="WorkflowExecution",
        features=[
            "Executor registry: tekton (PipelineRun + OCI bundle) and job (K8s Job)",
            "Deterministic resource naming prevents duplicate executions",
            "Lease-based resource locking from targetResource identity",
            "5-minute cooldown before lock release (prevents rapid re-execution)",
            "Structured failure details: TaskFailed, OOMKilled, DeadlineExceeded, ImagePullBackOff",
            "External deletion handling: detects PipelineRun/Job deleted outside controller",
            "Dedicated execution namespace (kubernaut-workflows)",
            "Audit events: workflow.started, workflow.completed, workflow.failed",
        ],
    )


def slide_effectiveness_monitor(prs):
    _service_slide(prs,
        title="Effectiveness Monitor",
        purpose="Assesses whether the remediation actually worked by running four independent checks "
                "after a stabilization window, then emitting structured audit events. Closes the feedback loop.",
        arch="Kubernetes controller",
        crd="EffectivenessAssessment",
        features=[
            "4 assessment components, each scored 0.0-1.0:",
            "  Health: pod status, readiness, restarts, CrashLoopBackOff, OOMKilled",
            "  Alert resolution: queries AlertManager for signal resolution",
            "  Metrics: 5 PromQL queries (CPU, memory, latency p95, error rate, throughput)",
            "  Spec hash: SHA-256 comparison of pre/post target .spec",
            "Kind-aware health: Deployment/RS/SS/DS -> label-based pod listing; Pod -> direct; others -> N/A",
            "Stabilization window (default 5m) + validity window (default 30m)",
            "Spec drift guard: invalidates if target changes during assessment",
            "Assessment reasons: full, partial, no_execution, expired, spec_drift, metrics_timed_out",
            "5 typed audit events emitted to DataStorage",
        ],
    )


def slide_notification(prs):
    _service_slide(prs,
        title="Notification",
        purpose="Delivers notifications to configured channels with retries, label-based routing, "
                "and circuit breakers. Informs operators about remediation outcomes and escalations.",
        arch="Kubernetes controller",
        crd="NotificationRequest",
        features=[
            "Phase flow: Pending > Sending > Retrying > Sent / PartiallySent / Failed",
            "Delivery channels: Slack (webhook + circuit breaker), Console, File, Log",
            "Label-based routing: ConfigMap-driven rules with hot-reload (no restart)",
            "Exponential backoff retries",
            "Circuit breaker prevents cascading failures to external webhooks",
            "Notification types: approval requests, timeout escalations, completion summaries",
            "Audit events: message.sent, message.failed",
            "Atomic status updates per delivery attempt",
        ],
    )


def slide_datastorage(prs):
    _service_slide(prs,
        title="DataStorage",
        purpose="Centralized HTTP API for audit events, workflow catalog, and remediation history. "
                "The only service that talks directly to PostgreSQL. All others use its REST API.",
        arch="HTTP server (OpenAPI-driven, ogen-generated)",
        crd=None,
        features=[
            "Audit events API: batch write, query by correlation ID, hash chain verification (SOC2)",
            "Workflow catalog API: CRUD, action-type taxonomy, version lifecycle (active/deprecated)",
            "Workflow discovery API: 3-step protocol for LLM-driven remediation selection",
            "Remediation history API: two-tier windowing (24h recent + 90d historical)",
            "Three-way hash comparison for spec drift detection",
            "OpenAPI-first: data-storage-v1.yaml with auto-generated Go and Python clients",
            "PostgreSQL primary storage, Redis dead letter queue for failed audit writes",
            "K8s auth: TokenReview + SubjectAccessReview",
            "Graceful shutdown with connection draining",
        ],
    )


def slide_authwebhook(prs):
    _service_slide(prs,
        title="AuthWebhook",
        purpose="Kubernetes admission webhook that injects authenticated user identity into CRD "
                "status updates for SOC2 CC8.1 audit compliance. Ensures every remediation action "
                "is traceable to a human or service account.",
        arch="Mutating + Validating admission webhook",
        crd=None,
        features=[
            "Mutating webhooks for identity injection:",
            "  WorkflowExecution: status.initiatedBy, status.approvedBy",
            "  RemediationApprovalRequest: status.approvedBy, status.rejectedBy",
            "  RemediationRequest: status.lastModifiedBy, status.lastModifiedAt",
            "Validating webhook: audit event before NotificationRequest deletion",
            "Forgery detection: overwrites user-provided fields, logs tampering attempts",
            "Decision validation: enforces Approved/Rejected/Expired for approvals",
            "Namespace selector: kubernaut.ai/audit-enabled=true",
            "mTLS via cert-manager; failure policy Fail (rejects if webhook is down)",
        ],
    )


def slide_coverage(prs):
    """Slide 13: Coverage Snapshot table."""
    slide = prs.slides.add_slide(prs.slide_layouts[6])
    _set_slide_bg(slide, WHITE)

    _add_title_box(slide, "Code Coverage (PR #90, February 2026)",
                   Inches(0.5), Inches(0.3), Inches(9), Inches(0.6))

    rows = [
        ("Service", "Unit", "Integration", "E2E", "All Tiers"),
        ("Signal Processing", "87.3%", "61.4%", "58.2%", "85.4%"),
        ("AI Analysis", "80.0%", "73.6%", "53.8%", "87.6%"),
        ("Workflow Execution", "74.0%", "67.9%", "56.0%", "82.9%"),
        ("Remediation Orchestrator", "79.9%", "59.8%", "49.1%", "82.1%"),
        ("Notification", "75.5%", "57.6%", "49.5%", "73.3%"),
        ("Effectiveness Monitor", "72.1%", "64.9%", "68.8%", "81.9%"),
        ("Gateway", "65.5%", "42.5%", "59.0%", "81.5%"),
        ("DataStorage", "60.1%", "34.9%", "48.7%", "65.4%"),
        ("HolmesGPT-API", "79.0%", "62.1%", "59.1%", "94.1%"),
        ("AuthWebhook", "50.0%", "49.0%", "41.6%", "78.4%"),
    ]

    n_rows = len(rows)
    n_cols = len(rows[0])
    left = Inches(0.8)
    top = Inches(1.2)
    width = Inches(8.4)
    height = Inches(5.0)

    table_shape = slide.shapes.add_table(n_rows, n_cols, left, top, width, height)
    table = table_shape.table

    # Column widths
    col_widths = [Inches(2.8), Inches(1.2), Inches(1.6), Inches(1.2), Inches(1.6)]
    for i, w in enumerate(col_widths):
        table.columns[i].width = w

    for row_idx, row_data in enumerate(rows):
        for col_idx, cell_text in enumerate(row_data):
            cell = table.cell(row_idx, col_idx)
            cell.text = cell_text
            p = cell.text_frame.paragraphs[0]
            p.font.size = Pt(11)
            p.font.name = FONT_BODY

            if row_idx == 0:
                # Header row
                p.font.bold = True
                p.font.color.rgb = WHITE
                cell.fill.solid()
                cell.fill.fore_color.rgb = TABLE_HEADER_BG
                p.alignment = PP_ALIGN.CENTER
            else:
                p.font.color.rgb = DARK_GRAY
                if col_idx == 0:
                    p.alignment = PP_ALIGN.LEFT
                    p.font.bold = True
                else:
                    p.alignment = PP_ALIGN.CENTER
                # Alternating row colors
                if row_idx % 2 == 0:
                    cell.fill.solid()
                    cell.fill.fore_color.rgb = TABLE_ALT_ROW
                else:
                    cell.fill.solid()
                    cell.fill.fore_color.rgb = WHITE

            # Vertical alignment
            cell.vertical_anchor = MSO_ANCHOR.MIDDLE


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    prs = Presentation()
    prs.slide_width = Inches(10)
    prs.slide_height = Inches(7.5)

    slide_title(prs)
    slide_pipeline_overview(prs)
    slide_gateway(prs)
    slide_signal_processing(prs)
    slide_ai_analysis(prs)
    slide_hapi(prs)
    slide_remediation_orchestrator(prs)
    slide_workflow_execution(prs)
    slide_effectiveness_monitor(prs)
    slide_notification(prs)
    slide_datastorage(prs)
    slide_authwebhook(prs)
    slide_coverage(prs)

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    prs.save(str(OUTPUT))
    print(f"Generated: {OUTPUT}")
    print(f"Slides:    {len(prs.slides)}")
    print(f"Size:      {OUTPUT.stat().st_size / 1024:.1f} KB")


if __name__ == "__main__":
    main()
