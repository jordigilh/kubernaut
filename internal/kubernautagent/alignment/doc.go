/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package alignment implements the shadow agent alignment check subsystem
// for the Kubernaut Agent.
//
// The shadow agent runs as a transparent proxy layer around the primary
// investigation pipeline. It monitors every piece of content entering the
// agentic loop — LLM responses (via [LLMProxy]), tool outputs (via
// [ToolProxy]), and signal context — and evaluates each for prompt
// injection indicators using a dedicated or shared LLM.
//
// # Architecture
//
// The subsystem is composed of the following components:
//
//   - [InvestigatorWrapper]: Wraps the inner InvestigationRunner. Creates a
//     per-investigation [Observer], runs a canary integrity check, delegates
//     to the inner runner, collects the verdict, and applies enforcement
//     policy (enforce vs monitor mode).
//
//   - [Observer]: Per-investigation collector that receives steps via
//     [Observer.SubmitAsync], dispatches them to the [Evaluator] as
//     concurrent goroutines (capped at [DefaultMaxConcurrentEvals]),
//     and renders a final [Verdict] via [Observer.RenderVerdict].
//
//   - [Evaluator]: Sends each step to the shadow LLM wrapped in
//     cryptographically random boundary markers. Parses the LLM's
//     structured response into an [Observation] (suspicious/aligned +
//     explanation). Boundary escape attempts are immediately flagged.
//
//   - [LLMProxy]: Transparent decorator around [llm.Client] that intercepts
//     Chat calls and submits each LLM response to the [Observer] as a
//     StepKindLLMReasoning step.
//
//   - [ToolProxy]: Transparent decorator around the tool executor that
//     submits each tool result to the [Observer] as a StepKindToolResult step.
//
// # Fail-Closed Design
//
// The subsystem is designed to fail closed: any error path results in a
// suspicious verdict rather than silently passing content through.
//
//   - Evaluator timeout → suspicious observation
//   - Evaluator panic → suspicious observation (recovered, logged)
//   - Pending evaluations after verdictTimeout → suspicious verdict
//   - Canary failure → human review escalation (enforce mode or forceEscalation)
//
// # Verdict Values
//
// Verdicts use the constants [VerdictClean] ("aligned") and
// [VerdictSuspicious] ("suspicious"). These values are used in Prometheus
// metric labels, audit event data, API responses, and CRD status fields.
//
// # Metrics
//
// The package registers the following Prometheus metrics:
//
//   - kubernaut_alignment_verdict_total (labels: result, mode)
//   - kubernaut_alignment_step_total (labels: outcome)
//   - kubernaut_alignment_canary_total (labels: result)
//   - kubernaut_alignment_verdict_duration_seconds
//   - kubernaut_alignment_shadow_audit_total (labels: event_type)
package alignment
