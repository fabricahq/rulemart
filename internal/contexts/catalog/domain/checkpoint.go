// Deciding whether a library needs ingesting again: what the catalog stored of its release tags, compared with what
// its repository lists now.

package domain

import "maps"

// ReleaseTags maps library release numbers to the IDs of their annotated release/<number> tag objects.
type ReleaseTags map[int]string

// Checkpoint is what the catalog stored of a library when ingestion last wrote it.
type Checkpoint struct {
	// CloneURL is where that ingestion fetched the library; empty when the release that ingested it didn't record
	// it.
	CloneURL string
	// Tags are the stored releases' tags. A release whose tag ID wasn't recorded maps to "".
	Tags ReleaseTags
	// MissingContent reports that a stored rule version lacks its content: a release before every version's file
	// was stored, which stored only the current version's, ingested the library.
	MissingContent bool
	// Unrendered reports a current rule version whose reading guidance has no HTML rendered from it, because a
	// release that didn't render it stored the guidance.
	Unrendered bool
	// MissingAssets reports a stored version without its tags: a release that read neither tags nor assets stored
	// it, so the library's assets are missing too.
	MissingAssets bool
}

// Current reports whether listed, the release tags the library's repository lists now, are exactly the ones the
// catalog stored: the same numbers, each pointing to the same tag object. Ingesting the library again would then
// read the same releases. It's false when the checkpoint lacks the clone URL, a tag ID, a version's content, or the
// HTML of a reading guidance, or its assets, so a library a release before these were recorded stored is ingested
// again.
func (c Checkpoint) Current(listed ReleaseTags) bool {
	if c.CloneURL == "" || c.MissingContent || c.Unrendered || c.MissingAssets || len(listed) == 0 {
		return false
	}
	return maps.EqualFunc(c.Tags, listed, func(stored, listed string) bool { return stored != "" && stored == listed })
}
