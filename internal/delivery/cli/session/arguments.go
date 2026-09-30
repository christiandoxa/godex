package session

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type listOptions struct {
	query                       sessionmodel.Query
	json, idOnly, resumeCommand bool
}

func parseArguments(current bool, arguments []string) (listOptions, error) {
	options := listOptions{}
	flags := flag.NewFlagSet("session", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.json, "json", false, "")
	flags.BoolVar(&options.idOnly, "id-only", false, "")
	flags.BoolVar(&options.resumeCommand, "resume-command", false, "")
	flags.StringVar(&options.query.Profile, "profile", "", "")
	flags.StringVar(&options.query.Text, "query", "", "")
	flags.IntVar(&options.query.Limit, "limit", 0, "")
	flags.BoolVar(&options.query.ParentOnly, "parent-only", false, "")
	includeSubagents := flags.Bool("include-subagents", false, "")
	if current {
		flags.StringVar(&options.query.CurrentDir, "cwd", "", "")
	}
	if err := flags.Parse(arguments); err != nil {
		return options, err
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "limit" {
			options.query.LimitSet = true
		}
	})
	if flags.NArg() != 0 {
		return options, errors.New("session list/current does not accept positional arguments")
	}
	if options.query.Limit < 0 {
		return options, errors.New("session limit must not be negative")
	}
	if *includeSubagents && options.query.ParentOnly {
		return options, errors.New("--include-subagents cannot be combined with --parent-only")
	}
	modes := 0
	for _, selected := range []bool{options.json, options.idOnly, options.resumeCommand} {
		if selected {
			modes++
		}
	}
	if modes > 1 {
		return options, errors.New("--json, --id-only, and --resume-command cannot be combined")
	}
	if current {
		var err error
		if options.query.CurrentDir == "" {
			options.query.CurrentDir, err = os.Getwd()
		} else {
			options.query.CurrentDir, err = filepath.Abs(options.query.CurrentDir)
		}
		if err != nil {
			return options, err
		}
	}
	return options, nil
}
