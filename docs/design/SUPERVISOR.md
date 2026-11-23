# go-supervise/supervisor — a cross-platform service supervisor with stated semantics

Status: **DRAFT 7, 2026-11-23. No code.** Draft 7 records the user's decisions on D2 (option A), D-M2 and the binary name (`svd`). This draft resolves every finding of
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
     - if I is a singleton, use it;
     - else if Π_s ≠ ∅: if Π_s ∩ I has exactly one element, use it; if Π_s has a point outside I → `ErrConflict` (a site preference may not widen); if Π_s ∩ I has several elements → `ErrConflict` (two sites disagree);
     - else if Π_r ∩ I has exactly one element, use it;
     - else if Π_r ∩ I is empty → `ErrIncomplete` (the met interval excludes every recipe preference, and no site chose a point);
     - else → `ErrConflict` (several recipe preferences remain);
  3. **post-meet predicates** (§1.5 completeness, §1.6). They read resolved values, including `ErrWaitTooShort` and the StopWait bound SW.

  Consequence: a site writing `"60s"` against a recipe's `{5s, 120s, prefer 30s}` gets 60 s. The met interval is the singleton [60s, 60s], so the prefer set is not consulted.
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

  A want- or order-target contributes its one-attempt time to W, but it never sends its wanter to Failed (§3.5). The predicate is on require-targets only, because only they arm WaitTO:

  `wait.timeout(A) < max over B ∈ req(A) of U(B)` is `ErrWaitTooShort`, computed on resolved values (§1.4 phase order).

  The REVIEW-3 counter-example (A require B, B want C, C mid-restart) is covered: C's attempt is inside W(B), hence inside U(B). The budget covers **one** failed attempt of each upstream service, through every edge kind. More flapping sends A to Failed(wait), which recovers automatically (§3.5). That is the stated behaviour of a hard dependency.
- **Typed parameters** (N7). A `param` must declare a typed domain, one of:
  - integer interval;
  - enumeration;
  - port;
  - duration interval;
  - path region (§1.3 K5).

  The domain is in the completeness set Q. A `param` without a domain, or with a free-form string domain, is `ErrUntypedParam`. A site value outside the domain is `ErrConflict`. Values are substituted into argv as single arguments, never through a shell.

  **svd's "no shell" does not bind the recipe (R3-L4).** A recipe could run `sh -c "… $p"` itself. Two rules close that:
  - **Path values have a restricted alphabet:** `[A-Za-z0-9._/+@,=-]`, with no leading `-`. A path with a space, `;`, a quote, `$`, a backquote, a newline or NUL is `ErrConflict` against the domain.
  - **No parameter inside a shell command string.** When `exec[0]` resolves to a known shell (`sh`, `bash`, `dash`, `zsh`, `cmd.exe`, `powershell`, `pwsh`) and argv holds `-c` or `/c`, a parameter substituted into the command-string argument is `ErrParamInShell`. Parameters passed as positional arguments after the command string (`sh -c '… "$1"' sh <param>`) remain allowed.
- **Mode U run_as.** In mode U, `run_as` ≠ the instance owner gives `ErrRunAsInUserMode`.
- **Delegated subtree** (L2). Every service in a subtree delegated to principal p must run as p (`ErrDelegationUid`).

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
| **G** instance | `api.admin`, `api.read` (K5); instance intensity; per-principal budgets (§3.3); `enabled`; `terminal`; `schema`; `all {}` (contents restricted, §1.5) | **forbidden** (`ErrFieldAuthority`) | writes |
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
- `ErrDelegationUid`;
- `ErrForkingDaemon`;
- `ErrUntypedParam` (N7);
- `ErrParamInShell` (R3-L4);
- `ErrSchemaTooNew{baseline, running}` (D5);
- `ErrGrantNotClosed{principal, granted, requirer}` (D2, §8.2).

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
- goal(A) = up ⟺ x(A) ≠ ∅ ∨ ∃A′: goal(A′) = up ∧ edge(A′, A) ∈ {require, want}. This is the least fixpoint over the DAG.
- Goal depends only on holders and edges, never on states. It is therefore **fixed during a step's propagation**.
- **`stop X` (N12).** Let D(X) be the set of services that keep goal(X) = up through a require or want edge, given current holders.
  - If D(X) is non-empty, `stop X` is refused with `ErrStillWanted{by: D(X)}`. Clearing x(X) alone would leave X running, which is what REVIEW-2 found.
  - **Order of checks (R3-L7).** For a non-owner caller the grant check of §8.2 runs first, so a caller outside grant gets `ErrOutsideGrant` and learns nothing about D(X). Then `by` is filtered to D(X) ∩ grant(p), plus a count of the others when the caller lacks `api.read`.
  - `stop X --with-dependents` clears the holders of X and of every service in the reverse require/want closure that keeps X up. This is systemd's behaviour.
  - Either way, every open transaction that held a cleared holder is aborted. Those transactions report `superseded` and do not roll back.
  - Both forms are subject to the grant check of §8.2.

### 2.4 Transactions and rollback (default on)

- `start X` creates request h, adds h to x(X), and opens a transaction T = {services whose goal goes down→up because of h}.
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

1. **One external event** is taken from the queue. Internal events (SEt, SEp) take priority.
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
    - The Waiting sojourn has one wait deadline. It is armed, once, at the first moment in the sojourn when some require-target is not in {Ready, Done}: at entry if one is pending then, otherwise at the propagation step where one first becomes pending. It is never disarmed or re-armed within the sojourn, so a toggling require-target does not restart the budget.
    - When WTO fires, Waiting × WTO sets the sojourn flag `wexp`, then evaluates **pending** ⟺ ∃B ∈ req(A): state(B) ∉ {Ready, Done}. If pending holds, → Failed(wait); otherwise the event changes nothing else. A deadline queued before the last require-target became Ready therefore never fails a service whose require-targets are all Ready.
    - While `wexp` holds, the Waiting guard G_wexp ⟺ wexp ∧ pending sends A to Failed(wait) (§3.5): the sojourn's budget is spent, so a require-target that becomes pending again after the deadline fails A at once instead of waiting without bound.
    - `wexp` is reset on every state entry, with the generation.
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
- **O2, exit before empty.** `TreeEmpty(e)` is enqueued only after `MainExit(e)`.
  - Tree emptiness is observed through cgroup.events `populated 0`, `JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO`, or reaper status *(Windows and FreeBSD unverified)*.
  - The source holds TreeEmpty back until the main process has been reaped.
- **O3, synchronous spawn failure, before any child exists (D7).** SEt and SEp cover **only** failures that svd itself detects before a child process exists: the fork, `clone3` or `CreateProcess*` call fails, or a check that svd runs before forking fails. No tree exists in that case, so I2 holds. They are classified (N11) as one of:
  - **SEt**, transient: `EAGAIN`, `ENOMEM`, `EMFILE`, `ENFILE`;
  - **SEp**, permanent: the exec-helper binary is missing or not executable; an unknown account, which svd resolves before forking.

  An unclassified error counts as transient.
- **O4, failures after the fork go through the tree (D7).** Everything the exec-helper detects happens in a forked child inside the epoch's tree: the region conflict of §7.2, `ErrProtectionUnavailable`, `ENOENT`, `EACCES` or `ENOEXEC` from the final `execve`, and setup errors. The helper reports them as follows.
  - It writes a classified status on a **CLOEXEC status pipe** and exits with status 127. A successful `execve` closes the pipe, so the parent reads EOF.
  - The source reads the pipe to EOF before it enqueues MainExit, as in O1. It delivers `MainExit(fail)` with the sub-status `perm` (permanent) or `fail` (transient or unclassified).
  - Starting × X1 records that sub-status (§3.2 `record`). The tree then empties through StopWait/Stopping as usual, and **settle** sends a `perm` exit to Failed(spawn) (§3.2 rule 2). Failed(spawn) is therefore reached only at TreeEmpty, so I2 holds and the helper's exit is never seen in Failed.
- **Transient spawn errors are not charged (R3-L9).** `EAGAIN`, `ENOMEM`, `EMFILE` and `ENFILE` before the fork usually measure the **supervisor's** exhaustion, not the service's. Charging them to R_A and R_inst would let supervisor-side pressure escalate the instance. Starting × SEt goes to Backoff **uncharged**, with the delay backoff.max, and is reported. Retries are therefore unbounded in number but bounded in rate, at one per backoff.max per service. A post-fork transient failure (O4, sub-status `fail`) is the service's own and is charged as usual.

### 3.2 Variables (M7)

Per service:

- `state` ∈ S = {Inactive, Waiting, Starting, Ready, Done, StopWait, Stopping, Killing, Backoff, Failed, Leaked} — 11 states;
- `goal` (derived);
- `epoch`;
- `gen`;
- `cause` ∈ {none, dep, restart, fail, exit} — why the current stop happens;
- `exit` ∈ {none, ok, fail, perm, sig} — the recorded status of the main process (D1, D7):
  - `ok` and `fail`: a **natural** exit, recorded while no stop signal had been sent;
  - `perm`: a permanent post-fork spawn failure reported on the status pipe (O4);
  - `sig`: any exit recorded after `term` or `kill` was sent, whatever its status;
- `wexp` — the wait deadline of the current Waiting sojourn has passed (D3);
- `reason` ∈ {exited, spawn, intensity, wait, dep, unkillable} — meaningful in Failed only, reported;
- `R_A` — timestamps of charged restarts, read through the window (N1);
- `x(A)` — holders. Each holder records its principal.

Per instance:

- `R_inst`;
- `R_p` per API principal p ≠ owner (N2);
- `mode` ∈ {Running, Quiescing(status)}.

The schema fields used by the table:

- `kind` ∈ {daemon, oneshot};
- `restart.on` ∈ {never, on-failure, always} (K1). The table reads it through `permits(x)`:

| `restart.on` | permits(ok) | permits(fail) |
|---|---|---|
| never | false | false |
| on-failure | false | true |
| always | true | true |

**Helpers.** Each entry into a state increments `gen` and arms that state's single timer, if it has one.

- `spawn`: epoch++, exit := none, cause := none, create the tree, spawn main, open the channel → Starting.
- `stopwait(c)`: cause := c if cause = none → StopWait (arms StopWaitTO, §3.6).
- `term`: send `stop.signal` to the whole tree → Stopping (arms `stop.timeout`).
- `kill`: SIGKILL the tree (`TerminateJobObject` on Windows) → Killing (arms `kill.timeout`).
- `record(x)`: if exit = none then exit := x; and if cause = none **and x ∈ {ok, fail, perm}** then cause := exit. A `sig` exit never sets the cause.
  - In Starting, Ready and StopWait, no stop signal has been sent, so the exit is natural: X0 records `ok`, X1 records `fail`, or `perm` when the status pipe says so (O4).
  - In Stopping, Killing and Leaked, the exit is the expected result of `term`/`kill`. X0 and X1 both record **`sig`**, whatever the status. A `sig` exit is never charged (rule 4 below) and **never counts as success** (rule 3).
- `bump(A)`: append now to R_A.
- `pay(q)`: append now to the counter of payer q, where q is the owner (R_inst) or a delegated principal p (R_p). Who pays for what is fixed in §3.3 (D2, option A).
- `charge(A)` (D4): bump(A), then pay(payer(A)), with payer(A) from §3.3. R_A and the payer's counter are separate appends, so no helper appends to R_inst unconditionally.
- `restart(A)`: charge(A); then
  - if |R_A(now)| > intensity.max → Failed(intensity);
  - else → Backoff, arming delay = min(initial · factor^(|R_A(now)|−1), max).
- `settle(A)`, applied on TreeEmpty. Rules are evaluated in order; the first that matches wins (N11, D1, D7):
  1. goal = down → Inactive;
  2. exit = perm → Failed(spawn), uncharged (O4);
  3. kind = oneshot ∧ exit = ok → Done, whatever the cause. Here `ok` is a **natural** success: the oneshot exited with status 0 before any stop signal was sent. A completed oneshot, such as a migration that finished during StopWait(dep), is never re-run by a dependency loss;
     - a oneshot that **was stopped** by StartTO, by a dependency loss, or by a restart has exit = `sig` (it died of our signal) and never matches this rule. It falls through to rule 4 or 5 and is run again from Waiting or Backoff, never Done (D1);
  4. cause ∈ {dep, restart, none}:
     - if exit = fail, a natural crash recorded before `term`, then charge(A), and if |R_A(now)| > max → Failed(intensity), else → Waiting;
     - otherwise (exit ∈ {none, ok, sig}) → Waiting, uncharged.

     A daemon that crashes on its own during StopWait(dep) is therefore charged;
  5. cause = fail → restart(A);
  6. cause = exit ∧ permits(exit) → restart(A);
  7. otherwise → Failed(exited).

  **Why rule 3 cannot see a killed oneshot (D1).** `sig` is recorded in exactly the states where a signal has been sent, and rule 3 reads only `ok`. The REVIEW-3 sequence (Starting × StartTO → StopWait → term → SIGTERM death) records `sig`, its cause is `fail`, and settle reaches rule 5: restart(A), charged, never Done. Its require-dependents therefore never see Done for a half-run migration.

  **Rule D1-n (natural completion during StopWait, confirmed 2026-11-23).** A oneshot that exits with status 0 while in StopWait, before `term` is sent, records `ok` and settles to **Done**, whatever the cause of the stop. *Justification:* no signal from svd reached it, so its status 0 is its own report that the work completed, and re-running a completed migration is the error N11 removed.

### 3.3 Restart intensity and escalation (OTP model)

- **Charging.** `restart(A)` charges, and so does settle rule 4 for a spontaneous exit(fail). Both are reached only by a service's own failure: cause ∈ {fail, exit}, a post-fork transient spawn failure (O4), or a crash during StopWait. A pre-fork SEt is not charged (R3-L9), and a `sig` exit is never charged.
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
  - **payer(A)** = p when `run_as(A)` is a delegated principal p, which §1.6 makes true of every service in grant(p); otherwise the owner. charge(A) bumps R_A once and pays payer(A) once.
  - **Grant closure** (§8.2): every service that requires a granted service is itself in the grant, or Compile fails with `ErrGrantNotClosed`.
  - **Retry** (the Failed × Ry cell) is `R_A := ∅; pay(caller)`. It does not call charge(A), so after a Retry |R_A| = 0, as "Retry clears R_A" says.
  - **Two kinds of charge to R_p, and two different overflow rules:**
    - an **API charge** (Retry, and every other write that pays) that would overflow R_p is **refused** with `ErrPrincipalBudget`, before it takes effect;
    - a **crash charge**, which pays R_p because a crash of a delegated incarnation happened, cannot be refused. When it takes |R_p(now)| past budget_p, the crashed service goes to **Failed(intensity)**, as a service overflow would. The overflow stays local: mode is unchanged, and every further write from p is refused with `ErrPrincipalBudget` until the window drains.
  - **R_p never escalates**, under either rule.
  - **Effect-set check.** A non-owner's `start`, `stop`, `restart` or `retry` is refused with `ErrOutsideGrant{services}` when the set of services whose goal or state the operation would change is not inside grant(p). That set includes:
    - the G_lost reverse require closure, for `restart`;
    - the reverse closure, for `stop --with-dependents`.
  - A loop of retries, or of `stop`/`start` (REVIEW-2 shows that denying Retry alone does not suffice), is therefore bounded in number by R_p, and the crashes each retry re-enables pay R_p too, since the service runs as p.

  **Hypothesis H_OS.** p holds OS privilege only over processes running as p: its own uid, plus the cgroup of its own subtree (§8.2, `ErrDelegationUid`). Pressure on shared host resources outside the class-H limits (host memory, the pid space) is out of scope.

  **Theorem G (delegation cannot escalate).** Under H_OS, a principal p ≠ owner cannot cause Quiescing(1), and cannot change the state of any service outside grant(p) except through want and order edges, which never send their source to Failed (§3.5).

  *Proof.*
  - Quiescing(1) needs an R_inst overflow, or a terminal service reaching Failed.
  - R_inst is paid only by crashes of services whose `run_as` is not a delegated principal, and by owner Retries. p cannot create owner holders or owner Retries: owner operations come from the owner's peer credentials.
  - p cannot make such a service crash through the API, because the effect-set check refuses every operation that changes a service outside grant(p). It cannot make it crash through the OS, by H_OS.
  - Failed(dep) travels only along require edges. By grant closure, no service outside grant(p) requires a service inside it, so no failure inside grant(p) reaches a service outside it. Terminal services are never in a grant, hence never reached.
  - An overflow of R_p fails the crashed service locally and never changes `mode` (§3.3, I5). ∎

  **Draft 5's proof was withdrawn in draft 6.** REVIEW-3 D2 gave two counter-examples, and option A answers each:
  - **Through the API.** A ∈ grant(p) is boot-enabled, hence owner-held, and crashes on its own. Each `retry A` by p costs R_p one charge, but re-enables A, whose next crashes are charged to R_inst because the incarnation is owner-held. Two retries take R_inst past max_inst → Quiescing(1).
  - **Outside the API.** §1.6 makes every delegated service run as p, and §8.2 may hand p its cgroup. p can therefore crash an owner-held A ∈ grant(p) with `kill -SEGV`. If a terminal T ∉ grant(p) requires A, then A → Failed(intensity) makes T Failed(dep), which escalates without any Retry.

  Under option A, the first counter-example's crashes pay R_p, because A runs as p; R_inst is untouched. In the second, T ∉ grant(p) requiring A ∈ grant(p) is `ErrGrantNotClosed` at Compile.
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

Columns: Ry = Retry; Rs = Restart (operator); SEt, SEp = SpawnErr transient or permanent (N11); X0 = MainExit(ok); X1 = MainExit(fail); TE = TreeEmpty; Ri = Rise; Fa = Fall; WTO, STO, SWTO, StTO, KTO, BTO = state timers.

There is no Expire column: the windows are read, not evented (N1).

| state | Ry | Rs | SEt | SEp | X0 | X1 | TE | Ri | Fa | WTO | STO | SWTO | StTO | KTO | BTO |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| **Inactive** | · | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Waiting** | · | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | wexp := ✓; if pending →Failed(wait), else · (D3) | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Starting** | · | stopwait(restart) | →Backoff, uncharged, delay backoff.max (R3-L9) | →Failed(spawn) | record(ok); stopwait | record(fail or perm, from the status pipe); stopwait | ⊘ (O2) | →Ready, arm liveness | ⊘ (liveness not armed) | ⊘ | stopwait(fail) | ⊘ | ⊘ | ⊘ | ⊘ |
| **Ready** | · | stopwait(restart) | ⊘ | ⊘ | record(ok); stopwait | record(fail); stopwait | ⊘ (O2) | · (latched) | stopwait(fail) | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Done** | · | →Waiting (re-run; cause = restart) | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **StopWait** | · | · | ⊘ | ⊘ | record(ok) | record(fail) | settle | · | ⊘ | ⊘ | ⊘ | term (report T2 overrun) | ⊘ | ⊘ | ⊘ |
| **Stopping** | · | · | ⊘ | ⊘ | record(sig) | record(sig) | settle | · | ⊘ | ⊘ | ⊘ | ⊘ | kill | ⊘ | ⊘ |
| **Killing** | · | · | ⊘ | ⊘ | record(sig) | record(sig) | settle | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | →Leaked (report) | ⊘ |
| **Backoff** | · | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | →Waiting |
| **Failed** | goal = up: R_A := ∅; pay(caller) (§3.3, D4); →Waiting; else · | as Ry | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |
| **Leaked** | · (API: `ErrLeaked`) | · (API: `ErrLeaked`) | ⊘ | ⊘ | record(sig) | record(sig) | →Failed(unkillable) | · | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ | ⊘ |

Notes on the cells:

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
- **G_free(B)** ⟺ both of:
  - no A with B ∈ req(A) is in {Starting, Ready, StopWait, Stopping, Killing, Leaked};
  - no A with B ∈ wnt(A) ∪ ord(A) is in {StopWait, Stopping, Killing, Leaked}.

  **Leaked blocks (N3)**, so a leaked dependent is bounded by SWTO and reported. It is never silently passed over.
  - Require-dependents that are still running block B. They are about to be pushed down by G_lost, so the result does not depend on evaluation order.

Guards are tried in the order given in each row. The first enabled guard fires.

| state | guards, in priority order |
|---|---|
| Inactive | G_up → Waiting |
| Waiting | G_down → Inactive; G_failed → Failed(dep); G_wexp → Failed(wait) (D3); G_ok → spawn. The sojourn's single wait deadline is armed at the first moment a require-target is pending, and never re-armed (§3.1, D3) |
| Starting | G_down → stopwait(none); G_lost → stopwait(dep) |
| Ready | G_down → stopwait(none); G_lost → stopwait(dep) |
| Done | G_down → Inactive |
| StopWait | G_free → term |
| Stopping, Killing, Leaked | none |
| Backoff | G_down → Inactive |
| Failed | G_down → Inactive; reason ∈ {dep, wait} ∧ G_up ∧ G_ok → Waiting, uncharged (**automatic recovery**, M5) |

Grid: 11 states × 7 guards (G_up, G_down, G_ok, G_failed, G_lost, G_free, G_wexp) = 77 cells.

**Termination of propagation.**

This argument replaces draft 4's chain, which was wrong (N14). It is an induction over the DAG.

- Goals are fixed during propagation (§2.3).
- **Per-service graph with fixed inputs.** Fix the states of A's targets, and fix whether G_free holds. A's guard moves then form an acyclic graph:
  - with goal up: Inactive → Waiting → {Starting | Failed}; Starting/Ready → StopWait → Stopping; Failed → Waiting;
  - with goal down: Waiting, Done, Backoff, Failed → Inactive; Starting/Ready → StopWait → Stopping.
  - The only possible cycle, Waiting → Failed → Waiting, needs G_failed or G_wexp, and then G_ok. With fixed targets G_ok excludes both, since each implies that a require-target is not Ready or Done.
  - So with fixed inputs A moves at most 3 times.
- **Inputs change finitely often.**
  - All guards except G_free read only A's targets. G_free reads its dependents, but the move it enables, StopWait → Stopping, is one-way and happens at most once per step.
  - By induction in topological order, from sinks up: a sink's inputs are fixed, so it stabilises after ≤ 3 moves. If every target of A changes state finitely often, A's inputs change finitely often, and A moves at most 3 times per input change.
  - So propagation terminates.
- **No closed-form bound is claimed (R3-L8).** The induction gives "at most 3 moves per input change", and input changes compound along the depth of the DAG, so it does not yield a linear bound such as 4N. Gate 1 records the **measured** maximum number of moves per step over its whole scope, writes it into a golden file, and asserts it. A change that raises it must update the golden file in review.

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
- **I4:** the wanted set is closed under require and want.
- **I5** (M8, N1): state(A) ≠ Failed ⇒ |R_A(now)| ≤ intensity.max.
  - Failed may hold max + 1 until the window drains or Retry clears it.
  - mode = Running ⇒ |R_inst(now)| ≤ max_inst.
  - **R_p (D4).** API-originated payments to R_p are refused when they would take |R_p(now)| past budget_p. Crash payments cannot be refused: one that passes the budget sends the crashed service to Failed(intensity), refuses p's writes until the window drains, and never changes `mode`. So the invariant is not |R_p| ≤ budget_p but: **an overflow of R_p never changes `mode`.**
- **I6, on trees:** at most one populated tree per service. Spawn happens only from Waiting, where I2 says the tree is empty, and Leaked blocks Retry.
- **T1:** spawn only when G_ok holds.
- **T2:** B is signalled (term) only when G_free(B) holds, or when StopWaitTO fires and the overrun is reported. Escalation and rollback stop services through this path (H1).

**Progress, under weak fairness of timers and reports:**

- **P1:** goal = down ⇒ ◇ state ∈ {Inactive, Failed, Leaked}.
- **P2:** goal = up ∧ every requirement eventually stays in {Ready, Done} ⇒ ◇ state ∈ {Ready, Done, Failed}.
- **P3:** Theorem R. Owner Retry pays R_inst, and delegated Retry pays R_p (N2, D4). Crashes pay payer(A): R_p for a service run as p, R_inst otherwise (D2, option A).

### 3.8 Verification (M9)

**The table is data.**

```go
type Cell struct {
	Target  State     // or Same
	Actions ActionSet // bitset: Spawn, Term, Kill, Record, Charge, ArmLiveness, Report, …
	Cond    CondID    // e.g. GoalUp, ReasonRecoverable
	Reach   Reach     // Reachable or Unreachable(ByEpoch|ByGen|O1|O2|O3)
}
```

- The event table, the guard table, settle (as a decision list) and `permits` are all data of this form.
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
    - two principals, owner and delegated, with grants (N2);
    - Retry and Restart at every state.
  - Checks:
    - I1–I6, T1 and T2;
    - **D1:** a oneshot whose exit is `sig` never reaches Done;
    - propagation termination, with the measured maximum number of moves recorded in a golden file and asserted (no closed-form bound, R3-L8), and confluence;
    - SCC search for P1–P3 under fairness.
- **Gate 2, mutation.**
  - Every Reachable cell is mutated: every other Target, and every one-action change. Each mutation must turn Gate 1 red.
  - An equivalent mutation, with the same Target and Actions, is not a mutation.
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
  - **Grant closure (D2, option A).** If A ∈ grant(p) and B requires A, directly, then B ∈ grant(p); by induction the grant is closed under the reverse require closure. A violation is the Compile error **`ErrGrantNotClosed{principal, granted, requirer}`**. Since terminal services are never in a grant, no terminal service requires a delegated one. Want and order edges into a grant are allowed: they never fail their source (§3.5).
- On Linux, that subtree's cgroup may be handed to the principal's own instance, but only if every service in it runs as the principal (`ErrDelegationUid`, L2). Otherwise the principal would gain `cgroup.kill` and `cgroup.freeze` over other uids.

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
- `prefer` resolution (D-M2): a site prefer inside the met interval wins over the recipe's; a site prefer outside it is `ErrConflict`; no resolved value ever leaves the met interval.
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
- endpoint squatting on every OS (H5).

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
- D2: option A, delegated services pay their own crashes and grants are closed under reverse require (§13).

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
- the recipe signature format.

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

**REVIEW-1 contested points.** None of the findings is contested. Two are resolved differently from the review's suggestion:

- **C1.** The review proposed making `api.*` and `exempt_on` required-concrete. We prove (Theorem N) that no orientation can make absence fail closed, and use a mandatory **total** baseline plus completeness. This covers every hardening field, not only those two.
- **M5.** Rollback on Failed(wait) is kept as intended semantics. The fix is automatic recovery from Failed(wait) and Failed(dep), plus `ErrWaitTooShort`.

---

## 13. Decision D2: what delegation may cost the instance — DECIDED, option A

**Decision (user, 2026-11-23): option A.** A delegated service pays its own crashes, and a grant must include every service that requires a granted one (`ErrGrantNotClosed`, §8.2). Theorem G is stated and proved unconditionally under H_OS in §3.3. Options B and C are kept below as the record of what was weighed.

**The problem (REVIEW-3 D2).** Draft 5 charged a crash to R_inst whenever the incarnation was owner-held. A delegated principal p can re-enable an owner-held service by Retry, and since §1.6 makes p's services run as p, it can also crash them from outside the API. Both routes reach Quiescing(1), and the second also reaches a terminal service through a require edge. The FSM tables did not depend on the choice: it changes only payer(A) (§3.3) and the grant conditions (§8.2).

**A common hypothesis, H_OS.** p holds OS privilege only over processes running as p: its own uid, plus the cgroup of its own subtree (§8.2, `ErrDelegationUid`). Pressure on shared resources outside the class-H limits, such as host memory or the pid space, is out of scope. Every option states its theorem under H_OS.

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
  - The owner loses the ability to make an owner service **require** a delegated one: only want or order edges may point into a grant.
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
