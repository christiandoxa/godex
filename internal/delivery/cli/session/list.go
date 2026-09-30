package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func Run(ctx context.Context, catalog *sessionusecase.Catalog, out io.Writer, arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("session requires list, current, or resume")
	}
	if arguments[0] == "resume" {
		if len(arguments) != 2 {
			return errors.New("session resume requires one session id")
		}
		return catalog.Resume(ctx, arguments[1])
	}
	if arguments[0] != "list" && arguments[0] != "current" {
		return fmt.Errorf("unknown session command %q", arguments[0])
	}
	options, err := parseArguments(arguments[0] == "current", arguments[1:])
	if err != nil {
		return err
	}
	reports, err := catalog.List(ctx, options.query)
	if err != nil {
		return err
	}
	return printReports(out, reports, options)
}

func printReports(out io.Writer, reports []sessionmodel.Report, options listOptions) error {
	if options.json {
		return json.NewEncoder(out).Encode(reports)
	}
	if !options.idOnly && !options.resumeCommand {
		if _, err := fmt.Fprintln(out, "ID\tPROFILE\tUPDATED_AT\tNAME\tCWD"); err != nil {
			return err
		}
	}
	for _, report := range reports {
		var err error
		switch {
		case options.idOnly:
			_, err = fmt.Fprintln(out, report.ID)
		case options.resumeCommand:
			_, err = fmt.Fprintf(out, "godex session resume %s\n", strconv.Quote(report.ID))
		default:
			_, err = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", displayField(report.ID), displayField(report.Profile), displayField(report.UpdatedAt), displayField(report.ThreadName), displayField(report.CWD))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Rollout text is untrusted: do not let metadata inject terminal control codes.
func displayField(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, value)
}
