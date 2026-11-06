// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package term

// PETSCIIOut: what the menu writes, for a Commodore in its lower/upper
// case set: the letters' cases swapped as that set has them, a line end
// as RETURN, backspace as cursor left, escape sequences and what it has
// not dropped
func PETSCIIOut(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 0x1b:
			// an escape sequence: CSI's parameters to its final byte, or
			// one character
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
					i++
				}
			} else {
				i++
			}
		case c == '\r':
			out = append(out, 0x0d)
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		case c == '\n':
			out = append(out, 0x0d)
		case c == 0x08:
			out = append(out, 0x9d)
		case c >= 'a' && c <= 'z':
			out = append(out, c-'a'+0x41)
		case c >= 'A' && c <= 'Z':
			out = append(out, c-'A'+0xc1)
		case c == '{':
			out = append(out, '(')
		case c == '}':
			out = append(out, ')')
		case c == '|':
			out = append(out, '!')
		case c == '~':
			out = append(out, '-')
		case c == '`':
			out = append(out, '\'')
		case c == '\\':
			out = append(out, '/')
		case c == '_':
			out = append(out, '-')
		case c >= 0x20 && c < 0x7f:
			out = append(out, c)
		case c >= 0xc0:
			// a rune of UTF-8: one character the C64 has not
			out = append(out, '?')
		}
	}
	return out
}

// PETSCIIIn: a Commodore's key as ASCII: its unshifted letters lower
// case, its shifted ones upper case, DEL as DEL
func PETSCIIIn(c byte) byte {
	switch {
	case c >= 0x41 && c <= 0x5a:
		return c - 0x41 + 'a'
	case c >= 0xc1 && c <= 0xda:
		return c - 0xc1 + 'A'
	case c >= 0x61 && c <= 0x7a:
		return c - 0x61 + 'A'
	case c == 0x14:
		return 0x7f
	}
	return c
}
