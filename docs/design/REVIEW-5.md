# REVIEW-5 of SUPERVISOR.md (draft 8)

Confirmation pass, 2026-11-23. This review did not edit SUPERVISOR.md.

Scope: REVIEW-3 and REVIEW-4 in full; SUPERVISOR.md draft 8 in full (1442
lines). Line numbers are SUPERVISOR.md draft 8 unless prefixed `R3:` or `R4:`.
No external source was opened. OS facts marked *(from memory)* were not
checked against a man page or kernel source.

**Verdict: the FSM cell tables (event 165, guard 88, settle) may be
implemented once two mechanical edits land: F6 (the guard grid needs an
"absent" value and a priority) and F7 (two column headers contradict the
prose).** No cell is wrong in behaviour. **Gate 1 is not final**: its goal
formula depends on how F3 is fixed, and its P2 check goes red as specified
(F5). **Theorem G is not established under H_OS.** Two HIGH routes reach
Quiescing(1) or bypass A1 (F1, F2). Neither changes a cell. Both change
Compile, the API limits, or the hypothesis. They block the delegation claim
(P1 security audit), not P0.

---

## Part 1: REVIEW-4 E1–E14, and the replays

| E | draft 8 | status |
|---|---|---|
| E1 | ⟺ at 265, `ErrDelegationUid{…, direction}` (267-271, 393) | **Closed as a rule.** R4 replay: owner X ∉ grant(p) with run_as(X) = p is refused at Compile. Residue: F2(b), because "=" is not defined as uid equality. |
| E2 | closure under every edge kind (641, 1154) | **Closed.** R4 replay: D wants A ∈ grant(p) gives `ErrGrantNotClosed{p, A, D, want}`, so the restart-stall route cannot be configured. Holder scope, added for the proof, opens F3. |
| E3 | `charge` returns oA and oP; `overflow(A)` items 1-3 (592-600) | **Closed.** Replay: p-payer crash with \|R_A\| ≤ max and R_p past budget. Rule 4 gives Overflow (oP ∧ payer ≠ owner) and then Failed(intensity), and mode is untouched. Owner payer with oP and ¬oA gives Escalate, then Waiting. |
| E4 | `armed`, G_arm, I4w (498, 558, 760, 773, 846) | **Closed in semantics.** R4 replay: a target is pending at entry, then Ready, then pending again. G_arm fires once, and the second pending sees armed = ✓, so there is no re-arm. Residue: F6, an encoding defect of the grid, not of G_arm. |
| E5 | P2 conditional on finite SEt (542, 858-860) | **Not closed.** P2 is false on fair runs that contain no SEt (F5). The stated reason for rejecting a charged k-th SEt does not hold either (F5). |
| E6 | H_OS split into E1-E4 and A1-A2 (655-665) | **Partly closed.** The split is right. A1 can be falsified by svd's own plan (F2), and A2 leaves out svd itself (F1). |
| E7 | documented (589) | **Cost acceptable for safety**, because the error is a re-run and never a false Done. Line 607 still says, without qualification, that a completed oneshot "is never re-run by a dependency loss". It must carry E7's qualification (F8). |
| E8 | StopWait × X1 reads the status pipe (587, 722) | **Closed.** The same loss of `perm` remains in Stopping (F8, LOW). |
| E9 | W(A) over every target kind (239-243) | **Closed.** |
| E10 | Π_s ⊄ I tested first (122) | **Closed.** Replays: {30 s, 200 s} against [5 s, 120 s] gives a conflict, and {60 s, 30 s} against [60 s, 60 s] gives a conflict. |
| E11 | O4 table (528-538) | **Closed.** |
| E12 | per-OS alphabets (256-260) | **Closed.** Note: the Windows alphabet has no space, so `C:\Program Files\…` cannot be passed. State this or accept it. |
| E13 | `natural` (552) | **Closed.** |
| E14 | named in Theorem G item 2 (669, 1157) | **Cost acceptable.** It cannot outlast SWTO, and an overrun is reported. Replay: a Leaked granted A requires C. G_free(C) is false until SWTO, then C reaches Stopping and an overrun is reported. Neither T2 nor Theorem B is violated. |

REVIEW-3 replays on draft 8:

- **D1 (StartTO oneshot).** stopwait(fail), then term, then X in Stopping records `sig`. Rules 2, 3 and 4 miss (cause = fail), and rule 5 restarts it, charged. Closed.
- **D2 API route.** The re-enabled crashes pay R_p because A runs as p (640). Closed.
- **D2 OS route.** T ∉ grant requiring A ∈ grant is `ErrGrantNotClosed`. Closed.
- **D3.** G_arm arms at t = 0. At t = 30, WTO sets `wexp` and pending is false, so the state is unchanged, and A spawns when C settles. Closed.
- **D4-D7.** Closed. Their replays do not change from REVIEW-4.

---

## Part 2: Theorem G under H_OS = enforced E1-E4 + assumed A1, A2

### F1 (HIGH): svd's own resources carry influence through the API, and A2 does not name them

A2 (665) allows influence "through svd: the edges of the graph and the API".
The proof analyses API **operations** only: the grant check, R_p and the
effect set. It never considers the **load** that p places on svd itself. No
per-principal bound is stated anywhere on API connections, on refused writes
(which cost nothing), on reads, or on channel messages from p's services.

**Counter-example (Quiescing(1)).**

- Setup: terminal T ∉ grant requires C ∉ grant. p is a delegated principal and
  can connect to the API socket.
1. p opens connections to the API socket until svd reaches its own
   RLIMIT_NOFILE. That limit belongs to svd's process. It is not the "system
   file table" that A2 excludes.
2. C crashes once (owner-paid, normal), or an owner `restart C` arrives.
3. Before forking, svd needs a pipe, a status pipe and a notify socket. It
   gets `EMFILE`, which is **SEt** (O3, 522), so C goes to Backoff
   uncharged. This repeats for as long as p holds the fds. P2's hypothesis
   "SEt occurs finitely often" is now under **p's** control.
4. T: G_lost leads to stopwait(dep), then Waiting with pending. G_arm, then
   WTO, gives **Failed(wait)**. T is terminal, so **Quiescing(1)**.

Proof step (c) fails here: R_inst is untouched, but "a terminal T ∈ O reaching
Failed(wait) reads only req(T) ⊆ O" is true while the cause of C's stall is
p's.

**A second instance, through event intake.** A single queue (476) has no
stated fairness across sources, and nothing orders a channel message that was
readable before a deadline ahead of that deadline. A granted main process
that floods `STATUS=`/`WATCHDOG=1` datagrams, log lines or readiness bytes can
delay C's `READY=1` past C's StartTO. The result is Starting × StartTO, a
charge to R_inst, and with enough repeats, escalation.

**Fix.** Add an enforced hypothesis, **H_OS-E5 (svd resource isolation)**:

- a per-principal cap on API connections and on in-flight requests;
- a rate limit on refused writes;
- a reserved fd headroom that only spawns may use (refuse `accept` below it);
- bounded, per-service channel intake with round-robin dequeue across
  services;
- **O5**: before a state timer of service C is delivered, drain C's readiness
  channel (as O1 does before MainExit), so that a READY written before the
  deadline wins.

Add a chaos test: a granted service floods, and p holds N connections, while
an outside service restarts. Assert that the outside service reaches Ready
and that mode stays Running.

### F2 (HIGH): A1 can be falsified by the plan svd enforces, and E1's "=" is not uid equality

**(a) Capabilities.** A1 (664) says the OS confines p to processes running as
p. But svd itself hands K5 `capabilities` (356) to the services it spawns,
including granted ones, which run as p. Nothing restricts which capabilities a
granted service may hold.

- Counter-example: a granted service whose met plan allows `CAP_KILL` (a
  process-reaper recipe) can signal any uid. With `CAP_DAC_OVERRIDE` (common
  for backup jobs) it can write an outside service's configuration, and with
  `CAP_SYS_PTRACE` it can trace an outside service.
- p controls such a process through its inputs: its working directory and
  data files are owned by p, because it runs as p. p crashes an outside
  terminal service, which reaches Failed and escalates to Quiescing(1).
- A1 is then false on that host, by svd's own doing. The theorem holds only
  vacuously, and nothing warns the site.
- (On Linux, p cannot ptrace a process that holds more capabilities than p
  holds *(from memory)*. Control through its inputs is enough.)

**Fix.** Make it enforced rather than assumed:

- for every A ∈ grant(p), `capabilities ∩ C_cross = ∅`, otherwise
  `ErrDelegationPrivilege`;
- C_cross covers at least `CAP_KILL`, `CAP_SYS_PTRACE`, `CAP_SETUID`,
  `CAP_SETGID`, `CAP_DAC_OVERRIDE`, `CAP_FOWNER`, `CAP_CHOWN`,
  `CAP_SYS_ADMIN`, `CAP_SYS_MODULE`, `CAP_SYS_RAWIO`, `CAP_BPF`,
  `CAP_NET_ADMIN` and `CAP_SYS_TIME`. Better still, use an allow-list, as
  §1.7 does for rights;
- on Windows, the token privileges (`SeDebugPrivilege`,
  `SeTcbPrivilege`, …) *(unverified)*.

A1 then shrinks to the OS's uid rule.

**(b) What "runs as p" means.** E1's ⟺ (265) and A1 compare `run_as` to a
principal, but neither says that the comparison is on the **resolved uid**
(on Windows, the SID).

- Two names with one uid (FreeBSD `toor`/`root`, or a duplicated uid in
  passwd) pass a name comparison.
- Accounts are resolved at spawn (523), not at Compile, so a renumbering after
  Compile breaks the ⟺ silently.
- The Linux signal rule also matches the target's **saved** set-user-ID
  *(from memory)*. An outside service that executes a set-user-ID-p binary
  becomes signalable by p unless NNP is ✓ for it.

**Fix.** Define ⟺ on resolved uid and SID, re-check it pre-fork (a failure
is SEp), and state A1 operationally (below).

**A1 and A2 stated so that they can be tested.**

- **A1, per host, per outside process π.** For every process π of a tree of O,
  the real, effective and saved uids of π are ≠ p. A process of uid p
  holding the capability set svd gives grant(p) gets `EPERM` from
  `kill(π, 0)`, `ptrace(ATTACH)`, `process_vm_writev` and an `open` of
  `/proc/π/mem`.
  This is a per-host probe that Gate "per OS" can run against a sentinel
  outside service.
- **A2.** "Influence" cannot be observed, so A2 cannot be tested as written.
  State it as disjointness predicates over named sets, each one
  auditable:
  - *(files)* paths writable by uid p or by grant(p)'s plan ∩ paths read **or
    executed** by O's trees = ∅. Executed covers binaries, interpreters,
    libraries and configuration. svd knows only the first operand's plan
    part, and it cannot know the second, because v1 has no read/visibility
    field (358). Say so: A2-files is a site audit, not an svd check;
  - *(ports)* the ports p can bind ∩ the ports O listens on or connects to
    = ∅;
  - *(IPC)* the same for SysV/POSIX IPC keys, abstract sockets and D-Bus
    names;
  - *(svd)* none: svd itself moves from A2 to H_OS-E5 (F1).

### Other routes checked, which found nothing

- **Retry and restart loops by p.** They pay R_p, and crashes pay R_p.
- **G_free.** Bounded by SW(C).
- **Leaked granted dependents.** E14 applies.
- **p's `stop --with-dependents`.** Its closure lies inside G.
- **Cgroup moves between p's own epochs.** I2 can break inside G only.
- **Disjoint grants.** Grants of different principals are disjoint.
- **Owner Retry of a granted service.** This pays R_inst, but it is the
  owner's act.

---

## Part 3: §2.3 holder scope

**The truncation argument is sound** (448). Closure means that a path that
leaves G never re-enters it. The restriction therefore cuts paths only at the
boundary, and I4 (845) is consistent with it. p's `start` cannot raise an
outside goal, so p's rollback cannot lower one. The §10 scenario (1228)
behaves as stated:

1. A Waiting, pending;
2. G_arm, then WTO, then Failed(wait);
3. rollback removes h, so A goes Inactive, and C never moves.

### F3 (MEDIUM): owner holders on granted services still cross the boundary outward; Theorem G item 2 is false, and the effect-set check is not redundant

Holder scope restricts only q ≠ owner. The owner's fixpoint (including `boot`)
runs through granted services into O.

**Route 1, rollback.**

- Setup: the owner runs `start A`, with A ∈ grant(p) and A requires C ∉
  grant, and nothing else holds C. Then T = {A, C}, and C reaches Ready.
1. p prevents A from becoming Ready until the request deadline passes, for
   example with `restart A` before each Rise (paid to R_p, effect set {A},
   allowed), or with SIGSTOP.
2. T fails and the rollback removes the owner's h. goal(C) falls, so C goes
   Ready → StopWait → … → Inactive.
3. Without p's action, C stays Ready.

The state sequence of an outside service changed, so item 2 (669) is false.
Item 1 holds: C is Inactive, not Failed.

**Route 2, `stop`.**

- Setup: A ∈ grant(p) is enabled (it holds `boot`) and requires C ∉ grant,
  and C is held only through A.
- p runs `stop A`. stop clears **x(A)** (451), which includes the owner's
  `boot`, so goal(C) would fall.

Three statements are affected:

- The effect-set check (648) **does** refuse this, because C's goal changes.
  But 652 calls that refusal impossible ("a refusal is a bug"), so an
  implementation that turns it into an assertion or a panic gives p a way to
  crash svd.
- Proof step (b) (676) says that p "cannot … remove holders of other
  principals". That is false: `stop` clears them, and it also aborts the
  owner's open transactions on A (454), with no rollback.
- When C is held elsewhere, the stop passes and p silently supersedes owner
  requests.

**Fix (recommended).** Make the truncation apply to **every** principal:
propagation never follows an edge that leaves a grant. Then:

- goal(C) for C ∈ O depends only on holders on O;
- both routes vanish;
- the effect-set check really is redundant;
- the proof's (b) becomes true.

The cost: a granted service's outside require-targets must be held on their
own, for example enabled. Compile can warn when an enabled granted A requires
a non-enabled outside C.

**Alternative.** Keep the owner's propagation, state the two routes as
exceptions in item 2, keep the effect-set check as a real refusal, and define
which holders p's `stop` clears.

### F4 (MEDIUM): the principal classes are undefined

Holder scope, payer and Theorem G use "owner" versus "q ≠ owner". But §8.2
(1143-1146) has `api.admin`, which contains the owner and possibly other
principals, and `api.read`.

- A non-owner admin q with grant(q) = ∅ has, by 448, a goal_q restricted to ∅.
  **That admin's `start` raises nothing.**
- If such admins count as owner, then Theorem G's "p ≠ owner" (667) must
  exclude them.
- Theorem G also covers any `api.read` principal r, and E1's ⟺ applies only
  when grant(r) ≠ ∅. So an owner terminal service may run as r, and r can
  crash it, which reaches Quiescing(1).

**Fix.** Define three classes:

- owner-class = `api.admin`, with grant(·) = ∅ required;
- delegated = grant ≠ ∅, which must be disjoint from `api.admin`;
- readers.

Apply E1's ⇒ to every API principal, or scope Theorem G to delegated
principals and say that a non-delegated uid running an owner service is the
owner's choice.

---

## Part 4: the 88-cell guard grid with `armed`

**Determinism.** Read with the per-row order of 773-780, every pair of guards
that can be enabled together is resolved:

- Waiting: G_down/G_ok, G_failed/G_arm, G_failed/G_wexp (the last is
  label-only);
- Starting/Ready: G_down/G_lost;
- Failed: G_down/recovery.

G_arm excludes G_ok (pending) and G_wexp (wexp ⇒ armed). **Deterministic.**

**Termination.** With fixed inputs, the longest guard paths are:

- Inactive → Waiting → Starting;
- Inactive → Waiting → G_arm;
- Failed → Waiting → Starting;
- Starting/Ready → StopWait → Stopping;
- Waiting(wexp) → Failed(wait).

Each has **at most 2 moves**, so "≤ 3" (794) holds with a margin. The path
cited at 793, Inactive → Waiting → G_arm → Failed(dep), needs G_failed to
become true after G_arm, which is an input change. That is harmless, but the
justification names the wrong path.

**Totality.** The prose is total. The **data** is not (F6).

### F6 (MEDIUM): the grid as data cannot express "no guard", nor the priority

- 88 = 11 × 8 makes every (state, guard) pair a Cell, and "the zero Cell is
  invalid" (881). Most pairs are not guards of their row: Waiting × G_up,
  Starting × G_up, Inactive × G_down and others.
- The only filler that §3.8 offers is `·` (Target Same, empty actions, 877).
  A `·` cell for an enabled guard **fires and stops the row** ("the first
  enabled guard fires", 768).
- Example: if Waiting × G_up is `·` and the row is scanned in grid-column
  order (G_up first), a Waiting service with goal up never reaches G_failed,
  G_arm or G_ok, so it stalls.
- The `Cell` type has no priority field, so the order of 773 exists only in
  prose. It differs from the column order (G_up, G_down, G_ok, G_failed, …).
  The orders happen to agree on every non-label conflict, but that is luck,
  not a specification.
- The Failed row's recovery guard is a conjunction (reason ∈ {dep, wait} ∧
  G_up ∧ G_ok) and is not one of the 8 columns. It needs `Cond =
  ReasonRecoverable` on the Failed × G_ok cell. Say so.

**Fix.**

- Add `Absent` as a Reach value, or as a distinct Target, meaning "not a
  guard of this row: skip". It is distinct from `·` and asserted never to be
  enabled-and-chosen.
- Add a `Prio` field, or store each row as an ordered list.
- Gate 2 must then mutate the priority as well.

### F7 (LOW): column headers STO and StTO contradict the prose

In the header (715), the Starting row puts StartTO's cell (`stopwait(fail)`)
under **STO**, and the Stopping row puts StopTO's cell (`kill`) under
**StTO**. But the prose, and REVIEW-3/REVIEW-4, use "StTO" for **StartTO**
(587, 618; R3:53, R4:26).

An implementer who maps the columns by the prose gets Starting × StartTO = ⊘
and Stopping × StopTO = ⊘. Gate 1 would catch it, because the BFS would
visit an Unreachable cell, but the table is supposed to be the specification.

**Fix.** Rename the columns to `StartTO`/`StopTO` and expand every
abbreviation in the key (711).

---

## Part 5: Gate 1's non-interference check (896), made precise

As written, it is loose in three ways:

- "with the principal's action removed" is a **one-step** comparison, but p's
  influence is **delayed**. In F3 route 1, p's `restart A` changes nothing
  outside at that step; the rollback happens later.
- It names only p's "events or operations", but under A1 p also chooses **every
  process and channel event of G's trees**.
- It does not say what the excepted StopWait timing is compared against.

**Proposed specification (2-safety, by self-composition).**

- **Partition of inputs.**
  - **High:** every API operation by p, and every process, channel and timer
    event of a service in G = grant(p).
  - **Low:** every owner API operation (including those on granted services),
    every event of a service in O, and the abstract-time ticks that age the
    windows.
- **Observation** obs_O(s): for every C ∈ O, (state, goal, cause, exit, wexp,
  armed, reason, |R_C(now)|), plus mode, |R_inst(now)| and the outcome of
  every owner transaction whose member set lies in O. Leave out epoch and
  gen: they are names, not behaviour.
- **The excepted timing as a low oracle.** In the checked model, replace
  G_free(C) for every C ∈ O that has a dependent in G by an oracle bit
  g_C, chosen by the environment as a **low** input (so both copies get the
  same value).
  - A separate check asserts that the real G_free(C) or SWTO always produces
    a value the oracle could have produced: term occurs within the StopWait
    sojourn, and an SWTO path is reported.
  - This is exactly item 2's exception, and nothing more.
- **Property.** For every pair of runs (r₁, r₂) of the self-composed model
  from one initial state, with **identical low input sequences** and
  **arbitrary, independent** high inputs, obs_O(r₁[i]) = obs_O(r₂[i]) at every
  step i.
- **Algorithm.** Do a BFS over the product state (s₁, s₂). Low inputs are
  applied to both copies, and a high input to either copy alone, the other
  stuttering. The check fails on the first product state with
  obs_O(s₁) ≠ obs_O(s₂).
- **Scope.** The product squares the state space. Use N ≤ 3 with |G| ≥ 1 and
  |O| ≥ 1, plus symmetry reduction, and keep N ≤ 4 for the one-copy
  properties.
- **Required witnesses.**
  - The check must go **red** on draft 8's goal formula, through F3 route 1.
    That proves it can detect delayed influence.
  - It must go green after F3's fix.
  - It must go red on a mutant that removes `ErrGrantNotClosed` for a want
    edge (REVIEW-4 E2).
- **Stated limit.** svd-resource channels (F1) and capabilities (F2) are
  outside the abstract model. They need the H_OS-E5 tests and the A1 probe,
  not Gate 1.

---

## Part 6: other findings

### F5 (MEDIUM, blocks Gate 1): P2 is false on fair runs without SEt

- Windows age out (489), and Gate 1 models that (891).
- Take a service whose cycle Waiting → Starting → (StartTO) → StopWait →
  Stopping → Backoff takes longer than intensity.window / (max_A + 1), for
  example start.timeout = 90 s with window = 60 s and max = 3.
- It is charged on every cycle but never overflows, and it never reaches
  Ready, Done or Failed. The run is fair and contains no SEt, so P2 (858) is
  false.
- Gate 1's SCC search (860, 899) finds that SCC, because the only exit from
  Starting in it is StartTO, not SEt, so the gate goes **red**.
- Since real durations are not modelled (910), no restatement in time can
  rescue it inside Gate 1.

**Fix.**

- **P2a (Gate 1):** goal = up ∧ requirements stable ∧ finite SEt ⇒
  ◇(Ready ∨ Done ∨ Failed) ∨ □◇Charge(A). Every non-terminating attempt
  cycle is charged and reported.
- **P2b (Compile):** if intensity.window < (max_A + 1) × (start + SW + stop +
  kill + backoff.min), report or refuse it as a "never-failing loop"
  configuration.

**E5's stated reason is also not decisive.** Charging the k-th SEt was
rejected because it "would let supervisor-side pressure escalate a terminal
service" (542). But an uncharged SEt loop escalates it anyway, through any
terminal **require-dependent's** WaitTO, which is the F1 route. Keep the
choice, but state that cost.

### LOW

- **F8.**
  - Line 607, "never re-run by a dependency loss", contradicts E7 (589). Add
    "if its exit is dequeued before `term`".
  - Stopping/Killing × X1 record `sig` and do not read the status pipe. A
    helper `perm` that is dequeued after `term` therefore costs one charged
    re-run before Failed(spawn). The bound is unchanged.
- **F9.** p's `retry` pays R_p twice if "every write … is one API charge"
  (642) and the Failed × Ry cell's `pay(caller)` (726) are separate. State
  that they are the same payment.
- **F10.** When a subtree's cgroup is "handed to the principal's own
  instance" (1159), the per-epoch limits are written by p's instance, so
  H_OS-E3 is not enforced for that subtree *(inference; the ownership of
  cgroup v2 interface files after delegation is from memory)*. Either set
  the class-H limits at the delegation root, or drop the hand-over in v1.
  This is consistent with option A, where svd runs the granted trees.
- **F11.** The notify socket must receive with a control buffer sized for
  credentials only, so that an `SCM_RIGHTS` flood from a granted main
  process cannot install fds into svd *(from memory: the kernel discards fds
  that do not fit and sets `MSG_CTRUNC`)*. Belongs with H_OS-E5.

---

## Summary

| item | result |
|---|---|
| 1 | E1-E4 and E8-E13 are closed. E5 is not closed (F5). E6 is partly closed (F1, F2). The costs of E7 and E14 are acceptable; E7 needs a one-line fix at 607. Every REVIEW-3 and REVIEW-4 replay is closed. |
| 2 | Theorem G is **not** established. F1 (svd resources, HIGH) reaches Quiescing(1). F2 (capabilities, and uid identity, HIGH) bypasses A1. F3 and F4 falsify item 2 and the theorem's scope. A1 and A2 are restated as tests above. |
| 3 | Holder scope is sound for delegated holders, and rollback of p's requests is safe. Owner holders still cross the boundary (F3), and the admin class is undefined (F4). |
| 4 | The grid is deterministic and terminating: at most 2 moves with fixed inputs, so "≤ 3" holds. Its data encoding is not total (F6), and two headers are wrong (F7). |
| 5 | A 2-safety self-composition specification, with a low oracle for G_free, and required red witnesses (Part 5). |
