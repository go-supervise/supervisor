# REVIEW-2 of SUPERVISOR.md (draft 4)

Adversarial review, 2026-11-23. This review did not edit SUPERVISOR.md.

What I read:

- §1–§3 in full (lines 44–622);
- §4–§8.3 (lines 627–880);
- §12 (lines 963–992).

What I did not read: §9–§11. Anything marked *(from memory)* was not opened.

**Verdict: not ready for P0 yet. One short pass (draft 5) is enough.**

- Every REVIEW-1 critical and high finding is closed in substance.
- Four new defects sit in P0 scope: N1–N4. All of them are local edits:
  - N1 changes the timer model;
  - N3 changes the StopWait bound;
  - N4 changes the wait predicate.
- §1.7 gives N2 no fix. The decision belongs to the user, and the options are
  written out under N2.

---

## Part 1: are the REVIEW-1 findings closed?

| finding | draft 4 | status |
|---|---|---|
| C1 fail-open K5 | Theorem N, mandatory total B, `all {}`, completeness Q (§1.5) | **Closed.** A site that omits its baseline is **refused** (`ErrNoBaseline`), not left open. See the note below, and N9. |
| C2 widening through K1 / new keys | authorities, field classes, ssh-style ownership (§1.7–1.8) | **Closed for privilege**: the site *is* the owner, and the ownership check keeps everyone else out. Residues: N6, N7, N8. |
| H1 T2 | StopWait + G_free (§3.5) | Closed in the table. **The bound is wrong: see N3.** |
| H2 tree vs main | MainExit / TreeEmpty, settle at TE, O1–O3 | Closed. |
| H3 `forced` | Stopping and Killing are separate states; Leaked blocks Retry | Closed. |
| H4 same-epoch timers | per-sojourn generation (§3.1) | Closed for the state timers. **Reopened for Expire and the watchdog: see N1.** |
| H5 squatting and forgery | path sockets, sender check, AF_UNIX / pipe rules (§4.1, §8.1) | Closed in design, but the Windows part is *(unverified)*. **The sender check regresses: see N5.** |
| M1 ⊥err, needs ⊆ allows | §1.2, §1.6 | Closed. The homomorphism proof for K2 is correct. |
| M2 R0 cost | D-M2 `prefer` | Stated. For consistency with the laws, see N10. |
| M3 monotone enforcement | §1.9, exempt_on per protection | Closed. |
| M4 want, boot set | §2.1, §2.3 | Closed. But `want` is not actually soft: see N4. |
| M5 wait budget | auto-recovery plus `ErrWaitTooShort` | **Not closed: see N4.** |
| M6 holders, Quiescing | §2.3, §3.3 | Closed. New issues: N2 and N12. |
| M7 typing, edge vs level | §3.2, §3.5 | Closed. Residue: N11. |
| M8 I5, Retry | §3.3, §3.7 | Closed. **The fix creates N2.** |
| M9 verification | Cell as data, exemptions, TLA+ incl. propagation | Mostly closed. Gate 2 is still not achievable as written: N13. |
| M10, M11, L1–L4 | §5.1, §7.2, §4.1, §8.2, §3.3 | Closed. Unverified items are listed in §12. |

**Theorem N** (§1.5:128-139).

- The proof is valid. U forces absence to mean ⊤. Under Theorem M, ⊤ is the
  least restrictive value. So "fail-closed absence" would need a one-element
  domain.
- Moving fail-closed behaviour into a completeness predicate over the met
  value is the right construction. The laws do not see that predicate, so
  `Compile` stays a function of the set of layers. That is what REVIEW-1
  meant by "required-concrete".
- **One gap.** The document does not say how B is identified. If B is
  "the file named baseline", file names carry meaning, which contradicts
  law C (§1.2 table). Define B as the meet of the `all {}` blocks of all
  site-authority layers, and check totality on that meet.

---

## Part 2: new defects, most severe first

### HIGH

#### N1 — §3.1: the generation filter swallows `Expire`, and it cannot tell watchdog re-arms apart

- §3.1:352 says that "timer and liveness events carry a generation", and that
  `gen` increments on *every* state entry.

**Expire.** An `Expire` is armed at charge time, inside settle → Backoff. It
fires `intensity.window` later, by which point the service has passed through
Backoff → Waiting → Starting → Ready, so the generation has changed. The filter
therefore drops it, and R_A never shrinks.

- **Counter-example:** max = 3, window = 60 s, one crash per day.
- On day 4, the service reaches Failed(intensity).
- Theorem R's windowed bound becomes a cap on the service's whole lifetime.
- `ExpireInst` suffers the same fate.

**Watchdog.** In Ready, every keepalive re-arms the deadline *within the same
sojourn*, so the same generation.

- **Counter-example:** watchdog = 10 s.
  1. A keepalive arrives at t = 9.9 s, and the deadline is re-armed to 19.9 s.
  2. The t = 10 s deadline was already queued. Its generation matches, so it
     yields `Fall`.
  3. A healthy daemon is stopped and charged.

  This is H4 again, one level down.

**Fix.**

- Expire and ExpireInst carry no generation. Better: have no event at all, and
  compute R_A as {t ∈ R_A : t > now − window} at charge time and at every
  check.
- Fall from the watchdog is a **level** check at fire time: `now − lastKeepalive
  ≥ watchdog`. The alternative is a per-arm token.
- Gate 1 must inject a stale deadline from the *same* generation.

#### N2 — §3.3:445, §8.2: Retry is charged to R_inst, so a delegated admin can take down the whole instance

- Failed × Retry charges R_inst (§3.4 table).
- A principal who is admin over only a K5 subset of services (§8.2 "Subtree
  delegation") runs `start S` on a Failed service in that subset,
  max_inst + 1 times.
- The result is instance overflow, then Quiescing(1). **Every** service is
  stopped, including those outside the principal's grant. A bounded-retry
  mechanism has become a privilege boundary violation.
- `restart` of a service in the subset also has an effect outside it: G_lost
  stops require-dependents that lie outside the grant.

**Fix.**

- Charge Retry to a per-principal budget, and refuse with `ErrRetryBudget`
  when it is exhausted, never by escalating.
- Refuse `restart` and `stop` when they would move a service outside the
  caller's grant, unless the caller is the owner.

#### N3 — §3.6:543: StopWaitTO is computed over require-dependents only, but G_free also waits for want- and order-dependents

- **Counter-example:** `A order B`. Shutdown clears all holders.
  1. A enters Stopping, with stop.timeout = 30 s.
  2. B enters StopWait.
  3. B has no require-dependent, so SW(B) = ε.
  4. SWTO fires after ε, and B is terminated while A is still draining.
- T2 is violated on **every** shutdown that has an order or want edge, and it
  is reported as an "overrun". §3.6 claims that this happens only when a
  dependent is Leaked, or when a dependent started stopping after B.

**Fix.** Take SW(B) as the maximum over **all** dependents. In G_free (§3.5:501),
those are the require-dependents plus the want- and order-dependents.

**Related.** G_free does not block on a dependent in **Leaked**, so SWTO never
fires for it. The "a dependent became Leaked" overrun of §3.6 is therefore
never reported, and T2 is broken physically without a report. Add Leaked to
G_free's blocking set (SWTO then bounds it), or report it explicitly.

### MEDIUM

#### N4 — §1.6:171, §2.1, §2.4:339: `want` is not soft, and `ErrWaitTooShort` is unsound (M5 is still open)

- G_ok waits until every want- and order-target is **settled**, and Backoff is
  not settled (§3.5:494).
- A want-target B that flaps runs through up to (max_B + 1) × (start + StopWait
  + stop + kill + backoff) before Failed(intensity).
- `ErrWaitTooShort` sums only `start.timeout` along require/want paths. It
  ignores:
  - backoff delays;
  - restarts;
  - a previous incarnation still stopping (SW + stop + kill);
  - order-targets.
- So A reaches Failed(wait). The transaction fails, and rollback stops A, plus
  B if B's only holder was the request.
- That is exactly the behaviour `want` was introduced to avoid: "A fails if B
  fails: **no**" (§2.1 table).

**Fix.** Either of these:

- (a) Waiting has no timeout while its only unsettled targets are want- or
  order-targets that are progressing (in Backoff, Starting or Stop*). Their
  own timers already bound them, so Theorem B still holds through them.
- (b) Compute the budget from the worst-case path through restarts and stops,
  and admit that it will be large.

Option (a) is cleaner.

#### N5 — §4.1:640: the sd_notify cgroup-membership check brings back the attribution race

- §4.1:640 requires the sender pid to be the main pid *and* "a member of the
  epoch's cgroup".
- When the sender has already exited, `/proc/<pid>/cgroup` is gone, and the
  message is dropped.
- That is systemd's race (A§1.4), and draft 3 had avoided it. The §10 test "a
  ready message sent just before exit must still be attributed" will fail.

**Fix.** Check the pid only. The main process stays a zombie until zsup reaps
it, and O1 drains the socket before reaping, so the pid cannot be reused within
that window. The cgroup check adds nothing.

#### N6 — §1.5:146, §1.7:203: what may go in `all {}` is not specified

- `all {}` is class G, which "the site writes". Nothing restricts what an
  `all {}` block may *contain*. M-D's proof (§1.8:216) covers site
  restatements in `Lᵢ(s)`, but not `all {}`.
- **`all { env "LD_PRELOAD" = … }`.** If D-checks are not applied to it,
  this injects into every service. If they are applied, it is a widening for
  every service, which is acceptable. Say which.
- **`all { require "x" }`.** This also gives `x` an edge to itself, so it is
  always `ErrCycle`.
- **`all { stop.timeout = { prefer = … } }`.** This conflicts with every
  recipe's `prefer` (N10).

**Fix.** `all {}` admits classes H and T (without `prefer`) and policy only.
Anything else is `ErrFieldAuthority`.

#### N7 — §1.7:199: a parameter with no declared domain is ⊤, which means any string

- By law U, `param "x" {}` without a range has the domain ⊤.
- A site then "concretizes inside the domain" with any string. That gives argv
  injection into a service whose recipe runs it as root, for example
  `--config=/home/u/evil` or a `sh -c` wrapper.

**Fix.** Put the parameter domain in the completeness set Q, and allow only
typed domains (integer interval, enumeration, port, path region). Refuse
free-form string domains.

#### N8 — §1.7:182, §8.1:819: the ssh-style check reads mode bits and ignores ACLs

- Linux is safe: the group bits of the mode reflect a POSIX ACL's mask, so a
  named-user write entry shows up as group-writable *(from memory)*.
- macOS ACLs (`chmod +a "user:x allow write"`) and FreeBSD NFSv4/ZFS ACLs do
  not show in the mode. A directory with mode 0755 plus a write ACE passes the
  check.
- Windows has no mode bits at all.

**Fix.**

- Evaluate the ACL on every OS.
- Do the walk on fds (`openat` + `fstat` per component), so that the object
  checked is the object read.

On the per-user case, the check is otherwise sound: anything running as the
same uid can write the site directory, just as it can write
`~/.config/systemd/user`. Say so.

#### N9 — §1.5:145: totality makes every schema extension break existing instances

- Add one protection field in v0.2, and every existing B becomes
  `ErrBaselineNotTotal`.
- In mode C, the container's PID 1 then refuses to boot.
- Defaulting the new field to ⊤ instead would re-open C1.

**Fix.**

- B declares `schema = N`.
- A newer zsup refuses (fail-closed) with a migration message naming the new
  fields.
- `zsup validate --schema N+1` lets a site prepare before it upgrades.

#### N10 — §1.4:117-124, D-M2: `prefer` keeps the laws, but two spellings of the same value give different results

- **The laws hold.** Resolution is a function of the met value, the met
  `prefer` included, so `Compile` remains a function of the set, and the
  resolved point lies in the met interval (law L).
- **The inconsistency.** Against a recipe with `{5s, 120s, prefer 30s}`:
  - a site that writes `"60s"` gets 60 s;
  - a site that writes `{60s, 60s, prefer 60s}` gets `ErrConflict`, because
    the two preferences meet as K1.

  The same intent gives two different outcomes. For the same reason, B and
  `all {}` can never carry a `prefer`.
- **Fix.**
  - Collect the `prefer`s as a set, and use them only when the met interval is
    not a singleton. The resolved value is the unique `prefer` inside the met
    interval; otherwise the result is `ErrIncomplete` or `ErrConflict`. This is
    still a function of the set of layers.
  - State the phase order: meet → resolve → post-meet predicates. Both
    `ErrWaitTooShort` and SW need resolved values.

#### N11 — §3.2:421-427, §7.2:811: settle and SpawnErr

- **Rule 2 comes before rule 3.** A oneshot that exits 0 while it is in
  StopWait(dep) is **re-run**. A completed migration runs a second time.
  A daemon crash during StopWait(dep) is never charged.
  - Fix: `oneshot ∧ exit = ok ∧ goal = up → Done` should come before rule 2.
    Rule 2 should also charge when `exit = fail`.
- **Failed(spawn) is never assigned.**
  - §7.2:811 sends a deterministic `needs ⊄ allows` to Failed(spawn) "through
    SpawnErr". But Starting × SE = `restart(A)`, which charges and backs off.
    The deterministic error is retried max + 1 times.
  - Fix: split SpawnErr into a transient error and a permanent one
    (`ErrProtectionUnavailable`, region conflict), and send the permanent one
    straight to Failed(spawn).

#### N12 — §2.3:328: `stop X` does nothing while X is still wanted

- goal(X) is derived. If Y is up and requires or wants X, then `x(X) := ∅`
  leaves goal(X) = up.
- `zsup stop postgres` then returns, and postgres keeps running.

**Fix.** Either refuse with `ErrStillWanted{by}`, or define `stop` to clear
holders across the reverse closure. The latter is what systemd does.

### LOW

- **N13 §3.8:609-612, Gate 2.** Some mutations violate no property:
  - removing a `Report` action (StopWait × SWTO, Killing × KTO);
  - swapping Failed(wait) and Failed(dep), which recover identically.

  Gate 2 "every mutation red" cannot then be met. Either make reports an
  observable property, or exempt cells that differ only in reason or report,
  and assert that list.
- **N14 §3.5:523, propagation termination.** The chain "Inactive < Waiting <
  Starting < StopWait < Stopping" does not hold:
  - Waiting → Inactive, Done → Inactive and Backoff → Inactive go backwards;
  - Failed ↔ Waiting is outside the chain.

  A potential function is needed. Either way, the BFS assertion is the real
  gate.
- **N15 §3.3:444.** SIGTERM during Quiescing(1) turns it into Quiescing(0). The
  exit is then 0, and the host init never sees the escalation. Fix: keep the
  maximum status.
- **N16 §12:972, 977.** §12 records the home as `go-supervise/supervisor`, and
  leaves the binary name open. The brief I received says that the user
  accepted `go-zpkg/supervisor` and `zsup`. Check against the user's own words
  before P0 creates a repository.
