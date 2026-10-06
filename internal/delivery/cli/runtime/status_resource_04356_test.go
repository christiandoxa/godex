package runtime

import (
	"math"
	"testing"
	"time"
)

func TestProdex04356StatusProcParsersExtractResourceCounters(t *testing.T) {
	if got, ok := parseProcessCPUTicks("123 (godex worker) S 1 2 3 4 5 6 7 8 9 10 120 30 0 0 0"); !ok || got != 150 {
		t.Fatalf("process cpu ticks = %d/%t, want 150/true", got, ok)
	}
	if got, ok := parseSystemCPUTicks("cpu  10 20 30 40 50 60 70 80 90\ncpu0 1 2 3 4"); !ok || got != 360 {
		t.Fatalf("system cpu ticks = %d/%t, want 360/true", got, ok)
	}
	if got, ok := parseKiBField("VmRSS: 2048 kB\n", "VmRSS"); !ok || got != 2_097_152 {
		t.Fatalf("VmRSS = %d/%t, want 2097152/true", got, ok)
	}
	if got, ok := parseUintField("read_bytes: 123\nwrite_bytes: 456\n", "write_bytes"); !ok || got != 456 {
		t.Fatalf("write bytes = %d/%t, want 456/true", got, ok)
	}
}

func TestProdex04356StatusResourceSnapshotDerivesCPUAndDiskRates(t *testing.T) {
	at := time.Unix(100, 0)
	previous := &statusResourcePrevious{
		at: at,
		counters: statusResourceCounters{
			available: true, processCPUTicks: 100, systemCPUTicks: 1_000,
			diskReadBytes: 1_000, diskWriteBytes: 2_000,
		},
	}
	current := statusResourceCounters{
		available: true, processCPUTicks: 120, systemCPUTicks: 1_200,
		diskReadBytes: 3_000, diskWriteBytes: 5_000,
	}
	snapshot := statusResourceSnapshotFromCounters(previous, current, at.Add(2*time.Second))
	if snapshot.cpuPercent == nil || math.Abs(*snapshot.cpuPercent-10) > 0.0001 {
		t.Fatalf("cpu percent = %#v, want 10", snapshot.cpuPercent)
	}
	if snapshot.diskReadBytesPerSecond != 1_000 || snapshot.diskWriteBytesPerSecond != 1_500 {
		t.Fatalf("disk rates = %d/%d, want 1000/1500",
			snapshot.diskReadBytesPerSecond, snapshot.diskWriteBytesPerSecond)
	}
}

func TestProdex04356StatusNetworkQueueFiltersGodexSocketInodes(t *testing.T) {
	table := "" +
		"sl local_address rem_address st tx_queue:rx_queue tr tm->when retrnsmt uid timeout inode\n" +
		"0: 0100007F:1F90 00000000:0000 0A 00000010:00000020 00:00000000 00000000 1000 0 42\n" +
		"1: 0100007F:1F91 00000000:0000 0A 00000100:00000200 00:00000000 00000000 1000 0 99\n"
	rx, tx := parseNetworkQueues(table, map[uint64]struct{}{42: {}})
	if rx != 0x20 || tx != 0x10 {
		t.Fatalf("network queues = rx:%x tx:%x, want 20/10", rx, tx)
	}
}

func TestProdex04356StatusFieldsExposeUnavailableResources(t *testing.T) {
	fields := statusFields(mustOverview(t), statusResourceSnapshot{})
	for _, label := range []string{"Processes", "Memory", "Network", "Disk I/O"} {
		found := false
		for _, field := range fields {
			if field[0] == label {
				found = true
				if field[1] != "unavailable" {
					t.Fatalf("%s = %q, want unavailable", label, field[1])
				}
			}
		}
		if !found {
			t.Fatalf("resource field %q missing", label)
		}
	}
}

func TestProdex04356StatusProcessClassificationMatchesRuntimeCommands(t *testing.T) {
	for _, fixture := range []struct {
		argv []string
		want bool
	}{
		{[]string{"godex"}, true},
		{[]string{"godex", "run", "exec", "hello"}, true},
		{[]string{"godex", "super"}, true},
		{[]string{"godex", "gateway"}, true},
		{[]string{"godex", "__runtime-broker"}, true},
		{[]string{"godex", "status"}, false},
		{[]string{"godex", "__mcp-jsonl-bridge"}, false},
		{[]string{"godex", "__sub-agent-exec"}, false},
		{[]string{"godex", "exec", "hello"}, true},
	} {
		if got := godexCommandLaunchesRuntime(fixture.argv); got != fixture.want {
			t.Fatalf("runtime classification %#v = %t, want %t", fixture.argv, got, fixture.want)
		}
	}
}

func TestProdex04356StatusHumanBytesMatchesTaggedFormatting(t *testing.T) {
	for value, want := range map[uint64]string{
		0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 64 << 20: "64.0 MiB",
	} {
		if got := statusHumanBytes(value); got != want {
			t.Fatalf("human bytes %d = %q, want %q", value, got, want)
		}
	}
}
