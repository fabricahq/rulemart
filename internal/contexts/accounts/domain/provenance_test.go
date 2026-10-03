package domain

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Rulemart's own provenance file, as Code Rules 0.2.0 wrote it, names one source from GitHub, its release and groups,
// and every rule it holds at version 1.0.0.
func TestParseProvenanceReadsAFileCodeRulesWrote(t *testing.T) {
	data, err := os.ReadFile("testdata/rulemart-provenance.json")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := ParseProvenance(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d sources", len(sources))
	}
	s := sources[0]
	if s.Name != "fabrica" || s.Library != "fabricahq/public-rules" || s.Release != 1 || !slices.Contains(s.Groups, "techs/go") {
		t.Errorf("the source is %+v", s)
	}
	want := PinnedRule{Path: "practices/code-design/express-operations-as-meaningful-steps", Version: coderules.FirstRuleVersion}
	if len(s.Rules) < 10 || !slices.Contains(s.Rules, want) {
		t.Errorf("the source's rules are %+v", s.Rules)
	}
}

// A fork's rule, a local rule, and a rule whose ID or version Code Rules wouldn't write hold no library's version, so
// update counts leave them out; a source elsewhere than GitHub keeps no library.
func TestParseProvenanceKeepsOnlyRulesAtALibrarysPublishedVersion(t *testing.T) {
	sources, err := ParseProvenance([]byte(`{
		"sources": [
			{"name": "fabrica", "repository": "https://github.com/fabricahq/public-rules", "release": 3, "groups": ["techs/go"]},
			{"name": "elsewhere", "repository": "https://gitlab.com/acme/rules.git", "groups": []},
			{"name": "fabrica", "repository": "https://github.com/someone/else.git"}
		],
		"rules": [
			{"id": "fabrica:techs/go/return-errors", "origin": {"source": "fabrica", "version": "1.2.0"}},
			{"id": "local:techs/go/return-errors", "origin": {"source": "local", "version": null}},
			{"id": "fabrica:techs/go/forked", "origin": {"source": "fabrica", "version": null}},
			{"id": "fabrica:techs/go/odd", "origin": {"source": "fabrica", "version": "v1"}},
			{"id": "fabrica:../escape", "origin": {"source": "fabrica", "version": "1.0.0"}},
			{"id": "elsewhere:practices/testing/x", "origin": {"source": "elsewhere", "version": "2.0.0"}}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("got %+v, want the two sources once each", sources)
	}
	elsewhere, fabrica := sources[0], sources[1]
	if fabrica.Library != "fabricahq/public-rules" || len(fabrica.Rules) != 1 || fabrica.Rules[0].Path != "techs/go/return-errors" {
		t.Errorf("fabrica is %+v", fabrica)
	}
	if elsewhere.Library != "" || len(elsewhere.Rules) != 1 {
		t.Errorf("elsewhere is %+v", elsewhere)
	}
}

func TestParseProvenanceRefusesAFileThatIsntOne(t *testing.T) {
	for name, data := range map[string]string{
		"not JSON":   "{",
		"no sources": `{"sources": [], "rules": []}`,
		"too large":  `{"sources": [{"name": "a"}], "pad": "` + strings.Repeat("x", MaxProvenanceBytes) + `"}`,
		"a list":     `[]`,
	} {
		if sources, err := ParseProvenance([]byte(data)); err == nil {
			t.Errorf("%s: parsed %+v", name, sources)
		}
	}
}

// A project's libraries are named once, though two projects or two sources name one, in the order first named.
func TestLibraryNamesNamesEachImportedLibraryOnce(t *testing.T) {
	s := Snapshot{Projects: []Project{
		{Sources: []Source{{Library: "fabricahq/public-rules"}, {Library: ""}}},
		{Sources: []Source{{Library: "acme/rules"}, {Library: "FabricaHQ/Public-Rules"}}},
	}}
	if got := s.LibraryNames(); !slices.Equal(got, []string{"fabricahq/public-rules", "acme/rules"}) {
		t.Errorf("got %v", got)
	}
}

func TestInstallationSettingsAreTheVisitorsOrTheOrganizations(t *testing.T) {
	if got := (Installation{ID: 5, Account: "Mona"}).SettingsURL("mona"); got != "https://github.com/settings/installations/5" {
		t.Errorf("the visitor's own: %s", got)
	}
	if got := (Installation{ID: 6, Account: "octo-org"}).SettingsURL("mona"); got != "https://github.com/organizations/octo-org/settings/installations/6" {
		t.Errorf("an organization's: %s", got)
	}
}
