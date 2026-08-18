// Command autoseed is a CLI wrapper around entseed.Explain: it reads an
// ent schema straight from source and prints the same insertion-order and
// skip report the Go API's Explain returns, with no database connection
// and no generated *ent.Client needed.
//
// There is no gormseed equivalent, and no seed subcommand for either
// adapter: GORM has no path-based model discovery the way ent's
// entc.LoadGraph gives it, and writing real rows needs a live client this
// CLI has no way to obtain for an arbitrary project. Use the Go API
// (gormseed.Seed/Explain, entseed.Seed) from within your own module for
// those — see ARCHITECTURE.md's "Design decisions" for why.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `autoseed is a CLI for the autoseed database-seeding library.

Usage:
  autoseed explain --schema <path>
      Read an ent schema and print its insertion plan: order, deferred
      cycles, and skipped constructs. No database, no generated client.

autoseed explain works for ent schemas only. GORM models and writing real
rows (Seed, for either adapter) need a live client this CLI cannot obtain
for an arbitrary project -- use the Go API from within your own module.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "explain":
		return runExplain(args[1:], stdout, stderr)
	case "-h", "-help", "--help", "help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "autoseed: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
