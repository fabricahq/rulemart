package domain

import "testing"

func TestCheckpointIsCurrentOnlyWhenTheListedTagsMatchWhatWasStored(t *testing.T) {
	stored := Checkpoint{CloneURL: "https://github.com/example/rules.git", Tags: ReleaseTags{1: "aaa", 2: "bbb"}}
	for name, tc := range map[string]struct {
		checkpoint Checkpoint
		listed     ReleaseTags
		want       bool
	}{
		"the same tags":            {stored, ReleaseTags{1: "aaa", 2: "bbb"}, true},
		"a new release":            {stored, ReleaseTags{1: "aaa", 2: "bbb", 3: "ccc"}, false},
		"a removed release":        {stored, ReleaseTags{1: "aaa"}, false},
		"a rewritten release tag":  {stored, ReleaseTags{1: "aaa", 2: "ddd"}, false},
		"no releases listed":       {stored, ReleaseTags{}, false},
		"no clone URL stored":      {Checkpoint{Tags: stored.Tags}, ReleaseTags{1: "aaa", 2: "bbb"}, false},
		"a tag ID never recorded":  {Checkpoint{CloneURL: stored.CloneURL, Tags: ReleaseTags{1: "aaa", 2: ""}}, ReleaseTags{1: "aaa", 2: ""}, false},
		"nothing stored or listed": {Checkpoint{CloneURL: stored.CloneURL}, ReleaseTags{}, false},
		// A release before every version's content was stored left the older versions without it.
		"versions stored without content": {Checkpoint{CloneURL: stored.CloneURL, Tags: stored.Tags, MissingContent: true}, ReleaseTags{1: "aaa", 2: "bbb"}, false},
		"a reading guidance without HTML": {Checkpoint{CloneURL: stored.CloneURL, Tags: stored.Tags, Unrendered: true},
			ReleaseTags{1: "aaa", 2: "bbb"}, false},
		// A release before assets were read stored versions without their tags, and no assets.
		"versions stored without tags or assets": {Checkpoint{CloneURL: stored.CloneURL, Tags: stored.Tags, MissingAssets: true},
			ReleaseTags{1: "aaa", 2: "bbb"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.checkpoint.Current(tc.listed); got != tc.want {
				t.Fatalf("Current() = %v, want %v", got, tc.want)
			}
		})
	}
}
