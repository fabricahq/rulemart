package logging_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fabricahq/rulemart/internal/platform/logging"
)

// env returns a getenv that reads vars.
func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// lines decodes each JSON line in out.
func lines(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	var decoded []map[string]any
	for line := range strings.Lines(out.String()) {
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("a line isn't JSON: %q: %v", line, err)
		}
		decoded = append(decoded, fields)
	}
	return decoded
}

// messages returns the msg of each line in out.
func messages(t *testing.T, out *bytes.Buffer) []string {
	t.Helper()
	var msgs []string
	for _, line := range lines(t, out) {
		msgs = append(msgs, line["msg"].(string))
	}
	return msgs
}

func TestNewLogsAtTheLevelLogLevelNames(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  []string
	}{
		{"", []string{"info", "warn", "error"}},
		{"debug", []string{"debug", "info", "warn", "error"}},
		{"INFO", []string{"info", "warn", "error"}},
		{"warn", []string{"warn", "error"}},
		{"error", []string{"error"}},
	} {
		t.Run(tc.level, func(t *testing.T) {
			var out bytes.Buffer

			logger, err := logging.New(&out, env(map[string]string{"LOG_LEVEL": tc.level}))
			if err != nil {
				t.Fatal(err)
			}
			logger.Debug("debug")
			logger.Info("info")
			logger.Warn("warn")
			logger.Error("error")

			if got := messages(t, &out); strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("logged %v, want %v", got, tc.want)
			}
		})
	}
}

// A typo in LOG_LEVEL must stop the command rather than silently log at another level, but the command still needs
// a logger to say why it stopped.
func TestNewRejectsAnUnknownLevelAndStillReturnsALogger(t *testing.T) {
	var out bytes.Buffer

	logger, err := logging.New(&out, env(map[string]string{"LOG_LEVEL": "verbose"}))

	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") || !strings.Contains(err.Error(), `"verbose"`) {
		t.Fatalf("got error %v, want one naming LOG_LEVEL and its value", err)
	}
	logger.Debug("debug")
	logger.Info("info")
	if got := messages(t, &out); len(got) != 1 || got[0] != "info" {
		t.Fatalf("the fallback logger logged %v, want only the info line", got)
	}
}

func TestNewNamesTheReleaseOnEveryLine(t *testing.T) {
	for name, tc := range map[string]struct {
		release string
		want    string
	}{
		"a deployed release": {"v0.0.3", "v0.0.3"},
		"a local build":      {"", "dev"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer

			logger, err := logging.New(&out, env(map[string]string{"RULEMART_RELEASE": tc.release}))
			if err != nil {
				t.Fatal(err)
			}
			logger.Info("one")
			logger.With("component", "x").Warn("two")

			for _, line := range lines(t, &out) {
				if line["release"] != tc.want {
					t.Errorf("%v: release is %v, want %q", line["msg"], line["release"], tc.want)
				}
			}
		})
	}
}

func TestReadyReportsTheSchemaVersionAndStartupTime(t *testing.T) {
	var out bytes.Buffer
	logger, err := logging.New(&out, env(nil))
	if err != nil {
		t.Fatal(err)
	}

	logging.Ready(logger, 3, 1500*time.Microsecond)

	got := lines(t, &out)
	if len(got) != 1 || got[0]["msg"] != "ready" || got[0]["level"] != "INFO" || got[0]["schema"] != 3.0 || got[0]["init_ms"] != 1.5 {
		t.Fatalf("logged %v", got)
	}
}
