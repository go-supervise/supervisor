# REVIEW-3 of SUPERVISOR.md (draft 5)

Confirmation pass, 2026-11-23. This review did not edit SUPERVISOR.md.

Scope: REVIEW-2 in full; SUPERVISOR.md in full (1181 lines). Line numbers
below are SUPERVISOR.md draft 5 unless prefixed `R2:`. One external fact was
checked: `gh api orgs/go-supervise` gives `created_at 2026-11-23T13:00:53Z`
(14:00:53 CET), and the org holds no repository yet.

**Verdict: P0 can start on `Compile` (§1), but not on the FSM tables (§3)
until three edits land: D1 (settle rule 2), D2 (Theorem G) and D3 (WaitTO
re-arm).** All three are local. D1 and D3 are table/helper edits that Gate 1
would catch, but only if the model includes the cases named below. D2 needs
a design decision.

---

## Part 1: are N1–N16 closed?

| N | resolution (draft 5) | status |
|---|---|---|
| N1 | windows read, not evented (451); watchdog level check (452-454) | **Closed.** Residue L1. |
| N2 | owner-held charging, R_p, effect sets, Theorem G (551-570) | **Not closed. See D2 and D4.** |
| N3 | SW over all dependents (690-692); Leaked blocks G_free (634-638) | **Closed.** |
| N4 | settled ∋ Backoff, Leaked (624-627); WaitTO for require only (219, 646) | **Closed for softness.** The edit creates D3, and the budget W is incomplete (D6). |
| N5 | pid-only (794-796) | **Closed.** Residue L2. |
| N6 | B = ⊓ site `all {}` (149-153); contents restricted (168-177) | **Closed.** Residue L3. |
| N7 | typed domains, no shell (230-237) | **Closed.** Residue L4. |
| N8 | ACL per OS, fd walk (251-259) | **Closed in design.** Residue L5. |
| N9 | schema version, legacy ℓ_f, Theorem E (187-206) | **Closed for K4 only. See D5.** |
| N10 | normalisation, prefer set, phase order (116-128) | **Closed.** Residue L6. |
| N11 | settle reorder (522-532); SEt/SEp (474-478) | **Not closed: the reorder creates D1, and SEp after fork contradicts O3 (D7).** |
| N12 | `ErrStillWanted`, `--with-dependents` (413-417) | **Closed.** Residue L7. |
| N13 | label-only mutations exempt, unit-tested (765) | **Closed.** |
| N14 | DAG induction (657-671) | **Closed.** Residue L8. |
| N15 | maximum status (550) | **Closed.** |
| N16 | home `go-supervise/supervisor` (8, 1141) | **Closed for the home**: the org's creation time matches line 8. The binary name stays open (1146). R2:305-307 says that the earlier brief named `zsup` as accepted; I cannot see the user's words, so that needs the user's confirmation, not a design change. |

---

## Part 2: defects introduced or left open by draft 5, most severe first

### HIGH

#### D1 — §3.2:524: settle rule 2 turns every killed oneshot into Done

- `record` in Stopping and Killing stores `exit := ok` **whatever the status**
  (533, and the Stopping/Killing rows at 599-600).
- Rule 2 (524) now precedes the cause rules: `oneshot ∧ exit = ok → Done,
  whatever the cause`.
- **Counter-example (StartTO).** A migration oneshot with start.timeout = 60 s
  hangs.
  1. Starting × StTO → stopwait(fail) (595).
  2. StopWait → term → Stopping; SIGTERM kills it; X1 is recorded as ok (599).
  3. TE → settle: rule 1 does not match (goal up); rule 2 matches → **Done**.
  4. Its require-dependents see Done, which satisfies G_ok (630), and start
     against a half-migrated schema.
- The same happens for cause = dep (its DB went away mid-migration) and for an
  operator `restart` of a running oneshot (cause = restart).
- REVIEW-2's own fix (R2:266-267) produced this, because it did not account
  for the "ok whatever its status" rule.

**Fix.** Rule 2 must read an exit recorded **before** any signal. Either:

- keep a third value `exit = signalled` for exits recorded in Stopping and
  Killing, which never satisfies rule 2 and is never charged; or
- restrict rule 2 to `cause ∈ {none, exit}`.

Gate 1 must include a oneshot whose StartTO fires. Scenario test: "a oneshot
killed by StartTO, dep loss or restart is never Done".

#### D2 — §3.3:562-570: Theorem G is false, through the API alone

The proof's fourth step (568), "p cannot make an owner-held service fail
through the API", ignores that **Retry re-enables** a service that fails on
its own. The crashes that follow are charged to R_inst (554) because the
incarnation is owner-held, while p pays only one R_p charge per Retry (555).

- **Counter-example.** A ∈ grant(p), `enabled = ✓` (so it is owner-held
  through `boot`), not terminal. A crashes at start because of a bad
  parameter. Take max_A = 3, max_inst = 10, budget_p = 5, all windows 60 s,
  backoff 1 s × 2.
  1. A crashes 4 times in about 10 s. R_inst = 4, then A is Failed(intensity).
  2. p: `retry A`. The effect set is {A} (A has no dependents), so it is
     allowed. R_p = 1. Four more crashes: R_inst = 8.
  3. p: `retry A`. R_p = 2, and R_inst reaches 11 > 10 → **Quiescing(1)**.
     Every service stops, including those outside grant(p).
- **Second route, outside the API.** §1.6:239 requires every service of a
  delegated subtree to run as p, and §8.2:1016 may hand p its cgroup
  (`cgroup.kill`). p can therefore crash an owner-held A ∈ grant(p) at will
  with `kill -SEGV`. If a terminal T ∉ grant(p) requires A, then
  A → Failed(intensity) makes T Failed(dep), which escalates (542) with no
  Retry at all. The statement's "through the API" (562) excludes this route,
  but the proof's last step (570) claims to cover indirect causes, and
  `ErrDelegationUid` gives p that power systematically.

**Fix (decision needed).**

- Attribute an incarnation to the principal who **started or retried** it:
  after a p-Retry, charges go to R_p until the owner acts or the window
  drains.
- Charge crashes of a service whose `run_as` is a delegated principal to that
  principal, never to R_inst. Its overflow stays local, as Failed(intensity).
- Forbid a grant from containing any require-target of a terminal service, or
  of a service outside the grant. In other words, a grant must be closed under
  reverse require edges.
- Restate Theorem G with its OS-level hypothesis.

### MEDIUM

#### D3 — §3.5:646: WaitTO arm and disarm inside one sojourn reopens H4

- "WaitTO is armed only while some require-target is not in {Ready, Done}"
  (646). So WaitTO is armed and disarmed **within** a Waiting sojourn.
- `gen` changes only on state entry (440, 511). A WaitTO armed before a disarm
  keeps a valid generation.
- Waiting × WTO → Failed(wait) is unconditional (594).
- **Counter-example.** A requires B and wants C; wait.timeout = 30 s.
  1. A enters Waiting at t = 0, with B Starting: WaitTO is armed for t = 30.
  2. At t = 29.9 B is Ready, but C is still Stopping: WaitTO is "disarmed",
     and A stays in Waiting.
  3. The t = 30 deadline was already queued, its generation matches, and A
     goes to **Failed(wait)** while every require-target is Ready. Rollback
     follows (428).
- The same pattern produces the opposite failure. A require-target that
  toggles Ready/not-Ready re-arms a fresh WaitTO each time, so the budget
  restarts.

**Fix.** Use the N1 treatment: WTO is a level check at fire time
(∃B ∈ req: state(B) ∉ {Ready, Done}), or each arm carries a token. Keep one
deadline per sojourn, measured from the first time a require-target was
pending. Gate 1 must inject a same-generation stale WTO.

#### D4 — §3.2:518 vs §3.3:554-555: the charge helper was not updated

- `charge(A)` still "append[s] now to R_A **and to R_inst**" (518).
- settle rule 3 (526) and the Failed × Ry cell (602, "R_A := ∅, charge") call
  it.
- Read literally:
  - every charge reaches R_inst, which contradicts the owner-held rule (554);
  - a Retry leaves |R_A| = 1, not 0, which contradicts "Retry clears R_A"
    (555).
- The table is data and charging is a "fixed formula" (746). An implementer
  of §3.2 builds the draft-4 semantics, and Theorem G loses even its owner-held
  premise.
- **Related.** I5's clause "∀p: |R_p(now)| ≤ budget_p, by refusal" (721)
  cannot hold. Crash charges to R_p (554) are not operations and cannot be
  refused. The behaviour when a crash charge overflows R_p is unspecified.

**Fix.**

- Define `charge(A, payer)`, with payer ∈ {owner → R_inst, p → R_p}, and
  separate it from appending to R_A.
- Write the Ry cell as `R_A := ∅; charge-payer(caller)`.
- Restate I5 for R_p as "refusals keep API-originated charges ≤ budget_p", and
  say what a crash charge past the budget does.

#### D5 — §1.5:187-206 vs §1.5:178-182: legacy values for K3/K5 are ⊤, which completeness refuses

- ℓ_f for a new allow-set is "the set of all behaviours the previous version
  did not restrict" (193). That is the universe, which **is ⊤** in K5 (§1.3:87).
- For a new K3 limit, ℓ_f = +∞ = ⊤ (86).
- Completeness Q contains "every hardening field" (181) and demands non-⊤
  (178). Q is not made version-relative; only totality is (194).
- **Counter-example.** v2 adds K5 `ioctls`. A schema-1 baseline under zsup v2
  resolves `ioctls` = ℓ = universe = ⊤, and Compile returns
  `ErrIncomplete{ioctls}`. Every instance that is not migrated refuses to boot,
  including PID 1 in mode C. That is the N9 failure again, for every kind
  except K4.

**Theorem E** is otherwise sound. Its proof (200-203) holds if:

- ℓ_f is a value distinguishable from ⊤;
- Q is taken at version N_B;
- the requirement "a new version may not change the denotation of an existing
  field" (201) also forbids extending an existing K5 field's universe with
  values that the old enforcement already permitted implicitly.

**Fix.** Evaluate Q at version N_B, with fields v_f > N_B resolved to ℓ_f and
exempt from the non-⊤ test. State the extra condition in Theorem E. Also say
what happens when N_B > the running version (it should be fail-closed
`ErrUnknownKey`/`ErrSchemaTooNew`).

#### D6 — §1.6:219-229: W(A) omits the want/order waits of upstream services

- W(B) (221) covers only B's require-targets.
- B may itself sit in Waiting on a want- or order-target C. That wait is
  bounded by one attempt of C (627), but C's SW + stop + kill + backoff + start
  is in no term of W(A).
- **Counter-example.** A require B, B want C, with C in the middle of a
  restart. A's WaitTO = W(A) is exceeded by C's single attempt, so A goes to
  Failed(wait) and its transaction rolls back.
- This contradicts the claim that the budget "covers **one** failed attempt of
  each upstream service" (229).

**Fix.** Recurse W over req ∪ wnt ∪ ord. A want- or order-target contributes
its one-attempt bound to its wanter's W, but never sends the wanter to Failed.
Alternatively, restrict the claim to the require-closure.

#### D7 — §3.1:474-476, §7.2:967: an SEp from the exec-helper occurs after fork

- O3 says that a spawn failure is synchronous and "no tree exists".
- The region conflict of §7.2, and `ErrProtectionUnavailable` evaluated on the
  host (356, 949), are detected **inside the exec-helper**. That is a forked
  child in the epoch's cgroup.
- Starting × SEp → Failed(spawn) (595) then happens with a populated tree,
  which violates I2 (711). The helper's later X1/TE arrive in Failed, where the
  table marks them ⊘ (602).
- Gate 1 models O3 as "no tree", so it cannot see this.

**Fix.**

- The helper reports the classified error on a CLOEXEC status pipe and exits.
- zsup enqueues SEp only after that pid is reaped and TreeEmpty is observed.
- Alternatively, route it as MainExit with a "spawn-permanent" status through
  StopWait/TE and a settle rule → Failed(spawn).

### LOW

- **L1 §3.1:452-454.**
  - `lastKeepalive` has no stated initial value per Ready entry. It should be
    the Ready entry time.
  - No epoch filter is stated for keepalives, which update the source without
    reaching the table.
  - Gate 1 must model the source's re-arm, not only the table.
- **L2 §4.1:796 vs §5.1:841.** "Drains the socket before reaping" is not
  possible with a single `wait4(-1)`, which reaps as it reports. It needs
  `waitid(WNOWAIT)` or pidfds. The pid-only conclusion still holds, because
  after MainExit a Rise is a no-op (598), so a post-reap pid reuse has no
  effect *(inference; the SCM_CREDENTIALS semantics on a reaped sender are
  from memory)*.
- **L3 §1.5:168-172.** `all {}` admits K6 `needs` (class H, 277). A global
  `needs` is never a ceiling, only a mass `ErrConflict`; exclude it.
  `all { restart.on = … }` conflicts with every recipe that states another
  value; say so.
- **L4 §1.6:235.** A path-region parameter can contain `;`, spaces or a
  newline. "Never through a shell" binds zsup, not a recipe that runs
  `sh -c "… $p"`. Restrict the characters of path values, or forbid params
  inside `sh -c` recipes.
- **L5 §1.7:254-258.** "Can write" needs an exhaustive list of rights:
  - Windows also has `WRITE_OWNER`, `DELETE`, `FILE_DELETE_CHILD`,
    `FILE_ADD_FILE` and `FILE_ADD_SUBDIRECTORY`;
  - macOS also has `delete`, `delete_child`, `writesecurity` and `chown`.
  An owner-change right is a write.
- **L6 §1.4:117-124.** A site-stated `prefer` inside a non-singleton interval
  can only add a second element to Π, which gives `ErrConflict`. Forbid
  `prefer` in site layers (pinning is the site's tool), or document it.
- **L7 §2.3:414.** `ErrStillWanted{by: D(X)}` names services outside grant(p)
  to a delegated caller who may lack `api.read`. Run the grant check first,
  and filter `by`.
- **L8 §3.8:760.** "≤ 4N moves" is asserted, but the induction (669) gives
  only "3 per input change", which compounds along the depth. Gate 1 should
  record the measured maximum and assert it, not a bound the proof does not
  derive.
- **L9 §3.1:475.** SEt (`EMFILE`, `ENOMEM`) is usually the **supervisor's**
  exhaustion, not the service's. Charging it to R_A and R_inst lets
  supervisor-side pressure escalate the instance. Consider not charging, with
  a delay.
- **L10 §3.3:569.** "A terminal service inside grant(p) is the owner's explicit
  choice", followed by "refuses to delegate a terminal service": the sentence
  contradicts itself. Keep the second half.
