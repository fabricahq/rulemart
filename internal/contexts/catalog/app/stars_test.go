package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// Stars stars only a current rule of what the release vets, its library named as owner/name, and refuses any other
// text as no such rule, without starring anything. The starred list shows each rule's group as the canonical list
// does.
func TestStarsStarOnlyARuleOfAVettedLibraryNamedAsOwnerAndName(t *testing.T) {
	l := newListing(t, firstRelease(t))
	if _, err := l.ingester.Ingest(context.Background(), "https://github.com/example/rules"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	groups, err := domain.NewCanonicalGroups([]coderules.CanonicalGroup{{ID: "techs/go", Name: "Go"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The listings' store is the web function's, which keeps stars too.
	unvetted := app.Stars{Store: l.listings.Store.(store.Stars), Groups: groups}
	vetted := app.Stars{Store: unvetted.Store, Vetted: []domain.LibraryKey{listingKey}, Groups: groups}

	if err := unvetted.Star(ctx, l.account, "example/rules", returnErrors); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("starring a rule of a library the release doesn't vet: %v, want app.ErrNotFound", err)
	}
	for _, name := range [][2]string{
		{"", returnErrors}, {"example", returnErrors}, {"example/", returnErrors}, {"/rules", returnErrors},
		{"example/rules/more", returnErrors}, {"https://github.com/example/rules", returnErrors},
		{"example/rules\x00", returnErrors}, {"example\xff/rules", returnErrors},
		{"example/rules", ""}, {"example/rules", "techs/go/return-errors\x00"}, {"example/rules", "techs/go/\xff"},
		{"example/rules", "techs/go/missing"},
	} {
		if err := vetted.Star(ctx, l.account, name[0], name[1]); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("star %q in %q: %v, want app.ErrNotFound", name[1], name[0], err)
		}
		if err := vetted.Unstar(ctx, l.account, name[0], name[1]); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("unstar %q in %q: %v, want app.ErrNotFound", name[1], name[0], err)
		}
	}
	if stars, err := vetted.AccountStars(ctx, l.account); err != nil || len(stars) != 0 {
		t.Fatalf("got %+v, %v; want no stars", stars, err)
	}

	if err := vetted.Star(ctx, l.account, "Example/Rules", returnErrors); err != nil {
		t.Fatal(err)
	}
	stars, err := vetted.AccountStars(ctx, l.account)
	if err != nil || len(stars) != 1 || stars[0].Library.FullName() != "example/rules" || stars[0].Rule.Path != returnErrors ||
		stars[0].Rule.Stars != 1 || stars[0].CanonicalGroup == nil || stars[0].CanonicalGroup.Name != "Go" {
		t.Fatalf("got %+v, %v; want example/rules's return-errors, with one star, in the canonical Go group", stars, err)
	}
	if starred, err := vetted.Starred(ctx, l.account, "example", "rules", returnErrors); err != nil || !starred {
		t.Fatalf("starred %v, %v", starred, err)
	}
	if err := vetted.Unstar(ctx, l.account, "example/rules", returnErrors); err != nil {
		t.Fatal(err)
	}
	if starred, err := vetted.Starred(ctx, l.account, "example", "rules", returnErrors); err != nil || starred {
		t.Fatalf("starred %v, %v after unstarring", starred, err)
	}
}
