// Trim text the way the reference implementation does.

package coderules

// JavaScript trim includes BOM but excludes NEL, unlike unicode.IsSpace.
func jsWhitespace(r rune) bool {
	return r == 0x0009 || r == 0x000a || r == 0x000b || r == 0x000c || r == 0x000d ||
		r == 0x0020 || r == 0x00a0 || r == 0x1680 || (r >= 0x2000 && r <= 0x200a) ||
		r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff
}
