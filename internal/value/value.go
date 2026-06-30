// Package value defines the Weft runtime's tagged-union Value type and the
// instruction record shared across the lexer and interpreter.
package value

import (
	"strconv"
	"strings"
)

// Kind tags a Value. The language is monomorphic, so every value carries its
// kind explicitly (RUNTIME-ARCHITECTURE.md §4).
type Kind int

const (
	KInt Kind = iota
	KFloat
	KBool
	KStr
	KVec
	KClosure // v1: sub-stream / fn (ITERATION.md)
)

// Instruction is one tokenized line. It carries no resolved values, so this
// type has no dependencies and can live in the leaf value package — which lets
// a Closure buffer its body as []Instruction without an import cycle.
type Instruction struct {
	Handle  string   // "rN" or "_"
	Op      string   // mnemonic
	Args    []string // operand tokens, as written
	OpIndex int      // zero-based instruction-line index (blank lines excluded)
}

// Closure is the value a `fn` sub-stream produces (ITERATION.md §"Runtime additions").
type Closure struct {
	Arity    int
	Captures []Value
	Body     []Instruction
	Ret      int // local handle index the body yields
}

// Value is the tagged union. Only the field matching Kind is meaningful.
type Value struct {
	Kind Kind
	I    int64
	F    float64
	B    bool
	S    string
	V    []Value  // KVec
	C    *Closure // KClosure
}

// Constructors.

func Int(i int64) Value     { return Value{Kind: KInt, I: i} }
func Float(f float64) Value { return Value{Kind: KFloat, F: f} }
func Bool(b bool) Value     { return Value{Kind: KBool, B: b} }
func Str(s string) Value    { return Value{Kind: KStr, S: s} }
func Vec(v []Value) Value   { return Value{Kind: KVec, V: v} }
func Clo(c *Closure) Value  { return Value{Kind: KClosure, C: c} }

// KindName gives a human label for a kind (used in nothing user-facing in v0,
// but handy for trace output).
func KindName(k Kind) string {
	switch k {
	case KInt:
		return "int"
	case KFloat:
		return "float"
	case KBool:
		return "bool"
	case KStr:
		return "str"
	case KVec:
		return "vec"
	case KClosure:
		return "closure"
	}
	return "?"
}

// Format renders a value per RUNTIME-ARCHITECTURE.md §7 (the `out` format).
func Format(v Value) string {
	switch v.Kind {
	case KInt:
		return strconv.FormatInt(v.I, 10)
	case KFloat:
		return strconv.FormatFloat(v.F, 'g', -1, 64)
	case KBool:
		if v.B {
			return "true"
		}
		return "false"
	case KStr:
		return v.S
	case KVec:
		parts := make([]string, len(v.V))
		for i, e := range v.V {
			parts[i] = Format(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case KClosure:
		return "<closure/" + strconv.Itoa(v.C.Arity) + ">"
	}
	return "<?>"
}

// Equal compares two values of the same kind (used by eq/ne). Comparing
// different kinds is a caller-level type error, not handled here.
func Equal(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KInt:
		return a.I == b.I
	case KFloat:
		return a.F == b.F
	case KBool:
		return a.B == b.B
	case KStr:
		return a.S == b.S
	case KVec:
		if len(a.V) != len(b.V) {
			return false
		}
		for i := range a.V {
			if !Equal(a.V[i], b.V[i]) {
				return false
			}
		}
		return true
	}
	return false
}
