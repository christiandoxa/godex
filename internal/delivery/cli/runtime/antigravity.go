package runtime

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	"golang.org/x/term"
)

// NormalizeNativeAntigravityArguments accepts Prodex's `s`/`super` native-CLI
// spelling while keeping provider parsing in CLI delivery.
func NormalizeNativeAntigravityArguments(arguments []string) []string {
	if len(arguments) == 0 || (arguments[0] != "s" && arguments[0] != "super") {
		return arguments
	}
	arguments = arguments[1:]
	selection := runtimemodel.Selection{}
	features := runtimeFeatures{}
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			return arguments
		}
		next, handled, err := consumeWrapperArgument(arguments, index, &selection, &features)
		if err != nil {
			return arguments
		}
		if !handled {
			if arguments[index] != "gemini" || selection.Provider != "" {
				return arguments
			}
			normalized := append([]string(nil), arguments[:index]...)
			normalized = append(normalized, "--provider", "gemini")
			return append(normalized, arguments[index+1:]...)
		}
		index = next
	}
	return arguments
}

func parseNativeAntigravityArguments(arguments []string) (runtimemodel.Selection, []string, error) {
	selection := runtimemodel.Selection{}
	features := runtimeFeatures{}
	remaining := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			remaining = append(remaining, arguments[index+1:]...)
			break
		}
		next, handled, err := consumeWrapperArgument(arguments, index, &selection, &features)
		if err != nil {
			return runtimemodel.Selection{}, nil, err
		}
		if !handled {
			remaining = append(remaining, arguments[index])
			index++
			continue
		}
		index = next
	}
	return finishRunArguments(selection, features, remaining)
}

func normalizeRuntimeCLI(value string) (string, error) {
	if value == "agy" {
		return "agy", nil
	}
	return "", fmt.Errorf("invalid --cli: supported values are agy, got %q", value)
}

func UsesNativeAntigravity(arguments []string) bool {
	arguments = NormalizeNativeAntigravityArguments(arguments)
	selection := runtimemodel.Selection{}
	features := runtimeFeatures{}
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			return false
		}
		next, handled, err := consumeWrapperArgument(arguments, index, &selection, &features)
		if err != nil {
			return selection.CLI == "agy"
		}
		if !handled {
			index++
			continue
		}
		if selection.CLI == "agy" {
			return true
		}
		index = next
	}
	return false
}

func printAntigravityDryRun(out io.Writer) error {
	if out == nil {
		return errors.New("native Antigravity dry-run output is not configured")
	}
	if antigravityDryRunPanelAllowed(antigravityStdoutIsTTY(out)) {
		if err := printAntigravityDryRunPanel(out); err == nil {
			return nil
		}
	}
	_, err := io.WriteString(out, "Prodex dry run: launch diagnostics\nFlow: native-cli\nProvider: antigravity\nProfile: (native CLI owned)\nRuntime proxy: disabled\n")
	return err
}

func antigravityStdoutIsTTY(out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func antigravityDryRunPanelAllowed(stdoutTTY bool) bool {
	_, codexCI := os.LookupEnv("CODEX_CI")
	return stdoutTTY && !codexCI
}

type antigravityDryRunPanelModel struct{ width int }

func (model antigravityDryRunPanelModel) Init() tea.Cmd { return tea.Quit }

func (model antigravityDryRunPanelModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return model, nil
}

func (model antigravityDryRunPanelModel) View() string {
	width := max(model.width, 40)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	innerWidth := width - 2
	lines := []string{
		border.Render("┌" + strings.Repeat("─", innerWidth) + "┐"),
		border.Render("│") + " " + title.Render("Prodex Dry Run") + strings.Repeat(" ", innerWidth-2-lipgloss.Width("Prodex Dry Run")) + " " + border.Render("│"),
		border.Render("├" + strings.Repeat("─", innerWidth) + "┤"),
		antigravityPanelRow(border, width, "Flow", "native-cli"),
		antigravityPanelRow(border, width, "Provider", "antigravity"),
		antigravityPanelRow(border, width, "Profile", "(native CLI owned)"),
		antigravityPanelRow(border, width, "Runtime proxy", "disabled"),
		border.Render("└" + strings.Repeat("─", innerWidth) + "┘"),
	}
	return strings.Join(lines, "\n")
}

func antigravityPanelRow(border lipgloss.Style, width int, label, value string) string {
	label = label + ":"
	content := label + strings.Repeat(" ", 15-lipgloss.Width(label)) + value
	padding := max(0, width-4-lipgloss.Width(content))
	return border.Render("│") + " " + content + strings.Repeat(" ", padding) + " " + border.Render("│")
}

func printAntigravityDryRunPanel(out io.Writer) error {
	_, err := tea.NewProgram(
		antigravityDryRunPanelModel{width: antigravityDryRunPanelWidth(out)},
		tea.WithInput(nil),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	).Run()
	return err
}

func antigravityDryRunPanelWidth(out io.Writer) int {
	if columns, err := strconv.Atoi(strings.TrimSpace(os.Getenv("PRODEX_TERM_COLUMNS"))); err == nil && columns > 0 {
		return columns
	}
	if file, ok := out.(*os.File); ok {
		if _, columns, err := term.GetSize(int(file.Fd())); err == nil && columns > 0 {
			return columns
		}
	}
	return 110
}

func runAntigravityDryRun(runner *runtimeusecase.Runner, out io.Writer) error {
	if err := runner.PrepareAntigravityCodexHome(); err != nil {
		return err
	}
	return printAntigravityDryRun(out)
}
