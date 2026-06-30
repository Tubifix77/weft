# Weft runtime — architecture & build spec

Companion to `ARCHITECTURE.md` (the language spec). This document is the build contract for the **runtime**: the program that executes a `.we` file. `ARCHITECTURE.md` says what the language *is*; this says how to *run* it. Claude Code implements against this file plus the golden tests in `examples/`.

---

## 0. Interpreter, not compiler — and why it matters here

Two different jobs get lumped under one word:

- A **compiler** reads the whole program, translates it to something else (machine code, bytecode), and that output runs later.
- An **interpreter** reads the program and *does it directly*, now, producing no separate artifact. `python.exe` is an interpreter — written in C, compiled once to a native binary, and from then on it walks your `.py` and executes as it reads.

Weft's runtime is an **interpreter**, and the language spec already forced this choice: `ARCHITECTURE.md` §1 and §7 are built around *streaming* — executing each instruction the instant it arrives, overlapping with generation. A compiler needs the whole program before it can emit anything, which contradicts the streaming property. Building a compiler would fight the design.

The `python.exe` analogy, made exact: CPython is an interpreter written in C, compiled to a native exe. Weft's runtime is an interpreter written in **Go**, compiled to a native `weft.exe`. No host-language dependency at run time.

---

## 1. Host language: Go

Rationale: compiles to a single static binary with no runtime dependency (the direct analog of `python.exe` being a compiled C program); a line-walking interpreter is simple to express; matches an existing single-binary Go project on this machine (samovar). Rust is the faster alternative and a clean later swap — but v0 is correct-first, and Go is more than fast enough to prove the language runs. Do not start in Rust unless speed becomes the goal.

---

## 2. Scope of v0 — the core subset

Implement every opcode in `ARCHITECTURE.md` §5 **except** `map` / `fold` (the combinators). Their sub-stream syntax is now specified in `ITERATION.md`; `map`/`fold` are **v1**, built after the v0 core is green. v0 itself stays the core subset so the first runnable interpreter is small.

**v0 opcodes:** `lit.i` `lit.f` `lit.b` `lit.s`, `add` `sub` `mul` `div` `mod`, `eq` `ne` `lt` `le` `gt` `ge`, `and` `or` `not`, `sel`, `vec` `len` `idx`, `i2f`, `chk`, `out`.

**Deferred to v1 (do not build yet):** `map` `fold` `scan` and sub-stream syntax — blocked on §12.2. The constrained decoder (`ARCHITECTURE.md` §7 stage 1) is a separate project entirely; it runs *against* this interpreter once the interpreter exists, and needs a generating model. v0 runs `.we` files that already exist (hand-written, or written by Claude in chat).

---

## 3. File format

A `.we` file is UTF-8 text, one instruction per line. Blank lines are ignored (allowed so hand-written test files can breathe). There are no comments in v0 — per `ARCHITECTURE.md` §10, the generate-run-discard default omits them. A line is an instruction or it is blank; nothing else is legal.

**Instruction grammar (v0):**

```
line     := handle WS op (WS operand)*
handle   := 'r' digit+        ; a value-producing instruction's result
          | '_'               ; effect-only (out, chk) — produces no value
op       := lowercase mnemonic, may contain '.'  (e.g. lit.i)
operand  := handle | literal
literal  := int | float | bool ('true'|'false') | bareword
```

**Handle ordering rule:** the result handle of the Nth value-producing instruction must be exactly `rN`, counting from `r0`, with no gaps and no reuse. The interpreter enforces this (a violation is a `PARSE` error). This is what makes "use before definition" unspeakable: an operand `rK` is legal only if `rK` already exists, i.e. `K < current length`.

---

## 4. Core data structures (Go)

**Value** — a tagged union. Because the language is monomorphic and typed at creation (`ARCHITECTURE.md` §6), every value carries its kind explicitly:

```go
type Kind int
const ( KInt Kind = iota; KFloat; KBool; KStr; KVec )

type Value struct {
    Kind Kind
    I    int64
    F    float64
    B    bool
    S    string
    V    []Value   // for KVec
}
```

**Environment** — here is the payoff of the language's design. Because handles are strictly `r0, r1, …, rN` contiguous with no gaps, the environment is **a growable slice indexed by handle number**, not a hash map:

```go
env []Value   // env[K] is the value of handle rK
```

Each value-producing instruction appends exactly one element; a reference `rK` is `env[K]`, an O(1) index. Backward-only reference guarantees the index always exists. The "dictionary mapping names to values" that a normal interpreter needs collapses into an append-only array — a direct consequence of SSA + strict ordering. No symbol table, no scope chain.

---

## 5. Execution algorithm

```
env := []Value{}
opIndex := 0
scanner := bufio.NewScanner(input)        // streams line by line
for scanner.Scan():
    line := trim(scanner.Text())
    if line == "": continue
    tokens := fields(line)                // whitespace split
    handle := tokens[0]
    op     := tokens[1]
    args   := tokens[2:]
    result, err := dispatch(op, args, env, opIndex)
    if err != nil:
        emitError(err); exit(1)           // structured error to stderr
    if handle != "_":
        assert handleIndex(handle) == len(env)   // else PARSE error
        env = append(env, result)
    opIndex++
exit(0)
```

Reading with `bufio.Scanner` means the interpreter executes as it reads — the streaming property holds for free, whether the input is a file or (later) a pipe carrying an LLM's token stream. `opIndex` is the zero-based count of instruction lines processed (blank lines excluded), and is what error records report.

---

## 6. Opcode dispatch

A table maps each mnemonic to a handler. Operand resolution is op-specific:

- `lit.i lit.f lit.b lit.s` take **one inline literal** operand (the raw value) and produce a typed Value. `lit.b` accepts `true`/`false`. `lit.s` takes a bareword (v0 strings are single whitespace-free tokens — quoting is a v1 concern).
- `chk` takes `(pred-handle, code)` where **`code` is an inline bareword**, not a handle (see §8 — this resolves an ambiguity in `ARCHITECTURE.md` §5).
- **Every other op** resolves all operands as handles (`rK` → `env[K]`).

Type rules (monomorphic — mismatch is a `TYPE_MISMATCH` error):
- `add sub mul div mod`: both operands same numeric kind (both `KInt` or both `KFloat`). Result same kind.
- `eq ne`: both operands same kind. Result `KBool`.
- `lt le gt ge`: both operands same numeric kind. Result `KBool`.
- `and or`: both `KBool`. `not`: one `KBool`. Result `KBool`.
- `sel(cond, a, b)`: `cond` is `KBool`; `a` and `b` same kind; result is `a` if cond else `b`. (v0: both operands already evaluated — eager. The lazy-via-fn-ref variant in §5 of the spec is a v1 concern, tied to sub-streams.)
- `vec(x0..xn)`: all same kind; result `KVec`. `len(v)`: `v` is `KVec`; result `KInt`. `idx(v, i)`: `v` is `KVec`, `i` is `KInt`; bounds-checked (`IDX_OOB`); result is the element.
- `i2f(x)`: `x` is `KInt`; result `KFloat`.
- `div`/`mod` by zero (int) → `DIV_ZERO`.
- `out(x)`: prints `x` to stdout (§9 format); no result. `chk(pred, code)`: if `pred` false → `CHK_FAIL` error carrying `code`; else no-op, no result.

Unknown mnemonic → `BAD_OP`. Wrong operand count → `ARITY`. Reference to a nonexistent handle → `BAD_HANDLE`. Malformed handle ordering or unparseable literal → `PARSE`.

---

## 7. Output format (`out`)

- `KInt` → decimal (`16`)
- `KFloat` → shortest round-trip, `strconv.FormatFloat(f, 'g', -1, 64)` (so `3.5`; note `7.0` prints as `7` — acceptable in v0, document if it surprises)
- `KBool` → `true` / `false`
- `KStr` → the raw string
- `KVec` → `[a, b, c]` (elements formatted per their kind, comma-space separated)

One `out` per line on stdout.

---

## 8. Error model

Per `ARCHITECTURE.md` §9, errors are structured data, not prose. On the first error the interpreter prints one JSON line to **stderr** and exits non-zero:

```json
{"code":"DIV_ZERO","op_index":2,"operands":["r0","r1"]}
```

```go
type WeftError struct {
    Code     string   `json:"code"`
    OpIndex  int      `json:"op_index"`
    Operands []string `json:"operands"`   // operand tokens as written
}
```

Fixed code enum: `DIV_ZERO`, `IDX_OOB`, `CHK_FAIL`, `TYPE_MISMATCH`, `BAD_HANDLE`, `BAD_OP`, `ARITY`, `PARSE`.

**Two decisions resolved here (fold back into `ARCHITECTURE.md` §5/§9):**
1. `chk`'s second operand is an **inline bareword code**, not a handle — the code is a compile-time constant, so forcing a `lit.s` line before every guard would be pointless ceremony.
2. A failed `chk` raises code **`CHK_FAIL`** (keeping the enum fixed), and carries the user's bareword as its single operand — so a retry loop learns *which* check failed without the enum becoming open-ended.

---

## 9. CLI

```
weft run program.we      # execute a file
weft program.we          # 'run' is the default verb
weft run -               # read program from stdin (the streaming/pipe case)
weft run --trace prog.we # debug: after each instruction, print opIndex + env to stderr
```

Success: `out` values on stdout, exit 0. Failure: structured error on stderr, exit non-zero. Nothing else on stdout.

---

## 10. Module layout (Go)

```
weft/
  go.mod
  main.go                          # CLI entry, arg parsing
  internal/value/value.go          # Value, Kind, formatting
  internal/lexer/lexer.go          # line -> tokens, literal parsing
  internal/interp/interp.go        # env, execution loop, streaming reader
  internal/interp/ops.go           # opcode handlers + dispatch table
  internal/interp/errors.go        # WeftError, emit
  internal/interp/interp_test.go   # golden tests driven by examples/
  examples/                        # .we test programs + expected.json
```

---

## 11. Test strategy

Golden tests, table-driven. `examples/expected.json` maps each `.we` file to its expected `exit` code and either `stdout` (a list of lines) or `error` (a `WeftError`). The Go test runs every example, captures stdout/stderr/exit, and compares. This is the definition of correct.

---

## 12. Build order for Claude Code

Each step ends green (`go build` + the relevant golden tests pass) before the next begins.

1. `value` + `lexer` + env skeleton + execution loop; implement `lit.i`, `add`, `out` only → `arith.we` passes.
2. Remaining arithmetic + comparison + logic + `i2f` + `lit.f/lit.b/lit.s` → `float-logic.we`, `logic.we` pass.
3. `sel` → `max.we` passes.
4. `vec` / `len` / `idx` → `vector.we` passes.
5. `chk` + full error model + structured stderr + handle-ordering enforcement → `guard.we`, `error-divzero.we`, `error-chk.we` pass.
6. CLI polish (`run`, stdin `-`, `--trace`); all golden tests green.
7. Cross-compile: `weft.exe` (windows/amd64) and a linux/amd64 binary.

**Definition of done:** `go build ./...` produces `weft.exe`; `go test ./...` is green; every file in `examples/` behaves exactly as `examples/expected.json` says.

---

## 13. Explicitly out of scope for this build

- `scan` and nested iteration — not yet specified. (`map` / `fold` and sub-stream syntax ARE now specified in `ITERATION.md` and are **v1**, built after the v0 core below — no longer deferred.)
- The constrained decoder (`ARCHITECTURE.md` §7 stage 1) — separate project; it makes *generation* safe and runs against this interpreter. Not part of v0.
- Performance work. v0 is correct-first; a Rust rewrite or a bytecode pass is a later, separate effort.
