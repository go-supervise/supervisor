# SECURITY-1: threat model and attack review of the monitor/lane split (SUPERVISOR.md draft 12)

Security pass, 2026-11-23. This review did not edit SUPERVISOR.md. Line numbers
are draft 12's.

**Scope.** §3.3 H_OS-E5 (719-811), inventory H1-H13 (828-844), §4.1 (1147-1168),
§5.1 (1203-1219), §7.1-7.2 (1306-1335), §8.1 (1341-1361), §10 REVIEW-8 rows
(1531-1539), §12 (1641-1645), error list (433). REVIEW-8 in full.

**Adversary.** p is malicious, has code execution as the lane uid, and L_p's
sandbox (seccomp, Landlock, NNP) is assumed escaped to the lane uid's ambient
authority. p also has what the draft already grants it: the trees of grant(p)
running as uid p, and logins as uid p outside svd (H8).

**Evidence status.** There is no code, and this host is macOS, so **no attack
below was reproduced**. Each is *inferred* from the design text plus a primary
source that was opened (Part 4). Each names the test that would reproduce it.
Statements marked *(from memory)* were not checked.

**Verdict.**

- **The record channel is sound.** I found no route to root, to an owner uid,
  or to an object outside grant(p) through record parsing, record sequencing or
  fd passing as specified (Part 1).
- **The defects are around the channel**, in what M_p and its helper do with
  objects p can influence without sending a record: process attributes inherited
  from M_p, a p-owned directory, the freeze argument, the id-drop order, the
  seccomp filters, and the lane account. 2 HIGH, 6 MEDIUM, 5 LOW.
- **Delegation on Linux can be implemented after a draft 13** that fixes S1-S8.
  None needs a change of architecture; each is a rule to add or a line to
  correct. Work on `Compile`, the FSM, the core and the record decoder does not
  depend on them.

| # | sev | class | one line |
|---|---|---|---|
| S1 | HIGH | c, a | `oom_score_adj` −1000 is inherited by every granted tree, and p can keep it |
| S2 | HIGH | a, c | root creates, binds in and cleans a directory owned by p (notify socket) |
| S3 | MEDIUM | b | the freeze argument of the signalling rule is false; the cgroup check is the only barrier and is unspecified |
| S4 | MEDIUM | TCB | the seccomp filters cannot run in a Go process as stated, and do not cover what they claim |
| S5 | MEDIUM | a | « ids → caps » needs keep-caps: a uid-p process holding `CAP_SYS_ADMIN` exists |
| S6 | MEDIUM | d | the lane account: gid and groups unstated, host accounts unchecked, uniqueness stated in one place of three |
| S7 | MEDIUM | c | M_p's send side and its signal records are unbounded; « cannot block » contradicts the freeze wait |
| S8 | MEDIUM | a, b | the helper's mounts are by path, in regions p can rename |
| S9 | LOW | d | the core-to-lane snapshot: no mechanism makes it read-only |
| S10 | LOW | b | TREE_EMPTY from `populated` does not mean reaped; `trees/` stays frozen after a fault |
| S11 | LOW | c | `MS_PRIVATE` pins every host file system for the life of a granted service |
| S12 | LOW | a | descriptor hygiene across the helper's `execve` is unstated |
| S13 | LOW | b | the record carries no plan generation; `clone3` errors other than `EPERM` are charged |

---

## Part 1: the record channel (what holds)

Checked against 734-755, with a compromised L_p choosing every byte and the
timing of every datagram.

| attempt | result |
|---|---|
| Out-of-range `service`, `kind`, `signal`, non-zero reserved bytes | Fault. Every field is compared with M_p's own state before any use. The index is range-checked before it indexes the plan. |
| Length games (0, 63, 65, 64 KiB) | Fault. A 65-byte buffer distinguishes « too long » even without `MSG_TRUNC`. A `recvmsg` return of 0 is both EOF and a zero-length record on `SOCK_SEQPACKET`; draft 13 should say that both are the hang-up path. |
| `SCM_RIGHTS` to M_p | With no control buffer the kernel sets `MSG_CTRUNC` and installs nothing *(from memory: the rights are released with the skb)*. Fault. In-flight fds are accounted to the sender's uid. |
| Replay, reorder, epoch confusion | `seq` and `epoch` are M_p's, so a replay is a Fault and a stale epoch is a Refuse. An epoch can only name the current incarnation. |
| Steering a signal | The target is always an object M_p created (the `clone3` pidfd, the epoch cgroup fd). No field of the record is a pid, a path or a uid. |
| Receiving a privileged fd | fds flow from M_p to L_p only. Draft 13 should list them (log, status, notify) and state that a pidfd or a cgroup fd is never sent. |

Three gaps in the text, none exploitable against O:

- **`seq` after a Refuse** is not stated. If a refused record does not consume
  its `seq`, an honest L_p faults on its next record, and one stale epoch (which
  p can provoke by timing an exit) kills every tree of grant(p). State: every
  well-formed record consumes a `seq`, Act or Refuse.
- **A panic in M_p is a monitor death**, which is the orphan window. The
  decoder must be panic-free by construction, not by `recover`.
- **Never log L_p's bytes** from M_p or the core. The `FAULT` code is fixed
  (751); the same must hold for every diagnostic.

---

## Part 2: attacks

### S1 (HIGH, classes c and a): −1000 propagates to p

**Design line.** 778 and 1782: M_p's `oom_score_adj` is −1000. 730: M_p forks
every exec-helper. §7.1 (1310) lists the helper's steps; none touches
`oom_score_adj`.

**Sources opened.** kernel/fork.c `copy_signal`: the child receives both
`oom_score_adj` and `oom_score_adj_min` from the parent. fs/proc/base.c
`__set_oom_adj`: a write below `oom_score_adj_min` needs `CAP_SYS_RESOURCE`,
and `oom_score_adj_min` is assigned only when the **writer** holds
`CAP_SYS_RESOURCE`. proc_pid_oom_score_adj(5): −1000 « is equivalent to
disabling OOM-killing entirely for that task ».

**Steps.**
1. M_p is at −1000. Whoever wrote that held `CAP_SYS_RESOURCE` (it is required
   to lower), so M_p's `oom_score_adj_min` is −1000 as well.
2. The helper, then the service's main and every descendant, inherit both
   values. With no reset, **every process of grant(p) is exempt from the OOM
   killer**, global and memcg.
3. If draft 13 adds a reset to 0 in the helper, it is not enough. M_p's set
   (727) has no `CAP_SYS_RESOURCE`, so the helper's write raises the value but
   leaves the minimum at −1000. After `execve` the process is dumpable and owns
   `/proc/self/oom_score_adj`, so p writes −1000 back.
4. p fills `budget.memory`. `memory.max` still holds, but the memcg killer's
   only candidate is L_p. On any **global** OOM, p's whole budget is untouchable
   and the kernel kills owner services instead. Each such death is `fail` and
   charges R_inst.

**Why it matters.** OOM immunity is a privilege that needs a capability; E3
(717) says a granted service holds none. And it moves a host-level victim
choice from p to O, which is an input to obs_O outside the inventory.

**Smallest fix.** Drop −1000. M_p has a constant, small footprint, so the memcg
killer ranks it below every tree and below L_p; its death is already bounded
(777-781) and is p's cost. If the owner wants −1000 anyway: add
`CAP_SYS_RESOURCE` to the helper's pre-drop set and have the helper write the
plan's value (default 0) **before** the id drop, so that both the value and the
minimum are reset.

**Test.** `TestGrantedTreeOomAdj`: in a granted service, read
`/proc/self/oom_score_adj` (expect the plan's value) and write −1 (expect
`EACCES`).

### S2 (HIGH, classes a and c): root works in a directory p owns

**Design line.** 1158: the `sd_notify` socket lives « in a per-epoch directory:
0700, owned by the service's run_as uid, inside the instance runtime
directory ». 728: M_p creates the notify socket. Nothing says who removes the
directory.

**Steps.**
- *Creation.* M_p makes the directory, gives it to p, and binds in it by path.
  If the `chown` precedes the `bind`, a uid-p login plants the name first and
  the spawn fails; that is only p's cost, but it shows the order is
  load-bearing and unstated.
- *Removal.* At epoch end a root process removes a tree p controls. p creates a
  subdirectory, and swaps it for a symbolic link to an owner directory between
  the walk's `readdir` and its descent. uid 0 owns `/etc`, so the unlink needs
  no capability (REVIEW-8 N14's own argument). **Arbitrary file deletion as
  root** if the removal is a path-based recursive walk.
- *Space.* The directory is p-writable and sits on the tmpfs that holds the
  core's sockets. Its pages are charged to p's memcg, but the mount's size and
  inode limits are shared, so p fills `/run` within `budget.memory` and the
  core's next `bind` or `mkdir` gets `ENOSPC`.

**Smallest fix.** Use §8.1's listener recipe, which the draft already has: the
directory is **root-owned, 0711**; M_p binds, then `fchownat`s the socket path
to p with mode 0600 and `lstat`-verifies it. No directory is ever owned by p,
so there is no walk and no quota question. Removal is one `unlinkat` of a known
name and one `rmdir`, relative to a directory fd.

**Test.** `TestNotifyDirNotOwnedByGrantee`; a chaos run that plants links in
every svd-created directory and asserts that nothing outside the runtime
directory changes.

### S3 (MEDIUM, class b): the freeze does not pin the pids

**Design line.** 766: « no pid in the list can be freed and reused before its
pidfd is held », because a frozen process cannot reap.

**Sources opened.** Kernel cgroup-v2 document, `cgroup.freeze`: frozen
processes can be killed by a fatal signal, and can enter or leave the cgroup by
an explicit move or when the freeze races with `fork()`. pidfd_open(2): a pid
is guaranteed not to be recycled only if « the disposition of SIGCHLD has not
been explicitly set to SIG_IGN », `SA_NOCLDWAIT` was not used, and the zombie
was not reaped elsewhere.

**Steps.**
1. A tree process P sets `SIGCHLD` to `SIG_IGN` and forks x. Both are in the
   epoch cgroup.
2. M_p freezes the cgroup and reads `cgroup.procs`, which lists x.
3. A uid-p **login** (H8, outside the cgroup, not frozen) sends `SIGKILL` to x.
   x dies. Its parent ignores `SIGCHLD`, so the kernel releases x at once; no
   frozen process had to reap.
4. The same login cycles the pid counter until an O process takes x's number.
5. M_p's `pidfd_open(x)` now names an O process, and M_p holds `CAP_KILL`.

**What stops it.** Only the next step of 766, « check through the pidfd that
the process is in that epoch cgroup ». The draft presents it as a second check;
it is the **only** one, and its mechanism is not given. pidfd_open(2) lists no
cgroup query.

**Smallest fix.** Either of:
- **Signal with p's credentials** (preferred). M_p forks a short-lived child
  that drops to uid p with empty capability sets and sends the signals. The
  kernel's own uid rule (A1) then refuses any O process, whatever happens to
  the pid. M_p no longer needs `CAP_KILL` for the tree, and H12 is closed by
  the kernel, not by an argument.
- Keep the walk, state that the membership check is load-bearing, and specify
  it: read `/proc/<pid>/cgroup`, compare with the epoch cgroup, then confirm
  that the pidfd still names a live process before signalling *(the
  `PIDFD_GET_INFO` cgroup id is from memory)*. Correct 766's sentence.

**Test.** Extend the pid-cycling chaos run with an external uid-p killer and
`SIGCHLD` ignored in the tree; assert no O sentinel receives a signal.

### S4 (MEDIUM, TCB): the seccomp filters as stated

**Design line.** 727, 767: L_p and M_p deny `kill`, `tkill`, `tgkill`; « so a
pid can never be signalled ».

- **Go's runtime needs `tgkill`.** runtime/os_linux.go (opened): `signalM`
  calls `tgkill(getpid(), int(mp.procid), sig)`. Asynchronous preemption goes
  through it. A filter that kills on `tgkill` ends M_p within its first
  milliseconds, which is a monitor death. A filter that returns an error
  silently disables preemption, and a stop-the-world can then wait on a loop.
  The implementer will have to relax the filter, and an unspecified relaxation
  is where a hole appears.
- **The claim is false as written.** `rt_sigqueueinfo` and `rt_tgsigqueueinfo`
  signal by pid and are not in the list. In M_p, `pidfd_open` on a number is
  allowed by necessity (S3).
- **L_p's filter is a denylist.** It leaves user namespaces, `io_uring`, `bpf`,
  `keyctl`, `perf_event_open` and the rest of the kernel's surface to a
  compromised parser *(list from memory)*.

**Smallest fix.** Allow `tgkill` only when its first argument equals the
process's own pid (an argument filter installed after start); deny `kill`,
`tkill`, `rt_sigqueueinfo`, `rt_tgsigqueueinfo`. Make **both** filters
allowlists generated from a recorded trace of the test suite, and state M_p's
filter as a lint against an accidental call, not as the H12 proof.

### S5 (MEDIUM, class a): a uid-p process with `CAP_SYS_ADMIN`

**Design line.** 1310: « mount ns → NNP → Landlock → ids → caps → seccomp ».
727: `CAP_SETPCAP` is for « the helper's id and bounding-set drop ».

A change from uid 0 to uid p clears the permitted and effective sets, so the
bounding-set drop that follows has no `CAP_SETPCAP` left *(capabilities(7), from
memory)*. To run the stated order the helper must set keep-caps. There is then a
window in which a process whose real uid is p holds `CAP_SYS_ADMIN`,
`CAP_SETUID` and `CAP_KILL`. Any uid-p process may signal it (stop it there
indefinitely); only the non-dumpable flag stands between p and its memory.

**Smallest fix.** State the order as: `setgroups` → `setresgid` → bounding-set
drop → `setresuid` (all three ids) → **verify** all five sets are empty and
that `setuid(0)` fails → seccomp → `execve`. No keep-caps, no
`PR_SET_DUMPABLE`. Add the order to `TestMonitorCapSet`.

### S6 (MEDIUM, class d): the lane account

**Design line.** 726, 433, 1782.

- **gid and groups are not stated.** `lane.uids` names uids only. A lane that
  keeps gid 0 reads every root-group file of mode 0640. State a dedicated gid,
  an empty supplementary list, and the same refusals as for the uid.
- **Host accounts are not checked.** `ErrLaneAccount` compares the uid with the
  plan's principals and services. A range that overlaps a login account, a
  daemon, `nobody`, a dynamic-user range or a container's sub-uid range gives a
  compromised L_p that account's files and processes. **[Host+13]:** refuse a
  lane uid that resolves to an account not reserved for svd, or that any
  running process holds.
- **Uniqueness appears once.** 433 refuses a uid « shared with another lane »;
  726 and 1782 omit it. Two principals on one lane uid would let either parser
  signal and trace the other, which voids E2. Make the three lines agree.
- **Stability.** If uid X serves p in one load and q in the next, anything X
  still owns crosses principals. Bind the allocation to the principal and
  persist it, or prove that the lane uid can own nothing.
- **What the lane uid can own.** With the sandbox escaped: files in
  world-writable directories, IPC objects, and a set-user-ID file that hands
  the lane uid to p outside the sandbox. State that the lane uid has no
  writable path as a host check, not only as a Landlock rule.
- **L_p is dumpable** after its `execve`, so any process of the lane uid can
  trace it. That is only L_p itself while the uid is unique, which is one more
  reason uniqueness must be a load check.

### S7 (MEDIUM, class c): M_p's bounds are incomplete

**Design line.** 752, 755 (« cannot block »), 766 (« wait … for at most
`freeze.timeout` »).

- **Sends.** Each Refuse answers `NACK`. A compromised L_p floods stale records
  and never reads. Either M_p blocks in `send` and stops reaping, or its replies
  are dropped and the protocol desynchronises. State: sends are non-blocking,
  and a full buffer is a Fault.
- **Signal records have no rate.** Only `SPAWN` is limited. Each `SIGNAL_TREE`
  costs a freeze, up to `freeze.timeout` of waiting, and one `pidfd_open` per
  process. The decoder cannot block, but the action can, so the sentence at 755
  is false for the process. Give every kind a per-service count per window,
  from the plan, and make the freeze wait an event, not a wait.
- **Shared kernel locks** *(from memory, unverified)*. Freeze, kill, `mkdir`
  and `rmdir` of cgroups take a lock that the core's own cgroup operations
  take. M_p runs under p's `cpu.max`; if it can be throttled while it holds
  that lock, a record flood delays owner spawns by up to a quota period, which
  exceeds Λ (820). Add a record flood under a saturated `cpu.max` to the N2/N3
  chaos test and measure the core's `mkdir` latency.

### S8 (MEDIUM, classes a and b): mounts by path

**Design line.** 1310, 1324-1326. `openat2` and by-fd are stated for Landlock.
The mount step is not: « applies K6 `read_only` ».

The helper is root with `CAP_SYS_ADMIN`, and `mount(2)` follows links. If a
read-only region lies under a directory p can write, p renames it and leaves a
link in its place between `Compile` and the mount. The remount lands on the
link's target, and the intended region stays writable: E3 (717) is not
enforced for that service. The damage stays inside the new namespace.

**Smallest fix.** Mount what was opened: resolve with `openat2(RESOLVE_NO_SYMLINKS
| RESOLVE_NO_MAGICLINKS)`, then bind and remount through the descriptor
*(`open_tree` / `mount_setattr`, or the `/proc/self/fd` path; from memory)*.
A failure to make the tree private must be fatal before any other mount.

### LOW

- **S9.** 720 says the core's snapshot is « read-only in the lane *(mechanism
  per OS unverified)* ». 773's seals protect the other direction. If a lane
  receives a writable descriptor of a snapshot that other lanes read, p rewrites
  O's state as seen by q. State one memfd per lane, or a seal against future
  writes applied after the core's own mapping *(from memory)*. Hole punching in
  the lane's own snapshot moves at most one snapshot of memory to the core's
  charge; state it as a constant.
- **S10.** cgroups(7) counts only non-zombie processes for `populated` and for
  removal. `TREE_EMPTY` therefore does not imply « reaped » (752's `SPAWN`
  gate says both; keep both). After a Fault, `trees/` is left frozen (751);
  `svd lane restart p` must thaw it or every new helper is born frozen.
- **S11.** mount_namespaces(7): a private mount receives no event, so a host
  `umount` leaves the copy alive in the service's namespace and the device
  busy. The page's own example for this purpose is `MS_SLAVE | MS_REC`. Use it.
- **S12.** clone(2) and pidfd_open(2) set close-on-exec on pidfds. State the
  rule for every descriptor of M_p (core socket, lane socket, `trees/`, cgroup
  fds), and have the helper close everything not named by the plan before
  `execve`.
- **S13.** A plan reload that renumbers grant(p) makes an honest L_p name the
  wrong service. Put a plan generation in the reserved bytes; a mismatch is a
  Refuse. clone(2) lists `EACCES`, `EBUSY` (a domain controller enabled in the
  target) and `EOPNOTSUPP` for `CLONE_INTO_CGROUP`; 577 classifies only
  `EPERM`, so the others would be charged. Epoch cgroups must stay leaves with
  an empty `cgroup.subtree_control`.

### Checked and found sound

- **p moving a process out of its epoch cgroup.** cgroups(7) and the kernel
  document: the writer needs write permission on `cgroup.procs` of the
  destination **and** of the nearest common ancestor. For svd's subtree and p's
  own delegated user cgroup that ancestor is root-owned. 766's « p cannot move
  a process » holds.
- **p re-mounting a read-only region** from a user namespace.
  mount_namespaces(7): `MS_RDONLY` is locked when a mount reaches a less
  privileged namespace.
- **`cgroup.kill` against forks and moves.** The kernel document states both.
- **A granted tree spawning through another manager** (the user bus, `at`,
  `ssh localhost`) leaves p's caps. 810 already assigns it to A2(host).

---

## Part 3: the TCB

**Is M_p small enough to audit?** The decoder, yes: one function over
`[64]byte`, about a hundred lines. **The TCB as listed at 757 is not small.**
It contains the spawn path and « the helper's setup up to `execve` », which
runs as root with `CAP_SYS_ADMIN`, resolves host paths p can rename, mounts,
decrypts credentials, and drops ids. S1, S2, S5 and S8 are all there, and none
is in the decoder. The Go runtime and `syscall` are in the TCB too and 757 does
not say so. Draft 13 should measure and report the helper separately, and
order the audit: helper, then signalling, then decoder.

**What the record parser must guarantee.**

1. **Total.** `decode([64]byte, state) → (Act | Refuse | Fault, state')`. No
   panic, no fourth outcome. A stateful fuzz target compares it with a
   reference model written from the table at 736-746, over sequences.
2. **Pure, and separate from acting.** Decode touches no descriptor. `state'`
   differs from `state` only on Act and on the `seq` of a Refuse.
3. **No allocation.** `testing.AllocsPerRun` = 0 on the receive-decode-reply
   path, in addition to the fuzz target's heap assertion.
4. **No blocking**, on receive, reply or action (S7).
5. **Fixed offsets only.** No length, offset or count read from the record ever
   indexes anything.
6. **Reference check.** Every Act names an object from M_p's own table; the
   fuzz target asserts it.
7. **Silence.** No byte of a record reaches a log, an error string or the core.
8. **Corpus committed**, as 755 says, with the zero-length datagram and one
   record per `seq`/`epoch` boundary.

---

## Part 4: sources opened

| claim | source | result |
|---|---|---|
| `clone3` with `CLONE_NEWNS` needs `CAP_SYS_ADMIN` (1310, from memory) | https://man7.org/linux/man-pages/man2/clone.2.html (man-pages 6.19) | **Confirmed.** « Only a privileged process (CAP_SYS_ADMIN) can employ CLONE_NEWNS. » `EPERM` otherwise. Also: `CLONE_NEWNS` with `CLONE_FS` is not permitted. |
| kernel versions of the `clone3` flags (1644) | same page | **Confirmed.** `clone3` 5.3; `CLONE_PIDFD` 5.2, close-on-exec set; `CLONE_INTO_CGROUP` 5.7, descriptor opened `O_RDONLY` or `O_PATH`. **Added:** `EACCES`, `EBUSY`, `EOPNOTSUPP` (S13). |
| `pidfd_open` (803) | https://man7.org/linux/man-pages/man2/pidfd_open.2.html (6.19) | 5.3; close-on-exec set. **Correction to 766:** the no-recycling guarantee is conditional on `SIGCHLD` not ignored, no `SA_NOCLDWAIT`, and no reap elsewhere (S3). The page gives no cgroup query. |
| `pidfd_send_signal` (765) | https://man7.org/linux/man-pages/man2/pidfd_send_signal.2.html (6.19) | 5.1. `ESRCH` once the target « has terminated and been waited on ». The page calls the pidfd « a stable reference to a specific process ». It does **not** say the permission rule equals kill(2)'s; that stays from memory. |
| frozen processes cannot reap; `cgroup.kill` kills a frozen cgroup (766, 1644) | https://docs.kernel.org/admin-guide/cgroup-v2.html (first 100 000 characters read) | **Partly.** Frozen processes can be killed by a fatal signal, so kill-after-freeze works. **« Cannot reap » is not stated.** **Correction:** a frozen cgroup's membership can change by kill, by move and by a fork race (S3). |
| `cgroup.kill` (764) | same | **Confirmed.** Kills the cgroup and all descendants, « will deal with concurrent forks appropriately and is protected against migrations »; `EOPNOTSUPP` in a threaded cgroup. Its kernel version was not in the text read. |
| root-owned `cgroup.procs` confines p (766); ownership after delegation (716, from memory) | same, and https://man7.org/linux/man-pages/man7/cgroups.7.html (6.19) | **Confirmed.** Common-ancestor rule; « The delegater should not change the ownership of any of the controller interfaces files. » `nsdelegate` makes cgroup namespaces delegation boundaries. |
| no-internal-process rule (790, from memory) | cgroups(7) | **Confirmed.** « processes may reside only in leaf nodes », root excepted. |
| a zombie counts in `pids.max` until reaped (779, from memory) | both | **Not stated by either page.** `pids.current` is « the number of processes currently in the cgroup ». Stays unverified; `TestZombieCountsInPidsMax` is the witness. cgroups(7) does say `populated` and removal count non-zombie processes only (S10). |
| the memcg killer respects −1000 (778, from memory) | cgroup-v2 document, `memory.oom.group`; https://man7.org/linux/man-pages/man5/proc_pid_oom_score_adj.5.html (6.19) | **Confirmed.** « Tasks with the OOM protection (oom_score_adj set to -1000) are treated as an exception and are never killed. » −1000 « is equivalent to disabling OOM-killing entirely for that task ». |
| (new) the value is inherited | https://raw.githubusercontent.com/torvalds/linux/master/kernel/fork.c ; https://raw.githubusercontent.com/torvalds/linux/master/fs/proc/base.c | `copy_signal` copies `oom_score_adj` and `oom_score_adj_min`. `__set_oom_adj` requires `CAP_SYS_RESOURCE` below the minimum and assigns the minimum only for a capable writer (S1). |
| (new) Go needs `tgkill` | https://raw.githubusercontent.com/golang/go/master/src/runtime/os_linux.go | `signalM` calls `tgkill(getpid(), int(mp.procid), sig)` (S4). |
| mounts made private in the helper (1310) | https://man7.org/linux/man-pages/man7/mount_namespaces.7.html (6.19) | A replicated mount joins the peer group of the original, so the private-or-slave step is required. The page's example is `MS_SLAVE | MS_REC` (S11). Read-only is locked in a less privileged namespace. |

**Not opened, still from memory:** capabilities(7) (S5), seccomp(2), Landlock,
unix(7) on `SCM_RIGHTS` without a control buffer, the `PIDFD_GET_INFO` ioctl,
the scheduler's throttling point (S7), the memfd seals (S9), and every FreeBSD
page. FreeBSD was not attacked in this pass: M_p is full root there and the
platform row already refuses delegation until P4.

---

## Part 5: what draft 13 must contain

1. S1: no −1000, or a reset of value **and** minimum in the helper;
   `TestGrantedTreeOomAdj`.
2. S2: the notify directory is root-owned; socket path `chown`ed; removal by
   descriptor.
3. S3: signal the tree with p's credentials, or specify the membership check
   and correct 766.
4. S4: allowlist filters; `tgkill` restricted to self; 767's claim reworded.
5. S5: the id-drop order, with verification, no keep-caps.
6. S6: lane gid and groups; host-account refusal; uniqueness in 726 and 1782;
   stable allocation.
7. S7: non-blocking sends, a rate for every record kind, no wait in the action.
8. S8: mounts by descriptor.
9. S9-S13 as one line each.
10. §12: move the rows confirmed in Part 4 to « opened », and keep « cannot
    reap », zombie accounting and the `cgroup.kill` version as unverified.
