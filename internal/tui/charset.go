// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

package tui

import (
	"strings"
	"unicode/utf8"
)

// Charset is what the terminal shows the screen's lines and shading in.
type Charset int

const (
	UTF8  Charset = iota // as they are
	CP437                // the IBM PC's code page: what BBS terminals have
	DEC                  // a VT's line-drawing set (ESC ( 0), the rest ASCII
	ASCII                // + - | and letters for the arrows
)

// ParseCharset is a charset by its name: utf8, cp437, dec, ascii.
func ParseCharset(s string) (Charset, bool) {
	switch strings.ToLower(strings.ReplaceAll(s, "-", "")) {
	case "utf8":
		return UTF8, true
	case "cp437", "ibm", "pc", "437":
		return CP437, true
	case "dec", "vt", "vt100":
		return DEC, true
	case "ascii":
		return ASCII, true
	}
	return UTF8, false
}

func (c Charset) String() string {
	return [...]string{"utf8", "cp437", "dec", "ascii"}[c]
}

// cp437High is code page 437 from 0x80 to 0xFF.
const cp437High = "ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒáíóúñÑªº¿⌐¬½¼¡«»" +
	"░▒▓│┤╡╢╖╕╣║╗╝╜╛┐└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀" +
	"αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ "

var cp437 = func() map[rune]byte {
	m := map[rune]byte{}
	i := 0x80
	for _, r := range cp437High {
		m[r] = byte(i)
		i++
	}
	// its glyphs in the control range (► ▲ ▼...) are not sent: some
	// terminals take them as controls and the columns after slip
	m['►'], m['↕'], m['…'] = 0xaf, 0xf0, 0xaf
	return m
}()

// what the PC's code page has no place for that the screens draw
var plain = map[rune]string{
	'◄': "<", '↑': "^", '↓': "v", '→': ">", '←': "<", '…': "~", '►': ">",
	'↕': "|", '▲': "^", '▼': "v", '•': "*", '✓': "*",
}

// the VT's line-drawing characters (in ESC ( 0), double lines as single
var decGraphics = map[rune]byte{
	'┘': 'j', '┐': 'k', '┌': 'l', '└': 'm', '┼': 'n', '─': 'q', '├': 't', '┤': 'u', '┴': 'v', '┬': 'w', '│': 'x',
	'╝': 'j', '╗': 'k', '╔': 'l', '╚': 'm', '═': 'q', '║': 'x', '╟': 't', '╢': 'u',
	'░': 'a', '▒': 'a', '▓': 'a', '·': '~', '°': 'f', '±': 'g', '≤': 'y', '≥': 'z', '£': '}',
}

// and as ASCII
var asciiLines = map[rune]byte{
	'┘': '+', '┐': '+', '┌': '+', '└': '+', '┼': '+', '─': '-', '├': '+', '┤': '+', '┴': '+', '┬': '+', '│': '|',
	'╝': '+', '╗': '+', '╔': '+', '╚': '+', '═': '=', '║': '|', '╟': '+', '╢': '+',
	'░': ' ', '▒': ' ', '▓': '#', '█': '#', '·': '.', '■': '#',
}

// Encode is s (text and escape sequences) as a terminal of cs takes it.
func Encode(s string, cs Charset) []byte {
	if cs == UTF8 {
		return []byte(s)
	}
	out := make([]byte, 0, len(s))
	graphics := false // DEC: ESC ( 0 is in
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			// an escape sequence, as it is: ESC [ ... final, or ESC and one
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
			}
			j = min(j+1, len(s))
			out = append(out, s[i:j]...)
			i = j
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		if r < 0x80 {
			if graphics {
				out, graphics = append(out, "\x1b(B"...), false
			}
			out = append(out, byte(r))
			continue
		}
		switch cs {
		case CP437:
			if b, ok := cp437[r]; ok {
				out = append(out, b)
			} else if p, ok := plain[r]; ok {
				out = append(out, p...)
			} else {
				out = append(out, '?')
			}
		case DEC:
			if b, ok := decGraphics[r]; ok {
				if !graphics {
					out, graphics = append(out, "\x1b(0"...), true
				}
				out = append(out, b)
				continue
			}
			if graphics {
				out, graphics = append(out, "\x1b(B"...), false
			}
			out = append(out, ascii(r)...)
		default:
			out = append(out, ascii(r)...)
		}
	}
	if graphics {
		out = append(out, "\x1b(B"...)
	}
	return out
}

// ascii is r in ASCII, as near as it goes.
func ascii(r rune) string {
	if b, ok := asciiLines[r]; ok {
		return string(b)
	}
	if p, ok := plain[r]; ok {
		return p
	}
	return "?"
}
