package interp

import (
	"fmt"
	"math"

	"weft/internal/lexer"
	"weft/internal/value"
)

// dispatch executes one non-fn/ret instruction in the given scope. It returns
// the produced value (if any), whether a value was produced, and an error.
func (ip *Interp) dispatch(inst value.Instruction, ctx *execCtx) (value.Value, bool, *WeftError) {
	op := inst.Op
	args := inst.Args
	oi := inst.OpIndex

	switch op {

	// ---- literals: one inline operand, not a handle ----
	case "lit.i":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		n, ok := lexer.ParseInt(args[0])
		if !ok {
			return value.Value{}, false, newError("PARSE", oi, args[0])
		}
		return value.Int(n), true, nil
	case "lit.f":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		f, ok := lexer.ParseFloat(args[0])
		if !ok {
			return value.Value{}, false, newError("PARSE", oi, args[0])
		}
		return value.Float(f), true, nil
	case "lit.b":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		b, ok := lexer.ParseBool(args[0])
		if !ok {
			return value.Value{}, false, newError("PARSE", oi, args[0])
		}
		return value.Bool(b), true, nil
	case "lit.s":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		return value.Str(args[0]), true, nil

	// ---- arithmetic ----
	case "add", "sub", "mul", "div", "mod":
		return ip.arith(op, args, ctx, oi)

	// ---- equality ----
	case "eq", "ne":
		if len(args) != 2 {
			return arityErr(oi, args)
		}
		a, b, err := resolve2(ctx, args, oi)
		if err != nil {
			return value.Value{}, false, err
		}
		if a.Kind != b.Kind {
			return typeErr(oi, args)
		}
		eq := value.Equal(a, b)
		if op == "ne" {
			eq = !eq
		}
		return value.Bool(eq), true, nil

	// ---- ordered comparison (numeric) ----
	case "lt", "le", "gt", "ge":
		return ip.compare(op, args, ctx, oi)

	// ---- logic ----
	case "and", "or":
		if len(args) != 2 {
			return arityErr(oi, args)
		}
		a, b, err := resolve2(ctx, args, oi)
		if err != nil {
			return value.Value{}, false, err
		}
		if a.Kind != value.KBool || b.Kind != value.KBool {
			return typeErr(oi, args)
		}
		var r bool
		if op == "and" {
			r = a.B && b.B
		} else {
			r = a.B || b.B
		}
		return value.Bool(r), true, nil
	case "not":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		a, ok := ctx.resolve(args[0])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
		}
		if a.Kind != value.KBool {
			return typeErr(oi, args)
		}
		return value.Bool(!a.B), true, nil

	// ---- selection ----
	case "sel":
		if len(args) != 3 {
			return arityErr(oi, args)
		}
		cond, ok := ctx.resolve(args[0])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
		}
		a, ok := ctx.resolve(args[1])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[1])
		}
		b, ok := ctx.resolve(args[2])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[2])
		}
		if cond.Kind != value.KBool {
			return typeErr(oi, args)
		}
		if a.Kind != b.Kind {
			return typeErr(oi, args)
		}
		if cond.B {
			return a, true, nil
		}
		return b, true, nil

	// ---- vectors ----
	case "vec":
		elems := make([]value.Value, 0, len(args))
		var kind value.Kind
		for i, tok := range args {
			v, ok := ctx.resolve(tok)
			if !ok {
				return value.Value{}, false, newError("BAD_HANDLE", oi, tok)
			}
			if i == 0 {
				kind = v.Kind
			} else if v.Kind != kind {
				return typeErr(oi, args)
			}
			elems = append(elems, v)
		}
		return value.Vec(elems), true, nil
	case "len":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		v, ok := ctx.resolve(args[0])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
		}
		if v.Kind != value.KVec {
			return typeErr(oi, args)
		}
		return value.Int(int64(len(v.V))), true, nil
	case "idx":
		if len(args) != 2 {
			return arityErr(oi, args)
		}
		v, i, err := resolve2(ctx, args, oi)
		if err != nil {
			return value.Value{}, false, err
		}
		if v.Kind != value.KVec || i.Kind != value.KInt {
			return typeErr(oi, args)
		}
		if i.I < 0 || i.I >= int64(len(v.V)) {
			return value.Value{}, false, newError("IDX_OOB", oi, args...)
		}
		return v.V[i.I], true, nil

	// ---- conversion ----
	case "i2f":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		v, ok := ctx.resolve(args[0])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
		}
		if v.Kind != value.KInt {
			return typeErr(oi, args)
		}
		return value.Float(float64(v.I)), true, nil

	// ---- guard: (pred-handle, inline code bareword) ----
	case "chk":
		if len(args) != 2 {
			return arityErr(oi, args)
		}
		pred, ok := ctx.resolve(args[0])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
		}
		if pred.Kind != value.KBool {
			return typeErr(oi, args)
		}
		if !pred.B {
			// Carry the user's bareword code as the single operand.
			return value.Value{}, false, newError("CHK_FAIL", oi, args[1])
		}
		return value.Value{}, false, nil

	// ---- effect: print ----
	case "out":
		if len(args) != 1 {
			return arityErr(oi, args)
		}
		v, ok := ctx.resolve(args[0])
		if !ok {
			return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
		}
		fmt.Fprintln(ip.Stdout, value.Format(v))
		return value.Value{}, false, nil

	// ---- iteration (v1) ----
	case "map":
		return ip.mapOp(args, ctx, oi)
	case "fold":
		return ip.foldOp(args, ctx, oi)
	}

	return value.Value{}, false, newError("BAD_OP", oi, op)
}

func (ip *Interp) arith(op string, args []string, ctx *execCtx, oi int) (value.Value, bool, *WeftError) {
	if len(args) != 2 {
		return arityErr(oi, args)
	}
	a, b, err := resolve2(ctx, args, oi)
	if err != nil {
		return value.Value{}, false, err
	}
	if a.Kind != b.Kind || (a.Kind != value.KInt && a.Kind != value.KFloat) {
		return typeErr(oi, args)
	}
	if a.Kind == value.KInt {
		switch op {
		case "add":
			return value.Int(a.I + b.I), true, nil
		case "sub":
			return value.Int(a.I - b.I), true, nil
		case "mul":
			return value.Int(a.I * b.I), true, nil
		case "div":
			if b.I == 0 {
				return value.Value{}, false, newError("DIV_ZERO", oi, args...)
			}
			return value.Int(a.I / b.I), true, nil
		case "mod":
			if b.I == 0 {
				return value.Value{}, false, newError("DIV_ZERO", oi, args...)
			}
			return value.Int(a.I % b.I), true, nil
		}
	}
	// KFloat
	switch op {
	case "add":
		return value.Float(a.F + b.F), true, nil
	case "sub":
		return value.Float(a.F - b.F), true, nil
	case "mul":
		return value.Float(a.F * b.F), true, nil
	case "div":
		return value.Float(a.F / b.F), true, nil
	case "mod":
		return value.Float(math.Mod(a.F, b.F)), true, nil
	}
	return value.Value{}, false, newError("BAD_OP", oi, op)
}

func (ip *Interp) compare(op string, args []string, ctx *execCtx, oi int) (value.Value, bool, *WeftError) {
	if len(args) != 2 {
		return arityErr(oi, args)
	}
	a, b, err := resolve2(ctx, args, oi)
	if err != nil {
		return value.Value{}, false, err
	}
	if a.Kind != b.Kind || (a.Kind != value.KInt && a.Kind != value.KFloat) {
		return typeErr(oi, args)
	}
	var r bool
	if a.Kind == value.KInt {
		switch op {
		case "lt":
			r = a.I < b.I
		case "le":
			r = a.I <= b.I
		case "gt":
			r = a.I > b.I
		case "ge":
			r = a.I >= b.I
		}
	} else {
		switch op {
		case "lt":
			r = a.F < b.F
		case "le":
			r = a.F <= b.F
		case "gt":
			r = a.F > b.F
		case "ge":
			r = a.F >= b.F
		}
	}
	return value.Bool(r), true, nil
}

// mapOp implements map(vec, fn) → vec.
func (ip *Interp) mapOp(args []string, ctx *execCtx, oi int) (value.Value, bool, *WeftError) {
	if len(args) != 2 {
		return arityErr(oi, args)
	}
	vec, ok := ctx.resolve(args[0])
	if !ok {
		return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
	}
	fn, ok := ctx.resolve(args[1])
	if !ok {
		return value.Value{}, false, newError("BAD_HANDLE", oi, args[1])
	}
	if vec.Kind != value.KVec {
		return typeErr(oi, args)
	}
	if fn.Kind != value.KClosure {
		return value.Value{}, false, newError("BAD_CLOSURE", oi, args[1])
	}
	if fn.C.Arity != 1 {
		return value.Value{}, false, newError("BAD_ARITY", oi, args[1])
	}
	results := make([]value.Value, 0, len(vec.V))
	for _, elem := range vec.V {
		r, err := ip.runClosure(fn.C, []value.Value{elem})
		if err != nil {
			return value.Value{}, false, err
		}
		if len(results) > 0 && r.Kind != results[0].Kind {
			return typeErr(oi, args)
		}
		results = append(results, r)
	}
	return value.Vec(results), true, nil
}

// foldOp implements fold(vec, init, fn) → value.
func (ip *Interp) foldOp(args []string, ctx *execCtx, oi int) (value.Value, bool, *WeftError) {
	if len(args) != 3 {
		return arityErr(oi, args)
	}
	vec, ok := ctx.resolve(args[0])
	if !ok {
		return value.Value{}, false, newError("BAD_HANDLE", oi, args[0])
	}
	acc, ok := ctx.resolve(args[1])
	if !ok {
		return value.Value{}, false, newError("BAD_HANDLE", oi, args[1])
	}
	fn, ok := ctx.resolve(args[2])
	if !ok {
		return value.Value{}, false, newError("BAD_HANDLE", oi, args[2])
	}
	if vec.Kind != value.KVec {
		return typeErr(oi, args)
	}
	if fn.Kind != value.KClosure {
		return value.Value{}, false, newError("BAD_CLOSURE", oi, args[2])
	}
	if fn.C.Arity != 2 {
		return value.Value{}, false, newError("BAD_ARITY", oi, args[2])
	}
	for _, elem := range vec.V {
		r, err := ip.runClosure(fn.C, []value.Value{acc, elem})
		if err != nil {
			return value.Value{}, false, err
		}
		if r.Kind != acc.Kind {
			return typeErr(oi, args)
		}
		acc = r
	}
	return acc, true, nil
}

// resolve2 resolves exactly two operand handles.
func resolve2(ctx *execCtx, args []string, oi int) (value.Value, value.Value, *WeftError) {
	a, ok := ctx.resolve(args[0])
	if !ok {
		return value.Value{}, value.Value{}, newError("BAD_HANDLE", oi, args[0])
	}
	b, ok := ctx.resolve(args[1])
	if !ok {
		return value.Value{}, value.Value{}, newError("BAD_HANDLE", oi, args[1])
	}
	return a, b, nil
}

func arityErr(oi int, args []string) (value.Value, bool, *WeftError) {
	return value.Value{}, false, newError("ARITY", oi, args...)
}

func typeErr(oi int, args []string) (value.Value, bool, *WeftError) {
	return value.Value{}, false, newError("TYPE_MISMATCH", oi, args...)
}
