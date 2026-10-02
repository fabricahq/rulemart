// Compare two texts line by line, as a unified diff with the changed words of each replaced line marked.

package textdiff

import "strings"

// context is how many unchanged lines a hunk shows on each side of a change, as git does by default.
const context = 3

// LineDiff is a unified diff of two texts.
type LineDiff struct {
	// Hunks are the changed stretches, each with up to context unchanged lines around it, in order. There are none
	// when the texts have the same lines.
	Hunks []Hunk
	// Added and Removed count the inserted and deleted lines.
	Added, Removed int
}

// Hunk is one stretch of a unified diff. OldStart and NewStart number its first line on each side, from 1, and
// OldLines and NewLines count its lines on each side; as in git's hunk header, a side with no lines starts at the
// line before the hunk.
type Hunk struct {
	OldStart, OldLines, NewStart, NewLines int
	Lines                                  []Line
}

// Line is one line of a hunk. Old and New number it on each side, from 1; a deleted line has no New, and an
// inserted one no Old, so that number is 0.
type Line struct {
	Op       Op
	Old, New int
	// Segments are the line's text. A deleted line paired with an inserted one, its replacement, marks the words and
	// whitespace that changed: Delete segments on the deleted line, Insert segments on the inserted one. Any other line is one
	// Equal segment, or none when it's blank.
	Segments []Segment
}

// Segment is a run of text that a diff keeps, deletes, or inserts.
type Segment struct {
	Op   Op
	Text string
}

// Lines compares old and new line by line. A final newline ends the last line rather than starting another, and a
// carriage return before a newline is dropped, so line endings alone don't count as changes.
func Lines(old, new string) LineDiff {
	a, b := splitLines(old), splitLines(new)
	edits := diff(a, b)
	marks := pairReplacedLines(edits, a, b)
	shown := nearChanges(edits)
	var result LineDiff
	var hunk *Hunk
	oldNumber, newNumber := 0, 0
	for k, e := range edits {
		if e.op != Insert {
			oldNumber++
		}
		if e.op != Delete {
			newNumber++
		}
		switch e.op {
		case Delete:
			result.Removed++
		case Insert:
			result.Added++
		}
		if !shown[k] {
			hunk = nil
			continue
		}
		if hunk == nil {
			result.Hunks = append(result.Hunks, startHunk(edits, shown, k, oldNumber, newNumber))
			hunk = &result.Hunks[len(result.Hunks)-1]
		}
		line := Line{Op: e.op, Segments: marks[k]}
		if e.op != Insert {
			line.Old = oldNumber
		}
		if e.op != Delete {
			line.New = newNumber
		}
		if line.Segments == nil {
			text := ""
			if e.op == Insert {
				text = b[e.j]
			} else {
				text = a[e.i]
			}
			line.Segments = plain(text)
		}
		hunk.Lines = append(hunk.Lines, line)
	}
	return result
}

// splitLines returns text's lines, without their line endings.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}

// nearChanges reports, for each edit, whether a hunk shows it: whether it's within context edits of a change.
func nearChanges(edits []edit) []bool {
	shown := make([]bool, len(edits))
	for k, e := range edits {
		if e.op == Equal {
			continue
		}
		for d := max(0, k-context); d <= min(len(edits)-1, k+context); d++ {
			shown[d] = true
		}
	}
	return shown
}

// startHunk returns the hunk that starts at edits[k], numbered oldNumber and newNumber on each side, with its line
// counts but no lines.
func startHunk(edits []edit, shown []bool, k, oldNumber, newNumber int) Hunk {
	var h Hunk
	for e := k; e < len(edits) && shown[e]; e++ {
		if edits[e].op != Insert {
			h.OldLines++
		}
		if edits[e].op != Delete {
			h.NewLines++
		}
	}
	h.OldStart, h.NewStart = oldNumber, newNumber
	// The counters already passed edits[k] on the sides it has a line on; a side it has no line on starts at the
	// line before the hunk, as git's header does, unless the hunk has no lines on that side at all.
	if edits[k].op == Insert && h.OldLines > 0 {
		h.OldStart++
	}
	if edits[k].op == Delete && h.NewLines > 0 {
		h.NewStart++
	}
	return h
}

// pairReplacedLines pairs the deleted and inserted lines of each change in order, and returns, by edit index, the
// segments that mark each pair's changed words. Lines without a partner aren't in the result.
func pairReplacedLines(edits []edit, a, b []string) map[int][]Segment {
	marks := map[int][]Segment{}
	for k := 0; k < len(edits); {
		if edits[k].op == Equal {
			k++
			continue
		}
		var deleted, inserted []int
		for ; k < len(edits) && edits[k].op != Equal; k++ {
			if edits[k].op == Delete {
				deleted = append(deleted, k)
			} else {
				inserted = append(inserted, k)
			}
		}
		for x := range min(len(deleted), len(inserted)) {
			d, i := deleted[x], inserted[x]
			marks[d], marks[i], _ = compareWords(a[edits[d].i], b[edits[i].j], true)
		}
	}
	return marks
}
