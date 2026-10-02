// Compare two Markdown texts block by block, marking the words that changed, and fold the unchanged blocks away
// from a change.

package textdiff

import (
	"strings"
	"unicode"
)

// Part is a run of blocks in a word diff: shown, or folded away because nothing near it changed.
type Part struct {
	// Folded parts hold only unchanged blocks, which a reader opens to see.
	Folded bool
	Blocks []Block
}

// Block is a Markdown block of a word diff: a run of lines between blank lines, with a fenced code block kept
// whole.
type Block struct {
	// Changed blocks mark their changes: Delete segments hold old words, and Insert segments new ones, in reading
	// order. An unchanged block is one Equal segment of the new text.
	Changed  bool
	Segments []Segment
}

// Words compares old and new Markdown block by block, ignoring changes in whitespace alone, such as a rewrapped
// paragraph, and marks the words that changed in each changed stretch. It shows the unchanged block on each side of
// a change for context, unless it's the frontmatter, which says nothing about the change, and folds the other
// unchanged blocks. Identical texts give one folded part, or none when they're empty.
func Words(old, new string) []Part {
	a, b := markdownBlocks(old), markdownBlocks(new)
	edits := diff(normalized(a), normalized(b))
	var parts []Part
	var unchanged []string
	seenChange := false
	for k := 0; k < len(edits); {
		if edits[k].op == Equal {
			unchanged = append(unchanged, b[edits[k].j])
			k++
			continue
		}
		parts = appendUnchanged(parts, unchanged, !seenChange, false)
		unchanged, seenChange = nil, true
		var deleted, inserted []string
		for ; k < len(edits) && edits[k].op != Equal; k++ {
			if edits[k].op == Delete {
				deleted = append(deleted, a[edits[k].i])
			} else {
				inserted = append(inserted, b[edits[k].j])
			}
		}
		_, _, both := compareWords(strings.Join(deleted, "\n\n"), strings.Join(inserted, "\n\n"))
		parts = append(parts, Part{Blocks: []Block{{Changed: true, Segments: both}}})
	}
	return appendUnchanged(parts, unchanged, !seenChange, true)
}

// appendUnchanged appends a run of unchanged blocks to parts: the first and last shown for context, unless the run
// starts or ends the text or that block is frontmatter, and the rest folded.
func appendUnchanged(parts []Part, run []string, atStart, atEnd bool) []Part {
	if len(run) == 0 {
		return parts
	}
	context := func(block string) bool { return !strings.HasPrefix(block, "---\n") }
	head, tail := 0, 0
	if !atStart && context(run[0]) {
		head = 1
	}
	if !atEnd && context(run[len(run)-1]) && len(run) > head {
		tail = 1
	}
	shown := func(blocks []string) Part {
		p := Part{}
		for _, block := range blocks {
			p.Blocks = append(p.Blocks, Block{Segments: plain(block)})
		}
		return p
	}
	if head > 0 {
		parts = append(parts, shown(run[:head]))
	}
	if hidden := run[head : len(run)-tail]; len(hidden) > 0 {
		folded := shown(hidden)
		folded.Folded = true
		parts = append(parts, folded)
	}
	if tail > 0 {
		parts = append(parts, shown(run[len(run)-tail:]))
	}
	return parts
}

// markdownBlocks splits text into runs of lines between blank lines, keeping a fenced code block whole.
func markdownBlocks(text string) []string {
	var blocks, current []string
	fenced := false
	for _, line := range splitLines(text) {
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			fenced = !fenced
		}
		if !fenced && strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
			}
			current = nil
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}
	return blocks
}

// normalized returns each block with its runs of whitespace collapsed to one space and trimmed, so blocks that
// differ only in whitespace compare equal.
func normalized(blocks []string) []string {
	keys := make([]string, len(blocks))
	for i, block := range blocks {
		keys[i] = strings.Join(strings.Fields(block), " ")
	}
	return keys
}

// compareWords compares old and new as runs of words and whitespace, and returns three views of the result: old's
// text with its deleted words marked, new's with its inserted words marked, and both together in reading order.
// Whitespace always compares equal, so only words change, and a space between two changed words joins them into one
// change.
func compareWords(old, new string) (oldMarks, newMarks, both []Segment) {
	a, b := splitWords(old), splitWords(new)
	edits := diff(wordKeys(a), wordKeys(b))
	var o, n, t segmentBuilder
	var deleted, inserted strings.Builder
	flush := func() {
		if text := deleted.String(); strings.TrimSpace(text) != "" {
			o.add(Delete, text)
			t.add(Delete, text)
		} else {
			o.add(Equal, text)
		}
		if text := inserted.String(); strings.TrimSpace(text) != "" {
			n.add(Insert, text)
			t.add(Insert, text)
		} else {
			n.add(Equal, text)
			t.add(Equal, text)
		}
		deleted.Reset()
		inserted.Reset()
	}
	for k, e := range edits {
		switch {
		case e.op == Delete:
			deleted.WriteString(a[e.i])
		case e.op == Insert:
			inserted.WriteString(b[e.j])
		case isSpace(b[e.j]) && deleted.Len()+inserted.Len() > 0 && k+1 < len(edits) && edits[k+1].op != Equal:
			deleted.WriteString(a[e.i])
			inserted.WriteString(b[e.j])
		default:
			flush()
			o.add(Equal, a[e.i])
			n.add(Equal, b[e.j])
			t.add(Equal, b[e.j])
		}
	}
	flush()
	return o.segments(), n.segments(), t.segments()
}

// splitWords splits text into alternating runs of whitespace and of other characters.
func splitWords(text string) []string {
	var words []string
	start := 0
	for i, r := range text {
		if i > start && unicode.IsSpace(r) != isSpace(text[start:i]) {
			words = append(words, text[start:i])
			start = i
		}
	}
	if start < len(text) {
		words = append(words, text[start:])
	}
	return words
}

// isSpace reports whether a run from splitWords is whitespace.
func isSpace(word string) bool {
	for _, r := range word {
		return unicode.IsSpace(r)
	}
	return false
}

// wordKeys returns what each run compares by: whitespace as one space, so any two runs of it are equal.
func wordKeys(words []string) []string {
	keys := make([]string, len(words))
	for i, word := range words {
		if isSpace(word) {
			keys[i] = " "
		} else {
			keys[i] = word
		}
	}
	return keys
}

// segmentBuilder joins runs of text into segments, one for each run of the same op.
type segmentBuilder struct {
	done    []Segment
	op      Op
	pending strings.Builder
}

// add appends text with op, ignoring empty text.
func (b *segmentBuilder) add(op Op, text string) {
	if text == "" {
		return
	}
	if b.pending.Len() > 0 && b.op != op {
		b.done = append(b.done, Segment{b.op, b.pending.String()})
		b.pending.Reset()
	}
	b.op = op
	b.pending.WriteString(text)
}

// segments returns the segments added so far.
func (b *segmentBuilder) segments() []Segment {
	if b.pending.Len() == 0 {
		return b.done
	}
	return append(b.done, Segment{b.op, b.pending.String()})
}

// plain returns text as one unchanged segment, or none when it's empty.
func plain(text string) []Segment {
	if text == "" {
		return nil
	}
	return []Segment{{Equal, text}}
}
