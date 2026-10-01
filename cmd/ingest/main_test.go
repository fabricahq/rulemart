package main

import (
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
		{app.Result{Releases: 1, Rules: 1, Changed: 1}, "ingested fabricahq/rules (github repository 42): 1 library release, 1 current rule, 1 row changed"},
		{app.Result{Releases: 3, Rules: 0, Changed: 0}, "ingested fabricahq/rules (github repository 42): 3 library releases, 0 current rules, 0 rows changed"},
	} {
		if got := summary(repo, tc.result); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
