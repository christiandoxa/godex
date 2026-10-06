//go:build windows

package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type superRTKWrapper struct {
	command     string
	subcommands []string
}

func configureSuperRTKWrappers(home, rtk string) error {
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"rtk", "godex-rtk"} {
		if err := writeSuperExecutable(filepath.Join(binDir, name+".cmd"), superCmdWrapper(rtk)); err != nil {
			return err
		}
	}
	for _, wrapper := range superRTKWrappers() {
		command, err := exec.LookPath(wrapper.command)
		if err != nil {
			continue
		}
		if err := writeSuperExecutable(
			filepath.Join(binDir, wrapper.command+".cmd"),
			superRTKCommandWrapper(rtk, command, wrapper.subcommands),
		); err != nil {
			return err
		}
	}
	return nil
}

func superRTKWrappers() []superRTKWrapper {
	return []superRTKWrapper{
		{"git", []string{"diff", "show", "log", "status", "grep", "blame"}},
		{"cargo", []string{"test", "build", "check", "clippy", "bench", "run"}},
		{"npm", []string{"test", "run", "build", "install", "ci", "update", "audit"}},
		{"yarn", []string{"test", "run", "build", "install", "add", "upgrade"}},
		{"pnpm", []string{"test", "run", "build", "install", "add", "update"}},
		{"bun", []string{"test", "run", "build", "install", "add"}},
		{"pytest", nil},
		{"go", []string{"test", "build", "vet"}},
		{"docker", []string{"build", "compose", "logs", "pull", "push", "run"}},
		{"kubectl", []string{"logs", "describe", "get", "events", "top"}},
		{"rg", nil}, {"find", nil}, {"ls", nil}, {"tree", nil},
	}
}

func superRTKCommandWrapper(rtk, command string, subcommands []string) string {
	quotedRTK := superCmdQuote(rtk)
	quotedCommand := superCmdQuote(command)
	var routing string
	if len(subcommands) == 0 {
		routing = "set \"PRODEX_RTK_AUTO_WRAP_DEPTH=1\"\r\n" + superRTKStateExports() +
			quotedRTK + " " + quotedCommand + " %*\r\nexit /b %ERRORLEVEL%\r\n"
	} else {
		var checks strings.Builder
		for _, subcommand := range subcommands {
			fmt.Fprintf(&checks, "if /I \"%%~1\"==\"%s\" goto godex_rtk_wrap\r\n", superCmdEscape(subcommand))
		}
		routing = ":godex_rtk_scan\r\nif \"%~1\"==\"\" goto godex_rtk_raw\r\n" +
			checks.String() + "shift\r\ngoto godex_rtk_scan\r\n" +
			":godex_rtk_wrap\r\nset \"PRODEX_RTK_AUTO_WRAP_DEPTH=1\"\r\n" +
			superRTKStateExports() + quotedRTK + " " + quotedCommand +
			" %*\r\nexit /b %ERRORLEVEL%\r\n"
	}
	return "@echo off\r\n" +
		"if \"%PRODEX_RTK_DISABLE_AUTO_WRAP%\"==\"1\" goto godex_rtk_raw\r\n" +
		"if defined PRODEX_RTK_AUTO_WRAP_DEPTH goto godex_rtk_raw\r\n" +
		routing + ":godex_rtk_raw\r\n" + quotedCommand +
		" %*\r\nexit /b %ERRORLEVEL%\r\n"
}

func superCmdWrapper(command string) string {
	return "@echo off\r\n" + superRTKStateExports() + superCmdQuote(command) +
		" %*\r\nexit /b %ERRORLEVEL%\r\n"
}

func superRTKStateExports() string {
	root := superRTKStateRoot()
	if root == "" {
		return ""
	}
	var builder strings.Builder
	for _, pair := range [][2]string{
		{"RTK_DB_PATH", filepath.Join(root, "history.db")},
		{"RTK_TEE_DIR", filepath.Join(root, "tee")},
		{"RTK_AUDIT_DIR", filepath.Join(root, "audit")},
	} {
		fmt.Fprintf(&builder, "set \"%s=%s\"\r\n", pair[0], superCmdEscape(pair[1]))
	}
	return builder.String()
}

func superRTKStateRoot() string {
	root := strings.TrimSpace(os.Getenv("GODEX_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		root = filepath.Join(home, ".godex")
	}
	if !filepath.IsAbs(root) {
		if absolute, err := filepath.Abs(root); err == nil {
			root = absolute
		}
	}
	return filepath.Join(root, "optimizer-state", "rtk")
}

func superCmdEscape(value string) string { return strings.ReplaceAll(value, "%", "%%") }

func superCmdQuote(value string) string { return "\"" + superCmdEscape(value) + "\"" }

func writeSuperExecutable(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
