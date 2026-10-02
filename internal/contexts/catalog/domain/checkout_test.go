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

// item returns a cart item of acme/backend unless owner/name says otherwise.
func item(library string, kind CartItemKind, path string) CartItem {
	owner, name, _ := strings.Cut(library, "/")
	return CartItem{Owner: owner, Name: name, Kind: kind, Path: path}
}

// One vetted library, with a group and two rules: one in another group, and one the group imports already, which the
// configuration leaves out.
func TestCheckoutOfOneLibrary(t *testing.T) {
	checkout := NewCheckout([]CheckoutLibrary{{
		Owner: "fabricahq", Name: "public-rules", Release: 1, Vetted: true,
		Items: []CartItem{
			item("fabricahq/public-rules", CartRule, "techs/go/errors-include-useful-diagnostic-data"),
			item("fabricahq/public-rules", CartGroup, "practices/testing"),
			item("fabricahq/public-rules", CartRule, "practices/testing/keep-tests-independent"),
			item("fabricahq/public-rules", CartRule, "techs/go/comment-non-obvious-struct-fields"),
		},
	}})

	assertGolden(t, "checkout-one-library.config.yaml", checkout.Config())
	assertGolden(t, "checkout-one-library.prompt.md", checkout.Prompt())
}

// Two libraries, one whole and one by a group and a rule, and a third that Rulemart doesn't vet, which the prompt
// names and asks the agent to review.
func TestCheckoutOfSeveralLibrariesNamesTheUnvettedOne(t *testing.T) {
	checkout := NewCheckout([]CheckoutLibrary{
		{
			Owner: "fabricahq", Name: "public-rules", Release: 1, Vetted: true,
			Items: []CartItem{
				item("fabricahq/public-rules", CartGroup, "practices/testing"),
				item("fabricahq/public-rules", CartRule, "techs/go/errors-include-useful-diagnostic-data"),
			},
		},
		{
			Owner: "fabricahq", Name: "code-rules-test-library",
			Release: 6, Vetted: true,
			Items: []CartItem{
				item("fabricahq/code-rules-test-library", CartRule, "techs/go/close-bodies"),
				item("fabricahq/code-rules-test-library", CartLibrary, ""),
			},
		},
		{
			Owner: "stranger", Name: "Rules", Release: 3,
			Items: []CartItem{item("stranger/Rules", CartRule, "techs/go/use-go")},
		},
	})

	assertGolden(t, "checkout-several-libraries.config.yaml", checkout.Config())
	assertGolden(t, "checkout-several-libraries.prompt.md", checkout.Prompt())
}

// Each library gets a source name Code Rules accepts, from its repository's name, or its owner's when the repository
// is an organization's .code-rules or has no letters or digits. A name that would start with a digit, or be local,
// which Code Rules reserves, and names several libraries would share, get their owner's too, and a number when even
// that matches.
func TestCheckoutNamesEachSourceDistinctly(t *testing.T) {
	library := func(owner, name string) CheckoutLibrary {
		return CheckoutLibrary{Owner: owner, Name: name, Release: 1, Vetted: true, Items: []CartItem{item(owner+"/"+name, CartLibrary, "")}}
	}
	checkout := NewCheckout([]CheckoutLibrary{
		library("acme", "rules"),
		library("Beta", "Rules"),
		library("acme", ".code-rules"),
		library("Zeta_Corp", "Go.Rules"),
		library("x", "2fa"),
		library("y", "local"),
		library("a-b", "c"),
		library("a", "b-c"),
		library("q", "_"),
		library("y", "c"),
		library("z", "b-c"),
		library("local", ".code-rules"),
	})
	var names []string
	for _, source := range checkout.Sources {
		names = append(names, source.Library.FullName()+" "+source.Name)
	}
	want := []string{
		"a/b-c a-b-c", "a-b/c a-b-c-2", "acme/.code-rules acme", "acme/rules acme-rules", "Beta/Rules beta-rules",
		"local/.code-rules library-local", "q/_ q", "x/2fa x-2fa", "y/c y-c", "y/local y-local", "z/b-c z-b-c", "Zeta_Corp/Go.Rules go-rules",
	}
	if !slices.Equal(names, want) {
		t.Errorf("got sources\n%q\nwant\n%q", names, want)
	}
}

// The configuration names a library's Git address on GitHub, as GitHub spells its owner and name now.
func TestCheckoutNamesTheLibrarysGitHubAddress(t *testing.T) {
	lib := CheckoutLibrary{Owner: "Acme", Name: "backend.rules"}
	if got, want := lib.Repository(), "https://github.com/Acme/backend.rules.git"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A cart with nothing to check out has no sources, and no prompt.
func TestAnEmptyCheckoutHasNothingToSay(t *testing.T) {
	checkout := NewCheckout(nil)
	if len(checkout.Sources) != 0 || checkout.Config() != "" || checkout.Prompt() != "" {
		t.Errorf("got %+v, config %q, prompt %q", checkout.Sources, checkout.Config(), checkout.Prompt())
	}
}

// Two repositories with the same name, of different owners, each get their owner's name too, in owner order.
func TestCheckoutNamesSameNamedRepositoriesByOwner(t *testing.T) {
	library := func(owner string) CheckoutLibrary {
		return CheckoutLibrary{Owner: owner, Name: "engineering-rules", Release: 1, Vetted: true, Items: []CartItem{item(owner+"/engineering-rules", CartLibrary, "")}}
	}
	checkout := NewCheckout([]CheckoutLibrary{library("zeta"), library("Acme")})
	var names []string
	for _, source := range checkout.Sources {
		names = append(names, source.Name)
	}
	if want := []string{"acme-engineering-rules", "zeta-engineering-rules"}; !slices.Equal(names, want) {
		t.Errorf("got %q, want %q", names, want)
	}
	if config := checkout.Config(); strings.Count(config, "repository: https://github.com/") != 2 {
		t.Errorf("the configuration doesn't import both:\n%s", config)
	}
}
