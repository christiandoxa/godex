//go:build linux

package superexpose

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type systemSessionProcessInspector struct{}

func (systemSessionProcessInspector) currentUID() (uint32, error) {
	return uint32(os.Getuid()), nil
}

func (systemSessionProcessInspector) list() ([]processRecord, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, sessionVerificationInconclusive
	}
	result := make([]processRecord, 0, len(entries)/8)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid64, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil || pid64 == 0 {
			continue
		}
		record, err := readLinuxProcessRecord(uint32(pid64))
		if err == nil {
			result = append(result, record)
		}
	}
	return result, nil
}

func (systemSessionProcessInspector) inspect(pid uint32) (*processDetails, error) {
	record, err := readLinuxProcessRecord(pid)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, sessionVerificationInconclusive
	}
	environment, err := readLinuxTargetEnvironment(pid)
	if err != nil {
		return nil, err
	}
	files, err := readLinuxAuthoritativeOpenFiles(pid)
	if err != nil {
		return nil, err
	}
	return &processDetails{record: record, environment: environment, openFiles: files}, nil
}

func readLinuxProcessRecord(pid uint32) (processRecord, error) {
	root := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10))
	status, err := os.ReadFile(filepath.Join(root, "status"))
	if err != nil {
		return processRecord{}, err
	}
	uid := uint32(0)
	state := processDead
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
			if len(fields) > 0 {
				value, parseErr := strconv.ParseUint(fields[0], 10, 32)
				if parseErr == nil {
					uid = uint32(value)
				}
			}
		}
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(strings.TrimPrefix(line, "State:"))
			if len(fields) > 0 && len(fields[0]) > 0 {
				state = linuxProcessState(fields[0][0])
			}
		}
	}
	if uid == 0 && os.Getuid() != 0 {
		return processRecord{}, fmt.Errorf("process uid unavailable")
	}

	stat, err := os.ReadFile(filepath.Join(root, "stat"))
	if err != nil {
		return processRecord{}, err
	}
	closeParen := strings.LastIndex(string(stat), ")")
	if closeParen < 0 || closeParen+2 > len(stat) {
		return processRecord{}, fmt.Errorf("process stat malformed")
	}
	fields := strings.Fields(string(stat)[closeParen+2:])
	if len(fields) <= 19 {
		return processRecord{}, fmt.Errorf("process stat incomplete")
	}
	parent64, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil {
		return processRecord{}, err
	}
	startTime := fields[19]

	cmdline, err := os.ReadFile(filepath.Join(root, "cmdline"))
	if err != nil {
		return processRecord{}, err
	}
	argv := make([]string, 0)
	for _, item := range strings.Split(string(cmdline), "\x00") {
		if item != "" {
			argv = append(argv, item)
		}
	}
	if len(argv) == 0 {
		return processRecord{}, fmt.Errorf("process argv unavailable")
	}

	executable, err := os.Readlink(filepath.Join(root, "exe"))
	if err != nil {
		return processRecord{}, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return processRecord{}, err
	}
	cwd, err := os.Readlink(filepath.Join(root, "cwd"))
	if err != nil {
		return processRecord{}, err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return processRecord{}, err
	}
	return processRecord{
		pid: pid, parentPID: uint32(parent64), uid: uid, state: state,
		executable: executable, argv: argv, cwd: cwd, startTime: startTime,
		birthIdentity: linuxProcessBirthIdentity(startTime),
	}, nil
}

func linuxProcessState(value byte) processState {
	switch value {
	case 'R', 'S', 'D', 'I':
		return processRunning
	case 'T', 't':
		return processStopped
	case 'Z':
		return processZombie
	default:
		return processDead
	}
}

func linuxProcessBirthIdentity(startTime string) string {
	if startTime == "" {
		return ""
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	boot := strings.TrimSpace(string(bootID))
	if boot == "" {
		return ""
	}
	return "linux:" + boot + ":" + startTime
}

func readLinuxTargetEnvironment(pid uint32) (targetEnvironment, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "environ"))
	if err != nil {
		return targetEnvironment{}, sessionTargetEnvUnavailable
	}
	values := map[string]string{}
	for _, entry := range strings.Split(string(raw), "\x00") {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "HOME", "CODEX_HOME", "CODEX_SQLITE_HOME", "PWD":
			values[key] = value
		}
	}
	return targetEnvironment{
		home: values["HOME"], codexHome: values["CODEX_HOME"],
		codexSQLiteHome: values["CODEX_SQLITE_HOME"], pwd: values["PWD"],
	}, nil
}

func readLinuxAuthoritativeOpenFiles(pid uint32) ([]openProcessFile, error) {
	root := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "fd")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, sessionThreadIdentityUnavailable
	}
	sockets := readLinuxUnixSocketPaths()
	result := make([]openProcessFile, 0)
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
			inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			if path := sockets[inode]; path != "" && isControlSocket(path) {
				result = append(result, openProcessFile{path: path})
			}
			continue
		}
		if authoritativeOpenPath(target) {
			result = append(result, openProcessFile{path: target})
		}
	}
	return result, nil
}

func authoritativeOpenPath(path string) bool {
	clean := strings.TrimSuffix(path, " (deleted)")
	name := filepath.Base(clean)
	if name == "queue_1.sqlite" ||
		strings.HasPrefix(name, "state_") && strings.HasSuffix(name, ".sqlite") ||
		rolloutFileName(name) {
		return true
	}
	return strings.HasSuffix(name, ".lock") && filepath.Base(filepath.Dir(clean)) == "thread-writer-locks"
}

func readLinuxUnixSocketPaths() map[string]string {
	content, err := os.ReadFile("/proc/net/unix")
	if err != nil {
		return map[string]string{}
	}
	result := map[string]string{}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		result[fields[6]] = fields[7]
	}
	return result
}
