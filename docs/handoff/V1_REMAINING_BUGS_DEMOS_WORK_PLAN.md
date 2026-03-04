# Work plan: feature/v1.0-remaining-bugs-demos

**Scope**: P0 + P1 + P3 per [PR_V1_REMAINING_BUGS_DEMOS_SCOPE.md](./PR_V1_REMAINING_BUGS_DEMOS_SCOPE.md).  
**PR**: [#249](https://github.com/jordigilh/kubernaut/pull/249)

---

## Phase 0: Git setup (start here)

Do this first so all work is on a clean branch based on latest `main`.

| Step | Command | Notes |
|------|---------|--------|
| 1 | `git fetch origin` | Get latest from remote |
| 2 | `git checkout main` | Switch to main |
| 3 | `git pull -r origin main` | Rebase main onto origin/main |
| 4 | `git checkout -b feature/v1.0-remaining-bugs-demos` | Create and switch to the work branch |

If the branch already exists locally and you want to refresh it from main instead:

- `git checkout main && git pull -r origin main`
- `git branch -D feature/v1.0-remaining-bugs-demos` (optional: delete old local branch)
- `git checkout -b feature/v1.0-remaining-bugs-demos`
- Then either push with `--force-with-lease` after first commit, or create a new PR if you prefer a fresh PR.

---

## Phase 1: P0 — Security

Fix before any other changes; keeps security patches easy to review and backport.

| Order | Issue | Title | Suggested approach |
|-------|--------|--------|---------------------|
| 1.1 | [#229](https://github.com/jordigilh/kubernaut/issues/229) | security(rbac): Tighten cluster-wide RBAC + consolidate notification namespace into kubernaut-system | Review cluster RBAC manifests; move notification into `kubernaut-system`; tighten roles per least privilege. |
| 1.2 | [#204](https://github.com/jordigilh/kubernaut/issues/204) | sec(helm): Move hardcoded PostgreSQL/Redis credentials from values.yaml to Kubernetes Secrets | Replace hardcoded values with refs to Secrets (e.g. `existingSecret` or Helm-generated Secrets from values not in defaults). |

**Checkpoint**: Run `make lint` / relevant tests; commit P0 as one or two logical commits; push.

---

## Phase 2: P1 — Bugs

Ordered by service/area to reduce context switching. Fix one issue (or a small related set), run tests, commit, then next.

| Order | Issue | Title | Area |
|-------|--------|--------|------|
| 2.1 | [#240](https://github.com/jordigilh/kubernaut/issues/240) | Guard EA creation to only fire after successful WorkflowExecution | RO |
| 2.2 | [#205](https://github.com/jordigilh/kubernaut/issues/205) | EffectivenessAssessment created for Failed/ManualReviewRequired RR with no WFE | RO |
| 2.3 | [#214](https://github.com/jordigilh/kubernaut/issues/214) | CheckConsecutiveFailures ignores completed-but-ineffective remediations | RO |
| 2.4 | [#242](https://github.com/jordigilh/kubernaut/issues/242) | Gateway: enforce exponential backoff cooldown before creating new RRs | Gateway |
| 2.5 | [#227](https://github.com/jordigilh/kubernaut/issues/227) | K8s Event adapter fallback fingerprint includes event reason (dedup) | Gateway |
| 2.6 | [#230](https://github.com/jordigilh/kubernaut/issues/230) | GW-DEDUP-002: FlakeAttempts pollute shared gatewayNamespace with stale RRs | Gateway |
| 2.7 | [#246](https://github.com/jordigilh/kubernaut/issues/246) | EffectivenessAssessment healthScore incorrect when all pods healthy | EM |
| 2.8 | [#211](https://github.com/jordigilh/kubernaut/issues/211) | DS effectiveness query non-deterministic ordering (spec_drift) | DS |

**Checkpoint**: Full test run (unit + integration where relevant); commit per issue or small batch; push.

---

## Phase 3: P3 — Test hygiene & demos

Only after P0 and P1 are done, so demos run without known bugs.

| Order | Issue | Title | Suggested approach |
|-------|--------|--------|--------------------|
| 3.1 | [#194](https://github.com/jordigilh/kubernaut/issues/194) | Test hygiene: Remove stale Skip/PIt placeholders and implement missing test coverage | Find `Skip`/`PIt` in test code; replace with real tests or remove; add coverage where missing (per BRs). |
| 3.2 | [#144](https://github.com/jordigilh/kubernaut/issues/144) | test(demo): Validate all 17 demo scenarios end-to-end before recording | Run each scenario (e.g. from `deploy/demo/scenarios/`); fix any regressions; document status. |
| 3.3 | [#101](https://github.com/jordigilh/kubernaut/issues/101) | feat(demo): Create VHS terminal recordings for all 17 demo scenarios | Record VHS for each scenario; add/update `.tape` and outputs (e.g. GIF/MP4 if used); store under `deploy/demo/scenarios/<name>/`. |

**Checkpoint**: All 17 scenarios pass; VHS recordings done; commit and push.

---

## Summary checklist

- [ ] **Phase 0**: On `main`, pulled with `-r` from `origin/main`; branch `feature/v1.0-remaining-bugs-demos` created.
- [ ] **Phase 1**: #229, #204 fixed and committed.
- [ ] **Phase 2**: #240, #205, #214, #242, #227, #230, #246, #211 fixed and committed.
- [ ] **Phase 3**: #194, #144, #101 done and committed.
- [ ] CI green; PR #249 updated/ready for review.

---

## Reference

- Scope and in-scope issue list: [PR_V1_REMAINING_BUGS_DEMOS_SCOPE.md](./PR_V1_REMAINING_BUGS_DEMOS_SCOPE.md)
- P2/P4 (postponed): #190, #189, WE RBAC (#187, #186, #185), demos (#172, #171, #170, #168, #122), #80, #60, #105, #50
