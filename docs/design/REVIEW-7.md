# REVIEW-7 of SUPERVISOR.md (draft 10)

Review pass, 2026-11-23. This review did not edit SUPERVISOR.md.

Scope: REVIEW-6 in full. SUPERVISOR.md draft 10: header (3), §1.6 (221-286),
§1.7 (288-333), §2.3-2.4 (465-508), §3.1-3.8 (512-1050), §5.1 (1112-1138),
§8.1-8.2 (1246-1305), §9 (1331-1337), §12 (1505-1512, 1597-1611). Line numbers
are draft 10's. Two external sources were opened (Part 4). Statements marked
*(from memory)* were not checked against a source.

**Recorded user decision (binding, not in draft 10).** A **terminal** service
exposed to a grant (H_OS-A4) is **refused at Compile**. A non-terminal one keeps
`WarnExposedToGrant`. §12 (1512) still says « left for the user », and §3.3
(765) still says the warning covers it. Draft 11 must change both. N10 below
shows that the decision, as worded, does not cover every case in which one
crash escalates.

**Verdict.**

- **In the single-queue model, Theorem G holds** under E1-E5, A1-A4. N1's fix
  is correct: no cell, guard or settle rule of O reads G (864-870, 936-943).
- **For the runtime, Theorem G is not yet established.** The inventory H1-H9
  misses two inputs that p controls, and both reach the **core**:
  - N7: G's orphans are reparented to the core;
  - N8: the lane's progress reaches the core through the owner-to-lane queue,
    owner replies and the status snapshot.
  Neither is a direct escalation trace. Both break the lane premise « p's load
  reaches the core only through the kernel » (3, 716). N8 also breaks
  item 1's « the same low inputs are enabled in both ».
- **H9/A4 is honest, but its warning is wrong** about which C escalate (N10).
- No HIGH finding.

---

## Part 1: is the inventory H1-H9 complete?

**No.** Two inputs are missing (N7, N8), one row's mechanism is unspecified
(N9), and H9's scope is under-stated (N10). H1-H6 are sound for what they list,
and so are H8's API and lane-death halves.

### N7 (MEDIUM): orphans of G's trees are reparented to the core

**Rule.** « As PID 1, svd also reaps orphans » (1117). Reaping uses
`waitid(…, WNOWAIT)` on P_ALL (561, 1116). Nothing makes the lane a subreaper:
`subreaper` appears nowhere in the draft. FreeBSD's `PROC_REAP_ACQUIRE` (1125)
is held by the process that tracks trees. Since draft 10 that is the lane for
G, and the draft does not say so.

**Trace (Linux, mode C, svd is PID 1).**

1. p's granted A runs a loop: fork a child; the child forks a grandchild and
   exits at once; the grandchild exits at once.
2. Each grandchild is reparented to PID 1, the **core**, not to the lane. Its
   cgroup stays `deleg/<uid>/…`, so `pids.max` bounds the population, not the
   rate.
3. The core's P_ALL observe returns each zombie of p. The core must route it
   (there is no epoch of O), then reap it. That costs CPU in the **core's**
   cgroup, at a rate p chooses: one fork-and-exit pair per few µs per CPU of
   `budget.cpu`.
4. The reaper is a single source. p's zombies and the MainExit of every O
   service share that source, so round-robin (518) gives O no fairness
   against them.
5. p has turned capped CPU into uncapped core CPU. That is N3's route,
   which the lanes were meant to close. H3 (« G's process events are G's
   states ») and H6 (« the core holds no … of p's ») are false here.

**Same effect at lane death (717).** The trees of a dying lane are reparented
to the core before `cgroup.kill` completes. This is bounded by `budget.pids`
once per owner restart, so it is acceptable, but it should be stated.

**Fix.**

- The lane sets `PR_SET_CHILD_SUBREAPER` (Linux) and `PROC_REAP_ACQUIRE`
  (FreeBSD) at start *(from memory)*.
- The core observes O's exits through per-epoch pidfds, or through
  `waitid(P_PID)`, never through a P_ALL that p's processes can fill.
- Orphan reaping becomes a source of its own in the round-robin, bounded per
  round.
- Add H10 to the inventory, and add a fork-and-exit flood to the F1 chaos test.

### N8 (MEDIUM): the lane's progress is an input to the core

p controls how fast its lane runs. The lane shares p's `cpu.max` and
`memory.max` (716, 726), so p's busy trees throttle it, and p's trees held near
`memory.max` keep it in direct reclaim. Three couplings let that rate reach the
core. The draft specifies none of them.

**(a) The owner-to-lane queue (716) is bounded.** The draft does not say what
happens when it is full.
- If the core's enqueue blocks, the core stalls.
- If it does not block, nothing says what the owner receives.

**(b) Owner operations on G wait for the lane.** An owner transaction on A ∈ G
runs in the lane, and so does its deadline timer (502, 716). While the owner's
request waits for the outcome, it holds a connection, an fd and a buffer in the
**core**.
- **Trace.** An owner-class cron job runs `svd restart A` every minute. p keeps
  its lane starved, so no answer ever comes.
- The core's owner-class connections and fds grow. The core refuses `accept`
  at its spawn reserve (720), which protects spawns. But the owner's next
  `svd start C`, for C ∈ O, is then **refused** in this run and accepted in
  the run without p.
- A low input is enabled in one run and not in the other. Item 1 (768)
  asserts the opposite.
- The core's memory bound (721), « a fixed term plus owner-class terms », also
  assumes that owner requests end.

**(c) `svd status` reads the lane's snapshot under a sequence lock (716).**
- A writer stopped while the sequence is odd makes an unbounded reader spin in
  the core. `cpu.max` throttling can stop the lane mid-write for most of a
  period, repeatedly.
- If the lane runs as p (N9), p can write arbitrary bytes there, and the core
  parses them.

**Fix.**

- The enqueue is non-blocking. A full queue answers the owner with
  `ErrLaneBusy`.
- The **core** owns the deadline of every owner request that it forwards.
- Snapshot reads use bounded retries and validate what they read.
- State the « control fd per lane » protocol (720): what, if anything, the core
  reads from it.
- Add H11, « the lane's progress, as seen by the core », to the inventory.

### N9 (MEDIUM): the lane's uid is not stated, and each choice breaks a claim

**If the lane runs as p:**
- p can ptrace it, `SIGSTOP` it indefinitely (N8 c), and forge everything the
  lane sends to the core;
- the lane writes the epoch cgroups' interface files, so p can write them
  too, which contradicts E2's « written by svd and never by p » (712, 725).

**If the lane runs as root (or as the owner):**
- Linux is fine, because the cgroup counts the lane whatever its uid;
- on FreeBSD the cap is `rctl` on `user:<uid>` (727), which **does not count a
  root lane**. The lane is then uncapped, and draft 9's N3 route returns
  through the lane.

**Fix.**

- State: the lane runs as root (system instance) and drops every capability
  that it does not use.
- On FreeBSD, add `process:<lanepid>` rules for the lane, or run the lane in a
  per-principal jail or login class. rctl(8) lists exactly these four subject
  types: process, user, loginclass, jail.

### N10 (MEDIUM): H9/A4 is broader than any Compile rule can see, and the `terminal` flag is wrong

**The flag.** 765 says the `terminal` flag « marks the case in which a single
crash of C escalates ». That is false. A non-terminal C that a terminal T
requires escalates the instance too: C reaches Failed(intensity), T reaches
Failed(dep) through G_failed (860), and a terminal reaching Failed escalates
(682).

**Trace (non-terminal C, so the user's rule does not refuse it).**

- Setup: T = `app`, terminal, requires C = `postgres`, which is not terminal.
  p's `reporting` ∈ grant(p) requires `postgres`, so `postgres` is exposed.
- p sends a query that crashes `postgres`, intensity.max(postgres) + 1 times
  within the window.
- `postgres` reaches Failed(intensity), `app` reaches Failed(dep), and the
  instance goes to Quiescing(1).
- With the user's rule, this plan compiles with a warning only.

**Second route, any exposed C.** C's crashes pay R_inst (payer = owner). So
max_inst + 1 crashes of exposed services escalate the instance, whatever
their terminal flags are.

**Third, and the one that cannot be closed.** p reaches C's listener whether
or not an edge exists. Since draft 10 a cut edge does nothing for C (870,
1299). So refusing on an edge invites the owner to delete the edge, which
removes svd's only view of the exposure and keeps the exposure itself.

**Recommendation.**

- (i) Extend the user's refusal from « C terminal » to « some terminal T has a
  require path to C », the set whose Failed escalates. Name it in the error.
- (ii) Rename the warning flag to `escalates`, and state the R_inst route in
  A4 for every exposed C.
- (iii) Tell the user plainly what Theorem G now says: « delegation cannot
  escalate **unless p can crash an owner service that it can reach** ». A4
  carries most of the remaining risk, and svd sees only the part of it that
  has an edge.
- An attribution-free mitigation exists, but it is a **user decision**, not a
  fix: a per-service `instance.charge = false` that makes C's crashes charge
  R_C only.

### Rows checked and found sound

- **H1, H2.** One holder per (principal, service) (470), and one partition per
  request (486).
- **H4.** The defensive walk (724).
- **H5.** O5′ drains only req(C) ⊆ O (563-567).
- **H6.** Sound on Linux, apart from N7.
- **The configuration.** It is not a p input: recipe layers are signed, and
  site layers are root- or owner-owned (294-295). p authors no layer.

### N11 (LOW): the serialisation lemma transfers the O projection only

**What 519 claims.** « a run of the partitioned runtime is a run of the
single-queue model ».

**Trace.** B ∈ O crashes. In one core step, C ∈ O goes StopWait, G_free(C)
holds (its only dependent A ∈ G is cut), and C is signalled. The lane sees the
new snapshot later. Until then, A ∈ G is Ready while req(A) ∋ C ∉
{Ready, Done}. The single-queue model never shows this state after
propagation (I1, 955).

**Consequences.**
- Theorem G is unaffected, because it needs obs_O only.
- But Gate 1's one-copy results (I1, T1) do **not** carry over to granted
  services with outside targets. A lane reading a stale snapshot can also
  spawn A after C has begun stopping.

**Fix.** Restate the lemma as a refinement of the O projection, and state I1
for cut targets « up to snapshot lag ». This cost falls on p alone.

---

## Part 2: N1-N6 and Gate 1 gaps 1-5

| item | status |
|---|---|
| N1 | **Closed** in the model: G_free over uncut dependents (864-870), SW likewise (936-943), T2 restated (973). |
| N2 | **Closed**: O5′ (563-567), and its « one level suffices » argument holds through I1 inside O. |
| N3 | **Closed on Linux.** FreeBSD: the action column is wrong (Part 4), and the lane is uncapped if it runs as root (N9). macOS: reader lanes are uncapped (Part 5). |
| N4 | **Closed** (470, 503, 721). LOW: say that the audit file is per lane. A shared file lets p's records rotate the owner's records out. |
| N5, N6 | **Closed** (982, 724). |
| gap 1 | **Closed** (767, 1025). |
| gap 2 | **Closed** (1028). |
| gap 3 | **Closed** (502, 1024). |
| gap 4 | Oracle removed (1026). The witnesses are discussed below. |
| gap 5 | Partly closed. The chaos test lacks N7's fork-and-exit flood and N8's starved-lane run. |

### The witnesses

**`freeAcrossCut` goes red, but not for the reason its fixture gives (N12,
LOW).**

- The fixture (1030) has C's **goal go down** and expects a crash of C to be
  « charged to R_inst in one copy only ».
- With goal down, settle rule 1 (658) fires first: Inactive, uncharged. The
  R_inst divergence never happens.
- The check is still red: state(C) is StopWait in one copy and Stopping in the
  other, and both are in obs_O.

**Fix.** Use REVIEW-6 N1's route: B ∈ O crashes, C goes StopWait(dep) with its
goal up, and its exit `fail` is charged by rule 4. The witness then also shows
the `mode` divergence that N1 claimed.

**`swAcrossCut` cannot go red in Gate 1 (N13, LOW).**

- SW is a duration. Gate 1 works in abstract time, and « real durations are not
  modelled » (1047). SWTO is a low timer event of C that may fire at any step
  in both copies, so SW's value never enters the model.
- The witness must be a `Compile` unit test instead: SW(C) is unchanged when a
  cut dependent's `stop.timeout` changes.

---

## Part 3: does the cut make an owner service miss a stop or a free?

**No.** By closure no edge goes from O into G (477), so no service of O
depends on a granted one. The only dependents that G_free and SW now skip are
granted ones, and C's own correctness never needed them stopped.

**« A dependency that is gone but never noticed » does not arise.** A ∈ G
notices C's loss through G_lost, which reads C in the uncut direction. The
notice comes late, by the snapshot lag (N11). A dead lane notices nothing,
but then its trees are killed (717).

The owner's costs (a)-(c) (480-485) are stated correctly, and the operator
recipe is present.

---

## Part 4: bibliography, opened

### unix(7)

Read at https://man7.org/linux/man-pages/man7/unix.7.html (Linux man-pages
6.19, 2026-02-08).

- **Confirmed.** « On Linux, connecting to a stream socket object requires
  write permission on that socket ». Also: « The owner, group, and permissions
  of a pathname socket can be changed (using chown(2) and chmod(2)) ».
- **Not confirmed: `fchown` on the socket fd (719, 1264).** The page names
  chown(2) and chmod(2) on the pathname. It mentions fchown(2) and fchmod(2)
  only for **abstract** sockets, where they « ha[ve] no effect on the
  accessibility of the socket ».
- `fchown` on a socket fd acts on the socket's own inode, not on the inode
  bound at the path *(from memory, not in the page)*.
- **Fix.** Bind in the root-owned 0711 directory with a restrictive umask, then
  `fchownat`/`chown` the path, then `lstat`-verify the owner and mode before
  `listen`. As written, the socket keeps its creator's ownership:
  - with umask 077, p cannot connect (fails closed);
  - with a loose umask, anyone can connect, and the peer-credential check is
    the only gate.
- **Portability.** The page also says: « on some systems (e.g., older BSDs),
  the socket permissions are ignored. Portable programs should not rely on
  this feature for security. » So the FreeBSD and macOS row « the same layout »
  (1265) cannot rest on unix(7). Keep the per-principal 0700 directory for
  those OSes, or cite their own sources.

### FreeBSD rctl

Read at https://man.freebsd.org/cgi/man.cgi?query=rctl&sektion=8 (FreeBSD 15.1
page) and https://man.freebsd.org/cgi/man.cgi?query=rctl&sektion=4.

- **Names confirmed:**
  - `maxproc`;
  - `memoryuse` (« resident set size »);
  - `swapuse`;
  - `pcpu` (« %CPU, in percents of a single CPU core »);
  - `readbps`, `writebps`, `readiops`, `writeiops`.
- **Corrected: the actions.** 727 says « each `:deny` or `:throttle` ». The
  page says:
  - `deny` is « not supported for cputime, wallclock, readbps, writebps,
    readiops, and writeiops »;
  - `throttle` is « only supported for readbps, writebps, readiops, and
    writeiops ».

  So the rows are `maxproc`, `memoryuse`, `swapuse` and `pcpu` with `:deny`,
  and the four IO resources with `:throttle`. An unsupported pair « will result
  in error », so draft 10's wording would fail at rule load.
- **`kern.racct.enable`.** rctl(8) does not mention it. rctl(4) does: « This
  defaults to 1, unless options RACCT_DEFAULT_TO_DISABLED is set in the kernel
  configuration file ». GENERIC sets that option, so it is off by default and
  needs a loader tunable and a reboot *(from memory, unverified)*.
- **Subjects.** The page lists exactly process, user, loginclass and jail.
  This bears on N9.
- `memoryuse` is RSS, not committed memory, and the swap is counted
  separately. The capacity sum (732) adds the two, which is consistent.

---

## Part 5: is « macOS refuses grants » honest?

**Honest as a refusal, but the user should hear three things.**

1. **Delegation is Linux-only in v1.**
   - macOS refuses every grant (728).
   - FreeBSD needs racct, which is likely off by default (Part 4).
   - Windows is unverified and in P3.

   One HCL plan with a grant therefore loads on Linux and is refused on the
   other platforms the user listed. That is a product decision, not a footnote.
2. **macOS readers are not capped, and the draft says otherwise.**
   - `budget.memory` and `budget.cpu` are required for readers (715), but no
     macOS mechanism enforces them.
   - 728 says the residual is « the CPU-contention part of H_OS-A3 », but A3 is
     defined as contention **below the caps** (743).
   - `budget.inflight` bounds concurrency, not rate. `budget.refusals` counts
     refused **writes** only. So a reader can keep its lane busy on many cores
     with valid reads.

   **Fix.** On an OS without a cap, a reader lane runs with `GOMAXPROCS=1` and
   a per-reader request rate (`budget.requests`), or readers are refused too.
3. **No mechanism was investigated.** Candidates exist that throttle without
   a quota: `RLIMIT_CPU` kills rather than caps, and taskpolicy/`setiopolicy_np`
   apply background throttling *(from memory)*. None is a cap. « Refused until
   verified » is the right default, but §12 should list this as an open feature
   gap, not leave it implicit.

---

## Summary

| item | result |
|---|---|
| 1 | Not complete. **N7** (MEDIUM): orphans reparented to the core. **N8** (MEDIUM): lane progress reaches the core. **N9** (MEDIUM): the lane's uid is unstated. **N10** (MEDIUM): the A4 flag is wrong, and the user's terminal-only refusal misses the require-ancestors of a terminal and the R_inst route. **N11** (LOW): the lemma covers obs_O only. |
| 2 | N1, N2, N4-N6 and gaps 1-3 are closed. N3 is closed on Linux only. `freeAcrossCut` is red on draft 9, with a wrong fixture narrative (N12). `swAcrossCut` cannot be red in abstract time (N13). |
| 3 | No owner service needs a granted dependent. A notices C's loss late, by the snapshot lag. |
| 4 | unix(7): the write rule is confirmed. `fchown` is not supported by the page; use chown on the path. The page disclaims BSD behaviour. rctl: the names are confirmed. The deny/throttle column is corrected. racct is a loader tunable. |
| 5 | The refusal is honest. Delegation is Linux-only in v1, and the user should be told. macOS reader lanes are uncapped, contrary to 715 and 728. |
| G | **The model holds under A3/A4. The runtime does not yet hold:** N7 and N8 are missing inventory rows. A4 is the dominant residual risk. |
