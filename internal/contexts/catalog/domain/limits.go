// Every limit ingestion keeps, in one place: what fetching a library may hold in memory, and what assembling it may
// read and hold until it's stored.

package domain

// Limits bound the memory one ingestion uses, so a library's repository can't exhaust it.
type Limits struct {
	Fetch   FetchLimits
	Content ContentLimits
}

// FetchLimits bound what fetching a library's release tags may hold in memory.
type FetchLimits struct {
	// Tags bounds the release tags.
	Tags int
	// TagBytes bounds one release tag object, its message and any signature included.
	TagBytes int64
	// PackBytes bounds the packfile the remote sends, which is held while it's checked.
	PackBytes int64
	// Objects bounds the objects the packfile holds.
	Objects int
	// ObjectBytes bounds one object, inflated, and TotalBytes all of them together.
	ObjectBytes, TotalBytes int64
}

// ContentLimits bound what assembling a library reads and holds.
type ContentLimits struct {
	// FileBytes bounds each rule, group, or manifest file assembly reads.
	FileBytes int64
	// ContentBytes bounds the content assembly holds until the library is stored: every current rule's Markdown,
	// its title, impact description, and reading guidance, its HTML and the links rendering rewrites, and every
	// group's metadata file. A release's files share storage however many paths have the same content, so what a
	// source fetches can't bound this: a small release can list thousands of rules or groups that share one large
	// file.
	ContentBytes int64
}

// DefaultLimits leave room for any library Code Rules publishes, which holds at most 10,000 files, while keeping an
// ingestion well under a gigabyte.
var DefaultLimits = Limits{
	Fetch: FetchLimits{
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
	Content: ContentLimits{
		// Each rule, group, or manifest file read.
		FileBytes: 1 << 20,
		// Every current rule's Markdown, metadata, and HTML, and every group's metadata: tens of thousands of long
		// rules.
		ContentBytes: 256 << 20,
	},
}
