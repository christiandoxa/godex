package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
	"golang.org/x/term"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type apiKeyPromptStage uint8

const (
	apiKeyPromptSecret apiKeyPromptStage = iota
	apiKeyPromptBaseURL
	apiKeyPromptProfileName
	apiKeyPromptDone
)

type apiKeyPromptModel struct {
	stage            apiKeyPromptStage
	input            string
	apiKey           string
	baseURL          string
	baseURLSpecified bool
	profileName      string
	fixedBaseURL     bool
	fixedProfileName bool
	cancelled        bool
}

func newAPIKeyPromptModel(options LoginOptions) apiKeyPromptModel {
	return apiKeyPromptModel{
		stage:   apiKeyPromptSecret,
		baseURL: options.BaseURL, baseURLSpecified: options.BaseURLSpecified,
		profileName:      options.Name,
		fixedBaseURL:     options.BaseURLSpecified,
		fixedProfileName: strings.TrimSpace(options.Name) != "",
	}
}

func (model apiKeyPromptModel) Init() tea.Cmd { return nil }

func (model apiKeyPromptModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch key.String() {
	case "esc", "ctrl+c", "ctrl+z":
		model.cancelled = true
		return model, tea.Quit
	case "backspace":
		if len(model.input) > 0 {
			_, size := lastRune(model.input)
			model.input = model.input[:len(model.input)-size]
		}
		return model, nil
	case "enter":
		return model.submit()
	}
	if key.Type == tea.KeyRunes {
		model.input += string(key.Runes)
	}
	return model, nil
}

func (model apiKeyPromptModel) submit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(model.input)
	switch model.stage {
	case apiKeyPromptSecret:
		if value == "" {
			return model, nil
		}
		model.apiKey = value
		model.input = ""
		if model.fixedBaseURL {
			return model.advancePastBaseURL()
		}
		model.stage = apiKeyPromptBaseURL
	case apiKeyPromptBaseURL:
		model.baseURL = value
		model.baseURLSpecified = true
		model.input = ""
		return model.advancePastBaseURL()
	case apiKeyPromptProfileName:
		if value == "" {
			value = profileusecase.DefaultAPIKeyProfileName(model.baseURL)
		}
		model.profileName = value
		model.stage = apiKeyPromptDone
		return model, tea.Quit
	}
	return model, nil
}

func (model apiKeyPromptModel) advancePastBaseURL() (tea.Model, tea.Cmd) {
	if model.fixedProfileName {
		model.stage = apiKeyPromptDone
		return model, tea.Quit
	}
	model.stage = apiKeyPromptProfileName
	return model, nil
}

func (model apiKeyPromptModel) View() string {
	label, detail, defaultValue, secret := model.promptFields()
	display := model.input
	if secret {
		display = strings.Repeat("*", len([]rune(model.input)))
	} else if display == "" {
		display = defaultValue
	}
	return fmt.Sprintf(
		"Godex Login\n\n%s\n\n%s\n\n> %s_\n\nenter accept • backspace delete • esc cancel",
		label, detail, display,
	)
}

func (model apiKeyPromptModel) promptFields() (label, detail, defaultValue string, secret bool) {
	switch model.stage {
	case apiKeyPromptSecret:
		return "OpenAI/OpenAI-compatible API key", "Paste the key for OpenAI or an OpenAI-compatible provider.", "", true
	case apiKeyPromptBaseURL:
		return "OpenAI-compatible base URL", "Use the default OpenAI endpoint or enter a local/provider endpoint such as http://localhost:11434/v1.", defaultOpenAIBaseURL, false
	case apiKeyPromptProfileName:
		return "Profile name", "Leave empty to use the suggested managed profile name.", profileusecase.DefaultAPIKeyProfileName(model.baseURL), false
	default:
		return "Login complete", "", "", false
	}
}

func PromptAPIKeyLogin(ctx context.Context, in io.Reader, out io.Writer, options LoginOptions) (profilemodel.APIKeyLoginInput, error) {
	if LoginMenuInteractive(in, out) {
		return promptAPIKeyLoginTUI(ctx, in, out, options)
	}
	return promptAPIKeyLoginPlain(in, out, options)
}

func promptAPIKeyLoginTUI(ctx context.Context, in io.Reader, out io.Writer, options LoginOptions) (profilemodel.APIKeyLoginInput, error) {
	program := tea.NewProgram(newAPIKeyPromptModel(options), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen(), tea.WithContext(ctx))
	result, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return profilemodel.APIKeyLoginInput{}, ctx.Err()
	}
	if err != nil {
		return profilemodel.APIKeyLoginInput{}, fmt.Errorf("API-key login TUI failed: %w", err)
	}
	model, ok := result.(apiKeyPromptModel)
	if !ok || model.cancelled || model.stage != apiKeyPromptDone {
		return profilemodel.APIKeyLoginInput{}, errors.New("login input cancelled")
	}
	return model.apiKeyInput(), nil
}

func promptAPIKeyLoginPlain(in io.Reader, out io.Writer, options LoginOptions) (profilemodel.APIKeyLoginInput, error) {
	reader := bufio.NewReader(in)
	apiKey, err := readSecretPrompt(reader, in, out, "OpenAI/OpenAI-compatible API key: ")
	if err != nil {
		return profilemodel.APIKeyLoginInput{}, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return profilemodel.APIKeyLoginInput{}, errors.New("API key cannot be empty")
	}
	baseURL, specified := options.BaseURL, options.BaseURLSpecified
	if !specified {
		baseURL, err = readPromptLine(reader, out, "OpenAI-compatible base URL [default https://api.openai.com/v1]: ")
		if err != nil {
			return profilemodel.APIKeyLoginInput{}, err
		}
		specified = true
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		defaultName := profileusecase.DefaultAPIKeyProfileName(strings.TrimSpace(baseURL))
		name, err = readPromptLine(reader, out, "Profile name ["+defaultName+"]: ")
		if err != nil {
			return profilemodel.APIKeyLoginInput{}, err
		}
		if strings.TrimSpace(name) == "" {
			name = defaultName
		}
	}
	return profilemodel.APIKeyLoginInput{Name: name, APIKey: strings.TrimSpace(apiKey), BaseURL: strings.TrimSpace(baseURL), BaseURLSpecified: specified}, nil
}

func readSecretPrompt(reader *bufio.Reader, in io.Reader, out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	if file, ok := in.(*os.File); ok && loginMenuTerminal(file) {
		content, err := term.ReadPassword(int(file.Fd()))
		_, _ = fmt.Fprintln(out)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func readPromptLine(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (model apiKeyPromptModel) apiKeyInput() profilemodel.APIKeyLoginInput {
	return profilemodel.APIKeyLoginInput{
		Name: model.profileName, APIKey: model.apiKey,
		BaseURL: model.baseURL, BaseURLSpecified: model.baseURLSpecified,
	}
}

func lastRune(value string) (rune, int) {
	var last rune
	var size int
	for _, current := range value {
		last = current
		size = len(string(current))
	}
	return last, size
}
