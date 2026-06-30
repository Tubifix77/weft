// Command weft executes a .we file (RUNTIME-ARCHITECTURE.md §9).
package main

import (
	"fmt"
	"io"
	"os"

	"weft/internal/interp"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	trace := false
	var positional []string
	for _, a := range argv {
		switch {
		case a == "--trace":
			trace = true
		case a == "run":
			// optional verb; the default action is run
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) != 1 {
		fmt.Fprintln(os.Stderr, "usage: weft [run] [--trace] <file|->")
		return 2
	}

	var r io.Reader
	if positional[0] == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(positional[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		defer f.Close()
		r = f
	}

	ip := &interp.Interp{Stdout: os.Stdout, Stderr: os.Stderr, Trace: trace}
	if werr := ip.Run(r); werr != nil {
		fmt.Fprintln(os.Stderr, werr.JSON())
		return 1
	}
	return 0
}
