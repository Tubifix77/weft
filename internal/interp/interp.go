// Package interp is the Weft execution engine: a line-walking interpreter that
// streams instructions, keeps the environment as a slice indexed by handle
// number, and dispatches each opcode to a handler.
package interp

import (
	"bufio"
	"fmt"
	"io"

	"weft/internal/lexer"
	"weft/internal/value"
)

// Interp holds the I/O endpoints and run-time flags for one execution.
type Interp struct {
	Stdout io.Writer
	Stderr io.Writer
	Trace  bool
}

// execCtx is the resolution scope for operand references. At the top level only
// env (rK) is populated; inside a sub-stream body, args (aK) and captures (cK)
// are non-nil too.
type execCtx struct {
	env      []value.Value
	args     []value.Value
	captures []value.Value
}

// resolve turns an operand token into a value, honoring the rK / aK / cK
// namespaces. ok=false means an undefined or malformed reference.
func (c *execCtx) resolve(tok string) (value.Value, bool) {
	if idx, ok := lexer.RefIndex(tok, 'r'); ok {
		if idx >= 0 && idx < len(c.env) {
			return c.env[idx], true
		}
		return value.Value{}, false
	}
	if idx, ok := lexer.RefIndex(tok, 'a'); ok {
		if idx >= 0 && idx < len(c.args) {
			return c.args[idx], true
		}
		return value.Value{}, false
	}
	if idx, ok := lexer.RefIndex(tok, 'c'); ok {
		if idx >= 0 && idx < len(c.captures) {
			return c.captures[idx], true
		}
		return value.Value{}, false
	}
	return value.Value{}, false
}

// Run executes the program read from r. It returns nil on success (with `out`
// values already written to Stdout), or the first WeftError encountered.
func (ip *Interp) Run(r io.Reader) *WeftError {
	env := []value.Value{}
	opIndex := 0

	// Sub-stream (fn) buffering state — a single bit plus its working buffers.
	inFn := false
	var fnArity int
	var fnCaptures []value.Value
	var fnBody []value.Instruction
	var fnLocalCount int
	var fnOpenIndex int // opIndex of the `fn` line, for NO_RET reporting

	scanner := bufio.NewScanner(r)
	// Allow generous line lengths for hand-written or generated programs.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		fields := lexer.Fields(line)
		if len(fields) == 0 {
			continue // blank line — ignored, not counted
		}
		if len(fields) < 2 {
			return newError("PARSE", opIndex, fields...)
		}
		inst := value.Instruction{
			Handle:  fields[0],
			Op:      fields[1],
			Args:    fields[2:],
			OpIndex: opIndex,
		}

		if inFn {
			switch inst.Op {
			case "ret":
				cl, err := ip.closeClosure(inst, fnArity, fnCaptures, fnBody, fnLocalCount, &env)
				if err != nil {
					return err
				}
				_ = cl
				inFn = false
				fnBody = nil
				fnCaptures = nil
			case "fn":
				return newError("NEST", opIndex)
			default:
				if err := appendBodyLine(inst, &fnBody, &fnLocalCount); err != nil {
					return err
				}
			}
			opIndex++
			continue
		}

		if inst.Op == "fn" {
			arity, captures, err := ip.openClosure(inst, env)
			if err != nil {
				return err
			}
			// The closure takes the next top-level handle; validate ordering now.
			h, ok := lexer.HandleIndex(inst.Handle)
			if !ok || h != len(env) {
				return newError("PARSE", opIndex, inst.Handle)
			}
			inFn = true
			fnArity = arity
			fnCaptures = captures
			fnBody = nil
			fnLocalCount = 0
			fnOpenIndex = opIndex
			opIndex++
			continue
		}

		if inst.Op == "ret" {
			// `ret` outside a sub-stream is meaningless.
			return newError("BAD_OP", opIndex, inst.Op)
		}

		result, hasVal, err := ip.dispatch(inst, &execCtx{env: env})
		if err != nil {
			return err
		}

		if inst.Handle != "_" {
			h, ok := lexer.HandleIndex(inst.Handle)
			if !ok || h != len(env) {
				return newError("PARSE", opIndex, inst.Handle)
			}
			if !hasVal {
				// A value handle on an effect-only op breaks the ordering invariant.
				return newError("PARSE", opIndex, inst.Handle)
			}
			env = append(env, result)
		}

		if ip.Trace {
			ip.traceLine(opIndex, env)
		}
		opIndex++
	}

	if inFn {
		return newError("NO_RET", fnOpenIndex)
	}
	if err := scanner.Err(); err != nil {
		return newError("PARSE", opIndex)
	}
	return nil
}

// openClosure parses a `fn <arity> <capture>*` header, snapshotting capture
// values from the top-level env.
func (ip *Interp) openClosure(inst value.Instruction, env []value.Value) (int, []value.Value, *WeftError) {
	if len(inst.Args) < 1 {
		return 0, nil, newError("ARITY", inst.OpIndex, inst.Args...)
	}
	arity, ok := lexer.ParseInt(inst.Args[0])
	if !ok || arity < 0 {
		return 0, nil, newError("PARSE", inst.OpIndex, inst.Args[0])
	}
	captures := []value.Value{}
	for _, tok := range inst.Args[1:] {
		idx, ok := lexer.HandleIndex(tok)
		if !ok || idx < 0 || idx >= len(env) {
			return 0, nil, newError("BAD_HANDLE", inst.OpIndex, tok)
		}
		captures = append(captures, env[idx])
	}
	return int(arity), captures, nil
}

// appendBodyLine validates a sub-stream body line's handle ordering against the
// local counter and appends it to the body buffer.
func appendBodyLine(inst value.Instruction, body *[]value.Instruction, localCount *int) *WeftError {
	if inst.Handle != "_" {
		h, ok := lexer.HandleIndex(inst.Handle)
		if !ok || h != *localCount {
			return newError("PARSE", inst.OpIndex, inst.Handle)
		}
		*localCount++
	}
	*body = append(*body, inst)
	return nil
}

// closeClosure handles a `_ ret rK` line: it records the yielded local handle,
// finalizes the closure value, and appends it to the top-level env.
func (ip *Interp) closeClosure(inst value.Instruction, arity int, captures []value.Value, body []value.Instruction, localCount int, env *[]value.Value) (value.Value, *WeftError) {
	if inst.Handle != "_" {
		return value.Value{}, newError("PARSE", inst.OpIndex, inst.Handle)
	}
	if len(inst.Args) != 1 {
		return value.Value{}, newError("ARITY", inst.OpIndex, inst.Args...)
	}
	ret, ok := lexer.HandleIndex(inst.Args[0])
	if !ok || ret < 0 || ret >= localCount {
		return value.Value{}, newError("BAD_HANDLE", inst.OpIndex, inst.Args[0])
	}
	cl := &value.Closure{
		Arity:    arity,
		Captures: captures,
		Body:     body,
		Ret:      ret,
	}
	v := value.Clo(cl)
	*env = append(*env, v)
	return v, nil
}

// runClosure executes a buffered closure body with the given bound inputs and
// returns the value at the closure's ret handle.
func (ip *Interp) runClosure(cl *value.Closure, bound []value.Value) (value.Value, *WeftError) {
	local := make([]value.Value, 0, len(cl.Body))
	for _, inst := range cl.Body {
		ctx := &execCtx{env: local, args: bound, captures: cl.Captures}
		v, hasVal, err := ip.dispatch(inst, ctx)
		if err != nil {
			return value.Value{}, err
		}
		if inst.Handle != "_" && hasVal {
			local = append(local, v)
		}
	}
	if cl.Ret < 0 || cl.Ret >= len(local) {
		return value.Value{}, newError("NO_RET", 0)
	}
	return local[cl.Ret], nil
}

func (ip *Interp) traceLine(opIndex int, env []value.Value) {
	parts := make([]string, len(env))
	for i, v := range env {
		parts[i] = fmt.Sprintf("r%d=%s", i, value.Format(v))
	}
	fmt.Fprintf(ip.Stderr, "op %d | env: %v\n", opIndex, parts)
}
