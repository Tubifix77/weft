# Measurement — token economy (2026-07-01)

One of Weft's three stated reasons to exist (`ARCHITECTURE.md` §1) was **token economy**: the claim that, with no human reader, a Weft program would cost fewer tokens to generate than the equivalent in a conventional language. This document records a direct test of that claim. **It does not hold.**

## Method

Nine semantically equivalent program pairs — the same computation written once in Weft (the actual `examples/*.we` files) and once in idiomatic Python. Each program text was tokenized with `prompt_eval_count` in Ollama raw mode (bypasses the chat template, so the count is exactly the program's tokens), baseline-subtracted. Run across three tokenizer families to rule out a single-BPE artifact.

## Result

Per-program, with `qwen2.5-coder:7b`:

| Task | Weft | Python | Ratio (W / Py) |
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

Cross-checked across tokenizer families (totals):

| Tokenizer | Weft | Python | Ratio |
|---|---|---|---|
| qwen2.5-coder:7b | 472 | 132 | 3.58x |
| phi4:14b | 466 | 126 | 3.70x |
| gemma3:12b | 509 | 159 | 3.20x |

Every program loses, in every tokenizer, by roughly 3–4x. **Weft costs ~3.6x more tokens than idiomatic Python**, not fewer.

## Why (structural, not a tuning bug)

1. **One-op-per-line decomposition.** A nested expression like `(5+3)*2` is one Python expression but five Weft instructions. Flattening multiplies instruction count.
2. **Explicit handles.** Every instruction spends tokens naming its result (`r2`) and referencing operands by handle (`r0 r1`) where Python writes an inline `+`.
3. **Typed-literal opcodes.** `r0 lit.i 5` introduces a constant in ~4 tokens; Python writes `5`.
4. **Poor BPE fit.** Weft's invented mnemonics (`lit.i`, `fn`, `r0`) aren't in code-trained merge tables, so they fragment (~1.2 chars/token vs Python's ~1.7). Meanwhile the keyword savings Weft was meant to bank barely apply — small idiomatic Python has almost no boilerplate to remove.

The deeper point: the features that make Weft foolproof and streamable (flat structure, explicit single-assignment handles, one op per line, explicit typing) are *the same features* that make it token-expensive. They are in direct tension. You cannot get the foolproofing without paying the token cost under this design.

## Could it be fixed?

A custom tokenizer with Weft opcodes and handle forms as single tokens would recover part of the gap — but that requires training or adapting the model's tokenizer, which contradicts the design's "untouched model + constrained decoding" premise (`ARCHITECTURE.md` §7). It is not a free fix, and it trades away the property that lets Weft run on any stock model.

## Effect on the three reasons to exist

- **Token economy (§1, reason 1): refuted.** ~3.6x worse, tokenizer-robust.
- **Streaming speed (§1, reason 2): still untested — and the bar is now higher.** Any wall-clock win depends on overlapping generation and execution (needs the constrained decoder + a live model, neither built). That overlap must now overcome a 3.6x token penalty before it nets out positive, so this result makes the claim *less* likely to pay off, not merely unproven.
- **Foolproof-by-construction (§1, reason 3): stands.** Demonstrated by the working interpreter — malformed programs can't run, errors are structured, iteration can't loop forever.

**Honest current value of Weft: a foolproof, terminating-by-construction execution sandbox — not a cheaper or (yet) faster one.** That is a real property, but narrower than the original pitch, and the central economy claim is now known to be wrong.
