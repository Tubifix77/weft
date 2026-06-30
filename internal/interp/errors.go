package interp

import "encoding/json"

// WeftError is the structured error emitted to stderr (RUNTIME-ARCHITECTURE.md §8).
type WeftError struct {
	Code     string   `json:"code"`
	OpIndex  int      `json:"op_index"`
	Operands []string `json:"operands"`
}

func (e *WeftError) Error() string {
	return e.JSON()
}

// JSON renders the single-line JSON form. Operands always serialize as a list
// (never null), even when empty.
func (e *WeftError) JSON() string {
	if e.Operands == nil {
		e.Operands = []string{}
	}
	b, _ := json.Marshal(e)
	return string(b)
}

func newError(code string, opIndex int, operands ...string) *WeftError {
	if operands == nil {
		operands = []string{}
	}
	return &WeftError{Code: code, OpIndex: opIndex, Operands: operands}
}
