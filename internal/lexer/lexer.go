// Package lexer turns a line of Weft source into tokens and parses literals.
package lexer

import (
	"strconv"
	"strings"
)

// Fields splits a line on whitespace (RUNTIME-ARCHITECTURE.md §5).
func Fields(line string) []string {
	return strings.Fields(line)
}

// HandleIndex parses a result handle "rN" into its index N. Returns ok=false
// for anything that is not exactly 'r' followed by one or more digits.
func HandleIndex(tok string) (int, bool) {
	return refIndex(tok, 'r')
}

// RefIndex parses an operand reference of the given prefix ('r', 'a', or 'c')
// into its index. Used by the interpreter's operand resolver.
func RefIndex(tok string, prefix byte) (int, bool) {
	return refIndex(tok, prefix)
}

func refIndex(tok string, prefix byte) (int, bool) {
	if len(tok) < 2 || tok[0] != prefix {
		return 0, false
	}
	for i := 1; i < len(tok); i++ {
		if tok[i] < '0' || tok[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(tok[1:])
	if err != nil {
		return 0, false
	}
	return n, true
}

// ParseInt parses a lit.i operand.
func ParseInt(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// ParseFloat parses a lit.f operand.
func ParseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// ParseBool parses a lit.b operand (only the barewords true/false).
func ParseBool(s string) (bool, bool) {
	switch s {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}
