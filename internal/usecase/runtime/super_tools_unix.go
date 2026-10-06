//go:build !windows

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
		if err := writeSuperExecutable(filepath.Join(binDir, name), superShellWrapper(rtk, nil)); err != nil {
			return err
		}
	}
	for _, wrapper := range superRTKWrappers() {
		command, err := exec.LookPath(wrapper.command)
		if err != nil {
			continue
		}
		if filepath.Clean(command) == filepath.Clean(filepath.Join(binDir, wrapper.command)) {
			continue
		}
		if err := writeSuperExecutable(
			filepath.Join(binDir, wrapper.command),
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
	quotedRTK := superShellQuote(rtk)
	quotedCommand := superShellQuote(command)
	state := superRTKStateExports()
	var routing string
	if len(subcommands) == 0 {
		routing = "export PRODEX_RTK_AUTO_WRAP_DEPTH=1\n" + state +
			"exec " + quotedRTK + " " + quotedCommand + " \"$@\"\n"
	} else {
		pattern := strings.Join(subcommands, "|")
		routing = "for prodex_rtk_arg in \"$@\"; do\n" +
			"  case \"$prodex_rtk_arg\" in\n" +
			"    " + pattern + ")\n" +
			"      export PRODEX_RTK_AUTO_WRAP_DEPTH=1\n" +
			"      " + state +
			"exec " + quotedRTK + " " + quotedCommand + " \"$@\"\n" +
			"      ;;\n" +
			"  esac\n" +
			"done\n" +
			"exec " + quotedCommand + " \"$@\"\n"
	}
	disabled := "$" + "{PRODEX_RTK_DISABLE_AUTO_WRAP:-}"
	depth := "$" + "{PRODEX_RTK_AUTO_WRAP_DEPTH:-}"
	return "#!/usr/bin/env sh\n" +
		"if [ \"" + disabled + "\" = \"1\" ] || [ -n \"" + depth + "\" ]; then\n" +
		"  exec " + quotedCommand + " \"$@\"\n" +
		"fi\n" + routing
}

func superShellWrapper(command string, args []string) string {
	var rendered strings.Builder
	rendered.WriteString("#!/usr/bin/env sh\n")
	rendered.WriteString(superRTKStateExports())
	rendered.WriteString("exec ")
	rendered.WriteString(superShellQuote(command))
	for _, arg := range args {
		rendered.WriteByte(' ')
		rendered.WriteString(superShellQuote(arg))
	}
	rendered.WriteString(" \"$@\"\n")
	return rendered.String()
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
		fmt.Fprintf(&builder, "export %s=%s\n", pair[0], superShellQuote(pair[1]))
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

func superShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func writeSuperExecutable(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}
