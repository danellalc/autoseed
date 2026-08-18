package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_ExplainPrintsInsertionOrder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"explain", "--schema", "./testdata/schema"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(explain) exit code = %d, want 0, stderr = %s", code, stderr.String())
	}

	report := stdout.String()
	customerPos := strings.Index(report, "Customer")
	orderPos := strings.Index(report, "Order")
	if customerPos == -1 || orderPos == -1 || customerPos > orderPos {
		t.Fatalf("Customer must be reported before Order, got:\n%s", report)
	}
}

func TestRun_ExplainMissingSchemaFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"explain"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(explain, no --schema) exit code = %d, want 2", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr empty, want usage text naming the missing --schema flag")
	}
}

func TestRun_ExplainBadSchemaPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"explain", "--schema", "./does-not-exist"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run(explain, bad path) exit code = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr empty, want an error naming the unreadable schema path")
	}
	if strings.Contains(stderr.String(), "autoseed: autoseed:") || strings.Contains(stderr.String(), "autoseed: entseed:") {
		t.Fatalf("stderr = %q, want no doubled prefix -- entseed's own error already self-identifies", stderr.String())
	}
}

// TestRun_ExplainSchemaPathWithNoEntSchema guards a plausible user
// mistake -- pointing --schema at a real, syntactically valid Go
// package that just isn't an ent schema (e.g. the generated ent output
// directory instead of ent/schema) -- fails cleanly with exit 1 and a
// message naming the path, not a panic or an empty report.
func TestRun_ExplainSchemaPathWithNoEntSchema(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"explain", "--schema", "./testdata/notaschema"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run(explain, schema-less package) exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing written on failure", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr empty, want an error naming the schema-less path")
	}
}

// TestRun_ExplainEmptySchemaFlagValue guards --schema= (the flag
// present with an explicit empty value) the same way as --schema being
// entirely absent -- both mean "no schema path was given."
func TestRun_ExplainEmptySchemaFlagValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"explain", "--schema="}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(explain, --schema=) exit code = %d, want 2", code)
	}
}

// TestRun_ExplainTrailingArgument guards that an unrecognized positional
// argument after a valid --schema flag is rejected as a usage error,
// not silently ignored -- a mistyped second flag (missing its leading
// --) should not look like a successful run.
func TestRun_ExplainTrailingArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"explain", "--schema", "./testdata/schema", "unexpected"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(explain, trailing argument) exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unexpected") {
		t.Fatalf("stderr = %q, want it to name the unexpected argument", stderr.String())
	}
}

// TestRun_ExplainHelp guards that explain --help/-h succeeds (exit 0,
// usage on stdout) the same way the top-level --help does, rather than
// being treated as a usage error.
func TestRun_ExplainHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{"explain", flag}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("run(explain, %s) exit code = %d, want 0", flag, code)
		}
		if stdout.Len() == 0 {
			t.Fatalf("run(explain, %s): stdout empty, want usage text", flag)
		}
		if stderr.Len() != 0 {
			t.Fatalf("run(explain, %s): stderr = %q, want nothing written to stderr on a successful help request", flag, stderr.String())
		}
	}
}

func TestRun_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(no args) exit code = %d, want 2", code)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run(bogus) exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "bogus") {
		t.Fatalf("stderr = %q, want it to name the unknown command", stderr.String())
	}
}

func TestRun_Help(t *testing.T) {
	for _, arg := range []string{"-h", "-help", "--help", "help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{arg}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("run(%s) exit code = %d, want 0", arg, code)
		}
		if stdout.Len() == 0 {
			t.Fatalf("run(%s): stdout empty, want usage text", arg)
		}
	}
}
