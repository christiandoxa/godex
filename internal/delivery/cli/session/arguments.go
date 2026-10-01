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
	flags, includeSubagents := newSessionFlagSet(&options, current)
	if err := flags.Parse(arguments); err != nil {
		return options, err
	}
	markLimitSet(flags, &options)
	if err := validateListOptions(flags, options, *includeSubagents); err != nil {
		return options, err
	}
	if current {
		if err := resolveCurrentDirectory(&options.query); err != nil {
			return options, err
		}
	}
	return options, nil
}

func newSessionFlagSet(options *listOptions, current bool) (*flag.FlagSet, *bool) {
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
	return flags, includeSubagents
}

func markLimitSet(flags *flag.FlagSet, options *listOptions) {
	flags.Visit(func(current *flag.Flag) {
		if current.Name == "limit" {
			options.query.LimitSet = true
		}
	})
}

func validateListOptions(flags *flag.FlagSet, options listOptions, includeSubagents bool) error {
	if flags.NArg() != 0 {
		return errors.New("session list/current does not accept positional arguments")
	}
	if options.query.Limit < 0 {
		return errors.New("session limit must not be negative")
	}
	if includeSubagents && options.query.ParentOnly {
		return errors.New("--include-subagents cannot be combined with --parent-only")
	}
	if selectedOutputModes(options) > 1 {
		return errors.New("--json, --id-only, and --resume-command cannot be combined")
	}
	return nil
}

func selectedOutputModes(options listOptions) int {
	modes := 0
	for _, selected := range []bool{options.json, options.idOnly, options.resumeCommand} {
		if selected {
			modes++
		}
	}
	return modes
}

func resolveCurrentDirectory(query *sessionmodel.Query) error {
	var err error
	if query.CurrentDir == "" {
		query.CurrentDir, err = os.Getwd()
	} else {
		query.CurrentDir, err = filepath.Abs(query.CurrentDir)
	}
	return err
}
