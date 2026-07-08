package pipertts

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestHelperProcess is NOT a real test — it's a stand-in "piper" process for
// the warmWorker tests below, following the classic Go self-exec test-helper
// pattern (see os/exec_test.go). Run normally via `go test`, it's a no-op
// (PIPERTTS_HELPER_PROCESS is unset). The tests below invoke it as
// exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--") with that env
// var set, giving warmWorker a real subprocess to talk to — with piper's exact
// line-in/path-out stdio contract — without needing a real piper install.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("PIPERTTS_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	outDir := ""
	for i, a := range os.Args {
		if a == "--output_dir" && i+1 < len(os.Args) {
			outDir = os.Args[i+1]
		}
	}
	mode := os.Getenv("PIPERTTS_HELPER_MODE") // "" (echo) | "silent-then-exit"

	reader := bufio.NewReader(os.Stdin)
	n := 0
	for {
		if _, err := reader.ReadString('\n'); err != nil {
			return // stdin closed -> exit cleanly, mirroring real piper's EOF exit
		}
		n++
		if mode == "silent-then-exit" {
			return // die without responding: simulates an unexpected crash mid-request
		}
		name := filepath.Join(outDir, fmt.Sprintf("%d.wav", n))
		if err := os.WriteFile(name, []byte("fake-wav-content"), 0o644); err != nil {
			return
		}
		fmt.Println(name)
	}
}

// helperCommand builds an *exec.Cmd that re-execs this test binary as
// TestHelperProcess, with the same argv shape warmWorker.start() would pass a
// real piper binary (so the pipe-wiring code under test is unchanged).
func helperCommand(t *testing.T, scratchDir, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--",
		"-m", "fake-model.onnx", "--output_dir", scratchDir)
	cmd.Env = append(os.Environ(), "PIPERTTS_HELPER_PROCESS=1", "PIPERTTS_HELPER_MODE="+mode)
	return cmd
}

func newTestWorker(t *testing.T, mode string) *warmWorker {
	t.Helper()
	w := &warmWorker{
		exePath:    os.Args[0],
		modelPath:  "fake-model.onnx",
		scratchDir: t.TempDir(),
		closed:     make(chan struct{}),
	}
	if err := w.startWithCommand(helperCommand(t, w.scratchDir, mode)); err != nil {
		t.Fatalf("startWithCommand: %v", err)
	}
	t.Cleanup(w.stop)
	return w
}

func TestWarmWorkerRoundTrip(t *testing.T) {
	w := newTestWorker(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	path, err := w.synthesize(ctx, "Mez break")
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("synthesize returned a path that doesn't exist: %v", statErr)
	}
	if w.dead.Load() {
		t.Error("worker should not be dead after a successful synthesize")
	}
}

func TestWarmWorkerSequentialReuse(t *testing.T) {
	w := newTestWorker(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		path, err := w.synthesize(ctx, fmt.Sprintf("phrase %d", i))
		if err != nil {
			t.Fatalf("synthesize %d: %v", i, err)
		}
		if seen[path] {
			t.Errorf("synthesize %d reused a path from an earlier call: %s", i, path)
		}
		seen[path] = true
	}
	if w.dead.Load() {
		t.Error("worker should still be alive after repeated successful requests")
	}
}

func TestWarmWorkerDiesOnUnexpectedExit(t *testing.T) {
	w := newTestWorker(t, "silent-then-exit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := w.synthesize(ctx, "this request gets no response")
	if err == nil {
		t.Fatal("expected an error when the child exits without responding")
	}
	if !w.dead.Load() {
		t.Error("worker should be marked dead after the child exits unexpectedly")
	}

	// A dead worker must reject further requests immediately (not hang).
	if _, err := w.synthesize(ctx, "another phrase"); err == nil {
		t.Error("dead worker should reject further synthesize calls")
	}
}

func TestWarmWorkerStopIsIdempotentAndCleansScratchDir(t *testing.T) {
	w := newTestWorker(t, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := w.synthesize(ctx, "hello"); err != nil {
		t.Fatalf("synthesize: %v", err)
	}

	scratchDir := w.scratchDir
	w.stop()
	w.stop() // must not panic or block on a second call

	if _, err := os.Stat(scratchDir); !os.IsNotExist(err) {
		t.Errorf("scratch dir should be removed after stop(), stat err = %v", err)
	}
}

func TestWarmWorkerMatches(t *testing.T) {
	w := &warmWorker{exePath: "/bin/piper", modelPath: "/voices/amy.onnx"}
	if !w.matches(Config{ExePath: "/bin/piper", ModelPath: "/voices/amy.onnx"}) {
		t.Error("matches should be true for identical exe+model")
	}
	if w.matches(Config{ExePath: "/bin/piper", ModelPath: "/voices/alba.onnx"}) {
		t.Error("matches should be false for a different model")
	}
	if w.matches(Config{ExePath: "/bin/other", ModelPath: "/voices/amy.onnx"}) {
		t.Error("matches should be false for a different exe")
	}
}
