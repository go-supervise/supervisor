# BIBLIO-A — Prior art: host service managers

Status: bibliography for the go-zpkg supervisor design. Read for ideas and
pitfalls, **never as a contract** (user decision 2026-11-23).
Written 2026-11-23.

## Provenance and trust levels

| Mark | Meaning |
|---|---|
| **[F]** | File read directly. The launchd man pages come from macOS 27.2.1 (build 26B131), rendered with `mandoc -T markdown`. `L<n>` is a line of that rendering. |
| **[W]** | Page fetched with WebFetch. The text passes through a summarising model, so quotes are near-verbatim, not byte-exact. Re-open the URL before quoting it in a normative document. |
| **[S]** | Source file at a tag, fetched raw. Cited by function name; line numbers were not captured. |
| **[U]** | UNVERIFIED. Seen only in search results, or the page was blocked. Do not rely on it. |

Versions current on 2026-11-23:
- systemd: latest tag `v262.2`. Man pages read at freedesktop `/latest/`; `transaction.c` read at `v257`.
- OpenRC: `0.64.1`. Guides read at `master`.
- FreeBSD: latest `release/15.1.0-p7`. Man pages read at manpath `FreeBSD 15.0-RELEASE`.
- macOS: 27.2.1.

---

## 1. systemd (system and user managers)

### 1.1 Config model and composition laws
- A unit file plus drop-ins. `.conf` files in `foo.service.d/` are merged in alphanumeric order after the main file. Files with different names are applied in lexicographic order regardless of their directory. **[W]** https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html
- Directory precedence is `/etc` > `/run` > `/usr/lib`. In prefix drop-ins (`foo-.service.d/`), an equally named file further down the hierarchy overrides. **[W]** same page.
- An empty assignment (`ExecStart=`) resets a list.
- **Lesson (inferred):** the merge is a left fold over an ordered sequence. It is not commutative, and the "reset" element makes it non-monotone: a later layer can remove what an earlier one added. There is no stated algebra; the semantics is "whatever the parser does in this order".

### 1.2 Dependency semantics
All **[W]**, from systemd.unit(5) unless noted.
- Requirement and ordering are orthogonal. "requirement dependencies do not influence the order in which services are started or stopped".
- `Requires=`:
  - Stop propagation happens "with or without specifying After=".
  - The unit is withheld when the required unit fails only if `After=` is also set. Without `After=`, the two start in parallel, and the failure of the required unit does not prevent the requiring one from starting.
- `Wants=`: a failure "has no impact on the validity of the transaction".
- `Requisite=`: fails immediately if the target is not already active.
- `BindsTo=`: stops the unit on an unexpected stop of the target.
- `PartOf=`: one-way propagation of stop and restart.
- `Upholds=`: restarts the listed units whenever they are inactive.
- `Conflicts=`: "does not imply an ordering dependency".
- `OnFailure=`, with `OnFailureJobMode=`.
- Targets add `After=` to their `Wants=`/`Requires=` automatically. `DefaultDependencies=no` removes only the non-essential implicit dependencies.
- Job modes: fail, replace (the default), replace-irreversibly, isolate, ignore-dependencies, ignore-requirements, flush, triggering, restart-dependencies. **[W]** https://www.freedesktop.org/software/systemd/man/latest/systemctl.html
- **Re-checked 2026-11-23 against the local clone at `v262.2`** (`git describe --exact-match`), in `src/core/transaction.c`:
  - `transaction_verify_order_one` is at :412;
  - the "deleted to break ordering cycle" log is at :492;
  - `BUS_ERROR_TRANSACTION_ORDER_IS_CYCLIC` is at :520;
  - "Probably contradicting requirement" is at :256;
  - "is destructive" is at :646.

  The man-page lines at v262.2:
  - `systemd.unit.xml`: requirement vs ordering :670; `BindsTo=` :731; `SuccessAction=` :1106; `StartLimitIntervalSec=` :1214;
  - `systemd.service.xml`: `RestartSteps=` :619; `RestartMaxDelaySec=` :690; `WatchdogSec=` :879;
  - `sd_notify.xml`: `sd_notify_barrier` :83; `BARRIER=1` :495;
  - `systemd.resource-control.xml`: `Delegate=` :1421.
- Transaction engine, **[S]** `src/core/transaction.c@v257` (https://raw.githubusercontent.com/systemd/systemd/v257/src/core/transaction.c):
  - `transaction_activate` runs these steps:
    1. Find the jobs that matter to the anchor.
    2. Minimise impact (in fail mode).
    3. Drop redundant jobs.
    4. Loop {collect garbage, verify order}.
    5. Loop {merge jobs}.
    6. Drop redundant jobs.
    7. Check destructiveness.
    8. Apply.
  - `transaction_verify_order_one` handles a cycle. It picks a victim: the first job on the backward walk for which `!job_matters_to_anchor(k)`. It then logs "Job %s/%s deleted to break ordering cycle starting with %s/%s". Only when no such job exists does it return `BUS_ERROR_TRANSACTION_ORDER_IS_CYCLIC`.
  - `transaction_merge_jobs`: "Transaction contains conflicting jobs ... Probably contradicting requirement dependencies configured."
  - `transaction_is_destructive`: "Transaction for %s/%s is destructive (%s has '%s' job queued, but '%s' is included in transaction)".

### 1.3 Lifecycle FSM
- ActiveState is one of active, inactive, failed, activating, deactivating, maintenance, reloading, refreshing. **[W]** systemctl(1).
- LoadState is one of loaded, error, masked. **[W]** https://www.freedesktop.org/software/systemd/man/latest/org.freedesktop.systemd1.html
- JobRemoved results are done, canceled, timeout, failed, dependency, skipped. **[W]** same page.
- The FSM is the C implementation (`service.c` has a much finer SubState). No published transition table was found. **[U]** No formal model is known to us.

### 1.4 Readiness and liveness
- `Type=` decides when a unit counts as started. **[W]** https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html
  - simple: after fork, before exec.
  - exec: after `execve`.
  - forking: when the parent exits.
  - oneshot: after exit.
  - dbus: when the bus name is acquired.
  - notify / notify-reload: on `READY=1`.
  - idle.
- `ExitType=main|cgroup`, `RemainAfterExit=`.
- `sd_notify` sends an AF_UNIX datagram to `$NOTIFY_SOCKET`, carrying SCM_CREDENTIALS and gated by `NotifyAccess=`. **[W]** https://www.freedesktop.org/software/systemd/man/latest/sd_notify.html
  - Fields: READY, RELOADING, STOPPING (irreversible since v259), WATCHDOG=1, WATCHDOG=trigger, MONOTONIC_USEC, STATUS, ERRNO, MAINPID, FDSTORE, EXTEND_TIMEOUT_USEC, BARRIER=1.
  - **Race:** if the sender exits before PID 1 processes the message, the message may not be attributed to the unit. `sd_notify_barrier()` exists to close that race.
- Liveness: `WatchdogSec=`. A missed `WATCHDOG=1` ping sends SIGABRT. **[W]** systemd.service(5).

### 1.5 Restart and backoff
All **[W]**, from systemd.service(5) and systemd.unit(5).
- `Restart=` takes no, on-success, on-failure, on-abnormal, on-watchdog or always. `RestartPreventExitStatus=` and `RestartForceExitStatus=` adjust it.
- `RestartSec=` defaults to 100 ms. `RestartSteps=` and `RestartMaxDelaySec=` make the delay grow exponentially, with ratio (Max/Sec)^(1/Steps).
- Restarts count against `StartLimitIntervalSec=`/`StartLimitBurst=`. Past the limit, there are no more automatic restarts until `reset-failed`. This is a terminal state that needs an operator.
- Stopping uses `TimeoutStopSec=`: SIGTERM, then SIGKILL.

### 1.6 Non-root privilege and delegation
- A user manager runs with `--user`. `loginctl enable-linger` gives "a user manager is spawned for the user at boot and kept around after logouts". **[W]** https://www.freedesktop.org/software/systemd/man/latest/loginctl.html. Who may enable lingering is not stated there. **[U]** polkit policy.
- On the system bus, reads are open to every client. Writes go through polkit actions `org.freedesktop.systemd1.manage-units`, `manage-unit-files`, `set-environment` and `reload-daemon`. **[W]** org.freedesktop.systemd1(5).

### 1.7 Nesting under another manager
- cgroup delegation. **[W]** https://systemd.io/CGROUP_DELEGATION/
  - `Delegate=` hands the subtree to the service and chowns it to `User=`. Only services and scopes can delegate, never slices.
  - On cgroup v2 the kernel forbids processes in inner nodes, so the delegated manager must move itself into a sub-cgroup. A nested manager must insert that extra level.
  - Single-writer rule: never create cgroups outside the delegated subtree, never write to systemd's own cgroups. The kernel does NOT enforce this.
- Container interface. **[W]** https://systemd.io/CONTAINER_INTERFACE/
  - `$container` env var, read-only `/sys`, SIGRTMIN+3 for a clean shutdown.
  - `/run/host/notify` serves as `$NOTIFY_SOCKET` and `/run/host/credentials/` holds credentials. A PID-1 manager inside a container speaks the same protocol upward that its own services speak to it.
- Transient units via `systemd-run`, as a `.service` or a `.scope`. A scope inherits the caller's environment; a service gets a clean one. Failed units stay loaded until reset; `--collect` unloads them anyway. **[W]** https://www.freedesktop.org/software/systemd/man/latest/systemd-run.html

### 1.8 Credentials
All **[W]**, from https://systemd.io/CREDENTIALS/.
- Sources: `LoadCredential=`, `LoadCredentialEncrypted=`, `SetCredential=` (for non-secrets only), `ImportCredential=`.
- `$CREDENTIALS_DIRECTORY` is immutable at runtime, readable only by the service's user, and lives on a non-swappable ramfs.
- Encryption uses TPM2 and/or `/var/lib/systemd/credential.secret`, through `systemd-creds`.
- Handoff: a container manager sets `$CREDENTIALS_DIRECTORY` for its PID 1. VMs receive credentials via SMBIOS or fw_cfg.
- Environment variables are rejected because they are "inherited down the process tree, have size limitations, and issues with binary data".

### 1.9 Logs
- journald captures stdout/stderr. Not opened in this pass. **[U]**

### 1.10 Control API and authorization
- D-Bus `org.freedesktop.systemd1`, with polkit on writes (see 1.6). `StartTransientUnit` releases the unit once it is neither running nor referenced. **[W]**

### 1.11 Documented defects and critiques
- systemd#3829, "Make dependency cycles easier to detect" (2016): "systemd happily will kill a random service in the dependency cycle", and the deletion is "rather hard to notice". Closed 2025-11-13 via PR #39210. **[W]** https://github.com/systemd/systemd/issues/3829
- Launchpad #1727687: "Breaking ordering cycle by deleting job network.target/stop". The cycle ran network-online.target → … → nbd-client.service, and the machine booted without network. Closed Won't Fix on 2021-06-30. **[W]** https://bugs.launchpad.net/ubuntu/+source/systemd/+bug/1727687
- Found in search results but not opened **[U]**: systemd-stable#69, RH bz 1693374, the systemd-devel "Transaction is destructive" threads, and RH solution 5444661. systemd-devel 2017-11/039871 returned 403.
- **Inferred:** `Requires=` without `After=` is a silent footgun. The configuration is valid, but its meaning (start in parallel, ignore the failure at start) is almost never what the author wanted, and nothing warns.

---

## 2. launchd (macOS)

### 2.1 Config model and composition laws
- One plist per job. `Label` is required (`launchd.plist.5` L104). **[F]**
- The Disabled state is now kept outside the file (L108-115). **[F]**
- Search locations are listed at L930-950 of `launchd.plist.5` and at L70-90 of `launchd.8`. **[F]**
  - `~/Library/LaunchAgents`, `/Library/LaunchAgents`, `/Library/LaunchDaemons`.
  - `/System/Library/LaunchAgents` and `/System/Library/LaunchDaemons`.
- No overlay or drop-in mechanism: a job is a single file.
- Ownership rules: agents in `$HOME` are owned by the user, daemons by root, and no file may be group- or world-writable (`launchctl.1` L526-536). **[F]**

### 2.2 Dependency semantics
- "launchd has no explicit dependency model. Interdependencies are expected to be solved through the use of IPC" (`launchd.plist.5` L874-881). **[F]**
- Instead of dependencies, launchd uses on-demand activation from Sockets and MachServices (L733-811, check-in via `launch_activate_socket(3)`). **[F]**
- `OtherJobEnabled` only checks that the other job is loaded, not running, and is "highly discouraged" (L336-346). **[F]**

### 2.3 Lifecycle FSM
- Domains: system, user/<uid>, login/<asid>, gui/<uid> and pid (`launchctl.1` L24-81). **[F]**
- Verbs: bootstrap / bootout (L99-113); enable / disable, persistent across boots (L115-120); kickstart `-k`/`-p` (L122-138). **[F]**
- No published state machine. The output of `print` "is NOT API" (L315-322). **[F]**
- Jobs must not daemonize (`launchd.plist.5` L17-35). **[F]**

### 2.4 Readiness and liveness
- There is no readiness protocol. Readiness is implicit: the job owns the socket that launchd holds and hands it over at check-in.
- `HideUntilCheckIn` exists but is discouraged (L716-726). **[F]**
- `KeepAlive` conditions are ORed: SuccessfulExit (L297-301), Crashed (L348-351), NetworkState, PathState and OtherJobEnabled (L266-282). **[F]**
  - NetworkState "never acted how most users expected" (L303-304).
  - PathState and WatchPaths are "inherently race-prone and lossy" (L332-334, L460-464).

### 2.5 Restart and backoff
- `ThrottleInterval` defaults to 10 s (L437-444). **[F]**
- "Jobs that exit quickly and frequently ... will be throttled" (L266-282). **[F]**
- No exponential backoff and no terminal failed state are documented.
- Stopping:
  - `ExitTimeOut`: 0 means infinity and "can stall system shutdown forever" (L429-435). **[F]**
  - `EnableTransactions`: SIGTERM then SIGKILL while a transaction is open, immediate SIGKILL otherwise (L216-229). **[F]**
  - `AbandonProcessGroup` (L665-669). **[F]**

### 2.6 Non-root privilege and delegation
- The `user/<uid>` domain "may exist independently of a logged-in user" (`launchctl.1` L44-48). **[F]**
- System domain: "Anyone may read or query the system domain, but root privileges are required to make modifications" (L37-42). **[F]**
- `UserName` only applies in the system domain (`launchd.plist.5` L117-119). **[F]**
- `setuid` is not enough to assume a user's full identity, because of the Mach bootstrap namespace and the audit session. An agent is "the only way" (`launchd.8` L58-68). **[F]**

### 2.7 Nesting under another manager
- launchd is PID 1 and "You cannot invoke launchd directly" (`launchd.8` L24-31). **[F]**
- A nested supervisor is just a job. There is no delegation protocol (inferred).

### 2.8 Credentials
- No secret mechanism in these man pages (inferred from the absence; Keychain is a separate API). **[U]**

### 2.9 Logs
- `StandardOutPath` and `StandardErrorPath` (L522-563). **[F]**
- Unified logging is not covered in these man pages. **[U]**

### 2.10 Control API and authorization
- `launchctl` over XPC/Mach. Authorization is by domain: root for the system domain, the uid for its own domain (L37-48). **[F]**

### 2.11 Documented defects and critiques
- `load`/`unload` "will only return a non-zero exit code due to improper usage. Otherwise, zero is always returned" (`launchctl.1` L588-595). **[F]** The exit status lies.
- The `print` output is not an API (L315-322). **[F]**
- NetworkState, PathState and WatchPaths are self-declared unreliable (above). **[F]**
- An octal-permission bug is noted for `Sockets` (L809). **[F]**

---

## 3. Windows Service Control Manager

### 3.1 Config model and composition laws
- Configuration lives in the registry, under `HKLM\System\CurrentControlSet\Services`. It is written by `CreateService`/`ChangeServiceConfig`, not by files. **[W]** https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-createservicew
- The "Group" value "can override the setting of the DependOnService value". **[W]** same page.
- Start types: AUTO, DEMAND, DISABLED (plus BOOT and SYSTEM for drivers). **[W]**
- Delayed auto-start runs after the other auto-start services plus a delay, one service at a time, honouring dependencies. It gives no timing guarantee and cannot be used in a load-order group. **[W]** https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_delayed_auto_start_info

### 3.2 Dependency semantics
All **[W]**, from CreateServiceW.
- `lpDependencies` names services or groups (groups carry the SC_GROUP_IDENTIFIER prefix).
- Group semantics are an OR: "this service can run if at least one member of the group is running after an attempt to start all members".
- **Cycles are rejected at creation** with `ERROR_CIRCULAR_DEPENDENCY`.
- Start failure: `ERROR_SERVICE_DEPENDENCY_FAIL`. **[W]** https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicew
- Triggers start or stop a service on device events, firewall ports, domain join, ETW and others. **[W]** https://learn.microsoft.com/en-us/windows/win32/services/service-trigger-events
  - "A service that has dependent services running cannot be stopped in response to a trigger event."
  - Triggers are not guaranteed under low memory.

### 3.3 Lifecycle FSM
All **[W]**, from https://learn.microsoft.com/en-us/windows/win32/services/service-status-transitions.
- States: STOPPED, START_PENDING, RUNNING, STOP_PENDING, PAUSE_PENDING, PAUSED, CONTINUE_PENDING.
- The service starts in STOPPED. The SCM sets START_PENDING and calls ServiceMain; the service then reports RUNNING plus the controls it accepts.
- After STOP_PENDING, STOPPED is the only valid next state.
- **The FSM is not enforced:** "if a service reports a transition that is not valid, the SCM does not fail the call". The diagram exists only as an image.

### 3.4 Readiness and liveness
- Readiness is the service reporting RUNNING via `SetServiceStatus`. While pending, it reports `dwCheckPoint`/`dwWaitHint` (default 2 s); callers poll `QueryServiceStatus`. **[W]** StartServiceW.
- There is no liveness watchdog. A hang is detected only through wait-hint timeouts during transitions (inferred).

### 3.5 Restart and backoff
All **[W]**, from https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actionsa.
- A failure is a service that "terminates without reporting a status of SERVICE_STOPPED".
- Action arrays: the Nth failure runs action [N-1], and past the end the last action repeats. `dwResetPeriod` resets the counter.
- Actions: RESTART, REBOOT, RUN_COMMAND (under the service's account).
- The FAILURE_ACTIONS_FLAG variant extends the actions to non-crash failures.
- **Inferred:** the action list is a step function. If the last action is RESTART, retries never end.

### 3.6 Non-root privilege and delegation
- SCM rights: Administrators get ALL_ACCESS; authenticated users get only CONNECT, ENUMERATE and QUERY_LOCK_STATUS. `CreateService` requires admin. **[W]** https://learn.microsoft.com/en-us/windows/win32/services/service-security-and-access-rights
- Each service has its own DACL (SERVICE_START, STOP, PAUSE_CONTINUE, CHANGE_CONFIG, USER_DEFINED_CONTROL, …), delegated with `SetServiceObjectSecurity`. **[W]** same page.
- Warning: granting CHANGE_CONFIG or STOP can "possibly allow them to run applications under the LocalSystem account". **[W]**
- Per-user services are created from registry templates at sign-in and deleted at sign-out, with a `_LUID` suffix; `UserServiceFlags=0` prevents them. Administrators define them. **[W]** https://learn.microsoft.com/en-us/windows/application-management/per-user-services-in-windows
- **Inferred:** users cannot author their own services.
- Accounts: LocalSystem (the default when NULL), LocalService, NetworkService, managed service accounts, virtual `NT SERVICE\name`. **[W]** CreateServiceW.

### 3.7 Nesting under another manager
- A nested supervisor is just a service. There is no delegation of sub-services (inferred).

### 3.8 Credentials
- A service account and its password are stored by the SCM (`lpPassword`). The per-service secret mechanism (LSA secrets) was not opened. **[U]**

### 3.9 Logs
- The Event Log. Not opened. **[U]**

### 3.10 Control API and authorization
- Win32 API with per-object DACLs (3.6).
- `EnumServicesStatusEx` silently omits the services the caller cannot query. **[W]** service-security-and-access-rights.
- **Global serialization:** "StartService will block for 30 seconds if any service is busy handling a control code ... the SCM processes only one service control notification at a time". The database is locked during a service's initialization. **[W]** StartServiceW.

### 3.11 Documented defects and critiques
- The unenforced FSM (3.3).
- The 30 s global block (3.10).
- CHANGE_CONFIG as a privilege-escalation vector (3.6).
- Delayed auto-start makes clients fail and retry (3.1).
- An idle self-stop races with trigger events; the documented workaround is to return ERROR_SHUTDOWN_IN_PROGRESS, after which the SCM queues the events. **[W]** service-trigger-events.

---

## 4. SysV init with LSB headers (Debian insserv)

The LSB core spec (refspecs.linuxfoundation.org and refspecs.linuxbase.org) is behind an Anubis challenge and could not be read. **[U]** Everything below comes from Debian's documentation.

### 4.1 Config model and composition laws
- Shell scripts in `/etc/init.d` with an `### BEGIN INIT INFO` comment header. Runlevels are symlink farms (`rcN.d/S##name`).
- `/etc/insserv/overrides/` replaces a script's header wholesale. `/etc/insserv.conf` defines the facilities. **[W]** https://manpages.debian.org/trixie/insserv/insserv.8.en.html

### 4.2 Dependency semantics
- `Required-Start` "defines facilities that must be available to start the script". With `Should-Start`, the script can still start when the facility is missing. **[W]** https://wiki.debian.org/LSBInitScripts
- Other keywords: `Provides`, `X-Start-Before`, `X-Stop-After`, `Default-Start`/`Default-Stop`.
- Facilities: `$local_fs`, `$network`, `$named`, `$portmap`, `$remote_fs`, `$syslog`, `$time`, and `$all` (which goes last). **[W]** same pages.
- insserv computes the order **once, at install time**, and writes `.depend.boot/.start/.stop` for startpar. **[W]** insserv(8).
- Cycle handling is not stated on the page read. **[U]**

### 4.3–4.11 Other axes
- Lifecycle FSM: none. A script implements start, stop, restart, status ad hoc, and there is no state store (inferred).
- Readiness: the script returning (inferred). No liveness, no restart.
- Non-root: none. Credentials: none. Logs: whatever the script does.
- **Main critique (inferred):** the order is decided at install time but executed at boot. It goes stale whenever headers change without re-running insserv. "Required" means "ordered after", not "running".

---

## 5. OpenRC (0.64.1)

### 5.1 Config model and composition laws
- `/etc/init.d` scripts in `openrc-run` shell, plus `/etc/conf.d` overrides. **[W]** https://raw.githubusercontent.com/OpenRC/openrc/master/service-script-guide.md
- Runlevels are symlink directories. Stacked runlevels are undocumented: the guide says "FIXME: Document stacked runlevels". **[W]** https://raw.githubusercontent.com/OpenRC/openrc/master/user-guide.md
- The metadata cache under `${RC_SVCDIR}/cache` is invalidated by mtime. **[W]** user-guide.md.

### 5.2 Dependency semantics
- Keywords, from service-script-guide.md **[W]**:
  - `need`: hard.
  - `use`: soft, and only if the service is in the runlevel.
  - `want`: try to start.
  - `after`/`before`: ordering only.
  - `provide`: virtual service.
  - `keyword`: platform overrides.
- Dependency pairs, **[S]** `src/librc/librc-depend.c@0.64.1`: ineed/needsme, iuse/usesme, iwant/wantsme, iafter/ibefore, iprovide/providedby.
- Missing dependency: "Service '%s' needs non existent service '%s'". **[S]**
- **Cycles:** phase 5 of `rc_deptree_update()` **silently deletes** the `ibefore` edge (and the matching `iafter`) when the target is already in the service's closure. `visit_service()` keeps a visited list. There is no loop diagnostic. **[S]**
- Multiple providers: `get_provided()` picks by runlevel and state, then falls back to the first listed provider. This is order-dependent. **[S]**
- `rc_parallel` enables parallel start. **[W]** user-guide.md.

### 5.3 Lifecycle FSM
- **[S]** `src/librc/rc.h.in@0.64.1`, `RC_SERVICE` bit flags.
  - Exclusive primary states: STOPPED 0x1, STARTED 0x2, STOPPING 0x4, STARTING 0x8, INACTIVE 0x10.
  - Add-on flags: HOTPLUGGED 0x100, FAILED 0x200, SCHEDULED 0x400, WASINACTIVE 0x800, CRASHED 0x1000.
- The rule that exactly one primary state holds at all times is only a **comment**. The type is a bitmask, so illegal combinations are representable.
- A custom restart function "will not work, this is a design decision": restart is stop then start. **[W]** service-script-guide.md.

### 5.4 Readiness and liveness
- From `man/supervise-daemon.8@0.64.1` **[S]**:
  - `--notify fd:N` (the service writes a newline) or `$NOTIFY_SOCKET` with READY=1.
  - `--healthcheck-timer` and `--healthcheck-delay`.

### 5.5 Restart and backoff
- `--respawn-delay` (default 0), `--respawn-max` (default 10; 0 means unlimited), `--respawn-period` (default 12 s). Past the limit it "will give up and exit". **[S]** supervise-daemon.8@0.64.1.
- The backoff is a fixed delay, not exponential.

### 5.6 Non-root privilege and delegation
- User services. **[W]** user-guide.md
  - Run with `openrc --user`, from `/etc/user/init.d` and `${XDG_CONFIG_HOME}/rc/init.d`.
  - Require `XDG_RUNTIME_DIR`. Started by `pam_openrc.so`, or at boot through a `user.$username` symlink in a runlevel.
- For user services, `command_user` should not be set. PID files must be writable only by root. **[W]** service-script-guide.md.

### 5.7–5.11 Other axes
- Nesting: none documented. Credentials: none documented. Logs: supervise-daemon output redirection. That part of the page was not read. **[U]**
- Control API: `rc-service` and `rc-status`, backed by state files under `RC_SVCDIR`. No authorization beyond file permissions (inferred).
- Critiques: the silent removal of a before-edge (5.2), the unenforced exclusive state (5.3), and the undocumented stacked runlevels (5.1).

---

## 6. FreeBSD rc.d

### 6.1 Config model and composition laws
- `/etc/rc.d` and `/usr/local/etc/rc.d` scripts using `rc.subr`, with `${name}_*` variables set in rc.conf. The rc.conf override order (`/etc/defaults/rc.conf` → `/etc/rc.conf` → `rc.conf.local` → `rc.conf.d/`) was not opened. **[U]**
- The enable check: "The value of rcvar is checked with checkyesno", which accepts YES/TRUE/ON/1 and NO/FALSE/OFF/0. **[W]** https://man.freebsd.org/cgi/man.cgi?query=rc.subr&sektion=8&manpath=FreeBSD+15.0-RELEASE

### 6.2 Dependency semantics
All **[W]**, from https://man.freebsd.org/cgi/man.cgi?query=rcorder&sektion=8&manpath=FreeBSD+15.0-RELEASE.
- Comment lines `# PROVIDE:`, `# REQUIRE:`, `# BEFORE:`, `# KEYWORD:`. The format is strict, and parsing stops at the first line that does not match.
- BUGS: `REQUIRE` means only "ordered after", not that the daemon is running.
- **Cycles:** rcorder prints the cycle "and exit[s] with a non-zero status". It also reports how many times each node was seen in cycles, to guide the fix.
- A missing provider: "Requirement %s in file %s has no providers." This is an error.
- `-k`/`-s` filter by keyword. `-p` puts files that can run simultaneously "on the same line" (parallel levels).
- The page does not say whether the output is a deterministic total order. **[U]**
- `required_files`, `required_dirs` and `required_modules` are checked before start: precondition checks, not dependencies. **[W]** rc.subr(8).

### 6.3 Lifecycle FSM
- `run_rc_command` handles start, stop, restart ("Perform a stop then a start"), status, rcvar and others. **[W]** rc.subr(8)
  - Prefixes: `fast` skips the running check; `force` skips rcvar and "always returns zero"; `one` skips rcvar only; `quiet`.
- There is no state store. "Running" is recomputed every time, through `check_pidfile` (the first word of the pidfile) or `check_process` (processes "with a first argument that matches procname"). **[W]**
- **Inferred:** name matching gives false positives and PID reuse; `force` makes the exit status lie.

### 6.4 Readiness and liveness
- None in rc.subr. **[W]** (absent from the page).

### 6.5 Restart and backoff
- From daemon(8) **[W]** https://man.freebsd.org/cgi/man.cgi?query=daemon&sektion=8&manpath=FreeBSD+15.0-RELEASE
  - `-r` restarts "after a one-second delay".
  - `-R` sets the delay, from 1 to 31536000 s.
  - `-C` caps the restart count ("at most restart_count times", maximum 128).
  - Exit status 3 means "already running", detected through a locked pidfile.
- A fixed delay, no exponential growth.
- Caveat: signalling the child's pidfile (`-p`) causes a restart, not a stop. To stop, signal the supervisor's pidfile (`-P`). **[W]**

### 6.6 Non-root privilege and delegation
- `${name}_user` uses `su`, or `chroot` when `${name}_chroot` is set. `${name}_login_class` and `${name}_limits` go through limits(1). **[W]** rc.subr(8).
- `daemon -u` requires superuser. **[W]** daemon(8).
- No per-user service manager is documented. **[U]**

### 6.7 Nesting under another manager
- `service -j` and service jails (`${name}_svcj`) were not covered on the page read. **[U]**

### 6.8 Credentials
- None documented. **[W]** (absent).

### 6.9 Logs
- `daemon -o` appends output to a file (created with mode 0600). `-H` reopens it on SIGHUP. `-S`, `-l` and `-T` send output to syslog. **[W]** daemon(8).
- rc.subr's `info`, `warn` and `err` go through logger(1). **[W]** rc.subr(8).

### 6.10 Control API and authorization
- `service(8)` and the scripts themselves. Authorization is Unix file permissions and root (inferred).

### 6.11 Documented defects and critiques
- `REQUIRE` means "ordered after", not "running" (rcorder BUGS).
- `force` always returns zero.
- Process detection by name.
- Signalling the child's pidfile restarts instead of stopping.

---

## 7. Comparison matrix

| Axis | systemd | launchd | Windows SCM | SysV/LSB | OpenRC | FreeBSD rc.d |
|---|---|---|---|---|---|---|
| 1 Config and composition | Unit plus ordered drop-ins; dir precedence; empty value resets. Non-commutative, non-monotone | One plist per job; no overlay; disabled state kept outside the file | Registry via API; Group can override DependOnService | Script with LSB header; override replaces the whole header | Script plus conf.d; mtime cache; stacked runlevels undocumented | Script plus rc.conf variables; override order **[U]** |
| 2 Dependencies | Requirement ⟂ ordering; ~10 relations; job modes; **cycle → silently deletes a job** | **None** ("solved through IPC"); socket and Mach activation | Service or group (group = OR); **cycle rejected at creation** | Required/Should; order computed at install time; cycles **[U]** | need/use/want/after/before/provide; **cycle → silently drops a before-edge** | REQUIRE/PROVIDE/BEFORE; **cycle → error, non-zero exit**; REQUIRE = order only |
| 3 Lifecycle FSM | Rich ActiveState/SubState; no published table | Not published; `print` "is NOT API" | 7 states; **invalid transitions accepted** | None | Bitmask; exclusivity **only a comment** | None; recomputed from pidfile or process name |
| 4 Readiness and liveness | Type=; sd_notify (needs a barrier); WatchdogSec | Socket check-in; KeepAlive conditions (some lossy) | SetServiceStatus with checkpoint/hint; no watchdog | Script returns | fd or NOTIFY_SOCKET; healthcheck timer | None |
| 5 Restart and backoff | Restart= policies; **exponential** (RestartSteps); start limit → failed until reset | Throttle 10 s; no terminal state | Action array, last action repeats; reset period | None | Fixed delay; respawn-max 10 / 12 s → gives up | daemon -r/-R fixed delay; -C ≤ 128 |
| 6 Non-root | `--user` manager plus linger; polkit | user/<uid> domain, works without login; setuid ≠ identity | Per-service DACL; CHANGE_CONFIG ⇒ escalation; per-user services are admin-defined | None | `openrc --user`, XDG, PAM | su/chroot as root; no user manager **[U]** |
| 7 Nesting | Delegate= plus the single-writer rule (unenforced); container interface (/run/host/notify, credentials) | Just a job | Just a service | n/a | n/a | jails **[U]** |
| 8 Credentials | Files in a ramfs dir; encrypted; handed down to PID 1 or VMs; **never environment variables** | None documented **[U]** | Account password in the SCM; LSA **[U]** | None | None | None |
| 9 Logs | journald **[U]** | StandardOut/ErrPath; unified logging **[U]** | Event Log **[U]** | Script | supervise-daemon **[U]** | daemon -o/-H, syslog; logger(1) |
| 10 Control and authorization | D-Bus; reads open, writes via polkit actions | XPC; domain = authority (root for system) | Win32; per-object DACL; **one control at a time, 30 s block**; enumeration hides | Run the script as root | rc-service; file permissions | service(8); root |
| 11 Defects | Random cycle victim (#3829, LP#1727687); Requires without After | load/unload always exit 0; lossy watchers; ExitTimeOut=0 can stall shutdown | Unenforced FSM; global lock; config right ⇒ LocalSystem | Order goes stale; Required ≠ running | Silent edge drop; bitmask state | REQUIRE ≠ running; `force` returns 0; name matching |

## 8. Unverified list (do not cite without opening)
- The LSB 5.0 core spec text (blocked by Anubis).
- insserv's cycle behaviour.
- systemd-stable#69, RH bz 1693374, RH solution 5444661, and the systemd-devel threads.
- Who may run `enable-linger` (polkit).
- journald, macOS unified logging, the Windows Event Log, and OpenRC's output logging.
- FreeBSD: the rc.conf override order, service jails, `service -j`, and whether rcorder's output order is deterministic.
- Credential stores on launchd and SCM (Keychain, LSA secrets).
