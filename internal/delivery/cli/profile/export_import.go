package profile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
)

const (
	exportPasswordEnv = "PRODEX_PROFILE_EXPORT_PASSWORD"
	importPasswordEnv = "PRODEX_PROFILE_IMPORT_PASSWORD"
)

func exportProfiles(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	request, err := parseExport(arguments)
	if err != nil {
		return err
	}
	result, err := catalog.Export(ctx, request)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Exported %d profile(s).\nPath: %s\nEncrypted: %t\n", result.ProfileCount, result.Path, result.Encrypted)
	return err
}

func importProfiles(ctx context.Context, catalog *profileusecase.Catalog, out io.Writer, arguments []string) error {
	if len(arguments) != 1 || strings.HasPrefix(arguments[0], "-") {
		return errors.New("usage: godex profile import PATH")
	}
	request := profilemodel.ImportRequest{Path: arguments[0], Password: os.Getenv(importPasswordEnv)}
	result, err := catalog.Import(ctx, request)
	if err != nil {
		if request.Password == "" && strings.Contains(err.Error(), "password") {
			return fmt.Errorf("profile export bundle is password-protected; set %s", importPasswordEnv)
		}
		return err
	}
	_, err = fmt.Fprintf(out, "Imported %d profile(s); updated %d existing profile(s).\nPath: %s\nEncrypted: %t\n", result.ImportedCount, result.UpdatedCount, result.Path, result.Encrypted)
	return err
}

type exportParseState struct {
	request         profilemodel.ExportRequest
	passwordProtect bool
	noPassword      bool
}

func parseExport(arguments []string) (profilemodel.ExportRequest, error) {
	state := exportParseState{}
	for index := 0; index < len(arguments); index++ {
		next, err := state.consume(arguments, index)
		if err != nil {
			return profilemodel.ExportRequest{}, err
		}
		index = next
	}
	return state.finish()
}

func (state *exportParseState) consume(arguments []string, index int) (int, error) {
	argument := arguments[index]
	switch {
	case argument == "--password-protect":
		state.passwordProtect = true
	case argument == "--no-password":
		state.noPassword = true
	case isExportProfileOption(argument):
		name, next, err := exportProfileValue(arguments, index)
		if err != nil {
			return index, err
		}
		state.request.Profiles = append(state.request.Profiles, name)
		return next, nil
	case strings.HasPrefix(argument, "-"):
		return index, fmt.Errorf("unknown profile export option %q", argument)
	case state.request.OutputPath == "":
		state.request.OutputPath = argument
	default:
		return index, errors.New("profile export accepts at most one output path")
	}
	return index, nil
}

func (state exportParseState) finish() (profilemodel.ExportRequest, error) {
	if state.passwordProtect && state.noPassword {
		return profilemodel.ExportRequest{}, errors.New("--password-protect cannot be combined with --no-password")
	}
	if !state.passwordProtect && !state.noPassword {
		return profilemodel.ExportRequest{}, fmt.Errorf("profile export requires --password-protect with %s set, or --no-password", exportPasswordEnv)
	}
	if state.passwordProtect {
		state.request.Password = os.Getenv(exportPasswordEnv)
		if strings.TrimSpace(state.request.Password) == "" {
			return profilemodel.ExportRequest{}, fmt.Errorf("password protection requested; set %s", exportPasswordEnv)
		}
	}
	if state.request.OutputPath == "" {
		path, err := defaultExportPath()
		if err != nil {
			return profilemodel.ExportRequest{}, err
		}
		state.request.OutputPath = path
	}
	return state.request, nil
}

func isExportProfileOption(argument string) bool {
	return argument == "-p" || argument == "--profile" || strings.HasPrefix(argument, "--profile=") || strings.HasPrefix(argument, "-p=")
}

func exportProfileValue(arguments []string, index int) (string, int, error) {
	argument := arguments[index]
	for _, flag := range []string{"-p", "--profile"} {
		if argument == flag {
			if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
				return "", index, fmt.Errorf("%s requires a profile name", flag)
			}
			return arguments[index+1], index + 1, nil
		}
		prefix := flag + "="
		if strings.HasPrefix(argument, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
			if value == "" {
				return "", index, fmt.Errorf("%s requires a profile name", flag)
			}
			return value, index, nil
		}
	}
	return "", index, errors.New("profile export selector is invalid")
}

func defaultExportPath() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	stem := "prodex-profiles-" + time.Now().Format("20060102-150405")
	for suffix := 0; suffix <= 10_000; suffix++ {
		name := stem
		if suffix > 0 {
			name = fmt.Sprintf("%s-%d", stem, suffix)
		}
		path := filepath.Join(directory, name+".json")
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("failed to allocate a unique default profile export path")
}
