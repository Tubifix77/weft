package interp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// expectation mirrors one entry in examples/expected.json.
type expectation struct {
	Exit   int        `json:"exit"`
	Stdout []string   `json:"stdout"`
	Error  *WeftError `json:"error"`
}

func TestGolden(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	data, err := os.ReadFile(filepath.Join(root, "expected.json"))
	if err != nil {
		t.Fatalf("read expected.json: %v", err)
	}
	var cases map[string]expectation
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse expected.json: %v", err)
	}

	for name, exp := range cases {
		t.Run(name, func(t *testing.T) {
			src, err := os.Open(filepath.Join(root, name))
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer src.Close()

			var stdout, stderr bytes.Buffer
			ip := &Interp{Stdout: &stdout, Stderr: &stderr}
			werr := ip.Run(src)

			gotExit := 0
			if werr != nil {
				gotExit = 1
			}
			if gotExit != exp.Exit {
				t.Fatalf("exit: got %d want %d (err=%v, stdout=%q)", gotExit, exp.Exit, werr, stdout.String())
			}

			if exp.Error != nil {
				if werr == nil {
					t.Fatalf("expected error %+v, got success (stdout=%q)", exp.Error, stdout.String())
				}
				if werr.Code != exp.Error.Code || werr.OpIndex != exp.Error.OpIndex || !reflect.DeepEqual(werr.Operands, exp.Error.Operands) {
					t.Fatalf("error mismatch:\n got  %+v\n want %+v", werr, exp.Error)
				}
				if stdout.Len() != 0 {
					t.Fatalf("expected no stdout on error, got %q", stdout.String())
				}
				return
			}

			if werr != nil {
				t.Fatalf("unexpected error: %v", werr)
			}
			var gotLines []string
			s := stdout.String()
			if s != "" {
				gotLines = strings.Split(strings.TrimRight(s, "\n"), "\n")
			}
			want := exp.Stdout
			if want == nil {
				want = []string{}
			}
			if gotLines == nil {
				gotLines = []string{}
			}
			if !reflect.DeepEqual(gotLines, want) {
				t.Fatalf("stdout mismatch:\n got  %q\n want %q", gotLines, want)
			}
		})
	}
}
