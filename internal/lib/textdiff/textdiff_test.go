package textdiff

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

// apply rebuilds both sides from edits, failing when they don't turn a into b in order.
func apply(t *testing.T, a, b []string, edits []edit) {
	t.Helper()
	var gotA, gotB []string
	for _, e := range edits {
		switch e.op {
		case Equal:
			if a[e.i] != b[e.j] {
				t.Fatalf("edit %+v keeps %q as %q", e, a[e.i], b[e.j])
			}
			gotA, gotB = append(gotA, a[e.i]), append(gotB, b[e.j])
		case Delete:
			gotA = append(gotA, a[e.i])
		case Insert:
			gotB = append(gotB, b[e.j])
		}
	}
	if !slices.Equal(gotA, a) || !slices.Equal(gotB, b) {
		t.Fatalf("edits %+v rebuild %q and %q, want %q and %q", edits, gotA, gotB, a, b)
	}
}

// lcsLength is the length of a longest common subsequence of a and b, by the textbook table.
func lcsLength(a, b []string) int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	return table[0][0]
}

func randomPieces(r *rand.Rand, n, alphabet int) []string {
	pieces := make([]string, n)
	for i := range pieces {
		pieces[i] = fmt.Sprint(r.IntN(alphabet))
	}
	return pieces
}

// Small comparisons are exact: they keep a longest common subsequence, and every edit script rebuilds both sides.
func TestDiffKeepsALongestCommonSubsequenceOfSmallInputs(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		a, b := randomPieces(r, r.IntN(12), 4), randomPieces(r, r.IntN(12), 4)

		edits := diff(a, b)

		apply(t, a, b, edits)
		kept := 0
		for _, e := range edits {
			if e.op == Equal {
				kept++
			}
		}
		if want := lcsLength(a, b); kept != want {
			t.Fatalf("diff(%q, %q) keeps %d pieces, want %d", a, b, kept, want)
		}
	}
}

// Inputs too large to compare exactly are anchored on pieces that occur once on each side, and still rebuild both
// sides, quickly, even when nothing anchors them.
func TestDiffStaysCorrectAndFastOnLargeInputs(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	lines := func(n int) []string {
		result := make([]string, n)
		for i := range result {
			result[i] = fmt.Sprintf("line %d", i)
		}
		return result
	}
	edited := lines(20000)
	for range 300 {
		k := r.IntN(len(edited))
		edited[k] = "changed " + edited[k]
	}
	repeated := slices.Repeat([]string{"same"}, 5000)
	for name, tc := range map[string]struct {
		a, b    []string
		minKept int
	}{
		"a long file with scattered edits": {lines(20000), edited, 19000},
		"no piece unique on either side":   {repeated, append(slices.Repeat([]string{"other"}, 3000), repeated[:2000]...), 0},
		"random pieces from a small set":   {randomPieces(r, 8000, 50), randomPieces(r, 8000, 50), 0},
	} {
		t.Run(name, func(t *testing.T) {
			started := time.Now()

			edits := diff(tc.a, tc.b)

			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Errorf("took %s", elapsed)
			}
			apply(t, tc.a, tc.b, edits)
			kept := 0
			for _, e := range edits {
				if e.op == Equal {
					kept++
				}
			}
			if kept < tc.minKept {
				t.Errorf("kept %d pieces, want at least %d", kept, tc.minKept)
			}
		})
	}
}

// describeLines writes a line diff as git would, with changed words in [-…-] and {+…+}.
func describeLines(d LineDiff) string {
	var s strings.Builder
	fmt.Fprintf(&s, "+%d -%d\n", d.Added, d.Removed)
	for _, h := range d.Hunks {
		fmt.Fprintf(&s, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			fmt.Fprintf(&s, "%d %d %s%s\n", line.Old, line.New, map[Op]string{Equal: " ", Delete: "-", Insert: "+"}[line.Op], describeSegments(line.Segments))
		}
	}
	return s.String()
}

func describeSegments(segments []Segment) string {
	var s strings.Builder
	for _, seg := range segments {
		switch seg.Op {
		case Equal:
			s.WriteString(seg.Text)
		case Delete:
			s.WriteString("[-" + seg.Text + "-]")
		case Insert:
			s.WriteString("{+" + seg.Text + "+}")
		}
	}
	return s.String()
}

func numbered(from, to int) string {
	var s strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&s, "line %d\n", i)
	}
	return s.String()
}

func TestLinesShowsHunksWithContextAndChangedWords(t *testing.T) {
	for name, tc := range map[string]struct{ old, new, want string }{
		"identical texts":   {"a\nb\n", "a\nb\n", "+0 -0\n"},
		"line endings only": {"a\r\nb\r\n", "a\nb", "+0 -0\n"},
		"a replaced line marks its changed words": {numbered(1, 3) + "Retry three times.\n" + numbered(5, 9),
			numbered(1, 3) + "Retry two times.\n" + numbered(5, 9), `+1 -1
@@ -1,7 +1,7 @@
1 1  line 1
2 2  line 2
3 3  line 3
4 0 -Retry [-three-] times.
0 4 +Retry {+two+} times.
5 5  line 5
6 6  line 6
7 7  line 7
`},
		"changes far apart make two hunks": {numbered(1, 20), strings.Replace(strings.Replace(numbered(1, 20), "line 2\n", "", 1), "line 19", "line nineteen", 1), `+1 -2
@@ -1,5 +1,4 @@
1 1  line 1
2 0 -line 2
3 2  line 3
4 3  line 4
5 4  line 5
@@ -16,5 +15,5 @@
16 15  line 16
17 16  line 17
18 17  line 18
19 0 -line [-19-]
0 18 +line {+nineteen+}
20 19  line 20
`},
		// As in git, a side with no lines starts at the line before the hunk.
		"a new file":             {"", "one\ntwo\n", "+2 -0\n@@ -0,0 +1,2 @@\n0 1 +one\n0 2 +two\n"},
		"lines added at the end": {"a\n", "a\nb\n", "+1 -0\n@@ -1,1 +1,2 @@\n1 1  a\n0 2 +b\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := describeLines(Lines(tc.old, tc.new)); got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// describeParts writes a word diff one block per line, with folded parts in parentheses.
func describeParts(parts []Part) string {
	var s strings.Builder
	for _, p := range parts {
		if p.Folded {
			fmt.Fprintf(&s, "(%d folded)\n", len(p.Blocks))
			continue
		}
		for _, b := range p.Blocks {
			mark := " "
			if b.Changed {
				mark = "*"
			}
			fmt.Fprintf(&s, "%s%s\n", mark, strings.ReplaceAll(describeSegments(b.Segments), "\n", "⏎"))
		}
	}
	return s.String()
}

func TestWordsMarksChangedWordsAndFoldsUnchangedBlocks(t *testing.T) {
	frontmatter := "---\ntitle: Retry\n---\n\n"
	paragraphs := func(words ...string) string {
		var s []string
		for _, w := range words {
			s = append(s, "Paragraph "+w+".")
		}
		return strings.Join(s, "\n\n") + "\n"
	}
	for name, tc := range map[string]struct{ old, new, want string }{
		"identical texts":                    {"One.\n\nTwo.\n", "One.\n\nTwo.\n", "(2 folded)\n"},
		"empty texts":                        {"", "", ""},
		"a rewrapped paragraph is unchanged": {"A long\nsentence here.\n", "A long sentence\nhere.\n", "(1 folded)\n"},
		"one changed word shows its neighbors and folds the rest": {
			frontmatter + paragraphs("a", "b", "c", "d", "e"), frontmatter + paragraphs("a", "b", "C", "d", "e"),
			"(2 folded)\n Paragraph b.\n*Paragraph [-c.-]{+C.+}\n Paragraph d.\n(1 folded)\n"},
		// The frontmatter says nothing about a change beside it, so it folds rather than showing as context.
		"a change after the frontmatter": {frontmatter + paragraphs("a"), frontmatter + paragraphs("b"),
			"(1 folded)\n*Paragraph [-a.-]{+b.+}\n"},
		"an added block": {paragraphs("a", "c"), paragraphs("a", "b", "c"), " Paragraph a.\n*{+Paragraph b.+}\n Paragraph c.\n"},
		// A space between two changed words joins them into one change.
		"neighboring changed words are one change": {"Stop after three tries.\n", "Stop before four tries.\n",
			"*Stop [-after three-]{+before four+} tries.\n"},
		// A fenced code block is one block, blank lines and all.
		"a code block stays whole": {"```go\na := 1\n\nb := 2\n```\n", "```go\na := 1\n\nb := 3\n```\n",
			"*```go⏎a := 1⏎⏎b := [-2-]{+3+}⏎```\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := describeParts(Words(tc.old, tc.new)); got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// One huge paragraph of repeated words still compares within the work limit.
func TestWordsStaysFastOnOneHugeBlock(t *testing.T) {
	old := strings.Repeat("retry the request again ", 40000)
	new := strings.Repeat("retry the call again ", 40000)
	started := time.Now()

	parts := Words(old, new)

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("took %s", elapsed)
	}
	if len(parts) != 1 || !parts[0].Blocks[0].Changed {
		t.Fatalf("got %d parts, want one changed block", len(parts))
	}
}
