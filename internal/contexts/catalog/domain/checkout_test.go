package domain

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// update rewrites the golden files from what checkout writes now: go test ./internal/contexts/catalog/domain -update.
var update = flag.Bool("update", false, "rewrite the golden files")

// assertGolden compares got with testdata/name, or writes it there with -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	file := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s (run with -update to write it): %v", file, err)
	}
	if got != string(want) {
		t.Errorf("%s differs; run with -update and review the diff.\ngot:\n%s", file, got)
	}
}

// assertCheckout compares the checkout's commands and prompt with the golden files named for it.
func assertCheckout(t *testing.T, name string, checkout Checkout) {
	t.Helper()
	assertGolden(t, "checkout-"+name+".commands.txt", checkout.Commands())
	assertGolden(t, "checkout-"+name+".prompt.txt", checkout.Prompt())
}

var (
	goGroup      = CheckoutGroup{ID: "techs/go", Name: "Go"}
	testingGroup = CheckoutGroup{ID: "practices/testing", Name: "Testing"}
)

// checkoutRule returns a rule of group at version 1.0.0, named by its slug.
func checkoutRule(group CheckoutGroup, slug string) CheckoutRule {
	return CheckoutRule{ID: group.ID + "/" + slug, Group: group, Version: "1.0.0"}
}

// Two vetted libraries of one owner, whose aliases would collide, and an unvetted one, for a project Rulemart knows
// nothing of: a whole group with a fork of one of its rules, which needs a reason, single rules, one of them in the
// whole group and so left out, and the unvetted library pinned to the commit Rulemart saw and named for review.
func TestCheckoutForAProjectRulemartDoesNotKnow(t *testing.T) {
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectUnknown, Repository: "acme/api"}, []CheckoutLibrary{
		{
			Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1,
			Groups: []CheckoutGroup{goGroup},
			Rules:  []CheckoutRule{checkoutRule(testingGroup, "keep-tests-independent"), checkoutRule(goGroup, "return-errors")},
			Forks:  []CheckoutRule{checkoutRule(goGroup, "errors-include-useful-diagnostic-data")},
		},
		{
			Owner: "fabricahq", Name: "code-rules-test-library", Vetted: true, Release: 6,
			Rules: []CheckoutRule{checkoutRule(goGroup, "close-bodies")},
		},
		{
			Owner: "Stranger-HQ", Name: "Rules", Release: 3, Commit: strings.Repeat("3", 40),
			Rules: []CheckoutRule{checkoutRule(goGroup, "use-go")},
		},
	})

	assertCheckout(t, "unknown-project", checkout)
}

// A new project, with no repository named: also adding the other rules of a rule's group imports the group whole, so a
// fork of one of its rules needs a reason, and a group that isn't canonical is named by its ID.
func TestCheckoutForANewProjectAddsTheRestOfTheGroups(t *testing.T) {
	golang := CheckoutGroup{ID: "techs/golang", Name: "techs/golang"}
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectNew}, []CheckoutLibrary{{
		Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1, Full: true,
		Groups: []CheckoutGroup{golang},
		Rules:  []CheckoutRule{checkoutRule(testingGroup, "keep-tests-independent"), checkoutRule(testingGroup, "name-tests")},
		Forks:  []CheckoutRule{checkoutRule(testingGroup, "cover-boundary-cases"), checkoutRule(goGroup, "return-errors")},
	}})

	assertCheckout(t, "new-project", checkout)
}

// A known project that imports one library already under its own name: its groups and rules go in that source's
// configuration, a fork copies from it after a sync, and a library with only a fork copies from its address, then
// the guidance is built.
func TestCheckoutForAKnownProjectAddsToItsSources(t *testing.T) {
	target := CheckoutTarget{Mode: ProjectKnown, Repository: "acme/api", Sources: map[string]string{
		"fabricahq/public-rules": "team", "other/lib": "fabrica",
	}}
	checkout := NewCheckout(target, []CheckoutLibrary{
		{
			Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1,
			Groups: []CheckoutGroup{testingGroup},
			Rules:  []CheckoutRule{checkoutRule(goGroup, "return-errors")},
			Forks:  []CheckoutRule{checkoutRule(testingGroup, "keep-tests-independent")},
		},
		{
			Owner: "fabricahq", Name: "code-rules-test-library", Vetted: true, Release: 6,
			Forks: []CheckoutRule{checkoutRule(goGroup, "close-bodies")},
		},
	})

	assertCheckout(t, "known-project", checkout)
}

// Forks alone, from a library the project doesn't import, copy from its address and need no sync: the build after
// them makes the guidance.
func TestCheckoutOfForksAloneBuildsWithoutSyncing(t *testing.T) {
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{{
		Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1,
		Forks: []CheckoutRule{checkoutRule(goGroup, "return-errors")},
	}})

	got := checkout.Commands()

	want := "code-rules project add rule techs/go/return-errors \\\n  --from https://github.com/fabricahq/public-rules.git@1.0.0\n\ncode-rules project build"
	if !strings.HasSuffix(got, want) || strings.Contains(got, "project sync") {
		t.Errorf("got\n%s\nwant it to end with\n%s\nand never sync", got, want)
	}
}

func TestCheckoutWithNothingToImportWritesNothing(t *testing.T) {
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{{Owner: "a", Name: "b", Vetted: true}})

	if len(checkout.Sources) != 0 || checkout.Commands() != "" || checkout.Prompt() != "" {
		t.Errorf("got %d sources, commands %q, prompt %q, want none", len(checkout.Sources), checkout.Commands(), checkout.Prompt())
	}
}

// Each library gets an alias Code Rules accepts, its owner's name without a trailing hq, distinct from every other
// source and from the known project's own: libraries of one owner add their repository's name, and a name Code Rules
// can't take starts with library-.
func TestCheckoutNamesEachSourceDistinctly(t *testing.T) {
	library := func(owner, name string) CheckoutLibrary {
		return CheckoutLibrary{Owner: owner, Name: name, Vetted: true, Release: 1, Groups: []CheckoutGroup{goGroup}}
	}
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectKnown, Sources: map[string]string{"x/y": "taken"}}, []CheckoutLibrary{
		library("fabricahq", "public-rules"),
		library("FabricaHQ", "Code.Rules"),
		library("Acme-Corp", "rules"),
		library("2fa", "rules"),
		library("local", "rules"),
		library("hq", ".code-rules"),
		library("taken", "rules"),
		library("x", "y"),
		library("solo", "a"),
	})
	var names []string
	for _, source := range checkout.Sources {
		names = append(names, source.FullName()+" "+source.Alias)
	}
	want := []string{
		"fabricahq/public-rules fabrica-public-rules", "FabricaHQ/Code.Rules fabrica-code-rules", "Acme-Corp/rules acme-corp",
		"2fa/rules library-2fa", "local/rules library-local", "hq/.code-rules library", "taken/rules taken-rules",
		"x/y taken", "solo/a solo",
	}
	if !slices.Equal(names, want) {
		t.Errorf("got sources\n%q\nwant\n%q", names, want)
	}
}

// The Commands tab's footnote names the latest release of the first vetted library a project would follow, one it
// imports groups or rules from; an unvetted library is pinned already, and a library only forked from isn't followed.
func TestCheckoutPinExampleIsTheFirstVettedLibraryItImportsFrom(t *testing.T) {
	forked := CheckoutLibrary{Owner: "fabricahq", Name: "forked", Vetted: true, Release: 2, Forks: []CheckoutRule{checkoutRule(goGroup, "a")}}
	unvetted := CheckoutLibrary{Owner: "stranger", Name: "rules", Release: 3, Rules: []CheckoutRule{checkoutRule(goGroup, "b")}}
	followed := CheckoutLibrary{Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 6, Groups: []CheckoutGroup{testingGroup}}

	pin, ok := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{forked, unvetted, followed}).PinExample()

	if !ok || pin != (ReleasePin{Library: "fabricahq/public-rules", Release: 6}) || pin.Option() != "--ref release/6" {
		t.Errorf("got %+v, %t, option %q, want fabricahq/public-rules at release 6", pin, ok, pin.Option())
	}
	if pin, ok := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{forked, unvetted}).PinExample(); ok {
		t.Errorf("got %+v, want none without a vetted library it imports from", pin)
	}
}
