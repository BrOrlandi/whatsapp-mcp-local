package localasr

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/platform"
)

// Why the engine is installed but cannot run here (Status.Unavailable).
const (
	// UnavailableWindowsRuntime: Windows could not load a library the engine
	// needs, the Microsoft Visual C++ runtime (exit 0xC0000135). The Windows
	// package uses whisper.cpp's official build, which needs it; until that
	// package is rebuilt (an issue on GitHub asks for help), transcription on
	// such a computer is not available.
	UnavailableWindowsRuntime = "windows-runtime"
	// UnavailableEngine: the engine did not start for another reason.
	UnavailableEngine = "engine-failed"
)

// ErrUnavailable means the engine is installed but cannot run on this
// computer; errors.Is finds it in what Transcribe and the install return.
var ErrUnavailable = errors.New("local transcription is not available on this computer")

// statusDLLNotFound is Windows' STATUS_DLL_NOT_FOUND: a program's library
// is missing, and Windows stops it before it prints anything.
const statusDLLNotFound = 0xC0000135

// selfTestOn says whether the engine is run once before it counts as ready.
// Only on Windows, where the published package depends on a runtime that a
// clean install lacks; elsewhere the engine is one self-contained program
// and nothing changes. A variable so tests can turn it on.
var selfTestOn = runtime.GOOS == "windows"

// selfTestTimeout bounds the test. Past it the answer is unknown (an
// antivirus scanning the program the first time it runs), and the engine
// keeps counting as ready.
var selfTestTimeout = 10 * time.Second

// failedTestTTL is how long a failed test is believed before it runs again
// by itself: installing the runtime fixes the computer without changing the
// program. Opening the settings runs it again at once (Recheck).
var failedTestTTL = 10 * time.Minute

// unknownTestTTL is how long a test that took too long is believed (as
// working) before it runs again, so a slow computer does not wait for it on
// every status.
var unknownTestTTL = time.Minute

type selfTestResult struct {
	key     string // the program's path, size and modification time
	reason  string // empty: it runs
	unknown bool   // it took too long: counted as running, for a while
	at      time.Time
}

// fresh says the result still holds for the program at key.
func (r selfTestResult) fresh(key string) bool {
	switch {
	case r.key != key:
		return false
	case r.unknown:
		return time.Since(r.at) < unknownTestTTL
	case r.reason != "":
		return time.Since(r.at) < failedTestTTL
	}
	return true
}

// unavailable runs the engine once (whisper-cli -h, which only prints the
// options) and says why it cannot run, or "" when it does or the test is
// off. A passing result is kept until the program changes; a failing one
// for failedTestTTL or until Recheck.
func (e *Engine) unavailable(whisper string) string {
	if !selfTestOn || whisper == "" {
		return ""
	}
	fi, err := os.Stat(whisper)
	if err != nil {
		return "" // a missing program is said by Missing
	}
	key := fmt.Sprintf("%s|%d|%d", whisper, fi.Size(), fi.ModTime().UnixNano())
	e.testMu.Lock()
	defer e.testMu.Unlock()
	if e.tested.fresh(key) {
		return e.tested.reason
	}
	reason, known := runSelfTest(whisper)
	e.tested = selfTestResult{key: key, reason: reason, unknown: !known, at: time.Now()}
	return reason
}

// Recheck forgets a test that failed, so the next Status runs the engine
// again: the settings page calls it when it opens. A test that passed holds
// until the program changes, and one that took too long holds for its
// minute: run again at every opening, a slow computer would wait for it
// each time, and every status with it.
func (e *Engine) Recheck() {
	e.testMu.Lock()
	if e.tested.reason != "" {
		e.tested = selfTestResult{}
	}
	e.testMu.Unlock()
}

// markUnavailable keeps what a failed transcription learned, so the status
// says it without waiting for the next test.
func (e *Engine) markUnavailable(whisper, reason string) {
	fi, err := os.Stat(whisper)
	if err != nil || !selfTestOn {
		return
	}
	e.testMu.Lock()
	e.tested = selfTestResult{key: fmt.Sprintf("%s|%d|%d", whisper, fi.Size(), fi.ModTime().UnixNano()), reason: reason, at: time.Now()}
	e.testMu.Unlock()
}

// runSelfTest runs the program with -h. known is false when the answer is
// not certain (it took too long), and then reason is empty.
func runSelfTest(whisper string) (reason string, known bool) {
	ctx, cancel := context.WithTimeout(context.Background(), selfTestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, whisper, "-h")
	cmd.WaitDelay = time.Second // a child holding the output open does not hold the test
	platform.Background(cmd)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return "", true
	}
	if ctx.Err() != nil {
		log.Printf("transcription: the engine test took over %s; counting it as working", selfTestTimeout)
		return "", false
	}
	reason = UnavailableEngine
	if missingRuntime(err) {
		reason = UnavailableWindowsRuntime
	}
	log.Printf("transcription: the engine does not start on this computer (%s): %s", exitDetail(err), lastLines(string(out), 3))
	return reason, true
}

// missingRuntime says the program stopped because Windows could not load a
// library it needs (the Visual C++ runtime, for whisper.cpp's official
// build). Windows gives the NTSTATUS as the exit code.
func missingRuntime(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && isDLLNotFound(ee.ExitCode())
}

func isDLLNotFound(code int) bool { return uint32(code) == statusDLLNotFound }

// exitDetail says how a program ended, for an error that would otherwise
// be empty: a program Windows stops for a missing library prints nothing.
func exitDetail(err error) string {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return err.Error()
	}
	code := ee.ExitCode()
	if isDLLNotFound(code) {
		return "exit status 0xC0000135, a library is missing: the Microsoft Visual C++ runtime"
	}
	if code > 255 || code < -1 {
		return fmt.Sprintf("exit status 0x%08X", uint32(code))
	}
	return ee.Error()
}

// failureText is what a program printed, or how it ended when it printed
// nothing.
func failureText(out []byte, err error, n int) string {
	if s := lastLines(string(out), n); s != "" {
		return s
	}
	return exitDetail(err)
}

// unavailableError is the error for an engine that cannot run here, for
// the AI and the log; the panel says it in its own words.
func unavailableError(reason string) error {
	if reason == UnavailableWindowsRuntime {
		return fmt.Errorf("%w: the engine cannot start on this Windows (a library is missing: the Microsoft Visual C++ runtime); local transcription does not work on Windows yet", ErrUnavailable)
	}
	return fmt.Errorf("%w: the engine does not start here", ErrUnavailable)
}
