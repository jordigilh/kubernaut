# DD-AUDIT-010: Idempotent WorkflowExecution Audit Contract

**Status**: Approved and implemented  
**Version**: 1.0  
**Date**: 2026-10-15  
**Issue**: #2512  
**Related**: BR-AUDIT-005, ADR-034, DD-AUDIT-002, DD-WE-010

## Decision

WorkflowExecution lifecycle audit events use a deterministic client-supplied
`event_id` derived from the WFE namespace/name and logical event action. The
DataStorage audit write contract accepts this optional UUID and records it in
the unpartitioned `audit_event_idempotency` registry, treating repeated writes
of the same logical event as idempotent.

The typed `WorkflowExecutionAuditPayload` also carries a generic execution
reference and RR UID:

- execution resource kind, name, namespace, and cluster ID;
- `remediation_request_uid`.

The existing `pipelinerun_name` field remains for backward compatibility, but
new consumers use the generic execution reference.

## Rationale

A Job can be created successfully while the WFE status update fails. A retry
must not emit a second logical `execution.started`/terminal event. The existing
buffered audit interface is asynchronous and the request previously had no
client event ID, so controller-only in-memory deduplication could not survive
restarts or batch delivery.

RR name correlation is operationally useful but names can be recreated. RR UID
and the generic execution reference make AU-3/CC7.2 reconstruction unambiguous
across recreated RR objects, Job and Tekton backends, and fleet clusters.

## DataStorage behavior

- `event_id` remains optional for backward compatibility; absent values retain
  server-generated UUID behavior.
- Present duplicate `event_id` values are idempotent and do not append a second
  audit row or advance the hash chain. The registry is required because the
  partitioned audit table cannot infer a global unique arbiter on `event_id`
  alone.
- The registry stores a canonical payload fingerprint (excluding chain position
  and write timestamp); reusing an event ID for changed event content is
  rejected rather than silently acknowledged.
- A duplicate is not silently treated as a different event; the existing row is
  the authoritative logical event.
- The OpenAPI schema, generated ogen client, conversion layer, single-event and
  batch repository paths, and their tests remain aligned.

## Control objectives

- FedRAMP AU-2/AU-3/AU-9/AU-11
- SOC2 CC7.2; CC8.1 for controlled schema/change implementation
- OWASP ASVS 5.0 V16.2.1/V16.3.3/V16.5.3 and V8.2.2

## Alternatives rejected

| Alternative | Reason rejected |
|---|---|
| Controller-local `sync.Map` deduplication | Lost on restart and ineffective across replicas |
| Status condition as the only marker | Cannot atomically coordinate a durable audit write and K8s status update |
| RR name-only correlation | Recreated RR names can conflate lifecycles |
| Event-data-only execution identity | Does not prevent duplicate audit rows without durable event idempotency |
