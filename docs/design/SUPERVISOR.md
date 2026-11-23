# go-supervise/supervisor — a cross-platform service supervisor with stated semantics

Status: **DRAFT 9, 2026-11-23. The FSM has no code; `Compile` of draft 8 is implemented in go-supervise/supervisor.** Draft 9 resolves `REVIEW-5.md` (F1–F11 and its Gate 1 specification; map in §12): svd's own resources partitioned per principal (H_OS-E5, F1), "runs as p" defined on resolved ids and granted services held to p's privileges (F2), goal propagation truncated at every grant boundary for every principal (F3), three principal classes (F4), P2 split into P2a (Gate 1) and P2b (Compile) (F5), the guard grid's `Absent` value and priority (F6), the timer columns renamed (F7), and Gate 1's non-interference check as 2-safety by self-composition. Every draft-9 rule that `Compile` must newly enforce is tagged **[Compile+9]**, and every new host-side check at load or before a fork **[Host+9]**; §12 lists both. Draft 8 resolved `REVIEW-4.md` (E1–E14; map in §12): `run_as` = delegated principal ⇔ granted (E1), grants closed under the reverse of every edge kind (E2), H_OS split into what svd enforces and what it assumes (E6), Theorem G re-proved, the payer's overflow in `charge` (E3), the sojourn bit `armed` (E4), P2 restated (E5). Draft 7 recorded the user's decisions on D2 (option A), D-M2 and the binary name (`svd`). This draft resolves every finding of
`REVIEW-1.md` (C1–C2, H1–H5, M1–M11, L1–L4), of `REVIEW-2.md` (N1–N16) and of `REVIEW-3.md` (D1, D3–D7, L1–L10). REVIEW-3's D2 (Theorem G) was decided by the user as option A (§13), and Theorem G holds under its OS-privilege hypothesis. Finding IDs are cited inline; REVIEW-3's low items are written R3-L1…R3-L10 to keep them apart from REVIEW-1's L1–L4. The
bibliographies in this directory are cited as A§n (`BIBLIO-A.md`) and B§n
(`BIBLIO-B.md`). Items marked *(unverified)* were not opened by anyone.

**Home:** the GitHub org `go-supervise`, repository `supervisor` (N16). The user created the org on 2026-11-23 at 14:01, which supersedes the earlier `go-zpkg/supervisor` idea that REVIEW-1 and REVIEW-2 were briefed with. **The binary is `svd`** (user decision, 2026-11-23); drafts up to 6 used a placeholder name.

User decisions (2026-11-23), applied in this draft:

| decision | where it applies |
|---|---|
| R0: no relaxation | §1.4 |
| API grants only shrink across layers (K5) | §1.8, §8 |
| no macOS credentials on disk | §6 |
| rollback by default | §2.4 |
| s6 output decided after v0.1.0 | §11 |

User direction (2026-11-23):

- Platforms: Linux, macOS, Windows, then FreeBSD.
- Configuration in HCL2.
- No compatibility with Pebble.
- Correctness first.
- The supervisor runs nested behind any init, and can be managed without root.

---

## 0. Rejected: a Pebble-compatible model

Pebble's model was rejected because:

- its layers are order-dependent: its merge is "associative only" (B§12);
- it is non-monotone: `-name` removes and `replace` overwrites;
- its defaults are hidden;
- the pkgx precedent shows what compatibility costs.

systemd drop-ins share the first two defects (A§1.1).

---

## 1. Configuration: composition by meet, under authority

### 1.1 Sources

- **CUE.** Unification is the meet of a value lattice: commutative, associative and idempotent, with `{a: 1} & {a: 2}` = `_|_` (cuelang.org/docs/reference/spec, B§11).
- **Nix.** `mergeEqualOption` is our K1 (B§5, options.nix:498-519). Nix lists and `mergeAttrs` are not commutative, and `mkForce` is not monotone (B§5.2).
- B found no prior paper on lattice configuration merging (B§11). The proofs below are self-contained.

### 1.2 Operator, laws, and the conflict element (M1)

A **layer** is a partial description. A field the layer does not mention is ⊤. Each field kind K is a meet-semilattice, extended with a separate error element `⊥err`:

- `⊥err` sits below every value and absorbs under meet.
- `⊥err` is **not** the order's least legitimate value.
  - In K1 and K2 the meet itself can produce `⊥err`, from unequal values or an empty interval.
  - In K3–K7 the meet is total: `capabilities = ∅`, `0`, ✓ and `require` are ordinary values.
  - In those kinds, `⊥err` arises only from the post-meet predicates of §1.6.

**Proof that adjoining ⊥err keeps the laws.**

- In K1, the meet of a multiset is defined as v when every non-⊤ operand equals v, and as ⊥err otherwise. That definition is symmetric, and insensitive to grouping and repetition.
- In K2, let h map ∅ to ⊥err and every non-empty interval to itself. ∅ absorbs under ∩, so h is a homomorphism. ∩ is ACI, and the image of an ACI operation under a homomorphism is ACI.
- K3–K7 never produce ⊥err by meet, so adjoining an unreachable absorbing element changes nothing.
- Hence the laws hold for all kinds, with Z stated against ⊥err. ∎

| law | statement |
|---|---|
| A | (a⊓b)⊓c = a⊓(b⊓c) |
| C | a⊓b = b⊓a: layer order and file names carry no meaning |
| I | a⊓a = a |
| U | a⊓⊤ = a |
| Z | a⊓⊥err = ⊥err: a conflict anywhere is a conflict in the plan |
| L | a⊓b ⊑ a |

Law Z is fuzzed against ⊥err, never against `∅` or `0`.

### 1.3 Field kinds

| kind | domain and order | meet | ⊤ | can the meet give ⊥err? |
|---|---|---|---|---|
| K1 flat | incomparable values | equal → v; unequal → ⊥err | unset | yes |
| K2 interval | non-empty closed intervals of a total order, ordered by ⊆ | ∩; ∅ → ⊥err | [0, ∞] | yes |
| K3 bound | a chain, smaller is tighter | min | +∞ | no |
| K4 protection | chain ✓ (required) ⊑ waived ⊑ ⊤ | min | ⊤ | no |
| K5 allow-set | ⊆ | ∩ | universe | no |
| K6 demand-set | ⊇ | ∪ | ∅ | no |
| K7 edge | chain require ⊑ want ⊑ order ⊑ ⊤ (none) | min | none | no |

Notes on K4, K5 and K7:

- **K4 waived.** `waived` is the explicit "not required". It exists so that completeness (§1.5) can be satisfied without requiring the protection. The literal `false` is refused as `ErrNotALatticeValue`.
- **K5 path regions** (M11). A path region meets **symbolically**: the meet of allow-regions is the ACI *set of operands*, and enforcement is the conjunction of those operands. The intersection is computed only on the target host by the exec-helper, under that filesystem's case and link rules (§7.2). Compile never has to decide `/Users/x ∩ /users/x`.

Theorem P (product): structs and maps meet pointwise, so the laws lift componentwise. ∎

There is no `replace`, no `remove` and no ordered list, because each is either non-monotone or non-commutative.

### 1.4 Monotone hardening under R0 (user decision), and its honest cost (M2)

There is **no relaxation construct**. Theorem M (§1.8) holds without exception.

**What R0 costs. These three things cannot be done by any site layer:**

1. loosen a protection, limit or allow-set stated by any layer;
2. change a definition field (class D, §1.7);
3. move a timing or count outside the interval a recipe stated.

The only recourse is a new signed recipe revision. That is the price of a theorem without exceptions, and the user accepted it on 2026-11-23.

**Mitigation: recipes state intervals and a preference, not points.**

- For every class-T field (timing and counts), a recipe states an interval and a `prefer` point inside it. Example: `stop.timeout = { min = "5s", max = "120s", prefer = "30s" }`.
- A site may then choose any value in the interval, and composition remains a meet.
- **Normalisation (N10).** A point `"60s"` is written sugar for `{ min = 60s, max = 60s, prefer = 60s }`. Two spellings of one intent are therefore one value.
- **Site prefers take precedence (D-M2, decided; replaces draft 6's R3-L6 rule).** Prefers are kept in two sets, per authority: Π_r from recipe layers and Π_s from site layers. A site may therefore **narrow** the interval (the meet) and **move** the preferred point inside the met interval, but it can **never widen**, because the resolved value always lies in the met interval I. A site `prefer` no longer adds a second element to the recipe's set, which was the REVIEW-3 L6 conflict. `all {}` still admits no free `prefer`: one point cannot fit every service's interval.
- **Prefers form a K6 set (N10).** A field's `prefer` values are collected as a set (∪), not met as K1.
- **Compile phase order (N10):**
  1. **meet**, of every field, over the set of layers;
  2. **resolve** each class-T field from its met interval I and its prefer sets Π_s (site) and Π_r (recipe):
     - **first, unconditionally (E10):** if Π_s ⊄ I, some site prefer lies outside the met interval → `ErrConflict` (a site preference may not widen, and two sites that disagree are not resolved by a third narrowing). This test runs even when I is a singleton;
     - then, if I is a singleton, use it;
     - else if Π_s ≠ ∅ (now Π_s ⊆ I): exactly one element → use it; several → `ErrConflict` (two sites disagree);
     - else if Π_r ∩ I has exactly one element, use it;
     - else if Π_r ∩ I is empty → `ErrIncomplete` (the met interval excludes every recipe preference, and no site chose a point);
     - else → `ErrConflict` (several recipe preferences remain);
  3. **post-meet predicates** (§1.5 completeness, §1.6). They read resolved values, including `ErrWaitTooShort` and the StopWait bound SW.

  Consequence: a site writing `"60s"` against a recipe's `{5s, 120s, prefer 30s}` gets 60 s. The met interval is the singleton [60s, 60s], and the site's point form contributes Π_s = {60s} ⊆ I, so the E10 test passes. A second site stating `{5s, 120s, prefer 30s}` beside it gives Π_s = {60s, 30s} ⊄ [60s, 60s] → `ErrConflict`.
- **Laws:** the meet is untouched. Resolution is a function of the met value (I, Π_s, Π_r), and the authority of a layer is part of its identity, so `Compile` stays a function of the set of (layer, authority) pairs (§1.10). The resolved point lies in I (law L), so **no layer can widen a class-T field (R0)**.
- **Theorem M:** untouched, because class T holds no hardening field.
- **D-M2, decided by the user (2026-11-23): intervals plus a `prefer` point.** `prefer` is a stated, visible value, and the one sanctioned departure from "defaults are ⊤ or ⊥": it is confined to class T, it is always inside the met interval, and `svd plan` reports which layer's prefer was used. Strict pinning was rejected because it makes every deployment restate every timing field.

### 1.5 Fail-closed completeness and the universal baseline (C1)

**Theorem N (no restrictive absence).** No assignment of a meaning to an absent field can at the same time:

- keep law U;
- keep Theorem M (⊑ means "at least as restrictive", per §1.9);
- make an absent field fail closed.

*Proof.*

- Let a be the value that absence denotes. U requires a ⊓ x = x for every x, so x ⊑ a for every x, and a = ⊤.
- Under Theorem M's reading, ⊤ is the *least* restrictive value.
- Absence would therefore have to be the least and the most restrictive value at once. That is possible only in a one-element domain.
- The other way out, dropping U, means that a layer which is silent on a field changes the plan. The meaning of a partial layer is then lost, along with the property that layers about other things do not interact. ∎

Re-orienting the lattice so that absence is restrictive is therefore rejected. **Fail-closed is obtained by a completeness predicate over the result**, which the laws do not see. The construction:

1. **The baseline is a meet, not a file.** B is defined as the meet of the `all {}` blocks of **all site-authority layers**, together with their instance blocks (`api`, `schema`):

   `B = ⊓ { L.all : L a site layer }`

   No file name carries meaning, so law C holds.
   - If no site layer has an `all {}` block, B = ⊤ → `ErrNoBaseline`.
   - B must be **total** over the hardening schema and the instance fields of its declared schema version (N9 below), or the result is `ErrBaselineNotTotal{fields}`. Totality covers:
     - every K3 bound;
     - every K4 protection (✓ or `waived`);
     - every K5 allow-set, with a concrete set;
     - every protection's `exempt_on`;
     - `api.admin` and `api.read`.
   - Totality is checked on the meet, not on any single file.
2. **Universal application.** For every service key s, the plan is

   `Plan(s) = B ⊓ (⊓_i L_i(s))`

   This is still a meet over a set, so ACI holds. **Every service meets B whatever layer defines it**, which closes the "baseline only constrains named services" hole.

   **What `all {}` may contain (N6, R3-L3).** Only:
   - class H (hardening) **except K6 `needs`**. A global `needs` is never a ceiling. It could only produce a mass `ErrConflict`;
   - class T, as intervals or the point form. A free `prefer` is excluded (N10, R3-L6);
   - policy (`restart.on`). A global `restart.on` is K1, so it gives `ErrConflict` against every recipe that states a different value. That is intended: it is a site assertion that every recipe agrees, not a default.

   Anything else inside `all {}` is `ErrFieldAuthority{field, "all"}`. That covers:
   - class D (`env`, `exec`, `run_as`), which would be a widening of every service;
   - class P;
   - edges (`all { require "x" }` would give x an edge to itself);
   - K6 `needs`;
   - a free `prefer`.
3. **Completeness.** After the meet, every field in the schema's required set Q must be non-⊤, or the result is `ErrIncomplete{path}`. Q contains:
   - every class-D field a service needs;
   - every hardening field;
   - every `exempt_on`;
   - the instance's grants.

   B's totality makes the hardening part automatic. The predicate reads only the met value, so it is a function of the set of layers.
4. **Required test.** Plans whose layers never mention some K5 field must hold B's value for it, never the universe. Without B, the result must be `ErrNoBaseline`.

5. **Schema evolution never widens (N9).**
   - Every site layer may state `schema = N` (K1, so all site layers agree). The instance schema version N_B is that value, and it is required.
   - Every field f introduced in schema version v_f > 1 must ship with a **legacy value** ℓ_f. ℓ_f is defined as the value whose enforcement under the new version equals the enforcement of version v_f − 1, where f did not exist:

     eff_{v_f}(f, ℓ_f) = eff_{v_f − 1}

     For a new protection, ℓ_f = `waived`. For a new allow-set, ℓ_f = the set of all behaviours the previous version did not restrict.
   - When v_f > N_B and B does not state f, Compile uses ℓ_f. Totality is checked against version N_B, so an older baseline is not refused.
   - When v_f ≤ N_B, B must state f: totality applies as before.
   - **Completeness is version-relative too (D5).** For a K3 limit ℓ_f = +∞, and for a K5 allow-set ℓ_f is the universe. Both are ⊤, which the non-⊤ test of item 3 would refuse. So the required set is Q(N_B): a field with v_f > N_B is not in Q(N_B). It is resolved to ℓ_f and **tagged `legacy`** in the plan. The tag is not a lattice value: it does not change the meet, only the completeness test and the report. Without this, a schema-1 baseline under a version that adds a K5 field would fail with `ErrIncomplete` and refuse to boot, which is the failure N9 was about.
   - **A baseline newer than the binary (D5).** N_B > the running schema version is `ErrSchemaTooNew{N_B, running}`. This fails closed: a field the binary does not know cannot be enforced, so it must not be silently ignored.
   - `svd plan` lists every field resolved to its legacy value (`legacy`). `svd validate --schema M` reports what a baseline would lack at version M.

   **Theorem E.** Upgrading svd without changing any layer never widens enforcement.

   **Hypothesis (D5).** A new version may not change the denotation of an existing field. That includes **the universe of an existing K5 field**: a behaviour that the old enforcement permitted implicitly, because the field did not enumerate it, may not become a new member of that field's universe. It must become a new field, with its own ℓ_f. Otherwise a baseline set that was a ceiling under version N would stop covering that behaviour under version N+1. A test pins eff per field, per version and per universe.

   *Proof.*
   - For a field f that already existed, the met value is unchanged, and so is its denotation, by the hypothesis.
   - For a new field f with v_f > N_B, its value is ℓ_f, or ℓ_f ⊓ (recipe values) when a newer recipe states it. Because f may only meet below ℓ_f, eff ⊆ eff(ℓ_f) = eff_{v_f−1} (§1.9 monotonicity).
   - So eff after the upgrade ⊆ eff before it, field by field. ∎

   - Why this does not reopen C1: ℓ_f is not "absent means everything". It is "absent means exactly what was enforced before this field existed", and it applies only to baselines that declare an older schema. A baseline that declares N_B ≥ v_f must state f.
   - The upgrade is also **not a hardening**: protecting with a new field requires the site to state it, or a recipe to require it.

Consequences:

- **K5 is now a ceiling.** For example, B states `capabilities ⊆ {CAP_NET_BIND_SERVICE}`, and recipes can only narrow it.
- **exempt_on is per protection** (M3). Each protection is a block, `protect "landlock" { level = ✓, exempt_on = ["macos", "windows"] }`. `exempt_on` is a K5 set of OS names, required in B. A recipe can therefore exempt NNP on macOS while requiring Landlock everywhere else.

### 1.6 Post-meet predicates (M1)

Each predicate yields ⊥err. Each reads only the met plan.

- **needs ⊆ allows.** Every K5 allow-set is paired with a K6 `needs` set. If `needs ⊄ allows`, the result is `ErrConflict{field, needs, allows}`. A recipe that needs `inet`, met with a site that allows only `unix`, is now a conflict rather than a valid plan for a daemon that cannot run. For path regions this check runs on the host (§7.2).
- **Check timing.** 0 < timeout < period, and rise and fall ≥ 1 (`ErrCheckTiming`).
- **Wait budget** (M5, N4, D6). WaitTO only covers **require-targets** (§3.5: want- and order-targets settle on their first failure, and their own timers bound them). But a require-target B may itself wait on **any** of its targets, so the time B takes to come up recurses over every edge kind. Two quantities:
  - the one-attempt time of B,

    U(B) = W(B) + SW(B) + stop(B) + kill(B) + backoff.max(B) + start(B)

    where the terms account, in order, for B's own wait, a previous incarnation of B still stopping, one backoff and one start;
  - the wait of A, over **all** of A's targets,

    W(A) = max over C ∈ req(A) ∪ wnt(A) ∪ ord(A) of U(C)

    with W = 0 for a service without targets.

  A want- or order-target contributes its one-attempt time to W, but it never sends its wanter to Failed (§3.5). The predicate applies to every service with a require-target, because only those arm WaitTO, and it is checked against **all** of A's targets (E9):

  `req(A) ≠ ∅ ∧ wait.timeout(A) < W(A)` is `ErrWaitTooShort`, computed on resolved values (§1.4 phase order).

  *Why W(A) and not the require-targets alone (E9).* With A require B and want C, a budget sized on U(B) alone lets one lawful attempt of C outlast `wait.timeout(A)`: WTO fires with B Ready, sets `wexp`, and a later restart of B inside B's own budget fails A at once. With wait.timeout(A) ≥ W(A) ≥ U(C), one attempt of each target fits in the sojourn's budget, so `wexp` is set only when some target took more than one attempt. A want-target restarted by an operator during A's sojourn is more than one attempt; that is the operator's act, and it is inside one principal's grant or wholly outside every grant (§8.2, E2).

  The REVIEW-3 counter-example (A require B, B want C, C mid-restart) is covered: C's attempt is inside W(B), hence inside U(B). The budget covers **one** failed attempt of each upstream service, through every edge kind. More flapping sends A to Failed(wait), which recovers automatically (§3.5). That is the stated behaviour of a hard dependency.
- **Typed parameters** (N7). A `param` must declare a typed domain, one of:
  - integer interval;
  - enumeration;
  - port;
  - duration interval;
  - path region (§1.3 K5).

  The domain is in the completeness set Q. A `param` without a domain, or with a free-form string domain, is `ErrUntypedParam`. A site value outside the domain is `ErrConflict`. Values are substituted into argv as single arguments, never through a shell.

  **svd's "no shell" does not bind the recipe (R3-L4).** A recipe could run `sh -c "… $p"` itself. Two rules close that:
  - **Path values have a restricted alphabet, per target OS (E12):**
    - POSIX (Linux, macOS, FreeBSD): `[A-Za-z0-9._/+@,=-]`, with no leading `-`;
    - Windows (P3): an optional drive prefix `[A-Za-z]:\`, then `[A-Za-z0-9._\\+@-]`, with no leading `-`. `,` and `=` are excluded because cmd.exe treats them as argument separators, and `/` because Windows programs read it as an option prefix; `:` is allowed only in the drive prefix (it also names NTFS alternate data streams).

    A path outside its OS's alphabet (a space, `;`, a quote, `$`, `%`, a backquote, a newline or NUL, among others) is `ErrConflict` against the domain. A path domain names its OS family, so one recipe can declare both.
  - **No parameter inside a shell command string.** When `exec[0]` resolves to a known shell (`sh`, `bash`, `dash`, `zsh`, `cmd.exe`, `powershell`, `pwsh`) and argv holds `-c` or `/c`, a parameter substituted into the command-string argument is `ErrParamInShell`. Parameters passed as positional arguments after the command string (`sh -c '… "$1"' sh <param>`) remain allowed.
- **Mode U run_as.** In mode U, `run_as` ≠ the instance owner gives `ErrRunAsInUserMode`.
- **Principal classes (F4).** A principal is an account, identified by its **resolved id**: the uid on POSIX, the SID on Windows (F2b). Every API principal is in exactly one class:
  - **owner-class** = `api.admin` (the instance owner is always in it, §8.2). An owner-class principal has grant(·) = ∅. It is trusted as the owner is: "owner" below means any owner-class principal;
  - **delegated** = the principals p with grant(p) ≠ ∅. They are disjoint from `api.admin`;
  - **readers** = `api.read` minus the two classes above. They issue no write.

  **[Compile+9]** A principal in `api.admin` with grant(p) ≠ ∅ is `ErrPrincipalClass{principal}`. **[Host+9]** The same test is repeated at load on resolved ids, so two names of one uid (FreeBSD `toor` and `root`, or a duplicated passwd entry) are one principal, and a delegated principal whose id is 0 (on Windows SYSTEM, or a member of Administrators *(unverified)*) is `ErrDelegationPrivilege{-, principal, "uid0"}`.
- **Delegated subtree** (L2, E1, F2b, F4). For every service X and every **non-owner-class** principal p (delegated or reader):

  runs_as(X, p) ⟺ X ∈ grant(p)

  where **runs_as(X, p)** means: the resolved uid (SID) of run_as(X) equals the id of p, and the group set of X's processes is exactly p's resolved primary and supplementary groups. For a reader r, grant(r) = ∅, so no service runs as a reader. Both directions are checked, and a violation of either is `ErrDelegationUid{service, principal, direction}`, with direction ∈ {granted, outside-grant, reader}:
  - ⇐: a granted service runs as its grantee;
  - ⇒ (E1): no service outside grant(p) runs as p. Without it, an owner service X ∉ grant(p) with run_as(X) = p could be crashed by p with `kill -SEGV`, and, if terminal or required by a terminal service, escalate the instance;
  - ⇒ for readers **[Compile+9]** (F4): an owner service run as a reader r could be crashed by r in the same way.

  **Where it is checked (F2b).** `Compile` checks the names, as in draft 8. **[Host+9]** At load, svd resolves every `run_as` and every principal to ids and checks the ⟺ on ids (`ErrDelegationUid`). Before every fork it resolves `run_as(X)` again; an id different from the load-time one is an SEp (`ErrAccountChanged{service}`, §3.1 O3), so a renumbering after load fails closed. After `setuid`, the exec-helper of a granted service checks that the real, effective and saved uids and gids, and the supplementary group list, are exactly p's; any difference is `perm` (O4).

  Consequences: grants of distinct principals are disjoint (a service in both would run as two accounts); a terminal service, never in a grant (§8.2), never runs as a delegated principal; in mode U, where run_as is always the owner, every grant is empty.
- **Delegated privilege (F2a) [Compile+9].** A granted service holds no privilege beyond its grantee's. For every A ∈ grant(p):
  - the met K5 `capabilities` of A is ∅, otherwise `ErrDelegationPrivilege{A, p, "capabilities"}`. On Linux the exec-helper then clears the permitted, effective, inheritable and ambient sets **and** the bounding set (`PR_CAPBSET_DROP` for every capability) before `execve` *(the bounding-set step is from memory)*. On Windows, `capabilities` denotes the token's privileges; a granted service's token keeps only `SeChangeNotifyPrivilege` *(mapping unverified, P3)*, so `SeDebugPrivilege`, `SeTcbPrivilege`, `SeImpersonatePrivilege`, `SeBackupPrivilege`, `SeRestorePrivilege`, `SeTakeOwnershipPrivilege` and every other one are refused by the same error. macOS and FreeBSD have no per-process capability set in this schema; `capabilities` must still be ∅;
  - the K4 protection `nnp` of A is ✓, with `exempt_on` ⊆ {macos, windows}, otherwise `ErrDelegationPrivilege{A, p, "nnp"}`. NNP keeps set-user-ID files and file capabilities from raising the tree above p's uid (Linux `PR_SET_NO_NEW_PRIVS`, FreeBSD `PROC_NO_NEW_PRIVS_CTL` *(unverified)*). macOS has no such mechanism in §5.3, and Windows has no set-user-ID exec; there, a raise through an executable is one p could make from its own login too, so it falls under H_OS-A1's "p cannot raise its own privilege", stated per OS;
  - A has no `run_as.group` or supplementary-group setting other than p's own (`ErrDelegationPrivilege{A, p, "groups"}`).

  This is an allow-list (everything granted must be ∅ or p's own), as §1.7 does for rights, so a capability or privilege missing from any list is refused too. **H_OS-A1 then shrinks to the OS's uid rule** (§3.3).

### 1.7 Layer authority (C2)

**Authorities.** A layer has exactly one authority, determined by *where it was loaded from and how it was verified*, never by its content.

| authority | source | admission check |
|---|---|---|
| **recipe** | a recipe artifact | signature verified against the instance's trust root before parsing; otherwise `ErrUnsignedRecipe`. The signature format follows the recipe pipeline *(open)* |
| **site** | the instance's site directory | ownership check, as in ssh's StrictModes: every path component and every file is owned by root or the instance owner and is not group- or world-writable; otherwise `ErrUnsafePath{path, owner, mode}` |
| runtime | the API | adds and removes **holders** (§2.3) only, never layers |

**The ownership check reads ACLs and works on fds (N8).**

- The walk opens each component with `openat(…, O_NOFOLLOW)` and `fstat`s the fd, so the object checked is the object read.
- A component fails the check if **any** principal other than root (or SYSTEM/Administrators) and the owner can write it. **"Write" means any right that can change the content, the name, the permissions or the owner (R3-L5).** A right to change the owner or the ACL is a write, because it yields every other right. The check is evaluated on:
  - Linux: the mode bits. A POSIX ACL's mask appears in the group bits *(unverified)*.
  - macOS: the mode plus the extended ACL, through `acl_get_fd` *(unverified)*. An allow ACE for another principal fails the check if it grants any of `write`, `append`, `delete`, `delete_child`, `add_file`, `add_subdirectory`, `writeattr`, `writeextattr`, `writesecurity` or `chown`.
  - FreeBSD: the mode plus POSIX.1e or NFSv4 ACLs (`acl_get_fd_np`) *(unverified)*. For NFSv4 ACLs the list is the same as macOS's, under FreeBSD's names (`write_data`, `append_data`, `delete`, `delete_child`, `write_acl`, `write_owner`, …).
  - Windows: the DACL. There are no mode bits; `GetSecurityInfo` is used *(unverified)*. An allow ACE for another SID fails the check if it grants any of `FILE_WRITE_DATA`/`FILE_ADD_FILE`, `FILE_APPEND_DATA`/`FILE_ADD_SUBDIRECTORY`, `FILE_WRITE_EA`, `FILE_WRITE_ATTRIBUTES`, `FILE_DELETE_CHILD`, `DELETE`, `WRITE_DAC`, `WRITE_OWNER`, or a generic right that maps to one of them (`GENERIC_WRITE`, `GENERIC_ALL`).
  - The lists are closed by construction: the check **allows** only the read and execute rights and fails on anything else, so a right missing from the lists above still fails.
- **Per-user instances.** Any process running as the owner's uid can write the site directory, exactly as it can write `~/.config/systemd/user`. Mode U gives no protection against the owner's own processes, and none is claimed.

**Who owns a layer, per instance:**

- **System instance:**
  - site directory root-owned, 0755, files 0644;
  - trust root root-owned.
- **User instance (mode U):**
  - site directory owned by the user, 0700;
  - the user may add keys to their own trust root. They gain nothing beyond their own uid (§1.6, mode-U run_as).

**Field classes, and what each authority may do with them:**

| class | fields | recipe | site |
|---|---|---|---|
| **D** definition | the service set; `exec`; `run_as.*`; `working_dir`; env **keys**; `kind`; `readiness.channel` and liveness kind; `stop.signal`; credential **names**; declared parameters | writes | may only restate an existing value: idempotent, otherwise `ErrWideningField`. A new service key or env key is a widening |
| **P** parameter values | values of the recipe's declared `param`s (argv and env values); credential **sources** | declares the domain (interval or enumeration) | concretizes, inside the domain |
| **T** timing and counts | every K2 field | interval and `prefer` | meets |
| **H** hardening | K3 limits; K4 protections; K5 allow-sets and their K6 needs; `exempt_on` | meets | meets |
| **E** edges | `require`, `want`, `order` | meets | meets |
| **G** instance | `api.admin`, `api.read` (K5); instance intensity; per-principal budgets (§3.3) and per-principal resource budgets (H_OS-E5, F1); `enabled`; `terminal`; `schema`; `all {}` (contents restricted, §1.5) | **forbidden** (`ErrFieldAuthority`) | writes |
| policy | `restart.on` (K1 ∈ {never, on-failure, always}) | meets | meets |

Notes:

- **Edges.** A site may add an edge because an edge adds activation and ordering, never privilege. The site owner can start any defined service through the API anyway.
- **Credential sources.** Only the site binds a credential name to a host secret, and the site is the owner of those secrets. A recipe cannot name a host secret. A site cannot invent a credential name, because that is class D.

### 1.8 Theorem M, per authority (C2)

Let P = Compile(Rs ∪ Ss), where Rs are recipe layers and Ss are site layers.

- **(M-H)** For every layer L of either authority and every hardening field f: P(f) ⊑ L(f). This is law L restricted to class H.
- **(M-D)** For every service s and every class-D field f: P(s).f = Compile(Rs)(s).f, or P = ⊥err. Proof:
  - site layers may only restate class-D values;
  - a site value equal to the recipe value is absorbed (law I);
  - an unequal value, or a new key, is `ErrWideningField`;
  - the service set is class D. ∎
- **(M-P)** Every parameter value lies in its recipe-declared domain (meet with the domain).
- **(M-G)** Grants are K5, written by site layers only, and P.api ⊑ every site layer's grants. They can only shrink.
- **Corollary.** No layer, of any authority, causes a service to:
  - run code, under an identity, with an environment variable, or receiving a credential that no recipe declared;
  - run with a protection, limit or allow-set looser than any layer stated;

  and no layer can make a principal an API user that some site layer did not grant.

  Each of REVIEW-1's counter-examples fails:
  - `LD_PRELOAD`: an env key, class D → `ErrWideningField`;
  - `run_as = root`: class D;
  - a new service: class D;
  - a borrowed credential: names are class D, sources belong to the owner.

**Required test.** For every class-D field, a site layer that changes it or adds a key must produce `ErrWideningField`.

### 1.9 What is enforced is monotone (M3)

Theorem M is about ⊑. It speaks about the running system only if enforcement is monotone. Let eff_os(f, v) be the set of behaviours that value v of field f permits on that OS. The required property, per field and per OS, is:

**v ⊑ w ⇒ eff_os(f, v) ⊆ eff_os(f, w).**

| field | denotation | monotone because |
|---|---|---|
| K3 limits | rlimit, cgroup or job value | a smaller limit permits a subset |
| K4 protections | ✓ applies the mechanism; waived and ⊤ apply nothing; on an OS in `exempt_on`, ✓ is waived when the mechanism is unavailable | applying a mechanism removes behaviours |
| K5 `capabilities`, `syscalls`, `address_families` | the allow-list given to the mechanism | a subset allows a subset |
| K5 `read_write` regions | paths that may be opened for writing | as above |
| K6 `read_only` regions | paths remounted read-only, or denied write by Landlock | **they never make a path visible**: visibility is outside the schema (v1 has no "hide" field), so adding a region only removes write behaviours |
| K5 `exempt_on` | OSes where an unavailable mechanism is tolerated | a smaller set refuses on more OSes |

Fields that are not restrictions are not listed under Theorem M. Their guarantee is authority (§1.8):

- `terminal` and `enabled` are class G;
- credentials are class D/P.

**Test.** For each field and each OS in CI, a property test applies v ⊑ w and asserts that a probe workload allowed under v is also allowed under w.

### 1.10 HCL2 surface, and errors

- Syntax only. Literals are the only expressions; there are no variables and no functions.
- Intervals are written `"10s"` or `{ min, max, prefer }`.

The typed errors:

- `ErrUnknownKey`;
- `ErrNotALatticeValue`;
- `ErrConflict{path, values, layers}`;
- `ErrIncomplete{path}`;
- `ErrNoBaseline`;
- `ErrBaselineNotTotal`;
- `ErrFieldAuthority{field, authority}`;
- `ErrWideningField{service, field, layer}`;
- `ErrUnsignedRecipe`;
- `ErrUnsafePath`;
- `ErrCycle{services}`;
- `ErrUnknownService`;
- `ErrCheckTiming`;
- `ErrWaitTooShort`;
- `ErrProtectionUnavailable{field, os}`, evaluated on the host;
- `ErrSecretInEnvironment`;
- `ErrNoSuchAccount`;
- `ErrRunAsInUserMode`;
- `ErrDelegationUid{service, principal, direction}` (L2, both directions since E1);
- `ErrForkingDaemon`;
- `ErrUntypedParam` (N7);
- `ErrParamInShell` (R3-L4);
- `ErrSchemaTooNew{baseline, running}` (D5);
- `ErrGrantNotClosed{principal, granted, dependent, edge}` (D2, E2, §8.2), with edge ∈ {require, want, order};
- `ErrPrincipalClass{principal}` (F4, §1.6) **[Compile+9]**;
- `ErrDelegationPrivilege{service, principal, what}` (F2a, §1.6), with what ∈ {capabilities, nnp, groups} **[Compile+9]**, and {uid0} **[Host+9]**;
- `ErrNeverFailingLoop{service, window, need}` (F5, §3.7 P2b) **[Compile+9]**;
- `ErrIncomplete` now also covers the per-principal resource budgets of every delegated principal (H_OS-E5) **[Compile+9]**;
- `ErrResourceBudget{resource, need, have}` and `ErrIsolationUnavailable{resource, os}` (H_OS-E5), evaluated on the host at load **[Host+9]**;
- `ErrAccountChanged{service}` (F2b), an SEp before a fork **[Host+9]**.

One draft-9 diagnostic is a **warning**, not an error **[Compile+9]**: `WarnUnheldOutsideTarget{service, target}`, when an enabled granted service requires an outside service that is not enabled (F3, §2.3). It is reported by `svd validate` and `svd plan`, and does not refuse the plan.

These are API errors, not Compile errors:

- `ErrStillWanted{by}` (N12);
- `ErrOutsideGrant{services}` (N2);
- `ErrPrincipalBudget` (N2);
- `ErrQuiescing`;
- `ErrLeaked`.

`Compile` is a function of the set of (layer, authority) pairs, and is the only constructor of `Plan`.

---

## 2. Dependency graph

### 2.1 Three edge kinds (M4)

| edge A→B | activates B | A starts after B has settled | I1 (A runs only while B is Ready/Done) | A fails if B fails |
|---|---|---|---|---|
| `require` | yes | yes (B Ready or Done) | yes | yes |
| `want` | yes | yes (B Ready, Done or Failed) | no | no |
| `order` | no | yes, if goal(B) = up | no | no |

Constraint order: require ⊑ want ⊑ order ⊑ none (K7).

How this maps to other systems:

- dinit's `depends-on` = `require`.
- dinit's `waits-for` = **`want`**. It "activates the target" and waits for start or failure (BIBLIO-B.md:126). Draft 3 mapped `waits-for` to `order`, which was wrong.
- dinit's `after` = `order`.
- systemd's `Wants=`+`After=` = `want`.
- dinit's `depends-ms` has no counterpart.

systemd keeps requirement and ordering orthogonal (`man/systemd.unit.xml:670` @ v262.2). That makes `Requires=` without `After=` a footgun (A§1.11). Here, every activating edge orders.

### 2.2 Load time

- The union of all edges must be acyclic; otherwise `ErrCycle`.
- systemd instead deletes a job to break an ordering cycle (`src/core/transaction.c:412`, :492 @ v262.2). That has caused machines to boot without networking (A§1.11).
- **Out of scope for v1:**
  - socket activation;
  - mutually-calling peers (A ↔ B). These must be written without an edge, with each daemon retrying its peer itself.

### 2.3 Wanted set: holders (M6), boot set (M4)

- `x(A)` is a **set of holders**. A holder is one of:
  - `boot`, added at instance start iff `enabled(A) = ✓` (class G);
  - a request ID, added by `start`.
- goal(A) = up ⟺ x(A) ≠ ∅ ∨ ∃A′: goal(A′) = up ∧ edge(A′, A) ∈ {require, want} ∧ ¬cut(A′, A). This is the least fixpoint over the DAG.
- **Goal truncation at every grant boundary (draft 9, F3; replaces draft 8's per-principal holder scope).** An edge is **cut** when it leaves a grant:

  cut(A′, A) ⟺ ∃p: A′ ∈ grant(p) ∧ A ∉ grant(p)

  The cut applies to **every** holder, whoever created it: the owner's requests and `boot` included. Draft 8 restricted only delegated holders, so an owner holder on a granted service still raised an outside goal, and p could lower it again by `stop`, or by stalling the owner's transaction into rollback (REVIEW-5 F3, routes 1 and 2).
  - *Which edges are cut.* By grant closure (§8.2, E2) no edge enters a grant from outside it, and grants are disjoint (§1.6), so an edge between two grants is impossible. The cut edges are therefore exactly the edges from a grant to O, the set of services in no grant.
  - *What it gives.* For C ∈ O, goal(C) is a function of the holders on services of O alone: every uncut path into C starts in O, because a path that enters a grant can leave it only by a cut edge. For A ∈ grant(p), goal(A) is a function of the holders on grant(p) alone. Every transaction's member set (§2.4) therefore lies wholly in one grant or wholly in O.
  - *The cost, stated.* A granted service's outside require-targets are not raised by it. They must be held on their own, for example `enabled`, or started by the owner. A granted A that requires an outside C whose goal is down waits for C and reaches Failed(wait) locally; `svd status` reports C as "held down outside grant". **[Compile+9]** When an `enabled` granted A requires an outside C that is not `enabled`, `Compile` reports `WarnUnheldOutsideTarget{A, C}` (§1.10): the plan is valid, but A cannot come up at boot unless the owner starts C.
- **Who may remove which holders (F3).** The holders on a service of O are created and removed only by owner-class operations, by the rollback of an owner transaction whose members lie in O, and by Quiescing. A delegated p's operations reach services of grant(p) only (§8.2 grant check), and there they may clear **any** holder, the owner's requests and `boot` included (`stop`, below): the services are p's subtree, and by the cut those holders raise nothing outside it. A `boot` holder cleared this way is re-added only at the next instance start.
- Goal depends only on holders and edges, never on states. It is therefore **fixed during a step's propagation**.
- **`stop X` (N12).** Let D(X) be the set of services that keep goal(X) = up through an uncut require or want edge, given current holders. For X ∈ O, D(X) ⊆ O, so neither the outcome of an owner's `stop X` nor its `ErrStillWanted` list depends on the state of any grant (F3).
  - If D(X) is non-empty, `stop X` is refused with `ErrStillWanted{by: D(X)}`. Clearing x(X) alone would leave X running, which is what REVIEW-2 found.
  - **Order of checks (R3-L7).** For a non-owner caller the grant check of §8.2 runs first, so a caller outside grant gets `ErrOutsideGrant` and learns nothing about D(X). Then `by` is filtered to D(X) ∩ grant(p), plus a count of the others when the caller lacks `api.read`.
  - `stop X --with-dependents` clears the holders of X and of every service in the reverse uncut require/want closure that keeps X up. This is systemd's behaviour. For X ∈ grant(p) that closure lies in grant(p) (closure and cut), and for X ∈ O it lies in O.
  - Either way, every open transaction that held a cleared holder is aborted. Those transactions report `superseded` and do not roll back. When p's `stop` aborts an owner transaction this way, that transaction's members lie in grant(p) (§2.3 cut), so no outside service is affected (F3).
  - Both forms are subject to the grant check of §8.2.

### 2.4 Transactions and rollback (default on)

- `start X` creates request h, adds h to x(X), and opens a transaction T = {services whose goal goes down→up because of h}. Goals are computed with the cut of §2.3, so T lies wholly in grant(p) when X ∈ grant(p), and wholly in O otherwise (F3: the owner's `start A` with A ∈ grant(p) requiring C ∈ O gives T = {A}; C is not a member, and the rollback of T cannot lower C).
  - **T succeeds** when every member is Ready or Done.
  - **T fails** when a member reaches Failed, or when the request deadline passes.
- **On failure, rollback removes h, and only h.** The goal recomputes, and every service whose goal falls stops through the normal stop path, so T2 applies.
  - Services held by other holders are untouched. That removes the lost update: a concurrent `start X --no-rollback` keeps its own holder.
- `--no-rollback` makes h permanent until `stop`.
- s6-rc has no rollback (B§12).
- Rollback is triggered by any Failed member, including Failed(wait) (§3.4). A request that does not complete within its deadline is undone. This is the intended semantics of rollback by default, and REVIEW-1 M5's "tears B down" is that intent.

---

## 3. Lifecycle: a total FSM over trees

### 3.1 Execution model

**Step.**

1. **One external event** is taken from the queue. Internal events (SEt, SEp) take priority. **Fairness across sources (F1, H_OS-E5):** external events are queued per source (each service's process, channel and timer events; each principal's API operations) in bounded queues, and the next event is taken round-robin across the non-empty sources, so no source can delay another's events by more than one round.
2. **Filter.**
   - Process and channel events carry an **epoch**.
   - Timer and liveness events carry a **generation**: a per-service counter incremented on *every* state entry (H4).
   - A mismatch drops the event before the table.
3. **Event transition**, from the event table (§3.4).
4. **Propagation**: guards (§3.5) are applied to a fixpoint, in pure state. Goals are fixed (§2.3).
5. **Execution**: the actions collected in phases 3–4 are emitted, such as signals, spawns and timer changes.

Notes:

- **Timers and windows (N1).**
  - **State timers** (WaitTO, StartTO, SWTO, StopTO, KillTO, BackoffTO) carry the generation.
  - **The restart windows are not events.** R_A(now) = {t ∈ R_A : t > now − intensity.window}, and likewise for R_inst and the principal budgets. They are evaluated at every charge and every check, and entries older than the window are pruned there. A crash per day therefore never reaches Failed(intensity) when max = 3 and window = 60 s.
  - **Liveness deadlines are level checks.**
    - The watchdog deadline carries the generation. When it fires, the liveness source evaluates `now − lastKeepalive ≥ watchdog`, and emits `Fall` only if that holds.
    - Otherwise the deadline re-arms for lastKeepalive + watchdog, and no event reaches the table. A deadline queued before a keepalive arrived therefore never yields Fall.
    - **Initial value (R3-L1).** On every entry to Ready, lastKeepalive := the entry time, and the deadline is armed for entry + watchdog. A keepalive left over from a previous Ready sojourn can therefore not extend this one.
    - **Keepalives are epoch-filtered (R3-L1).** A keepalive updates the source only if it carries the current epoch, by the same filter as phase 2. Keepalives never reach the table.
    - A probe result carries a per-run sequence number, and only the latest run counts.
  - **WaitTO is a level check, with one deadline per sojourn (D3).**
    - The Waiting sojourn has one wait deadline. It is armed, once, at the first moment in the sojourn when some require-target is not in {Ready, Done}. It is never disarmed or re-armed within the sojourn, so a toggling require-target does not restart the budget.
    - **The sojourn bit `armed` (E4).** "Armed once" is state, not a moment: §3.2 holds a per-service bit `armed`, reset on every state entry with `wexp`. The arming is the Waiting self-guard **G_arm** ⟺ pending ∧ ¬armed, whose move is `armed := ✓; arm WTO` with target Same (§3.5). G_arm fires in the propagation phase of whatever step first makes a require-target pending: the step that entered Waiting, if one was pending then, or a later one. A self-guard is not a state entry, so `gen` does not change and the WTO it arms carries the sojourn's generation. Waiting is therefore the one state whose timer is not armed at entry.
    - When WTO fires, Waiting × WTO sets the sojourn flag `wexp`, then evaluates **pending** ⟺ ∃B ∈ req(A): state(B) ∉ {Ready, Done}. If pending holds, → Failed(wait); otherwise the event changes nothing else. A deadline queued before the last require-target became Ready therefore never fails a service whose require-targets are all Ready.
    - While `wexp` holds, the Waiting guard G_wexp ⟺ wexp ∧ pending sends A to Failed(wait) (§3.5): the sojourn's budget is spent, so a require-target that becomes pending again after the deadline fails A at once instead of waiting without bound.
    - `wexp` and `armed` are reset on every state entry, with the generation. WTO can fire only after G_arm, so wexp ⇒ armed.
- **Two phases.** dinit's propagation-then-execution split is adopted (B§3.3).
- **No acquisition count.** dinit's `required_by` count is replaced by the goal derived from holders. A count cannot go stale, so dinit's "incorrect acquisition count" race cannot be expressed.
- **No pins** in v1.

**Why the generation fixes H4.**

- A timer armed in one sojourn can fire only in that same sojourn.
- The REVIEW-1 sequence (WaitTO#1 surviving Failed → Retry → Waiting) is dropped, because Retry → Waiting increments the generation.
- The BFS injects same-epoch stale timers (§3.8).

**Epochs** identify trees. A PID is never used as an identity, because PIDs are reused (Shepherd guards on PID, `service.scm:665-687`, B§13).

**Event-source obligations.** These are implementation obligations, tested in §10.

- **O1, channel before exit.** Every channel message of epoch e that was readable before the main process of e exited is delivered before `MainExit(e)` (L1). The supervisor drains the fd or socket before it enqueues MainExit.
  - **Observe, drain, then reap (R3-L2).** A single `wait4(-1)` reaps as it reports, so it cannot drain first. The source observes the exit without reaping, drains, and only then reaps: `waitid(P_ALL, …, WEXITED | WNOWAIT)` then `waitid(P_PID, pid, WEXITED)` on Linux and FreeBSD, or a pidfd where available (Linux 5.3+). On macOS, `waitid` with `WNOWAIT` *(unverified)*, otherwise a kqueue `EVFILT_PROC NOTE_EXIT` followed by the drain and `waitpid`. Windows has no reaping: the process handle keeps the pid from reuse until it is closed, and it is closed after the drain.
- **O5, channel before a state timer (F1).** Before a state timer of service C (WaitTO, StartTO, StopWaitTO, StopTO, KillTO, BackoffTO) is delivered to the table, svd drains C's readiness channel and C's keepalive channel, as O1 does before MainExit. A `READY=1`, a `\n` or a keepalive that was readable when the timer is dequeued, and in particular one written before the deadline, is therefore delivered first: Starting × Rise wins over Starting × StartTO. A flood from another service delays the dequeue, never the order.
- **O2, exit before empty.** `TreeEmpty(e)` is enqueued only after `MainExit(e)`.
  - Tree emptiness is observed through cgroup.events `populated 0`, `JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO`, or reaper status *(Windows and FreeBSD unverified)*.
  - The source holds TreeEmpty back until the main process has been reaped.
- **O3, synchronous spawn failure, before any child exists (D7).** SEt and SEp cover **only** failures that svd itself detects before a child process exists: the fork, `clone3` or `CreateProcess*` call fails, or a check that svd runs before forking fails. No tree exists in that case, so I2 holds. They are classified (N11) as one of:
  - **SEt**, transient: `EAGAIN`, `ENOMEM`, `EMFILE`, `ENFILE`;
  - **SEp**, permanent: the exec-helper binary is missing or not executable; an unknown account, which svd resolves before forking; an account whose id differs from the one resolved at load (`ErrAccountChanged`, F2b, **[Host+9]**).

  Under H_OS-E5 (§3.3) an SEt of a service outside every grant cannot be produced by a delegated principal: svd's fds, memory and spawn headroom are partitioned so that p's load cannot exhaust what an outside spawn needs (F1).

  An unclassified error counts as transient.
- **O4, failures after the fork go through the tree (D7).** Everything the exec-helper detects happens in a forked child inside the epoch's tree: the region conflict of §7.2, `ErrProtectionUnavailable`, `ENOENT`, `EACCES` or `ENOEXEC` from the final `execve`, and setup errors. The helper reports them as follows.
  - It writes a classified status on a **CLOEXEC status pipe** and exits with status 127. A successful `execve` closes the pipe, so the parent reads EOF.
  - **Classification (E11).** The helper classifies by a closed table; the table is data and has a unit test per row.

    | failure in the helper | sub-status |
    |---|---|
    | final `execve`: `ENOENT`, `ENOTDIR`, `EACCES`, `EPERM`, `ENOEXEC`, `ELOOP`, `ENAMETOOLONG`, `EISDIR`, `E2BIG`, `ELIBBAD` | `perm` (a property of the plan or the file system, the same on every retry) |
    | final `execve`: `ENOMEM`, `EAGAIN`, `ETXTBSY`, `EMFILE`, `ENFILE`, `EIO` | `fail` |
    | region conflict (§7.2), `ErrProtectionUnavailable` (§7.1), `setgroups`/`setgid`/`setuid` `EPERM`, `EINVAL` from a protection syscall (Landlock, seccomp, NNP) | `perm` |
    | `setuid` `EAGAIN` (RLIMIT_NPROC), any `ENOMEM`, any `EINTR` left after the helper's retry | `fail` |
    | anything else | `fail`, reported with the raw errno |

    Windows equivalents (`ERROR_FILE_NOT_FOUND`, `ERROR_ACCESS_DENIED`, `ERROR_BAD_EXE_FORMAT` → `perm`) are *(unverified)* and belong to P3. An unclassified post-fork error is `fail`, hence charged and bounded by intensity: it can never loop without bound.
  - The source reads the pipe to EOF before it enqueues MainExit, as in O1. It delivers `MainExit(fail)` with the sub-status `perm` (permanent) or `fail` (transient or unclassified).
  - Starting × X1 records that sub-status (§3.2 `record`). The tree then empties through StopWait/Stopping as usual, and **settle** sends a `perm` exit to Failed(spawn) (§3.2 rule 2). Failed(spawn) is therefore reached only at TreeEmpty, so I2 holds and the helper's exit is never seen in Failed.
- **Transient spawn errors are not charged (R3-L9).** `EAGAIN`, `ENOMEM`, `EMFILE` and `ENFILE` before the fork usually measure the **supervisor's** exhaustion, not the service's. Charging them to R_A and R_inst would let supervisor-side pressure escalate the instance. Starting × SEt goes to Backoff **uncharged**, with the delay backoff.max, and is reported. Retries are therefore unbounded in number but bounded in rate, at one per backoff.max per service. A post-fork transient failure (O4, sub-status `fail`) is the service's own and is charged as usual.
  - **Liveness cost, stated (E5).** A pre-fork SEt that recurs forever makes the cycle Waiting → Starting → Backoff → Waiting a fair run that never reaches Ready, Done or Failed. P2 (§3.7) is therefore stated under the environment hypothesis that SEt occurs finitely often for each service, and Gate 1 checks it under that fairness constraint, not by weakening it silently. Two alternatives were rejected: charging the k-th consecutive SEt would let supervisor-side pressure escalate a terminal service, which R3-L9 removed; and making unclassified pre-fork errors permanent would send a terminal service to Failed(spawn), hence the instance to Quiescing(1), on an errno nobody classified.
  - **The cost of not charging SEt, stated (REVIEW-5 F5).** The first rejection's reason is not decisive: an uncharged SEt loop on a require-target B of a terminal service T escalates the instance anyway, through T's WaitTO → Failed(wait) → Quiescing(1). The choice is kept, because that route escalates only when the host itself keeps svd from forking for longer than T's wait budget, which is the case escalation to the host init exists for. Under H_OS-E5, a delegated principal cannot be the cause of such a loop on a service outside its grant (F1).

### 3.2 Variables (M7)

Per service:

- `state` ∈ S = {Inactive, Waiting, Starting, Ready, Done, StopWait, Stopping, Killing, Backoff, Failed, Leaked} — 11 states;
- `goal` (derived);
- `epoch`;
- `gen`;
- `cause` ∈ {none, dep, restart, fail, natural} — why the current stop happens; `natural` means the main process exited on its own (E13: draft 7 called this constant `exit`, which collided with the variable below);
- `exit` ∈ {none, ok, fail, perm, sig} — the recorded status of the main process (D1, D7):
  - `ok` and `fail`: a **natural** exit, recorded while no stop signal had been sent;
  - `perm`: a permanent post-fork spawn failure reported on the status pipe (O4);
  - `sig`: any exit recorded after `term` or `kill` was sent, whatever its status;
- `wexp` — the wait deadline of the current Waiting sojourn has passed (D3);
- `armed` — the wait deadline of the current Waiting sojourn has been armed (E4). Both bits are reset on every state entry; both are meaningful in Waiting only;
- `reason` ∈ {exited, spawn, intensity, wait, dep, unkillable} — meaningful in Failed only, reported;
- `R_A` — timestamps of charged restarts, read through the window (N1);
- `x(A)` — holders. Each holder records its principal.

Per instance:

- `R_inst`;
- `R_p` per delegated principal p (N2, F4);
- `mode` ∈ {Running, Quiescing(status)}.

The schema fields used by the table:

- `kind` ∈ {daemon, oneshot};
- `restart.on` ∈ {never, on-failure, always} (K1). The table reads it through `permits(x)`:

| `restart.on` | permits(ok) | permits(fail) |
|---|---|---|
| never | false | false |
| on-failure | false | true |
| always | true | true |

**Helpers.** Each entry into a state increments `gen`, resets `wexp` and `armed`, and arms that state's single timer, if it has one, **except Waiting**, whose WTO is armed by the self-guard G_arm (§3.1, §3.5, E4).

- `spawn`: epoch++, exit := none, cause := none, create the tree, spawn main, open the channel → Starting.
- `stopwait(c)`: cause := c if cause = none → StopWait (arms StopWaitTO, §3.6).
- `term`: send `stop.signal` to the whole tree → Stopping (arms `stop.timeout`).
- `kill`: SIGKILL the tree (`TerminateJobObject` on Windows) → Killing (arms `kill.timeout`).
- `record(x)`: if exit = none then exit := x; and if cause = none **and x ∈ {ok, fail, perm}** then cause := natural. A `sig` exit never sets the cause.
  - In Starting, Ready and StopWait, no stop signal has been sent, so the exit is natural: X0 records `ok`, X1 records `fail`, or `perm` when the status pipe says so (O4). StopWait is reachable from Starting before the helper has exec'd (Starting × Rs, Starting × StartTO), so StopWait × X1 reads the status pipe too (E8); Ready × X1 cannot be `perm`, because Ready is entered only after Rise, which follows the `execve` that closed the pipe.
  - In Stopping, Killing and Leaked, the exit is the expected result of `term`/`kill`. X0 records **`sig`**, and so does X1, **unless the status pipe holds `perm`**, in which case X1 records `perm` (F8, draft 9). A `perm` status is the helper's report that the plan cannot be exec'd, written before the helper exited; it is a fact about the plan, not about our signal, so reading it in these states only avoids one charged, futile re-run before Failed(spawn). A helper killed before it wrote a status leaves EOF and records `sig`. A `sig` exit is never charged (rule 4 below) and **never counts as success** (rule 3); `perm` never matches rule 3 either.
  - **Classification is by the state in which the exit is dequeued, not by the time of the exit (E7, documented, not changed).** A oneshot that exits 0 just before G_free emits `term`, but whose X0 is dequeued after it, records `sig` and is run again. Distinguishing the two would need the kernel's order of "exit" and "signal delivered", which no portable interface gives: `WIFSIGNALED` is false for a process that handles SIGTERM and exits 0, which is exactly the half-done case D1 must not call Done. The error is on the safe side for D1 (re-run, never a false Done). Hence the **oneshot obligation**: a oneshot must tolerate being run again after a completed run that was stopped, and rule D1-n holds for exits dequeued in StopWait, i.e. up to the step boundary.
- `bump(A)`: append now to R_A.
- `pay(q)`: append now to the counter of payer q, where q is the owner (R_inst) or a delegated principal p (R_p). Who pays for what is fixed in §3.3 (D2, option A). **One write pays once (F9):** for a delegated caller, the API charge of §3.3 and the `pay` in the Failed × Ry cell are the same append, made once; the API layer checks it for refusal and the cell performs it.
- `charge(A)` (D4, E3): bump(A), then pay(payer(A)), with payer(A) from §3.3. R_A and the payer's counter are separate appends, so no helper appends to R_inst unconditionally. charge returns two bits, both evaluated **after both appends, at the same `now`**:
  - **oA** ⟺ |R_A(now)| > intensity.max(A);
  - **oP** ⟺ the payer's counter overflowed: |R_inst(now)| > max_inst when payer(A) = owner, |R_p(now)| > budget_p when payer(A) = p.
- **`overflow(A)`** — the outcome of a charge, applied in this order (E3):
  1. if oP ∧ payer(A) = owner → mode := Quiescing(max(1, current status)) (§3.3). This is a mode action, not a state target; A's state is decided by the next item, and escalation then lowers every goal;
  2. if oA ∨ (oP ∧ payer(A) ≠ owner) → A's target is **Failed(intensity)**. A delegated principal's crash overflow fails the crashed service only (§3.3);
  3. otherwise A's target is the caller's non-overflow target (Backoff for restart, Waiting for settle rule 4).

  In the data of §3.8, `charge` is the action `Charge`, item 1 is the action `Escalate`, and items 2 and 3 are the decision-list conditions `Overflow` / `¬Overflow`, where Overflow ⟺ oA ∨ (oP ∧ payer ≠ owner). An implementer of §3.2 alone therefore builds all three outcomes.
- `restart(A)`: charge(A); then by `overflow(A)`:
  - Overflow → Failed(intensity);
  - else → Backoff, arming delay = min(initial · factor^(|R_A(now)|−1), max).
- `settle(A)`, applied on TreeEmpty. Rules are evaluated in order; the first that matches wins (N11, D1, D7):
  1. goal = down → Inactive;
  2. exit = perm → Failed(spawn), uncharged (O4);
  3. kind = oneshot ∧ exit = ok → Done, whatever the cause. Here `ok` is a **natural** success: the oneshot exited with status 0 before any stop signal was sent. A completed oneshot, such as a migration that finished during StopWait(dep), is never re-run by a dependency loss **if its exit is dequeued before `term`** (F8; otherwise E7 applies and it is re-run once, never falsely Done);
     - a oneshot that **was stopped** by StartTO, by a dependency loss, or by a restart has exit = `sig` (it died of our signal) and never matches this rule. It falls through to rule 4 or 5 and is run again from Waiting or Backoff, never Done (D1);
  4. cause ∈ {dep, restart, none}:
     - if exit = fail, a natural crash recorded before `term`, then charge(A), and by `overflow(A)` (E3): Overflow → Failed(intensity), else → Waiting (an owner-payer overflow also escalates, item 1);
     - otherwise (exit ∈ {none, ok, sig}) → Waiting, uncharged.

     A daemon that crashes on its own during StopWait(dep) is therefore charged;
  5. cause = fail → restart(A);
  6. cause = natural ∧ permits(exit) → restart(A);
  7. otherwise → Failed(exited).

  **Why rule 3 cannot see a killed oneshot (D1).** `sig` is recorded in exactly the states where a signal has been sent, and rule 3 reads only `ok`. The REVIEW-3 sequence (Starting × StartTO → StopWait → term → SIGTERM death) records `sig`, its cause is `fail`, and settle reaches rule 5: restart(A), charged, never Done. Its require-dependents therefore never see Done for a half-run migration.

  **Rule D1-n (natural completion during StopWait, confirmed 2026-11-23).** A oneshot whose exit with status 0 is **dequeued** while it is in StopWait, i.e. before `term` is sent (E7: an exit that happens before `term` but is dequeued after it records `sig`), records `ok` and settles to **Done**, whatever the cause of the stop. *Justification:* no signal from svd reached it, so its status 0 is its own report that the work completed, and re-running a completed migration is the error N11 removed.

### 3.3 Restart intensity and escalation (OTP model)

- **Charging.** `restart(A)` charges, and so does settle rule 4 for a spontaneous exit(fail). Both are reached only by a service's own failure: cause ∈ {fail, natural}, a post-fork transient spawn failure (O4), or a crash during StopWait. A pre-fork SEt is not charged (R3-L9), and a `sig` exit is never charged.
  - Dependents dragged down through the DepLost guard have cause = dep and settle uncharged.
  - A step has one external event, hence at most one root cause.
  - **A restart dragging N dependents therefore charges exactly once**, as in OTP's `add_restart` (B§8.5, `supervisor.erl:1333-1352`).
- **Service overflow** (|R_A| > max) → Failed(intensity). Require-dependents reach Failed(dep) through the guard.
- **Instance overflow** (|R_inst| > instance.intensity.max), or a `terminal = ✓` service reaching Failed, → **escalation**: mode := Quiescing(1). In Quiescing (M6):
  - all holders are cleared;
  - **API writes are refused** (`ErrQuiescing`, HTTP 503);
  - open transactions are aborted without rollback;
  - the instance exits when every service is in {Inactive, Failed, Leaked}.
- **The exit is bounded by the stop bound of §3.6.** At exit:
  - Leaked trees are listed on stderr and in the upward `STATUS=` (M10);
  - SIGTERM enters Quiescing(0);
  - **the status is the maximum** of the statuses requested (N15). A SIGTERM during Quiescing(1) keeps status 1, so the host init still sees the escalation.
- **Who is charged: owner versus delegated principals (N2, D4, D2 decided as option A of §13).**
  - Every holder records the principal that created it: `boot` and the owner are the *owner*. Holders decide goals, **not** who pays (draft 5's owner-held rule is withdrawn).
  - **payer(A)** = p when `run_as(A)` is a delegated principal p, which §1.6 makes true **exactly** of the services in grant(p) (E1); otherwise the owner. charge(A) bumps R_A once and pays payer(A) once, and `overflow(A)` (§3.2) decides the outcome.
  - **Grant closure** (§8.2, E2): every service with a require, want **or order** edge to a granted service is itself in the grant, or Compile fails with `ErrGrantNotClosed{…, edge}`.
  - **Which writes pay R_p.** Every write by a delegated principal (`start`, `stop`, `restart`, `retry`) is one API charge to R_p. Owner writes pay nothing, except Retry, which pays payer(A).
  - **Retry** (the Failed × Ry cell) is `R_A := ∅; pay(payer(A))` (draft 9; draft 8 had `pay(caller)`). It does not call charge(A), so after a Retry |R_A| = 0, as "Retry clears R_A" says. A delegated caller's Retry targets A ∈ grant(p), so payer(A) = p = caller, as before. An owner's Retry of A ∉ every grant pays R_inst, as before. **What changed (found while re-proving Theorem G as 2-safety, §3.8):** an owner's Retry of a granted A now pays R_p, not R_inst. Under draft 8, whether that Retry paid R_inst depended on whether A was in Failed, a state p controls, so p could decide whether a low owner operation moved R_inst, and with enough such Retries, whether the instance escalated. A payment to R_p made by an owner Retry is never refused (it is the owner's act) and, like a crash charge, never changes `mode`; it may leave p's own writes refused until the window drains.
  - **Two kinds of charge to R_p, and two different overflow rules:**
    - an **API charge** (Retry, and every other write that pays) that would overflow R_p is **refused** with `ErrPrincipalBudget`, before it takes effect;
    - a **crash charge**, which pays R_p because a crash of a delegated incarnation happened, cannot be refused. When it takes |R_p(now)| past budget_p, the crashed service goes to **Failed(intensity)**, as a service overflow would. The overflow stays local: mode is unchanged, and every further write from p is refused with `ErrPrincipalBudget` until the window drains.
  - **R_p never escalates**, under either rule.
  - **Effect-set check.** A non-owner's `start`, `stop`, `restart` or `retry` is refused with `ErrOutsideGrant{services}` when the set of services whose goal or state the operation would change is not inside grant(p). That set includes:
    - the G_lost reverse require closure, for `restart`;
    - the reverse closure, for `stop --with-dependents`.

    Since draft 9 both closures lie inside grant(p) by construction (E2), and no holder of any principal on a granted service changes an outside goal (§2.3, cut, F3), so for a target inside grant(p) the check is expected never to refuse. It is kept as a **real refusal**, not an assertion (F3): when it refuses, the operation is refused with `ErrOutsideGrant`, has no effect, and the refusal is also logged as an internal inconsistency. It never panics and never stops svd, because it is reachable from a delegated principal's request.
  - A loop of retries, or of `stop`/`start` (REVIEW-2 shows that denying Retry alone does not suffice), is therefore bounded in number by R_p, and the crashes each retry re-enables pay R_p too, since the service runs as p.

  **Hypothesis H_OS (E6: split into what svd enforces and what it assumes).** Theorem G is conditional on the assumed part; svd does not provide host isolation.

  *Enforced by svd* (Compile errors, or refusals at spawn):
  - **H_OS-E1, uid separation.** runs_as(X, p) ⟺ X ∈ grant(p), on resolved ids (§1.6, `ErrDelegationUid`, both directions since E1, readers since F4). So the processes svd runs as p are exactly the trees of grant(p), no terminal service runs as p, and no service runs as a reader.
  - **H_OS-E2, no hand-over of a tree container (F10, draft 9).** svd never hands a cgroup, a job object or a reaper to a principal. Draft 8 allowed handing a granted subtree's cgroup to p's own instance; that is withdrawn for v1, because the per-epoch and per-principal limits (H_OS-E3, H_OS-E5) would then be written by p's instance, not by svd *(the ownership of cgroup v2 interface files after delegation is from memory)*. Under option A svd runs the granted trees itself, so nothing is lost. p therefore holds neither `cgroup.kill` nor `cgroup.freeze` over any tree.
  - **H_OS-E3, the met class-H plan of every granted service** (writable path regions, socket families, rlimits, §1.5), enforced by the exec-helper (§7), **and delegated privilege** (§1.6, F2a): no capability or token privilege, NNP where the OS has it, p's own groups only. These narrow the channels of H_OS-A2 as far as the baseline states them; they do not close them.
  - **H_OS-E4, graph and goal structure.** Grant closure under every edge kind (E2), and goal truncation at every grant boundary for every principal (§2.3, F3).
  - **H_OS-E5, svd resource isolation (F1, draft 9).** What svd itself spends to serve one principal is drawn from that principal's partition, so p cannot exhaust what svd needs to serve a service outside grant(p). Per delegated or reader principal p, class G fields (§1.7) state `budget.connections`, `budget.inflight`, `budget.refusals` (refused writes per second) and, for delegated principals, `budget.pids` and `budget.memory`; **[Compile+9]** they are in the completeness set Q whenever p is a non-owner-class principal (`ErrIncomplete`). The mechanisms:
    - **One API listener per principal.** Each non-owner-class principal gets its own API socket, in its own directory under the instance runtime directory (§8.1), owned by root (the owner in mode U), mode 0700, with one ACL entry granting search to that principal's id only *(Linux POSIX ACL, macOS and FreeBSD extended ACLs, Windows DACL: unverified)*. Owner-class principals share one listener that no other principal can reach. Each listener has its own kernel backlog, so p's connection flood fills only p's backlog. svd accepts round-robin across listeners, still checks peer credentials on every connection (a mismatch is closed), and stops accepting on p's listener while p holds `budget.connections` connections or `budget.inflight` requests. A refused write beyond `budget.refusals` per second closes the connection and pauses p's listener for the rest of that second.
    - **fd headroom.** At start svd raises its `RLIMIT_NOFILE` soft limit to the hard limit and computes the need: the spawn reserve (for every service, the fds of one spawn: log pipe, readiness pipe, status pipe, notify socket, tree handle, pidfd), plus the owner-class reserve, plus Σ_p `budget.connections`. **[Host+9]** If the hard limit is below the need, the load fails with `ErrResourceBudget{fds, need, have}`. svd refuses `accept` on any listener when the free fds would drop below the spawn reserve. So an outside spawn never sees `EMFILE` because of p. Windows has no per-process handle limit of this kind *(from memory)*; there the bound is svd's memory (next item).
    - **svd's memory.** Every per-principal buffer is bounded: request size, per-connection buffer, `budget.inflight` requests, and per-service channel intake (bounded queues, §3.1 step 1; log lines beyond the ring are dropped and counted, §9). svd's own memory is therefore at most a fixed term plus a sum of per-principal and per-service terms computed from the plan; svd reports it in `svd plan`.
    - **Event intake.** Per-service bounded queues dequeued round-robin (§3.1 step 1), and O5, so a flood of `STATUS=`, `WATCHDOG=1`, log lines or readiness bytes from a granted service delays no outside service's events by more than one round, and cannot move a READY behind its own StartTO.
    - **Notify sockets (F11).** svd's `recvmsg` on a notify socket passes a control buffer sized for one credentials message only (`CMSG_SPACE` of `struct ucred` on Linux, `cmsgcred` on FreeBSD *(unverified)*). A datagram with `MSG_CTRUNC` set, or with any control message other than credentials, is dropped and counted. The kernel discards `SCM_RIGHTS` descriptors that do not fit the buffer *(from memory)*, so a granted process cannot install fds into svd. The notify socket is per epoch (§4.1), so its receive buffer is per service.
    - **Process slots and memory of p's trees.** svd's forks of outside services need host process slots and memory that p's granted trees could exhaust. They are bounded per principal, as an aggregate over grant(p), written by svd and never by p (H_OS-E2):
      - Linux: a per-principal parent cgroup `deleg/<uid>/`, holding every epoch cgroup of grant(p), with `pids.max` = `budget.pids`, `memory.max` = `budget.memory`, and a `cpu.weight` stated in B; plus `RLIMIT_NPROC` = `budget.pids` in every granted tree (it counts processes per real uid);
      - FreeBSD: `rctl` rules `user:<uid>:maxproc:deny` and `user:<uid>:memoryuse:deny` *(requires `kern.racct.enable`; unverified)*; without racct, `RLIMIT_NPROC` = `budget.pids` and a per-process `RLIMIT_AS` with `budget.pids` × `RLIMIT_AS` ≤ `budget.memory`;
      - macOS: `RLIMIT_NPROC` = `budget.pids` (per uid) and a per-process `RLIMIT_AS`, with the same product bound *(enforcement of `RLIMIT_AS` on macOS unverified)*;
      - Windows: a per-principal job object, parent of the per-epoch jobs (nested jobs), with `JOB_OBJECT_LIMIT_ACTIVE_PROCESS` and `JOB_OBJECT_LIMIT_JOB_MEMORY` *(unverified, P3)*.

      **[Host+9]** A host that lacks the mechanism its OS row names refuses to load a plan with a non-empty grant: `ErrIsolationUnavailable{resource, os}`. There is no opt-out: a delegation whose isolation cannot be enforced is not offered.
    - **What E5 does not cover.** It bounds what svd spends and what svd's trees of grant(p) take. p's processes outside svd (its logins) and the sufficiency of the budgets against the host's capacity remain in H_OS-A2 (*host*).

  *Assumed, not enforced, each stated so that it can be tested (F2):*
  - **H_OS-A1, OS privilege (the uid rule).** For every process π of a tree of O, the real, effective and saved uids of π differ from p's id; and a process of uid p holding the privileges svd gives grant(p) (none, §1.6) gets `EPERM` from `kill(π, 0)`, `ptrace(PTRACE_ATTACH)`, `process_vm_writev` and an `open` of `/proc/π/mem` (on Windows, `OpenProcess` with any access right beyond `PROCESS_QUERY_LIMITED_INFORMATION` is denied *(unverified)*). Also, p cannot raise its own privilege on that host (no sudoers entry, no privileged group, no set-user-ID path it may run that leads above p). **Test:** the per-OS suite (§10) runs this probe from a uid-p process, against a sentinel outside service, on each CI host.
  - **H_OS-A2, non-interference, as disjointness predicates over named sets.** "Influence" cannot be observed, so A2 is stated as conditions that a site audit can check, each one per host:
    - *(files)* the paths writable by uid p, or by grant(p)'s plan, are disjoint from the paths read **or executed** by O's trees (binaries, interpreters, libraries, configuration). A file owned by p counts as writable by p, since its owner can change its mode; this covers a set-user-ID-p file executed by an outside service, which would make that process signalable by p through its saved uid. svd knows only the plan part of the first set, and cannot know the second, because v1 has no read or visibility field (§1.9); **A2-files is a site audit, not an svd check**;
    - *(ports)* the ports p can bind are disjoint from the ports O listens on or connects to;
    - *(IPC)* the same for System V and POSIX IPC keys, abstract AF_UNIX sockets and D-Bus names;
    - *(host)* the processes of uid p outside svd, together with Σ over grant(p) of the E5 budgets, leave the memory, CPU, pid space, disk space, inodes and system file table that O and svd need. svd checks none of this;
    - *(svd)* nothing: svd's own resources moved to H_OS-E5 (F1).

  **Theorem G (delegation cannot escalate), re-proved in draft 9.** Let p be a non-owner-class principal (delegated or reader, §1.6), G = grant(p) (∅ for a reader) and O its complement, other principals' grants included (their holders and events are low with respect to p). Split the inputs of a run (REVIEW-5 Part 5):
  - **high:** every API operation by p, and every process, channel and timer event of a service in G;
  - **low:** every owner-class API operation (including those on services of G), every event of a service in O, the request deadlines of owner transactions, SIGTERM to svd, and the abstract-time ticks that age the windows;
  - **the StopWait oracle:** for every C ∈ O with a dependent in G, the step at which G_free(C) becomes true during a StopWait sojourn of C, a value in [entry, entry + SW(C)]; it is treated as a low input.

  Let obs_O(s) be, for every C ∈ O, (state, goal, cause, exit, wexp, armed, reason, |R_C(now)|), together with `mode`, |R_inst(now)| and the outcome of every owner transaction whose members lie in O (by §2.4 every transaction lies wholly in O or wholly in one grant). Epochs and generations are names, not behaviour, and are left out. Under H_OS-E1…E5 and H_OS-A1, H_OS-A2:
  1. **(non-interference)** obs_O after every step is a function of the low inputs and the oracle values alone: two runs with the same low inputs and oracle values and **any** high inputs have equal obs_O at every step;
  2. **(no escalation)** in particular, since `mode` ∈ obs_O, every run that reaches Quiescing(1) is matched step for step on O by the run with the same low inputs and oracle values in which p issues no operation and G's trees emit no event. p's only lever on O is the oracle: the length of a StopWait sojourn of an outside C that has a dependent in G, which is bounded by SW(C), ends in Stopping either way (by G_free or by SWTO), and is reported as a T2 overrun when SWTO ends it (E14). Any such length is one C's own configuration admits. When no outside service has a dependent in G, there is no oracle, and p cannot cause Quiescing(1) at all.

  *What item 2 concedes, stated.* By choosing the oracle, p can decide whether an outside C's **own** crash, dequeued during a lengthened StopWait, is recorded before `term` (charged, rule 4) or after it (`sig`, uncharged). The crash itself is a low event, and the charge is what C's configuration specifies for that sojourn length; p cannot create a crash, and each StopWait sojourn holds at most one exit of C.

  *Proof.* By induction over steps, on the pair of runs (r₁, r₂) of item 1; the hypothesis is obs_O(r₁[i]) = obs_O(r₂[i]), together with equal holders on O and equal R_inst.
  - **(a) Structure, from Compile and the load checks.** By grant closure under every edge kind (E2), no edge goes from O into G, so every path that starts in O stays in O. By the cut (§2.3), no goal propagates from G into O. By H_OS-E1 on resolved ids (F2b), svd runs a process as p iff it belongs to a tree of G; terminal services are in O; no service runs as a reader.
  - **(b) What a transition of C ∈ O reads.** Each item is equal in r₁ and r₂:
    - *its own events.* Process and channel events come from C's tree. No process of that tree has p's real, effective or saved uid, and the privileges svd gives G are none (H_OS-E1, F2a), so p cannot signal, trace or write it (H_OS-A1, testable form), and p holds no tree container (H_OS-E2). Outside svd, p reaches C's inputs through no named channel (H_OS-A2). Inside svd, p's load cannot delay C's events past one round, cannot reorder C's READY behind C's StartTO (§3.1 step 1, O5), and cannot cause an SEt of C, because C's spawn draws on a reserve no principal can consume (H_OS-E5). So C's events are low inputs. Timer events of C are C's own. API operations on C by p are refused by the grant check (§8.2) before any effect; a reader writes nothing;
    - *the states of req(C) ∪ wnt(C) ∪ ord(C)*, all in O by (a), hence in obs_O, equal by hypothesis: the event and guard tables of C read only O (G_ok, G_failed, G_lost, G_wexp, G_arm, settled);
    - *goal(C)*: by the cut it is a function of the holders on O (§2.3). Those holders change only by low inputs: owner-class operations, the rollback of an owner transaction whose members lie in O (its success or failure reads only O states and a low deadline), and Quiescing (`mode` is in obs_O). p's `stop` and `stop --with-dependents` clear holders on G only, and abort only transactions whose members lie in G (§2.3, F3). So the holders on O are equal. *This item replaces draft 8's step (b), which wrongly said that p cannot remove holders of other principals: p can, on G, and that is harmless only because of the cut;*
    - *G_free(C)*, which reads C's dependents, some possibly in G. It enables only StopWait → Stopping, which SWTO also enables after SW(C). Its value is the oracle, which is equal by construction. This is the only read of G by a service of O;
    - *R_C and R_inst.* R_C is appended only by charges of C. R_inst is appended only by charges of services whose payer is the owner, which by E1 are services of O, at transitions that are equal by the items above; and by owner Retries of services of O (§3.3, draft 9: an owner Retry of a granted service pays R_p), whose effect reads only the state of a service of O. So |R_inst(now)| is equal.
  - Hence C takes the same transition in r₁ and r₂, and obs_O stays equal. This proves item 1.
  - **(c) Item 2.** `mode` is in obs_O, so Quiescing(1) occurs in r₁ iff it occurs in r₂; take r₂ to be the run with no high input. For R_p: its overflow sends only the crashed service, in G, to Failed(intensity), and never changes `mode` (overflow(A) item 2; I5); an owner Retry's payment to R_p never changes `mode` either. ∎

  **What draft 8's proof missed, and where each gap is now closed.** F1: svd's own resources, now H_OS-E5 and O5, used in (b) "its own events". F2: privileges svd itself grants, and "=" not on ids, now §1.6 and the testable H_OS-A1. F3: owner holders crossing the boundary outward, now the cut, used in (a) and (b) "goal(C)". The owner-Retry payment, found by the 2-safety formulation, now pays payer(A). F4: the principal classes, now §1.6, so the theorem covers every non-owner-class principal, readers included.

  **Grants of different principals do not interact either.** By E1 they are disjoint, and by E2 an edge from grant(p) into grant(q) would put its source in grant(q) too. Taking O to be everything outside grant(p), the theorem holds for each delegated principal against all others at once.

  **Draft 5's proof was withdrawn in draft 6.** REVIEW-3 D2 gave two counter-examples, and option A answers each:
  - **Through the API.** A ∈ grant(p) is boot-enabled, hence owner-held, and crashes on its own. Each `retry A` by p costs R_p one charge, but re-enables A, whose next crashes are charged to R_inst because the incarnation is owner-held. Two retries take R_inst past max_inst → Quiescing(1).
  - **Outside the API.** §1.6 makes every delegated service run as p, and §8.2 may hand p its cgroup. p can therefore crash an owner-held A ∈ grant(p) with `kill -SEGV`. If a terminal T ∉ grant(p) requires A, then A → Failed(intensity) makes T Failed(dep), which escalates without any Retry.

  Under option A, the first counter-example's crashes pay R_p, because A runs as p; R_inst is untouched. In the second, T ∉ grant(p) requiring A ∈ grant(p) is `ErrGrantNotClosed` at Compile.

  **REVIEW-4's two routes (draft 7) and their answers.**
  - **E1.** An owner service X ∉ grant(p) with run_as(X) = p, crashed by p with `kill -SEGV`, while X is terminal or required by a terminal service. Draft 8: `ErrDelegationUid{X, p, outside-grant}` at Compile.
  - **E2.** Terminal T requires D ∉ grant, D wants A ∈ grant(p); p restarts A just before each start.timeout, so A is never settled, D waits, and T's single wait deadline fires → Failed(wait) → Quiescing(1). Draft 8: D has a want edge into grant(p), so `ErrGrantNotClosed{p, A, D, want}` at Compile. The per-sojourn `settled` latch (REVIEW-4 fix b) was not adopted: closure removes every path from outside into the grant, which proves the whole of item 2, while the latch would bound only this one stall and change the guard table.
- **Retry and the windows (M8, N1).** The windows R_A, R_inst and R_p are read through `now − window` (§3.1). There is no Expire event.
- **Theorem R (bounded restart):**
  - in any window of length W_A, A is charged at most max_A + 1 times before Failed;
  - in any instance window, the instance accepts at most max_inst payments to R_inst before it escalates;
  - each delegated principal's API charges are bounded by its own budget, and refused beyond it; its crash charges (every crash of a service run as p) past the budget fail the crashed service locally and never escalate;
  - escalation height is 2 (service, then instance) before control passes to the host init, whose policy then applies (systemd `StartLimitIntervalSec=`, `systemd.unit.xml:1214` @ v262.2).

### 3.4 Event table (11 states × 15 events = 165 cells)

Draft 4's 15 columns included Expire. Draft 5 removes Expire (N1) and splits SpawnErr into transient and permanent (N11), so the count stays at 15.

Key:

- `·` — reachable, no state change;
- `⊘` — **unreachable**: the epoch or generation filter, or O1–O3, makes this event impossible in this state. These cells are listed in the exemption list of §3.8.

Columns: Ry = Retry; Rs = Restart (operator); SEt, SEp = SpawnErr transient or permanent (N11); X0 = MainExit(ok); X1 = MainExit(fail); TE = TreeEmpty; Ri = Rise; Fa = Fall. State timers (F7, draft 9: draft 8's headers `STO` and `StTO` were swapped against the prose and are expanded): WTO = WaitTO (`wait.timeout`); StartTO = `start.timeout`; SWTO = StopWaitTO (derived SW, §3.6); StopTO = `stop.timeout`; KTO = KillTO (`kill.timeout`); BTO = BackoffTO (the backoff delay). The abbreviations `STO` and `StTO` are no longer used anywhere.

There is no Expire column: the windows are read, not evented (N1).

| state | Ry | Rs | SEt | SEp | X0 | X1 | TE | Ri | Fa | WTO | StartTO | SWTO | StopTO | KTO | BTO |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| **Inactive** | · | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Waiting** | · | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | wexp := ✓; if pending →Failed(wait), else · (D3). Reached only with armed = ✓ (E4) | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Starting** | · | stopwait(restart) | →Backoff, uncharged, delay backoff.max (R3-L9) | →Failed(spawn) | record(ok); stopwait | record(fail or perm, from the status pipe); stopwait | ⊘ (O2) | →Ready, arm liveness | ⊘ (liveness not armed) | ⊘ | stopwait(fail) | ⊘ | ⊘ | ⊘ | ⊘ |
| **Ready** | · | stopwait(restart) | ⊘ | ⊘ | record(ok); stopwait | record(fail); stopwait | ⊘ (O2) | · (latched) | stopwait(fail) | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Done** | · | →Waiting (re-run; cause = restart) | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **StopWait** | · | · | ⊘ | ⊘ | record(ok) | record(fail or perm, from the status pipe) (E8) | settle | · | ⊘ | ⊘ | ⊘ | term (report T2 overrun) | ⊘ | ⊘ | ⊘ |
| **Stopping** | · | · | ⊘ | ⊘ | record(sig) | record(sig, or perm from the status pipe) (F8) | settle | · | ⊘ | ⊘ | ⊘ | ⊘ | kill | ⊘ | ⊘ |
| **Killing** | · | · | ⊘ | ⊘ | record(sig) | record(sig, or perm from the status pipe) (F8) | settle | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | →Leaked (report) | ⊘ |
| **Backoff** | · | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | →Waiting |
| **Failed** | goal = up: R_A := ∅; pay(payer(A)) (§3.3, D4, draft 9); →Waiting; else · | as Ry | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Leaked** | · (API: `ErrLeaked`) | · (API: `ErrLeaked`) | ⊘ | ⊘ | record(sig) | record(sig, or perm from the status pipe) (F8) | →Failed(unkillable) | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |

Notes on the cells:

- **Retry's payment and overflow (E3, draft 9).** In Failed × Ry, `pay(payer(A))` is evaluated like oP of §3.2: when payer(A) = owner (A in no grant, so the caller is owner-class), an overflow of R_inst takes the action `Escalate` (mode := Quiescing(1)), and the cell still moves to Waiting, whose goal escalation then lowers. When payer(A) = p and the caller is p, an overflowing payment never reaches the table, because the API refuses it first with `ErrPrincipalBudget`; this refusal check and the cell's `pay` are one payment (F9). When payer(A) = p and the caller is owner-class, the payment is made, never refused, and never escalates (§3.3).
- **Leaked × X1 can record `perm` (F8).** A tree that leaked while its helper had reported `perm` reaches Failed(unkillable) at TreeEmpty, not Failed(spawn): the Leaked × TE cell does not call settle. The `perm` is kept in `exit` and reported with the Failed state.
- **Leaked is a state** (H3). While a retired tree is populated, Retry is refused, so a second incarnation can never start beside a leaked one.
- **No `forced` flag** (H3). Stopping and Killing are separate states, so a later epoch's first StopTO always goes to Killing.
- **Main exit is not the end of the unit** (H2).
  - Starting, Ready → StopWait → Stopping signals the rest of the tree.
  - The decision is taken only at TreeEmpty, through settle.
  - The new epoch is spawned only after the old tree is empty. The `EADDRINUSE` loop is gone.
- **Starting × StartTO, Ready × Fall** go through StopWait like every other stop, with cause = fail (M7: no elided cells).
- **Done × Restart** re-runs a oneshot, uncharged.
- **Rise in StopWait, Stopping, Killing and Leaked** is a late message for a still-current epoch: no-op.

### 3.5 Guard table, level-triggered propagation (M7)

The cascades are **guards** evaluated on the current snapshot, not edge-triggered events. A service that enters Waiting with its requirements already Ready therefore starts in the same step.

Definitions:

- req(A), wnt(A), ord(A) — the edge targets of A.
- **settled(B)** ⟺ goal(B) = down ∨ state(B) ∈ {Ready, Done, Failed, Backoff, Leaked}. This is dinit's `waits-for` meaning, "started **or failed**" (B§3).
  - Backoff means that the current attempt has failed.
  - A want- or order-target that flaps therefore releases its dependents at its first failure.
  - **`want` is genuinely soft (N4).** No state of a want- or order-target can send the dependent to Failed, and the dependent waits at most for one attempt of the target. That attempt is bounded by the target's own timers (Theorem B), which is why WaitTO covers only require-targets (§1.6).
- **G_up** ⟺ goal = up. **G_down** ⟺ goal = down.
- **G_ok** ⟺
  - ∀B ∈ req: state(B) ∈ {Ready, Done};
  - ∀B ∈ wnt ∪ ord: settled(B).
- **G_failed** ⟺ ∃B ∈ req: state(B) ∈ {Failed, Leaked}.
- **G_lost** ⟺ ∃B ∈ req: state(B) ∉ {Ready, Done}. In Waiting the same predicate is called **pending**.
- **G_wexp** ⟺ wexp ∧ pending (D3). It reads the sojourn flag set by WTO, and it excludes G_ok.
- **G_arm** ⟺ pending ∧ ¬armed (E4). Its move is a **self-move**: target Same, action `ArmWait` (armed := ✓, arm WTO with the current gen). It excludes G_ok (pending) and G_wexp (wexp ⇒ armed).
- **G_free(B)** ⟺ both of:
  - no A with B ∈ req(A) is in {Starting, Ready, StopWait, Stopping, Killing, Leaked};
  - no A with B ∈ wnt(A) ∪ ord(A) is in {StopWait, Stopping, Killing, Leaked}.

  **Leaked blocks (N3)**, so a leaked dependent is bounded by SWTO and reported. It is never silently passed over.
  - Require-dependents that are still running block B. They are about to be pushed down by G_lost, so the result does not depend on evaluation order.

Guards are tried in the order given in each row. The first enabled guard fires.

| state | guards, in priority order |
|---|---|
| Inactive | G_up → Waiting |
| Waiting | G_down → Inactive; G_failed → Failed(dep); G_wexp → Failed(wait) (D3); G_arm → Same, `ArmWait` (E4); G_ok → spawn. G_arm fires at most once per sojourn, so the sojourn's single wait deadline is never re-armed (§3.1, D3) |
| Starting | G_down → stopwait(none); G_lost → stopwait(dep) |
| Ready | G_down → stopwait(none); G_lost → stopwait(dep) |
| Done | G_down → Inactive |
| StopWait | G_free → term |
| Stopping, Killing, Leaked | none |
| Backoff | G_down → Inactive |
| Failed | G_down → Inactive; reason ∈ {dep, wait} ∧ G_up ∧ G_ok → Waiting, uncharged (**automatic recovery**, M5) |

Grid: 11 states × 8 guards (G_up, G_down, G_ok, G_failed, G_lost, G_free, G_wexp, G_arm) = 88 cells.

**How the grid encodes the rows (F6, draft 9).** Draft 8 gave 88 cells but no way to say that a guard is not tried in a row, nor in which order the row's guards are tried. Draft 9 adds both to the Cell (§3.8):

- **`Absent`**, a Reach value: "this guard is not a guard of this row; skip it". It is distinct from `·` (the guard is tried and its move is Same with no action) and from `Unreachable` (the guard is tried and the BFS asserts it never holds). For example, Inactive × G_down is `Absent`, and Stopping × every guard is `Absent`.
- **`Prio`**, an integer per present cell, unique within a row: the row's guards are tried in increasing `Prio`, and the first enabled one fires. The table above gives the order; for Waiting it is G_down 1, G_failed 2, G_wexp 3, G_arm 4, G_ok 5.
- **Conditions on a guard.** A row entry that conjoins a guard with a further predicate carries it as `Cond`. Failed × G_ok has `Cond = ReasonRecoverable ∧ GoalUp` (reason ∈ {dep, wait} ∧ G_up), target Waiting, no Charge. Failed × G_down is a separate present cell with `Prio` 1.

Exactly 15 cells are present (Inactive 1, Waiting 5, Starting 2, Ready 2, Done 1, StopWait 1, Backoff 1, Failed 2); the other 73 are `Absent`. Gate 1 asserts that the evaluator never reads an `Absent` cell, and Gate 2 mutates `Prio` (§3.8).

**Termination of propagation.**

This argument replaces draft 4's chain, which was wrong (N14). It is an induction over the DAG.

- Goals are fixed during propagation (§2.3).
- **Per-service graph with fixed inputs.** Fix the states of A's targets, and fix whether G_free holds. A's guard moves then form an acyclic graph:
  - with goal up: Inactive → Waiting → {Starting | Failed}; Starting/Ready → StopWait → Stopping; Failed → Waiting;
  - with goal down: Waiting, Done, Backoff, Failed → Inactive; Starting/Ready → StopWait → Stopping.
  - The only possible cycle, Waiting → Failed → Waiting, needs G_failed or G_wexp, and then G_ok. With fixed targets G_ok excludes both, since each implies that a require-target is not Ready or Done.
  - **The self-move G_arm (E4)** counts as a move. It sets `armed` from ¬✓ to ✓, and `armed` is reset only by a state entry, so within one sojourn it fires at most once and adds no cycle. It also needs pending, which excludes the move Waiting → Starting.
  - **Longest paths with fixed inputs (corrected in draft 9, F6).** Draft 8 cited Inactive → Waiting → G_arm → Failed(dep) as a 3-move path. It is not one: G_failed has a higher priority than G_arm, so if G_failed holds the move from Waiting is Failed(dep) directly, and if it does not hold, it cannot start holding with fixed inputs; G_wexp needs `wexp`, which only the WTO event sets, never propagation. Every maximal path has at most 2 moves:
    - goal up: Inactive → Waiting → {Starting (G_ok) | Failed(dep) (G_failed) | Failed(wait) (G_wexp) | Same with `ArmWait` (G_arm)};
    - goal up: Failed(dep|wait) → Waiting → Starting (both moves need G_ok, which excludes G_failed, G_wexp and G_arm);
    - goal down: Starting or Ready → StopWait → Stopping (the second move needs G_free);
    - goal down: Waiting, Done, Backoff or Failed → Inactive (1 move);
    - goal up with G_lost: Starting or Ready → StopWait(dep) → Stopping (the second move needs G_free).
  - So with fixed inputs A moves at most 2 times, self-moves included. Gate 1 asserts this bound per service.
- **Inputs change finitely often.**
  - All guards except G_free read only A's targets. G_free reads its dependents, but the move it enables, StopWait → Stopping, is one-way and happens at most once per step.
  - By induction in topological order, from sinks up: a sink's inputs are fixed, so it stabilises after ≤ 2 moves. If every target of A changes state finitely often, A's inputs change finitely often, and A moves at most 2 times per input change.
  - So propagation terminates.
- **No closed-form bound is claimed (R3-L8).** The induction gives "at most 2 moves per input change", and input changes compound along the depth of the DAG, so it does not yield a linear bound such as 4N. Gate 1 records the **measured** maximum number of moves per step over its whole scope, writes it into a golden file, and asserts it. A change that raises it must update the golden file in review.

**Confluence** across evaluation orders is *asserted by the BFS*, by comparing all orders for N ≤ 4. It is not claimed by hand.

### 3.6 Theorem B: every transient state is bounded (restated after H3)

**Transient states**, with their timers:

| state | timer | bound |
|---|---|---|
| Waiting | `wait.timeout`, one deadline per sojourn, armed at the first moment a require-target is pending (D3) | → Failed(wait), which recovers automatically. With only want- or order-targets pending, the bound is one attempt of each target, by induction over the DAG (N4) |
| Starting | `start.timeout` | |
| StopWait | StopWaitTO, derived | see below |
| Stopping | `stop.timeout` | |
| Killing | `kill.timeout` | |
| Backoff | backoff delay | |

**StopWaitTO** is not a configuration field. `Compile` computes it:

- SW(B) = max over **all dependents** A of (SW(A) + stop(A) + kill(A)) + ε, with SW = ε for a service without dependents (N3).
  - "All dependents" means every A with B ∈ req(A) ∪ wnt(A) ∪ ord(A), matching G_free.
  - The REVIEW-2 counter-example (`A order B`, where B was terminated after ε while A drained) now waits SW(B) ≥ stop(A) + kill(A).
- The DAG is finite, so SW is finite.
- **If it fires**, B is terminated anyway, and the overrun is reported as a T2 violation. Only two cases can cause that:
  - a dependent is Leaked. G_free blocks on Leaked, so this case is always reported;
  - a want- or order-dependent began stopping after B had entered StopWait.

**Stable states:** Inactive, Ready, Done, Failed and **Leaked**.

**Theorem B.** Assuming timers fire, every sojourn in a transient state ends within its timer.

- Stopping and Killing each run once per stop. They are separate states, so there is no `forced` reuse.
- **Leaked is not transient, by definition.** It ends only when the OS empties the tree. It is the one stated exception to "bounded stop", and it is reported.
- **Total stop bound:** SW + stop + kill.

### 3.7 Invariants and progress

- **I1** state(A) ∈ {Starting, Ready} ⇒ ∀B ∈ req(A): state(B) ∈ {Ready, Done}. This holds after propagation, because of G_lost.
- **I2, on trees** (H2):
  - tree(A) populated ⇒ state(A) ∈ {Starting, Ready, StopWait, Stopping, Killing, Leaked};
  - state(A) ∈ {Inactive, Waiting, Backoff, Done, Failed} ⇒ tree(A) empty.

  The tree is part of the model state: populated or empty.
- **I3:**
  - state ∈ {Waiting, Starting, Ready, Done, Backoff} ⇒ goal = up, after propagation;
  - Inactive ⇒ goal = down.
- **I4 (restated in draft 9, F3):** the wanted set is closed under require and want along **uncut** edges: if goal(A) = up and A → C is a require or want edge with ¬cut(A, C), then goal(C) = up (§2.3). A cut edge (A ∈ grant(p), C ∉ grant(p)) propagates nothing, for every holder, owner included.
- **I4w (E4):** state(A) = Waiting ∧ wexp ⇒ armed; and state(A) = Waiting ∧ pending ⇒ armed, after propagation.
- **I5** (M8, N1): state(A) ≠ Failed ⇒ |R_A(now)| ≤ intensity.max.
  - Failed may hold max + 1 until the window drains or Retry clears it.
  - mode = Running ⇒ |R_inst(now)| ≤ max_inst.
  - **R_p is per delegated principal (draft 9).** An owner-class Retry of a granted service pays R_p (payer(A), §3.3); that payment is never refused and never changes `mode`.
  - **R_p (D4).** API-originated payments to R_p by p itself are refused when they would take |R_p(now)| past budget_p. Crash payments cannot be refused: one that passes the budget sends the crashed service to Failed(intensity), refuses p's writes until the window drains, and never changes `mode`. So the invariant is not |R_p| ≤ budget_p but: **an overflow of R_p never changes `mode`.** Since E1 this clause is unconditional: a service whose crashes pay R_p is in grant(p), hence never terminal, so its Failed(intensity) cannot escalate either (draft 7's clause was false for an owner service run as p).
- **I6, on trees:** at most one populated tree per service. Spawn happens only from Waiting, where I2 says the tree is empty, and Leaked blocks Retry.
- **T1:** spawn only when G_ok holds.
- **T2:** B is signalled (term) only when G_free(B) holds, or when StopWaitTO fires and the overrun is reported. Escalation and rollback stop services through this path (H1).

**Progress, under weak fairness of timers and reports:**

- **P1:** goal = down ⇒ ◇ state ∈ {Inactive, Failed, Leaked}.
- **P2 is split in draft 9 (F5).** Draft 8's P2 concluded ◇{Ready, Done, Failed} from charges alone, but in abstract time a window can drain between two charges, so a service that crashes forever at a rate below max/window never reaches Failed(intensity), and it omitted Leaked and operator restarts. Draft 9 states two properties whose conjunction gives P2 in real time.
  - **P2a (Gate 1, abstract time):** ◇□(goal(A) = up ∧ ∀B ∈ req(A): state(B) ∈ {Ready, Done}) ∧ **finitely many SEt for A** ∧ **finitely many operator Restart of A** ⇒ ◇ state(A) ∈ {Ready, Done, Failed, Leaked} ∨ □◇ Charge(A).
    - The SEt conjunct is an environment hypothesis, stated (E5, §3.1): a pre-fork transient error that recurs forever is uncharged by design. What holds without it is **P2-rate**: each such cycle lasts at least backoff.max, and every SEt is reported.
    - Gate 1 checks P2a by SCC search with SEt and Restart as fairness constraints: every fair SCC reachable under the hypothesis either contains a stable state of the conclusion or a Charge(A) transition.
  - **P2b [Compile+9]:** `Compile` refuses `ErrNeverFailingLoop{service, window, need}` when intensity.window(A) ≤ intensity.max(A) × U(A), with `need` = max × U(A) + 1 s. U(A) is defined here: the longest time between two consecutive charges of A that never reaches Ready, start.timeout + stop.timeout + kill.timeout + backoff.max (all post-meet values). Then max + 1 consecutive charges span at most max × U(A) < window, so they all lie in one window, and □◇Charge(A) without Ready reaches the overflow and Failed(intensity). Charges after a Ready already satisfy P2's conclusion.
  - P2a ∧ P2b ⇒ P2: goal up with requirements eventually stable, finitely many SEt and operator restarts ⇒ ◇ state ∈ {Ready, Done, Failed, Leaked}.
- **P3:** Theorem R. Every Retry pays payer(A): R_p for A ∈ grant(p), whoever the caller is, and R_inst otherwise (N2, D4; draft 9 for the owner Retry of a granted service). Crashes pay payer(A): R_p for a service run as p, R_inst otherwise (D2, option A).

### 3.8 Verification (M9)

**The table is data.**

```go
type Cell struct {
	Target  State     // or Same
	Actions ActionSet // bitset: Spawn, Term, Kill, Record, Charge, Escalate, ArmLiveness, ArmWait, SetWexp, Report, …
	Cond    CondID    // e.g. GoalUp, ReasonRecoverable, Pending, Overflow
	Prio    uint8     // guard grid only: order within the row, unique (F6)
	Reach   Reach     // Reachable, Unreachable(ByEpoch|ByGen|O1|O2|O3), or Absent (guard grid only, F6)
}
```

- **Totality with `Absent` (F6).** Every one of the 88 guard cells is explicit: present (with a unique `Prio`) or `Absent`. The zero Cell is still invalid, so a forgotten cell fails at load. The BFS asserts that the evaluator never chooses an `Absent` cell.

- The event table, the guard table, settle (as a decision list) and `permits` are all data of this form.
- **Sojourn bits (E4).** The model state of a service includes `wexp` and `armed`, reset by every state entry. A self-move is a Cell with Target = Same and a non-empty ActionSet (G_arm: `ArmWait`; Waiting × WTO: `SetWexp`, then the `Pending` condition chooses Failed(wait) or Same). A Same cell with an empty ActionSet is `·`.
- **Overflow (E3).** settle rule 4 and `restart` are decision-list entries with `Charge`, followed by `Escalate` when the owner's counter overflows, and a target chosen by the `Overflow` condition of §3.2.
- There are no closures. Goal derivation, charging and holder updates are fixed formulas over that data.
- **Gate 1:** a Go exhaustive test over the same values that the runtime executes.
  - Totality: the zero Cell is invalid.
  - BFS over every labelled DAG with N ≤ 4, with:
    - two abstract holders;
    - an explicit tree bit per service;
    - all orderings permitted by O1–O3;
    - **same-epoch stale timers**, i.e. an earlier generation;
    - **same-generation stale watchdog deadlines**, i.e. a deadline queued before a keepalive (N1). The liveness source's re-arm is part of the model, not only the table (R3-L1);
    - **same-generation stale WTO**, i.e. a wait deadline that fires after every require-target became Ready, and a require-target that toggles Ready/not-Ready within one sojourn (D3);
    - **oneshots stopped in every way**: by StartTO, by a dependency loss, by an operator restart, and by a natural exit during StopWait (D1);
    - **post-fork spawn failures** through the status pipe, with a populated tree (D7);
    - abstract time, so that restart-window entries age out (N1);
    - two principals, owner and delegated, with grants closed under every edge kind and the grant-boundary cut of §2.3 (N2, E2, F3), and budgets small enough that R_A, R_p and R_inst each overflow within the scope (E3);
    - Retry and Restart at every state.
  - Checks:
    - I1–I6 (with I4w), T1 and T2;
    - **Theorem G, item 1, as 2-safety by self-composition (REVIEW-5 Gate 1, draft 9).** Draft 8's check compared one step with and without the principal's action, from the same state. That is a 1-step, 1-run property: it cannot see a high input whose effect on obs_O appears several steps later, through a holder or a goal (F3 route 1 passed it). Draft 9 checks the 2-run property directly:
      - **product BFS** over pairs (s₁, s₂) of model states, starting from equal initial states;
      - every **low** input (owner and reader operations, events of services in O, abstract time ticks, the request deadline of §3.1, which is modelled as a low input) is applied to **both** copies; every **high** input (operations of p, events of services in grant(p)) is applied to **one** copy at a time, chosen by the BFS;
      - the **oracle** values (the StopWait → Stopping time of G_free and SWTO for C ∈ O with a dependent in grant(p), Theorem G item 2) are modelled as a low input g_C chosen by the BFS and applied to both copies; a separate one-copy check asserts that the real G_free/SWTO move always lies within the oracle's range;
      - the assertion: obs_O(s₁) = obs_O(s₂) at every reachable pair;
      - scope: N ≤ 3 with symmetry reduction over service and holder permutations, because the product squares the state space; the one-copy properties keep N ≤ 4;
      - **required witnesses**, each a named fixture: the check is **red** on draft 8's goal formula (fixture `goalDraft8`, holder scope without the cut, red through F3 route 1), **green** on draft 9's, **red** on the mutant that drops `ErrGrantNotClosed` for want edges, and **red** on the Retry cell mutant `pay(caller)` (the owner-Retry finding of §3.3);
      - **stated limit:** E5 (svd's own resources) and A1 (OS privilege) are outside the model; the F1 chaos test and the A1 probes of §10 cover them;
    - **D1:** a oneshot whose exit is `sig` never reaches Done;
    - propagation termination, with the measured maximum number of moves recorded in a golden file and asserted (no closed-form bound, R3-L8), and confluence;
    - the guard evaluator never reads an `Absent` cell, and each service moves at most 2 times per propagation with fixed inputs (§3.5);
    - SCC search for P1, P2a and P3 under fairness, with P2a's SEt and operator-Restart hypotheses as fairness constraints (E5, F5). P2b is a `Compile` check, tested in §10.
- **Gate 2, mutation.**
  - Every Reachable cell is mutated: every other Target, and every one-action change. Each mutation must turn Gate 1 red.
  - An equivalent mutation, with the same Target and Actions, is not a mutation.
  - **Priority mutations (F6).** In every guard row with two or more present cells, every swap of two `Prio` values is a mutation and must turn Gate 1 red, or be listed as equivalent with the reason (two guards that are mutually exclusive in every reachable state).
  - **Label-only mutations are exempt (N13).** These are mutations that change only `reason` (e.g. Failed(wait) ↔ Failed(dep), which recover identically) or only the `Report` action. They violate no behavioural property. The list is asserted, and a unit test per cell checks the reason and the report.
  - **Unreachable cells are exempt, and the list of exemptions is itself asserted.** The BFS must never visit a cell marked Unreachable.
- **Gate 3:** a TLA+ module generated from the same data, *including propagation, settle and holders*, checked by TLC on a schedule for N ≤ 5 (Lamport, `tla/high-level-view.html`).

**Stated limits.**

- **Small scope.** N ≤ 4 or 5 is a hypothesis, not a proof for every N.
- **Real durations** are not modelled. The duration-dependent obligations (`ErrWaitTooShort` and StopWaitTO) are checked by `Compile` and its tests.
- **What a wrong cell can slip through.** The properties are written by hand, independently of the table, so a wrong cell that violates one of them is caught. A wrong cell that violates none of them is allowed by the specification, and only review catches it.

**Prior art.** One search found no TLA+ specification of an init system or supervisor, which "is not proof of absence" (B§11). Castro et al., "A Case Study on Verifying a Supervisor Component Using McErlang", PROLE 2010, model-checks a supervisor (B§11, abstract only). **No novelty is claimed.**

---

## 4. Readiness and liveness: push first

### 4.1 Readiness channels

`readiness.channel` is class D, K1.

1. **`fd`.** The preferred channel, on every OS.
   - Each epoch gets a pipe, whose write end is inherited at fd N.
   - `Rise` = the first `\n` (the s6 and dinit protocol, B§3.4).
   - **It cannot be forged**: only processes that inherited the fd can write to it.
   - A grandchild holding the write end makes "closed without ready" undetectable. StartTO is then the only bound (L1, documented).
2. **`sd_notify`.** For daemons linked to libsystemd.
   - **Path sockets only, never abstract** (H5).
   - The socket is an AF_UNIX datagram socket in a per-epoch directory: 0700, owned by the service's run_as uid, inside the instance runtime directory (§8.1).
   - **Sender check** (H5, the analogue of systemd's `NotifyAccess=main`):
     - Linux: `SO_PASSCRED`/`SCM_CREDENTIALS` gives the pid, which must equal the epoch's main pid. **The pid is the only check (N5).**
       - A cgroup-membership check would drop a message whose sender has already exited, because `/proc/<pid>` is gone. That is systemd's race again.
       - The pid alone is sufficient: the main process stays a zombie until svd reaps it, and O1 drains the socket before reaping (observe with `WNOWAIT` or a pidfd, drain, then reap: §3.1, R3-L2), so the pid cannot be reused within that window. A message that arrives after the reap is late anyway: after MainExit, Rise is a no-op (§3.4).
     - FreeBSD: `SCM_CREDS` *(unverified)*;
     - **macOS: the `sd_notify` channel is unavailable**, because datagram sender credentials are not available there *(unverified)*. Asking for it there is `ErrProtectionUnavailable`; use `fd`.
   - Messages from any other sender are dropped and counted.
   - `READY=1` → Rise. `BARRIER=1` is honoured (`man/sd_notify.xml:83`, `:495` @ v262.2).
   - **Nested daemons** (L1). The exec-helper sets `NOTIFY_SOCKET` for the main process. A grandchild's `READY=1` fails the main-pid check, so it cannot cause a premature Rise.
   - **The same protocol goes upward.** svd reports its own readiness to the host init (mode B), or to `/run/host/notify` in a container (A§1.7).
3. **`probe`.** The fallback: exec, tcp or http, polled. `plan` shows `readiness = polled`.
4. **`exit`.** For oneshots.

**Formal semantics:**

- Push: Rise is the first ready message of the current epoch.
- Poll: probes run at t_k = t₀ + k·period, with 0 < timeout < period, so at most one is in flight. Rise is the least k with r consecutive passes.
- In both cases, StartTO bounds the wait. **O1 orders the channel before MainExit** (L1).
- Readiness is latched per epoch.

### 4.2 Startup gate

Liveness is armed only on entering Ready, and its events carry the generation. In Starting, Fall is unreachable (⊘). This is the structural form of Kubernetes' startup probe (B§9).

### 4.3 Liveness

- **`watchdog`** (push). The keepalive is `WATCHDOG=1` from the main process, or `\n` on a second inherited fd. Fall fires when no keepalive has arrived in (t − watchdog, t] (cf. `WatchdogSec=`, `systemd.service.xml:879` @ v262.2).
- **`probe`** (poll). Fall fires on f consecutive failures.

**Liveness does not cascade:**

1. A liveness check observes only its own service. A probe that tests a dependency is a recipe error, which review must catch.
2. A dependency's failure reaches dependents only through G_lost, uncharged.
3. Every Fall is charged, so a probe that flaps under load hits intensity, then Failed or escalation.

**Health endpoint.** `/v1/health` returns:

- 200 iff every service with goal = up is in Ready or Done;
- 503 with the list otherwise.

---

## 5. Process control

### 5.1 Trees, reaping, and what may outlive the supervisor (M10)

**Reaping.**

- One reaper routes exits to epochs. It observes with `waitid(…, WNOWAIT)` (or pidfds), lets O1 and O4 drain the channel and the status pipe, and only then reaps (R3-L2). A plain `wait4(-1)` would reap before the drain.
- As PID 1, svd also reaps orphans.

**Tree tracking:**

| OS | mechanism |
|---|---|
| Linux | a cgroup per epoch |
| Windows | a job object with `KILL_ON_JOB_CLOSE`, assigned at creation |
| FreeBSD | `PROC_REAP_ACQUIRE` |
| macOS | the process group only |

**Pdeathsig is a secondary measure.**

- On Linux, `Pdeathsig` belongs to the **forking thread** (golang/go#27505 *(unverified)*).
- All spawns therefore come from **one dedicated goroutine** that has called `runtime.LockOSThread` and never returns, so that thread never exits while svd runs.

**What may outlive svd. This is the stated exception to "children never outlive svd":**

- trees in **Leaked**;
- on macOS, a process that called `setsid` and double-forked out of the process group.

Both are listed at exit, on stderr and in the upward `STATUS=`.

### 5.2 No daemonization

- There is no `forking` mode and no pidfile support (`ErrForkingDaemon`).
- The main process is the one spawned. Its exit leads to StopWait, then to a stop of the rest of the tree (§3.4).
- A macOS double-fork escape is covered by a chaos test.

### 5.3 Nested behind another init, and without root

There are three modes:

- **C:** PID 1 in a container;
- **B:** one service of the host init;
- **U:** a per-user instance.

**Linux, systemd.**

- Mode B runs svd as `Type=notify` with `Delegate=yes` (`systemd.resource-control.xml:1421` @ v262.2).
- Mode U runs it as the user manager's unit, kept alive by `loginctl enable-linger` (A§1.6).
- svd moves itself into a sub-cgroup. It never writes outside its subtree: the single-writer rule (A§1.7) is enforced on itself and tested.
- Without delegation, a cgroup limit gives `ErrProtectionUnavailable`.

**Linux, s6, runit, OpenRC.**

- svd runs in the foreground.
- Trees are tracked by process group unless svd is started in a writable cgroup.

**macOS.**

- A LaunchDaemon runs the system instance; modifying the system domain needs root (`launchctl.1` L37-42).
- Mode U is a LaunchAgent in `gui/<uid>` or `user/<uid>`. An agent is "the only way" to run as a user (`launchd.8` L58-68), and `user/<uid>` can exist without a login (L44-48, A§2.6).

**Windows.**

- The system instance is an SCM service.
- Users cannot author SCM services (A§3.6), so mode U is a logon-started process with nested jobs.
- Delegation uses svd's own grants, never SCM DACLs: granting CHANGE_CONFIG or STOP can lead to LocalSystem (A§3.6).

**FreeBSD.**

- The system instance runs under `rc.d`.
- Mode U is a session-started process with `PROC_REAP_ACQUIRE`. No per-user manager is documented (A§6.6).

**Unprivileged hardening in mode U:**

- Linux: NNP, Landlock, seccomp, and user namespaces where they are allowed;
- Windows: a restricted token, AppContainer and mitigation policies;
- FreeBSD: `PROC_NO_NEW_PRIVS_CTL` and Capsicum.

A mechanism that needs root gives `ErrProtectionUnavailable`, unless the OS is in that protection's `exempt_on`.

---

## 6. Credentials

- **Never in the environment** (`ErrSecretInEnvironment`). systemd lists the reasons: inheritance, size, binary data (A§1.8).
- **Delivery** is through `$CREDENTIALS_DIRECTORY`, decrypted per epoch by the exec-helper (sdcreds, linked natively).
- **Where the delivery directory lives:**
  - Linux: ramfs or tmpfs;
  - FreeBSD: tmpfs or md *(unverified)*;
  - Windows: a DACL-protected directory, plus a DPAPI LOCAL_MACHINE wrap;
  - **macOS: no credential ever touches disk** (user decision, L3). The delivery mechanism on macOS is open, with two candidates:
    - an inherited fd per credential (a pipe that the exec-helper fills);
    - the keychain *(unverified)*.

    Until one is chosen, a service with credentials on macOS gives `ErrProtectionUnavailable`.
- **Mode C.** Credentials arrive through `/run/host/credentials/` or `$CREDENTIALS_DIRECTORY` (A§1.7–1.8).
- **Mode U.** A per-user host key is used. It is weaker against root, and that is stated.
- **Authority.** Credential names are class D (recipe); sources are class P (site) (§1.7).

---

## 7. Exec-helper

### 7.1 Setup order

| OS | order |
|---|---|
| Linux | mount ns → NNP → Landlock → ids → caps → seccomp, last |
| FreeBSD | jail → NNP → ids |
| Windows | `CreateProcessAsUserW` attributes |
| macOS | ids → rlimits → umask; seatbelt is opt-in |

The plan arrives on an inherited fd.

**Refusal rule.** Refuse to start when the protection is ✓ in the plan, the host OS ∉ that protection's `exempt_on`, and the mechanism is unavailable.

### 7.2 Paths on the host (M11)

The exec-helper resolves every region on the target host.

- **Linux:**
  - open with `openat2(RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS)`;
  - give Landlock **the opened fd**, not the path;
  - this closes the Compile-to-exec TOCTOU window.
- **macOS:**
  - compare under the volume's case sensitivity (`pathconf _PC_CASE_SENSITIVE` *(unverified)*);
  - refuse symlinked components (`O_NOFOLLOW` per component).
- **Windows:**
  - refuse UNC paths, device paths and reparse points (junctions, symlinks);
  - expand 8.3 names (`GetLongPathNameW`);
  - compare case-insensitively *(unverified)*.
- **FreeBSD:** `O_RESOLVE_BENEATH` and per-component `O_NOFOLLOW` *(unverified)*.
- **Conflicts.** `needs ⊆ allows` for regions is evaluated here, under these rules. A failure gives `ErrConflict`. The helper is a forked child inside the epoch's tree, so this is a **post-fork** permanent failure (O4, D7): the helper writes `perm` on the status pipe and exits. The tree empties through StopWait, and settle rule 2 sends the service to Failed(spawn), without charging and without retries (N11). The same path carries `ErrProtectionUnavailable` (§7.1 refusal rule) and every `execve` error.

---

## 8. API, authorization, CLI

### 8.1 Endpoints that cannot be squatted (H5)

**Instance runtime directory, checked at startup.** Every component must be owned by root or the owner, and none may be group- or world-writable; otherwise `ErrUnsafePath`.

| OS | system instance | mode U |
|---|---|---|
| Linux | `/run/<bin>/<instance>/`, root, 0755 | `$XDG_RUNTIME_DIR/<bin>/` (checked, not trusted) |
| macOS | `/var/run/<bin>/` | `~/Library/Application Support/<bin>/run/`, 0700, created and checked by svd. macOS has no `XDG_RUNTIME_DIR`. The AF_UNIX path must fit in 104 bytes, or the result is `ErrPathTooLong` *(limit unverified)* |
| FreeBSD | `/var/run/<bin>/` | `~/.local/state/<bin>/run/`, 0700 |
| Windows | see below | see below |

**API socket.**

- An AF_UNIX stream socket in that directory.
- Authentication by peer credentials: `SO_PEERCRED` or `getpeereid`.
- **Per-principal listeners (F1, H_OS-E5, draft 9).** Besides the owner-class socket, svd opens one listener per delegated principal p, in a subdirectory of mode 0700 whose only other access is a per-principal ACL for p *(the ACL mechanism per OS is unverified; open item, §12)*, and one listener shared by readers. Peer credentials are still checked on every connection; the socket a connection arrived on is a second check, never a substitute. Listeners are served round-robin, and each has its own `budget.connections`, `budget.inflight` and `budget.refusals` (§3.3, H_OS-E5), so p's connections cannot exhaust what the owner and other principals need.

**Windows.**

- The preferred transport is **AF_UNIX** (Windows 10 1803+ *(unverified)*), in an ACL-protected directory, with peer identity from `SIO_AF_UNIX_GETPEERPID` *(unverified)*.
- If that peer check is not available, a named pipe is used, under these rules:
  - the name is random;
  - it is published in a file in the ACL-protected instance directory;
  - the server creates it with `FILE_FLAG_FIRST_PIPE_INSTANCE`;
  - clients open it with `SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION`;
  - clients check `GetNamedPipeServerProcessId` against the published server pid.

  All of these points are *(unverified)*, and they are an open item before P3.

**sd_notify sockets.** Path sockets only, in per-epoch 0700 directories, with the sender check of §4.1.

### 8.2 Authorization (K5 grants, user decision)

- `api.admin` and `api.read` are K5 sets of principals.
- They are written by site authority only, and B must state them (§1.5). Unconfigured never means everyone.
- They can only shrink across layers.
- The instance owner is always admin; no layer can remove that.
- Reads are not open to everyone. systemd's are, through polkit for writes (A§1.6); we differ on purpose.

**Subtree delegation.**

- A principal may be made admin over a K5 set of services, grant(p).
  - A service with `terminal = ✓` cannot be in any grant (`ErrFieldAuthority`). A terminal service is never delegated: draft 5's phrase "a terminal service inside grant(p) is the owner's explicit choice" contradicted this and is removed (R3-L10).
  - Every non-owner operation passes the effect-set check, and its API charges pay R_p (§3.3).
  - **Grant closure (D2 option A, E2).** If A ∈ grant(p) and B has an edge to A of **any** kind (require, want or order), then B ∈ grant(p); by induction the grant is closed under the reverse closure over every edge kind, so no path from outside the grant enters it. A violation is the Compile error **`ErrGrantNotClosed{principal, granted, dependent, edge}`**, which names the edge kind. Since terminal services are never in a grant, no terminal service has a path to a delegated one.
    - *Why every kind, not only require (E2).* A want or order edge never fails its source, but its source's own require-dependents are bounded by WaitTO, whose budget W assumes one attempt of each upstream service (§1.6). A delegated principal's `restart` is not one attempt: with T terminal, T requires D, D wants A ∈ grant(p), p restarting A before each start.timeout keeps A unsettled until T's wait deadline fails T → Quiescing(1). Closure over every kind removes the path; it also makes the effect-set check of §3.3 redundant for grant-internal targets.
    - *What the owner gives up.* No owner service may depend on a delegated one in any way, ordering included. A dependency of the owner's on a delegated service must be written without an edge (the owner's daemon retries its peer itself, as for the mutual peers of §2.2), or the service must be kept out of every grant.
    - *Edges out of a grant are allowed* (E14): a granted A may require, want or order an outside C. Their only influence on C is the bounded StopWait timing of Theorem G item 2, and C's goal is never raised by a delegated holder (§2.3).
- **Grants and accounts (E1, F2).** runs_as(X, p) ⟺ X ∈ grant(p), both directions, for every non-owner-class principal p, on resolved uid/SID and group set (§1.6, `ErrDelegationUid`), and a granted service holds no privilege beyond p's (`ErrDelegationPrivilege`).
- **Principal classes (F4, draft 9) [Compile+9].** Every principal named in `api.admin`, `api.read` or a grant is in exactly one class (§1.6, `ErrPrincipalClass`):
  - **owner-class**: the instance owner and every member of `api.admin`; grant = ∅; may act on every service;
  - **delegated**: grant(p) ≠ ∅, p ∉ `api.admin`; may write only to grant(p); pays R_p;
  - **reader**: in `api.read` only; no write; no service may run as a reader (the reader direction of `ErrDelegationUid`).
- **Withdrawn in draft 9 (F10):** draft 8 let a granted subtree's Linux cgroup be handed to the principal's own instance. svd never hands a tree container to a principal (H_OS-E2, §3.3).

### 8.3 Endpoints and CLI

**Endpoints:**

- `/v1/services`;
- `/v1/plan`, with provenance per field, its authority, and `notEnforced`;
- `/v1/logs`;
- `/v1/health`;
- `/v1/changes`, the audit log.

**Rules:**

- Transactions follow §2.4.
- In Quiescing, writes return `ErrQuiescing`.
- Layers cannot be added at runtime.

**CLI** (cobra):

- `run [--user]`;
- `start [--no-rollback]`, `stop`, `restart`;
- `status`, `plan`, `validate`, `logs`, `version`.

---

## 9. Logs

- A pipe per service feeds a ring buffer and a file.
- The file is opened `O_NOFOLLOW` and rotated by size and count (class T).
- A slow sink drops lines and counts the drops. It never blocks a child.
- Logs are never deleted on restart. supervisord does delete them (B§12).

---

## 10. Tests

**Algebra.**

- Fuzz A, C, I and U per kind, and fuzz Z against ⊥err.
- Permutation and duplicate invariance of `Compile`.
- `ErrNoBaseline` and `ErrBaselineNotTotal`.
- A field no layer mentions takes B's value, never the universe (C1).
- `ErrWideningField` for every class-D field, including new service keys and env keys (C2).
- `ErrFieldAuthority` for every class-G field in a recipe.
- `needs ⊄ allows` → `ErrConflict` (M1).
- `prefer` resolution (D-M2, E10): a site prefer inside the met interval wins over the recipe's; a site prefer outside it is `ErrConflict`, tested before the singleton case; no resolved value ever leaves the met interval.
- eff-monotonicity per field and per OS (M3).

**FSM.** Gates 1–3 of §3.8.

**Event sources.**

- O1: a ready message written just before exit must be seen first.
- O2: TreeEmpty must follow MainExit, with workers that outlive their master.
- Same-epoch stale timers (H4).
- A forged `READY=1` from a non-main process must be dropped (H5).
- A grandchild's `READY=1` must be dropped (L1).

**Scenarios.**

- REVIEW-1's counter-examples, each as a named regression test: C1, C2, H1–H4, M5, M6 and M8.
- Escalation:
  - a cascade of N dependents charges once;
  - an owner Retry loop escalates;
  - a delegated principal's Retry loop, or `stop`/`start` loop, is refused with `ErrPrincipalBudget` once R_p is spent (N2), and REVIEW-3 D2's re-enabled crashes pay R_p, never R_inst: mode stays Running;
  - a terminal service, or any service outside grant(p), that requires a granted service is `ErrGrantNotClosed` at Compile (D2, second route);
  - REVIEW-4 E1: an owner service X ∉ grant(p) with run_as(X) = p is `ErrDelegationUid{X, p, outside-grant}`; a granted service not run as p is `ErrDelegationUid{…, granted}`; a service in two grants is refused;
  - REVIEW-4 E2: D ∉ grant wanting, or ordering after, A ∈ grant(p) is `ErrGrantNotClosed{p, A, D, want|order}`; one case per edge kind, each asserting the `edge` field;
  - grant-boundary cut (draft 8's holder scope): p's `start A`, A ∈ grant(p) requiring C ∉ grant with C's goal down, leaves C Inactive and ends in A Failed(wait) with p's transaction rolled back; C's goal never moves;
  - E3: settle rule 4 and `restart` reach Failed(intensity) on an R_p overflow alone, and Quiescing(1) on an R_inst overflow alone, each with |R_A| ≤ max;
  - E4: a require-target pending at entry, Ready, pending again: one WTO with the sojourn's gen, armed by G_arm once;
  - E7: a oneshot's X0 dequeued after `term` records `sig` and is re-run; E8: Starting × Rs during setup, then a `perm` from the helper, reaches Failed(spawn) uncharged;
  - E9: A require B, want C, C's one attempt close to U(C): with wait.timeout(A) ≥ W(A), WTO does not fire while B is Ready, and `wait.timeout(A) < W(A)` is `ErrWaitTooShort`;
  - E10: Π_s = {30 s, 200 s} against I = [5 s, 120 s] is `ErrConflict`, and so is Π_s = {60 s, 30 s} against the singleton [60 s, 60 s];
  - E11: one case per row of the O4 classification table; E12: a Windows path parameter `C:\data\db` is accepted under the Windows alphabet, `a,b` and `x=y` are refused there;
  - E14: a Leaked granted A that requires an outside C delays C's stop by at most SW(C), with a T2 overrun report, and C still reaches Stopping;
  - a oneshot that exits 0 during StopWait before `term` is Done (rule D1-n);
  - a oneshot stopped by StartTO, by a dependency loss, or by a restart is never Done (D1);
  - a Retry leaves |R_A| = 0; a crash of a service run as p pays R_p and never R_inst, and a crash of any other service pays R_inst and never R_p (D4, D2);
  - a wait deadline that fires after the last require-target became Ready does not fail the service; a toggling require-target does not restart the budget (D3);
  - a schema-1 baseline under a version that adds a K5 or K3 field compiles, with the field tagged `legacy`; a baseline with a newer schema than the binary is `ErrSchemaTooNew` (D5);
  - A require B, B want C, C mid-restart: A does not reach Failed(wait) when `wait.timeout(A)` passes `ErrWaitTooShort` (D6);
  - a region conflict found by the helper reaches Failed(spawn) only after TreeEmpty (D7);
  - a pre-fork `EMFILE` loop never charges R_inst (R3-L9);
  - an operation whose effect set leaves the grant is refused (N2);
  - one crash per day never reaches Failed(intensity) (N1);
  - a keepalive just before a queued deadline does not yield Fall (N1);
  - `A order B` shutdown waits for A (N3);
  - a flapping want-target never fails its wanter (N4);
  - a schema-N baseline loads under schema N+1 with legacy values, and eff does not widen (N9);
  - `"60s"` and `{60s, 60s, prefer 60s}` give the same plan (N10);
  - a oneshot that completes during StopWait(dep) is Done and is not re-run (N11);
  - `stop X` while X is still wanted → `ErrStillWanted` (N12);
  - SIGTERM during Quiescing(1) exits 1 (N15);
  - Quiescing refuses writes.
- **REVIEW-5 (draft 9)**, each a named regression test:
  - F2: one case per `ErrDelegationPrivilege` `what` (capabilities, nnp, groups, uid0), each asserting the `service`, `principal` and `what` fields; `ErrAccountChanged` when p's account is renamed or re-numbered between load and fork; the helper reports `perm` when resuid/resgid/groups after setuid differ from p's;
  - F3 route 1: the owner holds A ∈ grant(p), A requires C ∉ grant(p) with C down; C's goal stays down, and p's `restart A` or `stop A` never changes C's state or goal (obs_O equal with and without p's operation); F3 route 2: the owner's `start A` gives T = {A}, and `WarnUnheldOutsideTarget{A, C}` is emitted at Compile;
  - F4: a principal in both `api.admin` and a grant is `ErrPrincipalClass`; a service run as a reader is `ErrDelegationUid{…, reader}`;
  - F5: intensity.window ≤ max × U(A) is `ErrNeverFailingLoop` with the `need` field; a service that crashes forever with window > max × U(A) reaches Failed(intensity);
  - F6: a guard row with a missing cell fails the table load; an `Absent` cell is never evaluated; swapping G_failed and G_arm in Waiting turns Gate 1 red;
  - F8: a helper `perm` arriving while the service is Stopping, Killing or Leaked is recorded as `perm`, and the service reaches Failed(spawn) (or Failed(unkillable) from Leaked), uncharged;
  - F9: a delegated Retry with R_p one below budget pays once, succeeds, and leaves |R_p| = budget; a second Retry is `ErrPrincipalBudget`;
  - owner Retry of A ∈ grant(p) pays R_p, never R_inst, never changes `mode`, also at |R_p| = budget_p (§3.3, new in draft 9);
  - F11: a notify socket flooded with `SCM_RIGHTS` messages: every message with `MSG_CTRUNC` is dropped, svd's fd count stays below its headroom, and no fd leaks (count before and after);
  - F2 A1 probes, per OS: from a granted service run as p, `kill(π, 0)`, `ptrace` attach, `process_vm_writev` and an open of `/proc/π/mem` against an outside service's pid π all fail (Linux; the macOS, FreeBSD and Windows equivalents are listed with the open items of §12).
- Rollback removes only its own holder.

**Per OS and mode.** Real start and stop in modes B and U, and mode C on Linux. Delegation never writes outside its subtree.

**Chaos.**

- `kill -9` of svd;
- a child that ignores signals (→ Killing → Leaked → Failed(unkillable) once the tree empties);
- a master that exits while its workers hold the port (H2);
- a double-fork;
- a zombie storm;
- a fork bomb;
- a log flood;
- a flapping probe;
- endpoint squatting on every OS (H5);
- **F1, the E5 budgets (draft 9):** a granted service floods its notify socket and the event intake while p holds `budget.connections` connections on its listener, forks up to its pids limit and allocates up to its memory limit; meanwhile an outside service is restarted by the owner. Assert: the outside service reaches Ready within its own timers, the owner's API answers within its request deadline, svd's fd count stays below its headroom, and `mode` stays Running.

**Security audit** before v0.1.0, by a fable `security` agent. It covers:

- authority admission;
- peer credentials;
- endpoints;
- per-epoch sockets;
- host path resolution;
- delegation.

---

## 11. Phasing

| phase | content |
|---|---|
| P0 | `Compile`, with kinds, ⊥err, authority, the baseline and completeness, and typed errors; fuzzed laws. FSM data tables with Gates 1–3. |
| P1 = v0.1.0, Linux | modes C, B and U; fd and sd_notify channels; trees through cgroups; intensity, escalation and Quiescing; holders and rollback; exec-helper with `openat2` + Landlock-by-fd; sdcreds; API; chaos; security audit. **The s6 output is decided after this phase** (user). |
| P2 | macOS, including the credential delivery decision |
| P3 | Windows, after the endpoint open item of §8.1 is verified |
| P4 | FreeBSD |

---

## 12. Decisions and open items

**Decided by the user (2026-11-23):**

- R0;
- K5 grants;
- no macOS credentials on disk;
- rollback by default;
- s6 decided after v0.1.0;
- home `go-supervise/supervisor`;
- binary name `svd`;
- D-M2: intervals plus a `prefer` point, with site prefers taking precedence inside the met interval (§1.4);
- D2: option A, delegated services pay their own crashes and grants are closed (§13). Draft 8 applies the user's REVIEW-4 rulings: closure under the reverse of every edge kind (E2), and run_as = delegated principal ⇔ granted (E1). Draft 9 applies REVIEW-5 within those decisions: "runs as" is resolved-id equality (F2), and the grant boundary truncates goal propagation for every holder (F3).

**Left for the user:**

Nothing. The three items left open in draft 6 were decided on 2026-11-23:

1. **D-M2:** intervals plus a `prefer` point (§1.4).
2. **The binary name:** `svd`.
3. **D2:** option A (§13).

**Open technical items, all marked *(unverified)* above:**

- Windows AF_UNIX peer identity, and the named-pipe rules;
- macOS datagram credentials, and the credential delivery mechanism;
- `_PC_CASE_SENSITIVE`;
- FreeBSD `SCM_CREDS` and `O_RESOLVE_BENEATH`;
- golang/go#27505;
- the recipe signature format;
- draft 9 (H_OS-E5 and F2), each *(unverified)* until tested on the OS:
  - per-principal ACLs on listener sockets, per OS (§8.1);
  - dropping the capability bounding set for delegated services (Linux), and the Windows token reduced to `SeChangeNotifyPrivilege` (§1.6);
  - FreeBSD `rctl` with `racct` enabled, and the NPROC + AS fallback;
  - macOS `RLIMIT_NPROC` and `RLIMIT_AS` as a per-principal bound;
  - Windows nested job objects with per-principal limits;
  - `MSG_CTRUNC` behaviour on every OS when the control buffer is too small for `SCM_RIGHTS` (whether the kernel closes the truncated fds);
  - the A1 probes on macOS, FreeBSD and Windows (§10).

**REVIEW-2 resolutions** (none contested):

| finding | resolution |
|---|---|
| N1 | windows read, not evented; watchdog level check |
| N2 | R_p refusals; effect-set check; Theorem G. Draft 5's owner-held charging and proof were withdrawn in draft 6 and replaced by option A in draft 7 (D2, §13) |
| N3 | SW over all dependents; Leaked blocks |
| N4 | settled includes Backoff and Leaked; WaitTO for require-targets only; budget W with backoff and stop |
| N5 | pid-only check |
| N6 | B = meet of site `all {}`; contents restricted |
| N7 | typed parameters |
| N8 | ACLs; fd walk |
| N9 | schema version, legacy values, Theorem E |
| N10 | prefer set; normalisation; phase order |
| N11 | settle order; SEt/SEp |
| N12 | `ErrStillWanted` / `--with-dependents` |
| N13 | label-only mutations exempt |
| N14 | termination by DAG induction |
| N15 | maximum status |
| N16 | home confirmed |

**REVIEW-3 resolutions.** No finding is rejected. D2 waits for the user (§13).

| finding | resolution | where |
|---|---|---|
| D1 | new exit value `sig`, recorded in Stopping, Killing and Leaked; settle rule 3 (Done) reads only a natural `ok`. A oneshot stopped by StartTO, dependency loss or restart is never Done. A oneshot that exits 0 on its own before `term` is Done | §3.2, §3.4, §3.8, §10 |
| D2 | **decided by the user: option A.** payer(A) by `run_as`; grant closure (`ErrGrantNotClosed`); Theorem G proved under H_OS | §3.3, §8.2, §13 |
| D3 | WaitTO: one deadline per sojourn, armed at the first pending moment, never re-armed; level check at fire time; `wexp` flag and guard G_wexp | §3.1, §3.4, §3.5, §3.6 |
| D4 | `bump`, `pay(q)` and `charge(A)` separated; Retry cell is `R_A := ∅; pay(caller)`; API versus crash charges to R_p; I5 restated | §3.2, §3.3, §3.4, §3.7 |
| D5 | completeness at version N_B, fields newer than N_B tagged `legacy` and exempt from the non-⊤ test; Theorem E hypothesis forbids growing an existing K5 universe; `ErrSchemaTooNew` | §1.5, §1.10 |
| D6 | U(B) and W(A) recurse over require, want and order; `ErrWaitTooShort` on require-targets' U | §1.6 |
| D7 | SEt/SEp only before a child exists (O3); post-fork failures go through a CLOEXEC status pipe as MainExit(`perm`/`fail`) and reach Failed(spawn) at TreeEmpty (O4, settle rule 2) | §3.1, §3.2, §3.4, §7.2 |
| R3-L1 | lastKeepalive := Ready entry time; keepalives epoch-filtered; Gate 1 models the source's re-arm | §3.1, §3.8 |
| R3-L2 | observe with `WNOWAIT` or a pidfd, drain, then reap | §3.1, §4.1, §5.1 |
| R3-L3 | K6 `needs` excluded from `all {}`; a global `restart.on` is a K1 assertion that conflicts with recipes stating otherwise | §1.5 |
| R3-L4 | path alphabet restricted; `ErrParamInShell` for a parameter inside a shell command string | §1.6, §1.10 |
| R3-L5 | exhaustive write rights per OS; the check allows only read and execute rights, so the lists are closed | §1.7 |
| R3-L6 | draft 6: a site `prefer` only in the point form. Draft 7 (D-M2 decided): site prefers form their own set and take precedence inside the met interval, so the conflict cannot arise | §1.4, §1.5 |
| R3-L7 | grant check before `ErrStillWanted`; `by` filtered to the grant | §2.3 |
| R3-L8 | "≤ 4N moves" dropped; the measured maximum is a golden value | §3.5, §3.8 |
| R3-L9 | pre-fork transient spawn errors are not charged: Backoff at backoff.max | §3.1, §3.3, §3.4 |
| R3-L10 | contradictory sentence removed with draft 5's proof of Theorem G | §3.3, §8.2 |

**REVIEW-4 resolutions (draft 8).** E1, E2 and E6 follow the user's rulings; the rest are local edits. No finding is rejected; E5, E7 and E14 are each resolved by stating a cost rather than changing behaviour, with the reason given.

| finding | resolution | where |
|---|---|---|
| E1 | run_as(X) = p ⟺ X ∈ grant(p) for every delegated principal p, both directions, `ErrDelegationUid{service, principal, direction}`. Grants of distinct principals are disjoint; terminal services never run as a delegated principal; I5's R_p clause is now unconditional | §1.6, §1.10, §3.3, §3.7, §8.2, §10 |
| E2 | grants closed under the reverse of **every** edge kind; `ErrGrantNotClosed{principal, granted, dependent, edge}`. The per-sojourn `settled` latch (fix b) was not adopted, and the guard table's `settled` is unchanged. Added for the proof: **holder scope**, a delegated holder raises goals inside its grant only (§2.3), so neither p's `start` nor the rollback of p's transaction moves an outside goal. Theorem G re-proved with E1 and E2 | §2.3, §3.3, §3.7, §8.2, §10, §13 |
| E3 | `charge` returns oA and oP after both appends; `overflow(A)`: owner-payer overflow → `Escalate`, then oA ∨ delegated oP → Failed(intensity), else the normal target. Retry's owner payment escalates the same way | §3.2, §3.4, §3.8 |
| E4 | sojourn bit `armed`, reset on entry; Waiting self-guard G_arm (pending ∧ ¬armed → Same, `ArmWait`) ahead of G_ok; Cell expresses self-moves as Target Same with actions; guard grid 88 cells; termination still ≤ 3 moves with self-moves; I4w | §3.1, §3.2, §3.4, §3.5, §3.7, §3.8 |
| E5 | P2 restated with the hypothesis "SEt occurs finitely often", checked by Gate 1 as a fairness constraint; P2-rate holds without it. Charging the k-th SEt and making unclassified pre-fork errors permanent are rejected (reasons in §3.1) | §3.1, §3.7, §3.8 |
| E6 | H_OS split into enforced (H_OS-E1…E4: uid separation, cgroup delegation, class-H plan, graph and goal structure) and assumed (H_OS-A1 OS privilege, H_OS-A2 non-interference with the excluded channels named). Theorem G is conditional on A1 and A2 | §3.3 |
| E7 | documented, not changed: classification is by the state at dequeue; `WIFSIGNALED` cannot tell a graceful SIGTERM handler from a natural exit 0; oneshot obligation stated; D1-n holds up to the step boundary | §3.2 |
| E8 | StopWait × X1 records `fail` or `perm` from the status pipe | §3.2, §3.4 |
| E9 | `ErrWaitTooShort` checks wait.timeout against W(A), over every target kind, for every service with a require-target | §1.6, §10 |
| E10 | Π_s ⊄ I is tested first, unconditionally, singleton included | §1.4, §10 |
| E11 | O4 classification table (`perm` / `fail` per errno and setup step); unclassified post-fork errors are `fail`, hence charged and bounded | §3.1, §10 |
| E12 | path alphabet per OS family; Windows alphabet with drive prefix and `\`, without `,`, `=`, `/` | §1.6, §10 |
| E13 | the cause constant `exit` renamed `natural` | §3.2, §3.3 |
| E14 | edges out of a grant stay allowed; their influence (the length of an outside target's StopWait, ≤ SW, reported as T2 overrun) is named in Theorem G item 2 instead of forbidden, because forbidding them would stop a delegated application from requiring the owner's database | §3.3, §8.2, §10 |

**REVIEW-5 resolutions (draft 9).** No finding is rejected. Where REVIEW-5 offered alternatives, the choice is given.

| finding | resolution | where |
|---|---|---|
| E12 note | **accepted, stated:** the Windows path alphabet has no space, so a parameter such as `C:\Program Files\…` cannot be passed as a parameter; such a path must be written in the recipe itself. Adding the space would reopen argument splitting in shell strings, which the alphabet exists to prevent | §12 |
| F1 | new **H_OS-E5**: svd's own resources partitioned per principal — per-principal listeners with round-robin accept and `budget.connections/inflight/refusals`; fd headroom and `ErrResourceBudget`; bounded per-source event queues with round-robin dequeue and O5; credential-only notify control buffer; per-principal pids/memory (Linux cgroup + RLIMIT_NPROC, FreeBSD rctl, macOS rlimits, Windows nested job), `ErrIsolationUnavailable` with no opt-out. Unverified mechanisms are listed above. Chaos test | §1.7, §1.10, §3.1, §3.3, §8.1, §10 |
| F2 | runs_as = resolved uid/SID and exact group set; load-time check and pre-fork re-resolve (`ErrAccountChanged`); helper verifies ids after setuid; `ErrDelegationPrivilege` for capabilities, nnp, groups, uid 0. A1 restated as probes, A2 as disjointness predicates (a file owned by p counts as p-writable, which covers setuid-p binaries; forcing NNP on outside services was rejected for that reason) | §1.6, §1.10, §3.3, §10 |
| F3 | the recommended fix: `cut(A′, A)` truncates propagation at every grant boundary for **every** holder, owner included; D(X), `--with-dependents` and T computed on uncut edges; `WarnUnheldOutsideTarget`; the effect-set check is a real refusal, never a panic (line 652 text replaced); Theorem G proof step (b) corrected | §2.3, §2.4, §3.3, §3.7 I4 |
| F4 | three principal classes (owner-class, delegated, reader); `ErrPrincipalClass`; the reader direction of `ErrDelegationUid` | §1.6, §8.2 |
| F5 | P2 split: P2a (abstract time, Gate 1, with Leaked and finite operator restarts) and P2b (`ErrNeverFailingLoop`, window > max × U(A)) | §3.7, §3.8 |
| F6 | `Absent` Reach value and `Prio` per guard cell; 15 present, 73 Absent; Failed × G_ok carries `ReasonRecoverable ∧ GoalUp`; termination bound corrected from 3 to 2 moves (draft 8's 3-move path did not exist); Gate 2 mutates priorities | §3.5, §3.8 |
| F7 | event-table headers renamed StartTO and StopTO, matching the prose | §3.4 |
| F8 | fixed in the cells, not only documented: Stopping, Killing and Leaked × X1 record `perm` from the status pipe | §3.2, §3.4 |
| F9 | the API refusal check and the cell's `pay` are one payment | §3.2, §3.4 |
| F10 | cgroup hand-over to a principal withdrawn (H_OS-E2) | §3.3, §8.2 |
| F11 | notify `recvmsg` control buffer sized for credentials only; `MSG_CTRUNC` drops the message | §3.3, §10 |
| Gate 1 | Theorem G item 1 checked as 2-safety by self-composition (product BFS, low inputs to both copies, high to one, oracle as a low input, N ≤ 3 with symmetry); red on `goalDraft8`, green on draft 9, red on the want-closure and `pay(caller)` mutants | §3.8 |
| G1 (new, found by the 2-safety re-proof) | an owner Retry of A ∈ grant(p) paid R_inst, or not, according to whether A was Failed, a state p controls: p could thereby move R_inst. The Retry cell now pays payer(A); an owner payment to R_p is never refused and never escalates | §3.3, §3.4, §3.7 |

**Rules `Compile` must newly enforce in draft 9 [Compile+9]:** `ErrPrincipalClass`; the reader direction of `ErrDelegationUid`; `ErrDelegationPrivilege` for `capabilities`, `nnp` and `groups`; `ErrNeverFailingLoop` (P2b); the E5 budget fields in Q, whose absence is `ErrIncomplete`; the warning `WarnUnheldOutsideTarget`. **New host checks [Host+9]:** the resolved-id check at load and `ErrDelegationPrivilege{…, uid0}`; `ErrAccountChanged` before each fork; `ErrResourceBudget`; `ErrIsolationUnavailable`. The cut in goal propagation, the Retry payer and the cell changes are runtime and FSM data, not `Compile`. go-supervise/supervisor implements draft 8's `Compile` and is not changed by this draft.

**REVIEW-1 contested points.** None of the findings is contested. Two are resolved differently from the review's suggestion:

- **C1.** The review proposed making `api.*` and `exempt_on` required-concrete. We prove (Theorem N) that no orientation can make absence fail closed, and use a mandatory **total** baseline plus completeness. This covers every hardening field, not only those two.
- **M5.** Rollback on Failed(wait) is kept as intended semantics. The fix is automatic recovery from Failed(wait) and Failed(dep), plus `ErrWaitTooShort`.

---

## 13. Decision D2: what delegation may cost the instance — DECIDED, option A

**Decision (user, 2026-11-23): option A.** A delegated service pays its own crashes, and a grant must include every service that has an edge of any kind to a granted one (`ErrGrantNotClosed`, §8.2; extended from require to every kind by the user's REVIEW-4 E2 ruling). A service runs as a delegated principal exactly when it is in that principal's grant (E1). Theorem G is stated and proved in §3.3, conditional on the assumed part of H_OS (E6). The text of options A–C below is kept as weighed in draft 6; where it says "reverse require", draft 8 reads "reverse of every edge kind". Draft 9 further reads it with the grant-boundary cut on goal propagation for every holder (F3, §2.3) and with svd's own resources partitioned per principal (H_OS-E5, §3.3); where option A says "Retry pays the caller", draft 9 reads "Retry pays payer(A)". Options B and C are kept below as the record of what was weighed.

**The problem (REVIEW-3 D2).** Draft 5 charged a crash to R_inst whenever the incarnation was owner-held. A delegated principal p can re-enable an owner-held service by Retry, and since §1.6 makes p's services run as p, it can also crash them from outside the API. Both routes reach Quiescing(1), and the second also reaches a terminal service through a require edge. The FSM tables did not depend on the choice: it changes only payer(A) (§3.3) and the grant conditions (§8.2).

**A common hypothesis, H_OS.** p holds OS privilege only over processes running as p: its own uid, plus the cgroup of its own subtree (§8.2, `ErrDelegationUid`). Pressure on shared resources outside the class-H limits, such as host memory or the pid space, is out of scope. Every option states its theorem under H_OS. Draft 8 splits H_OS into an enforced and an assumed part, and names the excluded channels (§3.3, E6).

### Option A: delegated services pay their own crashes, and grants are closed under reverse require (CHOSEN)

- **Rule.**
  - payer(A) = p when `run_as(A)` is a delegated principal p, which §1.6 makes true of every service in grant(p). Otherwise payer(A) = the owner. Holders no longer decide who pays.
  - Retry pays the caller, as now.
  - **Grant closure.** If A ∈ grant(p) and B requires A, then B ∈ grant(p). Otherwise Compile gives `ErrGrantNotClosed{A, B}`. Terminal services are never in a grant, so no terminal service requires a delegated one, directly or transitively.
- **Restored theorem.** Under H_OS, p cannot cause Quiescing(1), and cannot change any service outside grant(p) except through want and order edges, which never send their source to Failed (§3.5).
  *Proof sketch.*
  - R_inst is paid only by crashes of services not run as a delegated principal, and by owner Retries.
  - p can crash such services neither through the API (effect-set check) nor through the OS (H_OS).
  - Failed(dep) travels only along require edges, and by closure no require edge leaves grant(p) towards a dependent outside it. So no terminal service can reach Failed because of p.
  - R_p overflow is local (§3.3). ∎
- **What p loses.**
  - Nothing that p's own services can do among themselves.
  - The owner loses the ability to make an owner service **require** a delegated one. (Draft 6 still allowed want and order edges into a grant; since draft 8, E2, no edge of any kind may point into a grant.)
  - A boot-enabled delegated service that flaps fails locally, as Failed(intensity). It never escalates the instance, even when the owner would have wanted it to; to escalate on it, the owner must keep it out of every grant.

### Option B: delegation is a nested instance

- **Rule.**
  - grant(p) is no longer a subset of the outer plan. p receives **its own instance** in mode U, running as p (§5.3), whose site layers p writes. The outer instance runs that nested supervisor as one service S_p, `run_as = p`.
  - S_p's crashes pay a per-principal budget, never R_inst. No outer service may require S_p (`ErrGrantNotClosed`); want and order are allowed.
  - p has no access to the outer API at all.
- **Restored theorem.** Under H_OS, p cannot change the outer instance's state except S_p's, and cannot cause the outer Quiescing(1).
  *Proof sketch.* p's only outer surface is S_p's process tree. Its crashes are charged outside R_inst, and no require edge leaves S_p. ∎ The proof is the simplest of the three, because the boundary is the OS, not a rule in the table.
- **What p loses.**
  - Its services can have no edge to or from outer services except want and order on S_p as a whole.
  - The configuration splits into two instances, each with its own baseline, schema and API endpoint, and there is one more supervisor process per principal.
  - The owner cannot `start` or `stop` one of p's services from the outer API; it goes through p's instance or stops S_p.

### Option C: accept that delegation is trusted up to escalation

- **Rule.** Keep draft 5's owner-held charging. Report every Quiescing(1) with the principals whose operations or services contributed to the overflow.
- **Restated theorem (weaker).** p cannot change any service outside grant(p) **through the API**, except by causing Quiescing(1), which is attributed and reported. There is no guarantee against escalation.
- **What p loses.** Nothing. The **owner** loses availability isolation: a delegated principal can stop the whole instance.

### Recommendation (draft 6), adopted by the user

**Option A.** It keeps one instance and one configuration, it restores the full statement of Theorem G under an explicit OS hypothesis, and its only cost, no require edge from an outsider into a grant, is the condition that makes the proof work. Option B is the stronger boundary and the right choice if delegation to untrusted principals becomes common; it can be added later without changing the FSM. Option C is not recommended: it names the hole instead of closing it.

The two REVIEW-3 counter-examples are named regression tests (§10), with option A's outcomes: R_inst untouched, and `ErrGrantNotClosed` at Compile.
