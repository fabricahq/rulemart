// What assembly needs from rendering: a rule's page, and a renderer that keeps within a byte allowance.

package domain

import "errors"

// RulePage is where a rule's Markdown came from, which its relative links and images resolve against.
type RulePage struct {
	// Repository is the library's GitHub repository, as owner/name.
	Repository string
	// Path is the rule's file, such as practices/testing/verify-retry-limits.md.
	Path string
	// Title is the rule's title, which the page shows above the body.
	Title string
	// Tag published the rule's version, and LatestTag is the library's latest release. Files the rule's version
	// covers, its Markdown and its own assets, link to Tag; library-wide files link to LatestTag.
	Tag, LatestTag string
}

// Render returns the HTML a rule's page shows for its Markdown body, and how many bytes of allowance it used: the
// HTML, and each link it rewrote. It fails with ErrOverAllowance, without building past allowance, when the HTML
// would need more. render.Rule implements it; assembly takes it as a function so that what reads the catalog
// doesn't carry a Markdown renderer.
type Render func(body string, page RulePage, allowance int64) (html string, used int64, err error)

// ErrOverAllowance reports a render that would build more than its allowance.
var ErrOverAllowance = errors.New("the rule's HTML needs more bytes than its allowance")
