# Weft iteration — sub-streams, map, and fold

Resolves the open question in `ARCHITECTURE.md` §12.2. This is the v1 addition to the language and runtime: the iteration mechanism. With it, Weft stops being a straight-line calculator and can process collections.

## The gap this closes

`ARCHITECTURE.md` §5 lists `map` and `fold` and says each takes a "fn-ref" pointing to a "sub-stream" — but never defines what a sub-stream looks like, how the per-element value gets into it, or which outer values it may see. So the opcodes were unrunnable: there was no syntax for the function they consume. Iteration is also the feature that makes the language able to do real work, and the "no infinite loops" guarantee (`ARCHITECTURE.md` §5) depends on `map`/`fold` being *the* sanctioned way to repeat. So this had to be specified, not deferred.

## The constraint that shapes the design

Everything that makes Weft foolproof must survive *inside* the per-element function too: flat (no nesting tally), single assignment, backward-only reference, explicit typing, nothing implicit. But a function body is inherently a block, and blocks are nesting — the thing the language deleted, because nesting forces a depth tally and tallying is the operation transformers are worst at.

The resolution: **a sub-stream may not contain another sub-stream.** One level deep, exactly. So at any moment you are either at the top level or inside exactly one sub-stream — a single bit of state, never a counter. The depth-tally problem never returns, because there is no depth to tally.

## A sub-stream is a closure value

A closure is created by the `fn` opcode like any other value — it takes the next top-level handle:

```
r4 fn <arity> <capture-handle>*
... body lines ...
_ ret <local-handle>
```

- `<arity>` — how many iteration inputs the body binds: `1` for `map` (the element), `2` for `fold` (accumulator, then element).
- `<capture-handle>*` — an explicit, possibly empty list of *already-existing top-level handles* the body is allowed to see. Nothing outside this list is visible; there is no implicit closure over the enclosing program.
- The body is a normal flat instruction sequence with its **own local handle namespace** that restarts at `r0`.
- `_ ret <local-handle>` names the body's single result and closes the closure. There is exactly one `ret`, and it is the last line of the body.

Inside the body, three kinds of name are visible and nothing else:

- `a0, a1, …` — the bound iteration inputs (map: `a0` = element; fold: `a0` = accumulator, `a1` = element).
- `c0, c1, …` — the captured values, in the order listed in the `fn` header.
- `r0, r1, …` — the body's own locals, strict-ordered and backward-only, exactly as at the top level but in a fresh namespace.

The body may never reference a top-level `rN` directly. If it needs an outer value, that value must be named in the capture list, where it becomes a `cK`. This is the closure rule: everything the body can see is declared in its header. Capture is by value at definition time — but since values are single-assignment and immutable, by-value and by-reference are indistinguishable; the captured value simply cannot change.

## map and fold

- `map(vec, fn) → vec` — `fn` must have arity 1. Runs the body once per element with `a0` = element, collecting each `ret`. Result is a vector of the results, same length. All results must share one kind (else `TYPE_MISMATCH`).
- `fold(vec, init, fn) → value` — `fn` must have arity 2. The accumulator starts as `init`; for each element the body runs with `a0` = accumulator and `a1` = element, and its `ret` becomes the new accumulator. Result is the final accumulator. Each `ret` must match the accumulator's kind.

Both iterate a vector whose length is already a known value, so the body runs exactly that many times. There is no loop condition, so "runs forever" has no syntax — termination is structural, the same property claimed in `ARCHITECTURE.md` §5.

## Worked examples

**Square each element** — `[1,2,3]` → `[1, 4, 9]` (`examples/map-square.we`):

```
r0 lit.i 1
r1 lit.i 2
r2 lit.i 3
r3 vec r0 r1 r2
r4 fn 1
r0 mul a0 a0
_ ret r0
r5 map r3 r4
_ out r5
```

**Sum a vector** — `[1,2,3]` with init `0` → `6` (`examples/fold-sum.we`):

```
r0 lit.i 1
r1 lit.i 2
r2 lit.i 3
r3 vec r0 r1 r2
r4 lit.i 0
r5 fn 2
r0 add a0 a1
_ ret r0
r6 fold r3 r4 r5
_ out r6
```

**Scale each element by an outer constant** — `[10,20]` × `5` → `[50, 100]`, showing capture (`examples/map-capture.we`):

```
r0 lit.i 10
r1 lit.i 20
r2 vec r0 r1
r3 lit.i 5
r4 fn 1 r3
r0 mul a0 c0
_ ret r0
r5 map r2 r4
_ out r5
```

`r4` captures `r3` (= 5) as `c0`; the body multiplies each element by it.

## Streaming note

`fn` does not execute its body — it *buffers* the body lines into the closure value, then `ret` closes it and top-level execution resumes. The body runs later, when `map`/`fold` is reached. The top-level instruction stream is still prefix-executable (`ARCHITECTURE.md` §1); a closure is simply a value that carries deferred instructions, the same way any language defines a function before calling it. The buffering is bounded (one level, small body), so it never reintroduces the whole-program lookahead a compiler would need.

## Honest limitation (v1)

Because a sub-stream cannot contain `fn`, a sub-stream body cannot itself call `map`/`fold` — there is no nested iteration in v1 (no mapping over a vector of vectors via an inner map). This keeps the one-level model and the single-bit nesting state intact. Nested iteration is a future extension and would need its own design pass; it is deliberately out of scope here.

## Runtime additions (for the interpreter)

- **New Value kind** `KClosure`: `{ arity int, captures []Value, body []Instruction, ret int }`, where `ret` is the local handle index the body yields.
- **Parsing/execution:** a 1-bit `inFn` state. `fn` opens it (records arity, snapshots capture values, starts a fresh local instruction buffer and a fresh local handle counter); each body line is appended to the buffer, its handle ordering checked against the *local* counter; `_ ret rK` records `ret = K`, clears `inFn`, and finalizes the closure as the next top-level handle. The top-level handle counter is untouched by body locals.
- **Executing a closure** (from map/fold): make a fresh local env, place captures at `c0..` and bound inputs at `a0..`, run the buffered body resolving `aK`/`cK`/`rK` against that env, return `env[ret]`.
- **New error codes** (extend `ARCHITECTURE.md` §9 / `RUNTIME-ARCHITECTURE.md` §8): `NEST` (a `fn` while already `inFn`), `BAD_CLOSURE` (a map/fold operand that isn't a `KClosure`), `BAD_ARITY` (closure arity ≠ 1 for map, ≠ 2 for fold), `NO_RET` (a `fn` block closed without exactly one `ret`). Reuse `TYPE_MISMATCH` for non-uniform map results and fold accumulator-kind drift.
- **Build order:** this is v1, built after the v0 core in `RUNTIME-ARCHITECTURE.md` §12 is green. Add `KClosure` + the `inFn` buffering, then `fn`/`ret`, then `map`, then `fold`. The three example programs above are the golden tests.
