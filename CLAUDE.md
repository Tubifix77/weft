# CLAUDE.md — build contract for the Weft runtime

You are implementing the **Weft interpreter** (`weft` / `weft.exe`): a program that executes `.we` files.

## Read first, in order
1. `ARCHITECTURE.md` — the language specification (what Weft is, why each decision was made).
2. `RUNTIME-ARCHITECTURE.md` — the build spec for this runtime (how to run it). **This is your contract.** It pins the host language, the v0 scope, the data structures, the execution algorithm, every opcode's behaviour, the error model, the CLI, the module layout, the build order, and the definition of done.
3. `examples/` — golden test programs and `examples/expected.json` (the source of truth for correctness).

## The job in one paragraph
Write a line-walking interpreter in **Go** that reads a `.we` file one instruction per line, keeps the environment as a slice indexed by handle number (`env[K]` = value of `rK`), dispatches each opcode to a handler, and prints `out` values to stdout — or emits one structured JSON error to stderr and exits non-zero. It compiles to a single dependency-free `weft.exe`. Follow the build order in `RUNTIME-ARCHITECTURE.md` §12; do not start in any language other than Go; do not build `map`/`fold` or the constrained decoder (both out of scope — see §13).

## Scope guardrails (do not exceed)
- v0 opcodes only: `lit.i lit.f lit.b lit.s`, `add sub mul div mod`, `eq ne lt le gt ge`, `and or not`, `sel`, `vec len idx`, `i2f`, `chk`, `out`.
- No `map`/`fold`/`scan`, no sub-streams — blocked on `ARCHITECTURE.md` §12.2.
- No constrained decoder — separate project.
- No performance work — correct-first.

## Two language decisions resolved in the runtime spec (already reflected in the examples)
- `chk`'s second operand is an **inline bareword code**, not a handle.
- A failed `chk` raises code **`CHK_FAIL`** and carries that bareword as its single operand.
These should eventually be folded back into `ARCHITECTURE.md` §5/§9; note it, don't block on it.

## Golden tests — what each example must do
Canonical machine-readable form is `examples/expected.json`. Human-readable summary:

| File | Exits | Produces |
|---|---|---|
| `arith.we` | 0 | stdout: `16` |
| `max.we` | 0 | stdout: `7` |
| `vector.we` | 0 | stdout: `3` then `20` |
| `guard.we` | 0 | stdout: `5` |
| `float-logic.we` | 0 | stdout: `3.5` |
| `logic.we` | 0 | stdout: `true` |
| `error-divzero.we` | non-zero | stderr error `{code:DIV_ZERO, op_index:2, operands:[r0,r1]}`, no stdout |
| `error-chk.we` | non-zero | stderr error `{code:CHK_FAIL, op_index:4, operands:[DIVISOR_ZERO]}`, no stdout |

## Definition of done
`go build ./...` produces `weft.exe`; `go test ./...` is green; every file in `examples/` behaves exactly as `examples/expected.json` specifies; a `weft.exe` (windows/amd64) and a linux/amd64 binary are produced.
