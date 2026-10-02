package web

import (
	"slices"
	"testing"
)

// A browse page lists its kind's canonical groups as the home page's tiles do: by rule count, and groups with as many
// by name, whatever order the catalog gives them in. Groups that aren't canonical stay off it.
func TestBrowseViewListsCanonicalGroupsByRuleCountThenName(t *testing.T) {
	named := func(id, name string, rules int) groupSummaryView {
		return groupSummaryView{label: groupLabel{id: id, name: name, canonical: true}, rules: rules}
	}
	index := groupIndexView{techs: []groupSummaryView{
		named("techs/zustand", "Zustand", 10), named("techs/go", "Go", 9), named("techs/react", "React", 39),
		named("techs/tanstack-router", "TanStack Router", 10), {label: groupLabel{id: "techs/golang"}, rules: 50},
	}}

	v := newBrowseView(techsKind, index)

	if want := []string{"techs/react", "techs/tanstack-router", "techs/zustand", "techs/go"}; !slices.Equal(tileIDs(v.groups), want) {
		t.Errorf("groups = %q, want %q", tileIDs(v.groups), want)
	}
	if v.others != 1 {
		t.Errorf("others = %d, want 1", v.others)
	}
}
