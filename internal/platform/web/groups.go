// A group's page, and how it shapes what it reads into what it shows: its rules across libraries, one ranked list
// beside the filter sidebar.

package web

import (
	"errors"
	"net/http"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/app"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/views"
)

// groupPageView is what a group's page shows.
type groupPageView struct {
	// href is the page's own address, without choices, and address the address asked for, in its own spelling, whose
	// choices may include one the visitor can't make.
	href, address string
	kind          groupKind
	label         groupLabel
	icon          groupIcon
	description   string
	list          ruleListView
	rows          []ruleRowView
}

// newGroupPageView returns what the page of the group at address shows of page, read with choices; offersMine offers
// My libraries.
func newGroupPageView(page views.GroupPage, address string, choices domain.ListChoices, offersMine bool, iconURL func(file string) string) groupPageView {
	v := groupPageView{
		href: groupHref(page.Path), address: address, kind: kindOf(page.Path), label: newGroupLabel(page.Path, page.Canonical),
		icon: newGroupIcon(page.Canonical, iconURL),
	}
	if page.Canonical != nil {
		v.description = page.Canonical.Description
	}
	v.list = newRuleListView(domain.GroupListPage, v.href, nil, choices, offersMine, page.Rules)
	for _, r := range page.Rules.Rows {
		v.rows = append(v.rows, newListedRuleRow(r))
	}
	return v
}

// indexed reports whether search engines may index the page: only at its own address, with no choice in it.
func (v groupPageView) indexed() bool { return v.address == v.href }

// groupAddress is the address of the page of the group whose ID is path, with choices, in the one spelling the page's
// own address has.
func groupAddress(path string, choices domain.ListChoices) string {
	return addressOf(groupHref(path), choices.Values(domain.GroupListPage), nil)
}

// truncated reports whether more rules pass the filters than the page lists.
func (v groupPageView) truncated() bool { return v.list.total > len(v.rows) }

// title is the group's page's document title.
func (v groupPageView) title() string { return v.label.display() + " rules · Rulemart" }

// summary describes the group to search engines: its description, or else what it holds.
func (v groupPageView) summary() string {
	if v.description != "" {
		return v.description
	}
	return v.label.id + ": rules for coding agents from the Code Rules libraries that chose this group, on Rulemart."
}

// group shows the page of the group of kind that the path names, with the choices its address holds that the visitor
// can make. It redirects another spelling of a canonical group's name, or of the choices, to the page's own address.
func (s *server) group(kind groupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := string(kind) + "/" + r.PathValue("name")
		asked := domain.ParseListChoices(domain.GroupListPage, r.URL.Query())
		choices, mine, ok := s.myLibraries(w, r, asked)
		if !ok {
			return
		}
		page, err := s.catalog.GroupPage(r.Context(), id, choices, mine)
		if errors.Is(err, app.ErrNotFound) {
			s.notFound(w, r)
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		own := groupAddress(page.Path, asked)
		if page.Path != id || !spelledAs(r, own) {
			redirect(w, r, own)
			return
		}
		view := newGroupPageView(page, own, choices, visitorOf(r.Context()).account != nil, s.assets.iconURL)
		s.render(w, r, http.StatusOK, groupPage(s.pageChrome(view.href), view))
	}
}
