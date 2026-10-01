package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const configMaxBytes = 2 << 20
const copilotConfigFileName = "config.json"

type configFile struct {
	LastLoggedInUser *configUser       `json:"lastLoggedInUser"`
	LoggedInUsers    []configUser      `json:"loggedInUsers"`
	CopilotTokens    map[string]string `json:"copilotTokens"`
}

type configUser struct {
	Host  string `json:"host"`
	Login string `json:"login"`
}

func (source *Source) readConfig() (configFile, error) {
	root, err := source.configRoot()
	if err != nil {
		return configFile{}, err
	}
	path := filepath.Join(root, copilotConfigFileName)
	info, err := os.Lstat(path)
	if err != nil {
		return configFile{}, fmt.Errorf("failed to read Copilot config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > configMaxBytes {
		return configFile{}, errors.New("Copilot config must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return configFile{}, errors.New("failed to read Copilot config")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, configMaxBytes+1))
	if err != nil || len(content) > configMaxBytes {
		return configFile{}, errors.New("failed to read Copilot config")
	}
	return parseConfig(content)
}

func (source *Source) configRoot() (string, error) {
	if configured := strings.TrimSpace(source.getenv("COPILOT_HOME")); configured != "" {
		absolute, err := filepath.Abs(configured)
		if err != nil {
			return "", err
		}
		return filepath.Clean(absolute), nil
	}
	home, err := source.homeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("failed to determine Copilot config directory")
	}
	return filepath.Join(home, ".copilot"), nil
}

func parseConfig(content []byte) (configFile, error) {
	var config configFile
	if len(strings.TrimSpace(string(content))) == 0 {
		return config, nil
	}
	if err := json.Unmarshal(content, &config); err == nil {
		return config, nil
	}
	withoutComments := stripLineComments(string(content))
	if withoutComments == string(content) || strings.TrimSpace(withoutComments) == "" {
		return configFile{}, errors.New("failed to parse Copilot config")
	}
	if err := json.Unmarshal([]byte(withoutComments), &config); err != nil {
		return configFile{}, errors.New("failed to parse Copilot config")
	}
	return config, nil
}

func stripLineComments(raw string) string {
	var output strings.Builder
	output.Grow(len(raw))
	inString, escaped := false, false
	for index := 0; index < len(raw); index++ {
		current := raw[index]
		if inString {
			appendJSONStringByte(&output, current, &escaped, &inString)
			continue
		}
		if current == '"' {
			inString = true
			output.WriteByte(current)
			continue
		}
		if startsLineComment(raw, index) {
			index = skipLineComment(raw, index+2, &output)
			continue
		}
		output.WriteByte(current)
	}
	return output.String()
}

func appendJSONStringByte(output *strings.Builder, current byte, escaped, inString *bool) {
	output.WriteByte(current)
	switch {
	case *escaped:
		*escaped = false
	case current == '\\':
		*escaped = true
	case current == '"':
		*inString = false
	}
}

func startsLineComment(raw string, index int) bool {
	return raw[index] == '/' && index+1 < len(raw) && raw[index+1] == '/'
}

func skipLineComment(raw string, index int, output *strings.Builder) int {
	for index < len(raw) {
		if raw[index] == '\n' {
			output.WriteByte('\n')
			return index
		}
		index++
	}
	return len(raw)
}

func candidateUsers(config configFile) []configUser {
	result := make([]configUser, 0, len(config.LoggedInUsers)+1)
	if config.LastLoggedInUser != nil {
		result = append(result, *config.LastLoggedInUser)
	} else if len(config.LoggedInUsers) > 0 {
		result = append(result, config.LoggedInUsers[0])
	}
	for _, user := range config.LoggedInUsers {
		duplicate := false
		for _, existing := range result {
			if existing.Host == user.Host && existing.Login == user.Login {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, user)
		}
	}
	return result
}

func accountKey(host, login string) string {
	return strings.TrimSpace(host) + ":" + strings.TrimSpace(login)
}

func configToken(config configFile, host, login string) string {
	return strings.TrimSpace(config.CopilotTokens[accountKey(host, login)])
}
