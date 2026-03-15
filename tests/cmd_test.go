package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"miniDocker/cmd"
	"miniDocker/state"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newLM(t *testing.T) *state.LifecycleManager {
	t.Helper()
	sm, err := state.NewStateManagerWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("state manager: %v", err)
	}
	return state.NewLifecycleManager(sm)
}

func seedContainer(t *testing.T, lm *state.LifecycleManager, id string, status state.ContainerStatus) {
	t.Helper()
	cfg := &state.ContainerConfig{Image: "/tmp/img", Command: []string{"/bin/sh"}}
	if err := lm.InitContainer(id, cfg); err != nil {
		t.Fatalf("InitContainer: %v", err)
	}
	if status == state.StatusRunning || status == state.StatusExited || status == state.StatusError {
		if err := lm.MarkRunning(id, 10000+len(id)); err != nil {
			t.Fatalf("MarkRunning: %v", err)
		}
	}
	switch status {
	case state.StatusExited:
		if err := lm.MarkExited(id, 0); err != nil {
			t.Fatalf("MarkExited: %v", err)
		}
	case state.StatusError:
		if err := lm.MarkError(id, "simulated"); err != nil {
			t.Fatalf("MarkError: %v", err)
		}
	}
}

// ── cmd/helpers unit tests ────────────────────────────────────────────────────

func TestHelpers_TruncateID(t *testing.T) {
	if got := cmd.TruncateID("abcdef123456789", 12); got != "abcdef123456" {
		t.Errorf("got %q", got)
	}
	if got := cmd.TruncateID("short", 12); got != "short" {
		t.Errorf("got %q", got)
	}
}

func TestHelpers_TruncateStr(t *testing.T) {
	if got := cmd.TruncateStr("hello world", 8); got != "hello..." {
		t.Errorf("got %q", got)
	}
	if got := cmd.TruncateStr("hi", 8); got != "hi" {
		t.Errorf("got %q", got)
	}
}

func TestHelpers_FormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KiB"},
		{1024 * 1024, "1.0MiB"},
		{256 * 1024 * 1024, "256.0MiB"},
		{-1, "-"},
	}
	for _, c := range cases {
		if got := cmd.FormatBytes(c.in); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHelpers_FormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{3661 * time.Second, "1h1m"},
		{48 * time.Hour, "2d0h"},
	}
	for _, c := range cases {
		if got := cmd.FormatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestHelpers_FormatPercent(t *testing.T) {
	if got := cmd.FormatPercent(25, 100); got != "25.00%" {
		t.Errorf("got %q", got)
	}
	if got := cmd.FormatPercent(0, 0); got != "0.00%" {
		t.Errorf("got %q", got)
	}
}

func TestHelpers_PadRight(t *testing.T) {
	if got := cmd.PadRight("hi", 6); got != "hi    " {
		t.Errorf("got %q", got)
	}
	if got := cmd.PadRight("toolong", 3); got != "toolong" {
		t.Errorf("got %q", got)
	}
}

func TestHelpers_ReadCgroupFile_Missing(t *testing.T) {
	v, err := cmd.ReadCgroupFile("/nonexistent/path", "memory.current")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if v != "" {
		t.Errorf("expected empty string, got %q", v)
	}
}

func TestHelpers_ReadCgroupFile_Present(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/memory.current", []byte("67108864\n"), 0644); err != nil {
		t.Fatal(err)
	}
	v, err := cmd.ReadCgroupFile(dir, "memory.current")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "67108864" {
		t.Errorf("got %q", v)
	}
}

// ── ps ────────────────────────────────────────────────────────────────────────

func TestPs_BadFlag(t *testing.T) {
	if err := cmd.Ps([]string{"--no-such-flag"}); err == nil {
		t.Error("expected error")
	}
}

func TestPs_FilterRunning(t *testing.T) {
	lm := newLM(t)
	seedContainer(t, lm, "aaaa111111111111", state.StatusRunning)
	seedContainer(t, lm, "bbbb222222222222", state.StatusExited)
	seedContainer(t, lm, "cccc333333333333", state.StatusCreated)

	all, _ := lm.ListContainers()
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}

	var running []*state.ContainerState
	for _, c := range all {
		if c.Status == state.StatusRunning {
			running = append(running, c)
		}
	}
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

// ── logs ─────────────────────────────────────────────────────────────────────

func TestLogs_NoID(t *testing.T) {
	if err := cmd.Logs([]string{}); err == nil {
		t.Error("expected error")
	}
}

func TestLogs_BadFlag(t *testing.T) {
	if err := cmd.Logs([]string{"--no-such-flag", "someid"}); err == nil {
		t.Error("expected error")
	}
}

func TestLogs_ReadWrite(t *testing.T) {
	dir := t.TempDir()
	lm, err := state.NewLogManager("testcont", dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(lm.StdoutWriter(), "line %02d\n", i)
	}
	lm.Close()

	lm2, _ := state.NewLogManager("testcont", dir)
	defer lm2.Close()

	data, err := lm2.GetLogs(5)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[4], "20") {
		t.Errorf("last line should be line 20, got %q", lines[4])
	}
}

// ── stop ─────────────────────────────────────────────────────────────────────

func TestStop_NoID(t *testing.T) {
	if err := cmd.Stop([]string{}); err == nil {
		t.Error("expected error")
	}
}

func TestStop_NotRunning(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root (state dir /var/run/miniDocker)")
	}
	err := cmd.Stop([]string{"notarealcontainer0000"})
	t.Logf("Stop non-existent: %v", err)
}

// ── remove ───────────────────────────────────────────────────────────────────

func TestRemove_NoID(t *testing.T) {
	if err := cmd.Remove([]string{}); err == nil {
		t.Error("expected error")
	}
}

func TestRemove_RunningWithoutForce(t *testing.T) {
	lm := newLM(t)
	id := "removerunning11111111"
	seedContainer(t, lm, id, state.StatusRunning)

	cs, _ := lm.GetState(id)
	if cs.Status.IsTerminal() {
		t.Fatal("should not be terminal")
	}
}

func TestRemove_ExitedContainer(t *testing.T) {
	lm := newLM(t)
	id := "removeexited111111111"
	seedContainer(t, lm, id, state.StatusExited)

	if err := lm.Cleanup(id); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := lm.GetState(id); err == nil {
		t.Error("expected container gone after cleanup")
	}
}

// ── inspect ───────────────────────────────────────────────────────────────────

func TestInspect_NoID(t *testing.T) {
	if err := cmd.Inspect([]string{}); err == nil {
		t.Error("expected error")
	}
}

func TestInspect_JSONRoundtrip(t *testing.T) {
	lm := newLM(t)
	id := "inspecttest111111111"
	cfg := &state.ContainerConfig{
		Image:   "/opt/img",
		Command: []string{"/bin/nginx"},
		Memory:  "512m",
		CPU:     "1.0",
	}
	lm.InitContainer(id, cfg)
	lm.MarkRunning(id, 42)
	lm.RecordNetwork(id, "172.20.0.10", "172.20.0.1", "miniDocker0")
	lm.RecordCgroupPath(id, "/sys/fs/cgroup/miniDocker/"+id)

	cs, err := lm.GetState(id)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out state.ContainerState
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.ID != cs.ID {
		t.Errorf("ID: got %q want %q", out.ID, cs.ID)
	}
	if out.IPAddress != "172.20.0.10" {
		t.Errorf("IP: got %q", out.IPAddress)
	}
}

// ── stats ─────────────────────────────────────────────────────────────────────

func TestStats_BadFlag(t *testing.T) {
	if err := cmd.Stats([]string{"--no-such-flag"}); err == nil {
		t.Error("expected error")
	}
}

// ── lifecycle integration ─────────────────────────────────────────────────────

func TestPhase7_FullLifecycle(t *testing.T) {
	lm := newLM(t)
	id := "lifecycletest111111"
	cfg := &state.ContainerConfig{Image: "/tmp/rootfs", Command: []string{"/bin/sh"}}

	if err := lm.InitContainer(id, cfg); err != nil {
		t.Fatal(err)
	}
	cs, _ := lm.GetState(id)
	if cs.Status != state.StatusCreated {
		t.Errorf("want created, got %s", cs.Status)
	}

	lm.MarkRunning(id, 99)
	lm.RecordNetwork(id, "172.20.0.5", "172.20.0.1", "miniDocker0")
	lm.RecordCgroupPath(id, "/sys/fs/cgroup/miniDocker/"+id)

	cs, _ = lm.GetState(id)
	if cs.Status != state.StatusRunning {
		t.Errorf("want running, got %s", cs.Status)
	}

	lm.MarkExited(id, 0)
	cs, _ = lm.GetState(id)
	if cs.Status != state.StatusExited {
		t.Errorf("want exited, got %s", cs.Status)
	}

	if err := lm.Cleanup(id); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := lm.GetState(id); err == nil {
		t.Error("expected container gone")
	}
}

func TestPhase7_Persistence(t *testing.T) {
	dir := t.TempDir()

	sm1, _ := state.NewStateManagerWithDir(dir)
	lm1 := state.NewLifecycleManager(sm1)

	id := "persisttest1111111"
	lm1.InitContainer(id, &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}})
	lm1.MarkRunning(id, 5555)
	lm1.RecordNetwork(id, "172.20.0.77", "172.20.0.1", "miniDocker0")

	sm2, _ := state.NewStateManagerWithDir(dir)
	lm2 := state.NewLifecycleManager(sm2)

	cs, err := lm2.GetState(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cs.IPAddress != "172.20.0.77" {
		t.Errorf("IP not persisted: %s", cs.IPAddress)
	}
	if cs.Pid != 5555 {
		t.Errorf("PID not persisted: %d", cs.Pid)
	}
}

func TestPhase7_MultipleContainers(t *testing.T) {
	lm := newLM(t)
	seeds := []struct {
		id     string
		status state.ContainerStatus
	}{
		{"aaa1111111111111", state.StatusRunning},
		{"bbb2222222222222", state.StatusRunning},
		{"ccc3333333333333", state.StatusExited},
		{"ddd4444444444444", state.StatusCreated},
		{"eee5555555555555", state.StatusError},
	}
	for _, s := range seeds {
		seedContainer(t, lm, s.id, s.status)
	}

	all, _ := lm.ListContainers()
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}

	runCount := 0
	for _, c := range all {
		if c.Status == state.StatusRunning {
			runCount++
		}
	}
	if runCount != 2 {
		t.Errorf("expected 2 running, got %d", runCount)
	}
}

// ── binary-level integration tests (require root + built binary) ──────────────

func TestPhase7_BinaryHelp(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)
	out, _ := exec.Command("../miniDocker_test", "help").CombinedOutput()
	for _, word := range []string{"run", "ps", "logs", "stop", "rm", "inspect", "stats"} {
		if !strings.Contains(string(out), word) {
			t.Errorf("help missing %q", word)
		}
	}
}

func TestPhase7_BinaryPs(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)
	out, err := exec.Command("../miniDocker_test", "ps", "-a").CombinedOutput()
	t.Logf("ps -a: %s err=%v", out, err)
}

func TestPhase7_BinaryInspectMissing(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)
	_, err := exec.Command("../miniDocker_test", "inspect", "deadbeef00000000").CombinedOutput()
	if err == nil {
		t.Error("expected non-zero exit for missing container")
	}
}

func TestPhase7_BinaryStatsNoStream(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)
	out, err := exec.Command("../miniDocker_test", "stats", "--no-stream", "-a").CombinedOutput()
	t.Logf("stats: %s err=%v", out, err)
}

func TestPhase7_BinaryStopMissing(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)
	_, err := exec.Command("../miniDocker_test", "stop", "notarealid000000").CombinedOutput()
	if err == nil {
		t.Error("expected error")
	}
}

func TestPhase7_BinaryRmMissing(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)
	_, err := exec.Command("../miniDocker_test", "rm", "notarealid000000").CombinedOutput()
	if err == nil {
		t.Error("expected error")
	}
}

func TestPhase7_BinaryRunAndPs(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)

	exec.Command("../miniDocker_test", "run", "/usr", "/bin/true").CombinedOutput()

	out, err := exec.Command("../miniDocker_test", "ps", "-a").CombinedOutput()
	t.Logf("ps -a after run: %s err=%v", out, err)
}

func TestPhase7_BinaryLogs(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	if testing.Short() {
		t.Skip("short mode")
	}
	buildBinary(t)

	psOut, err := exec.Command("../miniDocker_test", "ps", "-a", "--format", "ids").CombinedOutput()
	if err != nil {
		t.Skipf("ps failed: %v", err)
	}

	ids := strings.Fields(string(psOut))
	if len(ids) == 0 {
		t.Skip("no containers for logs test")
	}

	out, err := exec.Command("../miniDocker_test", "logs", "--tail", "10", ids[0]).CombinedOutput()
	t.Logf("logs: %s err=%v", out, err)
}
// ─── Performance Tests ───────────────────────────────────────────────────────

// BenchmarkContainerCreate benchmarks container creation performance
func BenchmarkContainerCreate(b *testing.B) {
	t := testing.T{}
	lm := newLM(&t)
	cfg := &state.ContainerConfig{
		Image:   "/img",
		Command: []string{"/bin/sh"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("bench-create-%d", i)
		lm.InitContainer(id, cfg)
	}
}

// BenchmarkStateTransitions benchmarks state machine transitions
func BenchmarkStateTransitions(b *testing.B) {
	t := testing.T{}
	lm := newLM(&t)
	cfg := &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("bench-state-%d", i)
		lm.InitContainer(id, cfg)
		lm.MarkRunning(id, 10000+i)
		lm.MarkExited(id, 0)
	}
}

// BenchmarkContainerListing benchmarks container enumeration
func BenchmarkContainerListing(b *testing.B) {
	t := testing.T{}
	lm := newLM(&t)
	cfg := &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}}

	// Create 100 containers
	for i := 0; i < 100; i++ {
		lm.InitContainer(fmt.Sprintf("bench-list-%d", i), cfg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm.ListContainers()
	}
}

// BenchmarkLogWrite benchmarks log write throughput
func BenchmarkLogWrite(b *testing.B) {
	dir := b.TempDir()
	lm, _ := state.NewLogManager("bench-log", dir)
	defer lm.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fmt.Fprintf(lm.StdoutWriter(), "benchmark log line %d\n", i)
	}
}

// BenchmarkLogRead benchmarks log read performance
func BenchmarkLogRead(b *testing.B) {
	dir := b.TempDir()
	lm, _ := state.NewLogManager("bench-log", dir)

	// Write 1000 lines
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(lm.StdoutWriter(), "log line %d\n", i)
	}
	lm.Close()

	lm2, _ := state.NewLogManager("bench-log", dir)
	defer lm2.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lm2.GetLogs(100)
	}
}

// BenchmarkConcurrentReads benchmarks concurrent state access
func BenchmarkConcurrentReads(b *testing.B) {
	t := testing.T{}
	lm := newLM(&t)
	id := "bench-concurrent"
	cfg := &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}}
	lm.InitContainer(id, cfg)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lm.GetState(id)
		}
	})
}

// TestBench_StateManagerPerformance comprehensive performance characterization
func TestBench_StateManagerPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}

	lm := newLM(t)
	cfg := &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}}

	// Measure creation throughput
	start := time.Now()
	for i := 0; i < 100; i++ {
		lm.InitContainer(fmt.Sprintf("perf-test-%d", i), cfg)
	}
	duration := time.Since(start)
	throughput := 100.0 * float64(time.Second) / float64(duration)
	t.Logf("Container creation: 100 containers in %v (%.0f/sec)", duration, throughput)
	if throughput < 50 {
		t.Logf("WARNING: Container creation throughput below expected (%.0f/sec < 50/sec)", throughput)
	}

	// Measure listing performance with many containers
	start = time.Now()
	for i := 0; i < 1000; i++ {
		lm.ListContainers()
	}
	duration = time.Since(start)
	avgLatency := duration / 1000
	t.Logf("Container listing: 1000 lists in %v (%.3f ms avg)", duration, float64(avgLatency.Microseconds())/1000)
	if avgLatency > 10*time.Millisecond {
		t.Logf("WARNING: Container listing latency high (%.3f ms > 10 ms)", float64(avgLatency.Microseconds())/1000)
	}
}

// TestBench_LogManagerPerformance comprehensive log performance characterization
func TestBench_LogManagerPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}

	dir := t.TempDir()
	lm, _ := state.NewLogManager("perf-log", dir)
	defer lm.Close()

	// Measure write throughput
	start := time.Now()
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(lm.StdoutWriter(), "line %d\n", i)
	}
	duration := time.Since(start)
	throughput := 10000.0 * float64(time.Second) / float64(duration)
	t.Logf("Log writing: 10000 lines in %v (%.0f lines/sec)", duration, throughput)
	if throughput < 1000 {
		t.Logf("WARNING: Log write throughput below expected (%.0f lines/sec < 1000/sec)", throughput)
	}

	// Measure read latency
	lm.Close()
	lm2, _ := state.NewLogManager("perf-log", dir)
	defer lm2.Close()

	start = time.Now()
	for i := 0; i < 100; i++ {
		lm2.GetLogs(100)
	}
	duration = time.Since(start)
	avgLatency := duration / 100
	t.Logf("Log reading: 100 reads in %v (%.3f ms avg)", duration, float64(avgLatency.Microseconds())/1000)
	if avgLatency > 50*time.Millisecond {
		t.Logf("WARNING: Log read latency high (%.3f ms > 50 ms)", float64(avgLatency.Microseconds())/1000)
	}
}