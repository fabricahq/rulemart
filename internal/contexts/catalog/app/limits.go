// Every limit ingestion keeps, in one place: what fetching a library may hold in memory, and what assembling it may
// read and hold until it's stored.

package app

import (
	"github.com/fabricahq/rulemart/internal/contexts/catalog/domain"
	"github.com/fabricahq/rulemart/internal/contexts/catalog/source/git"
)

// Limits bound the memory one ingestion uses, so a library's repository can't exhaust it.
type Limits struct {
	Fetch   git.Limits
	Content domain.Limits
}

// DefaultLimits leave room for any library Code Rules publishes, which holds at most 10,000 files, while keeping an
// ingestion well under a gigabyte.
var DefaultLimits = Limits{
	Fetch: git.Limits{
		// One release tag per library release.
		Tags: 10_000,
		// Code Rules bounds the tags it publishes to 8 MiB, release notes and signature included.
		TagBytes: 8 << 20,
		// The packfile is held while it's checked, before any object is inflated.
		PackBytes: 128 << 20,
		// The tagged commits' trees and files; history isn't fetched.
		Objects: 100_000,
		// One object, and all of them together, inflated.
		ObjectBytes: 32 << 20,
		TotalBytes:  256 << 20,
	},
	Content: domain.Limits{
		// Each rule, group, or manifest file read.
		FileBytes: 1 << 20,
		// Every current rule's Markdown, metadata, and HTML, and every group's metadata: tens of thousands of long
		// rules.
		ContentBytes: 256 << 20,
	},
}
