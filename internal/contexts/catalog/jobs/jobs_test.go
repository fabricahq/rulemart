package jobs_test

import (
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/jobs"
)

func TestJobsReadBackAsTheyWereWritten(t *testing.T) {
	library := domain.LibraryKey{Host: domain.GitHub, RepositoryID: "42"}
	for body, want := range map[string]jobs.Job{
		jobs.Update(library): {Library: library},
		jobs.CheckListing(7): {Listing: 7},
	} {
		got, err := jobs.Parse(body)
		if err != nil || got != want {
			t.Errorf("%s: got %+v, %v; want %+v", body, got, err, want)
		}
	}
	if body := jobs.Update(library); body != `{"host":"github","repositoryID":"42"}` {
		t.Errorf("a library's job is %s, which the release before listings can't read", body)
	}
	if body := jobs.CheckListing(7); body != `{"listing":7}` {
		t.Errorf("a listing's job is %s", body)
	}
}

// A job names a library or a listing and nothing else, so a queued message can't point the worker at a URL.
func TestParseRefusesWhatIsNotExactlyOneJob(t *testing.T) {
	for name, body := range map[string]string{
		"a URL":                `{"host":"github","repositoryID":"42","url":"https://example.com/rules.git"}`,
		"a listing with a URL": `{"listing":7,"url":"https://example.com/rules.git"}`,
		"both":                 `{"host":"github","repositoryID":"42","listing":7}`,
		"neither":              `{}`,
		"a listing of 0":       `{"listing":0}`,
		"a negative listing":   `{"listing":-7}`,
		"a listing as text":    `{"listing":"7"}`,
		"a fractional listing": `{"listing":7.5}`,
		"not JSON":             `hello`,
		"two jobs":             `{"listing":7}{"listing":8}`,
		"two jobs apart":       `{"listing":7} {"listing":8}`,
		"trailing text":        `{"listing":7}]`,
	} {
		if job, err := jobs.Parse(body); err == nil {
			t.Errorf("%s: accepted %+v", name, job)
		}
	}
}
