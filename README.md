# Weft

**A negative result: designing a programming language for an LLM to write made code ~3.6x more token-expensive, not cheaper — and the features that made it safe are the same ones that made it expensive.**

Weft is a language designed to be written by a large language model and read by no human. Every feature whose only job was to serve a human reader, writer, or reviewer was removed; everything quietly doing real work for the actual consumer — an LLM writing, a streaming interpreter reading — was kept or strengthened. It is not a cipher over Python and not an esolang. Illegibility is a consequence of the design, never its goal.

The headline claim was **token economy**: with no human reader, keywords, descriptive names, indentation, and most punctuation become pure overhead, so a Weft program should cost fewer tokens to generate than the equivalent in a conventional language. That claim was measured directly. **It is false.**

## The result

Nine semantically equivalent program pairs — the same computation written once in Weft (the actual `examples/*.we` files) and once in idiomatic Python. Each program text tokenized with `prompt_eval_count` in Ollama raw mode, which bypasses the chat template so the count is exactly the program's tokens, baseline-subtracted.

| Task | Weft | Python | Ratio |
|---|---|---|---|
| `(5+3)*2` → print | 42 | 11 | 3.82x |
| `max(7,4)` | 37 | 8 | 4.62x |
| len + index of `[10,20,30]` | 65 | 23 | 2.83x |
| guarded `10 // 2` | 52 | 23 | 2.26x |
| int→float, `7 / 2` | 37 | 7 | 5.29x |
| bool `(t or f) and not f` | 39 | 11 | 3.55x |
| square each `[1,2,3]` | 63 | 17 | 3.71x |
| sum `[1,2,3]` | 72 | 11 | 6.55x |
| scale `[10,20]` by 5 | 65 | 21 | 3.10x |
| **TOTAL** | **472** | **132** | **3.58x** |

Not a single-tokenizer artifact — the totals hold across three families:

| Tokenizer | Weft | Python | Ratio |
|---|---|---|---|
| qwen2.5-coder:7b | 472 | 132 | 3.58x |
| phi4:14b | 466 | 126 | 3.70x |
| gemma3:12b | 509 | 159 | 3.20x |

Every program loses, in every tokenizer, by roughly 3–4x. Full method and per-run detail in [MEASUREMENT.md](MEASUREMENT.md).

### Replicating the count

No install, no internet, no tokenizer library — any local Ollama model will report its own exact token count:

```sh
curl http://localhost:11434/api/generate -d '{
  "model": "qwen2.5-coder:7b",
  "prompt": "<program text here>",
  "raw": true,
  "options": {"num_predict": 1}
}'
```

`raw: true` bypasses the chat template, so the returned `prompt_eval_count` is exactly the prompt's tokens and nothing else. Subtract the empty-prompt baseline. Swap the model to measure against a different tokenizer — that is how the three-family cross-check above was produced. This generalises to any "how many tokens is this?" question.

## Why it loses (structural, not a tuning bug)

1. **One-op-per-line decomposition.** `(5+3)*2` is one Python expression but five Weft instructions. Flattening multiplies instruction count.
2. **Explicit handles.** Every instruction spends tokens naming its result (`r2`) and referencing operands by handle (`r0 r1`) where Python writes an inline `+`.
3. **Typed-literal opcodes.** `r0 lit.i 5` introduces a constant in ~4 tokens; Python writes `5`.
4. **Poor BPE fit.** Invented mnemonics (`lit.i`, `fn`, `r0`) aren't in code-trained merge tables, so they fragment — ~1.2 chars/token against Python's ~1.7. Meanwhile the keyword savings Weft was meant to bank barely apply, because small idiomatic Python has almost no boilerplate to remove in the first place.

## The finding that generalizes

The four causes above are not incidental overhead. Flat structure, explicit single-assignment handles, one operation per line, and explicit typing are **simultaneously** what makes Weft foolproof-by-construction and what makes it token-expensive. They are the same properties viewed from two directions:

- No nesting means no depth tally to lose track of — and it means a nested expression must be spread across many lines.
- Single-assignment handles mean no aliasing and no stale-value bugs — and they mean every value costs a token to name and a token to reference.
- Explicit typing means no silent coercion with no reviewer to catch it — and it means two opcodes where a literal would do.

So the safety and the cost cannot be separated under this design. You do not get to pick the foolproofing and decline the 3.6x. Any project reasoning about "a language designed for LLMs to write" should expect to meet this tradeoff rather than assume terseness and machine-friendliness point the same way.

A custom tokenizer with Weft opcodes and handle forms as single tokens would recover part of the gap — but that requires training or adapting the model's tokenizer, which contradicts the untouched-model + constrained-decoding premise ([ARCHITECTURE.md](ARCHITECTURE.md) §7) and trades away the property that lets Weft run on any stock model. It is not a free fix.

## Scorecard against the three original reasons to exist

| Claim | Status |
|---|---|
| **Token economy** — fewer tokens per unit of work | **Refuted.** ~3.6x worse, tokenizer-robust. |
| **Streaming execution** — every prefix of the stream is runnable, so generation and execution overlap | **Untested, and the bar is now higher.** Any wall-clock win needs the constrained decoder plus a live model, neither built — and must now overcome a 3.6x token penalty before it nets out positive. This result makes the claim *less* likely to pay off, not merely unproven. |
| **Foolproof by construction** — whole bug classes made unspeakable by the grammar | **Stands.** Demonstrated by the working interpreter: malformed programs can't run, errors are structured, iteration can't loop forever. |

**Honest current value: a foolproof, terminating-by-construction execution sandbox — not a cheaper or (yet) faster one.** That is a real property, but narrower than the original pitch, and the central economy claim is known wrong.

## What the language looks like

A program is a flat, ordered sequence of instructions, one per line, in static single-assignment three-address form:

```
<result-handle> <op> <operand>*
```

`(5 + 3) * 2`, printed:

```
r0 lit.i 5
r1 lit.i 3
r2 add r0 r1
r3 lit.i 2
r4 mul r2 r3
_  out r4
```

Two invariants do the foolproofing. **Single assignment**: a handle is written exactly once, so there is no mutation, no aliasing, and no "what is the value of `x` now" question. **Backward-only reference**: an operand handle must already exist, so "use before def" is not a bug you can write — it has no legal syntax.

Control flow carries the same treatment. Conditionals are `sel`, *selection* over two values that both already exist — no jump, no skipped instruction, no unreachable code. Iteration is `map`/`fold` over a vector whose length is already a value, so "infinite loop" has no syntax; termination is structural. Sub-streams (`fn`/`ret`) are the only nesting, exactly one level deep, so the nesting state is a single bit and never a counter.

Errors are data, not prose — `{ code, op_index, operands }` with `code` from a fixed enum — because the consumer is a retry loop, not a person at a terminal.

## Status

**v0.1.0, released 2026-07-01.** The interpreter is built and green: `go build ./...` produces a single dependency-free static binary, `go vet` and `go test ./...` are clean on go1.26.3, and all 11 golden programs in `examples/` behave exactly as `examples/expected.json` specifies (independently re-verified 2026-08-27: 11/11, including both structured-error cases). Binaries for windows/amd64 and linux/amd64 are attached to the GitHub release rather than committed — they are regenerable, so the tree ignores them.

Built:
- v0 core — literals, arithmetic, comparison, logic, `sel`, `vec`/`len`/`idx`, `i2f`, `chk`, `out`
- v1 iteration — `fn`/`ret` sub-streams, `map`, `fold`, explicit capture lists
- Structured error model with `op_index` and operands
- The token-economy measurement above

Not built:
- **The semantic-correctness layer** ([ARCHITECTURE.md](ARCHITECTURE.md) §8) — design intent, not a mechanism. This is the gap between "runs" and "right", and the highest-value next prototype. Everything Weft guarantees is well-formedness; a program can be well-formed, typed, terminating, runnable, and still confidently compute the wrong answer.
- **The constrained decoder** — the grammar mask that would enforce the language at sampling time rather than asking for it in a prompt. Separate project, deliberately out of scope for the runtime.
- Nested iteration (a sub-stream cannot contain `fn`), and `scan`.

## Running it

```sh
go build ./...
weft examples/arith.we          # → 16
weft examples/map-capture.we    # → [50, 100]
weft --trace examples/guard.we  # instruction-by-instruction
weft -                          # read a program from stdin
```

Exit 0 on success; exit 1 with one structured JSON error on stderr on failure.

## Repo map

| File | Contents |
|---|---|
| [MEASUREMENT.md](MEASUREMENT.md) | The token-economy test — method, per-program and per-tokenizer results, why it fails |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Language specification: what Weft is, and the reason behind each decision |
| [RUNTIME-ARCHITECTURE.md](RUNTIME-ARCHITECTURE.md) | Build spec for the interpreter — data structures, execution algorithm, per-opcode behaviour, error model, CLI |
| [ITERATION.md](ITERATION.md) | The v1 iteration design: sub-streams, `map`, `fold`, and the capture rule |
| `examples/` | Golden test programs and `expected.json`, the correctness source of truth |

## Name

"Weft" is a working codename. A weft is the thread a loom draws through once and leaves in place — which is the data model: every value written once, never moved. Rename freely.
