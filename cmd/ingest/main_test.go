package main

import (
	"context"
	"strings"
	"testing"
)

// An invalid repository URL is reported as such before the command looks for a database, so the error names the
// mistake the operator made rather than a missing setting.
func TestRunRejectsAnInvalidRepositoryURLBeforeConnecting(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_PARAMETER", "")

	_, err := run(context.Background(), "https://gitlab.com/example/rules")

	if err == nil || !strings.Contains(err.Error(), "expected https://github.com/<owner>/<repository>") {
		t.Fatalf("got error %v, want one about the repository URL", err)
	}
}
