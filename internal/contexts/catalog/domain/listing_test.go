package domain

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseListedRepositoryAcceptsTheWaysListersGiveARepository(t *testing.T) {
	for _, text := range []string{
		"fabricahq/public-rules",
		" fabricahq/public-rules\n",
		"fabricahq/public-rules/",
		"fabricahq/public-rules.git",
		"github.com/fabricahq/public-rules",
		"www.github.com/fabricahq/public-rules",
		"https://github.com/fabricahq/public-rules",
		"https://github.com/fabricahq/public-rules/",
		"https://github.com/fabricahq/public-rules.git",
		"https://www.github.com/fabricahq/public-rules",
	} {
		owner, name, err := ParseListedRepository(text)
		if err != nil || owner != "fabricahq" || name != "public-rules" {
			t.Errorf("%q: got %q, %q, %v", text, owner, name, err)
		}
	}
	owner, name, err := ParseListedRepository("Old-Name/rules_v2.x")
	if err != nil || owner != "Old-Name" || name != "rules_v2.x" {
		t.Errorf("got %q, %q, %v", owner, name, err)
	}
}

func TestParseListedRepositoryRefusesWhatIsNotAGitHubRepository(t *testing.T) {
	for _, text := range []string{
		"",
		"fabricahq",
		"fabricahq/",
		"/public-rules",
		"fabricahq/public-rules/tree/main",
		"https://gitlab.com/fabricahq/public-rules",
		"http://github.com/fabricahq/public-rules",
		"https://github.com/fabricahq/public-rules?tab=readme",
		"https://github.com/fabricahq/public-rules#readme",
		"https://user@github.com/fabricahq/public-rules",
		"https://github.com.evil.example/fabricahq/public-rules",
		"fabric_hq/public-rules",
		strings.Repeat("a", 40) + "/rules",
		"fabricahq/" + strings.Repeat("r", 101),
		"fabricahq/..",
		"fabricahq/.",
		"fabricahq/public rules",
		"fabricahq/public%2Frules",
		"../etc/passwd",
	} {
		if owner, name, err := ParseListedRepository(text); !errors.Is(err, ErrInvalidRepository) {
			t.Errorf("%q: got %q, %q, %v, want ErrInvalidRepository", text, owner, name, err)
		}
	}
}

func TestStateOfAListingPrefersVettedThenIngested(t *testing.T) {
	for _, c := range []struct {
		vetted, ingested, failed bool
		want                     ListingState
	}{
		{false, false, false, ListingChecking},
		{false, false, true, ListingFailed},
		{false, true, false, ListingListed},
		{false, true, true, ListingListed},
		{true, false, false, ListingVetted},
		{true, true, true, ListingVetted},
	} {
		if got := StateOf(c.vetted, c.ingested, c.failed); got != c.want {
			t.Errorf("StateOf(%v, %v, %v) = %s, want %s", c.vetted, c.ingested, c.failed, got, c.want)
		}
	}
}

func TestFailureKeepsAShortReasonAndCutsALongOneAtACharacter(t *testing.T) {
	if got := Failure(errors.New("  the repository has no release tags \n")); got != "the repository has no release tags" {
		t.Errorf("got %q", got)
	}
	long := Failure(errors.New(strings.Repeat("é", MaxFailureLength)))
	if len(long) > MaxFailureLength || !utf8.ValidString(long) || !strings.HasSuffix(long, "…") {
		t.Errorf("got %d bytes, valid %v: %q", len(long), utf8.ValidString(long), long[len(long)-10:])
	}
}
