# REVIEW-6 of SUPERVISOR.md (draft 9)

Review pass, 2026-11-23. This review did not edit SUPERVISOR.md.

Scope: REVIEW-5 in full. SUPERVISOR.md draft 9: §1.6 (221-286), §2 (435-498),
§3 (501-1006), §8 (1201-1279), and the F rows of §12 (1509-1519). Line numbers
are draft 9's. No external source was opened. OS facts marked *(from memory)*
were not checked against a man page or kernel source.

**Verdict.**

- **The FSM may be implemented (P0 part 2):** the event table (165 cells), the
  guard grid (88 cells with `Absent` and `Prio`) and settle. No cell is wrong
  in behaviour, and REVIEW-5 F6 and F7 are closed.
- **One guard formula should wait for a decision.** G_free's definition and
  Gate 1's oracle depend on how N1 below is fixed. The recommended fix changes
  G_free and SW. It changes no cell.
- **Theorem G is still not established.** N1 (HIGH) shows that the « bounded »
  StopWait lever that draft 9 concedes is enough to escalate the instance.
  N1 blocks the delegation claim (P1). It does not block P0.

---

## Part 1: REVIEW-5 F1–F11, and the replays

| F | draft 9 | status |
|---|---|---|
| F1 | H_OS-E5 (697-710), fair dequeue (507), O5 (549), F1 chaos test (1509) | **Closed for the two routes REVIEW-5 gave.** Replay of the EMFILE route: p's connections stop at `budget.connections`, and `accept` is refused below the spawn reserve (699), so C's pre-fork pipe never sees `EMFILE` because of p. Replay of the flood against C's own StartTO: O5 drains C's channel before C's timer, so Rise wins. **Two residues remain.** A cross-service ordering is not covered (N2). CPU is not partitioned (N3). |
| F2 | `ErrDelegationPrivilege` as an allow-list (281-286); resolved ids, pre-fork re-resolve, post-`setuid` id check (278) | **Closed.** Replay: a granted recipe with `CAP_KILL`, or `CAP_DAC_OVERRIDE`, is refused at Compile. `toor`/`root` count as one principal (268). A renumbering after load is SEp. A set-user-ID-p binary run by O is covered by A2-files (715). |
| F3 | cut(A′, A) for every holder (471-479), D(X), T, I4 (921) | **Closed.** Replay of route 1: the owner's `start A` gives T = {A}, C is not raised, and the rollback cannot lower C (490). Replay of route 2: p's `stop A` clears `boot` on A, but the cut makes goal(C) independent of that holder. The effect-set check is now a real refusal (687). The cost is examined in Part 3. |
| F4 | three classes, `ErrPrincipalClass`, the reader direction (263-276, 1252) | **Closed.** |
| F5 | P2a and P2b (935-940) | **Closed in substance.** One LOW naming collision remains (N5). |
| F6 | `Absent`, `Prio`, `Cond` (846-852, 957) | **Closed.** I recounted the present cells: 1+5+2+2+1+1+1+2 = 15. |
| F7 | headers StartTO and StopTO (772, 776) | **Closed.** |
| F8 | 642; Stopping, Killing and Leaked × X1 read `perm` (623, 784-788) | **Closed.** |
| F9 | one append (626, 792) | **Closed.** |
| F10 | H_OS-E2, no hand-over (694, 1256) | **Closed.** |
| F11 | credential-only control buffer (702) | **Closed, but it rests on a fact the draft does not state** (N6). |

**Earlier replays on draft 9.**

- **REVIEW-3 D1 (oneshot stopped by StartTO).** The oneshot records `sig`, so
  rule 5 applies and it is never Done. Closed.
- **REVIEW-3 D2, API route.** The crashes it re-enables pay R_p. Closed.
- **REVIEW-3 D2, OS route.** It is `ErrGrantNotClosed`. Closed.
- **REVIEW-3 D3.** G_arm arms once per sojourn. Closed.
- **REVIEW-3 D4-D7.** These cells are unchanged, so the replays are unchanged.
- **REVIEW-4 E1.** It is `ErrDelegationUid{outside-grant}`, now on ids. Closed.
- **REVIEW-4 E2.** `ErrGrantNotClosed{want}`. Closed.
- **REVIEW-4 E14.** It is subsumed by N1, and it disappears under N1's fix.

---

## Part 2: Theorem G under H_OS-E1..E5, A1 and A2

### N1 (HIGH): the StopWait oracle is chosen by p, and it is enough to escalate

**What the draft says.** Draft 9 treats the time at which G_free(C) becomes
true, for C ∈ O with a dependent in G, as a **low** input (724). Item 2 (728)
then concludes that p cannot cause Quiescing(1), and 730 concedes that p
decides whether C's own exit during that StopWait is charged.

**Why the conclusion does not follow.** That value is a function of G's
process events, which are high. The comparison run of item 2 (« p issues no
operation and G's trees emit no event ») has a different oracle value: G_free
never holds there, so StopWait always ends at SWTO. That run therefore cannot
match p's run « with the same oracle values ». Item 1 is true only relative to
an oracle that p controls. The concession at 730 is in fact an escalation
route.

**Counter-example (R_inst).**

- Setup: B, C ∈ O, both enabled; C requires B; A ∈ grant(p) requires C (an
  edge out of a grant, allowed by 1250); max_inst = 2.
- B crashes. This is a low event, and it pays R_inst in both runs.
- In the same step, B goes to StopWait, C gets G_lost and goes to
  StopWait(dep), and A gets G_lost, goes to StopWait(dep), then `term` A, then
  Stopping. G_free(C) is false while A is in Stopping (824).
- C's daemon has lost its socket to B, and it exits 1 about 50 ms later. This
  exit is low and happens in both runs.
- **Benign run (p's A exits on SIGTERM within 1 ms).** A reaches TreeEmpty, then
  Waiting. G_free(C) holds, so `term` C is sent and C goes to Stopping. C's
  exit is then recorded as `sig`, which is uncharged (623). Each crash of B
  costs one payment.
- **p's run (A ignores SIGTERM until stop(A) + kill(A)).** C's exit is
  dequeued in StopWait, so it records `fail`. StopWait × TE calls settle, and
  rule 4 (cause = dep, exit = fail) charges C and pays R_inst (645). Each crash
  of B costs two payments.
- With two crashes of B in the window:
  - benign: |R_inst| = 2, so there is no escalation;
  - p's run: |R_inst| = 4 > 2, so the instance goes to **Quiescing(1)**.
- Each additional O service with a dependent in G that exits when B dies adds
  one more payment per crash. With a terminal C and a small intensity.max(C),
  a single conversion gives Failed(intensity), and then Quiescing(1).

**Gate 1 cannot see N1.** Gate 1 models g_C as low and applies it to both
copies (982), so the 2-safety check is green on this route by construction.
In abstract time, the one-copy « within range » check is vacuous, because any
step before SWTO is in range.

**Fix (recommended): cut the stop ordering at the grant boundary as well.**

- G_free(C) ranges over the dependents of C that are not across a cut edge:
  only dependents A with ¬cut(A, C).
- SW(C) (894) is computed over the same set.

What the fix gives:

- No transition of a service of O reads G any more, so item 1 holds with no
  oracle at all, and item 2 holds as stated.
- E14 and the oracle paragraph of §3.8 disappear, and the model equals the
  runtime.
- I1 still holds: A enters StopWait in the same step through G_lost, and I1
  constrains Starting and Ready only.

The cost:

- A granted dependent may still be in its graceful stop when C is terminated.
- The ordering guarantee T2 then holds only towards dependents in O.
- This cost falls on p alone.

**Alternative.** Keep the oracle, and rewrite item 2 as « p can escalate
exactly when some oracle choice in range escalates ». Theorem G then no longer
states « delegation cannot escalate ». I do not recommend it.

**Witness to add.** The 2-safety check, with no oracle, must be **red** on the
mutant in which G_free reads dependents across a cut.

### N2 (MEDIUM): O5 orders a service's own timer only. p's load can still reorder a require-target's READY behind a dependent's WaitTO.

**The rule.** O5 (549) drains C's channel before a state timer of **C**.
Waiting × WTO of **T** reads the states of req(T) (530). Round-robin (507)
bounds delay at one round. It does not order the sources of a round by the
time their events arrived.

**Counter-example.**

- C ∈ req(T), with T terminal and in O.
- C writes `READY=1` shortly before T's wait deadline.
- p keeps every source of G and its API listener full. N3 makes svd itself
  slow as well.
- Within the round, T's timer source is served before C's channel, so T reaches
  Failed(wait), and then Quiescing(1).
- Without p's load, the same READY would have been processed within
  microseconds, before the deadline.

This is a race, but p widens its window at will. Proof step (b) treats the
interleaving of O's low events as fixed. Under load, the interleaving is not
fixed.

**Fix.** O5′: before delivering WTO(T), drain into the table every event of
the sources of req(T) that was readable at dequeue time: channel, MainExit and
TreeEmpty. These are exactly the states the cell reads. Generalise O5 to
« before a timer cell, drain the sources of every service that the cell
reads ».

### N3 (MEDIUM): CPU, IO and swap are not partitioned, and A2(host) cannot be audited for them

E5 gives deleg/<uid>/ only a `cpu.weight` (704). A weight is a share under
contention, not a cap, so « Σ budgets leave the CPU that O needs » (718) has no
enforced term that a site could sum.

**Route.** Granted trees busy-loop on every core. An O service C overruns its
`start.timeout`, which is charged and pays R_inst. With enough such overruns,
the instance escalates.

**Inside svd as well.** §9 says that a log pipe « never blocks a child », so
svd reads p's log flood at whatever rate p writes. svd's own CPU is therefore
unbounded per principal, and that feeds N2.

**Two statements claim too much.** O3 (557) and proof step (b) (735) say that
E5 alone prevents an SEt of C. But `ENFILE`, the global pid space and host
memory are in A2(host). Cite E5 ∧ A2(host).

**Fix.** Add `budget.cpu` and `budget.io` to E5. On Linux, `cpu.max`,
`io.max` and `memory.swap.max` on deleg/<uid>/. For a granted service's log
pipe, back-pressure the writer instead of reading at any rate: blocking only
harms p. Then A2(host) for CPU becomes the check Σ cpu.max ≤ the capacity left
for O.

### N4 (MEDIUM): holders grow without bound, so E5's bound on svd's memory is false

- Each `start X` adds a fresh request holder (490), and a successful one is
  never removed until `stop`.
- R_p bounds the **rate** at which p can add holders, not their total. Over
  days, p grows x(X) and the transaction and audit records without limit.
- E5's « svd's memory is at most a fixed term plus per-principal terms
  computed from the plan » (700) is therefore false.

**Fix.**

- Allow at most one request holder per (principal, service): a repeated `start`
  reuses that holder.
- Or bound the number of holders per principal in E5.
- Bound the in-memory audit log, or spill it to disk under A2(host).

### Is E5 implementable on Linux?

**Yes, on cgroup v2**, provided svd owns a delegated subtree with the `pids`
and `memory` controllers enabled. deleg/<uid>/ holds only child cgroups, so the
no-internal-process rule is met. Notes, none of which blocks implementation:

- **Per-principal listener.** No ACL is needed on Linux. `fchown` the socket
  inode to p, with mode 0600: `connect` needs write permission on the socket
  inode *(from memory)*. Keep the `SO_PEERCRED` check. This removes the
  « ACL mechanism unverified » open item for Linux.
- **The fd reserve (699)** must count the steady-state fds of **every running
  service**: the log read end, the per-epoch notify socket, the pidfd, and the
  cgroup.events watch. To that it must add the transient fds of a spawn: both
  ends of the status pipe and of the readiness pipe. « The fds of one spawn per
  service » undercounts if it is read literally.
- **Event intake.** A full per-source queue must mean « stop polling that fd »
  (kernel back-pressure). A lifecycle event (MainExit, TreeEmpty) must never be
  dropped. State this.
- **The non-racct fallback on FreeBSD and macOS.** A per-process `RLIMIT_AS`
  bounds address space, not memory, and it will break Go programs and JVMs,
  which reserve large address ranges. It is safe, but most granted runtimes
  become unusable under it. Say so.
- **Readers.** E5 gives per-principal budgets to readers (697), while §8.1
  gives readers one shared listener (1218). So listener-level back-pressure is
  per-class for readers. This does not reach O, but the text is inconsistent.

### Are the « unverified » marks honest?

**Mostly, yes.** The ACLs, Windows, FreeBSD, macOS `WNOWAIT` and `RLIMIT_AS`
are all marked.

Two load-bearing facts are not stated:

- **N6 (LOW).** F11's safety depends on the kernel placing `SCM_CREDENTIALS`
  before `SCM_RIGHTS` when `SO_PASSCRED` is set. On 64-bit,
  `CMSG_SPACE(4)` = 24 ≤ `CMSG_SPACE(sizeof ucred)` = 32, so with any other
  order an `SCM_RIGHTS` with up to four fds would fit *(from memory)*. Add a
  defensive step: walk the received cmsgs and close every `SCM_RIGHTS` fd, as
  systemd does.
- **N3's overclaim (O3 557, proof (b) 735)** is unmarked.

---

## Part 3: does F3's cut break a legitimate owner use?

**« An owner service that requires a delegated one » was already impossible
before F3.** Closure (E2, draft 8) forces such a service into the grant, and
E1 then makes it run as p (1249). F3 does not change that.

**What F3 newly costs.**

- (a) **The owner's `start A`**, with A ∈ G requiring C ∈ O, no longer raises
  C. A ends in Failed(wait), and the transaction rolls back.
- (b) **An « on-demand » C**, one that should run only while p's A runs, must
  now be held on its own.
- (c) **The owner's `stop C`** no longer reports `ErrStillWanted` for a
  running A. It tears A down through G_lost.

**These costs are acceptable, and necessary rather than a defect.** If any
holder on G could raise goal(C), p would control when C starts and stops, by
holding or stopping A. Each start of a C that crashes on startup pays R_inst,
so that is an escalation route again.

**Ask (LOW).** Add two things:

- an operator recipe: `enable C`, or `svd start C` before `svd start A`;
- a stated invariant: a request's targets lie in one partition, so a future
  multi-target `start A C` must open one transaction per partition. Otherwise
  F3 route 1 returns.

---

## Part 4: the owner Retry fix, and the 2-move bound

### The owner Retry now pays payer(A)

**Correct.** The finding behind it holds:

- p reaches Failed(exited) on its own A, through a single high X1 when
  restart = never.
- An owner Retry is low and applied to both copies. Under `pay(caller)` it
  pays R_inst in one copy, while Ready × Ry is `·` in the other.
- |R_inst| differs between the copies, so the `pay(caller)` witness (985) does
  go red.

Under `pay(payer(A))`, every R_inst payment reads only O:

- crashes of O;
- owner Retries of O.

Owner `start`, `stop` and `restart` pay nothing (677).

### The 2-move bound

**I re-derived it, and it holds.** The paths at 865-869 are exhaustive. The
Waiting priorities make G_arm exclusive of G_failed, G_wexp and G_ok, and the
recovery from Failed needs G_ok, which excludes pending.

**The justification at 872 names the wrong reason (LOW).** « One-way, at most
once per step » is true but not sufficient. G_free's upward read is harmless
because StopWait and Stopping are indistinguishable to every guard that reads
B:

- neither state is in {Ready, Done, Failed, Leaked};
- neither state is settled while the goal is up;
- both states block G_free of B's own targets (824-825).

So the move changes no other service's input, and the induction from the sinks
upward survives the upward read.

**N5 (LOW).** P2b (939) redefines U(A) as start + stop + kill + backoff.max.
§1.6 (230) already uses U(B) for the one-attempt time that includes W and SW.
Rename one of them, for example V(A).

---

## Part 5: is Gate 1's 2-safety specification precise enough to implement?

**Almost.** It is red on draft 8, and I traced it:

- `goalDraft8`, through F3 route 1:
  - the owner's `start A` applies to both copies;
  - A's Rise is applied to copy 1 only;
  - the low request deadline is then applied to both copies;
  - copy 2 rolls back, so goal(C) differs, and the check is red.
- The `pay(caller)` witness is red, as traced in Part 4.

Close five gaps before coding it:

1. **A low input that is enabled in one copy and not in the other must be red,
   never skipped.** obs_O leaves out C's tree bit and the generations, but they
   decide whether TE and the timers of O are enabled. Either add them to the
   compared state, or assert that their enabledness is equal.
2. **Symmetry reduction must preserve the partition.** Permute within G and
   within O only, and jointly on both copies.
3. **The deadline of a transaction of p is high.** Only owner deadlines are
   low. 981 says « the request deadline of §3.1 », but the request deadline is
   defined in §2.4.
4. **The oracle.** After N1's fix, remove g_C entirely and add the
   G_free-across-a-cut witness. If the oracle is kept, the « range » check is
   vacuous in abstract time and must say so.
5. **Gate 1 does not cover the stated limits N2 and N3.** Extend the F1 chaos
   test:
   - p floods its API listener, its channels and its logs, and busy-loops on
     every core;
   - an O require-target becomes READY near T's deadline;
   - assert that T reaches Starting and that mode stays Running.

---

## Summary

| item | result |
|---|---|
| 1 | F2-F11 are closed. F1 is closed for its two routes, with residues N2 and N3. Every REVIEW-3, -4 and -5 replay is closed. |
| 2 | Theorem G is **not** established. N1 (HIGH): p chooses the StopWait oracle and so converts O crashes into R_inst charges, which reaches Quiescing(1). N2 (MEDIUM): the cross-service WTO/READY order. N3 (MEDIUM): CPU, IO and swap are uncapped. N4 (MEDIUM): holders are unbounded. E5 is implementable on Linux under cgroup v2. The marks are mostly honest, apart from N6 and N3's overclaim. |
| 3 | The cut breaks no use that was legal in draft 8. Its costs ((a)-(c)) are acceptable and necessary. Document the recipe and the one-partition-per-request invariant. |
| 4 | The owner Retry fix is correct, and its witness goes red. The 2-move bound holds. The justification at 872 should name invisibility, and U is defined twice (N5). |
| 5 | The specification is implementable after gaps 1-5. It is red on draft 8, as required. It is green on draft 9 partly by construction, through the low oracle (N1). |
