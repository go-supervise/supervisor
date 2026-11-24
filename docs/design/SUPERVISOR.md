# go-supervise/supervisor — a cross-platform service supervisor with stated semantics

Status: **DRAFT 16, 2026-11-24. `Compile` of draft 8 is implemented in go-supervise/supervisor; the pure FSM (PR #5, `fsm/`) is under review. This draft changes no cell of the event table, the guard grid or settle; it fixes the evaluation order of propagation and the readings of §2.3, §2.4 and §3.8 that PR #5 found ambiguous. §7 is unchanged.**

**Changelog, draft 16** (review of go-supervise/supervisor PR #5; edits confined to §2.3, §2.4, §3.4 notes, §3.5 and §3.8 Gate 1): §3.5's "confluence across all evaluation orders" is **false** in both directions and is withdrawn: an order that puts a dependent before its target lets the dependent read the target's transient state, and an order that puts a target before its dependent lets G_free fire before the dependent has left Ready, so a `want`/`order` target is signalled in the same step as its dependent (PR #5's fixed targets-first order does exactly that: owner `stop` of {A, B} with A `order`/`want` B, both Ready, sends term to B and A in one step). Propagation is now **stratified** (phase 1: every guard but G_free, targets first, to a fixpoint; phase 2: G_free once) and **rounds** end at a rollback or an escalation, so goals are fixed within a round; the claim becomes "every order respecting the strata gives the same world", proved and checked (§3.5); open transactions are per holder h(q, X), a repeated `start` is not an error, and `stop` aborts with `superseded` (§2.3, §2.4); a member in Leaked fails its transaction (§2.4); ⊘ cells, the `perm` column and Done × Restart's cause are stated (§3.4 notes); Gate 1's state is made finite by a stated abstraction, a bounded search is named a smoke test and is not Gate 1, and the plan scope "every labelled DAG" is kept (§3.8).

**Changelog, draft 15** (resolves `SECURITY-1.md` Part 7; map and user decisions in §12; edits confined to §7.1.1, §7.1.2, §10 and §12): `svd-drop`'s shape, build and two gates with red controls (T13, §7.1.1 item 4); D5b fresh session keyring (T12); H1 `CLOSE_RANGE_CLOEXEC` (T6); D10 without the oom read (T7); D9a `capset` to K and D10's verification of K (T15); every rlimit in the frame, D5a scheduling reset, cwd check in D11 (T16); versioned install directory, frame magic and digest, helper fd pinned in `Files` (T17); `syscall.Exec` outside lane mode, plan in a sealed memfd (T18); TSYNC on `svd signal`'s second filter, `MFD_NOEXEC_SEAL`, pidfd claim and `waitid` confirmation (T19); §7.1.2 rewritten: lists reconciled, reachability source half, enforcing dynamic half, symbol-absence half, negative control (T14); `svd-monitor` as its own executable. Draft-15 rules are tagged **[Host+15]**.

Draft 14 resolves `SECURITY-1.md` Part 6 (T1–T11, items 3 and 4; map in §12). It changes the **process structure of the spawn path**, not the architecture: M_p stays the parent of every tree and `clone3`'s pidfd still names the main, because the path is four images of **one pid** (Go's fork path, `svd helper`, `svd-drop`, the service); every per-thread attribute (NNP, Landlock, securebits, capability sets, ids, the seccomp filter) is now set in `svd-drop`, which has **one thread from its `execve` to the service's**, so no root thread ever coexists with a uid-p thread (T2, S3, S5); a new session and explicit fds 0–2 for every spawned process (T1); rows for credentials, rlimits, umask, working directory, environment and signal state (T3); the service's filter installed by the last system call before `execve`, never under the Go runtime (T4); rollback of steps M2–M4 (T5); the record decision checks the plan generation before every plan-dependent Fault (T8); and M_p stays unfiltered for P1 by the user's decision, with a syscall-surface lint (§7.1.2). Draft-14 rules are tagged **[Compile+14]** and **[Host+14]**. Draft 13 (2026-11-23) resolves `SECURITY-1.md` (S1–S13, its Part 1 gaps, its Part 3 TCB remarks and its Part 4 corrections; map in §12). It changes no architecture: the lane/monitor split, the record channel and the FSM stay. It adds rules around the channel: no granted tree inherits an OOM exemption (S1); svd never works in a directory a grantee can write (S2); every signal to a granted tree is sent with p's credentials by a short-lived **signaller**, so M_p holds no `CAP_KILL` and never handles a pid (S3); an allowlist seccomp filter for L_p and the signaller, and **none on M_p**, because a filter on M_p would be inherited by every granted service (S4, stated in §12); the helper's id drop in a verified order, with no keep-caps (S5); the lane account's gid, groups, host reservation, uniqueness and persistence, and a lane that can own nothing (S6); non-blocking sends and a rate for every record kind (S7); mounts by descriptor (S8); and S9–S13. The spawn path is listed as a numbered sequence with the privilege held at each step (§7.1.1), so that it can be audited as the decoder was. Draft-13 rules are tagged **[Compile+13]** and **[Host+13]**. Draft 12 resolves `REVIEW-8.md` (N14–N20 and its bibliography; map in §12). REVIEW-8 found the model converged and changed none of it: no edit of this draft touches the event table, the guard table, the settle rules, the cut or the proof of Theorem G over the single-queue model. It changes the **runtime privilege layer**: draft 11's root lane with three capabilities is replaced by a **privilege split** per delegated principal p (§3.3 H_OS-E5): an **unprivileged lane L_p** under a dedicated uid, with no capability and only pre-opened descriptors, parses every byte p writes; a **root monitor M_p** reads nothing but fixed-size, versioned, range-checked records from L_p (format and total behaviour on a malformed record stated), is the parent and subreaper of every tree of grant(p), creates the exec-helper's mount namespace (N15), and signals only through pidfds, `cgroup.kill` and freeze-then-enumerate on Linux, `PROC_REAP_KILL` on FreeBSD, **never by a pid** (N14, N16); inventory rows **H12** (pid reuse against svd's privileged signals) and **H13** (p's bytes relayed to owner terminals, N19); freeze before kill at a monitor's death, with the count bound stated (N17); the core's orphan source skips every child it tracks and runs whenever the core is a reaper of any kind, mode B included (N18); five platform-table fixes (N20); and REVIEW-8's bibliography corrections, with the two points no opened page states marked *unverified, test in P1/P4* with the test named. Draft-12 rules are tagged **[Compile+12]** and **[Host+12]**. Draft 11 resolves `REVIEW-7.md` (N7–N13, its bibliography and its platform finding; map in §12): every lane is a **subreaper**, so the core never reaps p's processes (N7, inventory row H10); the core never waits on a lane: forwarded owner operations are enqueued without blocking and answered at once, and lane snapshots are read with bounded retries and validated (N8, row H11), which restores item 1's equal enabledness; the lane runs as **root with its capabilities reduced** and is capped by the principal's container on every OS that allows grants (cgroup on Linux, `loginclass` rctl on FreeBSD, N9); a service that some terminal service reaches by a require path and that is exposed to a grant is **refused** (`ErrExposedEscalates`, user decision refined by N10); the serialisation lemma is restated as a refinement of the O projection (N11); the `freeAcrossCut` fixture is corrected and `swAcrossCut` becomes a `Compile` test (N12, N13); the listener socket's **path** is `chown`ed (unix(7)); the rctl actions and racct are corrected from rctl(8)/rctl(4); and one table states, per OS, whether grants and readers are allowed and which hypotheses hold (§3.3, **macOS now refuses readers too**). Draft-11 rules are tagged **[Compile+11]** and **[Host+11]**. Draft 10 resolves `REVIEW-6.md` (N1–N6, the line-872 justification, the F3 costs and Gate 1's five gaps; map in §12): G_free and SW ignore dependents across a grant boundary (N1), so no transition of a service outside a grant reads a granted service; Theorem G is re-proved over an **explicit inventory of every input p controls**, each shown confined to grant(p) or cut at the boundary (§3.3); svd's work for each non-owner principal runs in a **per-principal lane process** inside that principal's cgroup (H_OS-E5), so p's load reaches svd's core only through the kernel, and the one residual that no supervisor can remove (shared-hardware timing) is stated as the hypothesis H_OS-A3 with a test; a timer cell drains the sources of every service it reads (O5′, N2); CPU, IO, swap and log-read caps per principal (N3); one request holder per (principal, service) and a bounded audit log (N4); U renamed V in P2b (N5); a defensive `SCM_RIGHTS` walk (N6). Draft-10 rules for `Compile` are tagged **[Compile+10]** and new host checks **[Host+10]**; §12 lists both. Draft 9 resolves `REVIEW-5.md` (F1–F11 and its Gate 1 specification; map in §12): svd's own resources partitioned per principal (H_OS-E5, F1), "runs as p" defined on resolved ids and granted services held to p's privileges (F2), goal propagation truncated at every grant boundary for every principal (F3), three principal classes (F4), P2 split into P2a (Gate 1) and P2b (Compile) (F5), the guard grid's `Absent` value and priority (F6), the timer columns renamed (F7), and Gate 1's non-interference check as 2-safety by self-composition. Every draft-9 rule that `Compile` must newly enforce is tagged **[Compile+9]**, and every new host-side check at load or before a fork **[Host+9]**; §12 lists both. Draft 8 resolved `REVIEW-4.md` (E1–E14; map in §12): `run_as` = delegated principal ⇔ granted (E1), grants closed under the reverse of every edge kind (E2), H_OS split into what svd enforces and what it assumes (E6), Theorem G re-proved, the payer's overflow in `charge` (E3), the sojourn bit `armed` (E4), P2 restated (E5). Draft 7 recorded the user's decisions on D2 (option A), D-M2 and the binary name (`svd`). This draft resolves every finding of
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
  - the met K5 `capabilities` of A is ∅, otherwise `ErrDelegationPrivilege{A, p, "capabilities"}`. On Linux the exec-helper then clears the permitted, effective, inheritable and ambient sets **and** the bounding set (`PR_CAPBSET_DROP` for every capability) before `execve`; **draft 13 (SECURITY-1 S5):** the bounding set is dropped **before** `setresuid`, while `CAP_SETPCAP` is still held, in the numbered order of §7.1.1 (steps 15–17, test `TestHelperDropOrder`, P1). On Windows, `capabilities` denotes the token's privileges; a granted service's token keeps only `SeChangeNotifyPrivilege` *(mapping unverified, P3)*, so `SeDebugPrivilege`, `SeTcbPrivilege`, `SeImpersonatePrivilege`, `SeBackupPrivilege`, `SeRestorePrivilege`, `SeTakeOwnershipPrivilege` and every other one are refused by the same error. macOS and FreeBSD have no per-process capability set in this schema; `capabilities` must still be ∅;
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
- `ErrAccountChanged{service}` (F2b), an SEp before a fork **[Host+9]**;
- `ErrExposedEscalates{service, principal, terminal, path}` (REVIEW-7 N10, user decision; §3.3 H_OS-A4) **[Compile+11]**: `service` is exposed to grant(`principal`) and the terminal service `terminal` reaches it by the require path `path` (of length 0 when `service` is itself terminal);
- `ErrIsolationUnavailable{listener, freebsd}` and `ErrIsolationUnavailable{cpu, macos}` for a **reader** (§3.3 platform scope) **[Host+11]**.

Two diagnostics are **warnings**, not errors: `WarnUnheldOutsideTarget{service, target}` **[Compile+9]**, when an enabled granted service requires an outside service that is not enabled (F3, §2.3); and `WarnExposedToGrant{service, principal}` **[Compile+10, flag removed in 11]**, for an exposed outside service that no terminal service reaches by a require path (§3.3 H_OS-A4). They are reported by `svd validate` and `svd plan`, and do not refuse the plan.

These are API errors, not Compile errors:

- `ErrStillWanted{by}` (N12);
- `ErrOutsideGrant{services}` (N2);
- `ErrPrincipalBudget` (N2);
- `ErrQuiescing`;
- `ErrLeaked`;
- `ErrLaneBusy{principal}` (REVIEW-7 N8, draft 11): an owner-class operation on a granted service found the owner-to-lane queue full; it had no effect.
- `ErrLaneAccount{principal, uid, reason}` (REVIEW-8 N14, draft 12; reasons extended in draft 13, SECURITY-1 S6): the lane id allocated to a principal from `lane.uids` (used as both uid and gid) is 0 (`zero`), equals a principal's uid or gid or a service's resolved uid or gid (`plan`), is shared with another lane (`shared`), resolves to a host account or group not reserved for svd (`host-account`), lies in a sub-uid or sub-gid range or a dynamic-user range (`subid`), is held by a running process (`in-use`), or the range has no unallocated id left (`exhausted`); `Load` refuses, nothing starts. This is the one statement of the lane-id rule; §3.3 and §12 refer to it.

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
  - a request holder, added by `start`.
- **At most one request holder per (principal, service) (REVIEW-6 N4, draft 10).** The request holder of principal q on X is named h(q, X), not by a fresh request ID. A `start X` by q while h(q, X) ∈ x(X) adds nothing and opens no transaction: it answers with the open transaction of h(q, X) if there is one (the caller then waits for that transaction's outcome), and otherwise with X's current state. A repeated `start X --no-rollback` marks h(q, X) permanent, so a later rollback of its transaction no longer removes it (the lost update of §2.4 stays removed). Hence |x(X)| ≤ 1 + |{q : q may write X}|, a number fixed by the plan, and the holders of the whole instance number at most |S| × (1 + |principals|). Draft 9 added a fresh holder per `start`, so R_p bounded only the rate at which p added holders, and svd's memory grew without bound (N4).
  - **Reading (draft 16, PR #5).** A repeated `start X` by q is **not an error**: it never returns `ErrTxBusy` or any refusal that a fresh `start` would not return, and it opens no transaction, empty or not (an empty transaction would "succeed" at once and write an outcome nobody asked for). Open transactions are indexed by **holder** h(q, X), not by principal or by (principal, partition): q may have one open transaction on X and another on Y at the same time. A bound per principal is a different, stricter semantics that blocks q's `start Y` behind its own `start X`; per (principal, partition) it is also what Gate 1 needs, but per holder it is what §2.4 states and it subsumes the partition split, since a holder lies in one partition.
- goal(A) = up ⟺ x(A) ≠ ∅ ∨ ∃A′: goal(A′) = up ∧ edge(A′, A) ∈ {require, want} ∧ ¬cut(A′, A). This is the least fixpoint over the DAG.
- **Goal truncation at every grant boundary (draft 9, F3; replaces draft 8's per-principal holder scope).** An edge is **cut** when it leaves a grant:

  cut(A′, A) ⟺ ∃p: A′ ∈ grant(p) ∧ A ∉ grant(p)

  The cut applies to **every** holder, whoever created it: the owner's requests and `boot` included. Draft 8 restricted only delegated holders, so an owner holder on a granted service still raised an outside goal, and p could lower it again by `stop`, or by stalling the owner's transaction into rollback (REVIEW-5 F3, routes 1 and 2).
  - *Which edges are cut.* By grant closure (§8.2, E2) no edge enters a grant from outside it, and grants are disjoint (§1.6), so an edge between two grants is impossible. The cut edges are therefore exactly the edges from a grant to O, the set of services in no grant.
  - *What it gives.* For C ∈ O, goal(C) is a function of the holders on services of O alone: every uncut path into C starts in O, because a path that enters a grant can leave it only by a cut edge. For A ∈ grant(p), goal(A) is a function of the holders on grant(p) alone. Every transaction's member set (§2.4) therefore lies wholly in one grant or wholly in O.
  - *The cost, stated.* A granted service's outside require-targets are not raised by it. They must be held on their own, for example `enabled`, or started by the owner. A granted A that requires an outside C whose goal is down waits for C and reaches Failed(wait) locally; `svd status` reports C as "held down outside grant".
  - *The owner's costs, stated (REVIEW-6 Part 3, draft 10).* They are necessary, not incidental: if any holder on a granted service could raise goal(C) for C ∈ O, p would decide when C starts and stops by holding or stopping that service, and each start of a C that crashes on startup pays R_inst. Concretely, with A ∈ grant(p) requiring C ∈ O:
    - (a) the owner's `start A` does not raise C. Without another holder on C, A ends in Failed(wait) and the owner's transaction rolls back;
    - (b) an "on-demand" C, meant to run only while A runs, cannot be expressed: C must be held on its own, for as long as A may need it;
    - (c) the owner's `stop C` does not report `ErrStillWanted` for a running A (D(C) ⊆ O, below). It tears A down: A goes to StopWait(dep) through G_lost, and since draft 10 C does not wait for A's graceful stop either (G_free, §3.5, N1).

    **Operator recipe.** Hold an outside target explicitly before the granted service that needs it: `enabled = true` on C, or `svd start C` and then `svd start A`. `WarnUnheldOutsideTarget` (below) names the case at Compile.
  - **Invariant: one partition per request (draft 10).** The targets of one transaction lie in one partition, O or a single grant. A future multi-target `start A C` with targets in different partitions must open one transaction per partition, each with its own holder, deadline and rollback; otherwise the rollback of the granted part, which p can provoke, would lower C again (F3 route 1). v1's `start` takes one target, so the invariant holds by construction; any API that adds targets must keep it, and Gate 1's 2-safety check covers it once modelled.
  - **[Compile+9]** When an `enabled` granted A requires an outside C that is not `enabled`, `Compile` reports `WarnUnheldOutsideTarget{A, C}` (§1.10): the plan is valid, but A cannot come up at boot unless the owner starts C.
- **Who may remove which holders (F3).** The holders on a service of O are created and removed only by owner-class operations, by the rollback of an owner transaction whose members lie in O, and by Quiescing. A delegated p's operations reach services of grant(p) only (§8.2 grant check), and there they may clear **any** holder, the owner's requests and `boot` included (`stop`, below): the services are p's subtree, and by the cut those holders raise nothing outside it. A `boot` holder cleared this way is re-added only at the next instance start.
- Goal depends only on holders and edges, never on states. It is therefore **fixed during a step's propagation round** (§3.5). Two things change holders inside a step after the input: the rollback of a failed transaction (§2.4) and escalation (§3.3, a terminal service entering Failed through G_failed or G_wexp). **Draft 16:** both are applied **between rounds**, never in the middle of one: the round runs to its fixpoint with the goals it started with, then the pending rollbacks and the escalation (mode := Quiescing, clear every holder, abort every open transaction) are applied, and a new round starts. A step ends when a round ends with nothing pending. There are at most 1 + (number of open transactions) + 1 rounds, since each round boundary consumes a transaction or the single escalation. An implementation that clears holders in the middle of a round makes the result depend on where in the order the escalating service sits.
- **`stop X` (N12).** Let D(X) be the set of services that keep goal(X) = up through an uncut require or want edge, given current holders. For X ∈ O, D(X) ⊆ O, so neither the outcome of an owner's `stop X` nor its `ErrStillWanted` list depends on the state of any grant (F3).
  - If D(X) is non-empty, `stop X` is refused with `ErrStillWanted{by: D(X)}`. Clearing x(X) alone would leave X running, which is what REVIEW-2 found.
  - **Order of checks (R3-L7).** For a non-owner caller the grant check of §8.2 runs first, so a caller outside grant gets `ErrOutsideGrant` and learns nothing about D(X). Then `by` is filtered to D(X) ∩ grant(p), plus a count of the others when the caller lacks `api.read`.
  - `stop X --with-dependents` clears the holders of X and of every service in the reverse uncut require/want closure that keeps X up. This is systemd's behaviour. For X ∈ grant(p) that closure lies in grant(p) (closure and cut), and for X ∈ O it lies in O.
  - Either way, every open transaction that held a cleared holder is aborted. Those transactions report `superseded` and do not roll back. When p's `stop` aborts an owner transaction this way, that transaction's members lie in grant(p) (§2.3 cut), so no outside service is affected (F3).
  - Both forms are subject to the grant check of §8.2.
  - **Reading (draft 16, PR #5).** These three rules are part of the pure FSM, not of the API layer: `ErrStillWanted` is computed from holders and edges only; p's `stop X` on X ∈ grant(p) clears **every** holder of X (the owner's and `boot` included), not only h(p, X); and the abort with `superseded` happens in the same step. A model in which `stop` clears only the caller's holder and leaves the transaction open is wrong in three observable ways: X keeps running under another holder with no refusal (REVIEW-2's finding), the transaction later fails at its deadline and reports `failed` instead of `superseded`, and until then the holder's next `start` is blocked.

### 2.4 Transactions and rollback (default on)

- `start X` by q, when h(q, X) ∉ x(X) (§2.3, N4), adds h = h(q, X) to x(X) and opens a transaction T = {services whose goal goes down→up because of h}. Goals are computed with the cut of §2.3, so T lies wholly in grant(p) when X ∈ grant(p), and wholly in O otherwise (F3: the owner's `start A` with A ∈ grant(p) requiring C ∈ O gives T = {A}; C is not a member, and the rollback of T cannot lower C).
  - **T succeeds** when every member is Ready or Done.
  - **T fails** when a member reaches Failed or Leaked, or when the request deadline passes. *(Draft 16: Leaked added. A Leaked member is stable, never Ready, and already counts as failed for its require-dependents' G_failed, §3.5; waiting for the deadline only delays the same rollback.)* A transaction whose holder is cleared by `stop` or by Quiescing ends `superseded` or `aborted` and never reaches either outcome.
  - **The request deadline** is defined here: `start` takes it from the class-T field `api.request.timeout` (or a shorter per-request value). It belongs to the transaction's principal: an owner transaction's deadline is a low input of Theorem G, and a deadline of p's transaction is a **high** input, acting on members in grant(p) only (§3.3, REVIEW-6 Gate 1 gap 3).
  - **Bounded records (N4).** There is at most one open transaction per holder, so open transactions are bounded as holders are. A closed transaction becomes an audit record (`/v1/changes`). The audit log in memory is a ring of `audit.ring` records per lane (§3.3 H_OS-E5), class T; older records are appended to a size-rotated file under the instance state directory (`audit.file.size` × `audit.file.count`, class T), whose disk use is in H_OS-A2 (*host*). **The file is per process (REVIEW-7, N4 note, draft 11):** the core and each lane rotate their own file, so p's records can rotate out only p's. A record dropped because the file cannot be written is counted, never retained in memory.
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
   - **Partitioned execution (draft 10, H_OS-E5).** At run time there is one step loop per partition: the **core** runs the step loop of O (every service in no grant) and serves owner-class principals; each non-owner-class principal p has a **lane**, a separate svd process that runs the step loop of grant(p) and serves p's listener (§3.3 H_OS-E5). A lane reads O's state from a snapshot that the core publishes after each of its steps, and never writes O. The core reads nothing a lane produces into its step loop, and never waits on a lane (§3.3 H_OS-E5, the lane protocol, REVIEW-7 N8). *Serialisation lemma, restated as a refinement of the O projection (REVIEW-7 N11, draft 11).* No cell, guard or settle rule of a service of O reads a service of a grant (§3.5, N1; asserted statically by Gate 1, §3.8), so **the projection onto O** of a run of the partitioned runtime is the projection onto O of a run of the single-queue model: order every lane step after the core step whose snapshot it read. Draft 10 claimed the whole run, which is false: after a core step in which C ∈ O goes to StopWait and is signalled (G_free(C) skips its cut dependent A ∈ G), the lane still sees C Ready until the next snapshot, so A ∈ G is Ready while C ∉ {Ready, Done}, a state the single-queue model never shows after propagation; and a lane reading a stale snapshot may spawn A after C has begun stopping. Hence Gate 1's results transfer as follows: **Theorem G**, which needs obs_O only, holds for the runtime; the **one-copy** properties (I1–I6, T1, T2) hold for every service of O, and for a granted service towards its targets inside its grant; towards an **outside** (cut) target they hold **up to the snapshot lag**: A notices C's loss at the first lane step after the snapshot that shows it (G_lost), and a spawn of A from a stale snapshot is followed by that loss. This cost falls on p alone.
   - **Bounded intake, stated (REVIEW-6).** A full per-source queue means "stop polling that fd" until it drains: the kernel buffer then back-pressures the writer. A lifecycle event (MainExit, TreeEmpty) is never dropped: each source holds at most one pending MainExit and one pending TreeEmpty per epoch, outside the bounded queue, and the observe-drain-reap order of O1 keeps the reap behind the drain.
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
- **O5′, drain what the timer cell reads (REVIEW-6 N2, draft 10; generalises O5).** O5 orders a service's own timer only, but Waiting × WTO of T reads the states of req(T) (pending), and round-robin does not order the sources of one round by arrival. So: **before a state timer cell of C is delivered, svd drains into the table every event that was readable, at the timer's dequeue, on the sources of every service the cell reads.** For each timer column, the read set is:
  - WTO: C itself and every B ∈ req(C); for each B, its readiness and keepalive channels, then its MainExit, then its TreeEmpty, in the O1/O2 order;
  - StartTO, SWTO, StopTO, KTO, BTO: C itself (O5). SWTO's cell sends `term` whatever G_free says, so it reads no other service.

  Each drained event is its own step (with propagation) before the timer's step; the set is fixed at dequeue, so the drain terminates. One level suffices: a `READY=1` of B can be readable only while B is in Starting, whose own requirements are already Ready or Done (I1). In the partitioned runtime (step 1) req(C) ⊆ O for C ∈ O, by grant closure, so the drain reads only the core's sources. **[Gate 1]** asserts that the read set of every timer cell, computed from the data, is contained in the drained set.
- **O2, exit before empty.** `TreeEmpty(e)` is enqueued only after `MainExit(e)`.
  - Tree emptiness is observed through cgroup.events `populated 0`, `JOB_OBJECT_MSG_ACTIVE_PROCESS_ZERO`, or reaper status *(Windows and FreeBSD unverified)*.
  - The source holds TreeEmpty back until the main process has been reaped.
- **O3, synchronous spawn failure, before any child exists (D7).** SEt and SEp cover **only** failures that svd itself detects before a child process exists: the fork, `clone3` or `CreateProcess*` call fails, or a check that svd runs before forking fails. No tree exists in that case, so I2 holds. They are classified (N11) as one of:
  - **SEt**, transient: `EAGAIN`, `ENOMEM`, `EMFILE`, `ENFILE`;
  - **SEp**, permanent: the exec-helper binary is missing or not executable; an unknown account, which svd resolves before forking; an account whose id differs from the one resolved at load (`ErrAccountChanged`, F2b, **[Host+9]**); `EPERM` from `clone3` with `CLONE_NEWNS` or `CLONE_INTO_CGROUP` (draft 12, REVIEW-8 N15: draft 11 would have classified it `fail` and charged it, so every granted service with a read-only region crash-looped on R_p); **draft 13 (SECURITY-1 S13):** also `EACCES`, `EBUSY` and `EOPNOTSUPP` from `CLONE_INTO_CGROUP` (a target that is not a leaf, has a domain-invalid type, or is not writable). Epoch cgroups therefore stay leaves with an empty `cgroup.subtree_control`, written only by M_p (test `TestCloneIntoCgroupErrnos`, P1). For a granted service the spawn is M_p's, and M_p reports the class in its `NACK` reason code, a fixed enumeration.

  Under H_OS-E5 **and H_OS-A2 (*host*)** (§3.3) an SEt of a service outside every grant cannot be produced by a delegated principal: svd's fds, memory and spawn headroom are partitioned so that p's load cannot exhaust what an outside spawn needs (F1), and the host-wide resources that E5 does not partition (`ENFILE`'s system file table, the global pid space, host memory) are covered by A2(*host*) and the [Host+10] sums of §3.3. Draft 9 cited E5 alone, which claimed too much (REVIEW-6 N3).

  An unclassified error counts as transient.
- **O4, failures after the fork go through the tree (D7).** Everything the exec-helper detects happens in a forked child inside the epoch's tree: the region conflict of §7.2, `ErrProtectionUnavailable`, `ENOENT`, `EACCES` or `ENOEXEC` from the final `execve`, and setup errors. The helper reports them as follows.
  - It writes a classified status on a **CLOEXEC status pipe** and exits with status 127. A successful `execve` closes the pipe, so the parent reads EOF.
  - **Classification (E11).** The helper classifies by a closed table; the table is data and has a unit test per row.

    | failure in the helper | sub-status |
    |---|---|
    | final `execve`: `ENOENT`, `ENOTDIR`, `EACCES`, `EPERM`, `ENOEXEC`, `ELOOP`, `ENAMETOOLONG`, `EISDIR`, `E2BIG`, `ELIBBAD` | `perm` (a property of the plan or the file system, the same on every retry) |
    | final `execve`: `ENOMEM`, `EAGAIN`, `ETXTBSY`, `EMFILE`, `ENFILE`, `EIO` | `fail` |
    | region conflict (§7.2), `ErrProtectionUnavailable` (§7.1), `setgroups`/`setgid`/`setuid` `EPERM`, `EINVAL` from a protection syscall (Landlock, seccomp, NNP), `EPERM` from `mount` (making mounts private, or a K6 read-only remount; draft 12, REVIEW-8 N15) | `perm` |
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
  - **H_OS-E5, svd resource isolation (F1, draft 9).** What svd itself spends to serve one principal is drawn from that principal's partition, so p cannot exhaust what svd needs to serve a service outside grant(p). Per delegated or reader principal p, class G fields (§1.7) state `budget.connections`, `budget.inflight`, `budget.refusals` (refused writes per second) and, for delegated principals, `budget.pids` and `budget.memory`; **[Compile+9]** they are in the completeness set Q whenever p is a non-owner-class principal (`ErrIncomplete`). **[Compile+10]** Draft 10 adds `budget.cpu` (a CPU quota, in CPUs), `budget.io` (read and write bytes per second and IOPS), `budget.swap` and `budget.log` (log bytes per second read from p's services) for delegated principals, and `budget.memory` and `budget.cpu` for readers too (their lane, below, runs in a per-principal cgroup); all are in Q for their class (`ErrIncomplete`). The mechanisms:
    - **Per-principal lanes (draft 10, REVIEW-6 N1-class closure).** Every non-owner-class principal p has a **lane**: an svd process (the same binary, `svd lane`), started by the core at load, placed in p's per-principal container (`deleg/<uid>/lane` on Linux, below) and therefore charged to p's caps. The lane runs the step loop of grant(p) (§3.1 step 1), has grant(p)'s trees spawned, signalled and reaped by p's **monitor** (draft 12, privilege split below; draft 11's lane did this itself), reads their channels, notify sockets and logs, owns p's API listener, keeps p's transactions, holders and audit ring, and pays R_p. It reads O's state from a fixed-size snapshot the core publishes (a shared mapping written by the core under a sequence lock, read-only in the lane *(mechanism per OS unverified)*). **Draft 13 (SECURITY-1 S9) [Host+13]:** one snapshot memfd per lane (never one shared by all lanes), mapped read-only by the lane and sealed `F_SEAL_FUTURE_WRITE` after the core has made its own writable mapping, so no new writable mapping can be made from it. Confinement does not rest on the seal: a lane that corrupted its own snapshot would corrupt only p's view (test `TestSnapshotReadOnlyInLane`, P1: a `PROT_WRITE` mapping and an `mprotect` to write fail in L_p). The core does no work on p's behalf: it never reads a granted service's fd, never accepts on p's listener, never runs a step of grant(p), and **never reaps a process of p's** (subreaper, below). Owner-class operations on a granted service are forwarded by the core to the lane (low inputs: the owner issues them), under the lane protocol below; `svd status` of granted services reads the lane's own fixed-size snapshot on the owner's request only, and nothing read there enters the core's step loop. *Why a process and not a thread:* a thread of svd shares the core's allocator, garbage collector and scheduler slot, so p's flood would pause the core through GC; a process in p's cgroup does not.
      - ~~**The lane's account (REVIEW-7 N9, draft 11).** The lane runs as root with `CAP_SETUID`, `CAP_SETGID` and `CAP_KILL`.~~ **Withdrawn in draft 12 (REVIEW-8 N14–N16).** The reduction was nominal: uid 0 still owns every root-owned file, cgroup interface files included, so a lane under p's control could write `/etc` or the core's `cgroup.freeze` with no capability (DAC owner match, *from memory*, REVIEW-8); `CAP_SETUID` is not limited to p's uid, so it could become an O service's uid and void A1; FreeBSD has no capability sets, so the FreeBSD lane was full root, unstated (N14). The set also could not run §7.1, whose first Linux step needs `CAP_SYS_ADMIN` (N15), and `CAP_KILL` on pids read from a list made pid reuse an escalation route (N16, row H12). Draft 11's two reasons for a non-p uid stand (a lane running as p can be traced, stopped and forged by p; the cgroup files it writes would be p-writable), and are met below without giving the parser any privilege.
      - **Privilege split: monitor and lane (REVIEW-8 recommendation, draft 12) [Host+12].** For each **delegated** principal p the core starts two processes, both its children, both in p's container (Linux `deleg/<uid>/monitor` and `deleg/<uid>/lane`; FreeBSD `loginclass:svd-<uid>`), so both are capped as draft 11's lane was. A **reader** gets a lane only, with no monitor (it has no trees). This is the OpenSSH monitor pattern with Capsicum-style pre-opened descriptors. **Invariant: no process that holds a privilege reads a byte p wrote, and no process that reads p's bytes holds a privilege.**

        | | **L_p, the lane** (`svd lane`) | **M_p, the monitor** (`svd monitor`) |
        |---|---|---|
        | uid | a dedicated id n from the class-G range `lane.uids` ([Compile+12], in Q whenever a non-owner principal exists), **unique per principal** and checked against the plan and the host by the rule of `ErrLaneAccount` (§1.10, **[Host+12]**, extended **[Host+13]**); real = effective = saved = fs uid = n. **gid (draft 13, SECURITY-1 S6):** real = effective = saved = fs gid = n, never 0; **supplementary groups: none** (`setgroups(0, NULL)`). **Allocation is stable [Host+13]:** the map principal → n is persisted in the instance state directory (root, 0600, replaced atomically); a principal keeps its id across loads, and an id once allocated is never given to another principal (a removed principal's id stays retired; the range is widened by the owner, never recycled), so nothing an id still owns can cross principals. **L_p can own nothing [Host+13]:** it is started through the helper sequence of §7.1.1 in lane mode, in its own mount namespace whose every mount is read-only, and in new IPC and network namespaces; so even with its sandbox escaped the lane id can create no file (hence no set-user-ID file), no System V or POSIX IPC object and no abstract socket (test `TestLaneOwnsNothing`, P1). L_p sets `PR_SET_DUMPABLE` 0 at its start, so no other process of its id could trace it even if uniqueness failed | 0 (root) |
        | privilege | none: empty permitted, effective, inheritable, ambient and bounding sets; NNP set; Landlock with an empty write set once its descriptors are held (Linux); `cap_enter` (FreeBSD). **seccomp is an allowlist (draft 13, SECURITY-1 S4) [Host+13]:** default action `SECCOMP_RET_KILL_PROCESS`, installed after NNP with `SECCOMP_FILTER_FLAG_TSYNC` so every thread of the Go runtime carries it (test `TestLaneFilterAllThreads`, P1). The list is not written by hand: a Go tool (`cmd/seccompgen`, tested) derives it from a `SECCOMP_RET_LOG` trace of the lane's test suite under the pinned toolchain, and the result is committed; a toolchain bump regenerates it and its diff is reviewed. Argument rules: `tgkill` only when the full 64-bit `args[0]` equals L_p's own pid, which the filter holds as a constant (the runtime's preemption signals go to its own threads, and a tgid equal to its own pid names no other process); `kill` only when `args[0]` equals the own pid (the runtime's `raiseproc`); `clone` only with `CLONE_THREAD` set; `clone3` returns `ENOSYS`, so the runtime falls back to `clone`, whose flags a classic BPF filter can read (it cannot read `clone3`'s struct). **Absent from the list, so denied:** `rt_sigqueueinfo`, `rt_tgsigqueueinfo`, `tkill`, `pidfd_open`, `pidfd_send_signal`, `pidfd_getfd`, `ptrace`, `process_vm_writev`, `execve`, `execveat`, the `setuid` family, `mount` and every new-mount call. L_p forks nothing, which is why it can carry a filter (tests `TestLaneSeccompAllowlist`, `TestSeccompTgkillSelfOnly`, P1). Draft 12's denylist missed `rt_sigqueueinfo` (S4) | Linux (draft 13): **C_M = {`CAP_SYS_ADMIN`** (new mount namespace and mounts by fd for the helper, N15), **`CAP_SETUID`, `CAP_SETGID`, `CAP_SETPCAP`** (the helper's id and bounding-set drop), **`CAP_SYS_RESOURCE`** (the helper's oom reset, S1)**}**; every other capability dropped, bounding set included *(the exact set is unverified, test `TestMonitorCapSet`, P1)*. **`CAP_KILL` is removed (S3):** M_p signals nothing itself; every graceful signal is sent by the signaller under p's credentials (signalling rule below). **This set is root-equivalent (`CAP_SYS_ADMIN`), and is stated so: what bounds M_p is its input surface and its TCB below, not its capabilities.** **M_p carries no seccomp filter (draft 13, a deviation from SECURITY-1 S4's « allowlist for M_p », §12):** a filter is inherited by `fork` and preserved across `execve` *(seccomp(2), from memory; the test below measures it)*, and M_p forks every helper, so any filter on M_p binds every granted service; draft 12's filter would have denied `kill` to every granted service. A filtered M_p would need a separate unfiltered spawner, which changes the architecture. In its place: M_p has no `CAP_KILL`, never holds a pid as a number, and `TestNoRawKill` covers `internal/monitor`; **test `TestGrantedServiceNoInheritedFilter`** (P1) reads `Seccomp:` in `/proc/self/status` inside a granted service and must find only the plan's own filter. FreeBSD: **full root**, no capability sets exist; bounded the same way |
        | descriptors | all pre-opened, received at start or by `SCM_RIGHTS` from M_p: p's listening socket (from the core, §8.1); the control fd and the snapshot memfd (from the core); its seqpacket socket to M_p; per spawn, the read ends of each epoch's log pipe and status pipe and its notify socket (from M_p, which creates them). **Draft 13 [Host+13]: these three per epoch are the only fds M_p ever sends L_p; a pidfd or a cgroup fd never reaches L_p** (test `TestLaneOwnsNothing` also lists L_p's fds). It opens nothing afterwards | its seqpacket socket to the core (the plan of grant(p), trusted), its seqpacket socket to L_p, `deleg/<uid>/trees/` (a directory fd), **`deleg/<uid>/signal/`** (draft 13, the signaller's cgroup), **p's notify directory** (draft 13, S2) and **the helper binary** (an `O_PATH` fd opened by the core, executed with `execveat`), and per epoch the pidfd from `clone3(CLONE_PIDFD)` and the epoch cgroup's fd |
        | reads | every byte p writes: API requests, notify datagrams, readiness bytes, log lines, the status pipe | records from L_p (below), kernel events (pidfd readiness, `waitid` with `WNOWAIT`, `cgroup.events`), and the core's plan. **Never** a pipe, socket or file p can write |
        | does | the step loop of grant(p), transactions, holders, audit ring, R_p, API, snapshot; decides every spawn, signal and kill of grant(p) and asks M_p by record | forks every exec-helper of grant(p) (`clone3(CLONE_PIDFD \| CLONE_INTO_CGROUP \| CLONE_NEWNS)` on Linux, so the helper starts in a new mount namespace inside its epoch cgroup, N15; the numbered sequence is §7.1.1); creates and writes the epoch cgroups under `deleg/<uid>/trees/`; is the subreaper of every tree; reaps; freezes, thaws and writes `cgroup.kill`; forks the **signaller** for every graceful signal (draft 13: M_p sends no signal itself) |

        *Why E2 holds.* p cannot trace, stop or forge L_p or M_p, because neither runs as p (A1; **[Host+12]** the A1 probes run against L_p's uid and M_p as well as against O's services). Every cgroup interface file is root-owned and written by M_p or by the core, never by L_p (uid ≠ 0, Landlock) and never by p. *Why a compromised L_p is confined:* its only privileged channel is the record socket to M_p, and every record M_p accepts names a service of grant(p), the current epoch of that service, and a signal from that service's plan; so a compromised L_p can do what p's own `stop`/`restart` can do to G, and nothing to O. *Why readers hold nothing:* a reader's lane has no trees, so it needs no monitor and no privilege (REVIEW-8 N20 d).

        **Record format, L_p → M_p [Host+12].** One `SOCK_SEQPACKET` datagram per record, **exactly 64 bytes**, little-endian, no pointers, no lengths, no strings:

        | offset | size | field | valid values |
        |---|---|---|---|
        | 0 | 2 | magic | `0x5653` |
        | 2 | 1 | version | `2` (draft 13, **[Compile+13]**; draft 12's version 1 is refused) |
        | 3 | 1 | kind | 1 `SPAWN`, 2 `SIGNAL_MAIN`, 3 `SIGNAL_TREE`, 4 `KILL_TREE`, 5 `REAP_ACK` |
        | 4 | 4 | service | an index < \|grant(p)\|, into M_p's own copy of the plan, which comes from the core |
        | 8 | 8 | epoch | `SPAWN`: M_p's last epoch for that service + 1; every other kind: M_p's current epoch for that service |
        | 16 | 4 | signal | `SIGNAL_*`: a member of that service's plan signal set (`stop.signal`, `reload.signal`); every other kind: 0 |
        | 20 | 4 | flags | 0 (reserved) |
        | 24 | 8 | seq | M_p's last accepted seq + 1 |
        | 32 | 8 | generation | the plan generation M_p holds (draft 13, SECURITY-1 S13): the core bumps it on every plan it sends M_p, and M_p sends it to L_p with each plan change |
        | 40 | 24 | reserved | all zero |

        M_p receives with a 65-byte buffer, `MSG_TRUNC` to learn the true length, and **no control buffer** (`msg_controllen` = 0). **Draft 13 [Host+13]:** a `recvmsg` that returns 0 is L_p's hang-up (a seqpacket peer's close), handled as L_p's death below, never as an empty record; the decoder is panic-free by construction (fixed offsets into a fixed array, no slicing by a decoded value, no map, no conversion that can fail), and `FuzzMonitorRecord` runs with the race detector and asserts no panic; L_p's bytes are never logged, not even on a Fault (the code is fixed). Records M_p sends to L_p (`SPAWNED`, `EXITED{wstatus}`, `TREE_EMPTY`, `NACK{seq, reason}`, `FAULT`) use the same 64-byte frame; L_p parses them unprivileged, and the fds of a new epoch travel with `SPAWNED` from M_p to L_p, never the other way.

        **M_p's decision is a total function of (record, M_p's state), with three outcomes and no fourth [Host+12]:**

        **Order of the checks (draft 14, SECURITY-1 T8) [Host+14].** The outcomes below are unchanged; what draft 14 fixes is the order in which the decoder evaluates them, because a plan change shrinks `grant(p)` or a service's signal set, and an honest L_p that decided under generation g−1 can then send a record that a plan-dependent check of generation g would call a Fault. The decoder evaluates, and stops at the first hit: **(1)** the plan-independent framing checks (length, `MSG_TRUNC`/`MSG_CTRUNC`, magic, version, kind, flags, reserved bytes, seq ≠ last + 1), any failure a Fault; **(2)** the generation: greater than M_p's current one is a Fault (an honest L_p cannot know a generation M_p has not been told), smaller is a Refuse `generation` that consumes the seq; **(3)** only for a record of the current generation, the plan-dependent checks (service ≥ \|grant(p)\|, a signal outside the service's set, a non-zero signal for another kind), any failure a Fault; **(4)** the Refuse conditions of the next bullet; **(5)** Act. So no record an honest L_p sends under a stale plan is ever a Fault. `FuzzMonitorRecord` gains the case « generation g−1, service index valid under g−1 and ≥ \|grant(p)\| under g » and asserts Refuse. *This changes the decoder's decision order, not a table of §3.*
        - **Fault** (the record is not one an honest L_p can send): length ≠ 64, `MSG_TRUNC` or `MSG_CTRUNC` set, a bad magic or version, kind ∉ 1..5, a non-zero flags or reserved byte, seq ≠ last + 1, service ≥ \|grant(p)\|, a signal outside the service's set for `SIGNAL_*`, or a non-zero signal for any other kind. M_p stops reading L_p's socket, sends `FAULT` to the core (a fixed code, not L_p's bytes), freezes and then `cgroup.kill`s `deleg/<uid>/trees/` (FreeBSD: `PROC_REAP_KILL` with `SIGKILL` on its own subtree), reaps, and stays alive as the subreaper until the core retires it. The core handles a fault as a lane death (below): p's cost, once per owner restart, R_inst and `mode` untouched.
        - **Refuse** (well formed, but stale or over budget, which an honest L_p can send because an epoch may end between L_p's decision and M_p's read): an epoch that is not current, or for `SPAWN` not next; a `SPAWN` while the previous tree of that service is not empty and reaped (so two incarnations never coexist, H3's Leaked rule); a `SPAWN` beyond the service's plan intensity (max per window, counted by M_p); a `SIGNAL_MAIN` whose main has exited. **Draft 13 (SECURITY-1 S7, S13) [Compile+13, Host+13]:** a generation other than M_p's current one (`generation`: a plan changed between L_p's decision and M_p's read); a non-`SPAWN` kind beyond **`monitor.rate`** (class T, in Q whenever a delegated principal exists: per service and per kind, the records accepted per second; `rate`); a `SIGNAL_*` or `KILL_TREE` while a signal operation of that service is outstanding (one at a time per service; `busy`). M_p does nothing, counts it, and answers `NACK{seq, reason}`. **Every well-formed record consumes a seq**, Refused or Acted, so L_p's and M_p's counters cannot drift apart on a Refuse.
        - **Act**: exactly the action of the kind, on the named epoch only, through the mechanisms of the signalling rule below.

        M_p reads at most `monitor.batch` records per round (class T, [Compile+12]); its decoder is a pure function over a fixed array, allocates nothing, and cannot block; so no sequence of records makes M_p hang, grow, or address a process outside the epoch cgroups of grant(p). **M_p never blocks on L_p (draft 13, SECURITY-1 S7) [Host+13]:** every send to L_p is `sendmsg` with `MSG_DONTWAIT`, from a fixed reply ring of 2·\|grant(p)\| + `monitor.batch` entries; when the ring is full M_p **stops reading records** until it drains (L_p's own back-pressure, no Fault, no wait), and keeps serving kernel events and the core. The freeze wait of the signalling rule is a `cgroup.events` readiness event with a deadline in M_p's event loop, never a blocking wait (tests `TestMonitorSendNeverBlocks`, `TestMonitorRecordRate`, P1). **TREE_EMPTY (S10)** is sent only once the epoch cgroup reports `populated 0` **and** M_p has reaped every waitable child of that epoch, so a zombie cannot outlive the record that announces the tree gone (test `TestTreeEmptyAfterReap`, P1). **Test:** `FuzzMonitorRecord` (corpus committed) feeds arbitrary byte strings and record sequences and asserts that every input yields one of the three outcomes, that M_p's heap is constant, and that no fault leaves a tree populated.

        **M_p's TCB, kept small [Host+12].** What runs with privilege and must be correct for E1–E5 to hold against a compromised L_p: the record decoder above; the spawn path (`clone3`, cgroup creation, the helper's setup up to `execve`, which reads the core's plan, never L_p's bytes; numbered in §7.1.1); the reaper (`waitid` with `WNOWAIT`, pidfds); the signalling path, **the signaller included (draft 13)**; **the Go runtime and the `syscall` and `golang.org/x/sys/unix` packages these use (draft 13, SECURITY-1 Part 1: a privileged Go process trusts its runtime)**; and the kernel. **The helper (`svd helper`, a separate small binary, executed from an fd) is measured separately** in `svd plan`, and the **audit order is: the helper, then the signalling path, then the decoder** (§10). **Outside the TCB:** the HTTP and JSON API decoder, the notify-datagram parser, the log reader, the readiness reader, the FSM step loop of grant(p), transactions and the snapshot writer, which all run in L_p. M_p is the package `internal/monitor`, reached only from `svd monitor`'s entry point; **test `TestMonitorImportClosure`** (P1) fails if that package's import closure contains the API, JSON, notify or FSM packages, and `svd plan` reports M_p's measured code size. The fuzz targets of §10 and the security audit cover the TCB first.

        **Cross-process O1 (observe, drain, reap).** M_p observes a main's exit by its pidfd (or `waitid(P_PID, …, WEXITED | WNOWAIT)`), sends `EXITED` to L_p, L_p drains that epoch's channel and status pipe and answers `REAP_ACK`, and only then M_p reaps. An L_p that never acks leaves a zombie in p's `pids.max`, at p's cost; at L_p's death M_p reaps without acks.

        **The core's view.** The core never reads a record and never waits on M_p or L_p (H11). It learns a monitor's death from M_p's pidfd (M_p is its child), and a fault from a fixed code on its socket to M_p. **The Quiescing deadline for grant(p)** is enforced by the core's own `cgroup.kill` on `deleg/<uid>/` (FreeBSD: `PROC_REAP_KILL` on M_p's subtree) when §3.6's stop bound expires, never by waiting on M_p.
      - **Subreaper (REVIEW-7 N7, draft 11; moved to M_p in draft 12) [Host+11, Host+12].** **M_p**, not the lane, makes itself the subreaper of its descendants **before its first fork**: `prctl(PR_SET_CHILD_SUBREAPER, 1)` on Linux (prctl(2), man-pages 6.19, opened by REVIEW-8: orphans go « to the nearest still living ancestor subreaper »; the attribute is not inherited across fork, so M_p sets it itself); `procctl(P_PID, getpid(), PROC_REAP_ACQUIRE)` on FreeBSD (procctl(2), FreeBSD 15.1, opened by REVIEW-8: the call applies to **future children only**, so « before its first fork » is required, not a precaution; EBUSY if already a reaper). Every process of grant(p)'s trees descends from M_p (M_p forks every exec-helper), so an orphan of p's is reparented to M_p and reaped there, in p's container, at p's cost; the process that parses p's input (L_p) is no tree's parent, so its death orphans nothing (REVIEW-8 N17). M_p reaps orphans that are not the main of a live epoch, at most `reap.batch` per round. Windows has no reparenting (process handles, job objects) and macOS refuses every grant; neither needs it. A monitor that cannot become subreaper exits at start, and the load fails with `ErrIsolationUnavailable{reaper, os}`. Draft 10 left the core as the reaper of every orphan, so p could fork-and-exit at a rate it chose and turn capped CPU into the core's reaping work, sharing one source with O's MainExit events (REVIEW-7 N7).
      - **Signalling rule: never a pid read from a list (REVIEW-8 N16, row H12, draft 12) [Host+12].** It binds **every** svd process that signals a tree, the core for O's trees as well as M_p for grant(p)'s (through draft 9 the core had the same race). A pid read from `cgroup.procs`, `/proc` or `waitid` is never passed to `kill(2)`; between the read and the kill, p can let the process exit, reap it, and cycle the pid counter so that an O process takes the pid. Instead, on Linux:
        - a tree's `SIGKILL` is a write of `1` to its epoch cgroup's `cgroup.kill`;
        - **every graceful signal of grant(p), the main's included, is sent by the signaller (draft 13, SECURITY-1 S3) [Host+13]**, a short-lived process that holds **p's credentials and no capability**, so the kernel's own permission rule for signals (A1: an unprivileged sender reaches only processes of its real or saved uid) is the barrier, and a pid that p cycled onto an O process cannot be reached whatever list it came from. Its sequence, with the privilege at each step:
          1. M_p (R) writes `1` to the epoch's `cgroup.freeze` and waits, as an event with deadline `freeze.timeout` (class T, [Compile+12]), for `frozen 1`; it opens that epoch's `cgroup.procs` read-only (an fd).
          2. M_p (R) `clone3(CLONE_PIDFD | CLONE_INTO_CGROUP)` into `deleg/<uid>/signal/`, with no new namespace, passing the `cgroup.procs` fd, the main's pidfd (for a main signal) and the signal number from the plan; the child `execveat`s the helper binary in signal mode (§7.1.1 steps 5–6; privilege still R). **Draft 14 (T1, T2):** the child is created as in §7.1.1 step M3 (new session, explicit fds 0–2, CLOEXEC everywhere else), and every per-thread step below (3, 4) runs in the single-threaded drop stage `svd-drop` (§7.1.1), never in the multi-threaded Go helper.
          3. Signaller (R): oom value 0 (`CAP_SYS_RESOURCE`, S1); `setgroups(0)`; `setresgid(gp)`; inheritable := ∅, `PR_CAP_AMBIENT_CLEAR_ALL`, securebits as §7.1.1 step 15, `PR_CAPBSET_DROP` of every capability (`CAP_SETPCAP`).
          4. Signaller: `setresuid(p,p,p)`; privilege now none. It verifies ids, groups and all five sets empty and `setresuid(0)` failing `EPERM`, sets NNP, installs its own allowlist filter (`read`, `pidfd_open`, `pidfd_send_signal`, `exit_group` and what the runtime needs, generated as L_p's is), else exits `perm`. **Draft 14 [Host+14]:** steps 3 and 4 are done by `svd-drop` (one thread, `PR_SET_DUMPABLE` 0 re-asserted after each id change, §7.1.1 D-steps), whose filter also allows `execveat` of the one `svd signal` fd; `svd signal`, a Go binary, sets `PR_SET_DUMPABLE` 0 and stacks a second filter **without** `execve`/`execveat` as its first actions. Between its `execve` and that `prctl`, `svd signal` is dumpable (execve(2), man-pages 6.19: dumpable is set to 1 on exec of a non-set-user-ID file) **and every thread of it is already uid p with no capability**, so an attaching uid-p process gains nothing p does not hold; M_p writes `1` to `deleg/<uid>/signal/cgroup.kill` after every signaller, exited or not, so no p-forked residue survives in `signal/`. Step 5's `cgroup.procs` parse is strict: decimal digits only, a bounded line length, pids ≤ 1 skipped, anything else `perm`.
          5. Signaller (p, no capability): a main signal goes through the inherited pidfd (`pidfd_send_signal`); a tree signal reads `cgroup.procs` from the fd, and for each pid `pidfd_open` then `pidfd_send_signal`; it exits.
          6. M_p (R) reaps the signaller by its pidfd and writes `0` to `cgroup.freeze`. If the signaller has not exited by the deadline **`signal.timeout`** (class T, [Compile+14]; draft 13 named no deadline), M_p writes `1` to `deleg/<uid>/signal/cgroup.kill` and thaws; if the freeze did not complete, the graceful signal to the rest of the tree is skipped and the epoch proceeds by `cgroup.kill` at StopTO.

          *What the freeze is for, corrected (SECURITY-1 Part 4).* Draft 12 claimed that a frozen cgroup's pid list could not be recycled before `pidfd_open` held each pid, and that `pidfd_open` guarantees no recycling. Both were over-stated: a frozen cgroup's membership can still change (a process can be killed, or migrated by root, while frozen), and pidfd_open(2) guarantees only that the pidfd refers to the process the pid named **at the call**, which may already be a recycled pid. So **the freeze is for completeness only** (no process forks a child past the walk); **the barrier is the signaller's uid**. A recycled pid that reaches a uid-p process is p's own cost; it cannot reach O's. `TestFreezeBlocksReap` is kept as a measurement and is **no longer load-bearing**. That `pidfd_send_signal` applies the same uid permission rule as `kill(2)` is *from memory*: **test `TestPidfdSendSignalUidRule`** (P1: a uid-p signaller's `pidfd_send_signal` to a uid-q process must fail `EPERM`); until it passes, the signaller uses `kill(2)` for tree signals, whose rule A1's probes already measure, and the main's signal still goes through its pidfd, which names the process `clone3` created (test `TestSignallerCredentials`, P1);
        - M_p holds no `CAP_KILL` and no filter (privilege row above); **test `TestNoRawKill`** (P1) fails if `internal/monitor` or the core's signalling package calls `kill`, `tkill`, `tgkill`, `rt_sigqueueinfo` or `pidfd_send_signal` itself. **What « no `CAP_KILL` » does and does not mean (draft 14, SECURITY-1 T11) [Host+14]:** it removes the override, not the uid rule: M_p runs as uid 0, so under A1 it can still signal every process whose real or saved uid is 0, the core and every root O service included. That reach is closed by `TestNoRawKill` and by the syscall-surface lint of §7.1.2 (no signal-sending call in M_p's application list), not by the kernel; the same holds for the signaller of an O service whose `run_as` is root, which is why such a signaller is short-lived, exec'd from a fixed binary, and filtered to the two pidfd calls. **The core's own trees (O)** are signalled the same way, by a signaller with the target service's uid, never by root.

        On **FreeBSD**, M_p signals a tree with `procctl(P_PID, getpid(), PROC_REAP_KILL)` and `REAPER_KILL_SUBTREE` on that epoch's subtree (procctl(2), opened by REVIEW-8), and the main through its process descriptor (`pdfork`, `pdkill`) *(from memory, P4)*; the kernel walks the reaper's descendants itself, so no pid is read and re-used. That the subtree id survives an orphan's reparenting to M_p is *from memory* (test `TestReapKillSubtreeAfterOrphan`, P4). **macOS** has no non-owner principal, so H12 does not arise there; **Windows** signals through job and process handles, never ids.
      - **The lane protocol: the core never waits on a lane (REVIEW-7 N8, draft 11).** Three couplings carry a lane's progress, which p controls by loading its own container, and each is made unable to delay or block the core:
        - *Owner-to-lane queue.* One `SOCK_SEQPACKET` socketpair per lane (the **control fd**), written by the core and read by the lane. The core writes a forwarded owner operation **without blocking**; if the socket buffer and the core's fixed per-lane queue (`lane.queue`, class T) are both full, the owner's operation is answered at once with `ErrLaneBusy{p}` and has no effect. **The core reads nothing from the control fd**: it watches it only for hang-up, and learns the lane's death from the lane's own MainExit.
        - *Owner requests that wait.* The core answers a forwarded operation **when the enqueue succeeds**, with a ticket (`202 Accepted {lane: p, change: id, deadline, mac}`); it holds no connection, fd, buffer or timer for it afterwards. The operation's transaction and its outcome live in the lane, which publishes the outcome in its snapshot. **The deadline is in the ticket (REVIEW-8 N20 c, draft 12):** the core computes it at enqueue from the transaction's deadline (the low input of §2.4) and binds (p, id, deadline) with an HMAC under a per-boot key that never leaves the core. A client that waits (`svd start A`, A ∈ grant(p)) polls `/v1/tickets/<ticket>` (draft 11 wrote `/v1/changes/<id>`, which §8.3 names as the audit log); each poll is a separate owner request that the core answers in bounded time: a bad MAC is `400`; past the deadline the core answers `expired` **without reading the snapshot and without state**; otherwise it answers from the snapshot. The CLI stops polling at the ticket's deadline and polls with exponential backoff, so a starved lane can no longer keep the owner's own pollers alive indefinitely on owner-class slots. So every owner-class request occupies the core's owner-class connection and inflight slots for a time that does not depend on the lane, and the core's memory bound (below) holds with only the fixed term `lane.queue` per lane.
        - *Snapshot reads.* The lane publishes its fixed-size snapshot in a shared mapping, under a sequence counter. The core reads it with at most `lane.snapshot.retries` attempts (class T, default 3); a read that never sees an even, unchanged counter, or whose records fail validation (fixed layout, enumerated state values, names that exist in grant(p)), yields "grant(p): status unavailable" in that response, and is counted. A reader stopped mid-write therefore costs the core a constant per request. Validation matters because the lane parses p's input and must be presumed compromised. Every string field of the snapshot (`STATUS=` text, unit-reported reasons) is fixed-size and length-checked, and is relayed to the owner under row H13's escaping rule, never interpreted by the core. The snapshot memfd is created by the core at its fixed size and sealed with `F_SEAL_SHRINK` and `F_SEAL_GROW` before it is handed to the lane, so the lane cannot truncate it under the core's mapping *(that a shrink-sealed memfd spares the reader `SIGBUS` is from memory, test `TestSnapshotSealNoSigbus`, P1)*.

        Under these rules **every owner-class operation is enabled (accepted by the core) in a run iff it is enabled in the same run without p**, and an operation on a service of O has the same effect in both. Only the **outcome** of an owner operation on a granted service may differ (`ErrLaneBusy`, a late or unavailable change): that outcome is grant(p)'s, not O's, and is outside obs_O (§3.3, item 1).
      - *A lane's death* (for example p's trees exhaust `budget.memory`, which the lane shares) is p's cost: the core reaps it, kills p's container (`cgroup.kill` on `deleg/<uid>/`, `TerminateJobObject` on the per-principal job), reports it, and does **not** restart the lane; `svd lane restart p` (owner-class) does, and **(draft 13, SECURITY-1 S10) [Host+13]** it first verifies that `deleg/<uid>/trees/` and `deleg/<uid>/signal/` report `populated 0`, then writes `0` to `cgroup.freeze` on `deleg/<uid>/` and every child it froze, so a restarted lane never starts into a frozen container (test `TestLaneRestartThaws`, P1). The core's work per lane death is a constant, once per owner restart, so p cannot repeat it at a rate of its choosing. R_inst and `mode` are never touched. *Draft 11's statement that the lane's descendants are reparented to the core at its death is withdrawn:* since draft 12 the lane is no tree's parent. On L_p's death (or a record fault), M_p sees the hang-up of its L_p socket, **freezes** `deleg/<uid>/trees/` and then writes its `cgroup.kill`, and reaps every tree process itself; nothing reaches the core.
      - **A monitor's death (REVIEW-8 N17, draft 12) [Host+12].** M_p is the parent and subreaper of grant(p)'s trees, so its death is the remaining orphan window. It is made rare and bounded:
        - *rare:* M_p allocates nothing per record or per spawn after start, and its `oom_score_adj` is −1000 (written by the core), so p's exhaustion of `budget.memory` makes the memory controller's OOM killer pick another process of `deleg/<uid>/` *(the cgroup OOM killer's respect of −1000: opened by SECURITY-1 Part 4; test `TestMonitorNotOomVictim`, P1, kept)*. **No granted process inherits it (draft 13, SECURITY-1 S1) [Host+13].** `oom_score_adj` and its floor `oom_score_adj_min` are inherited by `fork`, and an unprivileged process may lower its value down to that floor *(proc(5))*, so draft 12 handed every granted tree an unkillable −1000. Now every process M_p forks (helper, signaller) writes `0` to `/proc/self/oom_score_adj` while it still holds `CAP_SYS_RESOURCE`, which sets both the value and the floor to 0, **before** its id drop (§7.1.1 step 8); and the core writes `0` for L_p through `/proc/<pid>/oom_score_adj` of its not-yet-reaped child before L_p reads its first input. **Test `TestGrantedTreeOomAdj`** (P1): inside a granted service, `oom_score_adj` reads 0 and a write of `-1` fails `EACCES`; the same for L_p;
        - *bounded by a count, not a rate:* M_p is the core's child, so the core holds its pidfd. Within a round, the core drains every monitor-death source **before** its orphan source (§5.1), and its first action on a monitor's death is to write `1` to `deleg/<uid>/cgroup.freeze`, then `1` to `deleg/<uid>/cgroup.kill`. A frozen process neither forks nor exits voluntarily, so after the freeze p creates no further orphan. `pids.max` bounds the population of `deleg/<uid>/`, and a zombie still counts in it until it is reaped *(from memory, test `TestZombieCountsInPidsMax`, P1)*, so the processes of p's that the core reaps per monitor death are at most **`budget.pids` + `reap.batch`** (the population at the freeze, plus one orphan batch the core may have reaped in the round in progress before it saw M_p's pidfd), once per owner restart. REVIEW-8 N17 is right that draft 11's « at most `budget.pids` » bounded a population and called it a count; with the freeze first the count bound holds.
        - *where they go:* on Linux, whether a dead subreaper's descendants go to the next living ancestor subreaper is **unverified** (prctl(2) does not say; REVIEW-8 Part 4); the core is a subreaper in modes B and C (§5.1), so they would come to the core. **Test in P1: `TestSubreaperDeathReparent`** (core subreaper → M_p subreaper → child → grandchild; kill M_p; assert the grandchild's parent is the core, and that the orphan source reaps it). The bound above does not depend on the answer: if they go to the host init instead, they are still frozen and killed by `cgroup.kill`, and the core reaps none. On FreeBSD procctl(2) **states** that a dead reaper's descendants are « reassigned to the reaper process's reaper », which is the core (§5.1); FreeBSD has no freezer, so the core's first action is `PROC_REAP_KILL` with `SIGKILL` on M_p's former subtree *(that the subtree id of M_p's descendants survives M_p's death is from memory, test `TestReapKillSubtreeAfterReaperDeath`, P4)*, and the FreeBSD bound is `maxproc` plus what p forks during one core round before the kill, stated rather than claimed constant.
        - M_p is not restarted automatically; `svd lane restart p` restarts both processes of p.
      - In mode U every grant is empty and there are no readers outside the owner (§1.6), so mode U has no lanes.
    - **One API listener per principal, owned by its lane.** Each non-owner-class principal, readers included (draft 9's single shared reader listener was inconsistent with per-principal reader budgets, REVIEW-6), gets its own API socket, in its own directory under the instance runtime directory (§8.1). **Linux (draft 11, REVIEW-7 Part 4):** the directory is root-owned, mode 0711; the core binds with umask 0177, `fchownat`s the **path** (`AT_SYMLINK_NOFOLLOW`) to p, `lstat`-verifies socket type, owner p and mode 0600, calls `listen`, and hands the listening fd to the lane at its start (so the lane needs no `CAP_CHOWN`); a mismatch fails the load. unix(7) (<https://man7.org/linux/man-pages/man7/unix.7.html>, man-pages 6.19) states « On Linux, connecting to a stream socket object requires write permission on that socket » and that a pathname socket's owner and mode are changed « using chown(2) and chmod(2) »; it mentions fchown(2) only for abstract sockets, where it has no effect on accessibility. Draft 10's `fchown` of the socket fd is withdrawn: it would leave the path with its creator's ownership. No ACL is needed. **FreeBSD:** unix(7) also warns that « on some systems (e.g., older BSDs), the socket permissions are ignored », so svd does not rely on the socket's mode there: the socket lives in a root-owned directory of mode 0700 with one POSIX.1e ACL entry granting search to p's id only (`u:<uid>:x`) *(setfacl(1), unverified)*; a file system without ACL support is `ErrIsolationUnavailable{listener, freebsd}` **[Host+11]**. **macOS** has no non-owner principal (platform table below). **Windows:** DACL *(unverified, P3)*. Owner-class principals share the core's listener, which no other principal can reach. Each listener has its own kernel backlog and its own process (the lane), so p's connection flood fills only p's backlog and costs only p's lane. The lane still checks peer credentials on every connection (a mismatch is closed), and stops accepting while p holds `budget.connections` connections or `budget.inflight` requests. A refused write beyond `budget.refusals` per second closes the connection and pauses the listener for the rest of that second. Every response is bounded in size, `/v1/logs` included (N3: a log read returns at most `api.response.max` bytes per request).
    - **fd headroom, per process.** At start the core and each lane raise their `RLIMIT_NOFILE` soft limit to the hard limit and compute their need. The **spawn reserve** of a process counts, for every service it runs (O for the core, grant(p) for p's lane), the **steady-state** fds of a running service (log read end, per-epoch notify socket, pidfd or process handle, cgroup.events watch or job handle) **plus**, for one concurrent spawn per service, the **transient** fds of a spawn (both ends of the status pipe and of the readiness pipe, the log pipe's write end until the fork) (REVIEW-6: "the fds of one spawn per service" undercounted). The core adds the owner-class reserve and one control fd per lane; a lane adds its `budget.connections`. **[Host+9]** If a hard limit is below its need, the load fails with `ErrResourceBudget{fds, need, have}`. A process refuses `accept` when its free fds would drop below its spawn reserve. The core's fd table holds no fd of p's, so an outside spawn never sees `EMFILE` because of p. Windows has no per-process handle limit of this kind *(from memory)*; there the bound is the process's memory (next item).
    - **svd's memory, per process (N4, draft 10).** The core's memory is a fixed term plus per-service terms over O plus the owner-class terms plus `lane.queue` forwarded operations per lane (draft 11: owner requests no longer wait on a lane, so the owner-class terms end with each request, REVIEW-7 N8); a lane's is a fixed term plus per-service terms over grant(p) plus p's API terms. Every term is bounded by the plan: request size, per-connection buffer, `budget.inflight` requests, per-service bounded intake queues (§3.1 step 1), the log ring (§9), the holders (at most |S| × (1 + |principals|), §2.3), open transactions (at most one per holder, §2.4), and the audit ring (`audit.ring`, §2.4). svd reports each process's bound in `svd plan`. Draft 9's bound was false because holders and audit records grew without bound (REVIEW-6 N4).
    - **Event intake.** Each process (core, lane) has its own per-service bounded queues dequeued round-robin (§3.1 step 1), with O5 and O5′. A flood of `STATUS=`, `WATCHDOG=1`, log lines or readiness bytes from a granted service fills only its lane's queues, and costs CPU in p's cgroup only.
    - **Log reads (N3, draft 10).** A granted service's log pipe is read by its lane at most `budget.log` bytes per second across grant(p); beyond that the lane stops reading, the pipe fills, and the **writer blocks**: back-pressure harms only p's own trees (§9). The core reads O's log pipes with the §9 ring-and-drop policy.
    - **Notify sockets (F11, N6).** svd's `recvmsg` on a notify socket passes a control buffer sized for one credentials message only (`CMSG_SPACE` of `struct ucred` on Linux, `cmsgcred` on FreeBSD *(unverified)*). A datagram with `MSG_CTRUNC` set, or with any control message other than credentials, is dropped and counted. **Defensive walk (draft 10, N6):** after every `recvmsg`, whatever the flags, svd walks every received control message and `close`s every fd of every `SCM_RIGHTS` message before it drops the datagram, as systemd does. The buffer size alone is not enough: on 64-bit Linux `CMSG_SPACE(4)` = 24 ≤ `CMSG_SPACE(sizeof(struct ucred))` = 32, so if the kernel placed `SCM_RIGHTS` before `SCM_CREDENTIALS` (with `SO_PASSCRED` set), a rights message carrying up to four fds would fit and be installed *(the kernel's order and the sizes are from memory)*. With the walk, an installed fd lives only until the end of that `recvmsg` handling, in the lane, never in the core. The notify socket is per epoch (§4.1), so its receive buffer is per service.
    - **Process slots, memory, swap, CPU and IO of p's container (draft 10 adds CPU, IO and swap, REVIEW-6 N3).** svd's forks of outside services, and O's trees and the core once running, need host process slots, memory, CPU time and IO bandwidth that p's granted trees and p's lane could take. They are **capped** per principal, as an aggregate over p's lane and grant(p), written by svd and never by p (H_OS-E2). A weight is a share under contention, not a cap, so draft 9's `cpu.weight` gave no term a site could sum; it is replaced:
      - Linux, cgroup v2: a per-principal parent cgroup `deleg/<uid>/`, holding p's monitor (`deleg/<uid>/monitor`, draft 12), p's lane (`deleg/<uid>/lane`) and every epoch cgroup of grant(p) under `deleg/<uid>/trees/` (draft 12: a separate parent, so that M_p can freeze and kill every tree without freezing itself), and **`deleg/<uid>/signal/`** (draft 13: the signaller's leaf, outside `trees/` so that it runs while the trees are frozen, and inside p's caps), with `pids.max` = `budget.pids`, `memory.max` = `budget.memory`, **`memory.swap.max` = `budget.swap`**, **`cpu.max` = `budget.cpu` × period, period** and **`io.max` = `budget.io`** written for every block device (major:minor) that holds a path O's plan reads or writes, the instance state directory, or swap; plus `RLIMIT_NPROC` = `budget.pids` in every granted tree (it counts processes per real uid). **[Host+10]** svd's delegated subtree must have the `pids`, `memory`, `cpu` and `io` controllers enabled in `cgroup.subtree_control`; the device list is resolved at load, and a device that cannot be resolved is `ErrIsolationUnavailable{io, linux}`. deleg/<uid>/ holds only child cgroups, so the no-internal-process rule is met *(controller file semantics from memory)*;
      - FreeBSD (draft 11, REVIEW-7 N9 and Part 4): the per-principal container is a **login class**, `svd-<uid>`: the core sets it on the lane and on the monitor (draft 12), and the exec-helper, forked by the root monitor, sets it on every granted tree before it drops to p (`setloginclass(2)`: « restricted to the super-user », and « processes inherit login class from their parents », confirmed by REVIEW-8 from the FreeBSD 15.1 page), so p cannot leave it. **Unverified, test in P4:** that the class survives `execve` and the helper's `setuid`, which the page does not state; **test `TestLoginClassSurvivesExecve`** (the N9 test of §10, renamed) reads the class of a granted service's process after `execve` and after a further `execve` of a set-user-ID-p binary, and must find `svd-<uid>`. Until it passes, FreeBSD delegation stays refused (platform table). `rctl` rules on `loginclass:svd-<uid>` cap the lane and grant(p)'s trees as one aggregate, as `deleg/<uid>/` does on Linux; draft 10's `user:<uid>` subject did not count a root lane (N9). rctl(8) lists exactly four subject types, process, user, loginclass and jail (<https://man.freebsd.org/cgi/man.cgi?query=rctl&sektion=8>, FreeBSD 15.1). The rules, with the actions rctl(8) allows: `maxproc`, `memoryuse` (resident set size), `swapuse` and `pcpu` (« %CPU, in percents of a single CPU core ») with **`:deny`**; `readbps`, `writebps`, `readiops` and `writeiops` with **`:throttle`**. rctl(8): `deny` is « not supported for cputime, wallclock, readbps, writebps, readiops, and writeiops », `throttle` is « only supported for readbps, writebps, readiops, and writeiops », and an unsupported pair is an error, so draft 10's « each `:deny` or `:throttle` » would have failed at rule load. `RLIMIT_NPROC` = `budget.pids` in every granted tree, as on Linux. **racct:** rctl(4) (<https://man.freebsd.org/cgi/man.cgi?query=rctl&sektion=4>) says `kern.racct.enable` « defaults to 1, unless options RACCT_DEFAULT_TO_DISABLED is set in the kernel configuration file »; it is a **loader tunable**, set in `loader.conf` and read at boot, so enabling it needs a reboot (GENERIC setting that option is *from memory, unverified*). svd reads `kern.racct.enable` at load; when it is 0 there is no CPU or IO cap, so **[Host+10]** delegation is refused (`ErrIsolationUnavailable{cpu, freebsd}`), and since draft 11 so is every reader **[Host+11]**; the draft-9 fallback of `RLIMIT_NPROC` plus a per-process `RLIMIT_AS` is withdrawn for delegation. It was safe but, as REVIEW-6 noted, `RLIMIT_AS` bounds address space, not memory, and breaks Go programs and JVMs, which reserve large address ranges; most granted runtimes would have been unusable under it;
      - macOS: no per-uid or per-process-group CPU or IO cap is known to this draft *(from memory, unverified)*, so **[Host+10]** a plan with a non-empty grant is refused on macOS (`ErrIsolationUnavailable{cpu, macos}`) until a mechanism is verified. **Readers are refused too (draft 11, REVIEW-7 Part 5) [Host+11]:** draft 10 allowed reader lanes and required `budget.memory` and `budget.cpu` of them, but no macOS mechanism enforces either, and its residual was misnamed (H_OS-A3 is contention *below* the caps, and there was no cap). `budget.inflight` bounds concurrency, not rate, and `budget.refusals` counts refused writes only, so a reader could keep its lane busy on many cores with valid reads. A weaker mode was considered and not adopted: a reader lane with `GOMAXPROCS=1` and a per-reader request rate would bound the lane to about one core, which is a bound, not a cap, and would make Theorem G's H6 row false on macOS. So on macOS only owner-class principals exist, and Theorem G holds vacuously (no p). Candidates that throttle without a quota (`RLIMIT_CPU` kills rather than caps; taskpolicy and `setiopolicy_np` apply background throttling *(from memory)*) are listed as an open feature gap in §12. The draft-9 `RLIMIT_NPROC` + `RLIMIT_AS` row is withdrawn for the reason above;
      - Windows: a per-principal job object, parent of the lane's job and of the per-epoch jobs (nested jobs), with `JOB_OBJECT_LIMIT_ACTIVE_PROCESS`, `JOB_OBJECT_LIMIT_JOB_MEMORY` (committed memory, which includes pagefile-backed memory, so it bounds swap too), `JOB_OBJECT_CPU_RATE_CONTROL_HARD_CAP` and an IO rate limit (`JobObjectIoRateControlInformation`) *(all unverified, P3)*.

      **[Host+9]** A host that lacks the mechanism its OS row names refuses to load a plan with a non-empty grant: `ErrIsolationUnavailable{resource, os}`. There is no opt-out: a delegation whose isolation cannot be enforced is not offered.

      **Platform scope (draft 11, REVIEW-7 Parts 2 and 5) [Host+11].** The one place that states, per OS, what a plan with non-owner principals gets. « Holds » means: svd enforces the E-hypotheses by the mechanism named, and the A-hypotheses remain assumptions with the tests of §10. A row whose mechanism is *unverified* refuses until its phase's tests pass (§11), by [Host+9].

      **Scope of the table (draft 12, REVIEW-8 N20 e) [Host+12].** It describes the **system instance in modes C and B** only: mode U has no lanes (§1.6). On Linux it assumes a writable cgroup v2 subtree (systemd `Delegate=yes`, or a writable cgroup under s6, runit or OpenRC, §5.3); where svd tracks trees by process group instead (§5.3), a plan with a grant or a reader is refused with `ErrIsolationUnavailable{cgroup, linux}`, since neither the cap, `cgroup.kill` nor the freezer exists there.

      | OS (phase) | grants | readers | cap (aggregate monitor + lane + grant(p)) | privilege split (draft 12) | subreaper and tree signals | listener | Theorem G |
      |---|---|---|---|---|---|---|---|
      | Linux (P1) | allowed, if `pids`, `memory`, `cpu`, `io` controllers are enabled, every `io.max` device resolves, and the kernel has `clone3` with `CLONE_PIDFD` and `CLONE_INTO_CGROUP`, `pidfd_open`, `pidfd_send_signal`, `cgroup.kill` and `cgroup.freeze` (else `ErrIsolationUnavailable{pidfd\|cgroupkill\|freeze, linux}`), and (draft 13, S8) `open_tree`, `move_mount` and `mount_setattr` (else `ErrIsolationUnavailable{mountapi, linux}`) | allowed, capped (`memory.max`, `cpu.max`); lane only, **no capability** (N20 d) | cgroup `deleg/<uid>/`, uid-independent | M_p root with C_M = `CAP_SYS_ADMIN`, `CAP_SETUID`, `CAP_SETGID`, `CAP_SETPCAP`, `CAP_SYS_RESOURCE` (draft 13: no `CAP_KILL`; root-equivalent, stated), no seccomp filter (§12); L_p dedicated uid and gid, no capability, NNP, allowlist seccomp, Landlock | M_p: `PR_SET_CHILD_SUBREAPER` (prctl(2), opened); `cgroup.kill`; graceful signals by the signaller under p's uid, freeze for completeness only (H12, draft 13) | root 0711 dir, path `chown`ed to p, 0600 (unix(7)) | holds: E1–E5 enforced, A1–A4 assumed; N3 closed **because M_p creates the helper's mount namespace (N15)**: draft 11's capability set could not, so every service with a K6 read-only region failed to spawn (REVIEW-8 N20 a) |
      | FreeBSD (P4) | allowed only with `kern.racct.enable` = 1 (loader tunable); refused otherwise | same condition as grants; capped by `pcpu`, `memoryuse` `:deny`; lane only | rctl on `loginclass:svd-<uid>` (`setloginclass` root-only and inherited: opened; across `execve`: **unverified**, `TestLoginClassSurvivesExecve`) | **M_p full root** (FreeBSD has no capability sets, N20 b); L_p dedicated uid, `cap_enter` | M_p: `PROC_REAP_ACQUIRE` before its first fork (procctl(2), opened: future children only); per-epoch tree = M_p's reaper **subtree** (§5.1), killed by `PROC_REAP_KILL` + `REAPER_KILL_SUBTREE` *(subtree semantics from memory)* | root 0700 dir + ACL search entry for p; socket mode not relied on | holds once P4 verifies `execve` inheritance of the class, the reaper subtree tests, rctl rules and the A1 probes; until then refused |
      | macOS (P2) | **refused** (`ErrIsolationUnavailable{cpu, macos}`) | **refused** (same error) | none known | not needed | not needed | not needed | vacuous: only owner-class principals |
      | Windows (P3) | refused until P3 verifies nested job objects with CPU hard cap and IO rate control | refused until P3 | per-principal job object *(unverified)* | not established (token equivalent of the split is a P3 item) | not needed (no reparenting); job and process handles, never ids | DACL *(unverified)* | not established |

      **Delegation is Linux-only in v1** (REVIEW-7 Part 5): the same HCL plan with a grant or a reader loads on Linux, and is refused on macOS, on Windows until P3 and on FreeBSD until P4 (and on FreeBSD hosts without racct). This is a product decision recorded in §12, not a footnote.
    - **Capacity sums (draft 10, [Host+10]).** The owner states, in class G, `instance.reserve.cpu` and `instance.reserve.memory`: what O and the core need. At load svd checks Σ_p `budget.cpu` ≤ online CPUs − `instance.reserve.cpu`, and Σ_p (`budget.memory` + `budget.swap`) ≤ physical memory − `instance.reserve.memory`; a failure is `ErrResourceBudget{cpu|memory, need, have}`. This turns the CPU and memory parts of A2(*host*) from an audit into a check, for svd's trees and lanes. **[Compile+10]** both reserve fields are in Q when any non-owner-class principal exists.
    - **What E5 does not cover.** It bounds what svd spends and what svd's trees of grant(p) take. p's processes outside svd (its logins), host-wide tables that no per-principal cap partitions (the system file table, the global pid space, disk space, inodes), and contention for shared hardware below the caps (caches, memory bandwidth, SMT siblings, interrupts) remain in H_OS-A2 (*host*) and H_OS-A3.

  *Assumed, not enforced, each stated so that it can be tested (F2):*
  - **H_OS-A1, OS privilege (the uid rule).** For every process π of a tree of O, the real, effective and saved uids of π differ from p's id; and a process of uid p holding the privileges svd gives grant(p) (none, §1.6) gets `EPERM` from `kill(π, 0)`, `ptrace(PTRACE_ATTACH)`, `process_vm_writev` and an `open` of `/proc/π/mem` (on Windows, `OpenProcess` with any access right beyond `PROCESS_QUERY_LIMITED_INFORMATION` is denied *(unverified)*). Also, p cannot raise its own privilege on that host (no sudoers entry, no privileged group, no set-user-ID path it may run that leads above p). **Test:** the per-OS suite (§10) runs this probe from a uid-p process, against a sentinel outside service, on each CI host.
  - **H_OS-A2, non-interference, as disjointness predicates over named sets.** "Influence" cannot be observed, so A2 is stated as conditions that a site audit can check, each one per host:
    - *(files)* the paths writable by uid p, or by grant(p)'s plan, are disjoint from the paths read **or executed** by O's trees (binaries, interpreters, libraries, configuration). A file owned by p counts as writable by p, since its owner can change its mode; this covers a set-user-ID-p file executed by an outside service, which would make that process signalable by p through its saved uid. svd knows only the plan part of the first set, and cannot know the second, because v1 has no read or visibility field (§1.9); **A2-files is a site audit, not an svd check**;
    - *(ports)* the ports p can bind are disjoint from the ports O listens on or connects to;
    - *(IPC)* the same for System V and POSIX IPC keys, abstract AF_UNIX sockets and D-Bus names;
    - *(host)* the processes of uid p outside svd, together with Σ over grant(p) of the E5 budgets, leave the memory, CPU, pid space, disk space, inodes and system file table that O and svd need. Since draft 10 svd **checks** the CPU and memory sums for its own lanes and trees (capacity sums, H_OS-E5, [Host+10]); p's logins, the pid space, disk, inodes and the system file table stay a site audit;
    - *(svd)* nothing: svd's own resources moved to H_OS-E5 (F1), and since draft 10 the work svd does for p runs in p's lane, inside p's caps.
  - **H_OS-A3, timing below the caps (draft 10).** A p-controlled quantity that no supervisor can remove, stated rather than treated as exogenous: on shared hardware, p's lane and trees, though capped in CPU time, memory, swap, IO and pids, still contend with the core and O's trees for caches, memory bandwidth, SMT siblings, interrupt handling and scheduler wake-up latency. A3 states that this contention moves the time at which the core observes and processes an O event, or emits an action for O, by at most **Λ**, and that no O deadline decision is made within Λ of the event it races. Λ is not a configuration field; it is measured. **Test:** the N2/N3 chaos test (§10) runs p's maximal load (every cap saturated, API, channel and log floods, busy loops on every core within `budget.cpu`) and measures the core's event-to-step latency; the run fails if its maximum exceeds `instance.timing.lambda` (class G, default 10 ms), and `svd plan` warns when any O timer is below 100 × Λ. A site that needs a tighter A3 confines `deleg/` to a cpuset disjoint from the core's and O's; v1 does not manage cpusets. **Why it cannot be enforced:** a cap bounds an amount over a period, not the instantaneous contention for shared microarchitectural resources, and no portable OS interface partitions those. A3 is the only place where Theorem G assumes a p-influenced quantity to be low; it is named, bounded and measured, and every other p-controlled input is confined or cut (inventory below).

  **Theorem G (delegation cannot escalate), re-proved in draft 10.** Let p be a non-owner-class principal (delegated or reader, §1.6), G = grant(p) (∅ for a reader) and O its complement, other principals' grants included (their holders and events are low with respect to p). Split the inputs of a run (REVIEW-5 Part 5):
  - **high:** every input p controls, listed exhaustively in the inventory below (H1–H13; H12 and H13 added in draft 12);
  - **low:** every owner-class API operation (including those on services of G), every event of a service in O, the request deadlines of owner transactions (§2.4), SIGTERM to svd, and the abstract-time ticks that age the windows.

  There is **no oracle** (draft 9 had a "StopWait oracle" for G_free(C), treated as low although its value was a function of G's events: REVIEW-6 N1 showed that p could choose it and convert O crashes into R_inst charges, reaching Quiescing(1)). Since draft 10 G_free and SW of a service ignore its dependents across a cut edge (§3.5, §3.6), so no transition of a service of O reads G.

  **Inventory of the inputs p controls (draft 10).** Four reviews found the same defect class: a proof that treats a p-controlled input as exogenous (REVIEW-3 D2, REVIEW-4 E2, REVIEW-5 F1/F3, REVIEW-6 N1). The class is closed by enumeration: every input on which p's choices can act is listed, and each is shown either **confined** (its effects reach only G, R_p and p's lane) or **cut** (a named rule removes every path from it to obs_O). An input missing from this list is a defect in the theorem, not in the runtime.

  | # | input p controls | how it is confined or cut | rule |
  |---|---|---|---|
  | H1 | p's API operations: which, their arguments, their order, their rate and timing, connection floods, malformed requests | arrive on p's listener only, served by p's lane (E5); grant check and effect-set check refuse any target outside G before any effect (§8.2, §3.3); holders created or removed by p lie on G (§2.3); the cut makes goal(C), C ∈ O, independent of every holder on G; API charges pay R_p, whose overflow never changes `mode` | cut + confined |
  | H2 | the deadlines of p's own transactions, and their rollbacks | members of p's transactions lie in G (§2.4, one partition per request); rollback removes h(p, X) only | confined |
  | H3 | process behaviour of G's trees: exit status and **timing**, **signal handling** (ignoring SIGTERM, slow graceful stop), forking, tree-emptiness timing, becoming Leaked | G's process events are G's states, observed and reaped by p's lane only (H10); a G event reaches no O cell, because: goal is cut (§2.3); G_free and SW of C ∈ O read no dependent across a cut edge (§3.5, §3.6, **N1**); every other guard and cell of C reads req/wnt/ord(C) ⊆ O (closure E2); crash charges of G pay R_p (E1), never R_inst | cut (N1) + confined |
  | H4 | channel output of G: `READY=1`, `STATUS=`, `WATCHDOG=1`, readiness bytes, their content, timing and rate, malformed datagrams, `SCM_RIGHTS` and credential messages, log volume | read only by p's lane (E5); the lane's intake is bounded per source and its CPU and memory are p's caps; log reads are bounded by `budget.log` with back-pressure on the writer (N3); every `SCM_RIGHTS` fd is closed (N6); content changes G's states only | confined |
  | H5 | the **ordering** of G's events against O's events and O's timers | G's events never enter the core's queue (E5 lanes); the core orders O's events and O's timers among themselves by O1, O2, O5 and **O5′** (N2), which drain only O's sources (req(C) ⊆ O); by the serialisation lemma (§3.1 step 1) a lane step can be placed anywhere after the snapshot it read without changing O | cut |
  | H6 | **load**: CPU, memory, swap, IO, pids and fds used by p's trees and by p's lane; svd's own work for p | capped per principal in p's container (E5: `cpu.max`, `memory.max`, `memory.swap.max`, `io.max`, `pids.max`, `RLIMIT_NPROC`; N3), with the capacity sums checked against the owner's reserve ([Host+10]); the core holds no fd, buffer, holder or audit record of p's (E5, N4); host-wide tables in A2(*host*) | confined, residual in A3 |
  | H7 | **timing through shared hardware** below the caps: caches, memory bandwidth, SMT, interrupts, wake-up latency | not removable by a supervisor; bounded by Λ and measured (H_OS-A3) | **assumed** (A3) |
  | H8 | p's processes outside svd (logins): signals, ptrace, files, ports, IPC, set-user-ID binaries; and the death of p's own lane | OS uid rule (A1, testable probes); disjointness of files, ports and IPC (A2); a lane's death is handled by the core once per owner restart, touches neither R_inst nor `mode` (E5) | assumed (A1, A2) + confined |
  | H9 | **what p's processes send to O's services**: connections to an outside C's listeners (the typical use of an edge from a granted A to C), requests, IPC messages, files C reads | not cut by svd: svd cannot tell a crash of C caused by p's request from any other crash of C, and C's crash pays R_inst. Stated as H_OS-A4 below; of the C that svd sees exposed (an edge from G), those a terminal service reaches by a require path are refused (`ErrExposedEscalates`, [Compile+11]) and the others are named (`WarnExposedToGrant`) | **assumed** (A4), partly refused |
  | H10 | **parentage of G's processes** (REVIEW-7 N7, draft 11): orphans, at a rate p chooses (fork-and-exit loops), and the zombies they leave | p's **monitor** M_p is the subreaper of every tree of G (E5, `PR_SET_CHILD_SUBREAPER` / `PROC_REAP_ACQUIRE`; draft 11 made the lane the subreaper), so every orphan of p's is reparented to and reaped by M_p, in p's container; the core observes O's exits through per-epoch pidfds or `waitid(P_PID)` and reaps orphans through a source of its own, bounded per round, that skips every child the core tracks (§5.1, N18 a); it receives p's processes only at M_p's death, after freezing `deleg/<uid>/`, at most `budget.pids` + `reap.batch` of them, once per owner restart (N17) | confined |
  | H11 | **the lane's progress, as seen by the core** (REVIEW-7 N8, draft 11): how fast p's lane drains forwarded owner operations, answers, and updates its snapshot, which p controls by loading its own container | the core never waits on a lane (E5, lane protocol): the enqueue never blocks and a full queue answers `ErrLaneBusy` at once; a forwarded operation is answered on enqueue and holds no core resource afterwards; the core reads nothing from the control fd; a snapshot read is bounded in retries and validated, and never enters the step loop. So no core resource held for an owner request, and no owner operation's enabledness, depends on the lane; only the outcome of an owner operation on G does, and it lies outside obs_O. Since draft 12 a ticket carries its own deadline and the core answers `expired` statelessly (N20 c) | cut |
  | H12 | **pid lifetime against svd's privileged signals** (REVIEW-8 N16, draft 12): when p's processes exit and are reaped (p's processes reap their own children at times p chooses), and how fast the pid counter advances (fork-and-exit within `pids.max`, while p loads its own lane and monitor to widen the window between a pid read and a signal) | **cut** by the signalling rule (E5): no svd process passes a pid read from a list to `kill(2)`; a tree is killed by `cgroup.kill`; **draft 13 (SECURITY-1 S3, Part 4):** every graceful signal is sent by the signaller, which holds p's uid and no capability, so the kernel's uid rule (A1) is the barrier and a recycled pid can reach only a uid-p process; the freeze only makes the walk complete; M_p holds no `CAP_KILL` and sends no signal (Linux); `PROC_REAP_KILL` on a reaper subtree and process descriptors (FreeBSD); handles (Windows). So a signal svd sends on G's behalf reaches a process of p or no process, never a process of O | cut |
  | H13 | **p's bytes relayed to owner clients** (REVIEW-8 N19, draft 12): `STATUS=` text, unit-reported reasons, log lines, change outcomes, as served through the snapshot and through `svd logs A` / `svd status` to an owner's terminal | not an input of any O cell: the core copies these bytes from fixed-size, length-checked snapshot fields or from p's lane, and never interprets them, so they do not touch obs_O. Toward the owner's **terminal** (a confused-deputy channel: escape sequences, forged lines that look like svd's own) they are cut at the client: the CLI writes every relayed byte outside printable UTF-8 (C0 and C1 controls, `ESC`, `DEL`, and the Unicode bidirectional overrides U+202A–U+202E, U+2066–U+2069) as `\xNN` or `\u{NNNN}`, and prefixes every relayed line with its provenance (`[p/<principal>:<service>]`), a prefix no line of svd's own carries; JSON output (`--json`) escapes by encoding. The API returns these fields in a JSON string marked `"untrusted": true`, so other clients can apply the same rule | confined (obs_O) + cut (terminal) |

  **H_OS-A4, robustness of exposed outside services (draft 10).** Every C ∈ O that a process of uid p can reach (a listener p may connect to, an IPC object p may open, a file C reads that p may write; the last is already A2-files) handles any input p can send without crashing, hanging or overrunning its own timers. svd knows only the exposure it can see: an edge (require, want or order) from grant(p) to C; C is then **exposed to p**.

  **[Compile+11] Refusal of exposed services whose failure escalates (user decision, 2026-11-23, refined by REVIEW-7 N10).** Let Esc be the set of services C ∈ O such that some terminal service T reaches C by a require path of length ≥ 0 (T itself included). Every C ∈ Esc that is exposed to some principal p is refused: `Compile` fails with `ErrExposedEscalates{C, p, T, path}`, naming one such T and its require path. *Why that set:* C reaching Failed(intensity) sends each require-ancestor to Failed(dep) through G_failed (§3.5), and a terminal reaching Failed escalates (above), so a single run of crashes of C within its own window escalates the instance exactly when C ∈ Esc. Draft 10's `terminal` flag claimed this of terminal C only, which was false (REVIEW-7 N10: `app` terminal requires `postgres`, which p's `reporting` requires; `postgres` crashed max + 1 times escalates). By closure (E2), every require path from a terminal T (T ∈ O by E1) stays in O, so Esc ⊆ O and is computed from the plan alone.

  **[Compile+11] `WarnExposedToGrant{C, p}`** remains for every exposed C ∉ Esc. The `terminal` flag is removed: after the refusal no reported C escalates by its own Failed, and the warning instead states the route every exposed C keeps: **C's crashes pay R_inst** (payer = owner), so instance.intensity.max + 1 crashes of exposed services within the instance window escalate the instance, whatever their terminal flags.

  **What Theorem G now says, plainly (REVIEW-7 N10 iii).** Delegation cannot escalate the instance **unless p can crash an outside service that p's processes can reach**. A4 carries that residual risk. svd sees, and refuses or names, only the part of it that has an edge: p reaches C's listener whether or not an edge exists, and since draft 10 a cut edge does nothing for C inside svd. So deleting an edge to silence `ErrExposedEscalates` removes svd's only view of the exposure and keeps the exposure; the refusal is a diagnostic of the plan, not a closure of A4. An attribution-free mitigation, a per-service `instance.charge = false` that would make C's crashes charge R_C only, is a user decision not taken in this draft (§12). A4 is a property of the owner's software, not of svd, and is tested by the site (fuzzing C's protocol), not by svd.

  Let obs_O(s) be, for every C ∈ O, (state, goal, cause, exit, wexp, armed, reason, |R_C(now)|), **the tree bit of C and the generation-match of each of C's pending timers** (REVIEW-6 Gate 1 gap 1: these decide whether a low TE or timer event is enabled), together with `mode`, |R_inst(now)| and the outcome of every owner transaction whose members lie in O (by §2.4 every transaction lies wholly in O or wholly in one grant). Epoch and generation values are names and are left out; only their match with pending events is observed. Under H_OS-E1…E5 and H_OS-A1…A4:
  1. **(non-interference)** obs_O after every step is a function of the low inputs alone: two runs with the same low inputs and **any** values of H1–H8 and H10–H13 (and of H9 within A4) have equal obs_O at every step, and the same low inputs are enabled in both. *Enabled* means accepted by the core; draft 11 restores this claim, which REVIEW-7 N8 showed false for draft 10 (a starved lane held owner connections in the core until `accept` was refused): by the lane protocol (H_OS-E5, H11) no core resource an owner request occupies depends on a lane. The **outcome** of an owner operation on a service of G (`ErrLaneBusy`, its transaction's result) may differ between the runs; it is G's, and outside obs_O;
  2. **(no escalation)** in particular, since `mode` ∈ obs_O, every run that reaches Quiescing(1) is matched step for step on O by the run with the same low inputs in which p issues no operation and G's trees emit no event. p has no lever on O. The cost of the cut falls on p alone: a granted dependent of an outside C may still be in its graceful stop when C is terminated, and T2's ordering holds towards dependents in O only (§3.7).

  *Proof.* By induction over steps of the single-queue model, which the runtime refines by the serialisation lemma (§3.1 step 1), on the pair of runs (r₁, r₂) of item 1; the hypothesis is obs_O(r₁[i]) = obs_O(r₂[i]), together with equal holders on O and equal R_inst. H7 enters only through A3, H8 only through A1, A2 and E5, H9 only through A4 (with Esc refused), H10 and H11 only through E5's subreaper and lane protocol, H12 only through E5's signalling rule, and H13 not at all (no O cell reads it), as the table states; every other input is used below as confined or cut.
  - **(a) Structure, from Compile and the load checks.** By grant closure under every edge kind (E2), no edge goes from O into G, so every path that starts in O stays in O. By the cut (§2.3), no goal propagates from G into O. By H_OS-E1 on resolved ids (F2b), svd runs a process as p iff it belongs to a tree of G; terminal services are in O; no service runs as a reader.
  - **(b) What a transition of C ∈ O reads.** Each item is equal in r₁ and r₂:
    - *its own events.* Process and channel events come from C's tree. No process of that tree has p's real, effective or saved uid, and the privileges svd gives G are none (H_OS-E1, F2a), so p cannot signal, trace or write it (H_OS-A1, testable form), and p holds no tree container (H_OS-E2). Outside svd, p reaches C's inputs through no named channel (H_OS-A2). Inside svd, p's API traffic, channel datagrams and logs are handled by p's own lane, never by the core that runs O (§3.1 step 1, H_OS-E5), and the core reads G only through the cut, so no O cell reads a G cell (serialisation lemma, asserted by Gate 1). p's orphans are reaped by p's monitor, never by the core's reaper, whose O sources are per-epoch (H10); the core never waits on p's lane or monitor (H11); and **svd's own deputies** cannot be steered onto C: no svd process signals a pid read from a list, so neither the core's signals for O nor M_p's for G can reach C through a reused pid (H12, signalling rule; draft 11's proof assumed that only A1 governs who signals C, and REVIEW-8 N16 showed svd's own root lane was not covered by it). p's load cannot delay C's events past one round, cannot reorder C's READY behind C's StartTO or WTO (O5, and O5′ for the sources of req(C)), and cannot cause an SEt of C, because C's spawn draws on a reserve no principal can consume and p's CPU, IO, swap and memory are capped (H_OS-E5 ∧ A2(host)). What remains of p's load is timing through shared hardware, bounded by Λ (H_OS-A3). So C's events are low inputs. Timer events of C are C's own. API operations on C by p are refused by the grant check (§8.2) before any effect; a reader writes nothing;
    - *the states of req(C) ∪ wnt(C) ∪ ord(C)*, all in O by (a), hence in obs_O, equal by hypothesis: the event and guard tables of C read only O (G_ok, G_failed, G_lost, G_wexp, G_arm, settled);
    - *goal(C)*: by the cut it is a function of the holders on O (§2.3). Those holders change only by low inputs: owner-class operations, the rollback of an owner transaction whose members lie in O (its success or failure reads only O states and a low deadline), and Quiescing (`mode` is in obs_O). p's `stop` and `stop --with-dependents` clear holders on G only, and abort only transactions whose members lie in G (§2.3, F3). So the holders on O are equal. *This item replaces draft 8's step (b), which wrongly said that p cannot remove holders of other principals: p can, on G, and that is harmless only because of the cut;*
    - *G_free(C) and SW(C)*, which read C's dependents. Draft 10 restricts both to **uncut** dependents (§3.5, §3.6): a dependent A with cut(A, C) is not read. A dependent A ∈ G of C ∈ O is always cut (A's goal never reaches C, §2.3), so every dependent read lies in O and is equal by hypothesis. *Draft 9 read all dependents and closed the gap with an oracle that equalised G_free by construction; REVIEW-6 N1 showed that the runtime had no such oracle (A ∈ G ignoring SIGTERM delayed C's Stopping, and with it R_inst). The oracle is withdrawn: no service of O reads any state of G;*
    - *R_C and R_inst.* R_C is appended only by charges of C. R_inst is appended only by charges of services whose payer is the owner, which by E1 are services of O, at transitions that are equal by the items above; and by owner Retries of services of O (§3.3, draft 9: an owner Retry of a granted service pays R_p), whose effect reads only the state of a service of O. So |R_inst(now)| is equal.
  - Hence C takes the same transition in r₁ and r₂, and obs_O stays equal. This proves item 1.
  - **(c) Item 2.** `mode` is in obs_O, so Quiescing(1) occurs in r₁ iff it occurs in r₂; take r₂ to be the run with no high input. For R_p: its overflow sends only the crashed service, in G, to Failed(intensity), and never changes `mode` (overflow(A) item 2; I5); an owner Retry's payment to R_p never changes `mode` either. p has no lever on O: every row of the inventory is confined, cut or assumed. The cut's cost is p's: an A ∈ G that still runs when C stops loses C (T2, §3.7, only towards dependents in O), and that loss is charged to R_p. ∎

  **What draft 8's proof missed, and where each gap is now closed.** F1: svd's own resources, now H_OS-E5 and O5, used in (b) "its own events". F2: privileges svd itself grants, and "=" not on ids, now §1.6 and the testable H_OS-A1. F3: owner holders crossing the boundary outward, now the cut, used in (a) and (b) "goal(C)". The owner-Retry payment, found by the 2-safety formulation, now pays payer(A). F4: the principal classes, now §1.6, so the theorem covers every non-owner-class principal, readers included.

  **What draft 9's proof missed (REVIEW-6), and where each gap is now closed.** N1: the G_free oracle had no runtime counterpart; withdrawn, G_free and SW read only uncut dependents, and the defect class is closed by the inventory H1–H9, every row confined, cut or assumed. N2: WTO could fire before the READY of its target's sources was drained; now O5′. N3: CPU, IO and swap were uncapped; now H_OS-E5 caps, A2(host) sums, and A3 for what caps cannot reach. N4: holders were unbounded; now one per (principal, service), §2.3.

  **What draft 10's runtime missed (REVIEW-7), and where each gap is now closed.** The single-queue model was sound; the inventory missed two inputs that reach the **core**. N7: G's orphans were reparented to the core, whose P_ALL reaper shared one source with O's exits; now H10, the lane is a subreaper and O's exits are observed per epoch (§5.1). N8: a starved lane could block the core's enqueue, hold owner connections and spin a snapshot reader; now H11, the lane protocol. N9: the lane's uid was unstated; now capped by the container on every OS that allows grants (draft 11's root-with-reduced-capabilities answer is withdrawn in draft 12, below). N10: the `terminal` flag named the wrong set; now `ErrExposedEscalates` on Esc. N11: the serialisation lemma covers the O projection only, which is all Theorem G needs.

  **What draft 11's runtime missed (REVIEW-8), and where each gap is now closed.** The model was not touched. N9's root lane was root-equivalent despite its reduced set (N14), could not create §7.1's mount namespace (N15), and could be steered by pid reuse onto O (N16); now the privilege split, in which the process that parses p's bytes holds no privilege and the process that holds privilege reads only 64-byte records, and the signalling rule, inventory row H12. The lane-death window counted a population as a count (N17); now the trees' parent is M_p, and at M_p's death the core freezes before it kills. The orphan source could reap a dead lane before its MainExit, and FreeBSD's reaper-of-the-reaper rule was missed (N18); now §5.1. The owner-relay channel (N19) is row H13.

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
- **Done × Restart** re-runs a oneshot, uncharged. Its "cause = restart" is an **assignment**, not `cause := restart if cause = none`: Done is reached by settle rule 3, which leaves no cause pending, and the next spawn resets it, so the two readings agree on every reachable world; the assignment is the one stated.
- **Readings fixed by draft 16 (PR #5).**
  - *A ⊘ cell* is a precondition of the pure step, not a behaviour: the step **refuses** the input (`ErrUnreachable`) and leaves the world unchanged. The runtime treats that refusal as an internal defect (log, counter, input dropped), never as a crash; the filters O1–O3 run first, so a stale event is dropped as stale before any cell is looked up.
  - *`perm` exists only on X1*, and only in cells whose record action can hold it (`record(fail)` with the status pipe in Starting and StopWait, `record(sig)` in Stopping, Killing, Leaked). Ready × X1 cannot carry `perm` (the helper's pipe is closed once the service has exec'd); a `perm` there is an input the step refuses.
  - *`permits(x)`* is defined for x ∈ {ok, fail}; for any other x it is false, so rule 6 never restarts a `sig` or `perm` exit (rule 2 has already taken `perm`).
  - *Fall* needs only a current generation and an armed liveness deadline; the tree bit is implied by Ready.
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
- **G_free(B)** ⟺ both of, over **uncut** dependents only (draft 10, REVIEW-6 N1):
  - no A with B ∈ req(A) ∧ ¬cut(A, B) is in {Starting, Ready, StopWait, Stopping, Killing, Leaked};
  - no A with B ∈ wnt(A) ∪ ord(A) ∧ ¬cut(A, B) is in {StopWait, Stopping, Killing, Leaked}.

  **Leaked blocks (N3)**, so a leaked uncut dependent is bounded by SWTO and reported. It is never silently passed over.
  - Uncut require-dependents that are still running block B. They are about to be pushed down by G_lost, so the result does not depend on evaluation order.
  - **A dependent across a cut edge is never read (N1).** cut(A, B) holds exactly when A ∈ grant(p) and B ∉ grant(p) (§2.3). Such an A does not hold B up (I4), and B does not wait for its graceful stop: when B stops, A loses B (G_lost, T2 towards A) and stops on its own clock, and the cost is p's (Theorem G item 2). Draft 9 read every dependent, so a granted A that ignored SIGTERM held an outside B in StopWait for up to SW(B), and could choose B's Stopping time; that is how p reached R_inst. **[Compile+10]** The evaluator's dependent index for G_free is built from uncut edges only; Gate 1 is red on the mutant that reads a cut dependent (§3.8).

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
  - All guards except G_free read only A's targets. G_free reads A's uncut dependents, an upward read. It is harmless because **the move it enables is invisible to every guard that reads A** (REVIEW-6, the draft-9 justification « one-way, at most once per step » was true but not sufficient): StopWait and Stopping are indistinguishable to them, since neither is in {Ready, Done, Failed, Leaked} (so G_ok, G_failed and G_lost of a dependent read the same value), neither is settled while the goal is up, and both block G_free of A's own targets (§3.5, G_free lists both). So StopWait → Stopping changes no other service's input, and the induction from the sinks upward survives the upward read.
  - By induction in topological order, from sinks up: a sink's inputs are fixed, so it stabilises after ≤ 2 moves. If every target of A changes state finitely often, A's inputs change finitely often, and A moves at most 2 times per input change.
  - So propagation terminates.
- **No closed-form bound is claimed (R3-L8).** The induction gives "at most 2 moves per input change", and input changes compound along the depth of the DAG, so it does not yield a linear bound such as 4N. Gate 1 records the **measured** maximum number of moves per step over its whole scope, writes it into a golden file, and asserts it. A change that raises it must update the golden file in review.

**Evaluation order (draft 16; replaces draft 4–15's "confluence across evaluation orders is asserted by the BFS").** That claim is **false**, and in both directions, so no single order of services fixes it:

- *Dependent before target.* A dependent reads a target's state before the target has made its move (PR #5, `TestArbitraryOrderIsNotConfluent`: a Waiting S2 spawns on a transient Ready of S1 that the same step stops).
- *Target before dependent.* G_free is the one upward read. If a target B in StopWait is evaluated before its uncut `want`/`order` dependent A, G_free(B) still sees A in Ready (Ready blocks G_free only across a `require` edge) and B is signalled at once; evaluated after A, B sees A in StopWait and waits. Measured on PR #5 (head d74ceab): plan {B, A `order` B} (same with `want`), both Ready, owner `stop` of both: the targets-first order moves B Ready→StopWait→Stopping and A Ready→StopWait→Stopping, both signalled in one step; the order (A, B) leaves B in StopWait. The second outcome is the intended one: it is what SW(B) ≥ stop(A) + kill(A) in §3.6 (REVIEW-2's `A order B` case) is sized for, and the first makes that bound vacuous for every `want`/`order` edge. The termination argument above is not affected: it shows that StopWait → Stopping is invisible to other services, not that A → StopWait is invisible to G_free(B), which is false.

**The rule.** Propagation runs in **rounds** (§2.3); a round is two phases:

1. **Phase 1, every guard except G_free**, service by service in a **topological order with targets first** (over require, want and order edges), each service stabilised before the next. Each of these guards reads only the service itself, its targets and the round's fixed goals (`GuardReads`), so when a service is evaluated its inputs are final: one pass reaches the fixpoint, and **every** targets-first topological order gives the same world (induction over the order; the same induction as the termination argument).
2. **Phase 2, G_free once:** let F be the set of services in StopWait whose G_free holds in the world phase 1 ended in; every service of F moves StopWait → Stopping (term). Phase 2 is order-independent: a move of phase 2 can only put a service in Stopping, which blocks G_free of its own targets as StopWait already did, so it changes no member of F; and it changes no input of a phase-1 guard (the invisibility argument above), so phase 1 stays at its fixpoint and the round ends.

A dependent's move to StopWait is therefore always visible to its target's G_free in the same round, and a target is signalled only when none of its uncut dependents is in a blocking state of §3.5's G_free row (for `want`/`order`: StopWait, Stopping, Killing, Leaked), as T2 states; a dependent that starts stopping in the same step blocks it. **Claim, replacing confluence:** any evaluation order that respects the two phases and is targets-first inside phase 1 gives the same world; arbitrary orders do not, and are not claimed. Gate 1 checks the claim by enumerating, for N ≤ 4, every targets-first topological order and every order of F, and asserts that each service moves at most 2 times per round (the bound above, per phase-1 sweep plus the single phase-2 move).

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

- SW(B) = max over **uncut dependents** A of (SW(A) + stop(A) + kill(A)) + ε, with SW = ε for a service without uncut dependents (N3; restricted in draft 10, N1). **[Compile+10]**
  - "Uncut dependents" means every A with B ∈ req(A) ∪ wnt(A) ∪ ord(A) and ¬cut(A, B), matching G_free. Draft 9 said "all dependents", which made SW(B) of an outside B a function of a granted A's stop and kill timeouts, fields p's application may set.
  - The REVIEW-2 counter-example (`A order B`, where B was terminated after ε while A drained) now waits SW(B) ≥ stop(A) + kill(A) whenever the edge is uncut. Across a cut edge it does not wait, by design: A ∈ grant(p) drains on its own clock after B has gone, which is p's cost.
- The DAG is finite, so SW is finite.
- **If it fires**, B is terminated anyway, and the overrun is reported as a T2 violation. Only two cases can cause that:
  - an uncut dependent is Leaked. G_free blocks on Leaked, so this case is always reported;
  - an uncut want- or order-dependent began stopping after B had entered StopWait.
  A cut dependent never causes an overrun of B, because neither G_free(B) nor SW(B) reads it.

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
- **T2:** B is signalled (term) only when G_free(B) holds, or when StopWaitTO fires and the overrun is reported. Escalation and rollback stop services through this path (H1). Since draft 10 (N1), the ordering T2 promises is **towards uncut dependents only**: B waits for every dependent A with ¬cut(A, B), and for none across a cut edge, so a granted A may still be running when an outside B is signalled; A then loses B through G_lost, and the consequence is charged to p.

**Progress, under weak fairness of timers and reports:**

- **P1:** goal = down ⇒ ◇ state ∈ {Inactive, Failed, Leaked}.
- **P2 is split in draft 9 (F5).** Draft 8's P2 concluded ◇{Ready, Done, Failed} from charges alone, but in abstract time a window can drain between two charges, so a service that crashes forever at a rate below max/window never reaches Failed(intensity), and it omitted Leaked and operator restarts. Draft 9 states two properties whose conjunction gives P2 in real time.
  - **P2a (Gate 1, abstract time):** ◇□(goal(A) = up ∧ ∀B ∈ req(A): state(B) ∈ {Ready, Done}) ∧ **finitely many SEt for A** ∧ **finitely many operator Restart of A** ⇒ ◇ state(A) ∈ {Ready, Done, Failed, Leaked} ∨ □◇ Charge(A).
    - The SEt conjunct is an environment hypothesis, stated (E5, §3.1): a pre-fork transient error that recurs forever is uncharged by design. What holds without it is **P2-rate**: each such cycle lasts at least backoff.max, and every SEt is reported.
    - Gate 1 checks P2a by SCC search with SEt and Restart as fairness constraints: every fair SCC reachable under the hypothesis either contains a stable state of the conclusion or a Charge(A) transition.
  - **P2b [Compile+9]:** `Compile` refuses `ErrNeverFailingLoop{service, window, need}` when intensity.window(A) ≤ intensity.max(A) × V(A), with `need` = max × V(A) + 1 s. V(A) is defined here: the longest time between two consecutive charges of A that never reaches Ready, start.timeout + stop.timeout + kill.timeout + backoff.max (all post-meet values). *Renamed from U(A) in draft 10 (REVIEW-6 N5): U(B) is the one-attempt time of §1.6, which also counts W and SW.* Then max + 1 consecutive charges span at most max × V(A) < window, so they all lie in one window, and □◇Charge(A) without Ready reaches the overflow and Failed(intensity). Charges after a Ready already satisfy P2's conclusion.
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
      - every **low** input (owner and reader operations, events of services in O, abstract time ticks, and the request deadline of an **owner** transaction, defined in §2.4) is applied to **both** copies; every **high** input (operations of p, events of services in grant(p), and the request deadline of a transaction of p, §2.4) is applied to **one** copy at a time, chosen by the BFS (REVIEW-6 gap 3: draft 9 cited "the request deadline of §3.1" and made every deadline low);
      - **enabledness (gap 1).** A low input enabled in one copy and not in the other is a **red** pair, never a skipped one. obs_O includes, since draft 10, C's tree bit and the generation-match of each pending timer of C (§3.3), which decide whether TE and O's timers are enabled; the check also asserts equal enabledness directly, so an omission from obs_O cannot hide a divergence;
      - **no oracle (gap 4, N1).** Draft 9's low input g_C is removed: G_free and SW read only uncut dependents (§3.5, §3.6), so no O cell has a G-dependent input to model;
      - the assertion: obs_O(s₁) = obs_O(s₂) at every reachable pair;
      - scope: N ≤ 3, because the product squares the state space; the one-copy properties keep N ≤ 4. **Symmetry reduction preserves the partition (gap 2):** a permutation maps G to G and O to O, holders of p to holders of p and owner holders to owner holders, and is applied **jointly** to both copies of a pair, never to one copy alone;
      - **static serialisation check (§3.1 step 1).** For every cell of the event and guard tables and every settle entry, the set of services it reads is computed from the data; the check asserts that no cell of a service in O reads a service in G, and that each timer cell's read set is contained in the set O5′ drains before it. This is the lemma the runtime's partitioned execution relies on;
      - **required witnesses**, each a named fixture: the check is **red** on draft 8's goal formula (fixture `goalDraft8`, holder scope without the cut, red through F3 route 1), **green** on draft 10's, **red** on the mutant that drops `ErrGrantNotClosed` for want edges, **red** on the Retry cell mutant `pay(caller)` (the owner-Retry finding of §3.3), and **red** on the mutant `freeAcrossCut`, in which G_free reads a dependent across a cut edge. *Fixture (corrected in draft 11, REVIEW-7 N12):* A ∈ grant(p) requires C ∈ O, C requires B ∈ O, C's goal stays **up** (owner holder), and |R_inst| = instance.intensity.max − 1. B crashes (low), so C goes to StopWait(dep) and is signalled; A's TreeEmpty is delayed in one copy only (high). Under the mutant, C stays in StopWait in that copy while it reaches Stopping in the other, so the pair is already red on state(C); then C's main process exits with `fail` (low, both copies): in the StopWait copy settle rule 4 charges it to R_inst, which overflows, so `mode` becomes Quiescing(1) in one copy only. Draft 10's fixture let C's goal go down, so settle rule 1 sent C to Inactive uncharged and the R_inst divergence it described never happened. **`swAcrossCut` is not a Gate 1 witness (REVIEW-7 N13, draft 11):** SW is a duration, Gate 1 works in abstract time where SWTO may fire at any step in both copies, so no value of SW enters the model and the mutant cannot turn the check red. It is a `Compile` unit test instead (§10), red on the mutant that computes SW over cut dependents;
      - **stated limit (gap 5):** E5 (svd's own resources, the lanes, the subreaper and the lane protocol), A1 (OS privilege) and A3 (shared-hardware timing) are outside the model. The F1 chaos test of §10, extended in draft 10 with CPU, IO, log and channel floods and a READY near a WaitTO deadline and in draft 11 with a fork-and-exit flood and a starved lane, and the A1 probes cover them;
    - **D1:** a oneshot whose exit is `sig` never reaches Done;
    - propagation termination, with the measured maximum number of moves recorded in a golden file and asserted (no closed-form bound, R3-L8), and **order independence within the stratified order of §3.5** (draft 16; "confluence across all orders" is false and withdrawn), with the two witnesses of §3.5 kept as red controls: an arbitrary order disagrees, and the unstratified targets-first order signals a `want`/`order` target in the same step as its dependent;
    - the guard evaluator never reads an `Absent` cell, and each service moves at most 2 times per propagation with fixed inputs (§3.5);
    - SCC search for P1, P2a and P3 under fairness, with P2a's SEt and operator-Restart hypotheses as fairness constraints (E5, F5). P2b is a `Compile` check, tested in §10.
  - **Finite state, stated (draft 16, PR #5).** "Exhaustive" means the search closes, so the model state must be finite; a search capped by a node count or by a horizon on `Now` is a **smoke test**, may be kept as one, and is never reported as Gate 1. PR #5's search (3000 pair-nodes, Now ≤ 2, Window 3, two hand-picked plans, current-generation inputs only) is such a smoke test: with Now ≤ 2 < Window no restart-window entry ever ages out, so the N1 item above is not exercised at all. The abstraction that makes the space finite, each step an over-approximation (so it can add red pairs, never hide one):
    - **no absolute time in the state.** Timers already fire nondeterministically (abstract time, REVIEW-7 N13); the clock is read only by the windows and by request deadlines, and both become inputs: a request deadline is the `deadline` input of its transaction (low for the owner's, high for p's), and a window is a **count** capped at max + 1 with an input `age(W)` that drops its oldest entry. Every interleaving of `age` and new entries is realised by some real timing, and conversely, so counts with `age` are exact for the FIFO windows of §3.2. `age` on R_inst and on R_A for A ∈ O is low; on R_p and on R_A for A ∈ grant(p) it is high (applied to one copy at a time), since those entries are added at times p chooses;
    - **generations and epochs as match bits.** A pending timer or event is "current" or "stale (earlier generation)", one bit per kind, not a counter; obs_O already projects them so (§3.3). The stale variants of the items above are inputs, not a counter that grows;
    - **holders as bits, transactions per holder** (§2.3, §2.4), with the deadline as an input, not a time;
    - **symmetry** as stated above, jointly on both copies.

    With N ≤ 3 in the product and N ≤ 4 in one copy, every component is then bounded (11 states × the bits × counts ≤ max + 1 per window), and the search must close. If it does not close in the CI budget, the scope is **reduced and stated** (smaller budgets, fewer plans), never capped silently; the plan scope "every labelled DAG" (with every edge kind and every grant assignment allowed by closure) is part of the gate, and two fixtures are not it.
- **Gate 2, mutation.**
  - Every Reachable cell is mutated: every other Target, and every one-action change. Each mutation must turn Gate 1 red.
  - An equivalent mutation, with the same Target and Actions, is not a mutation.
  - **Priority mutations (F6).** In every guard row with two or more present cells, every swap of two `Prio` values is a mutation and must turn Gate 1 red, or be listed as equivalent with the reason (two guards that are mutually exclusive in every reachable state).
  - **Label-only mutations are exempt (N13).** These are mutations that change only `reason` (e.g. Failed(wait) ↔ Failed(dep), which recover identically) or only the `Report` action. They violate no behavioural property. The list is asserted, and a unit test per cell checks the reason and the report.
  - **Unreachable cells are exempt, and the list of exemptions is itself asserted.** The BFS must never visit a cell marked Unreachable.
- **Gate 3:** a TLA+ module generated from the same data, *including propagation, settle and holders*, checked by TLC on a schedule for N ≤ 5 (Lamport, `tla/high-level-view.html`).

**Stated limits.**

- **Small scope.** N ≤ 4 or 5 is a hypothesis, not a proof for every N.
- **Real durations** are not modelled. The duration-dependent obligations (`ErrWaitTooShort`, StopWaitTO, and since draft 11 SW over uncut dependents, `swAcrossCut`) are checked by `Compile` and its tests.
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
   - ~~The socket is an AF_UNIX datagram socket in a per-epoch directory: 0700, owned by the service's run_as uid.~~ **Withdrawn in draft 13 (SECURITY-1 S2, HIGH):** a directory owned by the service's uid is one that service can fill with links and directories, and that root then deleted and walked at epoch end: a root-privileged unlink and traversal inside a directory the grantee controls. **Draft 13 [Host+13]:** the socket is an AF_UNIX datagram socket named `<service>.<epoch>` in a **per-principal notify directory** (the core's own for O), owned by root, mode 0711, inside the instance runtime directory (§8.1), whose every ancestor is root-owned and not group- or other-writable (else `ErrUnsafePath`). The binding process (M_p for grant(p), the core for O) holds an fd of that directory and works only relative to it: **how it binds relative to an fd (draft 14, SECURITY-1 T10) [Host+14]:** Linux has no `bindat`, and `bind(2)` takes a path; the binder passes `sun_path` = `/proc/self/fd/<dirfd>/<service>.<epoch>`, so the kernel resolves the directory through the fd M_p holds, not through a path p could race. That this resolution follows the fd's directory even if the directory were renamed is *from memory* (test `TestNotifyBindViaProcFd`, P1: rename the directory between open and bind, assert the socket lands in it). `sun_path` holds 108 bytes, so the name is bounded: **[Compile+14]** `ErrNotifyNameTooLong{service}` when `/proc/self/fd/` (14) + 10 fd digits + `/` + the service name + `.` + 20 epoch digits exceeds 107, i.e. a service name over 61 bytes in a grant; **[Host+14]** the absolute `NOTIFY_SOCKET` path given to the service (runtime directory + name) must also fit in 107 bytes, else `ErrUnsafePath` at load. It binds with umask 0177, `fchownat(dirfd, name, uid, gid, AT_SYMLINK_NOFOLLOW)` to the service's run_as, then `fstatat(dirfd, name, AT_SYMLINK_NOFOLLOW)` and verifies socket type, owner and mode 0600, else the spawn is `perm`. At epoch end it removes the socket with **one `unlinkat(dirfd, name, 0)`**; it never walks, follows or deletes recursively anything in a directory p can write, and p can write nothing in this one (0711: search only). **Space:** the directory holds at most one socket inode per live epoch per service, so at most \|grant(p)\| entries plus those of epochs ending in the current round; p creates no entry there. p's other writable places on `/run` (its own `$XDG_RUNTIME_DIR`, `/dev/shm`) are p's own quota under A2(host), not svd's (test `TestNotifyDirNotOwnedByGrantee`, P1, with a chaos run in which the service plants symlinks, hard links and directories under every name it can reach and asserts no root operation touches them).
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

- One reaper per process (core, lane) routes exits to epochs. It observes with `waitid(…, WNOWAIT)` (or pidfds), lets O1 and O4 drain the channel and the status pipe, and only then reaps (R3-L2). A plain `wait4(-1)` would reap before the drain.
- **Per-epoch observation in the core (REVIEW-7 N7, draft 11).** The core observes the main process of each O epoch through its pidfd (Linux 5.3+) or `waitid(P_PID, pid, WEXITED | WNOWAIT)`, never through a `waitid(P_ALL, …)` that other processes can fill. Each epoch is its own source in the round-robin of §3.1.
- **Orphans.** The core reaps orphans through an **orphan source** of its own in the round-robin, which reaps at most `reap.batch` (class T) orphans per round: it peeks with `waitid(P_ALL, …, WEXITED | WNOWAIT | WNOHANG)` and reaps the reported pid only if it is **not a child the core tracks**; if it is, the orphan source yields for this round, since that child is reaped by its own source after its handling, within the round. **The tracked set (REVIEW-8 N18 a, draft 12)** is every child the core forked and has not yet reaped: the main of every live O epoch, every lane L_p and every monitor M_p. Draft 11 excluded only epoch mains, so the orphan source could reap a dead lane before the core handled the lane's MainExit, on which the lane protocol relies. Within a round, monitor-death and lane-death sources are drained before the orphan source (§3.3, a monitor's death).
- **When the orphan source runs (REVIEW-8 N18 b, draft 12) [Host+12].** Whenever the core is a reaper of any kind, not only as PID 1: in mode C (PID 1); in mode B, where the core now makes itself a subreaper (`PR_SET_CHILD_SUBREAPER`) or reaper (`PROC_REAP_ACQUIRE`, before its first fork, since procctl(2) applies it to future children only) at start; and in mode U on FreeBSD, where §5.3 already makes it a reaper. procctl(2) (FreeBSD 15.1, opened by REVIEW-8) states that at a reaper's exit its descendants are « reassigned to the reaper process's reaper »: draft 11's « or to the host init » was wrong on FreeBSD, and those zombies, never reaped, counted against p's `maxproc` across lane restarts. Every monitor is a subreaper (§3.3 H_OS-E5), so the orphans of granted trees go to their monitor, not to the core; the core's orphan source receives O's orphans, and p's processes only at the death of p's monitor (at most `budget.pids` + `reap.batch` on Linux, after the freeze; §3.3). Draft 10 reaped every orphan in the core through P_ALL, so p could fork-and-exit at a rate it chose and charge the reaping to the core (REVIEW-7 N7).

**Tree tracking:**

| OS | mechanism |
|---|---|
| Linux | a cgroup per epoch |
| Windows | a job object with `KILL_ON_JOB_CLOSE`, assigned at creation |
| FreeBSD | one reaper per **process that spawns** (`PROC_REAP_ACQUIRE`: the core for O, M_p for grant(p)), not one per tree; an epoch's tree is the reaper's **subtree** rooted at that epoch's main, a direct child of the reaper; tree emptiness from `PROC_REAP_GETPIDS` filtered by subtree; kill by `PROC_REAP_KILL` with `REAPER_KILL_SUBTREE` (draft 12, REVIEW-8 N20 b: draft 11's row was written as if each tree had its own reaper) *(the subtree field and its persistence across reparenting are from memory, tests `TestReapKillSubtreeAfterOrphan` and `TestReapKillSubtreeAfterReaperDeath`, P4)* |
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
| Linux | mount ns → oom reset → mounts by fd → credentials → **exec of the single-threaded drop stage** → NNP → Landlock → rlimits, umask, cwd, signal state → groups, gid → caps and bounding set → uid → verify → seccomp → `execve`, last; **the authoritative numbered sequence, with the privilege at each step, is §7.1.1 (draft 14; draft 13's single-helper order is superseded)**. *Draft 14:* « makes every mount `MS_PRIVATE` » below is superseded by §7.1.1 step H5 (`MS_SLAVE`, S11). **Who creates the mount namespace (REVIEW-8 N15, draft 12):** `unshare(2)` (man-pages 6.19, opened by REVIEW-8): « Use of CLONE_NEWNS requires the CAP_SYS_ADMIN capability ». The helper is forked with `CLONE_NEWNS` by a process that holds it: the core for O's services, the root monitor M_p for grant(p)'s (§3.3 H_OS-E5), never the unprivileged lane; the helper starts inside the new namespace, makes every mount `MS_PRIVATE` recursively before any remount, applies K6 `read_only`, then continues the order. That `clone3` with `CLONE_NEWNS` has the same requirement as `unshare` is *from memory* (test `TestMonitorSpawnReadOnly`, P1). An `EPERM` from `clone3` is SEp (§3.1 O3, no child exists) and an `EPERM` from a remount in the helper is `perm` (O4); neither is `fail`, so a host that refuses them fails the service at once, uncharged, instead of crash-looping on its payer's window |
| FreeBSD | jail → NNP → ids |
| Windows | `CreateProcessAsUserW` attributes |
| macOS | ids → rlimits → umask; seatbelt is opt-in |

The plan arrives on an inherited fd.

**Refusal rule.** Refuse to start when the protection is ✓ in the plan, the host OS ∉ that protection's `exempt_on`, and the mechanism is unavailable.

#### 7.1.1 The spawn path, Linux (draft 15, SECURITY-1 Parts 1, 6 and 7) [Host+13, Host+14, Host+15]

The one audited path from a privileged svd process to a granted service's `execve`. **R** = uid 0 with C_M (§3.3 H_OS-E5: `CAP_SYS_ADMIN`, `CAP_SETUID`, `CAP_SETGID`, `CAP_SETPCAP`, `CAP_SYS_RESOURCE`) and `oom_score_adj` −1000 with floor −1000 inherited from M_p; for O's services the spawner is the core, with its own set, and the steps are the same. It reads only the core's plan entry, never a byte of L_p's.

**Why draft 13's order was replaced (SECURITY-1 T2, S3, S5).** Draft 13 ran every step in the Go helper « on one locked OS thread ». That is not a drop: capabilities are « a per-thread attribute » (capabilities(7), man-pages 6.19), at the kernel level the ids are per-thread too (setresuid(2), man-pages 6.19: the per-process semantics are the C library's propagation), `commit_creds` changes the creds of `current` only (`kernel/cred.c`, torvalds/linux master), and ptrace's access check reads the target **task's** creds (`__ptrace_may_access`, `kernel/ptrace.c`). So a Go helper that drops on one thread keeps root threads beside a uid-p thread, and a uid-p process could reach the dropped thread while its siblings still hold C_M. Go cannot be made single-threaded after `execve`: `runtime.main` starts `sysmon` on its own M (`src/runtime/proc.go:179`, Go 1.27.4) before the program's `main` runs (`proc.go:301`). `syscall.AllThreadsSyscall` is rejected too: during its sweep, threads with old and new credentials coexist. **Draft 14's rule [Host+14]: every per-thread attribute is set in a process that has exactly one thread.** The path is four images of **one pid**, so `clone3`'s pidfd names the main throughout:

1. **M_p** (Go, R): steps M1–M4.
2. **Go's fork child** (`syscall.ForkExec`'s raw path, one thread, no allocation): step C1.
3. **`svd helper`** (Go, multi-threaded, R throughout, **never changes an id, a capability or a per-thread attribute**): only process-wide and namespace-wide work, steps H1–H10, ending in an `execveat` of the drop stage. `execve` destroys « all threads other than the calling thread » (execve(2), man-pages 6.19).
4. **`svd-drop`** (one thread from its `execve` to the next): steps D1–D14. It reads one sealed, fixed-layout frame and performs a fixed list of system calls; it parses nothing else. **Draft 15 (SECURITY-1 T13, reproduced in `evidence-part7/`, linux/arm64 under ptrace, amd64 under emulation):** draft 14's sentence « the Go runtime is linked but never initialized … its code contains no `clone` » was true only of what the entry reaches: the linked file holds 1065 `runtime.*` functions, `runtime.newosproc` among them, because the linker's dead-code pass roots at the entry symbol **and** at `runtime.unreachableMethod` (`cmd/link/internal/ld/deadcode.go:80-84`, Go 1.27.4). The stage is therefore specified by **what is reachable from its entry in the shipped artifact**, and gated on that:
   - **Kept as a Go-toolchain binary (decision of this draft).** The alternatives were weighed. *Hand-written assembly for the whole D-list* is rejected for the reason SECURITY-1 gives: some hundreds of lines per architecture in the most sensitive place of the TCB. *A freestanding C binary* removes none of the hazards below, it only moves them: a C compiler emits implicit `memcpy`/`memset` calls, the stack protector reads its canary through `%fs:0x28` on x86-64 (the same FS hazard as Go's `FS:-8`), and a `-static` libc runs its start code (TLS setup, `IRELATIVE` relocations) as root before `main` *(from memory)*; so it needs the same two gates, and it adds a second compiler, a cross-compiler per architecture and a second statement of the frame layout to a `CGO_ENABLED=0` build. In Go the frame is **one type** (`internal/frame`) that the helper writes and the stage reads, which is also T17's version tie. **The cost, stated:** the stage depends on behaviour the Go toolchain does not promise (an `-E` entry with an uninitialized runtime, the ABIInternal `g` register: R14 on amd64, R28 on arm64; the wrapper's `XORPS X15, X15`, seen in `out/run.txt`). A toolchain upgrade can break it; Gates 1 and 2 turn that into a red CI on the upgrade's pull request, never into a silent loss of T2. If a toolchain cannot be made to pass them, the fallback is a freestanding C stage under the **same** gates (user decision, §12).
   - **Entry, per architecture** (about ten instructions of assembly, `entry_amd64.s`, `entry_arm64.s`): (1) D1, `prctl(PR_SET_DUMPABLE, 0)`, issued from assembly, so it stays **the first system call on both architectures**; (2) install a static `g` (a zeroed package-level block, so `stackguard0` = 0): arm64 `MOVD $·fakeg(SB), g`; amd64 `arch_prctl(ARCH_SET_FS, &tls[1])` with `tls[0] = &fakeg`, the **second** system call on amd64 only (the order of `evidence-part7/stage/entry_amd64.s`, `dropEntryG`; SECURITY-1's « `arch_prctl` first » assumed the opposite order); (3) `CALL`/`BL` the Go function `stage` through its ABI0 wrapper; (4) `exit_group(fixed code)`, then `INT $3` / `BRK $0`. Without step (2), amd64 dies at the wrapper's `MOVQ FS:0xfffffff8, R14` and arm64 at the first stack check `MOVD 16(R28), R16` (both reproduced).
   - **The Go subset.** Every stage function is `//go:nosplit`, so no prologue calls `runtime.morestack*` and the linker's nosplit stack check bounds the depth *(from memory: the link fails with « nosplit stack overflow »; red control (f) below decides)*; no allocation, `go`, `defer`, `recover`, map, interface value, closure, string concatenation, `switch` dense enough to become a jump table, recursion, or index the compiler cannot prove (explicit length checks instead); buffers are package-level arrays; every system call is its own assembly stub with an **immediate** number (generated from one table by a Go program, both architectures). These rules are **not** enforced by review: Gate 1 refuses the artifact whenever one of them is broken. CPU-feature variables (`internal/cpu`, `runtime.useAVXmemmove`, …) are never initialized and read as zero, so `memmove` and `memclrNoHeapPointers` take their generic paths, which is correct.
   - **`main.main`** exists only for a build without `-E` (under `-E` the linker drops it, `out/run.txt`): it writes « svd-drop: built without -E » to fd 2 and exits with its own fixed code. The frame starts with a magic and the frame-layout digest of `internal/frame`; D2 compares that digest with the one compiled into the stage (T17).
   - **Build [Host+15].** Linux **amd64 and arm64 only**; on any other `GOARCH` svd's Linux build fails (a build-constrained file), until an entry exists for it. Built only by `cmd/buildstage` (a Go program in go-supervise/supervisor), which runs the pinned toolchain with `CGO_ENABLED=0`, `GOFLAGS` **emptied** (so a distribution's `-buildmode=pie` default cannot apply), `-buildmode=exe`, `-ldflags='-linkmode=internal -E main.dropEntry'`, `-trimpath`, `-buildvcs=false`, `GOAMD64=v1`, `GOARM64=v8.0`, never `-race`, `-cover`, `-msan`, `-asan`; the symbol table is kept (Gate 1 reads it). Reproducible: two builds in different directories and caches are byte-identical, and the release records the SHA-256. `go install` cannot carry `-E`, so the release installs the stage, never `go install`; and at load the core checks the installed stage's ELF type, program headers and entry symbol (`debug/elf`, no execution): anything but Gate 1's shape is `ErrIsolationUnavailable{dropstage, linux}`.
   - **Exit classes.** D2–D11 write `perm` on the status pipe before their fixed exit code; D13–D14 report by exit code only (below). A fault (`SIGSEGV`, `SIGBUS`, `SIGILL`, or the entry's trailing `INT $3`/`BRK`, e.g. a nil dereference, which dies with the kernel's default action because no handler is ever installed) leaves EOF with no status byte: an unclassified post-fork exit, `fail` under E11, charged and bounded; in CI any such death is a test failure. Coverage instrumentation runs but never writes its counters, so the stage is tested by **mutation** with Gate 2 as the oracle, not by a coverage figure.
   - **Gate 1, static** (`cmd/syscallsurface stage <artifact>`, on the shipped file, both architectures): ELF `ET_EXEC`; program headers ⊆ {`PT_PHDR`, `PT_NOTE`, `PT_LOAD`, `PT_GNU_STACK`} (no `PT_INTERP`, `PT_DYNAMIC`, `PT_TLS`), stack not executable; the entry address is `main.dropEntry.abi0`; disassembling from the entry (`golang.org/x/arch`), only direct calls and branches are reached (an indirect call or branch, a jump table included, is red); the call graph is acyclic; the reached functions are the stage package's and **exactly the allowlist {`runtime.memmove`, `runtime.memclrNoHeapPointers`}** (any other `runtime.*` reached, `morestack*`, `panicIndex`, `mallocgc`, `gcWriteBarrier*` included, is red); every `SYSCALL`/`SVC` site reached loads its number from an immediate, and the set of numbers equals the committed D-list set for that architecture (amd64 adds `arch_prctl`).
   - **Gate 2, dynamic** (`TestDropStageSingleThread`, P1, root, run **natively** on amd64 and arm64, every mode and fixture frame): the shipped artifact runs under ptrace from its `execve` with `PTRACE_O_TRACECLONE | PTRACE_O_TRACESYSGOOD | PTRACE_O_EXITKILL`, and the test **fails** if: any `PTRACE_EVENT_CLONE` stop occurs (a thread appeared); `/proc/<pid>/task` has more than one entry at any stop; the first system call is not `prctl(PR_SET_DUMPABLE, 0)`; the **sequence** of numbers differs from the committed sequence for (architecture, mode, fixture), so a runtime that initializes fails at its first call (the arm64 control issued 163 calls, starting `sched_getaffinity`, `openat`); any call follows D13's `seccomp` other than D14's `execve`/`execveat` and `exit_group`. `Threads: 1` alone (draft 14) is not enough: it does not see a runtime that initialized and has not yet started a thread.
   - **Red controls, part of the gate** (`TestDropStageGatesCatch`, same CI job; each must turn the named gate red, and the job fails if one stays green): (a) the same package linked without `-E` (Gates 1 and 2: the runtime starts, 2 tasks in the control); (b) `-buildmode=pie` (Gate 1: `ET_DYN`, `PT_INTERP`; Gate 2: 8 loader calls as root before D1, measured); (c) build tag `stagecanary_index`, an index the compiler cannot prove (Gate 1: `runtime.panicIndex` reached); (d) the amd64 entry without `arch_prctl` (Gate 2: `SIGSEGV`); (e) a planted `clone` stub called once (Gate 1: number not in the list; Gate 2: clone stop); (f) a nosplit chain deeper than the linker's limit (the build fails). Controls (a) and (b) are the shapes already measured in `evidence-part7/out/`.

Each step that fails exits with a fixed code that M_p maps to SEp (steps M1–M4, after the rollback of T5 below) or `perm` (every later step), except where the O4 table of §3.1 says `fail` (`execve` `EAGAIN`). **Draft-13 step numbers used elsewhere map as:** 1–4 → M1–M4; 5 → C1; 6–10 → H1–H6; 11–12 → D3–D4; 13–17 → D6–D10; 18–19 → D12–D14.

| # | process (threads) | action | privilege after the step | rollback / failure |
|---|---|---|---|---|
| M1 | M_p (n) | decode the `SPAWN` record (pure, §3.3, check order of T8); look up the plan entry of that service | R | Refuse/Fault only, nothing created |
| M2 | M_p (n) | `mkdirat` the epoch cgroup on the `trees/` dirfd; open it; write its limits relative to that fd; `cgroup.subtree_control` stays empty (a leaf) | R | SEp; `unlinkat(trees_fd, epoch, AT_REMOVEDIR)` |
| M3 | M_p (n) | `pipe2(O_CLOEXEC)` the log, status and readiness pipes; bind the notify socket (§4.1, S2, T10); **draft 15 (T14):** `/dev/null` is no longer opened here: M_p receives one `/dev/null` fd from the core at its start, with its other start-up fds, and passes it as fd 0; **draft 15 (T18):** the plan entry goes into a `memfd_create(MFD_CLOEXEC \| MFD_ALLOW_SEALING \| MFD_NOEXEC_SEAL)`, written in full, then sealed with the four seals of H9, so M_p never writes to a reader that does not exist yet. **Every fd M_p owns is `O_CLOEXEC` [Host+14]** | R | SEp; close every fd of M3, `unlinkat(notify_dirfd, name, 0)`, then M2's rollback |
| M4 | M_p (n) | `syscall.ForkExec` with `SysProcAttr{Setsid: true, Cloneflags: CLONE_NEWNS, UseCgroupFD: true, CgroupFD, PidFD}`; Go then issues `clone3` with `CLONE_INTO_CGROUP` and `CLONE_PIDFD` (`src/syscall/exec_linux.go:315-334`, Go 1.27.4) and adds `CLONE_VFORK \| CLONE_VM` (`:307-309`), harmless here because parent and child are both R; `argv[0]` = `/proc/self/fd/k`, because Go's child issues only `SYS_EXECVE` (`:673`); `Files` = the helper set H below, **whose last entry, index k, is the helper's `O_PATH` fd (draft 15, T17)**: Go's child places only the `Files` entries at known numbers (its first pass may `dup3` other fds onto numbers from `len(Files)` upward *(from memory)*), so an fd outside `Files` named in the path could name another file by the time of `execve`; `Env: []string{}` (an explicit empty environment; nil is empty too, `exec_unix.go:166`, but the literal keeps it so if the call is refactored to `os/exec`). An error is SEp (§3.1 O3) | R (child in the new mount ns and the epoch cgroup) | SEp; M3's then M2's rollback |
| C1 | fork child (1) | Go's raw path: `setsid()` (`:385`; the child is never a group leader, so it cannot fail `EPERM`, setsid(2)); `dup3` set H onto 0..k; `execve` the helper; no allocation, no lock | R, new session, no controlling tty | `perm` from here on: the tree exists |
| H1 | helper (n) | `close_range(k, ~0U, CLOSE_RANGE_CLOEXEC)` (S12, `TestHelperFdHygiene`; **draft 15, T6:** `CLOEXEC`, not flag 0, because flag 0 would also close any fd the helper's own runtime opened before `main`; from k, so the helper's own `O_PATH` fd does not survive H10); verify fd 0 is `/dev/null`, fds 1 and 2 are the log pipe, `getsid(0) == getpid()`, and `open("/dev/tty")` fails `ENXIO` (no controlling terminal; `TestHelperNoCtty`) | R | `perm` |
| H2 | helper (n) | read the plan entry from fd 3, **a sealed memfd (draft 15, T18)**: `F_GET_SEALS` equals the four seals, else `perm` (written by M_p from the core's plan; trusted) | R | `perm` |
| H3 | helper (n) | write `0` to `/proc/self/oom_score_adj` and read it back (S1); `oom_score_adj` is per-process (shared `signal_struct`), so this step is not per-thread | R, oom (0, 0) | `perm` |
| H4 | helper (n) | read `/proc/sys/fs/suid_dumpable`; `1` is `perm` (`ErrIsolationUnavailable{suiddumpable, linux}`, also checked at load, below) | R | `perm` |
| H5 | helper (n) | `mount(NULL, "/", NULL, MS_SLAVE \| MS_REC, NULL)` **before any other mount** (S11, `TestHelperPropagationSlave`) | R | `perm` |
| H6 | helper (n) | for each K6 region, **opened once, here, by the helper (T7)**; M_p passes no region fd: `openat2(RESOLVE_NO_SYMLINKS \| RESOLVE_NO_MAGICLINKS)`; for a read-only region `open_tree(fd, "", OPEN_TREE_CLONE \| AT_RECURSIVE \| AT_EMPTY_PATH)`, `mount_setattr(…, RDONLY \| NOSUID \| NODEV, AT_RECURSIVE)`, `move_mount(tree, "", fd, "", MOVE_MOUNT_F_EMPTY_PATH \| MOVE_MOUNT_T_EMPTY_PATH)`, reopen the region through its parent fd and verify `STATX_MNT_ID` and `ST_RDONLY` (S8, `TestMountByFd`). The region fds stay open for D4 | R | `perm` |
| H7 | helper (n) | **credentials (T3) [Host+14]:** open each credential source and the decryption key, if any, `openat2(… O_CLOEXEC, RESOLVE_NO_SYMLINKS)`; `fsopen("tmpfs")`, `fsconfig` size bound, `fsmount(…, MOUNT_ATTR_NOSUID \| MOUNT_ATTR_NODEV \| MOUNT_ATTR_NOEXEC)`; write each credential as a file mode 0400 owned by u through the mount fd; `mount_setattr(RDONLY)`; `move_mount` onto the delivery directory's fd; zero the key buffer and close every source fd. The mount exists only in this mount namespace | R | `perm` |
| H8 | helper (n) | **lane mode only (T9):** `fsmount` an empty tmpfs, `mount_setattr(RDONLY \| NOSUID \| NODEV \| NOEXEC)`, `fchdir` to it, `pivot_root(".", ".")`, `umount2(".", MNT_DETACH)`; afterwards L_p reaches the file system only through the directory fds the plan names (state and runtime directories), and both the stage and L_p's binary are executed by fd. The `pivot_root(".", ".")` idiom is *from memory* (test `TestLaneEmptyRoot`, P1: from L_p, `openat(AT_FDCWD, "/etc/passwd")` fails `ENOENT`) | R | `perm` |
| H9 | helper (n) | open the working directory `O_PATH \| O_DIRECTORY` (resolved as R, checked as u at **D11**: D5 runs as R, so draft 14's « checked as u at D5 » could not be, T16); write the **frame**: a fixed-layout record (magic and the `internal/frame` layout digest, T17; ids, group list, the plan's capability set (T15), **every rlimit** 0..`RLIM_NLIMITS`−1, those the plan does not name taken from the values the core recorded at its own start, before it raised `RLIMIT_NOFILE` (T16), umask, signal-state flag, the scheduling fields of D5a, argv, envp built **from the plan only**, the compiled filter, fd numbers), each field length-bounded, into a `memfd_create(MFD_CLOEXEC \| MFD_ALLOW_SEALING \| MFD_NOEXEC_SEAL)` (**draft 15, T19:** with `vm.memfd_noexec` = 2 a memfd created without `MFD_NOEXEC_SEAL` is refused *(from memory)*), then `F_ADD_SEALS(F_SEAL_WRITE \| F_SEAL_GROW \| F_SEAL_SHRINK \| F_SEAL_SEAL)` | R | `perm` |
| H10 | helper (n) | lay out the stage's fds: 0–2, the status write end, the readiness fd, the frame, the cwd fd, the region fds; `close_range(…, CLOSE_RANGE_CLOEXEC)` on every other fd; **draft 15 (T18):** in normal and signal modes, `syscall.Exec("/proc/self/fd/<stage_fd>", argv, []string{})`, which takes the runtime's exec lock against concurrent thread creation *(from memory)* and restores the helper's original `RLIMIT_NOFILE` (`exec_unix.go:286`); in lane mode, where H8 left no `/proc`, `execveat(stage_fd, "", NULL-free argv, empty envp, AT_EMPTY_PATH)` as in draft 14. The residual race of a raw `execveat` with the runtime creating a thread can end the helper in the runtime's exit status 2 with no status byte; every fixed exit code of the H- and D-steps is outside {1, 2}, so that exit is unclassified and `fail` under E11 (one charged restart), never `perm` and never Failed(spawn). Tests `TestHelperExecLocked`, and `TestHelperExecFdPinned` (M4's fd k survives Go's `dup3` pass with 64 `Files`) | R, **one thread** | `perm` |
| D1 | svd-drop (1) | `prctl(PR_SET_DUMPABLE, 0)` — **the first system call** (execve just set dumpable to 1), issued by the assembly entry on both architectures; **amd64 only, draft 15 (T13):** `arch_prctl(ARCH_SET_FS)` follows as the second, before any Go code | R, not dumpable | `perm` (exit code only: no frame read yet) |
| D2 | svd-drop (1) | `F_GET_SEALS` on the frame equals the four seals, else `perm`; read the frame into a fixed buffer; every length checked against its bound; **draft 15 (T17):** the magic and the layout digest equal the stage's own, else `perm` (`ErrStageMismatch`) | R | `perm` |
| D3 | svd-drop (1) | `prctl(PR_SET_NO_NEW_PRIVS, 1)` | R, NNP | `perm` |
| D4 | svd-drop (1) | Landlock ruleset from the region fds; `landlock_restrict_self`, which binds « the calling thread » (landlock_restrict_self(2), man-pages 6.19), here the whole process; close the region fds. Survival of the domain across `execve` is not stated in the page: test `TestLandlockSurvivesExecve` (P1) | R, NNP, Landlock | `perm` |
| D5 | svd-drop (1) | **process state (T3) [Host+14]:** `prlimit64(0, …)` for **every** rlimit of the frame (draft 15, T16: not only those the plan names, so none is inherited from the helper's runtime), and `RLIMIT_NPROC` := the principal's `pids` budget, soft = hard (per real uid, getrlimit(2); not enforced while R holds `CAP_SYS_RESOURCE`, so it takes effect at D9; limits are « preserved across execve »); `umask`; `fchdir(cwd_fd)`, close it; `rt_sigprocmask(SIG_SETMASK, ∅)`; `rt_sigaction(SIG_DFL)` for every signal but `SIGKILL` and `SIGSTOP`, because `execve` keeps ignored dispositions (execve(2)) | R | `perm` |
| D5a | svd-drop (1) | **scheduling state, draft 15 (T16) [Host+15]:** `sched_setattr(0, SCHED_OTHER, nice 0)`, `ioprio_set(IOPRIO_WHO_PROCESS, 0, IOPRIO_CLASS_NONE)`, `sched_setaffinity(0, every CPU)` (the kernel intersects it with the epoch cgroup's `cpuset`), `prctl(PR_SET_TIMERSLACK, 0)` (the default), `personality(PER_LINUX)`, each replaced by the plan's value where the plan names one; done as R because lowering the nice value needs `CAP_SYS_NICE`. Without it every service inherits the core's policy, nice, I/O class, affinity, timer slack and personality (`ADDR_NO_RANDOMIZE` included), and a host that starts the core real-time hands that to every granted service | R | `perm` |
| D5b | svd-drop (1) | **session keyring, draft 15 (T12) [Host+15]:** `keyctl(KEYCTL_JOIN_SESSION_KEYRING, NULL)`, in **every mode** (service, lane, signaller): the thread joins a new, empty, anonymous session keyring, created while R so it is charged to root's key quota and p cannot make it fail with `EDQUOT`. It is in the stage because the session keyring is part of the thread's credentials (one thread here), and before D6 so the id changes that follow do not matter. The new keyring is linked to no user keyring; a service that wants its user keyring links it itself. Reproduced need: without it a uid-1000 child read root's session key (`evidence-part7/keyexp`, kernel 7.0.14), and the same child after this call got « required key not available ». Test `TestServiceKeyringIsFresh` (P1): a `user` key planted in the core's session keyring is not found by `KEYCTL_SEARCH` from a granted service, from L_p, or from the signaller; fault injection « row removed » turns it red. The core and M_p keep the core's keyring: both are root and in the TCB | R, fresh empty session keyring | `perm` |
| D6 | svd-drop (1) | `setgroups` to the run_as account's supplementary groups (lane mode: none), resolved at load and checked again by F2b (§2.2) | R | `perm` |
| D7 | svd-drop (1) | `setresgid(g, g, g)`; then `prctl(PR_SET_DUMPABLE, 0)` again, because a change of egid or fsgid resets dumpable to `fs.suid_dumpable` (PR_SET_DUMPABLE(2const); `commit_creds`, `kernel/cred.c`) | R, gid g, not dumpable | `perm` |
| D8 | svd-drop (1) | inheritable := ∅; `PR_CAP_AMBIENT_CLEAR_ALL`; securebits := `NOROOT` \| `NOROOT_LOCKED` \| `NO_SETUID_FIXUP_LOCKED` \| `KEEP_CAPS_LOCKED` (with `NO_SETUID_FIXUP` and `KEEP_CAPS` 0); `PR_CAPBSET_DROP` for every capability 0..`CAP_LAST_CAP` (needs `CAP_SETPCAP`, so **before** D9, S5). **Draft 15 (T15):** the frame carries the plan's capability set K. K = ∅ for every granted service, every lane and the signaller (F2: held to p's privileges), and the row is then exactly as above. For a service of O with K ≠ ∅, the bounding drop spares K (capset(2) refuses an inheritable set outside inheritable ∪ bounding), and when also u ≠ 0, `NO_SETUID_FIXUP` is 1 (still locked), so D9 keeps the permitted set for D9a to cut down to K; `KEEP_CAPS` stays 0 and locked in every case | permitted and effective still the spawner's; bounding K (∅ for p); inheritable, ambient ∅ | `perm` |
| D9 | svd-drop (1) | `setresuid(u, u, u)`; with keep-caps 0 and every uid non-zero the kernel clears permitted, effective and ambient (capabilities(7)); then `prctl(PR_SET_DUMPABLE, 0)` again (reset by the euid change). Since Linux 3.1 `setresuid` no longer fails `EAGAIN` on `RLIMIT_NPROC`; the final `execve` does (setresuid(2)), which the O4 table already classifies `fail` | **none**, not dumpable | `perm` |
| D9a | svd-drop (1) | **draft 15 (T15):** `capset(permitted = effective = inheritable = K)`, then `PR_CAP_AMBIENT_RAISE` for each capability of K (with `NOROOT` set, an `execve` of an ordinary file gives the new permitted set from ambient only, for u = 0 too). For K = ∅ this is an explicit `capset(∅)`, so the result no longer depends on the kernel's uid fixup: with u = 0 (a root service of O), `setresuid(0,0,0)` clears nothing and draft 14's D9 left C_M in place, which D10 then refused (`perm`, fail-closed but a service that could never start) | K (∅ for p), not dumpable | `perm` |
| D10 | svd-drop (1) | verify: real, effective, saved and fs uids and gids; the group list; permitted, effective, inheritable and ambient = K and bounding = K (`capget`, `PR_CAPBSET_READ`, `PR_CAP_AMBIENT_IS_SET`), i.e. all five empty for p; securebits; NNP; `PR_GET_DUMPABLE` = 0; when u ≠ 0 and `CAP_SETUID` ∉ K, `setresuid(0,0,0)` fails `EPERM` (`TestHelperDropOrder`). **Draft 15 (T7): no oom read here.** At D10 the stage is under Landlock, and in lane mode H8 has left no `/proc`, so draft 14's « oom 0 » ended every lane start `perm`. The value is written and read back in H3; `oom_score_adj` is per-process, survives `execve`, and no step between H3 and D14 writes it (Gate 1 bounds the stage's calls, and none opens a file); `TestGrantedTreeOomAdj` checks it from the service | none (K for a service of O) | `perm` |
| D11 | svd-drop (1) | **preflight (T4):** `faccessat2(AT_FDCWD, ".", X_OK, AT_EACCESS)` on the working directory D5 entered (**draft 15, T16:** H9's « checked as u », moved here because D5 runs as R), then `faccessat2(AT_FDCWD, path, X_OK, AT_EACCESS)` and an `O_PATH` open of the service binary, as u, under Landlock; `ENOENT`, `EACCES`, `ENOEXEC`-class results are written to the status pipe as `perm` (the O4 rows of the final `execve`); then write the one-byte record `armed` | none | `perm` |
| D12 | svd-drop (1) | `fcntl(F_SETFD, FD_CLOEXEC)` on the status write end and the frame; `close_range` everything that is not in the service set S = {0, 1, 2, readiness} | none | `perm` |
| D13 | svd-drop (1) | `seccomp(SECCOMP_SET_MODE_FILTER, 0, plan filter)`, if any — **the last system call but one**; no Go runtime and no svd code runs under the service's filter | none, filtered | exit code only (filter may deny `write`) |
| D14 | svd-drop (1) | `execve(path, argv, envp)` from the frame; on return `exit_group(fixed code)` | none | after `armed`: `fail` (below) |

**Whole-process drop, stated [Host+14].** Steps D1–D14 are the only place in svd that changes a process's ids or capabilities. They run in a process with one thread, so **at no instant does a thread with uid 0 or any capability share a process (and so an address space, fd table or `signal_struct`) with a thread of uid u.** No step sets `PR_SET_KEEPCAPS`. **Dumpable:** 0 from D1 to D14, re-asserted after each credential change (D7, D9), so a uid-u process cannot `PTRACE_ATTACH` the stage while it is half-dropped: with dumpable ≠ 1 ptrace's access check requires `CAP_SYS_PTRACE` in the target's user namespace (ptrace(2), « Ptrace access mode checking », step 4). **[Host+14]** the load refuses delegation when `fs.suid_dumpable` is 1 (`ErrIsolationUnavailable{suiddumpable, linux}`), because a value of 1 makes every credential change leave the process dumpable (proc_sys_fs(5): « insecure … allows unprivileged users to examine the memory contents of privileged processes »); values 0 and 2 both give « not dumpable » until D1's `prctl` runs again. The service's own dumpable state is set by its `execve`.

**The final `execve` and the filter (T4) [Host+14, Compile+14].** The filter is installed by D13, immediately before D14. **[Compile+14] `ErrSeccompNoExec{service}`** when a plan's filter does not allow `execve` and `exit_group`: svd refuses the filter rather than silently widening it (R0). A preflight failure in D11 is `perm` as the O4 table requires. A failure of D14 itself after `armed` (the binary removed or replaced between D11 and D14, or `EAGAIN` from `RLIMIT_NPROC`) is reported only by the exit code, because the plan's filter may deny `write`; M_p classifies « `armed`, then EOF, then exit » as `fail`, charged and bounded by the window, which is the O4 table's `fail` row and changes no table. **Rejected: « death by signal before `execve` = `perm` »** (SECURITY-1 T4's alternative): it would add a row to the O4 classification table of §3.1, its cause (the Go runtime under the service's filter) no longer exists, and a uid-u process that kills its own stage is p's cost under D2 option A, charged to R_p.

**Rollback of M2–M4 (T5) [Host+14].** Each of M2–M4 undoes, in reverse order, what it and the earlier steps created (table above); any failure there is SEp. **Rejected: « consume the epoch on a failed spawn »**: it would change the record protocol and L_p's epoch arithmetic (§3.3). Instead, an `EEXIST` on M2's `mkdirat` or M3's bind can only be a leftover of svd's own (p can create nothing in `trees/` or in the 0711 notify directory, §4.1): M_p removes it once (`unlinkat`, with `AT_REMOVEDIR` for the cgroup) and retries once; a second failure is SEp (test `TestSpawnFailureLeavesNothing`, P1: 10⁴ injected failures at each of M2, M3, M4 leave no cgroup, socket or fd behind).

**Session and terminal (T1) [Host+14].** Every process M_p or the core spawns, helper, signaller and lane included, starts in a new session (C1) with fd 0 `/dev/null` and fds 1–2 the log pipe, and inherits no fd that names a terminal. `TIOCSTI` is therefore closed as a vector: `tiocsti()` returns `-EPERM` when `current->signal->tty != tty` and the caller lacks `CAP_SYS_ADMIN` (`drivers/tty/tty_io.c`, torvalds/linux master), and a process with no controlling terminal and no tty fd has no tty to inject into; the core's own terminal, if it has one, is never reachable from a tree.

**Modes.** The **lane mode** (L_p's start) runs C1, H1–H6, H8–H10 and D1–D14 with the lane's id n as u and g, an empty group list, every mount of its namespace read-only, `CLONE_NEWIPC \| CLONE_NEWNET` added in M4, and L_p's binary exec'd by fd in D14 (`execveat(…, AT_EMPTY_PATH)`). The **signal mode** (the signaller, §3.3 signalling rule) adds no namespace and skips H4–H8, D4, D5 and D5a, **but not D5b (draft 15, T12: the keyring row runs in every mode)**; its D13 installs the signaller allowlist (with `execveat` of the one `svd signal` fd) and D14 `execveat`s `svd signal`, which sets `PR_SET_DUMPABLE` 0 and stacks a second filter without `execve`/`execveat` as its first actions (§3.3 step 4). **Draft 15 (T19) [Host+15]:** that second filter is installed with `SECCOMP_FILTER_FLAG_TSYNC`, because `svd signal` is a Go program and other threads exist by the time its « first actions » run; a TSYNC failure (the returned thread id) is `perm`. This supersedes §3.3 step 4 where it is silent. Filters are « preserved » across an allowed `execve` (seccomp(2)).

**Which files are the helper and the stage (draft 15, T17) [Host+15].** svd's internal executables (`svd-monitor`, `svd helper`, `svd-drop`, `svd signal`) are installed together in one root-owned, versioned directory. The core opens that directory once at its start, then each executable relative to it with `openat2(O_PATH \| O_CLOEXEC, RESOLVE_BENEATH \| RESOLVE_NO_SYMLINKS \| RESOLVE_NO_MAGICLINKS)`, checks owner 0 and no group or other write bit, and hands M_p the helper, stage and signal fds at M_p's start (§3.3 descriptors row); nothing reopens them by path afterwards, so an upgrade that replaces the files takes effect at the next start of the core, never between the helper and the stage of one spawn. The frame carries the layout digest of `internal/frame`, which D2 compares with the stage's own (`ErrStageMismatch`, `perm`): two builds that disagree on the frame cannot run together. The helper's fd in the fork child is M4's index k (above).

**The pidfd across the helper's exec (draft 15, T19).** The helper is multi-threaded, and its `execve`/`execveat` (H10) may run on a thread that is not the group leader, which then takes over the leader's pid. That M_p's pidfd stays quiet across this is a kernel claim, not an opened source: test `TestPidfdQuietAcrossHelperExec` (P1, H10 forced onto a non-leader thread 10⁴ times). Independently of that test, M_p never declares an epoch's main dead on pidfd readiness alone: it confirms with `waitid(P_PIDFD, pidfd, WEXITED \| WNOHANG \| WNOWAIT)` returning the exit, and treats a readable pidfd with no exit as spurious (re-armed, reported).

#### 7.1.2 M_p's system-call surface (draft 14, SECURITY-1 item 4; draft 15, SECURITY-1 T14) [Host+14, Host+15]

**Decision (user, binding):** M_p stays **unfiltered for P1** (§3.3 H_OS-E5 privilege row). **Rejected: a separate unfiltered spawner with M_p filtered** (SECURITY-1 option B as written): the spawner would act on M_p's requests, so a compromised M_p dictates the spawn plan and the filter on M_p protects nothing; the corrected form (the spawner reads the plan only from the core, SECURITY-1 §6.3's four rules) needs a third process per principal and a three-party reap protocol, and is recorded as later hardening, not P1. What replaces the filter is a **lint that bounds M_p's surface in CI**, in **three** halves (draft 15; draft 14 had two) that must all pass, and a negative control that must make each of them fail.

**Draft 15, what changed and why (T14).** Draft 14's dynamic half read `SECCOMP_RET_LOG` records from the audit log: reading them needs privilege on the CI host, records are rate-limited and can be dropped, and a dropped record is a missed system call, i.e. a green *(from memory)*; a record carries the number, not the arguments, so the runtime's self-directed `tgkill` could not be attributed. Its source half « maps every call into `syscall`, `x/sys/unix` and `os` » from the import closure, which either counts the whole standard library or stops at svd's packages and misses `os.ReadFile`'s `openat(AT_FDCWD)` and every call through an interface or a function value. Both are replaced below. **[Host+15] M_p is built as its own executable, `svd-monitor`** (`cmd/svd-monitor`, same module; the internal entry point `svd monitor` of §8.3 names it), so that the linker's dead-code pass has already removed what M_p cannot reach and the third half is exact; as one multi-call `svd`, M_p's address space would hold the helper's mount and credential code and the lint would bound only the calls the monitor package makes.

0. **The lists.** **A_M** is committed in two parts: **A_M^start**, the calls M_p may issue before its first `recvmsg` on L_p's socket (descriptor checks, `prctl(PR_SET_CHILD_SUBREAPER)`, `epoll_create1`, `prlimit64` for its own `RLIMIT_NOFILE`), and **A_M^steady**, the rest (the list below). **Draft 15 reconciles A_M with §7.1.1:** M3 no longer opens `/dev/null` (M_p receives it from the core), so no `openat(AT_FDCWD, …)` remains in either part; `wait4` is in A_M^steady, because `syscall.ForkExec` reaps a child that failed to exec (`exec_unix.go:236`); `memfd_create` is in A_M^steady for M3's plan memfd (T18). **R_go**, the Go runtime's own calls, is generated by a committed Go program from a **reference program that does not import `internal/monitor`** nor any svd package (it exercises the scheduler, the GC, timers, the netpoller, `os/signal` and `syscall.ForkExec` of a stub), under the recorder of half 2, once per toolchain; so regenerating it cannot launder a new monitor call into the runtime's list. The generator asserts **R_go ∩ Never = {`tgkill`}**, and `tgkill` is admitted only with its first argument equal to the caller's own tgid; any other intersection fails the regeneration.
1. **Source half, `cmd/syscallsurface monitor`** (a Go program with its own tests, in go-supervise/supervisor): builds the SSA form of the **whole program** rooted at `svd-monitor`'s `main` (`golang.org/x/tools/go/ssa`, the standard library included) and its call graph by **reachability** (VTA), so calls through interfaces and function values are resolved, and `os`, `net` or `io/fs` wrappers count by the system calls they reach. The sinks are the raw entry points (`syscall.Syscall*`, `syscall.RawSyscall*`, `golang.org/x/sys/unix.Syscall*`/`RawSyscall*` and their `linkname`d runtime targets); at each reached sink the system-call number must be a constant (a non-constant number is refused), and constant arguments are evaluated. A reached call the analysis cannot resolve (`reflect.Value.Call`, `MethodByName`, a `linkname` pull into svd code) is refused. Every `SysProcAttr` that reaches `syscall.ForkExec` must be a composite literal setting only `Setsid`, `Cloneflags`, `UseCgroupFD`, `CgroupFD` and `PidFD`, because the raw `setresuid`/`setgroups` calls of Go's fork child live inside `forkAndExecInChild1` and no symbol-level check can separate them. The reached set must be ⊆ A_M ∪ R_go, where the committed application list **A_M^steady** is: `recvmsg`, `sendmsg`, `read`, `write`, `openat` (dirfd-relative only; `AT_FDCWD` refused), `mkdirat`, `unlinkat`, `fchownat`, `newfstatat`/`statx`, `close`, `fcntl`, `pipe2`, `socket`, `bind`, `epoll_create1`, `epoll_ctl`, `epoll_pwait`, `clone3`, `waitid`, `wait4` (draft 15), `memfd_create` (draft 15), and Go's fork-child path (`setsid`, `dup3`, `execve`, `exit`, `prlimit64`); `prctl` (`PR_SET_CHILD_SUBREAPER` only) moves to A_M^start. **Never allowed**, whatever the list says: `kill`, `tkill`, `tgkill` (except the runtime's self-directed use, admitted by argument in half 2), `rt_sigqueueinfo`, `rt_tgsigqueueinfo`, `pidfd_open`, `pidfd_send_signal`, `pidfd_getfd`, `ptrace`, `process_vm_readv`/`writev`, the `set*uid`/`set*gid` family, `setgroups`, `capset`, `mount` and the new mount API, `pivot_root`, `unshare`, `setns`, `init_module`/`finit_module`/`delete_module`, `bpf`, `keyctl`, `perf_event_open`, `kexec_*`, `reboot`, `chroot`; and no `SysProcAttr.Credential` or `AmbientCaps`. It also refuses cgo and `unsafe` in `internal/monitor`. The **stage mode** of the same program is Gate 1 of §7.1.1 (draft 15: on the linked artifact, by reachability from the entry symbol, not on the assembly source or the symbol table).
2. **Dynamic half, `TestMonitorSyscallSurface`** (P1, root): **enforcing, so it cannot lose a call.** A test launcher (Go, `runtime.LockOSThread`) installs on itself, with `SECCOMP_FILTER_FLAG_TSYNC`, a filter that allows A_M^start ∪ A_M^steady ∪ R_go, admits `tgkill` only when its first argument is the launcher's own tgid (which `svd-monitor` keeps after the `execve`), refuses `openat` with `AT_FDCWD` as its first argument, and answers **everything else `SECCOMP_RET_KILL_PROCESS`**; then it `execve`s `svd-monitor` (the filter is « preserved », seccomp(2); the launcher is root with `CAP_SYS_ADMIN`, so no NNP is needed and M_p runs as in production otherwise). The kernel evaluates the filter on every system call of every thread, raw or not, and there is no record to drop: a new call **is** the red, as M_p's death by `SIGSYS`, which the test reads from `waitid` and which no later green can hide (the test also requires M_p's normal exit at the end of the suite). The filter is inherited by what M_p spawns, so the harness gives M_p a stub helper (a runtime-free binary built like `svd-drop`, writing `armed` and exiting) and stub signallers. For attribution and for what a BPF program cannot check, the same run is **recorded** losslessly under ptrace (`PTRACE_O_TRACECLONE | PTRACE_O_TRACESYSGOOD | PTRACE_O_EXITKILL`; each thread stops at every system-call entry until the recorder resumes it): (thread, number, six arguments); the recorder fails the test if a call of A_M^start \ A_M^steady follows M_p's first `recvmsg`, and names the call site of any `SIGSYS`. **Coverage, stated:** lossless capture covers what runs; the suite must drive every record kind, every `Fault`/`Refuse` path of the decoder and the 10⁴ injected failures of `TestSpawnFailureLeavesNothing` under this filter, and half 1 covers what no test drives.
3. **Exact half, `TestMonitorSymbolAbsence`** (draft 15, P1): `debug/elf` on the shipped `svd-monitor` must show **none** of a committed list of functions, among them `syscall.Kill`, `syscall.Setresuid`, `syscall.Setgroups`, `syscall.AllThreadsSyscall`, `syscall.PtraceAttach`, `syscall.Mount`, `golang.org/x/sys/unix.Kill`, `.Tgkill`, `.PidfdOpen`, `.PidfdSendSignal`, `.PidfdGetfd`, `.Mount`, `.MoveMount`, `.Fsopen`, `.Fsmount`, `.OpenTree`, `.MountSetattr`, `.PivotRoot`, `.Unshare`, `.Setns`, `.Setresuid`, `.Setgroups`, `.Capset`, `.KeyctlInt`, `.KeyctlString`, `.PtraceAttach`, `.InitModule`, `.Reboot`, `.Chroot`, and `os.(*Process).Signal`. It is exact about the functions it lists, and blind to a raw call inside a function it does not list, which is half 1's job (the fork child's `setresuid` above).

**Negative control (draft 15), `TestMonitorSyscallSurfaceCatches`** (P1, same CI job as the gate; the job fails if any expected red stays green): `svd-monitor` is built three more times behind build tags, each planting one call on the path of the first record: **`canary_never`**, `unix.Kill(unix.Getpid(), 0)`, which must turn **all three** halves red (half 1 names `kill` and its call site, half 3 finds `unix.Kill`, half 2 sees `SIGSYS`); **`canary_new`**, `unix.Sysinfo(&si)`, a call in none of A_M, R_go and Never (asserted), which must turn halves 1 and 2 red (half 3 is expected green: `Sysinfo` is not on its list); **`canary_rawnr`**, `syscall.Syscall(uintptr(n), …)` with `n` read from a variable, which half 1 must refuse as non-constant. Without these controls, « does the lint fail on a new system call » is unknown.

The lint is a bound on **code**, not a kernel barrier: a memory-corruption bug in M_p is not stopped by it, which is why M_p's input is a 64-byte fixed record decoded by a pure, allocation-free function under `FuzzMonitorRecord` with the race detector, and why the audit order of §3.3 puts the spawn path and decoder first.

### 7.2 Paths on the host (M11)

The exec-helper resolves every region on the target host.

- **Linux:**
  - open with `openat2(RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS)`;
  - give Landlock **the opened fd**, not the path;
  - make a read-only region **by fd** (`open_tree`, `mount_setattr`, `move_mount`, §7.1.1 step 10), never by a `mount(2)` call that resolves the path again (draft 13, SECURITY-1 S8);
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
- **Per-principal listeners (F1, H_OS-E5; draft 10).** Besides the owner-class socket, which the core serves, svd opens one listener per non-owner principal, **readers included** (draft 9 shared one listener among readers, so one reader could exhaust the others' budget). Each listener is served by that principal's **lane** (`svd lane`, §3.1 step 1, H_OS-E5), a process inside the principal's cgroup, never by the core. Placement:
  - **Linux:** the listener lives in a per-principal subdirectory owned by root, mode 0711; the core binds, `chown`s the socket **path** to p with mode 0600, `lstat`-verifies it and hands the listening fd to the lane, so only p (and root) may connect. No ACL is used. unix(7) (<https://man7.org/linux/man-pages/man7/unix.7.html>, opened by REVIEW-7): connecting to a stream socket requires write permission on it, and a pathname socket's owner and mode are changed with chown(2) and chmod(2); draft 10's `fchown` on the fd is withdrawn (§3.3 H_OS-E5).
  - **FreeBSD:** unix(7) warns that some BSDs ignore socket permissions, so the socket's mode is not relied on: a root-owned 0700 directory with one ACL search entry for p *(unverified, P4)*, else `ErrIsolationUnavailable{listener, freebsd}` [Host+11]; the peer-credential check below is the authority.
  - **macOS:** no per-principal listener exists, since grants and readers are refused there (§3.3 platform scope, draft 11).
  - **Windows:** the ACL route of draft 9, still *(unverified)*.
  Peer credentials are still checked on every connection; the socket a connection arrived on is a second check, never a substitute. Each listener has its own `budget.connections`, `budget.inflight` and `budget.refusals` (§3.3, H_OS-E5), and each response is bounded by `api.response.max` (class T, [Compile+10]); a lane's work is CPU-, memory- and IO-capped with p's cgroup, so p's connections cannot slow what the owner and other principals need beyond A3.

**Windows.**

- The preferred transport is **AF_UNIX** (Windows 10 1803+ *(unverified)*), in an ACL-protected directory, with peer identity from `SIO_AF_UNIX_GETPEERPID` *(unverified)*.
- If that peer check is not available, a named pipe is used, under these rules:
  - the name is random;
  - it is published in a file in the ACL-protected instance directory;
  - the server creates it with `FILE_FLAG_FIRST_PIPE_INSTANCE`;
  - clients open it with `SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION`;
  - clients check `GetNamedPipeServerProcessId` against the published server pid.

  All of these points are *(unverified)*, and they are an open item before P3.

**sd_notify sockets.** Path sockets only, in a root-owned 0711 per-principal directory, each socket `chown`ed to its service's run_as by `fchownat` relative to the directory fd and removed by one `unlinkat` (draft 13, SECURITY-1 S2; §4.1), with the sender check of §4.1. Draft 12's per-epoch 0700 directories owned by run_as are withdrawn.

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
    - *Edges out of a grant are allowed* (E14): a granted A may require, want or order an outside C. **Since draft 10 they have no influence on C inside svd** (REVIEW-6 N1): C's goal is never raised through a cut edge (§2.3), and G_free(C) and SW(C) do not read A (§3.5, §3.6), so C no longer waits for A's graceful stop. Draft 8 and 9 accepted a bounded StopWait delay of C as the edge's cost; N1 showed that p could choose that delay and so reach R_inst. The cost moves to p: A may lose C while still running. What A's processes send to C outside svd (connections, requests) is H9, assumed by H_OS-A4: `Compile` refuses such a C when a terminal service reaches it by a require path (`ErrExposedEscalates`, [Compile+11]) and names every other one (`WarnExposedToGrant`).
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
- `/v1/changes`, the audit log;
- `/v1/tickets/<ticket>`, the outcome of an owner operation forwarded to a lane (draft 12, REVIEW-8 N20 c): the ticket carries its deadline under the core's HMAC, and the core answers `expired` past it without state (§3.3, lane protocol).

**Rules:**

- **Relayed bytes are marked and escaped (REVIEW-8 N19, row H13, draft 12).** Every field whose bytes a delegated principal's process wrote (`STATUS=` text, unit-reported reasons, log lines, change outcomes of grant(p)) is returned in the API as a JSON string with `"untrusted": true` and its provenance (principal, service). The CLI writes such bytes with every byte outside printable UTF-8, and every Unicode bidirectional override, escaped (`\xNN`, `\u{NNNN}`), and prefixes each relayed line with `[p/<principal>:<service>]`, which no line svd writes on its own carries. `svd logs A` applies the same rule to every line of a granted service. **Test `TestRelayEscapes`** (P1): a granted service writes `ESC ] 0 ; title BEL`, a CSI clear-screen, U+202E, and a line beginning `svd:`; `svd status` and `svd logs` print each escaped and prefixed.
- Transactions follow §2.4.
- In Quiescing, writes return `ErrQuiescing`.
- Layers cannot be added at runtime.

**CLI** (cobra):

- `run [--user]`;
- `start [--no-rollback]`, `stop`, `restart`;
- `status`, `plan`, `validate`, `logs`, `version`;
- `lane restart <principal>` (owner-class), which restarts p's lane and, for a delegated p, its monitor (§3.3).

`svd lane` and `svd monitor` are internal entry points started by the core, never by a user; each refuses to run unless its expected descriptors are present at start.

---

## 9. Logs

- A pipe per service feeds a ring buffer and a file.
- The file is opened `O_NOFOLLOW` and rotated by size and count (class T).
- **Services of O (draft 10, unchanged behaviour):** a slow sink drops lines and counts the drops. It never blocks a child.
- **Services of grant(p) (draft 10, N3):** their pipes are read by p's lane, not by the core, and the lane applies **back-pressure** once p's unwritten log bytes reach `budget.log` (H_OS-E5): it stops reading the pipe, so p's writers block in the kernel on a full pipe, at p's own cost, instead of making svd drop O's lines or spend unbounded memory. The drops of O's services are therefore never caused by p's volume.
- **Bounded reads.** `/v1/logs` and `svd logs` return at most `api.response.max` bytes per response (class T, [Compile+10]), paged by offset; a reader's or p's log read is served by its own lane, so a large read costs its caller's caps.
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
  - E14 (replaced in draft 10, N1): a Leaked granted A that requires an outside C does **not** delay C's stop: C goes StopWait → Stopping at the same step as with A absent, no T2 overrun is reported for C, and A reaches StopWait(dep) through G_lost;
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
  - F5: intensity.window ≤ max × V(A) is `ErrNeverFailingLoop` with the `need` field; a service that crashes forever with window > max × V(A) reaches Failed(intensity) (V, renamed from U in draft 10, N5);
  - F6: a guard row with a missing cell fails the table load; an `Absent` cell is never evaluated; swapping G_failed and G_arm in Waiting turns Gate 1 red;
  - F8: a helper `perm` arriving while the service is Stopping, Killing or Leaked is recorded as `perm`, and the service reaches Failed(spawn) (or Failed(unkillable) from Leaked), uncharged;
  - F9: a delegated Retry with R_p one below budget pays once, succeeds, and leaves |R_p| = budget; a second Retry is `ErrPrincipalBudget`;
  - owner Retry of A ∈ grant(p) pays R_p, never R_inst, never changes `mode`, also at |R_p| = budget_p (§3.3, new in draft 9);
  - F11: a notify socket flooded with `SCM_RIGHTS` messages: every message with `MSG_CTRUNC` is dropped, svd's fd count stays below its headroom, and no fd leaks (count before and after);
  - F2 A1 probes, per OS: from a granted service run as p, `kill(π, 0)`, `ptrace` attach, `process_vm_writev` and an open of `/proc/π/mem` against an outside service's pid π all fail (Linux; the macOS, FreeBSD and Windows equivalents are listed with the open items of §12).
- **REVIEW-6 (draft 10)**, each a named regression test:
  - N1: A ∈ grant(p) requires B ∈ O; C ∈ O also requires B; B crashes (charged to R_inst), C exits, and A ignores SIGTERM. Run twice, once with A handling SIGTERM promptly; assert B reaches Stopping at the same step in both runs and |R_inst| is equal. The same fixture under draft 9's G_free is red (`freeAcrossCut`);
  - N1, SW: SW(B) is unchanged when a granted dependent's `stop.timeout` or `kill.timeout` is raised (`swAcrossCut`; a `Compile` unit test since draft 11, red on the mutant that computes SW over cut dependents, REVIEW-7 N13);
  - N2: an O require-target S of T writes `READY=1` in the same round as T's WaitTO is dequeued; T reaches Starting, never Failed(wait) (O5′ drains S's channel first);
  - N4: p's 1 000 `start X` requests leave exactly one holder h(p, X); `--no-rollback` marks it permanent; svd's heap after the loop is within a fixed bound of its heap before; the audit file never exceeds `audit.file.size` × `audit.file.count`;
  - N6: a notify datagram carrying `SCM_CREDENTIALS` and an `SCM_RIGHTS` fd in either order: the fd is closed after `recvmsg` (fd count before and after), and the datagram is dropped;
  - capacity sums: Σ `budget.cpu` above CPUs − `instance.reserve.cpu` is `ErrResourceBudget{cpu}`, and Σ (`budget.memory` + `budget.swap`) above RAM − `instance.reserve.memory` is `ErrResourceBudget{memory}`; a non-owner principal with no reserve configured is a Compile error (missing class-G field in Q);
  - [Host+10]: on Linux with the `io` controller disabled, a grant is refused with `ErrIsolationUnavailable{io, linux}`; on macOS, every grant **and every reader** is `ErrIsolationUnavailable{cpu, macos}` (draft 11; draft 10 started a reader lane);
  - lanes: `kill -9` of p's lane leaves `mode` Running and O's services untouched; p's API is unavailable until `svd lane restart p`;
  - H9: an edge from A ∈ grant(p) to C ∈ O gives `WarnExposedToGrant{C, p}` when no terminal service reaches C by a require path (superseded in draft 11, below);
  - F3 costs: the owner's `stop C`, with A ∈ grant(p) requiring C, tears A down to StopWait(dep) and C reaches Stopping without waiting for A.
- **REVIEW-7 (draft 11)**, each a named regression test:
  - N7: a granted service forks children that fork a grandchild and exit at once, in a loop; assert the core's reaped-orphan counter stays at 0 and the core's CPU time over the run is within a fixed bound of an idle run; a monitor started without `PR_SET_CHILD_SUBREAPER` (fault injection) fails the load with `ErrIsolationUnavailable{reaper, linux}`. *Draft 12 (REVIEW-8 Part 3):* the assertion « its parent pid is the lane's » is dropped, because a process of p's may set `PR_SET_CHILD_SUBREAPER` itself and become the nearest subreaper of its own subtree, which is harmless and at p's cost;
  - N8: p's lane is starved (its `cpu.max` saturated by p's trees, and `SIGSTOP` sent by root to the lane mid-snapshot-write); the owner sends `svd restart A` (A ∈ grant(p)) every 100 ms; assert every such request is answered at once (ticket or `ErrLaneBusy`), the core's owner-class connection count returns to its idle value after each, the owner's `svd start C` (C ∈ O) is accepted in every trial, and `svd status` returns "grant(p): status unavailable" after `lane.snapshot.retries` attempts instead of spinning; a snapshot with an invalid state value is rejected and counted;
  - N9 (restated in draft 12 for the split): L_p's real, effective and saved uids are its `lane.uids` uid and all five of its capability sets are empty; M_p's uids are 0 and its capability sets are exactly the stated set (`TestMonitorCapSet`); from a uid-p process, `ptrace` and `kill(…, SIGSTOP)` against L_p and M_p, and an `open` for write of an epoch cgroup's `cgroup.procs`, all fail; on FreeBSD (P4), M_p, L_p and a granted tree all report login class `svd-<uid>` after `execve` (`TestLoginClassSurvivesExecve`), and a uid-p `setloginclass` fails;
  - N10: `app` terminal requires `postgres`, and p's `reporting` requires `postgres`: `ErrExposedEscalates{postgres, p, app, [app, postgres]}`; with `postgres` terminal itself, the path has length 0; with `app` not terminal, `WarnExposedToGrant{postgres, p}`;
  - listener (unix(7)): after load, `lstat` of p's socket path reports owner p, mode 0600; a second uid's `connect` gets `EACCES`; a mode or owner mismatch injected between bind and listen fails the load;
  - rctl (P4): the generated rule set has `:deny` on `maxproc`, `memoryuse`, `swapuse`, `pcpu` and `:throttle` on the four IO resources, and `rctl -a` accepts each; with `kern.racct.enable` = 0 a grant and a reader are both refused.
- **REVIEW-8 (draft 12)**, each a named regression test:
  - N14, split: a lane whose API decoder is replaced by one that obeys p (fault injection: « compromised L_p ») tries, for each, a `setresuid` to an O service's uid, a write to `/etc/svd-probe`, a write to the core's `cgroup.freeze`, `kill` and `pidfd_open` of an O pid; every attempt fails (`EPERM`, `EACCES` or the seccomp error), and O's obs_O equals the run without the injection; the same lane sending records for a service index ≥ \|grant(p)\| or a signal outside the plan set produces `FAULT`, after which `deleg/<uid>/trees/` is empty and `mode` is Running;
  - record decoder: `FuzzMonitorRecord` (corpus committed: every valid kind, each single-field violation, every length 0–65, `MSG_CTRUNC`), asserting the three outcomes are exhaustive, the heap is constant, and no fault leaves a tree populated; `TestMonitorImportClosure`;
  - N15: a granted service with a K6 `read_only` region starts and reaches Ready, and a write to that region from the service fails with `EROFS` (`TestMonitorSpawnReadOnly`); with `CAP_SYS_ADMIN` removed from M_p (fault injection), the service reaches Failed(spawn) uncharged (SEp), never Failed(intensity);
  - N16, H12: `TestFreezeBlocksReap`; `TestNoRawKill`; the pid-cycling chaos run below;
  - N17: `TestMonitorNotOomVictim`, `TestZombieCountsInPidsMax`, `TestSubreaperDeathReparent`; `kill -9` of M_p while p runs a fork-and-exit flood: the core's reaped-orphan count for that death is ≤ `budget.pids` + `reap.batch`, `deleg/<uid>/` is frozen before it is killed (event order from `cgroup.events`), and `mode` is Running;
  - N18 a: a lane killed in the same round as an O orphan burst of `reap.batch` + 1 processes: the lane's MainExit is handled by its own source (the orphan source never reaps it), and the lane-death report is emitted;
  - N18 b: mode B on Linux and FreeBSD: the core is a (sub)reaper; on FreeBSD (P4), `kill -9` of M_p leaves no zombie of p's after one round of the orphan source (`TestReapKillSubtreeAfterReaperDeath`, `TestReapKillSubtreeAfterOrphan`);
  - N19: `TestRelayEscapes` (§8.3);
  - N20 c: a ticket polled after its deadline answers `expired` with the lane `SIGSTOP`ped, and a ticket with a flipped MAC byte answers `400`; N20 d: a reader's lane has empty capability sets; N20 e: on Linux without a writable cgroup, a grant is `ErrIsolationUnavailable{cgroup, linux}`;
  - `TestSnapshotSealNoSigbus`: the lane attempts `ftruncate` of its snapshot memfd to 0; the call fails and the core's next read completes.
- **SECURITY-1 (draft 13)**, each a named P1 test; a design rule that rests on a « from memory » kernel property is not relied on until its test passes (§12):
  - S1: `TestGrantedTreeOomAdj`: inside a granted service and inside L_p, `oom_score_adj` reads 0 and writing `-1` fails `EACCES`; `TestMonitorNotOomVictim` kept;
  - S2: `TestNotifyDirNotOwnedByGrantee`: p's notify directory is root-owned 0711 with root-owned ancestors; the service's socket is owner run_as, 0600; a chaos run in which the service plants symlinks, hard links and directories under every name it can reach, across many epochs, asserts with an fanotify or audit watch that no root operation follows or removes any of them, and that the directory never holds more than \|grant(p)\| plus one round's ending epochs;
  - S3: `TestPidfdSendSignalUidRule` (a uid-p `pidfd_send_signal` to a uid-q process fails `EPERM`); `TestSignallerCredentials` (inside the signaller before its first signal: ids p, groups ∅, five capability sets empty, NNP, filter present); the pid-cycling chaos run, now with the fault injection « signaller runs as root », which must be able to go red;
  - S4: `TestLaneSeccompAllowlist` (each denied call, `rt_sigqueueinfo` and `rt_tgsigqueueinfo` included, kills L_p; the whole lane suite passes under the filter); `TestSeccompTgkillSelfOnly` (`tgkill` and `kill` with another process's pid kill L_p; a high-bits-set copy of the own pid is refused, so the comparison is on all 64 bits); `TestLaneFilterAllThreads` (`Seccomp: 2` in every `/proc/<pid>/task/*/status`); `TestGrantedServiceNoInheritedFilter`;
  - S5: `TestHelperDropOrder` (a syscall trace of the helper shows steps 13–19 of §7.1.1 in order, `PR_CAPBSET_DROP` before `setresuid`, no `PR_SET_KEEPCAPS`, and no instant at which the uid is p while any capability set is non-empty); **draft 14:** the steps are D6–D14 of `svd-drop`, and the test reads **every** `/proc/<pid>/task/*/status` at each traced stop, asserting one task and no task with uid p beside a task with uid 0 or a non-empty capability set; a fault-injection variant that performs a raw per-thread `setresuid` in a multi-threaded Go process must turn it red;
  - **SECURITY-1 Part 6 (draft 14), all P1:** `TestDropStageSingleThread` (draft 15: Gate 2 of §7.1.1 item 4, an exact system-call sequence and one task under ptrace, natively on amd64 and arm64; plus Gate 1, the stage mode of `cmd/syscallsurface`); `TestHelperNoCtty` (helper, signaller, lane and service: new session, no controlling tty, fds 0–2 as §7.1.1 H1, `ioctl(TIOCSTI)` on any tty they could name fails); `TestDumpableDuringDrop` (a uid-p process polling `PTRACE_ATTACH` on the stage from D1 to D14 never succeeds, with `fs.suid_dumpable` 0 and 2; with 1 the load refuses); `TestSpawnFailureLeavesNothing` (§7.1.1 T5); `TestHelperStrictFilter` (a plan filter denying every call except `execve` and `exit_group` still yields a running service and a correct O4 classification; one denying `execve` is `ErrSeccompNoExec`); `TestLandlockSurvivesExecve`; `TestLaneEmptyRoot`; `TestNotifyBindViaProcFd`; `TestMonitorSyscallSurface` (§7.1.2); `TestCredentialsNotInService` (no fd of the service names the key, a credential source or the plan; the credential files are 0400, owned by u, on a read-only `nosuid,nodev,noexec` mount); `TestSignallerResidue` (after every signaller, `signal/` is empty); `FuzzMonitorRecord` gains the stale-generation case of T8 (§3.3);
  - **SECURITY-1 Part 7 (draft 15), all P1:** `TestDropStageGatesCatch` (§7.1.1 item 4, red controls (a)–(f): each planted defect must turn its gate red); `TestServiceKeyringIsFresh` (D5b: the service's session keyring is new, empty and not the core's); `TestHelperExecLocked` and `TestHelperExecFdPinned` (H10, T17–T18); `TestPidfdQuietAcrossHelperExec` (T19); `TestMonitorSyscallSurface` becomes enforcing and `TestMonitorSymbolAbsence` and `TestMonitorSyscallSurfaceCatches` are added (§7.1.2 items 2–3 and its negative control);
  - S6: `TestLaneAccountHost` (each `ErrLaneAccount` reason: an id that is a host account or group, in a subordinate id range, or held by a live process, refuses the load); `TestLaneIdStable` (a principal keeps its id across loads, a removed principal's id is never re-issued); `TestLaneOwnsNothing` (L_p's mounts are all read-only, its IPC and network namespaces are new, its fd table holds only the stated fds);
  - S7: `TestMonitorRecordRate` (beyond `monitor.rate`, `NACK rate`; a second outstanding signal, `NACK busy`; seq consumed on each); `TestMonitorSendNeverBlocks` (L_p stops reading: M_p's reply ring fills, M_p stops reading records, keeps reaping, and its round time stays within a fixed bound); `TestMonitorFloodCoreCgroupLatency`: the N2/N3 chaos run adds a record flood at `monitor.rate` under saturated `cpu.max`, so M_p's cgroup-file and freezer work contends on the kernel's cgroup locks; the core's latency for its own cgroup writes enters the measured Λ (H_OS-A3);
  - S8: `TestMountByFd`: a region path swapped for a symlink between `openat2` and the mount (race harness) never yields a read-only mount elsewhere; the region is read-only and its mount id is the new one;
  - S9: `TestSnapshotReadOnlyInLane`; S10: `TestTreeEmptyAfterReap`, `TestLaneRestartThaws`; S11: `TestHelperPropagationSlave` (a mount made in the helper's namespace is absent from the host's, and a host unmount completes while the service runs); S12: `TestHelperFdHygiene` (the service's fd table is exactly the fixed set); S13: `TestRecordPlanGeneration` (a record with the previous generation is `NACK generation`, never Acted), `TestCloneIntoCgroupErrnos` (each of `EACCES`, `EBUSY`, `EOPNOTSUPP` injected is SEp, uncharged);
  - `TestMonitorCapSet` updated: M_p's sets are exactly C_M (no `CAP_KILL`, `CAP_SYS_RESOURCE` present), and `TestNoRawKill` covers `internal/monitor`, the signaller and the core's signalling package.
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
- **F1 extended in draft 10 (REVIEW-6 gap 5, N2, N3):** in addition, p floods its API listener, its channels and its logs, busy-loops on every core, and saturates IO on the device of an O service; meanwhile an O require-target S of a terminal T becomes READY within Λ of T's WaitTO deadline, over many trials with the offset swept across [−10 Λ, +10 Λ]. Assert: T reaches Starting in every trial with READY before the deadline, `mode` stays Running, and the core's measured event latency is recorded as the measured Λ for the host; a measured Λ above `instance.timing.lambda` fails the test and names the host (H_OS-A3).
- **F1 extended in draft 11 (REVIEW-7 N7, N8, gap 5):** in addition, p runs a fork-and-exit flood at its `budget.cpu`, and p's lane is starved for the whole run while the owner issues forwarded operations on grant(p) and ordinary operations on O. Assert, besides the draft-10 assertions: the core reaps none of p's processes, no owner operation on O is refused, and the measured Λ does not rise above the draft-10 run's by more than its own trial spread.
- **Pid cycling (draft 12, REVIEW-8 N16, row H12):** p's trees saturate `cpu.max` and cycle the pid counter with fork-and-exit within `pids.max`, while a granted service A with a multi-process tree is stopped and restarted by p in a loop (each restart one R_p charge) and an O service with a short-lived worker pool respawns workers continuously, over many instance windows. Assert: no process of O ever receives a signal from svd that its own plan did not cause (an O sentinel logs every signal with `si_pid`), |R_inst| stays 0, and `mode` stays Running. Run once with the signalling rule replaced by `cgroup.procs` + `kill(2)` (fault injection); that run must be able to go red, or the test does not measure H12.

**Security audit** before v0.1.0, by a fable `security` agent. It covers:

- authority admission;
- peer credentials;
- endpoints;
- per-epoch sockets;
- host path resolution;
- delegation;
- **the monitor's TCB first** (draft 12): the record decoder, the spawn path up to `execve`, the reaper and the signalling path, against a lane presumed compromised. **Draft 13 order:** the helper (§7.1.1, every step and its privilege), then the signalling path and the signaller, then the decoder; the Go runtime's behaviour under the lane's allowlist is in scope.

---

## 11. Phasing

| phase | content |
|---|---|
| P0 | `Compile`, with kinds, ⊥err, authority, the baseline and completeness, and typed errors; fuzzed laws. FSM data tables with Gates 1–3. |
| P1 = v0.1.0, Linux | modes C, B and U; fd and sd_notify channels; trees through cgroups; intensity, escalation and Quiescing; holders and rollback; per-principal lanes and listeners, cgroup `pids`, `memory`, `cpu.max`, `io.max` and `memory.swap.max` caps (draft 10); the monitor/lane privilege split, the record protocol and the signalling rule (draft 12; REVIEW-8: P1's work on `Compile`, the FSM and the core does not depend on these and may start before them); exec-helper with `openat2` + Landlock-by-fd; sdcreds; API; chaos; security audit. **The s6 output is decided after this phase** (user). |
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
  - per-principal ACLs on listener sockets, per OS (§8.1); *draft 10: Linux uses socket ownership instead; draft 11: a `chown` of the path, per unix(7); the item remains for FreeBSD (directory ACL) and Windows (DACL)*;
  - dropping the capability bounding set for delegated services (Linux), and the Windows token reduced to `SeChangeNotifyPrivilege` (§1.6);
  - FreeBSD `rctl` with `racct` enabled (*draft 10: the NPROC + AS fallback is withdrawn, delegation is refused without racct*);
  - macOS `RLIMIT_NPROC` and `RLIMIT_AS` as a per-principal bound;
  - Windows nested job objects with per-principal limits;
  - `MSG_CTRUNC` behaviour on every OS when the control buffer is too small for `SCM_RIGHTS` (whether the kernel closes the truncated fds);
  - the A1 probes on macOS, FreeBSD and Windows (§10).
- draft 10 (REVIEW-6), each *(unverified)* until tested or re-opened:
  - **the lane snapshot mechanism**: how the core publishes O's states to the lanes (a shared read-only memory region with a sequence counter, or a pipe of deltas), and its cost per event; the serialisation lemma needs only that lanes never write O, which holds for either;
  - Linux cgroup v2 `cpu.max` and `io.max` semantics under contention (whether a capped group can still delay an uncapped one through throttling latency), and `io.max` device resolution for overlay, loop, device-mapper and network file systems;
  - ~~FreeBSD rctl resource names `pcpu`, `readbps`, `writebps`, `readiops`, `writeiops`, `swapuse`, cited from memory~~ **closed in draft 11**: names and allowed actions confirmed in rctl(8), racct default in rctl(4), opened by REVIEW-7 (§3.3);
  - Windows job objects' CPU hard cap and IO rate control;
  - macOS: no per-principal CPU or IO cap is known, so grants **and, since draft 11, readers** are refused there; a mechanism found later would lift that refusal (open feature gap below);
  - ~~the Linux rule that connecting to an AF_UNIX path socket needs write permission on the socket inode~~ **closed in draft 11**: confirmed in unix(7) by REVIEW-7; the placement now `chown`s the path, since the page supports chown(2) on the pathname and not `fchown` on the fd (§3.3, §8.1);
  - the order of control messages when a datagram carries both `SCM_CREDENTIALS` and `SCM_RIGHTS`, and `CMSG_SPACE` sizes per architecture (the N6 walk closes the fds in either order, so only the cost is open);
  - ~~whether a terminal C exposed to a grant should be a refusal rather than a warning: left for the user~~ **decided by the user (2026-11-23), refined by REVIEW-7 N10:** every exposed C that some terminal service reaches by a require path (the set Esc, §3.3 H_OS-A4) is refused with `ErrExposedEscalates{C, p, T, path}` [Compile+11]; every other exposed C keeps `WarnExposedToGrant{C, p}`, whose `terminal` flag is removed.
- draft 11 (REVIEW-7), each *(unverified)* until tested or re-opened:
  - ~~`PR_SET_CHILD_SUBREAPER` (prctl(2)) and `PROC_REAP_ACQUIRE` (procctl(2)) semantics, cited from memory~~ **opened in draft 12** (REVIEW-8 Part 4: prctl(2) man-pages 6.19; procctl(2) FreeBSD 15.1), with two corrections applied: `PROC_REAP_ACQUIRE` covers **future children only**, and a dead reaper's descendants go to **that reaper's reaper**, not to init (§3.3, §5.1). Still open: whether a pidfd per O epoch plus the orphan source's `WNOWAIT` peek can starve under a burst of O exits;
  - ~~the lane's minimal capability set on Linux (`CAP_SETUID`, `CAP_SETGID`, `CAP_KILL`)~~ **withdrawn in draft 12** (REVIEW-8 N14–N16): the lane holds no capability; the open item is now M_p's exact set (`TestMonitorCapSet`), and the Windows token equivalent of the split (P3);
  - FreeBSD: ~~that `setloginclass(2)` needs root, that the login class is inherited across fork, and that `rctl` rules on `loginclass:` aggregate the lane with its trees~~ **opened in draft 12** (REVIEW-8 Part 4: setloginclass(2), rctl(8), FreeBSD 15.1); POSIX.1e directory ACLs for the listener; GENERIC's `RACCT_DEFAULT_TO_DISABLED` (from memory: if it holds, every FreeBSD host needs `kern.racct.enable=1` in `loader.conf` and a reboot before it can delegate);
  - the snapshot mapping per OS (shared memory with a sequence counter) and the `SOCK_SEQPACKET` control socketpair on FreeBSD and Windows;
  - **open feature gap, macOS:** no capping mechanism for a non-owner principal. Candidates investigated by nobody: `RLIMIT_CPU` (kills, does not cap), taskpolicy and `setiopolicy_np` (background throttling, not a quota). Until one caps CPU and memory per principal, macOS has owner-class principals only;
  - **product decision, recorded:** delegation (grants and readers) is Linux-only in v1, by the platform scope table of §3.3;
  - **user decision not taken:** a per-service `instance.charge = false`, which would make an exposed C's crashes charge R_C only and close A4's R_inst route without attribution (REVIEW-7 N10). It changes Theorem R's instance bound, so it is the user's call.
- draft 12 (REVIEW-8). **Two points no opened page states are marked « unverified, test in P1/P4 » and are never asserted;** the design's bounds are stated so as not to depend on either:
  - **Linux subreaper death:** whether the descendants of a dead subreaper go to the next living ancestor subreaper (prctl(2) is silent; the kernel's `find_new_reaper` is from memory). *Unverified, test in P1:* **`TestSubreaperDeathReparent`** (core subreaper → M_p subreaper → child → grandchild; `kill -9` M_p; assert the grandchild's parent is the core and the orphan source reaps it). The N17 bound holds either way (§3.3, a monitor's death);
  - **login class across `execve`** (and the helper's `setuid`): setloginclass(2) states inheritance « from their parents » only. *Unverified, test in P4:* **`TestLoginClassSurvivesExecve`**; FreeBSD delegation stays refused until it passes;
  - from memory, each with its named test (P1 unless stated; *draft 13 moved three of these to opened and corrected the freeze item, see below*): M_p's exact capability set (`TestMonitorCapSet`); `clone3(CLONE_NEWNS)` needing `CAP_SYS_ADMIN` as `unshare` does (`TestMonitorSpawnReadOnly`); a frozen cgroup's processes cannot reap, and `cgroup.kill` kills a frozen cgroup (`TestFreezeBlocksReap`); a zombie counts in `pids.max` until reaped (`TestZombieCountsInPidsMax`); the memcg OOM killer respects `oom_score_adj` −1000 (`TestMonitorNotOomVictim`); a shrink-sealed memfd spares the reader `SIGBUS` (`TestSnapshotSealNoSigbus`); the kernel versions of `clone3` flags, `pidfd_*`, `cgroup.kill`; uid 0's DAC owner match on root-owned files without `CAP_DAC_OVERRIDE` (REVIEW-8 N14, the reason for the split, not a property the design relies on); FreeBSD (P4): the reaper subtree field and its persistence after an orphan's reparenting and after the reaper's death (`TestReapKillSubtreeAfterOrphan`, `TestReapKillSubtreeAfterReaperDeath`), `pdfork`/`pdkill` for the main;
  - **cost, stated:** one more process per delegated principal; O1's observe-drain-reap crosses two processes (M_p peeks, L_p drains and acks, M_p reaps); M_p is root-equivalent on Linux and full root on FreeBSD, bounded by its input surface (64-byte records) and its TCB, not by its privileges.
- **Draft 13 (SECURITY-1), open items:**
  - **opened by SECURITY-1 Part 4, moved out of « from memory »:** `clone3` with `CLONE_NEWNS` needs `CAP_SYS_ADMIN`; the kernel versions of `clone3`, `CLONE_PIDFD`, `CLONE_INTO_CGROUP` and `pidfd_*`; the memcg OOM killer's respect of −1000 (`TestMonitorNotOomVictim` stays as a regression test). **Corrected by Part 4:** pidfd_open(2)'s « no recycling » guarantee is conditional (the pidfd names the process the pid names at the call, which may already be a recycled one), and a frozen cgroup's membership can change; so draft 12's freeze argument is withdrawn, the freeze serves completeness only, and the barrier is the signaller's uid (§3.3 signalling rule);
  - **still unverified, each a named P1 test, and no rule depends on it until it passes:** that a frozen cgroup's processes cannot reap (`TestFreezeBlocksReap`, no longer load-bearing); that a zombie counts in `pids.max` until reaped (`TestZombieCountsInPidsMax`; the N17 count bound uses it, and until it passes the bound is stated as `budget.pids` + `reap.batch` + the zombies of p's at the freeze, which `pids.max` still bounds by population); the kernel version that introduced `cgroup.kill` (probed at load: the file's presence, else `ErrIsolationUnavailable{cgroupkill, linux}`, so no version is relied on); that `pidfd_send_signal` applies `kill(2)`'s uid rule (`TestPidfdSendSignalUidRule`; until it passes the signaller uses `kill(2)` for tree signals); that a seccomp filter is inherited across `fork` and `execve` (`TestGrantedServiceNoInheritedFilter`, which fails closed either way); that `CAP_SYS_RESOURCE` lowers `oom_score_adj_min` with the value (`TestGrantedTreeOomAdj`; until it passes a granted spawn is `perm`); that `F_SEAL_FUTURE_WRITE` refuses new writable mappings and leaves the core's (`TestSnapshotReadOnlyInLane`; confinement does not rest on it); that the Go runtime under the lane's allowlist needs only self-directed `tgkill`/`kill` and `CLONE_THREAD` clones (`TestLaneSeccompAllowlist`, regenerated per toolchain);
  - **deviation from SECURITY-1 S4, stated: M_p carries no seccomp filter.** A filter is inherited by every child and kept across `execve`, and M_p is the parent of every granted tree, so an M_p allowlist would bind every granted service (draft 12's denylist on M_p would already have denied `kill` to them). The only design that filters M_p is a separate unfiltered spawner process between M_p and the trees, which changes the architecture the brief fixes; it is recorded here as the alternative and not taken. M_p is bounded instead by: no `CAP_KILL`, no pid handled as a number, `TestNoRawKill` over `internal/monitor`, its 64-byte input surface and its TCB audit (helper first). ~~This needs the user's confirmation;~~ **Closed in draft 14: the user confirmed « no filter on M_p » for P1; the syscall-surface lint of §7.1.2 is the added bound, and the separate spawner is recorded there as rejected for P1;**
  - **cost, stated:** one short-lived signaller process per graceful signal of grant(p), at p's cost (in `deleg/<uid>/signal/`, under p's caps and `monitor.rate`); the helper becomes a separate binary; the lane's seccomp list must be regenerated on every Go toolchain bump.

**REVIEW-2 resolutions** (none contested):

| finding | resolution |
|---|---|
| N1 | windows read, not evented; watchdog level check |
| N2 | R_p refusals; effect-set check; Theorem G. Draft 5's owner-held charging and proof were withdrawn in draft 6 and replaced by option A in draft 7 (D2, §13) |
| N3 | SW over all dependents; Leaked blocks. *Draft 10 (REVIEW-6 N1): over uncut dependents* |
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
| E14 | edges out of a grant stay allowed; their influence (the length of an outside target's StopWait, ≤ SW, reported as T2 overrun) is named in Theorem G item 2 instead of forbidden, because forbidding them would stop a delegated application from requiring the owner's database. *Draft 10 (REVIEW-6 N1): that influence is removed, G_free and SW no longer read across the cut* | §3.3, §8.2, §10 |

**REVIEW-5 resolutions (draft 9).** No finding is rejected. Where REVIEW-5 offered alternatives, the choice is given.

| finding | resolution | where |
|---|---|---|
| E12 note | **accepted, stated:** the Windows path alphabet has no space, so a parameter such as `C:\Program Files\…` cannot be passed as a parameter; such a path must be written in the recipe itself. Adding the space would reopen argument splitting in shell strings, which the alphabet exists to prevent | §12 |
| F1 | new **H_OS-E5**: svd's own resources partitioned per principal — per-principal listeners with round-robin accept and `budget.connections/inflight/refusals`; fd headroom and `ErrResourceBudget`; bounded per-source event queues with round-robin dequeue and O5; credential-only notify control buffer; per-principal pids/memory (Linux cgroup + RLIMIT_NPROC, FreeBSD rctl, macOS rlimits, Windows nested job), `ErrIsolationUnavailable` with no opt-out. Unverified mechanisms are listed above. Chaos test | §1.7, §1.10, §3.1, §3.3, §8.1, §10 |
| F2 | runs_as = resolved uid/SID and exact group set; load-time check and pre-fork re-resolve (`ErrAccountChanged`); helper verifies ids after setuid; `ErrDelegationPrivilege` for capabilities, nnp, groups, uid 0. A1 restated as probes, A2 as disjointness predicates (a file owned by p counts as p-writable, which covers setuid-p binaries; forcing NNP on outside services was rejected for that reason) | §1.6, §1.10, §3.3, §10 |
| F3 | the recommended fix: `cut(A′, A)` truncates propagation at every grant boundary for **every** holder, owner included; D(X), `--with-dependents` and T computed on uncut edges; `WarnUnheldOutsideTarget`; the effect-set check is a real refusal, never a panic (line 652 text replaced); Theorem G proof step (b) corrected | §2.3, §2.4, §3.3, §3.7 I4 |
| F4 | three principal classes (owner-class, delegated, reader); `ErrPrincipalClass`; the reader direction of `ErrDelegationUid` | §1.6, §8.2 |
| F5 | P2 split: P2a (abstract time, Gate 1, with Leaked and finite operator restarts) and P2b (`ErrNeverFailingLoop`, window > max × U(A); U renamed V(A) in draft 10, REVIEW-6 N5) | §3.7, §3.8 |
| F6 | `Absent` Reach value and `Prio` per guard cell; 15 present, 73 Absent; Failed × G_ok carries `ReasonRecoverable ∧ GoalUp`; termination bound corrected from 3 to 2 moves (draft 8's 3-move path did not exist); Gate 2 mutates priorities | §3.5, §3.8 |
| F7 | event-table headers renamed StartTO and StopTO, matching the prose | §3.4 |
| F8 | fixed in the cells, not only documented: Stopping, Killing and Leaked × X1 record `perm` from the status pipe | §3.2, §3.4 |
| F9 | the API refusal check and the cell's `pay` are one payment | §3.2, §3.4 |
| F10 | cgroup hand-over to a principal withdrawn (H_OS-E2) | §3.3, §8.2 |
| F11 | notify `recvmsg` control buffer sized for credentials only; `MSG_CTRUNC` drops the message | §3.3, §10 |
| Gate 1 | Theorem G item 1 checked as 2-safety by self-composition (product BFS, low inputs to both copies, high to one, oracle as a low input, N ≤ 3 with symmetry); red on `goalDraft8`, green on draft 9, red on the want-closure and `pay(caller)` mutants | §3.8 |
| G1 (new, found by the 2-safety re-proof) | an owner Retry of A ∈ grant(p) paid R_inst, or not, according to whether A was Failed, a state p controls: p could thereby move R_inst. The Retry cell now pays payer(A); an owner payment to R_p is never refused and never escalates | §3.3, §3.4, §3.7 |

**Rules `Compile` must newly enforce in draft 9 [Compile+9]:** `ErrPrincipalClass`; the reader direction of `ErrDelegationUid`; `ErrDelegationPrivilege` for `capabilities`, `nnp` and `groups`; `ErrNeverFailingLoop` (P2b); the E5 budget fields in Q, whose absence is `ErrIncomplete`; the warning `WarnUnheldOutsideTarget`. **New host checks [Host+9]:** the resolved-id check at load and `ErrDelegationPrivilege{…, uid0}`; `ErrAccountChanged` before each fork; `ErrResourceBudget`; `ErrIsolationUnavailable`. The cut in goal propagation, the Retry payer and the cell changes are runtime and FSM data, not `Compile`. go-supervise/supervisor implements draft 8's `Compile` and is not changed by this draft.

**REVIEW-6 resolutions (draft 10).** No finding is rejected.

| finding | resolution | where |
|---|---|---|
| N1 (HIGH) | G_free and SW read only **uncut** dependents; the oracle is withdrawn; T2 is promised towards uncut dependents only; the E14 cost moves to p. The defect class is closed by Theorem G's inventory H1–H9 of every input p controls, each confined, cut or assumed; H9 (what p's processes send to O's services outside svd) is the new H_OS-A4 with `WarnExposedToGrant` | §3.3, §3.5, §3.6, §3.7, §8.2, §10 |
| N2 | O5′: before a timer cell is delivered, svd drains the sources of every service the cell reads (WTO: C and req(C)); Gate 1 checks each timer cell's read set statically | §3.1, §3.8, §10 |
| N3 | caps per principal: CPU, IO, swap, log back-pressure (Linux `cpu.max`, `io.max`, `memory.swap.max`; FreeBSD rctl; macOS grants refused; Windows jobs, unverified); capacity sums; per-principal lanes so svd's own work for p runs in p's cgroup; O3 now cites E5 ∧ A2(host); the shared-hardware residual stated as H_OS-A3 with Λ and a measuring chaos test | §3.1, §3.3, §8.1, §9, §10, §11 |
| N4 | one request holder per (principal, service), reused by repeated `start`; at most \|S\| × (1 + \|principals\|) holders; one open transaction per holder; audit ring and size-rotated audit file | §2.3, §2.4, §10 |
| N5 | P2b's U(A) renamed V(A) | §3.7, §10 |
| N6 | a defensive walk closes every `SCM_RIGHTS` fd after each `recvmsg`, whatever the kernel did on truncation | §3.3, §10 |
| 872 | the upward read of G_free is justified by invisibility: StopWait and Stopping read alike to every guard that reads the service | §3.5 |
| F3 costs | stated: the owner must start or enable C explicitly; an owner `stop C` tears down A ∈ grant(p) that requires C, and C no longer waits for A | §2.3, §10 |
| Gate 1 gaps 1–5 | (1) unequal enabledness is red, tree bit and generation-match added to obs_O; (2) symmetry preserves G, O and holder classes, jointly on both copies; (3) p's request deadline is high, defined in §2.4; (4) oracle removed, `freeAcrossCut`/`swAcrossCut` witnesses; (5) F1 chaos test extended to floods, CPU and IO load and READY near WaitTO, measuring Λ | §3.3, §3.8, §10 |

**Rules `Compile` must newly enforce in draft 10 [Compile+10]:** `budget.cpu`, `budget.io`, `budget.swap` and `budget.log` in Q for every delegated principal, and `budget.memory` and `budget.cpu` for every reader; `instance.reserve.cpu` and `instance.reserve.memory` in Q when any non-owner-class principal exists; new class-T fields `api.request.timeout`, `api.response.max`, `audit.ring`, `audit.file.size`, `audit.file.count`; new class-G field `instance.timing.lambda`; SW computed over uncut dependents, and the G_free dependent index built from uncut edges; `ErrNeverFailingLoop` stated on V(A); the warning `WarnExposedToGrant{C, p, terminal}`; `svd plan` warns when an O timer is below 100 × Λ. **New host checks [Host+10]:** Linux: the `pids`, `memory`, `cpu` and `io` controllers enabled for svd's subtree and every `io.max` device resolved, else `ErrIsolationUnavailable{cpu|io, linux}`; FreeBSD: delegation refused without racct (the `RLIMIT_AS` fallback is withdrawn); macOS: every grant refused, `ErrIsolationUnavailable{cpu, macos}`; capacity sums, `ErrResourceBudget{cpu|memory}`; a lane that dies is not restarted automatically.

**REVIEW-7 resolutions (draft 11).** No finding is rejected.

| finding | resolution | where |
|---|---|---|
| user decision + N10 | `ErrExposedEscalates{C, p, T, path}` for every exposed C in Esc (reached by a require path from a terminal T, length ≥ 0); `WarnExposedToGrant{C, p}` for the others, `terminal` flag removed, R_inst route stated; Theorem G stated plainly (« unless p can crash an outside service it can reach »); deleting an edge hides the exposure, it does not remove it; `instance.charge = false` left to the user | §1.10, §3.3, §8.2, §10, §12 |
| N7 | the lane is a subreaper (`PR_SET_CHILD_SUBREAPER`, `PROC_REAP_ACQUIRE`); the core observes O's exits per epoch, never by P_ALL; a bounded orphan source; lane-death reparenting stated; inventory row **H10**; fork-and-exit flood in the chaos test | §3.3, §5.1, §10 |
| N8 | lane protocol: non-blocking enqueue with `ErrLaneBusy`; forwarded operations answered on enqueue with a ticket, the deadline and outcome in the lane; the core reads nothing from the control fd; snapshot reads bounded and validated; item 1's equal enabledness restored and its scope (outcome of owner operations on G) stated; inventory row **H11**; starved-lane chaos run | §1.10, §3.1, §3.3, §10 |
| N9 | the lane runs as root with reduced capabilities, capped by its container (cgroup on Linux; `loginclass:svd-<uid>` rctl on FreeBSD), so E2 holds and the lane is capped wherever grants are allowed; the cost (root parser of p's input) stated | §3.3, §10, §12 |
| N11 | serialisation lemma restated as a refinement of the O projection; I1, T1, T2 hold towards cut targets up to the snapshot lag, at p's cost | §3.1 |
| N12 | `freeAcrossCut` fixture: C's goal stays up, B ∈ O crashes, C's exit in StopWait is charged by settle rule 4, `mode` diverges | §3.8 |
| N13 | `swAcrossCut` removed from Gate 1's witnesses; a `Compile` unit test instead | §3.8, §10 |
| N4 note | the audit file is per process | §2.4 |
| Part 4 | unix(7): `chown` of the path, `lstat`-verified, fd handed to the lane; BSD socket permissions not relied on (FreeBSD: directory ACL). rctl(8): `:deny` for `maxproc`, `memoryuse`, `swapuse`, `pcpu`, `:throttle` for the four IO resources; rctl(4): `kern.racct.enable` is a loader tunable | §3.3, §8.1, §12 |
| Part 5 / N3 scope | one platform table (grants, readers, cap, subreaper, listener, Theorem G per OS); macOS readers refused, the uncapped-reader claim withdrawn; delegation Linux-only in v1 | §3.3, §8.1, §10, §12 |

**Rules `Compile` must newly enforce in draft 11 [Compile+11]:** `ErrExposedEscalates{service, principal, terminal, path}` for every service exposed to a grant (an edge of any kind from the grant) that a terminal service reaches by a require path of length ≥ 0; `WarnExposedToGrant{service, principal}` without the `terminal` flag for every other exposed service; new class-T fields `lane.queue`, `lane.snapshot.retries` and `reap.batch`. **New host checks [Host+11]:** a lane that cannot become subreaper, `ErrIsolationUnavailable{reaper, os}`; the lane's capability set reduced before it reads p's input; the Linux listener path `chown`ed and `lstat`-verified before `listen`; FreeBSD: the login class `svd-<uid>` on the lane and every granted tree, rctl rules on it with the corrected actions, `kern.racct.enable` read at load and readers refused with grants when it is 0, a listener directory without ACL support `ErrIsolationUnavailable{listener, freebsd}`; macOS: every reader refused, `ErrIsolationUnavailable{cpu, macos}`; Windows: grants and readers refused until P3. go-supervise/supervisor is not changed by this draft.

**REVIEW-8 resolutions (draft 12).** No finding is rejected. REVIEW-8's recommendation (privilege split) is adopted; its weaker alternative (one root lane with seccomp argument filters) is not, because N15 would still force `CAP_SYS_ADMIN` into the process that parses p's bytes.

| finding | resolution | where |
|---|---|---|
| N14 (MEDIUM) | draft 11's root lane withdrawn. **L_p**: dedicated uid from `lane.uids`, no capability, NNP, seccomp, Landlock or `cap_enter`, pre-opened fds only; parses every byte p writes. **M_p**: root, Linux set {`CAP_SYS_ADMIN`, `CAP_SETUID`, `CAP_SETGID`, `CAP_SETPCAP`, `CAP_KILL`}, stated root-equivalent; FreeBSD full root, stated; reads only 64-byte versioned records, every field range-checked, total three-outcome decision (act, refuse with `NACK`, fault); TCB named and kept small (`internal/monitor`, import-closure test). Readers: lane only, no capability | §3.3 H_OS-E5, §10, §12 |
| N15 (MEDIUM) | M_p forks the helper with `clone3(CLONE_NEWNS \| CLONE_PIDFD \| CLONE_INTO_CGROUP)`, holding `CAP_SYS_ADMIN`; the core does so for O. `clone3` `EPERM` is SEp, a remount `EPERM` is `perm`, so neither is charged | §3.1 O3/O4, §3.3, §7.1, §10 |
| N16 (MEDIUM), H12 | signalling rule for every svd process: pidfd for the main, `cgroup.kill` for a tree, freeze-then-enumerate-then-pidfd for the rest; FreeBSD `PROC_REAP_KILL` + `REAPER_KILL_SUBTREE`; seccomp denies `kill`/`tkill`/`tgkill` in M_p; inventory row **H12** (cut); Theorem G (b) « its own events » covers svd's own deputies; pid-cycling chaos run with a fault-injected control that must go red | §3.3, §10 |
| N17 (LOW) | the trees' parent is M_p, so L_p's death orphans nothing; at M_p's death the core drains the monitor-death source first and **freezes** `deleg/<uid>/` before `cgroup.kill`; bound stated as a count, `budget.pids` + `reap.batch`; M_p allocates nothing after start and has `oom_score_adj` −1000; FreeBSD bound stated as `maxproc` plus one round's forks | §3.3, §10 |
| N18 a (LOW) | the orphan source skips every child the core tracks (epoch mains, lanes, monitors); monitor- and lane-death sources drain first | §5.1, §10 |
| N18 b (LOW) | FreeBSD: a dead reaper's descendants go to its reaper (procctl(2)); the orphan source runs whenever the core is a reaper of any kind; in mode B the core makes itself a (sub)reaper at start | §5.1, §10 |
| N19 (LOW), H13 | relayed bytes marked `"untrusted"` with provenance in the API; the CLI escapes non-printable bytes and bidi overrides and prefixes provenance; inventory row **H13** (confined for obs_O, cut at the terminal) | §3.3, §8.3, §10 |
| N20 a | Linux row: « N3 closed » now holds because M_p creates the mount namespace | §3.3 table |
| N20 b | FreeBSD row: M_p full root, stated; tree tracking by reaper subtree with one reaper per spawning process (§5.1 table corrected) | §3.3 table, §5.1 |
| N20 c | tickets moved to `/v1/tickets/<ticket>`; the deadline lives in the ticket under the core's HMAC, and the core answers `expired` statelessly; the CLI stops at the deadline with backoff | §3.3, §8.3, §10 |
| N20 d | reader lanes hold no capability | §3.3, §10 |
| N20 e | the table's scope: system instance in modes C and B; Linux without a writable cgroup refuses delegation (`ErrIsolationUnavailable{cgroup, linux}`) | §3.3, §10 |
| Part 3 | the N7 test's parent-pid assertion dropped (a p process may make itself a subreaper) | §10 |
| Part 4 | prctl(2), procctl(2), rctl(8) `loginclass` aggregation and setloginclass(2)'s root requirement moved to opened; « future children only » and « the reaper's reaper » applied; subreaper-death reparenting and `execve` inheritance of the login class marked unverified with named tests | §3.3, §5.1, §12 |

**Rules `Compile` must newly enforce in draft 12 [Compile+12]:** class-G field `lane.uids` (a uid range) in Q whenever a non-owner-class principal exists (`ErrIncomplete`); new class-T fields `monitor.batch` and `freeze.timeout`. **New host checks [Host+12]:** each lane uid is distinct from 0, from every principal's uid and from every service's resolved uid (`ErrLaneAccount{principal, uid, reason}`); L_p starts with empty capability sets, NNP, its seccomp filter and Landlock (Linux) or `cap_enter` (FreeBSD) before it reads any input; M_p's capability set is exactly the stated one, its seccomp filter denies `kill`/`tkill`/`tgkill` *(superseded in draft 13: M_p has no filter and no `CAP_KILL`)*, it becomes subreaper or reaper before its first fork, and its `oom_score_adj` is −1000 *(kept in draft 13, but no process M_p forks keeps it, S1)*; the cgroup layout `deleg/<uid>/{monitor, lane, trees/}` *(draft 13 adds `signal/`)*; Linux kernel support for `clone3` with `CLONE_PIDFD` and `CLONE_INTO_CGROUP`, `pidfd_open`, `pidfd_send_signal`, `cgroup.kill` and `cgroup.freeze`, else `ErrIsolationUnavailable{pidfd|cgroupkill|freeze, linux}`; delegation refused where trees are tracked by process group (`ErrIsolationUnavailable{cgroup, linux}`); the core a (sub)reaper in mode B, with its orphan source running whenever it is a reaper; the snapshot memfd sealed against shrink and growth before it is handed to a lane; the A1 probes also run against L_p's uid and M_p. go-supervise/supervisor is not changed by this draft.

**SECURITY-1 resolutions (draft 13).** No finding is rejected; one recommendation (an allowlist on M_p, S4) is met differently, as stated in the open items above.

| finding | resolution | where |
|---|---|---|
| S1 (HIGH) | M_p keeps −1000; every process it forks writes oom 0 with `CAP_SYS_RESOURCE` before its id drop (value and floor), the core does the same for L_p; `TestGrantedTreeOomAdj` | §3.3 monitor's death, §7.1.1 step 8, §10 |
| S2 (HIGH) | per-principal root-owned 0711 notify directory, root-owned ancestors; bind, `fchownat`, `fstatat`-verify and a single `unlinkat`, all relative to the dirfd; nothing p can write is walked or deleted; space bounded by live epochs; `TestNotifyDirNotOwnedByGrantee` with a link-planting chaos run | §4.1, §8.1 |
| S3 | the signaller: a helper in signal mode with p's credentials and no capability sends every graceful signal; M_p loses `CAP_KILL`; the freeze serves completeness only; `TestPidfdSendSignalUidRule`, `TestSignallerCredentials`, `kill(2)` fallback | §3.3 signalling rule, H12 row, Linux platform row |
| S4 | L_p: generated allowlist, `KILL_PROCESS` default, TSYNC, self-only `tgkill`/`kill` on all 64 bits, `CLONE_THREAD`-only `clone`, `clone3` → `ENOSYS`, `rt_sigqueueinfo` closed; signaller: own allowlist; M_p: no filter (deviation, §12 open items) | §3.3 privilege row, §10 |
| S5 | no keep-caps; bounding, inheritable and ambient dropped and securebits locked **before** `setresuid`; verification after; `TestHelperDropOrder` | §2.2 F2a, §7.1.1 steps 13–17 |
| S6 | lane gid = uid = n, no groups, host account / subid / in-use checks, unique, persisted, never reissued; L_p owns nothing (read-only mounts, new IPC and net ns) | §1.10 `ErrLaneAccount`, §3.3 uid row |
| S7 | `MSG_DONTWAIT` sends from a fixed ring; reading stops when it is full; freeze wait is an event; `monitor.rate` and one outstanding signal per service; cgroup-lock contention measured into Λ | §3.3 record channel, §10 |
| S8 | read-only regions by fd (`open_tree`, `mount_setattr`, `move_mount`), verified by mount id; kernel support probed (`mountapi`) | §7.1.1 step 10, §7.2, platform row |
| S9 | one snapshot memfd per lane, `F_SEAL_FUTURE_WRITE`; confinement does not rest on it | §3.3 lanes |
| S10 | TREE_EMPTY after populated 0 and reaping; `svd lane restart` thaws after verifying empty | §3.3 |
| S11 | `MS_SLAVE \| MS_REC` instead of `MS_PRIVATE`, fatal before any mount | §7.1.1 step 9 |
| S12 | `close_range` in the helper; the service's fd table is the fixed set | §7.1.1 step 6 |
| S13 | record version 2 with plan `generation`; `EACCES`, `EBUSY`, `EOPNOTSUPP` from `CLONE_INTO_CGROUP` are SEp; epoch cgroups stay leaves | §3.3 record format, §3.1 O3 |
| Part 1, 3 | the Go runtime and `syscall` packages are in the TCB; the helper is a separate binary measured separately; audit order helper → signalling → decoder | §3.3 TCB, §10 audit |
| Part 4 | three items moved to opened, two corrections applied (pidfd_open, frozen membership), the rest named P1 tests with no rule depending on them until they pass | §3.3, §12 open items |

**Rules `Compile` must newly enforce in draft 13 [Compile+13]:** the class-T field `monitor.rate` (per service and per non-`SPAWN` record kind) in Q whenever a delegated principal exists (`ErrIncomplete`); record version 2 with the plan `generation`. **New host checks [Host+13]:** `ErrLaneAccount` reasons `host-account`, `subid`, `in-use` and `exhausted`, and the persisted, never-reissued id map; L_p's gid, empty groups, read-only mount namespace, new IPC and network namespaces, `PR_SET_DUMPABLE` 0 and allowlist filter with TSYNC; the core's oom 0 for L_p and the helper's oom 0 before every id drop; M_p's set C_M (no `CAP_KILL`, with `CAP_SYS_RESOURCE`) and no filter; the signaller's sequence; the helper sequence of §7.1.1 in its order, its verification and its fd hygiene; the notify directory's ownership, mode and ancestors (`ErrUnsafePath`); Linux kernel support for `open_tree`, `move_mount` and `mount_setattr` (`ErrIsolationUnavailable{mountapi, linux}`); the cgroup layout `deleg/<uid>/{monitor, lane, signal/, trees/}`; one snapshot memfd per lane with `F_SEAL_FUTURE_WRITE`; `svd lane restart` thaws only empty subtrees. go-supervise/supervisor is not changed by this draft.

**Draft-14 open items.**

- *(Answered in draft 15.)* The `-E` build claim was built by SECURITY-1 Part 7 (T13, `evidence-part7/`): it links, and stays single-threaded only under the shape §7.1.1 item 4 now states; Gates 1 and 2 with their red controls replace « `TestDropStageSingleThread` is the gate ».
- Seven kernel claims are tests, not opened sources: `TestLandlockSurvivesExecve`, `TestNotifyBindViaProcFd`, `TestLaneEmptyRoot` (`pivot_root(".", ".")`), `TestPidfdSendSignalUidRule` (draft 13), `TestPidfdQuietAcrossHelperExec` (draft 15, T19: M_p's pidfd stays quiet when the helper execs from a non-leader thread; whatever it does, a readable pidfd is confirmed by `waitid(P_PIDFD, WNOHANG|WNOWAIT)` before the epoch is declared dead), and the claims that `clone3` with `CLONE_INTO_CGROUP` from Go's fork path behaves as `exec_linux.go` reads and that `open("/dev/tty")` fails `ENXIO` without a controlling terminal (`TestHelperNoCtty`).
- **For the FSM implementer:** T8 changes the **order** of M_p's decoder checks (§3.3, « Order of the checks »), not its outcomes or any table of §3; a decoder written from draft 13's bullet order calls a stale-plan record a Fault where draft 14 calls it a Refuse.

**SECURITY-1 Part 6 resolutions (draft 14).** No table of §3 is changed. Two alternatives proposed in Part 6 are rejected with reasons; every finding is otherwise applied.

| item | resolution | where |
|---|---|---|
| T1 (tty, `TIOCSTI`) | new session in Go's fork child, fd 0 `/dev/null`, fds 1–2 the log pipe, no tty fd; `tiocsti()` refuses a non-controlling tty without `CAP_SYS_ADMIN` (`tty_io.c`); `TestHelperNoCtty` | §7.1.1 C1, H1, T1 paragraph |
| T2, S3, S5 (per-thread drop) | every id, capability and per-thread attribute set in `svd-drop`, one thread from `execve` to `execve`; `PR_SET_DUMPABLE` 0 first and after each credential change; `fs.suid_dumpable` = 1 refused; `AllThreadsSyscall` rejected (mixed credentials during the sweep) | §7.1.1 D1–D14, whole-process paragraph |
| T3 (missing rows) | credentials (H7), rlimits with `RLIMIT_NPROC`, umask, cwd, signal mask and dispositions (D5), argv and envp from the plan only (H9 frame) | §7.1.1 |
| T4 (filter placement) | filter is D13, the last call before `execve`; preflight keeps `perm`; residual race `fail`; `ErrSeccompNoExec`. **Rejected:** « death by signal = `perm` » (would change the O4 table; cause removed; p's own cost under D2 option A) | §7.1.1 |
| T5 (rollback) | reverse-order rollback of M2–M4; own `EEXIST` removed once and retried. **Rejected:** consuming the epoch (changes the protocol and L_p's epoch arithmetic) | §7.1.1 |
| T6 (fork path) | `syscall.ForkExec` with `Setsid`, `CLONE_NEWNS`, `UseCgroupFD`, `PidFD`; exec via `/proc/self/fd/N`; helper and service fd sets; M_p's fds all `O_CLOEXEC` | §7.1.1 M3–M4, C1, H10, D12 |
| T7 (region fds) | regions opened once by the helper after H5; `MOVE_MOUNT_F_EMPTY_PATH` added; no `/proc` oom re-read under Landlock | §7.1.1 H6, D10 |
| T8 (decoder order) | generation checked before every plan-dependent Fault; stale generation is Refuse | §3.3 |
| T9 (lane root) | empty read-only tmpfs root by `pivot_root`, the old root detached; L_p reaches files only through plan fds | §7.1.1 H8 |
| T10 (notify bind) | bind through `/proc/self/fd/<dirfd>/name`; `sun_path` bound (`ErrNotifyNameTooLong`, ≤ 61-byte service name; absolute path ≤ 107) | §4.1 |
| T11 (`CAP_KILL`) | stated: uid 0 still signals every uid-0 process under A1; the barrier is `TestNoRawKill` and the lint, not the kernel | §3.3 |
| item 3 (signaller) | `signal.timeout` named; `signal/cgroup.kill` after every signaller; strict `cgroup.procs` parse; `svd signal` drops dumpable and stacks a filter without exec | §3.3 steps 2–6 |
| item 4 (M_p filter) | M_p unfiltered for P1 (user decision); syscall-surface lint, source and dynamic halves; separate spawner rejected because a compromised M_p dictates its plan | §7.1.2 |

**Rules `Compile` must newly enforce in draft 14 [Compile+14]:** the class-T field `signal.timeout` in Q whenever a delegated principal exists (`ErrIncomplete`); `ErrSeccompNoExec{service}` for a plan filter that denies `execve` or `exit_group`; `ErrNotifyNameTooLong{service}` for a granted service name over 61 bytes. **New host checks [Host+14]:** `fs.suid_dumpable` ≠ 1 at load and in H4 (`ErrIsolationUnavailable{suiddumpable, linux}`); the absolute `NOTIFY_SOCKET` path ≤ 107 bytes (`ErrUnsafePath`); the spawn path M1–M4, C1, H1–H10, D1–D14 in its order, with one thread in D1–D14; every fd of M_p `O_CLOEXEC`; the rollback of M2–M4; the decoder check order of T8; the lane's empty root; the signaller's residue kill; the syscall-surface lint of §7.1.2 in CI. go-supervise/supervisor's `Compile` gains the three [Compile+14] rules; the FSM is unchanged.

**SECURITY-1 Part 7 resolutions (draft 15).** No table of §3 is changed, and `Compile` gains no rule. Every finding of Part 7 is applied; none is rejected.

| item | resolution | where |
|---|---|---|
| T6 residual | H1 marks every fd ≥ k close-on-exec with `close_range(k, ~0U, CLOSE_RANGE_CLOEXEC)`, so the O_PATH fd it still needs survives until H10 | §7.1.1 H1 |
| T7 residual | D10 no longer reads `/proc/self/oom_score_adj`; the oom value is set and verified before the drop, as draft 14 states | §7.1.1 D10 |
| T12 (keyring) | D5b `keyctl(KEYCTL_JOIN_SESSION_KEYRING, NULL)` as R in every mode, so the service starts with a fresh empty session keyring; `TestServiceKeyringIsFresh` | §7.1.1 D5b |
| T13 (`svd-drop`) | kept as a Go-toolchain binary of a stated shape: `-E main.dropEntry`, static `fakeg`, `prctl` first (amd64: `arch_prctl(ARCH_SET_FS)` second), nosplit subset, `main.main` refuses, non-PIE `ET_EXEC`, build by `cmd/buildstage` only, ELF shape checked at load; Gate 1 (reachability on the artifact) and Gate 2 (ptrace, exact sequence, one task) with red controls (a)–(f) in `TestDropStageGatesCatch` | §7.1.1 item 4 |
| T14 (lint) | lists A_M^start / A_M^steady reconciled with M3 (`wait4`, `memfd_create`), R_go from a reference program; source half by whole-program reachability with constant numbers; dynamic half **enforcing** (`SECCOMP_RET_KILL_PROCESS` harness filter) plus a lossless ptrace recorder; symbol-absence half; negative control `TestMonitorSyscallSurfaceCatches` | §7.1.2 items 0–3 |
| T15 (capabilities) | D9a `capset` to the plan's set K (∅ by default) and its ambient raise, independent of the uid fixup; D10 verifies K; `setresuid(0)` probe only when u ≠ 0 | §7.1.1 D8–D10 |
| T16 (state) | (a) the frame carries every rlimit, `RLIMIT_NPROC` soft = hard (D5); (b) D5a resets policy, nice, I/O priority, affinity, timer slack, personality; (c) the cwd check moves to D11 (`faccessat2`) | §7.1.1 H9, D5, D5a, D11 |
| T17 (files) | helper and stage opened once from a versioned root-owned install directory by `openat2`; frame magic and layout digest checked by D2 (`ErrStageMismatch`); the helper's `O_PATH` fd is the last entry k of `Files`, exec'd as `/proc/self/fd/k` | §7.1.1 M4, H9, D2, « Which files » |
| T18 (last call) | normal and signal modes exec with `syscall.Exec`; lane mode keeps a raw `execveat`, its residual exit 2 is `fail` under E11 and the stage's fixed codes avoid {1, 2}; the plan reaches the helper in a sealed memfd; `TestHelperExecLocked`, `TestHelperExecFdPinned` | §7.1.1 M3, H2, H10 |
| T19 (small) | TSYNC on `svd signal`'s second filter (supersedes §3.3 step 4's wording); `MFD_NOEXEC_SEAL` on every memfd; `TestPidfdQuietAcrossHelperExec` and the `waitid` confirmation | §7.1.1 modes, H9, « The pidfd »; open items |

**New host checks [Host+15]:** svd's Linux build only on amd64 and arm64; `svd-drop` built by `cmd/buildstage` only, reproducible, its ELF shape checked at load (`ErrIsolationUnavailable{dropstage, linux}`); frame magic and layout digest (`ErrStageMismatch`); helper and stage opened once from the versioned install directory; the session keyring replaced (D5b); every rlimit and the D5a scheduling state set; K set and verified (D9a, D10); the plan and the frame in sealed `MFD_NOEXEC_SEAL` memfds; `close_range` with `CLOSE_RANGE_CLOEXEC` in H1; `svd-monitor` as its own executable; Gates 1–2 of §7.1.1 and the three halves of §7.1.2 with their negative controls in CI.

**Draft-15 decisions for the user.**

1. **`svd-drop` stays in Go**, relying on linker behaviour (`-E` to an assembly entry that never initializes the runtime) that Go does not promise; the gates make a toolchain change a red build, not a silent regression. The alternative is a freestanding C stage under the same two gates (a second language and compiler in the TCB).
2. **svd on Linux is amd64 and arm64 only** until a stage entry is written and gated for another architecture.
3. **M_p ships as its own executable `svd-monitor`** (a packaging change), so that the symbol-absence half reads exactly M_p's code. If refused, half 3 runs on `svd` and becomes a report, not a gate.
4. **T15 semantics for O's services:** a root or capability-holding service of O gets exactly the plan's K (empty unless named), with `SECBIT_NO_SETUID_FIXUP` only when u ≠ 0; a service of O that relied on inheriting root's full set must name it.
5. **The service's session keyring is new and empty**, not linked to any user keyring; a service of O that read root's session keyring (for instance a `KEYRING:` Kerberos ccache) loses it and must be given its credential another way.

**Draft-15 open items.** Gate 2 has run only under emulation on amd64: it must run natively there before `svd-drop` merges. Six claims are from memory, not opened, each the subject of a named test or control: the linker's nosplit stack-limit check (control (f)); first-pass `dup3` clobbering in Go's fork child (`TestHelperExecFdPinned`); the runtime's exec lock (`TestHelperExecLocked`); `vm.memfd_noexec` and `MFD_NOEXEC_SEAL`; pidfd behaviour across an exec by a non-leader thread (`TestPidfdQuietAcrossHelperExec`); the C-compiler hazards weighed in decision 1. The lane-mode raw `execveat` race remains, bounded and charged `fail`. The dynamic half of §7.1.2 sees only what the suite drives.

**REVIEW-1 contested points.** None of the findings is contested. Two are resolved differently from the review's suggestion:

- **C1.** The review proposed making `api.*` and `exempt_on` required-concrete. We prove (Theorem N) that no orientation can make absence fail closed, and use a mandatory **total** baseline plus completeness. This covers every hardening field, not only those two.
- **M5.** Rollback on Failed(wait) is kept as intended semantics. The fix is automatic recovery from Failed(wait) and Failed(dep), plus `ErrWaitTooShort`.

---

## 13. Decision D2: what delegation may cost the instance — DECIDED, option A

**Decision (user, 2026-11-23): option A.** A delegated service pays its own crashes, and a grant must include every service that has an edge of any kind to a granted one (`ErrGrantNotClosed`, §8.2; extended from require to every kind by the user's REVIEW-4 E2 ruling). A service runs as a delegated principal exactly when it is in that principal's grant (E1). Theorem G is stated and proved in §3.3, conditional on the assumed part of H_OS (E6). The text of options A–C below is kept as weighed in draft 6; where it says "reverse require", draft 8 reads "reverse of every edge kind". Draft 9 further reads it with the grant-boundary cut on goal propagation for every holder (F3, §2.3) and with svd's own resources partitioned per principal (H_OS-E5, §3.3); where option A says "Retry pays the caller", draft 9 reads "Retry pays payer(A)". Draft 10 reads it with no oracle: G_free and SW of an outside service ignore its granted dependents (REVIEW-6 N1), svd's work for each non-owner principal runs in that principal's lane, and Theorem G rests on an explicit inventory of p's inputs with A3 (shared-hardware timing) and A4 (exposed outside services) as the stated residual hypotheses (§3.3). Options B and C are kept below as the record of what was weighed.

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
