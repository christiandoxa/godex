package runtime

import (
	"fmt"
	"io"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const noDoctorPolicySuggestion = "No policy.toml suggestion matched the sampled runtime markers."

func doctorPolicySuggestions(runtime *runtimemodel.DoctorRuntime) []runtimemodel.DoctorPolicySuggestion {
	if runtime == nil || runtime.PolicySuggestions == nil {
		return nil
	}
	return *runtime.PolicySuggestions
}

func writeDoctorPolicySuggestions(out io.Writer, runtime *runtimemodel.DoctorRuntime) error {
	if _, err := fmt.Fprintln(out, "Runtime Policy Suggestions"); err != nil {
		return err
	}
	suggestions := doctorPolicySuggestions(runtime)
	if len(suggestions) == 0 {
		_, err := fmt.Fprintln(out, noDoctorPolicySuggestion)
		return err
	}
	for _, suggestion := range suggestions {
		if _, err := fmt.Fprintf(out, "- %s: %s\n", suggestion.Title, suggestion.Reason); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "  policy.toml:"); err != nil {
			return err
		}
		for _, line := range strings.Split(suggestion.Snippet, "\n") {
			if _, err := fmt.Fprintf(out, "  %s\n", line); err != nil {
				return err
			}
		}
	}
	return nil
}

func doctorPolicySuggestionPanel(runtime *runtimemodel.DoctorRuntime) doctorPanel {
	suggestions := doctorPolicySuggestions(runtime)
	if len(suggestions) == 0 {
		return doctorPanel{title: "Policy Suggestions", fields: [][2]string{{"Status", noDoctorPolicySuggestion}}}
	}
	fields := make([][2]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		fields = append(fields, [2]string{suggestion.Title, suggestion.Reason + "\n" + suggestion.Snippet})
	}
	return doctorPanel{title: "Policy Suggestions", fields: fields}
}
