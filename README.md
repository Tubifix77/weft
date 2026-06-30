# Weft

A working codename for a programming language designed to be **written by an LLM and read by no human**.

It is not a cipher over Python and not an esolang. Illegibility is a consequence of the design, not its goal: every feature whose only job was to serve a human reader, writer, or reviewer is removed, and everything that was quietly doing real work for the actual consumer (an LLM writing, a streaming interpreter reading) is kept or strengthened.

Three reasons to exist, which compound: token economy, streaming execution (every prefix of the stream is runnable), and foolproof-by-construction (whole bug classes made unspeakable by the grammar rather than caught at runtime).

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full specification.

> Status: design document. The semantic-correctness layer (ARCHITECTURE.md §8) is intent, not a built mechanism — it is the gap between "runs" and "right" and the highest-value next prototype.
