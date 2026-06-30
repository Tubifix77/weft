# CLAUDE.md — build contract for the Weft runtime

You are implementing the **Weft interpreter** (`weft` / `weft.exe`): a program that executes `.we` files.

## Read first, in order
1. `ARCHITECTURE.md` — the language specification (what Weft is, why each decision was made).
2. `RUNTIME-ARCHITECTURE.md` — the build spec for this runtime (how to run it). **This is your contract.** It pins the host language, the v0 scope, the data structures, the execution algorithm, every opcode's behaviour, the error model, the CLI, the module layout, the build order, and the definition of done.
3. `ITERATION.md` — the **v1** spec: sub-streams (`fn`/`ret`), `map`, `fold`, and the capture rule. Build this only after the v0 core is green.
4. `examples/` — golden test programs and `examples/expected.json` (the source of truth for correctness).

## The job in one paragraph
Write a line-walking interpreter in **Go** that reads a `.we` file one instruction per line, keeps the environment as a slice indexed by handle number (`env[K]` = value of `rK`), dispatches each opcode to a handler, and prints `out` values to stdout — or emits one structured JSON error to stderr and exits non-zero. It compiles to a single dependency-free `weft.exe`. Follow the build order in `RUNTIME-ARCHITECTURE.md` §12 for v0, then `ITERATION.md` for v1. Do not start in any language other than Go. Do not build the constrained decoder (separate project, out of scope).

## Scope: two phases
- **v0 (core, build first):** `lit.i lit.f lit.b lit.s`, `add sub mul div mod`, `eq ne lt le gt ge`, `and or not`, `sel`, `vec len idx`, `i2f`, `chk`, `out`.
- **v1 (iteration, build after v0 is green):** `fn`/`ret` sub-streams, `map`, `fold` — fully specified in `ITERATION.md`. Adds the `KClosure` value kind, the 1-bit `inFn` state, and error codes `NEST`, `BAD_CLOSURE`, `BAD_ARITY`, `NO_RET`.
- **Out of scope entirely:** `scan` and nested iteration (a sub-stream cannot contain `fn`); the constrained decoder; any performance work.

## Two language decisions resolved in the runtime spec (already reflected in the examples)
- `chk`'s second operand is an **inline bareword code**, not a handle.
- A failed `chk` raises code **`CHK_FAIL`** and carries that bareword as its single operand.
These should eventually be folded back into `ARCHITECTURE.md` §5/§9; note it, don't block on it.

## Golden tests — what each example must do
Canonical machine-readable form is `examples/expected.json`. Human-readable summary:

| File | Phase | Exits | Produces |
|---|---|---|---|
| `arith.we` | v0 | 0 | stdout: `16` |
| `max.we` | v0 | 0 | stdout: `7` |
| `vector.we` | v0 | 0 | stdout: `3` then `20` |
| `guard.we` | v0 | 0 | stdout: `5` |
| `float-logic.we` | v0 | 0 | stdout: `3.5` |
| `logic.we` | v0 | 0 | stdout: `true` |
| `error-divzero.we` | v0 | non-zero | stderr error `{code:DIV_ZERO, op_index:2, operands:[r0,r1]}`, no stdout |
| `error-chk.we` | v0 | non-zero | stderr error `{code:CHK_FAIL, op_index:4, operands:[DIVISOR_ZERO]}`, no stdout |
| `map-square.we` | v1 | 0 | stdout: `[1, 4, 9]` |
| `fold-sum.we` | v1 | 0 | stdout: `6` |
| `map-capture.we` | v1 | 0 | stdout: `[50, 100]` |

## Definition of done
**v0:** `go build ./...` produces `weft.exe`; `go test ./...` is green; every v0 file in `examples/` behaves exactly as `examples/expected.json` specifies; a `weft.exe` (windows/amd64) and a linux/amd64 binary are produced.
**v1:** the three iteration examples additionally pass, with no regression in the v0 set.
