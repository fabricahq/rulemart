// Package textdiff compares two texts for a reader: as a unified diff of lines, or as Markdown blocks with the
// changed words marked. Its work is bounded whatever the texts hold, so a page can compare texts it didn't write.
package textdiff

// Op is what an edit does to the old text: keep a piece, delete it, or insert a piece of the new text.
type Op int8

const (
	Equal Op = iota
	Delete
	Insert
)

// edit is one step from old to new: Equal keeps old[i] as new[j], Delete drops old[i], and Insert adds new[j].
type edit struct {
	op   Op
	i, j int
}

const (
	// maxCells bounds one exact comparison, which takes time and memory in proportion to the product of the two
	// lengths: about 4 MiB.
	maxCells = 1 << 20
	// maxWork bounds a whole comparison, counted in the pieces it visits. Past it, what's left to compare shows as
	// deleted and inserted whole, which is correct, if not minimal.
	maxWork = 64 << 20
)

// diff returns the edits that turn a into b, in order, with each change's deletions before its insertions. Between
// common pieces it finds a longest common subsequence exactly when the stretch is small enough, and otherwise
// anchors on pieces that occur once on each side, as patience diff does.
func diff(a, b []string) []edit {
	d := differ{a: a, b: b, work: maxWork}
	d.compare(0, len(a), 0, len(b))
	return d.edits
}

// differ compares a and b, appending edits as it goes.
type differ struct {
	a, b  []string
	edits []edit
	// work is what's left of maxWork.
	work int
}

// compare appends the edits that turn a[a0:a1] into b[b0:b1].
func (d *differ) compare(a0, a1, b0, b1 int) {
	for a0 < a1 && b0 < b1 && d.a[a0] == d.b[b0] {
		d.edits = append(d.edits, edit{Equal, a0, b0})
		a0, b0 = a0+1, b0+1
	}
	suffix := 0
	for a0 < a1-suffix && b0 < b1-suffix && d.a[a1-1-suffix] == d.b[b1-1-suffix] {
		suffix++
	}
	a1, b1 = a1-suffix, b1-suffix
	n, m := a1-a0, b1-b0
	switch {
	case n == 0 || m == 0 || d.work <= 0:
		d.replace(a0, a1, b0, b1)
	case n*m <= maxCells:
		d.work -= n * m
		d.exact(a0, a1, b0, b1)
	default:
		d.work -= n + m
		d.anchored(a0, a1, b0, b1)
	}
	for k := range suffix {
		d.edits = append(d.edits, edit{Equal, a1 + k, b1 + k})
	}
}

// replace appends edits that delete a[a0:a1] and insert b[b0:b1].
func (d *differ) replace(a0, a1, b0, b1 int) {
	for i := a0; i < a1; i++ {
		d.edits = append(d.edits, edit{Delete, i, b0})
	}
	for j := b0; j < b1; j++ {
		d.edits = append(d.edits, edit{Insert, a1, j})
	}
}

// exact appends a minimal edit script for a[a0:a1] and b[b0:b1], from a table of the longest common subsequence of
// every pair of suffixes.
func (d *differ) exact(a0, a1, b0, b1 int) {
	n, m := a1-a0, b1-b0
	w := m + 1
	lcs := make([]uint32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if d.a[a0+i] == d.b[b0+j] {
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			} else {
				lcs[i*w+j] = max(lcs[(i+1)*w+j], lcs[i*w+j+1])
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case d.a[a0+i] == d.b[b0+j]:
			d.edits = append(d.edits, edit{Equal, a0 + i, b0 + j})
			i, j = i+1, j+1
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			d.edits = append(d.edits, edit{Delete, a0 + i, b0 + j})
			i++
		default:
			d.edits = append(d.edits, edit{Insert, a0 + i, b0 + j})
			j++
		}
	}
	d.replace(a0+i, a1, b0+j, b1)
}

// anchored splits a[a0:a1] and b[b0:b1] at the longest run, in order on both sides, of pieces that occur exactly once
// on each side, and compares the stretches between them. Without such pieces, the stretch is replaced whole.
func (d *differ) anchored(a0, a1, b0, b1 int) {
	anchors := uniqueAnchors(d.a[a0:a1], d.b[b0:b1])
	if len(anchors) == 0 {
		d.replace(a0, a1, b0, b1)
		return
	}
	i, j := a0, b0
	for _, anchor := range anchors {
		d.compare(i, a0+anchor.i, j, b0+anchor.j)
		d.edits = append(d.edits, edit{Equal, a0 + anchor.i, b0 + anchor.j})
		i, j = a0+anchor.i+1, b0+anchor.j+1
	}
	d.compare(i, a1, j, b1)
}

// uniqueAnchors returns the longest sequence of pairs (i, j), increasing on both sides, where a[i] == b[j] and that
// piece occurs exactly once in a and once in b.
func uniqueAnchors(a, b []string) []edit {
	type count struct{ a, b, i, j int }
	counts := map[string]*count{}
	for i, piece := range a {
		c := counts[piece]
		if c == nil {
			c = &count{}
			counts[piece] = c
		}
		c.a++
		c.i = i
	}
	for j, piece := range b {
		if c := counts[piece]; c != nil {
			c.b++
			c.j = j
		}
	}
	var pairs []edit
	for i, piece := range a {
		if c := counts[piece]; c.a == 1 && c.b == 1 {
			pairs = append(pairs, edit{Equal, i, c.j})
		}
	}
	return longestIncreasing(pairs)
}

// longestIncreasing returns the longest subsequence of pairs, which are in increasing i order, whose j also
// increases, by patience sorting.
func longestIncreasing(pairs []edit) []edit {
	// tails[k] is the index in pairs of the smallest j that ends an increasing run of length k+1, and previous links
	// each pair to the one before it in its run.
	var tails []int
	previous := make([]int, len(pairs))
	for p, pair := range pairs {
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if pairs[tails[mid]].j < pair.j {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		previous[p] = -1
		if lo > 0 {
			previous[p] = tails[lo-1]
		}
		if lo == len(tails) {
			tails = append(tails, p)
		} else {
			tails[lo] = p
		}
	}
	run := make([]edit, len(tails))
	for k, p := len(tails)-1, -1; k >= 0; k-- {
		if p == -1 {
			p = tails[len(tails)-1]
		} else {
			p = previous[p]
		}
		run[k] = pairs[p]
	}
	return run
}
