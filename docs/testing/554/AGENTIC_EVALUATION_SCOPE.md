# Threat Remediation: Agentic Attacker-Defender Evaluation Scope

**Status**: Proposed evaluation scope; not an approved execution plan or test evidence

**Created**: 2026-10-23

**Business requirement**: [BR-SECURITY-554: Threat Remediation](../../requirements/BR-SECURITY-554-threat-remediation.md), primarily BR-SECURITY-554-010

**Effectiveness signals**: Undecided; selection and validation require a future spike

**Related issue**: [#554](https://github.com/jordigilh/kubernaut/issues/554)

## 1. Purpose and Claim Boundary

Define a reusable scope for evaluating Kubernaut against an adaptive agentic
adversary when Threat Remediation is ready for testing. ARTEX is the candidate
AI penetration-testing system; Kubernaut is the governed remediation system.
The experiment is not limited to generating an alert and checking that a
workflow runs. The attacker continues to pursue its objective as remediation
changes the environment, including after Kubernaut escalates.

The central effectiveness hypothesis is:

> ARTEX reliably succeeds without Kubernaut, but succeeds less often with it,
> without unacceptable operational damage.

"Reliably" and "unacceptable" require pre-agreed definitions and repeated
measurements. This is a hypothesis to test, not a promised result. Evidence can
support effectiveness against the tested adversary, objectives, and conditions;
it cannot establish protection against every agentic threat.

The comparative goal is defined; the threat-remediation effectiveness signals
are not. When the team is ready, a dedicated spike must determine which signals
can reliably establish the relevant outcomes. Examples in this scope are
questions and candidate observations, not selected signals or a scoring design.

This document preserves the evaluation intent for future test planning and
effectiveness assessment. It does not approve implementation, install ARTEX,
authorize attack execution, select a production integration, or claim that
Threat Remediation is shipped. The parent requirement's approval gates remain
in force.

## 2. Requirements and the Open Effectiveness-Signal Design

| Requirement | Evaluation evidence sought |
|---|---|
| BR-SECURITY-554-001 / -002 | Authoritative findings, source provenance, and incident/run correlation survive an adaptive attack. |
| BR-SECURITY-554-003 / -004 | Investigation identifies affected workloads, identities, uncertainty, and evidence-backed response options. |
| BR-SECURITY-554-005 / -006 | Policy and approval remain independent of the model; only registered, versioned workflows make remediation changes. |
| BR-SECURITY-554-007 / -008 | Responses preserve evidence and service safety; containment, recurrence, and residual access are independently verified. |
| BR-SECURITY-554-009 / -010 | The complete encounter is reconstructable, with baseline comparisons, escalation outcomes, and operational feedback. |

The existing [Effectiveness Monitor overview](../../services/crd-controllers/07-effectivenessmonitor/overview.md)
describes deterministic health, alert, metric, and spec/hash observations, with
DataStorage calculating the aggregate effectiveness score. They are not a
selected or validated signal set for Threat Remediation. Neither a healthy
workload nor a high existing score proves that credentials were revoked,
secondary access was removed, or an attacker was contained. Collateral-damage
attribution is also not an existing Level 1 capability.

The parent requirement defines desired outcomes and pilot measurement needs;
it does not settle which operational signals reliably prove those outcomes.
The future spike must define the threat-specific signals rather than assume
that the existing operational-health model transfers to security response.
It must not waive the parent requirement's outcome or audit obligations.

Do not reinterpret the existing aggregate score, select a replacement formula,
invent an Effectiveness Monitor API, or assume automatic ingestion of benchmark
results. Whether any current observations are useful, what new observations
are needed, and how results should eventually integrate with effectiveness
assessment remain open. Production changes require a separate approved design,
implementation, and wiring tests after the spike.

## 3. Scope and Non-Goals

### In scope

- Real, bounded attack actions against explicitly authorized disposable lab
  targets, rather than synthetic findings alone.
- Adaptive attacker behavior after a response, including retries, alternative
  paths, persistence, and continued activity during escalation.
- Kubernaut's investigation, governed response, outcome verification, and
  recognition of its autonomous-remediation limits.
- Repeated comparisons with and without Kubernaut, including independent
  evidence of attacker objectives, service health, and forensic preservation.
- Versioned development improvements informed by observed failures, followed
  by new evaluations and deterministic regression tests.

### Out of scope

- Public or production targets, real customer data, uncontrolled destructive
  actions, or reproducing an actual victim's compromise.
- Proving universal protection, reproducing the scale of a nation-state
  campaign, or treating ARTEX's reputation as benchmark validity.
- Replacing detection products, preventive controls, or a customer's incident
  response process.
- Giving Kubernaut's investigation model arbitrary state-changing execution
  rights or bypassing policy to improve benchmark results.
- Automatic model retraining, self-modifying production code, or uncontrolled
  attacker-defender self-improvement.

## 4. Roles and Information Boundaries

| Role | Responsibility and boundary |
|---|---|
| ARTEX: attacker | Pursue a declared objective using approved tools and identities within fixed scope and budgets. Adapt to observable target behavior, not privileged defender transcripts. |
| Security source: detector | Produce authoritative findings from observed activity. Preserve source identity, timestamps, resource/principal identity, and original evidence. |
| Kubernaut: defender | Investigate, recommend, and route authorized changes through registered workflows; verify outcomes or escalate. Do not consume the attacker's private plan as an oracle. |
| Independent evaluator: referee | Observe effects and adjudicate security and safety outcomes independently of either agent's self-reported success. This is a logical evaluation role, not a mandated new service. |
| Lab operator / human responder | Approve scope, enforce external limits, operate the stop mechanism, and provide human response only in a declared experimental arm. |

Neither agent may modify the evaluator, evidence store, run limits, or the
other agent's control plane. Preserve the realistic feedback that each agent
could legitimately observe. Administrative access used to provision the lab
must not become attacker access or an undocumented defender advantage.

An initial campaign should select one authoritative source and two or three
response journeys, as required by BR-SECURITY-554-010. Source selection must
match the chosen behaviors: a runtime sensor is not automatically evidence of
cloud identity activity, and an ARTEX transcript is not a detector integration.

## 5. Candidate Encounters

These are scenario families for later selection, not implemented tests or
claims of built-in ARTEX Kubernetes coverage. Their local identifiers are
evaluation references, not production types or assigned UT/IT/E2E test IDs.
The spike must establish which signals could prove the candidate outcomes;
this table does not prescribe their instrumentation or scoring.

| Reference | Attacker objective | Defender outcome to examine |
|---|---|---|
| EVAL-TR-001: trusted identity abuse | Reach a protected dummy resource using a disposable workload identity. | Attribute identity and workload; apply only approved scope correction or credential response; verify denied unauthorized access while legitimate access still works. |
| EVAL-TR-002: unauthorized egress | Reach a lab-controlled protected endpoint from a disposable workload. | Apply a registered isolation response; verify blocked protected-system access, retained forensic access, and acceptable service impact. |
| EVAL-TR-003: persistence / alternative access | Retain access through an explicitly seeded test-only secondary identity or privilege grant. | Do not close the incident merely because the original access path stopped working; verify the remaining path or escalate the uncertainty. |

For every selected encounter, define the starting foothold separately from the
attacker's final objective. If the attacker starts inside a workload, that
precondition is not itself an attacker win. "Break-in" must mean a specified,
independently observable objective, not a vague change in agent confidence.

Each encounter also evaluates unavailable safe responses, missing or
contradictory evidence, recurrence, and verification failure where applicable.
Do not manufacture unsafe permissions or disable defenses solely to force a
particular outcome.

## 6. Comparison Design

### Experimental arms

| Arm | Conditions | Purpose |
|---|---|---|
| Baseline | ARTEX attacks with the same preventive controls, sensors, workload, and evaluator, but without Kubernaut remediation or human containment during the observation budget. | Establish attacker capability and exposure under the selected conditions. |
| Kubernaut autonomous response | The same starting conditions plus Kubernaut's approved automation policy and registered workflows. Human-review requests remain visible; no operator rescues the run. | Measure the contribution and limits of autonomous remediation. |
| Human-assisted response, optional | The Kubernaut arm plus a predeclared responder procedure and response delay. | Measure assisted defense separately; do not attribute human containment to autonomous Kubernaut. |

Recommendation-only validation precedes the response arms. It can establish
investigation quality and safe workflow selection, but cannot demonstrate
containment benefit from unexecuted recommendations. State-changing trials
require the parent requirement's approvals and safety/wiring evidence first.

### Comparability and repetition

1. Use separate calibration runs to establish that the encounter is meaningful.
   Freeze the evaluation scenarios before collecting scored results; do not
   retain only attacks that happened to succeed.
2. Restore equivalent initial workload, identity, network, data, and relevant
   agent-memory state for each matched trial. Keep model versions, prompts,
   toolsets, budgets, detector rules, and preventive controls comparable.
3. Repeat trials with predeclared counts and vary or randomize arm order to
   reduce order effects. Record supported seeds, model parameters, provider
   versions, and environmental differences; do not promise deterministic LLM
   trajectories or identical attacks after the defender changes the target.
4. Declare invalidation rules before execution. Infrastructure/evaluator
   failures and missing evidence must be reported, not silently discarded.
   A failure caused by remediation is an outcome, not an unrelated lab fault.
5. Report all valid runs and every exclusion, with counts, denominators, and
   uncertainty. A baseline with no observed attacker success cannot establish
   that Kubernaut reduced attacker success for that encounter.

## 7. Adaptation and Escalation

### Two meanings of evolution

**Within an encounter**, ARTEX may change tactics based on actual responses.
Kubernaut may investigate new findings and adapt its recommendations within
the same declared policy, tool, and workflow boundaries. Record these changes
as behavioral adaptation, not evidence of model training.

**Between campaigns**, reviewed changes to prompts, models, detectors,
workflows, or policies can address observed failures. Assign a new benchmark
configuration/version and rerun comparisons. Keep held-out encounters when
assessing generalization; do not present tuning against a known trace as
independent validation. Preserve supported history-based behavior within an
encounter, but prevent undocumented cross-arm memory leakage.

### Escalation is an observation, not an automatic win or loss

Kubernaut may exhaust its available safe responses or be unable to establish
a verified outcome. Record escalation and its reason independently from the
attacker's objective and from service health.

- Keep ARTEX active after escalation within the original scope and remaining
  budget. Do not end the experiment merely because Kubernaut needs help.
- Preserve established containment and preventive controls. Escalation does
  not intentionally remove them, grant attacker access, or authorize bypass
  of required human approval.
- In the autonomous arm, do not insert unreported human rescue. In the
  assisted arm, record handoff, responder latency, actor, and actions.
- Determine whether the attacker achieved its objective before escalation,
  afterward, or not within the observation period. Existing containment may
  continue to hold even when additional remediation is unavailable.
- Adjudicate escalation correctness separately: a justified safety escalation
  can coexist with subsequent compromise; premature escalation or failure to
  escalate can reveal a different defect.

Record the times of escalation, attacker objective achievement, and any human
intervention. The escalation-to-objective interval is meaningful only when
the objective occurs afterward. Otherwise report the ordering or that the
objective was not observed, rather than fabricating an interval.

## 8. Effectiveness Questions and Required Future Spike

No threat-remediation effectiveness signal catalog, measurement contract,
scoring formula, weights, thresholds, or integration design is selected by
this document. Signal definition is intentionally deferred until the team is
ready to investigate it. This document does not start that spike.

### Questions the evaluation must eventually answer

| Dimension | Open measurement question |
|---|---|
| Attacker objective | What independent observations establish objective achievement, non-achievement within a bounded run, or an unknown result? |
| Security containment | What distinguishes sustained containment from interruption, alert clearance, recurrence, or an unobserved alternative access path? |
| Remediation / escalation | How can response completion, justified escalation, failure, and human intervention be distinguished from actual security effectiveness? |
| Operational and forensic safety | Which observations establish acceptable service impact, legitimate access, data integrity, and forensic preservation? |
| Evaluation validity | Which missing, stale, contradictory, or tampered observations make a conclusion unreliable, and how should uncertainty be reported? |

A completed workflow, a cleared alert, a stopped attacker process, or an
agent saying "fixed" is not sufficient proof of containment. Conversely, no
observed attacker success before a time limit is not proof of permanent
prevention. These distinctions constrain the future signal design; they do
not supply it.

### Spike scope when ready

The spike should produce evidence-backed decisions on these five areas:

| Area | Required spike output |
|---|---|
| Signal discovery | Candidate threat-specific effectiveness signals, their sources, trust boundaries, resource/identity scope, and collection feasibility. Start from applicable security control objectives, including versioned OWASP ASVS requirements for application/API surfaces. Distinguish the objective, detector input, workflow status, and independent outcome evidence. |
| Signal validation | Lab evidence showing which candidates meaningfully prove the selected security and safety outcomes, where they fail, and what they cannot observe. |
| Measurement semantics | Definitions for freshness, observation/recurrence windows, missing or contradictory data, uncertainty, causal attribution, and escalation/human-response effects. |
| Comparison and interpretation | An operational definition of attacker success, suitable measures and denominators for baseline comparisons, uncertainty methods, and proposed acceptance/safety thresholds. Avoid conclusions biased by missing observations or selected successful runs. |
| Integration recommendation | Whether and how existing effectiveness observations can be reused, what new capabilities are necessary, and which design/implementation work needs separate approval. No production architecture or scoring model is presumed. |

The spike's completion criterion is a reviewed recommendation backed by
evidence, with supported signals, rejected candidates, limitations, and
unresolved questions made explicit. A proposed signal without evidence of
what it proves is not enough to declare measurement readiness.

Only after the spike and subsequent approvals should an executable plan
freeze selected signals, collection rules, metrics, sample sizes, thresholds,
and reporting semantics. Report negative and inconclusive results as well as
successes. A single showcase encounter or arbitrary weighted score must not
substitute for the agreed comparative evidence.

### Prior-art guidance for the spike (non-binding)

The following sources are starting points for investigation, not endorsements
of a particular tool, signal catalog, or reward function. The observations
below were researched on 2026-10-23; confirm the applicable versions and
methodologies when the spike begins. None establishes that its measurements
transfer unchanged to an ARTEX-versus-Kubernaut Kubernetes lab.

| Prior art | Useful idea to investigate | Limit on reuse |
|---|---|---|
| [MITRE ATT&CK Evaluations methodology](https://attackevals.mitre-engenuity.org/methodology-overview) | Scope encounters by adversary behavior and vary procedures implementing the same technique. Use technique-level evidence to explain which behavior was attempted and observed. | ATT&CK coverage or detection of a procedure is not proof of sustained remediation, and a finite emulation plan is not an adaptive adversary's entire search space. |
| [MITRE Caldera operation results](https://caldera.readthedocs.io/en/latest/Operation-Results.html) | Preserve attempted actions, execution times, outputs, planner/configuration context, and reasons for skipped actions. Distinguish an action never attempted from one attempted and blocked. | Command status and agent-reported output are attack-side evidence, not an independent containment verdict. Full reports and flattened event logs contain different information; adopting Caldera is not required. |
| [CAGE Challenge 4 / CybORG](https://cage-challenge.github.io/cage-challenge-4/pages/) | Evaluate repeated red/blue encounters alongside legitimate-user activity. Its green agents make service disruption observable, and defensive actions can incur mission-impact penalties. | This is a simulated environment with a specified red-agent strategy and reward model, not evidence of LLM-adversary performance on Kubernetes. Borrow the separation of attack effects and mission impact, not its reward values or episode counts. |
| [NIST SP 800-53A Rev. 5](https://csrc.nist.gov/pubs/sp/800/53/a/r5/final) | Tailor assessment procedures to the objective, relevant controls, and risk tolerance; connect each effectiveness claim to evidence that actually assesses it. | A control mapping is not a measurement implementation or certification. The publication does not select Kubernaut's threat-remediation signals or acceptance thresholds. |
| [Kubernetes NetworkPolicy semantics](https://kubernetes.io/docs/concepts/services-networking/network-policies/) | Test enforcement rather than object existence. Policy application is asynchronous; enforcement needs a supporting network plugin, and effects on existing connections are implementation-defined. | A successful API write or changed spec hash does not establish effective isolation. Validate the actual network-plugin version, paths, protocols, and connection states used by the encounter. |

### OWASP ASVS control objectives for applicable application/API surfaces

[OWASP Application Security Verification Standard (ASVS)](https://owasp.org/www-project-application-security-verification-standard/)
is a directly relevant source of security control objectives: it describes
the application/API behavior that should hold, rather than just the attack
procedure or the mechanics of running a benchmark. Use applicable objectives
to inform the signal-definition spike, alongside the existing BRs and
platform-specific security objectives.

These examples reference the stable [ASVS 5.0.0 release](https://github.com/OWASP/ASVS/tree/v5.0.0).
Pin the standard's version and use version-qualified requirement identifiers;
do not use moving `latest` or bleeding-edge content as a frozen benchmark
baseline. Reassess the applicable stable release when the spike begins.

Keep three complementary questions distinct:

1. **Adversary behavior**: ATT&CK and attack traces describe what ARTEX tried.
2. **Security objective**: applicable ASVS requirements describe the expected
   secure application/API behavior before or after an authorized response.
3. **Effectiveness evidence**: the future spike establishes which independent
   observations demonstrate that behavior, its persistence, and its limits.

The following are candidate mappings for applicability review, not selected
controls, approved verification levels, or chosen effectiveness signals. The
identifiers and their meanings were checked against the pinned release;
descriptions below are scoped interpretations, not replacements for its text.

| Objective family and source | Example ASVS 5.0.0 requirements | Question for the future spike |
|---|---|---|
| [Authorization and tenant boundaries](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md) | `v5.0.0-8.2.1`, `v5.0.0-8.2.2`, `v5.0.0-8.4.1` | Where applicable, can the unauthorized consumer no longer invoke the protected function or access another consumer's data, while the legitimate consumer still can? Establish actual enforcement, not just an RBAC/policy object change. |
| [Application session termination](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x16-V7-Session-Management.md) | `v5.0.0-7.4.1`, `v5.0.0-7.4.2` | Where application sessions are involved, does termination or account disablement prevent continued use of the affected sessions? Do not assume that rotating one credential terminates existing sessions. |
| [Trusted validation and ordered business flows](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x11-V2-Validation-and-Business-Logic.md) | `v5.0.0-2.2.1`, `v5.0.0-2.2.2`, `v5.0.0-2.3.1` | Can untrusted finding content or an adversarial caller bypass backend validation or skip an application-defined approval/response step? Verify the applicable flow's required sequence, rather than trusting the model's account of it. |
| [Authorization changes](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x17-V8-Authorization.md) and [secure failure handling](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x25-V16-Security-Logging-and-Error-Handling.md) | `v5.0.0-8.3.1`, `v5.0.0-8.3.2`, `v5.0.0-16.5.3` | Are authorization decisions enforced by a trusted service, do permission changes take effect as required, and do relevant errors avoid fail-open behavior? Assess each requirement's conditions; an alert or later reversal cannot undo information already disclosed. |
| [Attributable, protected security evidence](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x25-V16-Security-Logging-and-Error-Handling.md) | `v5.0.0-16.2.1`, `v5.0.0-16.3.1`, `v5.0.0-16.3.2`, `v5.0.0-16.4.2`, `v5.0.0-16.4.3` | Can authentication/authorization events and their actors be reconstructed from protected, logically separate evidence? Useful evidence must still be distinguished from proof that the attack objective was prevented or contained. |

Assess two surfaces separately: the protected workload application's security
objectives and Kubernaut's own intake, approval, and response-control interfaces.
Evidence that Kubernaut protects its own API does not by itself prove that it
contained the attacker in a workload.

ASVS is not a complete Kubernetes runtime, node-isolation, or agentic-threat
standard. Do not automatically equate application session termination with
Kubernetes service-account-token invalidation, or application tenant controls
with namespace isolation. Identify the relevant platform semantics and record
unmapped objectives explicitly. Conventional input-validation requirements
also do not, by themselves, establish protection against LLM prompt injection.

For each applicable objective, the spike should record its versioned ID and
verification level, the scoped expected behavior, candidate observations,
independent proving/counterexample tests, and limitations or non-applicability.
Some examples above are level-specific; no ASVS level is selected here. Do not
convert a percentage of ASVS requirements passed into a threat-effectiveness
score, or imply whole-ASVS compliance from a small encounter set. Control
objectives guide what must be shown; the observations and scoring remain open.

### Candidate observation families, not selected effectiveness signals

Use these examples to generate hypotheses and counterexamples. The spike may
reject or replace any of them; they are not a minimum signal catalog or an
instruction to extend the current Effectiveness Monitor.

| Candidate family | Example question or lab observation | What it would not prove alone |
|---|---|---|
| Attacker-objective effects | Did the protected lab endpoint independently record access to the dummy resource by the test identity? | A request attempt or successful tool exit does not prove the resource was reached. Later containment cannot erase an already achieved objective; prevention and subsequent recovery must remain distinguishable. |
| Effective capability removal | Does the previously usable test credential or permission cease enabling the protected operation, including relevant existing sessions or cached credentials? | A revoked object, rotated secret, or denied fresh login does not prove that every prior session, secondary identity, or alternate grant was removed. |
| Persistence and alternative paths | Can a seeded secondary identity or permitted alternative route still achieve the same objective after the original path is blocked, including after escalation? | Failure on sampled paths during a bounded window does not demonstrate that no other path exists. A pod restart or resource/spec-hash change must not reset the incident's security history. |
| Legitimate mission and forensic impact | Can a benign workload/client still perform its intended operation while the unauthorized operation is blocked, and is required forensic access/evidence preserved? | A failed attack caused by shutting down the entire service is not safe containment. Readiness alone may miss legitimate-client failures or evidence loss. |
| Observation confidence over time | Are the relevant observations fresh, attributable, and available across propagation delays, recurrence, escalation, and human handoff? | Quiet logs or an idle attacker do not establish containment: the collector could be unavailable, ARTEX could have exhausted its budget, or the target could be down. Missing visibility must remain explicit. |

### Suggested validation approach

1. **Calibrate one candidate against known states.** In an independently
   authorized spike, select one dummy objective and establish known-exposed,
   known-contained, and deliberately unobservable cases. Determine whether
   the candidate distinguishes them before relying on an LLM encounter to
   discover its meaning.
2. **Pair unauthorized and legitimate operations.** Investigate a blocked
   unauthorized operation alongside an allowed benign control from the
   relevant identity/network context. This can distinguish effective scope
   restriction from an unavailable endpoint, broken network, or blanket
   shutdown. The probes themselves must stay within scope and their effects
   on the experiment must be recorded.
3. **Try to falsify apparent success.** Candidate counterexamples include a
   policy object without effective enforcement, a cleared finding while the
   objective remains reachable, a completed workflow with retained secondary
   access, and an attacker stopping because its budget expired. Include
   benign-only or authorized look-alike activity to reveal unnecessary
   containment and misleading threat labels.
4. **Test the temporal and trust limits.** Investigate propagation delays,
   existing sessions, workload replacement, recurrence, and stale or missing
   observations. Record actual effects separately from configuration changes
   and agent claims. Treat unknown outcomes and runs ending before the
   required window as unresolved, not successful remediation.
5. **Then test generalization and attribution.** Combine supported candidates
   in bounded adaptive encounters, compare against the matched baseline, and
   use held-out variations. Separate prevention, post-compromise containment,
   recovery, and human-assisted outcomes. Report all runs and missingness;
   choose denominators, observation windows, and any aggregate scoring only
   after validating what the underlying observations prove.

For each candidate, keep a short evidence record: the claim it could support,
collection point and trust boundary, calibration results and counterexamples,
temporal/missing-data behavior, and the resulting retain/reject/revise decision.
Prefer a small set of independently validated observations over a large set
of attractive but correlated proxies. This record is spike working material,
not a prescribed production schema or a decision to adopt any signal above.

## 9. Evidence and Run Records

For each trial, retain a versioned manifest and an independently reconstructable
timeline. The following are prospective evidence categories, not a selected
effectiveness-signal schema. Exact outcome observations and collection methods
depend on the future spike; the traceability obligations remain:

1. **Identity and configuration**: run/campaign identifier, arm, encounter,
   starting snapshot, scope, attacker objective, budgets, and versions of
   ARTEX, Kubernaut, models, prompts, tools, policies, workflows, and sensors.
2. **Source and attack observations**: detector finding IDs, timestamps,
   resource/principal identities, attack tool calls and responses, and actual
   target effects. Retain private agent state separately from what its
   opponent could observe.
3. **Defender lifecycle**: incident/correlation identifiers, investigation
   evidence and outputs, policy version, approval/rejection, workflow identity
   and actor, observed changes, escalation, and human actions.
4. **Independent outcomes**: the spike-selected observations, their provenance,
   limitations, and evaluator evidence references. Protected-resource probes,
   access checks, or service checks are examples to investigate, not selected
   signals.
5. **Adjudication**: all verdicts, failure attribution, unresolved uncertainty,
   exclusions, safety stops, and deviations from the declared protocol.

Correlate one encounter with all related incidents/remediation attempts rather
than assuming one attack creates exactly one remediation request. Synchronize
timestamps and document clock uncertainty. Agent transcripts are supporting
evidence, not the sole source of truth. Preserve access controls, integrity,
retention, and sensitive-data redaction outside either agent's control.

## 10. Safety, Readiness, and Execution Gates

Before any live exercise, approve the following:

1. **Scope and ownership**: named lab owner, security reviewer, evaluator, and
   responder; explicitly authorized targets and allowed actions; dummy data
   and disposable identities; restoration procedure and stop authority.
2. **Independent containment of the experiment**: enforce network, identity,
   tool, time, volume, and cost limits outside ARTEX. Protect control planes
   and evidence. Review the selected fork/build, dependencies, image digests,
   license, and actual guard behavior; application-level prompts and approval
   hooks are not a security boundary.
3. **Product readiness**: approved source contract and selected journeys,
   proven recommendation-only behavior, registered response workflows,
   scope/permission and approval checks, recovery behavior, and security
   verification with an independent evaluator. No direct model execution or
   implied unwired automatic policy-deny behavior.
4. **Measurement readiness**: completed and reviewed effectiveness-signal spike,
   followed by an approved comparison protocol, baseline calibration,
   validated signal collection, service/evidence constraints, observation
   windows, trial counts, escalation handling, and reporting/invalidation rules.
5. **Engineering and compliance readiness**: an approved formal test plan,
   requirement-to-UT/IT/E2E mapping, wiring manifests and passing proving
   tests for newly authorized components, and the applicable control review.

Use the [test plan template](../TEST_PLAN_TEMPLATE.md) when converting this
scope into an executable plan. [AGENTS.md](../../../AGENTS.md) remains the
methodology authority, including current scenario-based coverage policy if
older template wording differs. This scope is not a substitute for TDD,
wiring proof, or a full production-readiness assessment.

The live adversarial exercise is a separately authorized evaluation, not an
unbounded ordinary CI job or a wall-clock benchmark merge gate. Harness logic
and discovered regression cases should receive deterministic automated tests
following the repository's Ginkgo/Gomega and BR-traceability requirements.

Suspend a run on scope escape, unexpected real credentials/data, loss of the
external stop mechanism, exhausted limits, or unacceptable safety impact.
Loss of evaluator visibility also requires suspension rather than a success
verdict. Preserve evidence and report the stop; do not automatically relabel
defender-caused damage as an unrelated invalid run. Resume only after the
cause is understood and the lab owner reauthorizes execution.

### Applicable control objectives

| Control group | Evidence to design and assess; not a compliance certification |
|---|---|
| SOC 2 CC7.2 | Independent observations, investigation support, outcome verification, and complete encounter reconstruction. |
| FedRAMP AU-2 / AU-3 | Attributed audit events and correlation across findings, decisions, approvals, execution, and verification; project-required event identity/action/outcome/actor fields. |
| FedRAMP AU-9 / AU-11 | Protected evidence, integrity, access controls, and approved retention under the parent requirement and audit policy. |
| FedRAMP AC-4 / AC-6 / SC-8 / SI-10 | Experiment boundaries, scoped execution, confidential transport, and validation/sanitization of untrusted source content. |
| SOC 2 CC8.1 | Approved changes to the harness, integrations, workflows, and evaluation configuration through the project development/review process; not a mapping for audit-trail completeness. |

## 11. Future Deliverables and Reuse

First, complete the future signal-definition spike described in Section 8.
Its findings must inform the formal test plan; they are not supplied by this
scope document. Once measurement and execution are authorized, the evaluation
should produce:

1. A frozen protocol and per-run manifests with reproducible starting
   conditions and a complete deviation/exclusion log.
2. A baseline-versus-Kubernaut report with counts, uncertainty, independent
   security/safety verdicts, and separate human-assisted results if tested.
3. A failure/escalation analysis distinguishing detection gaps, reasoning
   errors, missing or unsafe workflows, permission constraints, timing,
   ineffective containment, and verification blind spots.
4. An evidence bundle linking the report to source findings, incident/audit
   records, workflow effects, and independent observations, with sensitive
   artifacts handled under the approved evidence policy.
5. Reviewed development follow-ups and deterministic regression cases for
   observed failures, with a newly versioned campaign after changes.

These artifacts can inform Threat Remediation effectiveness assessment,
workflow/catalog prioritization, release evidence, and future product claims.
Do not report them as passing evidence until trials have actually run. New
storage, scoring, feedback-learning, or production integrations remain
separate approval and implementation work. Their signal definitions and
assessment semantics must follow the future spike's reviewed findings rather
than the existing operational-health score by default.

The eventual report should state exactly which configuration, scenarios,
budgets, detection sources, preventive controls, observation windows, and
human assumptions its conclusion covers. Preserve this wording as the target
interpretation, subject to the actual results:

> Under the declared conditions, the tested agentic adversary achieved its
> objective less often with Kubernaut than without it, while operational and
> forensic impact stayed within the agreed limits. The result characterizes
> the tested configuration and does not establish universal threat protection.

## 12. References and Motivation

- [Threat Remediation Product Discovery](../../requirements/enhancements/THREAT_REMEDIATION_PRODUCT_DISCOVERY.md)
- [BR-SECURITY-554: Threat Remediation](../../requirements/BR-SECURITY-554-threat-remediation.md)
- [Effectiveness Monitor overview](../../services/crd-controllers/07-effectivenessmonitor/overview.md)
- [Unified Audit Table Design](../../architecture/decisions/ADR-034-unified-audit-table-design.md)
- [Service Audit Trace Requirements](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)

External motivation:

- [ARTEX English-translation fork](https://github.com/BaaRai/ARTEX): candidate
  agentic penetration tester, not a selected production dependency or a
  verified Kubernetes scenario library.
- [Microsoft Security, 2026-10-21: AI-driven, large-scale nation-state campaign using ARTEX enables supply chain compromise](https://www.microsoft.com/en-us/security/blog/2026/10/21/storm-0940-uses-artex-agentic-ai-connectwise-supply-chain-compromise/):
  motivates adaptive identity-abuse and persistence evaluation. The reported
  cloud/Microsoft 365 behaviors inform lab analogues; this is not a reproduction
  of the ConnectWise compromise or evidence that the linked fork/version was
  used in that campaign.

**Document version**: 1.2-proposed
