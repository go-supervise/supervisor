# BIBLIO-B — supervision, dependency managers, composition theory

Written 2026-11-23 for the go-zpkg supervisor design (`SUPERVISOR.md`). This is
prior art, **not a contract**: no compatibility is implied.

**Rule followed.** Every claim cites what was opened:

- a URL, fetched 2026-11-23;
- `file:line@tag`, downloaded at that tag;
- a paper section.

Claims marked **[unverified]** come from a search-result summary only, or from
a page that could not be fetched. Quotes taken through a page summariser are
marked (s). Their wording is close to the source but was not checked
character by character.

Sources downloaded at a tag:

- `supervisor.erl@OTP-27.0`: raw.githubusercontent.com/erlang/otp/OTP-27.0/lib/stdlib/src/supervisor.erl
- `service.h@dinit-v0.19.4`: raw.githubusercontent.com/davmac314/dinit/v0.19.4/src/includes/service.h
- `service.scm@shepherd-v1.0.0` and `support.scm@shepherd-v1.0.0`: git.savannah.gnu.org/cgit/shepherd.git/plain/modules/shepherd/…?h=v1.0.0
- `modules.nix`, `options.nix` and `types.nix@nixpkgs-25.05`: raw.githubusercontent.com/NixOS/nixpkgs/25.05/lib/…
- `plan.go` and `access.go@pebble-v1.33.2`: raw.githubusercontent.com/canonical/pebble/v1.33.2/internals/{plan/plan.go,daemon/access.go}. v1.33.2 is the latest release per `gh api repos/canonical/pebble/releases/latest`.
- `types.go@k8s-api-v0.34.0`: raw.githubusercontent.com/kubernetes/api/v0.34.0/core/v1/types.go
- `kubelet.go@k8s-v1.34.0`: raw.githubusercontent.com/kubernetes/kubernetes/v1.34.0/pkg/kubelet/kubelet.go
- Armstrong 2003 thesis: erlang.org/download/armstrong_thesis_2003.pdf, pages read in the PDF.

---

## 1. daemontools / runit / s6 / s6-rc (skarnet)

The cr.yp.to/daemontools.html fetch failed with an HTTP parse error, so
daemontools is covered only through runit and s6. Both describe themselves as
compatible with it.

1. **Configuration model.**
   - A service is a directory. s6 uses `run`, `finish`, `down`, `notification-fd` and `timeout-finish` (https://skarnet.org/software/s6/s6-supervise.html (s)).
   - In s6-rc, a source directory has a `type` file (`oneshot`, `longrun` or `bundle`), `dependencies.d/`, `contents.d/`, `up`/`down`, `producer-for`/`consumer-for`, and `timeout-up`/`timeout-down` (https://skarnet.org/software/s6-rc/s6-rc-compile.html (s)).
   - **No composition operator and no layering:** one directory is one service.
   - Bundles are named sets, and "A bundle, however, cannot have dependencies" (https://skarnet.org/software/s6-rc/overview.html (s)).
2. **Dependencies.**
   - s6-rc "will make sure to never start A until it knows that B is up" and "never stop B until it knows that A is down" (overview.html (s)).
   - The graph is compiled **offline**. "If s6-rc-compile detects a cycle in dependencies across services, it will complain and exit 1". Bundle cycles are refused too, and pipelines are checked for cycles and collisions (s6-rc-compile.html (s)).
   - Closure:
     - `s6-rc -u` computes "the closed selection", meaning the dependencies.
     - `-d` closes over **dependents**.
     - `-p` prunes, bringing the others down first and then the wanted ones up (https://skarnet.org/software/s6-rc/s6-rc.html (s)).
   - Parallelism: "as soon as a service that needs to change state has all its dependencies fulfilled, it undergoes its transition". Independent services have no guaranteed order (s6-rc.html (s)).
3. **Lifecycle.**
   - s6-supervise emits events: `u` (run spawned), `U` (ready), `d` (run died), `D` (finish died), and `O` (finish exited 125: "does not restart the service") (s6-supervise.html (s)).
   - The overview lists the states up, ready, down and "really down (when the ./finish process has also died)" (https://skarnet.org/software/s6/overview.html (s)).
   - The s6-rc live state is updated after each successful transition (s6-rc.html (s)).
   - **Not formalised.** No published FSM or proof was found.
4. **Readiness.**
   - The daemon writes a newline to the fd named in `notification-fd`, then closes it. s6-supervise then emits `U`.
   - Polling is called "suboptimal" (https://skarnet.org/software/s6/notifywhenup.html (s)).
   - There is no liveness probe in s6 itself.
5. **Restart.**
   - "minimum 1-second delay between two ./run spawns". A service that was ready for more than 1 s is restarted at once (s6-supervise.html (s)).
   - `./finish` must exit within 5 s by default (`timeout-finish`). Exit 125 means permanent failure.
   - runsv: "If ./run or ./finish exit immediately, runsv waits a second" (http://smarden.org/runit/runsv.8.html (s)).
   - **No intensity bound:** a crashing service is retried forever at about 1 Hz.
6. **Non-root and delegation.**
   - `s6-svperms` sets three separate permissions:
     - **status** read: owner, owner+group, or public;
     - **control**: owner, or owner+group;
     - **event subscription**.
   - Groups are the delegation unit, through `-g`/`-G`/`-O`/`-E`. The default is owner-only control (https://skarnet.org/software/s6/s6-svperms.html (s)).
   - The 66 comparison page lists s6 user scope as "s6 -u" (see §2).
7. **Nesting and PID 1.**
   - As PID 1: "init does not have the right to die" and "The reaper takes care of every zombie on the system". Boot has three stages (https://skarnet.org/software/s6/s6-svscan-1.html (s)).
   - s6-linux-init: "s6-svscan runs as process 1 for the whole machine lifetime" (https://skarnet.org/software/s6-linux-init/ (s)).
   - Under another init, run as an inittab `respawn` entry, an Upstart job with `respawn`, a systemd unit with `Restart=always`, or a BSD `/etc/ttys` entry. Caveat: "if this process dies, the pipes will be closed and some logs may be lost" (https://skarnet.org/software/s6/s6-svscan-not-1.html (s)).
   - s6-rc: "You can run s6-rc with any init system of your choosing" (https://skarnet.org/software/s6-rc/why.html (s)).
8. **Credentials.** Nothing beyond `s6-applyuidgid`, which drops privileges in a `run` script (overview.html (s)). There is no secret store.
9. **Logs.**
   - Each service has a paired logger. s6-svscan holds the daemon→logger pipe, so "the logs are never lost between the daemon and the logger" (overview.html (s)).
   - A catch-all logger is fed through a FIFO (s6-svscan-1.html (s)).
   - runsv creates a pipe to `service/log` (runsv.8 (s)).
10. **Control API.**
    - Commands go to the supervisor, not to a PID: "you send a command to its supervisor, which will then send a signal" (overview.html (s)).
    - runsv uses one-letter commands on `supervise/control`, such as u, d, o, p, c, h, a, i, q, 1, 2, t, k and x (runsv.8 (s)).
    - Authorization is filesystem permission (s6-svperms).
11. **Critiques, and the separation essay.**
    - "a supervision suite is not a service manager". Supervisors "do not perform dependency management" (why.html (s)).
    - On runit's stage split: "some one-time initialization may depend on a daemon being up".
    - On OpenRC's parallel mode: "heavily prone to race conditions".
    - s6-rc's own gaps (s6-rc.html (s)):
      - a failed transition is **not rolled back**: dependents are skipped, independent work continues, and the exit status is 1;
      - with notification and no `timeout-up`, "s6-rc will wait forever";
      - oneshots "should be ideally transactional, or at the very least idempotent", which puts the burden on the script.

## 2. 66 (Obarun)

Source: https://docs.obarun.org/66/0.9.1.2/ and its subpages (s). The
comparison page is the project's own advocacy.

- **Model.** "declares services in a readable INI format, compiles them ahead of time, and supervises each one with a dedicated supervisor process". It no longer builds on s6 libraries (0.9.1.0 upgrade notes, search summary **[unverified]**).
- **Dependencies** (66-dependencies.html (s)):
  - `Depends` (must start successfully first);
  - `RequiredBy` (the inverse, written on the other node);
  - `OptsDepends` (the first available one; **list order matters**);
  - `Conflict`;
  - `Provide` (aliases).
  - The chain is resolved "recursively, as a directed acyclic graph (DAG)", and stopping runs in reverse. Cycle handling is not documented.
- **Per-user.**
  - Each user has their own scandir `/run/66/scandir/<uid>` and their own `$HOME/.66/service` (66-container.html (s)).
  - `66-userd` brings a user's services "up on the first session and down on the last" (66-vs-other-init-systems.html (s)).
- **PID 1 in a container.**
  - 66 "stays pid 1 for the lifetime of the container", and boot failure makes it exit 111.
  - The entrypoint must `exec`, or "the shell stays pid 1" (66-container.html (s)).
- **Critique worth keeping** (advocacy): systemd's `DefaultDependencies=` means "the graph is not what the unit files say". Implicit edges make the declared graph lie (66-vs-other-init-systems.html (s)).

## 3. dinit

Sources: https://davmac.org/projects/dinit/man-pages-html/dinit-service.5.html (s),
…/dinit.8.html (s), and `service.h@dinit-v0.19.4`.

1. **Configuration.** One key=value file per service, with types `process`, `bgprocess`, `scripted`, `internal` and `triggered` (dinit-service.5 (s)). There is no layering or merge operator.
2. **Dependencies.** Five relations (dinit-service.5 (s)):

   | relation | activates the target | needs the target to have started | dependent stops when the target stops |
   |---|---|---|---|
   | `depends-on` | yes | yes | yes |
   | `depends-ms` (milestone) | yes | fails to start otherwise | no ("does not affect the dependent") |
   | `waits-for` | yes | no: waits for start *or failure* | no |
   | `after` / `before` | no | ordering only | no |

3. **Lifecycle, with the best-documented state model of the group** (`service.h@dinit-v0.19.4`):
   - **Total state = current × desired** (lines 28-44): current ∈ {STOPPED, STARTING, STARTED, STOPPING} and desired ∈ {STARTED, STOPPED}. All 8 combinations are given meanings, for example STOPPING/STARTED means "will be re-started again once it stops".
   - **Interrupted transitions** (51-57): a STARTING service can turn to STOPPING, except while waiting on an external process ("simply killing that process might leave a mess").
   - **Acquisition count `required_by`** (59-83): it is +1 if explicitly started and +1 per dependent that is not stopped. A change 0→1 starts the service and 1→0 stops it.
   - **Pinning** (94-111): "pinning prevents, but never causes, state transition". The deciders are ranked pins > force-stop > desired state.
   - **Two-phase transition** (113-128): a *propagation* phase lets the acquisition count "settle" before the *execution* phase sets STARTED/STOPPED. This avoids "an incorrect acquisition count, which may cause it to restart when it should not". Linked-list queues avoid recursion of unknown depth.
   - **Not formally verified.** The comments are prose.
4. **Readiness.** `ready-notification = pipefd:N | pipevar:VAR`. It is a newline on a pipe, "compatible with S6" (dinit-service.5 (s)). There is no liveness probe.
5. **Restart.**
   - `restart-delay` defaults to 0.2 s, `restart-limit-interval` to 10 s and `restart-limit-count` to 3 (dinit-service.5 (s)).
   - `smooth-recovery` restarts the process "without first stopping any dependent services".
   - `start-timeout` defaults to 60 s and `stop-timeout` to 10 s.
6. **Non-root.**
   - `--user` is "the default if not invoked as the root user".
   - The socket is `$XDG_RUNTIME_DIR/dinitctl` or `$HOME/.dinitctl` (dinit.8 (s)).
7. **Nesting and PID 1.**
   - `--system` is the default as PID 1.
   - `--container`/`-o` means "do not perform system management functions … will simply exit rather than executing the shutdown program" (dinit.8 (s)).
8. **Credentials.** `run-as` (dinit-service.5 (s)). There is no secret mechanism in the pages opened.
9. **Logs.** `log-type` = file | buffer | pipe | none, default none (dinit-service.5 (s)).
10. **Control.**
    - `dinitctl` talks over the control socket. dinit refuses to start if an active socket already exists (dinit.8 (s)).
    - The authorization model is **not documented** in the pages opened **[unverified: presumably socket file mode]**.
    - Stopping a service that would stop dependents requires `--force` (https://github.com/davmac314/dinit/blob/master/doc/DESIGN (s), master, not a tag).
11. **Critiques.** No independent critique was opened. 66's advocacy says dinit "is a session service manager there and nothing more" on macOS (66-vs-other-init-systems.html (s)).

## 4. GNU Shepherd

The online manual (doc.guix.gnu.org) returned 403, and the .texi download at
the tag returned HTTP 500. Everything below is from source at v1.0.0.

1. **Configuration is Scheme code.**
   - The config file is evaluated with `primitive-eval` in a fresh module (`support.scm@shepherd-v1.0.0:496-537`).
   - A service record has `provision`, `requirement`, `one-shot?`, `transient?`, `respawn?`, `respawn-limit`, `respawn-delay`, `start`, `stop`, `actions` and `termination-handler` (`service.scm@shepherd-v1.0.0:322-400`).
   - **Configuration is a program, so there are no composition laws.** Composing means calling Scheme.
2. **Dependencies.** `requirement` is a list of provisions. The search depth was not checked further **[unverified: details of requirement start/stop propagation]**.
3. **Lifecycle.**
   - Each service is an **actor**: a fiber, `service-controller`, serving messages on a channel. Its state is a loop variable `status ∈ {stopped, starting, running, stopping}` (`service.scm:451-482`).
   - Transitions are `match` clauses on (message, status). For example, `stop` while `starting` waits on a condition, then re-sends `stop` to itself (`service.scm:601-651`).
   - **Stale-event guard:** a `handle-termination` message is ignored if the status is `stopping`/`stopped`, or if the PID does not match the current process (`service.scm:665-687`). This is an epoch check by PID.
   - Not formalised.
4. **Readiness and liveness.** Neither appears in the code opened.
5. **Restart.**
   - The defaults are `default-respawn-delay` = 0.1 s and `default-respawn-limit` = `(5 . 7)`: "Respawning CAR times in CDR seconds will disable the service" (`service.scm:261-272`).
   - The comment admits a workaround: "The terrible hack … using SIGALRM to work around unreliable SIGCHLD delivery … arrange for the car to be lower than the cdr" (`service.scm:268-271`).
   - `respawn-limit-hit?` is a sliding window over respawn timestamps (`service.scm:274-288`).
   - Once the limit is hit, the service is **disabled**, with "Respawning too fast." (`service.scm:2533-2561`).
6. **Non-root.**
   - A user instance uses `$XDG_CONFIG_HOME/shepherd`, `$XDG_STATE_HOME/shepherd` for logs, and `$XDG_RUNTIME_DIR` (else `/run/user/<uid>`) for runtime files (`support.scm:405-420`).
   - The socket directory is `<runtime>/shepherd` for a user and `<runstatedir>/shepherd` for root (`support.scm:478-494`).
7. **PID 1.** The code mentions "When bootstrapping and running as PID 1, /etc/{passwd,shadow} may be unavailable" (`support.scm:400-402`). The PID 1 behaviour was not read further.
8. **Credentials.** None seen.
9. **Logs.** There is a per-service `logger` channel (`service.scm:477, 656-657`). The default user log dir is shown above.
10. **Control and authorization.**
    - Commands go through the Unix socket used by `herd`.
    - **Authorization is the socket directory**: it is created `0700`, and startup fails with "socket directory has insecure permissions" unless the owner is the uid and the mode is exactly 0700 (`support.scm:539-552`).
    - **No delegation.**
11. **Critiques.** The SIGALRM/SIGCHLD comment above is the project's own.

## 5. NixOS module system: prior art for composition laws

Sources: `modules.nix`, `options.nix` and `types.nix@nixpkgs-25.05`, plus
https://nixos.org/manual/nixos/stable/ (s).

1. **Merge semantics, as implemented.**
   - **Priorities.**
     - `mkOverride = priority: content: {_type="override"; …}` (`modules.nix:1380`).
     - `mkOptionDefault = mkOverride 1500`, `mkDefault = mkOverride 1000` and `defaultOverridePriority = 100` (`modules.nix:1385-1387`).
     - `mkImageMediaOverride = mkOverride 60` and `mkForce = mkOverride 50` (`modules.nix:1388-1389`).
   - **`filterOverrides'`** keeps **only** the definitions whose priority equals the minimum (highest) priority, then strips the wrappers (`modules.nix:1242-1254`).
   - **Order.** `mkOrder`, with `mkBefore = mkOrder 500`, `defaultOrderPriority = 1000` and `mkAfter = mkOrder 1500` (`modules.nix:1399-1406`). Properties are sorted by that priority (`modules.nix:1256-1281`).
   - **The type's merge then runs on the survivors.**
     - `types.str` uses `merge = mergeEqualOption` (`types.nix:505-511`). That function accepts N equal values and throws "has conflicting definition values" otherwise (`options.nix:498-519`). **This is exactly a flat lattice** (the draft's K1).
     - `mergeUniqueOption` throws "is defined multiple times while it's expected to be unique" (`options.nix:469-483`).
     - `mergeDefaultOption` concatenates lists, merges attrs with `foldl' mergeAttrs` (right-biased), ORs bools, concatenates strings, and accepts ints only if equal (`options.nix:425-445`).
   - The manual: for list options "the lists of packages will be concatenated" and "The value in `configuration.nix` is merged last" (s).
2. **Laws (my analysis of the code above, not a claim of the source).**
   - Per option, merge is filter-by-min-priority, then the type merge.
   - For `str`/`int`/`bool`, the result is associative, commutative and idempotent over the multiset of definitions. min is ACI, `mergeEqualOption` is ACI, and OR is ACI.
   - List concatenation and `mergeAttrs` are **not commutative**. The result depends on module evaluation order, corrected only by `mkOrder`.
   - **Priorities break monotonicity (the draft's law L).** A `mkForce` definition *discards* every lower-priority definition, so the plan need not be ⊑ a layer that was overridden.
   - So Nix keeps ACI but gives up "composition only constrains". **That is the price of `mkForce`, and it is the counter-example `SUPERVISOR.md` §1.4 asked for.** Formally, it is the lexicographic product (priority, value) under a min-then-meet order.
   - Nix needed it so that a downstream layer can fix an upstream value without editing the upstream source.
3. to 11. The modules generate systemd units. Their runtime semantics are systemd's, which is BIB-A's share, not this one.

## 6. supervisord

Sources: https://supervisord.org/subprocess.html (s) and
https://supervisord.org/configuration.html (s).

1. **Configuration.** INI with `[program:x]` and `[group:x]`, plus `[include] files` globs. Recursive includes are not supported. There is no merge semantics: sections are just collected.
2. **Dependencies.** **None.** Only `priority` (default 999): "Lower priorities indicate programs that start first and shut down last".
3. **Lifecycle.**
   - The states are STOPPED(0), STARTING(10), RUNNING(20), BACKOFF(30), STOPPING(40), EXITED(100), FATAL(200) and UNKNOWN(1000), "supervisord programming error".
   - A process that cannot be stopped "will stay in the STOPPING state forever".
   - Not formalised.
4. **Readiness.** It is a time heuristic: `startsecs` (default 1 s) is "seconds which the program needs to stay running … to consider the start successful". There is no liveness.
5. **Restart.**
   - `startretries` defaults to 3. Delays grow **linearly**: "wait one, two and then three seconds". Exhaustion → FATAL.
   - `autorestart` = false | true | unexpected (default unexpected, by exit code).
   - **Retries bound only the start phase.** A process that runs longer than `startsecs` and then crashes restarts forever.
6. **Non-root.** "The user can only be switched if supervisord is run as the root user".
7. **Nesting and PID 1.** Not addressed in the pages opened.
8. **Credentials.** None.
9. **Logs.**
   - `stdout_logfile` defaults to AUTO, with 50 MB × 10 backups.
   - "AUTO log files and their backups will be deleted when supervisord restarts".
10. **Control.**
    - The Unix socket defaults to `chmod` 0700, with an optional username/password (cleartext or `{SHA}`).
    - `inet_http_server`: "Never expose the inet HTTP server to the public internet". It has no authentication by default.
11. **Critiques.**
    - "Programs meant to be run under supervisor should not daemonize themselves".
    - Restart is unbounded after `startsecs`.
    - AUTO logs are deleted on restart.

## 7. canonical/pebble

Sources: `plan.go` and `access.go@pebble-v1.33.2`. The layer-spec page URLs
returned 404 or a redirect, so this section relies on source only.

1. **Layers.**
   - `CombineLayers` (`plan.go:703`). Every service, check and log target in a layer **must** declare `override` ∈ {`merge`, `replace`}, or the result is the error ``layer %q must define "override"`` (`plan.go:432-433, 738-794`).
   - `Service.Merge` (`plan.go:266-325`):
     - scalars are **right-biased overwrite**, and only when the later value is non-empty or non-nil;
     - `After`/`Before`/`Requires` are **appended**, with no dedup;
     - `Environment` and `OnCheckFailure` are a map union, right-biased.
   - **Laws (my analysis).**
     - Associative (overwrite and append are associative).
     - **Not commutative.**
     - Not idempotent for lists, which duplicate.
     - `replace` is non-monotone.
     - The empty string doubles as "unset", so **a field cannot be cleared by merge**.
2. **Dependencies.** `requires`, `before` and `after`. Combined layers are checked for order cycles (`plan.go:1124-1157`).
3. **Lifecycle.** The startup values are enabled/disabled (`plan.go:423`). Actions: `restart`, `shutdown`, `ignore`, `failure-shutdown` and `success-shutdown` (`plan.go:441-447`). Not formalised in what was opened.
4. **Checks.**
   - `level` ∈ {"", `alive`, `ready`} (`plan.go:521-527`).
   - The defaults are period 10 s, timeout 3 s and threshold 3 (`plan.go:79-81`).
   - The response to a failure is per service: `on-check-failure` maps a check to an action (`plan.go:315-320`).
5. **Restart.** Exponential backoff: `defaultBackoffDelay = 500ms`, `defaultBackoffFactor = 2.0` and `defaultBackoffLimit = 30s` (`plan.go:75-77`). **There is no cap on the number of attempts** in the defaults opened.
6. **Non-root.** Services carry `user`/`group`/`user-id`/`group-id` (`plan.go:282-293`). Switching user presumably needs root **[unverified]**.
10. **Access.**
    - `AccessChecker` implementations (`access.go:27-125`):
      - `OpenAccess`: everyone, "even without user credentials";
      - `UserAccess`: a valid user with the ReadAccess or AdminAccess role;
      - `AdminAccess`;
      - `MetricsAccess`: HTTP only with the metrics role;
      - `PairingAccess`: mTLS pairing during a "pairing window", with "proof of server ownership … such as a controlled power cycle or button press".
    - This is **role-based** delegation over the socket and HTTPS.
7, 8, 9, 11. Not opened.

## 8. Erlang/OTP supervisors: the most formal restart model

Sources: https://www.erlang.org/doc/system/sup_princ.html (s),
https://www.erlang.org/doc/apps/kernel/application.html (s), and
`supervisor.erl@OTP-27.0`.

1. **Configuration.**
   - The child spec has `restart` ∈ {permanent (default), transient, temporary}.
   - `shutdown` ∈ {`brutal_kill`, ms (default 5000 for workers), `infinity` (default for supervisors)}.
   - `type` ∈ {worker, supervisor}, plus `significant`.
   - Children start in list order and terminate in **reverse** order (sup_princ (s)).
2. **Dependencies are implicit in start order and strategy.**
   - `one_for_one`: "only that process is restarted".
   - `one_for_all`: "all remaining child processes are terminated … all … restarted".
   - `rest_for_one`: "the child processes after the terminated process in start order are terminated", then restarted.
   - `simple_one_for_one`: dynamic, homogeneous children (sup_princ (s)).
   - rest_for_one is a dependency edge "later depends on earlier" on a **total order**.
3. **Lifecycle.** gen_server state plus a child list. Not formally specified by OTP. The model-checking work is in §11.
4. **Readiness.** Implicit: `start_link` returns only after `init/1` returns, so the start is synchronous.
5. **Restart intensity: the exact semantics.**
   - `restart/2` calls `add_restart(State)` **once per restart event** (`supervisor.erl:1333-1352`). Under `one_for_all`/`rest_for_one`, restarting N siblings therefore counts as **one** restart.
   - `add_restart/1` (`supervisor.erl:2075-2095`):
     - time is `erlang:monotonic_time(1)`, monotonic with **1-second granularity**;
     - it keeps restarts `R >= Now - Period` with `lists:takewhile` and prepends `Now`;
     - it continues if `length(R1) =< Intensity`, else returns `{terminate,…}`.
   - On terminate: `reached_max_restart_intensity` is reported, the supervisor terminates all children and itself (`supervisor.erl:1348-1351`), with reason `shutdown` (sup_princ (s)).
   - The defaults are intensity = 1 and period = 5 (sup_princ (s)).
   - **No delay or backoff**: a restart is immediate.
   - **Termination argument (mine, from the code).** Each supervisor permits at most `I` restarts in any `P`-second window. Exceeding that converts the failure into the supervisor's **own** exit. The parent counts that exit as one restart of its own.
   - Escalation is therefore bounded by tree height. At the root, the application restart type decides: "If a permanent application terminates, all other applications and the entire Erlang node are also terminated". `transient` does the same only on an abnormal exit, and `temporary` is just reported (application.html (s)).
   - **Failure is never retried forever at any level.** It is pushed up until something with a different policy, or the node, stops.
   - `auto_shutdown` ∈ {never, any_significant, all_significant}. It is invalid with `permanent` significant children (sup_princ (s)).
6 to 9. Not applicable: BEAM processes, not OS processes.
10. **Control.** `supervisor:start_child`, `terminate_child`, `restart_child` and `delete_child` (sup_princ (s)). Authorization is node membership (cookie) **[unverified]**.
11. **Armstrong's own self-critique** is thesis §8.3.5 "Deficiencies in OTP structure" (p.181). It was **not read**; only the table of contents was.

### Armstrong thesis, chapter 5.2 (pp. 118-123, read in the PDF)

- The idea is "Try to perform a task. If you cannot perform the task, then try to perform a simpler task" (p.118).
- The rules: "Supervision trees are trees of Supervisors … Supervisors monitor Workers and Supervisors … Workers are instances of Behaviours … Behaviours are parameterised by Well-behaved functions … Well-behaved functions raise exceptions when errors occur" (p.119).
- The SSRS is a "Start Stop and Restart Specification" (p.120).
- **Linear supervision:** "If my parent stops me then I should stop all my children. If any of my children dies then I must try to restart that child" (p.121).
- **AND/OR trees.** An AND supervisor stops and restarts all its children, and an OR supervisor restarts only the one that died. "AND supervision is used for dependent, or co-ordinated processes"; OR is for "independent processes" (pp.122-123).
- Goals are equated with **invariants**: "the goal has been achieved if the invariant associated with the goal is not false" (p.123).
- Errors are "a deviation between the observed behaviour of a system and the desired behaviour" (p.124).
- p.123 cites Candea and Fox [22], "recursively-restartable Java components" (see §11).

## 9. Kubernetes probes

Sources: https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/ (s),
https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/ (s, offset 100000),
`types.go@k8s-api-v0.34.0:2705-2741`, and `kubelet.go@k8s-v1.34.0`.

**Field definitions** (`types.go:2705-2741`). **There is no `initialThreshold` field.** The brief's term does not exist; the fields are:

| field | definition | default and minimum |
|---|---|---|
| `initialDelaySeconds` | seconds "after the container has started before liveness probes are initiated" | — |
| `timeoutSeconds` | — | default 1, min 1 |
| `periodSeconds` | — | default 10, min 1 |
| `successThreshold` | "Minimum consecutive successes … after having failed" | default 1, **must be 1 for liveness and startup** |
| `failureThreshold` | "Minimum consecutive failures … after having succeeded" | default 3, min 1 |
| `terminationGracePeriodSeconds` | per-probe override of the pod grace period on probe failure (beta) | — |

**Semantics** (probes page (s)):

- **Startup:** while it is configured, "Kubernetes does not execute liveness or readiness probes until the startup probe succeeds". On failure, the container is killed and the restart policy applies. It runs only at startup.
- **Liveness:** "determine when to restart a container". It does not wait for readiness.
- **Readiness:** on failure, the Pod IP is removed from EndpointSlices. It runs during the whole lifecycle. **No restart.**
- **Caution:** "Incorrect implementation of liveness probes can lead to cascading failures … restarting of container under high load".

**Restart backoff** (pod-lifecycle (s); `kubelet.go:156-161, 216`):

- The delay is exponential: "10s, 20s, 40s, …", capped at 300 s.
- The timer resets after the container "executed for 10 minutes without any problems".
- `backOffPeriod = 10s`, and `MaxCrashLoopBackOff = v1beta1.MaxContainerBackOff`. A feature gate reduces the cap to 60 s (`reducedMaxCrashLoopBackOff`).
- **No attempt cap**: CrashLoopBackOff continues forever.

Items 1-2 and 6-11 are pod-level and outside this share.

## 10. Upstart

Sources: https://manpages.ubuntu.com/manpages/trusty/man5/init.5.html (s),
…/man7/upstart-events.7.html (s), and http://0pointer.de/blog/projects/systemd.html (s).
The Upstart cookbook (upstart.ubuntu.com) no longer resolves (DNS ENOTFOUND),
and the web archive was not fetchable.

1. **Configuration.** Job files with `start on`/`stop on` event expressions. "no job processes are started until the overall expression evaluates to true" (init.5 (s)).
2. **Dependencies are expressed as events, not as a graph.**
   - `starting`/`stopping` are **hook** events: "Blocking. Waits for events that start on or stop on this event".
   - `started`/`stopped` are **signals**: "Non-blocking" (upstart-events.7 (s)).
3. **Lifecycle.** `task` vs service (the default). Instances: "only one instance of any job is permitted to exist at one time" by default (init.5 (s)). The full job-state list is in the unreachable cookbook **[unverified]**.
4. **Readiness.**
   - `expect stop` uses SIGSTOP as the ready signal.
   - `expect fork` means "will fork once", and `expect daemon` means "will fork twice". Without these, "init is unable to supervise daemon processes and will believe them to have stopped" (init.5 (s)).
   - **Getting the fork count wrong breaks supervision.** The ptrace mechanism behind it is not on the man page **[unverified]**.
5. **Restart.** `respawn limit COUNT INTERVAL`: "Default COUNT is 10. Default INTERVAL is 5 seconds" (init.5 (s)).
6. **Non-root.** `init --user` runs "in User Session mode" (init.5 (s)). Also `setuid`/`setgid`.
9. **Logs.** `console log` uses a pty-based log, and existing files "are appended to" (init.5 (s)).
11. **Critiques.**
    - Poettering, under "On Upstart": "the event logic turns around all dependencies, from the feet onto their head", and "the person who is writing Upstart files must now translate the dependencies manually into these event/action rules" (0pointer.de (s)).
    - **Abandonment** **[unverified, search summary only]**: Shuttleworth's "Losing graciously" (Feb 2014) followed Debian's choice of systemd. Ubuntu switched in 15.04, per LWN https://lwn.net/Articles/586421/ (not opened).

## 11. Theory

### CUE value lattice

Source: https://cuelang.org/docs/reference/spec/ (s), sections Values,
Unification, Disjunction and Default values.

- "All possible values are ordered in a lattice, a partial order where every two elements have a single greatest lower bound". Top is `_` and bottom is `_|_`.
- Unification `&` "is defined as the greatest lower bound … It is commutative, associative, and idempotent".
- Disjunction `|` is the least upper bound, and is also ACI.
- **Defaults are outside the pure lattice.** "non-starred elements are dropped in favor of starred ones if the starred ones do not resolve to bottom". `(*"tcp"|"udp") & (*"udp"|"tcp")` gives `"tcp" | "udp"` with **no default** (default `_|_`). Conflicting defaults silently vanish rather than erroring.
- **Closedness:** "Closing a struct is equivalent to adding `..._|_` to it". A closed schema is therefore itself a lattice element, and an unknown field gives ⊥.

### Lattice-based configuration merging

- NixOS is §5: ACI with priorities, but non-monotone.
- Pebble is §7: associative only.
- No dedicated paper on lattice configuration merging was opened. One searched-for item was not found, so that is listed as a gap.

### Papers on supervision-tree correctness and init dependency

- **Castro, Gulías, Benac Earle, Fredlund, Rivas, "A Case Study on Verifying a Supervisor Component Using McErlang", PROLE 2010, pp. 23-40.** Abstract (https://dcastrop.github.io/publication/2010-prole (s)): formal verification of "an alternative implementation of the standard supervisor behaviour", "checked against several safety and liveness properties". The properties themselves are not listed on the abstract page.
  - A follow-up exists at PROLE 2011, and a timed McErlang extension at FMOODS/FORTE 2012, which uses a supervision component as its example **[unverified: search summary]**.
  - State explosion is cited as the limit **[unverified: search summary]**.
- **Candea & Fox, "Recursive Restartability: Turning the Reboot Sledgehammer into a Scalpel", HotOS-VIII 2001, DOI 10.1145/1047915.1047919.** The PDF (radlab.cs.berkeley.edu/…/c19.pdf) failed on a TLS certificate. **[unverified beyond citation and title; cited by Armstrong p.123]**
- **Init dependency resolution.** A search for formal models of systemd or init dependency resolution found **no paper**. The closest hit was Spichkova, "Formalisation and Analysis of Component Dependencies" (Isabelle/HOL, arXiv 1405.3017), **not opened**.

### TLA+ or model-checked service managers

- A search ("TLA+ specification service manager supervisor init system model checking") found **no TLA+ spec of an init system or service supervisor**. It found only general TLA+ material and the ZooKeeper specs.
- The McErlang work above is the only model-checked supervisor found.
- **Absence from one search is not proof of absence** **[searched, not exhaustive]**.

---

## 12. Comparison matrix

Key: — = absent; ? = not found in the sources opened.

| axis | s6 / s6-rc | 66 | dinit | Shepherd | NixOS modules | supervisord | Pebble | Erlang/OTP | k8s probes | Upstart |
|---|---|---|---|---|---|---|---|---|---|---|
| 1. config and composition laws | dirs; no merge | INI, compiled; no merge | key=value; no merge | Scheme code; none | priority filter + typed merge: ACI for scalars, lists non-commutative, **non-monotone** (mkForce) | INI + include; none | layers, right-biased overwrite + append: **associative only** | child spec list; none | per-container fields | job files; none |
| 2. dependencies | DAG compiled offline, cycles refused; closure up/down | Depends/RequiredBy/OptsDepends(ordered)/Conflict; DAG | 5 kinds: hard, milestone, soft, after, before | `requirement` list | n/a (systemd) | **none**, priority only | requires/before/after; order-cycle check | start order + strategy (rest_for_one = total-order dependency) | n/a | events (hook/signal) |
| 3. lifecycle FSM, formalised? | events u/U/d/D/O; no | ? | current×desired (8), 2-phase, pins; prose only | actor, 4 states; no | n/a | 8 states; prose | ?; no | implicit; McErlang case study for one implementation | Waiting/Running/Terminated | ?; no |
| 4. readiness / liveness | fd newline / — | via s6-style | pipefd/pipevar / — | ? / — | n/a | `startsecs` timer / — | checks alive/ready, threshold 3 | sync `init/1` / links | startup gates; readiness = traffic; liveness = restart | SIGSTOP / fork counting / — |
| 5. restart and bounds | 1 s min delay, **unbounded** | ? | 0.2 s; 3 in 10 s | 0.1 s; 5 in 7 s → disabled | n/a | linear +1 s, 3 start tries → FATAL; **unbounded after startsecs** | exp 0.5 s ×2 ≤ 30 s; **unbounded** | immediate; I in P s window, then **escalate to parent** | exp 10 s ×2 ≤ 300 s, reset at 10 min; **unbounded** | 10 in 5 s |
| 6. non-root / delegation | user tree; svperms groups (status/control/events) | per-uid scandir; 66-userd | `--user` default for non-root | XDG dirs; socket dir 0700 | n/a | no user switch without root | user/group fields; roles | n/a | n/a | `--user` session |
| 7. nested / PID 1 | respawn under any init; s6-svscan as PID 1 | PID 1 in container; exit 111 | `--container`, `--system` | PID 1 supported | n/a | ? | ? | n/a | n/a | was PID 1 |
| 8. credentials | — | ? | run-as | — | n/a | — | — | n/a | n/a | setuid/setgid |
| 9. logs | paired logger, pipe held by svscan | own loggers | file/buffer/pipe | per-service logger | n/a | files, 50 MB × 10, AUTO deleted on restart | log targets | n/a | n/a | pty `console log` |
| 10. control and authz | fifo/control file; fs perms | ? | socket; ? | socket; fs only | n/a | XML-RPC socket 0700 + optional password; inet unauth | HTTP API; Open/User/Admin/Metrics/Pairing roles | API, node cookie [unv.] | n/a | initctl |
| 11. defects | no rollback; wait forever without timeout-up | advocacy page | — | SIGALRM hack | mkForce non-monotone | restart unbounded; no deps | merge not commutative, can't unset | 1 s clock; no backoff | liveness cascades | inverted dependencies; abandoned |

## 13. Answers to the draft's open questions (`SUPERVISOR.md`)

These are my inferences from the sections cited above.

- **§1.4, "no relax" vs `mkForce` (§5).**
  - Nix keeps ACI but loses law L: a priority-50 definition erases the lower ones.
  - If go-zpkg ever needs a relaxation, the Nix shape is the one to copy. It is an explicit, audited rank, and two definitions at the same rank still conflict.
  - It must not be an implicit last-wins, as in Pebble (§7).
  - Otherwise, refusing relaxation is a deliberate choice of rigidity, and the price is that a downstream layer cannot fix an upstream value.
- **§3, consecutive count `n` with StableTO vs Erlang (§8).**
  - OTP counts every restart in a sliding window, per supervisor, with no delay, and escalates when the window overflows.
  - The draft counts *consecutive* failures per service and resets them on StableTO, with backoff, and stops at `Failed`.
  - Neither is wrong, but the draft has no escalation. A `Failed` leaf is terminal unless something above it reacts. State this explicitly.
  - Also state whether a restart that drags dependents along (the `one_for_all` / `rest_for_one` analogue) counts once or N times. OTP counts it once (`supervisor.erl:1333-1352`).
- **Epochs (§3) and their prior art.**
  - Shepherd's PID-match guard (`service.scm:665-687`) is the same idea keyed on the PID.
  - An epoch is stronger, because PIDs are reused.
- **Acquisition and stopping.**
  - dinit's `required_by` count and its two-phase propagation then execution (`service.h:59-128`) exist to stop a service from restarting on a stale count.
  - The draft's "DepLost in the same step" should be checked against that exact race.
- **Timeouts.**
  - s6-rc waits forever without `timeout-up`, and supervisord sits in STOPPING forever.
  - Every transient state needs a finite timeout. The draft's P1–P3 liveness properties depend on it.
- **Kubernetes `initialThreshold`.** This field does not exist (`types.go:2705-2741`). The fields are `initialDelaySeconds`, `successThreshold` and `failureThreshold`.

## 14. Not opened or unverified

- the daemontools page (HTTP parse error);
- the Shepherd manual (403 and 500);
- the Upstart cookbook (DNS);
- the Pebble docs (404);
- the Candea & Fox PDF (TLS);
- Shuttleworth's "Losing graciously";
- the follow-ups to the PROLE 2010 work;
- Spichkova (arXiv 1405.3017);
- Armstrong §8.3.5;
- dinit's control-socket authorization;
- 66's skalibs removal.
