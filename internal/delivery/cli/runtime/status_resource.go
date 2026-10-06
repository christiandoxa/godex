package runtime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type statusProcessInfo struct {
	pid     int
	command string
	runtime bool
}

type statusResourceCounters struct {
	available           bool
	processCount        int
	runtimeProcessCount int
	processCPUTicks     uint64
	systemCPUTicks      uint64
	residentBytes       uint64
	memoryTotalBytes    uint64
	diskReadBytes       uint64
	diskWriteBytes      uint64
	socketCount         int
	networkRXQueueBytes uint64
	networkTXQueueBytes uint64
}

type statusResourceSnapshot struct {
	available               bool
	processCount            int
	runtimeProcessCount     int
	cpuPercent              *float64
	residentBytes           uint64
	memoryTotalBytes        uint64
	diskReadBytes           uint64
	diskWriteBytes          uint64
	diskReadBytesPerSecond  uint64
	diskWriteBytesPerSecond uint64
	socketCount             int
	networkRXQueueBytes     uint64
	networkTXQueueBytes     uint64
}

type statusResourceTracker struct {
	previous *statusResourcePrevious
	now      func() time.Time
}

type statusResourcePrevious struct {
	at       time.Time
	counters statusResourceCounters
}

func newStatusResourceTracker() *statusResourceTracker {
	return &statusResourceTracker{now: time.Now}
}

func (tracker *statusResourceTracker) sample() statusResourceSnapshot {
	if tracker == nil {
		return statusResourceSnapshot{}
	}
	if tracker.now == nil {
		tracker.now = time.Now
	}
	processes := collectGodexProcesses()
	now := tracker.now()
	current := collectStatusResourceCounters(processes)
	var previous *statusResourcePrevious
	if tracker.previous != nil {
		copy := *tracker.previous
		previous = &copy
	}
	snapshot := statusResourceSnapshotFromCounters(previous, current, now)
	tracker.previous = &statusResourcePrevious{at: now, counters: current}
	return snapshot
}

func collectGodexProcesses() []statusProcessInfo {
	currentPID := os.Getpid()
	currentBase := "godex"
	if executable, err := os.Executable(); err == nil {
		if name := filepath.Base(executable); name != "" {
			currentBase = name
		}
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	processes := make([]statusProcessInfo, 0, 4)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == currentPID {
			continue
		}
		dir := filepath.Join("/proc", entry.Name())
		commandBytes, err := os.ReadFile(filepath.Join(dir, "comm"))
		if err != nil {
			continue
		}
		command := strings.TrimSpace(string(commandBytes))
		argsBytes, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil {
			continue
		}
		args := splitProcCmdline(argsBytes)
		span, ok := godexArgvSpan(command, args, currentBase)
		if !ok {
			continue
		}
		processes = append(processes, statusProcessInfo{
			pid: pid, command: godexProcessLabel(span), runtime: godexCommandLaunchesRuntime(span),
		})
	}
	return processes
}

func splitProcCmdline(content []byte) []string {
	values := make([]string, 0, 8)
	for _, chunk := range strings.Split(string(content), "\x00") {
		if chunk != "" {
			values = append(values, chunk)
		}
	}
	return values
}

func godexArgvSpan(command string, args []string, currentBase string) ([]string, bool) {
	matches := func(value string) bool {
		base := filepath.Base(value)
		return base == "godex" || base == currentBase
	}
	if matches(command) {
		if len(args) == 0 {
			return []string{command}, true
		}
		if matches(args[0]) {
			return args, true
		}
	}
	for index, argument := range args {
		if matches(argument) {
			return args[index:], true
		}
	}
	return nil, false
}

func godexProcessLabel(argv []string) string {
	if len(argv) <= 1 {
		return "run"
	}
	switch argv[1] {
	case "profile":
		return "profile"
	case "use":
		return "use"
	case "current":
		return "current"
	case "info":
		return "info"
	case "status":
		return "status"
	case "log":
		return "log"
	case "session":
		return "session"
	case "doctor":
		return "doctor"
	case "login":
		return "login"
	case "logout":
		return "logout"
	case "update":
		return "update"
	case "quota":
		return "quota"
	case "redeem":
		return "redeem"
	case "ping":
		return "ping"
	case "run":
		return "run"
	case "super", "s":
		return "super"
	case "gateway":
		return "gateway"
	case "__super-expose":
		return "super-expose"
	case "__runtime-broker":
		return "__runtime-broker"
	case "__mcp-jsonl-bridge":
		return "__mcp-jsonl-bridge"
	case "__sub-agent-exec":
		return "__sub-agent-exec"
	default:
		return "run"
	}
}

func godexCommandLaunchesRuntime(argv []string) bool {
	if len(argv) <= 1 {
		return true
	}
	command := argv[1]
	switch command {
	case "run", "super", "s", "gateway", "__super-expose", "__runtime-broker":
		return true
	case "login", "logout", "accounts", "current", "import-current", "account", "profile", "use", "remove",
		"quota", "redeem", "ping", "update", "session", "info", "status", "log", "doctor",
		"__mcp-jsonl-bridge", "__sub-agent-exec", "version", "--version", "-version", "help", "--help", "-h":
		return false
	default:
		// Unknown top-level arguments are Codex passthroughs and launch through the runtime.
		return true
	}
}

func collectStatusResourceCounters(processes []statusProcessInfo) statusResourceCounters {
	pids := make(map[int]struct{}, len(processes)+1)
	runtimeCount := 0
	for _, process := range processes {
		pids[process.pid] = struct{}{}
		if process.runtime {
			runtimeCount++
		}
	}
	pids[os.Getpid()] = struct{}{}

	systemText, err := os.ReadFile("/proc/stat")
	if err != nil {
		return statusResourceCounters{processCount: len(pids), runtimeProcessCount: runtimeCount}
	}
	systemTicks, ok := parseSystemCPUTicks(string(systemText))
	if !ok {
		return statusResourceCounters{processCount: len(pids), runtimeProcessCount: runtimeCount}
	}
	counters := statusResourceCounters{
		available: true, processCount: len(pids), runtimeProcessCount: runtimeCount,
		systemCPUTicks: systemTicks,
	}
	if meminfo, err := os.ReadFile("/proc/meminfo"); err == nil {
		counters.memoryTotalBytes, _ = parseKiBField(string(meminfo), "MemTotal")
	}
	socketInodes := make(map[uint64]struct{})
	for pid := range pids {
		dir := filepath.Join("/proc", strconv.Itoa(pid))
		if text, err := os.ReadFile(filepath.Join(dir, "stat")); err == nil {
			if ticks, ok := parseProcessCPUTicks(string(text)); ok {
				counters.processCPUTicks = saturatingAdd(counters.processCPUTicks, ticks)
			}
		}
		if text, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
			if bytes, ok := parseKiBField(string(text), "VmRSS"); ok {
				counters.residentBytes = saturatingAdd(counters.residentBytes, bytes)
			}
		}
		if text, err := os.ReadFile(filepath.Join(dir, "io")); err == nil {
			if bytes, ok := parseUintField(string(text), "read_bytes"); ok {
				counters.diskReadBytes = saturatingAdd(counters.diskReadBytes, bytes)
			}
			if bytes, ok := parseUintField(string(text), "write_bytes"); ok {
				counters.diskWriteBytes = saturatingAdd(counters.diskWriteBytes, bytes)
			}
		}
		collectSocketInodes(filepath.Join(dir, "fd"), socketInodes)
	}
	counters.socketCount = len(socketInodes)
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6", "/proc/net/udp", "/proc/net/udp6"} {
		table, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rx, tx := parseNetworkQueues(string(table), socketInodes)
		counters.networkRXQueueBytes = saturatingAdd(counters.networkRXQueueBytes, rx)
		counters.networkTXQueueBytes = saturatingAdd(counters.networkTXQueueBytes, tx)
	}
	return counters
}

func statusResourceSnapshotFromCounters(
	previous *statusResourcePrevious,
	current statusResourceCounters,
	now time.Time,
) statusResourceSnapshot {
	snapshot := statusResourceSnapshot{
		available:           current.available,
		processCount:        current.processCount,
		runtimeProcessCount: current.runtimeProcessCount,
		residentBytes:       current.residentBytes,
		memoryTotalBytes:    current.memoryTotalBytes,
		diskReadBytes:       current.diskReadBytes,
		diskWriteBytes:      current.diskWriteBytes,
		socketCount:         current.socketCount,
		networkRXQueueBytes: current.networkRXQueueBytes,
		networkTXQueueBytes: current.networkTXQueueBytes,
	}
	if previous == nil || !previous.counters.available || !current.available {
		return snapshot
	}
	systemDelta := saturatingSub(current.systemCPUTicks, previous.counters.systemCPUTicks)
	processDelta := saturatingSub(current.processCPUTicks, previous.counters.processCPUTicks)
	if systemDelta > 0 {
		cpu := float64(processDelta) / float64(systemDelta) * 100
		if cpu < 0 {
			cpu = 0
		}
		if cpu > 100 {
			cpu = 100
		}
		snapshot.cpuPercent = &cpu
	}
	seconds := now.Sub(previous.at).Seconds()
	if seconds < 0.001 {
		seconds = 0.001
	}
	snapshot.diskReadBytesPerSecond = uint64(float64(saturatingSub(
		current.diskReadBytes, previous.counters.diskReadBytes,
	)) / seconds)
	snapshot.diskWriteBytesPerSecond = uint64(float64(saturatingSub(
		current.diskWriteBytes, previous.counters.diskWriteBytes,
	)) / seconds)
	return snapshot
}

func parseProcessCPUTicks(text string) (uint64, bool) {
	index := strings.LastIndex(text, ")")
	if index < 0 || index+1 >= len(text) {
		return 0, false
	}
	fields := strings.Fields(text[index+1:])
	if len(fields) <= 12 {
		return 0, false
	}
	user, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, false
	}
	system, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, false
	}
	return saturatingAdd(user, system), true
}

func parseSystemCPUTicks(text string) (uint64, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return 0, false
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 2 || fields[0] != "cpu" {
		return 0, false
	}
	total := uint64(0)
	limit := min(9, len(fields))
	for _, value := range fields[1:limit] {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return 0, false
		}
		total = saturatingAdd(total, parsed)
	}
	return total, true
}

func parseKiBField(text, key string) (uint64, bool) {
	for _, line := range strings.Split(text, "\n") {
		candidate, value, ok := strings.Cut(line, ":")
		if !ok || candidate != key {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return 0, false
		}
		kib, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, false
		}
		if kib > ^uint64(0)/1024 {
			return ^uint64(0), true
		}
		return kib * 1024, true
	}
	return 0, false
}

func parseUintField(text, key string) (uint64, bool) {
	for _, line := range strings.Split(text, "\n") {
		candidate, value, ok := strings.Cut(line, ":")
		if !ok || candidate != key {
			continue
		}
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func collectSocketInodes(fdDir string, inodes map[uint64]struct{}) {
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		value, ok := strings.CutPrefix(target, "socket:[")
		if !ok || !strings.HasSuffix(value, "]") {
			continue
		}
		inode, err := strconv.ParseUint(strings.TrimSuffix(value, "]"), 10, 64)
		if err == nil {
			inodes[inode] = struct{}{}
		}
	}
}

func parseNetworkQueues(text string, inodes map[uint64]struct{}) (uint64, uint64) {
	var rx, tx uint64
	lines := strings.Split(text, "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) <= 9 {
			continue
		}
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		if _, ok := inodes[inode]; !ok {
			continue
		}
		txHex, rxHex, ok := strings.Cut(fields[4], ":")
		if !ok {
			continue
		}
		txValue, _ := strconv.ParseUint(txHex, 16, 64)
		rxValue, _ := strconv.ParseUint(rxHex, 16, 64)
		tx = saturatingAdd(tx, txValue)
		rx = saturatingAdd(rx, rxValue)
	}
	return rx, tx
}

func saturatingAdd(left, right uint64) uint64 {
	if ^uint64(0)-left < right {
		return ^uint64(0)
	}
	return left + right
}

func saturatingSub(left, right uint64) uint64 {
	if left < right {
		return 0
	}
	return left - right
}

func statusMemoryPercent(snapshot statusResourceSnapshot) float64 {
	if snapshot.memoryTotalBytes == 0 {
		return 0
	}
	return float64(snapshot.residentBytes) / float64(snapshot.memoryTotalBytes) * 100
}

func statusHumanBytes(bytes uint64) string {
	const units = 5
	names := [units]string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit+1 < len(names) {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatUint(bytes, 10) + " B"
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + names[unit]
}
