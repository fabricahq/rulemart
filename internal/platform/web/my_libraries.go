// The rows of the dashboard's My libraries: the libraries the visitor and their organizations publish, and those the
// visitor listed for others, each with its state, and what the visitor can do with their own listing of it.

package web

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// libraryState is the state a row of My libraries shows, as a chip.
type libraryState int

const (
	libraryVetted libraryState = iota
	libraryUnvetted
	libraryChecking
	libraryFailed
)

// myLibraryView is a row of My libraries: a library on Rulemart, or one the visitor's listing is still adding.
type myLibraryView struct {
	// href is the library's page, or, while the visitor's listing is checked or after its check failed, the page that
	// follows the check. nofollow is true for a library Rulemart doesn't vet.
	href, owner, name string
	nofollow, isNew   bool
	state             libraryState
	// stars is the library's star total, and hasStars true for a row the catalog describes, which shows it.
	stars    int
	hasStars bool
	// detail follows the repository on the row's second line: its rule count, when it was listed, how its check is
	// going, or why it failed.
	detail string
	// retry is where Try again posts, for a listing of the visitor's whose last check failed, and removeConfirm the
	// page that confirms removing the visitor's listing of it; both are empty for a library the visitor didn't list.
	retry, removeConfirm string
}

// fullName is the row's repository, as owner/name.
func (m myLibraryView) fullName() string { return m.owner + "/" + m.name }

// newMyLibraries returns My libraries' two groups as of now: mine, the libraries owners, the visitor and their
// organizations, publish, and others, the libraries the visitor listed for anyone else. A listing of the visitor's
// joins the row of the library it names, in mine, and otherwise takes a row of its own: in mine for a repository an
// owner holds, and in others for any other. mine lists owned's libraries first, as owned does, then the visitor's other
// listings, newest first, as listings does.
func newMyLibraries(owned []views.OwnedLibrary, listings []views.AccountListing, owners []string, now time.Time) (mine, others []myLibraryView) {
	byName := map[string]views.AccountListing{}
	for _, l := range listings {
		byName[strings.ToLower(listingName(l))] = l
	}
	for _, o := range owned {
		row := myLibraryView{
			href: libraryHref(o.Library.Owner, o.Library.Name), owner: o.Library.Owner, name: o.Library.Name,
			nofollow: !o.Vetted, isNew: now.Sub(o.AddedAt) < newWithin, state: libraryUnvetted,
			stars: o.Stars, hasStars: true, detail: plural(o.Rules, "rule", "rules"),
		}
		if o.Vetted {
			row.state = libraryVetted
		}
		key := strings.ToLower(o.Library.FullName())
		if l, ok := byName[key]; ok {
			row.retry, row.removeConfirm = listingActions(l)
			if l.Failure != "" {
				row.detail += " · The last check failed: " + l.Failure
			}
			delete(byName, key)
		}
		mine = append(mine, row)
	}
	for _, l := range listings {
		if _, ok := byName[strings.ToLower(listingName(l))]; !ok {
			continue
		}
		row := newListingRow(l, now)
		if slices.ContainsFunc(owners, func(owner string) bool { return strings.EqualFold(owner, row.owner) }) {
			mine = append(mine, row)
		} else {
			others = append(others, row)
		}
	}
	return mine, others
}

// listingName is the repository a listing names: its library's, as the code host spells it now, once the catalog
// stores it, and otherwise as its lister gave it.
func listingName(l views.AccountListing) string {
	if l.Library.Owner != "" {
		return l.Library.FullName()
	}
	return l.Owner + "/" + l.Name
}

// newListingRow is the row of a listing of the visitor's that no library row of theirs holds, as of now.
func newListingRow(l views.AccountListing, now time.Time) myLibraryView {
	row := myLibraryView{owner: l.Owner, name: l.Name}
	if l.Library.Owner != "" {
		row.owner, row.name = l.Library.Owner, l.Library.Name
	}
	row.retry, row.removeConfirm = listingActions(l)
	switch l.State {
	case domain.ListingVetted, domain.ListingListed:
		row.href, row.nofollow, row.state = libraryHref(row.owner, row.name), l.State == domain.ListingListed, libraryUnvetted
		if l.State == domain.ListingVetted {
			row.state = libraryVetted
		}
		row.detail = "Listed " + date(l.ListedAt)
		if l.Failure != "" {
			row.detail += " · The last check failed: " + l.Failure
		}
	case domain.ListingFailed:
		row.href, row.state, row.detail = runPageHref(row.fullName()), libraryFailed, l.Failure
	default:
		row.href, row.state = runPageHref(row.fullName()), libraryChecking
		if now.Sub(l.RequestedAt) > checkingLonger {
			row.detail = "Taking longer than usual: Rulemart checks it again within the hour. You asked " + moment(l.RequestedAt, now) + "."
		} else {
			row.detail = "Rulemart is checking it on GitHub. You asked " + moment(l.RequestedAt, now) + "."
		}
	}
	return row
}

// listingActions returns where Try again posts for the listing l, empty unless its last check failed, and the page
// that confirms removing it.
func listingActions(l views.AccountListing) (retry, removeConfirm string) {
	query := "?" + url.Values{"listing": {strconv.FormatInt(l.ID, 10)}}.Encode()
	if l.Failure != "" {
		retry = retryListingHref + query
	}
	return retry, removeListingHref + query
}
