package main

import (
	"context"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
)

func TestSummaryCountsInWordsThatAgreeWithTheirNumbers(t *testing.T) {
	repo := domain.Repository{Host: "github", ID: "42", Owner: "fabricahq", Name: "rules"}
	for _, tc := range []struct {
		result app.Result
		want   string
	}{
		{app.Result{Repository: repo, Releases: 1, Rules: 1, Changed: 1}, "ingested fabricahq/rules (github repository 42): 1 library release, 1 current rule, 1 row changed"},
		{app.Result{Repository: repo, Releases: 3, Rules: 0, Changed: 0}, "ingested fabricahq/rules (github repository 42): 3 library releases, 0 current rules, 0 rows changed"},
	} {
		if got := summary(tc.result); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

// An invalid repository URL is reported as such before the command looks for a database, so the error names the
// mistake the operator made rather than a missing setting.
func TestRunRejectsAnInvalidRepositoryURLBeforeConnecting(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_PARAMETER", "")

	err := run(context.Background(), "https://gitlab.com/example/rules")

	if err == nil || !strings.Contains(err.Error(), "expected https://github.com/<owner>/<repository>") {
		t.Fatalf("got error %v, want one about the repository URL", err)
	}
}
