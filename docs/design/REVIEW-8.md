# REVIEW-8 of SUPERVISOR.md (draft 11)

Review pass, 2026-11-23. This review did not edit SUPERVISOR.md.

Scope: REVIEW-7 in full. SUPERVISOR.md draft 11: header (3), §3.1 (517-599),
§3.3 (678-835), §3.8 witnesses (1062, 1079), §5.1-5.3 (1142-1221), §7.1-7.2
(1244-1275), §8.1-8.3 (1281-1361), §9 (1365-1372), §10 chaos rows (1449-1462),
§12 (1508-1678). Line numbers are draft 11's. Six external pages were opened
(Part 4). Statements marked *(from memory)* were not checked against a source.

**Verdict.**

- **N7-N13 are closed** as REVIEW-7 traced them (Part 1). One residual for
  each of N7 and N8 is LOW.
- **No HIGH finding.** The HIGH trend is now 2, 2, 2, 1, 0, 0.
- **The model has converged.** No finding in this review touches the event
  table, the guard table, the settle rules, the cut, or the proof of
  Theorem G over the single-queue model.
- **The runtime's privilege architecture has not converged.** N9's answer
  (a root lane with three capabilities) opens three MEDIUM defects:
  - N14: the capability reduction is mostly nominal;
  - N15: the stated capability set cannot run §7.1's first step;
  - N16: a new inventory row, H12. p can steer the lane's `CAP_KILL` onto a
    process of O through pid reuse. This is an escalation route.
- **Recommendation (Part 2).** Split the lane into two processes:
  - an **unprivileged parser**, under a dedicated uid, holding only
    pre-opened fds;
  - a **root monitor** that never reads a byte p wrote.

  This keeps E2 and the caps, and it closes N14-N16 together.

---

## Part 1: replay of REVIEW-7's traces

| item | trace replayed on draft 11 | status |
|---|---|---|
| N7 | p's fork-and-exit loop: each grandchild is orphaned and goes to « the nearest still living ancestor subreaper » (prctl, Part 4). That is the lane, which sets the attribute before its first spawn (721). The core observes O by pidfd or `waitid(P_PID)` (1149). Its orphan source sees none of p's processes while the lane lives (1150). | **Closed** (residual N17, LOW) |
| N8 (a) | The enqueue does not block; a full queue answers `ErrLaneBusy` at once (723). | **Closed** |
| N8 (b) | A cron job runs `svd restart A` every minute against a starved lane. Each request is answered `202` on enqueue, and the core holds nothing afterwards (724). The owner's `svd start C`, C ∈ O, is accepted. | **Closed** in the core (residual N20 c, LOW, on the client side) |
| N8 (c) | Snapshot reads are bounded by `lane.snapshot.retries` and validated (725). | **Closed** |
| N9 | Linux: the lane sits in `deleg/<uid>/lane` and is capped whatever its uid. FreeBSD: `loginclass:svd-<uid>` with the default `per` aggregates the lane and the trees (Part 4, confirmed). p cannot write the cgroup files. | **Closed as stated**, but the answer opens N14-N16 |
| N10 | `app` (terminal) requires `postgres`, which is in Esc. The plan is refused with `ErrExposedEscalates` (791). The R_inst route is stated (793), and the plain statement is given (795). I also checked the want and order variant (T requires D, D wants C, p crashes C). It does not escalate: each crash puts C in Backoff, which is settled (REVIEW-2 N4), so D is released. | **Closed** |
| N11 | The lemma is restated over the O projection, with I1, T1 and T2 « up to the snapshot lag » (522). | **Closed** |
| N12 | The new fixture (1062): B crashes, so \|R_inst\| = max. C is in StopWait(dep) in one copy and in Stopping in the other. C exits `fail`: rule 4 charges it in StopWait and overflows, while Stopping records `sig`. The two copies diverge on `mode`. The witness is red for the stated reason. | **Closed** |
| N13 | `swAcrossCut` becomes a `Compile` test (1079, 1450). | **Closed** |

---

## Part 2: the root lane (N9's answer)

### Is it a new surface?

**Partly.** It is not new in kind. Through draft 9 the **core** parsed p's API
as full root, so a root parser of p's input has existed since draft 1. Draft
11 is strictly smaller than draft 9.

It is new in three specific ways.

### N14 (MEDIUM): « reduced capabilities » leaves the lane root-equivalent

720 drops every capability except `CAP_SETUID`, `CAP_SETGID` and `CAP_KILL`,
and calls the residual « a privilege escalation for p ». The text implies that
the reduction bounds that escalation. It does not, for three reasons.

- **uid 0 still owns every root-owned file.** Without `CAP_DAC_OVERRIDE`, the
  DAC owner check still matches uid 0 against root-owned inodes, and `chmod`
  by the owner needs no capability *(from memory)*. Examples:
  - `/etc` and its files (0755 and 0644), and `/usr/bin`;
  - every cgroup v2 interface file, the core's own `cgroup.freeze` included.

  So a lane under p's control writes the host and can freeze the core.
- **`CAP_SETUID` is not limited to p's uid.** A lane under p's control can
  `setresuid` to the uid of any O service. A1 then no longer holds: it can
  ptrace or signal that service.
- **FreeBSD has no Linux capability sets.** The FreeBSD lane is full root,
  and 720 does not say so.

**Failure scenario.** A logic defect in the lane's API decoder lets p choose a
cgroup path or a uid, with no memory corruption needed. The lane then freezes
the core's cgroup, or runs as postgres's uid and kills it. Either way the
instance reaches Quiescing(1) or stops.

### N15 (MEDIUM): the stated capability set cannot run §7.1

- The exec-helper is forked by the lane (721) and inherits its bounding set.
- Its Linux order begins with « mount ns » (1250).
- unix man-pages 6.19 unshare(2): « Use of CLONE_NEWNS requires the
  CAP_SYS_ADMIN capability. » (Part 4).
- The lane removes `CAP_SYS_ADMIN` from its bounding set (720), so no granted
  service can get a mount namespace. K6 `read_only` « paths remounted
  read-only » (373) cannot be applied.
- The error is EPERM from `unshare`. It is not in the `perm` rows of the O4
  table (589), so it is classified `fail`, which is **charged**.

**Failure scenario.** Every granted service with a read-only region
crash-loops on R_p and reaches Failed(intensity). The way out is to add
`CAP_SYS_ADMIN` to the lane, which makes it root in all but name.

### N16 (MEDIUM): new inventory row H12, pid-addressed privileged signals

**Missing row.**

| # | input p controls | mechanism |
|---|---|---|
| H12 | when p's processes exit and are reaped, and how fast the pid counter advances, against svd's privileged actions addressed by pid | p's processes reap their own children at times p chooses, and p loads its own lane |

**Trace (Linux).**

1. A ∈ G is stopping, and the lane signals « the rest of the tree » (870). The
   draft does not specify the mechanism. The natural one is to read
   `cgroup.procs` and call `kill(pid)`.
2. p saturates its own `cpu.max`, so the lane is descheduled between the read
   and the kill.
3. Meanwhile:
   - x exits;
   - x's parent, a process of p's, reaps x at once;
   - p has cycled the pid counter, using fork-and-exit within `pids.max`, so
     that the next fork in the system takes x's pid. Often that fork is one of
     O's.
4. The lane's `kill(x, SIGTERM or SIGKILL)` succeeds under `CAP_KILL`.
5. A Ready main of O that dies of a signal records `fail` and charges R_inst.

The route is probabilistic, but p can retry it without bound across instance
windows: each `restart A` costs one R_p charge.

**Before draft 11.** A lane running as p would have received EPERM, so this is
new to N9's answer. Through draft 9 the same race existed in the core. Theorem
G's (b) item « its own events » (804) assumes that only A1 can signal C. svd's
own deputy is not covered.

**Fix: never signal a pid read from a list.**
- **Linux:**
  - `SIGKILL` of a tree goes through `cgroup.kill`.
  - A graceful signal goes to the main process through the pidfd that
    `clone3(CLONE_PIDFD)` returned.
  - The rest of the tree is signalled only while it is frozen: write
    `cgroup.freeze`, then enumerate, then `pidfd_open` each pid, then check
    its cgroup, then `pidfd_send_signal`, then thaw. A frozen process cannot
    reap its children.
  - A seccomp filter forbids `kill` and `tgkill` in the signalling process.
- **FreeBSD:** `PROC_REAP_KILL` with `REAPER_KILL_SUBTREE` (procctl(2),
  Part 4). This is in the kernel and has no race.
- **Inventory:** add H12, and add a pid-cycling chaos run.

### Recommendation: privilege separation, so that no root process parses p's bytes

This is the OpenSSH monitor pattern, with the Capsicum idea of pre-opened
descriptors. There are two processes per delegated principal, both in p's
container (`deleg/<uid>/` or `loginclass:svd-<uid>`), so both are capped as
the lane is today.

**M_p, the monitor.**
- Runs as root. It is forked by the core and is a subreaper.
- It is the **parent of every tree** of grant(p). It holds `CAP_SYS_ADMIN`
  for the helper's mount namespace, and it calls `setloginclass` on FreeBSD.
- Its only input is **fixed-size records** from L_p:
  - `spawn(service index, epoch)`;
  - `signal(epoch, sig ∈ plan)`;
  - `kill(epoch)`;
  - `reaped-ack(epoch)`.
- It validates each record against its own copy of the plan, which comes from
  the core, never from L_p. It rate-limits spawns per service to the plan's
  intensity.
- It never reads a byte that p wrote. It signals only through pidfds,
  `cgroup.kill` or freeze, so N16 is closed.

**L_p, the lane.**
- Runs under a dedicated uid `svd-l-<uid>`. That uid is not p, not 0, and not
  the uid of any O service. No-new-privileges is set.
- It holds no capability. Its seccomp filter has no `kill`, `ptrace` or
  `pidfd_open`. Landlock (Linux), or `cap_enter` (Capsicum) on FreeBSD,
  applies once it holds its descriptors.
- It receives every descriptor pre-opened:
  - the listener, from the core;
  - the control socket;
  - the socket to M_p;
  - the log read ends and the notify sockets, from M_p;
  - its snapshot memfd, sealed with `F_SEAL_SHRINK` so that the core cannot
    take SIGBUS *(from memory)*.
- It runs the step loop and parses all of p's input. This is the lane of
  today without its privileges.
- **Readers** get an L_p only.

**Why E2 holds.** p cannot ptrace, stop or forge L_p, because L_p's uid is not
p's (A1 applies, and its probe must cover `svd-l-<uid>`). Every cgroup file is
root-owned and written by M_p.

**What a compromised L_p can do.** It can send valid records for grant(p)
only. That is confined to G and capped.

**Cost.**
- One more process per principal.
- The O1 observe-drain-reap order crosses the two processes. M_p peeks with
  `WNOWAIT`, notifies L_p, L_p drains, and M_p reaps.

**It also closes the lane-death window (N17).** The trees' parent is M_p, not
the process that parses p's input.

**Weaker alternative.** Keep one root lane, but add:
- seccomp argument filters, with `setresuid` only to p and no `kill(2)`;
- `pidfd_send_signal` only on pidfds from `clone3`;
- Landlock, so that it cannot write outside `deleg/<uid>/`.

N15 still forces `CAP_SYS_ADMIN` into that process, so this review does not
recommend it.

---

## Part 3: H10, H11, and is the list complete?

**H10 is confined on Linux and FreeBSD while the lane lives.** Two notes:

- A p process may set `PR_SET_CHILD_SUBREAPER` itself. It is then the nearest
  subreaper for its own subtree. That is harmless, at p's cost. But the
  assertion of the N7 chaos test, « its parent pid is the lane's » (1460),
  becomes false. Keep only the core's counter assertion.
- **N17 (LOW): the lane-death window.**
  - 728 bounds the core's reaping at the lane's death by « at most
    `budget.pids` ». `pids.max` bounds the **population**, not the count.
  - Between the lane's exit and `cgroup.kill`, p's surviving processes may
    fork and exit at will, and every orphan goes to the core.
  - The real bound is p's fork rate × the core's detection latency.
  - Fix: the core writes `cgroup.kill` on control-fd hang-up, before any
    reaping, and the text states the real bound. Part 2's M_p removes the
    window.

**N18 (LOW): reaping gaps in the core.**
- (a) The orphan source reaps any P_ALL zombie that is « not the main process
  of a live epoch » (1150). **A lane is the core's child and is not an
  epoch.** The orphan source can reap a dead lane before the lane's MainExit
  is handled (723 relies on that MainExit). The exclusion set must be « every
  child the core tracks ».
- (b) FreeBSD, procctl(2): on a reaper's exit, its descendants are
  « reassigned to the reaper process's reaper ».
  - The core is a reaper on FreeBSD (1158).
  - So at a lane's death, p's processes go to the **core** even when svd is
    not PID 1. 728 says « or to the host init ».
  - The orphan source exists only « as PID 1 » (1150), so these zombies are
    never reaped. They count against p's `maxproc` across lane restarts.
  - Fix: run the orphan source whenever the core is a reaper of any kind.

**H11 is cut.**

**The list is not complete.**
- **H12 (N16)** is missing.
- **N19 (LOW): one more input to add.** p writes bytes that the core relays
  to owner clients: `STATUS=` text, log lines, unit-reported strings, through
  the snapshot and `svd logs A`.
  - It does not touch obs_O.
  - It is a confused-deputy channel to the owner's terminal: escape
    sequences, and forged lines that look like svd's own.
  - Fix: the CLI escapes non-printable bytes, and every relayed line carries
    its provenance, as journalctl escapes *(from memory)*.

**Other candidates, checked and found sound:**
- p `chmod`s its own listener socket, which is p-owned after the `chown`. The
  peer-credential check closes a connection from any other principal (730).
- Kernel memory of p's sockets is charged to p's memcg *(from memory)*.
- Bytes p sends to C's listener are A4.

---

## Part 4: bibliography, opened

| claim (draft 11) | source opened | result |
|---|---|---|
| `prctl(PR_SET_CHILD_SUBREAPER, 1)` (721) | https://man7.org/linux/man-pages/man2/PR_SET_CHILD_SUBREAPER.2const.html (man-pages 6.19, 2026-02-08) | **Confirmed.** « If set is nonzero, set the "child subreaper" attribute ». An orphan goes « to the nearest still living ancestor subreaper ». The attribute is « not inherited by children created by fork(2) and clone(2) » and is preserved across execve, so `svd lane` must set it itself, as 721 does. Since Linux 3.4. No privilege is required. **The page says nothing about the subreaper's own death.** Reparenting to the next living ancestor subreaper, else the namespace's init, is *(from memory: kernel `find_new_reaper`)*. 728 is therefore right only if the core is not itself a subreaper, which the draft never makes it. |
| `procctl(P_PID, getpid(), PROC_REAP_ACQUIRE)` (721) | https://man.freebsd.org/cgi/man.cgi?query=procctl&sektion=2 (FreeBSD 15.1) | **Confirmed**, with two corrections. EPERM if « idtype … is not equal to P_PID, or id is not equal to the pid of the calling process ». EBUSY if already a reaper. (1) The call applies « for **future** children »: existing children keep their reaper, so « before its first spawn » is required, not optional. (2) On the reaper's exit, its processes are « reassigned to the reaper process's reaper », not to init (N18 b). `PROC_REAP_KILL` with `REAPER_KILL_SUBTREE` exists (N16 fix). |
| rctl subject `loginclass:` aggregates the lane and the trees (738) | https://man.freebsd.org/cgi/man.cgi?query=rctl&sektion=8 (15.1 set) | **Confirmed.** The subjects are process, user, loginclass and jail. « per … defaults to the value of subject ». The page's own example says « the sum … by all processes of all users belonging to that login class ». The `pcpu`, `memoryuse` and `maxproc` definitions and the deny/throttle notes match 738. |
| `setloginclass(2)` needs root, and the class is inherited (738, 1558) | https://man.freebsd.org/cgi/man.cgi?query=setloginclass&sektion=2 (15.1 set) | **Confirmed:** « restricted to the super-user ». « Processes inherit login class from their parents. » **Not in the page:** that the class survives `execve` and the helper's `setuid`. This stays unverified, and the N9 test at 1462 checks it on a real tree, which is the right witness. |
| a mount namespace from the helper (1250) | https://man7.org/linux/man-pages/man2/unshare.2.html (6.19) | « Use of CLONE_NEWNS requires the CAP_SYS_ADMIN capability. » This contradicts 720's set (N15). |

**§12 (1556, 1558) can move** the subreaper and procctl rows, rctl `loginclass`
aggregation, and `setloginclass`'s privilege to « opened ». The rows that stay
open are:
- `execve` inheritance of the login class;
- subreaper-death reparenting;
- GENERIC's `RACCT_DEFAULT_TO_DISABLED`.

---

## Part 5: is the platform table (746-751) honest and consistent?

**Honest on refusals.** The macOS row is vacuous and says so. The Windows row
is « not established ». Delegation is stated to be Linux-only (753).

**N20 (LOW): inconsistencies with the text.**
- (a) **Linux row, « N3 closed ».** That holds only once N15 is resolved.
  Until then, every granted service with a K6 read-only region fails to
  spawn.
- (b) **FreeBSD row.**
  - It should say that the lane is full root (N14).
  - With one reaper per lane, per-epoch tree tracking and TreeEmpty need the
    reaper's subtree notion. §5.1's table entry « `PROC_REAP_ACQUIRE` » (1158)
    is written for one reaper per tree *(subtree field from memory)*.
- (c) **The lane protocol.**
  - `/v1/changes` is « the audit log » (1349), but 724 uses
    `/v1/changes/<id>` for ticket outcomes.
  - The ticket has no deadline that the core can answer: the deadline lives in
    a starved lane. A waiting `svd start A` therefore polls forever, and the
    owner's own pollers pile up on owner-class slots at a rate that p
    influences.
  - Fix: encode the deadline in the ticket, and answer « expired » from the
    core without keeping state.
- (d) **Readers.** A reader lane has no trees but runs as root with
  `CAP_SETUID` and `CAP_KILL` (720 applies to every lane). It should hold no
  capability.
- (e) **Modes.** The table should name its scope:
  - system instance in modes C and B only, since mode U has no lanes (729);
  - Linux under s6, runit or OpenRC without a writable cgroup tracks trees by
    process group (1197), so delegation is refused there.

---

## Summary

| item | result |
|---|---|
| 1 | N7-N13 **closed**, with the traces replayed. Residuals: N17 and N18 (LOW), N20 c (LOW). |
| 2 | The root lane is **not new in kind** (the core parsed p's input as root through draft 9). But its reduction is nominal (**N14**, MEDIUM), its capability set cannot run §7.1 (**N15**, MEDIUM), and `CAP_KILL` creates **H12** (**N16**, MEDIUM, an escalation by pid reuse). **Recommended:** privilege separation, with an unprivileged lane under a dedicated uid and pre-opened fds, plus a root monitor that reads fixed-size records only. |
| 3 | H10 is confined and H11 is cut. The list misses **H12** (N16), and the owner-relay channel (N19, LOW) should be added. |
| 4 | The subreaper and `PROC_REAP_ACQUIRE` are confirmed, with corrections: « future children » only; descendants go to the reaper's reaper. `loginclass` aggregation and `setloginclass`'s root requirement are confirmed. `execve` inheritance and subreaper-death reparenting are not in the pages. `CLONE_NEWNS` needs `CAP_SYS_ADMIN`. |
| 5 | The table is honest on refusals. N20: five consistency gaps. |
| convergence | **The model has converged**, with HIGH findings at 2, 2, 2, 1, 0, 0. **The runtime privilege layer has not:** N9's answer produced three MEDIUM findings. |
| P1 | **Yes, after a draft 12 that** adopts the monitor/lane split (or states an equivalent that resolves N14-N16), adds H12, and fixes N17 and N18 a. P1's work on `Compile`, the FSM and the core does not depend on these and may start now. |
