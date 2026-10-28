# Kubernaut Investigator: Hermes Agent Prototype

Proof-of-concept for replacing Kubernaut's custom Go investigation loop with NousResearch Hermes Agent as the runtime harness.

## Structure

```
hermes-kubernaut-prototype/
├── config.yaml              # Hermes agent configuration (model, tools, budget)
├── SOUL.md                  # Agent persona (investigation protocol, output format, constraints)
├── skills/
│   ├── k8s-investigate/     # Kubernetes cluster inspection tools
│   │   └── SKILL.md
│   ├── workflow-catalog/    # Kubernaut workflow catalog search
│   │   └── SKILL.md
│   └── submit-result/       # Structured output submission
│       └── SKILL.md
└── README.md
```

## How It Maps to Current Kubernaut Agent

| Current Go code | This prototype |
|---|---|
| `internal/kubernautagent/investigator/investigator_loop.go` | Hermes `conversation_loop.py` (built-in) |
| `internal/kubernautagent/investigator/investigator_rca.go` | SOUL.md Phase 1 protocol + `k8s-investigate` skill |
| `internal/kubernautagent/investigator/investigator_workflow_selection.go` | SOUL.md Phase 2 protocol + `workflow-catalog` skill |
| `internal/kubernautagent/investigator/investigator_tools.go` | Hermes `model_tools.py` dispatch (built-in) |
| `internal/kubernautagent/investigator/investigator_gates.go` | SOUL.md constraints (rollback verification, preconditions) |
| `pkg/kubernautagent/tools/k8s/*.go` | `skills/k8s-investigate/SKILL.md` (Python kubernetes client) |
| `pkg/kubernautagent/tools/registry/` | Hermes `tools/registry.py` (built-in auto-discovery) |
| `pkg/kubernautagent/llm/` | Hermes native model support (config.yaml model block) |
| `internal/kubernautagent/prompt/templates/` | SOUL.md + per-skill instructions |

## To Deploy on OpenShift (Phase 0 Spike)

```bash
# 1. Create namespace
oc new-project kubernaut-hermes-spike

# 2. Create ConfigMaps for skills
oc create configmap soul-md --from-file=SOUL.md=SOUL.md
oc create configmap skill-k8s --from-file=SKILL.md=skills/k8s-investigate/SKILL.md
oc create configmap skill-catalog --from-file=SKILL.md=skills/workflow-catalog/SKILL.md
oc create configmap skill-submit --from-file=SKILL.md=skills/submit-result/SKILL.md
oc create configmap hermes-config --from-file=config.yaml=config.yaml

# 3. Create ServiceAccount with investigation RBAC
oc create sa hermes-investigator
oc adm policy add-cluster-role-to-user kubernaut-agent-investigator -z hermes-investigator

# 4. Deploy (based on cluster's working hermes-outreach pattern)
# See deployment.yaml (TODO: generate from this prototype)

# 5. Test
# Port-forward or route to Hermes dashboard
# Send: "Investigate Deployment/payment-api in demo-storefront"
```

## Validation Criteria

1. Hermes correctly identifies ImagePullBackOff on payment-api
2. Hermes checks ReplicaSet history and finds revision 4 is healthy
3. Hermes searches workflow catalog and selects `rollback-deployment-v1`
4. Hermes fills in correct parameters (TARGET_RESOURCE_NAME, TARGET_NAMESPACE, REVISION)
5. Output matches `InvestigationResult` schema
6. Total investigation time < 5 minutes

## Key Differences from Current Agent

| Aspect | Current Go Agent | Hermes Prototype |
|---|---|---|
| Loop | Custom `investigator_loop.go` | Hermes built-in `conversation_loop.py` |
| Tool execution | Sequential (one at a time) | Concurrent (up to 4 parallel) |
| Budget control | `maxTurns: 40` (flat) | `IterationBudget` (with refunds for cheap ops) |
| Error handling | Custom retry in `investigator_gates.go` | Hermes `classify_api_error` + failover |
| Streaming | Custom `emitToSink` + LazySink | Hermes native streaming via gateway |
| Model support | Custom clients per provider | Hermes native (any OpenAI-compatible) |
| Framework maintenance | ~4000 lines custom Go | 0 lines (use upstream Hermes) |

## Next Steps After Spike

If the spike validates the approach (investigation produces correct results):

1. Build HTTP adapter (FastAPI wrapper exposing `/api/v1/incident/analyze`)
2. Wire audit hooks (emit events to DataStorage on each tool call)
3. Add session management (map Hermes sessions to Kubernaut InvestigationSession CRDs)
4. Package as container image for Helm chart
5. Integration test against AI Analysis controller
