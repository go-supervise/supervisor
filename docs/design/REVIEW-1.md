# REVIEW-1 of SUPERVISOR.md (draft 3)

Reviewer: adversarial review, 2026-11-23. This review did not edit SUPERVISOR.md.
Read: SUPERVISOR.md in full. Spot-checked: BIBLIO-B.md:118-127 (dinit relations).
Anything marked *(from memory)* was not opened in this pass.

Decisions the user accepted on 2026-11-23, and which this review assumes: R0;
`go-zpkg/supervisor` / `zsup`; s6 output decided after v0.1.0; API grants as
K5, so they can only shrink; no macOS credentials on disk; rollback by default.

**Verdict: not ready for P0.** P0 is `Compile` plus the FSM table. C1, C2,
M1 and M2 change `Compile`. H1–H4, M7 and M8 change the table. Each fix below
is a local edit, so one more draft (draft 4) should be enough.

---

## CRITICAL

### C1 — §1.3 K5, §8, §1.5: an allow-set's ⊤ is the universe, so leaving the field out opens everything (fail-open)

- K5 has unit ⊤ = U (§1.3, table row K5). An absent field is ⊤ (§1.2).
- **Counter-example (API grants, which the user decided are K5).** No layer
  writes `api {}`. Then `admin = ⊤ = U`, and **every local principal is admin**
  of the instance. The orientation the user chose ("can only shrink") starts
  from "everyone".
- **Counter-example (`exempt_on`).** A recipe sets `no_new_privileges = ✓` and
  says nothing about `exempt_on`. Then `exempt_on = U`, which includes linux.
  The refusal rule in §7 ("target ∉ exempt_on ∧ unavailable → refuse") can
  therefore never fire, and the protection is silently not enforced.
  `capabilities`, `syscalls` and `address_families` fail open the same way.
- **The baseline cannot fix this.** Theorem P meets maps pointwise **by key**.
  A baseline layer only constrains the services it names, so a service defined
  in any other layer never meets the baseline. The §1.5 example
  (`capabilities = ∅`, `exempt_on = {macos, windows}`) protects nothing that it
  does not list by name.
- **Fix.**
  - Define `Plan(s) = B ⊓ ⊓ᵢ Lᵢ(s)` for every service key `s`, where B is a
    mandatory, universal baseline element that `Compile` refuses to run
    without (`ErrNoBaseline`). This is still a meet, so the ACI laws are kept.
  - Make `api.admin`, `api.read` and `exempt_on` required-concrete. The
    alternative is a baseline that states owner-only and `exempt_on = ∅`.
  - Add a test: a plan whose layers never mention a K5 field must not hold U
    for it.

### C2 — §1.4 Theorem M: it covers K3–K6 only, while privilege also flows through K1 fields and through new map keys

- Theorem M is true of the hardening fields. But "composition only adds
  constraint" (law L) also counts it as *constraint* when a layer makes a ⊤
  field concrete.
- **Counter-examples.** Any writer of any layer can:
  - add `env "LD_PRELOAD" = "/home/u/x.so"` (an env map key absent elsewhere
    is ⊤);
  - set `run_as.user = "root"` where the recipe left it ⊤;
  - define an entirely new `service "x" { exec = …, run_as.user = "root" }`;
  - add a K6 required credential, so that a service it controls receives
    another service's secret.

  None of these is a "relaxation" in lattice terms, and all of them are
  privilege escalation. The document never says who may write a layer, nor
  with what file permissions.
- **Fix.**
  - Tag the privilege-bearing fields (`exec`, `run_as.*`, `working_dir`, env
    keys, credentials, the service set itself) as **recipe-origin only**. A
    non-recipe layer may only meet them with a value they already have
    (`ErrWideningField`).
  - Specify how layer files must be owned and protected, per instance, and
    check it at load like ssh does.

## HIGH

### H1 — §3.4 vs T2 (§2.3, §3.6): the table does not implement stop ordering

- Ready × GoalDown → Stopping **[signal]**, immediately.
- **Counter-example.** A requires B, and `stop` clears x for both. In one step,
  both go to Stopping and both are signalled. T2 is violated on the first BFS
  step.
- If phase 3 instead delays B's signal, B's StopTO is already running. B is
  then force-killed, or reaches Failed(unkillable), without ever having
  received SIGTERM while A drained.
- **Fix.**
  - Add a state `StopWait`, entered with a guard that dependents are not live.
  - Give it a timer, because Theorem B must extend to it.
  - Escalation (§3.3) and rollback (§2.3) both rely on T2, so the fix covers
    them too.

### H2 — §3.4 vs §5.2: `Exit` is the main process's exit, but the unit is the tree

- Ready × Exit → afterExit → Backoff → Waiting → Starting (epoch+1) never
  passes through a state that waits until the old tree is empty.
- **Counter-example.** A daemon's master process exits while its workers still
  hold the port, because a cgroup or job kill is asynchronous. The new epoch
  then gets `EADDRINUSE` and loops until it reaches intensity.
- I2 and I6 hold only because a "live epoch" is defined by the state.
  Physically, two incarnations run at once.
- T2's "no live process" cannot be decided from the states.
- **Fix.** Split the event in two: `MainExit` → `Draining` [kill tree, arm
  StopTO] → `TreeEmpty`. TreeEmpty is observed through cgroup.events
  `populated 0`, `JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO`, or reaper status. Redefine
  I2 and I6 on trees.

### H3 — §3.4 Stopping × StopTO: `forced` is never reset

- **Counter-example.**
  1. Epoch 5 is stopping: StopTO fires, then kill, then `forced = 1`, then
     Exit, then Waiting.
  2. Epoch 6 runs and is later stopped.
  3. Its **first** StopTO hits `forced = 1` and goes to Failed(unkillable),
     with **no SIGKILL ever sent** to epoch 6.
- It gets worse: Failed × Retry → Waiting spawns epoch 7 while epoch 6, with a
  leaked tree, is still alive. That means two writers on one data directory.
- **Fix.**
  - Reset `forced := 0` on every entry into Stopping. The cleaner option is to
    make `forced` part of the state: `Stopping`, `Killing`.
  - Make Retry refuse while a retired epoch's tree is still populated (state
    `Leaked`).

### H4 — §3.1 epochs: they increment only at spawn, so timers armed within one epoch cannot be told apart

- Epochs increment only at Waiting → Starting.
- **Counter-example** (wait.timeout = 60 s):
  - t = 0: Waiting, WaitTO#1 armed (epoch e).
  - t = 5: DepFailed → Failed. The table does not disarm the timer.
  - t = 50: Retry → Waiting, WaitTO#2 armed (epoch e).
  - t = 60: WaitTO#1 fires. The epoch matches, so the service goes to Failed
    [cause=wait] after only 10 s.

  Waiting → GoalDown → Inactive → GoalUp → Waiting gives the same result.
- **Fix.** Give every armed timer its own generation (a sojourn counter), and
  match the timer, not the epoch. The BFS must inject *same-epoch* stale timers.

### H5 — §8, §4.1: the endpoints can be squatted and forged

- **Windows.** Named pipes live in the flat global `\\.\pipe\` namespace, not
  in "the instance directory". Any local user can create the predictable name
  first. A client that connects without `SECURITY_SQOS_PRESENT |
  SECURITY_IDENTIFICATION` lets the squatter **impersonate an admin client**
  *(from memory)*.
- **Unix.** The safety of the instance directory is never stated. macOS has no
  `XDG_RUNTIME_DIR`.
- **sd_notify.** Attribution by socket drops systemd's sender check
  (`NotifyAccess=`). If the socket is abstract, which is scoped to the netns,
  or if its path is reachable, any process can send `READY=1` or `WATCHDOG=1`
  and defeat liveness.
- **Fix.**
  - Windows: use `FILE_FLAG_FIRST_PIPE_INSTANCE`, publish a random pipe name
    in an ACL-protected file, have clients use SECURITY_IDENTIFICATION and check
    `GetNamedPipeServerProcessId`. The alternative is AF_UNIX, which exists on
    Windows 10 1803 and later *(from memory)*.
  - Unix: verify the owner and mode of every path component.
  - sd_notify: use path sockets only, inside a 0700 per-epoch directory, plus
    an `SCM_CREDENTIALS` check that the sender is in the epoch's tree.

## MEDIUM

### M1 — §1.2/§1.3: ⊥ is used both for the lattice bottom and for a conflict

- §1.2 says "a ⊥ in any field is ErrConflict". But for K3–K7, the bottom
  element is a legitimate value:
  - K5: `capabilities = ∅` (used by the baseline);
  - K4: ✓;
  - K3: 0;
  - K7: require.
- Read literally, Theorem P's smash product turns `capabilities = ∅` into
  ErrConflict. Only K2 ("∅ = ⊥") is consistent.
- **Consequence.** K5 **can never conflict**. A recipe that needs `inet`, met
  with a site that allows only `unix`, gives `{inet} ∩ {unix} = ∅`. That is a
  valid plan for a daemon that cannot run.
- **Fix.**
  - Adjoin ⊥_err to every kind explicitly, and fuzz law Z against ⊥_err, not
    against the order's bottom.
  - Pair every K5 field with a K6 `needs` set, and add the check
    `needs ⊆ allows`, else ErrConflict.

### M2 — §1.5 and R0: no defaults means no site override of any concrete value

- Under R0, a recipe that writes `stop.timeout = "10s"` cannot be raised by a
  site: [10s,10s] ∩ [60s,60s] = ⊥.
- If the recipe leaves an interval instead, **every** deployment must pin
  every timing field, or get ErrIncomplete. "Default 10 s, tunable" cannot be
  written at all.
- R0 was accepted for *hardening*. This is a separate cost that the user has
  not been told about.
- **Fix.** Use CUE-style defaults, resolved **after** the meet, and only on
  non-hardening K1/K2 fields:
  - if the met value is a non-singleton interval that contains the default,
    use the default;
  - meet the defaults themselves as K1, so two different defaults conflict.

  ACI is unaffected, and Theorem M is unaffected, because it does not concern
  K1/K2.

### M3 — §1.4: Theorem M is proved for the order, not for what is enforced

- Law L gives `P ⊑ Lᵢ`. The claim "restricts at least as much" also needs the
  enforcement map to be monotone.
- **Gaps.**
  - Does `read_only` (K6) make a path *visible*? If it does, adding a path
    widens access.
  - `exempt_on` is one set per service, not one per protection. A recipe
    therefore cannot exempt NNP on macOS while requiring Landlock everywhere
    else.
  - `terminal` and credentials are K4 and K6, but they are not restrictions.
- **Fix.**
  - State the denotation `eff(plan)` per OS, and prove or test that it is
    monotone.
  - Make `exempt_on` a field per protection.

### M4 — §2.1: the dinit mapping is wrong, and soft dependencies cannot be expressed

- BIBLIO-B.md:126 shows that `waits-for` **activates** its target. Our
  `order` activates nothing, so the two are not the same.
- systemd's `Wants=`+`After=` cannot be written either: "start B if possible,
  start me after it, survive its failure".
- Socket-activated or mutually-calling peers (A ↔ B) are a cycle, and
  therefore ErrCycle.
- `x` is set only by the API (§2.3), so **no layer can declare the set of
  services wanted at boot**.
- **Fix.**
  - Add `want` (activate + order, no I1, no DepFailed). It sits between
    require and order in K7's chain.
  - Add a K4 `enabled` field per service that seeds `x` at instance start.
  - State that socket activation is out of scope.

### M5 — §3.4/§3.5: Waiting's timer fails dependents for good on a transient upstream failure

- **Counter-example.** B is in backoff for a total of more than A's
  wait.timeout. A goes to Failed [cause=wait] and **stays there**, because
  `start B` emits Retry only for B's closure, not for B's dependents.
- With rollback on by default, the transaction then also tears B down.
- **Fix.**
  - Either have Waiting time out only for order targets that are not
    progressing, or check `wait.timeout ≥` the worst-case upstream sum at
    `Compile`.
  - Make DepFailed dependents retry automatically when the cause becomes Ready.

### M6 — §2.3/§3.3: rollback overwrites a later request, and escalation does not freeze the API

- **Counter-example (lost update).**
  1. T1 is `start X`.
  2. A second client issues `start X --no-rollback`.
  3. T1 fails, restores `x(X) := false`, and so cancels the second request.
- **Counter-example (escalation).** While zsup is escalating ("unwant all,
  exit 1"), an API `start` sets x again. zsup then either never quiesces, or
  exits with services still starting.
- **Fix.**
  - Make `x` a set of holders (request IDs). A rollback removes only its own
    holder.
  - Make escalation a terminal mode that refuses writes and aborts the
    transactions that are open.

### M7 — §3.2/§3.4: the table does not type-check

- `cause` is declared as {none, dep, fail}. The cells use `wait`, `exited`
  and `unkillable`.
- `restart.on` is K1 (one value), but it is used with `x ∈ restart.on`.
- Several cells are elided: `[…]`, `[none]`, and Ready×Fall without "signal,
  arm StopTO".
- 8 × 15 = 120 is not the real count:
  - the table has 14 columns, plus Expire;
  - `Exit(ok|fail)` is two events;
  - the brief's 7×13 matches neither.
- **Edge or level?** The document does not say whether DepsOK, DepLost and
  DepFailed are edge-triggered or level-triggered. If they are edges, a
  service that enters Waiting when its requirements are *already* Ready never
  receives DepsOK, and fails at WaitTO.
- The `restart` verb has no semantics: x is cleared and set again inside one
  step, so the goal does not change and no event fires.
- **Fix.**
  - Model the cascades as **guards** that are evaluated in propagation.
  - Enumerate the cells, including Exit(ok) and Exit(fail).
  - Define `restart` as two steps, or give it its own event.

### M8 — §3.6 I5 is false as written, and §3.3's bound has an operator hole

- Failed keeps |R_A| = max+1 until Expire drops an entry. That is many steps,
  not "within the step", so the BFS will flag I5 at once.
- Retry sets R_A := ∅. A script that loops on `zsup start` therefore restarts
  without bound, and P3 does not hold.
- **Fix.**
  - Restate I5 as `state ≠ Failed ⇒ |R_A| ≤ max`.
  - State P3 as "absent Retry".
  - Optionally rate-limit Retry through R_inst.

### M9 — §3.7: the verification as specified

- **What generation needs.** To generate TLA+, a Cell must be pure data
  (guard, actions, target), not Go closures.
- **What drifts.** Propagation, goal derivation, charge and rollback are
  *outside* the table, so the TLA+ for them would be hand-written and would
  drift.
- **What both checks miss.**
  - real durations (M5);
  - tree physics (H2, H3);
  - the order in which the OS reports events (Rise vs Exit);
  - API concurrency (M6);
  - N > 4;
  - any wrong cell, since both checks read the same table.
- **The mutation gate cannot be met.** Cells that the epoch filter makes
  unreachable (the "— stale" cells), and mutations that change nothing, cannot
  turn the suite red.
- **Fix.**
  - Generate propagation from the same data, and put it in the TLA+ module.
  - Exempt cells that are proved unreachable, and list them.
  - Model the tree, the holders and same-epoch timers.

### M10 — §5.1/§3.3: children outliving zsup

- Go's `Pdeathsig` belongs to the *forking thread*, not to the process
  (golang/go#27505, *from memory*). If the runtime retires that thread, the
  children are killed spuriously. Fix: spawn from one locked thread that never
  exits.
- Escalation exits while trees that were Failed(unkillable) or double-forked
  on macOS are still alive. That contradicts "children never outlive zsup".
  Fix: state it as a documented exception, and report those trees in the exit
  status.

### M11 — §1.3 path regions on platforms other than Linux

- APFS (the macOS default) and NTFS are case-insensitive.
  `/Users/x ∩ /users/x` = ∅ silently, for both K5 and K6.
- Windows has drive letters, UNC paths, 8.3 short names and junctions.
- Symlinks are checked at `Compile` and used at exec time, which leaves a
  TOCTOU window between the two.
- **Fix.**
  - Canonicalize the paths on the target host, in the exec-helper.
  - Compare with the case rules of the filesystem.
  - Open paths through an `openat2(RESOLVE_NO_SYMLINKS)` equivalent.

## LOW

- **L1 §4.1.**
  - Rise vs Exit in the same epoch: the outcome depends on whether the reaper
    or the socket reader is read first. Drain the channel before applying
    Exit.
  - `NOTIFY_SOCKET` is inherited by grandchildren, so a nested daemon's
    READY=1 is taken as a premature Rise. Unset it in the exec-helper, or
    document it.
  - When a grandchild holds the fd-channel pipe, "closed without ready"
    cannot be detected, and StartTO is the only remaining bound.
- **L2 §8, delegating a cgroup to a principal's zsup.**
  - The subtree may hold processes of other uids.
  - The principal gets `cgroup.kill` and `cgroup.freeze` over them.
  - Require uid(subtree) = principal.
- **L3.** Draft 3 has not absorbed the user's decisions:
  - §1.4 R1 text, `ErrRankInRecipe` and §12 should go;
  - §6 still says "macOS: on disk";
  - §8 still offers K6 grants.
- **L4 §3.3.** The `Expire` of R_inst has no column.
