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

// runMain runs main in a new process with env added to the test's environment, and returns its standard output's
// JSON lines and its exit code. ADDR is one it can't listen on, so a local server stops as soon as it starts.
func runMain(t *testing.T, env ...string) ([]map[string]any, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), append([]string{runMainEnv + "=1", "AWS_LAMBDA_RUNTIME_API=", "ADDR=256.0.0.1:1"}, env...)...)
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

// A function that can't start says why in a line the logs can search for, and exits, so Lambda reports the failed
// start instead of serving errors.
func TestStartupFailureIsLoggedAndStopsTheCommand(t *testing.T) {
	for name, env := range map[string][]string{
		"no database":      {"DATABASE_URL=", "DATABASE_URL_PARAMETER="},
		"an unknown level": {"DATABASE_URL=postgres://localhost/rulemart", "LOG_LEVEL=verbose"},
	} {
		t.Run(name, func(t *testing.T) {
			lines, code := runMain(t, append(env, "RULEMART_RELEASE=v9.9.9")...)

			if code != 1 {
				t.Fatalf("exited %d, want 1", code)
			}
			if len(lines) != 1 || lines[0]["msg"] != "startup failed" || lines[0]["level"] != "ERROR" || lines[0]["error"] == "" ||
				lines[0]["release"] != "v9.9.9" {
				t.Fatalf("logged %v", lines)
			}
		})
	}
}

// Started, the command says so with the schema version it needs, and never logs the connection string it was given.
func TestStartupLogsReadyWithoutTheConnectionString(t *testing.T) {
	lines, _ := runMain(t, "DATABASE_URL=postgres://rulemart:hunter2@db.example/rulemart", "DATABASE_URL_PARAMETER=")

	if len(lines) == 0 || lines[0]["msg"] != "ready" || lines[0]["schema"] == nil || lines[0]["init_ms"] == nil {
		t.Fatalf("logged %v, want ready first", lines)
	}
	for _, line := range lines {
		for key, value := range line {
			if s, ok := value.(string); ok && strings.Contains(s, "hunter2") {
				t.Fatalf("%s logged the password: %v", key, line)
			}
		}
	}
}
