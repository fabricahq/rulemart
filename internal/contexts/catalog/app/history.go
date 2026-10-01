// Build each rule's version history from a library's release records, checking that the records agree.

package app

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// buildHistory returns every rule that records publish, with its history but no group or content, in rule ID
// order. records are a library's
// release records in release order, and must be numbered from 1 without gaps.
//
// Each record must follow from the one before it, as Code Rules publishes them: a rule it doesn't change keeps its
// version, a change starts from the rule's previous version, a new rule has none, a retirement retires the rule's
// current version, every rule the previous record listed is either still listed or retired, and a retired ID never
// returns. Errors name the release and field that broke the history.
func buildHistory(records []coderules.ReleaseRecord) ([]domain.Rule, error) {
	histories := map[string]*domain.Rule{}
	previous := map[string]coderules.RuleVersion{}
	for i, record := range records {
		tag := "release/" + strconv.Itoa(record.Release)
		if record.Release != i+1 {
			return nil, fmt.Errorf("%s: expected release/%d next; library releases are numbered from 1 without gaps", tag, i+1)
		}
		if err := followsFrom(record, previous, histories, tag); err != nil {
			return nil, err
		}
		for _, id := range slices.Sorted(maps.Keys(record.Changes)) {
			change := record.Changes[id]
			history := histories[id]
			if history == nil {
				history = &domain.Rule{Path: id}
				histories[id] = history
			}
			history.Versions = append(history.Versions, domain.Version{
				Number: record.Rules[id], Release: record.Release, Change: change.Change, Summaries: change.Summaries,
			})
		}
		for id, retired := range record.Retired {
			histories[id].RetiredIn = record.Release
			histories[id].ReplacedBy = retired.ReplacedBy
			histories[id].RetirementSummaries = retired.Summaries
		}
		previous = record.Rules
	}
	result := make([]domain.Rule, 0, len(histories))
	for _, id := range slices.Sorted(maps.Keys(histories)) {
		result = append(result, *histories[id])
	}
	return result, nil
}

// followsFrom checks that record follows from the rule versions the previous record listed, given the histories
// built so far. Code Rules' parser has already checked that each change leads to the version record lists.
func followsFrom(record coderules.ReleaseRecord, previous map[string]coderules.RuleVersion, histories map[string]*domain.Rule, tag string) error {
	for _, id := range slices.Sorted(maps.Keys(record.Rules)) {
		version := record.Rules[id]
		before, listed := previous[id]
		change, changed := record.Changes[id]
		location := tag + ".rules." + id
		switch {
		case changed && change.Change == coderules.ChangeNew && listed:
			return fmt.Errorf("%s: the rule is new, but release/%d already listed it at %s", tag+".changes."+id, record.Release-1, before)
		case changed && change.Change == coderules.ChangeNew && histories[id] != nil:
			return fmt.Errorf("%s: the rule is new, but release/%d retired that ID, and retired IDs can't be reused", tag+".changes."+id, histories[id].RetiredIn)
		case changed && change.Change != coderules.ChangeNew && !listed:
			return fmt.Errorf("%s: the rule changes from %s, but release/%d didn't list it", tag+".changes."+id, change.From, record.Release-1)
		case changed && change.Change != coderules.ChangeNew && *change.From != before:
			return fmt.Errorf("%s: the rule changes from %s, but release/%d listed it at %s", tag+".changes."+id, change.From, record.Release-1, before)
		case !changed && !listed:
			return fmt.Errorf("%s: the rule appears without a change; a new rule needs a change of new", location)
		case !changed && version != before:
			return fmt.Errorf("%s: the rule moves from %s to %s without a change", location, before, version)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(previous)) {
		retired, isRetired := record.Retired[id]
		_, listed := record.Rules[id]
		switch {
		case !listed && !isRetired:
			return fmt.Errorf("%s: release/%d listed the rule, and this release neither lists nor retires it", tag+".rules."+id, record.Release-1)
		case isRetired && retired.LastVersion != previous[id]:
			return fmt.Errorf("%s: the rule's last version is %s, but release/%d listed it at %s", tag+".retired."+id+".lastVersion", retired.LastVersion, record.Release-1, previous[id])
		}
	}
	for _, id := range slices.Sorted(maps.Keys(record.Retired)) {
		if _, listed := previous[id]; !listed {
			return fmt.Errorf("%s: release/%d didn't list the rule, so it can't be retired", tag+".retired."+id, record.Release-1)
		}
	}
	return nil
}
