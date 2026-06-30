# Weft — architecture reference

> **Weft** is a working codename, not a final name. A weft is the thread a loom draws through once and leaves in place — which is exactly the data model: every value is written once and never moves. Rename freely.

This document describes a programming language designed to be **written by a large language model and read by no human**. It is not a cipher over Python, and not an esolang. The illegibility is a *consequence* of the design, never its goal. Every decision below removes something whose only job was to serve a human reader, writer, or reviewer — and keeps or strengthens everything that was quietly doing real work for whatever actually consumes the code.

---

## 1. Why this exists

A reader who only ever sees Python-with-stranger-spelling is right to dismiss it. So the reason to exist has to be a property that *removing the human unlocks*, not one that merely survives their absence. Weft claims three, and they compound:

1. **Token economy.** Keywords, multi-character names, indentation, and most punctuation exist for a human. None of it does anything for an interpreter. With no reader, all of it becomes pure overhead to be deleted — fewer output tokens per unit of work, which is directly cheaper and faster to generate.
2. **Streaming execution.** The grammar is built so that *every prefix of the token stream is already a valid, runnable program*. An interpreter sitting on the model's output can execute each instruction the instant it is emitted, overlapping generation and execution instead of waiting for generation to finish and then parsing. This is wall-clock latency, not just token count.
3. **Foolproof by construction.** The human reviewer was the safety net that caught the model's mistakes. Remove the net and the *grammar itself* has to become the reviewer. Whole classes of bug are made unspeakable — not caught at runtime, but impossible to express. Illegibility never protected anyone; restriction did.

The honest limit, stated up front so it is never mistaken for solved: **none of this addresses semantic correctness.** Weft guarantees a program is well-formed, type-consistent, terminating, and runnable. It cannot guarantee the program computes the *right* thing. "I will never read a line of it" constrains *how* the model writes; it does nothing about *what* the model decided to compute. Section 8 is the mechanism that turns the empty reviewer slot into an automated one. It is the part that matters most and is the least finished.

---

## 2. The design principle: subtract the human, keep the consumer

Most of what a conventional language looks like on the page solves a problem specific to *being a human*: a small, decaying working memory and eyes that fixate on one spot at a time. The design method is to take each feature, name the limitation it compensates for, and ask whether the actual consumer — an LLM writing, a streaming interpreter reading — shares that limitation.

| Feature in normal languages | Whose limitation it serves | Kept in Weft? | Why |
|---|---|---|---|
| Indentation / brace nesting | Human vision (see scope without tracing) | **No** | The program is flat (§4). Scope-by-nesting forces a depth *tally*, and tallying is the operation transformers are *worst* at. Flatness deletes the tally rather than optimizing it. |
| Descriptive names (`user_id`) | Human memory (don't re-derive meaning) | **No** for recall; replaced by stable handles (§4) | An LLM attends directly to where a value was produced; it doesn't decay. But a *reference* must still be cheap — a literal token copy (strong op), not a recomputed offset (weak op). Handles give the copy without the recall tax. |
| Infix + operator precedence | Inherited arithmetic convention | **No** | Each instruction is a single operation with explicit operands (§4). No precedence table exists, so none can be applied wrongly — regardless of any frequency-acquired bias toward infix. |
| Syntax sugar (3 ways to write one loop) | Human cognitive diversity | **No** | There is no "way the model thinks" that one form serves better. Every redundant form is just another place to be inconsistent. One canonical spelling per operation is strictly better. |
| Comments (why, not how) | Whoever returns without the original context | **Conditional** (§9) | Not a human-only need — a *future* writer's need, and a future LLM session has exactly the original author's blind spot. Survives if the lifecycle is patch-across-sessions; dead weight if generate-run-discard. |
| Unambiguous scope extent | Any consumer, not just visual ones | **Yes**, strengthened | Disambiguation is owed to the interpreter too. Flatness provides it without nesting. |
| Type information | Compiler/runtime correctness | **Yes**, made explicit (§6) | With no reviewer to catch a coercion bug, implicit typing is a liability. Types are declared at creation, never inferred. |
| Structured error data | A retry loop or a debugger | **Yes**, strengthened (§7) | A prose stack trace is for a human at a terminal. The consumer here is a retry loop, so errors are machine-actionable data, not sentences to re-parse. |

The conclusion is not "terse beats readable." It is that **compactness and readability stop being the same axis once vision and decay are gone.** Anything whose only job was visual or mnemonic goes to zero. Anything doing disambiguation, intent-signaling, or state-visibility stays and gets *more* explicit than conventional languages bother with — because there is no reviewer left to catch the case where implicit was also wrong.

---

## 3. Resolving the RPN-vs-handles contradiction

Two representations were sketched while designing this, and they conflict. A spec has to pick one.

- **Stack/RPN form** (`5 3 + 2 *`): no names, no precedence, no brackets. But the stack is implicit — to reference a value you must know the current stack *depth*, and depth-tracking is the same counting operation that nesting forced. RPN trades the nesting tally for a stack tally. No real gain on the weak operation.
- **Relative-offset form** (`^0`, `^1` meaning "N results back"): no names, but a reference is a recomputed *distance*, which is again counting, and it drifts past the first couple of steps.

Weft uses neither. It uses **static single-assignment (SSA) three-address form**: every instruction names its own result with a fresh, permanent handle (`r0`, `r1`, `r2`, …) and lists its operands explicitly *by handle*. This keeps the RPN *benefits* (no precedence, every instruction self-contained and runnable on arrival) through a different mechanism (explicit operands) than RPN's stack — and a reference becomes a literal copy of an earlier token, the strong operation, with no tally of any kind. This is not an invention for its own sake: it is what compiler intermediate representations have used internally for decades, built for a machine consumer, never for a human writer. Weft lifts it wholesale rather than redesigning it worse.

---

## 4. Core data model

A Weft program is a **flat, ordered sequence of instructions, one per line.** There are no blocks, no nesting, no indentation that carries meaning.

Each instruction has the form:

```
<result-handle> <op> <operand>*
```

- **`result-handle`** is the next unused register name: `r0`, then `r1`, then `r2`, in strict order. The handle is written explicitly (one token), which converts every future reference from a count into a copy.
- **`op`** is a single mnemonic token (§5).
- **`operand`** is either an earlier handle or an inline literal, fixed in count and type per op (its *arity*).
- Effect-only instructions that produce no value use `_` as the result handle.

Worked example — `(5 + 3) * 2`, printed:

```
r0 lit.i 5
r1 lit.i 3
r2 add r0 r1
r3 lit.i 2
r4 mul r2 r3
_  out r4
```

Two invariants do the foolproofing, and both are enforced at generation time (§7), not hoped for:

- **Single assignment.** A handle is written exactly once. There is no mutation, so there is no aliasing, no stale-value bug, and no "what is the value of `x` *now*" question.
- **Backward-only reference.** An operand handle must already exist. "Use a value before it exists" has no legal syntax — it is not a bug you can write, it is an unspeakable sentence.

---

## 5. Opcode table

Mnemonics are short ASCII tokens chosen to land on single tokenizer tokens. Exotic Unicode (∮, ⊕) is deliberately avoided: rare codepoints usually split into 2–3 tokens, while plain ASCII punctuation is almost always one. Reusing familiar ASCII with *new fixed meanings* is also more disorienting to a Python/C habit than exotic glyphs — close enough to misread on autopilot, which forces real grammar-tracking instead of pattern completion.

| Op | Arity | Result type | Meaning |
|---|---|---|---|
| `lit.i` | 1 literal | int | integer literal |
| `lit.f` | 1 literal | float | float literal |
| `lit.b` | 1 literal | bool | boolean literal |
| `lit.s` | 1 literal | str | string literal |
| `add` `sub` `mul` `div` `mod` | 2 handles | numeric | arithmetic (operands must share a numeric type) |
| `eq` `ne` `lt` `le` `gt` `ge` | 2 handles | bool | comparison |
| `and` `or` | 2 handles (bool) | bool | logic |
| `not` | 1 handle (bool) | bool | negation |
| `sel` | 3 handles: cond, a, b | type of a/b | **selection, not branching**: yields `a` if `cond` else `b`. Both operands already exist; nothing is skipped. |
| `vec` | N handles (same type) | vec | build a fixed-length vector |
| `len` | 1 handle (vec) | int | element count |
| `idx` | 2 handles: vec, int | element type | bounds-checked element access |
| `map` | 2: vec, fn-ref | vec | apply a sub-stream to each element (§iteration) |
| `fold` | 3: vec, init, fn-ref | acc type | left fold over a vector |
| `chk` | 2: pred (bool), code | — | assert a predicate; on false, raise a structured error (§7, §8) |
| `out` | 1 handle | — | emit a value as program output |

### Control flow without branches or loops

- **Conditionals are selection, not branching.** `sel` evaluates a condition and returns one of two values that *both already exist*. There is no jump, no skipped instruction, no unreachable code, and no path the interpreter has to predict before it can stream. (Where laziness matters — an expensive unused branch — `sel` takes fn-refs and forces only the chosen one; this is the single concession to lazy evaluation.)
- **Iteration is bounded combinators, not imperative loops.** An imperative `while`/`for` needs mutation and a runtime termination condition — both forbidden here (single assignment; no condition-driven repeat). Instead, iteration is `map`/`fold`/`scan` over a vector that *already exists as a value*. Each element yields a new value rather than mutating one, so single assignment is preserved, and termination is guaranteed by the input length (itself a value). "Infinite loop" is therefore as unspeakable as "use before def": there is no construct whose repeat count isn't already bounded by existing data.

The `fn-ref` consumed by `map`/`fold` points to a small, named sub-stream (a closed block of the same instruction form with its own local handles plus the bound element/accumulator). Sub-streams are the *only* nesting in the language, they are one level deep, and they are referenced by handle like everything else — so the flat-stream and backward-reference invariants hold inside them too.

---

## 6. Type model

Every value is typed **at creation**, never inferred. `lit.i` and `lit.f` are distinct opcodes precisely so the interpreter never has to guess whether `5` is an int or a float. Operations are monomorphic: `add` requires both operands to share a numeric type, `and`/`or`/`not` require bool, `idx` requires (vec, int). Because there is no reviewer to catch a silent coercion, **there is no implicit coercion** — a type mismatch is either rejected by the decoder before emission (where the constraint state tracks operand types, §7) or raised as a structured error before execution. Conversions are explicit ops (e.g. `i2f`), never automatic.

---

## 7. Runtime pipeline

The pipeline is the architecture; each stage only makes sense once you accept there is no human reader.

```
            ┌─────────────────────────────────────────────┐
            │  1. LLM generator (constrained decoding)      │
            │     next-token distribution is MASKED to      │
            │     only grammar-legal continuations          │
            └───────────────────────┬───────────────────────┘
                                    │  one complete instruction
                                    ▼
            ┌─────────────────────────────────────────────┐
            │  2. Instruction stream                        │
            │     flat · SSA · postfix-operand · typed      │
            └───────────────────────┬───────────────────────┘
                                    │  emitted token-by-token
                                    ▼
            ┌─────────────────────────────────────────────┐
            │  3. Streaming interpreter                     │
            │     executes each instruction on arrival;     │
            │     no lookahead, no bracket-matching         │
            └───────────────────────┬───────────────────────┘
                                    │
                                    ▼
            ┌─────────────────────────────────────────────┐
            │  4. Structured result                         │
            │     a value, OR { code, op-index, operands }  │
            └───────────────────────┬───────────────────────┘
                                    │ on error
                                    ▼
              ↻ regenerate under the same grammar — no human reviewer in the loop
```

**Stage 1 — constrained decoding is the load-bearing safety mechanism.** Good prompting cannot reliably out-pull a model trained heavily on C and Python; at the margins it will reach for `if (` or `def`. So the grammar is enforced at the *sampling* level, not asked for in a prompt. A grammar mask (the mechanism behind structured/JSON output modes, and tools like GBNF) restricts the next-token distribution to only the tokens that are a legal continuation of the grammar so far. This works on a completely untouched, never-fine-tuned model, because it filters what comes *out* rather than retraining what the model *wants*. Under the mask, `if (` is not a managed risk — it is a string that is not a legal next token, full stop. The constraint state tracks: which handles exist (enforces backward-only reference and strict handle ordering), the type of each handle (enforces monomorphic ops), and the arity remaining for the current op.

**Stage 3 — streaming is what the flat/SSA/postfix form buys.** Because every instruction is self-contained and every prefix is valid, the interpreter never waits for a closing brace that flat code does not have. Generation and execution overlap.

**Stage 4 → 1 — the feedback loop is the spine.** With the human reviewer gone, this loop is the only thing left holding well-formedness. The error must be actionable *on arrival*, which is why it is structured data (§ below), not prose.

---

## 8. The semantic-correctness layer (the unfinished part)

Everything above guarantees a program that is well-formed, typed, terminating, and runnable. It does **not** guarantee correctness — a program can be all of those things and still confidently compute the wrong answer, because meaning is not a syntax property. The reviewer slot that used to catch wrong logic is empty.

The intended fill, stated as design intent rather than finished mechanism: the generator also emits a **specification** — a set of `chk` predicates and/or property assertions describing what the program *should* satisfy (input/output relations, invariants, examples) — and an automated harness runs the program against that spec, feeding failures back through the same loop as structured errors. This is the "plan-validate-execute" pattern: produce a verifiable intermediate artifact, validate it mechanically, only then trust the output. It is the part of Weft most worth prototyping next, because it is what would let the language be trusted for real work rather than admired as a thought experiment. Until it exists, Weft's guarantee stops at "runs," not "right."

---

## 9. Error model

Errors are not sentences. A failure produces a structured record:

```
{ code: <enum>, op_index: <int>, operands: [<handle|value>...] }
```

- `code` is a fixed enum (e.g. `DIV_ZERO`, `IDX_OOB`, `CHK_FAIL`, `TYPE_MISMATCH`), not free text.
- `op_index` points to the exact instruction.
- `operands` carries the exact values involved, so the retry loop never has to re-parse prose to recover them.

This is lifted from the corners of computing that never had a human in the error-reading seat to begin with — a code plus its operands, designed for a consumer that acts on it, not a person who reads it.

---

## 10. Lifecycle and the comment question

Comments are the one feature whose fate depends on how Weft is *used*, not on what consumes it:

- **Generate-run-discard** (rebuilt from zero whenever a requirement changes): no future reader exists, so comments are dead weight and are omitted entirely.
- **Patched-across-sessions** (the common "vibe-coding" reality — the same artifact edited over many separate sessions): a future LLM session reopening the file has exactly the original author's blind spot — full access to *what* was decided, none to *why*. In this mode a minimal structured annotation channel (intent + spec reference, not prose) survives, because the need it serves is real and species-independent.

The decision is therefore a deployment choice, recorded here so it is made deliberately rather than by default.

---

## 11. What is deliberately absent, and why

| Absent | Reason |
|---|---|
| Nesting / indentation | Forces a depth tally; transformers are worst at counting. Flatness deletes it. |
| Variable names | Recall is a human need; reference cost is met by handles (a copy, not a count). |
| Infix & precedence | No precedence table can be misapplied if none exists. |
| Mutation | Single assignment removes aliasing and stale-value bugs wholesale. |
| Imperative loops | Replaced by bounded combinators; "infinite loop" becomes unspeakable. |
| Branch/jump | `sel` is selection over existing values; no unreachable code, no path prediction. |
| Implicit type coercion | No reviewer to catch a silent coercion; all conversion is explicit. |
| Forward references | Backward-only operands make "use before def" unspeakable. |
| Syntax sugar / multiple spellings | One canonical form removes a consistency hazard for zero benefit. |
| Prose errors | The consumer is a retry loop, not a person at a terminal. |
| Exotic Unicode opcodes | Splits into multiple tokens; defeats the token-economy goal. |

---

## 12. Open questions

1. **Spec layer (§8)** is design intent, not a built mechanism. It is the highest-value next prototype.
2. **Sub-stream semantics** for `map`/`fold` need a precise closure rule for which outer handles a sub-stream may capture (proposal: explicit capture list, same backward-only rule).
3. **Constraint-state cost.** Type-tracking in the decoder mask is more grammar state than handle-tracking alone; the boundary between "rejected at sampling" and "raised before execution" needs measuring on a real tokenizer.
4. **Tokenizer fit.** The opcode mnemonics are chosen to be single tokens *in principle*; this must be verified against the specific tokenizer of whatever model writes Weft, because the whole token-economy claim depends on it.
5. **Name.** "Weft" is a placeholder.
