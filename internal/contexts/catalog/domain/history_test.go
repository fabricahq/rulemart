package domain

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fabricahq/rulemart/internal/lib/coderules"
)

// publishA publishes rule a.
const publishA = `formatVersion: 1
release: 1
rules: {techs/go/a: 1.0.0}
changes: {techs/go/a: {change: new, summaries: [Add the rule.]}}
`

func TestBuildHistoryRejectsRecordsThatDontFollowFromTheOneBefore(t *testing.T) {
	for name, tc := range map[string]struct {
		records []string
		want    string
	}{
		"a gap in release numbers": {
			records: []string{publishA, "formatVersion: 1\nrelease: 3\nrules: {techs/go/a: 1.0.0}\n"},
			want:    "release/3: expected release/2 next",
		},
		"a version change without a change entry": {
			records: []string{publishA, "formatVersion: 1\nrelease: 2\nrules: {techs/go/a: 1.1.0}\n"},
			want:    "release/2.rules.techs/go/a: the rule moves from 1.0.0 to 1.1.0 without a change",
		},
		"a rule that appears without a change entry": {
			records: []string{publishA, "formatVersion: 1\nrelease: 2\nrules: {techs/go/a: 1.0.0, techs/go/c: 1.0.0}\n"},
			want:    "release/2.rules.techs/go/c: the rule appears without a change",
		},
		"a new rule that was already listed": {
			records: []string{publishA, `formatVersion: 1
release: 2
rules: {techs/go/a: 1.0.0}
changes: {techs/go/a: {change: new, summaries: [Add it again.]}}
`},
			want: "release/2.changes.techs/go/a: the rule is new, but release/1 already listed it at 1.0.0",
		},
		"a retired ID reused": {
			records: []string{publishA, `formatVersion: 1
release: 2
rules: {}
retired: {techs/go/a: {lastVersion: 1.0.0, summaries: [Retire it.]}}
`, `formatVersion: 1
release: 3
rules: {techs/go/a: 1.0.0}
changes: {techs/go/a: {change: new, summaries: [Bring it back.]}}
`},
			want: "release/3.changes.techs/go/a: the rule is new, but release/2 retired that ID",
		},
		"a retirement of the wrong version": {
			records: []string{publishA, `formatVersion: 1
release: 2
rules: {}
retired: {techs/go/a: {lastVersion: 1.2.0, summaries: [Retire it.]}}
`},
			want: "release/2.retired.techs/go/a.lastVersion: the rule's last version is 1.2.0, but release/1 listed it at 1.0.0",
		},
		"a retirement of a rule that wasn't listed": {
			records: []string{publishA, `formatVersion: 1
release: 2
rules: {techs/go/a: 1.0.0}
retired: {techs/go/z: {lastVersion: 1.0.0, summaries: [Retire it.]}}
`},
			want: "release/2.retired.techs/go/z: release/1 didn't list the rule",
		},
	} {
		t.Run(name, func(t *testing.T) {
			records := make([]coderules.ReleaseRecord, len(tc.records))
			for i, text := range tc.records {
				record, err := coderules.ParseReleaseRecord([]byte(text), "release/"+strconv.Itoa(i+1))
				if err != nil {
					t.Fatalf("record %d: %v", i+1, err)
				}
				records[i] = record
			}

			_, err := buildHistory(records)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want one containing %q", err, tc.want)
			}
		})
	}
}
