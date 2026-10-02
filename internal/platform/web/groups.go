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
	// href is the page's own address, without choices.
	href        string
	kind        groupKind
	label       groupLabel
	icon        groupIcon
	description string
	list        ruleListView
	rows        []ruleRowView
}

func newGroupPageView(page views.GroupPage, choices domain.ListChoices, iconURL func(file string) string) groupPageView {
	v := groupPageView{
		href: groupHref(page.Path), kind: kindOf(page.Path), label: newGroupLabel(page.Path, page.Canonical),
		icon: newGroupIcon(page.Canonical, iconURL),
	}
	if page.Canonical != nil {
		v.description = page.Canonical.Description
	}
	v.list = newRuleListView(domain.GroupListPage, v.href, nil, choices, page.Rules)
	for _, r := range page.Rules.Rows {
		v.rows = append(v.rows, newListedRuleRow(r))
	}
	return v
}

// indexed reports whether search engines may index the page: only at its own address, with no choice in it.
func (v groupPageView) indexed() bool { return v.list.href(v.list.choices) == v.href }

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

// group shows the page of the group of kind that the path names, with the choices its address holds. It redirects
// another spelling of a canonical group's name, or of the choices, to the page's own address.
func (s *server) group(kind groupKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := string(kind) + "/" + r.PathValue("name")
		choices := domain.ParseListChoices(domain.GroupListPage, r.URL.Query())
		page, err := s.catalog.GroupPage(r.Context(), id, choices)
		if errors.Is(err, app.ErrNotFound) {
			s.notFound(w, r)
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		view := newGroupPageView(page, choices, s.assets.iconURL)
		if page.Path != id || !spelledAs(r, view.list.href(choices)) {
			redirect(w, r, view.list.href(choices))
			return
		}
		s.render(w, r, http.StatusOK, groupPage(s.pageChrome(view.href), view))
	}
}
