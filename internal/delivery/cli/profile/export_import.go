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
	request, err := parseExportWithContext(ctx, arguments)
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
	options, err := parseImport(arguments)
	if err != nil {
		return err
	}
	if builtinImportSource(options.path) {
		result, err := catalog.ImportBuiltin(ctx, profilemodel.BuiltinImportRequest{
			Source: options.path, Name: options.name, Activate: options.activate, Insecure: options.insecure,
		})
		if err != nil {
			return err
		}
		return writeBuiltinImportResult(out, result)
	}
	if options.name != "" || options.activate {
		return errors.New("--name and --activate are only supported for built-in import sources such as `claude`, `copilot`, or `kiro`")
	}
	request := profilemodel.ImportRequest{Path: options.path, Password: os.Getenv(importPasswordEnv)}
	result, err := catalog.Import(ctx, request)
	if err != nil && request.Password == "" && importRequiresPassword(err) {
		password, promptErr := resolveImportPassword(ctx)
		if promptErr != nil {
			return promptErr
		}
		request.Password = password
		result, err = catalog.Import(ctx, request)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Imported %d profile(s); updated %d existing profile(s).\nPath: %s\nEncrypted: %t\n", result.ImportedCount, result.UpdatedCount, result.Path, result.Encrypted)
	return err
}

type importCLIOptions struct {
	path     string
	name     string
	activate bool
	insecure bool
}

func parseImport(arguments []string) (importCLIOptions, error) {
	options := importCLIOptions{}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--activate":
			options.activate = true
		case argument == "--insecure":
			options.insecure = true
		case argument == "--name" || strings.HasPrefix(argument, "--name="):
			value, next, valueErr := profileImportOptionValue(arguments, index, "--name")
			if valueErr != nil {
				return importCLIOptions{}, valueErr
			}
			options.name, index = value, next
		case strings.HasPrefix(argument, "-"):
			return importCLIOptions{}, fmt.Errorf("unknown profile import option %q", argument)
		case options.path == "":
			options.path = argument
		default:
			return importCLIOptions{}, errors.New("profile import accepts exactly one path or built-in source")
		}
	}
	if options.path == "" {
		return importCLIOptions{}, errors.New("usage: godex profile import PATH_OR_SOURCE [--name NAME] [--activate] [--insecure]")
	}
	return options, nil
}

func profileImportOptionValue(arguments []string, index int, name string) (string, int, error) {
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

func builtinImportSource(path string) bool {
	if path != "claude" && path != "copilot" && path != "kiro" {
		return false
	}
	_, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist)
}

func writeBuiltinImportResult(out io.Writer, result profilemodel.BuiltinImportResult) error {
	verb := "Created"
	if result.Updated {
		verb = "Updated"
	}
	if _, err := fmt.Fprintf(out, "%s %s profile %q.\nProvider: %s\nActive: %t\n", verb, result.Provider, result.Profile, result.Provider, result.Active); err != nil {
		return err
	}
	if strings.TrimSpace(result.Warning) != "" {
		_, err := fmt.Fprintf(out, "Warning: %s\n", result.Warning)
		return err
	}
	return nil
}

func importRequiresPassword(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "password")
}

func resolveImportPassword(ctx context.Context) (string, error) {
	if password := os.Getenv(importPasswordEnv); strings.TrimSpace(password) != "" {
		return password, nil
	}
	if !profilePasswordTUIAvailable() {
		return "", fmt.Errorf("profile export bundle is password-protected; set %s or rerun in a terminal", importPasswordEnv)
	}
	password, err := runPasswordEntryTUI(ctx, "Profile Import", "Export password", "Enter the password for this encrypted profile bundle.")
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("import password cannot be empty")
	}
	return password, nil
}

type exportParseState struct {
	request         profilemodel.ExportRequest
	passwordProtect bool
	noPassword      bool
}

func parseExport(arguments []string) (profilemodel.ExportRequest, error) {
	return parseExportWithContext(context.Background(), arguments)
}

func parseExportWithContext(ctx context.Context, arguments []string) (profilemodel.ExportRequest, error) {
	state := exportParseState{}
	for index := 0; index < len(arguments); index++ {
		next, err := state.consume(arguments, index)
		if err != nil {
			return profilemodel.ExportRequest{}, err
		}
		index = next
	}
	return state.finish(ctx)
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

func (state exportParseState) finish(ctx context.Context) (profilemodel.ExportRequest, error) {
	protect, err := resolveExportPasswordMode(ctx, state)
	if err != nil {
		return profilemodel.ExportRequest{}, err
	}
	if protect {
		state.request.Password, err = resolveExportPassword(ctx)
		if err != nil {
			return profilemodel.ExportRequest{}, err
		}
	}
	if state.request.OutputPath == "" {
		path, pathErr := defaultExportPath()
		if pathErr != nil {
			return profilemodel.ExportRequest{}, pathErr
		}
		state.request.OutputPath = path
	}
	return state.request, nil
}

func resolveExportPasswordMode(ctx context.Context, state exportParseState) (bool, error) {
	if state.passwordProtect && state.noPassword {
		return false, errors.New("--password-protect cannot be combined with --no-password")
	}
	if state.passwordProtect {
		return true, nil
	}
	if state.noPassword {
		return false, nil
	}
	if !profilePasswordTUIAvailable() {
		return false, fmt.Errorf("non-interactive profile export requires --password-protect with %s set, or --no-password to write an unencrypted bundle", exportPasswordEnv)
	}
	return runPasswordModeTUI(ctx)
}

func resolveExportPassword(ctx context.Context) (string, error) {
	if password := os.Getenv(exportPasswordEnv); strings.TrimSpace(password) != "" {
		return password, nil
	}
	if !profilePasswordTUIAvailable() {
		return "", fmt.Errorf("password protection requested but no interactive terminal is available; set %s", exportPasswordEnv)
	}
	password, err := runPasswordEntryTUI(ctx, "Profile Export", "Export password", "Enter a password for the encrypted profile bundle.")
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("export password cannot be empty")
	}
	confirmation, err := runPasswordEntryTUI(ctx, "Profile Export", "Confirm export password", "Enter the same password again.")
	if err != nil {
		return "", err
	}
	if password != confirmation {
		return "", errors.New("export passwords did not match")
	}
	return password, nil
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
