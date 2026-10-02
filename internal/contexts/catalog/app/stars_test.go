package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/store"
)

// Stars stars only what the release vets, named as owner/name, and refuses any other text as no such library,
// without starring anything.
func TestStarsStarOnlyAVettedLibraryNamedAsOwnerAndName(t *testing.T) {
	l := newListing(t, firstRelease(t))
	if _, err := l.ingester.Ingest(context.Background(), "https://github.com/example/rules"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// The listings' store is the web function's, which keeps stars too.
	unvetted := app.Stars{Store: l.listings.Store.(store.Stars)}
	vetted := app.Stars{Store: unvetted.Store, Vetted: []domain.LibraryKey{listingKey}}

	if _, err := unvetted.Star(ctx, l.account, "example/rules"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("starring a library the release doesn't vet: %v, want app.ErrNotFound", err)
	}
	for _, text := range []string{"", "example", "example/", "/rules", "example/rules/more", "https://github.com/example/rules", "example/rules\x00", "example\xff/rules"} {
		if _, err := vetted.Star(ctx, l.account, text); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("star %q: %v, want app.ErrNotFound", text, err)
		}
		if err := vetted.Unstar(ctx, l.account, text); !errors.Is(err, app.ErrNotFound) {
			t.Errorf("unstar %q: %v, want app.ErrNotFound", text, err)
		}
	}
	if stars, err := vetted.AccountStars(ctx, l.account); err != nil || len(stars) != 0 {
		t.Fatalf("got %+v, %v; want no stars", stars, err)
	}

	ref, err := vetted.Star(ctx, l.account, "Example/Rules")
	if err != nil || ref.FullName() != "example/rules" {
		t.Fatalf("got %+v, %v; want example/rules", ref, err)
	}
	if starred, err := vetted.Starred(ctx, l.account, "example", "rules"); err != nil || !starred {
		t.Fatalf("starred %v, %v", starred, err)
	}
	if err := vetted.Unstar(ctx, l.account, "example/rules"); err != nil {
		t.Fatal(err)
	}
	if starred, err := vetted.Starred(ctx, l.account, "example", "rules"); err != nil || starred {
		t.Fatalf("starred %v, %v after unstarring", starred, err)
	}
}
