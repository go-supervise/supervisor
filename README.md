# supervisor (`svd`)

`svd` is a cross-platform service supervisor (Linux, macOS, Windows).

**Status: design, with phase P0 implemented.** The authoritative design is
[docs/design/SUPERVISOR.md](docs/design/SUPERVISOR.md) (draft 7); the
bibliographies and reviews it rests on sit beside it in `docs/design/`.

Phase P0 is `Compile`: a pure function from layers (recipes and site files,
HCL, literals only) to a resolved plan or a typed refusal. It meets the layers
in the lattices of section 1 (flat, interval, bound, protection, allow-set,
demand-set, edge), applies the authority table, the widening check, the
completeness predicate, the wait-budget and grant predicates (including
REVIEW-4 E1 and E2), and the schema-evolution rule. The result does not depend
on the order of the layers. Nothing is supervised yet: no process is started.

License: BSD 3-Clause.
