package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	"github.com/christiandoxa/godex/internal/version"
)

const (
	defaultDoctorTailBytes = 128 << 10
	maxDoctorTailBytes     = 8 << 20
	doctorBundlePrefix     = "--bundle="
)

type doctorOptions struct {
	quota        bool
	runtime      bool
	install      bool
	tailBytes    int
	json         bool
	bundle       string
	redacted     bool
	suggest      bool
	repairImport bool
	repairIndex  bool
}

type doctorRunner interface {
	Diagnose(context.Context, runtimemodel.DoctorOptions) (runtimemodel.DoctorDiagnostics, error)
	SaveBundle(string, []byte) (string, error)
}

type doctorSessionIndexRepairer interface {
	RepairSessionIndex(context.Context) error
}

type doctorPanel struct {
	title  string
	fields [][2]string
}

func Doctor(ctx context.Context, doctor doctorRunner, out io.Writer, arguments []string) error {
	return DoctorWithErrorOutput(ctx, doctor, out, io.Discard, arguments)
}

// DoctorWithErrorOutput writes the repair completion notice to errOut.
func DoctorWithErrorOutput(ctx context.Context, doctor doctorRunner, out, errOut io.Writer, arguments []string) error {
	if doctor == nil {
		return errors.New("doctor support is not configured")
	}
	if errOut == nil {
		errOut = io.Discard
	}
	options, err := parseDoctorArguments(arguments)
	if err != nil {
		return err
	}
	if err := rejectUnsupportedDoctorActions(options); err != nil {
		return err
	}
	if options.repairIndex {
		repairer, ok := doctor.(doctorSessionIndexRepairer)
		if !ok {
			return errors.New("doctor session index repair is not configured")
		}
		if err := repairer.RepairSessionIndex(ctx); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(errOut, "godex doctor: session index repair completed."); err != nil {
			return err
		}
	}
	report, err := doctor.Diagnose(ctx, runtimemodel.DoctorOptions{
		Runtime:                  options.runtime || options.bundle != "",
		Quota:                    options.quota,
		Install:                  options.install,
		RepairImportAuthJournals: options.repairImport,
		TailBytes:                options.tailBytes,
	})
	if err != nil {
		return err
	}
	if options.bundle != "" {
		return writeDoctorBundle(doctor, out, report, options.bundle)
	}
	if options.json {
		return writeDoctorJSON(out, report)
	}
	panels := doctorPanels(report, options)
	if writerIsTerminal(out) {
		return runDoctorTUI(out, panels)
	}
	return writeDoctorPanels(out, panels)
}

func parseDoctorArguments(arguments []string) (doctorOptions, error) {
	options := doctorOptions{tailBytes: defaultDoctorTailBytes}
	for index := 0; index < len(arguments); index++ {
		next, err := consumeDoctorArgument(arguments, index, &options)
		if err != nil {
			return doctorOptions{}, err
		}
		index = next
	}
	if err := validateDoctorOptions(options); err != nil {
		return doctorOptions{}, err
	}
	return options, nil
}

func consumeDoctorArgument(arguments []string, index int, options *doctorOptions) (int, error) {
	argument := arguments[index]
	switch argument {
	case "--quota":
		options.quota = true
	case "--runtime":
		options.runtime = true
	case "--install":
		options.install = true
	case "--json":
		options.json = true
	case "--redacted":
		options.redacted = true
	case "--suggest-policy":
		options.suggest = true
	case "--repair-import-auth-journals":
		options.repairImport = true
	case "--repair-session-index":
		options.repairIndex = true
	case "--help", "-h":
		return index, errors.New(doctorUsage)
	default:
		return consumeDoctorValueArgument(arguments, index, options)
	}
	return index, nil
}

func consumeDoctorValueArgument(arguments []string, index int, options *doctorOptions) (int, error) {
	argument := arguments[index]
	if argument == "--tail-bytes" || strings.HasPrefix(argument, "--tail-bytes=") {
		value, next, err := doctorOptionValue(arguments, index, argument, "--tail-bytes")
		if err != nil {
			return index, err
		}
		parsed, err := parseDoctorTailBytes(value)
		if err != nil {
			return index, err
		}
		options.tailBytes = parsed
		return next, nil
	}
	if argument == "--bundle" || strings.HasPrefix(argument, doctorBundlePrefix) {
		value, next, err := doctorBundleValue(arguments, index, argument)
		if err != nil {
			return index, err
		}
		options.bundle = value
		return next, nil
	}
	return index, fmt.Errorf("unknown doctor option %q", argument)
}

func validateDoctorOptions(options doctorOptions) error {
	if options.json && !options.runtime {
		return errors.New("doctor --json requires --runtime")
	}
	if options.json && options.bundle != "" {
		return errors.New("doctor --json cannot be combined with --bundle")
	}
	if options.bundle != "" && !options.redacted {
		return errors.New("doctor --bundle requires --redacted")
	}
	if options.redacted && options.bundle == "" {
		return errors.New("doctor --redacted requires --bundle")
	}
	if options.suggest && !options.runtime {
		return errors.New("doctor --suggest-policy requires --runtime")
	}
	return nil
}

func rejectUnsupportedDoctorActions(options doctorOptions) error {
	if options.suggest {
		return errors.New("doctor --suggest-policy is not available until runtime policy parity is implemented")
	}
	return nil
}

func doctorOptionValue(arguments []string, index int, argument, name string) (string, int, error) {
	if argument == name {
		if index+1 >= len(arguments) || strings.HasPrefix(arguments[index+1], "-") {
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

func doctorBundleValue(arguments []string, index int, argument string) (string, int, error) {
	if strings.HasPrefix(argument, doctorBundlePrefix) {
		value := strings.TrimSpace(strings.TrimPrefix(argument, doctorBundlePrefix))
		if value == "" {
			return "", index, errors.New("--bundle path must not be empty")
		}
		return value, index, nil
	}
	if index+1 < len(arguments) && (arguments[index+1] == "-" || !strings.HasPrefix(arguments[index+1], "-")) {
		return arguments[index+1], index + 1, nil
	}
	return "-", index, nil
}

func parseDoctorTailBytes(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 || parsed > maxDoctorTailBytes {
		return 0, fmt.Errorf("invalid --tail-bytes value %q; expected 0..%d", value, maxDoctorTailBytes)
	}
	return parsed, nil
}

func doctorPanels(report runtimemodel.DoctorDiagnostics, options doctorOptions) []doctorPanel {
	if options.install && !options.runtime && !options.quota && !options.repairImport && !options.repairIndex {
		return []doctorPanel{{title: "Install Checks", fields: doctorCheckFields(report.Install)}}
	}
	panels := []doctorPanel{{title: "Doctor", fields: [][2]string{
		{"Godex root", report.GodexHome},
		{"Codex", report.CodexVersion},
		{"Accounts", fmt.Sprintf("%d (%d enabled)", report.AccountCount, report.EnabledCount)},
	}}}
	if report.ImportAuthJournals != nil {
		panels[0].fields = append(panels[0].fields, [2]string{
			"Import auth journals", formatDoctorImportAuthJournals(*report.ImportAuthJournals),
		})
	}
	if options.install {
		panels = append(panels, doctorPanel{title: "Install Checks", fields: doctorCheckFields(report.Install)})
	}
	if report.Runtime != nil {
		panels = append(panels, doctorRuntimePanel(*report.Runtime))
	}
	for _, quota := range report.Quota {
		panels = append(panels, doctorQuotaPanel(quota))
	}
	return panels
}

func formatDoctorImportAuthJournals(status runtimemodel.DoctorImportAuthJournals) string {
	if status.RepairPerformed {
		if status.OrphanCount == 0 {
			return fmt.Sprintf("Repaired %d orphan journal(s).", status.Repaired)
		}
		return fmt.Sprintf("Repaired %d; %d orphan journal(s) remain.", status.Repaired, status.OrphanCount)
	}
	if status.OrphanCount > 0 {
		return fmt.Sprintf(
			"Warning: profile-import-auth-journal contains %d orphan journal(s); run `godex doctor --repair-import-auth-journals`.",
			status.OrphanCount,
		)
	}
	return "None"
}

func doctorCheckFields(checks []runtimemodel.DoctorCheck) [][2]string {
	fields := make([][2]string, 0, len(checks))
	for _, check := range checks {
		fields = append(fields, [2]string{check.Name, check.Status})
	}
	return fields
}

func doctorRuntimePanel(runtime runtimemodel.DoctorRuntime) doctorPanel {
	overview := runtime.Overview
	return doctorPanel{title: "Runtime", fields: [][2]string{
		{"Active profile", valueOrDash(overview.ActiveProfile)},
		{"Profiles", fmt.Sprint(overview.ProfileCount)},
		{"Inflight", fmt.Sprint(overview.Inflight)},
		{"Recent events", fmt.Sprint(overview.RecentEvents)},
		{"Tail events", fmt.Sprint(len(runtime.Events))},
		{"Tail bytes", fmt.Sprint(runtime.TailBytes)},
	}}
}

func yesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func writeDoctorPanels(out io.Writer, panels []doctorPanel) error {
	for index, panel := range panels {
		if index > 0 {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out, panel.title); err != nil {
			return err
		}
		for _, field := range panel.fields {
			if _, err := fmt.Fprintf(out, "%s: %s\n", field[0], field[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeDoctorJSON(out io.Writer, report runtimemodel.DoctorDiagnostics) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

type doctorBundleDocument struct {
	Bundle struct {
		Kind        string `json:"kind"`
		Redacted    bool   `json:"redacted"`
		GeneratedAt string `json:"generated_at"`
	} `json:"bundle"`
	Godex struct {
		Version      string `json:"version"`
		CodexVersion string `json:"codex_version"`
	} `json:"godex"`
	Paths struct {
		GodexRoot string `json:"godex_root"`
	} `json:"paths"`
	Config struct {
		ImportAuthJournals *runtimemodel.DoctorImportAuthJournals `json:"import_auth_journals,omitempty"`
	} `json:"config"`
	State struct {
		AccountCount  int    `json:"account_count"`
		EnabledCount  int    `json:"enabled_count"`
		ProfileCount  int    `json:"profile_count"`
		ActiveProfile string `json:"active_profile,omitempty"`
	} `json:"state"`
	Runtime *runtimemodel.DoctorRuntime `json:"runtime,omitempty"`
	Quota   []runtimemodel.DoctorQuota  `json:"quota_probes,omitempty"`
	Install []runtimemodel.DoctorCheck  `json:"install_checks,omitempty"`
}

func writeDoctorBundle(doctor doctorRunner, out io.Writer, report runtimemodel.DoctorDiagnostics, path string) error {
	document := doctorBundle(report)
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return errors.New("serialize redacted doctor bundle")
	}
	if path == "-" {
		_, err = fmt.Fprintln(out, string(content))
		return err
	}
	written, err := doctor.SaveBundle(path, content)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Doctor bundle: %s\n", written)
	return err
}

func doctorBundle(report runtimemodel.DoctorDiagnostics) doctorBundleDocument {
	var document doctorBundleDocument
	document.Bundle.Kind = "godex_doctor"
	document.Bundle.Redacted = true
	document.Bundle.GeneratedAt = report.GeneratedAt
	document.Godex.Version = version.String()
	document.Godex.CodexVersion = report.CodexVersion
	document.Paths.GodexRoot = report.GodexHome
	document.State.AccountCount = report.AccountCount
	document.State.EnabledCount = report.EnabledCount
	document.Config.ImportAuthJournals = report.ImportAuthJournals
	document.Runtime = report.Runtime
	document.Quota = report.Quota
	document.Install = report.Install
	if report.Runtime != nil {
		document.State.ProfileCount = report.Runtime.Overview.ProfileCount
		document.State.ActiveProfile = report.Runtime.Overview.ActiveProfile
	}
	return document
}

const doctorUsage = "usage: godex doctor [--quota] [--runtime] [--install] [--repair-import-auth-journals] [--repair-session-index] [--tail-bytes BYTES] [--json] [--bundle [PATH] --redacted]"

var _ doctorRunner = (*runtimeusecase.Doctor)(nil)
