# Enhancement Proposal: ModernBERT-Based Read-Only Console Intent Routing

**Status**: Proposed ModernBERT-based approach; implementation and runtime not approved
**Updated**: 2026-10-24
**Tracking**: [#856](https://github.com/jordigilh/kubernaut/issues/856)
**Parent**: [Standalone Console](STANDALONE_CONSOLE.md), [#855](https://github.com/jordigilh/kubernaut/issues/855)

## Goal and Initial Boundary

Fine-tune a pretrained ModernBERT encoder to recognize bounded console commands
instead of requiring a generative LLM call for every lookup. The initial scope
is **read-only navigation and filtering**, alongside normal controls in the
standalone application.

```text
User command and bounded view context
  -> candidate intent and parameters
  -> authoritative entity/scope resolution and validation
  -> registered view and authorized read handler
```

Keep the intent set small and grounded in supported interactions. Severity,
environment, namespace, cluster, resource, and time range are parameters where
applicable, not a separate class for every combination. Recognizing a category
alone does not establish its filters or target identity.

The model must not generate React/HTML, SQL, shell commands, arbitrary API
endpoints, or permission decisions. Application code maps accepted intent to
predefined interactions. A versioned API/response contract requires separate
review; historical draft examples are not an already implemented wire contract.

## Example Interactions

| Input | Permitted initial interaction |
|---|---|
| Show critical alerts across production | Open the fleet view with validated severity/environment filters |
| Show rollback workflows | Search/filter the catalog, not execute a workflow |
| What workflow ran for this investigation? | Resolve the selected record and open execution details |
| Approve the cert rotation | Locate/open a permitted approval review record; do not submit a decision |
| Investigate that PVC failure | Open existing context or prepare the existing interactive flow; do not implicitly start new investigation work |

Follow-ups should be grounded in retrieved data and carry supported, validated
intent/parameters. References such as "those failures" may use bounded, typed
view context; entity identity and scope must be resolved server-side. Ordinary
navigation, filters, breadcrumbs, and history remain coherent and usable without
the recognizer. Result summaries and counts should reflect actual retrieved
data; templated summaries need not use generated prose.

## Proposed Model and Evaluation Baselines

| Approach | Role in the proposal | What to validate |
|---|---|---|
| Fine-tuned ModernBERT | Proposed encoder-only intent classifier; token classification is available for extraction | Labeled data, joint recognition/extraction quality, serving footprint, latency, and memory |
| Rules and entity matching | Transparent baseline for supported phrases/values | Paraphrase coverage and maintenance |
| Small sentence encoder plus classifier | Smaller learned comparison baseline | Domain adaptation, examples, and parameter extraction |

ModernBERT is an **encoder**, not an autoregressive decoder. The proposed approach
fine-tunes pretrained weights; training from scratch is not proposed.
Parameter extraction can use rules and/or trained token/span classification.
Resource names must resolve against current permitted inventory rather than
become a fixed per-resource training taxonomy.

The ModernBERT checkpoint, training data, inference dependency/runtime, serving
architecture, and hardware footprint require evaluation before implementation
or deployment approval. No inference SLA, model accuracy, or future move to a
generative classifier is promised. The existing LLM investigation experience
remains separate and unchanged.

## Safety and Failure Contract

- Treat browser/model output as untrusted. Enforce supported intent, schema,
  resource identity, scope, and RBAC independently of classifier confidence.
- Initial dispatch must not create investigations, approve/reject requests,
  execute/roll back workflows, or mutate settings. Existing explicit interaction
  paths and attributable backend gates remain authoritative.
- Abstain or clarify when input, reference, scope, or parameters cannot be
  resolved safely. Test negation and compound requests; supported read-only
  exclusions may be represented as validated filters. Do not silently broaden scope.
- Preserve the current view and usable direct navigation/filters on recognition
  failure. Explain the unsupported interaction; an automatic LLM fallback is not
  required for basic navigation.
- Define model/contract attribution, redaction, retention, and routing
  audit/diagnostics before production use. Do not assume raw commands may be logged.

## Evaluation Requirements

### Recognition

- Compare ModernBERT and the baselines on the same reviewed, held-out corpus, measuring per-intent
  quality, **joint intent-and-parameter correctness**, target resolution, and
  abstention/coverage.
- Include paraphrases, typos, negation, unseen names, context references,
  unsupported/compound requests, and read-versus-action confusion.
- Separate paraphrase/template families and relevant entity examples between
  training and evaluation to limit near-duplicate leakage.
- Agree supported languages, acceptance thresholds, and confidence calibration
  from evidence. Maximum softmax score is not calibrated certainty.

### Runtime and Safety

- Measure warm/cold latency, concurrency behavior, and memory on intended
  hardware; record conditions without promising unmeasured advantages or creating
  noisy wall-clock CI merge gates.
- Unit evidence proves recognition/extraction/validation; integration evidence
  proves real authorized dispatch; browser journeys prove view/filter/context
  updates, follow-ups, and fallback.
- Negative tests must cover forged parameters, unauthorized/ambiguous targets,
  invalid model output, negation, and model failure without unauthorized access
  or state-changing dispatch.
- Preserve regression evidence for the existing investigation, approval,
  execution, and verification journey.

## Acceptance and Implementation Gates

- [ ] Command bar and supported follow-ups coexist with direct navigation and
  filters, producing grounded results and consistent view context.
- [ ] The proposed ModernBERT recognizer is evaluated against baselines for joint
  intent/parameter/target quality, calibrated abstention, and deployment footprint
  before implementation or deployment approval.
- [ ] Backend validation and authorization remain independent of model output;
  the initial dispatch surface is read-only.
- [ ] Unit, integration, browser, and negative-safety evidence prove the accepted
  interaction/failure contracts without changing the existing dynamic flow.
- [ ] Before implementation, approve BR mapping, API/security/model-serving
  design, test plan, and wiring manifest. No infrastructure or release date is
  approved by this proposal.

## References

- [Historical DD-CONSOLE-001](../../architecture/decisions/DD-CONSOLE-001-intent-driven-navigation-architecture.md)
  separates recognition from layout, but its RHDH integration, old delivery
  phases, and automatic rule-to-LLM progression are not current commitments.
- [ModernBERT model card](https://huggingface.co/answerdotai/ModernBERT-base)
- [ModernBERT sequence/token classification support](https://huggingface.co/docs/transformers/model_doc/modernbert)
