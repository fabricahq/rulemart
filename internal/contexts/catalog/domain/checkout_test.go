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
	assertGolden(t, "checkout-"+name+".commands.txt", stepsText(checkout.Commands()))
	assertGolden(t, "checkout-"+name+".prompt.txt", checkout.Prompt())
}

// stepsText returns steps as one text, as a golden file keeps them: each step's heading, when it has one, on a line of
// its own in brackets, then its commands, with a blank line between steps.
func stepsText(steps []CommandStep) string {
	var texts []string
	for _, step := range steps {
		text := step.Commands
		if step.Heading != "" {
			text = "[" + step.Heading + "]\n" + text
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n\n")
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

// An unvetted library's rule is never forked, even when the visitor chose to: a fork copies from the release a tag
// names, which its publisher could move after review, so it stays a rule synced from the source pinned to the
// reviewed commit, and the texts never copy it.
func TestCheckoutSyncsAnUnvettedLibrarysForkFromTheReviewedCommit(t *testing.T) {
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{{
		Owner: "stranger", Name: "rules", Release: 3, Commit: strings.Repeat("3", 40),
		Groups: []CheckoutGroup{testingGroup},
		Forks:  []CheckoutRule{checkoutRule(goGroup, "use-go"), checkoutRule(testingGroup, "name-tests")},
	}})

	commands, prompt := stepsText(checkout.Commands()), checkout.Prompt()

	if strings.Contains(commands, "add rule") || strings.Contains(prompt, "forked") {
		t.Errorf("the texts fork an unvetted library's rule:\n%s\n\n%s", commands, prompt)
	}
	want := "code-rules project add library stranger \\\n  --repository https://github.com/stranger/rules.git \\\n" +
		"  --ref " + strings.Repeat("3", 40) + " \\\n  --groups practices/testing \\\n  --rules techs/go/use-go\n\ncode-rules project sync"
	if !strings.Contains(commands, want) {
		t.Errorf("got\n%s\nwant the rule synced from the reviewed commit, and the one its group brings left out:\n%s", commands, want)
	}
}

// Commands that import an unvetted library come in two steps, so no one pastes the import with the review: first the
// commands that fetch each unvetted library for review, which end by saying to stop if a rule is unsafe, and only then
// the commands that import, which fetch nothing for review. Commands that import only vetted libraries are one step.
func TestCheckoutCommandsReviewAnUnvettedLibraryInAStepBeforeTheImport(t *testing.T) {
	vetted := CheckoutLibrary{Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1, Groups: []CheckoutGroup{goGroup}}
	unvetted := func(owner string) CheckoutLibrary {
		return CheckoutLibrary{Owner: owner, Name: "rules", Release: 3, Commit: strings.Repeat("3", 40), Rules: []CheckoutRule{checkoutRule(goGroup, "use-go")}}
	}

	steps := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{vetted, unvetted("stranger"), unvetted("other")}).Commands()

	if len(steps) != 2 || steps[0].Heading != "Fetch and review" || steps[1].Heading != "After you've reviewed, import" {
		t.Fatalf("got the steps\n%s\nwant Fetch and review, then the import", stepsText(steps))
	}
	review, imports := steps[0].Commands, steps[1].Commands
	for _, library := range []string{"stranger", "other"} {
		fetch := "git -C \"$d\" fetch -q --depth 1 https://github.com/" + library + "/rules.git " + strings.Repeat("3", 40)
		if !strings.Contains(review, "\n"+`d="$(mktemp -d)" && git -C "$d" init -q && `+fetch) {
			t.Errorf("the review doesn't fetch %s/rules as a command of its own:\n%s", library, review)
		}
	}
	if !strings.HasSuffix(review, "stop: don't run the next step.") || strings.Contains(review, "code-rules") {
		t.Errorf("the review should only fetch, and end by saying to stop on an unsafe rule:\n%s", review)
	}
	if strings.Contains(imports, "mktemp") || !strings.Contains(imports, "add library stranger") || !strings.Contains(imports, "add library fabrica") {
		t.Errorf("the import should add every library and fetch nothing for review:\n%s", imports)
	}

	if steps := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{vetted}).Commands(); len(steps) != 1 || steps[0].Heading != "" {
		t.Errorf("got the steps\n%s\nwant one without a heading for vetted libraries alone", stepsText(steps))
	}
}

// A new project, with no repository named: also adding the other rules of a rule's group imports the group whole, so a
// fork of one of its rules needs a reason, and a group that isn't canonical is named by its ID.
func TestCheckoutForANewProjectAddsTheRestOfTheGroups(t *testing.T) {
	golang := CheckoutGroup{ID: "techs/golang", Name: "techs/golang"}
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectNew}, []CheckoutLibrary{{
		Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1, RestOfGroups: true,
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
// them makes the guidance, which agents then read.
func TestCheckoutOfForksAloneBuildsWithoutSyncing(t *testing.T) {
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{{
		Owner: "fabricahq", Name: "public-rules", Vetted: true, Release: 1,
		Forks: []CheckoutRule{checkoutRule(goGroup, "return-errors")},
	}})

	got := stepsText(checkout.Commands())

	want := "code-rules project add rule techs/go/return-errors \\\n  --from https://github.com/fabricahq/public-rules.git@1.0.0\n\ncode-rules project build\n\n" +
		"# Then make sure AGENTS.md tells agents to read .code-rules/generated/RULES.md"
	if !strings.HasSuffix(got, want) || strings.Contains(got, "project sync") {
		t.Errorf("got\n%s\nwant it to end with\n%s\nand never sync", got, want)
	}
}

func TestCheckoutWithNothingToImportWritesNothing(t *testing.T) {
	checkout := NewCheckout(CheckoutTarget{Mode: ProjectUnknown}, []CheckoutLibrary{{Owner: "a", Name: "b", Vetted: true}})

	if len(checkout.sources) != 0 || checkout.Commands() != nil || checkout.Prompt() != "" {
		t.Errorf("got %d sources, commands %q, prompt %q, want none", len(checkout.sources), checkout.Commands(), checkout.Prompt())
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
	for _, source := range checkout.sources {
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
