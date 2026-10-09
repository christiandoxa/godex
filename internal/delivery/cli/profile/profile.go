package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
)

func Run(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	if catalog == nil {
		return errors.New("profile support is not configured")
	}
	if len(arguments) == 0 {
		return errors.New("profile requires add, export, import, list, current, use, remove, or import-current")
	}
	switch arguments[0] {
	case "add":
		return add(ctx, catalog, out, arguments[1:])
	case "list":
		return list(ctx, catalog, out, arguments[1:])
	case "export":
		return exportProfiles(ctx, catalog, out, arguments[1:])
	case "import":
		return importProfiles(ctx, catalog, out, arguments[1:])
	case "current":
		return current(ctx, catalog, out, arguments[1:])
	case "use":
		return use(ctx, catalog, out, arguments[1:])
	case "remove":
		return remove(ctx, catalog, out, arguments[1:])
	default:
		return fmt.Errorf("unknown profile command %q", arguments[0])
	}
}

func add(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	request, err := parseAdd(arguments)
	if err != nil {
		return err
	}
	report, err := catalog.Add(ctx, request)
	if err != nil {
		return err
	}
	storage := "external"
	if report.Profile.Managed {
		storage = "managed"
	}
	_, err = fmt.Fprintf(out, "Added profile %s.\nCODEX_HOME: %s\nStorage: %s\n", report.Profile.Name, report.Profile.CodexHome, storage)
	return err
}

func list(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	if len(arguments) != 0 {
		return errors.New("profile list does not accept arguments")
	}
	reports, err := catalog.List(ctx)
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		_, err = fmt.Fprintln(out, "No profiles configured.")
		return err
	}
	writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "CURRENT\tNAME\tPROVIDER\tMANAGED\tEMAIL\tCODEX_HOME"); err != nil {
		return err
	}
	for _, report := range reports {
		marker := ""
		if report.Active {
			marker = "*"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%s\t%s\n", marker, report.Profile.Name, report.Profile.Provider.Kind, report.Profile.Managed, report.Profile.Email, report.Profile.CodexHome); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func current(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	if len(arguments) != 0 {
		return errors.New("current does not accept arguments")
	}
	report, err := catalog.Current(ctx)
	if err != nil {
		if !errors.Is(err, profileusecase.ErrNoActiveProfile) {
			return err
		}
		reports, listErr := catalog.List(ctx)
		if listErr != nil {
			return listErr
		}
		if _, writeErr := fmt.Fprintln(out, "No active profile."); writeErr != nil {
			return writeErr
		}
		if len(reports) == 1 {
			_, writeErr := fmt.Fprintf(out, "Only profile: %s\nCODEX_HOME: %s\n", reports[0].Profile.Name, reports[0].Profile.CodexHome)
			return writeErr
		}
		return nil
	}
	_, err = fmt.Fprintf(out, "Profile: %s\nProvider: %s\nCODEX_HOME: %s\nManaged: %t\n", report.Profile.Name, report.Profile.Provider.Kind, report.Profile.CodexHome, report.Profile.Managed)
	return err
}

func use(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	name, err := parseProfileSelector(arguments)
	if err != nil {
		return err
	}
	report, err := catalog.Use(ctx, name)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Active profile: %s\n", report.Profile.Name)
	return err
}

func remove(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	request, err := parseRemove(arguments)
	if err != nil {
		return err
	}
	removed, err := catalog.Remove(ctx, request)
	if err != nil {
		return err
	}
	for _, report := range removed {
		if _, err := fmt.Fprintf(out, "Removed profile %s.\n", report.Profile.Name); err != nil {
			return err
		}
	}
	return nil
}

func parseAdd(arguments []string) (profilemodel.AddRequest, error) {
	request := profilemodel.AddRequest{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--copy-current":
			request.CopyCurrent = true
		case argument == "--activate":
			request.Activate = true
		case argument == "--insecure":
			request.Insecure = true
		case argument == "--codex-home" || strings.HasPrefix(argument, "--codex-home="):
			value, next, err := argumentValue(arguments, index, "--codex-home")
			if err != nil {
				return request, err
			}
			request.CodexHome, index = value, next
		case argument == "--copy-from" || strings.HasPrefix(argument, "--copy-from="):
			value, next, err := argumentValue(arguments, index, "--copy-from")
			if err != nil {
				return request, err
			}
			request.CopyFrom, index = value, next
		case strings.HasPrefix(argument, "-"):
			return request, fmt.Errorf("unknown profile add option %q", argument)
		case request.Name == "":
			request.Name = argument
		default:
			return request, errors.New("profile add accepts exactly one profile name")
		}
	}
	if request.Name == "" {
		return request, errors.New("usage: godex profile add NAME [--codex-home PATH|--copy-from PATH|--copy-current] [--activate] [--insecure]")
	}
	return request, nil
}

func parseRemove(arguments []string) (profilemodel.RemoveRequest, error) {
	request := profilemodel.RemoveRequest{}
	for _, argument := range arguments {
		switch argument {
		case "--all":
			request.All = true
		case "--delete-home":
			request.DeleteHome = true
		default:
			if strings.HasPrefix(argument, "-") {
				return request, fmt.Errorf("unknown profile remove option %q", argument)
			}
			if request.Name != "" {
				return request, errors.New("profile remove accepts one name or --all")
			}
			request.Name = argument
		}
	}
	if request.All && request.Name != "" {
		return request, errors.New("profile name cannot be combined with --all")
	}
	if !request.All && request.Name == "" {
		return request, errors.New("provide a profile name or pass --all")
	}
	return request, nil
}

func parseProfileSelector(arguments []string) (string, error) {
	if len(arguments) == 1 && !strings.HasPrefix(arguments[0], "-") {
		return arguments[0], nil
	}
	if len(arguments) == 2 && (arguments[0] == "--profile" || arguments[0] == "-p") && strings.TrimSpace(arguments[1]) != "" {
		return arguments[1], nil
	}
	if len(arguments) == 1 {
		for _, prefix := range []string{"--profile=", "-p="} {
			if strings.HasPrefix(arguments[0], prefix) {
				value := strings.TrimSpace(strings.TrimPrefix(arguments[0], prefix))
				if value != "" {
					return value, nil
				}
			}
		}
	}
	return "", errors.New("usage: godex profile use --profile NAME")
}

func argumentValue(arguments []string, index int, name string) (string, int, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
			return "", index, fmt.Errorf("%s requires a value", name)
		}
		return arguments[index+1], index + 1, nil
	}
	value := strings.TrimSpace(strings.TrimPrefix(argument, name+"="))
	if value == "" {
		return "", index, fmt.Errorf("%s requires a value", name)
	}
	return value, index, nil
}
