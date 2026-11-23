# REVIEW-4 of SUPERVISOR.md (draft 7)

Confirmation pass, 2026-11-23. This review did not edit SUPERVISOR.md.

Scope: REVIEW-3 in full; SUPERVISOR.md draft 7 in full (1334 lines). Line
numbers are SUPERVISOR.md draft 7 unless prefixed `R3:`. No external fact was
checked.

**Verdict: the FSM tables (P0 part 2) should not be implemented yet.** Two HIGH
findings (E1, E2) falsify Theorem G and the R_p clause of I5 under option A.
E1 is a one-line Compile rule. E2's fix may change `settled` (§3.5), which is a
guard-table cell, so the table is not final until a fix is chosen. Three
MEDIUM edits are also local to §3.1, §3.2 and §3.7: E3 (the overflow of the
payer's counter is missing from the helpers), E4 (WaitTO's "armed" bit is
missing from the state) and E5 (P2 contradicts the uncharged SEt). `Compile`
work (P0 part 1) is not blocked.

---

## Part 1: are D1–D7 and L1–L10 closed?

REVIEW-3's own counter-examples were replayed against draft 7.

| item | draft 7 | status |
|---|---|---|
| D1 | `exit = sig` in Stopping, Killing and Leaked (524-527, 556-558, 657-661); rule 3 reads only a natural `ok` (568) | **Closed.** StartTO replay: Starting×StTO → stopwait(fail) → term → X in Stopping records `sig` → rules 2, 3 and 4 miss → rule 5, restart, charged. The dep and restart replays reach rule 4 with `sig` → Waiting, uncharged. Never Done. Residue: E7. |
| D2 | option A: payer by `run_as` (601), grant closure (602, 1079), H_OS (613) | **Not closed.** See E1 and E2. Both of REVIEW-3's routes are closed; two new routes reach Quiescing(1). |
| D3 | one deadline per sojourn, level check at fire time, `wexp`, G_wexp (480-484, 652, 692, 705) | **Closed in semantics.** Replay: WTO at t = 30 with B Ready sets `wexp`, and pending is false, so A stays in Waiting and spawns when C settles. Residue: E4 (cannot be written as a table cell) and E9. |
| D4 | `bump`/`pay`/`charge` (559-561); Ry = `R_A := ∅; pay(caller)` (603, 660); I5 restated (780) | **Closed as written.** The same class of gap moved to the overflow check: E3. |
| D5 | Q(N_B), `legacy` tag, universe hypothesis, `ErrSchemaTooNew` (197-205) | **Closed.** Replay: v2 adds `ioctls`, N_B = 1 → v_f > N_B → ℓ_f, not in Q(1), tagged `legacy`, compiles. |
| D6 | U(B) and W(A) recurse over req ∪ wnt ∪ ord (226-242) | **Closed.** Replay: W(A) ≥ U(B) ≥ W(B) ≥ U(C) covers C's attempt. |
| D7 | O3 is pre-fork only (504-508); O4 uses a CLOEXEC status pipe → X1(`perm`) → settle rule 2 at TE (509-512, 567, 1030) | **Closed.** I2 holds: Failed(spawn) is reached only through TE. Residue: E8 and E11. |
| L1 | lastKeepalive := entry time; keepalives epoch-filtered; source re-arm modelled (477-478, 813) | **Closed.** |
| L2 | WNOWAIT or pidfd, then drain, then reap (500, 859, 904) | **Closed** (the macOS route is still marked unverified). |
| L3 | `needs` excluded from `all {}`; a global `restart.on` is a K1 assertion (171-180) | **Closed.** |
| L4 | path alphabet; `ErrParamInShell` (252-254) | **Closed.** The alphabet alone suffices: it holds no shell metacharacter. The `-c` denylist misses `sh -ec`, `env sh -c` and `python -c`, but that no longer matters. Residue: E12. |
| L5 | write rights listed per OS; the check is an allow-list of read and execute (271-276) | **Closed.** |
| L6 | replaced by D-M2: Π_s and Π_r kept apart (117, 121-126) | **Closed.** Residue: E10. |
| L7 | grant check first; `by` filtered (436) | **Closed.** |
| L8 | measured golden maximum (730, 823) | **Closed.** |
| L9 | SEt uncharged, delay backoff.max (513, 653) | **Closed, but it creates E5.** |
| L10 | sentence removed (1077) | **Closed.** |

---

## Part 2: Theorem G (§3.3:615-622) and I5 (§3.7:780) under option A

The proof's case analysis covers three cases:

- R_inst overflow (619-620);
- Failed(dep) leaving the grant along a require edge (621);
- R_p overflow (622).

It misses the two routes below. Both end in Quiescing(1).

### E1 (HIGH): a service outside grant(p) may run as p

**Where.**

- Only the direction "grant ⇒ run_as p" is enforced, by `ErrDelegationUid`
  (256, 1080).
- payer(A) is keyed on `run_as` (601).
- H_OS gives p OS privilege over **every** process running as p (613).
- Nothing forbids an owner-managed service X ∉ grant(p) with
  `run_as(X) = p`. Example: the owner's main `postgres` service is terminal
  and runs as `postgres`, while `postgres` is also granted a backup job.

**Counter-example.**

1. p runs `kill -SEGV` on X's main process, max_X + 1 times within the
   window.
2. X reaches Failed(intensity) through R_A (563).
3. X is terminal, so its Failed status escalates (590) → **Quiescing(1)**.
4. A variant: X is not terminal, but a terminal T requires X. Grant closure
   does not apply, because X ∉ grant. T reaches Failed(dep) → Quiescing(1).
5. A second variant, with no R_A involved: X's crashes pay R_p (601). An R_p
   overflow sends X to Failed(intensity) (606), so the I5 clause "an overflow
   of R_p never changes `mode`" (780) is false once X is terminal.

The owner's own flapping X also drains p's budget, so p's writes are refused
with `ErrPrincipalBudget`. That is a minor side effect.

**Fix.**

- Make `ErrDelegationUid` an equivalence: `run_as(X)` is a delegated principal
  p ⇔ X ∈ grant(p).
- Combined with "terminal ∉ any grant" (1077), this also keeps terminal
  services off delegated uids.
- Add the case to the proof and as a §10 test.

### E2 (HIGH): a want or order edge into the grant carries an unbounded stall to a require-dependent's WaitTO

**Where.**

- Grant closure covers reverse **require** edges only (602, 1079). "Want and
  order edges into a grant are allowed: they never fail their source."
- That is true of the source D. But D's own require-dependents are bounded by
  WaitTO, and the budget W assumes **one** attempt of each upstream service
  (242).
- p's `restart` is not one attempt. The effect-set check (608-610) counts only
  services whose state the operation changes **now**, and a Waiting D does
  not change.

**Counter-example.**

- Setup: terminal T ∉ grant requires D ∉ grant; D wants A ∈ grant(p).
- At boot, A is in Starting, and its READY timing belongs to p because A runs
  as p.
- Shortly before start.timeout(A), p sends `restart A`:
  1. Starting × Rs → stopwait(restart);
  2. → settle rule 4 → Waiting, uncharged;
  3. → Starting again.
- A is never settled (§3.5:682), so D stays in Waiting.
- After about wait.timeout(T) / start(A) restarts (a handful, inside any
  budget_p, and 605 does not even say that `restart` pays), T's single
  deadline fires with D pending → **Failed(wait)**.
- T is terminal → Quiescing(1).
- Without a terminal service, the same route gives Failed(wait) plus rollback
  for any outside require-dependent of D. That contradicts the theorem's "cannot
  change the state of any service outside grant(p)".

**Fix (pick one).**

- (a) Closure under reverse edges of **every** kind, for every service that
  has a require-dependent outside the grant. This is the simplest to prove.
- (b) Latch `settled(A)` per sojourn of the wanter, once A's first attempt
  ends (Ready or Backoff) during that sojourn. This also makes W's
  one-attempt assumption true for owner-only flapping, but it changes the
  guard table.
- (c) Put every Waiting want- or order-dependent of a target in the effect
  set of an operation that unsettles that target.

### E6 (MEDIUM): H_OS is about process control, but the proof needs non-interference

**Where.** The proof says that p "cannot make [an owner service] crash through
the OS, by H_OS" (620). H_OS only limits which **processes** p controls (613).

**Counter-example.** p's granted service binds the port, or rewrites a
world-writable lock file, that an owner service needs. The owner service then
crash-loops and pays R_inst → Quiescing(1). The exclusion "shared host
resources … host memory, the pid space" does not obviously cover ports or
files.

**Fix.** State the hypothesis as "p's processes do not influence non-p
processes except through svd edges", or enumerate the excluded channels.
Theorem G is then honestly conditional on host isolation that svd does not
provide.

---

## Part 3: new defects in drafts 6 and 7, most severe first

### MEDIUM

#### E3: the payer's overflow is not in the helpers (559-564, 571)

- `restart(A)` and settle rule 4 test only |R_A| > max.
- Two outcomes live only in the prose:
  - R_p overflow → Failed(intensity) (606);
  - R_inst overflow → Quiescing (590).
- The table is data, and charging is a fixed formula (805). An implementer of
  §3.2 alone builds neither outcome. This is the D4 pattern again.

**Fix.**

- `charge(A)` returns the overflow of R_A and of the payer's counter.
- Rule 4 and `restart(A)` go to Failed(intensity) on either overflow; that
  covers R_A and R_p.
- An R_inst overflow sets mode := Quiescing(1).
- State the order in which the checks are evaluated.

#### E4: WaitTO's "armed once" needs state that §3.2 lacks (481, 705)

- Arming at "the propagation step where one first becomes pending" is neither
  a state entry nor a guard move. The `Cell` type (796-801) has no
  representation for it.
- "Never re-armed" requires a sojourn bit (call it `warmed`, reset on entry
  like `wexp`).
- Without that bit, Gate 1 cannot model "a toggling require-target does not
  restart the budget" (814).

**Fix.**

- Add `warmed` to §3.2.
- Add a Waiting self-guard `pending ∧ ¬warmed → arm WTO; warmed := ✓` to the
  guard table, ahead of G_ok.
- Include it in the termination argument; it moves at most once per sojourn.

#### E5: P2 contradicts the uncharged SEt (513, 788)

- Starting × SEt → Backoff, uncharged, with no bound on the count. Unclassified
  errors also count as SEt (508).
- A persistent pre-fork failure therefore cycles Waiting → Starting → Backoff
  forever.
- P2 ("◇ Ready, Done or Failed") is false on that run, and it is fair. Gate 1's
  SCC search for P2 (824) goes red, or the property gets weakened silently
  during implementation.

**Fix (pick one).**

- Restate P2 with "unless SEt recurs forever".
- Charge the k-th consecutive SEt of a service.
- Make unclassified errors permanent.

### LOW

- **E7 (557-558).** `sig` is classified by the time the exit is **recorded**,
  not by its cause.
  - A oneshot exits 0 naturally just before G_free sends `term`, and X0 is
    dequeued in Stopping → `sig` → re-run.
  - This errs on the safe side for D1, but D1-n (581) then holds only up to
    the step boundary.
  - Fix: document it, or classify with `WIFSIGNALED` together with the record
    state.
- **E8 (656 vs 653).** StopWait × X1 records `fail`, never `perm`.
  - A helper that fails permanently after an operator `restart` during setup
    is charged once by rule 4 before the respawn reaches Failed(spawn).
  - Fix: make the cell record `fail` or `perm` from the status pipe, as
    Starting × X1 does.
- **E9 (483).** `wexp` survives want-slowness.
  - Setup: A requires B and wants C. C's single attempt lawfully outlasts
    wait.timeout(A), so WTO fires with B Ready and sets `wexp`.
  - Later, one restart of B, inside B's budget, sends A to Failed(wait) at
    once and rolls back the transaction.
  - Had C been fast, A would have reached Ready and only taken stopwait(dep).
  - Fix: count the pending time, or check `ErrWaitTooShort` against W(A).
- **E10 (122-123).** Resolution order is ambiguous.
  - A singleton I skips the "Π_s outside I → `ErrConflict`" test.
  - With Π_s = {30 s, 200 s} and I = [5 s, 120 s], it is unclear whether
    30 s is used or the result is a conflict.
  - The §10 test (1126) expects a conflict.
  - Fix: test Π_s ⊄ I first, unconditionally.
- **E11 (509-511).** O4 has no classification table. The text never says
  which `execve` errno (`ENOENT`, `EACCES`, `ENOEXEC`) or which setup error is
  `perm`. O3 has such a table (505-506).
- **E12 (253).** The path alphabet excludes `\` and `:`. A Windows path
  parameter is therefore impossible, and P3 will need a second alphabet.
  cmd.exe also treats `,` and `=` as separators.
- **E13 (556).** In "cause := exit", `exit` reads as the variable but must be
  the constant. Rename one of the two.
- **E14 (602).** A granted A that requires an outside C is allowed.
  - If A is Leaked, it blocks G_free(C) until SWTO on every stop of C, with a
    T2 overrun report each time.
  - This is bounded, but it is an influence along a require edge that
    Theorem G's exception list does not name.
