// Copyright (C) 2026 D-Modem Authors
// SPDX-License-Identifier: AGPL-3.0-only

// Package tui is the full screen of an ANSI terminal, drawn in character
// cells as the setup programs and BBSs of the 1990s drew theirs: a shaded
// blue desk, windows with shadows over it, a menu bar of pull-down menus
// at the top and a bar of keys at the foot.  What is drawn is kept, and
// only the cells that change are sent, so that a caller at 2400 bit/s is
// not sent the whole screen for a moved bar.  A terminal without UTF-8
// gets code page 437, a VT the DEC line-drawing set, others ASCII.
//
// The screen buffer and its rendering come from the bbs's setup screens
// (D-Modem/bbs internal/textui/desk.go).
package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// The sixteen colors of a PC's text mode: 0-7 as backgrounds too.
const (
	Black = iota
	Blue
	Green
	Cyan
	Red
	Magenta
	Brown
	LightGray
	DarkGray
	LightBlue
	LightGreen
	LightCyan
	LightRed
	LightMagenta
	Yellow
	White
)

// Attr is a cell's colors: foreground (0-15) and background (0-7).
type Attr struct{ FG, BG uint8 }

// A is an Attr.
func A(fg, bg uint8) Attr { return Attr{fg, bg} }

// cell is a character cell.
type cell struct {
	r rune
	a Attr
}

// Screen is a screen's cells.
type Screen struct {
	Cols, Rows int
	c          []cell
}

// NewScreen is a blank screen.
func NewScreen(cols, rows int) *Screen {
	s := &Screen{Cols: cols, Rows: rows, c: make([]cell, cols*rows)}
	for i := range s.c {
		s.c[i] = cell{' ', Attr{LightGray, Black}}
	}
	return s
}

// Clone is a copy of s.
func (s *Screen) Clone() *Screen {
	n := &Screen{Cols: s.Cols, Rows: s.Rows, c: make([]cell, len(s.c))}
	copy(n.c, s.c)
	return n
}

// at is the cell at row and column (from 0), nil off the screen.
func (s *Screen) at(row, col int) *cell {
	if row < 0 || col < 0 || row >= s.Rows || col >= s.Cols {
		return nil
	}
	return &s.c[row*s.Cols+col]
}

// Rune is the character at row and column (0 off the screen).
func (s *Screen) Rune(row, col int) rune {
	if c := s.at(row, col); c != nil {
		return c.r
	}
	return 0
}

// AttrAt is the colors at row and column.
func (s *Screen) AttrAt(row, col int) Attr {
	if c := s.at(row, col); c != nil {
		return c.a
	}
	return Attr{}
}

// Line is a row's characters, for tests and logs.
func (s *Screen) Line(row int) string {
	var b strings.Builder
	for x := 0; x < s.Cols; x++ {
		b.WriteRune(s.Rune(row, x))
	}
	return b.String()
}

// Text is the whole screen's characters, a line to a row.
func (s *Screen) Text() string {
	var b strings.Builder
	for y := 0; y < s.Rows; y++ {
		b.WriteString(strings.TrimRight(s.Line(y), " ") + "\n")
	}
	return b.String()
}

// Put writes str from row, col in a, cut at the screen's edge; how many
// cells it took.
func (s *Screen) Put(row, col int, str string, a Attr) int {
	n := 0
	for _, r := range str {
		if c := s.at(row, col+n); c != nil {
			*c = cell{r, a}
		}
		n++
	}
	return n
}

// PutW writes str in exactly w cells: cut, or padded with spaces.
func (s *Screen) PutW(row, col, w int, str string, a Attr) {
	s.Put(row, col, Fit(str, w), a)
}

// Fill fills a rectangle with r in a.
func (s *Screen) Fill(row, col, h, w int, r rune, a Attr) {
	for y := row; y < row+h; y++ {
		for x := col; x < col+w; x++ {
			if c := s.at(y, x); c != nil {
				*c = cell{r, a}
			}
		}
	}
}

// Paint sets the colors of a rectangle, keeping its characters.
func (s *Screen) Paint(row, col, h, w int, a Attr) {
	for y := row; y < row+h; y++ {
		for x := col; x < col+w; x++ {
			if c := s.at(y, x); c != nil {
				c.a = a
			}
		}
	}
}

// BoxStyle is a box's lines: corners, sides and the joins of a line
// across it.
type BoxStyle struct{ tl, tr, bl, br, h, v, lj, rj rune }

var (
	Double = BoxStyle{'╔', '╗', '╚', '╝', '═', '║', '╟', '╢'}
	Single = BoxStyle{'┌', '┐', '└', '┘', '─', '│', '├', '┤'}
)

// Box draws a box's frame and fills its inside.
func (s *Screen) Box(row, col, h, w int, st BoxStyle, frame, inside Attr) {
	s.Fill(row+1, col+1, h-2, w-2, ' ', inside)
	for x := col + 1; x < col+w-1; x++ {
		s.Put(row, x, string(st.h), frame)
		s.Put(row+h-1, x, string(st.h), frame)
	}
	for y := row + 1; y < row+h-1; y++ {
		s.Put(y, col, string(st.v), frame)
		s.Put(y, col+w-1, string(st.v), frame)
	}
	s.Put(row, col, string(st.tl), frame)
	s.Put(row, col+w-1, string(st.tr), frame)
	s.Put(row+h-1, col, string(st.bl), frame)
	s.Put(row+h-1, col+w-1, string(st.br), frame)
}

// Rule draws a line across a box at row, joined to its sides.
func (s *Screen) Rule(row, col, w int, st BoxStyle, a Attr) {
	s.Put(row, col, string(st.lj), a)
	for x := col + 1; x < col+w-1; x++ {
		s.Put(row, x, "─", a)
	}
	s.Put(row, col+w-1, string(st.rj), a)
}

// Title puts a title in a box's top line, centered: " Title ".
func (s *Screen) Title(row, col, w int, t string, a Attr) {
	t = " " + Trunc(t, w-6) + " "
	s.Put(row, col+(w-len([]rune(t)))/2, t, a)
}

// Shadow darkens the cells to the right of and below a rectangle, as a
// window's shadow on the desk.
func (s *Screen) Shadow(row, col, h, w int) {
	dark := Attr{DarkGray, Black}
	s.Paint(row+1, col+w, h, 2, dark)
	s.Paint(row+h, col+2, 1, w, dark)
}

// Render is what turns the terminal's screen, old (nil: unknown), into s:
// each changed run of cells, with its colors.  The last cell of the last
// line is left alone (writing it scrolls some terminals).
func (s *Screen) Render(old *Screen, mono bool) string {
	var b strings.Builder
	if old == nil || old.Cols != s.Cols || old.Rows != s.Rows {
		b.WriteString("\x1b[0m\x1b[2J")
		old = nil
	}
	cur := Attr{255, 255}
	for y := 0; y < s.Rows; y++ {
		last := s.Cols
		if y == s.Rows-1 {
			last--
		}
		at := -1 // where the cursor is after the last cell written (-1: unknown)
		for x := 0; x < last; x++ {
			c := s.c[y*s.Cols+x]
			if old != nil && old.c[y*s.Cols+x] == c {
				// unchanged: skipped, unless a change follows so near
				// that writing the cells costs less than moving
				if at != x || !s.changedWithin(old, y, x, last, 8) {
					continue
				}
			}
			if at != x {
				b.WriteString("\x1b[" + strconv.Itoa(y+1) + ";" + strconv.Itoa(x+1) + "H")
			}
			if c.a != cur {
				b.WriteString(sgr(c.a, mono))
				cur = c.a
			}
			r := c.r
			if r < ' ' || r == 0x7f || !utf8.ValidRune(r) {
				r = ' '
			}
			b.WriteRune(r)
			at = x + 1
		}
	}
	return b.String()
}

// changedWithin is whether a cell of row y from x, before last, within n
// of x, differs from old's.
func (s *Screen) changedWithin(old *Screen, y, x, last, n int) bool {
	for i := x; i < min(x+n, last); i++ {
		if old.c[y*s.Cols+i] != s.c[y*s.Cols+i] {
			return true
		}
	}
	return false
}

// sgr is the escape that gives a's colors (in mono: bright as bold, a light
// background as reverse).
func sgr(a Attr, mono bool) string {
	if mono {
		s := "\x1b[0"
		if a.FG >= 8 && a.FG != DarkGray {
			s += ";1"
		}
		if a.BG == LightGray || a.BG == Cyan || a.BG == Green {
			s += ";7"
		}
		return s + "m"
	}
	ansi := []int{0, 4, 2, 6, 1, 5, 3, 7} // the PC's order to ANSI's
	s := "\x1b[0"
	if a.FG >= 8 {
		s += ";1"
	}
	return s + ";" + strconv.Itoa(30+ansi[a.FG%8]) + ";" + strconv.Itoa(40+ansi[a.BG%8]) + "m"
}

// Fit is s cut or padded with spaces to exactly w runes.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

// Trunc is s cut to w runes, with "…" where it was cut.
func Trunc(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return string(r[:1])
	}
	return string(r[:w-1]) + "…"
}

// Wrap breaks s into lines of at most w runes, at spaces where it can.
func Wrap(s string, w int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, wd := range words {
			for len([]rune(wd)) > w {
				if line != "" {
					out = append(out, line)
					line = ""
				}
				r := []rune(wd)
				out = append(out, string(r[:w]))
				wd = string(r[w:])
			}
			switch {
			case line == "":
				line = wd
			case len([]rune(line))+1+len([]rune(wd)) <= w:
				line += " " + wd
			default:
				out = append(out, line)
				line = wd
			}
		}
		out = append(out, line)
	}
	return out
}

// Width is s's width in cells.
func Width(s string) int { return utf8.RuneCountInString(s) }
