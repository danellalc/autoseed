package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/danellalc/autoseed/entseed"
)

func runExplain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	schemaPath := fs.String("schema", "", "path to the ent schema package (e.g. ./ent/schema)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), "Usage: autoseed explain --schema <path>")
		fs.PrintDefaults()
	}

	for _, arg := range args {
		if arg == "-h" || arg == "-help" || arg == "--help" {
			fs.SetOutput(stdout)
			fs.Usage()
			return 0
		}
	}

	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *schemaPath == "" {
		fs.Usage()
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "autoseed: unexpected argument %q\n\n", fs.Arg(0))
		fs.Usage()
		return 2
	}

	plan, err := entseed.Explain(*schemaPath)
	if err != nil {
		// err is already self-identifying -- entseed's own errors start
		// with "entseed:", autoseed's core sentinels with "autoseed:" --
		// so no extra prefix is added here to avoid doubling it.
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	_, _ = fmt.Fprint(stdout, plan.Report())
	return 0
}
