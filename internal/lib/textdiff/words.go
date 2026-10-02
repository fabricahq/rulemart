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

// Words compares old and new Markdown block by block, ignoring changes in whitespace alone in prose, such as a
// rewrapped paragraph, and marks the words that changed in each changed stretch. Whitespace can change what code
// does, so a code block is compared, and marked, as it is. It shows the unchanged block on each side of
// a change for context, unless it's the frontmatter, which says nothing about the change, and folds the other
// unchanged blocks. Identical texts give one folded part, or none when they're empty.
func Words(old, new string) []Part {
	a, b := markdownBlocks(old), markdownBlocks(new)
	edits := diff(blockKeys(a), blockKeys(b))
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
		_, _, both := compareWords(blockWords(deleted), blockWords(inserted))
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

// blockKeys returns what each block compares by: a code block as it is, and any other with its runs of whitespace
// collapsed to one space and trimmed, so prose that differs only in whitespace compares equal.
func blockKeys(blocks []string) []string {
	keys := make([]string, len(blocks))
	for i, block := range blocks {
		if isCode(block) {
			keys[i] = block
		} else {
			keys[i] = strings.Join(strings.Fields(block), " ")
		}
	}
	return keys
}

// isCode reports whether a block from markdownBlocks is code: fenced, or with every line indented by four spaces or
// a tab.
func isCode(block string) bool {
	if strings.HasPrefix(block, "```") || strings.HasPrefix(block, "~~~") {
		return true
	}
	for _, line := range strings.Split(block, "\n") {
		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			return false
		}
	}
	return true
}

// word is a run of a text's words or whitespace, and what it compares by.
type word struct {
	text, key string
	// exact marks whitespace that compares, and shows as changed, as it is, such as in code.
	exact bool
}

// textWords splits text into runs of words and whitespace. Unless exact, whitespace compares as one space, so any
// two runs of it are equal.
func textWords(text string, exact bool) []word {
	runs := splitWords(text)
	words := make([]word, len(runs))
	for i, run := range runs {
		words[i] = word{text: run, key: run, exact: exact}
		if isSpace(run) && !exact {
			words[i].key = " "
		}
	}
	return words
}

// blockWords splits Markdown blocks, joined by blank lines, into runs of words and whitespace: a code block's
// whitespace exactly, and prose's, and the blank lines between blocks, as any whitespace.
func blockWords(blocks []string) []word {
	var words []word
	for i, block := range blocks {
		if i > 0 {
			words = append(words, word{text: "\n\n", key: " "})
		}
		words = append(words, textWords(block, isCode(block))...)
	}
	return words
}

// compareWords compares two texts' runs of words and whitespace, and returns three views of the result: the old
// text with its deleted words marked, the new with its inserted words marked, and both together in reading order. A
// space between two changed words joins them into one change. Whitespace alone shows as changed only where it's
// exact.
func compareWords(a, b []word) (oldMarks, newMarks, both []Segment) {
	keys := func(words []word) []string {
		k := make([]string, len(words))
		for i, w := range words {
			k[i] = w.key
		}
		return k
	}
	edits := diff(keys(a), keys(b))
	var o, n, t segmentBuilder
	// changed is a run of deleted or inserted text, and exact whether any of it is exact whitespace.
	type changed struct {
		text  strings.Builder
		exact bool
	}
	var deleted, inserted changed
	add := func(c *changed, w word) {
		c.text.WriteString(w.text)
		c.exact = c.exact || w.exact
	}
	// marked reports whether a run of changed text shows as a change: whitespace alone only where it's exact.
	marked := func(c *changed) bool {
		text := c.text.String()
		return text != "" && (c.exact || strings.TrimSpace(text) != "")
	}
	flush := func() {
		if text := deleted.text.String(); marked(&deleted) {
			o.add(Delete, text)
			t.add(Delete, text)
		} else {
			o.add(Equal, text)
		}
		if text := inserted.text.String(); marked(&inserted) {
			n.add(Insert, text)
			t.add(Insert, text)
		} else {
			n.add(Equal, text)
			t.add(Equal, text)
		}
		deleted, inserted = changed{}, changed{}
	}
	for k, e := range edits {
		switch {
		case e.op == Delete:
			add(&deleted, a[e.i])
		case e.op == Insert:
			add(&inserted, b[e.j])
		case isSpace(b[e.j].text) && deleted.text.Len()+inserted.text.Len() > 0 && k+1 < len(edits) && edits[k+1].op != Equal:
			add(&deleted, a[e.i])
			add(&inserted, b[e.j])
		default:
			flush()
			o.add(Equal, a[e.i].text)
			n.add(Equal, b[e.j].text)
			t.add(Equal, b[e.j].text)
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
