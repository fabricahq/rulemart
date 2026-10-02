package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runMainEnv makes the test binary run main instead of the tests, so a test can watch the command start.
const runMainEnv = "RULEMART_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) != "" {
		main()
		return
	}
	os.Exit(m.Run())
}

// runMain runs main in a new process with env added to the test's environment, outside Lambda, and returns its
// standard output's JSON lines and its exit code.
func runMain(t *testing.T, env ...string) ([]map[string]any, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), append([]string{runMainEnv + "=1", "AWS_LAMBDA_RUNTIME_API="}, env...)...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	code := 0
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	for line := range strings.Lines(stdout.String()) {
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("a line isn't JSON: %q", line)
		}
		lines = append(lines, fields)
	}
	return lines, code
}

// A worker that can't start says why in a line the logs can search for, and exits, so Lambda reports the failed
// start.
func TestStartupFailureIsLoggedAndStopsTheCommand(t *testing.T) {
	lines, code := runMain(t, "LOG_LEVEL=verbose", "RULEMART_RELEASE=v9.9.9")

	if code != 1 {
		t.Fatalf("exited %d, want 1", code)
	}
	if len(lines) != 1 || lines[0]["msg"] != "startup failed" || lines[0]["level"] != "ERROR" || lines[0]["release"] != "v9.9.9" {
		t.Fatalf("logged %v", lines)
	}
}

func TestStartupLogsReadyWithTheSchemaVersion(t *testing.T) {
	lines, _ := runMain(t, "RULEMART_RELEASE=v9.9.9")

	if len(lines) == 0 || lines[0]["msg"] != "ready" || lines[0]["schema"] == nil || lines[0]["init_ms"] == nil || lines[0]["release"] != "v9.9.9" {
		t.Fatalf("logged %v, want ready first", lines)
	}
}
