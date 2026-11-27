# Proposed amendment to SUPERVISOR.md: OpenBSD as an svd target (revision 2)

Status: PROPOSAL, for review. It edits no other file. The line numbers of SUPERVISOR.md refer to
`origin/main` `2346a907841ac403ee785734ce309bf88f2167fa`.

This revision answers the review of revision 1 (verdict FIX, findings F1 to F10, plus the F4 design
section). It is written against **sdcreds v0.4.0**, tag `v0.4.0` = commit
`64d90ac433b30206e4080677b3e0fde79e46863e` (go-authn/sdcreds, release PRs #10 and #11). Every
`file:line` below is at that tag, from a detached checkout of it. OpenBSD kernel behaviour is cited
only where the review opened the source (openbsd/src `958c7cbfd80cf0fbc1871d092889ab121c6d5851`,
`kern_sig.c` `cansignal()` and `killpg1()`); nothing here was executed on OpenBSD by this author.

## 0. Decisions, not re-opened

- OpenBSD is **P5** and **owner-only**: no grants, no readers (SUPERVISOR.md l.1800 says
  « grants and readers »; both are refused).
- Credentials, and the service's identity, go through `sdcreds run` v0.4.0.
- uid 0, gid 0 and any supplementary group 0 are refused everywhere (`wheel` is refused by name
  too). svd's plan validation refuses first, with its own error; sdcreds refuses again
  (README.md:241-243, `delivery/delivery.go:168`).
- Questions S3 and S4 of revision 1 are closed by the three lines above and are removed.

## 1. Install requirements (new, stated plainly)

1. **svd runs as root.** `sdcreds run` refuses a non-root caller
   (`cmd/sdcreds/run.go:193`, `delivery/delivery.go:75,168`).
2. **One dedicated uid per service**, which owns no other process. sdcreds refuses a user that
   already has processes (`delivery/killuid_openbsd.go:22-24`, `delivery/mfs_openbsd.go:81`), and a
   second service for the same user is refused at once (README.md:263-266, 219). svd refuses two
   services with the same `run_as` at **plan time**, and refuses a service whose uid is the
   owner's login uid.
3. **A reserved killer uid, `_sdkill`, created once at install**:
   `useradd -s /sbin/nologin -d /var/empty -c "sdcreds killer" _sdkill`
   (README.md:254-256, `delivery/uidkill.go:58-65`). It must own no process and must be neither
   root nor a service uid (`delivery/uidkill.go:92-110`). **A delivery is refused to start without
   it** (README.md:256). The name is a package variable (`KillerUser`, `uidkill.go:65`).
4. `vm.swapencrypt.enable` must be 1 (`cmd/sdcreds/run_openbsd.go:43-64`; README.md:272-276). There
   is no override. This is a hard precondition of every P5 service that has credentials.
5. The `sdcreds` binary is root-owned along a trusted path (`run_openbsd.go:284-288`,
   `trustedSelf`).
6. Every filesystem on which a service uid can write is mounted `nosuid` (see F5 below). This is an
   operator duty today; whether svd can check it at plan time is open.
7. The killer lock `/var/run/sdcreds-killer.lock` (`delivery/uidkill.go:152-154`) and the per-uid
   lock `/var/run/sdcreds/.uid-<N>.lock` (README.md:263-265) live under `/var/run`, which `etc/rc`
   empties at boot.

## 2. §6 credentials and the launch route (answers F1, F2, F3)

Replace the revision-1 text, which had svd's helper link `delivery.Prepare`, by:

> **openbsd**: svd execs, as root,
> `sdcreds run --service S --user U [--class C] [--pass-env ...] [--pass-fd ...] -- argv`, and
> that process is the service's main pid. The credentials live on a per-service MFS mounted
> `nosuid,nodev,noexec` (`delivery/mfs_openbsd.go:192`), owned by the service uid, and are removed
> when the program ends, when sdcreds dies (a helper holds the lock and watches it), at the next
> start, and at reboot. The kill-on-release guarantee is the reviewed `sdcreds run` code; svd
> contains no second implementation of it.

Consequences svd must honour:

- **Environment and descriptors (F2).** The program gets an allow-list: `LANG`, `LC_*`, `TZ`,
  `TERM`, a fixed `PATH`, `CREDENTIALS_DIRECTORY` and descriptors 0-2 (README.md:221-223).
  v0.4.0 adds `--pass-env NAME` (repeatable, passed as is, not passed when unset) and
  `--pass-fd N` (repeatable, N >= 4, open and not close-on-exec)
  (`cmd/sdcreds/gate_unix.go:67,105-109,133,209,245,258`; flags at `cmd/sdcreds/run.go:311-312`).
  Both are refused for what sdcreds owns: `PATH`, `CREDENTIALS_DIRECTORY`, `__SDCREDS_*`,
  descriptors 0-3 (`gate_unix.go:234-239`). They exist on OpenBSD only (`run.go:32`). A notify-type
  service is started with `--pass-env NOTIFY_SOCKET --pass-env LISTEN_FDS --pass-env LISTEN_PID
  --pass-fd 4` and so on (README.md:227-230). `LISTEN_PID` is passed unchanged: it names the pid
  the caller expected, not the program's, so a service that checks it must not be given it.
  Still true: the plan's **umask is not applied**, the program runs with umask 077
  (README.md:283-284).
- **Mode U is gone (F3).** A service under the owner's login uid is refused at plan time: the
  owner's own processes make the uid busy, and a release would kill all of them. The
  owner-only mode of OpenBSD means « the owner may start and stop it », not « it runs as the
  owner ».
- **Not started by the helper (F1).** Revision 1 §2 and §3 (`delivery.Prepare`,
  `Config.SpawnHelper`, `svd-drop` ordering) are withdrawn for OpenBSD. §7.1.1 stays
  Linux-only; the OpenBSD launch sequence is `sdcreds run`'s own.

## 3. Signalling rule, SUPERVISOR.md l.776 and l.787 (answers F4)

The rule says a pid read from a list is never passed to `kill(2)`, and that the core's trees are
signalled « by a signaller with the target service's uid, never by root » (l.787).

sdcreds v0.4.0 removes the root pid-list fallback. The release is a **split-credential killer**:

- root starts a short-lived killer process, which drops to
  `setresuid(killer, target, killer)` in **one** call (`delivery/killuid_openbsd.go:31-45`), after
  which it sends only `kill(-1, SIGKILL)`, repeatedly, until the kernel reports nothing left
  (`killuid_openbsd.go:57-61`, `delivery/uidkill.go:18`). The kernel walks its own table; no pid
  crosses user space.
- Why the target cannot stop it (from `cansignal()`, as read by the review): the killer's real and
  saved uid is the reserved uid and the target's processes match neither, so a signal from the
  target fails with EPERM, while the killer reaches every process whose real or saved uid is the
  target's. That is the reason for the reserved uid.
- Root only reads the table, to decide whether another pass is needed, and reports
  `ErrUIDSurvives` after the 20 s budget (`uidkill.go:40,112-150`). The kill refuses uid 0 and a
  killer equal to the target (`uidkill.go:67-90`).

**Amendment to l.787, OpenBSD**: « the signaller is a process whose **effective** uid is the target
service's and whose **real and saved** uid is a dedicated uid that owns no process; never root, and
never a process whose real uid is the target's ». This keeps the intent of the rule (the target's
uid is the authority, a pid list is never signalled) and fixes the one thing the Linux wording
cannot say on OpenBSD.

Residuals, stated as they are:

1. All killers share the real uid, so concurrent releases could signal each other; v0.4.0
   serialises them with one machine-wide lock (`uidkill.go:152-154`, README.md:252-253).
2. A fork already in the kernel when a pass ends is found by the next pass; the loop is bounded by
   the 20 s budget, then `ErrUIDSurvives` (fail closed).
3. `PS_NOBROADCASTKILL` (excluded from `kill(-1)`): the review found no setter reachable by a
   non-root process, which is **unverified**. If one exists the process survives, the root table
   check sees it, and the next start is refused: fail-closed, but a denial of service.
4. Whether a process of the target can `ptrace` the killer was not examined (`sys_process.c` not
   opened).
5. SIGKILL to root's own unreaped killer child on timeout is the one signal by pid; it is a direct
   child, so its pid cannot have been reused.

## 4. Process scan and set-id survivors (answers F5, partly)

- **Answered by v0.4.0.** The scan reads the whole table (`kernProcAll`,
  `delivery/kinfo.go:68-70`) and returns every live process whose **real, effective or saved** uid
  is the target (`parseKinfoAll`, `kinfo.go:73-77`; used at `killuid_openbsd.go:155`), because the
  kernel has no selector by saved uid. Zombies are not counted (README.md:262-263).
- **Left open, with the reason.** A process that holds the target uid only through a set-id file run
  by another user (real uid v, effective and saved u, `PS_SUGID` set) is counted by the start check
  but is **not reached by the killer** (README.md:257-260). The release then reports a survivor and
  the next start is refused until the process is gone. It is fail-closed, and it is a denial of
  service that lasts until an operator steps in. The mitigation is the `nosuid` requirement of §1.6.
  It is not closed by v0.4.0 and cannot be by the killer's credentials alone.

## 5. Restarts (answers F6)

- v0.4.0: a start **waits up to 30 s** for the previous run of the same service to finish its
  teardown, instead of being refused. The helper holds the lock until the user's processes are
  killed and the mount is gone, which can take up to 26 s (README.md:216-219;
  `cmd/sdcreds/run_openbsd.go:39,129-134`, `delivery/lock_unix.go:71-73`). A run still alive after
  30 s is refused with 125.
- svd side: the restart path needs no extra wait of its own for the same service. The **start
  timeout of an OpenBSD service must exceed 30 s plus the service's own start time**, or svd will
  charge the wait as a start timeout. Two services sharing a uid are refused at plan time (§1.2).

## 6. Exit statuses (answers F7)

sdcreds' own endings are reserved (`cmd/sdcreds/run.go:44-52`, `cmd/sdcreds/releaseexit.go:8-10`,
README.md:202-214):

| Status | Meaning | svd class |
|---|---|---|
| 125 | refused, nothing started (bad flag, unknown or root user, unencrypted swap, a run that is alive) | permanent: do not retry |
| 126 | failed after it began (delivery, helper lost, the program could not be executed) | transient |
| 70 | the program ended but the release failed (processes or mount may be left) | release failure, never a plain exit |
| other | the program's own status; `128+n` for a signal | by the service's own rules |

Before v0.4.0 a refusal exited 2 and a failure 1; revision 1's reading of those is withdrawn.

**Known limit, stated honestly:** a program that itself exits 125, 126 or 70 cannot be told apart
from sdcreds' own endings (README.md:213 says so for 125 and 126; 70 is the sysexits `EX_SOFTWARE`
value and shares the same limit). svd must not run a service that uses those statuses on OpenBSD,
and the plan validation cannot see them.

## 7. What `sdcreds run` does and does not do (answers F8)

Revision 1 §1 and §3 claimed more than the decided path does. Corrected:

- `--class` applies only the **login.conf resource limits**, lowering only (README.md:280-284). It
  does not apply priority, umask or path. svd's K3 limits reach the service only through the class;
  without `--class` the limits are those root had.
- sdcreds has **no pledge and no unveil**. The `nnp`, landlock, seccomp and `execpromises` rows of
  revision 1 §1 have nothing to run them and are declared `exempt_on openbsd` without an opt-in.
  Opening that gap is a feature request on sdcreds, not on svd, and is **left open**.
- Signals are forwarded to the program only for TERM, INT, HUP, QUIT, USR1 and USR2
  (`cmd/sdcreds/run_openbsd.go:199`). A `stop.signal` outside that set is never delivered; the plan
  validation refuses it on OpenBSD.
- No mount namespace: while the program runs, the credentials are visible to anything that can
  reach the directory, and root can always read them (README.md:235-236). The path
  `/var/run/sdcreds/<service>` discloses service names to local users; the contents are 0400/0500
  owned by the service uid. Accepted residual.
- Memory: `mlockall` is unusable from a Go process on OpenBSD (README.md:287-293); protection is
  encrypted swap, `RLIMIT_CORE` 0 and `kern.nosuidcoredump`. A crash dump taken by an
  administrator can still contain the plaintext.
- No cgroups, no freeze, no tree accounting. The uid kill is the tree kill for **a service run by
  `sdcreds run`**. Whether a service **without credentials** also runs under `sdcreds run` (to get
  the same kill) is not settled by v0.4.0's documentation, and is **left open**.

## 8. SUPERVISOR.md rows to add (answers F9)

These are the edits to apply to SUPERVISOR.md when this amendment lands (not applied here):

| SUPERVISOR.md | Content | Add |
|---|---|---|
| l.30 | « Platforms: Linux, macOS, Windows, then FreeBSD. » | OpenBSD (P5, owner-only) |
| l.265 | POSIX path alphabet | OpenBSD |
| l.776, l.787 | signalling rule | the OpenBSD wording of §3 |
| l.821-825 | §3.3 platform-scope table | OpenBSD row: no delegation, no grants, no readers |
| l.1275-1280 | §5.1 tree-tracking table | OpenBSD row: uid kill via `sdcreds run`, no freeze |
| §5.3 | modes | rc.d glue |
| l.1346-1361 | §6 credentials | the text of §2 |
| l.1466-1483 | §7.2 path rules | `/var/run/sdcreds`, `/var/run/sdcreds-killer.lock` |
| l.1495-1498 | §8.1 runtime-dir table | OpenBSD row |
| l.1739-1743 | §11 phases | P5 |
| l.1800 | delivery decision | reads « grants **and readers** » |

`ErrIsolationUnavailable` is `{resource, os}` (SUPERVISOR.md l.426, l.815): the refusal is
`{resource: "cgroup", os: "openbsd"}`.

## 9. Known limits (the list a reader should keep)

1. A program that itself exits 125, 126 or 70 is ambiguous (§6).
2. **`p_svuid` offset 568 is derived from the header only.** `kinfoSvUIDOff = 560 + 2*4`
   (`delivery/kinfo.go:50`) comes from the declaration order in `sys/sys/sysctl.h` (the source
   comment says lines 483-486 of master), not from a measurement of a running kernel. The offsets
   past 144 are « summed by hand » (`kinfo.go:31-33`). The code reads the table at a fixed stride
   and checks each element against the uid asked (`kinfo.go:35-38`); whether those checks would
   catch a wrong 568 specifically was not established. A wrong offset would misjudge saved uids,
   so a process might be missed or wrongly counted. It needs a check on a real OpenBSD host.
3. A set-id survivor is not killed (§4).
4. `PS_NOBROADCASTKILL` and ptrace of the killer are unverified (§3).
5. Nothing in this document was executed on OpenBSD by its author.

## 10. CI (F13, F14 stand as revision 1 had them)

The security-relevant lane (killer cannot be signalled, no collateral, convergence under a fork
storm, concurrency, the set-id survivor pinned, a source gate that no `Kill` takes a pid other than
-1 outside the killer's timeout) runs only on **a self-hosted OpenBSD image we produce**. A
third-party image run as root is not acceptable for it.

## 11. Findings, one by one

| # | Finding | Status | Where |
|---|---|---|---|
| F1 | wrong credential route | **answered** | §2; `run.go:193` |
| F2 | env, fds, readiness stripped | **answered**; umask still not passed | §2; `gate_unix.go:67-258`, `run.go:311-312` |
| F3 | mode U, root and a dedicated uid | **answered** by requirement | §1.1-1.2, §2; `run.go:193`, `killuid_openbsd.go:22-24` |
| F4 | signalling rule vs root pid list | **answered**, residuals listed | §3; `uidkill.go`, `killuid_openbsd.go:31-61` |
| F5 | set-id survivor | scan **answered**; kill of a set-id survivor **open** | §4; `kinfo.go:73-77`, README.md:257-260 |
| F6 | restart races the old helper | **answered** (30 s wait) | §5; `run_openbsd.go:39,133` |
| F7 | ambiguous statuses | **answered**; 125/126/70 ambiguity remains | §6; `run.go:44-52` |
| F8 | draft claims | **answered** by correction; pledge/unveil **open** | §7 |
| F9 | missing SUPERVISOR.md rows | **answered** (listed, to be applied) | §8 |
| F10 | re-opened decisions, wheel | **answered** | §0 |
| F11 | no tree kill without credentials | **open** | §7 |
| F12 | swap, trustedSelf, error shape | **answered** | §1.4-1.5, §8 |
