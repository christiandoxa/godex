package presidio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	presidioAnalyzerContainer   = "godex-presidio-analyzer"
	presidioAnonymizerContainer = "godex-presidio-anonymizer"
	presidioAnalyzerImage       = "ghcr.io/data-privacy-stack/presidio-analyzer:2.2.364@sha256:ae8f6f111ac2f04e3fec552f7f80edd0dcbfa2dd69ee1b9e030475be31669885"
	presidioAnonymizerImage     = "ghcr.io/data-privacy-stack/presidio-anonymizer:2.2.364@sha256:e567013893ebc80994e3799f6f55c86aa1f0b0fadb779571ab346f0ec45365c1"
	presidioMinimumVersion      = "2.2.364"
	presidioManagedLabel        = "com.godex.presidio.managed"
	presidioServiceLabel        = "com.godex.presidio.service"
)

type containerChange int

const (
	containerUnchanged containerChange = iota
	containerStarted
	containerCreated
)

func ResolveForLaunch(ctx context.Context, root string, required bool) (*proxymodel.PresidioConfig, error) {
	config, _, err := LoadConfig(root)
	if err != nil {
		return nil, err
	}
	if required && !config.FailClosed {
		return nil, fmt.Errorf("--require-tool presidio requires fail_mode = \"closed\" in %s", ConfigFileName)
	}
	redactor, err := NewRedactor(config)
	if err != nil {
		return nil, err
	}
	analyzer := redactor.Probe(ctx, config.AnalyzerURL)
	anonymizer := redactor.Probe(ctx, config.AnonymizerURL)
	if analyzer.OK && anonymizer.OK {
		return &config, nil
	}
	if startupDisabled() ||
		config.AnalyzerURL != DefaultAnalyzerURL ||
		config.AnonymizerURL != DefaultAnonymizerURL {
		if required {
			return nil, requiredServicesError(analyzer, anonymizer, startupBlockReason(config))
		}
		return &config, nil
	}
	if !dockerAvailable(ctx) {
		if required {
			return nil, requiredServicesError(analyzer, anonymizer, "Docker is unavailable")
		}
		return &config, nil
	}

	analyzerImage, err := resolveImage("GODEX_PRESIDIO_ANALYZER_IMAGE", "PRODEX_PRESIDIO_ANALYZER_IMAGE", "analyzer", presidioAnalyzerImage)
	if err != nil {
		if required {
			return nil, err
		}
		return &config, nil
	}
	anonymizerImage, err := resolveImage("GODEX_PRESIDIO_ANONYMIZER_IMAGE", "PRODEX_PRESIDIO_ANONYMIZER_IMAGE", "anonymizer", presidioAnonymizerImage)
	if err != nil {
		if required {
			return nil, err
		}
		return &config, nil
	}
	var changes []struct {
		name   string
		change containerChange
	}
	change, err := ensureContainer(ctx, presidioAnalyzerContainer, analyzerImage, "5002", "analyzer")
	if err == nil && change != containerUnchanged {
		changes = append(changes, struct {
			name   string
			change containerChange
		}{presidioAnalyzerContainer, change})
	}
	if err == nil {
		change, err = ensureContainer(ctx, presidioAnonymizerContainer, anonymizerImage, "5001", "anonymizer")
		if err == nil && change != containerUnchanged {
			changes = append(changes, struct {
				name   string
				change containerChange
			}{presidioAnonymizerContainer, change})
		}
	}
	if err != nil {
		_ = rollbackContainers(ctx, changes)
		if required {
			return nil, err
		}
		return &config, nil
	}

	if waitForServices(ctx, redactor, config) {
		return &config, nil
	}
	cleanupErr := rollbackContainers(context.WithoutCancel(ctx), changes)
	if required {
		reason := "services did not become healthy before launch"
		if cleanupErr != nil {
			reason += "; cleanup failed: " + cleanupErr.Error()
		}
		return nil, requiredServicesError(redactor.Probe(ctx, config.AnalyzerURL), redactor.Probe(ctx, config.AnonymizerURL), reason)
	}
	return &config, nil
}

func startupBlockReason(config proxymodel.PresidioConfig) string {
	if startupDisabled() {
		return "automatic startup is disabled"
	}
	if config.AnalyzerURL != DefaultAnalyzerURL || config.AnonymizerURL != DefaultAnonymizerURL {
		return "custom endpoints are not started automatically"
	}
	return "services are unavailable"
}

func requiredServicesError(analyzer, anonymizer Health, reason string) error {
	return fmt.Errorf(
		"required Presidio services are not ready: Analyzer %s; Anonymizer %s; %s",
		healthLabel(analyzer), healthLabel(anonymizer), reason,
	)
}

func healthLabel(health Health) string {
	if health.OK {
		return "ok (" + health.Message + ")"
	}
	return "failed (" + health.Message + ")"
}

func startupDisabled() bool {
	for _, key := range []string{"GODEX_PRESIDIO_AUTO_START", "PRODEX_PRESIDIO_AUTO_START"} {
		if value, ok := os.LookupEnv(key); ok {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "0", "false", "no", "off":
				return true
			case "1", "true", "yes", "on":
				return false
			}
		}
	}
	return false
}

func dockerAvailable(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(probeCtx, "docker", "version")
	return command.Run() == nil
}

func resolveImage(canonicalEnv, legacyEnv, service, fallback string) (string, error) {
	image := strings.TrimSpace(os.Getenv(canonicalEnv))
	if image == "" {
		image = strings.TrimSpace(os.Getenv(legacyEnv))
	}
	if image == "" {
		image = fallback
	}
	return validatePresidioImage(service, image)
}

func validatePresidioImage(service, image string) (string, error) {
	prefix := "ghcr.io/data-privacy-stack/presidio-" + service + ":"
	tagged, ok := strings.CutPrefix(image, prefix)
	if !ok {
		return "", fmt.Errorf("Presidio %s image must use the official %s<version> image", service, prefix)
	}
	version := tagged
	if before, _, found := strings.Cut(tagged, "@"); found {
		version = before
	}
	if compareVersion(version, presidioMinimumVersion) < 0 || strings.Contains(version, "-") {
		return "", fmt.Errorf("Presidio %s %s is incompatible; Godex requires %s or newer", service, version, presidioMinimumVersion)
	}
	if _, digest, found := strings.Cut(tagged, "@"); found {
		hex, ok := strings.CutPrefix(digest, "sha256:")
		if !ok || len(hex) != 64 {
			return "", fmt.Errorf("Presidio %s image digest must use a 64-character SHA-256", service)
		}
		for _, current := range hex {
			if !strings.ContainsRune("0123456789abcdefABCDEF", current) {
				return "", fmt.Errorf("Presidio %s image digest must use a 64-character SHA-256", service)
			}
		}
	}
	return image, nil
}

func compareVersion(left, right string) int {
	parse := func(value string) [3]int {
		var result [3]int
		core := strings.SplitN(strings.TrimPrefix(value, "v"), "-", 2)[0]
		for index, part := range strings.Split(core, ".") {
			if index == len(result) {
				break
			}
			result[index], _ = strconv.Atoi(part)
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := 0; index < 3; index++ {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

func ensureContainer(ctx context.Context, name, image, hostPort, service string) (containerChange, error) {
	container, exists, err := inspectContainer(ctx, name)
	if err != nil {
		return containerUnchanged, err
	}
	if exists {
		if err := validateContainer(container, name, hostPort, service); err != nil {
			return containerUnchanged, err
		}
		running, _ := containerPathBool(container, "State", "Running")
		if running {
			return containerUnchanged, nil
		}
		if err := runDocker(ctx, "start", name); err != nil {
			return containerUnchanged, err
		}
		return containerStarted, nil
	}
	port := "127.0.0.1:" + hostPort + ":3000"
	if err := runDocker(ctx,
		"run", "-d", "--name", name,
		"--label", presidioManagedLabel+"=true",
		"--label", presidioServiceLabel+"="+service,
		"-p", port, image,
	); err != nil {
		return containerUnchanged, err
	}
	return containerCreated, nil
}

func inspectContainer(ctx context.Context, name string) (map[string]any, bool, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "docker", "container", "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		text := string(output)
		if strings.Contains(text, "No such object") || strings.Contains(text, "No such container") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("docker inspect %s failed", name)
	}
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		return nil, false, fmt.Errorf("parse Docker inspection for %s: %w", name, err)
	}
	return value, true, nil
}

func validateContainer(value map[string]any, name, hostPort, service string) error {
	labels, _ := nestedMap(value, "Config", "Labels")
	if labels[presidioManagedLabel] != "true" || labels[presidioServiceLabel] != service {
		return fmt.Errorf("refusing existing Presidio container %s: Godex ownership labels do not match", name)
	}
	config, _ := nestedMap(value, "Config")
	image, _ := config["Image"].(string)
	if _, err := resolveExplicitImage(service, image); err != nil {
		return fmt.Errorf("refusing existing Presidio container %s: %w", name, err)
	}
	hostConfig, _ := nestedMap(value, "HostConfig", "PortBindings")
	bindings, _ := hostConfig["3000/tcp"].([]any)
	if len(bindings) != 1 {
		return fmt.Errorf("refusing existing Presidio container %s: port configuration does not match", name)
	}
	binding, _ := bindings[0].(map[string]any)
	if binding["HostIp"] != "127.0.0.1" || binding["HostPort"] != hostPort {
		return fmt.Errorf("refusing existing Presidio container %s: port configuration does not match", name)
	}
	return nil
}

func resolveExplicitImage(service, image string) (string, error) {
	return validatePresidioImage(service, image)
}

func containerPathBool(value map[string]any, path ...string) (bool, bool) {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return false, false
		}
		current, ok = object[key]
		if !ok {
			return false, false
		}
	}
	result, ok := current.(bool)
	return result, ok
}

func nestedMap(value map[string]any, path ...string) (map[string]any, bool) {
	var current any = value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	result, ok := current.(map[string]any)
	return result, ok
}

func runDocker(ctx context.Context, args ...string) error {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s failed: %s", strings.Join(args, " "), redactDockerOutput(string(output)))
	}
	return nil
}

func redactDockerOutput(value string) string {
	if len(value) > 4096 {
		value = value[:4096]
	}
	return strings.TrimSpace(value)
}

func rollbackContainers(ctx context.Context, changes []struct {
	name   string
	change containerChange
}) error {
	var result error
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		var err error
		switch change.change {
		case containerStarted:
			err = runDocker(ctx, "stop", change.name)
		case containerCreated:
			err = runDocker(ctx, "rm", "--force", change.name)
		}
		result = errors.Join(result, err)
	}
	return result
}

func waitForServices(ctx context.Context, redactor *Redactor, config proxymodel.PresidioConfig) bool {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		analyzer := redactor.Probe(ctx, config.AnalyzerURL)
		anonymizer := redactor.Probe(ctx, config.AnonymizerURL)
		if analyzer.OK && anonymizer.OK {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}
